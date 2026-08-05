package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreativeOrderQCAgentSnapshotRequiresFrozenLeaderAndReviewer(t *testing.T) {
	leaderID := uuid.NewString()
	reviewerID := uuid.NewString()
	leader, reviewer, err := creativeOrderQCAgentSnapshot(json.RawMessage(`{"squad_snapshot":{"leader_agent_id":"` + leaderID + `","reviewer_agent_id":"` + reviewerID + `"}}`))
	if err != nil || uuidToString(leader) != leaderID || uuidToString(reviewer) != reviewerID {
		t.Fatalf("valid frozen snapshot = leader %q reviewer %q err %v", uuidToString(leader), uuidToString(reviewer), err)
	}
	if _, _, err := creativeOrderQCAgentSnapshot(json.RawMessage(`{"squad_snapshot":{"leader_agent_id":"` + leaderID + `"}}`)); err == nil {
		t.Fatal("missing reviewer must not be accepted")
	}
	if _, _, err := creativeOrderQCAgentSnapshot(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("malformed snapshot must not be accepted")
	}
}

func creativeQCTaskContextForTest(t *testing.T, orderID, variantID, lane string) []byte {
	t.Helper()
	contextValue, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_" + lane,
		"creative_order_id": orderID, "variant_id": variantID, "revision": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return contextValue
}

func TestCreateCreativeSourceAnalysisRequiresCrawlRunEvidence(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "source analysis")
	var runID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_crawl_run (workspace_id, issue_id, connector_id, status, created_by_type, created_by_id)
VALUES ($1, $2, 'test', 'completed', 'member', $3) RETURNING id::text`, testWorkspaceID, issueID, testUserID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_material_crawl_run_candidate (run_id, candidate_id, workspace_id)
VALUES ($1, $2, $3)`, runID, candidateID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_material_issue_candidate
SET source_run_id = $3
WHERE issue_id = $1 AND candidate_id = $2`, issueID, candidateID, runID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/source-analyses", creativeSourceAnalysisInput{
		CandidateID: candidateID, AnalysisVersion: 1, Status: "completed", Summary: "Analysis completed",
		Result: json.RawMessage(`{"detected_text":["hello"]}`), TriggerEvidenceKind: "crawl_run", TriggerEvidenceReference: runID,
	})
	testHandler.CreateCreativeSourceAnalysis(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeSourceAnalysis: %d %s", w.Code, w.Body.String())
	}
	var analysis creativeSourceAnalysisResponse
	if err := json.NewDecoder(w.Body).Decode(&analysis); err != nil {
		t.Fatal(err)
	}
	if analysis.TriggerEvidenceReference != runID || analysis.Status != "completed" {
		t.Fatalf("analysis = %#v", analysis)
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT analysis_status FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2`, issueID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("analysis status = %q, want completed", status)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT analysis_status FROM creative_material_crawl_run_candidate WHERE run_id = $1 AND candidate_id = $2`, runID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "completed" {
		t.Fatalf("crawl run analysis status = %q, want completed", status)
	}
}

func TestCreativeOrderDetailReturnsAssetLineageQCAndPartialStatus(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "order detail")
	var orderID, itemID, completedVariantID, failedVariantID, assetID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, status)
VALUES ($1, 'v01', 'completed') RETURNING id::text`, itemID).Scan(&completedVariantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, status)
VALUES ($1, 'v02', 'failed') RETURNING id::text`, itemID).Scan(&failedVariantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '1080x1080', 1, 'delivered', '{"model_request_id":"req-1"}'::jsonb, '{"prime_manifest":"manifest-1"}'::jsonb, 'completed')
RETURNING id::text`, completedVariantID).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'passed', '{"checks":["dimensions"]}'::jsonb),
       ($1, 'visual', 1, 'warning', '{"warnings":["small logo"]}'::jsonb)`, completedVariantID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil)
	req = withURLParam(req, "id", orderID)
	testHandler.GetCreativeOrder(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
	}
	var order creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	if order.DerivedStatus != "awaiting_adoption" {
		t.Fatalf("derived status = %q, want awaiting_adoption", order.DerivedStatus)
	}
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 2 {
		t.Fatalf("order detail missing variants: %#v", order.Items)
	}
	var completed *creativeOrderVariantResponse
	for index := range order.Items[0].Variants {
		if order.Items[0].Variants[index].ID == completedVariantID {
			completed = &order.Items[0].Variants[index]
			break
		}
	}
	if completed == nil || len(completed.Assets) != 1 || completed.Assets[0].ID != assetID || len(completed.QCReports) != 2 {
		t.Fatalf("order detail missing asset lineage or QC reports: %#v", completed)
	}
	if string(completed.Assets[0].Metadata) == "{}" || string(completed.Assets[0].Evidence) == "{}" {
		t.Fatalf("asset lineage metadata/evidence missing: %#v", completed.Assets[0])
	}
	_ = failedVariantID
}

func TestCreativeOrderDerivedStatusSeparatesReviewFromActiveGeneration(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "derived order state")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'queued', '{}'::jsonb, $3) RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, status)
VALUES ($1, 'V01', 'completed'), ($1, 'V02', 'action_required')
`, itemID); err != nil {
		t.Fatal(err)
	}

	status, err := testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID))
	if err != nil || status != "awaiting_adoption" {
		t.Fatalf("terminal mixed order status = %q, %v; want awaiting_adoption", status, err)
	}

	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'action_required' WHERE order_item_id = $1`, itemID); err != nil {
		t.Fatal(err)
	}
	status, err = testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID))
	if err != nil || status != "action_required" {
		t.Fatalf("stopped blocked order status = %q, %v; want action_required", status, err)
	}

	agentID := createHandlerTestAgent(t, "creative-active-state-"+uuid.NewString(), nil)
	contextValue, err := json.Marshal(map[string]any{"type": "creative_domain_task", "creative_order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, context)
VALUES ($1, $2, $3, 'queued', $4::jsonb)
`, agentID, handlerTestRuntimeID(t), issueID, contextValue); err != nil {
		t.Fatal(err)
	}
	status, err = testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID))
	if err != nil || status != "partial" {
		t.Fatalf("actively progressing mixed order status = %q, %v; want partial", status, err)
	}
}

func TestCancelCreativeOrderStopsTasksPreservesHistoryAndFencesWrites(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "cancel creative order")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'queued', '{}'::jsonb, $3) RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-cancel-state-"+uuid.NewString(), nil)
	contextValue, err := json.Marshal(map[string]any{"type": "creative_domain_task", "creative_order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	var taskID, directTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, context)
VALUES ($1, $2, $3, 'queued', $4::jsonb) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), issueID, contextValue).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context)
VALUES ($1, $2, 'waiting_local_directory', $3::jsonb) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), contextValue).Scan(&directTaskID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/cancel", nil), "id", orderID)
	testHandler.CancelCreativeOrder(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("CancelCreativeOrder = %d %s", w.Code, w.Body.String())
	}
	var order creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	if order.Status != "cancelled" || order.DerivedStatus != "cancelled" || len(order.Items) != 1 {
		t.Fatalf("cancelled order = %#v", order)
	}
	var taskStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, taskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "cancelled" {
		t.Fatalf("task status = %q, want cancelled", taskStatus)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, directTaskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "cancelled" {
		t.Fatalf("direct task status = %q, want cancelled", taskStatus)
	}
	var activityCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_order_cancelled'`, issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("creative order cancellation activity count = %d, want 1", activityCount)
	}

	w = httptest.NewRecorder()
	req = withURLParam(newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "V01", Revision: 1, Status: "queued", Brief: json.RawMessage(`{}`),
	}), "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatalf("write to cancelled order = %d %s, want 409", w.Code, w.Body.String())
	}
}

func TestCreativeQCStatusAllowsAdoption(t *testing.T) {
	tests := map[string]bool{
		"passed":  true,
		"warning": true,
		"failed":  false,
		"pending": false,
		"":        false,
	}
	for status, want := range tests {
		if got := creativeQCStatusAllowsAdoption(status); got != want {
			t.Errorf("creativeQCStatusAllowsAdoption(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestAdoptCreativeOrderItemVariantSwitchesOneSelectionAndRecordsAudit(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "variant adoption")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'completed', '{}'::jsonb, $3) RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'completed') RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	var variantIDs []string
	createEligibleVariant := func(key, technicalStatus, visualStatus string) string {
		var variantID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, $2, 1, 'completed') RETURNING id::text
`, itemID, key).Scan(&variantID); err != nil {
			t.Fatal(err)
		}
		variantIDs = append(variantIDs, variantID)
		for _, size := range standardCreativeAssetSizes {
			var attachmentID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, 'image/png', 1024) RETURNING id::text
`, testWorkspaceID, issueID, testUserID, key+"-"+size+".png", "/uploads/"+key+"-"+size+".png").Scan(&attachmentID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed'),
       ($1, $2, 1, 'delivered', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, $2, '{}'::jsonb),
       ($1, 'visual', 1, $3, '{}'::jsonb)
`, variantID, technicalStatus, visualStatus); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'delivered', $2)
`, variantID, issueID); err != nil {
			t.Fatal(err)
		}
		return variantID
	}
	firstVariantID := createEligibleVariant("adopt-v01", "passed", "passed")
	secondVariantID := createEligibleVariant("adopt-v02", "warning", "passed")
	if status, err := testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID)); err != nil || status != "awaiting_adoption" {
		t.Fatalf("status before adoption = %q, %v; want awaiting_adoption", status, err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_feedback_event WHERE subject_id = ANY($1::uuid[])`, variantIDs)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_adopted'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE workspace_id = $1 AND filename LIKE 'adopt-v0%'`, testWorkspaceID)
	})

	adopt := func(variantID string) creativeOrderItemResponse {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: variantID})
		req = withURLParams(req, "id", orderID, "itemId", itemID)
		testHandler.AdoptCreativeOrderItemVariant(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("adopt variant %s = %d %s", variantID, w.Code, w.Body.String())
		}
		var item creativeOrderItemResponse
		if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
			t.Fatal(err)
		}
		return item
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET updated_at = now() - interval '1 hour' WHERE id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	var updatedBefore time.Time
	if err := testPool.QueryRow(t.Context(), `SELECT updated_at FROM creative_order WHERE id = $1`, orderID).Scan(&updatedBefore); err != nil {
		t.Fatal(err)
	}
	first := adopt(firstVariantID)
	if first.AdoptedVariantID != firstVariantID || first.AdoptedAt == "" || first.AdoptedBy != testUserID {
		t.Fatalf("first adoption = %#v", first)
	}
	if status, err := testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID)); err != nil || status != "completed" {
		t.Fatalf("status after adoption = %q, %v; want completed", status, err)
	}
	var updatedAfter time.Time
	if err := testPool.QueryRow(t.Context(), `SELECT updated_at FROM creative_order WHERE id = $1`, orderID).Scan(&updatedAfter); err != nil {
		t.Fatal(err)
	}
	if !updatedAfter.After(updatedBefore) {
		t.Fatalf("order updated_at after adoption = %s, want after %s", updatedAfter, updatedBefore)
	}
	second := adopt(secondVariantID)
	if second.AdoptedVariantID != secondVariantID {
		t.Fatalf("switched adoption = %#v", second)
	}
	_ = adopt(secondVariantID)

	var feedbackCount, activityCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_feedback_event
