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
	"github.com/jackc/pgx/v5/pgconn"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func createCreativeLifecycleTestOrder(t *testing.T, title string) (string, string, string) {
	t.Helper()
	issueID, candidateID := createCreativeFeedbackCandidate(t, title)
	var orderID, itemID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, $2, 'running', '{"pipeline_version":"candidate_v1","expected_sizes":["1080x1080","1200x628","800x1000"]}'::jsonb, 'manual', $3)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'running')
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	return orderID, itemID, issueID
}

func putCreativeLifecycleVariant(t *testing.T, orderID, itemID, key string, revision int, status string) creativeOrderVariantResponse {
	t.Helper()
	candidateState, primarySize := "", ""
	if revision == 1 {
		candidateState = "candidate"
		primarySize = "1080x1080"
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID:    itemID,
		VariantKey:     key,
		Brief:          json.RawMessage(`{}`),
		Revision:       revision,
		Status:         status,
		CandidateState: candidateState,
		PrimarySize:    primarySize,
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("variant put %s r%d = %d %s", key, revision, w.Code, w.Body.String())
	}
	var variant creativeOrderVariantResponse
	if err := json.NewDecoder(w.Body).Decode(&variant); err != nil {
		t.Fatal(err)
	}
	if revision == 1 {
		rank := int(key[len(key)-1] - '0')
		if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant
SET candidate_state = 'selected', selection_rank = $2
WHERE id = $1
`, variant.ID, rank); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant_revision
SET expected_sizes = $2::text[]
WHERE variant_id = $1 AND revision = 1
`, variant.ID, standardCreativeAssetSizes); err != nil {
			t.Fatal(err)
		}
		variant.CandidateState = "selected"
		variant.SelectionRank = rank
	}
	return variant
}

func TestCreativeTaskImageScopeUsesNarrowestWorkSet(t *testing.T) {
	tests := []struct {
		name    string
		context string
		allowed string
		denied  string
	}{
		{
			name:    "initial production uses expected sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_production","expected_sizes":["1080x1080","1200x628"]}`,
			allowed: "1080x1080", denied: "800x1000",
		},
		{
			name:    "missing sizes override expected sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_production","expected_sizes":["1080x1080","1200x628","800x1000"],"missing_sizes":["1200x628"]}`,
			allowed: "1200x628", denied: "1080x1080",
		},
		{
			name:    "missing sizes narrow inherited visual rework targets",
			context: `{"type":"creative_domain_task","workflow":"creative_production","expected_sizes":["1080x1080","1200x628","800x1000"],"missing_sizes":["1200x628"],"qc_visual_rework":{"target_sizes":["1200x628","800x1000"]}}`,
			allowed: "1200x628", denied: "800x1000",
		},
		{
			name:    "late recoveries narrow missing and visual rework sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_production","expected_sizes":["1080x1080","1200x628","800x1000"],"missing_sizes":["1200x628"],"qc_visual_rework":{"target_sizes":["1200x628","800x1000"]},"late_receipt_recoveries":[{"size_key":"1080x1080"}]}`,
			allowed: "1080x1080", denied: "1200x628",
		},
		{
			name:    "direct edit sizes override target and expected sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_direct_edit","expected_sizes":["1080x1080","1200x628","800x1000"],"target_size":"1080x1080","edit_sizes":["1200x628"]}`,
			allowed: "1200x628", denied: "1080x1080",
		},
		{
			name:    "direct target overrides expected sizes",
			context: `{"type":"creative_domain_task","workflow":"creative_direct_edit","expected_sizes":["1080x1080","1200x628","800x1000"],"target_size":"800x1000"}`,
			allowed: "800x1000", denied: "1080x1080",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed, err := creativeTaskAllowsImageSize(json.RawMessage(test.context), test.allowed)
			if err != nil || !allowed {
				t.Fatalf("allowed size %s = %t, err %v", test.allowed, allowed, err)
			}
			allowed, err = creativeTaskAllowsImageSize(json.RawMessage(test.context), test.denied)
			if err != nil || allowed {
				t.Fatalf("denied size %s = %t, err %v", test.denied, allowed, err)
			}
		})
	}
}

func TestCreativeVariantStagingDoesNotReplaceActiveDelivery(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "active creative revision")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "active-r1-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variant.ID), 1, standardCreativeAssetSizes); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	for rank := 2; rank <= 3; rank++ {
		key := "C0" + string(rune('0'+rank))
		ready := putCreativeLifecycleVariant(t, orderID, itemID, key, 1, "running")
		for _, size := range standardCreativeAssetSizes {
			attachmentID := createCreativeOrderAssetAttachment(t, "active-"+key+"-"+strings.ReplaceAll(size, "x", "-")+".png")
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, ready.ID, size, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
		readyTx, err := testPool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := activateCreativeVariantRevision(t.Context(), readyTx, parseUUID(ready.ID), 1, standardCreativeAssetSizes); err != nil {
			_ = readyTx.Rollback(t.Context())
			t.Fatal(err)
		}
		if err := readyTx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}

	staging := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 2, "running")
	if staging.ActiveRevision != 1 || staging.StagingRevision != 2 || staging.Revision != 2 {
		t.Fatalf("staging lifecycle = %#v, want active r1 and staging r2", staging)
	}
	staging = putCreativeLifecycleVariant(t, orderID, itemID, "C01", 2, "action_required")
	if staging.ActiveRevision != 1 || staging.StagingRevision != 2 {
		t.Fatalf("failed staging lifecycle = %#v, active delivery must remain r1", staging)
	}

	state := creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.DeliveryStatus != "awaiting_adoption" || state.ProductionStatus != "action_required" || state.DerivedStatus != "awaiting_adoption" {
		t.Fatalf("order workflow state = delivery %q production %q derived %q", state.DeliveryStatus, state.ProductionStatus, state.DerivedStatus)
	}
}

func TestSelectCreativeOrderVariantRevisionRestoresCompletedDelivery(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "select creative revision")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "completed")
	for _, revision := range []int{1, 2} {
		if revision == 2 {
			variant = putCreativeLifecycleVariant(t, orderID, itemID, "C01", revision, "completed")
		}
		for _, size := range standardCreativeAssetSizes {
			attachmentID := createCreativeOrderAssetAttachment(t, fmt.Sprintf("revision-select-r%d-%s.png", revision, strings.ReplaceAll(size, "x", "-")))
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, $3, 'delivered', $4, 'completed')
`, variant.ID, size, revision, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
		tx, err := testPool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variant.ID), revision, standardCreativeAssetSizes); err != nil {
			_ = tx.Rollback(t.Context())
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_feedback_event WHERE subject_id = $1`, variant.ID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_revision_selected'`, issueID)
	})

	w := httptest.NewRecorder()
	req := withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variant.ID+"/revisions/1/select", nil), "id", orderID, "variantId", variant.ID, "revision", "1")
	testHandler.SelectCreativeOrderVariantRevision(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("SelectCreativeOrderVariantRevision = %d %s", w.Code, w.Body.String())
	}
	var activeRevision, stagingRevision, currentRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT active_revision, staging_revision, revision
FROM creative_order_variant WHERE id = $1
`, variant.ID).Scan(&activeRevision, &stagingRevision, &currentRevision); err != nil {
		t.Fatal(err)
	}
	if activeRevision != 1 || stagingRevision != 1 || currentRevision != 2 {
		t.Fatalf("selected revision lifecycle = active %d staging %d current %d", activeRevision, stagingRevision, currentRevision)
	}
	w = httptest.NewRecorder()
	req = withURLParams(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/variants/"+variant.ID+"/revisions/2/select", nil), "id", orderID, "variantId", variant.ID, "revision", "2")
	testHandler.SelectCreativeOrderVariantRevision(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("restore latest creative revision = %d %s", w.Code, w.Body.String())
	}
	if err := testPool.QueryRow(t.Context(), `SELECT active_revision FROM creative_order_variant WHERE id = $1`, variant.ID).Scan(&activeRevision); err != nil {
		t.Fatal(err)
	}
	if activeRevision != 2 {
		t.Fatalf("restored active revision = %d", activeRevision)
	}
}

func TestCreativeOrderListIncludesOnlyActiveAndStagingRevisionScopes(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "creative list revision scopes")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order
SET trigger_evidence_kind = 'creative_direct_edit',
    input_snapshot = '{"pipeline_version":"direct_edit_v1","target_size":"1200x628"}'::jsonb
WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	var variantID string
	if err := tx.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, revision, status, candidate_state, primary_size
) VALUES ($1, 'V01', 3, 'running', 'selected', '1200x628')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
INSERT INTO creative_order_variant_revision (
  variant_id, revision, brief, status, expected_sizes, activated_at
) VALUES
  ($1, 1, '{}'::jsonb, 'completed', ARRAY['1200x628']::text[], now()),
  ($1, 2, '{}'::jsonb, 'action_required', ARRAY['800x1000']::text[], NULL),
  ($1, 3, '{}'::jsonb, 'running', ARRAY['1080x1080']::text[], NULL)
`, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
UPDATE creative_order_variant SET active_revision = 1, staging_revision = 3 WHERE id = $1
`, variantID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	testHandler.ListCreativeOrders(w, newRequest(http.MethodGet, "/api/creative/orders", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list creative revision scopes = %d %s", w.Code, w.Body.String())
	}
	var response struct {
		Orders []creativeOrderResponse `json:"orders"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	var listed *creativeOrderVariantResponse
	for orderIndex := range response.Orders {
		if response.Orders[orderIndex].ID != orderID || len(response.Orders[orderIndex].Items) != 1 {
			continue
		}
		for variantIndex := range response.Orders[orderIndex].Items[0].Variants {
			if response.Orders[orderIndex].Items[0].Variants[variantIndex].ID == variantID {
				listed = &response.Orders[orderIndex].Items[0].Variants[variantIndex]
			}
		}
	}
	if listed == nil {
		t.Fatal("list response omitted creative variant")
	}
	if listed.ActiveRevision != 1 || listed.StagingRevision != 3 || len(listed.Revisions) != 2 {
		t.Fatalf("list revision projection = active %d staging %d revisions %#v", listed.ActiveRevision, listed.StagingRevision, listed.Revisions)
	}
	if listed.Revisions[0].Revision != 1 || !slices.Equal(listed.Revisions[0].ExpectedSizes, []string{"1200x628"}) ||
		listed.Revisions[1].Revision != 3 || !slices.Equal(listed.Revisions[1].ExpectedSizes, []string{"1080x1080"}) {
		t.Fatalf("list target revision scopes = %#v", listed.Revisions)
	}
}

