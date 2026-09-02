package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreativeOrderQCAgentSnapshotRequiresFrozenLeaderAndReviewer(t *testing.T) {
	leaderID := uuid.NewString()
	producerID := uuid.NewString()
	reviewerID := uuid.NewString()
	leader, reviewer, err := creativeOrderQCAgentSnapshot(json.RawMessage(`{"squad_snapshot":{"leader_agent_id":"` + leaderID + `","producer_agent_id":"` + producerID + `","reviewer_agent_id":"` + reviewerID + `"}}`))
	if err != nil || uuidToString(leader) != leaderID || uuidToString(reviewer) != reviewerID {
		t.Fatalf("valid frozen snapshot = leader %q reviewer %q err %v", uuidToString(leader), uuidToString(reviewer), err)
	}
	if _, _, err := creativeOrderQCAgentSnapshot(json.RawMessage(`{"squad_snapshot":{"leader_agent_id":"` + leaderID + `","producer_agent_id":"` + producerID + `"}}`)); err == nil {
		t.Fatal("missing reviewer must not be accepted")
	}
	if _, _, err := creativeOrderQCAgentSnapshot(json.RawMessage(`not-json`)); err == nil {
		t.Fatal("malformed snapshot must not be accepted")
	}
}

func TestNormalizeCreativeOrderAllowsComplexProductionPrompt(t *testing.T) {
	base := creativeOrderInput{
		Status:        "queued",
		InputSnapshot: json.RawMessage(`{}`),
		Items: []creativeOrderItemInput{{
			CandidateID:  "candidate-1",
			CopySnapshot: json.RawMessage(`{}`),
			Direction:    strings.Repeat("x", 4252),
		}},
	}
	if _, err := normalizeCreativeOrder(base); err != nil {
		t.Fatalf("complex production prompt was rejected: %v", err)
	}
	base.Items[0].Direction = strings.Repeat("x", maxCreativeOrderDirectionLength+1)
	if _, err := normalizeCreativeOrder(base); err == nil {
		t.Fatal("oversized production prompt was accepted")
	}
}

func TestListCreativeOrdersSortsByCreationTime(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	olderOrderID, _, _ := createCreativeLifecycleTestOrder(t, "older creative order")
	newerOrderID, _, _ := createCreativeLifecycleTestOrder(t, "newer creative order")
	olderCreatedAt := time.Date(2026, time.August, 31, 12, 0, 0, 0, time.UTC)
	newerCreatedAt := olderCreatedAt.Add(time.Hour)
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET created_at = $1, updated_at = $2
WHERE id = $3
`, olderCreatedAt, newerCreatedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET created_at = $1, updated_at = $2
WHERE id = $3
`, newerCreatedAt, olderCreatedAt); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	testHandler.ListCreativeOrders(w, newRequest(http.MethodGet, "/api/creative/orders", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeOrders = %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Orders []creativeOrderResponse `json:"orders"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{}
	for index, order := range response.Orders {
		positions[order.ID] = index
	}
	newerPosition, newerFound := positions[newerOrderID]
	olderPosition, olderFound := positions[olderOrderID]
	if !newerFound || !olderFound {
		t.Fatalf("listed orders missing newer=%t older=%t", newerFound, olderFound)
	}
	if newerPosition >= olderPosition {
		t.Fatalf("creation order positions = newer %d older %d, want newer before older", newerPosition, olderPosition)
	}
}

func TestSummarizeCreativeOrderVariantBlockerKeepsModelFailureCause(t *testing.T) {
	raw := strings.Join([]string{
		"方形模型已成功返回完整原始 JSON。",
		"首次模型输出实际为 `916x1716`，无法规范化为方形。",
		"定向重生等待 `600472 ms` 后超时，未返回完整 JSON。",
		"Variant `bac5daf4-e83a-40d9-939f-fb5ca7d81b1c` 已标记为 `action_required`。",
	}, "\n")

	got := summarizeCreativeOrderVariantBlocker(raw)
	if !strings.Contains(got, "916x1716") || !strings.Contains(got, "600472 ms") {
		t.Fatalf("summary = %q, want model dimensions and timeout", got)
	}
	if strings.Contains(got, "action_required") {
		t.Fatalf("summary = %q, must not prefer bookkeeping status", got)
	}
}

func TestSettleCreativeProductionVariantTaskQueuesMissingSizes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "automatic production continuation")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]string{
			"leader_agent_id":   fixture.LeaderAgentID,
			"producer_agent_id": fixture.ProducerAgentID,
			"reviewer_agent_id": fixture.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'running', $3::jsonb, $4)
RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, itemID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb)
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'running')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	itemKey := variantID + ":r1"
	taskContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production",
		"item_key": itemKey, "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID,
		"revision": 1, "expected_sizes": standardCreativeAssetSizes,
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, max_attempts,
  trigger_evidence_kind, trigger_evidence_ref_id, completed_at
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', $2::jsonb, 1,
        'creative_order_item_production', $3, now())
RETURNING id::text
`, fixture.ProducerAgentID, taskContext, itemID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	parentTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), parentTask); err != nil {
		t.Fatalf("settle production task: %v", err)
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, variantID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "running" {
		t.Fatalf("variant status = %q, want running while continuation is queued", status)
	}
	var revisionStatus string
	var stagingRevision int
	var revisionSizes []string
	if err := testPool.QueryRow(t.Context(), `
SELECT revision.status, variant.staging_revision, revision.expected_sizes
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variantID).Scan(&revisionStatus, &stagingRevision, &revisionSizes); err != nil {
		t.Fatal(err)
	}
	if revisionStatus != "running" || stagingRevision != 1 || !slices.Equal(revisionSizes, standardCreativeAssetSizes) {
		t.Fatalf("continued revision = status %q staging %d sizes %#v", revisionStatus, stagingRevision, revisionSizes)
	}
	var childCount, childMaxAttempts int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*), COALESCE(max(max_attempts), 0)
FROM agent_task_queue
WHERE parent_task_id = $1 AND status = 'queued'
`, taskID).Scan(&childCount, &childMaxAttempts); err != nil {
		t.Fatal(err)
	}
	if childCount != 1 || childMaxAttempts != 3 {
		t.Fatalf("continuation = count %d max_attempts %d, want one child with max_attempts 3", childCount, childMaxAttempts)
	}
}

func creativeQCTaskContextForTest(t *testing.T, orderID, variantID, lane string, revisions ...int) []byte {
	t.Helper()
	revision := 1
	if len(revisions) > 0 {
		revision = revisions[0]
	}
	attempt := 1
	if len(revisions) > 1 {
		attempt = revisions[1]
	}
	contextValue, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_qc_" + lane,
		"creative_order_id": orderID, "variant_id": variantID, "revision": revision, "qc_attempt": attempt,
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
VALUES ($1, 'running', '{"expected_sizes":["1080x1080"]}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
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
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, 'completed', ARRAY['1080x1080']::text[]),
       ($2, 1, '{}'::jsonb, 'failed', ARRAY['1080x1080']::text[])
`, completedVariantID, failedVariantID); err != nil {
		t.Fatal(err)
	}
	deliveredAttachmentID := createCreativeOrderAssetAttachment(t, "order-detail-delivered.png")
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, '1080x1080', 1, 'delivered', $2, '{"model_request_id":"req-1"}'::jsonb, '{"prime_manifest":"manifest-1"}'::jsonb, 'completed')
RETURNING id::text`, completedVariantID, deliveredAttachmentID).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(completedVariantID), 1, []string{"1080x1080"}); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
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

func TestUpsertCreativeOrderDiagnosticAssetPersistsAttachmentReference(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "diagnostic asset persist")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'running', '{}'::jsonb, $3) RETURNING id::text
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
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'action_required') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	var attachmentID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, 'prime-collision-preview-1080x1080.png', 'oss://creative/preview.png', 'image/png', 12)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&attachmentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE id = $1`, attachmentID) })
	agentID := createHandlerTestAgent(t, "diagnostic-download-"+uuid.NewString(), nil)
	taskContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production",
		"creative_order_id": orderID, "creative_order_item_id": itemID,
		"variant_id": variantID, "revision": 1, "expected_sizes": standardCreativeAssetSizes,
	})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
	INSERT INTO agent_task_queue (
	  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context, work_dir
	)
	VALUES ($1, $2, 'running', 'creative_order_item_production', $3, $4::jsonb, $5) RETURNING id::text
	`, agentID, handlerTestRuntimeID(t), itemID, taskContext, t.TempDir()).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/diagnostic-assets", creativeOrderDiagnosticAssetInput{
		VariantID:    variantID,
		TaskID:       taskID,
		AttachmentID: attachmentID,
		SizeKey:      "1080x1080",
		Revision:     1,
		Workflow:     "creative_production",
		Label:        "Prime context",
		Filename:     "prime-context-1080x1080.png",
		Metadata:     json.RawMessage(`{"source":"preprime_collision"}`),
	})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	w := httptest.NewRecorder()

	testHandler.UpsertCreativeOrderDiagnosticAsset(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("UpsertCreativeOrderDiagnosticAsset = %d %s", w.Code, w.Body.String())
	}
	var saved creativeOrderDiagnosticAsset
	if err := json.NewDecoder(w.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if saved.AttachmentID != attachmentID || saved.URL != "/api/attachments/"+attachmentID+"/download" || saved.Workflow != "creative_production" {
		t.Fatalf("saved diagnostic asset = %#v", saved)
	}
	get := withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil), "id", orderID)
	w = httptest.NewRecorder()
	testHandler.GetCreativeOrder(w, get)
	if w.Code != http.StatusOK {
		t.Fatalf("GetCreativeOrder = %d %s", w.Code, w.Body.String())
	}
	var order creativeOrderResponse
	if err := json.NewDecoder(w.Body).Decode(&order); err != nil {
		t.Fatal(err)
	}
	got := order.Items[0].Variants[0].DiagnosticAssets
	if len(got) != 1 || got[0].AttachmentID != attachmentID || got[0].URL != "/api/attachments/"+attachmentID+"/download" {
		t.Fatalf("order diagnostic assets = %#v", got)
	}
}

func TestCreativeVariantTaskTokenDoesNotOutrunTaskCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "variant task cancellation fence")
	agentID := createHandlerTestAgent(t, "variant-task-cancel-"+uuid.NewString(), nil)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running','creative_order_item_plan',$2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_plan',
	          'creative_order_id',$3::uuid::text,'creative_order_item_id',$2::uuid::text
        ))
RETURNING id::text
`, agentID, itemID, orderID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	putVariant := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
			OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{}`), Revision: 1,
			Status: "running", CandidateState: "candidate", PrimarySize: "1080x1080",
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		return w
	}
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'cancelled', completed_at = now() WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- putVariant() }()
	select {
	case w := <-result:
		t.Fatalf("variant put bypassed task cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("variant put did not resume after task cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "cancelled") {
		t.Fatalf("cancelled task wrote variant = %d %s", w.Code, w.Body.String())
	}
	var variantCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_variant WHERE order_item_id = $1`, itemID).Scan(&variantCount); err != nil {
		t.Fatal(err)
	}
	if variantCount != 0 {
		t.Fatalf("cancelled planning task created %d variants", variantCount)
	}
}

func TestCreativeProductionDiagnosticDoesNotOutrunTaskCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "diagnostic task cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "diagnostic-task-cancel-"+uuid.NewString(), nil)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running','creative_order_item_production',$2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
	          'creative_order_id',$3::uuid::text,'creative_order_item_id',$2::uuid::text,
	          'variant_id',$4::uuid::text,'revision',1,
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var attachmentID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1,$2,'member',$3,'diagnostic-cancel.png','oss://creative/diagnostic-cancel.png','image/png',12)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&attachmentID); err != nil {
		t.Fatal(err)
	}
	putDiagnostic := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/diagnostic-assets", creativeOrderDiagnosticAssetInput{
			VariantID: variant.ID, TaskID: taskID, AttachmentID: attachmentID,
			SizeKey: "1080x1080", Revision: 1, Workflow: "creative_production", Label: "Prime context",
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderDiagnosticAsset(w, req)
		return w
	}
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'cancelled', completed_at = now() WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- putDiagnostic() }()
	select {
	case w := <-result:
		t.Fatalf("diagnostic put bypassed task cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("diagnostic put did not resume after task cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "no longer active") {
		t.Fatalf("cancelled task wrote diagnostic evidence = %d %s", w.Code, w.Body.String())
	}
	var diagnosticCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_diagnostic_asset WHERE variant_id = $1`, variant.ID).Scan(&diagnosticCount); err != nil {
		t.Fatal(err)
	}
	if diagnosticCount != 0 {
		t.Fatalf("cancelled production task created %d diagnostic assets", diagnosticCount)
	}
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
	if err != nil || status != "action_required" {
		t.Fatalf("terminal mixed order status = %q, %v; want action_required without a formal delivery", status, err)
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
	submissionKey := "creative:cancel-" + uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
UPDATE issue
SET metadata = jsonb_build_object('workflow', 'creative_order', 'creative_submission_key', $2::text)
WHERE id = $1
`, issueID, submissionKey); err != nil {
		t.Fatal(err)
	}
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by, submission_key)
VALUES ($1, $2, 'queued', '{}'::jsonb, $3, $4) RETURNING id::text
`, testWorkspaceID, issueID, testUserID, submissionKey).Scan(&orderID); err != nil {
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
	adjustmentIssueID := createCreativeDeliveryTestIssue(t, "cancel creative adjustment", issueID)
	var adjustmentTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, context)
