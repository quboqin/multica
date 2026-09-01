package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type creativeCandidateOrchestrationFixture struct {
	OrderID string
	ItemID  string
	IssueID string
	Squad   creativeOrderSquadFixture
}

func createCreativeCandidateOrchestrationFixture(t *testing.T, title string) creativeCandidateOrchestrationFixture {
	t.Helper()
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, title)
	squad := createCreativeOrderSquadFixture(t, "", "", true)
	snapshot, err := json.Marshal(map[string]any{
		"pipeline_version": creativePipelineCandidateV1,
		"expected_sizes":   standardCreativeAssetSizes,
		"squad_snapshot": map[string]any{
			"squad_id":          squad.SquadID,
			"leader_agent_id":   squad.LeaderAgentID,
			"planner_agent_id":  squad.PlannerAgentID,
			"producer_agent_id": squad.ProducerAgentID,
			"reviewer_agent_id": squad.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot = $2::jsonb WHERE id = $1`, orderID, snapshot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, itemID)
	})
	return creativeCandidateOrchestrationFixture{OrderID: orderID, ItemID: itemID, IssueID: issueID, Squad: squad}
}

func createCreativeCandidateOrchestrationVariant(
	t *testing.T,
	itemID, key, state string,
	rank any,
	status, primarySize string,
	expectedSizes []string,
) string {
	t.Helper()
	var variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, brief, revision, status, candidate_state, selection_rank, primary_size
)
VALUES ($1, $2, '{}'::jsonb, 1, $3, $4, $5, $6)
RETURNING id::text
`, itemID, key, status, state, rank, primarySize).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, $2, $3::text[])
`, variantID, status, expectedSizes); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET staging_revision = 1 WHERE id = $1`, variantID); err != nil {
		t.Fatal(err)
	}
	return variantID
}

func addCreativeCandidateOrchestrationAsset(t *testing.T, variantID, size, stage string) string {
	t.Helper()
	attachmentID := createCreativeOrderAssetAttachment(t, "orchestration-"+stage+"-"+size+".png")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, $3, $4, 'completed')
`, variantID, size, stage, attachmentID); err != nil {
		t.Fatal(err)
	}
	return attachmentID
}

func addCreativeCandidateOrchestrationProductionTask(
	t *testing.T,
	fixture creativeCandidateOrchestrationFixture,
	variantID, status, phase string,
) db.AgentTaskQueue {
	t.Helper()
	contextValue, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"creative_order_id":      fixture.OrderID,
		"creative_order_item_id": fixture.ItemID,
		"variant_id":             variantID,
		"revision":               1,
		"item_key":               variantID + ":r1",
		"expected_sizes":         standardCreativeAssetSizes,
		"production_phase":       phase,
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID pgtype.UUID
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context,
  completed_at, requesting_user_id, originator_user_id, accountable_user_id, originator_source
)
VALUES (
  $1, (SELECT runtime_id FROM agent WHERE id = $1), $2,
  'creative_order_item_production', $3, $4::jsonb,
  CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() ELSE NULL END,
  $5, $5, $5, 'direct_human'
)
RETURNING id
`, fixture.Squad.ProducerAgentID, status, fixture.ItemID, contextValue, testUserID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	task, err := testHandler.Queries.GetAgentTask(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func prepareCreativeCandidateSelectionTask(t *testing.T, title string) (creativeCandidateOrchestrationFixture, db.AgentTaskQueue, []string) {
	t.Helper()
	fixture := createCreativeCandidateOrchestrationFixture(t, title)
	variantIDs := make([]string, 0, 4)
	for index := 1; index <= 4; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "completed", "1080x1080", []string{"1080x1080"},
		)
		variantIDs = append(variantIDs, variantID)
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, "completed", "candidate_primary")
	}
	queued, err := testHandler.maybeQueueCreativeCandidateSelection(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)},
	)
	if err != nil || !queued {
		t.Fatalf("queue candidate selection = %v, err %v", queued, err)
	}
	var taskID pgtype.UUID
	if err := testPool.QueryRow(t.Context(), `
UPDATE agent_task_queue
SET status = 'running', started_at = now()
WHERE id = (
  SELECT id FROM agent_task_queue
  WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
  ORDER BY created_at DESC, id DESC LIMIT 1
)
RETURNING id
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	task, err := testHandler.Queries.GetAgentTask(t.Context(), taskID)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, task, variantIDs
}

func TestCreativeCandidateSelectionQueuesFourReadyAfterOneTerminalFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate comparison orchestration")
	var terminalTask db.AgentTaskQueue
	for index := 1; index <= 5; index++ {
		status := "completed"
		if index == 5 {
			status = "action_required"
		}
		variantID := createCreativeCandidateOrchestrationVariant(
			t,
			fixture.ItemID,
			fmt.Sprintf("C%02d", index),
			"candidate",
			nil,
			status,
			"1080x1080",
			[]string{"1080x1080"},
		)
		taskStatus := "completed"
		if index == 5 {
			taskStatus = "failed"
		} else {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		}
		task := addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, taskStatus, "candidate_primary")
		if index == 5 {
			terminalTask = task
		}
	}

	if err := testHandler.reconcileCreativeCandidateOrchestrationForProductionTask(t.Context(), terminalTask); err != nil {
		t.Fatal(err)
	}
	var taskCount, candidateCount int
	var workflow, itemKey string
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*), max(context->>'workflow'), max(context->>'item_key'),
       max(jsonb_array_length(context->'candidates'))
FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskCount, &workflow, &itemKey, &candidateCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || workflow != creativeCandidateSelectionWorkflow || itemKey != creativeCandidateSelectionItemKey || candidateCount != 4 {
		t.Fatalf("candidate comparison task = count %d workflow %q key %q candidates %d", taskCount, workflow, itemKey, candidateCount)
	}
	var taskIssueID pgtype.UUID
	var contextIssueID string
	if err := testPool.QueryRow(t.Context(), `
SELECT issue_id, context->>'issue_id'
FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskIssueID, &contextIssueID); err != nil {
		t.Fatal(err)
	}
	if taskIssueID.Valid {
		t.Fatalf("candidate comparison task issue_id = %s, want NULL", uuidToString(taskIssueID))
	}
	if contextIssueID != fixture.IssueID {
		t.Fatalf("candidate comparison context issue_id = %q, want %q", contextIssueID, fixture.IssueID)
	}
	var rejectedCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_order_variant