func TestCreativeActivatedRevisionIsImmutableAndIdempotent(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "immutable active creative revision")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	activeAttachments := map[string]string{}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "immutable-active-"+strings.ReplaceAll(size, "x", "-")+".png")
		activeAttachments[size] = attachmentID
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed'),
       ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variant.ID), 1, standardCreativeAssetSizes); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var frozenBrief, frozenUpdatedAt string
	if err := testPool.QueryRow(t.Context(), `
SELECT brief::text, updated_at::text
FROM creative_order_variant_revision
WHERE variant_id = $1 AND revision = 1
`, variant.ID).Scan(&frozenBrief, &frozenUpdatedAt); err != nil {
		t.Fatal(err)
	}

	replay := httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{}`), Revision: 1, Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(replay, req)
	if replay.Code != http.StatusOK {
		t.Fatalf("idempotent active revision replay = %d %s", replay.Code, replay.Body.String())
	}
	var replayUpdatedAt string
	if err := testPool.QueryRow(t.Context(), `SELECT updated_at::text FROM creative_order_variant_revision WHERE variant_id = $1 AND revision = 1`, variant.ID).Scan(&replayUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if replayUpdatedAt != frozenUpdatedAt {
		t.Fatalf("idempotent active replay changed revision timestamp: %q -> %q", frozenUpdatedAt, replayUpdatedAt)
	}

	changed := httptest.NewRecorder()
	req = newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/variants", creativeOrderVariantInput{
		OrderItemID: itemID, VariantKey: "C01", Brief: json.RawMessage(`{"changed":true}`), Revision: 1, Status: "completed",
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderVariant(changed, req)
	if changed.Code != http.StatusConflict || !strings.Contains(changed.Body.String(), "immutable") {
		t.Fatalf("active revision mutation = %d %s", changed.Code, changed.Body.String())
	}
	var variantBrief, revisionBrief string
	var activeRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.brief::text, revision.brief::text, variant.active_revision
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision ON revision.variant_id = variant.id AND revision.revision = 1
WHERE variant.id = $1
`, variant.ID).Scan(&variantBrief, &revisionBrief, &activeRevision); err != nil {
		t.Fatal(err)
	}
	if variantBrief != frozenBrief || revisionBrief != frozenBrief || activeRevision != 1 {
		t.Fatalf("active snapshot changed after rejected mutation: variant=%s revision=%s active=%d", variantBrief, revisionBrief, activeRevision)
	}

	var assetFamilyID, assetUpdatedAt string
	if err := testPool.QueryRow(t.Context(), `
SELECT asset_family_id::text, updated_at::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND size_key = '1080x1080' AND stage = 'delivered'
`, variant.ID).Scan(&assetFamilyID, &assetUpdatedAt); err != nil {
		t.Fatal(err)
	}
	putAsset := func(attachmentID string, metadata json.RawMessage) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		request := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variant.ID, AssetFamilyID: assetFamilyID, SizeKey: "1080x1080", Revision: 1,
			Stage: "delivered", AttachmentID: attachmentID, Metadata: metadata, Evidence: json.RawMessage(`{}`), Status: "completed",
		})
		request = withURLParam(request, "id", orderID)
		testHandler.UpsertCreativeOrderAsset(response, request)
		return response
	}
	replayedAsset := putAsset(activeAttachments["1080x1080"], json.RawMessage(`{}`))
	if replayedAsset.Code != http.StatusForbidden || !strings.Contains(replayedAsset.Body.String(), "registered by the platform") {
		t.Fatalf("external active delivery replay = %d %s", replayedAsset.Code, replayedAsset.Body.String())
	}
	var replayedAssetUpdatedAt string
	if err := testPool.QueryRow(t.Context(), `
SELECT updated_at::text FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND size_key = '1080x1080' AND stage = 'delivered'
`, variant.ID).Scan(&replayedAssetUpdatedAt); err != nil {
		t.Fatal(err)
	}
	if replayedAssetUpdatedAt != assetUpdatedAt {
		t.Fatalf("idempotent active asset replay changed timestamp: %q -> %q", assetUpdatedAt, replayedAssetUpdatedAt)
	}

	replacementAttachmentID := createCreativeOrderAssetAttachment(t, "immutable-active-replacement.png")
	changedAsset := putAsset(replacementAttachmentID, json.RawMessage(`{"changed":true}`))
	if changedAsset.Code != http.StatusForbidden || !strings.Contains(changedAsset.Body.String(), "registered by the platform") {
		t.Fatalf("active asset mutation = %d %s", changedAsset.Code, changedAsset.Body.String())
	}
	var retainedAttachmentID, retainedMetadata string
	if err := testPool.QueryRow(t.Context(), `
SELECT attachment_id::text, metadata::text FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND size_key = '1080x1080' AND stage = 'delivered'
`, variant.ID).Scan(&retainedAttachmentID, &retainedMetadata); err != nil {
		t.Fatal(err)
	}
	if retainedAttachmentID != activeAttachments["1080x1080"] || retainedMetadata != "{}" {
		t.Fatalf("active asset changed after rejected mutation: attachment=%s metadata=%s", retainedAttachmentID, retainedMetadata)
	}

	originalStorage := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	t.Cleanup(func() { testHandler.Storage = originalStorage })
	composed, err := testHandler.runCreativeOrderPrimeComposition(
		t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), parseUUID(testUserID), true, nil,
	)
	if err == nil || composed || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("forced active Prime composition = composed %t err %v", composed, err)
	}
	var retainedStatus, retainedRevisionStatus, retainedVariantBrief, retainedRevisionBrief string
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status, variant.brief::text, revision.brief::text
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.active_revision
WHERE variant.id = $1
`, variant.ID).Scan(&retainedStatus, &retainedRevisionStatus, &retainedVariantBrief, &retainedRevisionBrief); err != nil {
		t.Fatal(err)
	}
	if retainedStatus != "completed" || retainedRevisionStatus != "completed" ||
		retainedVariantBrief != variantBrief || retainedRevisionBrief != revisionBrief ||
		strings.Contains(retainedVariantBrief, "brand_composition_error") {
		t.Fatalf("forced active Prime mutated status/brief = %q/%q %s %s", retainedStatus, retainedRevisionStatus, retainedVariantBrief, retainedRevisionBrief)
	}
	leaseToken := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, attempt, lease_token, lease_expires_at
) VALUES ($1, 1, 'running', 1, $2, now() + interval '10 minutes')
`, variant.ID, leaseToken); err != nil {
		t.Fatal(err)
	}
	claim := creativePrimeCompositionClaim{
		WorkspaceID: parseUUID(testWorkspaceID), OrderID: parseUUID(orderID), OrderItemID: parseUUID(itemID),
		VariantID: parseUUID(variant.ID), Revision: 1, LeaseToken: parseUUID(leaseToken),
	}
	if err := testHandler.finishCreativePrimeComposition(
		t.Context(), claim, &creativePrimeImmutableRevisionError{message: "active revision won the composition race"},
	); err != nil {
		t.Fatal(err)
	}
	var immutableJobStatus, finalVariantStatus, finalRevisionStatus, finalVariantBrief, finalRevisionBrief string
	if err := testPool.QueryRow(t.Context(), `
SELECT job.status, variant.status, revision.status, variant.brief::text, revision.brief::text
FROM creative_prime_composition_job job
JOIN creative_order_variant variant ON variant.id = job.variant_id
JOIN creative_order_variant_revision revision
  ON revision.variant_id = job.variant_id AND revision.revision = job.revision
WHERE job.variant_id = $1 AND job.revision = 1
`, variant.ID).Scan(
		&immutableJobStatus, &finalVariantStatus, &finalRevisionStatus, &finalVariantBrief, &finalRevisionBrief,
	); err != nil {
		t.Fatal(err)
	}
	if immutableJobStatus != "cancelled" || finalVariantStatus != retainedStatus || finalRevisionStatus != retainedRevisionStatus ||
		finalVariantBrief != retainedVariantBrief || finalRevisionBrief != retainedRevisionBrief {
		t.Fatalf("immutable Prime lease finalization = job %q status %q/%q brief %s %s",
			immutableJobStatus, finalVariantStatus, finalRevisionStatus, finalVariantBrief, finalRevisionBrief)
	}
}

func TestCreativeAdoptionUsesActiveRevisionWhenStagingFailed(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "adopt active creative revision")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "adopt-active-r1-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed'),
       ($1, $2, 1, 'delivered', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings)
VALUES ($1, 'technical', 1, 'passed', '{}'::jsonb),
	       ($1, 'visual', 1, 'passed', '{}'::jsonb)
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_variant_qc_resolution (variant_id, revision, outcome, issue_id)
VALUES ($1, 1, 'delivered', $2)
	`, variant.ID, issueID); err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variant.ID), 1, standardCreativeAssetSizes); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	staging := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 2, "action_required")
	if staging.ActiveRevision != 1 || staging.StagingRevision != 2 {
		t.Fatalf("staging setup = %#v", staging)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_feedback_event WHERE subject_id = $1`, variant.ID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_variant_adopted'`, issueID)
	})

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/adoption", creativeOrderItemAdoptionInput{VariantID: variant.ID})
	req = withURLParams(req, "id", orderID, "itemId", itemID)
	testHandler.AdoptCreativeOrderItemVariant(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("adopt active revision behind failed staging = %d %s", w.Code, w.Body.String())
	}
	var item creativeOrderItemResponse
	if err := json.NewDecoder(w.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if item.AdoptedVariantID != variant.ID {
		t.Fatalf("active revision adoption item = %#v", item)
	}
	var adoptedRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT (context_snapshot->>'revision')::int
FROM creative_feedback_event
WHERE subject_id = $1 AND decision = 'accepted'
ORDER BY created_at DESC LIMIT 1
`, variant.ID).Scan(&adoptedRevision); err != nil {
		t.Fatal(err)
	}
	if adoptedRevision != 1 {
		t.Fatalf("adopted revision = %d, want active revision 1", adoptedRevision)
	}
}

func TestCreativeLegacyCompletedVariantWithoutDeliveryIsNotReady(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "legacy partial delivery")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "completed")
	if variant.ActiveRevision != 0 {
		t.Fatalf("variant without delivered assets has active revision %d", variant.ActiveRevision)
	}

	status, err := testHandler.derivedCreativeOrderDeliveryStatus(t.Context(), parseUUID(orderID))
	if err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Fatalf("delivery status = %q, want pending without an active revision", status)
	}
}

func TestCreativeQCExhaustionKeepsPreviousActiveRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "creative QC active revision fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-active-r1-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), tx, parseUUID(variant.ID), 1, standardCreativeAssetSizes); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = putCreativeLifecycleVariant(t, orderID, itemID, "C01", 2, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-staging-r2-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 2, 'primed', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	findings := `{"blocking_failures":[{"code":"actual_prime_obstruction","size_key":"1080x1080","diagnosis":"1080x1080：标题与 Prime 冲突；期望移动到 safe_content_frame y<=700"}]}`
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'technical', 2, 1, 'passed', '{}'::jsonb),
       ($1, 'visual', 2, 1, 'failed', $2::jsonb)
`, variant.ID, findings); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-qc-active-fence", []byte(`{}`))
	for attempt := 0; attempt < creativeVisualModelReworkMaxAttempts; attempt++ {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context, completed_at
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', 'creative_order_item_production', $2,
        jsonb_build_object('type','creative_domain_task','workflow','creative_production','variant_id',$3::text,'revision',2,'qc_visual_rework',jsonb_build_object()), now())
`, agentID, itemID, variant.ID); err != nil {
			t.Fatal(err)
		}
	}
	var qcTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text
`, agentID, variant.ID, creativeQCTaskContextForTest(t, orderID, variant.ID, "visual", 2)).Scan(&qcTaskID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{
		VariantID: variant.ID,
		Revision:  2,
	})
	req = withURLParam(req, "id", orderID)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", qcTaskID)
	testHandler.FinalizeCreativeOrderQC(w, req)
	var response creativeOrderQCFinalizeResponse
	if w.Code == http.StatusOK {
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
	}
	if w.Code != http.StatusOK || response.Outcome != "action_required" || response.DeliveredAssetCount != 0 {
		t.Fatalf("exhausted staging QC = %d %#v %s", w.Code, response, w.Body.String())
	}
	var status, revisionStatus string
	var activeRevision, stagingRevision int
	var deliveredCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, variant.active_revision, variant.staging_revision, revision.status,
       (SELECT count(*) FROM creative_order_asset asset
        WHERE asset.variant_id = variant.id AND asset.revision = 2 AND asset.stage = 'delivered')
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = 2
WHERE variant.id = $1
`, variant.ID).Scan(&status, &activeRevision, &stagingRevision, &revisionStatus, &deliveredCount); err != nil {
		t.Fatal(err)
	}
	if status != "action_required" || revisionStatus != "action_required" || activeRevision != 1 || stagingRevision != 2 || deliveredCount != 0 {
		t.Fatalf("exhausted staging lifecycle = status %q revision %q active %d staging %d delivered %d", status, revisionStatus, activeRevision, stagingRevision, deliveredCount)
	}
	var activityIssueID string
	if err := testPool.QueryRow(t.Context(), `
