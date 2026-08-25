package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestValidateCreativeProductionTaskContextAcceptsCurrentSizeAdjustment(t *testing.T) {
	variantID := uuid.NewString()
	adjustmentIssueID := uuid.NewString()
	context := map[string]any{
		"variant_id":     variantID,
		"revision":       float64(2),
		"expected_sizes": []any{"1080x1080", "1200x628", "800x1000"},
		"issue_id":       adjustmentIssueID,
		"scope":          "size",
		"order_adjustment": map[string]any{
			"adjustment_issue_id":  adjustmentIssueID,
			"source_revision":      float64(1),
			"target_size":          "1080x1080",
			"source_asset_id":      uuid.NewString(),
			"source_attachment_id": uuid.NewString(),
			"request":              "和主标题避让，调整布局",
		},
	}

	if err := validateCreativeProductionTaskContext(context, variantID+":r2"); err != nil {
		t.Fatalf("current-size adjustment context rejected: %v", err)
	}
	context["scope"] = "variant"
	if err := validateCreativeProductionTaskContext(context, variantID+":r2"); err == nil {
		t.Fatal("current-size adjustment without size scope was accepted")
	}
}

func TestValidateCreativeDirectEditTaskContextRequiresUnbrandedBaseAndTargetScope(t *testing.T) {
	variantID := uuid.NewString()
	issueID := uuid.NewString()
	context := map[string]any{
		"variant_id":              variantID,
		"revision":                float64(2),
		"source_revision":         float64(1),
		"expected_sizes":          []any{"1080x1080", "1200x628", "800x1000"},
		"target_size":             "1080x1080",
		"user_request":            "保留人物，移除右侧竞品标识",
		"delivery_mode":           "publish",
		"source_asset_id":         uuid.NewString(),
		"source_attachment_id":    uuid.NewString(),
		"reference_asset_id":      uuid.NewString(),
		"reference_attachment_id": uuid.NewString(),
		"reviewer_agent_id":       uuid.NewString(),
		"issue_id":                issueID,
		"direct_edit": map[string]any{
			"adjustment_issue_id": issueID,
		},
	}

	if err := validateCreativeDirectEditTaskContext(context, variantID+":r2"); err != nil {
		t.Fatalf("direct edit context rejected: %v", err)
	}
	context["source_asset_id"] = ""
	if err := validateCreativeDirectEditTaskContext(context, variantID+":r2"); err == nil {
		t.Fatal("direct edit context without unbranded source was accepted")
	}
}

