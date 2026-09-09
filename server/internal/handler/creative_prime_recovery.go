package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const creativePrimeCompositionLease = 12 * time.Minute

type creativePrimeCompositionClaim struct {
	VariantID        pgtype.UUID
	Revision         int
	WorkspaceID      pgtype.UUID
	OrderID          pgtype.UUID
	OrderItemID      pgtype.UUID
	RequestedBy      pgtype.UUID
	LeaseToken       pgtype.UUID
	CompositionReady bool
	CandidateState   string
}

type creativePrimeComposeTaskFence struct {
	TaskID  pgtype.UUID
	AgentID pgtype.UUID
}

type creativePrimeComposeTaskFenceContextKey struct{}

type creativePrimeTaskAuthorizationError struct {
	message string
}

func (e *creativePrimeTaskAuthorizationError) Error() string {
	if e == nil || strings.TrimSpace(e.message) == "" {
		return "task is not authorized for this creative Prime composition"
	}
	return e.message
}

func (h *Handler) queueCreativePrimeComposition(
	ctx context.Context,
	workspaceID, orderID, variantID pgtype.UUID,
	force bool,
) (int, string, bool, error) {
	var revision, activeRevision int
	var triggerKind, inputSnapshot, brief, candidateState, primarySize string
	var variantStatus, orderStatus string
	if err := h.DB.QueryRow(ctx, `
SELECT variant.revision, COALESCE(variant.active_revision, 0),
       order_row.trigger_evidence_kind, order_row.input_snapshot::text,
       variant.brief::text, variant.candidate_state, variant.primary_size,
       variant.status, order_row.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
`, variantID, orderID, workspaceID).Scan(
		&revision, &activeRevision, &triggerKind, &inputSnapshot, &brief,
		&candidateState, &primarySize, &variantStatus, &orderStatus,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", false, errors.New("creative variant does not belong to this order")
		}
		return 0, "", false, fmt.Errorf("load creative Prime job input: %w", err)
	}
	if variantStatus == "cancelled" || orderStatus == "cancelled" {
		return revision, "cancelled", false, &creativePrimeCancelledError{}
	}
	if activeRevision == revision {
		return revision, "completed", false, &creativePrimeImmutableRevisionError{message: "active creative revision is immutable; create a staging revision before recomposing Prime"}
	}
	expectedSizes, err := expectedCreativeVariantProductionSizes(
		triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief), candidateState, primarySize,
	)
	if err != nil {
		return revision, "", false, err
	}
	generated, generatedComplete, err := h.loadCreativePrimeGeneratedAssets(ctx, variantID, revision, expectedSizes)
	if err != nil || !generatedComplete {
		return revision, "", generatedComplete, err
	}
	inputFingerprint := creativePrimeGeneratedFingerprint(generated)

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return revision, "", true, fmt.Errorf("begin creative Prime job enqueue: %w", err)
	}
	defer tx.Rollback(ctx)
	var taskRevision int
	if fence, taskActor := ctx.Value(creativePrimeComposeTaskFenceContextKey{}).(creativePrimeComposeTaskFence); taskActor {
		var taskStatus, taskContextRaw string
		if err := tx.QueryRow(ctx, `
SELECT status, context::text
FROM agent_task_queue
WHERE id = $1 AND agent_id = $2
FOR UPDATE
`, fence.TaskID, fence.AgentID).Scan(&taskStatus, &taskContextRaw); err != nil {
			return revision, "", true, &creativePrimeTaskAuthorizationError{}
		}
		var taskContext struct {
			Type            string `json:"type"`
			Workflow        string `json:"workflow"`
			CreativeOrderID string `json:"creative_order_id"`
			VariantID       string `json:"variant_id"`
			Revision        int    `json:"revision"`
		}
		if json.Unmarshal([]byte(taskContextRaw), &taskContext) != nil ||
			(taskStatus != "dispatched" && taskStatus != "running") ||
			taskContext.Type != "creative_domain_task" ||
			(taskContext.Workflow != "creative_production" && taskContext.Workflow != "creative_direct_edit") ||
			taskContext.CreativeOrderID != uuidToString(orderID) ||
			taskContext.VariantID != uuidToString(variantID) || taskContext.Revision < 1 {
			return revision, "", true, &creativePrimeTaskAuthorizationError{}
		}
		taskRevision = taskContext.Revision
	}
	var lockedOrderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status, trigger_evidence_kind, input_snapshot::text
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&lockedOrderStatus, &triggerKind, &inputSnapshot); err != nil {
		return revision, "", true, fmt.Errorf("lock creative Prime order: %w", err)
	}
	if lockedOrderStatus == "cancelled" {
		return revision, "cancelled", true, &creativePrimeCancelledError{}
	}
	var lockedRevision, lockedActiveRevision int
	var lockedVariantStatus string
	if err := tx.QueryRow(ctx, `
SELECT variant.revision, COALESCE(variant.active_revision, 0), variant.status,
       variant.brief::text, variant.candidate_state, variant.primary_size
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant
`, variantID, orderID).Scan(
		&lockedRevision, &lockedActiveRevision, &lockedVariantStatus,
		&brief, &candidateState, &primarySize,
	); err != nil {
		return revision, "", true, fmt.Errorf("lock creative Prime job input: %w", err)
	}
	if lockedRevision != revision {
		return revision, "", true, &creativePrimeImmutableRevisionError{message: "creative variant revision changed before Prime composition was queued"}
	}
	if taskRevision > 0 && taskRevision != lockedRevision {
		return revision, "", true, &creativePrimeTaskAuthorizationError{message: "task revision is not authorized for this creative Prime composition"}
	}
	if lockedVariantStatus == "cancelled" {
		return revision, "cancelled", true, &creativePrimeCancelledError{}
	}
	if lockedActiveRevision == revision {
		return revision, "completed", true, &creativePrimeImmutableRevisionError{message: "active creative revision is immutable; create a staging revision before recomposing Prime"}
	}
	var previousJobStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_prime_composition_job WHERE variant_id=$1 AND revision=$2`, variantID, revision).Scan(&previousJobStatus); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return revision, "", true, err
	}
	var jobStatus string
	if force {
		jobStatus, err = resetCreativePrimeCompositionJob(ctx, tx, variantID, revision, expectedSizes, inputFingerprint)
	} else {
		jobStatus, err = enqueueCreativePrimeCompositionJob(ctx, tx, variantID, revision, expectedSizes, inputFingerprint)
	}
	if err != nil {
		return revision, "", true, err
	}
	if err := syncCreativeVariantRevisionFromVariant(ctx, tx, variantID, revision, expectedSizes); err != nil {
		return revision, "", true, fmt.Errorf("sync queued creative Prime revision: %w", err)
	}
	if jobStatus == "queued" && previousJobStatus != "queued" && previousJobStatus != "running" {
		if err := recordCreativeRecoveryDispatchTx(ctx, tx, pgtype.UUID{}); err != nil {
			return revision, "", true, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return revision, "", true, fmt.Errorf("commit creative Prime job enqueue: %w", err)
	}
	return revision, jobStatus, true, nil
}

func enqueueCreativePrimeCompositionJob(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string, inputFingerprint string) (string, error) {
	if revision < 1 {
		return "", errors.New("creative Prime composition revision must be positive")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, expected_sizes, input_fingerprint
)
VALUES ($1, $2, 'queued', $3::text[], $4)
ON CONFLICT (variant_id, revision) DO UPDATE SET
  status = 'queued',
  expected_sizes = EXCLUDED.expected_sizes,
  input_fingerprint = EXCLUDED.input_fingerprint,
  lease_token = NULL,
  lease_expires_at = NULL,
  last_error = '',
  composed_at = NULL,
  completed_at = NULL,
  updated_at = now()
WHERE creative_prime_composition_job.status NOT IN ('running', 'cancelled')
  AND (
    creative_prime_composition_job.expected_sizes IS DISTINCT FROM EXCLUDED.expected_sizes
    OR creative_prime_composition_job.input_fingerprint IS DISTINCT FROM EXCLUDED.input_fingerprint
  )
`, variantID, revision, expectedSizes, inputFingerprint); err != nil {
		return "", fmt.Errorf("queue creative Prime composition: %w", err)
	}
	var status string
	if err := tx.QueryRow(ctx, `
SELECT status FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = $2
`, variantID, revision).Scan(&status); err != nil {
		return "", fmt.Errorf("load creative Prime composition job: %w", err)
	}
	if status != "queued" && status != "running" {
		return status, nil
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'partial',
    brief = jsonb_set(
      brief - 'brand_composition_error' - 'creative_qc_handoff_error',
      '{prime_composition_pending}',
      jsonb_build_object('revision', $2::int, 'status', 'queued', 'queued_at', now()::text),
      true
    ),
    staging_revision = CASE WHEN active_revision = $2 THEN staging_revision ELSE $2 END,
    updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, variantID, revision); err != nil {
		return "", fmt.Errorf("mark creative Prime composition pending: %w", err)
	}
	return status, nil
}