WHERE workspace_id = $1 AND subject_type = 'variant' AND decision = 'accepted'
  AND subject_id = ANY($2::uuid[])
`, testWorkspaceID, variantIDs).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_adopted'
`, issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if feedbackCount != 2 || activityCount != 2 {
		t.Fatalf("audit counts = feedback %d activity %d, want 2 each", feedbackCount, activityCount)
	}
}

func TestAdoptCreativeOrderItemVariantRejectsCrossItemAndIncompletePackage(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "invalid variant adoption")
	_, otherCandidateID := createCreativeFeedbackCandidate(t, "cross-item variant adoption")
	var orderID, itemID, otherItemID, variantID, otherVariantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'completed', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE workspace_id = $1 AND filename LIKE 'incomplete-prime-%'`, testWorkspaceID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot) VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot) VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, otherCandidateID).Scan(&otherItemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'incomplete-v01', 1, 'completed') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'other-v01', 1, 'completed') RETURNING id::text
`, otherItemID).Scan(&otherVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'passed', '{}'::jsonb), ($1, 'visual', 1, 'passed', '{}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome)
VALUES ($1, 1, 'delivered')
`, variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		var attachmentID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, 'image/png', 1024) RETURNING id::text
`, testWorkspaceID, issueID, testUserID, "incomplete-prime-"+size+".png", "/uploads/incomplete-prime-"+size+".png").Scan(&attachmentID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}

	requestAdoption := func(targetVariantID string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: targetVariantID})
		req = withURLParams(req, "id", orderID, "itemId", itemID)
		testHandler.AdoptCreativeOrderItemVariant(w, req)
		return w
	}
	if w := requestAdoption(otherVariantID); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "does not belong") {
		t.Fatalf("cross-item adoption = %d %s", w.Code, w.Body.String())
	}
	if w := requestAdoption(variantID); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "delivery package is incomplete") {
		t.Fatalf("incomplete package adoption = %d %s", w.Code, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
SELECT variant_id, size_key, revision, 'delivered', attachment_id, 'completed'
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'primed'
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
DELETE FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'primed' AND size_key = '800x1000'
`, variantID); err != nil {
		t.Fatal(err)
	}
	if w := requestAdoption(variantID); w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "Prime package is incomplete") || !strings.Contains(w.Body.String(), "800x1000") {
		t.Fatalf("incomplete Prime package adoption = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeOrderWorkflowFailuresExposeOnlyLatestUnrecoveredTasks(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "workflow failure visibility")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}

	agentID := createHandlerTestAgent(t, "creative-workflow-failure-"+uuid.NewString(), nil)
	evidenceKind := "creative_order_item_production"
	insertTask := func(status, itemKey, contextType, contextOrderID, failureReason, taskError string, createdOffsetSeconds, attempt, maxAttempts int) string {
		t.Helper()
		contextValue, err := json.Marshal(map[string]any{
			"type":                   contextType,
			"workflow":               "creative_production",
			"creative_order_id":      contextOrderID,
			"creative_order_item_id": itemID,
			"scope":                  "order_item",
			"subject_id":             itemID,
			"item_key":               itemKey,
		})
		if err != nil {
			t.Fatal(err)
		}
		var taskID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  failure_reason, error, completed_at, attempt, max_attempts, created_at
)
VALUES (
  $1, $2, $3, $4::jsonb, $5, $6,
  NULLIF($7, ''), NULLIF($8, ''),
  CASE WHEN $3 IN ('completed', 'failed', 'cancelled') THEN now() + ($9 * interval '1 second') ELSE NULL END,
  $10, $11, now() + ($9 * interval '1 second')
)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), status, contextValue, evidenceKind, itemID, failureReason, taskError, createdOffsetSeconds, attempt, maxAttempts).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		return taskID
	}

	openItemKey := itemID + ":open:r1"
	openTaskID := insertTask("failed", openItemKey, "creative_domain_task", orderID, "timeout", "image provider timed out", -40, 1, 2)
	for index, recoveredStatus := range []string{"queued", "running", "completed"} {
		itemKey := itemID + ":recovered-" + recoveredStatus
		insertTask("failed", itemKey, "creative_domain_task", orderID, "timeout", "old failure", -120-index, 1, 2)
		insertTask(recoveredStatus, itemKey, "creative_domain_task", orderID, "", "", -80-index, 2, 2)
	}
	insertTask("failed", itemID+":wrong-type", "quick_create", orderID, "timeout", "wrong task type", -20, 1, 2)
	insertTask("failed", itemID+":wrong-order", "creative_domain_task", uuid.NewString(), "timeout", "wrong order", -10, 1, 2)

	getOrder := func() creativeOrderResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil)
		req = withURLParam(req, "id", orderID)
		testHandler.GetCreativeOrder(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
		}
		var order creativeOrderResponse
		if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
			t.Fatal(err)
		}
		return order
	}

	order := getOrder()
	if order.DerivedStatus != "partial" {
		t.Fatalf("derived status = %q, want partial while a sibling retry is running", order.DerivedStatus)
	}
	if len(order.WorkflowFailures) != 1 {
		t.Fatalf("workflow failures = %#v, want one open failure", order.WorkflowFailures)
	}
	failure := order.WorkflowFailures[0]
	if failure.TaskID != openTaskID || failure.AgentID != agentID || failure.Workflow != "creative_production" ||
		failure.Scope != "order_item" || failure.SubjectID != itemID || failure.ItemKey != openItemKey {
		t.Fatalf("workflow failure identity = %#v", failure)
	}
	if failure.TriggerEvidenceKind != evidenceKind || failure.TriggerEvidenceReference != itemID ||
		failure.FailureReason != "timeout" || failure.Error != "image provider timed out" || failure.FailedAt == "" || !failure.Retryable {
		t.Fatalf("workflow failure details = %#v", failure)
	}

	w := httptest.NewRecorder()
	testHandler.ListCreativeOrders(w, newRequest(http.MethodGet, "/api/creative/orders", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeOrders: %d %s", w.Code, w.Body.String())
	}
	var listed struct {
		Orders []creativeOrderResponse `json:"orders"`
	}
	if err := json.NewDecoder(w.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	var listedOrder *creativeOrderResponse
	for index := range listed.Orders {
		if listed.Orders[index].ID == orderID {
			listedOrder = &listed.Orders[index]
			break
		}
	}
	if listedOrder == nil || listedOrder.DerivedStatus != "partial" || len(listedOrder.WorkflowFailures) != 1 {
		t.Fatalf("list response did not expose open workflow failure: %#v", listedOrder)
	}

	insertTask("queued", openItemKey, "creative_domain_task", orderID, "", "", 0, 2, 2)
	order = getOrder()
	if order.DerivedStatus != "running" {
		t.Fatalf("recovered derived status = %q, want running while another retry is running", order.DerivedStatus)
	}
	if order.WorkflowFailures == nil || len(order.WorkflowFailures) != 0 {
		t.Fatalf("recovered workflow failures = %#v, want empty array", order.WorkflowFailures)
	}
}

func TestCreativeOrderWorkflowFailuresExposeCompletedTaskReportedActionRequired(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "completed task reported action required")
	var orderID, itemID, variantID, taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, brief, status)
VALUES ($1, 'v01', '{"persistent":"keep","error_code":"missing_api_key","error_message":"OPENAI_API_KEY is required"}'::jsonb, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}

	agentID := createHandlerTestAgent(t, "creative-reported-action-"+uuid.NewString(), nil)
	contextValue, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r1",
		"expected_sizes": []string{"1080x1080", "1200x628", "800x1000"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts, session_id, work_dir
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_item_production', $4, now(), 1, 2, 'old-session', 'old-work-dir')
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), contextValue, itemID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM task_message WHERE task_id = $1`, taskID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1 OR retry_of_task_id = $1`, taskID)
	})
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_message (task_id, seq, type, content)
VALUES ($1, 1, 'text', 'initial diagnostic'),
       ($1, 2, 'tool', 'ignored tool output'),
       ($1, 3, 'text', '   '),
       ($1, 4, 'text', 'OPENAI_API_KEY is required for image generation')
`, taskID); err != nil {
		t.Fatal(err)
	}

	getOrder := func() creativeOrderResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil), "id", orderID)
		testHandler.GetCreativeOrder(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
		}
		var order creativeOrderResponse
		if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
			t.Fatal(err)
		}
		return order
	}

	order := getOrder()
	if order.DerivedStatus != "action_required" || len(order.WorkflowFailures) != 1 {
		t.Fatalf("reported action-required order = %#v", order)
	}
	failure := order.WorkflowFailures[0]
	if failure.TaskID != taskID || failure.FailureReason != "agent_reported_action_required" || failure.Error != "OPENAI_API_KEY is required for image generation" || !failure.Retryable {
		t.Fatalf("reported action-required failure = %#v", failure)
	}

	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = max_attempts WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	if failure := getOrder().WorkflowFailures[0]; failure.Retryable {
		t.Fatalf("exhausted reported action-required failure is retryable: %#v", failure)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = 1 WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRecorder()
	retryRequest := withURLParams(
		newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/workflow-failures/"+taskID+"/retry", nil),
		"id", orderID, "taskId", taskID,
	)
	testHandler.RetryCreativeOrderWorkflowFailure(retry, retryRequest)
	if retry.Code != http.StatusOK {
		t.Fatalf("RetryCreativeOrderWorkflowFailure: %d %s", retry.Code, retry.Body.String())
	}
	var retried creativeOrderWorkflowRetryResponse
	if err := json.NewDecoder(retry.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	if retried.TaskID == "" {
		t.Fatalf("retry response did not include the queued task: %#v", retried)
	}
	var child struct {
		AgentID       string
		Status        string
		Attempt       int
		RetryOfTaskID string
		Context       []byte
		SessionID     string
		WorkDir       string
		FreshSession  bool
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT agent_id::text, status, attempt, retry_of_task_id::text, context,
  COALESCE(session_id, ''), COALESCE(work_dir, ''), force_fresh_session
FROM agent_task_queue WHERE id = $1
`, retried.TaskID).Scan(&child.AgentID, &child.Status, &child.Attempt, &child.RetryOfTaskID, &child.Context, &child.SessionID, &child.WorkDir, &child.FreshSession); err != nil {
		t.Fatal(err)
	}
	var childContext, parentContext map[string]any
	if err := json.Unmarshal(child.Context, &childContext); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contextValue, &parentContext); err != nil {
		t.Fatal(err)
	}
	if child.AgentID != agentID || child.Status != "queued" || child.Attempt != 2 || child.RetryOfTaskID != taskID || child.SessionID != "" || child.WorkDir != "" || !child.FreshSession || !reflect.DeepEqual(childContext, parentContext) {
		t.Fatalf("recovery child = %#v, want same direct task context with retry lineage", child)
	}
	var variantStatus, variantBrief string
	if err := testPool.QueryRow(t.Context(), `SELECT status, brief::text FROM creative_order_variant WHERE id = $1`, variantID).Scan(&variantStatus, &variantBrief); err != nil {
		t.Fatal(err)
	}
	var brief map[string]any
	if err := json.Unmarshal([]byte(variantBrief), &brief); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "running" || brief["persistent"] != "keep" || brief["error_code"] != nil || brief["error_message"] != nil {
		t.Fatalf("retry reset variant = status %q, brief %#v", variantStatus, brief)
	}
	if recovered := getOrder(); recovered.DerivedStatus != "queued" || len(recovered.WorkflowFailures) != 0 {
		t.Fatalf("recovered order = %#v, want a queued retry without current failure", recovered)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'completed' WHERE id = $1`, variantID); err != nil {
		t.Fatal(err)
	}
	if failures := getOrder().WorkflowFailures; len(failures) != 0 {
		t.Fatalf("completed variant retained reported failure: %#v", failures)
	}
}

func TestRetryCreativeOrderWorkflowFailureRejectsSingleLaneQCRetry(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, workflow := range []string{"creative_qc_technical", "creative_qc_visual", "creative_qc"} {
		t.Run(workflow, func(t *testing.T) {
			_, candidateID := createCreativeFeedbackCandidate(t, "atomic QC retry "+workflow)
			var orderID, itemID, variantID, taskID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
				t.Fatal(err)
			}
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, brief, status)
VALUES ($1, 'v01', '{"persistent":"keep","error_code":"qc_failed"}'::jsonb, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
				t.Fatal(err)
			}

			agentID := createHandlerTestAgent(t, "atomic-qc-retry-"+uuid.NewString(), nil)
			contextValue, err := json.Marshal(map[string]any{
				"type": "creative_domain_task", "workflow": workflow, "creative_order_id": orderID,
				"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r1",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_variant_qc', $4, now(), 1, 2)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), contextValue, variantID).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1 OR retry_of_task_id = $1`, taskID)
			})

			w := httptest.NewRecorder()
			req := withURLParams(
				newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/workflow-failures/"+taskID+"/retry", nil),
				"id", orderID, "taskId", taskID,
			)
			testHandler.RetryCreativeOrderWorkflowFailure(w, req)
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "atomic dual-lane recovery") {
				t.Fatalf("single-lane %s retry = %d %s", workflow, w.Code, w.Body.String())
			}

			var variantStatus, variantBrief string
			var childCount int
			if err := testPool.QueryRow(t.Context(), `SELECT status, brief::text FROM creative_order_variant WHERE id = $1`, variantID).Scan(&variantStatus, &variantBrief); err != nil {
				t.Fatal(err)
			}
			if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE retry_of_task_id = $1`, taskID).Scan(&childCount); err != nil {
				t.Fatal(err)
			}
			if variantStatus != "action_required" || !strings.Contains(variantBrief, "qc_failed") || childCount != 0 {
				t.Fatalf("blocked QC retry mutated state: status=%q brief=%s children=%d", variantStatus, variantBrief, childCount)
			}
		})
	}
}

func TestCreativeOrderWorkflowFailuresIgnoreCompletedProductionWithAllExpectedGeneratedAssets(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "completed production asset completeness")
	var orderID, itemID, variantID, productionTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v01', 2, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	productionAgentID := createHandlerTestAgent(t, "creative-production-complete-"+uuid.NewString(), nil)
	expectedSizes := []string{"1080x1080", "1200x628", "800x1000"}
	productionContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r2",
		"expected_sizes": expectedSizes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_item_production', $4, now(), 1, 2)
RETURNING id::text
`, productionAgentID, handlerTestRuntimeID(t), productionContext, itemID).Scan(&productionTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM task_message WHERE task_id = $1`, productionTaskID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE context->>'creative_order_id' = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_message (task_id, seq, type, content)
VALUES ($1, 1, 'text', 'OPENAI_API_KEY is required for image generation')
`, productionTaskID); err != nil {
		t.Fatal(err)
	}
	for _, size := range expectedSizes {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, $2, 2, 'generated', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size); err != nil {
			t.Fatal(err)
		}
	}
	getOrder := func() creativeOrderResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil), "id", orderID)
		testHandler.GetCreativeOrder(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
		}
		var order creativeOrderResponse
		if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
			t.Fatal(err)
		}
		return order
	}
	if failures := getOrder().WorkflowFailures; len(failures) != 0 {
		t.Fatalf("complete production assets synthesized a failure: %#v", failures)
	}
	if _, err := testPool.Exec(t.Context(), `
DELETE FROM creative_order_asset
WHERE variant_id = $1 AND revision = 2 AND stage = 'generated' AND size_key = '800x1000'
`, variantID); err != nil {
		t.Fatal(err)
	}
	if failures := getOrder().WorkflowFailures; len(failures) != 1 || failures[0].TaskID != productionTaskID || failures[0].FailureReason != "agent_reported_action_required" {
		t.Fatalf("incomplete production assets did not retain the reported failure: %#v", failures)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '800x1000', 2, 'generated', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID); err != nil {
		t.Fatal(err)
	}
	primeAgentID := createHandlerTestAgent(t, "creative-prime-active-"+uuid.NewString(), nil)
	primeContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_prime", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r2:prime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, $2, 'queued', $3::jsonb, 'creative_order_variant_prime', $4)
