package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	creativeQCAutomaticContractRecoveryKind   = "automatic_contract_recovery"
	creativeQCFinalizationRecoveryKind        = "finalization_commit_recovery"
	creativeQCMissingReportRecoveryKind       = "missing_report_recovery"
	creativeQCFinalizationRecoveryMaxAttempts = 2
)

// RecoverPendingCreativeQCFinalizations compensates for a terminal QC task
// that either lost qc-finalize after writing a report or incorrectly reported
// completion without persisting one. The replacement task reuses the same
// Prime assets and obtains a new attempt so it cannot overwrite or silently
// accept stale QC state.
func (h *Handler) RecoverPendingCreativeQCFinalizations(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		limit = 1
	}
	processed := 0
	for processed < limit {
		queued, err := h.recoverPendingCreativeQCFinalization(ctx)
		if err != nil {
			return processed, err
		}
		if !queued {
			return processed, nil
		}
		processed++
	}
	return processed, nil
}

func (h *Handler) recoverPendingCreativeQCFinalization(ctx context.Context) (bool, error) {
	if h.TaskService == nil {
		return false, errors.New("creative QC task service is unavailable")
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("start creative QC finalization recovery: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID, orderID, itemID, variantID, issueID, createdBy pgtype.UUID
	var revision, reportAttempt int
	var recoveryKind, triggerKind, inputSnapshot, brief string
	err = tx.QueryRow(ctx, `
WITH unresolved_qc AS (
  SELECT report.variant_id, report.revision, report.attempt, report.updated_at,
         $1::text AS recovery_kind,
         row_number() OVER (PARTITION BY report.variant_id, report.revision ORDER BY report.attempt DESC) AS row_number
  FROM creative_order_qc_report report
  WHERE report.status IN ('passed', 'warning', 'failed')
    AND NOT EXISTS (
      SELECT 1
      FROM creative_order_variant_qc_resolution resolution
      WHERE resolution.variant_id = report.variant_id
        AND resolution.revision = report.revision
        AND resolution.attempt = report.attempt
    )
  UNION ALL
  SELECT task.trigger_evidence_ref_id AS variant_id,
         COALESCE(NULLIF(task.context->>'revision', '')::int, 1) AS revision,
         COALESCE(NULLIF(task.context->>'qc_attempt', '')::int, 1) AS attempt,
         COALESCE(task.completed_at, task.created_at) AS updated_at,
         $2::text AS recovery_kind,
         row_number() OVER (
           PARTITION BY task.trigger_evidence_ref_id, COALESCE(NULLIF(task.context->>'revision', '')::int, 1)
           ORDER BY COALESCE(task.completed_at, task.created_at) DESC, task.id DESC
         ) AS row_number
  FROM agent_task_queue task
  WHERE task.trigger_evidence_kind = 'creative_order_variant_qc'
    AND task.context->>'type' = 'creative_domain_task'
    AND task.context->>'workflow' = 'creative_qc_visual'
    AND task.status IN ('completed', 'failed')
    AND task.trigger_evidence_ref_id IS NOT NULL
    AND NOT EXISTS (
      SELECT 1 FROM creative_order_qc_report report
      WHERE report.variant_id = task.trigger_evidence_ref_id
        AND report.revision = COALESCE(NULLIF(task.context->>'revision', '')::int, 1)
        AND report.attempt = COALESCE(NULLIF(task.context->>'qc_attempt', '')::int, 1)
        AND report.lane = 'visual'
    )
    AND NOT EXISTS (
      SELECT 1 FROM creative_order_variant_qc_resolution resolution
      WHERE resolution.variant_id = task.trigger_evidence_ref_id
        AND resolution.revision = COALESCE(NULLIF(task.context->>'revision', '')::int, 1)
        AND resolution.attempt = COALESCE(NULLIF(task.context->>'qc_attempt', '')::int, 1)
    )
)
SELECT order_row.workspace_id, order_row.id, item.id, variant.id, order_row.issue_id, order_row.created_by,
       variant.revision, unresolved_qc.attempt, unresolved_qc.recovery_kind, order_row.trigger_evidence_kind,
       order_row.input_snapshot::text, variant.brief::text
FROM unresolved_qc
JOIN creative_order_variant variant
  ON variant.id = unresolved_qc.variant_id AND variant.revision = unresolved_qc.revision
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE unresolved_qc.row_number = 1
  AND order_row.status <> 'cancelled'
  AND variant.status NOT IN ('cancelled', 'completed')
  AND NOT EXISTS (
    SELECT 1
    FROM agent_task_queue active
    WHERE active.context->>'type' = 'creative_domain_task'
      AND active.context->>'creative_order_id' = order_row.id::text
      AND active.context->>'variant_id' = variant.id::text
      AND COALESCE(NULLIF(active.context->>'revision', '')::int, 1) = variant.revision
      AND active.context->>'workflow' = 'creative_qc_visual'
      AND active.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
  )
  AND EXISTS (
    SELECT 1
    FROM agent_task_queue terminal
    WHERE terminal.context->>'type' = 'creative_domain_task'
      AND terminal.context->>'creative_order_id' = order_row.id::text
      AND terminal.context->>'variant_id' = variant.id::text
      AND COALESCE(NULLIF(terminal.context->>'revision', '')::int, 1) = variant.revision
      AND COALESCE(NULLIF(terminal.context->>'qc_attempt', '')::int, 1) = unresolved_qc.attempt
      AND terminal.context->>'workflow' = 'creative_qc_visual'
      AND terminal.status IN ('completed', 'failed')
  )
  AND (
    SELECT count(*)
    FROM activity_log recovery
    WHERE recovery.workspace_id = order_row.workspace_id
      AND recovery.action = 'creative_qc_recovery_queued'
      AND recovery.details->>'variant_id' = variant.id::text
      AND recovery.details->>'revision' = variant.revision::text
      AND recovery.details->>'recovery_kind' = unresolved_qc.recovery_kind
  ) < $3
ORDER BY unresolved_qc.updated_at, variant.id
FOR UPDATE OF variant, order_row SKIP LOCKED
LIMIT 1

`, creativeQCFinalizationRecoveryKind, creativeQCMissingReportRecoveryKind, creativeQCFinalizationRecoveryMaxAttempts).Scan(
		&workspaceID, &orderID, &itemID, &variantID, &issueID, &createdBy,
		&revision, &reportAttempt, &recoveryKind, &triggerKind, &inputSnapshot, &brief,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("find pending creative QC finalization: %w", err)
	}

	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, []byte(inputSnapshot), []byte(brief))
	if err != nil {
		return false, fmt.Errorf("load pending creative QC finalization sizes: %w", err)
	}
	if _, complete, err := h.creativePrimePackageComplete(ctx, variantID, revision, expectedSizes); err != nil {
		return false, err
	} else if !complete {
		return false, errors.New("pending creative QC finalization has an incomplete Prime package")
	}

	failedTaskIDs := []string{}
	rows, err := tx.Query(ctx, `
SELECT id::text
FROM agent_task_queue
WHERE context->>'type' = 'creative_domain_task'
  AND context->>'creative_order_id' = $1::text
  AND context->>'variant_id' = $2::text
  AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $3
  AND COALESCE(NULLIF(context->>'qc_attempt', '')::int, 1) = $4
  AND context->>'workflow' = 'creative_qc_visual'
  AND status IN ('completed', 'failed')
ORDER BY created_at, id
`, orderID, variantID, revision, reportAttempt)
	if err != nil {
		return false, fmt.Errorf("load pending creative QC finalization tasks: %w", err)
	}
	for rows.Next() {
		var taskID string
		if err := rows.Scan(&taskID); err != nil {
			rows.Close()
			return false, fmt.Errorf("read pending creative QC finalization tasks: %w", err)
		}
		failedTaskIDs = append(failedTaskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, fmt.Errorf("read pending creative QC finalization tasks: %w", err)
	}
	rows.Close()
	if len(failedTaskIDs) == 0 {
		return false, errors.New("pending creative QC finalization has no terminal task")
	}

	created, _, err := h.queueCreativeQCAutomaticRecovery(
		ctx, tx, workspaceID, createdBy, orderID, itemID, variantID, issueID,
		revision, []byte(inputSnapshot), expectedSizes, failedTaskIDs, recoveryKind,
	)
	if err != nil {
		return false, fmt.Errorf("queue pending creative QC finalization recovery: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		return false, fmt.Errorf("update recovered creative order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit pending creative QC finalization recovery: %w", err)
	}
	for _, task := range created {
		h.TaskService.NotifyTaskEnqueued(ctx, task)
	}
	return true, nil
}