func resetCreativePrimeCompositionJob(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string, inputFingerprint string) (string, error) {
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_prime_composition_job (
  variant_id, revision, status, expected_sizes, input_fingerprint
)
VALUES ($1, $2, 'queued', $3::text[], $4)
ON CONFLICT (variant_id, revision) DO UPDATE SET
  status = 'queued', expected_sizes = EXCLUDED.expected_sizes,
  input_fingerprint = EXCLUDED.input_fingerprint,
  lease_token = NULL, lease_expires_at = NULL,
  last_error = '', composed_at = NULL, completed_at = NULL, updated_at = now()
WHERE creative_prime_composition_job.status NOT IN ('running', 'cancelled')
`, variantID, revision, expectedSizes, inputFingerprint); err != nil {
		return "", fmt.Errorf("reset creative Prime composition job: %w", err)
	}
	return enqueueCreativePrimeCompositionJob(ctx, tx, variantID, revision, expectedSizes, inputFingerprint)
}

func (h *Handler) claimCreativePrimeComposition(ctx context.Context, targetVariant pgtype.UUID, targetRevision int) (creativePrimeCompositionClaim, bool, error) {
	if h == nil || h.TxStarter == nil {
		return creativePrimeCompositionClaim{}, false, nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return creativePrimeCompositionClaim{}, false, fmt.Errorf("begin creative Prime composition claim: %w", err)
	}
	defer tx.Rollback(ctx)

	leaseUUID := uuid.New()
	leaseToken := pgtype.UUID{Bytes: leaseUUID, Valid: true}
	var claim creativePrimeCompositionClaim
	err = tx.QueryRow(ctx, `
WITH candidate AS (
  SELECT job.variant_id, job.revision,
         order_row.workspace_id, order_row.id AS order_id, item.id AS order_item_id,
         order_row.created_by, job.composed_at IS NOT NULL AS composition_ready,
         variant.candidate_state
  FROM creative_prime_composition_job job
  JOIN creative_order_variant variant
    ON variant.id = job.variant_id AND variant.revision = job.revision
  JOIN creative_order_item item ON item.id = variant.order_item_id
  JOIN creative_order order_row ON order_row.id = item.order_id
  WHERE (job.status = 'queued'
     OR (job.status = 'running' AND job.lease_expires_at <= now()))
    AND order_row.status <> 'cancelled'
    AND variant.status <> 'cancelled'
    AND ($3::uuid IS NULL OR (job.variant_id = $3 AND job.revision = $4))
  ORDER BY job.updated_at, job.variant_id
  LIMIT 1
  FOR UPDATE OF job SKIP LOCKED
), claimed AS (
  UPDATE creative_prime_composition_job job
  SET status = 'running',
      attempt = job.attempt + 1,
      lease_token = $1,
      lease_expires_at = now() + $2::interval,
      last_error = '',
      started_at = COALESCE(job.started_at, now()),
      completed_at = NULL,
      updated_at = now()
  FROM candidate
  WHERE job.variant_id = candidate.variant_id AND job.revision = candidate.revision
  RETURNING job.variant_id, job.revision, candidate.workspace_id,
            candidate.order_id, candidate.order_item_id, candidate.created_by,
            job.lease_token, candidate.composition_ready, candidate.candidate_state
)
SELECT variant_id, revision, workspace_id, order_id, order_item_id, created_by,
       lease_token, composition_ready, candidate_state
FROM claimed
`, leaseToken, fmt.Sprintf("%d seconds", int(creativePrimeCompositionLease.Seconds())), targetVariant, targetRevision).Scan(
		&claim.VariantID, &claim.Revision, &claim.WorkspaceID,
		&claim.OrderID, &claim.OrderItemID, &claim.RequestedBy, &claim.LeaseToken,
		&claim.CompositionReady, &claim.CandidateState,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return creativePrimeCompositionClaim{}, false, nil
	}
	if err != nil {
		return creativePrimeCompositionClaim{}, false, fmt.Errorf("claim creative Prime composition: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return creativePrimeCompositionClaim{}, false, fmt.Errorf("commit creative Prime composition claim: %w", err)
	}
	return claim, true, nil
}

func assertCreativePrimeCompositionLease(ctx context.Context, tx pgx.Tx, claim creativePrimeCompositionClaim, requireReady bool) error {
	var valid bool
	err := tx.QueryRow(ctx, `
SELECT true
FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = $2
  AND status = 'running' AND lease_token = $3
  AND lease_expires_at > now()
  AND (NOT $4::boolean OR composed_at IS NOT NULL)
FOR UPDATE
`, claim.VariantID, claim.Revision, claim.LeaseToken, requireReady).Scan(&valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("creative Prime composition lease was lost before committing side effects")
	}
	if err != nil {
		return fmt.Errorf("validate creative Prime composition lease: %w", err)
	}
	if !valid {
		return errors.New("creative Prime composition lease was lost before committing side effects")
	}
	return nil
}

func markCreativePrimeCompositionReady(ctx context.Context, tx pgx.Tx, claim creativePrimeCompositionClaim) error {
	if err := assertCreativePrimeCompositionLease(ctx, tx, claim, false); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
UPDATE creative_prime_composition_job
SET composed_at = COALESCE(composed_at, now()), updated_at = now()
WHERE variant_id = $1 AND revision = $2
  AND status = 'running' AND lease_token = $3
`, claim.VariantID, claim.Revision, claim.LeaseToken)
	if err != nil {
		return fmt.Errorf("mark creative Prime composition ready: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("creative Prime composition lease was lost before marking composition ready")
	}
	return nil
}

