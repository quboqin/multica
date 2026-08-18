package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// creativeDirectEditInput creates the domain shell for one independently
// editable source image. The user-visible issue is deliberately created by
// the caller first so normal issue assignment and recovery remain available.
type creativeDirectEditInput struct {
	IssueID       string `json:"issue_id"`
	SubmissionKey string `json:"submission_key"`
	CandidateID   string `json:"candidate_id"`
	UserRequest   string `json:"user_request"`
	TargetSize    string `json:"target_size"`
	DeliveryMode  string `json:"delivery_mode"`
	SquadID       string `json:"squad_id"`
}

type creativeDirectEditResponse struct {
	Order       creativeOrderResponse        `json:"order"`
	Item        creativeOrderItemResponse    `json:"item"`
	Variant     creativeOrderVariantResponse `json:"variant"`
	SourceAsset creativeOrderAssetResponse   `json:"source_asset"`
}

var creativeDirectEditCapabilityBindings = []creativeOrderCapabilityBinding{
	{Capability: "direct_image_edit", SnapshotField: "direct_edit_agent_id"},
	{Capability: "quality_control", SnapshotField: "reviewer_agent_id"},
}

func scanCreativeOrderItem(row rowScanner) (creativeOrderItemResponse, error) {
	var item creativeOrderItemResponse
	var snapshot string
	err := row.Scan(&item.ID, &item.OrderID, &item.CandidateID, &item.SourceAnalysisID, &snapshot,
		&item.Direction, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	item.CopySnapshot = json.RawMessage(snapshot)
	return item, err
}

func (h *Handler) CreateCreativeDirectEdit(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var input creativeDirectEditInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid direct image edit")
		return
	}
	input, err := normalizeCreativeDirectEdit(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issueID, ok := parseUUIDOrBadRequest(w, input.IssueID, "issue_id")
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, input.CandidateID, "candidate_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, input.SquadID, "squad_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize direct image edit")
		return
	}
	defer tx.Rollback(r.Context())
	if input.SubmissionKey != "" {
		if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, uuidToString(workspaceID)+":"+input.SubmissionKey); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to initialize direct image edit")
			return
		}
	}

	var issueExists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM issue WHERE id = $1 AND workspace_id = $2)`, issueID, workspaceID).Scan(&issueExists); err != nil || !issueExists {
		writeError(w, http.StatusUnprocessableEntity, "issue does not belong to this workspace")
		return
	}

	var sourceAttachmentID pgtype.UUID
	err = tx.QueryRow(r.Context(), `
SELECT source_attachment_id
FROM creative_material_candidate
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, candidateID, workspaceID).Scan(&sourceAttachmentID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "candidate does not belong to this workspace")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load direct image source")
		return
	}
	if !sourceAttachmentID.Valid {
		writeError(w, http.StatusConflict, "source attachment is unavailable; wait for the material to finish archiving")
		return
	}
	var sourceAttachmentExists bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM attachment WHERE id = $1 AND workspace_id = $2)`, sourceAttachmentID, workspaceID).Scan(&sourceAttachmentExists); err != nil || !sourceAttachmentExists {
		writeError(w, http.StatusConflict, "source attachment is unavailable; wait for the material to finish archiving")
		return
	}
	// A retry after the client loses its response must not create a second
	// order for the same user-visible issue.
	var existingID string
	if input.SubmissionKey != "" {
		err = tx.QueryRow(r.Context(), `SELECT id::text FROM creative_order WHERE workspace_id = $1 AND submission_key = $2`, workspaceID, input.SubmissionKey).Scan(&existingID)
	} else {
		err = tx.QueryRow(r.Context(), `SELECT id::text FROM creative_order WHERE issue_id = $1`, issueID).Scan(&existingID)
	}
	if err == nil {
		response, loadErr := h.loadCreativeDirectEdit(r, tx, existingID)
		if loadErr != nil {
			writeError(w, http.StatusConflict, "issue already has a different creative order")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to recover direct image edit")
			return
		}
		writeJSON(w, http.StatusOK, response)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to initialize direct image edit")
		return
	}

	snapshotValue := map[string]any{
		"mode":                 "direct_edit",
		"candidate_id":         input.CandidateID,
		"source_attachment_id": uuidToString(sourceAttachmentID),
		"user_request":         input.UserRequest,
		"target_size":          input.TargetSize,
		"delivery_mode":        input.DeliveryMode,
		"squad_snapshot": map[string]string{
			"squad_id":         input.SquadID,
			"direct_edit_role": "图片直接修改智能体",
		},
	}
	if input.DeliveryMode == "publish" {
		var issueSnapshot string
		if err := tx.QueryRow(r.Context(), `