VALUES ($1, $2, $3, 'running', $4::jsonb) RETURNING id::text
`, agentID, handlerTestRuntimeID(t), adjustmentIssueID, contextValue).Scan(&adjustmentTaskID); err != nil {
		t.Fatal(err)
	}
	for _, activeTaskID := range []string{taskID, directTaskID, adjustmentTaskID} {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_token (token_hash, task_id, agent_id, workspace_id, user_id, expires_at)
VALUES ($1, $2, $3, $4, $5, now() + interval '1 hour')
`, "creative-cancel-token-"+activeTaskID, activeTaskID, agentID, testWorkspaceID, testUserID); err != nil {
			t.Fatal(err)
		}
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
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM agent_task_queue WHERE id = $1`, adjustmentTaskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "cancelled" {
		t.Fatalf("adjustment issue task status = %q, want cancelled", taskStatus)
	}
	var retainedTokens int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM task_token WHERE task_id = ANY($1::uuid[])
`, []string{taskID, directTaskID, adjustmentTaskID}).Scan(&retainedTokens); err != nil {
		t.Fatal(err)
	}
	if retainedTokens != 0 {
		t.Fatalf("creative order cancellation retained %d task tokens", retainedTokens)
	}
	var activityCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_order_cancelled'`, issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("creative order cancellation activity count = %d, want 1", activityCount)
	}
	var cancelledSubmissionKey string
	if err := testPool.QueryRow(t.Context(), `SELECT submission_key FROM creative_order WHERE id = $1`, orderID).Scan(&cancelledSubmissionKey); err != nil {
		t.Fatal(err)
	}
	if cancelledSubmissionKey != "" {
		t.Fatalf("cancelled order submission key = %q, want empty", cancelledSubmissionKey)
	}
	var activeIssueKey string
	var submissionHistory json.RawMessage
	if err := testPool.QueryRow(t.Context(), `
SELECT COALESCE(metadata->>'creative_submission_key', ''), COALESCE(metadata->'creative_submission_history', '[]'::jsonb)::text
FROM issue WHERE id = $1
`, issueID).Scan(&activeIssueKey, &submissionHistory); err != nil {
		t.Fatal(err)
	}
	if activeIssueKey != "" || !strings.Contains(string(submissionHistory), submissionKey) {
		t.Fatalf("retired issue submission = %q %s", activeIssueKey, submissionHistory)
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

func TestDeleteCreativeOrderRequiresNoActiveTasksAndUnlinksIssue(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, _, issueID := createCreativeLifecycleTestOrder(t, "delete creative order")
	if _, err := testPool.Exec(t.Context(), `
UPDATE issue
SET metadata = jsonb_build_object('workflow', 'creative_order', 'creative_order_id', $2::text)
WHERE id = $1
`, issueID, orderID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-delete-state-"+uuid.NewString(), nil)
	contextValue, err := json.Marshal(map[string]any{"type": "creative_domain_task", "creative_order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, context)
VALUES ($1, $2, $3, 'queued', $4::jsonb)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), issueID, contextValue).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	activityDetails, err := json.Marshal(map[string]any{"creative_order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_order_cancelled', $4::jsonb)
`, testWorkspaceID, issueID, testUserID, activityDetails); err != nil {
		t.Fatal(err)
	}
	feedbackContext, err := json.Marshal(map[string]any{"creative_order_id": orderID})
	if err != nil {
		t.Fatal(err)
	}
	feedbackSubjectID := uuid.NewString()
	var feedbackID, undoFeedbackID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, decision, context_snapshot
) VALUES ($1, $2, 'member', $3, 'candidate', $4, 'decision', 'rejected', $5::jsonb)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID, feedbackSubjectID, feedbackContext).Scan(&feedbackID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, context_snapshot, undo_of_id
) VALUES ($1, $2, 'member', $3, 'candidate', $4, 'undo', $5::jsonb, $6)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID, feedbackSubjectID, feedbackContext, feedbackID).Scan(&undoFeedbackID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodDelete, "/api/creative/orders/"+orderID, nil), "id", orderID)
	testHandler.DeleteCreativeOrder(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "end the creative order") {
		t.Fatalf("delete active creative order = %d %s", w.Code, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'cancelled' WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}

	w = httptest.NewRecorder()
	testHandler.DeleteCreativeOrder(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete stopped creative order = %d %s", w.Code, w.Body.String())
	}
	var orderCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order WHERE id = $1`, orderID).Scan(&orderCount); err != nil {
		t.Fatal(err)
	}
	if orderCount != 0 {
		t.Fatalf("deleted creative order count = %d", orderCount)
	}
	var workflow, linkedOrderID string
	if err := testPool.QueryRow(t.Context(), `
SELECT COALESCE(metadata->>'workflow', ''), COALESCE(metadata->>'creative_order_id', '')
FROM issue WHERE id = $1
`, issueID).Scan(&workflow, &linkedOrderID); err != nil {
		t.Fatal(err)
	}
	if workflow != "" || linkedOrderID != "" {
		t.Fatalf("deleted creative order issue metadata = workflow %q order %q", workflow, linkedOrderID)
	}
	var activityCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM activity_log
WHERE workspace_id = $1 AND details->>'creative_order_id' = $2
`, testWorkspaceID, orderID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 0 {
		t.Fatalf("deleted creative order activity count = %d", activityCount)
	}
	var feedbackCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_feedback_event WHERE id = ANY($1::uuid[])
`, []string{feedbackID, undoFeedbackID}).Scan(&feedbackCount); err != nil {
		t.Fatal(err)
	}
	if feedbackCount != 0 {
		t.Fatalf("deleted creative order feedback count = %d", feedbackCount)
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

func TestUnadoptCreativeOrderItemVariantRetainsDeliveryPackage(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "unadopt creative order")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "completed")
	attachmentID := createCreativeOrderAssetAttachment(t, "unadopt-delivery.png")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 1, 'delivered', $2, 'completed')
`, variant.ID, attachmentID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_item
SET adopted_variant_id = $2, adopted_at = now(), adopted_by = $3
WHERE id = $1
`, itemID, variant.ID, testUserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_feedback_event WHERE subject_id = $1`, variant.ID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_unadopted'`, issueID)
	})

	w := httptest.NewRecorder()
	req := withURLParams(newRequest(http.MethodDelete, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", nil), "id", orderID, "itemId", itemID)
	testHandler.UnadoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UnadoptCreativeOrderItemVariant = %d %s", w.Code, w.Body.String())
	}
	var item creativeOrderItemResponse
	if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if item.AdoptedVariantID != "" || item.AdoptedAt != "" || item.AdoptedBy != "" {
		t.Fatalf("unadopted item = %#v", item)
	}
	var retainedDeliveryCount, activityCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_order_asset
WHERE variant_id = $1 AND stage = 'delivered' AND status = 'completed'
`, variant.ID).Scan(&retainedDeliveryCount); err != nil {
		t.Fatal(err)
	}
	if retainedDeliveryCount != 1 {
		t.Fatalf("retained delivery asset count = %d", retainedDeliveryCount)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_unadopted'
`, issueID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 1 {
		t.Fatalf("unadoption activity count = %d", activityCount)
	}
}

func TestBindCreativeOrderVariantFrozenContractPreservesOnlyDerivedExecution(t *testing.T) {
	brief, err := bindCreativeOrderVariantFrozenContract(json.RawMessage(`{
		"keep": "brief value",
		"prime_layout_contract": {"layouts": {"1080x1080": {"hard_regions": ["untrusted"]}}},
		"creative_contract": {
			"parent_direction": "untrusted direction",
			"parent_direction_sha256": "untrusted hash",
			"variant_execution": {"visual_identity_strategy": "stadium night"}
		}
	}`), "世界杯主题，保留真实比赛氛围。", json.RawMessage(`{
		"market_pack": {"config": {"prime_layout_contract": {
			"guide_policy": "full_transparent_template_bands",
			"layouts": {"1080x1080": {"hard_regions": [], "top_key_content_exclusion_end": 100, "bottom_key_content_exclusion_start": 984}}
		}}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(brief, &decoded); err != nil {
		t.Fatal(err)
	}
	contract, ok := decoded["creative_contract"].(map[string]any)
	if !ok {
		t.Fatalf("creative contract = %#v", decoded["creative_contract"])
	}
	direction := "世界杯主题，保留真实比赛氛围。"
	if contract["parent_direction"] != direction || contract["parent_direction_sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(direction))) {
		t.Fatalf("parent contract = %#v", contract)
	}
	execution, ok := contract["variant_execution"].(map[string]any)
	if !ok || execution["visual_identity_strategy"] != "stadium night" || decoded["keep"] != "brief value" {
		t.Fatalf("derived execution was not preserved: %#v", decoded)
	}
	layout, ok := decoded["prime_layout_contract"].(map[string]any)
	if !ok || layout["guide_policy"] != "full_transparent_template_bands" {
		t.Fatalf("frozen prime layout was not bound: %#v", decoded["prime_layout_contract"])
	}
}

func TestBindCreativeDirectEditContractForcesFinalVisualValidation(t *testing.T) {
	brief, err := bindCreativeOrderVariantFrozenContract(
		json.RawMessage(`{
			"delivery_mode":"preview",
			"target_size":"800x1000",
			"creative_direct_edit_delivery":{"skip_qc":true,"final_visual_validation":false}
		}`),
		"保留人物，只调整标题位置。",
		json.RawMessage(`{
			"pipeline_version":"direct_edit_v1",
			"delivery_mode":"publish",
			"target_size":"1080x1080",
			"user_request":"保留人物，只调整标题位置。"
		}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(brief), "skip_qc") {
		t.Fatalf("frozen direct edit contract retained skip_qc: %s", brief)
	}
	config := parseCreativeDirectEditDeliveryConfig(brief)
	if config.DeliveryMode != "publish" || config.TargetSize != "1080x1080" || !config.FinalVisualValidation || config.RawUserRequest != "保留人物，只调整标题位置。" {
		t.Fatalf("frozen direct edit contract = %#v", config)
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
		variantStatus := "completed"
		qcOutcome := "delivered"
		if visualStatus == "failed" {
			variantStatus = "action_required"
			qcOutcome = "action_required"
		}
		var variantID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, $2, 1, $3) RETURNING id::text
	`, itemID, key, variantStatus).Scan(&variantID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, $2, $3::text[])
`, variantID, variantStatus, standardCreativeAssetSizes); err != nil {
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
VALUES ($1, $2, 1, 'primed', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
				t.Fatal(err)
			}
			if qcOutcome == "delivered" {
				if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
					t.Fatal(err)
				}
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
VALUES ($1, 1, $2, $3)
`, variantID, qcOutcome, issueID); err != nil {
			t.Fatal(err)
		}
		if qcOutcome == "delivered" {
			tx, err := testPool.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variantID), 1, standardCreativeAssetSizes); err != nil {
				_ = tx.Rollback(t.Context())
				t.Fatal(err)
			}
			if err := tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		return variantID
	}
	firstVariantID := createEligibleVariant("adopt-v01", "passed", "passed")
	secondVariantID := createEligibleVariant("adopt-v02", "warning", "passed")
	riskVariantID := createEligibleVariant("adopt-v03", "passed", "failed")
	completedRiskVariantID := createEligibleVariant("adopt-v04", "passed", "failed")
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'completed' WHERE id = $1`, completedRiskVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant_qc_resolution SET outcome = 'delivered_with_qc_risk' WHERE variant_id = $1 AND revision = 1`, completedRiskVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
SELECT variant_id, size_key, revision, 'delivered', attachment_id, status
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'primed'
`, completedRiskVariantID); err != nil {
		t.Fatal(err)
	}
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

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: riskVariantID})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "explicit risk acknowledgement") {
		t.Fatalf("risk adoption without acknowledgement = %d %s", w.Code, w.Body.String())
	}

	riskReason := "Launch deadline accepted with a known visual QC issue"
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{
		VariantID: riskVariantID, QCRiskAcknowledged: true, QCRiskReason: riskReason,
	})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("risk adoption = %d %s", w.Code, w.Body.String())
	}
	var riskItem creativeOrderItemResponse
	if err := json.NewDecoder(w.Body).Decode(&riskItem); err != nil {
		t.Fatal(err)
	}
	if riskItem.AdoptedVariantID != riskVariantID {
		t.Fatalf("risk adoption item = %#v", riskItem)
	}
	var deliveredCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'delivered' AND status = 'completed'
`, riskVariantID).Scan(&deliveredCount); err != nil {
		t.Fatal(err)
	}
	if deliveredCount != len(standardCreativeAssetSizes) {
		t.Fatalf("risk adoption delivered assets = %d, want %d", deliveredCount, len(standardCreativeAssetSizes))
	}

	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: completedRiskVariantID})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "explicit risk acknowledgement") {
		t.Fatalf("completed risk adoption without acknowledgement = %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{
		VariantID: completedRiskVariantID, QCRiskAcknowledged: true, QCRiskReason: riskReason,
	})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("completed risk adoption = %d %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&riskItem); err != nil {
		t.Fatal(err)
	}
	if riskItem.AdoptedVariantID != completedRiskVariantID {
		t.Fatalf("completed risk adoption item = %#v", riskItem)
	}

	var reasonCodes []string
	var feedbackComment, contextSnapshot string
	if err := testPool.QueryRow(t.Context(), `
