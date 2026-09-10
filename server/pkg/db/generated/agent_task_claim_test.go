package db

import (
	"strings"
	"testing"
)

func TestClaimAgentTaskOnlySerializesActualQuickCreate(t *testing.T) {
	if !strings.Contains(claimAgentTask, "COALESCE(atq.context ->> 'type', '') = 'quick_create'") {
		t.Fatal("claim query must identify queued quick-create tasks by context.type")
	}
	if !strings.Contains(claimAgentTask, "COALESCE(active.context ->> 'type', '') = 'quick_create'") {
		t.Fatal("claim query must only serialize against active quick-create tasks")
	}
}