SELECT issue_id::text FROM activity_log
WHERE issue_id = $1 AND action = 'creative_visual_rework_exhausted_action_required'
ORDER BY created_at DESC LIMIT 1
`, issueID).Scan(&activityIssueID); err != nil {
		t.Fatal(err)
	}
}

func TestCreativeCandidateSelectionRequiresPrimedPrimaryAndExcludesReserves(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "creative candidate selection")
	squad := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
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
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot = $2::jsonb WHERE id = $1`, orderID, inputSnapshot); err != nil {
		t.Fatal(err)
	}
	variantIDs := make([]string, 0, 5)
	attachments := make(map[string]string, 5)
	for index := 1; index <= 5; index++ {
		var variantID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, revision, status, candidate_state, primary_size
)
VALUES ($1, $2, 1, 'completed', 'candidate', '1080x1080')
RETURNING id::text
`, itemID, "C0"+string(rune('0'+index))).Scan(&variantID); err != nil {
			t.Fatal(err)
		}
		variantIDs = append(variantIDs, variantID)
		attachmentID := createCreativeOrderAssetAttachment(t, "candidate-primary-"+variantID+".png")
		attachments[variantID] = attachmentID
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 1, 'generated', $2, 'completed')
`, variantID, attachmentID); err != nil {
			t.Fatal(err)
		}
		if index < 5 {
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 1, 'primed', $2, 'completed')
`, variantID, attachmentID); err != nil {
				t.Fatal(err)
			}
		}
	}
	selection := creativeCandidateSelectionInput{
		SelectedIDs: variantIDs[:3],
		ReserveIDs:  variantIDs[3:],
	}
	selectCandidates := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/items/"+itemID+"/candidate-selection", selection)
		req = withURLParams(req, "id", orderID, "itemId", itemID)
		testHandler.SelectCreativeOrderItemCandidates(w, req)
		return w
	}
	blocked := selectCandidates()
	if blocked.Code != http.StatusConflict || !strings.Contains(blocked.Body.String(), "Prime") {
		t.Fatalf("selection before final Prime = %d %s", blocked.Code, blocked.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, '1080x1080', 1, 'primed', $2, 'completed')
`, variantIDs[4], attachments[variantIDs[4]]); err != nil {
		t.Fatal(err)
	}
	preselectionState := creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &preselectionState); err != nil {
		t.Fatal(err)
	}
	if preselectionState.ProductionStatus != "awaiting_selection" || preselectionState.DeliveryStatus != "pending" {
		t.Fatalf("candidate-ready workflow state = %#v", preselectionState)
	}
	selected := selectCandidates()
	if selected.Code != http.StatusOK {
		t.Fatalf("candidate selection = %d %s", selected.Code, selected.Body.String())
	}
	var item creativeOrderItemResponse
	if err := json.NewDecoder(selected.Body).Decode(&item); err != nil {
		t.Fatal(err)
	}
	if len(item.Variants) != 5 {
		t.Fatalf("selected item variants = %d, want 5", len(item.Variants))
	}
	for index, variant := range item.Variants {
		wantState := "selected"
		wantSizes := standardCreativeAssetSizes
		if index >= 3 {
			wantState = "reserve"
			wantSizes = []string{"1080x1080"}
		}
		if variant.SelectionRank != index+1 || variant.CandidateState != wantState || len(variant.Revisions) != 1 ||
			!slices.Equal(variant.Revisions[0].ExpectedSizes, wantSizes) {
			t.Fatalf("ranked candidate %d = %#v", index+1, variant)
		}
	}
	replayed := selectCandidates()
	if replayed.Code != http.StatusOK {
		t.Fatalf("identical candidate selection replay = %d %s", replayed.Code, replayed.Body.String())
	}
	selection.SelectedIDs[0], selection.SelectedIDs[1] = selection.SelectedIDs[1], selection.SelectedIDs[0]
	changed := selectCandidates()
	if changed.Code != http.StatusConflict || !strings.Contains(changed.Body.String(), "already finalized") {
		t.Fatalf("changed candidate selection replay = %d %s", changed.Code, changed.Body.String())
	}
	selection.SelectedIDs[0], selection.SelectedIDs[1] = selection.SelectedIDs[1], selection.SelectedIDs[0]
	for index, variantID := range variantIDs {
		var state string
		var rank int
		if err := testPool.QueryRow(t.Context(), `SELECT candidate_state, selection_rank FROM creative_order_variant WHERE id = $1`, variantID).Scan(&state, &rank); err != nil {
			t.Fatal(err)
		}
		wantState := "selected"
		if index >= 3 {
			wantState = "reserve"
		}
		if state != wantState || rank != index+1 {
			t.Fatalf("immutable candidate rank %d = %q/%d", index+1, state, rank)
		}
	}
	var expansionTasks int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'production_phase' = $2
  AND context->'missing_sizes' = '["1200x628", "800x1000"]'::jsonb
`, itemID, creativeSelectedExpansionPhase).Scan(&expansionTasks); err != nil {
		t.Fatal(err)
	}
	if expansionTasks != 3 {
		t.Fatalf("selected missing-size expansion tasks = %d, want 3", expansionTasks)
	}

	for _, variantID := range variantIDs[:3] {
		for _, size := range standardCreativeAssetSizes {
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, variantID, size, attachments[variantID]); err != nil {
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
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'failed' WHERE id = ANY($1::uuid[])
`, variantIDs[3:]); err != nil {
		t.Fatal(err)
	}
	state := creativeOrderResponse{ID: orderID}
	if err := testHandler.loadCreativeOrderWorkflowState(newRequest(http.MethodGet, "/", nil), &state); err != nil {
		t.Fatal(err)
	}
	if state.DeliveryStatus != "awaiting_adoption" || state.DerivedStatus != "awaiting_adoption" {
		t.Fatalf("reserve failures changed formal delivery: %#v", state)
	}
}

