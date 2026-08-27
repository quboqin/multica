package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestDeleteAttachmentProtectsActiveCreativeRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "active creative attachment deletion guard")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET input_snapshot = jsonb_set(input_snapshot, '{pipeline_version}', '"candidate_v1"'::jsonb)
WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	var variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, revision, status, candidate_state, selection_rank, primary_size
)
VALUES ($1, 'C01', 1, 'running', 'selected', 1, '1080x1080')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, 'running', $2::text[])
`, variantID, standardCreativeAssetSizes); err != nil {
		t.Fatal(err)
	}

	var deliveredAttachmentID string
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "delete-guard-delivered-"+strings.ReplaceAll(size, "x", "-")+".png")
		if deliveredAttachmentID == "" {
			deliveredAttachmentID = attachmentID
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	primedAttachmentID := createCreativeOrderAssetAttachment(t, "delete-guard-primed.png")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 1, 'primed', $2, 'completed')
`, variantID, primedAttachmentID); err != nil {
		t.Fatal(err)
	}
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

	deleteAttachment := func(attachmentID string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		request := withURLParam(newRequest(http.MethodDelete, "/api/attachments/"+attachmentID, nil), "id", attachmentID)
		testHandler.DeleteAttachment(response, request)
		return response
	}
	for _, attachmentID := range []string{deliveredAttachmentID, primedAttachmentID} {
		response := deleteAttachment(attachmentID)
		if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "active creative delivery") {
			t.Fatalf("active creative attachment deletion = %d %s", response.Code, response.Body.String())
		}
		var attachmentCount int
		if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM attachment WHERE id = $1`, attachmentID).Scan(&attachmentCount); err != nil {
			t.Fatal(err)
		}
		if attachmentCount != 1 {
			t.Fatalf("protected attachment %s count = %d, want 1", attachmentID, attachmentCount)
		}
	}

	inactiveAttachmentID := createCreativeOrderAssetAttachment(t, "delete-guard-inactive-r2.png")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 2, 'primed', $2, 'completed')
`, variantID, inactiveAttachmentID); err != nil {
		t.Fatal(err)
	}
	response := deleteAttachment(inactiveAttachmentID)
	if response.Code != http.StatusNoContent {
		t.Fatalf("inactive creative attachment deletion = %d %s", response.Code, response.Body.String())
	}
	var attachmentCount int
	var referenceCleared bool
	if err := testPool.QueryRow(t.Context(), `
SELECT (SELECT count(*) FROM attachment WHERE id = $1), attachment_id IS NULL
FROM creative_order_asset
WHERE variant_id = $2 AND revision = 2 AND size_key = '1080x1080' AND stage = 'primed'
`, inactiveAttachmentID, variantID).Scan(&attachmentCount, &referenceCleared); err != nil {
		t.Fatal(err)
	}
	if attachmentCount != 0 || !referenceCleared {
		t.Fatalf("inactive attachment deletion left count=%d reference_cleared=%v", attachmentCount, referenceCleared)
	}
}

func TestCreativeDeliveryStatusRequiresCompleteActiveRevisionAttachments(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "active creative delivery completeness")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET input_snapshot = jsonb_set(input_snapshot, '{pipeline_version}', '"candidate_v1"'::jsonb)
WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}

	var firstVariantID string
	for rank := 1; rank <= 3; rank++ {
		var variantID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, revision, status, candidate_state, selection_rank, primary_size
)
VALUES ($1, $2, 1, 'running', 'selected', $3, '1080x1080')
RETURNING id::text
`, itemID, "C0"+string(rune('0'+rank)), rank).Scan(&variantID); err != nil {
			t.Fatal(err)
		}
		if firstVariantID == "" {
			firstVariantID = variantID
		}
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, 1, '{}'::jsonb, 'running', $2::text[])
`, variantID, standardCreativeAssetSizes); err != nil {
			t.Fatal(err)
		}
		for _, size := range standardCreativeAssetSizes {
			attachmentID := createCreativeOrderAssetAttachment(t, "delivery-completeness-"+variantID+"-"+strings.ReplaceAll(size, "x", "-")+".png")
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
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

	state := creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.DeliveryStatus != "awaiting_adoption" || state.DerivedStatus != "awaiting_adoption" {
		t.Fatalf("complete active delivery state = %#v", state)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_item
SET adopted_variant_id = $2, adopted_at = now(), adopted_by = $3
WHERE id = $1
`, itemID, firstVariantID, testUserID); err != nil {
		t.Fatal(err)
	}
	state = creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.DeliveryStatus != "completed" || state.DerivedStatus != "completed" {
		t.Fatalf("adopted complete delivery state = %#v", state)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_asset
SET attachment_id = NULL
WHERE variant_id = $1 AND revision = 1 AND size_key = '800x1000' AND stage = 'delivered'
`, firstVariantID); err != nil {
		t.Fatal(err)
	}

	state = creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.DeliveryStatus != "partial" || state.DerivedStatus != "partial" || state.ProductionStatus != "completed" {
		t.Fatalf("active delivery with a missing attachment = delivery %q production %q derived %q", state.DeliveryStatus, state.ProductionStatus, state.DerivedStatus)
	}
}

func TestCreativeOrderStatusTreatsWaitingLocalDirectoryAsActiveStartedWork(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "waiting local creative task")
	var variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status, candidate_state, primary_size)
VALUES ($1, 'C01', 1, 'queued', 'candidate', '1080x1080')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-waiting-local-"+uuid.NewString(), nil)
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id
)
VALUES (
  $1, $2, $3, 'waiting_local_directory',
  jsonb_build_object(
    'type', 'creative_domain_task', 'workflow', 'creative_production',
    'creative_order_id', $4::text, 'creative_order_item_id', $5::text,
    'variant_id', $6::text, 'revision', 1
  ),
  'creative_order_item_production', $5::text::uuid
)
`, agentID, handlerTestRuntimeID(t), issueID, orderID, itemID, variantID); err != nil {
		t.Fatal(err)
	}

	state := creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.ProductionStatus != "running" || state.DerivedStatus != "running" {
		t.Fatalf("waiting local task state = production %q derived %q, want running", state.ProductionStatus, state.DerivedStatus)
	}
}