func completeCreativePrimeCompositionJob(ctx context.Context, tx pgx.Tx, claim creativePrimeCompositionClaim) error {
	var orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status FROM creative_order WHERE id = $1 FOR UPDATE
`, claim.OrderID).Scan(&orderStatus); err != nil {
		return fmt.Errorf("lock creative Prime handoff order: %w", err)
	}
	if orderStatus == "cancelled" {
		return &creativePrimeCancelledError{}
	}
	var currentRevision int
	var variantStatus string
	if err := tx.QueryRow(ctx, `
SELECT variant.revision, variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.id = $2 AND item.order_id = $3
FOR UPDATE OF variant
`, claim.VariantID, claim.OrderItemID, claim.OrderID).Scan(&currentRevision, &variantStatus); err != nil {
		return fmt.Errorf("lock creative Prime handoff lifecycle: %w", err)
	}
	if currentRevision != claim.Revision {
		return errors.New("creative Prime composition was superseded before handoff completion")
	}
	if variantStatus == "cancelled" {
		return &creativePrimeCancelledError{}
	}
	if err := assertCreativePrimeCompositionLease(ctx, tx, claim, true); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
UPDATE creative_prime_composition_job job
SET status = 'completed',
    lease_token = NULL,
    lease_expires_at = NULL,
    completed_at = now(),
    updated_at = now()
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE job.variant_id = $1 AND job.revision = $2
  AND job.status = 'running' AND job.lease_token = $3
  AND job.composed_at IS NOT NULL
  AND variant.id = job.variant_id AND variant.revision = job.revision
  AND variant.status <> 'cancelled' AND order_row.status <> 'cancelled'
`, claim.VariantID, claim.Revision, claim.LeaseToken)
	if err != nil {
		return fmt.Errorf("complete creative Prime composition job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errors.New("creative Prime composition was cancelled or superseded before handoff completion")
	}
	if _, err := tx.Exec(ctx, `
WITH cleared AS (
  UPDATE creative_order_variant
  SET brief = brief - 'prime_composition_pending', updated_at = now()
  WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
  RETURNING id, revision, brief
)
UPDATE creative_order_variant_revision revision
SET brief = cleared.brief, updated_at = now()
FROM cleared
WHERE revision.variant_id = cleared.id AND revision.revision = cleared.revision
`, claim.VariantID, claim.Revision); err != nil {
		return fmt.Errorf("clear completed creative Prime marker: %w", err)
	}
	return nil
}