func TestCreativeImageOperationDeduplicatesAndAcceptsLateSuccess(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "creative image operation")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-image-operation", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_item_production', $2::uuid,
		jsonb_build_object(
		  'type','creative_domain_task','workflow','creative_production',
		  'creative_order_id',$3::text,'creative_order_item_id',$2::text,
		  'variant_id',$4::text,'revision',1,
		  'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
		))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	rawAttachmentID := createCreativeOrderAssetAttachment(t, "operation-provider-raw.png")
	canonicalAttachmentID := createCreativeOrderAssetAttachment(t, "operation-output.png")
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })
	store.put("oss://creative/operation-output.png", creativeTestPNG(t, 1080, 1080))

	putOperationWithTask := func(input creativeImageOperationInput, actorTaskID string) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		if actorTaskID != "" {
			req.Header.Set("X-Actor-Source", "task_token")
			req.Header.Set("X-Agent-ID", agentID)
			req.Header.Set("X-Task-ID", actorTaskID)
		}
		testHandler.UpsertCreativeImageOperation(w, req)
		var response creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return w, response
	}
	putOperation := func(input creativeImageOperationInput) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		return putOperationWithTask(input, taskID)
	}
	base := creativeImageOperationInput{
		VariantID:      variant.ID,
		SizeKey:        "1080x1080",
		Revision:       1,
		OperationKind:  "generation",
		IdempotencyKey: "generation:" + variant.ID + ":r1:1080x1080",
		Status:         "running",
		Model:          "gpt-image-2",
		PromptSHA256:   creativePromptSHA256("operation linked prompt"),
		InputSnapshot:  json.RawMessage(`{"input_asset_sha256":"fixture"}`),
		Attempt:        1,
	}
	premature := base
	premature.IdempotencyKey = "premature-completed:" + variant.ID
	premature.Status = "completed"
	premature.ProviderRequestID = "premature-provider-request"
	premature.PromptSHA256 = creativePromptSHA256("premature operation prompt")
	premature.ResultReceipt = json.RawMessage(`{"provider_status":"succeeded"}`)
	premature.OutputAttachmentID = rawAttachmentID
	w, _ := putOperation(premature)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "start queued or running") {
		t.Fatalf("premature completed operation = %d %s", w.Code, w.Body.String())
	}
	missingContract := base
	missingContract.SizeKey = "1200x628"
	missingContract.IdempotencyKey = "missing-contract:" + variant.ID
	missingContract.InputSnapshot = json.RawMessage(`{}`)
	w, _ = putOperation(missingContract)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "input_snapshot") {
		t.Fatalf("operation without frozen input contract = %d %s", w.Code, w.Body.String())
	}
	deferredPrompt := base
	deferredPrompt.SizeKey = "800x1000"
	deferredPrompt.IdempotencyKey = "deferred-prompt:" + variant.ID
	deferredPrompt.PromptSHA256 = ""
	w, deferredOperation := putOperation(deferredPrompt)
	if w.Code != http.StatusOK || deferredOperation.Disposition != "invoke" || deferredOperation.PromptSHA256 != "" {
		t.Fatalf("start operation before prompt receipt = %d %#v %s", w.Code, deferredOperation, w.Body.String())
	}
	unverifiedPrompt := deferredPrompt
	unverifiedPrompt.PromptSHA256 = creativePromptSHA256("unverified prompt")
	w, _ = putOperation(unverifiedPrompt)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "completed result receipt") {
		t.Fatalf("operation accepted prompt before result receipt = %d %s", w.Code, w.Body.String())
	}
	deferredPrompt.Status = "completed"
	deferredPrompt.PromptSHA256 = creativePromptSHA256("prompt from atomic image receipt")
	deferredPrompt.ProviderRequestID = "provider-deferred-prompt"
	deferredPrompt.ProviderStatus = "completed"
	deferredPrompt.ResultReceipt = json.RawMessage(`{"request_id":"provider-deferred-prompt","prompt_sha256":"` + deferredPrompt.PromptSHA256 + `"}`)
	deferredPrompt.OutputAttachmentID = rawAttachmentID
	w, deferredOperation = putOperation(deferredPrompt)
	if w.Code != http.StatusOK || deferredOperation.Status != "completed" || deferredOperation.PromptSHA256 != deferredPrompt.PromptSHA256 {
		t.Fatalf("settle operation with atomic prompt receipt = %d %#v %s", w.Code, deferredOperation, w.Body.String())
	}
	w, operation := putOperation(base)
	if w.Code != http.StatusOK || operation.ID == "" || operation.Disposition != "invoke" || len(operation.Attempts) != 1 {
		t.Fatalf("start image operation = %d %#v %s", w.Code, operation, w.Body.String())
	}
	w, _ = putOperationWithTask(base, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("member mutated image operation = %d %s", w.Code, w.Body.String())
	}
	mutatedModel := base
	mutatedModel.Model = "different-image-model"
	w, _ = putOperation(mutatedModel)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "immutable") {
		t.Fatalf("operation model mutation = %d %s", w.Code, w.Body.String())
	}
	var unrelatedTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_item_production', $2::uuid,
		jsonb_build_object(
		  'type','creative_domain_task','workflow','creative_production',
		  'creative_order_id',$3::text,'creative_order_item_id',$2::text,
		  'variant_id',$4::text,'revision',1,
		  'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
		))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&unrelatedTaskID); err != nil {
		t.Fatal(err)
	}
	w, _ = putOperationWithTask(base, unrelatedTaskID)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "continuation chain") {
		t.Fatalf("unrelated task settled image operation = %d %s", w.Code, w.Body.String())
	}
	metadata, evidence := completedGeneratedAssetTrace("operation linked prompt", "provider-request-late", 1)
	putGeneratedAsset := func(operationID string, assetMetadata, assetEvidence json.RawMessage) *httptest.ResponseRecorder {
		t.Helper()
		assetWriter := httptest.NewRecorder()
		assetRequest := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
			AttachmentID: canonicalAttachmentID, OperationID: operationID, Metadata: assetMetadata, Evidence: assetEvidence,
		})
		assetRequest = withURLParam(assetRequest, "id", orderID)
		testHandler.UpsertCreativeOrderAsset(assetWriter, assetRequest)
		return assetWriter
	}
	w = putGeneratedAsset(operation.ID, metadata, evidence)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "completed image operation") {
		t.Fatalf("running operation borrowed by generated asset = %d %s", w.Code, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status
)
VALUES ($1, '1080x1080', 1, 'generation', $2, 'unknown')
`, variant.ID, "raw-conflict:"+variant.ID); err == nil {
		t.Fatal("database accepted a second in-flight operation for one variant revision and size")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			t.Fatalf("second in-flight operation error = %v", err)
		}
	}

	duplicate := base
	duplicate.IdempotencyKey = "different-key:" + variant.ID
	w, recovered := putOperation(duplicate)
	if w.Code != http.StatusOK || recovered.ID != operation.ID || recovered.Disposition != "reconcile" {
		t.Fatalf("in-flight recovery = %d %#v %s", w.Code, recovered, w.Body.String())
	}
	wrongCoordinates := base
	wrongCoordinates.SizeKey = "1200x628"
	w, _ = putOperation(wrongCoordinates)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "coordinates") {
		t.Fatalf("idempotency coordinate mutation = %d %s", w.Code, w.Body.String())
	}

	unknown := base
	unknown.Status = "unknown"
	unknown.ProviderRequestID = "provider-request-late"
	unknown.ProviderStatus = "pending"
	w, operation = putOperation(unknown)
	if w.Code != http.StatusOK || operation.Status != "unknown" || operation.Disposition != "reconcile" {
		t.Fatalf("unknown image operation = %d %#v %s", w.Code, operation, w.Body.String())
	}
	mutatedProvider := unknown
	mutatedProvider.ProviderRequestID = "different-provider-request"
	w, _ = putOperation(mutatedProvider)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "provider request is immutable") {
		t.Fatalf("attempt provider mutation = %d %s", w.Code, w.Body.String())
	}

	failed := base
	failed.Status = "failed"
	failed.ProviderRequestID = "provider-request-late"
	failed.ProviderStatus = "not_found"
	failed.ErrorType = "provider_receipt_not_found"
	failed.ErrorMessage = "provider receipt reconciliation confirmed that no result is available"
	w, _ = putOperation(failed)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "confirmed provider receipt reconciliation") {
		t.Fatalf("unconfirmed unknown operation termination = %d %s", w.Code, w.Body.String())
	}
	failed.ReconcileConfirmed = true
	w, _ = putOperationWithTask(failed, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("member-confirmed image operation failure = %d %s", w.Code, w.Body.String())
	}
	loadedUnknown, err := testHandler.loadCreativeImageOperation(t.Context(), parseUUID(operation.ID))
	if err != nil {
		t.Fatal(err)
	}
	if loadedUnknown.Status != "unknown" {
		t.Fatalf("member confirmation changed unknown operation to %q", loadedUnknown.Status)
	}
	w, operation = putOperation(failed)
	if w.Code != http.StatusOK || operation.Status != "failed" || operation.Disposition != "terminal" {
		t.Fatalf("task-confirmed failed image operation = %d %#v %s", w.Code, operation, w.Body.String())
	}

	reopenSameAttempt := base
	w, _ = putOperation(reopenSameAttempt)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "next attempt") {
		t.Fatalf("failed attempt reopened = %d %s", w.Code, w.Body.String())
	}
	retry := base
	retry.Attempt = 2
	retry.IdempotencyKey = "rotated-retry-key:" + variant.ID
	w, operation = putOperation(retry)
	if w.Code != http.StatusOK || operation.Status != "running" || operation.Disposition != "invoke" || len(operation.Attempts) != 2 {
		t.Fatalf("next image attempt = %d %#v %s", w.Code, operation, w.Body.String())
	}
	if operation.IdempotencyKey != base.IdempotencyKey {
		t.Fatalf("retry changed platform operation identity to %q, want %q", operation.IdempotencyKey, base.IdempotencyKey)
	}
	var operationCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM creative_image_operation
WHERE variant_id = $1 AND revision = 1 AND size_key = '1080x1080' AND operation_kind = 'generation'
`, variant.ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 1 {
		t.Fatalf("rotated retry key created %d coordinate operations, want 1", operationCount)
	}
	w, operation = putOperation(retry)
	if w.Code != http.StatusOK || operation.Status != "running" || operation.Disposition != "reconcile" || len(operation.Attempts) != 2 {
		t.Fatalf("running image attempt replay = %d %#v %s", w.Code, operation, w.Body.String())
	}

	httpStatus := http.StatusOK
	durationMS := int64(96_000)
	late := base
	late.Status = "completed"
	late.ProviderRequestID = "provider-request-late"
	late.ProviderStatus = "succeeded"
	late.PromptSHA256 = creativePromptSHA256("operation linked prompt")
	late.HTTPStatus = &httpStatus
	late.DurationMS = &durationMS
	late.ResultReceipt = json.RawMessage(`{"provider_status":"succeeded","late":true}`)
	late.OutputAttachmentID = rawAttachmentID
	w, operation = putOperation(late)
	if w.Code != http.StatusOK || operation.Status != "completed" || operation.ProviderRequestID != "provider-request-late" ||
		operation.Disposition != "reuse" || len(operation.Attempts) != 2 || operation.Attempts[0].Status != "completed" ||
		operation.Attempts[0].ErrorType != "provider_receipt_not_found" || operation.Attempts[1].Status != "cancelled" {
		t.Fatalf("late image success = %d %#v %s", w.Code, operation, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, operation_id, status)
VALUES ($1, '1200x628', 1, 'generated', $2, 'running')
`, variant.ID, operation.ID); err == nil {
		t.Fatal("database accepted an image operation on an asset with different coordinates")
	} else {
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Fatalf("cross-coordinate operation association error = %v", err)
		}
	}

	w = httptest.NewRecorder()
	req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
		AttachmentID: canonicalAttachmentID, Metadata: metadata, Evidence: evidence,
	})
	req = withURLParam(req, "id", orderID)
	testHandler.UpsertCreativeOrderAsset(w, req)
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "operation_id") {
		t.Fatalf("generated asset without operation = %d %s", w.Code, w.Body.String())
	}
	wrongMetadata, wrongEvidence := completedGeneratedAssetTrace("wrong operation prompt", "wrong-provider-request", 1)
	w = putGeneratedAsset(operation.ID, wrongMetadata, wrongEvidence)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "lineage") {
		t.Fatalf("mismatched operation evidence = %d %s", w.Code, w.Body.String())
	}
	w = putGeneratedAsset(operation.ID, metadata, evidence)
	if w.Code != http.StatusOK {
		t.Fatalf("operation asset association = %d %s", w.Code, w.Body.String())
	}
	var asset creativeOrderAssetResponse
	if err := json.NewDecoder(w.Body).Decode(&asset); err != nil {
		t.Fatal(err)
	}
	if asset.OperationID != operation.ID {
		t.Fatalf("generated asset operation_id = %q, want %q", asset.OperationID, operation.ID)
	}
	loaded, err := testHandler.loadCreativeImageOperation(t.Context(), parseUUID(operation.ID))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.OutputAttachmentID != rawAttachmentID || loaded.OutputAttachmentID == canonicalAttachmentID || loaded.OutputAssetID != asset.ID {
		t.Fatalf("settled image operation = %#v", loaded)
	}

	staleFailure := failed
	w, _ = putOperation(staleFailure)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "immutable") {
		t.Fatalf("late failure overwrote completed operation = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeDirectEditVisualReworkRequiresOneRejectedOutputAndUsesOneBudget(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "direct edit visual rework budget")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "direct-edit-visual-rework", []byte(`{}`))
	taskContext := fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_direct_edit",
  "creative_order_id":%q,"creative_order_item_id":%q,
  "variant_id":%q,"revision":1,
  "expected_sizes":["1080x1080"],"edit_sizes":["1080x1080"],
  "direct_edit":{"target_size":"1080x1080","edit_sizes":["1080x1080"],"visual_rework_budget":1}
}`, orderID, itemID, variant.ID)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_item_direct_edit', $2::uuid, $3::jsonb)
RETURNING id::text
`, agentID, itemID, taskContext).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	putOperation := func(input creativeImageOperationInput) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		var response creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return w, response
	}
	prompt := "move the title and table into the approved safe areas"
	direct := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "direct_edit",
		IdempotencyKey: "direct-edit:" + variant.ID + ":r1:1080x1080", Status: "running", Attempt: 1,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256(prompt),
		InputSnapshot: json.RawMessage(`{"input_attachment_id":"source","input_role":"source"}`),
	}
	w, directOperation := putOperation(direct)
	if w.Code != http.StatusOK || directOperation.Disposition != "invoke" {
		t.Fatalf("start direct edit = %d %#v %s", w.Code, directOperation, w.Body.String())
	}
	rawAttachmentID := createCreativeOrderAssetAttachment(t, "rejected-direct-edit-output.png")
	direct.Status = "completed"
	direct.ProviderRequestID = "rejected-direct-edit-provider-request"
	direct.ProviderStatus = "succeeded"
	direct.ResultReceipt = json.RawMessage(`{"request_id":"rejected-direct-edit-provider-request","provider_status":"succeeded"}`)
	direct.OutputAttachmentID = rawAttachmentID
	w, directOperation = putOperation(direct)
	if w.Code != http.StatusOK || directOperation.Status != "completed" || directOperation.OutputAttachmentID != rawAttachmentID || directOperation.OutputAssetID != "" {
		t.Fatalf("settle rejected direct edit = %d %#v %s", w.Code, directOperation, w.Body.String())
	}

	visualRework := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "visual_rework",
		IdempotencyKey: "visual-rework:" + variant.ID + ":r1:1080x1080:v1", Status: "running", Attempt: 1,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256("only correct the rejected safe-area findings"),
		InputSnapshot: json.RawMessage(`{"input_attachment_id":"` + rawAttachmentID + `","input_role":"rejected_direct_edit_output","acceptance_failures":["title_safe_area","table_safe_area"]}`),
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue
SET context = jsonb_set(context, '{direct_edit,visual_rework_budget}', '0'::jsonb)
WHERE id = $1
`, taskID); err != nil {
		t.Fatal(err)
	}
	w, _ = putOperation(visualRework)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not budgeted") {
		t.Fatalf("unbudgeted visual rework = %d %s", w.Code, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue
SET context = jsonb_set(context, '{direct_edit,visual_rework_budget}', '1'::jsonb)
WHERE id = $1
`, taskID); err != nil {
		t.Fatal(err)
	}
	w, reworkOperation := putOperation(visualRework)
	if w.Code != http.StatusOK || reworkOperation.Disposition != "invoke" || reworkOperation.OperationKind != "visual_rework" {
		t.Fatalf("start budgeted visual rework = %d %#v %s", w.Code, reworkOperation, w.Body.String())
	}
	duplicate := visualRework
	duplicate.IdempotencyKey = "visual-rework-duplicate:" + variant.ID
	w, recovered := putOperation(duplicate)
	if w.Code != http.StatusOK || recovered.ID != reworkOperation.ID || recovered.Disposition != "reconcile" {
		t.Fatalf("duplicate visual rework = %d %#v %s", w.Code, recovered, w.Body.String())
	}
}

func TestCreativeProductionVisualReworkUsesBoundedQCReworkScope(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "production visual rework scope")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "production-visual-rework", []byte(`{}`))
	taskContext := fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_production",
  "creative_order_id":%q,"creative_order_item_id":%q,
  "variant_id":%q,"revision":1,
  "expected_sizes":["1080x1080","1200x628","800x1000"],
  "qc_visual_rework":{"target_sizes":["1080x1080","1200x628"]}
}`, orderID, itemID, variant.ID)
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_item_production', $2::uuid, $3::jsonb)
RETURNING id::text
`, agentID, itemID, taskContext).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	putOperation := func(sizeKey string) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", creativeImageOperationInput{
			VariantID: variant.ID, SizeKey: sizeKey, Revision: 1, OperationKind: "visual_rework",
			IdempotencyKey: "production-visual-rework:" + variant.ID + ":r1:" + sizeKey, Status: "running", Attempt: 1,
			Model: "gpt-image-2", PromptSHA256: creativePromptSHA256("correct only the rejected Prime readability findings"),
			InputSnapshot: json.RawMessage(`{"input_asset_sha256":"source","input_role":"generated_body"}`),
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		var response creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return w, response
	}
	w, operation := putOperation("1080x1080")
	if w.Code != http.StatusOK || operation.Disposition != "invoke" || operation.OperationKind != "visual_rework" {
		t.Fatalf("start bounded production visual rework = %d %#v %s", w.Code, operation, w.Body.String())
	}
	w, _ = putOperation("800x1000")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "outside the task's current execution scope") {
		t.Fatalf("out-of-scope production visual rework = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeImageOperationCancellationFencesOrdinaryMutation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "creative image operation cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-image-operation-cancel-fence", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running', 'creative_order_item_production', $2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
          'creative_order_id',$3::text,'creative_order_item_id',$2::text,
          'variant_id',$4::text,'revision',1,
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}

	putOperation := func(input creativeImageOperationInput) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		return w
	}
	base := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "generation",
		IdempotencyKey: "cancel-fence:" + taskID + ":1080x1080", Status: "running", Attempt: 1,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256("cancel fence prompt"),
		InputSnapshot: json.RawMessage(`{"input_asset_sha256":"cancel-fence"}`),
	}
	w := putOperation(base)
	if w.Code != http.StatusOK {
		t.Fatalf("start image operation = %d %s", w.Code, w.Body.String())
	}
	var started creativeImageOperationResponse
	if err := json.NewDecoder(w.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}

	outputAttachmentID := createCreativeOrderAssetAttachment(t, "cancel-fence-provider-output.png")
	completed := base
	completed.Status = "completed"
	completed.ProviderRequestID = "cancel-fence-provider-request"
	completed.ProviderStatus = "succeeded"
	completed.ResultReceipt = json.RawMessage(`{"provider_status":"succeeded"}`)
	completed.OutputAttachmentID = outputAttachmentID

	// Hold an uncommitted terminal transition. A plain SELECT would still see
	// the previously committed running status and let the callback write. The
	// handler must wait on this task row and then reject the terminal task.
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE agent_task_queue
SET status = 'cancelled', completed_at = now()
WHERE id = $1
`, taskID); err != nil {
		t.Fatal(err)
	}
	completedResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		completedResult <- putOperation(completed)
	}()
	select {
	case result := <-completedResult:
		t.Fatalf("image operation mutation bypassed the task cancellation lock: %d %s", result.Code, result.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case w = <-completedResult:
	case <-time.After(5 * time.Second):
		t.Fatal("image operation mutation did not resume after task cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "no longer active") {
		t.Fatalf("terminal task completed image operation = %d %s", w.Code, w.Body.String())
	}

	newOperation := base
	newOperation.SizeKey = "1200x628"
	newOperation.IdempotencyKey = "cancel-fence:" + taskID + ":1200x628"
	w = putOperation(newOperation)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "no longer active") {
		t.Fatalf("terminal task created image operation = %d %s", w.Code, w.Body.String())
	}

	var operationStatus, attemptStatus string
	var outputPersisted bool
	var newOperationCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT operation.status, attempt.status, operation.output_attachment_id IS NOT NULL,
       (SELECT count(*) FROM creative_image_operation
        WHERE variant_id = $2 AND revision = 1 AND size_key = '1200x628')
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt
  ON attempt.operation_id = operation.id AND attempt.attempt = 1
WHERE operation.id = $1
`, started.ID, variant.ID).Scan(&operationStatus, &attemptStatus, &outputPersisted, &newOperationCount); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "running" || attemptStatus != "running" || outputPersisted || newOperationCount != 0 {
		t.Fatalf("task cancellation fence persisted mutation: operation=%q attempt=%q output=%t new=%d",
			operationStatus, attemptStatus, outputPersisted, newOperationCount)
	}
}

func TestCreativeImageOperationDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "image operation order cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-image-operation-order-cancel-fence", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running','creative_order_item_production',$2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
          'creative_order_id',$3::text,'creative_order_item_id',$2::text,
          'variant_id',$4::text,'revision',1,
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	putOperation := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", creativeImageOperationInput{
			VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "generation",
			IdempotencyKey: "order-cancel-fence:" + taskID, Status: "running", Attempt: 1,
			Model: "gpt-image-2", PromptSHA256: creativePromptSHA256("order cancellation fence prompt"),
			InputSnapshot: json.RawMessage(`{"input_asset_sha256":"order-cancel-fence"}`),
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		return w
	}

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- putOperation() }()
	select {
	case w := <-result:
		t.Fatalf("image operation bypassed the order cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("image operation did not resume after order cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "creative order is cancelled") {
		t.Fatalf("cancelled order accepted image operation = %d %s", w.Code, w.Body.String())
	}
	var operationCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_image_operation WHERE variant_id = $1
`, variant.ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 0 {
		t.Fatalf("cancelled order persisted %d image operations", operationCount)
	}
}

func TestCreativeQCFinalizationDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "QC finalization order cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-cancel-fence-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'visual', 1, 1, 'passed', '{}'::jsonb)
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	agentID := createHandlerTestAgent(t, "creative-qc-order-cancel-fence", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running',
        'creative_order_variant_qc', $2, $3::jsonb)
RETURNING id::text
`, agentID, variant.ID, creativeQCTaskContextForTest(t, orderID, variant.ID, "visual")).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })

	finalize := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/qc-finalize", creativeOrderQCFinalizeInput{
			VariantID: variant.ID,
			Revision:  1,
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.FinalizeCreativeOrderQC(w, req)
		return w
	}

	// Cancellation owns the order lock before touching variants. Finalization
	// may authenticate against its task, but must wait for this boundary and
	// re-check the order state before it copies delivery assets or activates.
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- finalize() }()
	select {
	case w := <-result:
		t.Fatalf("QC finalization bypassed the order cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("QC finalization did not resume after order cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), errCreativeOrderCancelled.Error()) {
		t.Fatalf("cancelled order accepted QC finalization = %d %s", w.Code, w.Body.String())
	}

	var resolutionCount, deliveredCount, activeRevision int
	if err := testPool.QueryRow(t.Context(), `
SELECT
  (SELECT count(*) FROM creative_order_variant_qc_resolution WHERE variant_id = $1),
  (SELECT count(*) FROM creative_order_asset WHERE variant_id = $1 AND stage = 'delivered'),
  COALESCE((SELECT active_revision FROM creative_order_variant WHERE id = $1), 0)
`, variant.ID).Scan(&resolutionCount, &deliveredCount, &activeRevision); err != nil {
		t.Fatal(err)
	}
	if resolutionCount != 0 || deliveredCount != 0 || activeRevision != 0 {
		t.Fatalf("cancelled QC finalization persisted state: resolutions=%d delivered=%d active=%d",
			resolutionCount, deliveredCount, activeRevision)
	}

	activateTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer activateTx.Rollback(t.Context())
	if err := activateCreativeVariantRevision(t.Context(), activateTx, parseUUID(variant.ID), 1, standardCreativeAssetSizes); !errors.Is(err, errCreativeOrderCancelled) {
		t.Fatalf("cancelled order activation error = %v, want %v", err, errCreativeOrderCancelled)
	}
}

func TestCreativeGeneratedAssetRegistrationDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "generated asset cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-generated-asset-cancel-fence", []byte(`{}`))
	var taskID, runtimeID string
	if err := testPool.QueryRow(t.Context(), `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,$2,'running','creative_order_item_production',$3::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
          'creative_order_id',$4::text,'creative_order_item_id',$3::text,
          'variant_id',$5::text,'revision',1,
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, runtimeID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	prompt, providerRequestID := "generated asset cancellation prompt", "generated-asset-cancel-provider"
	operationID := createCompletedCreativeImageOperation(t, variant.ID, "1080x1080", prompt, providerRequestID)
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation SET runtime_id = $2, task_id = $3 WHERE id = $1
`, operationID, runtimeID, taskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation_attempt SET runtime_id = $2, task_id = $3 WHERE operation_id = $1
`, operationID, runtimeID, taskID); err != nil {
		t.Fatal(err)
	}
	attachmentID := createCreativeOrderAssetAttachment(t, "generated-asset-cancel-fence.png")
	metadata, evidence := completedGeneratedAssetTrace(prompt, providerRequestID, 1)
	putAsset := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
			AttachmentID: attachmentID, OperationID: operationID, Metadata: metadata, Evidence: evidence,
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderAsset(w, req)
		return w
	}

	// CancelCreativeOrder locks and closes the order before it settles task
	// rows. Reproduce that transaction boundary: asset-put may authenticate
	// against the still-running task, but must wait for the order and reject
	// the canonical write after cancellation commits.
	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan *httptest.ResponseRecorder, 1)
	go func() { result <- putAsset() }()
	select {
	case w := <-result:
		t.Fatalf("asset registration bypassed the order cancellation lock: %d %s", w.Code, w.Body.String())
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("asset registration did not resume after order cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "creative order is cancelled") {
		t.Fatalf("cancelled order accepted generated asset = %d %s", w.Code, w.Body.String())
	}

	var assetCount int
	var operationLinked bool
	if err := testPool.QueryRow(t.Context(), `
SELECT count(asset.id), operation.output_asset_id IS NOT NULL
FROM creative_image_operation operation
LEFT JOIN creative_order_asset asset
  ON asset.operation_id = operation.id AND asset.stage = 'generated'
WHERE operation.id = $1
GROUP BY operation.output_asset_id
`, operationID).Scan(&assetCount, &operationLinked); err != nil {
		t.Fatal(err)
	}
	if assetCount != 0 || operationLinked {
		t.Fatalf("cancelled asset registration persisted canonical state: assets=%d operation_linked=%t", assetCount, operationLinked)
	}
}

func TestCreativeProductionCompletionQueuesDurablePrimeRecovery(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "durable Prime recovery")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-prime-recovery", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id,
  context, completed_at
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed',
        'creative_order_item_production', $2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
          'creative_order_id',$3::uuid::text,'creative_order_item_id',$2::uuid::text,
          'variant_id',$4::uuid::text,'revision',1,'item_key',$4::uuid::text || ':r1',
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ), now())
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "prime-recovery-generated-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
		for _, label := range creativeProductionProcessLabels {
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1, $2, $3, 1, 'creative_production', $4, $5, '{}'::jsonb)
`, variant.ID, attachmentID, size, label, label+"-"+size+".png"); err != nil {
				t.Fatal(err)
			}
		}
	}
	task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), task); err != nil {
		t.Fatalf("replayed production settlement: %v", err)
	}

	var variantStatus, revisionStatus, jobStatus string
	var pendingMarker bool
	var jobCount, jobAttempt, childCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status,
       variant.brief ? 'prime_composition_pending',
       count(job.*), COALESCE(max(job.status), ''), COALESCE(max(job.attempt), 0),
       (SELECT count(*) FROM agent_task_queue child WHERE child.parent_task_id = $2)
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
LEFT JOIN creative_prime_composition_job job
  ON job.variant_id = variant.id AND job.revision = variant.revision
WHERE variant.id = $1
GROUP BY variant.status, revision.status, variant.brief
`, variant.ID, taskID).Scan(
		&variantStatus, &revisionStatus, &pendingMarker,
		&jobCount, &jobStatus, &jobAttempt, &childCount,
	); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "partial" || revisionStatus != "partial" || !pendingMarker ||
		jobCount != 1 || jobStatus != "queued" || jobAttempt != 0 || childCount != 0 {
		t.Fatalf("pending Prime recovery = variant %q revision %q marker %t job %d/%q/%d children %d",
			variantStatus, revisionStatus, pendingMarker, jobCount, jobStatus, jobAttempt, childCount)
	}
	derivedStatus, err := testHandler.derivedCreativeOrderStatusWithFailures(
		newRequest(http.MethodGet, "/", nil), parseUUID(orderID), false,
	)
	if err != nil || derivedStatus != "partial" {
		t.Fatalf("pending Prime order status = %q err %v, want partial", derivedStatus, err)
	}

	firstClaim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1)
	if err != nil || !found || uuidToString(firstClaim.VariantID) != variant.ID || firstClaim.Revision != 1 {
		t.Fatalf("first Prime recovery claim = %#v found %t err %v", firstClaim, found, err)
	}
	if _, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1); err != nil || found {
		t.Fatalf("unexpired Prime lease was reclaimed: found %t err %v", found, err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_prime_composition_job
SET lease_expires_at = now() - interval '1 second'
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	secondClaim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1)
	if err != nil || !found || secondClaim.LeaseToken == firstClaim.LeaseToken {
		t.Fatalf("expired Prime recovery claim = %#v found %t err %v", secondClaim, found, err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT attempt FROM creative_prime_composition_job WHERE variant_id = $1 AND revision = 1
`, variant.ID).Scan(&jobAttempt); err != nil {
		t.Fatal(err)
	}
	if jobAttempt != 2 {
		t.Fatalf("recovered Prime job attempt = %d, want 2", jobAttempt)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue SET status = 'cancelled', completed_at = now() WHERE id = $1
`, taskID); err != nil {
		t.Fatal(err)
	}
	cancelledTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.reconcileCreativeLifecycleForCancelledTask(t.Context(), cancelledTask); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.finishCreativePrimeComposition(t.Context(), secondClaim, nil); err == nil {
		t.Fatal("cancelled Prime lease accepted a late completion")
	}
	if _, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1); err != nil || found {
		t.Fatalf("cancelled Prime job remained claimable: found %t err %v", found, err)
	}
	var cancelledJobStatus, cancelledVariantStatus, cancelledRevisionStatus string
	var pendingAfterCancel bool
	if err := testPool.QueryRow(t.Context(), `
SELECT job.status, variant.status, revision.status,
       variant.brief ? 'prime_composition_pending'
FROM creative_prime_composition_job job
JOIN creative_order_variant variant ON variant.id = job.variant_id
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = job.revision
WHERE job.variant_id = $1 AND job.revision = 1
`, variant.ID).Scan(
		&cancelledJobStatus, &cancelledVariantStatus, &cancelledRevisionStatus, &pendingAfterCancel,
	); err != nil {
		t.Fatal(err)
	}
	if cancelledJobStatus != "cancelled" || cancelledVariantStatus != "action_required" ||
		cancelledRevisionStatus != "action_required" || pendingAfterCancel {
		t.Fatalf("cancelled Prime lifecycle = job %q variant %q revision %q pending %t",
			cancelledJobStatus, cancelledVariantStatus, cancelledRevisionStatus, pendingAfterCancel)
	}
}

func TestCreativeProductionTaskCannotForgePlatformPrimeOutputs(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "platform-owned Prime outputs")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "creative-prime-ownership", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'running',
        'creative_order_item_production', $2::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
          'creative_order_id',$3::uuid::text,'creative_order_item_id',$2::uuid::text,
          'variant_id',$4::uuid::text,'revision',1,'item_key',$4::uuid::text || ':r1',
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}

	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "forged-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		assetWriter := httptest.NewRecorder()
		assetRequest := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variant.ID, SizeKey: size, Revision: 1, Stage: "primed", Status: "completed",
			AttachmentID: attachmentID, Metadata: json.RawMessage(`{}`), Evidence: json.RawMessage(`{}`),
		})
		assetRequest = withURLParam(assetRequest, "id", orderID)
		assetRequest.Header.Set("X-Actor-Source", "task_token")
		assetRequest.Header.Set("X-Agent-ID", agentID)
		assetRequest.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderAsset(assetWriter, assetRequest)
		if assetWriter.Code != http.StatusForbidden || !strings.Contains(assetWriter.Body.String(), "registered by the platform") {
			t.Fatalf("task forged primed %s = %d %s", size, assetWriter.Code, assetWriter.Body.String())
		}

		diagnosticWriter := httptest.NewRecorder()
		diagnosticRequest := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/diagnostic-assets", creativeOrderDiagnosticAssetInput{
			VariantID: variant.ID, TaskID: taskID, AttachmentID: attachmentID,
			SizeKey: size, Revision: 1, Workflow: "brand_components", Label: "Prime 合成成图",
			Filename: "forged-prime-" + strings.ReplaceAll(size, "x", "-") + ".png",
		})
		diagnosticRequest = withURLParam(diagnosticRequest, "id", orderID)
		diagnosticRequest.Header.Set("X-Actor-Source", "task_token")
		diagnosticRequest.Header.Set("X-Agent-ID", agentID)
		diagnosticRequest.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderDiagnosticAsset(diagnosticWriter, diagnosticRequest)
		if diagnosticWriter.Code != http.StatusForbidden || !strings.Contains(diagnosticWriter.Body.String(), "registered by the platform") {
			t.Fatalf("task forged Prime evidence %s = %d %s", size, diagnosticWriter.Code, diagnosticWriter.Body.String())
		}

		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
		for _, label := range creativeProductionProcessLabels {
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1, $2, $3, $4, 1, 'creative_production', $5, $6, '{}'::jsonb)
`, variant.ID, taskID, attachmentID, size, label, label+"-"+size+".png"); err != nil {
				t.Fatal(err)
			}
		}
	}
	var primedCount, primeEvidenceCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT
  (SELECT count(*) FROM creative_order_asset
   WHERE variant_id = $1 AND revision = 1 AND stage = 'primed'),
  (SELECT count(*) FROM creative_order_diagnostic_asset
   WHERE variant_id = $1 AND revision = 1 AND workflow = 'brand_components')
`, variant.ID).Scan(&primedCount, &primeEvidenceCount); err != nil {
		t.Fatal(err)
	}
	if primedCount != 0 || primeEvidenceCount != 0 {
		t.Fatalf("forged Prime state persisted: primed=%d evidence=%d", primedCount, primeEvidenceCount)
	}

	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue SET status = 'completed', completed_at = now() WHERE id = $1
