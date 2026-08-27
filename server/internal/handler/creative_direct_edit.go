package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
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
	TaskID      string                       `json:"task_id"`
}

var creativeDirectEditCapabilityBindings = []creativeOrderCapabilityBinding{
	{Capability: "direct_image_edit", SnapshotField: "direct_edit_agent_id", PoolSnapshotField: "direct_edit_agent_ids", AllowMultiple: true},
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
	if h.TaskService == nil {
		writeError(w, http.StatusServiceUnavailable, "creative production runtime is unavailable")
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

	var issueStatus string
	err = tx.QueryRow(r.Context(), `
SELECT status
FROM issue
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, issueID, workspaceID).Scan(&issueStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "issue does not belong to this workspace")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock direct image edit issue")
		return
	}
	if issueStatus == "cancelled" {
		writeError(w, http.StatusConflict, "cancelled issue cannot start a direct image edit")
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
		"pipeline_version":     creativePipelineDirectEditV1,
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
VALUES ($1,$2,'running',$3::jsonb,'creative_direct_edit',$4,$5,$6)
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
VALUES ($1,$2,$3::jsonb,$4,'running')
RETURNING id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, created_at::text, updated_at::text
`, order.ID, candidateID, copySnapshot, input.UserRequest))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit item")
		return
	}
	sourceBrief, _ := json.Marshal(map[string]any{
		"mode": "direct_edit", "user_request": input.UserRequest, "target_size": input.TargetSize, "delivery_mode": input.DeliveryMode,
		"creative_direct_edit_delivery": map[string]any{
			"final_visual_validation": true,
			"delivery_mode":           input.DeliveryMode,
			"target_size":             input.TargetSize,
			"scope":                   "size",
			"expected_sizes":          []string{input.TargetSize},
			"raw_user_request":        input.UserRequest,
		},
	})
	var taskBriefValue map[string]any
	if err := json.Unmarshal(sourceBrief, &taskBriefValue); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare direct image edit revision")
		return
	}
	delivery, _ := taskBriefValue["creative_direct_edit_delivery"].(map[string]any)
	delivery["source_revision"] = 1
	delivery["edit_sizes"] = []string{input.TargetSize}
	taskBrief, _ := json.Marshal(taskBriefValue)
	variant, err := scanCreativeOrderVariant(tx.QueryRow(r.Context(), `
	INSERT INTO creative_order_variant (order_item_id, variant_key, brief, revision, status, staging_revision)
	VALUES ($1,'direct_edit',$2::jsonb,2,'running',2)
	RETURNING id::text, order_item_id::text, variant_key, brief::text, revision, status,
	  COALESCE(active_revision, 0), COALESCE(staging_revision, 0), candidate_state,
	  COALESCE(selection_rank, 0), primary_size, false, false, created_at::text, updated_at::text
`, item.ID, taskBrief))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit variant")
		return
	}
	variant.QCStatus = "pending"
	if err := upsertCreativeVariantRevision(r.Context(), tx, parseUUID(variant.ID), 1, sourceBrief, "completed", []string{input.TargetSize}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image source revision")
		return
	}
	if err := upsertCreativeVariantRevision(r.Context(), tx, parseUUID(variant.ID), 2, taskBrief, "running", []string{input.TargetSize}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image edit revision")
		return
	}
	assetMetadata, _ := json.Marshal(map[string]any{
		"mode": "direct_edit", "role": "source", "source_attachment_id": uuidToString(sourceAttachmentID), "delivery_mode": input.DeliveryMode,
	})
	sourceAsset, err := scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, metadata, evidence, status)