SELECT reason_codes, comment, context_snapshot::text
FROM creative_feedback_event
WHERE workspace_id = $1 AND subject_type = 'variant' AND subject_id = $2 AND decision = 'accepted'
ORDER BY created_at DESC LIMIT 1
`, testWorkspaceID, riskVariantID).Scan(&reasonCodes, &feedbackComment, &contextSnapshot); err != nil {
		t.Fatal(err)
	}
	if len(reasonCodes) != 1 || reasonCodes[0] != "qc_risk_accepted" || feedbackComment != riskReason ||
		!strings.Contains(contextSnapshot, `"adoption_mode": "qc_risk_accepted"`) ||
		!strings.Contains(contextSnapshot, `"visual_status": "failed"`) ||
		!strings.Contains(contextSnapshot, `"failure_summary"`) {
		t.Fatalf("risk adoption feedback = reasons %#v comment %q context %s", reasonCodes, feedbackComment, contextSnapshot)
	}

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
	if feedbackCount != 4 || activityCount != 4 {
		t.Fatalf("audit counts = feedback %d activity %d, want 4 each", feedbackCount, activityCount)
	}
}

func TestAdoptCreativeOrderItemVariantUsesActiveRevisionWhileStagingFails(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "active revision adoption")
	var orderID, itemID, variantID string
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
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, brief, revision, status
)
VALUES ($1, 'active-v01', '{"revision_marker":"staging"}'::jsonb, 2, 'action_required')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes, activated_at)
VALUES
  ($1, 1, '{"revision_marker":"active"}'::jsonb, 'completed', $2::text[], now()),
  ($1, 2, '{"revision_marker":"staging"}'::jsonb, 'action_required', $2::text[], NULL)
`, variantID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant SET active_revision = 1, staging_revision = 2 WHERE id = $1
`, variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		var attachmentID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, 'image/png', 1024) RETURNING id::text
`, testWorkspaceID, issueID, testUserID, "active-revision-adopt-"+size+".png", "/uploads/active-revision-adopt-"+size+".png").Scan(&attachmentID); err != nil {
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
VALUES ($1, 'technical', 1, 'passed', '{}'::jsonb),
       ($1, 'visual', 1, 'passed', '{}'::jsonb),
       ($1, 'technical', 2, 'passed', '{}'::jsonb),
       ($1, 'visual', 2, 'failed', '{"blocking_failures":[{"code":"staging_failed"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'delivered', $2), ($1, 2, 'action_required', $2)
`, variantID, issueID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_feedback_event WHERE subject_id = $1`, variantID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_adopted'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE workspace_id = $1 AND filename LIKE 'active-revision-adopt-%'`, testWorkspaceID)
	})

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: variantID})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("adopt active revision = %d %s", w.Code, w.Body.String())
	}

	var adoptedVariantID, variantStatus string
	var activeRevision, stagingRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT item.adopted_variant_id::text, variant.status,
       COALESCE(variant.active_revision, 0), COALESCE(variant.staging_revision, 0)
FROM creative_order_item item
JOIN creative_order_variant variant ON variant.id = item.adopted_variant_id
WHERE item.id = $1
`, itemID).Scan(&adoptedVariantID, &variantStatus, &activeRevision, &stagingRevision); err != nil {
		t.Fatal(err)
	}
	if adoptedVariantID != variantID || variantStatus != "action_required" || activeRevision != 1 || stagingRevision != 2 {
		t.Fatalf("adoption changed staging state = variant %q status %q active %d staging %d", adoptedVariantID, variantStatus, activeRevision, stagingRevision)
	}

	var feedbackSnapshot, activitySnapshot string
	if err := testPool.QueryRow(t.Context(), `
SELECT context_snapshot::text
FROM creative_feedback_event
WHERE workspace_id = $1 AND subject_type = 'variant' AND subject_id = $2 AND decision = 'accepted'
ORDER BY created_at DESC LIMIT 1
`, testWorkspaceID, variantID).Scan(&feedbackSnapshot); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT details::text
FROM activity_log
WHERE issue_id = $1 AND action = 'creative_variant_adopted'
ORDER BY created_at DESC LIMIT 1
`, issueID).Scan(&activitySnapshot); err != nil {
		t.Fatal(err)
	}
	for label, raw := range map[string]string{"feedback": feedbackSnapshot, "activity": activitySnapshot} {
		var snapshot struct {
			Revision   int `json:"revision"`
			QCSnapshot struct {
				VisualStatus string `json:"visual_status"`
				Outcome      string `json:"outcome"`
			} `json:"qc_snapshot"`
		}
		if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
			t.Fatalf("decode %s snapshot: %v", label, err)
		}
		if snapshot.Revision != 1 || snapshot.QCSnapshot.VisualStatus != "passed" || snapshot.QCSnapshot.Outcome != "delivered" {
			t.Fatalf("%s snapshot used staging revision: %#v", label, snapshot)
		}
	}

	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET status = 'failed', findings = '{"blocking_failures":[{"code":"accepted_risk"}]}'::jsonb
WHERE variant_id = $1 AND revision = 1 AND lane = 'visual'
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant_qc_resolution SET outcome = 'action_required'
WHERE variant_id = $1 AND revision = 1
`, variantID); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{
		VariantID: variantID, QCRiskAcknowledged: true, QCRiskReason: "Previously accepted active revision risk",
	})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("re-adopt active risk revision = %d %s", w.Code, w.Body.String())
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
VALUES ($1, 'running', '{"pipeline_version":"candidate_v1","expected_sizes":["1080x1080","1200x628","800x1000"]}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
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
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 1 || order.Items[0].Variants[0].ActionRequired == nil {
		t.Fatalf("reported action-required variant did not expose blocker detail: %#v", order.Items)
	}
	blocker := order.Items[0].Variants[0].ActionRequired
	if blocker.TaskID != taskID || blocker.Workflow != "creative_production" || blocker.Detail != "OPENAI_API_KEY is required for image generation" || !blocker.Retryable {
		t.Fatalf("variant blocker = %#v, want task-level production diagnostic", blocker)
	}

	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = max_attempts WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	if failure := getOrder().WorkflowFailures[0]; !failure.Retryable {
		t.Fatalf("second production action-required failure should keep one manual retry after a platform fix: %#v", failure)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = 3, max_attempts = 3 WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	if failure := getOrder().WorkflowFailures[0]; !failure.Retryable {
		t.Fatalf("third production action-required failure should keep one bounded recovery retry: %#v", failure)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = 4, max_attempts = 4 WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	if failure := getOrder().WorkflowFailures[0]; !failure.Retryable {
		t.Fatalf("fourth production action-required failure should allow one confirmed-repair retry: %#v", failure)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = 5, max_attempts = 5 WHERE id = $1`, taskID); err != nil {
		t.Fatal(err)
	}
	if failure := getOrder().WorkflowFailures[0]; failure.Retryable {
		t.Fatalf("fifth production action-required failure is retryable: %#v", failure)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt = 1, max_attempts = 2 WHERE id = $1`, taskID); err != nil {
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
	if child.AgentID != agentID || child.Status != "queued" || child.Attempt != 2 || child.RetryOfTaskID != taskID || child.SessionID != "" || child.WorkDir != "" || !child.FreshSession {
		t.Fatalf("recovery child lineage = %#v, want queued retry with clean execution state", child)
	}
	for _, key := range []string{"type", "workflow", "creative_order_id", "creative_order_item_id", "variant_id", "expected_sizes"} {
		if fmt.Sprint(childContext[key]) != fmt.Sprint(parentContext[key]) {
			t.Fatalf("recovery child context[%s]=%#v, want %#v in %#v", key, childContext[key], parentContext[key], childContext)
		}
	}
	if childContext["scope"] != "variant" || childContext["subject_id"] != variantID || childContext["revision"] != float64(1) || childContext["item_key"] != variantID+":r1" {
		t.Fatalf("recovery child context was not normalized to current production identity: %#v", childContext)
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

func TestCreativeOrderWorkflowFailuresLinkManualCreativeTaskByVariant(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "manual creative production trace")
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	contextValue, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production",
		"variant_id": variantID, "revision": 1, "item_key": variantID + ":r1",
	})
	if err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "manual-creative-trace-"+uuid.NewString(), nil)
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'manual', $4, now(), 1, 2)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), contextValue, orderID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_message (task_id, seq, type, content)
VALUES ($1, 1, 'text', 'manual production task did not register generated assets')
`, taskID); err != nil {
		t.Fatal(err)
	}

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
	if order.DerivedStatus != "action_required" || len(order.WorkflowFailures) != 1 {
		t.Fatalf("manual creative task not linked to order: %#v", order)
	}
	if order.WorkflowFailures[0].TaskID != taskID || order.WorkflowFailures[0].TriggerEvidenceKind != "manual" {
		t.Fatalf("manual creative failure = %#v", order.WorkflowFailures[0])
	}
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 1 || order.Items[0].Variants[0].ActionRequired == nil {
		t.Fatalf("manual creative variant blocker missing: %#v", order.Items)
	}
}

func TestCreativeOrderWorkflowFailuresExposeCompletedProductionWithoutArtifacts(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "completed production without artifacts")
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'queued') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	contextValue, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r0",
		"revision": 0,
		"qc_visual_rework": map[string]any{
			"target_sizes": []string{"1080x1080"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-completed-no-artifacts-"+uuid.NewString(), nil)
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  issue_id, completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_item_production', $4, $5, now(), 1, 2)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), contextValue, itemID, issueID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO task_message (task_id, seq, type, content)
VALUES ($1, 1, 'text', 'Production task ended without registering generated assets')
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
		t.Fatalf("stalled order = %#v, want action_required with one failure", order)
	}
	failure := order.WorkflowFailures[0]
	if failure.TaskID != taskID || failure.FailureReason != "agent_reported_action_required" || !failure.Retryable {
		t.Fatalf("stalled failure = %#v", failure)
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
	var childContext map[string]any
	var childStatus, variantStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT status, context FROM agent_task_queue WHERE id = $1`, retried.TaskID).Scan(&childStatus, &contextValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(contextValue, &childContext); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, variantID).Scan(&variantStatus); err != nil {
		t.Fatal(err)
	}
	if childStatus != "queued" || variantStatus != "running" || childContext["revision"] != float64(1) || childContext["item_key"] != variantID+":r1" {
		t.Fatalf("retry child status=%q variant=%q context=%#v", childStatus, variantStatus, childContext)
	}
}

func TestRetryCreativeOrderWorkflowFailureRejectsDirectQCRetry(t *testing.T) {
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
			if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "visual QC recovery") {
				t.Fatalf("direct %s retry = %d %s", workflow, w.Code, w.Body.String())
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
		"revision": 2, "expected_sizes": expectedSizes,
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
UPDATE creative_order_variant SET revision = 3, status = 'running' WHERE id = $1
`, variantID); err != nil {
		t.Fatal(err)
	}
	if failures := getOrder().WorkflowFailures; len(failures) != 0 {
		t.Fatalf("stale revision production failure leaked into current order: %#v", failures)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant SET revision = 2, status = 'action_required' WHERE id = $1
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '800x1000', 2, 'generated', '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID); err != nil {
		t.Fatal(err)
	}
}