func (h *Handler) finishCreativePrimeComposition(ctx context.Context, claim creativePrimeCompositionClaim, runErr error) error {
	if h == nil || h.TxStarter == nil {
		return nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin creative Prime composition completion: %w", err)
	}
	defer tx.Rollback(ctx)

	status := "queued"
	errorMessage := ""
	markRevisionFailed := false
	if runErr != nil {
		status = "failed"
		markRevisionFailed = true
		errorMessage = strings.TrimSpace(runErr.Error())
		if len(errorMessage) > 4000 {
			errorMessage = errorMessage[:4000]
		}
		var immutableErr *creativePrimeImmutableRevisionError
		var cancelledErr *creativePrimeCancelledError
		if errors.As(runErr, &immutableErr) || errors.As(runErr, &cancelledErr) {
			status = "cancelled"
			markRevisionFailed = false
		}
	}
	var orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status FROM creative_order WHERE id = $1 FOR UPDATE
`, claim.OrderID).Scan(&orderStatus); err != nil {
		return fmt.Errorf("lock creative Prime completion order: %w", err)
	}
	var currentRevision int
	var variantStatus string
	var activeRevision pgtype.Int4
	if err := tx.QueryRow(ctx, `
SELECT variant.revision, variant.status, variant.active_revision
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant
`, claim.VariantID, claim.OrderID).Scan(&currentRevision, &variantStatus, &activeRevision); err != nil {
		return fmt.Errorf("lock creative Prime completion variant: %w", err)
	}
	var revisionActivated bool
	if err := tx.QueryRow(ctx, `
SELECT activated_at IS NOT NULL
FROM creative_order_variant_revision
WHERE variant_id = $1 AND revision = $2
FOR UPDATE
`, claim.VariantID, claim.Revision).Scan(&revisionActivated); err != nil {
		return fmt.Errorf("lock creative Prime completion revision: %w", err)
	}
	lifecycleCancelled := orderStatus == "cancelled" || variantStatus == "cancelled" ||
		currentRevision != claim.Revision || revisionActivated ||
		(activeRevision.Valid && int(activeRevision.Int32) == claim.Revision)
	if lifecycleCancelled {
		status = "cancelled"
		markRevisionFailed = false
		if errorMessage == "" {
			errorMessage = "creative order or revision was cancelled before Prime completion"
		}
	}
	tag, err := tx.Exec(ctx, `
UPDATE creative_prime_composition_job
SET status = $4,
    lease_token = NULL,
    lease_expires_at = NULL,
    last_error = $5,
    composed_at = CASE WHEN $4 = 'queued' THEN COALESCE(composed_at, now()) ELSE composed_at END,
    completed_at = CASE WHEN $4 IN ('failed', 'cancelled') THEN now() ELSE NULL END,
    updated_at = now()
WHERE variant_id = $1 AND revision = $2
  AND status = 'running' AND lease_token = $3
`, claim.VariantID, claim.Revision, claim.LeaseToken, status, errorMessage)
	if err != nil {
		return fmt.Errorf("finish creative Prime composition job: %w", err)
	}
	if tag.RowsAffected() != 1 {
		if lifecycleCancelled {
			var currentJobStatus string
			if err := tx.QueryRow(ctx, `
SELECT status FROM creative_prime_composition_job
WHERE variant_id = $1 AND revision = $2
FOR UPDATE
`, claim.VariantID, claim.Revision).Scan(&currentJobStatus); err == nil && currentJobStatus == "cancelled" {
				return tx.Commit(ctx)
			}
		}
		return errors.New("creative Prime composition lease was lost before completion")
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant_revision revision
SET brief = variant.brief,
    status = 'action_required',
    updated_at = now()
FROM creative_order_variant variant
WHERE revision.variant_id = $1 AND revision.revision = $2
  AND variant.id = revision.variant_id AND variant.revision = revision.revision
	AND revision.activated_at IS NULL
	AND variant.active_revision IS DISTINCT FROM revision.revision
	AND $3::boolean
`, claim.VariantID, claim.Revision, markRevisionFailed); err != nil {
		return fmt.Errorf("sync creative Prime composition revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit creative Prime composition completion: %w", err)
	}
	return nil
}