WHERE order_item_id = $1 AND candidate_state = 'rejected' AND status = 'action_required'
`, fixture.ItemID).Scan(&rejectedCount); err != nil {
		t.Fatal(err)
	}
	if rejectedCount != 1 {
		t.Fatalf("automatically rejected candidates = %d, want 1", rejectedCount)
	}
	if err := testHandler.reconcileCreativeCandidateOrchestrationForProductionTask(t.Context(), terminalTask); err != nil {
		t.Fatalf("idempotent failed-task reconciliation: %v", err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("candidate comparison tasks after replay = %d, want 1", taskCount)
	}
}

func TestCreativeCandidateSelectionsForSameIssueQueueIndependently(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate comparison parallel queueing")
	var secondCandidateID, secondItemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, 'parallel candidate', 'image', 'https://example.test/creative.png', '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, "candidate-selection-parallel-"+uuid.NewString()).Scan(&secondCandidateID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'running')
RETURNING id::text
`, fixture.OrderID, secondCandidateID).Scan(&secondItemID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, secondItemID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order_item WHERE id = $1`, secondItemID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, secondCandidateID)
	})
	secondFixture := fixture
	secondFixture.ItemID = secondItemID
	for _, current := range []creativeCandidateOrchestrationFixture{fixture, secondFixture} {
		for index := 1; index <= 4; index++ {
			variantID := createCreativeCandidateOrchestrationVariant(
				t, current.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "completed", "1080x1080", []string{"1080x1080"},
			)
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
			addCreativeCandidateOrchestrationProductionTask(t, current, variantID, "completed", "candidate_primary")
		}
		queued, err := testHandler.maybeQueueCreativeCandidateSelection(
			t.Context(), parseUUID(current.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)},
		)
		if err != nil || !queued {
			t.Fatalf("queue candidate selection for item %s = %v, err %v", current.ItemID, queued, err)
		}
	}
	var taskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = $1
  AND trigger_evidence_ref_id IN ($2, $3)
  AND issue_id IS NULL
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID, secondItemID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 2 {
		t.Fatalf("same-issue candidate comparison tasks = %d, want 2", taskCount)
	}
}

func TestCancelTasksForIssueCancelsCandidateSelectionByContextIssue(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture, task, _ := prepareCreativeCandidateSelectionTask(t, "candidate selection issue cancellation")
	if task.IssueID.Valid {
		t.Fatalf("candidate selection task issue_id = %s, want NULL", uuidToString(task.IssueID))
	}
	if err := testHandler.TaskService.CancelTasksForIssue(t.Context(), parseUUID(fixture.IssueID)); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, task.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" {
		t.Fatalf("candidate selection task status after issue cancellation = %q, want cancelled", status)
	}
}

func TestCreativeCandidateSelectionWaitsForPrimeJobAndWakesAfterHandoff(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate Prime handoff fence")
	var pendingVariantID, failedVariantID string
	for index := 1; index <= 5; index++ {
		variantStatus := "completed"
		taskStatus := "completed"
		if index == 4 {
			variantStatus = "partial"
		}
		if index == 5 {
			variantStatus = "action_required"
			taskStatus = "failed"
		}
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil,
			variantStatus, "1080x1080", []string{"1080x1080"},
		)
		if index != 5 {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		}
		if index <= 3 {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		}
		addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, taskStatus, "candidate_primary")
		if index == 4 {
			pendingVariantID = variantID
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_prime_composition_job (variant_id, revision, status)
VALUES ($1, 1, 'queued')
`, variantID); err != nil {
				t.Fatal(err)
			}
		}
		if index == 5 {
			failedVariantID = variantID
		}
	}

	queued, err := testHandler.maybeQueueCreativeCandidateSelection(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)},
	)
	if err != nil || queued {
		t.Fatalf("candidate selection while Prime pending = queued %t err %v", queued, err)
	}
	var rejectedCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_order_variant
WHERE order_item_id = $1 AND candidate_state = 'rejected'
`, fixture.ItemID).Scan(&rejectedCount); err != nil {
		t.Fatal(err)
	}
	if rejectedCount != 0 {
		t.Fatalf("Prime-pending candidate caused %d premature rejections", rejectedCount)
	}

	addCreativeCandidateOrchestrationAsset(t, pendingVariantID, "1080x1080", "primed")
	for _, statement := range []string{
		`UPDATE creative_order_variant SET status = 'completed' WHERE id = $1`,
		`UPDATE creative_order_variant_revision SET status = 'completed' WHERE variant_id = $1 AND revision = 1`,
		`UPDATE creative_prime_composition_job SET composed_at = now() WHERE variant_id = $1 AND revision = 1`,
	} {
		if _, err := testPool.Exec(t.Context(), statement, pendingVariantID); err != nil {
			t.Fatal(err)
		}
	}
	staleClaim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(pendingVariantID), 1)
	if err != nil || !found || !staleClaim.CompositionReady {
		t.Fatalf("claim composed Prime handoff = %#v found %t err %v", staleClaim, found, err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_prime_composition_job
SET lease_expires_at = now() - interval '1 second'
WHERE variant_id = $1 AND revision = 1
`, pendingVariantID); err != nil {
		t.Fatal(err)
	}
	claim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(pendingVariantID), 1)
	if err != nil || !found || claim.LeaseToken == staleClaim.LeaseToken || !claim.CompositionReady {
		t.Fatalf("reclaim composed Prime handoff = %#v found %t err %v", claim, found, err)
	}
	queued, err = testHandler.maybeQueueCreativeCandidateSelection(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)},
	)
	if err != nil || queued {
		t.Fatalf("candidate selection while composed Prime lease is running = queued %t err %v", queued, err)
	}
	if err := testHandler.processCreativePrimeCompositionClaim(t.Context(), staleClaim); err == nil || !strings.Contains(err.Error(), "lease was lost") {
		t.Fatalf("stale composed Prime handoff err = %v, want lost lease", err)
	}
	queued, err = testHandler.maybeQueueCreativeCandidateSelectionWithPrimeHandoff(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, &claim,
	)
	if err != nil || !queued {
		t.Fatalf("candidate selection before Prime completion = queued %t err %v", queued, err)
	}
	var handoffJobStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT status FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = 1
`, pendingVariantID).Scan(&handoffJobStatus); err != nil {
		t.Fatal(err)
	}
	if handoffJobStatus != "running" {
		t.Fatalf("crash-window Prime job = %q, want recoverable running", handoffJobStatus)
	}
	if err := testHandler.processCreativePrimeCompositionClaim(t.Context(), claim); err != nil {
		t.Fatal(err)
	}

	var selectionTasks, candidateCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*), COALESCE(max(jsonb_array_length(context->'candidates')), 0)
FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&selectionTasks, &candidateCount); err != nil {
		t.Fatal(err)
	}
	if selectionTasks != 1 || candidateCount != 4 {
		t.Fatalf("Prime handoff selection = tasks %d candidates %d", selectionTasks, candidateCount)
	}
	var failedState, jobStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT (SELECT candidate_state FROM creative_order_variant WHERE id = $1),
       (SELECT status FROM creative_prime_composition_job WHERE variant_id = $2 AND revision = 1)
`, failedVariantID, pendingVariantID).Scan(&failedState, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if failedState != "rejected" || jobStatus != "completed" {
		t.Fatalf("Prime handoff final state = failed candidate %q job %q", failedState, jobStatus)
	}
}

