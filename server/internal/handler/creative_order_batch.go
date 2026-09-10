package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

const creativeOrderFailuresBatchSQL = `
WITH ranked AS (
  SELECT q.*, binding.order_id AS binding_order_id,
    CASE
      WHEN jsonb_typeof(q.context->'expected_sizes') = 'array'
        AND jsonb_array_length(q.context->'expected_sizes') > 0
      THEN q.context->'expected_sizes'
      ELSE '["1080x1080","1200x628","800x1000"]'::jsonb
    END AS workflow_expected_sizes,
    row_number() OVER (
      PARTITION BY q.agent_id,
        COALESCE(q.trigger_evidence_kind, ''),
        COALESCE(q.trigger_evidence_ref_id::text, ''),
        COALESCE(q.context->>'item_key', '')
      ORDER BY q.created_at DESC, q.id DESC
    ) AS row_number
  FROM agent_task_queue q
  JOIN creative_task_binding binding ON binding.task_id=q.id
  WHERE binding.order_id=ANY($1::uuid[])
    AND COALESCE(q.context->>'superseded_by_process_result','false')<>'true'
)
SELECT binding_order_id::text, id::text,
  agent_id::text,
  COALESCE(context->>'workflow', ''),
  COALESCE(NULLIF(context->>'scope', ''),
    CASE
      WHEN NULLIF(context->>'variant_id', '') IS NOT NULL THEN 'variant'
      WHEN NULLIF(context->>'creative_order_item_id', '') IS NOT NULL THEN 'order_item'
      ELSE 'order'
    END),
  COALESCE(NULLIF(context->>'subject_id', ''),
    CASE COALESCE(NULLIF(context->>'scope', ''),
      CASE
        WHEN NULLIF(context->>'variant_id', '') IS NOT NULL THEN 'variant'
        WHEN NULLIF(context->>'creative_order_item_id', '') IS NOT NULL THEN 'order_item'
        ELSE 'order'
      END)
      WHEN 'variant' THEN NULLIF(context->>'variant_id', '')
      WHEN 'order_item' THEN NULLIF(context->>'creative_order_item_id', '')
      WHEN 'order' THEN NULLIF(context->>'creative_order_id', '')
      ELSE NULL
    END,
    NULLIF(context->>'variant_id', ''),
    NULLIF(context->>'creative_order_item_id', ''),
    NULLIF(context->>'creative_order_id', ''),
    COALESCE(trigger_evidence_ref_id::text, '')),
  COALESCE(context->>'item_key', ''),
  COALESCE(trigger_evidence_kind, ''),
  COALESCE(trigger_evidence_ref_id::text, ''),
  CASE
    WHEN status = 'completed' THEN 'agent_reported_action_required'
    ELSE COALESCE(NULLIF(failure_reason, ''), 'agent_error')
  END,
  CASE
    WHEN status = 'completed' THEN COALESCE((
      SELECT message.content
      FROM task_message message
      WHERE message.task_id = ranked.id
        AND message.type = 'text'
        AND btrim(message.content) <> ''
      ORDER BY message.seq DESC, message.id DESC
      LIMIT 1
    ), '')
    ELSE COALESCE(error, '')
  END,
  COALESCE(completed_at, created_at)::text,
  true
FROM ranked
WHERE row_number = 1
AND (
  NULLIF(context->>'variant_id', '') IS NULL
  OR EXISTS (
    SELECT 1
    FROM creative_order_variant variant
    JOIN creative_order_item item ON item.id = variant.order_item_id
    WHERE variant.id::text = context->>'variant_id'
      AND item.order_id = ranked.binding_order_id
      AND COALESCE(NULLIF(NULLIF(context->>'revision', '')::int, 0), variant.revision) = variant.revision
  )
)
AND (
  status = 'failed'
  OR (
    status = 'completed'
    AND NULLIF(context->>'variant_id', '') IS NOT NULL
    AND EXISTS(
      SELECT 1
      FROM creative_order_variant variant
      JOIN creative_order_item item ON item.id = variant.order_item_id
      WHERE variant.id::text = context->>'variant_id'
        AND item.order_id = ranked.binding_order_id
        AND variant.status NOT IN ('completed', 'cancelled')
        AND (
          (
            COALESCE(context->>'workflow', '') = 'creative_production'
            AND (
              SELECT count(DISTINCT asset.size_key)
              FROM creative_order_asset asset
              WHERE asset.variant_id = variant.id
                AND asset.revision = variant.revision
                AND asset.stage = 'generated'
                AND asset.status = 'completed'
                AND asset.size_key IN (
                  SELECT jsonb_array_elements_text(workflow_expected_sizes)
                )
            ) < jsonb_array_length(workflow_expected_sizes)
          )
          OR (
            COALESCE(context->>'workflow', '') IN ('creative_qc', 'creative_qc_visual')
            AND EXISTS (
              SELECT 1
              FROM creative_order_qc_report report
              WHERE report.variant_id = variant.id
                AND report.revision = variant.revision
                AND report.attempt = COALESCE(
                  CASE
                    WHEN NULLIF(context->>'qc_attempt', '') ~ '^[0-9]+$'
                      THEN (context->>'qc_attempt')::int
                  END,
                  1
                )
                AND report.lane = 'visual'
                AND (
                  context->>'workflow' = 'creative_qc_visual'
                  OR (context->>'workflow' = 'creative_qc' AND context->>'lane' = 'visual')
                )
                AND report.status = 'failed'
            )
          )
        )
    )
  )
)
ORDER BY COALESCE(completed_at, created_at), id
`

