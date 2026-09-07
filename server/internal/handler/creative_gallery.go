package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type creativeGalleryInput struct {
	VariantID          string `json:"variant_id"`
	Revision           int    `json:"revision"`
	IdempotencyKey     string `json:"idempotency_key"`
	QCRiskAcknowledged bool   `json:"qc_risk_acknowledged"`
	QCRiskReason       string `json:"qc_risk_reason"`
}

type creativeGalleryConflict string

func (e creativeGalleryConflict) Error() string { return string(e) }

func (h *Handler) ConfirmCreativeGalleryDelivery(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var input creativeGalleryInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid gallery delivery")
		return
	}
	h.confirmCreativeGalleryDelivery(w, r, workspaceID, userID, input)
}

func (h *Handler) confirmCreativeGalleryDelivery(w http.ResponseWriter, r *http.Request, workspaceID, userID pgtype.UUID, input creativeGalleryInput) {
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	input.QCRiskReason = strings.TrimSpace(input.QCRiskReason)
	if input.Revision < 1 || len(input.IdempotencyKey) > 200 || len(input.QCRiskReason) > 1000 || input.QCRiskAcknowledged != (input.QCRiskReason != "") {
		writeError(w, http.StatusBadRequest, "invalid gallery revision or risk acknowledgement")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to confirm gallery delivery")
		return
	}
	defer tx.Rollback(r.Context())
	event, err := confirmCreativeGalleryEntry(r.Context(), tx, workspaceID, userID, variantID, input)
	if err != nil {
		var conflict creativeGalleryConflict
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			writeError(w, http.StatusNotFound, "gallery variant or revision not found in this workspace")
		case errors.As(err, &conflict):
			writeError(w, http.StatusConflict, conflict.Error())
		default:
			writeError(w, http.StatusInternalServerError, "failed to confirm gallery delivery")
		}
		return
	}
	var issueID pgtype.UUID
	if event.IssueID != "" {
		issueID = parseUUID(event.IssueID)
	}
	activity, err := insertCreativeFeedbackActivity(r, tx, workspaceID, issueID, "member", userID, creativeFeedbackEventInput{
		SubjectType: "variant", SubjectID: event.SubjectID, EventType: "decision", Decision: "accepted", ContextSnapshot: event.ContextSnapshot,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record gallery confirmation")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit gallery delivery")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "variant_id": input.VariantID})
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "feedback", "subject_type": "variant", "subject_id": input.VariantID})
	if event.IssueID != "" {
		h.publishCreativeFeedbackActivity(workspaceID, parseUUID(event.IssueID), "member", uuidToString(userID), activity)
	}
	writeJSON(w, http.StatusOK, event)
}

func confirmCreativeGalleryEntry(ctx context.Context, tx pgx.Tx, workspaceID, userID, variantID pgtype.UUID, input creativeGalleryInput) (creativeFeedbackEventResponse, error) {
	var event creativeFeedbackEventResponse
	var orderID, itemID, issueID pgtype.UUID
	var orderStatus, variantKey, candidateState string
	var currentRevision, activeRevision int
	// Use the same order -> item -> variant lock order as revision activation.
	if err := tx.QueryRow(ctx, `SELECT o.id, o.status, o.issue_id FROM creative_order o
JOIN creative_order_item i ON i.order_id = o.id JOIN creative_order_variant v ON v.order_item_id = i.id
WHERE v.id = $1 AND o.workspace_id = $2 FOR UPDATE OF o`, variantID, workspaceID).Scan(&orderID, &orderStatus, &issueID); err != nil {
		return event, err
	}
	if err := tx.QueryRow(ctx, `SELECT i.id, v.variant_key, v.revision, COALESCE(v.active_revision, 0), v.candidate_state
FROM creative_order_variant v JOIN creative_order_item i ON i.id = v.order_item_id
WHERE v.id = $1 AND i.order_id = $2 FOR UPDATE OF i, v`, variantID, orderID).Scan(&itemID, &variantKey, &currentRevision, &activeRevision, &candidateState); err != nil {
		return event, err
	}
	if orderStatus == "cancelled" || candidateState != "selected" {
		return event, creativeGalleryConflict("only selected variants in an open order can be confirmed")
	}
	existing, err := scanCreativeFeedbackEvent(tx.QueryRow(ctx, creativeFeedbackEventSelect+`
WHERE workspace_id = $1 AND subject_type = 'variant' AND subject_id = $2 AND event_type = 'decision' AND decision = 'accepted'
ORDER BY created_at DESC, id DESC LIMIT 1 FOR UPDATE`, workspaceID, variantID))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return event, err
	}
	previousMembershipID := existing.ID
	if existing.ID != "" {
		var undone bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_feedback_event WHERE undo_of_id = $1)`, parseUUID(existing.ID)).Scan(&undone); err != nil {
			return event, err
		}
		if undone {
			existing = creativeFeedbackEventResponse{}
		}
	}
	snapshot := map[string]any{}
	if existing.ID != "" {
		if err := json.Unmarshal(existing.ContextSnapshot, &snapshot); err != nil {
			return event, err
		}
		if saved, ok := snapshot["revision"].(float64); ok && saved > 0 && saved != float64(input.Revision) {
			return event, creativeGalleryConflict("remove the gallery entry before selecting a different revision")
		}
		if acknowledged, _ := snapshot["qc_risk_acknowledged"].(bool); acknowledged && !input.QCRiskAcknowledged {
			input.QCRiskReason, _ = snapshot["qc_risk_reason"].(string)
			input.QCRiskAcknowledged = strings.TrimSpace(input.QCRiskReason) != ""
		}
	}
	var revisionStatus string
	var expectedSizes []string
	if err := tx.QueryRow(ctx, `SELECT status, expected_sizes FROM creative_order_variant_revision WHERE variant_id = $1 AND revision = $2 FOR UPDATE`, variantID, input.Revision).Scan(&revisionStatus, &expectedSizes); err != nil {
		return event, err
	}
	expectedSizes, err = normalizeCreativeExpectedSizes(expectedSizes)
	if err != nil {
		return event, creativeGalleryConflict("gallery delivery has no valid size contract")
	}
	var visualStatus, outcome string
	if err := tx.QueryRow(ctx, `WITH latest AS (
SELECT attempt, outcome FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2 ORDER BY attempt DESC, created_at DESC LIMIT 1)
SELECT COALESCE((SELECT outcome FROM latest), ''), COALESCE((SELECT status FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2 AND lane = 'visual' AND attempt = (SELECT attempt FROM latest)), '')`, variantID, input.Revision).Scan(&outcome, &visualStatus); err != nil {
		return event, err
	}
	risk := visualStatus == "failed" && creativeQCOutcomeAllowsRiskAdoption(outcome)
	if risk {
		if !input.QCRiskAcknowledged || input.QCRiskReason == "" {
			return event, creativeGalleryConflict("failed creative QC requires explicit risk acknowledgement and a reason")
		}
		if revisionStatus != "action_required" && revisionStatus != "completed" {
			return event, creativeGalleryConflict("gallery revision is still being produced")
		}
	} else if !creativeQCStatusAllowsAdoption(visualStatus) || outcome != "delivered" || revisionStatus != "completed" {
		return event, creativeGalleryConflict("gallery delivery requires finalized visual QC")
	}
	var deliveredCount, primedCount int
	if err := tx.QueryRow(ctx, `SELECT count(DISTINCT size_key) FILTER (WHERE stage = 'delivered'), count(DISTINCT size_key) FILTER (WHERE stage = 'primed')