func TestCreativeCandidateReconcileQueuesAfterCompletedTerminalTask(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "completed candidate comparison orchestration")
	var terminalTask db.AgentTaskQueue
	for index := 1; index <= 4; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "completed", "1080x1080", []string{"1080x1080"},
		)
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		terminalTask = addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, "completed", "candidate_primary")
	}
	if err := testHandler.reconcileCreativeCandidateOrchestrationForProductionTask(t.Context(), terminalTask); err != nil {
		t.Fatal(err)
	}
	var taskCount, candidateCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*), max(jsonb_array_length(context->'candidates'))
FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskCount, &candidateCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 || candidateCount != 4 {
		t.Fatalf("completed-task candidate comparison = tasks %d candidates %d", taskCount, candidateCount)
	}
}

func TestCreativeCandidateSelectionQueuesThreeReadyAfterTerminalFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate comparison minimum")
	for index := 1; index <= 4; index++ {
		status := "completed"
		if index == 4 {
			status = "action_required"
		}
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, status, "1080x1080", []string{"1080x1080"},
		)
		taskStatus := "completed"
		if index == 4 {
			taskStatus = "failed"
		} else {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		}
		addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, taskStatus, "candidate_primary")
	}
	queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{})
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("candidate comparison did not queue with three ready primary packages")
	}
	var taskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("candidate comparison tasks = %d, want 1", taskCount)
	}
	var rejectedCount, candidateCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FILTER (WHERE candidate_state = 'rejected'),
       COALESCE(max(jsonb_array_length(context->'candidates')), 0)
FROM creative_order_variant variant
LEFT JOIN agent_task_queue task
  ON task.trigger_evidence_kind = $2 AND task.trigger_evidence_ref_id = variant.order_item_id
WHERE variant.order_item_id = $1
`, fixture.ItemID, creativeCandidateSelectionEvidenceKind).Scan(&rejectedCount, &candidateCount); err != nil {
		t.Fatal(err)
	}
	if rejectedCount != 1 || candidateCount != 3 {
		t.Fatalf("three-ready handoff = rejected %d candidates %d", rejectedCount, candidateCount)
	}
}

func TestCreativeCandidateSelectionDoesNotTreatCancellationAsFailure(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate cancellation boundary")
	var cancelledVariantID string
	for index := 1; index <= 5; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "completed", "1080x1080", []string{"1080x1080"},
		)
		taskStatus := "completed"
		if index == 5 {
			taskStatus = "cancelled"
			cancelledVariantID = variantID
		} else {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
			addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		}
		addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, taskStatus, "candidate_primary")
	}
	queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{})
	if err != nil {
		t.Fatal(err)
	}
	if queued {
		t.Fatal("candidate comparison queued after an explicit cancellation")
	}
	var state string
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state FROM creative_order_variant WHERE id = $1`, cancelledVariantID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "candidate" {
		t.Fatalf("cancelled candidate state = %q, want candidate", state)
	}
}

func TestCreativeCandidateSelectionTaskIsScopedToItsOrderItem(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "candidate selection task scope")
	contextValue, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               creativeCandidateSelectionWorkflow,
		"creative_order_id":      fixture.OrderID,
		"creative_order_item_id": fixture.ItemID,
		"item_key":               creativeCandidateSelectionItemKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context,
  requesting_user_id, originator_user_id, accountable_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', $2, $3, $4::jsonb,
        $5, $5, $5, 'direct_human')
RETURNING id::text
`, fixture.Squad.ReviewerAgentID, creativeCandidateSelectionEvidenceKind, fixture.ItemID, contextValue, testUserID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	request := newRequest("POST", "/", nil)
	request.Header.Set("X-Actor-Source", "task_token")
	request.Header.Set("X-Agent-ID", fixture.Squad.ReviewerAgentID)
	request.Header.Set("X-Task-ID", taskID)
	if !testHandler.requireCreativeCandidateSelectionTask(
		httptest.NewRecorder(), request, parseUUID(fixture.OrderID), parseUUID(fixture.ItemID),
	) {
		t.Fatal("matching candidate selection task was rejected")
	}
	response := httptest.NewRecorder()
	if testHandler.requireCreativeCandidateSelectionTask(
		response, request, parseUUID(fixture.OrderID), parseUUID(uuid.NewString()),
	) {
		t.Fatal("candidate selection task was accepted for a different order item")
	}
	if response.Code != 403 {
		t.Fatalf("cross-item candidate selection status = %d, want 403", response.Code)
	}
}

func TestCreativeCandidateSelectionTaskCannotCompleteWithoutAtomicRanking(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture, task, _ := prepareCreativeCandidateSelectionTask(t, "candidate completion gate")
	w := httptest.NewRecorder()
	req := newDaemonTokenRequest(
		http.MethodPost,
		"/api/daemon/tasks/"+uuidToString(task.ID)+"/complete",
		TaskCompleteRequest{Output: "候选比较完成"},
		testWorkspaceID,
		"candidate-selection-daemon",
	)
	req = withURLParam(req, "taskId", uuidToString(task.ID))
	testHandler.CompleteTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("complete candidate selection without ranking = %d %s", w.Code, w.Body.String())
	}
	var status, failureReason, taskError string
	if err := testPool.QueryRow(t.Context(), `