func (h *Handler) completeCreativePrimeCompositionHandoff(ctx context.Context, claim creativePrimeCompositionClaim) error {
	if claim.CandidateState == "candidate" {
		tx, err := h.TxStarter.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin candidate Prime handoff completion: %w", err)
		}
		defer tx.Rollback(ctx)
		if err := completeCreativePrimeCompositionJob(ctx, tx, claim); err != nil {
			return err
		}
		var selectionTask db.AgentTaskQueue
		queued, err := h.queueCreativeCandidateSelectionTx(ctx, tx, claim.OrderItemID, creativeOrchestrationCause{RequestedBy: claim.RequestedBy}, nil, &selectionTask)
		if err != nil {
			return fmt.Errorf("queue candidate selection during Prime completion: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit candidate Prime handoff completion: %w", err)
		}
		if queued {
			h.TaskService.NotifyTaskEnqueued(ctx, selectionTask)
		}
		return nil
	}
	if err := h.enqueueCreativeVariantQC(ctx, claim.WorkspaceID, claim.OrderID, claim.VariantID, claim.RequestedBy, &claim); err != nil {
		return &creativeQCHandoffError{cause: err}
	}
	if _, err := h.maybeQueueCreativeCandidateSelection(
		ctx, claim.OrderItemID, creativeOrchestrationCause{RequestedBy: claim.RequestedBy},
	); err != nil {
		return fmt.Errorf("wake creative candidate selection after Prime: %w", err)
	}
	return nil
}

