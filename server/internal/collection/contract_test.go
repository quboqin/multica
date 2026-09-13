package collection

import (
	"encoding/json"
	"testing"
)

func TestNormalizeCollectionCanonicalizesAndFingerprints(t *testing.T) {
	first, err := NormalizeCollection("  Leads  ", []FieldInput{{Name: " Note ", Type: "text"}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NormalizeCollection("Leads", []FieldInput{{Name: "Note", Type: "text"}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint != second.Fingerprint || first.Name != "Leads" || first.Fields[0].Name != "Note" {
		t.Fatalf("normalization mismatch: %#v %#v", first, second)
	}
}

func TestNormalizeCollectionRejectsDuplicateAndUnknownFields(t *testing.T) {
	for _, fields := range [][]FieldInput{
		{{Name: "Note", Type: "text"}, {Name: "note", Type: "number"}},
		{{Name: "Owner", Type: "actor"}},
	} {
		if _, err := NormalizeCollection("Leads", fields); err == nil {
			t.Fatalf("expected rejection for %#v", fields)
		}
	}
}

func TestNormalizeRecordPreservesFalsyValues(t *testing.T) {
	spec, err := NormalizeRecord("", map[string]json.RawMessage{
		"text":  json.RawMessage(`""`),
		"count": json.RawMessage(`0`),
		"done":  json.RawMessage(`false`),
	}, []FieldDefinition{{ID: "text", Type: "text"}, {ID: "count", Type: "number"}, {ID: "done", Type: "checkbox"}})
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(spec.Fields, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["text"] != "" || fields["count"] != float64(0) || fields["done"] != false {
		t.Fatalf("falsy values were not preserved: %#v", fields)
	}
}

func TestNormalizeRecordRejectsNullAndWrongJSONTypes(t *testing.T) {
	definitions := []FieldDefinition{
		{ID: "text", Type: "text"},
		{ID: "count", Type: "number"},
		{ID: "done", Type: "checkbox"},
	}
	for _, test := range []struct {
		name  string
		field string
		value string
	}{
		{name: "text null", field: "text", value: `null`},
		{name: "number null", field: "count", value: `null`},
		{name: "checkbox null", field: "done", value: `null`},
		{name: "text wrong type", field: "text", value: `42`},
		{name: "number wrong type", field: "count", value: `"42"`},
		{name: "checkbox wrong type", field: "done", value: `0`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NormalizeRecord("", map[string]json.RawMessage{
				test.field: json.RawMessage(test.value),
			}, definitions); err == nil {
				t.Fatal("expected invalid JSON field value to be rejected")
			}
		})
	}
}

func TestDecodeTitleValueDistinguishesNullFromEmptyText(t *testing.T) {
	if _, err := DecodeTitleValue(json.RawMessage(`null`)); err == nil {
		t.Fatal("JSON null was accepted as an empty title")
	}
	if _, err := DecodeTitleValue(nil); err == nil {
		t.Fatal("missing title value was accepted")
	}
	if _, err := DecodeTitleValue(json.RawMessage(`false`)); err == nil {
		t.Fatal("wrong title type was accepted")
	}
	if value, err := DecodeTitleValue(json.RawMessage(`""`)); err != nil || value != "" {
		t.Fatalf("valid empty title = %q, %v", value, err)
	}
}

func TestCursorIsBoundToSourceAndLimit(t *testing.T) {
	encoded := EncodeCursor(PageCursor{Version: 1, WorkspaceID: "ws-a", CollectionID: "c-a", Query: "created_at_asc", Limit: 50, LastCreatedAt: "2026-01-01T00:00:00Z", LastID: "r-a"})
	if _, err := DecodeCursor(encoded, "ws-a", "c-a", 50); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		workspaceID, collectionID string
		limit                     int
	}{{"ws-b", "c-a", 50}, {"ws-a", "c-b", 50}, {"ws-a", "c-a", 100}} {
		if _, err := DecodeCursor(encoded, input.workspaceID, input.collectionID, input.limit); err == nil {
			t.Fatalf("expected cursor mismatch for %#v", input)
		}
	}
}