func TestCreativeOrderVariantBlockerIgnoresCompletedParentWithActiveContinuation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "active production continuation blocker")
	var orderID, itemID, variantID, parentTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'running', '{}'::jsonb, $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM task_message WHERE task_id IN (SELECT id FROM agent_task_queue WHERE context->>'creative_order_id' = $1)`, orderID)
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
VALUES ($1, 'V01', 2, 'running') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	productionAgentID := createHandlerTestAgent(t, "creative-production-continuation-"+uuid.NewString(), nil)
	productionContext, err := json.Marshal(map[string]any{
		"type": "creative_domain_task", "workflow": "creative_production", "creative_order_id": orderID,
		"creative_order_item_id": itemID, "variant_id": variantID, "scope": "variant", "item_key": variantID + ":r2",
		"revision": 2, "expected_sizes": standardCreativeAssetSizes,
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
`, productionAgentID, handlerTestRuntimeID(t), productionContext, itemID).Scan(&parentTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  attempt, max_attempts, parent_task_id, started_at
)
VALUES ($1, $2, 'running', $3::jsonb, 'creative_order_item_production', $4, 2, 3, $5, now())
`, productionAgentID, handlerTestRuntimeID(t), productionContext, itemID, parentTaskID); err != nil {
		t.Fatal(err)
	}

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
	if order.DerivedStatus != "running" || len(order.WorkflowFailures) != 0 {
		t.Fatalf("active continuation order state = status %q failures %#v", order.DerivedStatus, order.WorkflowFailures)
	}
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 1 {
		t.Fatalf("active continuation order items = %#v", order.Items)
	}
	variant := order.Items[0].Variants[0]
	if variant.Status != "running" || variant.ActionRequired != nil {
		t.Fatalf("active continuation variant = status %q blocker %#v", variant.Status, variant.ActionRequired)
	}
}

func TestCreativeOrderWorkflowFailuresIgnoreCompletedQCWarningWhileOtherLaneIsPending(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "completed QC warning")
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v01', 1, 'running') RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'warning', '{"blocking_failures":[]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-qc-warning-"+uuid.NewString(), nil)
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  completed_at, attempt, max_attempts
)
VALUES ($1, $2, 'completed', $3::jsonb, 'creative_order_variant_qc', $4, now(), 1, 2)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), creativeQCTaskContextForTest(t, orderID, variantID, "technical"), variantID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })

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
	if len(order.WorkflowFailures) != 0 {
		t.Fatalf("completed QC warning was exposed as workflow failure: %#v", order.WorkflowFailures)
	}
	if len(order.Items) != 1 || len(order.Items[0].Variants) != 1 || order.Items[0].Variants[0].ActionRequired != nil {
		t.Fatalf("completed QC warning exposed a variant blocker: %#v", order.Items)
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
				"type": "creative_domain_task", "workflow": "creative_production", "creative_order_id": orderID,
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
VALUES ($1, $2, 'failed', $3::jsonb, 'creative_order_item_production', $4, 'agent_error', 'production failed', now(), 1, 2)
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
			if test.variantStatus == "completed" && (order.ProductionStatus != "completed" || order.DeliveryStatus != "pending") {
				t.Fatalf("completed production without formal delivery = production %q delivery %q", order.ProductionStatus, order.DeliveryStatus)
			}
			if len(order.WorkflowFailures) != 1 || order.WorkflowFailures[0].Scope != "variant" || order.WorkflowFailures[0].SubjectID != variantID {
				t.Fatalf("inferred workflow failure scope = %#v", order.WorkflowFailures)
			}
		})
	}
}

func TestCreativeOrderStatusMarksQueuedOrderWithoutActiveTaskAsActionRequired(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "stale queued creative order")
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, created_by)
VALUES ($1, 'queued', '{}'::jsonb, $2)
RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb)
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, status)
VALUES ($1, 'V01', 'queued')
`, itemID); err != nil {
		t.Fatal(err)
	}

	status, err := testHandler.derivedCreativeOrderStatus(newRequest(http.MethodGet, "/", nil), parseUUID(orderID))
	if err != nil {
		t.Fatal(err)
	}
	if status != "action_required" {
		t.Fatalf("derived status = %q, want action_required for a queued order without active work", status)
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
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'visual', 2, 1, 'failed', '{"blocking_failures":[{"code":"visual_quality_failure"}]}'::jsonb),
       ($1, 'visual', 2, 2, 'passed', '{}'::jsonb)`, variantID); err != nil {
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
VALUES ($1, 'running', '{"pipeline_version":"candidate_v1","expected_sizes":["1080x1080","1200x628","800x1000"]}'::jsonb, $2) RETURNING id::text`, testWorkspaceID, testUserID).Scan(&orderID); err != nil {
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
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"version":1}`), Revision: 1, Status: "queued",
		CandidateState: "candidate", PrimarySize: "1080x1080",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create revision 1 variant: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "queued",
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
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "partial",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("advance same variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"version":2}`), Revision: 2, Status: "running",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("regress same variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"version":1}`), Revision: 1, Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale variant revision: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	metadata, evidence := completedGeneratedAssetTrace("stale prompt", "req-stale", 1)
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
		AttachmentID: createCreativeOrderAssetAttachment(t, "stale.png"), Metadata: metadata, Evidence: evidence,
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

func completedGeneratedAssetTrace(prompt, requestID string, attempts int) (json.RawMessage, json.RawMessage) {
	promptSHA256 := creativePromptSHA256(prompt)
	modelResult := map[string]any{
		"model": "gpt-image-2", "prompt": prompt, "prompt_sha256": promptSHA256,
		"request_id": requestID, "attempts": attempts,
		"actual_width": 1088, "actual_height": 1088, "actual_aspect_ratio": 1.0,
	}
	metadata, _ := json.Marshal(map[string]any{
		"prompt": prompt, "model": "gpt-image-2",
		"actual_width": 1088, "actual_height": 1088, "actual_aspect_ratio": 1.0,
	})
	evidence, _ := json.Marshal(map[string]any{
		"request_id": requestID, "attempts": attempts, "prompt_sha256": promptSHA256,
		"model_result": modelResult, "prompt_contract": map[string]any{"prompt_sha256": promptSHA256},
		"normalization": map[string]any{"target_size": map[string]int{"width": 1080, "height": 1080}},
	})
	return metadata, evidence
}

func completedGeneratedAssetTraceForSize(prompt, requestID string, attempts int, size string) (json.RawMessage, json.RawMessage) {
	metadata, evidence := completedGeneratedAssetTrace(prompt, requestID, attempts)
	width, height, ok := creativeAssetSizeDimensions(size)
	if !ok {
		return metadata, evidence
	}
	var evidenceObject map[string]any
	if json.Unmarshal(evidence, &evidenceObject) == nil {
		evidenceObject["normalization"] = map[string]any{"target_size": map[string]int{"width": width, "height": height}}
		evidence, _ = json.Marshal(evidenceObject)
	}
	return metadata, evidence
}

func TestValidateCreativeGeneratedAssetNormalizationTarget(t *testing.T) {
	_, evidence := completedGeneratedAssetTrace("normalization target", "req-normalization-target", 1)
	input := creativeOrderAssetInput{SizeKey: "1080x1080", Stage: "generated", Status: "completed", Evidence: evidence}
	if err := validateCreativeGeneratedAssetNormalizationTarget(input); err != nil {
		t.Fatalf("matching normalization target: %v", err)
	}

	var changed map[string]any
	if err := json.Unmarshal(evidence, &changed); err != nil {
		t.Fatal(err)
	}
	changed["normalization"] = map[string]any{"target_size": map[string]int{"width": 1200, "height": 628}}
	input.Evidence, _ = json.Marshal(changed)
	if err := validateCreativeGeneratedAssetNormalizationTarget(input); err == nil || !strings.Contains(err.Error(), "target_size 1200x628 does not match 1080x1080") {
		t.Fatalf("mismatched normalization target error = %v", err)
	}
}

func TestUpsertCreativeOrderAssetRejectsGeneratedAttachmentDimensionMismatch(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "generated asset dimension mismatch")
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, $2, 'running', '{"expected_sizes":["1080x1080"]}'::jsonb, 'manual', $3)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'running')
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'running')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}

	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })
	store.put("oss://creative/bad-generated.png", creativeTestPNG(t, 1254, 1254))
	attachmentID := createCreativeOrderAssetAttachment(t, "bad-generated.png")
	metadata, evidence := completedGeneratedAssetTrace("dimension mismatch", "req-dimension-mismatch", 1)
	operationID := createCompletedCreativeImageOperation(t, variantID, "1080x1080", "dimension mismatch", "req-dimension-mismatch")

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: variantID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
		AttachmentID: attachmentID, OperationID: operationID, Metadata: metadata, Evidence: evidence,
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderAsset(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "dimensions 1254x1254 do not match 1080x1080") {
		t.Fatalf("dimension mismatch response = %d %s", w.Code, w.Body.String())
	}
}

func creativeTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestValidateCompletedGeneratedAssetTraceAllowsLegacyFailedCopyValidation(t *testing.T) {
	metadata, evidence := completedGeneratedAssetTrace("legacy failed copy validation", "req-copy-warning", 1)
	var object map[string]any
	if err := json.Unmarshal(evidence, &object); err != nil {
		t.Fatal(err)
	}
	object["copy_validation"] = map[string]any{"passed": false, "reason": "legacy warning"}
	evidence, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCompletedGeneratedAssetTrace(metadata, evidence); err != nil {
		t.Fatalf("legacy failed copy validation should not block generated trace: %v", err)
	}
}

func createCreativeOrderAssetAttachment(t *testing.T, filename string) string {
	t.Helper()
	var attachmentID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, 'member', $2, $3, $4, 'image/png', 1024)
RETURNING id::text
`, testWorkspaceID, testUserID, filename, "oss://creative/"+filename).Scan(&attachmentID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE id = $1`, attachmentID) })
	return attachmentID
}

func createCompletedCreativeImageOperation(t *testing.T, variantID, size, prompt, requestID string) string {
	t.Helper()
	rawAttachmentID := createCreativeOrderAssetAttachment(t, "provider-raw-"+uuid.NewString()+".png")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
SELECT id, revision, brief, status, ARRAY['1080x1080','1200x628','800x1000']::text[]
FROM creative_order_variant
WHERE id = $1
ON CONFLICT (variant_id, revision) DO NOTHING
`, variantID); err != nil {
		t.Fatal(err)
	}
	var operationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, model,
  prompt_sha256, provider_request_id, result_receipt, output_attachment_id,
  started_at, completed_at
)
VALUES ($1, $2, 1, 'generation', $3, 'completed', 'gpt-image-2',
        $4, $5, jsonb_build_object('provider_status','succeeded'), $6, now(), now())
RETURNING id::text
`, variantID, size, "fixture:"+uuid.NewString(), creativePromptSHA256(prompt), requestID, rawAttachmentID).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (
  operation_id, attempt, status, provider_request_id, result_receipt, output_attachment_id, completed_at
)
VALUES ($1, 1, 'completed', $2, jsonb_build_object('provider_status','succeeded'), $3, now())
`, operationID, requestID, rawAttachmentID); err != nil {
		t.Fatal(err)
	}
	return operationID
}

func TestCreativeOrderGeneratedAssetRequiresAndFreezesPromptTrace(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "generated prompt trace")
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'running') RETURNING id::text`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}

	attachmentIDs := map[string]string{}
	operationIDs := map[string]string{}
	put := func(size string, metadata, evidence json.RawMessage) *httptest.ResponseRecorder {
		t.Helper()
		attachmentID := attachmentIDs[size]
		if attachmentID == "" {
			attachmentID = createCreativeOrderAssetAttachment(t, "generated-"+strings.ReplaceAll(size, "x", "-")+".png")
			attachmentIDs[size] = attachmentID
		}
		operationID := ""
		if validateCompletedGeneratedAssetTrace(metadata, evidence) == nil {
			var trace struct {
				Prompt string `json:"prompt"`
			}
			var receipt struct {
				RequestID string `json:"request_id"`
			}
			if json.Unmarshal(metadata, &trace) == nil && json.Unmarshal(evidence, &receipt) == nil {
				operationID = operationIDs[size]
				if operationID == "" {
					operationID = createCompletedCreativeImageOperation(t, variantID, size, trace.Prompt, receipt.RequestID)
					operationIDs[size] = operationID
				}
			}
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variantID, SizeKey: size, Revision: 1, Stage: "generated", Status: "completed",
			AttachmentID: attachmentID, OperationID: operationID, Metadata: metadata, Evidence: evidence,
		})
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderAsset(w, req)
		return w
	}

	metadata, evidence := completedGeneratedAssetTrace("exact final prompt", "req-prompt-1", 2)
	first := put("1080x1080", metadata, evidence)
	if first.Code != http.StatusOK {
		t.Fatalf("first generated asset = %d %s", first.Code, first.Body.String())
	}
	var firstAsset creativeOrderAssetResponse
	if err := json.NewDecoder(first.Body).Decode(&firstAsset); err != nil {
		t.Fatal(err)
	}
	replayed := put("1080x1080", metadata, evidence)
	if replayed.Code != http.StatusOK {
		t.Fatalf("idempotent generated asset = %d %s", replayed.Code, replayed.Body.String())
	}
	var replayedAsset creativeOrderAssetResponse
	if err := json.NewDecoder(replayed.Body).Decode(&replayedAsset); err != nil {
		t.Fatal(err)
	}
	if replayedAsset.ID != firstAsset.ID || replayedAsset.AssetFamilyID != firstAsset.AssetFamilyID {
		t.Fatalf("idempotent replay changed asset identity: first=%#v replay=%#v", firstAsset, replayedAsset)
	}

	differentMetadata, differentEvidence := completedGeneratedAssetTrace("different prompt", "req-prompt-2", 1)
	changed := put("1080x1080", differentMetadata, differentEvidence)
	if changed.Code != http.StatusConflict || !strings.Contains(changed.Body.String(), "lineage does not match the image operation") {
		t.Fatalf("different completed trace = %d %s", changed.Code, changed.Body.String())
	}
	noCopyMetadata, noCopyEvidence := completedGeneratedAssetTraceForSize("prompt without copy validation", "req-prompt-3", 1, "1200x628")
	withoutCopyValidation := put("1200x628", noCopyMetadata, noCopyEvidence)
	if withoutCopyValidation.Code != http.StatusOK {
		t.Fatalf("generated asset without copy validation = %d %s", withoutCopyValidation.Code, withoutCopyValidation.Body.String())
	}
	missing := put("800x1000", json.RawMessage(`{"prompt":"missing hash","model":"gpt-image-2","actual_width":800,"actual_height":992,"actual_aspect_ratio":0.8064516129}`), json.RawMessage(`{"request_id":"req-missing","attempts":1}`))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "prompt_sha256") {
		t.Fatalf("missing prompt hash = %d %s", missing.Code, missing.Body.String())
	}
	withoutAttachment := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: variantID, SizeKey: "800x1000", Revision: 1, Stage: "generated", Status: "completed",
		Metadata: metadata, Evidence: evidence,
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderAsset(withoutAttachment, req)
	if withoutAttachment.Code != http.StatusBadRequest || !strings.Contains(withoutAttachment.Body.String(), "attachment_id") {
		t.Fatalf("completed asset without attachment = %d %s", withoutAttachment.Code, withoutAttachment.Body.String())
	}

	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, metadata, evidence, status)
VALUES ($1, '800x1000', 1, 'generated', '{}'::jsonb, '{}'::jsonb, 'completed')`, variantID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req = newRequest(http.MethodGet, "/api/creative/orders/"+orderID, nil)
	req = withURLParam(req, "id", orderID)
	testHandler.GetCreativeOrder(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"size_key":"800x1000"`) {
		t.Fatalf("historical generated asset read = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeOrderAssetPutAcceptsAgentEnvelopeAliases(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "asset envelope alias")
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V01', 1, 'running') RETURNING id::text`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}

	assetPayload := func(size, prompt, requestID string) map[string]any {
		metadata, evidence := completedGeneratedAssetTraceForSize(prompt, requestID, 1, size)
		var metadataObject, evidenceObject map[string]any
		if err := json.Unmarshal(metadata, &metadataObject); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(evidence, &evidenceObject); err != nil {
			t.Fatal(err)
		}
		return map[string]any{
			"variant_id":    variantID,
			"operation_id":  createCompletedCreativeImageOperation(t, variantID, size, prompt, requestID),
			"size":          size,
			"revision":      1,
			"stage":         "generated",
			"status":        "completed",
			"attachment_id": createCreativeOrderAssetAttachment(t, "alias-"+strings.ReplaceAll(size, "x", "-")+".png"),
			"metadata":      metadataObject,
			"evidence":      evidenceObject,
		}
	}
	cases := []struct {
		name string
		body any
	}{
		{name: "size alias", body: assetPayload("1080x1080", "alias prompt", "req-alias")},
		{name: "asset wrapper", body: map[string]any{"asset": assetPayload("1200x628", "wrapped prompt", "req-wrapper")}},
		{name: "legacy asset type with inferred completion", body: func() map[string]any {
			metadata, evidence := completedGeneratedAssetTraceForSize("legacy kind prompt", "req-legacy-kind", 1, "800x1000")
			var metadataObject, evidenceObject map[string]any
			if err := json.Unmarshal(metadata, &metadataObject); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(evidence, &evidenceObject); err != nil {
				t.Fatal(err)
			}
			return map[string]any{
				"variant_id":    variantID,
				"operation_id":  createCompletedCreativeImageOperation(t, variantID, "800x1000", "legacy kind prompt", "req-legacy-kind"),
				"size_key":      "800x1000",
				"asset_type":    "generated",
				"attachment_id": createCreativeOrderAssetAttachment(t, "legacy-kind.png"),
				"metadata":      metadataObject,
				"evidence":      evidenceObject,
			}
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", tc.body)
			req = withURLParam(req, "id", orderID)
			testHandler.UpsertCreativeOrderAsset(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("asset put = %d %s", w.Code, w.Body.String())
			}
		})
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

func TestCreativeOrderQCWriteAppendsAttemptAfterFinalizedRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "append QC attempt")
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
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'visual', 1, 1, 'failed', '{"blocking_failures":[{"code":"visual_quality_failure"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, attempt, outcome)
VALUES ($1, 1, 1, 'action_required')
`, variantID); err != nil {
		t.Fatal(err)
	}

	agentID := createHandlerTestAgent(t, "creative-qc-append-attempt-"+uuid.NewString(), nil)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1, $2, 'running', $3::jsonb, 'creative_order_variant_qc', $4)
RETURNING id::text
`, agentID, handlerTestRuntimeID(t), creativeQCTaskContextForTest(t, orderID, variantID, "visual", 1, 2), variantID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/qc-reports", creativeOrderQCInput{
		VariantID: variantID, Lane: "visual", Revision: 1, Status: "passed",
	})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	testHandler.UpsertCreativeOrderQC(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("append QC attempt write = %d %s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response["attempt"] != float64(2) {
		t.Fatalf("append QC response = %#v, want attempt 2", response)
	}
	var attempt1Status, attempt2Status string
	if err := testPool.QueryRow(t.Context(), `
SELECT
  COALESCE(max(status) FILTER (WHERE attempt = 1), ''),
  COALESCE(max(status) FILTER (WHERE attempt = 2), '')
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = 1 AND lane = 'visual'
`, variantID).Scan(&attempt1Status, &attempt2Status); err != nil {
		t.Fatal(err)
	}
	if attempt1Status != "failed" || attempt2Status != "passed" {
		t.Fatalf("QC attempt statuses = %q/%q, want failed/passed", attempt1Status, attempt2Status)
	}
}

