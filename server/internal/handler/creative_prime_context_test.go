package handler

import (
	"encoding/json"
	"testing"
)

func TestBindCreativeGeneratedPrimeTemplate(t *testing.T) {
	files := []map[string]string{}
	for _, role := range []string{"square_light", "square_dark", "landscape_light", "landscape_dark", "portrait_light", "portrait_dark"} {
		files = append(files, map[string]string{"role": role, "attachment_id": "attachment-" + role})
	}
	snapshot, err := json.Marshal(map[string]any{
		"prime_context_policy_version": 1,
		"market_pack":                  map[string]any{"id": "market", "config": primeTemplateSetJSON(), "files": files},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := json.RawMessage(`{"model":"gpt-image-2.5-sunburst","quality":"xhigh"}`)
	for _, tc := range []struct {
		name, size, role string
		metadata         json.RawMessage
		wantError        bool
	}{
		{"CLI receipt gets exact context role", "1080x1080", "square_dark", metadata, false},
		{"missing input evidence", "1080x1080", "", metadata, true},
		{"wrong size", "1200x628", "square_dark", metadata, true},
		{"unapproved role", "1080x1080", "invented", metadata, true},
		{"cannot switch family", "1080x1080", "square_dark", json.RawMessage(`{"prime_template_source_role":"square_light"}`), true},
		{"metadata alone cannot replace context evidence", "1080x1080", "", json.RawMessage(`{"prime_template_source_role":"square_dark"}`), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bindCreativeGeneratedPrimeTemplate(snapshot, tc.size, tc.metadata, tc.role)
			if (err != nil) != tc.wantError {
				t.Fatalf("result=%s error=%v", got, err)
			}
			if err == nil {
				var fields map[string]any
				if err := json.Unmarshal(got, &fields); err != nil {
					t.Fatal(err)
				}
				if fields["prime_template_source_role"] != tc.role || fields["quality"] != "xhigh" {
					t.Fatalf("incorrect normalized metadata: %s", got)
				}
			}
		})
	}
	got, err := bindCreativeGeneratedPrimeTemplate(json.RawMessage(`{}`), "1080x1080", metadata, "")
	if err != nil || string(got) != string(metadata) {
		t.Fatalf("legacy receipt changed: %s %v", got, err)
	}
}