`, taskID); err != nil {
		t.Fatal(err)
	}
	task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	var jobStatus string
	var pendingMarker bool
	if err := testPool.QueryRow(t.Context(), `
SELECT job.status, variant.brief ? 'prime_composition_pending'
FROM creative_prime_composition_job job
JOIN creative_order_variant variant ON variant.id = job.variant_id
WHERE job.variant_id = $1 AND job.revision = 1
`, variant.ID).Scan(&jobStatus, &pendingMarker); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "queued" || !pendingMarker {
		t.Fatalf("platform Prime continuation after forgery = status %q pending %t", jobStatus, pendingMarker)
	}
}

func TestCreativePrimeHTTPUsesSingleDurableLease(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "durable Prime HTTP handoff")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "prime-http-generated-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	compose := func(force bool) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/prime-compose", creativePrimeComposeRequest{
			VariantID: variant.ID, Async: true, Force: force,
		})
		req = withURLParam(req, "id", orderID)
		testHandler.ComposeCreativeOrderPrime(w, req)
		return w
	}
	for attempt := 0; attempt < 2; attempt++ {
		w := compose(false)
		if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), "composition_queued") {
			t.Fatalf("durable Prime enqueue %d = %d %s", attempt+1, w.Code, w.Body.String())
		}
	}
	var jobCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_prime_composition_job WHERE variant_id = $1 AND revision = 1
`, variant.ID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Fatalf("Prime HTTP created %d durable jobs, want 1", jobCount)
	}
	claim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1)
	if err != nil || !found {
		t.Fatalf("claim durable Prime HTTP job = found %t err %v", found, err)
	}
	w := compose(true)
	if w.Code != http.StatusAccepted || !strings.Contains(w.Body.String(), "composition_queued") {
		t.Fatalf("force during leased Prime job = %d %s", w.Code, w.Body.String())
	}
	var retainedLease string
	if err := testPool.QueryRow(t.Context(), `
SELECT lease_token::text FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = 1 AND status = 'running'
`, variant.ID).Scan(&retainedLease); err != nil {
		t.Fatal(err)
	}
	if retainedLease != uuidToString(claim.LeaseToken) {
		t.Fatalf("force request stole Prime lease %q, want %q", retainedLease, uuidToString(claim.LeaseToken))
	}
	if _, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 1); err != nil || found {
		t.Fatalf("second server claimed active Prime lease: found %t err %v", found, err)
	}
}

func TestCreativePrimeQueueDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "Prime queue cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "prime-queue-cancel-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	type queueResult struct {
		status string
		err    error
	}
	result := make(chan queueResult, 1)
	go func() {
		_, status, _, queueErr := testHandler.queueCreativePrimeComposition(
			t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), false,
		)
		result <- queueResult{status: status, err: queueErr}
	}()
	select {
	case queued := <-result:
		t.Fatalf("Prime queue bypassed the order cancellation lock: status %q err %v", queued.status, queued.err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var queued queueResult
	select {
	case queued = <-result:
	case <-time.After(5 * time.Second):
		t.Fatal("Prime queue did not resume after cancellation committed")
	}
	var cancelledErr *creativePrimeCancelledError
	if queued.status != "cancelled" || !errors.As(queued.err, &cancelledErr) {
		t.Fatalf("cancelled Prime queue = status %q err %v", queued.status, queued.err)
	}
	var jobCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM creative_prime_composition_job WHERE variant_id = $1
`, variant.ID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 0 {
		t.Fatalf("cancelled Prime queue persisted %d jobs", jobCount)
	}
}

func TestCreativePrimeComposeTaskTokenRequiresActiveExactScopeAndCannotForce(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "Prime task authorization fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "prime-task-fence-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	agentID := createHandlerTestAgent(t, "creative-prime-task-fence", []byte(`{}`))
	createTask := func(status, contextOrderID, contextVariantID string, revision int) string {
		t.Helper()
		contextValue, err := json.Marshal(map[string]any{
			"type": "creative_domain_task", "workflow": "creative_production",
			"creative_order_id": contextOrderID, "variant_id": contextVariantID, "revision": revision,
		})
		if err != nil {
			t.Fatal(err)
		}
		var taskID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, $3::jsonb)
RETURNING id::text
`, agentID, status, contextValue).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE id = $1`, taskID) })
		return taskID
	}
	compose := func(taskID string, force bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/prime-compose", creativePrimeComposeRequest{
			VariantID: variant.ID,
			Async:     true,
			Force:     force,
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.ComposeCreativeOrderPrime(w, req)
		return w
	}

	cancelledTaskID := createTask("cancelled", orderID, variant.ID, 1)
	if w := compose(cancelledTaskID, false); w.Code != http.StatusForbidden {
		t.Fatalf("cancelled task Prime compose = %d %s", w.Code, w.Body.String())
	}
	crossOrderTaskID := createTask("running", uuid.NewString(), variant.ID, 1)
	if w := compose(crossOrderTaskID, false); w.Code != http.StatusForbidden {
		t.Fatalf("cross-order task Prime compose = %d %s", w.Code, w.Body.String())
	}
	crossVariantTaskID := createTask("running", orderID, uuid.NewString(), 1)
	if w := compose(crossVariantTaskID, false); w.Code != http.StatusForbidden {
		t.Fatalf("cross-variant task Prime compose = %d %s", w.Code, w.Body.String())
	}
	matchingTaskID := createTask("running", orderID, variant.ID, 1)
	if w := compose(matchingTaskID, true); w.Code != http.StatusForbidden {
		t.Fatalf("task force Prime compose = %d %s", w.Code, w.Body.String())
	}
	if w := compose(matchingTaskID, false); w.Code != http.StatusAccepted {
		t.Fatalf("authorized task Prime compose = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativePrimeJobRequeuesForExpandedContractAndRepairedInput(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "Prime contract generation")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "prime-generation-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
`, variant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	primaryAssets, complete, err := testHandler.loadCreativePrimeGeneratedAssets(
		t.Context(), parseUUID(variant.ID), 1, []string{"1080x1080"},
	)
	if err != nil || !complete {
		t.Fatalf("load candidate primary input = complete %t err %v", complete, err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, expected_sizes, input_fingerprint, composed_at, completed_at
) VALUES ($1, 1, 'completed', ARRAY['1080x1080']::text[], $2, now(), now())
`, variant.ID, creativePrimeGeneratedFingerprint(primaryAssets)); err != nil {
		t.Fatal(err)
	}

	revision, status, generatedComplete, err := testHandler.queueCreativePrimeComposition(
		t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), false,
	)
	if err != nil || revision != 1 || !generatedComplete || status != "queued" {
		t.Fatalf("expanded Prime contract enqueue = r%d/%q complete %t err %v", revision, status, generatedComplete, err)
	}
	fullAssets, complete, err := testHandler.loadCreativePrimeGeneratedAssets(
		t.Context(), parseUUID(variant.ID), 1, standardCreativeAssetSizes,
	)
	if err != nil || !complete {
		t.Fatalf("load expanded Prime inputs = complete %t err %v", complete, err)
	}
	var expectedSizes []string
	var fingerprint string
	if err := testPool.QueryRow(t.Context(), `
SELECT expected_sizes, input_fingerprint
FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = 1
`, variant.ID).Scan(&expectedSizes, &fingerprint); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(expectedSizes, standardCreativeAssetSizes) || fingerprint != creativePrimeGeneratedFingerprint(fullAssets) {
		t.Fatalf("expanded Prime job contract = sizes %v fingerprint %q", expectedSizes, fingerprint)
	}

	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_prime_composition_job
SET status = 'failed', last_error = 'contrast rejected', completed_at = now()
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	_, status, _, err = testHandler.queueCreativePrimeComposition(
		t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), false,
	)
	if err != nil || status != "failed" {
		t.Fatalf("unchanged failed Prime input enqueue = %q err %v, want failed", status, err)
	}
	_, status, _, err = testHandler.queueCreativePrimeComposition(
		t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), true,
	)
	if err != nil || status != "queued" {
		t.Fatalf("explicit unchanged Prime retry = %q err %v, want queued", status, err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_prime_composition_job
SET status = 'failed', last_error = 'contrast rejected again', completed_at = now()
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}

	repairedAttachmentID := createCreativeOrderAssetAttachment(t, "prime-generation-repaired-wide.png")
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_asset
SET attachment_id = $2, updated_at = now()
WHERE variant_id = $1 AND revision = 1 AND stage = 'generated' AND size_key = '1200x628'
`, variant.ID, repairedAttachmentID); err != nil {
		t.Fatal(err)
	}
	_, status, _, err = testHandler.queueCreativePrimeComposition(
		t.Context(), parseUUID(testWorkspaceID), parseUUID(orderID), parseUUID(variant.ID), false,
	)
	if err != nil || status != "queued" {
		t.Fatalf("repaired failed Prime input enqueue = %q err %v, want queued", status, err)
	}
}

func TestCreativeOrderCancellationFencesAllPrimeRevisions(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "cancel all Prime revisions")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	_ = putCreativeLifecycleVariant(t, orderID, itemID, "C01", 2, "running")
	leaseToken := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, attempt, lease_token, lease_expires_at
) VALUES
  ($1, 1, 'failed', 1, NULL, NULL),
  ($1, 2, 'running', 1, $2, now() + interval '10 minutes')
`, variant.ID, leaseToken); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/cancel", nil), "id", orderID)
	testHandler.CancelCreativeOrder(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel order with Prime jobs = %d %s", w.Code, w.Body.String())
	}
	rows, err := testPool.Query(t.Context(), `
