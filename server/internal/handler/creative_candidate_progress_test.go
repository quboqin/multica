package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func seedSixSetCandidatePrimaries(t *testing.T, f creativeCandidateOrchestrationFixture, count int) []string {
	t.Helper()
	var ids []string
	for index := 1; index <= count; index++ {
		id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "completed", "1080x1080", []string{"1080x1080"})
		addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "primed")
		addCreativeCandidateOrchestrationProductionTask(t, f, id, "completed", "candidate_primary")
		ids = append(ids, id)
	}
	return ids
}

func candidateProgressForTest(t *testing.T, f creativeCandidateOrchestrationFixture) *creativeCandidateProgress {
	t.Helper()
	p, err := loadCreativeCandidateProgress(t.Context(), testPool, parseUUID(f.OrderID), parseUUID(f.ItemID))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func candidateRecoveryForTest(t *testing.T, f creativeCandidateOrchestrationFixture, want int) creativeOrderWorkflowRetryResponse {
	t.Helper()
	w := httptest.NewRecorder()
	r := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+f.OrderID+"/items/"+f.ItemID+"/candidate-recovery", nil), "id", f.OrderID, "itemId", f.ItemID)
	testHandler.RecoverCreativeOrderCandidates(w, r)
	if w.Code != want {
		t.Fatalf("candidate recovery %d: %s", w.Code, w.Body.String())
	}
	var value creativeOrderWorkflowRetryResponse
	if want == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
	}
	return value
}

func seedCandidatePlanTask(t *testing.T, f creativeCandidateOrchestrationFixture, status string) db.AgentTaskQueue {
	t.Helper()
	task := addCreativeCandidateOrchestrationProductionTask(t, f, uuid.NewString(), status, "planning")
	_, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET agent_id=$2, trigger_evidence_kind='creative_order_item_plan', max_attempts=3, context=(context-'variant_id'-'expected_sizes'-'production_phase') || '{"workflow":"creative_plan"}'::jsonb WHERE id=$1`, task.ID, f.Squad.PlannerAgentID)
	if err != nil {
		t.Fatal(err)
	}
	task, err = testHandler.Queries.GetAgentTask(t.Context(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func TestCreativeCandidateProgressExposesSixOfEightAndRecoversOnlyPlanning(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 6)
	parent := seedCandidatePlanTask(t, f, "completed")
	p := candidateProgressForTest(t, f)
	if p.State != "planning_incomplete" || p.Planned != 6 || p.Expected != 8 || p.Generated != 6 {
		t.Fatalf("progress=%+v", p)
	}
	first := candidateRecoveryForTest(t, f, http.StatusOK)
	second := candidateRecoveryForTest(t, f, http.StatusOK)
	if first.TaskID == "" || first.TaskID != second.TaskID {
		t.Fatal("repeated recovery did not reuse planning task")
	}
	child, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(first.TaskID))
	if err != nil || child.ParentTaskID != parent.ID || child.Attempt != 2 {
		t.Fatalf("planning retry=%+v %v", child, err)
	}
	var assets int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset a JOIN creative_order_variant v ON v.id=a.variant_id WHERE v.order_item_id=$1`, f.ItemID).Scan(&assets); err != nil || assets != 12 {
		t.Fatalf("existing images changed: %d %v", assets, err)
	}
}

func TestCreativeCandidatePlanningCompletionRejectsMissingCandidatesAndDelegation(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	ids := seedSixSetCandidatePrimaries(t, f, 6)
	parent := seedCandidatePlanTask(t, f, "running")
	message, err := testHandler.creativePlanningCompletionError(t.Context(), parent, testWorkspaceID)
	if err != nil || !strings.Contains(message, "registered 6") {
		t.Fatalf("incomplete plan=%q %v", message, err)
	}
	for index := 7; index <= 8; index++ {
		ids = append(ids, createCreativeCandidateOrchestrationVariant(t, f.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, "queued", "1080x1080", []string{"1080x1080"}))
	}
	message, err = testHandler.creativePlanningCompletionError(t.Context(), parent, testWorkspaceID)
	if err != nil || !strings.Contains(message, "production tasks 6") {
		t.Fatalf("missing delegation=%q %v", message, err)
	}
	for _, id := range ids[6:] {
		addCreativeCandidateOrchestrationProductionTask(t, f, id, "queued", "candidate_primary")
	}
	message, err = testHandler.creativePlanningCompletionError(t.Context(), parent, testWorkspaceID)
	if err != nil || message != "" {
		t.Fatalf("complete plan=%q %v", message, err)
	}
}