SELECT snapshot::text
FROM creative_issue_context
WHERE issue_id = $1 AND workspace_id = $2
`, issueID, workspaceID).Scan(&issueSnapshot); err != nil {
			writeError(w, http.StatusConflict, "published direct image edit requires a frozen market pack")
			return
		}
		var issueContext struct {
			MarketPack json.RawMessage `json:"market_pack"`
		}
		if json.Unmarshal([]byte(issueSnapshot), &issueContext) != nil || len(issueContext.MarketPack) == 0 || string(issueContext.MarketPack) == "null" {
			writeError(w, http.StatusConflict, "published direct image edit requires a frozen market pack")
			return
		}
		var marketPack any
		if json.Unmarshal(issueContext.MarketPack, &marketPack) != nil {
			writeError(w, http.StatusConflict, "published direct image edit has an invalid frozen market pack")
			return
		}
		snapshotValue["market_pack"] = marketPack
	}
	snapshot, _ := json.Marshal(snapshotValue)
	snapshot, err = freezeCreativeOrderSquadSnapshot(r.Context(), tx, workspaceID, squadID, snapshot, creativeDirectEditCapabilityBindings)
	if err != nil {
		var validationErr *creativeOrderSquadValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusUnprocessableEntity, validationErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "failed to resolve direct image edit squad")
		}
		return
	}
	order, err := scanCreativeOrder(tx.QueryRow(r.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, trigger_evidence_ref_id, created_by, submission_key)
VALUES ($1,$2,'queued',$3::jsonb,'creative_direct_edit',$4,$5,$6)
RETURNING id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
`, workspaceID, issueID, snapshot, candidateID, userID, input.SubmissionKey))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit order")
		return
	}
	copySnapshot, _ := json.Marshal(map[string]any{
		"mode": "direct_edit", "source_attachment_id": uuidToString(sourceAttachmentID),
	})
	item, err := scanCreativeOrderItem(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, direction, status)
VALUES ($1,$2,$3::jsonb,$4,'ready')
RETURNING id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, created_at::text, updated_at::text
`, order.ID, candidateID, copySnapshot, input.UserRequest))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit item")
		return
	}
	brief, _ := json.Marshal(map[string]any{
		"mode": "direct_edit", "user_request": input.UserRequest, "target_size": input.TargetSize, "delivery_mode": input.DeliveryMode,
	})
	variant, err := scanCreativeOrderVariant(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, brief, revision, status)
VALUES ($1,'direct_edit',$2::jsonb,1,'queued')
RETURNING id::text, order_item_id::text, variant_key, brief::text, revision, status, false, false, created_at::text, updated_at::text
`, item.ID, brief))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit variant")
		return
	}
	variant.QCStatus = "pending"
	assetMetadata, _ := json.Marshal(map[string]any{
		"mode": "direct_edit", "role": "source", "source_attachment_id": uuidToString(sourceAttachmentID), "delivery_mode": input.DeliveryMode,
	})
	sourceAsset, err := scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1,$2,1,'generated',$3,$4::jsonb,'{}'::jsonb,'completed')
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variant.ID, input.TargetSize, sourceAttachmentID, assetMetadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image source asset")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize direct image edit")
		return
	}
	order.DerivedStatus = "queued"
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": order.ID})
	writeJSON(w, http.StatusCreated, creativeDirectEditResponse{Order: order, Item: item, Variant: variant, SourceAsset: sourceAsset})
}

func normalizeCreativeDirectEdit(input creativeDirectEditInput) (creativeDirectEditInput, error) {
	input.IssueID = strings.TrimSpace(input.IssueID)
	input.SubmissionKey = strings.TrimSpace(input.SubmissionKey)
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	input.UserRequest = strings.TrimSpace(input.UserRequest)
	input.TargetSize = strings.TrimSpace(input.TargetSize)
	input.DeliveryMode = strings.TrimSpace(input.DeliveryMode)
	input.SquadID = strings.TrimSpace(input.SquadID)
	if input.IssueID == "" || input.CandidateID == "" || input.SquadID == "" || input.UserRequest == "" || len(input.UserRequest) > 4000 || len(input.SubmissionKey) > 200 || !validCreativeAssetSize(input.TargetSize) || (input.DeliveryMode != "preview" && input.DeliveryMode != "publish") {
		return input, errors.New("invalid direct image edit")
	}
	return input, nil
}

// loadCreativeDirectEdit is intentionally narrow: a regular creative order
// cannot be silently treated as a direct-edit retry.
func (h *Handler) loadCreativeDirectEdit(r *http.Request, tx pgx.Tx, orderID string) (creativeDirectEditResponse, error) {
	order, err := scanCreativeOrder(tx.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
FROM creative_order WHERE id = $1
`, parseUUID(orderID)))
	if err != nil || order.TriggerEvidenceKind != "creative_direct_edit" {
		return creativeDirectEditResponse{}, errors.New("not a direct edit")
	}
	item, err := scanCreativeOrderItem(tx.QueryRow(r.Context(), `
SELECT id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, created_at::text, updated_at::text
FROM creative_order_item WHERE order_id = $1 ORDER BY created_at LIMIT 1
`, parseUUID(orderID)))
	if err != nil {
		return creativeDirectEditResponse{}, err
	}
	variant, err := scanCreativeOrderVariant(tx.QueryRow(r.Context(), `
SELECT id::text, order_item_id::text, variant_key, brief::text, revision, status, false, false, created_at::text, updated_at::text
FROM creative_order_variant WHERE order_item_id = $1 AND variant_key = 'direct_edit'
`, parseUUID(item.ID)))
	if err != nil {
		return creativeDirectEditResponse{}, err
	}
	variant.QCStatus = "pending"
	sourceAsset, err := scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
SELECT id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'generated'
ORDER BY created_at LIMIT 1
`, parseUUID(variant.ID)))
	if err != nil {
		return creativeDirectEditResponse{}, err
	}
	order.DerivedStatus = order.Status
	return creativeDirectEditResponse{Order: order, Item: item, Variant: variant, SourceAsset: sourceAsset}, nil
}