SELECT status, COALESCE(failure_reason, ''), COALESCE(error, '')
FROM agent_task_queue WHERE id = $1
`, task.ID).Scan(&status, &failureReason, &taskError); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || failureReason != "creative_output_missing" || !strings.Contains(taskError, "three selected ranks") {
		t.Fatalf("candidate completion gate = status %q reason %q error %q", status, failureReason, taskError)
	}
	var queuedReplacements int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2 AND status = 'queued'
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&queuedReplacements); err != nil {
		t.Fatal(err)
	}
	if queuedReplacements != 1 {
		t.Fatalf("candidate selection replacements = %d, want 1", queuedReplacements)
	}
}

func TestCreativeCandidateSelectionTaskCompletesAfterAtomicRanking(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture, task, variantIDs := prepareCreativeCandidateSelectionTask(t, "candidate completion after ranking")
	w := httptest.NewRecorder()
	req := newRequest(
		http.MethodPost,
		"/api/creative/orders/"+fixture.OrderID+"/items/"+fixture.ItemID+"/candidate-selection",
		creativeCandidateSelectionInput{SelectedIDs: variantIDs[:3], ReserveIDs: variantIDs[3:]},
	)
	req = withURLParams(req, "id", fixture.OrderID, "itemId", fixture.ItemID)
	testHandler.SelectCreativeOrderItemCandidates(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("atomic candidate selection = %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newDaemonTokenRequest(
		http.MethodPost,
		"/api/daemon/tasks/"+uuidToString(task.ID)+"/complete",
		TaskCompleteRequest{Output: "候选比较与晋级完成"},
		testWorkspaceID,
		"candidate-selection-daemon",
	)
	req = withURLParam(req, "taskId", uuidToString(task.ID))
	testHandler.CompleteTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("complete ranked candidate selection = %d %s", w.Code, w.Body.String())
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, task.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("ranked candidate selection task status = %q, want completed", status)
	}
	var selectionTaskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&selectionTaskCount); err != nil {
		t.Fatal(err)
	}
	if selectionTaskCount != 1 {
		t.Fatalf("ranked candidate selection tasks = %d, want 1", selectionTaskCount)
	}
}

func TestCreativeCandidateSelectionCompletesWithoutReserveAfterTerminalRejection(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture, task, variantIDs := prepareCreativeCandidateSelectionTask(t, "candidate completion without reserve")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant
SET candidate_state = 'rejected', status = 'action_required', selection_rank = NULL
WHERE id = $1
`, variantIDs[3]); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(
		http.MethodPost,
		"/api/creative/orders/"+fixture.OrderID+"/items/"+fixture.ItemID+"/candidate-selection",
		creativeCandidateSelectionInput{SelectedIDs: variantIDs[:3]},
	)
	req = withURLParams(req, "id", fixture.OrderID, "itemId", fixture.ItemID)
	testHandler.SelectCreativeOrderItemCandidates(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("three-only candidate selection = %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newDaemonTokenRequest(
		http.MethodPost,
		"/api/daemon/tasks/"+uuidToString(task.ID)+"/complete",
		TaskCompleteRequest{Output: "三个候选完成原子晋级"},
		testWorkspaceID,
		"candidate-selection-daemon",
	)
	req = withURLParam(req, "taskId", uuidToString(task.ID))
	testHandler.CompleteTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("complete three-only candidate selection = %d %s", w.Code, w.Body.String())
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, task.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("three-only candidate selection task status = %q, want completed", status)
	}
}

func TestCreativeCandidateSelectionDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture, task, variantIDs := prepareCreativeCandidateSelectionTask(t, "candidate selection cancellation fence")
	selectCandidates := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(
			http.MethodPost,
			"/api/creative/orders/"+fixture.OrderID+"/items/"+fixture.ItemID+"/candidate-selection",
			creativeCandidateSelectionInput{SelectedIDs: variantIDs[:3], ReserveIDs: variantIDs[3:]},
		)
		req = withURLParams(req, "id", fixture.OrderID, "itemId", fixture.ItemID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", uuidToString(task.AgentID))
		req.Header.Set("X-Task-ID", uuidToString(task.ID))
		testHandler.SelectCreativeOrderItemCandidates(w, req)
		return w
	}

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, fixture.OrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant
SET status = 'cancelled', updated_at = now()
WHERE order_item_id = $1
`, fixture.ItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant_revision revision
SET status = 'cancelled', updated_at = now()
FROM creative_order_variant variant
WHERE revision.variant_id = variant.id AND variant.order_item_id = $1
`, fixture.ItemID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- selectCandidates() }()
	select {
	case w := <-result:
		t.Fatalf("candidate selection bypassed the order cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("candidate selection did not resume after order cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "creative order is cancelled") {
		t.Fatalf("cancelled order accepted candidate selection = %d %s", w.Code, w.Body.String())
	}

	var candidateCount, rankedCount, cancelledVariantCount, cancelledRevisionCount, expansionTaskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FILTER (WHERE candidate_state = 'candidate'),
       count(*) FILTER (WHERE selection_rank IS NOT NULL),
       count(*) FILTER (WHERE status = 'cancelled'),
       (SELECT count(*)
        FROM creative_order_variant_revision revision
        JOIN creative_order_variant candidate ON candidate.id = revision.variant_id
        WHERE candidate.order_item_id = $1 AND revision.status = 'cancelled'),
       (SELECT count(*) FROM agent_task_queue production
        WHERE production.trigger_evidence_kind = 'creative_order_item_production'
          AND production.trigger_evidence_ref_id = $1
          AND production.context->>'production_phase' = $2)
FROM creative_order_variant
WHERE order_item_id = $1
`, fixture.ItemID, creativeSelectedExpansionPhase).Scan(
		&candidateCount, &rankedCount, &cancelledVariantCount, &cancelledRevisionCount, &expansionTaskCount,
	); err != nil {
		t.Fatal(err)
	}
	if candidateCount != 4 || rankedCount != 0 || cancelledVariantCount != 4 || cancelledRevisionCount != 4 || expansionTaskCount != 0 {
		t.Fatalf("cancelled candidate selection mutated state: candidates=%d ranked=%d variants=%d revisions=%d expansion_tasks=%d",
			candidateCount, rankedCount, cancelledVariantCount, cancelledRevisionCount, expansionTaskCount)
	}
}

func TestCreativeCandidateSelectionTerminalTasksCanBeRebuilt(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	t.Run("failed", func(t *testing.T) {
		fixture, task, _ := prepareCreativeCandidateSelectionTask(t, "failed candidate selection rebuild")
		w := httptest.NewRecorder()
		req := newDaemonTokenRequest(
			http.MethodPost,
			"/api/daemon/tasks/"+uuidToString(task.ID)+"/fail",
			TaskFailRequest{Error: "candidate comparison failed", FailureReason: "agent_error.unknown"},
			testWorkspaceID,
			"candidate-selection-daemon",
		)
		req = withURLParam(req, "taskId", uuidToString(task.ID))
		testHandler.FailTask(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("fail candidate selection = %d %s", w.Code, w.Body.String())
		}
		var replacements int
		if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2 AND status = 'queued'
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&replacements); err != nil {
			t.Fatal(err)
		}
		if replacements != 1 {
			t.Fatalf("failed candidate selection replacements = %d, want 1", replacements)
		}
	})
	t.Run("cancelled", func(t *testing.T) {
		fixture, task, _ := prepareCreativeCandidateSelectionTask(t, "cancelled candidate selection rebuild")
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/issues/"+fixture.IssueID+"/tasks/"+uuidToString(task.ID)+"/cancel", nil)
		req = withURLParams(req, "id", fixture.IssueID, "taskId", uuidToString(task.ID))
		testHandler.CancelTask(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("cancel candidate selection = %d %s", w.Code, w.Body.String())
		}
		queued, err := testHandler.maybeQueueCreativeCandidateSelection(
			t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)},
		)
		if err != nil || !queued {
			t.Fatalf("rebuild cancelled candidate selection = %v, err %v", queued, err)
		}
		var cancelled, replacements int
		if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FILTER (WHERE status = 'cancelled'), count(*) FILTER (WHERE status = 'queued')
FROM agent_task_queue
WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2
`, creativeCandidateSelectionEvidenceKind, fixture.ItemID).Scan(&cancelled, &replacements); err != nil {
			t.Fatal(err)
		}
		if cancelled != 1 || replacements != 1 {
			t.Fatalf("cancelled candidate selection tasks = cancelled %d queued %d", cancelled, replacements)
		}
	})
}