func TestClaimAgentTask_DirectFanoutParallelButQuickCreateSerial(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "direct-fanout-claim", []byte(`{}`))
	evidenceID := uuid.NewString()
	insert := func(context string) string {
		var taskID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO agent_task_queue (
				agent_id, runtime_id, status, priority, context,
				trigger_evidence_kind, trigger_evidence_ref_id
			) VALUES ($1, $2, 'queued', 0, $3::jsonb, 'creative_candidate', $4::uuid)
			RETURNING id`, agentID, testRuntimeID, context, evidenceID).Scan(&taskID); err != nil {
			t.Fatalf("insert task: %v", err)
		}
		return taskID
	}
	ids := []string{
		insert(`{"type":"creative_analysis","item_key":"one"}`),
		insert(`{"type":"creative_analysis","item_key":"two"}`),
		insert(`{"type":"quick_create","item_key":"quick-one"}`),
		insert(`{"type":"quick_create","item_key":"quick-two"}`),
	}
	t.Cleanup(func() {
		for _, id := range ids {
			testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, id)
		}
	})

	for i := 0; i < 2; i++ {
		task, err := testHandler.Queries.ClaimAgentTask(ctx, db.ClaimAgentTaskParams{
			AgentID:          parseUUID(agentID),
			PrepareLeaseSecs: 45,
		})
		if err != nil {
			t.Fatalf("claim direct task %d: %v", i, err)
		}
		if _, err := testHandler.Queries.StartAgentTask(ctx, task.ID); err != nil {
			t.Fatalf("start direct task %d: %v", i, err)
		}
	}

	quick, err := testHandler.Queries.ClaimAgentTask(ctx, db.ClaimAgentTaskParams{
		AgentID:          parseUUID(agentID),
		PrepareLeaseSecs: 45,
	})
	if err != nil {
		t.Fatalf("quick-create must claim while direct tasks run: %v", err)
	}
	if _, err := testHandler.Queries.StartAgentTask(ctx, quick.ID); err != nil {
		t.Fatalf("start quick-create: %v", err)
	}
	if _, err := testHandler.Queries.ClaimAgentTask(ctx, db.ClaimAgentTaskParams{
		AgentID:          parseUUID(agentID),
		PrepareLeaseSecs: 45,
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second quick-create claim error = %v, want no rows", err)
	}
}

func TestFanoutAgentTasks_TaskTokenPreservesDelegatingTask(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	delegatorID := createHandlerTestAgent(t, "direct-fanout-delegator", []byte(`{}`))
	targetID := createHandlerTestAgent(t, "direct-fanout-target", []byte(`{}`))
	var parentTaskID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, priority, originator_user_id, accountable_user_id, requesting_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, $2, $2, $2, 'direct_human')
RETURNING id::text
`, delegatorID, testUserID).Scan(&parentTaskID); err != nil {
		t.Fatalf("create delegating task: %v", err)
	}
	childTaskIDs := []string{}
	t.Cleanup(func() {
		for _, taskID := range childTaskIDs {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID)
		}
		_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, parentTaskID)
	})

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/agents/"+targetID+"/tasks/fanout", taskFanoutRequest{
		TriggerEvidenceKind:  "generic_test_evidence",
		TriggerEvidenceRefID: uuid.NewString(),
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: "delegated-item",
			Context: json.RawMessage(`{"type":"creative_analysis"}`),
		}},
	})
	req = withURLParam(req, "agentId", targetID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", delegatorID)
	req.Header.Set("X-Task-ID", parentTaskID)
	testHandler.FanoutAgentTasks(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("task-token fanout: %d %s", w.Code, w.Body.String())
	}
	var response taskFanoutResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Tasks) != 1 {
		t.Fatalf("fanout tasks = %#v", response.Tasks)
	}
	childTaskIDs = append(childTaskIDs, response.Tasks[0].ID)
	var delegatedFrom string
	if err := testPool.QueryRow(ctx, `SELECT COALESCE(delegated_from_task_id::text, '') FROM agent_task_queue WHERE id = $1`, response.Tasks[0].ID).Scan(&delegatedFrom); err != nil {
		t.Fatal(err)
	}
	if delegatedFrom != parentTaskID {
		t.Fatalf("delegated_from_task_id = %q, want %q", delegatedFrom, parentTaskID)
	}
}

func TestFanoutAgentTasks_ReusesActiveTaskAfterUniqueConflict(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID := createHandlerTestAgent(t, "direct-fanout-idempotent", []byte(`{}`))
	evidenceID := uuid.NewString()
	request := taskFanoutRequest{
		TriggerEvidenceKind:  "generic_test_evidence",
		TriggerEvidenceRefID: evidenceID,
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: "same-item",
			Context: json.RawMessage(`{"type":"creative_analysis"}`),
		}},
	}
	call := func() taskFanoutResponse {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/agents/"+agentID+"/tasks/fanout", request)
		req = withURLParam(req, "agentId", agentID)
		testHandler.FanoutAgentTasks(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("fanout: %d %s", w.Code, w.Body.String())
		}
		var response taskFanoutResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		if len(response.Tasks) != 1 {
			t.Fatalf("tasks = %#v", response.Tasks)
		}
		return response
	}
	first := call()
	second := call()
	if second.Tasks[0].ID != first.Tasks[0].ID {
		t.Fatalf("duplicate fanout task = %s, want %s", second.Tasks[0].ID, first.Tasks[0].ID)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, first.Tasks[0].ID)
	})
}

