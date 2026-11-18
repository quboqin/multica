package fields

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmailValue(t *testing.T) {
	for _, value := range []string{"Person+tag@example.com", " user@example.com "} {
		raw, _ := json.Marshal(value)
		actual, err := ValidateValue(Definition{Type: TypeEmail}, raw)
		if err != nil || string(actual) != `"`+strings.TrimSpace(value)+`"` {
			t.Fatalf("email %q: %s, %v", value, actual, err)
		}
	}
	for _, value := range []any{"", "missing-at", "Name <user@example.com>", "a@example.com,b@example.com", "a@example.com\r\nBcc: b@example.com", strings.Repeat("a", 255) + "@example.com", 42, []string{"a@example.com"}, nil} {
		raw, _ := json.Marshal(value)
		if _, err := ValidateValue(Definition{Type: TypeEmail}, raw); err == nil {
			t.Errorf("accepted invalid email %q", raw)
		}
	}
}

func TestListFieldsUseIssuePropertyValidation(t *testing.T) {
	for _, tc := range []struct{ kind, raw, want string }{
		{"multi_text", `["one","two","one"]`, `["one","two"]`},
		{"multi_url", `[" https://example.com/a ","https://example.com/b"]`, `["https://example.com/a","https://example.com/b"]`},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			got, err := ValidateValue(Definition{Type: tc.kind}, json.RawMessage(tc.raw))
			if err != nil || string(got) != tc.want {
				t.Fatalf("ValidateValue = %s, %v; want %s", got, err, tc.want)
			}
		})
	}
}