`, primeAgentID, handlerTestRuntimeID(t), primeContext, variantID); err != nil {
		t.Fatal(err)
	}
	order := getOrder()
	if len(order.WorkflowFailures) != 0 || order.DerivedStatus != "partial" {
		t.Fatalf("active Prime should surface as partial/generating without a production failure: %#v", order)
	}
}

func TestCreativeOrderWorkflowFailuresIgnoreCompletedPrimeWithCurrentPrimedAssets(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "completed Prime asset completeness")
	var orderID, itemID, variantID, primeTaskID, qcTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v01', 2, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	primeAgentID := createHandlerTestAgent(t, "creative-prime-complete-"+uuid.NewString(), nil)
	primeContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_prime", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r2:prime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_variant_prime', $4, now(), 1, 2)
RETURNING id::text
`, primeAgentID, handlerTestRuntimeID(t), primeContext, variantID).Scan(&primeTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_message (task_id, seq, type, content)
VALUES ($1, 1, 'text', 'Prime reported action required before downstream QC')
`, primeTaskID); err != nil {
		t.Fatal(err)
	}
	qcAgentID := createHandlerTestAgent(t, "creative-qc-downstream-"+uuid.NewString(), nil)
	qcContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_visual", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":visual:r2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  failure_reason, error, completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'failed', $3::jsonb, 'creative_order_variant_qc', $4, 'agent_error', 'visual QC failed', now(), 1, 2)
RETURNING id::text
`, qcAgentID, handlerTestRuntimeID(t), qcContext, variantID).Scan(&qcTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM task_message WHERE task_id = $1`, primeTaskID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE context->>'creative_order_id' = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})

	for _, revision := range []int{1, 2} {
		sizes := standardCreativeAssetSizes
		if revision == 2 {
			sizes = standardCreativeAssetSizes[:2]
		}
		for _, size := range sizes {
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, $2, $3, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size, revision); err != nil {
				t.Fatal(err)
			}
		}
	}
	listFailures := func() []creativeOrderWorkflowFailureResponse {
		t.Helper()
		failures, err := testHandler.listCreativeOrderWorkflowFailures(newRequest(http.MethodGet, "/", nil), parseUUID(orderID))
		if err != nil {
			t.Fatal(err)
		}
		return failures
	}
	failures := listFailures()
	if len(failures) != 2 {
		t.Fatalf("incomplete current Prime assets failures = %#v, want Prime and downstream QC", failures)
	}
	workflows := map[string]string{}
	for _, failure := range failures {
		workflows[failure.Workflow] = failure.TaskID
	}
	if workflows["creative_prime"] != primeTaskID || workflows["creative_qc_visual"] != qcTaskID {
		t.Fatalf("incomplete current Prime assets failures = %#v, want Prime and downstream QC", failures)
	}

	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '800x1000', 2, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID); err != nil {
		t.Fatal(err)
	}
	failures = listFailures()
	if len(failures) != 1 || failures[0].TaskID != qcTaskID || failures[0].Workflow != "creative_qc_visual" {
		t.Fatalf("complete current Prime assets mislabeled downstream QC failure: %#v", failures)
	}
}

func TestCreativeOrderWorkflowFailuresRespectSiblingProgressAndCompletion(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	tests := []struct {
		variantStatus string
		wantStatus    string
	}{
		{variantStatus: "running", wantStatus: "action_required"},
		{variantStatus: "completed", wantStatus: "awaiting_adoption"},
	}
	for _, test := range tests {
		t.Run(test.variantStatus, func(t *testing.T) {
			_, candidateID := createCreativeFeedbackCandidate(t, "workflow status "+test.variantStatus)
			var orderID, itemID, variantID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
				t.Fatal(err)
			}
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, status)
VALUES ($1, 'v01', $2) RETURNING id::text`, itemID, test.variantStatus).Scan(&variantID); err != nil {
				t.Fatal(err)
			}

			agentID := createHandlerTestAgent(t, "creative-workflow-status-"+test.variantStatus+"-"+uuid.NewString(), nil)
			contextValue, err := json.Marshal(map[string]any{
				"type": "creative_domain_task", "workflow": "creative_prime", "creative_order_id": orderID,
				"creative_order_item_id": itemID, "variant_id": variantID, "item_key": variantID + ":r1",
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  failure_reason, error, completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'failed', $3::jsonb, 'creative_order_variant_prime', $4, 'agent_error', 'prime failed', now(), 1, 2)
`, agentID, handlerTestRuntimeID(t), contextValue, variantID); err != nil {
				t.Fatal(err)
			}

			w := httptest.NewRecorder()
			req := newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil)
			req = withURLParam(req, "id", orderID)
			testHandler.GetCreativeOrder(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
			}
			var order creativeOrderResponse
			if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
				t.Fatal(err)
			}
			if order.DerivedStatus != test.wantStatus {
				t.Fatalf("derived status = %q, want %q", order.DerivedStatus, test.wantStatus)
			}
			if len(order.WorkflowFailures) != 1 || order.WorkflowFailures[0].Scope != "variant" || order.WorkflowFailures[0].SubjectID != variantID {
				t.Fatalf("inferred workflow failure scope = %#v", order.WorkflowFailures)
			}
		})
	}
}

func TestCreativeOrderQCDerivesFromCurrentVariantRevision(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "qc revision")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by) VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot) VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status) VALUES ($1, 'v01', 2, 'partial') RETURNING id::text`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{}'::jsonb),
	       ($1, 'visual', 1, 'failed', '{}'::jsonb)`, variantID); err != nil {
		t.Fatal(err)
	}
	status, err := testHandler.derivedCreativeVariantQCStatus(newRequest(http.MethodGet, "/", nil), parseUUID(variantID))
	if err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("current revision QC status before reports = %q, want pending", status)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 2, 'passed', '{}'::jsonb),
	       ($1, 'visual', 2, 'passed', '{}'::jsonb)`, variantID); err != nil {
		t.Fatal(err)
	}
	status, err = testHandler.derivedCreativeVariantQCStatus(newRequest(http.MethodGet, "/", nil), parseUUID(variantID))
	if err != nil {
		t.Fatal(err)
	}
	if status != "passed" {
		t.Fatalf("current revision QC status = %q, want passed", status)
	}
}

