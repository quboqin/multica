package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestNormalizeDirectTaskContext(t *testing.T) {
	t.Run("adds stable item key", func(t *testing.T) {
		raw, err := normalizeDirectTaskContext(json.RawMessage(`{"type":"creative_analysis","input":{"material_id":"m-1"}}`), "material-1")
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatal(err)
		}
		if got["item_key"] != "material-1" || got["type"] != "creative_analysis" {
			t.Fatalf("context = %#v", got)
		}
	})

	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"input":{}}`),
		json.RawMessage(`{"type":"quick_create"}`),
		json.RawMessage(`{"type":"creative_analysis","item_key":"other"}`),
		json.RawMessage(`[]`),
	} {
		if _, err := normalizeDirectTaskContext(raw, "item-1"); err == nil {
			t.Fatalf("normalizeDirectTaskContext(%s) succeeded", raw)
		}
	}
}

func TestDirectTaskMatchesAllowsDomainDirectTasks(t *testing.T) {
	evidence := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	task := db.AgentTaskQueue{
		Status:               "running",
		Context:              []byte(`{"type":"creative_production","item_key":"v01"}`),
		TriggerEvidenceKind:  pgtype.Text{String: "creative_variant", Valid: true},
		TriggerEvidenceRefID: evidence,
	}
	if !directTaskMatches(task, "creative_variant", evidence, "active") {
		t.Fatal("domain direct task must be active-matchable")
	}
	if got := directTaskItemKey(task); got != "v01" {
		t.Fatalf("item key = %q", got)
	}
	task.IssueID = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	if directTaskMatches(task, "creative_variant", evidence, "active") {
		t.Fatal("issue task must not match direct-task source operations")
	}
}

func TestLatestRetryableFailedDirectTasksUsesCurrentItemStateAndAttemptBudget(t *testing.T) {
	createdAt := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	task := func(id byte, itemKey, status string, offset time.Duration, attempt, maxAttempts int32) db.AgentTaskQueue {
		return db.AgentTaskQueue{
			ID:            pgtype.UUID{Bytes: [16]byte{15: id}, Valid: true},
			Status:        status,
			Context:       []byte(`{"type":"creative_domain_task","item_key":"` + itemKey + `"}`),
			CreatedAt:     pgtype.Timestamptz{Time: createdAt.Add(offset), Valid: true},
			Attempt:       attempt,
			MaxAttempts:   maxAttempts,
			FailureReason: pgtype.Text{String: "provider_rate_limited", Valid: status == "failed"},
		}
	}

	open := task(1, "open", "failed", 5*time.Minute, 1, 2)
	tasks := []db.AgentTaskQueue{
		open,
		task(3, "queued-recovery", "queued", 4*time.Minute, 2, 2),
		task(2, "queued-recovery", "failed", time.Minute, 1, 2),
		task(4, "running-recovery", "failed", time.Minute, 1, 2),
		task(5, "running-recovery", "running", 4*time.Minute, 2, 2),
		task(7, "completed-recovery", "completed", 4*time.Minute, 2, 2),
		task(6, "completed-recovery", "failed", time.Minute, 1, 2),
		task(8, "exhausted", "failed", 6*time.Minute, 2, 2),
	}

	got := latestRetryableFailedDirectTasks(tasks)
	if len(got) != 1 || got[0].ID != open.ID {
		t.Fatalf("latest retryable failures = %#v, want only provider-rate-limited open failure", got)
	}
}
