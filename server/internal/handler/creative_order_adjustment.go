package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type creativeOrderAdjustmentInput struct {
	AdjustmentIssueID string          `json:"adjustment_issue_id"`
	AssetID           string          `json:"asset_id"`
	SizeKey           string          `json:"size_key"`
	SourceRevision    int             `json:"source_revision"`
	Comment           string          `json:"comment"`
	EventType         string          `json:"event_type"`
	ReasonCodes       []string        `json:"reason_codes"`
	Annotation        json.RawMessage `json:"annotation"`
	ContextSnapshot   json.RawMessage `json:"context_snapshot"`
}

type creativeOrderAdjustmentResponse struct {
	TaskID   string `json:"task_id"`
	Revision int    `json:"revision"`
}

type creativeOrderAdjustmentIssueContext struct {
	Workflow       string `json:"workflow"`
	Source         string `json:"creative_adjustment_source"`
	OrderID        string `json:"creative_order_id"`
	ItemID         string `json:"creative_order_item_id"`
	VariantID      string `json:"creative_variant_id"`
	AssetID        string `json:"creative_asset_id"`
	AttachmentID   string `json:"creative_attachment_id"`
	Scope          string `json:"creative_scope"`
	SizeKey        string `json:"creative_size"`
	SourceRevision int    `json:"creative_source_revision"`
	Revision       int    `json:"creative_revision"`
}

// QueueCreativeOrderAdjustment starts an annotated size revision with the
// production agent frozen on the order. The child Issue remains the business
// collaboration record, but it is never sent through the generic direct-edit
// route.
func (h *Handler) QueueCreativeOrderAdjustment(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	if h.TaskService == nil {
		writeError(w, http.StatusServiceUnavailable, "creative production runtime is unavailable")
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	var input creativeOrderAdjustmentInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order adjustment")
		return
	}
	feedback, err := normalizeCreativeFeedbackEvent(creativeFeedbackEventInput{
		IssueID:         input.AdjustmentIssueID,
		SubjectType:     "asset",
		SubjectID:       input.AssetID,
		EventType:       input.EventType,
		Decision:        "needs_revision",
		ReasonCodes:     input.ReasonCodes,
		Comment:         input.Comment,
		Annotation:      input.Annotation,
		ContextSnapshot: input.ContextSnapshot,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	adjustmentIssueID, ok := parseUUIDOrBadRequest(w, feedback.IssueID, "adjustment_issue_id")
	if !ok {
		return
	}
	assetID, ok := parseUUIDOrBadRequest(w, feedback.SubjectID, "asset_id")
	if !ok {
		return
	}
	input.SizeKey = strings.TrimSpace(input.SizeKey)
	if !validCreativeAssetSize(input.SizeKey) || input.SourceRevision < 1 {
		writeError(w, http.StatusBadRequest, "adjustment size_key and source_revision are required")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative adjustment")
		return
	}
	defer tx.Rollback(r.Context())

	var rootIssueID, inputSnapshot string
	err = tx.QueryRow(r.Context(), `
SELECT COALESCE(issue_id::text, ''), input_snapshot::text
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&rootIssueID, &inputSnapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	}
	if err != nil || rootIssueID == "" {
		writeError(w, http.StatusConflict, "creative order cannot start an adjustment")
		return
	}

	var parentIssueID, issueMetadata string
	err = tx.QueryRow(r.Context(), `
SELECT COALESCE(parent_issue_id::text, ''), metadata::text
FROM issue
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, adjustmentIssueID, workspaceID).Scan(&parentIssueID, &issueMetadata)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "adjustment collaboration record does not belong to this workspace")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load adjustment collaboration record")
		return
	}
	var issueContext creativeOrderAdjustmentIssueContext
	if json.Unmarshal([]byte(issueMetadata), &issueContext) != nil ||
		issueContext.Workflow != "creative_adjustment" || issueContext.Source != "creative_order" ||
		parentIssueID != rootIssueID || issueContext.OrderID != uuidToString(orderID) ||
		issueContext.AssetID != uuidToString(assetID) || issueContext.Scope != "size" ||
		issueContext.SizeKey != input.SizeKey || issueContext.SourceRevision != input.SourceRevision ||
		issueContext.Revision != input.SourceRevision+1 {
		writeError(w, http.StatusConflict, "adjustment collaboration record no longer matches this order image")
		return
	}

	var itemID, candidateID, variantID, assetAttachmentID string
	var currentRevision int
	err = tx.QueryRow(r.Context(), `
SELECT item.id::text, item.candidate_id::text, variant.id::text, variant.revision,
  COALESCE(asset.attachment_id::text, '')
FROM creative_order_item item
JOIN creative_order_variant variant ON variant.order_item_id = item.id
JOIN creative_order_asset asset ON asset.variant_id = variant.id
WHERE item.order_id = $1
  AND variant.id::text = $2
  AND asset.id = $3
  AND asset.revision = $4
  AND asset.size_key = $5
  AND asset.status = 'completed'
  AND asset.attachment_id IS NOT NULL
FOR UPDATE OF item, variant, asset
`, orderID, issueContext.VariantID, assetID, input.SourceRevision, input.SizeKey).Scan(
		&itemID, &candidateID, &variantID, &currentRevision, &assetAttachmentID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "adjustment target is no longer the current completed image")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate adjustment target")
		return
	}
	if issueContext.ItemID != itemID || issueContext.VariantID != variantID || issueContext.AttachmentID != assetAttachmentID || currentRevision != input.SourceRevision {
		writeError(w, http.StatusConflict, "adjustment target has already changed")
		return
	}

	var activeTask bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM agent_task_queue
  WHERE context->>'type' = 'creative_domain_task'
    AND context->>'creative_order_id' = $1::text
    AND context->>'variant_id' = $2
    AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, orderID, variantID).Scan(&activeTask); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check existing creative work")
		return
	}
	if activeTask {
		writeError(w, http.StatusConflict, "this variant already has active work")
		return
	}

	leaderID, producerID, reviewerID, err := creativeOrderProductionAgentSnapshot(json.RawMessage(inputSnapshot))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	producer, err := h.Queries.WithTx(tx).GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: producerID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || producer.ArchivedAt.Valid || !producer.RuntimeID.Valid {
		writeError(w, http.StatusConflict, "the frozen creative production agent is unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative production agent")
		return
	}
	var producerCapable bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill bound_skill ON bound_skill.id = binding.skill_id
  WHERE binding.agent_id = $1
    AND binding.enabled
    AND bound_skill.workspace_id = $2
    AND bound_skill.config->>'kind' = 'creative_role'
    AND bound_skill.config->>'capability' = 'image_edit'
)
`, producerID, workspaceID).Scan(&producerCapable); err != nil || !producerCapable {
		writeError(w, http.StatusConflict, "the frozen creative production agent cannot process this adjustment")
		return
	}

	expectedSizes := []string{"1080x1080", "1200x628", "800x1000"}
	rows, err := tx.Query(r.Context(), `