func TestCreativeOrderRejectsStaleVariantAssetAndQCRevisions(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "stale order revision")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{"version":1}`), Revision: 1, Status: "queued",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create revision 1 variant: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "queued",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create revision 2 variant: %d %s", w.Code, w.Body.String())
	}
	var variant creativeOrderVariantResponse
	if err := json.NewDecoder(w.Body).Decode(&variant); err != nil {
		t.Fatal(err)
	}
	if variant.Revision != 2 {
		t.Fatalf("variant revision = %d, want 2", variant.Revision)
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "partial",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("advance same variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "running",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("regress same variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{"version":1}`), Revision: 1, Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderAsset(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale asset revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc", creativeOrderQCInput{
		VariantID: variant.ID, Lane: "technical", Revision: 1, Status: "passed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderQC(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unscoped stale QC revision: %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeQCWriteRejectsHumanAndTerminalTaskTokens(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "qc write boundary")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'running') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	input := creativeOrderQCInput{VariantID: variantID, Lane: "technical", Revision: 1, Status: "passed"}
	write := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc-reports", input)
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderQC(w, req)
		return w
	}
	if w := write(); w.Code != http.StatusForbidden {
		t.Fatalf("human QC report write = %d %s, want 403", w.Code, w.Body.String())
	}

	agentID := createHandlerTestAgent(t, "creative-qc-terminal-"+uuid.NewString(), nil)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, $2, 'failed', $3::jsonb, 'creative_order_variant_qc', $4)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), creativeQCTaskContextForTest(t, orderID, variantID, "technical"), variantID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc-reports", input)
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpsertCreativeOrderQC(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("terminal QC task write = %d %s, want 403", w.Code, w.Body.String())
	}
}

func TestCreativeOrderVariantQCRecoveryUsageIsExposedAndCannotBeRetried(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "used QC recovery")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, trigger_evidence_kind, input_snapshot, created_by)
VALUES ($1, $2, 'running', 'manual', '{}'::jsonb, $3) RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_qc_recovery_queued'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, $2, 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size); err != nil {
			t.Fatal(err)
		}
	}
	agentID := createHandlerTestAgent(t, "creative-qc-used-recovery-"+uuid.NewString(), nil)
	context, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_technical",
		"creative_order_id": orderID, "variant_id": variantID, "revision": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	var failedTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, completed_at)
VALUES ($1, $2, 'completed', $3::jsonb, now()) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), context).Scan(&failedTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{"blocking_failures":[{"code":"manifest_layout_contract_missing"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'action_required', $2)
`, variantID, issueID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, failedTaskID) })

	w := httptest.NewRecorder()
	get := withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil), "id", orderID)
	testHandler.GetCreativeOrder(w, get)
	if w.Code != http.StatusOK {
		t.Fatalf("GetCreativeOrder before recovery = %d %s", w.Code, w.Body.String())
	}
	var beforeRecovery creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&beforeRecovery); err != nil {
		t.Fatal(err)
	}
	if len(beforeRecovery.Items) != 1 || len(beforeRecovery.Items[0].Variants) != 1 || beforeRecovery.Items[0].Variants[0].QCRecoveryUsed || !beforeRecovery.Items[0].Variants[0].QCRecoveryAvailable {
		t.Fatalf("QC recovery availability before recovery = %#v", beforeRecovery.Items)
	}
	details, err := json.Marshal(map[string]any{"variant_id": variantID, "revision": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_qc_recovery_queued', $4::jsonb)
`, testWorkspaceID, issueID, testUserID, details); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	testHandler.GetCreativeOrder(w, get)
	if w.Code != http.StatusOK {
		t.Fatalf("GetCreativeOrder = %d %s", w.Code, w.Body.String())
	}
	var order creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 1 {
		t.Fatalf("order variants = %#v", order.Items)
	}
	variant := order.Items[0].Variants[0]
	if !variant.QCRecoveryUsed || variant.QCRecoveryAvailable {
		t.Fatalf("QC recovery flags = used:%t available:%t, want true:false", variant.QCRecoveryUsed, variant.QCRecoveryAvailable)
	}

	w = httptest.NewRecorder()
	retry := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variantID+"/qc/retry", nil), "id", orderID, "variantId", variantID)
	testHandler.RetryCreativeOrderVariantQC(w, retry)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already been used") {
		t.Fatalf("previously used QC recovery = %d %s, want 409 already used", w.Code, w.Body.String())
	}
}

func TestCreativeOrderVariantQCRecoveryResetsAfterPrimeRepairEpoch(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "QC recovery after Prime repair")
	squad := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]any{
			"leader_agent_id":   squad.LeaderAgentID,
			"reviewer_agent_id": squad.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, trigger_evidence_kind, input_snapshot, created_by)