SELECT revision, status, lease_token IS NULL, lease_expires_at IS NULL
FROM creative_prime_composition_job
WHERE variant_id = $1
ORDER BY revision
`, variant.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var revision int
		var status string
		var leaseTokenCleared, leaseExpiryCleared bool
		if err := rows.Scan(&revision, &status, &leaseTokenCleared, &leaseExpiryCleared); err != nil {
			t.Fatal(err)
		}
		if revision != seen+1 || status != "cancelled" || !leaseTokenCleared || !leaseExpiryCleared {
			t.Fatalf("cancelled Prime r%d = %q lease cleared %t/%t", revision, status, leaseTokenCleared, leaseExpiryCleared)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 2 {
		t.Fatalf("cancelled Prime revision count = %d, want 2", seen)
	}
	lateClaim := creativePrimeCompositionClaim{
		WorkspaceID: parseUUID(testWorkspaceID), OrderID: parseUUID(orderID), OrderItemID: parseUUID(itemID),
		VariantID: parseUUID(variant.ID), Revision: 2, LeaseToken: parseUUID(leaseToken),
	}
	if err := testHandler.finishCreativePrimeComposition(t.Context(), lateClaim, nil); err != nil {
		t.Fatalf("late Prime completion did not converge on cancelled state: %v", err)
	}
	if _, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variant.ID), 2); err != nil || found {
		t.Fatalf("cancelled order Prime job remained claimable: found %t err %v", found, err)
	}
}

func TestCreativePrimeFinishDoesNotOutrunOrderCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "Prime finish cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	leaseToken := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, attempt, lease_token, lease_expires_at
) VALUES ($1, 1, 'running', 1, $2, now() + interval '10 minutes')
`, variant.ID, leaseToken); err != nil {
		t.Fatal(err)
	}

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant_revision
SET status = 'cancelled', updated_at = now()
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_prime_composition_job
SET status = 'cancelled', lease_token = NULL, lease_expires_at = NULL,
    completed_at = now(), updated_at = now()
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}

	finishResult := make(chan error, 1)
	go func() {
		finishResult <- testHandler.finishCreativePrimeComposition(t.Context(), creativePrimeCompositionClaim{
			WorkspaceID: parseUUID(testWorkspaceID),
			OrderID:     parseUUID(orderID),
			OrderItemID: parseUUID(itemID),
			VariantID:   parseUUID(variant.ID),
			Revision:    1,
			LeaseToken:  parseUUID(leaseToken),
		}, errors.New("late Prime failure"))
	}()
	select {
	case finishErr := <-finishResult:
		t.Fatalf("Prime finish bypassed the order cancellation lock: %v", finishErr)
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case finishErr := <-finishResult:
		if finishErr != nil {
			t.Fatalf("late Prime finish after cancellation: %v", finishErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Prime finish did not resume after cancellation committed")
	}

	var orderStatus, variantStatus, revisionStatus, jobStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT creative_order.status, variant.status, revision.status, job.status
FROM creative_order
JOIN creative_order_item item ON item.order_id = creative_order.id
JOIN creative_order_variant variant ON variant.order_item_id = item.id
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = 1
JOIN creative_prime_composition_job job
  ON job.variant_id = variant.id AND job.revision = revision.revision
WHERE creative_order.id = $1 AND variant.id = $2
`, orderID, variant.ID).Scan(&orderStatus, &variantStatus, &revisionStatus, &jobStatus); err != nil {
		t.Fatal(err)
	}
	if orderStatus != "cancelled" || variantStatus != "cancelled" ||
		revisionStatus != "cancelled" || jobStatus != "cancelled" {
		t.Fatalf("late Prime finish revived lifecycle: order=%q variant=%q revision=%q job=%q",
			orderStatus, variantStatus, revisionStatus, jobStatus)
	}
}

func TestCreativeImageOperationTaskTokenBindsRuntime(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "task-bound image operation runtime")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "task-bound-image-runtime", []byte(`{}`))
	var taskID, runtimeID string
	if err := testPool.QueryRow(t.Context(), `SELECT runtime_id::text FROM agent WHERE id = $1`, agentID).Scan(&runtimeID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, $2, 'running', 'creative_order_item_production', $3::uuid,
        jsonb_build_object(
          'type','creative_domain_task','workflow','creative_production',
		  'creative_order_id',$4::uuid::text,'creative_order_item_id',$3::uuid::text,
		  'variant_id',$5::uuid::text,'revision',1,
          'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
        ))
RETURNING id::text
`, agentID, runtimeID, itemID, orderID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}

	putOperation := func(input creativeImageOperationInput) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		var response creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return w, response
	}
	base := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "generation",
		IdempotencyKey: "task-runtime:" + taskID + ":1080x1080", Status: "running", Attempt: 1,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256("task runtime prompt"),
		InputSnapshot: json.RawMessage(`{"input_asset_sha256":"task-runtime"}`),
	}
	w, operation := putOperation(base)
	if w.Code != http.StatusOK || operation.RuntimeID != runtimeID || operation.TaskID != taskID ||
		len(operation.Attempts) != 1 || operation.Attempts[0].RuntimeID != runtimeID || operation.Disposition != "invoke" {
		t.Fatalf("task-bound operation = %d %#v %s", w.Code, operation, w.Body.String())
	}

	spoofed := base
	spoofed.SizeKey = "1200x628"
	spoofed.IdempotencyKey = "task-runtime:" + taskID + ":1200x628"
	spoofed.RuntimeID = uuid.NewString()
	w, _ = putOperation(spoofed)
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "does not match") {
		t.Fatalf("spoofed task runtime = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeImageOperationAndAssetUseNarrowTaskSizeScope(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "narrow image task size scope")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "narrow-image-task-scope", []byte(`{}`))
	var taskID string
	baseContext := func(workflow string) map[string]any {
		return map[string]any{
			"type": "creative_domain_task", "workflow": workflow,
			"creative_order_id": orderID, "creative_order_item_id": itemID,
			"variant_id": variant.ID, "revision": 1,
			"expected_sizes": standardCreativeAssetSizes,
		}
	}
	initialContext, err := json.Marshal(baseContext("creative_production"))
	if err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running','creative_order_item_production',$2::uuid,$3::jsonb)
RETURNING id::text
`, agentID, itemID, initialContext).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	setTaskContext := func(contextValue map[string]any) {
		t.Helper()
		encoded, err := json.Marshal(contextValue)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context = $2::jsonb WHERE id = $1`, taskID, encoded); err != nil {
			t.Fatal(err)
		}
	}
	putOperation := func(input creativeImageOperationInput) (*httptest.ResponseRecorder, creativeImageOperationResponse) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		var operation creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&operation); err != nil {
				t.Fatal(err)
			}
		}
		return w, operation
	}
	prompt := "narrow task size scope prompt"
	base := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "generation",
		IdempotencyKey: "narrow-task-scope:" + taskID, Status: "running", Attempt: 1,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256(prompt),
		InputSnapshot: json.RawMessage(`{"input_asset_sha256":"narrow-task-scope"}`),
	}
	narrowContexts := []map[string]any{}
	missing := baseContext("creative_production")
	missing["missing_sizes"] = []string{"1200x628"}
	narrowContexts = append(narrowContexts, missing)
	visualRework := baseContext("creative_production")
	visualRework["missing_sizes"] = []string{"1200x628"}
	visualRework["qc_visual_rework"] = map[string]any{"target_sizes": []string{"800x1000"}}
	narrowContexts = append(narrowContexts, visualRework)
	lateRecovery := baseContext("creative_production")
	lateRecovery["missing_sizes"] = []string{"800x1000"}
	lateRecovery["late_receipt_recoveries"] = []map[string]any{{"size_key": "1200x628"}}
	narrowContexts = append(narrowContexts, lateRecovery)
	directEdit := baseContext("creative_direct_edit")
	directEdit["target_size"] = "1200x628"
	directEdit["edit_sizes"] = []string{"800x1000"}
	narrowContexts = append(narrowContexts, directEdit)
	for index, contextValue := range narrowContexts {
		setTaskContext(contextValue)
		w, _ := putOperation(base)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "current execution scope") {
			t.Fatalf("narrow scope %d accepted excluded image size = %d %s", index, w.Code, w.Body.String())
		}
	}
	var operationCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_image_operation WHERE variant_id = $1`, variant.ID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 0 {
		t.Fatalf("excluded task sizes created %d image operations", operationCount)
	}

	allowedContext := baseContext("creative_production")
	allowedContext["missing_sizes"] = []string{"1080x1080"}
	setTaskContext(allowedContext)
	w, operation := putOperation(base)
	if w.Code != http.StatusOK || operation.ID == "" {
		t.Fatalf("allowed narrow image size = %d %#v %s", w.Code, operation, w.Body.String())
	}
	rawAttachmentID := createCreativeOrderAssetAttachment(t, "narrow-task-provider-output.png")
	completed := base
	completed.Status = "completed"
	completed.ProviderRequestID = "narrow-task-provider-request"
	completed.ProviderStatus = "succeeded"
	completed.ResultReceipt = json.RawMessage(`{"provider_status":"succeeded"}`)
	completed.OutputAttachmentID = rawAttachmentID
	w, operation = putOperation(completed)
	if w.Code != http.StatusOK || operation.Status != "completed" {
		t.Fatalf("complete allowed narrow image size = %d %#v %s", w.Code, operation, w.Body.String())
	}

	canonicalAttachmentID := createCreativeOrderAssetAttachment(t, "narrow-task-canonical-output.png")
	metadata, evidence := completedGeneratedAssetTrace(prompt, completed.ProviderRequestID, 1)
	putAsset := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/assets", creativeOrderAssetInput{
			VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, Stage: "generated", Status: "completed",
			AttachmentID: canonicalAttachmentID, OperationID: operation.ID, Metadata: metadata, Evidence: evidence,
		})
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeOrderAsset(w, req)
		return w
	}
	setTaskContext(visualRework)
	w = putAsset()
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "current execution scope") {
		t.Fatalf("asset-put accepted excluded task size = %d %s", w.Code, w.Body.String())
	}
	setTaskContext(allowedContext)
	w = putAsset()
	if w.Code != http.StatusOK {
		t.Fatalf("asset-put rejected allowed task size = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativePrimeFailureUpdatesStagingRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "creative Prime failure revision")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")

	testHandler.markCreativePrimeCompositionFailed(t.Context(), parseUUID(variant.ID), errors.New("Prime contrast gate failed"))

	var variantStatus, revisionStatus, variantBrief, revisionBrief string
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status, variant.brief::text, revision.brief::text
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variant.ID).Scan(&variantStatus, &revisionStatus, &variantBrief, &revisionBrief); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "action_required" || revisionStatus != "action_required" {
		t.Fatalf("Prime failure lifecycle = variant %q revision %q", variantStatus, revisionStatus)
	}
	for label, raw := range map[string]string{"variant": variantBrief, "revision": revisionBrief} {
		var brief map[string]any
		if err := json.Unmarshal([]byte(raw), &brief); err != nil {
			t.Fatalf("decode %s brief: %v", label, err)
		}
		if _, ok := brief["brand_composition_error"]; !ok {
			t.Fatalf("%s brief is missing brand_composition_error: %s", label, raw)
		}
	}
}

func TestCreativeQCHandoffFailureCannotReviveCancelledOrActiveRevision(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "QC handoff cancellation fence")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now() WHERE id = $1
`, orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant SET status = 'cancelled', updated_at = now() WHERE id = $1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE creative_order_variant_revision SET status = 'cancelled', updated_at = now()
WHERE variant_id = $1 AND revision = 1
`, variant.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		testHandler.markCreativeQCHandoffFailed(t.Context(), parseUUID(variant.ID), errors.New("reviewer unavailable"))
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("QC handoff failure bypassed the order cancellation lock")
	case <-time.After(150 * time.Millisecond):
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("QC handoff failure did not resume after cancellation committed")
	}
	var variantStatus, revisionStatus string
	var handoffErrorPresent bool
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status, variant.brief ? 'creative_qc_handoff_error'
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = 1
WHERE variant.id = $1
`, variant.ID).Scan(&variantStatus, &revisionStatus, &handoffErrorPresent); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "cancelled" || revisionStatus != "cancelled" || handoffErrorPresent {
		t.Fatalf("cancelled handoff lifecycle = variant %q revision %q error %t",
			variantStatus, revisionStatus, handoffErrorPresent)
	}

	activeOrderID, activeItemID, _ := createCreativeLifecycleTestOrder(t, "QC handoff active revision fence")
	activeVariant := putCreativeLifecycleVariant(t, activeOrderID, activeItemID, "C01", 1, "running")
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "qc-handoff-active-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'delivered', $3, 'completed')
`, activeVariant.ID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	activateTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := activateCreativeVariantRevision(t.Context(), activateTx, parseUUID(activeVariant.ID), 1, standardCreativeAssetSizes); err != nil {
		_ = activateTx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := activateTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	testHandler.markCreativeQCHandoffFailed(t.Context(), parseUUID(activeVariant.ID), errors.New("late reviewer failure"))
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status, variant.brief ? 'creative_qc_handoff_error'
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = 1
WHERE variant.id = $1
`, activeVariant.ID).Scan(&variantStatus, &revisionStatus, &handoffErrorPresent); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "completed" || revisionStatus != "completed" || handoffErrorPresent {
		t.Fatalf("active handoff lifecycle = variant %q revision %q error %t",
			variantStatus, revisionStatus, handoffErrorPresent)
	}
}

func TestCreativeTerminalTaskMovesUnsettledImageOperationToUnknown(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "terminal image operation reconciliation")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "terminal-image-operation", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context, completed_at
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', 'creative_order_item_production', $2,
        jsonb_build_object('type','creative_domain_task','workflow','creative_production','variant_id',$3::text,'revision',1), now())