func TestRetryFailedAgentTasksBySourceRetriesOnlyLatestUnrecoveredWithinBudget(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	agentID := createHandlerTestAgent(t, "direct-retry-latest-"+uuid.NewString(), nil)
	evidenceKind := "generic_test_evidence"
	evidenceID := uuid.NewString()
	insert := func(itemKey, status, failureReason string, attempt, maxAttempts, offsetSeconds int) string {
		t.Helper()
		contextValue, err := json.Marshal(map[string]any{"type": "creative_domain_task", "item_key": itemKey})
		if err != nil {
			t.Fatal(err)
		}
		var taskID string
		if err := testPool.QueryRow(ctx, `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  failure_reason, error, completed_at, attempt, max_attempts, created_at
)
VALUES (
  $1, (SELECT runtime_id FROM agent WHERE id = $1), $2, $3::jsonb, $4, $5,
  NULLIF($6, ''), CASE WHEN $2 = 'failed' THEN 'provider returned 429' ELSE NULL END,
  CASE WHEN $2 IN ('completed', 'failed', 'cancelled') THEN now() + ($9 * interval '1 second') ELSE NULL END,
  $7, $8, now() + ($9 * interval '1 second')
)
RETURNING id::text
`, agentID, status, contextValue, evidenceKind, evidenceID, failureReason, attempt, maxAttempts, offsetSeconds).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		return taskID
	}

	openFailureID := insert("open", "failed", "provider_rate_limited", 1, 2, -30)
	recoveredFailureID := insert("recovered", "failed", "provider_rate_limited", 1, 2, -50)
	insert("recovered", "completed", "", 2, 2, -20)
	exhaustedFailureID := insert("exhausted", "failed", "provider_rate_limited", 2, 2, -10)

	if _, err := testHandler.Queries.CreateRetryTask(ctx, parseUUID(exhaustedFailureID)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("CreateRetryTask exhausted error = %v, want no rows", err)
	}

	retry := func() taskFanoutResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/agents/"+agentID+"/tasks/by-source/retry?trigger_evidence_kind="+evidenceKind+"&trigger_evidence_ref_id="+evidenceID, nil)
		req = withURLParam(req, "agentId", agentID)
		testHandler.RetryFailedAgentTasksBySource(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("RetryFailedAgentTasksBySource: %d %s", w.Code, w.Body.String())
		}
		var response taskFanoutResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	response := retry()
	if len(response.Tasks) != 1 {
		t.Fatalf("retried tasks = %#v, want only open failure", response.Tasks)
	}
	childID := response.Tasks[0].ID
	var retryOf string
	if err := testPool.QueryRow(ctx, `SELECT COALESCE(retry_of_task_id::text, '') FROM agent_task_queue WHERE id = $1`, childID).Scan(&retryOf); err != nil {
		t.Fatal(err)
	}
	if retryOf != openFailureID {
		t.Fatalf("retry_of_task_id = %q, want %q", retryOf, openFailureID)
	}
	var wronglyRetried int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM agent_task_queue WHERE retry_of_task_id IN ($1, $2)`, recoveredFailureID, exhaustedFailureID).Scan(&wronglyRetried); err != nil {
		t.Fatal(err)
	}
	if wronglyRetried != 0 {
		t.Fatalf("superseded or exhausted failures retried %d times", wronglyRetried)
	}

	if _, err := testPool.Exec(ctx, `
UPDATE agent_task_queue
SET status = 'failed', failure_reason = 'provider_rate_limited', error = 'provider returned 429', completed_at = now()
WHERE id = $1`, childID); err != nil {
		t.Fatal(err)
	}
	if second := retry(); len(second.Tasks) != 0 {
		t.Fatalf("exhausted latest attempt retried again: %#v", second.Tasks)
	}
}

func TestValidateCreativeTaskFanoutContextRequiresOrderTrace(t *testing.T) {
	ref := parseUUID(uuid.NewString())
	variantID := uuid.NewString()
	valid := []service.DirectTaskFanoutItem{{
		ItemKey: variantID + ":r1",
		Context: json.RawMessage(fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_production",
  "creative_order_id":"%s","issue_id":"%s","leader_agent_id":"%s",
  "creative_order_item_id":"%s","variant_id":"%s","revision":1,
  "expected_sizes":["1080x1080","1200x628","800x1000"]
}`, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuidToString(ref), variantID)),
	}}
	if err := validateCreativeTaskFanoutContext("creative_order_item_production", ref, valid); err != nil {
		t.Fatalf("valid production context: %v", err)
	}
	missingIssueVariantID := uuid.NewString()
	missingIssue := []service.DirectTaskFanoutItem{{
		ItemKey: missingIssueVariantID + ":r1",
		Context: json.RawMessage(fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_production",
  "creative_order_id":"%s","leader_agent_id":"%s",
  "creative_order_item_id":"%s","variant_id":"%s","revision":1,
  "expected_sizes":["1080x1080"]
}`, uuid.NewString(), uuid.NewString(), uuidToString(ref), missingIssueVariantID)),
	}}
	if err := validateCreativeTaskFanoutContext("creative_order_item_production", ref, missingIssue); err == nil {
		t.Fatal("production context without issue_id unexpectedly passed")
	}
	staleRevision := []service.DirectTaskFanoutItem{{
		ItemKey: variantID + ":r0",
		Context: json.RawMessage(fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_production",
  "creative_order_id":"%s","issue_id":"%s","leader_agent_id":"%s",
  "creative_order_item_id":"%s","variant_id":"%s","revision":0,
  "expected_sizes":["1080x1080"]
}`, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuidToString(ref), variantID)),
	}}
	if err := validateCreativeTaskFanoutContext("creative_order_item_production", ref, staleRevision); err == nil {
		t.Fatal("production context with revision 0 unexpectedly passed")
	}
}