VALUES ($1, $2, 'running', 'manual', $3::jsonb, $4) RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action IN ('creative_qc_recovery_queued', 'creative_prime_package_repair_queued')`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE context->>'creative_order_id' = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, $2, 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size); err != nil {
			t.Fatal(err)
		}
	}
	qcContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_technical",
		"creative_order_id": orderID, "variant_id": variantID, "revision": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, completed_at)
VALUES ($1, $2, 'completed', $3::jsonb, now())
`, squad.ReviewerAgentID, handlerTestRuntimeID(t), qcContext); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{"blocking_failures":[{"code":"attachment_download_primed_asset_failed"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'action_required', $2)
`, variantID, issueID); err != nil {
		t.Fatal(err)
	}
	details, err := json.Marshal(map[string]any{"variant_id": variantID, "revision": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details, created_at)
VALUES
  ($1, $2, 'member', $3, 'creative_qc_recovery_queued', $4::jsonb, now() - interval '2 minutes'),
  ($1, $2, 'member', $3, 'creative_prime_package_repair_queued', $4::jsonb, now() - interval '1 minute')
`, testWorkspaceID, issueID, testUserID, details); err != nil {
		t.Fatal(err)
	}

	getOrder := func() creativeOrderResponse {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.GetCreativeOrder(w, withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil), "id", orderID))
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
		}
		var order creativeOrderResponse
		if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
			t.Fatal(err)
		}
		return order
	}
	variant := getOrder().Items[0].Variants[0]
	if variant.QCRecoveryUsed || !variant.QCRecoveryAvailable {
		t.Fatalf("pre-repair recovery must not consume repaired epoch: used=%t available=%t", variant.QCRecoveryUsed, variant.QCRecoveryAvailable)
	}

	retry := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variantID+"/qc/retry", nil), "id", orderID, "variantId", variantID)
	w := httptest.NewRecorder()
	testHandler.RetryCreativeOrderVariantQC(w, retry)
	if w.Code != http.StatusOK {
		t.Fatalf("QC recovery after Prime repair = %d %s, want 200", w.Code, w.Body.String())
	}
	variant = getOrder().Items[0].Variants[0]
	if !variant.QCRecoveryUsed || variant.QCRecoveryAvailable {
		t.Fatalf("repaired epoch recovery flags = used:%t available:%t, want true:false", variant.QCRecoveryUsed, variant.QCRecoveryAvailable)
	}

	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'action_required' WHERE id = $1`, variantID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	testHandler.RetryCreativeOrderVariantQC(w, retry)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already been used") {
		t.Fatalf("second repaired epoch QC recovery = %d %s, want 409 already used", w.Code, w.Body.String())
	}
}

func TestCreativeOrderPrimePackageRepairReusesGeneratedAssetsAndIsSingleUse(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createPrimePackageRepairFixture(t, "prime package repair")

	getOrder := func() creativeOrderResponse {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.GetCreativeOrder(w, withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+fixture.orderID, nil), "id", fixture.orderID))
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeOrder: %d %s", w.Code, w.Body.String())
		}
		var order creativeOrderResponse
		if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
			t.Fatal(err)
		}
		return order
	}
	before := getOrder()
	variant := before.Items[0].Variants[0]
	if variant.PrimeRepairUsed || !variant.PrimeRepairAvailable {
		t.Fatalf("Prime repair flags before repair = used:%t available:%t", variant.PrimeRepairUsed, variant.PrimeRepairAvailable)
	}

	w := httptest.NewRecorder()
	req := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+fixture.orderID+"/variants/"+fixture.variantID+"/prime-package-repair", nil), "id", fixture.orderID, "variantId", fixture.variantID)
	testHandler.RepairCreativeOrderVariantPrimePackage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("RepairCreativeOrderVariantPrimePackage: %d %s", w.Code, w.Body.String())
	}
	var response creativeOrderPrimePackageRepairResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil || response.TaskID == "" {
		t.Fatalf("repair response = %#v err=%v", response, err)
	}

	var agentID, runtimeID, status string
	var forceFresh bool
	var contextRaw []byte
	if err := testPool.QueryRow(t.Context(), `
SELECT agent_id::text, runtime_id::text, status, force_fresh_session, context
FROM agent_task_queue WHERE id = $1
`, response.TaskID).Scan(&agentID, &runtimeID, &status, &forceFresh, &contextRaw); err != nil {
		t.Fatal(err)
	}
	if agentID != fixture.primeAgentID || runtimeID != fixture.runtimeID || status != "queued" || !forceFresh {
		t.Fatalf("repair task does not reuse Prime execution: agent=%s runtime=%s status=%s force_fresh=%t", agentID, runtimeID, status, forceFresh)
	}
	var context map[string]any
	if err := json.Unmarshal(contextRaw, &context); err != nil {
		t.Fatal(err)
	}
	if context["workflow"] != "creative_prime" || context["prime_package_repair"] != true || context["repair_of_task_id"] != fixture.primeTaskID {
		t.Fatalf("repair task context = %#v", context)
	}
	if context["item_key"] == fixture.primeItemKey {
		t.Fatalf("repair task reused the prior item key: %#v", context)
	}

	var generatedCount, reportCount, resolutionCount, activityCount int
	var variantStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset WHERE variant_id = $1 AND revision = 1 AND stage = 'generated' AND status = 'completed'`, fixture.variantID).Scan(&generatedCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_qc_report WHERE variant_id = $1 AND revision = 1`, fixture.variantID).Scan(&reportCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = 1`, fixture.variantID).Scan(&resolutionCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, fixture.variantID).Scan(&variantStatus); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_prime_package_repair_queued'`, fixture.issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if generatedCount != 3 || reportCount != 0 || resolutionCount != 0 || variantStatus != "running" || activityCount != 1 {
		t.Fatalf("repair state generated=%d reports=%d resolutions=%d variant=%q activities=%d", generatedCount, reportCount, resolutionCount, variantStatus, activityCount)
	}

	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'action_required' WHERE id = $1`, fixture.variantID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	testHandler.RepairCreativeOrderVariantPrimePackage(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "already been used") {
		t.Fatalf("second Prime package repair = %d %s, want 409 used", w.Code, w.Body.String())
	}

	after := getOrder()
	variant = after.Items[0].Variants[0]
	if !variant.PrimeRepairUsed || variant.PrimeRepairAvailable {
		t.Fatalf("Prime repair flags after repair = used:%t available:%t", variant.PrimeRepairUsed, variant.PrimeRepairAvailable)
	}
}

func TestCreativeOrderPrimePackageRepairRejectsActiveQC(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createPrimePackageRepairFixture(t, "prime repair active QC")
	qcAgentID := createHandlerTestAgent(t, "creative-prime-repair-qc-"+uuid.NewString(), nil)
	qcContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_visual", "creative_order_id": fixture.orderID,
		"creative_order_item_id": fixture.itemID, "variant_id": fixture.variantID, "revision": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, $2, 'running', $3::jsonb, 'creative_order_variant_qc', $4)
`, qcAgentID, fixture.runtimeID, qcContext, fixture.variantID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+fixture.orderID+"/variants/"+fixture.variantID+"/prime-package-repair", nil), "id", fixture.orderID, "variantId", fixture.variantID)
	testHandler.RepairCreativeOrderVariantPrimePackage(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "no active task") {
		t.Fatalf("Prime package repair with active QC = %d %s, want 409", w.Code, w.Body.String())
	}
	var activityCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_prime_package_repair_queued'`, fixture.issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 0 {
		t.Fatalf("active QC must not record a Prime repair, got %d", activityCount)
	}
}

type primePackageRepairFixture struct {
	issueID      string
	orderID      string
	itemID       string
	variantID    string
	primeTaskID  string
	primeAgentID string
	runtimeID    string
	primeItemKey string
}

func createPrimePackageRepairFixture(t *testing.T, title string) primePackageRepairFixture {
	t.Helper()
	issueID, candidateID := createCreativeFeedbackCandidate(t, title)
	fixture := primePackageRepairFixture{issueID: issueID, runtimeID: handlerTestRuntimeID(t)}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, trigger_evidence_kind, input_snapshot, created_by)
VALUES ($1, $2, 'running', 'manual', '{}'::jsonb, $3) RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&fixture.orderID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, fixture.orderID, candidateID).Scan(&fixture.itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'action_required') RETURNING id::text
`, fixture.itemID).Scan(&fixture.variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, $2, 1, 'generated', '{}'::jsonb, '{}'::jsonb, 'completed')
`, fixture.variantID, size); err != nil {
			t.Fatal(err)
		}
	}
	fixture.primeAgentID = createHandlerTestAgent(t, "creative-prime-repair-"+uuid.NewString(), nil)
	fixture.primeItemKey = fixture.variantID + ":prime:r1"
	context, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_prime", "creative_order_id": fixture.orderID,
		"creative_order_item_id": fixture.itemID, "variant_id": fixture.variantID, "revision": 1,
		"scope": "variant", "item_key": fixture.primeItemKey, "leader_agent_id": fixture.primeAgentID,
		"source_marker": "retain-this-context",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id, completed_at)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_variant_prime', $4, now()) RETURNING id::text
`, fixture.primeAgentID, fixture.runtimeID, context, fixture.variantID).Scan(&fixture.primeTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{}'::jsonb), ($1, 'visual', 1, 'failed', '{}'::jsonb)
`, fixture.variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'action_required', $2)
`, fixture.variantID, issueID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_prime_package_repair_queued'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE context->>'creative_order_id' = $1`, fixture.orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, fixture.orderID)
	})
	return fixture
}