VALUES ($1,$2,1,'generated',$3,$4::jsonb,'{}'::jsonb,'completed')
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
	  COALESCE(derived_from_asset_id::text, ''), COALESCE(operation_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variant.ID, input.TargetSize, sourceAttachmentID, assetMetadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create direct image source asset")
		return
	}
	leaderID, err := creativeDirectEditSnapshotAgent(snapshot, "leader_agent_id", "leader")
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	reviewerID, err := creativeDirectEditSnapshotAgent(snapshot, "reviewer_agent_id", "QC reviewer")
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	preferredDirectEditorID, err := creativeOrderDirectEditAgentSnapshot(snapshot)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	directEditor, err := h.selectCreativeDirectImageEditAgent(
		r.Context(), tx, h.Queries.WithTx(tx), workspaceID, snapshot, variant.ID, preferredDirectEditorID, false,
	)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	itemKey := fmt.Sprintf("%s:r2", variant.ID)
	sourceAssets := []creativeOrderAdjustmentSourceAsset{{
		SizeKey: input.TargetSize, AssetID: sourceAsset.ID, AttachmentID: uuidToString(sourceAttachmentID),
	}}
	taskContext, err := json.Marshal(map[string]any{
		"type":                    "creative_domain_task",
		"workflow":                "creative_direct_edit",
		"scope":                   "size",
		"subject_id":              variant.ID,
		"item_key":                itemKey,
		"creative_order_id":       order.ID,
		"creative_order_item_id":  item.ID,
		"candidate_id":            input.CandidateID,
		"variant_id":              variant.ID,
		"revision":                2,
		"expected_sizes":          []string{input.TargetSize},
		"issue_id":                input.IssueID,
		"leader_agent_id":         uuidToString(leaderID),
		"reviewer_agent_id":       uuidToString(reviewerID),
		"direct_edit_agent_id":    uuidToString(directEditor.ID),
		"direct_edit_runtime_id":  uuidToString(directEditor.RuntimeID),
		"user_request":            input.UserRequest,
		"raw_user_request":        input.UserRequest,
		"prompt_compilation":      "intent_normalization_required",
		"final_visual_validation": true,
		"delivery_mode":           input.DeliveryMode,
		"target_size":             input.TargetSize,
		"edit_sizes":              []string{input.TargetSize},
		"source_revision":         1,
		"source_asset_id":         sourceAsset.ID,
		"source_attachment_id":    uuidToString(sourceAttachmentID),
		"source_assets":           sourceAssets,
		"direct_edit": map[string]any{
			"scope":                   "size",
			"source_revision":         1,
			"target_size":             input.TargetSize,
			"expected_sizes":          []string{input.TargetSize},
			"edit_sizes":              []string{input.TargetSize},
			"source_asset_id":         sourceAsset.ID,
			"source_attachment_id":    uuidToString(sourceAttachmentID),
			"source_assets":           sourceAssets,
			"direct_edit_agent_id":    uuidToString(directEditor.ID),
			"direct_edit_runtime_id":  uuidToString(directEditor.RuntimeID),
			"request":                 input.UserRequest,
			"raw_user_request":        input.UserRequest,
			"prompt_compilation":      "intent_normalization_required",
			"final_visual_validation": true,
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare direct image edit task")
		return
	}
	if err := validateCreativeTaskFanoutContext("creative_order_item_direct_edit", parseUUID(item.ID), []service.DirectTaskFanoutItem{{
		ItemKey: itemKey, Context: taskContext,
	}}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate direct image edit task")
		return
	}
	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_item_direct_edit"), parseUUID(item.ID))
	tasks, createdTasks, err := h.TaskService.EnqueueDirectTaskFanoutTx(r.Context(), tx, service.DirectTaskFanout{
		Agent: directEditor, IssueID: issueID, RequestingUserID: userID, Attribution: attr,
		TriggerEvidenceKind: "creative_order_item_direct_edit", TriggerEvidenceRefID: parseUUID(item.ID),
		Items: []service.DirectTaskFanoutItem{{ItemKey: itemKey, Context: taskContext}},
	})
	if err != nil || len(tasks) != 1 {
		writeError(w, http.StatusConflict, "direct image edit could not be queued")
		return
	}
	issueUpdate, err := tx.Exec(r.Context(), `
UPDATE issue
SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{creative_order_id}', to_jsonb($2::text), true),
    assignee_type = 'squad', assignee_id = $3, status = 'in_progress', updated_at = now()
WHERE id = $1 AND workspace_id = $4 AND status <> 'cancelled'
`, issueID, order.ID, squadID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to link direct image edit issue")
		return
	}
	if issueUpdate.RowsAffected() != 1 {
		writeError(w, http.StatusConflict, "direct image edit issue was cancelled")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize direct image edit")
		return
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(r.Context(), createdTasks)
	order.DerivedStatus = "running"
	order.DeliveryStatus = "pending"
	order.ProductionStatus = "running"
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": order.ID})
	writeJSON(w, http.StatusCreated, creativeDirectEditResponse{
		Order: order, Item: item, Variant: variant, SourceAsset: sourceAsset, TaskID: uuidToString(tasks[0].ID),
	})
}

func creativeDirectEditSnapshotAgent(raw json.RawMessage, field, role string) (pgtype.UUID, error) {
	var snapshot struct {
		SquadSnapshot map[string]json.RawMessage `json:"squad_snapshot"`
	}
	if json.Unmarshal(raw, &snapshot) != nil {
		return pgtype.UUID{}, errors.New("creative order has an invalid frozen squad snapshot")
	}
	var rawID string
	if json.Unmarshal(snapshot.SquadSnapshot[field], &rawID) != nil {
		return pgtype.UUID{}, fmt.Errorf("creative order is missing a frozen %s agent", role)
	}
	id, err := parseUUIDString(strings.TrimSpace(rawID))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("creative order is missing a frozen %s agent", role)
	}
	return id, nil
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
SELECT id::text, order_item_id::text, variant_key, brief::text, revision, status,
  COALESCE(active_revision, 0), COALESCE(staging_revision, 0), candidate_state,
  COALESCE(selection_rank, 0), primary_size, false, false, created_at::text, updated_at::text
FROM creative_order_variant WHERE order_item_id = $1 AND variant_key = 'direct_edit'
`, parseUUID(item.ID)))
	if err != nil {
		return creativeDirectEditResponse{}, err
	}
	variant.QCStatus = "pending"
	sourceAsset, err := scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
SELECT id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
	  COALESCE(derived_from_asset_id::text, ''), COALESCE(operation_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = 1 AND stage = 'generated'
ORDER BY created_at LIMIT 1
`, parseUUID(variant.ID)))
	if err != nil {
		return creativeDirectEditResponse{}, err
	}
	var taskID string
	if err := tx.QueryRow(r.Context(), `
SELECT id::text
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_direct_edit'
  AND trigger_evidence_ref_id = $1
  AND context->>'workflow' = 'creative_direct_edit'
  AND context->>'creative_order_id' = $2
  AND context->>'variant_id' = $3
ORDER BY CASE WHEN status IN ('queued','dispatched','running','waiting_local_directory') THEN 0 ELSE 1 END,
         created_at DESC, id DESC
LIMIT 1
`, parseUUID(item.ID), order.ID, variant.ID).Scan(&taskID); err != nil {
		return creativeDirectEditResponse{}, err
	}
	order.DerivedStatus = order.Status
	order.DeliveryStatus = "pending"
	order.ProductionStatus = order.Status
	return creativeDirectEditResponse{Order: order, Item: item, Variant: variant, SourceAsset: sourceAsset, TaskID: taskID}, nil
}
