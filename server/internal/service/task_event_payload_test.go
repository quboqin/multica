package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestAddTaskEventRoutingIncludesTriggerEvidenceKind(t *testing.T) {
	taskID := uuid.New()
	agentID := uuid.New()
	payload := map[string]any{}
	addTaskEventRouting(payload, db.AgentTaskQueue{
		ID:                  pgtype.UUID{Bytes: taskID, Valid: true},
		AgentID:             pgtype.UUID{Bytes: agentID, Valid: true},
		TriggerEvidenceKind: pgtype.Text{String: "creative_crawl_run_analysis", Valid: true},
	})

	if got := payload["task_id"]; got != taskID.String() {
		t.Fatalf("task_id = %v, want %s", got, taskID)
	}
	if got := payload["agent_id"]; got != agentID.String() {
		t.Fatalf("agent_id = %v, want %s", got, agentID)
	}
	if got := payload["trigger_evidence_kind"]; got != "creative_crawl_run_analysis" {
		t.Fatalf("trigger_evidence_kind = %v", got)
	}
}

func TestAddTaskEventRoutingOmitsAbsentTriggerEvidenceKind(t *testing.T) {
	payload := map[string]any{}
	addTaskEventRouting(payload, db.AgentTaskQueue{})

	if _, exists := payload["trigger_evidence_kind"]; exists {
		t.Fatal("trigger_evidence_kind should be omitted when absent")
	}
}
