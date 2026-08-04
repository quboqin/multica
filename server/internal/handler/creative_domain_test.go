package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

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
	if order.DerivedStatus != "partial" {
		t.Fatalf("derived status = %q, want partial", order.DerivedStatus)
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
	if order.DerivedStatus != "action_required" {
		t.Fatalf("derived status = %q, want action_required", order.DerivedStatus)
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
	if listedOrder == nil || listedOrder.DerivedStatus != "action_required" || len(listedOrder.WorkflowFailures) != 1 {
		t.Fatalf("list response did not expose open workflow failure: %#v", listedOrder)
	}

	insertTask("queued", openItemKey, "creative_domain_task", orderID, "", "", 0, 2, 2)
	order = getOrder()
	if order.DerivedStatus != "queued" {
		t.Fatalf("recovered derived status = %q, want queued", order.DerivedStatus)
	}
	if order.WorkflowFailures == nil || len(order.WorkflowFailures) != 0 {
		t.Fatalf("recovered workflow failures = %#v, want empty array", order.WorkflowFailures)
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
		{variantStatus: "running", wantStatus: "partial"},
		{variantStatus: "completed", wantStatus: "completed"},
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

func TestCreativeOrderQCDerivesFromLatestRevision(t *testing.T) {
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
	if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_variant (order_item_id, variant_key, status) VALUES ($1, 'v01', 'partial') RETURNING id::text`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'failed', '{}'::jsonb),
       ($1, 'visual', 1, 'failed', '{}'::jsonb),
       ($1, 'technical', 2, 'passed', '{}'::jsonb),
       ($1, 'visual', 2, 'passed', '{}'::jsonb)`, variantID); err != nil {
		t.Fatal(err)
	}
	status, err := testHandler.derivedCreativeVariantQCStatus(newRequest(http.MethodGet, "/", nil), parseUUID(variantID))
	if err != nil {
		t.Fatal(err)
	}
	if status != "passed" {
		t.Fatalf("latest QC status = %q, want passed", status)
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
	if w.Code != http.StatusConflict {
		t.Fatalf("stale QC revision: %d %s", w.Code, w.Body.String())
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
	agentID := createHandlerTestAgent(t, "creative-qc-finalize", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2)
RETURNING id::text`, agentID, variantID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })
	var secondTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2)
RETURNING id::text`, agentID, variantID).Scan(&secondTaskID); err != nil {
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

	w = httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc-reports", creativeOrderQCInput{VariantID: variantID, Lane: "technical", Revision: 1, Status: "passed"})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderQC(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("QC write after finalization = %d %s", w.Code, w.Body.String())
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
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2)
RETURNING id::text`, agentID, failedVariantID).Scan(&failedTaskID); err != nil {
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
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&failedResponse) != nil || failedResponse.Outcome != "action_required" || failedResponse.DeliveredAssetCount != 0 || failedResponse.OrderAggregateStatus != "partial" {
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
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2)
RETURNING id::text`, agentID, fixture.variantID).Scan(&fixture.taskID); err != nil {
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
		Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{}`), Direction: "freeze agents"}},
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
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{}`), Direction: "recover submission"}},
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