func TestCreativePipelineVersionIsRequiredForStandardOrders(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	t.Run("missing version is rejected", func(t *testing.T) {
		orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "missing creative pipeline version")
		if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order SET input_snapshot = input_snapshot - 'pipeline_version' WHERE id = $1
`, orderID); err != nil {
			t.Fatal(err)
		}

		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
			OrderItemID:    itemID,
			VariantKey:     "C01",
			Brief:          json.RawMessage(`{}`),
			Revision:       1,
			Status:         "running",
			CandidateState: "candidate",
			PrimarySize:    "1080x1080",
		})
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "pipeline_version candidate_v1") {
			t.Fatalf("missing-version variant write = %d %s", w.Code, w.Body.String())
		}

		w = httptest.NewRecorder()
		req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/candidate-selection", creativeCandidateSelectionInput{
			SelectedIDs: []string{uuid.NewString(), uuid.NewString(), uuid.NewString()},
			ReserveIDs:  []string{uuid.NewString()},
		})
		req = withURLParams(req, "id", orderID, "itemId", itemID)
		testHandler.SelectCreativeOrderItemCandidates(w, req)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "pipeline_version candidate_v1") {
			t.Fatalf("missing-version candidate selection = %d %s", w.Code, w.Body.String())
		}
		queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(itemID), creativeOrchestrationCause{})
		if err != nil || queued {
			t.Fatalf("missing-version candidate selection queue = %v, err %v", queued, err)
		}
	})

	t.Run("unsupported version is rejected", func(t *testing.T) {
		orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "unsupported creative pipeline version")
		if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET input_snapshot = jsonb_set(input_snapshot, '{pipeline_version}', '"unsupported"'::jsonb)
WHERE id = $1
`, orderID); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
			OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{}`), Revision: 1,
			Status: "running", CandidateState: "candidate", PrimarySize: "1080x1080",
		})
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "pipeline_version candidate_v1") {
			t.Fatalf("unsupported-version variant write = %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("candidate order accepts C variants only", func(t *testing.T) {
		fixture := createCreativeCandidateOrchestrationFixture(t, "candidate pipeline isolation")
		put := func(key, state string) *httptest.ResponseRecorder {
			w := httptest.NewRecorder()
			req := newRequest(http.MethodPut, "/api/creative/orders/"+fixture.OrderID+"/variants", creativeOrderVariantInput{
				OrderItemID:    fixture.ItemID,
				VariantKey:     key,
				Brief:          json.RawMessage(`{}`),
				Revision:       1,
				Status:         "running",
				CandidateState: state,
				PrimarySize:    "1080x1080",
			})
			req = withURLParam(req, "id", fixture.OrderID)
			testHandler.UpsertCreativeOrderVariant(w, req)
			return w
		}
		candidate := put("C01", "candidate")
		if candidate.Code != http.StatusOK {
			t.Fatalf("candidate C01 write = %d %s", candidate.Code, candidate.Body.String())
		}
		legacy := put("V01", "selected")
		if legacy.Code != http.StatusConflict || !strings.Contains(legacy.Body.String(), "C01-C05") {
			t.Fatalf("candidate V01 write = %d %s", legacy.Code, legacy.Body.String())
		}
	})

	t.Run("direct edit bypasses candidate pipeline", func(t *testing.T) {
		orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "direct edit pipeline isolation")
		if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET trigger_evidence_kind = 'creative_direct_edit',
		    input_snapshot = '{"pipeline_version":"direct_edit_v1","target_size":"1200x628","delivery_mode":"publish","user_request":"调整标题位置"}'::jsonb
WHERE id = $1
`, orderID); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
			OrderItemID: itemID, VariantKey: "V01", Brief: json.RawMessage(`{}`), Revision: 1, Status: "running",
		})
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("direct-edit variant write = %d %s", w.Code, w.Body.String())
		}
	})
}

func TestSelectedCreativeProductionQueuesOnlyMissingSizesIdempotently(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "selected missing size orchestration")
	variantIDs := make([]string, 0, 4)
	for index := 1; index <= 4; index++ {
		state := "selected"
		expected := standardCreativeAssetSizes
		if index == 4 {
			state = "reserve"
			expected = []string{"1080x1080"}
		}
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), state, index, "completed", "1080x1080", expected,
		)
		variantIDs = append(variantIDs, variantID)
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
		if index == 1 {
			addCreativeCandidateOrchestrationAsset(t, variantID, "1200x628", "generated")
			addCreativeCandidateOrchestrationAsset(t, variantID, "800x1000", "generated")
		}
	}
	tasks, err := testHandler.queueSelectedCreativeProductionTasks(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, creativeSelectedExpansionPhase,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("selected missing-size tasks = %d, want 2", len(tasks))
	}
	initialTasks := append([]db.AgentTaskQueue(nil), tasks...)
	var taskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'production_phase' = $2
  AND context->'missing_sizes' = '["1200x628", "800x1000"]'::jsonb
  AND context->>'candidate_state' = 'selected'
