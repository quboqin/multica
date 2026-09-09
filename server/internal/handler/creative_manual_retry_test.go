package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreativeManualRetryResumesCancelledContinuation(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	v := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", 1, "running", "1080x1080", []string{"1080x1080"})
	parent := addCreativeCandidateOrchestrationProductionTask(t, f, v, "failed", "candidate_primary")
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), parent); err != nil {
		t.Fatal(err)
	}
	var cancelledID string
	if err := testPool.QueryRow(t.Context(), `UPDATE agent_task_queue SET status='cancelled',completed_at=now() WHERE retry_of_task_id=$1 RETURNING id::text`, parent.ID).Scan(&cancelledID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status='action_required' WHERE id=$1`, v); err != nil {
		t.Fatal(err)
	}
	// The page still exposes the preceding failure, not the cancelled task.
	orderResponse := httptest.NewRecorder()
	testHandler.GetCreativeOrder(orderResponse, withURLParam(newRequest(http.MethodGet, "/order", nil), "id", f.OrderID))
	if orderResponse.Code != http.StatusOK {
		t.Fatalf("get order: %d %s", orderResponse.Code, orderResponse.Body.String())
	}
	var order creativeOrderResponse
	if err := json.Unmarshal(orderResponse.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	blocker := order.Items[0].Variants[0].ActionRequired
	if blocker == nil || !blocker.Retryable || blocker.TaskID != uuidToString(parent.ID) {
		t.Fatalf("page retry target: %+v", blocker)
	}
	setCreativeRetryForTest(t, false)
	click := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		testHandler.RetryCreativeOrderWorkflowFailure(w, withURLParams(newRequest(http.MethodPost, "/retry", nil), "id", f.OrderID, "taskId", blocker.TaskID))
		return w
	}
	w := click()
	if w.Code != http.StatusOK {
		t.Fatalf("retry after cancelled continuation: %d %s", w.Code, w.Body.String())
	}
	var response creativeOrderWorkflowRetryResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	var retryOf, rerunOf string
	var claimable bool
	if err := testPool.QueryRow(t.Context(), `SELECT retry_of_task_id::text,rerun_of_task_id::text,creative_task_retry_allowed(id) FROM agent_task_queue WHERE id=$1`, response.TaskID).Scan(&retryOf, &rerunOf, &claimable); err != nil {
		t.Fatal(err)
	}
	if retryOf != cancelledID || rerunOf != cancelledID || !claimable {
		t.Fatalf("manual retry lost continuation lineage or was paused: retry=%s rerun=%s claimable=%v", retryOf, rerunOf, claimable)
	}
	if duplicate := click(); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate retry accepted: %d %s", duplicate.Code, duplicate.Body.String())
	}
}