SELECT size_key
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($3::text[])
FOR UPDATE
`, parseUUID(variantID), input.SourceRevision, expectedSizes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load generated adjustment base")
		return
	}
	generatedSizes := map[string]struct{}{}
	for rows.Next() {
		var size string
		if err := rows.Scan(&size); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read generated adjustment base")
			return
		}
		generatedSizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read generated adjustment base")
		return
	}
	rows.Close()
	if !creativeSizesMatchExpected(generatedSizes, expectedSizes) {
		writeError(w, http.StatusConflict, "adjustment requires a complete generated base package")
		return
	}

	newRevision := input.SourceRevision + 1
	if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, $3, 'generated', attachment_id, id,
  metadata,
  evidence || jsonb_build_object('order_adjustment', jsonb_build_object('source_revision', $2::integer, 'target_size', $5::text, 'reused_generated_base', true)),
  'completed'
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
  AND size_key <> $5
ON CONFLICT (variant_id, size_key, revision, stage) DO NOTHING
`, parseUUID(variantID), input.SourceRevision, newRevision, expectedSizes, input.SizeKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to preserve unaffected sizes for adjustment")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET revision = $2, status = 'running', updated_at = now()
WHERE id = $1
`, parseUUID(variantID), newRevision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin image adjustment")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_item
SET adopted_variant_id = NULL, adopted_at = NULL, adopted_by = NULL, updated_at = now()
WHERE id = $1 AND adopted_variant_id = $2
`, parseUUID(itemID), parseUUID(variantID)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear superseded adoption")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE issue SET status = 'in_progress', updated_at = now() WHERE id = $1`, adjustmentIssueID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update adjustment collaboration record")
		return
	}

	var feedbackContext map[string]any
	if json.Unmarshal(feedback.ContextSnapshot, &feedbackContext) != nil || feedbackContext == nil {
		writeError(w, http.StatusBadRequest, "adjustment context must be an object")
		return
	}
	feedbackContext["order_id"] = uuidToString(orderID)
	feedbackContext["adjustment_issue_id"] = uuidToString(adjustmentIssueID)
	feedbackContext["variant_id"] = variantID
	feedbackContext["size_key"] = input.SizeKey
	feedbackContext["revision"] = input.SourceRevision
	feedbackContext["scope"] = "size"
	feedback.ContextSnapshot, _ = json.Marshal(feedbackContext)

	taskContext, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"scope":                  "size",
		"subject_id":             variantID,
		"item_key":               fmt.Sprintf("%s:r%d", variantID, newRevision),
		"creative_order_id":      uuidToString(orderID),
		"creative_order_item_id": itemID,
		"candidate_id":           candidateID,
		"variant_id":             variantID,
		"revision":               newRevision,
		"expected_sizes":         expectedSizes,
		"issue_id":               uuidToString(adjustmentIssueID),
		"leader_agent_id":        uuidToString(leaderID),
		"reviewer_agent_id":      uuidToString(reviewerID),
		"order_adjustment": map[string]any{
			"adjustment_issue_id":  uuidToString(adjustmentIssueID),
			"source_revision":      input.SourceRevision,
			"target_size":          input.SizeKey,
			"source_asset_id":      uuidToString(assetID),
			"source_attachment_id": assetAttachmentID,
			"request":              feedback.Comment,
			"annotations":          feedbackContext["annotations"],
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare image adjustment")
		return
	}
	if err := validateCreativeTaskFanoutContext("creative_order_item_production", parseUUID(itemID), []service.DirectTaskFanoutItem{{
		ItemKey: fmt.Sprintf("%s:r%d", variantID, newRevision), Context: taskContext,
	}}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate image adjustment task")
		return
	}
	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_item_production"), parseUUID(itemID))
	task, err := h.Queries.WithTx(tx).CreateAgentTask(r.Context(), db.CreateAgentTaskParams{
		AgentID:              producer.ID,
		RuntimeID:            producer.RuntimeID,
		IssueID:              adjustmentIssueID,
		Priority:             0,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     userID,
		OriginatorUserID:     attr.UserID,
		AccountableUserID:    attr.AccountableUserID,
		OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
		TriggerEvidenceKind:  pgtype.Text{String: "creative_order_item_production", Valid: true},
		TriggerEvidenceRefID: parseUUID(itemID),
		Context:              taskContext,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "image adjustment could not be queued")
		return
	}
	actorType, actorID := h.resolveActor(r, uuidToString(userID), uuidToString(workspaceID))
	actorUUID, ok := parseUUIDOrBadRequest(w, actorID, "actor_id")
	if !ok {
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, decision, reason_codes, comment, annotation, context_snapshot
) VALUES ($1, $2, $3, $4, 'asset', $5, $6, 'needs_revision', $7, $8, $9::jsonb, $10::jsonb)
`, workspaceID, adjustmentIssueID, actorType, actorUUID, assetID, feedback.EventType, feedback.ReasonCodes, feedback.Comment, feedback.Annotation, feedback.ContextSnapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record image adjustment")
		return
	}
	adjustmentDetails, err := json.Marshal(map[string]any{
		"variant_id":      variantID,
		"size_key":        input.SizeKey,
		"source_revision": input.SourceRevision,
		"revision":        newRevision,
		"task_id":         uuidToString(task.ID),
		"request":         feedback.Comment,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare adjustment collaboration record")
		return
	}
	activity := &creativeFeedbackActivity{Action: "creative_order_adjustment_queued", Details: adjustmentDetails}
	var createdAt pgtype.Timestamptz
	if err := tx.QueryRow(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, $3, $4, 'creative_order_adjustment_queued', $5::jsonb)

RETURNING id::text, created_at
`, workspaceID, adjustmentIssueID, actorType, actorUUID, adjustmentDetails).Scan(&activity.ID, &createdAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record adjustment start")
		return
	}
	activity.CreatedAt = timestampToString(createdAt)
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue image adjustment")
		return
	}
	h.TaskService.NotifyTaskEnqueued(r.Context(), task)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": uuidToString(orderID), "variant_id": variantID, "revision": newRevision,
		"adjustment_issue_id": uuidToString(adjustmentIssueID), "task_id": uuidToString(task.ID),
	})
	h.publishCreativeFeedbackActivity(workspaceID, adjustmentIssueID, actorType, actorID, activity)
	writeJSON(w, http.StatusCreated, creativeOrderAdjustmentResponse{TaskID: uuidToString(task.ID), Revision: newRevision})
}