func TestCreativeRecoverableQCTaskIDsIncludesLegacyContractFailures(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "legacy QC contract recovery")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-qc-legacy-contract-"+uuid.NewString(), nil)
	legacyTasks := map[string]string{}
	for _, lane := range []string{"technical", "visual"} {
		context, err := json.Marshal(map[string]any{
			"type": "creative_qc", "workflow": "creative_qc", "lane": lane,
			"creative_order_id": orderID, "variant_id": variantID, "revision": 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		var taskID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, completed_at)
VALUES ($1, $2, 'completed', $3::jsonb, now()) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), context).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		legacyTasks[lane] = taskID
		findings := fmt.Sprintf(`{"blocking_failures":[{"code":"delegation_contract_missing_issue_id","message":"%s"}]}`, lane)
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, $2, 1, 'failed', $3::jsonb)
`, variantID, lane, findings); err != nil {
			t.Fatal(err)
		}
	}

	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	ids, err := creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(variantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || !slices.Contains(ids, legacyTasks["technical"]) || !slices.Contains(ids, legacyTasks["visual"]) {
		t.Fatalf("legacy recoverable QC task ids = %#v, want both lanes %#v", ids, legacyTasks)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET findings = '{"blocking_failures":[{"code":"visual_quality_failure"}]}'::jsonb
WHERE variant_id = $1 AND lane = 'visual'
`, variantID); err != nil {
		t.Fatal(err)
	}
	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(variantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != legacyTasks["technical"] {
		t.Fatalf("quality failure was incorrectly recoverable: %#v", ids)
	}

	var currentVariantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V02', 1, 'action_required') RETURNING id::text
`, itemID).Scan(&currentVariantID); err != nil {
		t.Fatal(err)
	}
	currentTasks := map[string]string{}
	for _, lane := range []string{"technical", "visual"} {
		context, err := json.Marshal(map[string]any{
			"type": "creative_domain_task", "workflow": "creative_qc_" + lane,
			"creative_order_id": orderID, "variant_id": currentVariantID, "revision": 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		var taskID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, completed_at)
VALUES ($1, $2, 'completed', $3::jsonb, now()) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), context).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		currentTasks[lane] = taskID
		status := "warning"
		findings := `{"blocking_failures":[]}`
		if lane == "technical" {
			status = "failed"
			findings = `{"blocking_failures":[{"code":"manifest_layout_contract_missing"},{"code":"qr_independent_redecode_failed"}]}`
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, $2, 1, $3, $4::jsonb)
`, currentVariantID, lane, status, findings); err != nil {
			t.Fatal(err)
		}
	}

	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(currentVariantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != currentTasks["technical"] {
		t.Fatalf("current contract failure task ids = %#v, want technical %s", ids, currentTasks["technical"])
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET findings = '{"blocking_failures":[{"code":"qr_independent_redecode_failed"}]}'::jsonb
WHERE variant_id = $1 AND lane = 'technical'
`, currentVariantID); err != nil {
		t.Fatal(err)
	}
	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(currentVariantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("pure QR quality failure was incorrectly recoverable: %#v", ids)
	}
}

func TestFinalizeCreativeOrderQCAtomicallyDeliversAndFencesReports(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "qc finalize")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM inbox_item WHERE details->>'creative_order_id' = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v01', 1, 'running') RETURNING id::text`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '1080x1080', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed'),
       ($1, '1200x628', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed'),
       ($1, '800x1000', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'passed', '{}'::jsonb),
       ($1, 'visual', 1, 'warning', '{}'::jsonb)`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET updated_at = now() - interval '1 hour' WHERE id = $1`, orderID); err != nil {
		t.Fatal(err)
	}
	var orderUpdatedBefore time.Time
	if err := testPool.QueryRow(t.Context(), `SELECT updated_at FROM creative_order WHERE id = $1`, orderID).Scan(&orderUpdatedBefore); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-qc-finalize", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, variantID, creativeQCTaskContextForTest(t, orderID, variantID, "technical")).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })
	var secondTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, variantID, creativeQCTaskContextForTest(t, orderID, variantID, "visual")).Scan(&secondTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, secondTaskID) })

	finalize := func(currentTaskID string) (*httptest.ResponseRecorder, creativeOrderQCFinalizeResponse) {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{VariantID: variantID, Revision: 1})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", currentTaskID)
		testHandler.FinalizeCreativeOrderQC(w, req)
		var response creativeOrderQCFinalizeResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return w, response
	}
	type finalizeResult struct {
		code     int
		response creativeOrderQCFinalizeResponse
		body     string
	}
	results := make(chan finalizeResult, 2)
	go func() {
		w, response := finalize(taskID)
		results <- finalizeResult{code: w.Code, response: response, body: w.Body.String()}
	}()
	go func() {
		w, response := finalize(secondTaskID)
		results <- finalizeResult{code: w.Code, response: response, body: w.Body.String()}
	}()
	first, second := <-results, <-results
	if first.code != http.StatusOK || second.code != http.StatusOK {
		t.Fatalf("concurrent finalizations = %#v %#v", first, second)
	}
	created := 0
	for _, result := range []finalizeResult{first, second} {
		if result.response.Created {
			created++
			if !result.response.Finalized || result.response.Outcome != "delivered" || result.response.DeliveredAssetCount != 3 {
				t.Fatalf("created finalization = %#v", result)
			}
		}
	}
	if created != 1 {
		t.Fatalf("concurrent finalization created %d resolutions, want one", created)
	}
	w, response := finalize(taskID)
	if w.Code != http.StatusOK || response.Created || !response.Finalized || response.Outcome != "delivered" {
		t.Fatalf("idempotent finalization = %d %#v %s", w.Code, response, w.Body.String())
	}
	var delivered, resolutions, inboxes int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset WHERE variant_id = $1 AND revision = 1 AND stage = 'delivered'`, variantID).Scan(&delivered); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = 1`, variantID).Scan(&resolutions); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM inbox_item WHERE type = 'creative_qc_delivered' AND details->>'variant_id' = $1`, variantID).Scan(&inboxes); err != nil {
		t.Fatal(err)
	}
	if delivered != 3 || resolutions != 1 || inboxes != 1 {
		t.Fatalf("delivered=%d resolutions=%d inboxes=%d", delivered, resolutions, inboxes)
	}
	var orderUpdatedAfter time.Time
	if err := testPool.QueryRow(t.Context(), `SELECT updated_at FROM creative_order WHERE id = $1`, orderID).Scan(&orderUpdatedAfter); err != nil {
		t.Fatal(err)
	}
	if !orderUpdatedAfter.After(orderUpdatedBefore) {
		t.Fatalf("order updated_at = %s, want after %s", orderUpdatedAfter, orderUpdatedBefore)
	}

	w = httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc-reports", creativeOrderQCInput{VariantID: variantID, Lane: "technical", Revision: 1, Status: "passed"})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderQC(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unscoped QC write after finalization = %d %s", w.Code, w.Body.String())
	}

	var failedVariantID, failedTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v02', 1, 'running') RETURNING id::text`, itemID).Scan(&failedVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '1080x1080', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed'),
       ($1, '1200x628', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed'),
       ($1, '800x1000', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')`, failedVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{"blocking_failures":["qr"]}'::jsonb),
       ($1, 'visual', 1, 'passed', '{}'::jsonb)`, failedVariantID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, failedVariantID, creativeQCTaskContextForTest(t, orderID, failedVariantID, "technical")).Scan(&failedTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, failedTaskID) })
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{VariantID: failedVariantID, Revision: 1})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", failedTaskID)
	testHandler.FinalizeCreativeOrderQC(w, req)
	var failedResponse creativeOrderQCFinalizeResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&failedResponse) != nil || failedResponse.Outcome != "action_required" || failedResponse.DeliveredAssetCount != 0 || failedResponse.OrderAggregateStatus != "awaiting_adoption" {
		t.Fatalf("failed QC finalization = %d %#v %s", w.Code, failedResponse, w.Body.String())
	}
	var failedVariantStatus, completedVariantStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, failedVariantID).Scan(&failedVariantStatus); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, variantID).Scan(&completedVariantStatus); err != nil {
		t.Fatal(err)
	}
	if failedVariantStatus != "action_required" {
		t.Fatalf("failed variant status = %q", failedVariantStatus)
	}
	if completedVariantStatus != "completed" {
		t.Fatalf("completed sibling variant status = %q", completedVariantStatus)
	}
}

func TestCreativeOrderVariantReworkLimitDependsOnMode(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "rework fence")
	create := func(trigger string) (string, string) {
		var orderID, itemID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2, $3) RETURNING id::text`, testWorkspaceID, trigger, testUserID).Scan(&orderID); err != nil {
			t.Fatal(err)
		}
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
		return orderID, itemID
	}
	upsert := func(orderID, itemID string, revision int) int {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{OrderItemID: itemID, VariantKey: "v01", Brief: json.RawMessage(`{}`), Revision: revision, Status: "running"})
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		return w.Code
	}

	standardOrderID, standardItemID := create("manual")
	if code := upsert(standardOrderID, standardItemID, 1); code != http.StatusOK {
		t.Fatalf("standard r1 = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, 2); code != http.StatusOK {
		t.Fatalf("standard r2 = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, 3); code != http.StatusConflict {
		t.Fatalf("standard r3 = %d, want 409", code)
	}

	directOrderID, directItemID := create("creative_direct_edit")
	if code := upsert(directOrderID, directItemID, 1); code != http.StatusOK {
		t.Fatalf("direct r1 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, 2); code != http.StatusOK {
		t.Fatalf("direct r2 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, 3); code != http.StatusOK {
		t.Fatalf("direct r3 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, 4); code != http.StatusConflict {
		t.Fatalf("direct r4 = %d, want 409", code)
	}
}

func TestNormalizeCreativeOrderQCFailsClosedForBlockingFindings(t *testing.T) {
	input, err := normalizeCreativeOrderQC(creativeOrderQCInput{
		VariantID: "variant", Lane: "visual", Revision: 1, Status: "warning",
		Findings: json.RawMessage(`{"blocking_failures":[{"code":"corner_overlap"}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Status != "failed" {
		t.Fatalf("status = %q, want failed", input.Status)
	}

	input, err = normalizeCreativeOrderQC(creativeOrderQCInput{
		VariantID: "variant", Lane: "visual", Revision: 1, Status: "warning",
		Findings: json.RawMessage(`{"blocking_failures":[]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Status != "warning" {
		t.Fatalf("empty blocking failures changed status to %q", input.Status)
	}

	_, err = normalizeCreativeOrderQC(creativeOrderQCInput{
		VariantID: "variant", Lane: "visual", Revision: 1, Status: "passed",
		Findings: json.RawMessage(`{"blocking_failures":"qr"}`),
	})
	if err == nil {
		t.Fatal("non-array blocking_failures must be rejected")
	}
}

func TestExpectedCreativeVariantSizesDependOnOrderMode(t *testing.T) {
	standard, err := expectedCreativeVariantSizes("manual", json.RawMessage(`{"expected_sizes":["1080x1080"]}`), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(standard) != 3 {
		t.Fatalf("standard sizes = %#v, want all three", standard)
	}

	direct, err := expectedCreativeVariantSizes("creative_direct_edit", json.RawMessage(`{"target_size":"1200x628"}`), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 1 || direct[0] != "1200x628" {
		t.Fatalf("direct target sizes = %#v", direct)
	}

	direct, err = expectedCreativeVariantSizes("creative_direct_edit", json.RawMessage(`{"expected_sizes":["1080x1080","800x1000"],"target_size":"1200x628"}`), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 2 || direct[0] != "1080x1080" || direct[1] != "800x1000" {
		t.Fatalf("direct expected sizes = %#v", direct)
	}
}

func TestFinalizeCreativeDirectEditQCUsesScopedSizesAndFindings(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "direct edit QC scope")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, 'running', '{"target_size":"1200x628"}'::jsonb, 'creative_direct_edit', $2)
RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM inbox_item WHERE details->>'creative_order_id' = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-direct-qc-finalize", []byte(`{}`))

	type variantFixture struct {
		variantID string
		taskID    string
	}
	createVariant := func(key string, technicalFindings string) variantFixture {
		var fixture variantFixture
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, brief, revision, status)
VALUES ($1, $2, '{"target_size":"1200x628"}'::jsonb, 1, 'running') RETURNING id::text`, itemID, key).Scan(&fixture.variantID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '1200x628', 1, 'primed', '{}'::jsonb, '{}'::jsonb, 'completed')`, fixture.variantID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'passed', $2::jsonb),
       ($1, 'visual', 1, 'passed', '{}'::jsonb)`, fixture.variantID, technicalFindings); err != nil {
			t.Fatal(err)
		}
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, fixture.variantID, creativeQCTaskContextForTest(t, orderID, fixture.variantID, "technical")).Scan(&fixture.taskID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, fixture.taskID)
		})
		return fixture
	}
	finalize := func(fixture variantFixture) creativeOrderQCFinalizeResponse {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{VariantID: fixture.variantID, Revision: 1})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", fixture.taskID)
		testHandler.FinalizeCreativeOrderQC(w, req)
		var response creativeOrderQCFinalizeResponse
		if w.Code != http.StatusOK {
			t.Fatalf("finalize = %d %s", w.Code, w.Body.String())
		}
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	blocked := finalize(createVariant("direct-blocked", `{"blocking_failures":["qr"]}`))
	if blocked.Outcome != "action_required" || blocked.DeliveredAssetCount != 0 || blocked.TechnicalStatus != "failed" {
		t.Fatalf("blocking findings finalization = %#v", blocked)
	}

	deliveredFixture := createVariant("direct-delivered", `{}`)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: deliveredFixture.variantID, SizeKey: "1080x1080", Revision: 1, Stage: "primed", Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderAsset(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("out-of-scope direct asset = %d %s", w.Code, w.Body.String())
	}
	delivered := finalize(deliveredFixture)
	if delivered.Outcome != "delivered" || delivered.DeliveredAssetCount != 1 {
		t.Fatalf("single-size direct finalization = %#v", delivered)
	}
	var deliveredCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'delivered'`, deliveredFixture.variantID).Scan(&deliveredCount); err != nil {
		t.Fatal(err)
	}
	if deliveredCount != 1 {
		t.Fatalf("delivered asset count = %d, want 1", deliveredCount)
	}
}

type creativeOrderSquadFixture struct {
	SquadID         string
	LeaderAgentID   string
	PlannerAgentID  string
	ProducerAgentID string
	PrimeAgentID    string
	ReviewerAgentID string
}

func createCreativeOrderSquadFixture(t *testing.T, missingCapability, duplicateCapability string, leaderCapable bool) creativeOrderSquadFixture {
	t.Helper()
	fixture := creativeOrderSquadFixture{
		LeaderAgentID:   createHandlerTestAgent(t, "creative-order-leader-"+uuid.NewString(), nil),
		PlannerAgentID:  createHandlerTestAgent(t, "creative-order-planner-"+uuid.NewString(), nil),
		ProducerAgentID: createHandlerTestAgent(t, "creative-order-producer-"+uuid.NewString(), nil),
		PrimeAgentID:    createHandlerTestAgent(t, "creative-order-prime-"+uuid.NewString(), nil),
		ReviewerAgentID: createHandlerTestAgent(t, "creative-order-reviewer-"+uuid.NewString(), nil),
	}
	agentsByCapability := map[string]string{
		"generation_plan": fixture.PlannerAgentID,
		"image_edit":      fixture.ProducerAgentID,
		"prime_compose":   fixture.PrimeAgentID,
		"quality_control": fixture.ReviewerAgentID,
	}
	skillIDs := make([]string, 0, len(agentsByCapability)+1)
	bindCapability := func(agentID, capability string) {
		t.Helper()
		var skillID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO skill (workspace_id, name, config, created_by)
VALUES ($1, $2, jsonb_build_object('kind', 'creative_role', 'capability', $3::text), $4)
RETURNING id::text
`, testWorkspaceID, "Creative order "+capability+" "+uuid.NewString(), capability, testUserID).Scan(&skillID); err != nil {
			t.Fatal(err)
		}
		skillIDs = append(skillIDs, skillID)
		if _, err := testPool.Exec(t.Context(), `INSERT INTO agent_skill (agent_id, skill_id, enabled) VALUES ($1, $2, TRUE)`, agentID, skillID); err != nil {
			t.Fatal(err)
		}
	}
	if leaderCapable {
		bindCapability(fixture.LeaderAgentID, "creative_leadership")
	}
	for capability, agentID := range agentsByCapability {
		if capability != missingCapability {
			bindCapability(agentID, capability)
		}
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO squad (workspace_id, name, leader_id, creator_id)
VALUES ($1, $2, $3, $4)
RETURNING id::text
`, testWorkspaceID, "Creative order squad "+uuid.NewString(), fixture.LeaderAgentID, testUserID).Scan(&fixture.SquadID); err != nil {
		t.Fatal(err)
	}
	for _, agentID := range []string{fixture.LeaderAgentID, fixture.PlannerAgentID, fixture.ProducerAgentID, fixture.PrimeAgentID, fixture.ReviewerAgentID} {
		if _, err := testPool.Exec(t.Context(), `INSERT INTO squad_member (squad_id, member_type, member_id) VALUES ($1, 'agent', $2)`, fixture.SquadID, agentID); err != nil {
			t.Fatal(err)
		}
	}
	if duplicateCapability != "" {
		duplicateAgentID := createHandlerTestAgent(t, "creative-order-duplicate-"+uuid.NewString(), nil)
		bindCapability(duplicateAgentID, duplicateCapability)
		if _, err := testPool.Exec(t.Context(), `INSERT INTO squad_member (squad_id, member_type, member_id) VALUES ($1, 'agent', $2)`, fixture.SquadID, duplicateAgentID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM squad WHERE id = $1`, fixture.SquadID)
		for _, skillID := range skillIDs {
			_, _ = testPool.Exec(t.Context(), `DELETE FROM skill WHERE id = $1`, skillID)
		}
	})
	return fixture
}

func TestCreateCreativeOrderFreezesSquadAgentsByCapability(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "capability snapshot")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"market_pack": map[string]any{"id": "market-pack", "version": 7},
		"squad_snapshot": map[string]any{
			"squad_id": fixture.SquadID, "squad_name": "client-visible name", "members": []any{map[string]any{"agent_id": "client-member"}},
			"leader_agent_id": "client-leader", "planner_agent_id": "client-planner", "producer_agent_id": "client-producer",
			"prime_agent_id": "client-prime", "reviewer_agent_id": "client-reviewer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
		Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
		Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":2,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`), Direction: "freeze agents"}},
	})
	testHandler.CreateCreativeOrder(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeOrder = %d %s", w.Code, w.Body.String())
	}
	var order creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, order.ID) })
	var frozen struct {
		MarketPack map[string]any `json:"market_pack"`
		Squad      map[string]any `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(order.InputSnapshot, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.MarketPack["id"] != "market-pack" || frozen.MarketPack["version"] != float64(7) {
		t.Fatalf("market pack was not preserved: %#v", frozen.MarketPack)
	}
	want := map[string]string{
		"squad_id": fixture.SquadID, "leader_agent_id": fixture.LeaderAgentID, "planner_agent_id": fixture.PlannerAgentID,
		"producer_agent_id": fixture.ProducerAgentID, "prime_agent_id": fixture.PrimeAgentID, "reviewer_agent_id": fixture.ReviewerAgentID,
	}
	for field, value := range want {
		if frozen.Squad[field] != value {
			t.Errorf("squad snapshot %s = %#v, want %q", field, frozen.Squad[field], value)
		}
	}
	if frozen.Squad["squad_name"] != "client-visible name" {
		t.Fatalf("unrelated squad snapshot fields were not preserved: %#v", frozen.Squad)
	}
}

func TestCreativeFinancialTokensRecognizeUnseparatedCurrency(t *testing.T) {
	want := map[string]struct{}{
		"currency:80000000": {},
	}
	for _, token := range creativeFinancialTokens("Rp80000000 / Rp.80.000.000 / IDR 80.000.000") {
		delete(want, token.key)
	}
	if len(want) != 0 {
		t.Fatalf("currency forms were not normalized: missing %#v", want)
	}
}

func TestCreateCreativeOrderValidatesCustomCopyAgainstPublishedFacts(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "custom copy financial facts")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	libraryID := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource (
  id, workspace_id, kind, name, description, status, version, published_version, config, created_by
) VALUES ($1, $2, 'copy_library', 'Approved custom copy facts', '', 'published', 1, 1, $3::jsonb, $4)
`, libraryID, testWorkspaceID, validComposableCopyLibraryJSON, testUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1, 1, 'Approved custom copy facts', '', $2::jsonb, $3)
`, libraryID, validComposableCopyLibraryJSON, testUserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_resource WHERE id = $1`, libraryID) })

	inputSnapshot, err := json.Marshal(map[string]any{
		"market_pack":    map[string]any{"config": map[string]any{"copy_library_id": libraryID}},
		"squad_snapshot": map[string]any{"squad_id": fixture.SquadID},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestOrder := func(benefit string) *httptest.ResponseRecorder {
		t.Helper()
		copySnapshot, marshalErr := json.Marshal(map[string]any{
			"schema_version": 2,
			"status":         "user_custom",
			"creative_type":  "num",
			"headline":       "Pinjaman fleksibel",
			"benefit":        benefit,
			"cta":            "Ajukan sekarang",
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
			Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: copySnapshot}},
		})
		testHandler.CreateCreativeOrder(w, req)
		return w
	}

	approved := requestOrder("Limit hingga Rp80.000.000")
	if approved.Code != http.StatusCreated {
		t.Fatalf("approved custom fact = %d %s", approved.Code, approved.Body.String())
	}
	var created creativeOrderResponse
	if err := json.NewDecoder(approved.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, created.ID) })

	rejected := requestOrder("Limit hingga Rp99.000.000 dengan bunga 1%")
	if rejected.Code != http.StatusUnprocessableEntity || !strings.Contains(rejected.Body.String(), "unapproved financial facts") {
		t.Fatalf("unapproved custom fact = %d %s", rejected.Code, rejected.Body.String())
	}

	requestApprovedSnapshot := func(benefit string) *httptest.ResponseRecorder {
		t.Helper()
		copySnapshot, marshalErr := json.Marshal(map[string]any{
			"schema_version": 2, "status": "approved", "id": "recipe-num",
			"library_id": libraryID, "library_version": 1, "recipe_id": "recipe-num", "recipe_key": "num", "creative_type": "num",
			"headline": "", "subheadline": "", "benefit": benefit, "supporting": "", "cta": "", "legal_text": "",
			"fragments":     []map[string]any{{"id": "fragment-num", "key": "num", "role": "benefit", "text": benefit}},
			"product_facts": []map[string]any{{"key": "limit", "label": "Limit", "value": "80000000", "copy_text": "Rp80.000.000", "source": "approved sheet"}},
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		w := httptest.NewRecorder()
		testHandler.CreateCreativeOrder(w, newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
			Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: copySnapshot}},
		}))
		return w
	}

	frozen := requestApprovedSnapshot("Limit hingga Rp80.000.000")
	if frozen.Code != http.StatusCreated {
		t.Fatalf("frozen approved recipe = %d %s", frozen.Code, frozen.Body.String())
	}
	var frozenOrder creativeOrderResponse
	if err := json.NewDecoder(frozen.Body).Decode(&frozenOrder); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, frozenOrder.ID) })

	tampered := requestApprovedSnapshot("Limit hingga Rp99.000.000")
	if tampered.Code != http.StatusUnprocessableEntity || !strings.Contains(tampered.Body.String(), "frozen published recipe") {
		t.Fatalf("tampered approved recipe = %d %s", tampered.Code, tampered.Body.String())
	}
}

