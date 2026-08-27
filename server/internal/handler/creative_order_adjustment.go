package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type creativeOrderAdjustmentInput struct {
	AdjustmentIssueID           string          `json:"adjustment_issue_id"`
	AssetID                     string          `json:"asset_id"`
	SizeKey                     string          `json:"size_key"`
	Scope                       string          `json:"scope"`
	SourceRevision              int             `json:"source_revision"`
	AnnotationGuideAttachmentID string          `json:"annotation_guide_attachment_id"`
	Comment                     string          `json:"comment"`
	EventType                   string          `json:"event_type"`
	ReasonCodes                 []string        `json:"reason_codes"`
	Annotation                  json.RawMessage `json:"annotation"`
	ContextSnapshot             json.RawMessage `json:"context_snapshot"`
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

type creativeOrderAdjustmentSourceAsset struct {
	SizeKey      string `json:"size_key"`
	AssetID      string `json:"asset_id"`
	AttachmentID string `json:"attachment_id"`
}

func normalizeCreativeOrderAdjustmentScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case "", "size":
		return "size"
	case "variant":
		return "variant"
	default:
		return ""
	}
}

func creativeOrderAdjustmentEditSizes(scope, targetSize string, expectedSizes []string) []string {
	if scope == "variant" {
		return append([]string(nil), expectedSizes...)
	}
	return []string{targetSize}
}