func TestCreativeCandidateReadyRecoveryQueuesOnceAndRejectsCancelledOrForeignItem(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 8)
	if p := candidateProgressForTest(t, f); p.State != "selection_ready" {
		t.Fatalf("ready=%+v", p)
	}
	first := candidateRecoveryForTest(t, f, http.StatusOK)
	second := candidateRecoveryForTest(t, f, http.StatusOK)
	if first.TaskID == "" || first.TaskID != second.TaskID {
		t.Fatal("selection duplicated")
	}
	if p := candidateProgressForTest(t, f); p.State != "selection_queued" {
		t.Fatalf("queued=%+v", p)
	}
	foreign := f
	foreign.ItemID = uuid.NewString()
	candidateRecoveryForTest(t, foreign, http.StatusNotFound)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET status='cancelled' WHERE id=$1`, f.OrderID); err != nil {
		t.Fatal(err)
	}
	candidateRecoveryForTest(t, f, http.StatusConflict)
}

func TestCreativeCandidateManualRecoveryAfterAutomaticBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 8)
	for range creativeCandidateSelectionMaxAttempts {
		if queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}); err != nil || !queued {
			t.Fatalf("seed automatic attempt: %v %v", queued, err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='failed',completed_at=now() WHERE trigger_evidence_kind=$1 AND trigger_evidence_ref_id=$2 AND status='queued'`, creativeCandidateSelectionEvidenceKind, f.ItemID); err != nil {
			t.Fatal(err)
		}
	}
	if queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}); err != nil || queued {
		t.Fatalf("automatic budget must remain exhausted: %v %v", queued, err)
	}
	first := candidateRecoveryForTest(t, f, http.StatusOK)
	second := candidateRecoveryForTest(t, f, http.StatusOK)
	if first.TaskID == "" || first.TaskID != second.TaskID {
		t.Fatal("manual recovery did not deduplicate the active task")
	}
	var target, assets, terminal int
	if err := testPool.QueryRow(t.Context(), `SELECT (context->>'target_variant_count')::int FROM agent_task_queue WHERE id=$1`, first.TaskID).Scan(&target); err != nil || target != 6 {
		t.Fatalf("target changed: %d %v", target, err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset a JOIN creative_order_variant v ON v.id=a.variant_id WHERE v.order_item_id=$1`, f.ItemID).Scan(&assets); err != nil || assets != 16 {
		t.Fatalf("primary assets changed: %d %v", assets, err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE trigger_evidence_kind=$1 AND trigger_evidence_ref_id=$2 AND status='failed'`, creativeCandidateSelectionEvidenceKind, f.ItemID).Scan(&terminal); err != nil || terminal != 3 {
		t.Fatalf("history changed: %d %v", terminal, err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='failed',completed_at=now() WHERE id=$1`, first.TaskID); err != nil {
		t.Fatal(err)
	}
	if queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}); err != nil || queued {
		t.Fatalf("manual retry reset automatic budget: %v %v", queued, err)
	}
	third := candidateRecoveryForTest(t, f, http.StatusOK)
	if third.TaskID == "" || third.TaskID == first.TaskID {
		t.Fatal("a new manual action must allow another attempt")
	}
}

func TestCreativeCandidateManualPlanningHasIndependentBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 6)
	parent := seedCandidatePlanTask(t, f, "failed")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt=12,max_attempts=12 WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	first := candidateRecoveryForTest(t, f, http.StatusOK)
	second := candidateRecoveryForTest(t, f, http.StatusOK)
	if first.TaskID != second.TaskID {
		t.Fatal("manual planning duplicated")
	}
	child, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(first.TaskID))
	if err != nil || child.Attempt != 13 || child.MaxAttempts != 14 || child.ParentTaskID != parent.ID || !child.ForceFreshSession {
		t.Fatalf("fresh manual budget: %+v %v", child, err)
	}
	old, err := testHandler.Queries.GetAgentTask(t.Context(), parent.ID)
	if err != nil || old.Attempt != 12 || old.MaxAttempts != 12 || old.Status != "failed" {
		t.Fatalf("manual retry rewrote history: %+v %v", old, err)
	}
}

func TestCreativeCandidateManualSourceRetryHasIndependentBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	parent := seedCandidatePlanTask(t, f, "failed")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt=12,max_attempts=12 WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	retry := func() taskFanoutResponse {
		t.Helper()
		w := httptest.NewRecorder()
		r := withURLParam(newRequest(http.MethodPost, "/retry?trigger_evidence_kind=creative_order_item_plan&trigger_evidence_ref_id="+f.ItemID, nil), "agentId", uuidToString(parent.AgentID))
		testHandler.RetryFailedAgentTasksBySource(w, r)
		if w.Code != http.StatusCreated {
			t.Fatalf("manual source retry: %d %s", w.Code, w.Body.String())
		}
		var response taskFanoutResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}
	first := retry()
	if len(first.Tasks) != 1 {
		t.Fatalf("manual source retry tasks: %+v", first.Tasks)
	}
	child, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(first.Tasks[0].ID))
	if err != nil || child.Attempt != 13 || child.MaxAttempts != 14 || child.RetryOfTaskID != parent.ID || child.RequestingUserID != parseUUID(testUserID) {
		t.Fatalf("manual source retry budget and lineage: %+v %v", child, err)
	}
	if duplicate := retry(); len(duplicate.Tasks) != 0 {
		t.Fatalf("stale parent retried twice: %+v", duplicate.Tasks)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='failed',attempt=max_attempts WHERE id=$1`, child.ID); err != nil {
		t.Fatal(err)
	}
	if next := retry(); len(next.Tasks) != 1 {
		t.Fatalf("second manual retry blocked by prior budget: %+v", next.Tasks)
	}
}

func TestCreativeCandidatePrimeConcurrentCompletionQueuesSelectionAtomically(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	ids := seedSixSetCandidatePrimaries(t, f, 8)
	var claims []creativePrimeCompositionClaim
	for _, id := range ids[6:] {
		if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_prime_composition_job (variant_id,revision,status,composed_at) VALUES ($1,1,'queued',now())`, id); err != nil {
			t.Fatal(err)
		}
		claim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(id), 1)
		if err != nil || !found || !claim.CompositionReady {
			t.Fatalf("claim=%+v %v %v", claim, found, err)
		}
		claims = append(claims, claim)
	}
	if p := candidateProgressForTest(t, f); p.State != "priming" {
		t.Fatalf("priming=%+v", p)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, claim := range claims {
		go func(c creativePrimeCompositionClaim) {
			<-start
			results <- testHandler.completeCreativePrimeCompositionHandoff(ctx, c)
		}(claim)
	}
	close(start)
	for range claims {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var completed, queued int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_prime_composition_job WHERE variant_id=ANY($1::uuid[]) AND status='completed'`, ids[6:]).Scan(&completed); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE trigger_evidence_kind=$1 AND trigger_evidence_ref_id=$2`, creativeCandidateSelectionEvidenceKind, f.ItemID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if completed != 2 || queued != 1 {
		t.Fatalf("completed=%d selection tasks=%d", completed, queued)
	}
	if added, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}); err != nil || added {
		t.Fatalf("replay=%v %v", added, err)
	}
}

func TestCreativeCandidatePrimeSelectionFailureRollsBackCompletion(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	ids := seedSixSetCandidatePrimaries(t, f, 8)
	id := ids[7]
	if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_prime_composition_job (variant_id,revision,status,composed_at) VALUES ($1,1,'queued',now())`, id); err != nil {
		t.Fatal(err)
	}
	claim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(id), 1)
	if err != nil || !found {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_skill SET enabled=false WHERE agent_id=$1`, f.Squad.ReviewerAgentID); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.completeCreativePrimeCompositionHandoff(t.Context(), claim); err == nil {
		t.Fatal("missing reviewer capability accepted")
	}
	var status string
	var lease pgtype.UUID
	if err := testPool.QueryRow(t.Context(), `SELECT status,lease_token FROM creative_prime_composition_job WHERE variant_id=$1`, id).Scan(&status, &lease); err != nil {
		t.Fatal(err)
	}
	if status != "running" || lease != claim.LeaseToken {
		t.Fatalf("failed enqueue lost recoverable claim: %s", status)
	}
}
