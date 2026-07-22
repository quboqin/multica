package broker

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateSafeJSONRejectsSensitiveKeys(t *testing.T) {
	raw := json.RawMessage(`{"filters":{"storage_state":"x"}}`)
	err := ValidateSafeJSON(raw)
	if !errors.Is(err, ErrUnsafeParams) {
		t.Fatalf("expected ErrUnsafeParams, got %v", err)
	}
}

func TestValidateSafeJSONAllowsBusinessFilters(t *testing.T) {
	raw := json.RawMessage(`{"keyword":"loan","date_range":{"from":"2026-01-01","to":"2026-01-31"},"limit":20}`)
	if err := ValidateSafeJSON(raw); err != nil {
		t.Fatalf("ValidateSafeJSON: %v", err)
	}
}