// QueueCreativeOrderAdjustment starts an annotated size revision with the
// direct image-edit agent frozen from the order squad. The selected final
// asset remains the collaboration reference, while the edit agent receives
// the matching unbranded generated base plus any annotated final-image brief.
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
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
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
	input.Scope = normalizeCreativeOrderAdjustmentScope(input.Scope)
	if !validCreativeAssetSize(input.SizeKey) || input.Scope == "" || input.SourceRevision < 1 {
		writeError(w, http.StatusBadRequest, "adjustment size_key, scope, and source_revision are required")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative adjustment")
		return
	}
	defer tx.Rollback(r.Context())

	var rootIssueID, inputSnapshot, orderStatus string
	err = tx.QueryRow(r.Context(), `
SELECT COALESCE(issue_id::text, ''), input_snapshot::text, status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&rootIssueID, &inputSnapshot, &orderStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	}
	if err != nil || rootIssueID == "" {
		writeError(w, http.StatusConflict, "creative order cannot start an adjustment")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "cancelled creative order cannot start an adjustment")
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
		issueContext.AssetID != uuidToString(assetID) || issueContext.Scope != input.Scope ||
		issueContext.SizeKey != input.SizeKey || issueContext.SourceRevision != input.SourceRevision ||
		issueContext.Revision != input.SourceRevision+1 {
		writeError(w, http.StatusConflict, "adjustment collaboration record no longer matches this order image")
		return
	}

	var itemID, candidateID, variantID, assetAttachmentID, sourceAssetID, sourceAttachmentID string
	var expectedSizes []string
	var currentRevision int
	err = tx.QueryRow(r.Context(), `
SELECT item.id::text, item.candidate_id::text, variant.id::text, variant.revision,
	  COALESCE(asset.attachment_id::text, ''),
	  COALESCE(source_asset.id::text, ''), COALESCE(source_asset.attachment_id::text, ''),
	  source_revision.expected_sizes
FROM creative_order_item item
JOIN creative_order_variant variant ON variant.order_item_id = item.id
JOIN creative_order_variant_revision source_revision
  ON source_revision.variant_id = variant.id
  AND source_revision.revision = $4
JOIN creative_order_asset asset ON asset.variant_id = variant.id
LEFT JOIN creative_order_asset source_asset
  ON source_asset.variant_id = variant.id
  AND source_asset.size_key = asset.size_key
  AND source_asset.revision = $4
  AND source_asset.stage = 'generated'
  AND source_asset.status = 'completed'
  AND source_asset.attachment_id IS NOT NULL
WHERE item.order_id = $1
  AND variant.id::text = $2
  AND asset.id = $3
	  AND asset.revision = $4
	  AND (variant.active_revision = $4 OR variant.staging_revision = $4)
	  AND asset.size_key = $5
  AND asset.status = 'completed'
  AND asset.attachment_id IS NOT NULL
FOR UPDATE OF item, variant, source_revision, asset
	`, orderID, issueContext.VariantID, assetID, input.SourceRevision, input.SizeKey).Scan(
		&itemID, &candidateID, &variantID, &currentRevision, &assetAttachmentID, &sourceAssetID, &sourceAttachmentID, &expectedSizes,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "adjustment target is no longer the current completed image")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate adjustment target")
		return
	}
	if issueContext.ItemID != itemID || issueContext.VariantID != variantID || issueContext.AttachmentID != assetAttachmentID {
		writeError(w, http.StatusConflict, "adjustment target has already changed")
		return
	}
	expectedSizes, err = normalizeCreativeExpectedSizes(expectedSizes)
	if err != nil {
		writeError(w, http.StatusConflict, "active creative revision has an invalid delivery contract")
		return
	}
	if !creativeSizeIsExpected(input.SizeKey, expectedSizes) {
		writeError(w, http.StatusConflict, "adjustment size is not part of the order delivery package")
		return
	}
	editSizes := creativeOrderAdjustmentEditSizes(input.Scope, input.SizeKey, expectedSizes)
	newRevision := input.SourceRevision + 1

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

	if currentRevision != input.SourceRevision {
		writeError(w, http.StatusConflict, "adjustment must explicitly target the current active or staging revision")
		return
	}

	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(json.RawMessage(inputSnapshot))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	squadIDText, err := creativeOrderSnapshotSquadID(json.RawMessage(inputSnapshot))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	squadUUID, err := uuid.Parse(strings.TrimSpace(squadIDText))
	if err != nil {
		writeError(w, http.StatusConflict, "creative order has an invalid frozen squad")
		return
	}
	squadID := pgtype.UUID{Bytes: squadUUID, Valid: true}
	frozenSnapshot, err := freezeCreativeOrderSquadSnapshot(r.Context(), tx, workspaceID, squadID, json.RawMessage(inputSnapshot), []creativeOrderCapabilityBinding{
		{Capability: "direct_image_edit", SnapshotField: "direct_edit_agent_id", PoolSnapshotField: "direct_edit_agent_ids", AllowMultiple: true},
	})
	if err != nil {
		var validationErr *creativeOrderSquadValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusConflict, validationErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "failed to resolve direct image edit agent")
		}
		return
	}
	preferredDirectEditAgentID, err := creativeOrderDirectEditAgentSnapshot(frozenSnapshot)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	pinPreferredDirectEditor := false
	if historical := selectedCreativeDirectEditAgentFromHistory(r.Context(), tx, workspaceID, parseUUID(variantID), newRevision); historical.Valid && historical == preferredDirectEditAgentID {
		preferredDirectEditAgentID = historical
		pinPreferredDirectEditor = true
	}
	directEditor, err := h.selectCreativeDirectImageEditAgent(r.Context(), tx, h.Queries.WithTx(tx), workspaceID, frozenSnapshot, variantID, preferredDirectEditAgentID, pinPreferredDirectEditor)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if sourceAssetID == "" || sourceAttachmentID == "" {
		writeError(w, http.StatusConflict, "adjustment requires an unbranded generated base asset")
		return
	}
	rows, err := tx.Query(r.Context(), `
SELECT size_key, id::text, attachment_id::text
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
	sourceAssetsBySize := map[string]creativeOrderAdjustmentSourceAsset{}
	for rows.Next() {
		var source creativeOrderAdjustmentSourceAsset
		if err := rows.Scan(&source.SizeKey, &source.AssetID, &source.AttachmentID); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read generated adjustment base")
			return
		}
		generatedSizes[source.SizeKey] = struct{}{}
		sourceAssetsBySize[source.SizeKey] = source
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
	sourceAssetRefs := make([]creativeOrderAdjustmentSourceAsset, 0, len(expectedSizes))
	for _, size := range expectedSizes {
		source := sourceAssetsBySize[size]
		if source.SizeKey == "" {
			writeError(w, http.StatusConflict, "adjustment requires a complete generated base package")
			return
		}
		sourceAssetRefs = append(sourceAssetRefs, source)
	}

	var feedbackContext map[string]any
	if json.Unmarshal(feedback.ContextSnapshot, &feedbackContext) != nil || feedbackContext == nil {
		writeError(w, http.StatusBadRequest, "adjustment context must be an object")
		return
	}
	annotationGuideAttachmentID := strings.TrimSpace(input.AnnotationGuideAttachmentID)
	if annotationGuideAttachmentID == "" {
		annotationGuideAttachmentID, _ = feedbackContext["annotation_guide_attachment_id"].(string)
		annotationGuideAttachmentID = strings.TrimSpace(annotationGuideAttachmentID)
	}
	annotations, _ := feedbackContext["annotations"].([]any)
	if len(annotations) > 0 && annotationGuideAttachmentID == "" {
		writeError(w, http.StatusBadRequest, "annotated adjustment requires an annotation guide attachment")
		return
	}
	if annotationGuideAttachmentID != "" {
		annotationGuideID, parseErr := uuid.Parse(annotationGuideAttachmentID)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "annotation guide attachment must be a UUID")
			return
		}
		var guideExists bool
		if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM attachment
  WHERE id = $1 AND workspace_id = $2 AND issue_id = $3
)
`, pgtype.UUID{Bytes: annotationGuideID, Valid: true}, workspaceID, adjustmentIssueID).Scan(&guideExists); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate annotation guide attachment")
			return
		}
		if !guideExists {
			writeError(w, http.StatusUnprocessableEntity, "annotation guide attachment does not belong to the adjustment")
			return
		}
	}

	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order
SET input_snapshot = $2::jsonb, updated_at = now()
WHERE id = $1 AND workspace_id = $3
`, orderID, frozenSnapshot, workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to freeze direct image edit agent")
		return
	}
	if input.Scope == "size" {
		if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, $3, 'generated', attachment_id, id,
  metadata || jsonb_build_object('order_adjustment', jsonb_build_object('source_revision', $2::integer, 'target_size', $5::text, 'scope', $6::text, 'reused_generated_base', true, 'source_asset_id', id::text)),
  evidence || jsonb_build_object('order_adjustment', jsonb_build_object('source_revision', $2::integer, 'target_size', $5::text, 'scope', $6::text, 'reused_generated_base', true, 'source_asset_id', id::text)),
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
`, parseUUID(variantID), input.SourceRevision, newRevision, expectedSizes, input.SizeKey, input.Scope); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to preserve unaffected sizes for adjustment")
			return
		}
	}
	if input.Scope == "size" {
		if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
SELECT variant_id, task_id, attachment_id, size_key, $3, workflow, label, filename,
  metadata || jsonb_build_object('order_adjustment', jsonb_build_object('source_revision', $2::integer, 'target_size', $4::text, 'scope', $5::text, 'reused_for_adjustment', true))
FROM creative_order_diagnostic_asset
WHERE variant_id = $1 AND revision = $2
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO NOTHING
`, parseUUID(variantID), input.SourceRevision, newRevision, input.SizeKey, input.Scope); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to preserve adjustment process evidence")
			return
		}
	}
	annotationsJSON, _ := json.Marshal(feedbackContext["annotations"])
	if len(annotationsJSON) == 0 || string(annotationsJSON) == "null" {
		annotationsJSON = []byte("[]")
	}
	var revisionBrief string
	if err := tx.QueryRow(r.Context(), `
UPDATE creative_order_variant
SET revision = $2,
    status = 'running',
	staging_revision = $2,
    brief = jsonb_set(
      brief - 'creative_direct_edit_error',
      '{creative_direct_edit_delivery}',
	      jsonb_build_object(
	        'final_visual_validation', true,
        'source_revision', $3::integer,
        'target_size', $4::text,
        'scope', $5::text,
        'expected_sizes', to_jsonb($6::text[]),
        'edit_sizes', to_jsonb($7::text[]),
        'raw_user_request', $8::text,
        'annotations', $9::jsonb,
        'annotation_guide_attachment_id', $10::text
      ),
      true
    ),
    updated_at = now()
WHERE id = $1
RETURNING brief::text
	`, parseUUID(variantID), newRevision, input.SourceRevision, input.SizeKey, input.Scope, expectedSizes, editSizes, feedback.Comment, string(annotationsJSON), annotationGuideAttachmentID).Scan(&revisionBrief); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin image adjustment")
		return
	}
	if err := upsertCreativeVariantRevision(r.Context(), tx, parseUUID(variantID), newRevision, json.RawMessage(revisionBrief), "running", expectedSizes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create image adjustment revision")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE issue SET status = 'in_progress', updated_at = now() WHERE id = $1`, adjustmentIssueID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update adjustment collaboration record")
		return
	}

	feedbackContext["order_id"] = uuidToString(orderID)
	feedbackContext["adjustment_issue_id"] = uuidToString(adjustmentIssueID)
	feedbackContext["variant_id"] = variantID
	feedbackContext["size_key"] = input.SizeKey
	feedbackContext["revision"] = input.SourceRevision
	feedbackContext["scope"] = input.Scope
	feedbackContext["expected_sizes"] = expectedSizes
	feedbackContext["edit_sizes"] = editSizes
	if annotationGuideAttachmentID != "" {
		feedbackContext["annotation_guide_attachment_id"] = annotationGuideAttachmentID
		feedbackContext["annotation_guide_source"] = "final_reference"
	}
	feedback.ContextSnapshot, _ = json.Marshal(feedbackContext)

	taskContext, err := json.Marshal(map[string]any{
		"type":                           "creative_domain_task",
		"workflow":                       "creative_direct_edit",
		"scope":                          input.Scope,
		"subject_id":                     variantID,
		"item_key":                       fmt.Sprintf("%s:r%d", variantID, newRevision),
		"creative_order_id":              uuidToString(orderID),
		"creative_order_item_id":         itemID,
		"candidate_id":                   candidateID,
		"variant_id":                     variantID,
		"revision":                       newRevision,
		"expected_sizes":                 expectedSizes,
		"issue_id":                       uuidToString(adjustmentIssueID),
		"leader_agent_id":                uuidToString(leaderID),
		"reviewer_agent_id":              uuidToString(reviewerID),
		"direct_edit_agent_id":           uuidToString(directEditor.ID),
		"direct_edit_runtime_id":         uuidToString(directEditor.RuntimeID),
		"user_request":                   feedback.Comment,
		"raw_user_request":               feedback.Comment,
		"prompt_compilation":             "intent_normalization_required",
		"final_visual_validation":        true,
		"visual_rework_budget":           1,
		"delivery_mode":                  "publish",
		"target_size":                    input.SizeKey,
		"edit_sizes":                     editSizes,
		"source_revision":                input.SourceRevision,
		"source_asset_id":                sourceAssetID,
		"source_attachment_id":           sourceAttachmentID,
		"source_assets":                  sourceAssetRefs,
		"reference_asset_id":             uuidToString(assetID),
		"reference_attachment_id":        assetAttachmentID,
		"annotation_guide_attachment_id": annotationGuideAttachmentID,
		"annotation_guide_source":        "final_reference",
		"direct_edit": map[string]any{
			"adjustment_issue_id":            uuidToString(adjustmentIssueID),
			"scope":                          input.Scope,
			"source_revision":                input.SourceRevision,
			"target_size":                    input.SizeKey,
			"expected_sizes":                 expectedSizes,
			"edit_sizes":                     editSizes,
			"source_asset_id":                sourceAssetID,
			"source_attachment_id":           sourceAttachmentID,
			"source_assets":                  sourceAssetRefs,
			"reference_asset_id":             uuidToString(assetID),
			"reference_attachment_id":        assetAttachmentID,
			"direct_edit_agent_id":           uuidToString(directEditor.ID),
			"direct_edit_runtime_id":         uuidToString(directEditor.RuntimeID),
			"annotation_guide_attachment_id": annotationGuideAttachmentID,
			"annotation_guide_source":        "final_reference",
			"request":                        feedback.Comment,
			"raw_user_request":               feedback.Comment,
			"prompt_compilation":             "intent_normalization_required",
			"final_visual_validation":        true,
			"visual_rework_budget":           1,
			"annotations":                    feedbackContext["annotations"],
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare image adjustment")
		return
	}
	if err := validateCreativeTaskFanoutContext("creative_order_item_direct_edit", parseUUID(itemID), []service.DirectTaskFanoutItem{{
		ItemKey: fmt.Sprintf("%s:r%d", variantID, newRevision), Context: taskContext,
	}}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate direct image adjustment task")
		return
	}
	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_item_direct_edit"), parseUUID(itemID))
	task, err := h.Queries.WithTx(tx).CreateAgentTask(r.Context(), db.CreateAgentTaskParams{
		AgentID:              directEditor.ID,
		RuntimeID:            directEditor.RuntimeID,
		IssueID:              adjustmentIssueID,
		Priority:             3,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     userID,
		OriginatorUserID:     attr.UserID,
		AccountableUserID:    attr.AccountableUserID,
		OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
		TriggerEvidenceKind:  pgtype.Text{String: "creative_order_item_direct_edit", Valid: true},
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
		"scope":           input.Scope,
		"expected_sizes":  expectedSizes,
		"edit_sizes":      editSizes,
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