func TestCreativeOrderVariantQCRecoveryUsageIsExposedAndCanAppendAttempts(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "used QC recovery")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]string{
			"leader_agent_id":   fixture.LeaderAgentID,
			"producer_agent_id": fixture.ProducerAgentID,
			"reviewer_agent_id": fixture.ReviewerAgentID,
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
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-used-recovery-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 1, 'primed', $3, '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	agentID := fixture.ReviewerAgentID
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
	if !variant.QCRecoveryUsed || !variant.QCRecoveryAvailable {
		t.Fatalf("QC recovery flags = used:%t available:%t, want true:true", variant.QCRecoveryUsed, variant.QCRecoveryAvailable)
	}

	w = httptest.NewRecorder()
	retry := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variantID+"/qc/retry", nil), "id", orderID, "variantId", variantID)
	testHandler.RetryCreativeOrderVariantQC(w, retry)
	if w.Code != http.StatusOK {
		t.Fatalf("previously used QC recovery append = %d %s", w.Code, w.Body.String())
	}
	var response creativeOrderQCRetryResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, response.VisualTaskID)
	})
	if response.Attempt != 2 || response.Revision != 1 || response.VisualTaskID == "" {
		t.Fatalf("QC recovery response = %#v, want attempt 2 visual task", response)
	}
	var reportCount, resolutionCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_qc_report WHERE variant_id = $1 AND revision = 1`, variantID).Scan(&reportCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = 1`, variantID).Scan(&resolutionCount); err != nil {
		t.Fatal(err)
	}
	if reportCount != 1 || resolutionCount != 1 {
		t.Fatalf("previous QC rows were not preserved: reports=%d resolutions=%d", reportCount, resolutionCount)
	}
	var retryContext []byte
	if err := testPool.QueryRow(t.Context(), `SELECT context FROM agent_task_queue WHERE id = $1`, response.VisualTaskID).Scan(&retryContext); err != nil {
		t.Fatal(err)
	}
	var retryContextObject map[string]any
	if err := json.Unmarshal(retryContext, &retryContextObject); err != nil {
		t.Fatal(err)
	}
	if retryContextObject["qc_attempt"] != float64(2) || retryContextObject["qc_rerun_of_attempt"] != float64(1) {
		t.Fatalf("retry context = %#v, want attempt 2 rerun of attempt 1", retryContextObject)
	}
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
	if len(ids) != 1 || ids[0] != legacyTasks["visual"] {
		t.Fatalf("legacy recoverable QC task ids = %#v, want visual %s", ids, legacyTasks["visual"])
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
	if len(ids) != 0 {
		t.Fatalf("quality failure was incorrectly recoverable: %#v", ids)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET findings = '{"blocking_failures":[{"code":"visual_inspection_model_unconfigured"}]}'::jsonb
WHERE variant_id = $1 AND lane = 'visual'
`, variantID); err != nil {
		t.Fatal(err)
	}
	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(variantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != legacyTasks["visual"] {
		t.Fatalf("legacy visual inspection system failure ids = %#v, want visual %s", ids, legacyTasks["visual"])
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
			findings = `{"blocking_failures":[{"code":"manifest_layout_contract_missing"},{"code":"edge_white_ratio_needs_visual_review"}]}`
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
	if len(ids) != 0 {
		t.Fatalf("technical contract failure was incorrectly recoverable: %#v", ids)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET status = 'failed',
    findings = '{"blocking_failures":[{"code":"manifest_layout_contract_missing"},{"code":"edge_white_ratio_needs_visual_review"}]}'::jsonb
WHERE variant_id = $1 AND lane = 'visual'
`, currentVariantID); err != nil {
		t.Fatal(err)
	}
	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(currentVariantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != currentTasks["visual"] {
		t.Fatalf("current visual contract failure task ids = %#v, want visual %s", ids, currentTasks["visual"])
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_qc_report
SET findings = '{"blocking_failures":[{"code":"edge_white_ratio_needs_visual_review"}]}'::jsonb
WHERE variant_id = $1 AND lane = 'visual'
`, currentVariantID); err != nil {
		t.Fatal(err)
	}
	ids, err = creativeRecoverableQCTaskIDs(t.Context(), tx, parseUUID(orderID), parseUUID(currentVariantID), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("pure visual warning was incorrectly recoverable: %#v", ids)
	}
}

func TestCreativeVisualModelReworkFindingsAcceptsOnlyFinalVisualDefects(t *testing.T) {
	expectedSizes := []string{"1080x1080", "1200x628", "800x1000"}
	findings, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：还款表格 与 bottom official Prime template content 冲突；期望移动到 safe_content_frame 内 y<=430"},
    {"code":"official_prime_text_unreadable","size_key":"800x1000","diagnosis":"800x1000：条款下方深色背景 与 top Prime terms 冲突；期望调整为该组件下方连续、低细节的浅色背景"}
  ]
}`), expectedSizes)
	if err != nil || len(findings) != 2 {
		t.Fatalf("valid visual findings = %#v, %v", findings, err)
	}
	mixedFindings, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：还款表格下沿与底部 Prime 法务文字实际叠压；期望移动到 safe_content_frame 内 y<=566"},
    {"code":"cross_size_design_dna_mismatch","size_key":"800x1000","diagnosis":"800x1000：与其他尺寸的排版结构不一致"}
  ]
}`), expectedSizes)
	if err != nil || len(mixedFindings) != 1 || mixedFindings[0].SizeKey != "1200x628" {
		t.Fatalf("mixed-size visual rework findings = %#v, %v", mixedFindings, err)
	}
	perSizeFindings, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：还款表格下沿与底部 Prime 法务文字实际叠压；期望移动到 safe_content_frame 内 y<=566"},
    {"code":"generated_content_missing","size_key":"1200x628","diagnosis":"1200x628：冻结文案缺失"},
    {"code":"official_prime_text_unreadable","size_key":"800x1000","diagnosis":"800x1000：条款下方深色背景 与 top Prime terms 冲突；期望调整为该组件下方连续、低细节的浅色背景"}
  ]
}`), expectedSizes)
	if err != nil || len(perSizeFindings) != 1 || perSizeFindings[0].SizeKey != "800x1000" {
		t.Fatalf("same-size blocking visual findings = %#v, %v", perSizeFindings, err)
	}
	if _, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：冻结标题首行进入顶部 Prime 禁区并与 AdaKami Logo 实际重叠；期望移动到 safe_content_frame 内 y>=107。"}
  ]
}`), expectedSizes); err != nil {
		t.Fatalf("agent-produced obstruction diagnosis rejected: %v", err)
	}
	if _, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：还款表格下沿与底部 Prime 法务文字实际叠压；期望移动到 safe_content_frame 内 y<=566"}
  ]
}`), expectedSizes); err != nil {
		t.Fatalf("overlap-synonym obstruction diagnosis rejected: %v", err)
	}
	if _, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"official_prime_text_unreadable","size_key":"1200x628","diagnosis":"1200x628：官方顶栏条款和底部合规文字在深色高细节背景上不可读；期望调整为官方模板文字下方连续、低细节的浅色背景"}
  ]
}`), expectedSizes); err != nil {
		t.Fatalf("unreadable official text diagnosis rejected: %v", err)
	}
	if _, err := creativeVisualModelReworkFindings(json.RawMessage(`{
  "blocking_failures": [
    {"code":"official_prime_text_unreadable","size_key":"800x1000","diagnosis":"800x1000：右上官方条款在最终 Prime 成图的深色背景上不可读；selected template foreground_polarity=dark，background_support.polarity=dark，relative luminance p10=1.0295 低于 threshold=3、texture p90=0.086863（threshold=0.18）；期望承托区匹配结构化极性并达到证据中的相对亮度与纹理门槛"}
  ]
}`), expectedSizes); err != nil {
		t.Fatalf("structured official text support diagnosis rejected: %v", err)
	}
	for _, invalid := range []json.RawMessage{
		json.RawMessage(`{"blocking_failures":["normal visual overlap"]}`),
		json.RawMessage(`{"blocking_failures":[{"code":"predicted_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：标题 与 bottom Prime 冲突；期望移动到 safe_content_frame 内 y<=430"}]}`),
		json.RawMessage(`{"blocking_failures":[{"code":"generated_content_missing","size_key":"1080x1080","diagnosis":"1080x1080：利益点组件 与 冻结文案缺失；期望补齐冻结文案 copy_snapshot"}]}`),
		json.RawMessage(`{"blocking_failures":[{"code":"official_prime_text_unreadable","size_key":"1200x628","diagnosis":"1200x628：官方条款不可读；期望承托区匹配结构化极性"}]}`),
		json.RawMessage(`{"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：标题 与 top official Prime template content 冲突；期望移动到 safe_content_frame 内 y<=430"},{"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：表格 与 bottom official Prime template content 冲突；期望移动到 safe_content_frame 内 y<=430"}]}`),
	} {
		if _, err := creativeVisualModelReworkFindings(invalid, expectedSizes); err == nil {
			t.Fatalf("invalid visual finding unexpectedly accepted: %s", invalid)
		}
	}
}

func TestCreativePrimeCriticalReadabilityFindingsFenceVisualQCBypass(t *testing.T) {
	criticalEvidence := func(contrast, texture, textureThreshold float64) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{
  "template_selection":{"visual_adequacy":{"status":"qc_risk"}},
  "visibility_audit":{"blocking":true,"background_support":{
    "relative_luminance_contrast":{"minimum_local_p10":%.4f},
    "texture":{"maximum_local_p90":%.6f,"threshold":%.6f}
  }}
}`, contrast, texture, textureThreshold))
	}
	inputs := map[string]json.RawMessage{
		"1080x1080": criticalEvidence(1.0345, 0.227531, 0.18),
		"1200x628":  criticalEvidence(1.2408, 0.076344, 0.18),
		"800x1000":  criticalEvidence(1.3690, 0.149559, 0.18),
	}
	critical := make([]creativeVisualModelReworkFinding, 0, 2)
	for size, evidence := range inputs {
		if finding, found := creativePrimeCriticalReadabilityFinding(size, evidence); found {
			critical = append(critical, finding)
		}
	}
	if len(critical) != 2 {
		t.Fatalf("critical readability findings = %#v, want square and landscape", critical)
	}
	input, err := mergeCreativePrimeCriticalReadabilityFailures(creativeOrderQCInput{
		Lane:   "visual",
		Status: "passed",
		Findings: json.RawMessage(`{
  "checked_assets":[{"size_key":"800x1000"}],
  "quality_warnings":[],
  "blocking_failures":[
    {"code":"actual_prime_obstruction","size_key":"1080x1080","diagnosis":"1080x1080：冻结标题与顶部 Prime 禁区冲突；期望移动到 safe_content_frame 内 y>99"},
    {"code":"official_prime_text_unreadable","size_key":"1080x1080","diagnosis":"1080x1080：selected template support has p10=1.0345 and texture=0.227531；期望调整为低纹理背景"},
    {"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：还款表与底部 Prime 禁区冲突；期望移动到 safe_content_frame 内 y<566"},
    {"code":"official_prime_text_unreadable","size_key":"1200x628","diagnosis":"1200x628：selected template support has p10=1.2408；期望调整为低纹理背景"}
  ]
}`),
	}, critical)
	if err != nil || input.Status != "failed" {
		t.Fatalf("merged critical report = %#v, %v", input, err)
	}
	findings, err := creativeVisualModelReworkFindings(input.Findings, []string{"1080x1080", "1200x628", "800x1000"})
	if err != nil || len(findings) != 2 {
		t.Fatalf("merged model rework findings = %#v, %v", findings, err)
	}
	sizes := map[string]bool{}
	for _, finding := range findings {
		sizes[finding.SizeKey] = true
	}
	if !sizes["1080x1080"] || !sizes["1200x628"] || sizes["800x1000"] {
		t.Fatalf("merged critical finding sizes = %#v", sizes)
	}
	for _, finding := range findings {
		if !strings.Contains(finding.Diagnosis, "实际不可读") || !strings.Contains(finding.Diagnosis, "safe_content_frame") {
			t.Fatalf("critical finding did not consolidate model findings: %#v", finding)
		}
	}
}

