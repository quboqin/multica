package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type creativeDirectEditDeliveryConfig struct {
	DeliveryMode                string
	TargetSize                  string
	Scope                       string
	SourceRevision              int
	EditSizes                   []string
	FinalVisualValidation       bool
	RawUserRequest              string
	AnnotationGuideAttachmentID string
	Annotations                 json.RawMessage
}

type creativeDirectAdjustmentDeliveryAsset struct {
	SizeKey      string
	Revision     int
	Stage        string
	AttachmentID string
	UpdatedAt    pgtype.Timestamptz
}

type creativeDirectAdjustmentDeliveryTarget struct {
	Issue          db.Issue
	TargetSize     string
	Scope          string
	SourceRevision int
	CompletedAt    pgtype.Timestamptz
}

type creativeDirectAdjustmentProcessAsset struct {
	SizeKey      string
	Revision     int
	Workflow     string
	Label        string
	Filename     string
	AttachmentID string
	UpdatedAt    pgtype.Timestamptz
}

func parseCreativeDirectEditDeliveryConfig(raw json.RawMessage) creativeDirectEditDeliveryConfig {
	var contract struct {
		DeliveryMode string `json:"delivery_mode"`
		TargetSize   string `json:"target_size"`
		Scope        string `json:"scope"`
		Delivery     struct {
			DeliveryMode                string          `json:"delivery_mode"`
			TargetSize                  string          `json:"target_size"`
			Scope                       string          `json:"scope"`
			SourceRevision              int             `json:"source_revision"`
			EditSizes                   []string        `json:"edit_sizes"`
			FinalVisualValidation       bool            `json:"final_visual_validation"`
			RawUserRequest              string          `json:"raw_user_request"`
			AnnotationGuideAttachmentID string          `json:"annotation_guide_attachment_id"`
			Annotations                 json.RawMessage `json:"annotations"`
		} `json:"creative_direct_edit_delivery"`
	}
	if json.Unmarshal(raw, &contract) != nil {
		return creativeDirectEditDeliveryConfig{}
	}
	targetSize := strings.TrimSpace(contract.Delivery.TargetSize)
	if targetSize == "" {
		targetSize = strings.TrimSpace(contract.TargetSize)
	}
	if !validCreativeAssetSize(targetSize) {
		targetSize = ""
	}
	scope := normalizeCreativeOrderAdjustmentScope(contract.Delivery.Scope)
	if scope == "" {
		scope = normalizeCreativeOrderAdjustmentScope(contract.Scope)
	}
	deliveryMode := strings.TrimSpace(contract.Delivery.DeliveryMode)
	if deliveryMode == "" {
		deliveryMode = strings.TrimSpace(contract.DeliveryMode)
	}
	return creativeDirectEditDeliveryConfig{
		DeliveryMode:                deliveryMode,
		TargetSize:                  targetSize,
		Scope:                       scope,
		SourceRevision:              contract.Delivery.SourceRevision,
		EditSizes:                   append([]string(nil), contract.Delivery.EditSizes...),
		FinalVisualValidation:       contract.Delivery.FinalVisualValidation,
		RawUserRequest:              strings.TrimSpace(contract.Delivery.RawUserRequest),
		AnnotationGuideAttachmentID: strings.TrimSpace(contract.Delivery.AnnotationGuideAttachmentID),
		Annotations:                 append(json.RawMessage(nil), contract.Delivery.Annotations...),
	}
}

// creativeDirectEditProcessingSizes narrows a size-scoped revision to the
// image the user changed. The revision itself retains its full delivery
// contract, so unchanged sizes can be carried forward without reprocessing.
func creativeDirectEditProcessingSizes(brief json.RawMessage, expectedSizes []string) ([]string, error) {
	delivery := parseCreativeDirectEditDeliveryConfig(brief)
	if !delivery.FinalVisualValidation || delivery.Scope != "size" {
		return append([]string(nil), expectedSizes...), nil
	}
	sizes := delivery.EditSizes
	if len(sizes) == 0 && delivery.TargetSize != "" {
		sizes = []string{delivery.TargetSize}
	}
	normalized, err := normalizeCreativeExpectedSizes(sizes)
	if err != nil {
		return nil, err
	}
	for _, size := range normalized {
		if !creativeSizeIsExpected(size, expectedSizes) {
			return nil, errors.New("creative direct-edit processing size is outside the delivery contract")
		}
	}
	return normalized, nil
}