func TestNormalizeCreativeProductionFanoutUsesCurrentVariantRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "production fanout canonical revision")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	rawContext, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"creative_order_id":      orderID,
		"issue_id":               uuid.NewString(),
		"leader_agent_id":        uuid.NewString(),
		"creative_order_item_id": itemID,
		"variant_id":             variantID,
		"revision":               0,
		"item_key":               variantID + ":r0",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := normalizeCreativeProductionFanoutItems(t.Context(), testPool, parseUUID(testWorkspaceID), parseUUID(itemID), []service.DirectTaskFanoutItem{{
		ItemKey: variantID + ":r0",
		Context: rawContext,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ItemKey != variantID+":r1" {
		t.Fatalf("normalized item key = %#v", items)
	}
	var normalized map[string]any
	if err := json.Unmarshal(items[0].Context, &normalized); err != nil {
		t.Fatal(err)
	}
	if normalized["revision"] != float64(1) || normalized["item_key"] != variantID+":r1" {
		t.Fatalf("normalized context = %#v", normalized)
	}
	expectedSizes, _ := normalized["expected_sizes"].([]any)
	if len(expectedSizes) != len(standardCreativeAssetSizes) {
		t.Fatalf("expected sizes = %#v", expectedSizes)
	}
}

func TestFanoutAgentTasksCreativeProductionReusesHistoricalPoolAgent(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "production fanout historical pool agent")
	fixture := createCreativeOrderSquadFixture(t, "", "image_edit", true)
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', $2::jsonb, $3) RETURNING id::text
`, testWorkspaceID, `{"squad_snapshot":{"squad_id":"`+fixture.SquadID+`"}}`, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	existingContext, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"creative_order_id":      orderID,
		"creative_order_item_id": itemID,
		"variant_id":             variantID,
		"revision":               1,
		"item_key":               variantID + ":r1",
		"expected_sizes":         standardCreativeAssetSizes,
		"issue_id":               uuid.NewString(),
		"leader_agent_id":        fixture.LeaderAgentID,
		"producer_agent_id":      fixture.DuplicateAgentID,
		"reviewer_agent_id":      fixture.ReviewerAgentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	var existingTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  originator_user_id, accountable_user_id, requesting_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'queued', $2::jsonb,
        'creative_order_item_production', $3, $4, $4, $4, 'direct_human')
RETURNING id::text
`, fixture.DuplicateAgentID, existingContext, itemID, testUserID).Scan(&existingTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, existingTaskID)
	})

	requestContext, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"creative_order_id":      orderID,
		"creative_order_item_id": itemID,
		"variant_id":             variantID,
		"revision":               1,
		"expected_sizes":         standardCreativeAssetSizes,
		"issue_id":               uuid.NewString(),
		"leader_agent_id":        fixture.LeaderAgentID,
		"producer_agent_id":      fixture.ProducerAgentID,
		"reviewer_agent_id":      fixture.ReviewerAgentID,
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPost, "/api/agents/"+fixture.ProducerAgentID+"/tasks/fanout", taskFanoutRequest{
		TriggerEvidenceKind:  "creative_order_item_production",
		TriggerEvidenceRefID: itemID,
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: variantID + ":r1",
			Context: requestContext,
		}},
	}), "agentId", fixture.ProducerAgentID)
	testHandler.FanoutAgentTasks(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("creative production fanout = %d %s", w.Code, w.Body.String())
	}
	var response taskFanoutResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Tasks) != 1 || response.Tasks[0].ID != existingTaskID {
		t.Fatalf("fanout tasks = %#v, want existing %s", response.Tasks, existingTaskID)
	}
}

