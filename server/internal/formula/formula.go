// Package formula evaluates bounded, deterministic expressions over one row.
// It interprets an allowlisted AST; it never executes code or accesses I/O.
package formula

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxExpressionBytes = 4096

type Config struct {
	Expression string            `json:"expression"`
	Bindings   map[string]string `json:"bindings,omitempty"`
}

type Program struct {
	root ast.Expr
	refs map[string]string
}

type Error string

func (e Error) Error() string { return string(e) }

const (
	ReferenceError Error = "#REF!"
	ValueError     Error = "#VALUE!"
	DivisionError  Error = "#DIV/0!"
	NumberError    Error = "#NUM!"
)

var arities = map[string][2]int{
	"IF": {3, 3}, "IFERROR": {2, 2}, "AND": {1, 50}, "OR": {1, 50}, "NOT": {1, 1},
	"SUM": {1, 50}, "AVG": {1, 50}, "MIN": {1, 50}, "MAX": {1, 50},
	"ROUND": {1, 2}, "ABS": {1, 1}, "CEIL": {1, 1}, "FLOOR": {1, 1},
	"CONCAT": {1, 50}, "LEN": {1, 1}, "LOWER": {1, 1}, "UPPER": {1, 1}, "TRIM": {1, 1},
	"ISBLANK": {1, 1}, "COALESCE": {1, 50}, "VALUE": {1, 1},
}