func (h *Handler) creativeOrderFailuresBatch(ctx context.Context, ids []pgtype.UUID) (map[string][]creativeOrderWorkflowFailureResponse, error) {
	result := make(map[string][]creativeOrderWorkflowFailureResponse, len(ids))
	rows, err := h.DB.Query(ctx, creativeOrderFailuresBatchSQL, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var f creativeOrderWorkflowFailureResponse
		if err := rows.Scan(&id, &f.TaskID, &f.AgentID, &f.Workflow, &f.Scope, &f.SubjectID, &f.ItemKey, &f.TriggerEvidenceKind, &f.TriggerEvidenceReference, &f.FailureReason, &f.Error, &f.FailedAt, &f.Retryable); err != nil {
			return nil, err
		}
		result[id] = append(result[id], f)
	}
	return result, rows.Err()
}

const creativeOrderProductionBatchSQL = `
WITH aggregate AS (
  SELECT o.id AS order_id, count(v.id) AS total,
	COALESCE(max(o.status), 'draft') AS order_status,
    count(*) FILTER (WHERE v.candidate_state = 'candidate') AS candidate_count,
    count(*) FILTER (WHERE v.candidate_state = 'selected') AS selected_count,
    count(*) FILTER (WHERE v.status = 'failed') AS failed,
    count(*) FILTER (WHERE v.status = 'action_required') AS action_required,
    count(*) FILTER (WHERE v.status = 'running') AS running,
    count(*) FILTER (WHERE v.status = 'partial') AS partial,
    count(*) FILTER (WHERE v.status = 'completed') AS completed,
    count(*) FILTER (WHERE v.status = 'cancelled') AS cancelled,
    (SELECT count(*)
      FROM agent_task_queue q JOIN creative_task_binding binding ON binding.task_id=q.id
      WHERE binding.order_id=o.id
        AND q.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')) AS active_tasks,
    (SELECT count(*)
      FROM agent_task_queue q JOIN creative_task_binding binding ON binding.task_id=q.id
      WHERE binding.order_id=o.id
        AND q.status IN ('dispatched', 'running', 'waiting_local_directory')) AS started_tasks
    ,(SELECT count(*)
      FROM creative_prime_composition_job job
      JOIN creative_order_variant prime_variant ON prime_variant.id = job.variant_id
      JOIN creative_order_item prime_item ON prime_item.id = prime_variant.order_item_id
      WHERE prime_item.order_id = o.id
        AND job.status IN ('queued', 'running')) AS active_prime_jobs,
    (SELECT count(*)
      FROM creative_prime_composition_job job
      JOIN creative_order_variant prime_variant ON prime_variant.id = job.variant_id
      JOIN creative_order_item prime_item ON prime_item.id = prime_variant.order_item_id
      WHERE prime_item.order_id = o.id
        AND job.status = 'running') AS started_prime_jobs
  FROM creative_order o
  LEFT JOIN creative_order_item i ON i.order_id = o.id
	  LEFT JOIN creative_order_variant v ON v.order_item_id = i.id AND v.candidate_state IN ('candidate', 'selected')
  WHERE o.id = ANY($1::uuid[]) GROUP BY o.id
), base AS (
  SELECT aggregate.*,
    CASE
      WHEN order_status = 'cancelled' THEN 'cancelled'
		WHEN active_tasks + active_prime_jobs > 0 AND (completed > 0 OR action_required > 0 OR failed > 0 OR partial > 0) THEN 'partial'
		WHEN active_tasks + active_prime_jobs > 0 AND started_tasks + started_prime_jobs > 0 THEN 'running'
		WHEN active_tasks + active_prime_jobs > 0 THEN 'queued'
		WHEN action_required > 0 OR failed > 0 OR partial > 0 OR running > 0 THEN 'action_required'
		WHEN candidate_count > 0 AND selected_count = 0 AND completed = total THEN 'awaiting_selection'
		WHEN total > 0 AND completed = total THEN 'completed'
		WHEN total > 0 AND cancelled = total THEN 'cancelled'
		WHEN total = 0 AND order_status IN ('failed', 'action_required') THEN 'action_required'
		WHEN total = 0 AND order_status = 'draft' THEN 'draft'
		WHEN total > 0 AND (completed > 0 OR cancelled > 0) THEN 'partial'
		WHEN total > 0 THEN 'action_required'
		ELSE 'queued'
	END AS status
  FROM aggregate
)
SELECT order_id::text, CASE
	WHEN NOT (order_id = ANY($2::uuid[])) THEN status
	WHEN status IN ('completed', 'awaiting_adoption', 'cancelled') THEN status
	WHEN active_tasks + active_prime_jobs > 0 THEN 'partial'
	ELSE 'action_required'
END FROM base
`