FROM creative_order_asset WHERE variant_id = $1 AND revision = $2 AND status = 'completed' AND attachment_id IS NOT NULL AND size_key = ANY($3::text[])`, variantID, input.Revision, expectedSizes).Scan(&deliveredCount, &primedCount); err != nil {
		return event, err
	}
	if primedCount != len(expectedSizes) {
		return event, creativeGalleryConflict("gallery Prime package is incomplete")
	}
	if deliveredCount != len(expectedSizes) {
		if !risk {
			return event, creativeGalleryConflict("gallery delivery package is incomplete")
		}
		if _, err := copyCreativePrimedAssetsToDelivered(ctx, tx, variantID, input.Revision, expectedSizes); err != nil {
			return event, creativeGalleryConflict(err.Error())
		}
	}
	if input.Revision == currentRevision && activeRevision != input.Revision {
		if err := activateCreativeVariantRevision(ctx, tx, variantID, input.Revision, expectedSizes); err != nil {
			return event, err
		}
	}
	var assetIDs []string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(array_agg(a.id::text ORDER BY s.ordinality), '{}'::text[])
FROM unnest($3::text[]) WITH ORDINALITY s(size_key, ordinality)
JOIN creative_order_asset a ON a.variant_id = $1 AND a.revision = $2 AND a.size_key = s.size_key
AND a.stage = 'delivered' AND a.status = 'completed' AND a.attachment_id IS NOT NULL`, variantID, input.Revision, expectedSizes).Scan(&assetIDs); err != nil {
		return event, err
	}
	if len(assetIDs) != len(expectedSizes) || len(assetIDs) == 0 {
		return event, creativeGalleryConflict("gallery delivery is incomplete")
	}
	snapshot["action"], snapshot["order_id"], snapshot["item_id"] = "add_to_gallery", uuidToString(orderID), uuidToString(itemID)
	snapshot["variant_key"], snapshot["revision"] = variantKey, input.Revision
	snapshot["delivery_package_sizes"], snapshot["delivery_asset_ids"] = expectedSizes, assetIDs
	snapshot["qc_risk_acknowledged"], snapshot["qc_risk_reason"] = risk, input.QCRiskReason
	snapshot["delivery_confirmed_by"] = uuidToString(userID)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return event, err
	}
	if existing.ID != "" {
		if _, err := tx.Exec(ctx, `UPDATE creative_feedback_event SET context_snapshot = $2::jsonb WHERE id = $1`, parseUUID(existing.ID), raw); err != nil {
			return event, err
		}
		return scanCreativeFeedbackEvent(tx.QueryRow(ctx, creativeFeedbackEventSelect+` WHERE id = $1`, parseUUID(existing.ID)))
	}
	if input.IdempotencyKey == "" {
		input.IdempotencyKey = fmt.Sprintf("gallery:%s:%s", uuidToString(variantID), previousMembershipID)
	}
	var reusedKey bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_feedback_event WHERE workspace_id = $1 AND idempotency_key = $2)`, workspaceID, input.IdempotencyKey).Scan(&reusedKey); err != nil {
		return event, err
	}
	if reusedKey {
		return event, creativeGalleryConflict("gallery membership changed; refresh before adding it again")
	}
	var id pgtype.UUID
	if err := tx.QueryRow(ctx, `INSERT INTO creative_feedback_event (workspace_id, issue_id, idempotency_key, actor_type, actor_id, subject_type, subject_id, event_type, decision, context_snapshot)
VALUES ($1, $2, $3, 'member', $4, 'variant', $5, 'decision', 'accepted', $6::jsonb) RETURNING id`, workspaceID, issueID, input.IdempotencyKey, userID, variantID, raw).Scan(&id); err != nil {
		return event, err
	}
	return scanCreativeFeedbackEvent(tx.QueryRow(ctx, creativeFeedbackEventSelect+` WHERE id = $1`, id))
}