// RewriteReferences visits field references outside quoted strings. Backslash
// escapes allow field names containing braces or backslashes.
func RewriteReferences(expression string, resolve func(string) (string, error)) (string, error) {
	var out strings.Builder
	for i := 0; i < len(expression); {
		c := expression[i]
		if c == '"' || c == '\'' {
			start := i
			i++
			closed := false
			for i < len(expression) {
				if expression[i] == '\\' {
					i += 2
					continue
				}
				if expression[i] == c {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "", fmt.Errorf("unterminated string")
			}
			out.WriteString(expression[start:i])
			continue
		}
		if c != '{' {
			out.WriteByte(c)
			i++
			continue
		}
		i++
		var ref strings.Builder
		for i < len(expression) && expression[i] != '}' {
			if expression[i] == '\\' {
				i++
				if i == len(expression) {
					return "", fmt.Errorf("unterminated field reference")
				}
			}
			ref.WriteByte(expression[i])
			i++
		}
		if i == len(expression) {
			return "", fmt.Errorf("unterminated field reference")
		}
		i++
		value, err := resolve(ref.String())
		if err != nil {
			return "", err
		}
		out.WriteString(value)
	}
	return out.String(), nil
}

func Reference(name string) string {
	return "{" + strings.NewReplacer("\\", "\\\\", "}", "\\}").Replace(name) + "}"
}

// Compile resolves every field to a stable identity and rejects unsupported
// syntax before a formula can be saved. An optional leading '=' is accepted.
func Compile(expression string, resolve func(string) (string, error)) (*Program, error) {
	if len(expression) > MaxExpressionBytes {
		return nil, fmt.Errorf("formula exceeds %d bytes", MaxExpressionBytes)
	}
	expression = strings.TrimSpace(expression)
	expression = strings.TrimPrefix(expression, "=")
	p := &Program{refs: map[string]string{}}
	code, err := RewriteReferences(expression, func(name string) (string, error) {
		id, err := resolve(name)
		if err != nil {
			return "", err
		}
		key := fmt.Sprintf("__field%d", len(p.refs))
		p.refs[key] = id
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	p.root, err = parser.ParseExpr(code)
	if err != nil {
		return nil, fmt.Errorf("invalid formula: %w", err)
	}
	nodes := 0
	var validate func(ast.Expr, int) error
	validate = func(expr ast.Expr, depth int) error {
		nodes++
		if nodes > 256 || depth > 32 {
			return fmt.Errorf("formula is too complex")
		}
		switch n := expr.(type) {
		case *ast.BasicLit:
			if n.Kind != token.INT && n.Kind != token.FLOAT && n.Kind != token.STRING {
				return fmt.Errorf("use numbers or double-quoted strings")
			}
		case *ast.Ident:
			if _, ok := p.refs[n.Name]; !ok && n.Name != "true" && n.Name != "false" && n.Name != "TRUE" && n.Name != "FALSE" && n.Name != "null" && n.Name != "NULL" {
				return fmt.Errorf("unknown name %q; reference fields with {field name}", n.Name)
			}
		case *ast.ParenExpr:
			return validate(n.X, depth+1)
		case *ast.UnaryExpr:
			if n.Op != token.ADD && n.Op != token.SUB && n.Op != token.NOT {
				return fmt.Errorf("unsupported operator %s", n.Op)
			}
			return validate(n.X, depth+1)
		case *ast.BinaryExpr:
			switch n.Op {
			case token.ADD, token.SUB, token.MUL, token.QUO, token.REM, token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ, token.LAND, token.LOR:
			default:
				return fmt.Errorf("unsupported operator %s", n.Op)
			}
			if err := validate(n.X, depth+1); err != nil {
				return err
			}
			return validate(n.Y, depth+1)
		case *ast.CallExpr:
			name, ok := n.Fun.(*ast.Ident)
			if !ok || n.Ellipsis.IsValid() {
				return fmt.Errorf("unsupported function")
			}
			bounds, ok := arities[strings.ToUpper(name.Name)]
			if !ok {
				return fmt.Errorf("unknown function %q", name.Name)
			}
			if len(n.Args) < bounds[0] || len(n.Args) > bounds[1] {
				return fmt.Errorf("%s expects %d to %d arguments", name.Name, bounds[0], bounds[1])
			}
			for _, arg := range n.Args {
				if err := validate(arg, depth+1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported formula syntax")
		}
		return nil
	}
	if err := validate(p.root, 0); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Program) Dependencies() []string {
	ids := make([]string, 0, len(p.refs))
	for _, id := range p.refs {
		ids = append(ids, id)
	}
	return ids
}

// Evaluate returns a JSON scalar or nil. Field errors propagate, except through
// lazy IF/IFERROR branches. Both expression size and result size are bounded.
func (p *Program) Evaluate(resolve func(string) (any, error)) (any, error) {
	return p.eval(p.root, resolve)
}

func number(v any) (float64, error) {
	if v == nil {
		return 0, nil
	}
	if n, ok := v.(float64); ok {
		return n, nil
	}
	return 0, ValueError
}

func truth(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	default:
		return false
	}
}

func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func bounded(v any) (any, error) {
	switch v.(type) {
	case nil, bool, string, float64:
	default:
		return nil, ValueError
	}
	if n, ok := v.(float64); ok && (math.IsInf(n, 0) || math.IsNaN(n)) {
		return nil, NumberError
	}
	if s, ok := v.(string); ok && len(s) > 8192 {
		return nil, ValueError
	}
	return v, nil
}

func (p *Program) eval(expr ast.Expr, resolve func(string) (any, error)) (any, error) {
	switch n := expr.(type) {
	case *ast.BasicLit:
		if n.Kind == token.STRING {
			value, err := strconv.Unquote(n.Value)
			if err != nil {
				return nil, ValueError
			}
			return bounded(value)
		}
		value, err := strconv.ParseFloat(n.Value, 64)
		if err != nil {
			return nil, NumberError
		}
		return bounded(value)
	case *ast.Ident:
		if id, ok := p.refs[n.Name]; ok {
			value, err := resolve(id)
			if err != nil {
				return nil, err
			}
			return bounded(value)
		}
		switch strings.ToUpper(n.Name) {
		case "TRUE":
			return true, nil
		case "FALSE":
			return false, nil
		default:
			return nil, nil
		}
	case *ast.ParenExpr:
		return p.eval(n.X, resolve)
	case *ast.UnaryExpr:
		v, err := p.eval(n.X, resolve)
		if err != nil {
			return nil, err
		}
		if n.Op == token.NOT {
			return !truth(v), nil
		}
		num, err := number(v)
		if err != nil {
			return nil, err
		}
		if n.Op == token.SUB {
			num = -num
		}
		return bounded(num)
	case *ast.BinaryExpr:
		a, err := p.eval(n.X, resolve)
		if err != nil {
			return nil, err
		}
		if n.Op == token.LAND && !truth(a) {
			return false, nil
		}
		if n.Op == token.LOR && truth(a) {
			return true, nil
		}
		b, err := p.eval(n.Y, resolve)
		if err != nil {
			return nil, err
		}
		if n.Op == token.LAND || n.Op == token.LOR {
			return truth(b), nil
		}
		if n.Op == token.EQL {
			return a == b, nil
		}
		if n.Op == token.NEQ {
			return a != b, nil
		}
		if aText, ok := a.(string); ok {
			bText, ok := b.(string)
			if !ok {
				return nil, ValueError
			}
			switch n.Op {
			case token.LSS:
				return aText < bText, nil
			case token.LEQ:
				return aText <= bText, nil
			case token.GTR:
				return aText > bText, nil
			case token.GEQ:
				return aText >= bText, nil
			}
		}
		x, err := number(a)
		if err != nil {
			return nil, err
		}
		y, err := number(b)
		if err != nil {
			return nil, err
		}
		switch n.Op {
		case token.ADD:
			return bounded(x + y)
		case token.SUB:
			return bounded(x - y)
		case token.MUL:
			return bounded(x * y)
		case token.QUO:
			if y == 0 {
				return nil, DivisionError
			}
			return bounded(x / y)
		case token.REM:
			if y == 0 {
				return nil, DivisionError
			}
			return bounded(math.Mod(x, y))
		case token.LSS:
			return x < y, nil
		case token.LEQ:
			return x <= y, nil
		case token.GTR:
			return x > y, nil
		case token.GEQ:
			return x >= y, nil
		}
	case *ast.CallExpr:
		name := strings.ToUpper(n.Fun.(*ast.Ident).Name)
		if name == "IF" {
			v, err := p.eval(n.Args[0], resolve)
			if err != nil {
				return nil, err
			}
			if truth(v) {
				return p.eval(n.Args[1], resolve)
			}
			return p.eval(n.Args[2], resolve)
		}
		if name == "IFERROR" {
			v, err := p.eval(n.Args[0], resolve)
			if err == nil {
				return v, nil
			}
			return p.eval(n.Args[1], resolve)
		}
		values := make([]any, 0, len(n.Args))
		for _, arg := range n.Args {
			v, err := p.eval(arg, resolve)
			if err != nil {
				return nil, err
			}
			if name == "AND" && !truth(v) {
				return false, nil
			}
			if name == "OR" && truth(v) {
				return true, nil
			}
			if name == "COALESCE" && v != nil && v != "" {
				return v, nil
			}
			values = append(values, v)
		}
		switch name {
		case "AND":
			return true, nil
		case "OR":
			return false, nil
		case "COALESCE":
			return nil, nil
		case "NOT":
			return !truth(values[0]), nil
		case "ISBLANK":
			return values[0] == nil || values[0] == "", nil
		case "CONCAT":
			var s strings.Builder
			for _, v := range values {
				s.WriteString(text(v))
				if s.Len() > 8192 {
					return nil, ValueError
				}
			}
			return s.String(), nil
		case "LEN":
			return float64(utf8.RuneCountInString(text(values[0]))), nil
		case "LOWER":
			return bounded(strings.ToLower(text(values[0])))
		case "UPPER":
			return bounded(strings.ToUpper(text(values[0])))
		case "TRIM":
			return strings.TrimSpace(text(values[0])), nil
		case "VALUE":
			v, err := strconv.ParseFloat(strings.TrimSpace(text(values[0])), 64)
			if err != nil {
				return nil, ValueError
			}
			return bounded(v)
		}
		numbers := make([]float64, len(values))
		for i, v := range values {
			value, err := number(v)
			if err != nil {
				return nil, err
			}
			numbers[i] = value
		}
		x := numbers[0]
		switch name {
		case "ABS":
			return bounded(math.Abs(x))
		case "CEIL":
			return bounded(math.Ceil(x))
		case "FLOOR":
			return bounded(math.Floor(x))
		case "ROUND":
			digits := 0.0
			if len(numbers) == 2 {
				digits = numbers[1]
			}
			if digits != math.Trunc(digits) || math.Abs(digits) > 10 {
				return nil, NumberError
			}
			factor := math.Pow10(int(digits))
			return bounded(math.Round(x*factor) / factor)
		case "SUM", "AVG":
			sum := 0.0
			for _, v := range numbers {
				sum += v
			}
			if name == "AVG" {
				sum /= float64(len(numbers))
			}
			return bounded(sum)
		case "MIN", "MAX":
			for _, v := range numbers[1:] {
				if name == "MIN" {
					x = math.Min(x, v)
				} else {
					x = math.Max(x, v)
				}
			}
			return bounded(x)
		}
	}
	return nil, ValueError
}