func creativePrimeBackgroundReworkFindings(runErr error) ([]creativeVisualModelReworkFinding, bool) {
	var compositionErr *creativePrimeCompositionError
	if !errors.As(runErr, &compositionErr) {
		return nil, false
	}
	reasons := creativePrimeFailureReasons(compositionErr.failures)
	if len(reasons) == 0 {
		return nil, false
	}
	findings := make([]creativeVisualModelReworkFinding, 0, len(reasons))
	seen := make(map[string]struct{}, len(reasons))
	for _, reason := range reasons {
		if reason.ErrorCode != "prime_no_adequate_template_for_size" || !validCreativeAssetSize(reason.Size) {
			return nil, false
		}
		if _, duplicate := seen[reason.Size]; duplicate {
			continue
		}
		seen[reason.Size] = struct{}{}
		findings = append(findings, creativeVisualModelReworkFinding{
			Code:    "prime_background_support_inadequate",
			SizeKey: reason.Size,
			Diagnosis: reason.Size + "：Prime 可见组件承托区的背景极性或纹理不合格；" +
				"只调整组件覆盖区下方背景，使其成为相反极性的低纹理承托区，保持主体、业务文字和布局不变",
		})
	}
	return findings, len(findings) > 0
}

func (h *Handler) queueCreativePrimeBackgroundRework(ctx context.Context, claim creativePrimeCompositionClaim, runErr error) (db.AgentTaskQueue, bool, error) {
	findings, eligible := creativePrimeBackgroundReworkFindings(runErr)
	if !eligible || h.TaskService == nil {
		return db.AgentTaskQueue{}, false, nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("begin creative Prime background rework: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID, orderID, itemID, candidateID, issueID pgtype.UUID
	var inputSnapshot string
	var currentRevision int
	var expectedSizes []string
	var triggerKind, variantStatus, orderStatus string
	err = tx.QueryRow(ctx, `
SELECT workspace_id, id, issue_id, input_snapshot::text, trigger_evidence_kind, status
FROM creative_order
WHERE id = $1
FOR UPDATE
`, claim.OrderID).Scan(&workspaceID, &orderID, &issueID, &inputSnapshot, &triggerKind, &orderStatus)
	if err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("lock creative Prime background rework order: %w", err)
	}
	if orderStatus == "cancelled" {
		return db.AgentTaskQueue{}, false, nil
	}
	err = tx.QueryRow(ctx, `
SELECT item.id, item.candidate_id, variant.revision, variant.status, revision.expected_sizes
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant, revision
`, claim.VariantID, orderID).Scan(
		&itemID, &candidateID, &currentRevision, &variantStatus, &expectedSizes,
	)
	if err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("load creative Prime background rework contract: %w", err)
	}
	if variantStatus == "cancelled" || currentRevision != claim.Revision {
		return db.AgentTaskQueue{}, false, nil
	}
	if triggerKind == "creative_direct_edit" || creativeOrderPipelineVersion(json.RawMessage(inputSnapshot)) != creativePipelineCandidateV1 {
		return db.AgentTaskQueue{}, false, nil
	}
	expectedSizes, err = normalizeCreativeExpectedSizes(expectedSizes)
	if err != nil {
		return db.AgentTaskQueue{}, false, err
	}
	for _, finding := range findings {
		if !creativeSizeIsExpected(finding.SizeKey, expectedSizes) {
			return db.AgentTaskQueue{}, false, errors.New("creative Prime background rework has an unexpected failed size")
		}
	}
	var priorAttempts int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND context->>'variant_id' = $1::text
  AND context->'qc_visual_rework'->'failures' @> '[{"code":"prime_background_support_inadequate"}]'::jsonb
`, claim.VariantID).Scan(&priorAttempts); err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("count creative Prime background rework attempts: %w", err)
	}
	if priorAttempts >= 1 {
		return db.AgentTaskQueue{}, false, nil
	}

	var parentTaskID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT id
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_order_item_production'
  AND trigger_evidence_ref_id = $1
  AND context->>'workflow' = 'creative_production'
  AND context->>'variant_id' = $2::text
  AND COALESCE(NULLIF(context->>'revision', '')::int, 0) = $3
ORDER BY created_at DESC, id DESC
LIMIT 1
`, itemID, claim.VariantID, claim.Revision).Scan(&parentTaskID); err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("load creative Prime background rework parent task: %w", err)
	}
	parentTask, err := h.Queries.WithTx(tx).GetAgentTask(ctx, parentTaskID)
	if err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("read creative Prime background rework parent task: %w", err)
	}
	task, err := h.queueCreativeVisualModelRework(
		ctx, tx, workspaceID, orderID, itemID, candidateID, claim.VariantID, issueID,
		json.RawMessage(inputSnapshot), claim.Revision, expectedSizes, parentTask, findings,
	)
	if err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("queue creative Prime background rework: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE agent_task_queue
SET context = jsonb_set(context, '{qc_visual_rework,reflow_strategy}', '"prime_background_support"'::jsonb, true)
WHERE id = $1
`, task.ID); err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("mark creative Prime background rework strategy: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.AgentTaskQueue{}, false, fmt.Errorf("commit creative Prime background rework: %w", err)
	}
	h.TaskService.NotifyTaskEnqueued(ctx, task)
	return task, true, nil
}

func (h *Handler) processCreativePrimeCompositionClaim(ctx context.Context, claim creativePrimeCompositionClaim) error {
	var runErr error
	if !claim.CompositionReady {
		var composed bool
		composed, runErr = h.runCreativeOrderPrimeComposition(
			ctx, claim.WorkspaceID, claim.OrderID, claim.VariantID, claim.RequestedBy, false, &claim,
		)
		if runErr == nil && !composed {
			runErr = errors.New("creative Prime recovery found an incomplete generated package")
			h.markCreativePrimeCompositionFailed(ctx, claim.VariantID, runErr)
		}
	}
	if runErr != nil {
		if err := h.finishCreativePrimeComposition(ctx, claim, runErr); err != nil {
			return err
		}
		if _, _, repairErr := h.queueCreativePrimeBackgroundRework(ctx, claim, runErr); repairErr != nil {
			return errors.Join(runErr, repairErr)
		}
		return runErr
	}
	return h.completeCreativePrimeCompositionHandoff(ctx, claim)
}

// RecoverPendingCreativePrimeCompositions runs bounded deterministic platform
// work after image generation. Jobs are durable and leased, so calling this
// concurrently from multiple server replicas is safe.
func (h *Handler) RecoverPendingCreativePrimeCompositions(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, nil
	}
	completed := 0
	var recoveryErrors []error
	for completed < limit {
		claim, found, err := h.claimCreativePrimeComposition(ctx, pgtype.UUID{}, 0)
		if err != nil {
			recoveryErrors = append(recoveryErrors, err)
			break
		}
		if !found {
			break
		}
		if err := h.processCreativePrimeCompositionClaim(ctx, claim); err != nil {
			recoveryErrors = append(recoveryErrors, err)
		}
		completed++
	}
	return completed, errors.Join(recoveryErrors...)
}