func TestCreativeQCFindingsNeedAutomaticRecoveryRecognizesContractFailureCode(t *testing.T) {
	if !creativeQCFindingsNeedAutomaticRecovery(json.RawMessage(`{"failure_code":"prompt_contract_parent_direction_sha256_missing","blocking_failures":[]}`)) {
		t.Fatal("prompt contract failure code should trigger automatic QC recovery")
	}
	if creativeQCFindingsNeedAutomaticRecovery(json.RawMessage(`{"failure_code":"visual_quality_failure","blocking_failures":[]}`)) {
		t.Fatal("ordinary visual quality failure should remain model-rework/manual work")
	}
}

func TestCreativeVisualModelReworkAttemptCountSupportsTwoAttempts(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	variantID := parseUUID(uuid.NewString())
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	attempts, err := creativeVisualModelReworkAttemptCount(t.Context(), tx, variantID)
	if err != nil || attempts != 0 {
		t.Fatalf("initial visual rework attempts = %d, %v", attempts, err)
	}
	agentID := createHandlerTestAgent(t, "creative-visual-rework-limit-"+uuid.NewString(), nil)
	for i := 0; i < creativeVisualModelReworkMaxAttempts; i++ {
		if _, err := tx.Exec(t.Context(), `
	INSERT INTO agent_task_queue (agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context)
	VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'queued', 'creative_order_item_production', $2, $3::jsonb)
	`, agentID, uuid.New(), fmt.Sprintf(`{"type":"creative_domain_task","workflow":"creative_production","variant_id":"%s","qc_visual_rework":{}}`, uuidToString(variantID))); err != nil {
			t.Fatal(err)
		}
	}
	attempts, err = creativeVisualModelReworkAttemptCount(t.Context(), tx, variantID)
	if err != nil || attempts != creativeVisualModelReworkMaxAttempts {
		t.Fatalf("queued visual rework attempts = %d, %v", attempts, err)
	}
	if _, err := tx.Exec(t.Context(), `
	INSERT INTO agent_task_queue (agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context, completed_at)
	VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', 'creative_order_item_production', $2, $3::jsonb, now())
	`, agentID, uuid.New(), fmt.Sprintf(`{"type":"creative_domain_task","workflow":"creative_production","variant_id":"%s","qc_visual_rework":{}}`, uuidToString(variantID))); err != nil {
		t.Fatal(err)
	}
	attempts, err = creativeVisualModelReworkAttemptCount(t.Context(), tx, variantID)
	if err != nil || attempts != creativeVisualModelReworkMaxAttempts {
		t.Fatalf("empty completed visual rework consumed an attempt = %d, %v", attempts, err)
	}
}

func TestQueueCreativeVisualModelReworkReusesOnlyPassingGeneratedSizes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "visual rework queue")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"pipeline_version": creativePipelineCandidateV1,
		"squad_snapshot": map[string]string{
			"squad_id":        fixture.SquadID,
			"leader_agent_id": fixture.LeaderAgentID, "producer_agent_id": fixture.ProducerAgentID,
			"reviewer_agent_id": fixture.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'running', $3::jsonb, $4) RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
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
	for _, size := range standardCreativeAssetSizes {
		var attachmentID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, 'image/png', 128) RETURNING id::text
`, testWorkspaceID, issueID, testUserID, "base-"+size+".png", "/uploads/base-"+size+".png").Scan(&attachmentID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 1, 'generated', $3, '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	var parentTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, 'running', 'creative_order_variant_qc', $3, $4::jsonb)
RETURNING id::text
`, fixture.ReviewerAgentID, issueID, variantID, creativeQCTaskContextForTest(t, orderID, variantID, "visual")).Scan(&parentTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, parentTaskID) })
	parentTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(parentTaskID))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	reworkTask, err := testHandler.queueCreativeVisualModelRework(
		t.Context(), tx, parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(itemID), parseUUID(candidateID), parseUUID(variantID), parseUUID(issueID),
		inputSnapshot, 1, standardCreativeAssetSizes, parentTask,
		[]creativeVisualModelReworkFinding{{
			Code: "actual_prime_obstruction", SizeKey: "1200x628",
			Diagnosis: "1200x628：还款表格 与 bottom official Prime template content 冲突；期望移动到 safe_content_frame 内 y<=430",
		}},
	)
	if err != nil || !reworkTask.ID.Valid {
		t.Fatalf("queue visual rework = %#v, %v", reworkTask, err)
	}
	var revision int
	if err := tx.QueryRow(t.Context(), `SELECT revision FROM creative_order_variant WHERE id = $1`, variantID).Scan(&revision); err != nil || revision != 2 {
		t.Fatalf("visual rework revision = %d, %v", revision, err)
	}
	var copiedSizes []string
	if err := tx.QueryRow(t.Context(), `
SELECT COALESCE(array_agg(size_key ORDER BY size_key), '{}'::text[])
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 2 AND stage = 'generated'
`, variantID).Scan(&copiedSizes); err != nil {
		t.Fatal(err)
	}
	if strings.Join(copiedSizes, ",") != "1080x1080,800x1000" {
		t.Fatalf("reused generated sizes = %#v", copiedSizes)
	}
	attempts, err := creativeVisualModelReworkAttemptCount(t.Context(), tx, parseUUID(variantID))
	if err != nil || attempts != 1 {
		t.Fatalf("queued visual rework attempts = %d, %v", attempts, err)
	}
}

func TestQueueCreativeDirectEditVisualReworkKeepsPrimeReferenceReadOnly(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "direct visual rework queue")
	fixture := createDirectEditSquadFixture(t)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]string{
			"squad_id":             fixture.SquadID,
			"leader_agent_id":      fixture.LeaderAgentID,
			"producer_agent_id":    fixture.DirectEditorAgentID,
			"reviewer_agent_id":    fixture.ReviewerAgentID,
			"direct_edit_agent_id": fixture.DirectEditorAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, created_by)
VALUES ($1, $2, 'running', $3::jsonb, $4) RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
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
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, brief, status)
VALUES ($1, 'V01', 1, $2::jsonb, 'running') RETURNING id::text
`, itemID, `{"creative_direct_edit_delivery":{"final_visual_validation":true,"raw_user_request":"标题和表格都要避开贴片"}}`).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeFeedbackAsset(t)
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 1, 'generated', $3, '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	primeAttachmentID := createCreativeFeedbackAsset(t)
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, '1200x628', 1, 'primed', $2, '{}'::jsonb, '{}'::jsonb, 'completed')
`, variantID, primeAttachmentID); err != nil {
		t.Fatal(err)
	}
	var parentTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, 'running', 'creative_order_variant_qc', $3, $4::jsonb)
RETURNING id::text
`, fixture.ReviewerAgentID, issueID, variantID, creativeQCTaskContextForTest(t, orderID, variantID, "visual")).Scan(&parentTaskID); err != nil {
		t.Fatal(err)
	}
	parentTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(parentTaskID))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	reworkTask, err := testHandler.queueCreativeVisualModelRework(
		t.Context(), tx, parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(itemID), parseUUID(candidateID), parseUUID(variantID), parseUUID(issueID),
		inputSnapshot, 1, standardCreativeAssetSizes, parentTask,
		[]creativeVisualModelReworkFinding{{
			Code: "actual_prime_obstruction", SizeKey: "1200x628",
			Diagnosis: "1200x628：标题和还款表格 与 bottom Prime content 冲突；期望移动到 safe_content_frame 内 y<=430",
		}},
	)
	if err != nil || !reworkTask.ID.Valid {
		t.Fatalf("queue direct visual rework = %#v, %v", reworkTask, err)
	}
	var contextValue struct {
		Workflow              string `json:"workflow"`
		TargetSize            string `json:"target_size"`
		ReferenceAttachmentID string `json:"reference_attachment_id"`
		FinalVisualValidation bool   `json:"final_visual_validation"`
		DirectEdit            struct {
			ValidationRework json.RawMessage `json:"validation_rework"`
		} `json:"direct_edit"`
	}
	if err := json.Unmarshal(reworkTask.Context, &contextValue); err != nil {
		t.Fatal(err)
	}
	if contextValue.Workflow != "creative_direct_edit" || contextValue.TargetSize != "1200x628" ||
		contextValue.ReferenceAttachmentID != primeAttachmentID || !contextValue.FinalVisualValidation || len(contextValue.DirectEdit.ValidationRework) == 0 {
		t.Fatalf("direct visual rework context = %#v", contextValue)
	}
	var copiedSizes []string
	if err := tx.QueryRow(t.Context(), `
SELECT COALESCE(array_agg(size_key ORDER BY size_key), '{}'::text[])
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 2 AND stage = 'generated'
`, variantID).Scan(&copiedSizes); err != nil {
		t.Fatal(err)
	}
	if strings.Join(copiedSizes, ",") != "1080x1080,800x1000" {
		t.Fatalf("direct rework reused generated sizes = %#v", copiedSizes)
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
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, 'running', $2::text[])
`, variantID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 1, 'primed', $3, '{}'::jsonb, '{}'::jsonb, 'completed')`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
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
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, 'running', $2::text[])
`, failedVariantID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-legacy-technical-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 1, 'primed', $3, '{}'::jsonb, '{}'::jsonb, 'completed')
`, failedVariantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
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
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&failedResponse) != nil || failedResponse.Outcome != "delivered" || failedResponse.TechnicalStatus != "" || failedResponse.VisualStatus != "passed" || failedResponse.DeliveredAssetCount != 3 || failedResponse.OrderAggregateStatus != "awaiting_adoption" {
		t.Fatalf("legacy technical QC finding must be ignored = %d %#v %s", w.Code, failedResponse, w.Body.String())
	}
	var failedVariantStatus, completedVariantStatus string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, failedVariantID).Scan(&failedVariantStatus); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_order_variant WHERE id = $1`, variantID).Scan(&completedVariantStatus); err != nil {
		t.Fatal(err)
	}
	if failedVariantStatus != "completed" {
		t.Fatalf("QC finding variant status = %q", failedVariantStatus)
	}
	if completedVariantStatus != "completed" {
		t.Fatalf("completed sibling variant status = %q", completedVariantStatus)
	}

	var exhaustedVariantID, exhaustedTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'v03', 3, 'running') RETURNING id::text`, itemID).Scan(&exhaustedVariantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 3, '{}'::jsonb, 'running', $2::text[])
`, exhaustedVariantID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-exhausted-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1, $2, 3, 'primed', $3, '{}'::jsonb, '{}'::jsonb, 'completed')`, exhaustedVariantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 3, 'passed', '{}'::jsonb),
       ($1, 'visual', 3, 'failed', '{"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：正文 与底部 Prime 法务文字实际遮挡；期望移动到 safe_content_frame 内 y<=430"}]}'::jsonb)`, exhaustedVariantID); err != nil {
		t.Fatal(err)
	}
	reworkTaskIDs := make([]string, 0, creativeVisualModelReworkMaxAttempts)
	for i := 0; i < creativeVisualModelReworkMaxAttempts; i++ {
		var reworkTaskID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', 'creative_order_item_production', $2, $3::jsonb)

RETURNING id::text`, agentID, itemID, fmt.Sprintf(`{"type":"creative_domain_task","workflow":"creative_production","variant_id":"%s","revision":%d,"qc_visual_rework":{}}`, exhaustedVariantID, i+2)).Scan(&reworkTaskID); err != nil {
			t.Fatal(err)
		}
		reworkTaskIDs = append(reworkTaskIDs, reworkTaskID)
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation (variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id)
VALUES ($1, $2, 3, 'visual_rework', $3, 'completed', $4)
`, exhaustedVariantID, standardCreativeAssetSizes[i], "exhausted-rework:"+reworkTaskID, reworkTaskID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, taskID := range reworkTaskIDs {
			_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID)
		}
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, exhaustedVariantID, creativeQCTaskContextForTest(t, orderID, exhaustedVariantID, "visual", 3)).Scan(&exhaustedTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, exhaustedTaskID)
	})
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{VariantID: exhaustedVariantID, Revision: 3})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", exhaustedTaskID)
	testHandler.FinalizeCreativeOrderQC(w, req)
	var exhaustedResponse creativeOrderQCFinalizeResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&exhaustedResponse) != nil || exhaustedResponse.Outcome != "action_required" || exhaustedResponse.DeliveredAssetCount != 0 {
		t.Fatalf("exhausted visual rework must remain staged = %d %#v %s", w.Code, exhaustedResponse, w.Body.String())
	}
	var exhaustedVariantStatus string
	var exhaustedActiveRevision, exhaustedStagingRevision int
	var exhaustedDelivered int
	if err := testPool.QueryRow(t.Context(), `
SELECT status, COALESCE(active_revision, 0), COALESCE(staging_revision, 0)
FROM creative_order_variant WHERE id = $1
`, exhaustedVariantID).Scan(&exhaustedVariantStatus, &exhaustedActiveRevision, &exhaustedStagingRevision); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset WHERE variant_id = $1 AND revision = 3 AND stage = 'delivered'`, exhaustedVariantID).Scan(&exhaustedDelivered); err != nil {
		t.Fatal(err)
	}
	if exhaustedVariantStatus != "action_required" || exhaustedActiveRevision != 0 || exhaustedStagingRevision != 3 || exhaustedDelivered != 0 {
		t.Fatalf("exhausted visual rework status=%q active=%d staging=%d delivered=%d", exhaustedVariantStatus, exhaustedActiveRevision, exhaustedStagingRevision, exhaustedDelivered)
	}

	// A later blocking inspection must be able to withdraw a revision that an
	// earlier visual report activated. The historical activation remains for
	// audit, but it must no longer be the active delivery revision.
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'visual', 1, 2, 'failed',
  '{"blocking_failures":[{"code":"prime_reinspection_conflict","size_key":"1080x1080","diagnosis":"same attachment received contradictory Prime inspection results"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}
	var lateFailureTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, priority, trigger_evidence_kind, trigger_evidence_ref_id, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 0, 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text`, agentID, variantID, creativeQCTaskContextForTest(t, orderID, variantID, "visual", 1, 2)).Scan(&lateFailureTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, lateFailureTaskID)
	})
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{VariantID: variantID, Revision: 1, Attempt: 2})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", lateFailureTaskID)
	testHandler.FinalizeCreativeOrderQC(w, req)
	var lateFailureResponse creativeOrderQCFinalizeResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&lateFailureResponse) != nil || lateFailureResponse.Outcome != "action_required" {
		t.Fatalf("late blocking inspection must withdraw delivery = %d %#v %s", w.Code, lateFailureResponse, w.Body.String())
	}
	var lateStatus, lateRevisionStatus string
	var lateActiveRevision, lateStagingRevision int
	var wasActivated bool
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, COALESCE(variant.active_revision, 0), COALESCE(variant.staging_revision, 0),
       revision.status, revision.activated_at IS NOT NULL
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variantID).Scan(&lateStatus, &lateActiveRevision, &lateStagingRevision, &lateRevisionStatus, &wasActivated); err != nil {
		t.Fatal(err)
	}
	if lateStatus != "action_required" || lateActiveRevision != 0 || lateStagingRevision != 1 || lateRevisionStatus != "action_required" || !wasActivated {
		t.Fatalf("late failure state=%q active=%d staging=%d revision=%q wasActivated=%t", lateStatus, lateActiveRevision, lateStagingRevision, lateRevisionStatus, wasActivated)
	}
}