`, fixture.ItemID, creativeSelectedExpansionPhase).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 2 {
		t.Fatalf("canonical selected expansion tasks = %d, want 2", taskCount)
	}
	var reserveTasks int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND context->>'variant_id' = $1
  AND context->>'production_phase' = $2
`, variantIDs[3], creativeSelectedExpansionPhase).Scan(&reserveTasks); err != nil {
		t.Fatal(err)
	}
	if reserveTasks != 0 {
		t.Fatalf("reserve production tasks = %d, want 0", reserveTasks)
	}
	tasks, err = testHandler.queueSelectedCreativeProductionTasks(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, creativeSelectedExpansionPhase,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 0 {
		t.Fatalf("selected expansion replay returned %d tasks, want 0", len(tasks))
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue
SET status = CASE WHEN id = $1 THEN 'cancelled' ELSE 'failed' END,
    completed_at = now()
WHERE id = ANY($2::uuid[])
`, initialTasks[0].ID, []pgtype.UUID{initialTasks[0].ID, initialTasks[1].ID}); err != nil {
		t.Fatal(err)
	}
	tasks, err = testHandler.queueSelectedCreativeProductionTasks(
		t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, creativeSelectedExpansionPhase,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 {
		t.Fatalf("selected expansion did not rebuild cancelled or failed tasks: %d, want 2", len(tasks))
	}
}

func TestSelectedCreativeProductionEnqueueDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "selected enqueue cancellation fence")
	for index := 1; index <= 3; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "selected", index, "completed", "1080x1080", standardCreativeAssetSizes,
		)
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
	}
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `UPDATE creative_order SET status = 'cancelled' WHERE id = $1`, fixture.OrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'cancelled' WHERE order_item_id = $1`, fixture.ItemID); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		tasks []db.AgentTaskQueue
		err   error
	}, 1)
	go func() {
		tasks, queueErr := testHandler.queueSelectedCreativeProductionTasks(
			t.Context(), parseUUID(fixture.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, creativeSelectedExpansionPhase,
		)
		result <- struct {
			tasks []db.AgentTaskQueue
			err   error
		}{tasks: tasks, err: queueErr}
	}()
	select {
	case value := <-result:
		t.Fatalf("selected enqueue bypassed cancellation lock: tasks %d err %v", len(value.tasks), value.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-result:
		if value.err != nil || len(value.tasks) != 0 {
			t.Fatalf("cancelled selected enqueue = tasks %d err %v", len(value.tasks), value.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("selected enqueue did not resume after cancellation committed")
	}
	var taskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'production_phase' = $2
`, fixture.ItemID, creativeSelectedExpansionPhase).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 0 {
		t.Fatalf("cancelled selected enqueue created %d tasks", taskCount)
	}
}

func TestCreativeReservePromotionDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "reserve promotion cancellation fence")
	failedID := createCreativeCandidateOrchestrationVariant(
		t, fixture.ItemID, "C01", "selected", 1, "action_required", "1080x1080", standardCreativeAssetSizes,
	)
	reserveID := createCreativeCandidateOrchestrationVariant(
		t, fixture.ItemID, "C04", "reserve", 4, "completed", "1080x1080", []string{"1080x1080"},
	)
	addCreativeCandidateOrchestrationAsset(t, reserveID, "1080x1080", "generated")
	addCreativeCandidateOrchestrationAsset(t, reserveID, "1080x1080", "primed")
	failedTask := addCreativeCandidateOrchestrationProductionTask(t, fixture, failedID, "failed", creativeSelectedExpansionPhase)
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `UPDATE creative_order SET status = 'cancelled' WHERE id = $1`, fixture.OrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'cancelled' WHERE order_item_id = $1`, fixture.ItemID); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		promoted bool
		tasks    []db.AgentTaskQueue
		err      error
	}, 1)
	go func() {
		promoted, tasks, promoteErr := testHandler.maybePromoteCreativeReserve(
			t.Context(), parseUUID(failedID), creativeOrchestrationCause{ParentTask: &failedTask},
		)
		result <- struct {
			promoted bool
			tasks    []db.AgentTaskQueue
			err      error
		}{promoted: promoted, tasks: tasks, err: promoteErr}
	}()
	select {
	case value := <-result:
		t.Fatalf("reserve promotion bypassed cancellation lock: promoted %t tasks %d err %v", value.promoted, len(value.tasks), value.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-result:
		if value.err != nil || value.promoted || len(value.tasks) != 0 {
			t.Fatalf("cancelled reserve promotion = promoted %t tasks %d err %v", value.promoted, len(value.tasks), value.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reserve promotion did not resume after cancellation committed")
	}
	var failedState, reserveState string
	var promotionTasks int
	if err := testPool.QueryRow(t.Context(), `
SELECT (SELECT candidate_state FROM creative_order_variant WHERE id = $1),
       (SELECT candidate_state FROM creative_order_variant WHERE id = $2),
       (SELECT count(*) FROM agent_task_queue
        WHERE trigger_evidence_kind = 'creative_order_item_production'
          AND trigger_evidence_ref_id = $3
          AND context->>'production_phase' = $4)
`, failedID, reserveID, fixture.ItemID, creativeReservePromotionPhase).Scan(&failedState, &reserveState, &promotionTasks); err != nil {
		t.Fatal(err)
	}
	if failedState != "selected" || reserveState != "reserve" || promotionTasks != 0 {
		t.Fatalf("cancelled reserve promotion mutated state: failed %q reserve %q tasks %d", failedState, reserveState, promotionTasks)
	}
}

func TestCreativePrimeNoAdequateTemplateQueuesSingleSizeNextRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "Prime background support rework")
	variantID := createCreativeCandidateOrchestrationVariant(
		t, fixture.ItemID, "C01", "selected", 1, "running", "1080x1080", standardCreativeAssetSizes,
	)
	for _, size := range standardCreativeAssetSizes {
		addCreativeCandidateOrchestrationAsset(t, variantID, size, "generated")
	}
	addCreativeCandidateOrchestrationProductionTask(t, fixture, variantID, "completed", creativeSelectedExpansionPhase)
	runErr := &creativePrimeCompositionError{
		cause: errors.New("Prime template support failed"),
		failures: []json.RawMessage{json.RawMessage(`{
  "size":"800x1000",
  "error_code":"prime_no_adequate_template_for_size",
  "template_selection":{"visual_adequacy":{"failure_code":"prime_no_adequate_template_for_size","inadequacy_codes":["prime_background_polarity_mismatch"]}}
}`)},
	}
	testHandler.markCreativePrimeCompositionFailed(t.Context(), parseUUID(variantID), runErr)
	task, queued, err := testHandler.queueCreativePrimeBackgroundRework(t.Context(), creativePrimeCompositionClaim{
		OrderID: parseUUID(fixture.OrderID), VariantID: parseUUID(variantID), Revision: 1,
	}, runErr)
	if err != nil || !queued || !task.ID.Valid {
		t.Fatalf("queue Prime background support rework = queued %t task %s err %v", queued, uuidToString(task.ID), err)
	}

	var revision int
	var status string
	var stagingRevision pgtype.Int4
	var expectedSizes []string
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.revision, variant.status, variant.staging_revision, revision.expected_sizes
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variantID).Scan(&revision, &status, &stagingRevision, &expectedSizes); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || status != "running" || !stagingRevision.Valid || stagingRevision.Int32 != 2 || !slices.Equal(expectedSizes, standardCreativeAssetSizes) {
		t.Fatalf("Prime background support revision = r%d/%q staging %v sizes %v", revision, status, stagingRevision, expectedSizes)
	}
	var preserved, failedSizeCopies int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FILTER (WHERE size_key <> '800x1000'),
       count(*) FILTER (WHERE size_key = '800x1000')
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 2 AND stage = 'generated'
`, variantID).Scan(&preserved, &failedSizeCopies); err != nil {
		t.Fatal(err)
	}
	if preserved != 2 || failedSizeCopies != 0 {
		t.Fatalf("Prime background support copied generated assets = preserved %d failed-size %d", preserved, failedSizeCopies)
	}
	var taskRevision int
	var strategy string
	var targetSizes []string
	if err := testPool.QueryRow(t.Context(), `
SELECT (context->>'revision')::int,
       context->'qc_visual_rework'->>'reflow_strategy',
       ARRAY(SELECT jsonb_array_elements_text(context->'qc_visual_rework'->'target_sizes'))
FROM agent_task_queue
WHERE id = $1
`, task.ID).Scan(&taskRevision, &strategy, &targetSizes); err != nil {
		t.Fatal(err)
	}
	if taskRevision != 2 || strategy != "prime_background_support" || !slices.Equal(targetSizes, []string{"800x1000"}) {
		t.Fatalf("Prime background support task = r%d/%q targets %v", taskRevision, strategy, targetSizes)
	}
	if _, queued, err := testHandler.queueCreativePrimeBackgroundRework(t.Context(), creativePrimeCompositionClaim{
		OrderID: parseUUID(fixture.OrderID), VariantID: parseUUID(variantID), Revision: 1,
	}, runErr); err != nil || queued {
		t.Fatalf("replayed Prime background support = queued %t err %v", queued, err)
	}
}

func TestCreativeCandidateSelectionTerminalReconcilesMissingSizeFanout(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, terminalStatus := range []string{"completed", "failed"} {
		t.Run(terminalStatus, func(t *testing.T) {
			fixture := createCreativeCandidateOrchestrationFixture(t, "selection terminal fanout "+terminalStatus)
			for index := 1; index <= 4; index++ {
				state := "selected"
				expectedSizes := standardCreativeAssetSizes
				if index == 4 {
					state = "reserve"
					expectedSizes = []string{"1080x1080"}
				}
				variantID := createCreativeCandidateOrchestrationVariant(
					t, fixture.ItemID, fmt.Sprintf("C%02d", index), state, index, "completed", "1080x1080", expectedSizes,
				)
				addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
			}
			contextValue, err := json.Marshal(map[string]any{
				"type":                   "creative_domain_task",
				"workflow":               creativeCandidateSelectionWorkflow,
				"creative_order_id":      fixture.OrderID,
				"creative_order_item_id": fixture.ItemID,
				"item_key":               creativeCandidateSelectionItemKey,
			})
			if err != nil {
				t.Fatal(err)
			}
			var taskID pgtype.UUID
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context,
  completed_at, requesting_user_id, originator_user_id, accountable_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, $3, $4, $5::jsonb,
        now(), $6, $6, $6, 'direct_human')
RETURNING id
`, fixture.Squad.ReviewerAgentID, terminalStatus, creativeCandidateSelectionEvidenceKind, fixture.ItemID, contextValue, testUserID).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			task, err := testHandler.Queries.GetAgentTask(t.Context(), taskID)
			if err != nil {
				t.Fatal(err)
			}
			if err := testHandler.reconcileCreativeCandidateOrchestrationForProductionTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			if err := testHandler.reconcileCreativeCandidateOrchestrationForProductionTask(t.Context(), task); err != nil {
				t.Fatalf("terminal selection fanout replay: %v", err)
			}
			var taskCount int
			if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'production_phase' = $2