func (h *Handler) notifyCreativeDirectAdjustmentDelivery(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID, revision int, targetSize string) {
	if err := h.postCreativeDirectAdjustmentDeliveryComment(ctx, workspaceID, orderID, variantID, revision, targetSize, false); err != nil {
		slog.Warn("creative direct adjustment delivery comment failed",
			"error", err,
			"workspace_id", uuidToString(workspaceID),
			"order_id", uuidToString(orderID),
			"variant_id", uuidToString(variantID),
			"revision", revision,
			"target_size", targetSize,
		)
	}
}

func (h *Handler) notifyCreativeDirectAdjustmentPreview(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID, revision int, targetSize string) {
	if err := h.postCreativeDirectAdjustmentDeliveryComment(ctx, workspaceID, orderID, variantID, revision, targetSize, true); err != nil {
		slog.Warn("creative direct adjustment preview comment failed",
			"error", err,
			"workspace_id", uuidToString(workspaceID),
			"order_id", uuidToString(orderID),
			"variant_id", uuidToString(variantID),
			"revision", revision,
			"target_size", targetSize,
		)
	}
}

func (h *Handler) postCreativeDirectAdjustmentDeliveryComment(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID, revision int, targetSize string, previewOnly bool) error {
	if h == nil || h.DB == nil || h.Queries == nil || !variantID.Valid || revision < 1 {
		return nil
	}
	targetSize = strings.TrimSpace(targetSize)
	if targetSize != "" && !validCreativeAssetSize(targetSize) {
		targetSize = ""
	}
	target, found, err := h.creativeDirectAdjustmentIssueForDelivery(ctx, workspaceID, orderID, variantID, revision)
	if err != nil || !found {
		return err
	}
	if targetSize == "" && validCreativeAssetSize(target.TargetSize) {
		targetSize = target.TargetSize
	}
	if target.Scope == "variant" {
		targetSize = ""
	}
	assets, err := h.creativeDirectAdjustmentDeliveryAssets(ctx, variantID, revision, targetSize)
	if err != nil {
		return err
	}
	var before []creativeDirectAdjustmentDeliveryAsset
	if target.SourceRevision > 0 {
		before, err = h.creativeDirectAdjustmentReferenceAssets(ctx, variantID, target.SourceRevision, targetSize)
		if err != nil {
			return err
		}
	}
	processAssets, err := h.creativeDirectAdjustmentProcessAssets(ctx, variantID, revision, targetSize)
	if err != nil {
		return err
	}
	if len(assets) == 0 && len(processAssets) == 0 {
		return nil
	}
	marker := creativeDirectAdjustmentDeliveryMarker(variantID, revision, targetSize, previewOnly)
	content := creativeDirectAdjustmentDeliveryCommentContent(revision, target, before, assets, processAssets, marker, previewOnly)
	comment, created, err := h.createCreativeDirectAdjustmentDeliveryComment(ctx, target.Issue, content, marker)
	if err != nil || !created {
		return err
	}
	if h.Bus != nil {
		h.publish(protocol.EventCommentCreated, uuidToString(target.Issue.WorkspaceID), "system", "", map[string]any{
			"comment":             commentToResponse(comment, nil, nil),
			"issue_title":         target.Issue.Title,
			"issue_assignee_type": textToPtr(target.Issue.AssigneeType),
			"issue_assignee_id":   uuidToPtr(target.Issue.AssigneeID),
			"issue_status":        target.Issue.Status,
		})
	}
	return nil
}