func TestValidateCreativeProductionFanoutExpectedSizesMatchFrozenVariant(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "production fanout frozen delivery sizes")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{"expected_sizes":["1200x628"]}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	contextValue, err := json.Marshal(map[string]any{
		"variant_id":     variantID,
		"expected_sizes": standardCreativeAssetSizes,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = testHandler.validateCreativeTaskFanoutExpectedSizes(t.Context(), parseUUID(testWorkspaceID), "creative_order_item_production", parseUUID(itemID), []service.DirectTaskFanoutItem{{Context: contextValue}})
	if err == nil || err.Error() != "creative production task context expected_sizes must match the current creative variant delivery sizes" {
		t.Fatalf("production frozen-size validation error = %v", err)
	}
}

func TestNormalizeManualCreativeProductionFanoutAddsOrderTrace(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "manual production fanout trace")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 2, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	rawContext, err := json.Marshal(map[string]any{
		"type":            "creative_domain_task",
		"workflow":        "creative_production",
		"issue_id":        uuid.NewString(),
		"leader_agent_id": uuid.NewString(),
		"variant_id":      variantID,
		"revision":        0,
		"item_key":        variantID + ":r0",
	})
	if err != nil {
		t.Fatal(err)
	}
	items, err := normalizeManualCreativeProductionFanoutItems(t.Context(), testPool, parseUUID(testWorkspaceID), parseUUID(orderID), []service.DirectTaskFanoutItem{{
		ItemKey: variantID + ":r0",
		Context: rawContext,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ItemKey != variantID+":r2" {
		t.Fatalf("normalized manual item = %#v", items)
	}
	var normalized map[string]any
	if err := json.Unmarshal(items[0].Context, &normalized); err != nil {
		t.Fatal(err)
	}
	if normalized["creative_order_id"] != orderID || normalized["creative_order_item_id"] != itemID || normalized["revision"] != float64(2) || normalized["item_key"] != variantID+":r2" {
		t.Fatalf("normalized manual context = %#v", normalized)
	}
}

func TestValidateCreativeQCFanoutContextRequiresLaneAndCompleteTrace(t *testing.T) {
	variantID := uuid.NewString()
	orderID := uuid.NewString()
	itemID := uuid.NewString()
	issueID := uuid.NewString()
	leaderID := uuid.NewString()
	qcItem := func(workflow, itemKey string, expectedSizes any) service.DirectTaskFanoutItem {
		contextValue, err := json.Marshal(map[string]any{
			"type":                   "creative_domain_task",
			"workflow":               workflow,
			"issue_id":               issueID,
			"leader_agent_id":        leaderID,
			"creative_order_id":      orderID,
			"creative_order_item_id": itemID,
			"variant_id":             variantID,
			"revision":               2,
			"expected_sizes":         expectedSizes,
		})
		if err != nil {
			t.Fatal(err)
		}
		return service.DirectTaskFanoutItem{ItemKey: itemKey, Context: contextValue}
	}
	valid := []service.DirectTaskFanoutItem{
		qcItem("creative_qc_technical", variantID+":technical:r2", []string{"1080x1080", "1200x628", "800x1000"}),
		qcItem("creative_qc_visual", variantID+":visual:r2", []string{"1080x1080", "1200x628", "800x1000"}),
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", parseUUID(variantID), valid); err != nil {
		t.Fatalf("valid QC fanout context: %v", err)
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", parseUUID(variantID), []service.DirectTaskFanoutItem{
		qcItem("creative_qc_technical", variantID+":technical:r2", []string{}),
	}); err == nil {
		t.Fatal("QC fanout without expected_sizes unexpectedly passed")
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", parseUUID(variantID), []service.DirectTaskFanoutItem{
		qcItem("creative_qc", variantID+":technical:r2", []string{"1080x1080"}),
	}); err == nil {
		t.Fatal("QC fanout with invalid lane workflow unexpectedly passed")
	}
}

func TestValidateCreativeQCFanoutExpectedSizesMatchFrozenVariant(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "QC fanout frozen delivery sizes")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	makeItem := func(expectedSizes []string, targetSizes []string) service.DirectTaskFanoutItem {
		contextValue, err := json.Marshal(map[string]any{
			"expected_sizes": expectedSizes,
			"qc_visual_rework": map[string]any{
				"target_sizes": targetSizes,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return service.DirectTaskFanoutItem{Context: contextValue}
	}
	partialExpected := makeItem([]string{"1080x1080"}, []string{"1080x1080"})
	err := testHandler.validateCreativeTaskFanoutExpectedSizes(t.Context(), parseUUID(testWorkspaceID), "creative_order_variant_qc", parseUUID(variantID), []service.DirectTaskFanoutItem{partialExpected})
	if err == nil || err.Error() != "creative QC task context expected_sizes must match the current creative variant delivery sizes" {
		t.Fatalf("QC frozen-size validation error = %v", err)
	}
	fullExpected := makeItem(standardCreativeAssetSizes, []string{"1080x1080"})
	if err := testHandler.validateCreativeTaskFanoutExpectedSizes(t.Context(), parseUUID(testWorkspaceID), "creative_order_variant_qc", parseUUID(variantID), []service.DirectTaskFanoutItem{fullExpected}); err != nil {
		t.Fatalf("QC visual rework target subset was rejected: %v", err)
	}
}

func TestFanoutAgentTasksRejectsUnknownCreativeEvidenceKind(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID := createHandlerTestAgent(t, "reject-unknown-creative-evidence", nil)
	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPost, "/api/agents/"+agentID+"/tasks/fanout", taskFanoutRequest{
		TriggerEvidenceKind:  "creative_future_workflow",
		TriggerEvidenceRefID: uuid.NewString(),
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: "future",
			Context: json.RawMessage(`{"type":"creative_domain_task"}`),
		}},
	}), "agentId", agentID)
	testHandler.FanoutAgentTasks(w, req)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unsupported creative trigger_evidence_kind") {
		t.Fatalf("unknown creative evidence = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeTaskRequiredCapability(t *testing.T) {
	tests := map[string]string{
		"creative_crawl_run_analysis":     "reference_analysis",
		"creative_order_item_plan":        "generation_plan",
		"creative_order_item_production":  "image_edit",
		"creative_order_variant_qc":       "quality_control",
		"creative_order_item_direct_edit": "direct_image_edit",
		"generic_test_evidence":           "",
	}
	for kind, want := range tests {
		if got := creativeTaskRequiredCapability(kind); got != want {
			t.Errorf("creativeTaskRequiredCapability(%q) = %q, want %q", kind, got, want)
		}
	}
}