`, fixture.ItemID, creativeSelectedExpansionPhase).Scan(&taskCount); err != nil {
				t.Fatal(err)
			}
			if taskCount != 3 {
				t.Fatalf("terminal selection missing-size tasks = %d, want 3", taskCount)
			}
		})
	}
}

func TestCreativeReservePromotionIsIdempotentAndProtectsActiveRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "reserve promotion orchestration")
	failedID := createCreativeCandidateOrchestrationVariant(
		t, fixture.ItemID, "C01", "selected", 1, "action_required", "1080x1080", standardCreativeAssetSizes,
	)
	stableIDs := make([]string, 0, 2)
	for index := 2; index <= 3; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "selected", index, "completed", "1080x1080", standardCreativeAssetSizes,
		)
		stableIDs = append(stableIDs, variantID)
		for _, size := range standardCreativeAssetSizes {
			addCreativeCandidateOrchestrationAsset(t, variantID, size, "generated")
		}
	}
	reserveIDs := make([]string, 0, 2)
	for index := 4; index <= 5; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "reserve", index, "completed", "1080x1080", []string{"1080x1080"},
		)
		reserveIDs = append(reserveIDs, variantID)
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, variantID, "1080x1080", "primed")
	}
	failedTask := addCreativeCandidateOrchestrationProductionTask(t, fixture, failedID, "failed", creativeSelectedExpansionPhase)
	promoted, tasks, err := testHandler.maybePromoteCreativeReserve(
		t.Context(), parseUUID(failedID), creativeOrchestrationCause{ParentTask: &failedTask},
	)
	if err != nil || promoted || len(tasks) != 0 {
		t.Fatalf("default reserve promotion = promoted %v tasks %d err %v", promoted, len(tasks), err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET input_snapshot = input_snapshot || '{"reserve_promotion_mode":"allow"}'::jsonb
WHERE id = $1
`, fixture.OrderID); err != nil {
		t.Fatal(err)
	}
	promoted, tasks, err = testHandler.maybePromoteCreativeReserve(
		t.Context(), parseUUID(failedID), creativeOrchestrationCause{ParentTask: &failedTask},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !promoted || len(tasks) != 1 {
		t.Fatalf("reserve promotion = promoted %v tasks %d", promoted, len(tasks))
	}
	var failedState, promotedState, remainingState string
	var failedRank, promotedRank, remainingRank pgtype.Int4
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, failedID).Scan(&failedState, &failedRank); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, reserveIDs[0]).Scan(&promotedState, &promotedRank); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, reserveIDs[1]).Scan(&remainingState, &remainingRank); err != nil {
		t.Fatal(err)
	}
	if failedState != "rejected" || failedRank.Valid || promotedState != "selected" || !promotedRank.Valid || promotedRank.Int32 != 1 || remainingState != "reserve" || !remainingRank.Valid || remainingRank.Int32 != 5 {
		t.Fatalf("promotion states = failed %q/%v promoted %q/%v remaining %q/%v", failedState, failedRank, promotedState, promotedRank, remainingState, remainingRank)
	}
	var promotedTaskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'variant_id' = $2
  AND context->>'production_phase' = $3
  AND context->'missing_sizes' = '["1200x628", "800x1000"]'::jsonb
`, fixture.ItemID, reserveIDs[0], creativeReservePromotionPhase).Scan(&promotedTaskCount); err != nil {
		t.Fatal(err)
	}
	if promotedTaskCount != 1 {
		t.Fatalf("promoted reserve production tasks = %d, want 1", promotedTaskCount)
	}
	var allPromotionTasks int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'production_phase' = $2
`, fixture.ItemID, creativeReservePromotionPhase).Scan(&allPromotionTasks); err != nil {
		t.Fatal(err)
	}
	if allPromotionTasks != 1 {
		t.Fatalf("reserve promotion queued unrelated selected variants: %d tasks", allPromotionTasks)
	}
	promoted, tasks, err = testHandler.maybePromoteCreativeReserve(
		t.Context(), parseUUID(failedID), creativeOrchestrationCause{ParentTask: &failedTask},
	)
	if err != nil || promoted || len(tasks) != 0 {
		t.Fatalf("reserve promotion replay = promoted %v tasks %d err %v", promoted, len(tasks), err)
	}

	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant
SET active_revision = 1, staging_revision = NULL, status = 'action_required'
WHERE id = $1
`, stableIDs[0]); err != nil {
		t.Fatal(err)
	}
	promoted, tasks, err = testHandler.maybePromoteCreativeReserve(
		t.Context(), parseUUID(stableIDs[0]), creativeOrchestrationCause{},
	)
	if err != nil || promoted || len(tasks) != 0 {
		t.Fatalf("active revision reserve protection = promoted %v tasks %d err %v", promoted, len(tasks), err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, reserveIDs[1]).Scan(&remainingState, &remainingRank); err != nil {
		t.Fatal(err)
	}
	if remainingState != "reserve" || !remainingRank.Valid || remainingRank.Int32 != 5 {
		t.Fatalf("active revision consumed remaining reserve: %q/%v", remainingState, remainingRank)
	}
}

func TestExhaustedCreativeQCFinalizationPromotesReserveOnce(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeCandidateOrchestrationFixture(t, "exhausted QC reserve promotion")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET input_snapshot = input_snapshot || '{"reserve_promotion_mode":"allow"}'::jsonb
WHERE id = $1
`, fixture.OrderID); err != nil {
		t.Fatal(err)
	}
	var failedID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, brief, revision, status, candidate_state, selection_rank, primary_size
)
VALUES ($1, 'C01', '{}'::jsonb, 3, 'running', 'selected', 1, '1080x1080')
RETURNING id::text
`, fixture.ItemID).Scan(&failedID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 3, '{}'::jsonb, 'running', $2::text[])
`, failedID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET staging_revision = 3 WHERE id = $1`, failedID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "orchestration-qc-exhausted-"+size+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 3, 'primed', $3, 'completed')
`, failedID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	for index := 2; index <= 3; index++ {
		variantID := createCreativeCandidateOrchestrationVariant(
			t, fixture.ItemID, fmt.Sprintf("C%02d", index), "selected", index, "completed", "1080x1080", standardCreativeAssetSizes,
		)
		for _, size := range standardCreativeAssetSizes {
			addCreativeCandidateOrchestrationAsset(t, variantID, size, "generated")
		}
	}
	reserveID := createCreativeCandidateOrchestrationVariant(
		t, fixture.ItemID, "C04", "reserve", 4, "completed", "1080x1080", []string{"1080x1080"},
	)
	addCreativeCandidateOrchestrationAsset(t, reserveID, "1080x1080", "generated")
	addCreativeCandidateOrchestrationAsset(t, reserveID, "1080x1080", "primed")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'technical', 3, 1, 'passed', '{}'::jsonb),
	       ($1, 'visual', 3, 1, 'failed',
	        '{"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628: 主体与顶部 Prime 区冲突，期望移动到 safe_content_frame y=0.30"}]}'::jsonb)
`, failedID); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < creativeVisualModelReworkMaxAttempts; attempt++ {
		contextValue, err := json.Marshal(map[string]any{
			"type":                   "creative_domain_task",
			"workflow":               "creative_production",
			"creative_order_id":      fixture.OrderID,
			"creative_order_item_id": fixture.ItemID,
			"variant_id":             failedID,
			"revision":               attempt + 1,
			"qc_visual_rework":       map[string]any{},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context,
  completed_at, requesting_user_id, originator_user_id, accountable_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed',
        'creative_order_item_production', $2, $3::jsonb, now(), $4, $4, $4, 'direct_human')
`, fixture.Squad.ProducerAgentID, fixture.ItemID, contextValue, testUserID); err != nil {
			t.Fatal(err)
		}
	}
	var qcTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context,
  requesting_user_id, originator_user_id, accountable_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running',
        'creative_order_variant_qc', $2, $3::jsonb, $4, $4, $4, 'direct_human')
RETURNING id::text
`, fixture.Squad.ReviewerAgentID, failedID, creativeQCTaskContextForTest(t, fixture.OrderID, failedID, "visual", 3), testUserID).Scan(&qcTaskID); err != nil {
		t.Fatal(err)
	}
	finalize := func() creativeOrderQCFinalizeResponse {
		t.Helper()
		response := httptest.NewRecorder()
		request := newRequest(http.MethodPost, "/api/creative/orders/"+fixture.OrderID+"/qc-finalize", creativeOrderQCFinalizeInput{
			VariantID: failedID,
			Revision:  3,
		})
		request = withURLParam(request, "id", fixture.OrderID)
		request.Header.Set("X-Actor-Source", "task_token")
		request.Header.Set("X-Agent-ID", fixture.Squad.ReviewerAgentID)
		request.Header.Set("X-Task-ID", qcTaskID)
		testHandler.FinalizeCreativeOrderQC(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("exhausted QC finalization = %d %s", response.Code, response.Body.String())
		}
		var result creativeOrderQCFinalizeResponse
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := finalize()
	if result.Outcome != "action_required" || result.DeliveredAssetCount != 0 {
		t.Fatalf("exhausted QC result = %#v", result)
	}
	var failedState, reserveState string
	var failedRank, reserveRank pgtype.Int4
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, failedID).Scan(&failedState, &failedRank); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, reserveID).Scan(&reserveState, &reserveRank); err != nil {
		t.Fatal(err)
	}
	if failedState != "rejected" || failedRank.Valid || reserveState != "selected" || !reserveRank.Valid || reserveRank.Int32 != 1 {
		t.Fatalf("QC reserve promotion states = failed %q/%v reserve %q/%v", failedState, failedRank, reserveState, reserveRank)
	}
	var delivered, promotionTasks int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset WHERE variant_id = $1 AND revision = 3 AND stage = 'delivered'`, failedID).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'variant_id' = $2
  AND context->>'production_phase' = $3
`, fixture.ItemID, reserveID, creativeReservePromotionPhase).Scan(&promotionTasks); err != nil {
		t.Fatal(err)
	}
	if delivered != 0 || promotionTasks != 1 {
		t.Fatalf("exhausted QC delivery=%d promotion tasks=%d", delivered, promotionTasks)
	}
	result = finalize()
	if result.Outcome != "action_required" {
		t.Fatalf("repeated exhausted QC result = %#v", result)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'variant_id' = $2
  AND context->>'production_phase' = $3
`, fixture.ItemID, reserveID, creativeReservePromotionPhase).Scan(&promotionTasks); err != nil {
		t.Fatal(err)
	}
	if promotionTasks != 1 {
		t.Fatalf("repeated exhausted QC promotion tasks = %d, want 1", promotionTasks)
	}
}