func (h *Handler) creativeDirectAdjustmentIssueForDelivery(ctx context.Context, workspaceID, orderID, variantID pgtype.UUID, revision int) (creativeDirectAdjustmentDeliveryTarget, bool, error) {
	workspaceFilter := workspaceID.Valid
	orderFilter := orderID.Valid
	var issueID pgtype.UUID
	var targetSize string
	var scope string
	var sourceRevision int
	var completedAt pgtype.Timestamptz
	err := h.DB.QueryRow(ctx, `
SELECT task.issue_id,
       COALESCE(NULLIF(task.context->>'target_size', ''), NULLIF(task.context->'direct_edit'->>'target_size', ''), '') AS target_size,
       COALESCE(NULLIF(task.context->>'scope', ''), NULLIF(task.context->'direct_edit'->>'scope', ''), 'size') AS scope,
       CASE
         WHEN task.context->>'source_revision' ~ '^[0-9]+$' THEN (task.context->>'source_revision')::int
         WHEN task.context->'direct_edit'->>'source_revision' ~ '^[0-9]+$' THEN (task.context->'direct_edit'->>'source_revision')::int
         ELSE 0
       END AS source_revision,
       task.completed_at
FROM agent_task_queue task
JOIN issue issue_row ON issue_row.id = task.issue_id AND issue_row.is_active = TRUE
WHERE task.issue_id IS NOT NULL
  AND task.context->>'type' = 'creative_domain_task'
  AND task.context->>'workflow' = 'creative_direct_edit'
  AND (NOT $1::boolean OR issue_row.workspace_id = $2)
  AND (NOT $3::boolean OR task.context->>'creative_order_id' = $4)
  AND task.context->>'variant_id' = $5
  AND task.context->>'revision' ~ '^[0-9]+$'
  AND (task.context->>'revision')::int = $6
  AND (
    NULLIF(task.context->>'adjustment_issue_id', '') = task.issue_id::text
    OR NULLIF(task.context->'direct_edit'->>'adjustment_issue_id', '') = task.issue_id::text
  )
ORDER BY task.created_at DESC, task.id DESC
LIMIT 1
`, workspaceFilter, workspaceID, orderFilter, uuidToString(orderID), uuidToString(variantID), revision).Scan(&issueID, &targetSize, &scope, &sourceRevision, &completedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return creativeDirectAdjustmentDeliveryTarget{}, false, nil
	}
	if err != nil {
		return creativeDirectAdjustmentDeliveryTarget{}, false, fmt.Errorf("load direct adjustment issue: %w", err)
	}
	issue, err := h.Queries.GetIssue(ctx, issueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return creativeDirectAdjustmentDeliveryTarget{}, false, nil
	}
	if err != nil {
		return creativeDirectAdjustmentDeliveryTarget{}, false, fmt.Errorf("load direct adjustment issue row: %w", err)
	}
	if workspaceFilter && issue.WorkspaceID != workspaceID {
		return creativeDirectAdjustmentDeliveryTarget{}, false, nil
	}
	return creativeDirectAdjustmentDeliveryTarget{
		Issue:          issue,
		TargetSize:     strings.TrimSpace(targetSize),
		Scope:          normalizeCreativeOrderAdjustmentScope(scope),
		SourceRevision: sourceRevision,
		CompletedAt:    completedAt,
	}, true, nil
}