func TestCreateCreativeOrderRejectsInvalidCapabilitySquad(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	tests := []struct {
		name                string
		missingCapability   string
		duplicateCapability string
		leaderCapable       bool
		wantMessage         string
	}{
		{name: "missing role", missingCapability: "quality_control", leaderCapable: true, wantMessage: "missing quality_control"},
		{name: "duplicate role", duplicateCapability: "image_edit", leaderCapable: true, wantMessage: "multiple agents with image_edit"},
		{name: "leader without capability", leaderCapable: false, wantMessage: "creative_leadership"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, candidateID := createCreativeFeedbackCandidate(t, tt.name)
			fixture := createCreativeOrderSquadFixture(t, tt.missingCapability, tt.duplicateCapability, tt.leaderCapable)
			w := httptest.NewRecorder()
			req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
				Status: "queued", InputSnapshot: json.RawMessage(`{"squad_snapshot":{"squad_id":"` + fixture.SquadID + `"}}`),
				Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{}`)}},
			})
			testHandler.CreateCreativeOrder(w, req)
			if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), tt.wantMessage) {
				t.Fatalf("CreateCreativeOrder = %d %s, want 422 containing %q", w.Code, w.Body.String(), tt.wantMessage)
			}
			var orderCount int
			if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_item WHERE candidate_id = $1`, candidateID).Scan(&orderCount); err != nil {
				t.Fatal(err)
			}
			if orderCount != 0 {
				t.Fatalf("invalid capability squad created %d order items", orderCount)
			}
		})
	}
}