func (h *Handler) creativeOrderProductionBatch(ctx context.Context, ids, failed []pgtype.UUID) (map[string]string, error) {
	result := make(map[string]string, len(ids))
	rows, err := h.DB.Query(ctx, creativeOrderProductionBatchSQL, ids, failed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		result[id] = status
	}
	return result, rows.Err()
}

const creativeOrderDeliveryBatchSQL = `
WITH item_delivery AS (
  SELECT item.id, item.order_id,
    item.adopted_variant_id IS NOT NULL AS adopted,
    bool_or(variant.candidate_state <> 'selected' OR variant.selection_rank IS NOT NULL) AS candidate_pipeline,
    count(*) FILTER (WHERE variant.candidate_state = 'selected') AS selected_count,
    count(*) FILTER (WHERE variant.candidate_state = 'selected' AND variant.active_revision IS NOT NULL) AS active_reference_count,
    count(*) FILTER (
      WHERE variant.candidate_state = 'selected'
        AND variant.active_revision IS NOT NULL
        AND EXISTS (
          SELECT 1
          FROM creative_order_variant_revision active_revision
          WHERE active_revision.variant_id = variant.id
            AND active_revision.revision = variant.active_revision
            AND (
              SELECT count(DISTINCT asset.size_key)
              FROM creative_order_asset asset
              WHERE asset.variant_id = variant.id
                AND asset.revision = active_revision.revision
                AND asset.stage = 'delivered'
                AND asset.status = 'completed'
                AND asset.attachment_id IS NOT NULL
                AND asset.size_key = ANY(active_revision.expected_sizes)
            ) = cardinality(active_revision.expected_sizes)
        )
    ) AS active_selected_count
  FROM creative_order_item item
  LEFT JOIN creative_order_variant variant ON variant.order_item_id = item.id
  WHERE item.order_id=ANY($1::uuid[])
  GROUP BY item.id, item.adopted_variant_id
), aggregate AS (
  SELECT order_row.id AS order_id, order_row.status AS order_status,
    order_row.trigger_evidence_kind,
    count(item_delivery.id) AS item_total,
    count(*) FILTER (WHERE item_delivery.adopted) AS adopted_items,
    count(*) FILTER (WHERE item_delivery.active_selected_count > 0) AS items_with_delivery,
    count(*) FILTER (WHERE item_delivery.active_reference_count > item_delivery.active_selected_count) AS incomplete_active_items,
    count(*) FILTER (WHERE CASE
      WHEN item_delivery.candidate_pipeline THEN item_delivery.selected_count = (targets.value)::int AND item_delivery.active_selected_count = (targets.value)::int
      ELSE item_delivery.active_selected_count > 0
    END) AS ready_items
  FROM creative_order order_row
  JOIN jsonb_each_text($2::jsonb) targets ON targets.key=order_row.id::text
  LEFT JOIN item_delivery ON item_delivery.order_id=order_row.id
  WHERE order_row.id=ANY($1::uuid[])
  GROUP BY order_row.id, targets.value
)
SELECT order_id::text, CASE
  WHEN order_status = 'cancelled' THEN 'cancelled'
  WHEN item_total > 0 AND adopted_items = item_total AND ready_items = item_total THEN 'completed'
  WHEN item_total > 0 AND ready_items = item_total AND trigger_evidence_kind = 'creative_direct_edit' THEN 'completed'
  WHEN item_total > 0 AND ready_items = item_total THEN 'awaiting_adoption'
  WHEN ready_items > 0 OR incomplete_active_items > 0 OR items_with_delivery > 0 THEN 'partial'
  ELSE 'pending'
END
FROM aggregate
`