func (h *Handler) creativeDirectAdjustmentDeliveryAssets(ctx context.Context, variantID pgtype.UUID, revision int, targetSize string) ([]creativeDirectAdjustmentDeliveryAsset, error) {
	rows, err := h.DB.Query(ctx, `
SELECT size_key, revision, stage, attachment_id::text, updated_at
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'delivered'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND ($3::text = '' OR size_key = $3)
ORDER BY array_position($4::text[], size_key), size_key
`, variantID, revision, targetSize, standardCreativeAssetSizes)
	if err != nil {
		return nil, fmt.Errorf("load direct adjustment delivered assets: %w", err)
	}
	defer rows.Close()
	assets := []creativeDirectAdjustmentDeliveryAsset{}
	for rows.Next() {
		var asset creativeDirectAdjustmentDeliveryAsset
		if err := rows.Scan(&asset.SizeKey, &asset.Revision, &asset.Stage, &asset.AttachmentID, &asset.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read direct adjustment delivered asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read direct adjustment delivered assets: %w", err)
	}
	return assets, nil
}

func (h *Handler) creativeDirectAdjustmentReferenceAssets(ctx context.Context, variantID pgtype.UUID, revision int, targetSize string) ([]creativeDirectAdjustmentDeliveryAsset, error) {
	rows, err := h.DB.Query(ctx, `
WITH ranked AS (
SELECT size_key, revision, stage, attachment_id::text, updated_at,
  row_number() OVER (
    PARTITION BY size_key
    ORDER BY CASE stage WHEN 'delivered' THEN 1 WHEN 'primed' THEN 2 ELSE 3 END, updated_at DESC, id DESC
  ) AS asset_rank
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND ($3::text = '' OR size_key = $3)
  AND stage = ANY(ARRAY['delivered','primed','generated']::text[])
  AND status = 'completed'
  AND attachment_id IS NOT NULL
)
SELECT size_key, revision, stage, attachment_id, updated_at
FROM ranked
WHERE asset_rank = 1
ORDER BY array_position($4::text[], size_key), size_key
`, variantID, revision, targetSize, standardCreativeAssetSizes)
	if err != nil {
		return nil, fmt.Errorf("load direct adjustment reference assets: %w", err)
	}
	defer rows.Close()
	assets := []creativeDirectAdjustmentDeliveryAsset{}
	for rows.Next() {
		var asset creativeDirectAdjustmentDeliveryAsset
		if err := rows.Scan(&asset.SizeKey, &asset.Revision, &asset.Stage, &asset.AttachmentID, &asset.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read direct adjustment reference asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read direct adjustment reference assets: %w", err)
	}
	return assets, nil
}

func (h *Handler) creativeDirectAdjustmentProcessAssets(ctx context.Context, variantID pgtype.UUID, revision int, targetSize string) ([]creativeDirectAdjustmentProcessAsset, error) {
	rows, err := h.DB.Query(ctx, `
SELECT size_key, revision, workflow, label, filename, attachment_id::text, updated_at
FROM creative_order_diagnostic_asset
WHERE variant_id = $1
  AND revision = $2
  AND attachment_id IS NOT NULL
  AND ($3::text = '' OR size_key = $3)
ORDER BY array_position($4::text[], size_key),
  CASE label
    WHEN 'Prime context' THEN 1
    WHEN '模型原图' THEN 2
    WHEN '规范化底图' THEN 3
    WHEN 'Prime 合成成图' THEN 4
    ELSE 5
  END,
  updated_at,
  id
`, variantID, revision, targetSize, standardCreativeAssetSizes)
	if err != nil {
		return nil, fmt.Errorf("load direct adjustment process assets: %w", err)
	}
	defer rows.Close()
	assets := []creativeDirectAdjustmentProcessAsset{}
	for rows.Next() {
		var asset creativeDirectAdjustmentProcessAsset
		if err := rows.Scan(&asset.SizeKey, &asset.Revision, &asset.Workflow, &asset.Label, &asset.Filename, &asset.AttachmentID, &asset.UpdatedAt); err != nil {
			return nil, fmt.Errorf("read direct adjustment process asset: %w", err)
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read direct adjustment process assets: %w", err)
	}
	return assets, nil
}

func creativeDirectAdjustmentDeliveryMarker(variantID pgtype.UUID, revision int, targetSize string, previewOnly bool) string {
	if targetSize == "" {
		targetSize = "all"
	}
	if previewOnly {
		return fmt.Sprintf("<!-- multica:creative-direct-adjustment-delivery:%s:r%d:%s:preview -->", uuidToString(variantID), revision, targetSize)
	}
	return fmt.Sprintf("<!-- multica:creative-direct-adjustment-delivery:%s:r%d:%s -->", uuidToString(variantID), revision, targetSize)
}

func creativeDirectAdjustmentDeliveryCommentContent(revision int, target creativeDirectAdjustmentDeliveryTarget, before []creativeDirectAdjustmentDeliveryAsset, assets []creativeDirectAdjustmentDeliveryAsset, processAssets []creativeDirectAdjustmentProcessAsset, marker string, previewOnly bool) string {
	var b strings.Builder
	if previewOnly {
		b.WriteString("调整后的预览图已生成，但未通过交付验收，尚未替换当前正式成图。\n\n")
	} else {
		b.WriteString("调整后的图片已生成。\n\n")
	}
	completedAt := target.CompletedAt
	if !completedAt.Valid && len(assets) > 0 {
		completedAt = assets[0].UpdatedAt
	}
	fmt.Fprintf(&b, "完成时间：%s\n\n", creativeDirectAdjustmentTimeLabel(completedAt))
	if target.Scope == "variant" {
		b.WriteString("调整范围：当前变体三尺寸\n\n")
	} else if target.TargetSize != "" {
		fmt.Fprintf(&b, "调整范围：%s\n\n", creativeDirectAdjustmentSizeLabel(target.TargetSize))
	}
	for _, asset := range before {
		fmt.Fprintf(&b, "**调整前原图** · %s · r%d · %s\n\n", creativeDirectAdjustmentSizeLabel(asset.SizeKey), asset.Revision, creativeDirectAdjustmentTimeLabel(asset.UpdatedAt))
		fmt.Fprintf(&b, "![调整前原图 %s r%d](%s)\n\n", asset.SizeKey, asset.Revision, attachmentDownloadPath(asset.AttachmentID))
	}
	for _, asset := range assets {
		fmt.Fprintf(&b, "**调整后结果** · %s · r%d · %s\n\n", creativeDirectAdjustmentSizeLabel(asset.SizeKey), revision, creativeDirectAdjustmentTimeLabel(asset.UpdatedAt))
		fmt.Fprintf(&b, "![调整后结果 %s r%d](%s)\n\n", asset.SizeKey, revision, attachmentDownloadPath(asset.AttachmentID))
	}
	if len(processAssets) > 0 {
		b.WriteString("**本次过程图片**\n\n")
		for _, asset := range processAssets {
			fmt.Fprintf(&b, "%s · %s · %s\n\n", creativeDirectAdjustmentProcessTitle(asset), creativeDirectAdjustmentSizeLabel(asset.SizeKey), creativeDirectAdjustmentTimeLabel(asset.UpdatedAt))
			fmt.Fprintf(&b, "![过程图片 %s %s r%d](%s)\n\n", asset.Label, asset.SizeKey, asset.Revision, attachmentDownloadPath(asset.AttachmentID))
		}
	}
	b.WriteString(marker)
	return b.String()
}

func creativeDirectAdjustmentSizeLabel(size string) string {
	switch size {
	case "1080x1080":
		return "方形 1080x1080"
	case "1200x628":
		return "横版 1200x628"
	case "800x1000":
		return "竖版 800x1000"
	default:
		return size
	}
}

func creativeDirectAdjustmentProcessTitle(asset creativeDirectAdjustmentProcessAsset) string {
	switch asset.Label {
	case "Prime context":
		return "调整前上下文"
	case "模型原图":
		return "模型回图"
	case "规范化底图":
		return "规范化后底图"
	case "Prime 合成成图":
		return "品牌贴片后结果"
	}
	if strings.TrimSpace(asset.Label) != "" {
		return asset.Label
	}
	return asset.Workflow
}

func creativeDirectAdjustmentTimeLabel(value pgtype.Timestamptz) string {
	if !value.Valid || value.Time.IsZero() {
		return "时间未知"
	}
	return value.Time.In(time.FixedZone("CST", 8*60*60)).Format("2006/01/02 15:04:05") + "（北京时间）"
}

func (h *Handler) createCreativeDirectAdjustmentDeliveryComment(ctx context.Context, issue db.Issue, content, marker string) (db.Comment, bool, error) {
	if h.TxStarter == nil {
		return db.Comment{}, false, errors.New("database transaction starter is unavailable")
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.Comment{}, false, fmt.Errorf("start direct adjustment comment: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::uuid::text, 0))`, issue.ID); err != nil {
		return db.Comment{}, false, fmt.Errorf("lock direct adjustment issue comments: %w", err)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM comment
  WHERE issue_id = $1
    AND workspace_id = $2
    AND is_active = TRUE
    AND content LIKE '%' || $3 || '%'
)
`, issue.ID, issue.WorkspaceID, marker).Scan(&exists); err != nil {
		return db.Comment{}, false, fmt.Errorf("check direct adjustment delivery comment: %w", err)
	}
	if exists {
		if err := tx.Commit(ctx); err != nil {
			return db.Comment{}, false, fmt.Errorf("finish direct adjustment comment check: %w", err)
		}
		return db.Comment{}, false, nil
	}
	comment, err := h.Queries.WithTx(tx).CreateComment(ctx, db.CreateCommentParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
		AuthorType:  "system",
		AuthorID:    pgtype.UUID{Valid: true},
		Content:     content,
		Type:        "system",
		ParentID:    pgtype.UUID{Valid: false},
	})
	if err != nil {
		return db.Comment{}, false, fmt.Errorf("create direct adjustment delivery comment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Comment{}, false, fmt.Errorf("save direct adjustment delivery comment: %w", err)
	}
	return comment, true, nil
}
