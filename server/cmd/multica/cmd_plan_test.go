package main

import (
	"strings"
	"testing"
)

func TestValidatePlanStatus(t *testing.T) {
	for _, s := range validPlanStatuses {
		if err := validatePlanStatus(s); err != nil {
			t.Errorf("status %q should be valid, got: %v", s, err)
		}
	}
	err := validatePlanStatus("active")
	if err == nil {
		t.Fatal("status \"active\" should be rejected")
	}
	if !strings.Contains(err.Error(), "planned") {
		t.Errorf("error should list valid statuses, got: %v", err)
	}
}
