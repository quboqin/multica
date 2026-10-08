package formula

import (
	"fmt"
	"strings"
	"testing"
)

func TestExpressions(t *testing.T) {
	values := map[string]any{"数量": float64(3), "单价": 12.5, "空": nil, "状态": "完成", "客户": "客户甲"}
	for _, tc := range []struct {
		expression string
		want       any
		err        string
	}{
		{"={数量} * {单价}", 37.5, ""},
		{"ROUND({单价} / 3, 2)", 4.17, ""},
		{"SUM({数量}, {单价}, {空})", 15.5, ""},
		{"AVG(2,4,6)", float64(4), ""},
		{"MIN(3,1,2) + MAX(4,9)", float64(10), ""},
		{`IF({状态} == "完成", "通过", 1/0)`, "通过", ""},
		{`IFERROR(1/0, "待填写")`, "待填写", ""},
		{`CONCAT({客户}, " / ", {数量})`, "客户甲 / 3", ""},
		{`LEN({客户})`, float64(3), ""},
		{`LOWER(TRIM(" ABC "))`, "abc", ""},
		{"AND({数量}>2, NOT(FALSE), OR(TRUE, 1/0))", true, ""},
		{"FALSE && 1/0 > 1", false, ""},
		{"ISBLANK({空})", true, ""},
		{"COALESCE({空}, 0, 1/0)", float64(0), ""},
		{`VALUE("12.5") * 2`, float64(25), ""},
		{"{空} + 1", float64(1), ""},
		{"-2 * (3+4) % 5", float64(-4), ""},
		{"{数量}/0", nil, "#DIV/0!"},
		{"{客户} * 2", nil, "#VALUE!"},
		{"1e300 * 1e300", nil, "#NUM!"},
		{"ROUND(1, 1000)", nil, "#NUM!"},
		{`CONCAT("{not a field}", "!")`, "{not a field}!", ""},
	} {
		t.Run(tc.expression, func(t *testing.T) {
			p, err := Compile(tc.expression, func(name string) (string, error) {
				if _, ok := values[name]; !ok {
					return "", fmt.Errorf("unknown field")
				}
				return name, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			v, err := p.Evaluate(func(id string) (any, error) { return values[id], nil })
			if tc.err != "" {
				if err == nil || err.Error() != tc.err {
					t.Fatalf("got %v, %v; want %s", v, err, tc.err)
				}
				return
			}
			if err != nil || v != tc.want {
				t.Fatalf("got %v, %v; want %v", v, err, tc.want)
			}
		})
	}
}

func TestRejectUnsafeAndUnboundedExpressions(t *testing.T) {
	for _, expression := range []string{"", "os.Exit(1)", "exec(1)", "SUM()", "IF(true,1)", "[]int{1}", "1<<1000", "func(){}()", "{missing}", strings.Repeat("(", 40) + "1" + strings.Repeat(")", 40), strings.Repeat("1+", 200) + "1", strings.Repeat("1", 5000)} {
		if _, err := Compile(expression, func(string) (string, error) { return "", fmt.Errorf("unknown field") }); err == nil {
			t.Errorf("accepted %q", expression)
		}
	}
}

func TestStableReferencesAndMissingDependency(t *testing.T) {
	name := `braces} and \ slashes`
	expression := Reference(name) + " + 1"
	p, err := Compile(expression, func(s string) (string, error) {
		if s != name {
			t.Fatalf("reference=%q", s)
		}
		return "stable-id", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Dependencies()) != 1 || p.Dependencies()[0] != "stable-id" {
		t.Fatal(p.Dependencies())
	}
	_, err = p.Evaluate(func(string) (any, error) { return nil, ReferenceError })
	if err != ReferenceError {
		t.Fatal(err)
	}
}
