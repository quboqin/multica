package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/fields"
	"github.com/multica-ai/multica/server/internal/formula"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Formula references bind to field IDs, so renaming a source never changes
// the meaning of a stored formula. Only scalar fields in this table are inputs.
func formulaSourceType(t string) bool {
	switch t {
	case "text", "number", "date", "url", "checkbox", "select", fields.TypeFormula:
		return true
	}
	return false
}

// Present current labels while retaining stable IDs, including when a source
// has been archived. A literal string that contains braces is never rewritten.
func formulaDisplayConfig(raw []byte, catalog []db.CollectionField) []byte {
	cfg := parsePropertyConfig(raw)
	if cfg.Formula == nil {
		return raw
	}
	names := map[string]string{}
	for _, f := range catalog {
		names[uuidToString(f.ID)] = f.Name
	}
	bindings := map[string]string{}
	expression, err := formula.RewriteReferences(cfg.Formula.Expression, func(old string) (string, error) {
		id := cfg.Formula.Bindings[old]
		name := names[id]
		if id == "title" {
			name = "title"
		} else if name == "title" {
			name = id
		}
		if name == "" {
			name = id
		}
		if name == "" {
			name = old
		}
		if bound, exists := bindings[name]; exists && bound != id {
			name = id
		}
		bindings[name] = id
		return formula.Reference(name), nil
	})
	if err != nil {
		return raw
	}
	cfg.Formula = &formula.Config{Expression: expression, Bindings: bindings}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return encoded
}

func validateFormulaConfig(catalog []db.CollectionField, collection db.Collection, fieldID string, cfg *PropertyConfig) ([]byte, error) {
	if cfg == nil || cfg.Formula == nil {
		return nil, fmt.Errorf("formula fields require config.formula.expression")
	}
	if len(cfg.Options) > 0 || cfg.Relation != nil {
		return nil, fmt.Errorf("formula fields cannot have options or relation targets")
	}
	bindings := map[string]string{}
	expression := strings.TrimSpace(cfg.Formula.Expression)
	p, err := formula.Compile(expression, func(name string) (string, error) {
		id := cfg.Formula.Bindings[name]
		if id == "title" || id == "" && (name == "title" || collection.TitleName != "" && name == collection.TitleName) {
			bindings[name] = "title"
			return "title", nil
		}
		for _, f := range catalog {
			fid := uuidToString(f.ID)
			if id != "" && fid == id || id == "" && (name == f.Name || name == fid) {
				if !formulaSourceType(f.Type) {
					return "", fmt.Errorf("field %q cannot be used in a formula", name)
				}
				bindings[name] = fid
				return fid, nil
			}
		}
		return "", fmt.Errorf("unknown or archived field %q", name)
	})
	if err != nil {
		return nil, err
	}
	graph := map[string][]string{fieldID: p.Dependencies()}
	for _, f := range catalog {
		id := uuidToString(f.ID)
		if f.Type != fields.TypeFormula || id == fieldID {
			continue
		}
		stored := parsePropertyConfig(f.Config).Formula
		if stored != nil {
			for _, dep := range stored.Bindings {
				graph[id] = append(graph[id], dep)
			}
		}
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("formula contains a circular reference")
		}
		if done[id] {
			return nil
		}
		visiting[id] = true
		for _, dep := range graph[id] {
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		done[id] = true
		return nil
	}
	if err := visit(fieldID); err != nil {
		return nil, err
	}
	return json.Marshal(PropertyConfig{Formula: &formula.Config{Expression: expression, Bindings: bindings}})
}

type collectionFormulaEvaluator struct {
	fields   map[string]db.CollectionField
	programs map[string]*formula.Program
	invalid  map[string]bool
}

func newCollectionFormulaEvaluator(catalog []db.CollectionField) *collectionFormulaEvaluator {
	e := &collectionFormulaEvaluator{fields: map[string]db.CollectionField{}, programs: map[string]*formula.Program{}, invalid: map[string]bool{}}
	for _, f := range catalog {
		id := uuidToString(f.ID)
		e.fields[id] = f
		if f.Type != fields.TypeFormula {
			continue
		}
		cfg := parsePropertyConfig(f.Config).Formula
		if cfg == nil {
			e.invalid[id] = true
			continue
		}
		p, err := formula.Compile(cfg.Expression, func(name string) (string, error) {
			id := cfg.Bindings[name]
			if id == "" {
				return "", formula.ReferenceError
			}
			return id, nil
		})
		if err != nil {
			e.invalid[id] = true
		} else {
			e.programs[id] = p
		}
	}
	return e
}

// apply projects computed values into the response only. Raw record.fields
// remains user-entered data; bulk writes/imports need no recalculation jobs.
func (e *collectionFormulaEvaluator) apply(response map[string]any) {
	if len(e.programs)+len(e.invalid) == 0 {
		return
	}
	values := map[string]any{}
	if raw, ok := response["fields"].(json.RawMessage); ok {
		_ = json.Unmarshal(raw, &values)
	}
	if values == nil {
		values = map[string]any{}
	}
	errors := map[string]string{}
	visiting, done := map[string]bool{}, map[string]bool{}
	var resolve func(string) (any, error)
	resolve = func(id string) (any, error) {
		if id == "title" {
			return response["title"], nil
		}
		f, exists := e.fields[id]
		if !exists || !formulaSourceType(f.Type) {
			return nil, formula.ReferenceError
		}
		if f.Type != fields.TypeFormula {
			v := values[id]
			if f.Type == "select" && v != nil {
				for _, o := range parsePropertyConfig(f.Config).Options {
					if o.ID == v {
						return o.Name, nil
					}
				}
				return nil, nil
			}
			return v, nil
		}
		if done[id] {
			if message := errors[id]; message != "" {
				return nil, formula.Error(message)
			}
			return values[id], nil
		}
		if visiting[id] || e.invalid[id] {
			return nil, formula.ReferenceError
		}
		visiting[id] = true
		v, err := e.programs[id].Evaluate(resolve)
		visiting[id] = false
		done[id] = true
		if err != nil {
			values[id] = nil
			errors[id] = err.Error()
			return nil, err
		}
		values[id] = v
		return v, nil
	}
	for id, f := range e.fields {
		if f.Type == fields.TypeFormula {
			if _, err := resolve(id); err != nil {
				values[id] = nil
				errors[id] = err.Error()
			}
		}
	}
	response["fields"] = values
	response["formula_errors"] = errors
}

func (h *Handler) attachFormulaValues(ctx context.Context, q *db.Queries, workspaceID, collectionID pgtype.UUID, responses []map[string]any) error {
	catalog, err := q.ListCollectionFields(ctx, db.ListCollectionFieldsParams{WorkspaceID: workspaceID, CollectionID: collectionID})
	if err != nil {
		return err
	}
	e := newCollectionFormulaEvaluator(catalog)
	for _, response := range responses {
		e.apply(response)
	}
	return nil
}