func TestCreativeOrderVariantReworkLimitDependsOnMode(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "rework fence")
	create := func(trigger string) (string, string, string) {
		var orderID, itemID string
		inputSnapshot := `{"pipeline_version":"candidate_v1","expected_sizes":["1080x1080","1200x628","800x1000"]}`
		variantKey := "C01"
		if trigger == "creative_direct_edit" {
			inputSnapshot = `{"pipeline_version":"direct_edit_v1","target_size":"1080x1080","delivery_mode":"publish","user_request":"调整图片"}`
			variantKey = "direct_edit"
		}
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, 'running', $2::jsonb, $3, $4) RETURNING id::text`, testWorkspaceID, inputSnapshot, trigger, testUserID).Scan(&orderID); err != nil {
			t.Fatal(err)
		}
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb) RETURNING id::text`, orderID, candidateID).Scan(&itemID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
		return orderID, itemID, variantKey
	}
	upsert := func(orderID, itemID, variantKey string, revision int, status string) int {
		w := httptest.NewRecorder()
		input := creativeOrderVariantInput{OrderItemID: itemID, VariantKey: variantKey, Brief: json.RawMessage(`{}`), Revision: revision, Status: status}
		if revision == 1 && variantKey == "C01" {
			input.CandidateState = "candidate"
			input.PrimarySize = "1080x1080"
		}
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", input)
		req = withURLParam(req, "id", orderID)
		testHandler.UpsertCreativeOrderVariant(w, req)
		return w.Code
	}

	standardOrderID, standardItemID, standardVariantKey := create("manual")
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 1, "running"); code != http.StatusOK {
		t.Fatalf("standard r1 = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 2, "running"); code != http.StatusOK {
		t.Fatalf("standard r2 = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 3, "running"); code != http.StatusConflict {
		t.Fatalf("standard r3 = %d, want 409", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 2, "action_required"); code != http.StatusOK {
		t.Fatalf("standard r2 action_required = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 3, "running"); code != http.StatusOK {
		t.Fatalf("standard failed r3 = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 3, "action_required"); code != http.StatusOK {
		t.Fatalf("standard failed r3 same-revision status update = %d", code)
	}
	if code := upsert(standardOrderID, standardItemID, standardVariantKey, 4, "running"); code != http.StatusConflict {
		t.Fatalf("standard r4 = %d, want 409", code)
	}

	directOrderID, directItemID, directVariantKey := create("creative_direct_edit")
	if code := upsert(directOrderID, directItemID, directVariantKey, 1, "running"); code != http.StatusOK {
		t.Fatalf("direct r1 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, directVariantKey, 2, "running"); code != http.StatusOK {
		t.Fatalf("direct r2 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, directVariantKey, 3, "running"); code != http.StatusOK {
		t.Fatalf("direct r3 = %d", code)
	}
	if code := upsert(directOrderID, directItemID, directVariantKey, 4, "running"); code != http.StatusConflict {
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

func TestCreativeQCPrimeReinspectionFenceRequiresChangedAttachments(t *testing.T) {
	previous := json.RawMessage(`{
  "checked_assets":[
    {"size_key":"1080x1080","attachment_id":"square-v3"},
    {"size_key":"1200x628","attachment_id":"landscape-v3"}
  ],
  "blocking_failures":[
    {"code":"actual_prime_obstruction","size_key":"1080x1080","diagnosis":"1080x1080: content overlaps Prime"}
  ]
}`)
	unchangedRetry := json.RawMessage(`{
  "checked_assets":[
    {"size_key":"1080x1080","attachment_id":"square-v3"},
    {"size_key":"1200x628","attachment_id":"landscape-v3"}
  ],
  "blocking_failures":[]
}`)
	conflicts := creativeQCHardPrimeReinspectionConflicts(previous, unchangedRetry)
	if len(conflicts) != 1 || conflicts[0].SizeKey != "1080x1080" || conflicts[0].PreviousCode != "actual_prime_obstruction" {
		t.Fatalf("unchanged Prime retry conflicts = %#v", conflicts)
	}
	fenced, err := mergeCreativeQCPrimeReinspectionConflicts(unchangedRetry, conflicts)
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := creativeQCFindingsHaveBlockingFailures(fenced)
	if err != nil || !blocked {
		t.Fatalf("fenced findings blocked=%t err=%v findings=%s", blocked, err, fenced)
	}

	changedRetry := json.RawMessage(`{
  "checked_assets":[
    {"size_key":"1080x1080","attachment_id":"square-v4"},
    {"size_key":"1200x628","attachment_id":"landscape-v3"}
  ],
  "blocking_failures":[]
}`)
	if conflicts := creativeQCHardPrimeReinspectionConflicts(previous, changedRetry); len(conflicts) != 0 {
		t.Fatalf("changed Prime assets must be eligible for a fresh QC conclusion: %#v", conflicts)
	}
}

func TestDecodeCreativeOrderQCInputAcceptsDirectReportShape(t *testing.T) {
	input, err := decodeCreativeOrderQCInput(json.RawMessage(`{
		"variant_id":"2f8f9b6b-2a4c-4f4e-bf0d-0b4b1b2aa111",
		"lane":"visual",
		"revision":1,
		"status":"failed",
		"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"1200x628","diagnosis":"1200x628：正文 与 Prime 冲突；期望移动到 safe_content_frame 内 y>=107。"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(input.Findings, []byte(`"blocking_failures"`)) {
		t.Fatalf("direct QC report findings = %s", input.Findings)
	}
	normalized, err := normalizeCreativeOrderQC(input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Status != "failed" {
		t.Fatalf("blocking direct QC report status = %q", normalized.Status)
	}
}

func TestDecodeCreativeOrderQCInputAcceptsWrappedReportAndQCStatus(t *testing.T) {
	input, err := decodeCreativeOrderQCInput(json.RawMessage(`{
		"variant_id":"2f8f9b6b-2a4c-4f4e-bf0d-0b4b1b2aa111",
		"lane":"visual",
		"revision":1,
		"report":{
			"qc_status":"failed",
			"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"800x1000","diagnosis":"800x1000：正文与 Prime 冲突。"}]
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if input.Status != "failed" || input.Lane != "visual" || input.VariantID == "" {
		t.Fatalf("wrapped QC report = %#v", input)
	}
	if !bytes.Contains(input.Findings, []byte(`"blocking_failures"`)) {
		t.Fatalf("wrapped QC findings = %s", input.Findings)
	}
	if _, err := normalizeCreativeOrderQC(input); err != nil {
		t.Fatal(err)
	}
}

func TestExpectedCreativeVariantSizesDependOnOrderMode(t *testing.T) {
	standard, err := expectedCreativeVariantSizes("manual", json.RawMessage(`{"expected_sizes":["1080x1080"]}`), json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(standard) != 1 || standard[0] != "1080x1080" {
		t.Fatalf("standard frozen sizes = %#v, want the snapshot scope", standard)
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

func TestExpectedCreativeVariantProductionSizesRequiresSquareCandidatePreview(t *testing.T) {
	sizes, err := expectedCreativeVariantProductionSizes("manual", json.RawMessage(`{}`), json.RawMessage(`{}`), "candidate", creativeCandidatePreviewSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 1 || sizes[0] != creativeCandidatePreviewSize {
		t.Fatalf("candidate preview sizes = %#v", sizes)
	}

	if _, err := expectedCreativeVariantProductionSizes("manual", json.RawMessage(`{}`), json.RawMessage(`{}`), "candidate", "1200x628"); err == nil || !strings.Contains(err.Error(), "must be 1080x1080") {
		t.Fatalf("non-square candidate error = %v", err)
	}

	reserve, err := expectedCreativeVariantProductionSizes("manual", json.RawMessage(`{}`), json.RawMessage(`{}`), "reserve", "1200x628")
	if err != nil || len(reserve) != 1 || reserve[0] != "1200x628" {
		t.Fatalf("legacy reserve sizes = %#v, err = %v", reserve, err)
	}
}

func TestCreativeDirectEditPublishRequiresFinalVisualValidation(t *testing.T) {
	context := creativeDirectEditTaskCompletionContext{
		DeliveryMode:          "publish",
		ExpectedSizes:         standardCreativeAssetSizes,
		FinalVisualValidation: true,
	}
	state := creativeDirectEditArtifactState{
		VariantExists: true, TargetGenerated: true,
		GeneratedCount: len(standardCreativeAssetSizes), PrimedCount: len(standardCreativeAssetSizes),
	}
	if got := creativeDirectEditArtifactError(context, state); got != "" {
		t.Fatalf("final visual validation should allow task completion after Prime handoff, got %q", got)
	}
	context.FinalVisualValidation = false
	if got := creativeDirectEditArtifactError(context, state); !strings.Contains(got, "requires final visual validation") {
		t.Fatalf("publish without final visual validation = %q", got)
	}
}

type creativeOrderSquadFixture struct {
	SquadID          string
	LeaderAgentID    string
	PlannerAgentID   string
	ProducerAgentID  string
	ReviewerAgentID  string
	DuplicateAgentID string
}

func createCreativeOrderSquadFixture(t *testing.T, missingCapability, duplicateCapability string, leaderCapable bool) creativeOrderSquadFixture {
	t.Helper()
	fixture := creativeOrderSquadFixture{
		LeaderAgentID:   createHandlerTestAgent(t, "creative-order-leader-"+uuid.NewString(), nil),
		PlannerAgentID:  createHandlerTestAgent(t, "creative-order-planner-"+uuid.NewString(), nil),
		ProducerAgentID: createHandlerTestAgent(t, "creative-order-producer-"+uuid.NewString(), nil),
		ReviewerAgentID: createHandlerTestAgent(t, "creative-order-reviewer-"+uuid.NewString(), nil),
	}
	agentsByCapability := map[string]string{
		"direct_image_edit": fixture.ProducerAgentID,
		"generation_plan":   fixture.PlannerAgentID,
		"image_edit":        fixture.ProducerAgentID,
		"quality_control":   fixture.ReviewerAgentID,
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
	for _, agentID := range []string{fixture.LeaderAgentID, fixture.PlannerAgentID, fixture.ProducerAgentID, fixture.ReviewerAgentID} {
		if _, err := testPool.Exec(t.Context(), `INSERT INTO squad_member (squad_id, member_type, member_id) VALUES ($1, 'agent', $2)`, fixture.SquadID, agentID); err != nil {
			t.Fatal(err)
		}
	}
	if duplicateCapability != "" {
		duplicateAgentID := createHandlerTestAgent(t, "creative-order-duplicate-"+uuid.NewString(), nil)
		fixture.DuplicateAgentID = duplicateAgentID
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
		"pipeline_version": creativePipelineCandidateV1,
		"squad_snapshot": map[string]any{
			"squad_id": fixture.SquadID, "squad_name": "client-visible name", "members": []any{map[string]any{"agent_id": "client-member"}},
			"leader_agent_id": "client-leader", "planner_agent_id": "client-planner", "producer_agent_id": "client-producer",
			"reviewer_agent_id": "client-reviewer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
		Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
		Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":3,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`), Direction: "freeze agents"}},
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
		PipelineVersion string         `json:"pipeline_version"`
		Squad           map[string]any `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(order.InputSnapshot, &frozen); err != nil {
		t.Fatal(err)
	}
	if frozen.PipelineVersion != creativePipelineCandidateV1 {
		t.Fatalf("pipeline version = %q, want %q", frozen.PipelineVersion, creativePipelineCandidateV1)
	}
	want := map[string]string{
		"squad_id": fixture.SquadID, "leader_agent_id": fixture.LeaderAgentID, "planner_agent_id": fixture.PlannerAgentID,
		"producer_agent_id": fixture.ProducerAgentID, "reviewer_agent_id": fixture.ReviewerAgentID,
	}
	for field, value := range want {
		if frozen.Squad[field] != value {
			t.Errorf("squad snapshot %s = %#v, want %q", field, frozen.Squad[field], value)
		}
	}
	producerIDs, ok := frozen.Squad["producer_agent_ids"].([]any)
	if !ok || len(producerIDs) != 1 || producerIDs[0] != fixture.ProducerAgentID {
		t.Fatalf("producer_agent_ids = %#v, want [%q]", frozen.Squad["producer_agent_ids"], fixture.ProducerAgentID)
	}
	if frozen.Squad["squad_name"] != "client-visible name" {
		t.Fatalf("unrelated squad snapshot fields were not preserved: %#v", frozen.Squad)
	}
}

func TestCreateCreativeOrderAllowsMultipleImageEditAgents(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "image edit pool")
	fixture := createCreativeOrderSquadFixture(t, "", "image_edit", true)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
		Status: "queued", InputSnapshot: json.RawMessage(`{"squad_snapshot":{"squad_id":"` + fixture.SquadID + `"}}`),
		Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":3,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`)}},
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
		Squad struct {
			ProducerAgentID  string   `json:"producer_agent_id"`
			ProducerAgentIDs []string `json:"producer_agent_ids"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(order.InputSnapshot, &frozen); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{}{fixture.ProducerAgentID: {}, fixture.DuplicateAgentID: {}}
	for _, id := range frozen.Squad.ProducerAgentIDs {
		delete(want, id)
	}
	if frozen.Squad.ProducerAgentID == "" || len(frozen.Squad.ProducerAgentIDs) != 2 || len(want) != 0 {
		t.Fatalf("producer pool snapshot = primary:%q pool:%#v missing:%#v", frozen.Squad.ProducerAgentID, frozen.Squad.ProducerAgentIDs, want)
	}
}

func TestCreateCreativeOrderLinksCandidateToIssue(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	_, candidateID := createCreativeFeedbackCandidate(t, "order candidate link")
	targetIssueID := createCreativeDeliveryTestIssue(t, "order candidate target", "")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot := json.RawMessage(`{"squad_snapshot":{"squad_id":"` + fixture.SquadID + `"}}`)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
		IssueID: targetIssueID, Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
		Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":3,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`)}},
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
	var status string
	if err := testPool.QueryRow(t.Context(), `
SELECT status FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2
`, targetIssueID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "selected" {
		t.Fatalf("candidate status on order issue = %q, want selected", status)
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

func TestCreateCreativeOrderAllowsManualFinancialCopyAndPersistsIt(t *testing.T) {
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
	requestOrder := func(snapshot map[string]any, direction string) *httptest.ResponseRecorder {
		t.Helper()
		copySnapshot, marshalErr := json.Marshal(snapshot)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders", creativeOrderInput{
			Status: "queued", InputSnapshot: inputSnapshot, TriggerEvidenceKind: "manual",
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: copySnapshot, Direction: direction}},
		})
		testHandler.CreateCreativeOrder(w, req)
		return w
	}

	approved := requestOrder(map[string]any{
		"schema_version": 3, "status": "user_custom", "creative_type": "num",
		"headline": "Pinjaman fleksibel", "benefit": "Limit hingga Rp80.000.000", "cta": "Ajukan sekarang",
	}, "Benefit: Limit hingga Rp80.000.000")
	if approved.Code != http.StatusCreated {
		t.Fatalf("approved custom fact = %d %s", approved.Code, approved.Body.String())
	}
	var created creativeOrderResponse
	if err := json.NewDecoder(approved.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, created.ID) })

	manualValue := "Limit hingga Rp99.000.000 dengan bunga 1%"
	manualDirection := "中心金额区 (benefit): " + manualValue
	manual := requestOrder(map[string]any{
		"schema_version": 3, "status": "model_pre_adapted", "creative_type": "num",
		"library_id": libraryID, "library_version": 1,
		"headline": "Pinjaman fleksibel", "benefit": manualValue, "cta": "Ajukan sekarang",
		"pre_adaptation": map[string]any{
			"schema_version": 1,
			"text_replacements": []map[string]any{{
				"block_id": "center-benefit", "replacement_text": manualValue, "source_kind": "manual", "status": "ready",
			}},
		},
	}, manualDirection)
	if manual.Code != http.StatusCreated {
		t.Fatalf("manual financial copy = %d %s", manual.Code, manual.Body.String())
	}
	var manualOrder creativeOrderResponse
	if err := json.NewDecoder(manual.Body).Decode(&manualOrder); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, manualOrder.ID) })
	var persistedSnapshot json.RawMessage
	var persistedDirection string
	if err := testPool.QueryRow(t.Context(), `SELECT copy_snapshot::text, direction FROM creative_order_item WHERE order_id = $1`, manualOrder.ID).Scan(&persistedSnapshot, &persistedDirection); err != nil {
		t.Fatal(err)
	}
	var persisted struct {
		Benefit       string `json:"benefit"`
		PreAdaptation struct {
			TextReplacements []struct {
				ReplacementText string `json:"replacement_text"`
				SourceKind      string `json:"source_kind"`
			} `json:"text_replacements"`
		} `json:"pre_adaptation"`
	}
	if err := json.Unmarshal(persistedSnapshot, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Benefit != manualValue || len(persisted.PreAdaptation.TextReplacements) != 1 || persisted.PreAdaptation.TextReplacements[0].ReplacementText != manualValue || persisted.PreAdaptation.TextReplacements[0].SourceKind != "manual" || persistedDirection != manualDirection {
		t.Fatalf("manual copy was not frozen for production: snapshot=%s direction=%q", persistedSnapshot, persistedDirection)
	}
}

func TestSelectCreativeImageEditAgentUsesCurrentSquadPool(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createCreativeOrderSquadFixture(t, "", "image_edit", true)
	inputSnapshot := json.RawMessage(`{"squad_snapshot":{"squad_id":"` + fixture.SquadID + `"}}`)
	preferred := parseUUID(fixture.ProducerAgentID)
	selected, err := testHandler.selectCreativeImageEditAgent(t.Context(), testPool, testHandler.Queries, parseUUID(testWorkspaceID), inputSnapshot, uuid.NewString(), preferred, false)
	if err != nil {
		t.Fatalf("select soft preferred producer: %v", err)
	}
	if uuidToString(selected.ID) != fixture.ProducerAgentID {
		t.Fatalf("selected producer = %s, want soft preferred %s", uuidToString(selected.ID), fixture.ProducerAgentID)
	}
	var busyTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id,
  originator_user_id, accountable_user_id, requesting_user_id, originator_source
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', '{}'::jsonb,
        'creative_order_item_production', $2, $3, $3, $3, 'direct_human')
RETURNING id::text
`, fixture.ProducerAgentID, uuid.NewString(), testUserID).Scan(&busyTaskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE id = $1`, busyTaskID)
	})
	selected, err = testHandler.selectCreativeImageEditAgent(t.Context(), testPool, testHandler.Queries, parseUUID(testWorkspaceID), inputSnapshot, uuid.NewString(), preferred, false)
	if err != nil {
		t.Fatalf("select less busy producer: %v", err)
	}
	if uuidToString(selected.ID) != fixture.DuplicateAgentID {
		t.Fatalf("selected producer with soft preferred = %s, want less busy %s", uuidToString(selected.ID), fixture.DuplicateAgentID)
	}
	selected, err = testHandler.selectCreativeImageEditAgent(t.Context(), testPool, testHandler.Queries, parseUUID(testWorkspaceID), inputSnapshot, uuid.NewString(), preferred, true)
	if err != nil {
		t.Fatalf("select pinned producer: %v", err)
	}
	if uuidToString(selected.ID) != fixture.ProducerAgentID {
		t.Fatalf("selected pinned producer = %s, want preferred %s", uuidToString(selected.ID), fixture.ProducerAgentID)
	}
	if _, err := testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, busyTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
	DELETE FROM squad_member WHERE squad_id = $1 AND member_id = $2
	`, fixture.SquadID, fixture.ProducerAgentID); err != nil {
		t.Fatal(err)
	}
	selected, err = testHandler.selectCreativeImageEditAgent(t.Context(), testPool, testHandler.Queries, parseUUID(testWorkspaceID), inputSnapshot, uuid.NewString(), preferred, false)
	if err != nil {
		t.Fatalf("select after pool removal: %v", err)
	}
	if uuidToString(selected.ID) != fixture.DuplicateAgentID {
		t.Fatalf("selected producer after removal = %s, want remaining %s", uuidToString(selected.ID), fixture.DuplicateAgentID)
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
		{name: "duplicate non-pool role", duplicateCapability: "quality_control", leaderCapable: true, wantMessage: "multiple agents with quality_control"},
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
			Items: []creativeOrderItemInput{{CandidateID: candidateID, CopySnapshot: json.RawMessage(`{"schema_version":3,"status":"user_custom","creative_type":"num","headline":"Pinjaman fleksibel"}`), Direction: "recover submission"}},
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

func TestPromoteCreativeOrderDiagnosticAssetActivatesComposedDirectEditResult(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "composed direct-edit promotion")
	const targetSize = "1080x1080"
	inputSnapshot := `{"expected_sizes":["1080x1080","1200x628","800x1000"]}`
	brief := `{"creative_direct_edit_delivery":{"scope":"size","target_size":"1080x1080","final_visual_validation":true}}`
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, $2, 'partial', $3::jsonb, 'creative_direct_edit', $4)
RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'completed')
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, brief, revision, active_revision, staging_revision, status, candidate_state
)
VALUES ($1, 'C01', $2::jsonb, 2, 1, 2, 'action_required', 'selected')
RETURNING id::text
`, itemID, brief).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for revision, status := range map[int]string{1: "completed", 2: "action_required"} {
		activatedAt := "NULL"
		if revision == 1 {
			activatedAt = "now()"
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes, activated_at)
VALUES ($1, $2, $3::jsonb, $4, $5::text[], `+activatedAt+`)
`, variantID, revision, brief, status, standardCreativeAssetSizes); err != nil {
			t.Fatal(err)
		}
	}

	primedAttachments := make(map[string]string, len(standardCreativeAssetSizes))
	for _, size := range standardCreativeAssetSizes {
		generatedAttachmentID := createCreativeFeedbackAsset(t)
		var generatedAssetID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
RETURNING id::text
`, variantID, size, generatedAttachmentID).Scan(&generatedAssetID); err != nil {
			t.Fatal(err)
		}
		primedAttachmentID := createCreativeFeedbackAsset(t)
		var primedAssetID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, derived_from_asset_id, status)
VALUES ($1, $2, 1, 'primed', $3, $4, 'completed')
RETURNING id::text
`, variantID, size, primedAttachmentID, generatedAssetID).Scan(&primedAssetID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, derived_from_asset_id, status)
VALUES ($1, $2, 1, 'delivered', $3, $4, 'completed')
`, variantID, size, primedAttachmentID, primedAssetID); err != nil {
			t.Fatal(err)
		}

		replacementBaseID := createCreativeFeedbackAsset(t)
		var replacementBaseAssetID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 2, 'generated', $3, 'completed')
RETURNING id::text
`, variantID, size, replacementBaseID).Scan(&replacementBaseAssetID); err != nil {
			t.Fatal(err)
		}
		if size == targetSize {
			continue
		}
		primedAttachments[size] = createCreativeFeedbackAsset(t)
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, derived_from_asset_id, status)
VALUES ($1, $2, 2, 'primed', $3, $4, 'completed')
`, variantID, size, primedAttachments[size], replacementBaseAssetID); err != nil {
			t.Fatal(err)
		}
	}

	composedAttachmentID := createCreativeFeedbackAsset(t)
	var composedDiagnosticID, rawDiagnosticID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1, $2, $3, 2, 'brand_components', 'Prime 合成成图', 'composed-adjustment.png', '{}'::jsonb)