func TestCreativeSubmissionKeyRecoversIssueAndConcurrentOrder(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "submission recovery")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot := json.RawMessage(`{"squad_snapshot":{"squad_id":"` + fixture.SquadID + `"}}`)
	key := "creative:test-" + uuid.NewString()
	createIssue := func() (int, IssueResponse) {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/issues", CreateIssueRequest{
			Title:    "Creative submission " + key,
			Status:   "todo",
			Metadata: map[string]json.RawMessage{"workflow": json.RawMessage(`"creative_order"`), "creative_submission_key": json.RawMessage(`"` + key + `"`)},
		})
		testHandler.CreateIssue(w, req)
		var issue IssueResponse
		if w.Code == http.StatusCreated || w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&issue); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, issue
	}
	firstCode, issue := createIssue()
	if firstCode != http.StatusCreated || issue.ID == "" {
		t.Fatalf("first issue = %d %#v", firstCode, issue)
	}
	secondCode, recovered := createIssue()
	if secondCode != http.StatusOK || recovered.ID != issue.ID {
		t.Fatalf("recovered issue = %d %#v, want %q", secondCode, recovered, issue.ID)
	}

	createOrder := func() (int, creativeOrderResponse) {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
			IssueID: issue.ID, SubmissionKey: key, Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":2,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`), Direction: "recover submission"}},
		})
		testHandler.CreateCreativeOrder(w, req)
		var order creativeOrderResponse
		if w.Code == http.StatusCreated || w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
				t.Fatal(err)
			}
		}
		return w.Code, order
	}
	results := make(chan struct {
		code  int
		order creativeOrderResponse
	}, 2)
	for range 2 {
		go func() {
			code, order := createOrder()
			results <- struct {
				code  int
				order creativeOrderResponse
			}{code, order}
		}()
	}
	first, second := <-results, <-results
	if first.code != http.StatusCreated && first.code != http.StatusOK {
		t.Fatalf("first order = %d", first.code)
	}
	if second.code != http.StatusCreated && second.code != http.StatusOK {
		t.Fatalf("second order = %d", second.code)
	}
	if first.order.ID == "" || first.order.ID != second.order.ID {
		t.Fatalf("concurrent orders = %#v %#v", first, second)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, first.order.ID) })
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order WHERE workspace_id = $1 AND submission_key = $2`, testWorkspaceID, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("orders with submission key = %d, want 1", count)
	}
}

func TestCreativeOrderRejectsUnknownCrawlRunEvidence(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, 'Unknown evidence', 'image', 'https://example.test/evidence.png', '{}'::jsonb)
RETURNING id::text`, testWorkspaceID, "unknown-evidence-"+uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/source-analyses", creativeSourceAnalysisInput{
		CandidateID: candidateID, AnalysisVersion: 1, Status: "pending", TriggerEvidenceKind: "crawl_run", TriggerEvidenceReference: uuid.NewString(),
	})
	testHandler.CreateCreativeSourceAnalysis(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unknown crawl evidence = %d %s", w.Code, w.Body.String())
	}
}

func TestCreateCreativeSourceAnalysisRejectsUnlinkedCrawlRunEvidence(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "unlinked crawl evidence")
	var runID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_crawl_run (workspace_id, issue_id, connector_id, status, created_by_type, created_by_id)
VALUES ($1, $2, 'test', 'completed', 'member', $3) RETURNING id::text`, testWorkspaceID, issueID, testUserID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/source-analyses", creativeSourceAnalysisInput{
		CandidateID: candidateID, AnalysisVersion: 1, Status: "pending", TriggerEvidenceKind: "crawl_run", TriggerEvidenceReference: runID,
	})
	testHandler.CreateCreativeSourceAnalysis(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unlinked crawl evidence = %d %s", w.Code, w.Body.String())
	}
}