func (h *Handler) creativeOrderDeliveryBatch(ctx context.Context, orders []creativeOrderResponse) (map[string]string, error) {
	ids := make([]pgtype.UUID, 0, len(orders))
	targets := map[string]int{}
	for _, o := range orders {
		counts, err := creativeOrderVariantCounts(o.InputSnapshot)
		if err != nil {
			return nil, err
		}
		ids = append(ids, parseUUID(o.ID))
		targets[o.ID] = counts.Target
	}
	encoded, err := json.Marshal(targets)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string, len(ids))
	rows, err := h.DB.Query(ctx, creativeOrderDeliveryBatchSQL, ids, encoded)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		result[id] = status
	}
	return result, rows.Err()
}
func (h *Handler) loadCreativeOrderWorkflowStates(ctx context.Context, orders []creativeOrderResponse) error {
	ids := make([]pgtype.UUID, 0, len(orders))
	for _, o := range orders {
		ids = append(ids, parseUUID(o.ID))
	}
	if len(ids) == 0 {
		return nil
	}
	failures, err := h.creativeOrderFailuresBatch(ctx, ids)
	if err != nil {
		return err
	}
	failed := []pgtype.UUID{}
	for id, values := range failures {
		if len(values) > 0 {
			failed = append(failed, parseUUID(id))
		}
	}
	production, err := h.creativeOrderProductionBatch(ctx, ids, failed)
	if err != nil {
		return err
	}
	delivery, err := h.creativeOrderDeliveryBatch(ctx, orders)
	if err != nil {
		return err
	}
	for i := range orders {
		o := &orders[i]
		o.WorkflowFailures = failures[o.ID]
		if o.WorkflowFailures == nil {
			o.WorkflowFailures = []creativeOrderWorkflowFailureResponse{}
		}
		o.ProductionStatus = production[o.ID]
		o.DeliveryStatus = delivery[o.ID]
		o.DerivedStatus = o.ProductionStatus
		if o.DeliveryStatus == "completed" || o.DeliveryStatus == "awaiting_adoption" {
			o.DerivedStatus = o.DeliveryStatus
		} else if o.DeliveryStatus == "partial" && o.ProductionStatus == "completed" {
			o.DerivedStatus = "partial"
		}
	}
	return nil
}
func (h *Handler) listCreativeOrderListItemsBatch(r *http.Request, orderIDs []pgtype.UUID) ([]creativeOrderItemResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT i.id::text, i.order_id::text, COALESCE(i.candidate_id::text, ''),
  COALESCE(i.source_analysis_id::text, ''), i.copy_snapshot::text, i.direction, i.status,
  COALESCE(i.adopted_variant_id::text, ''), COALESCE(i.adopted_at::text, ''),
  COALESCE(i.adopted_by::text, ''), i.created_at::text, i.updated_at::text, i.source_kind, COALESCE(i.copy_library_id::text, ''),
	  COALESCE(v.id::text, ''), COALESCE(v.order_item_id::text, ''), COALESCE(v.variant_key, ''),
	  COALESCE(v.brief::text, '{}'), COALESCE(v.revision, 0), COALESCE(v.status, ''),
	  COALESCE(v.active_revision, 0), COALESCE(v.staging_revision, 0), COALESCE(v.candidate_state, ''),
	  COALESCE(v.selection_rank, 0), COALESCE(v.primary_size, ''),
	  COALESCE(v.created_at::text, ''), COALESCE(v.updated_at::text, ''),
	  COALESCE((
	    SELECT jsonb_agg(
	      jsonb_build_object(
	        'revision', target.revision,
	        'brief', target.brief,
	        'status', target.status,
	        'expected_sizes', target.expected_sizes,
	        'activated_at', COALESCE(target.activated_at::text, ''),
	        'created_at', target.created_at::text,
	        'updated_at', target.updated_at::text
	      ) ORDER BY target.revision
	    )
	    FROM creative_order_variant_revision target
	    WHERE target.variant_id = v.id
	      AND target.revision IN (v.active_revision, v.staging_revision)
	  ), '[]'::jsonb)::text,
  COALESCE(a.id::text, ''), COALESCE(a.variant_id::text, ''), COALESCE(a.asset_family_id::text, ''),
	  COALESCE(a.size_key, ''), COALESCE(a.revision, 0), COALESCE(a.stage, ''),
	  COALESCE(a.attachment_id::text, ''), COALESCE(a.derived_from_asset_id::text, ''),
	  COALESCE(a.operation_id::text, ''),
	  COALESCE(a.metadata::text, '{}'), COALESCE(a.evidence::text, '{}'), COALESCE(a.status, ''),
  COALESCE(a.created_at::text, ''), COALESCE(a.updated_at::text, '')
FROM creative_order_item i
LEFT JOIN creative_order_variant v ON v.order_item_id = i.id
LEFT JOIN creative_order_asset a ON a.variant_id = v.id
WHERE i.order_id = ANY($1::uuid[])
ORDER BY i.created_at, v.variant_key, a.revision, a.size_key, a.stage
`, orderIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type listItem struct {
		item     creativeOrderItemResponse
		variants []*creativeOrderVariantResponse
		byID     map[string]*creativeOrderVariantResponse
	}
	items := make([]*listItem, 0)
	byItemID := make(map[string]*listItem)
	for rows.Next() {
		var item creativeOrderItemResponse
		var itemSnapshot string
		var variantID, variantItemID, variantKey, variantBrief, variantStatus, variantCreatedAt, variantUpdatedAt string
		var variantRevision, variantActiveRevision, variantStagingRevision, variantSelectionRank int
		var variantCandidateState, variantPrimarySize, variantRevisionsJSON string
		var assetID, assetVariantID, assetFamilyID, assetSizeKey, assetStage, assetAttachmentID string
		var assetDerivedFromID, assetOperationID, assetMetadata, assetEvidence, assetStatus, assetCreatedAt, assetUpdatedAt string
		var assetRevision int
		if err := rows.Scan(
			&item.ID, &item.OrderID, &item.CandidateID, &item.SourceAnalysisID, &itemSnapshot,
			&item.Direction, &item.Status, &item.AdoptedVariantID, &item.AdoptedAt, &item.AdoptedBy,
			&item.CreatedAt, &item.UpdatedAt, &item.SourceKind, &item.CopyLibraryID,
			&variantID, &variantItemID, &variantKey, &variantBrief, &variantRevision, &variantStatus,
			&variantActiveRevision, &variantStagingRevision, &variantCandidateState, &variantSelectionRank, &variantPrimarySize,
			&variantCreatedAt, &variantUpdatedAt, &variantRevisionsJSON,
			&assetID, &assetVariantID, &assetFamilyID, &assetSizeKey, &assetRevision, &assetStage,
			&assetAttachmentID, &assetDerivedFromID, &assetOperationID, &assetMetadata, &assetEvidence, &assetStatus,
			&assetCreatedAt, &assetUpdatedAt,
		); err != nil {
			return nil, err
		}
		item.CopySnapshot = json.RawMessage(itemSnapshot)
		entry := byItemID[item.ID]
		if entry == nil {
			entry = &listItem{item: item, byID: make(map[string]*creativeOrderVariantResponse)}
			byItemID[item.ID] = entry
			items = append(items, entry)
		}
		if variantID == "" {
			continue
		}
		variant := entry.byID[variantID]
		if variant == nil {
			var targetRevisions []creativeOrderVariantRevision
			if err := json.Unmarshal([]byte(variantRevisionsJSON), &targetRevisions); err != nil {
				return nil, fmt.Errorf("decode creative order list revisions: %w", err)
			}
			variant = &creativeOrderVariantResponse{
				ID: variantID, OrderItemID: variantItemID, VariantKey: variantKey,
				Brief: json.RawMessage(variantBrief), Revision: variantRevision, Status: variantStatus,
				ActiveRevision: variantActiveRevision, StagingRevision: variantStagingRevision,
				CandidateState: variantCandidateState, SelectionRank: variantSelectionRank, PrimarySize: variantPrimarySize,
				QCStatus: "pending", Assets: []creativeOrderAssetResponse{}, Revisions: targetRevisions,
				DiagnosticAssets: []creativeOrderDiagnosticAsset{}, QCReports: []creativeOrderQCReportResponse{},
				CreatedAt: variantCreatedAt, UpdatedAt: variantUpdatedAt,
			}
			entry.byID[variantID] = variant
			entry.variants = append(entry.variants, variant)
		}
		if assetID != "" {
			variant.Assets = append(variant.Assets, creativeOrderAssetResponse{
				ID: assetID, VariantID: assetVariantID, AssetFamilyID: assetFamilyID,
				SizeKey: assetSizeKey, Revision: assetRevision, Stage: assetStage,
				AttachmentID: assetAttachmentID, DerivedFromAssetID: assetDerivedFromID,
				OperationID: assetOperationID,
				Metadata:    json.RawMessage(assetMetadata), Evidence: json.RawMessage(assetEvidence),
				Status: assetStatus, CreatedAt: assetCreatedAt, UpdatedAt: assetUpdatedAt,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]creativeOrderItemResponse, 0, len(items))
	for _, entry := range items {
		entry.item.Variants = make([]creativeOrderVariantResponse, 0, len(entry.variants))
		for _, variant := range entry.variants {
			entry.item.Variants = append(entry.item.Variants, *variant)
		}
		result = append(result, entry.item)
	}
	return result, nil
}