RETURNING id::text
`, agentID, itemID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var operationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id
)
VALUES ($1, '1080x1080', 1, 'generation', $2, 'running', $3)
RETURNING id::text
`, variant.ID, "terminal-task:"+taskID, taskID).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (operation_id, attempt, status, task_id)
VALUES ($1, 1, 'running', $2)
`, operationID, taskID); err != nil {
		t.Fatal(err)
	}
	outputAttachmentID := createCreativeOrderAssetAttachment(t, "terminal-operation-output.png")
	var settledOperationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id, output_attachment_id
)
VALUES ($1, '1200x628', 1, 'generation', $2, 'running', $3, $4)
RETURNING id::text
`, variant.ID, "terminal-task-output:"+taskID, taskID, outputAttachmentID).Scan(&settledOperationID); err != nil {
		t.Fatal(err)
	}

	task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.reconcileCreativeImageOperationsForTerminalTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.reconcileCreativeImageOperationsForTerminalTask(t.Context(), task); err != nil {
		t.Fatalf("terminal image operation reconciliation is not idempotent: %v", err)
	}

	var operationStatus, attemptStatus, operationWithOutputStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT operation.status, attempt.status,
       (SELECT status FROM creative_image_operation WHERE id = $2)
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id AND attempt.attempt = 1
WHERE operation.id = $1
`, operationID, settledOperationID).Scan(&operationStatus, &attemptStatus, &operationWithOutputStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "unknown" || attemptStatus != "unknown" || operationWithOutputStatus != "running" {
		t.Fatalf("terminal reconciliation = operation %q attempt %q output operation %q", operationStatus, attemptStatus, operationWithOutputStatus)
	}
}

func TestCreativeContinuationAttemptTerminalReconciliationAllowsNextAttempt(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, _ := createCreativeLifecycleTestOrder(t, "continuation image operation reconciliation")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "continuation-image-operation", []byte(`{}`))
	taskContext := fmt.Sprintf(`{
  "type":"creative_domain_task","workflow":"creative_production",
  "creative_order_id":%q,"creative_order_item_id":%q,
  "variant_id":%q,"revision":1,
  "expected_sizes":["1080x1080","1200x628","800x1000"]
}`, orderID, itemID, variant.ID)
	var rootTaskID, childTaskID, nextTaskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context, completed_at
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'completed','creative_order_item_production',$2,$3::jsonb,now())
RETURNING id::text
`, agentID, itemID, taskContext).Scan(&rootTaskID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, parent_task_id, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running',$2,'creative_order_item_production',$3,$4::jsonb)
RETURNING id::text
`, agentID, rootTaskID, itemID, taskContext).Scan(&childTaskID); err != nil {
		t.Fatal(err)
	}
	var operationID string
	prompt := "continuation attempt reconciliation prompt"
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, model,
  runtime_id, task_id, prompt_sha256, input_snapshot, error_type
)
VALUES ($1,'1080x1080',1,'generation',$2,'failed','gpt-image-2',
        (SELECT runtime_id FROM agent_task_queue WHERE id = $3),$3,$4,'{"input_asset_sha256":"continuation"}'::jsonb,'provider_receipt_not_found')
RETURNING id::text
`, variant.ID, "continuation-attempt:"+rootTaskID, rootTaskID, creativePromptSHA256(prompt)).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (
  operation_id, attempt, status, runtime_id, task_id, error_type, completed_at
)
VALUES ($1,1,'failed',(SELECT runtime_id FROM agent_task_queue WHERE id = $2),$2,'provider_receipt_not_found',now())
`, operationID, rootTaskID); err != nil {
		t.Fatal(err)
	}
	putOperation := func(taskID string, input creativeImageOperationInput) (creativeImageOperationResponse, *httptest.ResponseRecorder) {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPut, "/api/creative/orders/"+orderID+"/image-operations", input)
		req = withURLParam(req, "id", orderID)
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		testHandler.UpsertCreativeImageOperation(w, req)
		var response creativeImageOperationResponse
		if w.Code == http.StatusOK {
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}
		}
		return response, w
	}
	base := creativeImageOperationInput{
		VariantID: variant.ID, SizeKey: "1080x1080", Revision: 1, OperationKind: "generation",
		IdempotencyKey: "continuation-attempt:" + rootTaskID, Status: "running", Attempt: 2,
		Model: "gpt-image-2", PromptSHA256: creativePromptSHA256(prompt),
		InputSnapshot: json.RawMessage(`{"input_asset_sha256":"continuation"}`),
	}
	response, w := putOperation(childTaskID, base)
	if w.Code != http.StatusOK || response.Disposition != "invoke" {
		t.Fatalf("continuation attempt 2 = %d disposition %q %s", w.Code, response.Disposition, w.Body.String())
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'completed', completed_at = now() WHERE id = $1`, childTaskID); err != nil {
		t.Fatal(err)
	}
	childTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(childTaskID))
	if err != nil {
		t.Fatal(err)
	}
	if err := testHandler.reconcileCreativeImageOperationsForTerminalTask(t.Context(), childTask); err != nil {
		t.Fatal(err)
	}
	var operationStatus, attemptOneStatus, attemptTwoStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT operation.status,
       (SELECT status FROM creative_image_operation_attempt WHERE operation_id = operation.id AND attempt = 1),
       (SELECT status FROM creative_image_operation_attempt WHERE operation_id = operation.id AND attempt = 2)
FROM creative_image_operation operation
WHERE operation.id = $1 AND operation.task_id = $2
`, operationID, rootTaskID).Scan(&operationStatus, &attemptOneStatus, &attemptTwoStatus); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "unknown" || attemptOneStatus != "failed" || attemptTwoStatus != "unknown" {
		t.Fatalf("continuation terminal reconciliation = operation %q attempts %q/%q", operationStatus, attemptOneStatus, attemptTwoStatus)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, status, parent_task_id, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1,(SELECT runtime_id FROM agent WHERE id = $1),'running',$2,'creative_order_item_production',$3,$4::jsonb)
RETURNING id::text
`, agentID, childTaskID, itemID, taskContext).Scan(&nextTaskID); err != nil {
		t.Fatal(err)
	}
	confirmedMissing := base
	confirmedMissing.Status = "failed"
	confirmedMissing.ReconcileConfirmed = true
	confirmedMissing.ErrorType = "provider_receipt_not_found"
	confirmedMissing.ErrorMessage = "provider confirmed no receipt"
	response, w = putOperation(nextTaskID, confirmedMissing)
	if w.Code != http.StatusOK || response.Disposition != "terminal" || response.Status != "failed" {
		t.Fatalf("confirmed missing attempt 2 = %d disposition/status %q/%q %s", w.Code, response.Disposition, response.Status, w.Body.String())
	}
	attemptThree := base
	attemptThree.Attempt = 3
	response, w = putOperation(nextTaskID, attemptThree)
	if w.Code != http.StatusOK || response.Disposition != "invoke" || response.TaskID != rootTaskID {
		t.Fatalf("continuation attempt 3 = %d disposition %q root task %q %s", w.Code, response.Disposition, response.TaskID, w.Body.String())
	}
	if len(response.Attempts) != 3 || response.Attempts[2].TaskID != nextTaskID || response.Attempts[2].Status != "running" {
		t.Fatalf("continuation attempt 3 ownership = %#v", response.Attempts)
	}
}

func TestCreativeSweeperFailuresRunTerminalReconciliation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, failureReason := range []string{"timeout", "runtime_offline"} {
		t.Run(failureReason, func(t *testing.T) {
			orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "creative sweeper "+failureReason)
			fixture := createCreativeOrderSquadFixture(t, "", "", true)
			inputSnapshot, err := json.Marshal(map[string]any{
				"pipeline_version": creativePipelineCandidateV1,
				"expected_sizes":   standardCreativeAssetSizes,
				"squad_snapshot": map[string]string{
					"squad_id":          fixture.SquadID,
					"leader_agent_id":   fixture.LeaderAgentID,
					"producer_agent_id": fixture.ProducerAgentID,
					"reviewer_agent_id": fixture.ReviewerAgentID,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot = $2::jsonb WHERE id = $1`, orderID, inputSnapshot); err != nil {
				t.Fatal(err)
			}
			variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
			taskContext, err := json.Marshal(map[string]any{
				"type":                   "creative_domain_task",
				"workflow":               "creative_production",
				"item_key":               variant.ID + ":r1",
				"creative_order_id":      orderID,
				"creative_order_item_id": itemID,
				"variant_id":             variant.ID,
				"revision":               1,
				"expected_sizes":         standardCreativeAssetSizes,
			})
			if err != nil {
				t.Fatal(err)
			}
			var taskID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, context, attempt, max_attempts,
  failure_reason, error, trigger_evidence_kind, trigger_evidence_ref_id, completed_at
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, 'failed', $3::jsonb, 3, 3,
        $4, $4, 'creative_order_item_production', $5, now())
RETURNING id::text
`, fixture.ProducerAgentID, issueID, taskContext, failureReason, itemID).Scan(&taskID); err != nil {
				t.Fatal(err)
			}
			var operationID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id
)
VALUES ($1, '1080x1080', 1, 'generation', $2, 'running', $3)
RETURNING id::text
`, variant.ID, "sweeper:"+failureReason+":"+taskID, taskID).Scan(&operationID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (operation_id, attempt, status, task_id)
VALUES ($1, 1, 'running', $2)
`, operationID, taskID); err != nil {
				t.Fatal(err)
			}
			task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
			if err != nil {
				t.Fatal(err)
			}
			if retried := testHandler.TaskService.HandleFailedTasks(t.Context(), []db.AgentTaskQueue{task}); retried != 0 {
				t.Fatalf("failed sweeper task retries = %d, want 0 at exhausted budget", retried)
			}
			var operationStatus, attemptStatus, variantStatus, revisionStatus string
			if err := testPool.QueryRow(t.Context(), `
SELECT operation.status, attempt.status, variant.status, revision.status
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id AND attempt.attempt = 1
JOIN creative_order_variant variant ON variant.id = operation.variant_id
JOIN creative_order_variant_revision revision ON revision.variant_id = variant.id AND revision.revision = operation.revision
WHERE operation.id = $1
`, operationID).Scan(&operationStatus, &attemptStatus, &variantStatus, &revisionStatus); err != nil {
				t.Fatal(err)
			}
			if operationStatus != "unknown" || attemptStatus != "unknown" || variantStatus != "action_required" || revisionStatus != "action_required" {
				t.Fatalf("%s reconciliation = operation %q attempt %q variant %q revision %q", failureReason, operationStatus, attemptStatus, variantStatus, revisionStatus)
			}
		})
	}
}

func TestCancelTaskReconcilesCreativeImageOperation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "cancel image operation reconciliation")
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "running")
	agentID := createHandlerTestAgent(t, "cancel-image-operation", []byte(`{}`))
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, 'running', 'creative_order_item_production', $3,
        jsonb_build_object('type','creative_domain_task','workflow','creative_production','variant_id',$4::text,'revision',1))
RETURNING id::text
`, agentID, issueID, itemID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	var operationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id
)
VALUES ($1, '1080x1080', 1, 'generation', $2, 'running', $3)
RETURNING id::text
`, variant.ID, "cancel-task:"+taskID, taskID).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (operation_id, attempt, status, task_id)
VALUES ($1, 1, 'running', $2)
`, operationID, taskID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/tasks/"+taskID+"/cancel", nil)
	req = withURLParams(req, "id", issueID, "taskId", taskID)
	testHandler.CancelTask(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("cancel creative image task = %d %s", w.Code, w.Body.String())
	}
	var taskStatus, operationStatus, attemptStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT task.status, operation.status, attempt.status
FROM agent_task_queue task
JOIN creative_image_operation operation ON operation.task_id = task.id
JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id
WHERE task.id = $1 AND operation.id = $2 AND attempt.attempt = 1
`, taskID, operationID).Scan(&taskStatus, &operationStatus, &attemptStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "cancelled" || operationStatus != "unknown" || attemptStatus != "unknown" {
		t.Fatalf("cancelled operation lifecycle = task %q operation %q attempt %q", taskStatus, operationStatus, attemptStatus)
	}
	var variantStatus, revisionStatus string
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variant.ID).Scan(&variantStatus, &revisionStatus); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "action_required" || revisionStatus != "action_required" {
		t.Fatalf("cancelled creative lifecycle = variant %q revision %q", variantStatus, revisionStatus)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'running' WHERE id = $1`, variant.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant_revision SET status = 'running' WHERE variant_id = $1 AND revision = 1`, variant.ID); err != nil {
		t.Fatal(err)
	}

	var batchTaskID, batchOperationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, trigger_evidence_kind, trigger_evidence_ref_id, context
)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), $2, 'running', 'creative_order_item_production', $3,
        jsonb_build_object('type','creative_domain_task','workflow','creative_production','variant_id',$4::text,'revision',1))
RETURNING id::text
`, agentID, issueID, itemID, variant.ID).Scan(&batchTaskID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, task_id
)
VALUES ($1, '1200x628', 1, 'generation', $2, 'running', $3)
RETURNING id::text
`, variant.ID, "batch-cancel-task:"+batchTaskID, batchTaskID).Scan(&batchOperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (operation_id, attempt, status, task_id)
VALUES ($1, 1, 'running', $2)
`, batchOperationID, batchTaskID); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.TaskService.CancelTasksForIssue(t.Context(), parseUUID(issueID)); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT task.status, operation.status, attempt.status
FROM agent_task_queue task
JOIN creative_image_operation operation ON operation.task_id = task.id
JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id
WHERE task.id = $1 AND operation.id = $2 AND attempt.attempt = 1
`, batchTaskID, batchOperationID).Scan(&taskStatus, &operationStatus, &attemptStatus); err != nil {
		t.Fatal(err)
	}
	if taskStatus != "cancelled" || operationStatus != "unknown" || attemptStatus != "unknown" {
		t.Fatalf("batch-cancelled operation lifecycle = task %q operation %q attempt %q", taskStatus, operationStatus, attemptStatus)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT variant.status, revision.status
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
WHERE variant.id = $1
`, variant.ID).Scan(&variantStatus, &revisionStatus); err != nil {
		t.Fatal(err)
	}
	if variantStatus != "action_required" || revisionStatus != "action_required" {
		t.Fatalf("batch-cancelled creative lifecycle = variant %q revision %q", variantStatus, revisionStatus)
	}
}
