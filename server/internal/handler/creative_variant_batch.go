package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

func (h *Handler) listCreativeOrderVariantsBatch(r *http.Request, itemIDs []pgtype.UUID) ([]creativeOrderVariantResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT variant.id::text, variant.order_item_id::text, variant.variant_key, variant.brief::text, variant.revision, variant.status,
  COALESCE(variant.active_revision, 0), COALESCE(variant.staging_revision, 0), variant.candidate_state,
  COALESCE(variant.selection_rank, 0), variant.primary_size,
  EXISTS (
    SELECT 1
    FROM activity_log recovery
    JOIN creative_order_item recovery_item ON recovery_item.id = variant.order_item_id
    JOIN creative_order recovery_order ON recovery_order.id = recovery_item.order_id
    WHERE recovery.workspace_id = recovery_order.workspace_id
      AND recovery.issue_id = recovery_order.issue_id
      AND recovery.action = 'creative_qc_recovery_queued'
      AND recovery.details->>'variant_id' = variant.id::text
      AND recovery.details->>'revision' = variant.revision::text
  ) AS qc_recovery_used,
  (
    variant.status NOT IN ('queued', 'running', 'cancelled')
    AND recovery_order.issue_id IS NOT NULL
    AND (
      SELECT count(DISTINCT asset.size_key)
      FROM creative_order_asset asset
      WHERE asset.variant_id = variant.id
        AND asset.revision = variant.revision
        AND asset.stage = 'primed'
        AND asset.status = 'completed'
        AND asset.size_key IN (
          SELECT jsonb_array_elements_text(expected_scope.expected_sizes)
        )
    ) = jsonb_array_length(expected_scope.expected_sizes)
    AND NOT EXISTS (
      SELECT 1 FROM agent_task_queue task
      WHERE task.context->>'creative_order_id' = recovery_order.id::text
        AND task.context->>'variant_id' = variant.id::text
        AND COALESCE(NULLIF(task.context->>'revision', '')::int, 1) = variant.revision
        AND task.context->>'workflow' = 'creative_qc_visual'
        AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
    )
	) AS qc_recovery_available,
  variant.created_at::text, variant.updated_at::text
FROM creative_order_variant variant
JOIN creative_order_item recovery_item ON recovery_item.id = variant.order_item_id
JOIN creative_order recovery_order ON recovery_order.id = recovery_item.order_id
CROSS JOIN LATERAL (
  SELECT CASE
    WHEN jsonb_typeof(recovery_order.input_snapshot->'expected_sizes') = 'array'
      AND jsonb_array_length(recovery_order.input_snapshot->'expected_sizes') > 0
    THEN recovery_order.input_snapshot->'expected_sizes'
    WHEN jsonb_typeof(recovery_order.input_snapshot->'delivery_scope'->'expected_sizes') = 'array'
      AND jsonb_array_length(recovery_order.input_snapshot->'delivery_scope'->'expected_sizes') > 0
    THEN recovery_order.input_snapshot->'delivery_scope'->'expected_sizes'
    ELSE '["1080x1080","1200x628","800x1000"]'::jsonb
  END AS expected_sizes
) expected_scope
WHERE variant.order_item_id = ANY($1::uuid[]) ORDER BY variant.variant_key
`, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []creativeOrderVariantResponse{}
	for rows.Next() {
		variant, err := scanCreativeOrderVariant(rows)
		if err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if err := h.hydrateCreativeVariants(r.Context(), variants); err != nil {
		return nil, err
	}
	return variants, nil
}

const creativeVariantQCBatchSQL = `
WITH latest AS (
  SELECT v.id, v.revision,
    GREATEST(
      COALESCE((SELECT max(report.attempt) FROM creative_order_qc_report report WHERE report.variant_id = v.id AND report.revision = v.revision), 0),
      COALESCE((SELECT max(resolution.attempt) FROM creative_order_variant_qc_resolution resolution WHERE resolution.variant_id = v.id AND resolution.revision = v.revision), 0),
      COALESCE((
        SELECT max(
          COALESCE(
            CASE
              WHEN NULLIF(task.context->>'qc_attempt', '') ~ '^[0-9]+$'
                THEN (task.context->>'qc_attempt')::int
            END,
            1
          )
        )
        FROM agent_task_queue task
        WHERE task.trigger_evidence_kind = 'creative_order_variant_qc'
          AND (
            task.trigger_evidence_ref_id = v.id
            OR task.context->>'variant_id' = v.id::text
          )
          AND COALESCE(NULLIF(task.context->>'revision', '')::int, v.revision) = v.revision
      ), 0)
    ) AS attempt
  FROM creative_order_variant v
  WHERE v.id = ANY($1::uuid[])
)
SELECT latest.id::text, CASE
  WHEN count(*) FILTER (WHERE q.status = 'failed') > 0 THEN 'failed'
  WHEN count(*) FILTER (WHERE q.status = 'warning') > 0 THEN 'warning'
  WHEN count(*) FILTER (WHERE q.lane = 'visual' AND q.status = 'passed') > 0 THEN 'passed'
  ELSE 'pending'
END
FROM latest
LEFT JOIN creative_order_qc_report q ON q.variant_id = latest.id AND q.revision = latest.revision AND q.attempt = latest.attempt
GROUP BY latest.id
`
const creativeVariantBlockerBatchSQL = `
WITH latest AS (
  SELECT DISTINCT ON (variant.id) variant.id AS variant_id, q.id,
    COALESCE(q.context->>'workflow', '') AS workflow,
    CASE
      WHEN q.status = 'completed' THEN 'agent_reported_action_required'
      ELSE COALESCE(NULLIF(q.failure_reason, ''), 'agent_error')
    END AS failure_reason,
    COALESCE(q.error, '') AS error,
    COALESCE(q.result->>'output', '') AS result_output,
    COALESCE(q.completed_at, q.created_at)::text AS failed_at,
    true AS retryable
  FROM agent_task_queue q
  JOIN creative_task_binding binding ON binding.task_id=q.id
  JOIN creative_order_variant variant ON variant.id=binding.variant_id AND variant.revision=binding.revision
  WHERE variant.id = ANY($1::uuid[])
    AND q.context->>'variant_id' = variant.id::text
    AND COALESCE(NULLIF(q.context->>'revision', '')::int, variant.revision) = variant.revision
    AND q.status IN ('failed', 'completed')
    AND NOT EXISTS (
      SELECT 1
      FROM agent_task_queue active
      WHERE active.id <> q.id
        AND active.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
        AND active.context->>'type' = 'creative_domain_task'
        AND active.context->>'workflow' = q.context->>'workflow'
        AND active.context->>'variant_id' = variant.id::text
        AND COALESCE(NULLIF(active.context->>'revision', '')::int, variant.revision) = variant.revision
    )
    AND (
      q.status = 'failed'
      OR (
        q.status = 'completed'
        AND (
          (
            q.context->>'workflow' = 'creative_production'
            AND (
              SELECT count(DISTINCT asset.size_key)
              FROM creative_order_asset asset
              WHERE asset.variant_id = variant.id
                AND asset.revision = variant.revision
                AND asset.stage = 'generated'
                AND asset.status = 'completed'
                AND asset.size_key IN (
                  SELECT jsonb_array_elements_text(
                    CASE
                      WHEN jsonb_typeof(q.context->'expected_sizes') = 'array'
                        AND jsonb_array_length(q.context->'expected_sizes') > 0
                      THEN q.context->'expected_sizes'
                      ELSE '["1080x1080","1200x628","800x1000"]'::jsonb
                    END
                  )
                )
            ) < jsonb_array_length(
              CASE
                WHEN jsonb_typeof(q.context->'expected_sizes') = 'array'
                  AND jsonb_array_length(q.context->'expected_sizes') > 0
                THEN q.context->'expected_sizes'
                ELSE '["1080x1080","1200x628","800x1000"]'::jsonb
              END
            )
          )
          OR (
            q.context->>'workflow' IN ('creative_qc', 'creative_qc_visual')
            AND EXISTS (
              SELECT 1
              FROM creative_order_qc_report report
              WHERE report.variant_id = variant.id
                AND report.revision = variant.revision
                AND report.attempt = COALESCE(
                  CASE
                    WHEN NULLIF(q.context->>'qc_attempt', '') ~ '^[0-9]+$'
                      THEN (q.context->>'qc_attempt')::int
                  END,
                  1
                )
                AND report.lane = 'visual'
                AND (
                  q.context->>'workflow' = 'creative_qc_visual'
                  OR (q.context->>'workflow' = 'creative_qc' AND q.context->>'lane' = 'visual')
                )
                AND report.status = 'failed'
            )
          )
        )
      )
    )
  ORDER BY variant.id, COALESCE(q.completed_at, q.created_at) DESC, q.id DESC
)
SELECT latest.variant_id::text, latest.id::text,
  latest.workflow,
  latest.failure_reason,
  COALESCE(NULLIF(latest_message.content, ''), NULLIF(latest.error, ''), NULLIF(latest.result_output, ''), ''),
  latest.failed_at,
  latest.retryable
FROM latest
LEFT JOIN LATERAL (
 SELECT message.content FROM task_message message WHERE message.task_id=latest.id AND message.type='text' AND btrim(message.content)<>''
 ORDER BY message.seq DESC,message.id DESC LIMIT 1
) latest_message ON true
`