RETURNING id::text
`, variantID, composedAttachmentID, targetSize).Scan(&composedDiagnosticID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1, $2, $3, 2, 'creative_direct_edit', '直接改图结果', 'uncomposed-adjustment.png', '{}'::jsonb)
RETURNING id::text
`, variantID, createCreativeFeedbackAsset(t), targetSize).Scan(&rawDiagnosticID); err != nil {
		t.Fatal(err)
	}

	reject := httptest.NewRecorder()
	rejectRequest := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variantID+"/process-images/"+rawDiagnosticID+"/adopt", nil),
		"id", orderID, "variantId", variantID, "assetId", rawDiagnosticID)
	testHandler.PromoteCreativeOrderDiagnosticAsset(reject, rejectRequest)
	if reject.Code != http.StatusConflict {
		t.Fatalf("uncomposed process image adoption = %d %s", reject.Code, reject.Body.String())
	}

	w := httptest.NewRecorder()
	req := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variantID+"/process-images/"+composedDiagnosticID+"/adopt", nil),
		"id", orderID, "variantId", variantID, "assetId", composedDiagnosticID)
	testHandler.PromoteCreativeOrderDiagnosticAsset(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PromoteCreativeOrderDiagnosticAsset: %d %s", w.Code, w.Body.String())
	}

	var status string
	var activeRevision, stagingRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT status, active_revision, COALESCE(staging_revision, 0)
FROM creative_order_variant
WHERE id = $1
`, variantID).Scan(&status, &activeRevision, &stagingRevision); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || activeRevision != 2 || stagingRevision != 0 {
		t.Fatalf("composed adjustment lifecycle = status %q active r%d staging r%d", status, activeRevision, stagingRevision)
	}

	delivered := map[string]string{}
	rows, err := testPool.Query(t.Context(), `
SELECT size_key, attachment_id::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 2 AND stage = 'delivered' AND status = 'completed'
`, variantID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var size, attachmentID string
		if err := rows.Scan(&size, &attachmentID); err != nil {
			t.Fatal(err)
		}
		delivered[size] = attachmentID
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(delivered) != len(standardCreativeAssetSizes) || delivered[targetSize] != composedAttachmentID {
		t.Fatalf("composed adjustment delivered package = %#v", delivered)
	}
	for _, size := range []string{"1200x628", "800x1000"} {
		if delivered[size] != primedAttachments[size] {
			t.Fatalf("composed adjustment retained %s = %q, want %q", size, delivered[size], primedAttachments[size])
		}
	}
}
