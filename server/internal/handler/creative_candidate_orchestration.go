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

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	creativeCandidateSelectionEvidenceKind = "creative_order_item_candidate_selection"
	creativeCandidateSelectionWorkflow     = "creative_candidate_selection"
	creativeCandidateSelectionItemKey      = "candidate-selection:v1"
	creativePipelineCandidateV1            = "candidate_v1"
	creativePipelineDirectEditV1           = "direct_edit_v1"
	creativeCandidateSelectionMaxAttempts  = 3
	creativeCandidateSelectionMinimumReady = 3
	creativeSelectedExpansionPhase         = "selected_missing_sizes"
	creativeReservePromotionPhase          = "reserve_promotion"
)

func creativeOrderPipelineVersion(raw json.RawMessage) string {
	var snapshot struct {
		PipelineVersion string `json:"pipeline_version"`
	}
	if json.Unmarshal(raw, &snapshot) != nil {
		return ""
	}
	return strings.TrimSpace(snapshot.PipelineVersion)
}

func freezeCreativeOrderPipelineVersion(raw json.RawMessage, version string) (json.RawMessage, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
		return nil, errors.New("input_snapshot must be an object")
	}
	encodedVersion, err := json.Marshal(version)
	if err != nil {
		return nil, err
	}
	snapshot["pipeline_version"] = encodedVersion
	return json.Marshal(snapshot)
}

func isCreativeCandidateVariantKey(value string) bool {
	if len(value) != 3 || value[0] != 'C' || value[1] != '0' {
		return false
	}
	return value[2] >= '1' && value[2] <= '5'
}

// creativeOrchestrationCause preserves the human attribution of the task that
// caused an automatic handoff. RequestedBy is the fallback for an HTTP action.
type creativeOrchestrationCause struct {
	ParentTask  *db.AgentTaskQueue
	RequestedBy pgtype.UUID
}

type creativeCandidatePrimary struct {
	VariantID              string          `json:"variant_id"`
	VariantKey             string          `json:"variant_key"`
	Revision               int             `json:"revision"`
	PrimarySize            string          `json:"primary_size"`
	Brief                  json.RawMessage `json:"brief"`
	GeneratedAssetID       string          `json:"generated_asset_id"`
	GeneratedAttachmentID  string          `json:"generated_attachment_id"`
	PrimedAssetID          string          `json:"primed_asset_id"`
	PrimedAttachmentID     string          `json:"primed_attachment_id"`
	productionTerminal     bool
	primaryPackageComplete bool
}

type creativeSelectedExpansion struct {
	VariantID             pgtype.UUID
	VariantIDText         string
	VariantKey            string
	Revision              int
	PrimarySize           string
	ExpectedSizes         []string
	MissingSizes          []string
	CandidateID           string
	OrderID               pgtype.UUID
	OrderIDText           string
	OrderItemID           pgtype.UUID
	OrderItemIDText       string
	WorkspaceID           pgtype.UUID
	IssueID               pgtype.UUID
	IssueIDText           string
	InputSnapshot         json.RawMessage
	CreatedBy             pgtype.UUID
	LeaderID              pgtype.UUID
	ReviewerID            pgtype.UUID
	PrimaryAssetID        string
	PrimaryAttachmentID   string
	ProductionPhase       string
	ReserveSourceVariant  string
	RejectedSourceVariant string
}

func creativeOrchestrationAttribution(cause creativeOrchestrationCause, fallbackUser pgtype.UUID, evidenceKind string, evidenceRefID pgtype.UUID) (attribution.Result, pgtype.UUID) {
	requestedBy := cause.RequestedBy
	if cause.ParentTask != nil && cause.ParentTask.ID.Valid {
		if !requestedBy.Valid {
			requestedBy = cause.ParentTask.RequestingUserID
		}
		if !requestedBy.Valid {
			requestedBy = fallbackUser
		}
		return attribution.DelegatedRun(
			cause.ParentTask.ID,
			cause.ParentTask.OriginatorUserID,
			cause.ParentTask.AccountableUserID,
			attribution.EvidenceKind(evidenceKind),
			evidenceRefID,
		), requestedBy
	}
	if !requestedBy.Valid {
		requestedBy = fallbackUser
	}
	return attribution.DirectHumanRun(requestedBy, attribution.EvidenceKind(evidenceKind), evidenceRefID), requestedBy
}

func isCreativeTaskTerminal(status string) bool {
	switch status {
	case "completed", "failed":
		return true
	default:
		return false
	}
}

func (h *Handler) requireCreativeCandidateSelectionTask(w http.ResponseWriter, r *http.Request, orderID, orderItemID pgtype.UUID) bool {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		return true
	}
	taskID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
	if !ok {
		return false
	}
	agentID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Agent-ID"), "agent_id")
	if !ok {
		return false
	}
	var authorized bool
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS (
  SELECT 1 FROM agent_task_queue task
  WHERE task.id = $1
    AND task.agent_id = $2
    AND task.status = 'running'
    AND task.trigger_evidence_kind = $3
    AND task.trigger_evidence_ref_id = $4
    AND task.context->>'type' = 'creative_domain_task'
    AND task.context->>'workflow' = $5
    AND task.context->>'creative_order_id' = $6::text
    AND task.context->>'creative_order_item_id' = $4::text
)
`, taskID, agentID, creativeCandidateSelectionEvidenceKind, orderItemID, creativeCandidateSelectionWorkflow, orderID).Scan(&authorized); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate creative candidate selection task")
		return false
	}
	if !authorized {
		writeError(w, http.StatusForbidden, "candidate selection task does not match this creative order item")
		return false
	}
	return true
}

// maybeQueueCreativeCandidateSelection creates exactly one comparison task
// after primary packages settle. Terminal failures are rejected before
// comparison so three usable candidates can still complete an order.
func (h *Handler) maybeQueueCreativeCandidateSelection(ctx context.Context, orderItemID pgtype.UUID, cause creativeOrchestrationCause) (bool, error) {
	return h.maybeQueueCreativeCandidateSelectionWithPrimeHandoff(ctx, orderItemID, cause, nil)
}

func (h *Handler) maybeQueueCreativeCandidateSelectionWithPrimeHandoff(ctx context.Context, orderItemID pgtype.UUID, cause creativeOrchestrationCause, primeClaim *creativePrimeCompositionClaim) (bool, error) {
	if h.TxStarter == nil || h.Queries == nil || h.TaskService == nil || !orderItemID.Valid {
		return false, nil
	}
	var handoffVariantID, handoffLeaseToken pgtype.UUID
	var handoffRevision int
	if primeClaim != nil {
		handoffVariantID = primeClaim.VariantID
		handoffRevision = primeClaim.Revision
		handoffLeaseToken = primeClaim.LeaseToken
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin creative candidate selection handoff: %w", err)
	}
	defer tx.Rollback(ctx)
	var workspaceID, orderID, issueID, createdBy pgtype.UUID
	var inputSnapshot string
	var orderStatus string
	if err := tx.QueryRow(ctx, `SELECT order_id FROM creative_order_item WHERE id = $1`, orderItemID).Scan(&orderID); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("resolve creative candidate selection order: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, creativeCandidateSelectionEvidenceKind+":"+uuidToString(orderItemID)); err != nil {
		return false, fmt.Errorf("lock creative candidate selection handoff: %w", err)
	}
	err = tx.QueryRow(ctx, `
SELECT workspace_id, issue_id, created_by, input_snapshot::text, status
FROM creative_order
WHERE id = $1
FOR UPDATE
`, orderID).Scan(&workspaceID, &issueID, &createdBy, &inputSnapshot, &orderStatus)
	if errors.Is(err, pgx.ErrNoRows) || orderStatus == "cancelled" {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("lock creative candidate selection order: %w", err)
	}
	var itemStatus string
	if err := tx.QueryRow(ctx, `
SELECT status FROM creative_order_item
WHERE id = $1 AND order_id = $2
FOR UPDATE
`, orderItemID, orderID).Scan(&itemStatus); errors.Is(err, pgx.ErrNoRows) || itemStatus == "cancelled" {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("lock creative candidate selection item: %w", err)
	}
	if creativeOrderPipelineVersion(json.RawMessage(inputSnapshot)) != creativePipelineCandidateV1 {
		return false, nil
	}

	var ranked, incompatibleVariants bool
	if err := tx.QueryRow(ctx, `
SELECT
  EXISTS (
    SELECT 1 FROM creative_order_variant
    WHERE order_item_id = $1 AND candidate_state IN ('selected', 'reserve') AND selection_rank IS NOT NULL
  ),
  EXISTS (
    SELECT 1 FROM creative_order_variant
    WHERE order_item_id = $1
      AND (variant_key !~ '^C0[1-5]$' OR (candidate_state = 'selected' AND selection_rank IS NULL))
  )
`, orderItemID).Scan(&ranked, &incompatibleVariants); err != nil {
		return false, fmt.Errorf("check creative candidate ranking: %w", err)
	}
	if ranked || incompatibleVariants {
		return false, nil
	}
	var activeTask bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM agent_task_queue
  WHERE trigger_evidence_kind = $1
    AND trigger_evidence_ref_id = $2
    AND context->>'item_key' = $3
	AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, creativeCandidateSelectionEvidenceKind, orderItemID, creativeCandidateSelectionItemKey).Scan(&activeTask); err != nil {
		return false, fmt.Errorf("check creative candidate selection task: %w", err)
	}
	if activeTask {
		return false, nil
	}
	var terminalAttempts int
	if err := tx.QueryRow(ctx, `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = $1
  AND trigger_evidence_ref_id = $2
  AND context->>'item_key' = $3
  AND status IN ('completed', 'failed', 'cancelled')
`, creativeCandidateSelectionEvidenceKind, orderItemID, creativeCandidateSelectionItemKey).Scan(&terminalAttempts); err != nil {
		return false, fmt.Errorf("count creative candidate selection attempts: %w", err)
	}
	if terminalAttempts >= creativeCandidateSelectionMaxAttempts {
		return false, nil
	}

	type candidateRow struct {
		id          pgtype.UUID
		variantKey  string
		revision    int
		status      string
		state       string
		primarySize string
		brief       string
	}
	rows, err := tx.Query(ctx, `
SELECT id, variant_key, revision, status, candidate_state, primary_size, brief::text
FROM creative_order_variant
WHERE order_item_id = $1
  AND candidate_state IN ('candidate', 'rejected')
  AND selection_rank IS NULL
ORDER BY created_at, id
FOR UPDATE
`, orderItemID)
	if err != nil {
		return false, fmt.Errorf("load creative candidates: %w", err)
	}
	candidateRows := make([]candidateRow, 0, 5)
	for rows.Next() {
		var row candidateRow
		if err := rows.Scan(&row.id, &row.variantKey, &row.revision, &row.status, &row.state, &row.primarySize, &row.brief); err != nil {
			rows.Close()
			return false, fmt.Errorf("read creative candidate: %w", err)
		}
		candidateRows = append(candidateRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, fmt.Errorf("read creative candidates: %w", err)
	}
	rows.Close()
	if len(candidateRows) < creativeCandidateSelectionMinimumReady || len(candidateRows) > 5 {
		return false, nil
	}

	ready := make([]creativeCandidatePrimary, 0, len(candidateRows))
	failed := make([]candidateRow, 0, 1)
	for _, row := range candidateRows {
		if row.state == "rejected" {
			continue
		}
		var generatedAssetID, generatedAttachmentID, primedAssetID, primedAttachmentID string
		var hasTask, activeTask, activePrimeJob bool
		var latestTaskStatus string
		if err := tx.QueryRow(ctx, `
SELECT
  COALESCE((SELECT asset.id::text FROM creative_order_asset asset
            WHERE asset.variant_id = $1 AND asset.revision = $2 AND asset.size_key = $3
              AND asset.stage = 'generated' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
            ORDER BY asset.updated_at DESC LIMIT 1), ''),
  COALESCE((SELECT asset.attachment_id::text FROM creative_order_asset asset
            WHERE asset.variant_id = $1 AND asset.revision = $2 AND asset.size_key = $3
              AND asset.stage = 'generated' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
            ORDER BY asset.updated_at DESC LIMIT 1), ''),
  COALESCE((SELECT asset.id::text FROM creative_order_asset asset
            WHERE asset.variant_id = $1 AND asset.revision = $2 AND asset.size_key = $3
              AND asset.stage = 'primed' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
            ORDER BY asset.updated_at DESC LIMIT 1), ''),
  COALESCE((SELECT asset.attachment_id::text FROM creative_order_asset asset
            WHERE asset.variant_id = $1 AND asset.revision = $2 AND asset.size_key = $3
              AND asset.stage = 'primed' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
            ORDER BY asset.updated_at DESC LIMIT 1), ''),
  EXISTS (SELECT 1 FROM agent_task_queue task
          WHERE task.trigger_evidence_kind = 'creative_order_item_production'
            AND task.trigger_evidence_ref_id = $4
            AND task.context->>'workflow' = 'creative_production'
            AND task.context->>'variant_id' = $1::text
		    AND (task.context->>'revision')::integer = $2),
  EXISTS (SELECT 1 FROM agent_task_queue task
          WHERE task.trigger_evidence_kind = 'creative_order_item_production'
            AND task.trigger_evidence_ref_id = $4
            AND task.context->>'workflow' = 'creative_production'
            AND task.context->>'variant_id' = $1::text
		    AND (task.context->>'revision')::integer = $2
            AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')),
  COALESCE((SELECT task.status FROM agent_task_queue task
            WHERE task.trigger_evidence_kind = 'creative_order_item_production'
              AND task.trigger_evidence_ref_id = $4
              AND task.context->>'workflow' = 'creative_production'
              AND task.context->>'variant_id' = $1::text
		      AND (task.context->>'revision')::integer = $2
            ORDER BY task.created_at DESC, task.id DESC LIMIT 1), ''),
  EXISTS (SELECT 1 FROM creative_prime_composition_job job
            WHERE job.variant_id = $1 AND job.revision = $2
              AND job.status IN ('queued', 'running')
              AND NOT (
                $5::uuid IS NOT NULL
                AND job.variant_id = $5
                AND job.revision = $6
                AND job.status = 'running'
                AND job.lease_token = $7
                AND job.composed_at IS NOT NULL
              ))
`, row.id, row.revision, row.primarySize, orderItemID,
			handoffVariantID, handoffRevision, handoffLeaseToken).Scan(
			&generatedAssetID,
			&generatedAttachmentID,
			&primedAssetID,
			&primedAttachmentID,
			&hasTask,
			&activeTask,
			&latestTaskStatus,
			&activePrimeJob,
		); err != nil {
			return false, fmt.Errorf("load creative candidate primary state: %w", err)
		}
		terminal := hasTask && !activeTask && !activePrimeJob && isCreativeTaskTerminal(latestTaskStatus)
		complete := generatedAttachmentID != "" && primedAttachmentID != ""
		if terminal && complete {
			ready = append(ready, creativeCandidatePrimary{
				VariantID:              uuidToString(row.id),
				VariantKey:             row.variantKey,
				Revision:               row.revision,
				PrimarySize:            row.primarySize,
				Brief:                  json.RawMessage(row.brief),
				GeneratedAssetID:       generatedAssetID,
				GeneratedAttachmentID:  generatedAttachmentID,
				PrimedAssetID:          primedAssetID,
				PrimedAttachmentID:     primedAttachmentID,
				productionTerminal:     terminal,
				primaryPackageComplete: complete,
			})
			continue
		}
		if terminal && !complete {
			failed = append(failed, row)
		}
	}

	nonRejectedCount := 0
	for _, row := range candidateRows {
		if row.state != "rejected" {
			nonRejectedCount++
		}
	}
	if len(ready)+len(failed) != nonRejectedCount || len(ready) < creativeCandidateSelectionMinimumReady {
		return false, nil
	}
	for _, rejected := range failed {
		if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET candidate_state = 'rejected', selection_rank = NULL, status = 'action_required',
    brief = brief || jsonb_build_object(
      'candidate_rejection_reason', 'primary_production_terminal_without_complete_prime',
      'candidate_rejected_automatically', true
    ),
    updated_at = now()
WHERE id = $1 AND candidate_state = 'candidate' AND active_revision IS NULL
`, rejected.id); err != nil {
			return false, fmt.Errorf("reject failed creative candidate: %w", err)
		}
		if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant_revision
SET status = 'action_required', updated_at = now()
WHERE variant_id = $1 AND revision = $2
		`, rejected.id, rejected.revision); err != nil {
			return false, fmt.Errorf("reject failed creative candidate revision: %w", err)
		}
		nonRejectedCount--
	}
	if len(ready) != nonRejectedCount || len(ready) < creativeCandidateSelectionMinimumReady || len(ready) > 5 {
		return false, nil
	}

	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(json.RawMessage(inputSnapshot))
	if err != nil {
		return false, err
	}
	reviewer, err := h.Queries.WithTx(tx).GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: reviewerID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || reviewer.ArchivedAt.Valid || !reviewer.RuntimeID.Valid {
		return false, errors.New("the frozen creative QC reviewer is unavailable")
	}
	if err != nil {
		return false, fmt.Errorf("load creative candidate reviewer: %w", err)
	}
	var reviewerCapable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM agent_skill binding
  JOIN skill skill_row ON skill_row.id = binding.skill_id
  WHERE binding.agent_id = $1 AND binding.enabled
    AND skill_row.workspace_id = $2
    AND skill_row.config->>'kind' = 'creative_role'
    AND skill_row.config->>'capability' = 'quality_control'
)
`, reviewer.ID, workspaceID).Scan(&reviewerCapable); err != nil {
		return false, fmt.Errorf("validate creative candidate reviewer: %w", err)
	}
	if !reviewerCapable {
		return false, errors.New("the frozen creative QC reviewer no longer provides quality_control")
	}

	contextValue, err := json.Marshal(map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               creativeCandidateSelectionWorkflow,
		"scope":                  "order_item",
		"subject_id":             uuidToString(orderItemID),
		"item_key":               creativeCandidateSelectionItemKey,
		"creative_order_id":      uuidToString(orderID),
		"creative_order_item_id": uuidToString(orderItemID),
		"issue_id":               uuidToString(issueID),
		"leader_agent_id":        uuidToString(leaderID),
		"reviewer_agent_id":      uuidToString(reviewer.ID),
		"selection_attempt":      terminalAttempts + 1,
		"candidate_count":        len(ready),
		"candidates":             ready,
	})
	if err != nil {
		return false, fmt.Errorf("encode creative candidate selection task: %w", err)
	}
	attr, requestedBy := creativeOrchestrationAttribution(cause, createdBy, creativeCandidateSelectionEvidenceKind, orderItemID)
	task, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID:              reviewer.ID,
		RuntimeID:            reviewer.RuntimeID,
		IssueID:              issueID,
		Priority:             0,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     requestedBy,
		OriginatorUserID:     attr.UserID,
		AccountableUserID:    attr.AccountableUserID,
		OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
		DelegatedFromTaskID:  attr.DelegatedFromTaskID,
		TriggerEvidenceKind:  pgtype.Text{String: creativeCandidateSelectionEvidenceKind, Valid: true},
		TriggerEvidenceRefID: orderItemID,
		Context:              contextValue,
	})
	if err != nil {
		return false, fmt.Errorf("queue creative candidate selection: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		return false, fmt.Errorf("touch creative order candidate selection: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit creative candidate selection handoff: %w", err)
	}
	h.TaskService.NotifyTaskEnqueued(ctx, task)
	return true, nil
}

func (h *Handler) creativeCandidateSelectionCommitted(ctx context.Context, workspaceID, orderID, orderItemID pgtype.UUID) (bool, error) {
	if !workspaceID.Valid || !orderID.Valid || !orderItemID.Valid {
		return false, nil
	}
	var committed bool
	err := h.DB.QueryRow(ctx, `
SELECT
  count(*) BETWEEN 3 AND 5
  AND count(*) FILTER (WHERE variant.variant_key !~ '^C0[1-5]$') = 0
  AND count(*) FILTER (WHERE variant.candidate_state = 'candidate') = 0
  AND count(*) FILTER (WHERE variant.candidate_state = 'selected') = 3
  AND count(*) FILTER (WHERE variant.candidate_state = 'selected' AND variant.selection_rank = 1) = 1
  AND count(*) FILTER (WHERE variant.candidate_state = 'selected' AND variant.selection_rank = 2) = 1
  AND count(*) FILTER (WHERE variant.candidate_state = 'selected' AND variant.selection_rank = 3) = 1
  AND count(*) FILTER (WHERE variant.candidate_state = 'reserve') BETWEEN 0 AND 2
  AND count(*) FILTER (WHERE variant.candidate_state = 'reserve' AND variant.selection_rank NOT IN (4, 5)) = 0
  AND count(*) FILTER (WHERE variant.candidate_state = 'reserve' AND variant.selection_rank = 4) =
    CASE WHEN count(*) FILTER (WHERE variant.candidate_state = 'reserve') >= 1 THEN 1 ELSE 0 END
  AND count(*) FILTER (WHERE variant.candidate_state = 'reserve' AND variant.selection_rank = 5) =
    CASE WHEN count(*) FILTER (WHERE variant.candidate_state = 'reserve') = 2 THEN 1 ELSE 0 END
  AND count(*) FILTER (
    WHERE variant.candidate_state IN ('selected', 'reserve') AND variant.selection_rank IS NULL
  ) = 0
  AND count(*) FILTER (
    WHERE variant.candidate_state IN ('candidate', 'rejected') AND variant.selection_rank IS NOT NULL
  ) = 0
  AND count(*) FILTER (WHERE variant.candidate_state = 'rejected') <= 2
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1
  AND order_row.id = $2
  AND order_row.workspace_id = $3
  AND order_row.input_snapshot->>'pipeline_version' = $4
`, orderItemID, orderID, workspaceID, creativePipelineCandidateV1).Scan(&committed)
	return committed, err
}

func (h *Handler) creativeCandidateSelectionCompletionError(ctx context.Context, task db.AgentTaskQueue, workspaceID string) (string, error) {
	if !task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != creativeCandidateSelectionEvidenceKind {
		return "", nil
	}
	if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
		return "", nil
	}
	var taskContext struct {
		Type                string `json:"type"`
		Workflow            string `json:"workflow"`
		CreativeOrderID     string `json:"creative_order_id"`
		CreativeOrderItemID string `json:"creative_order_item_id"`
		ItemKey             string `json:"item_key"`
	}
	if err := json.Unmarshal(task.Context, &taskContext); err != nil ||
		taskContext.Type != "creative_domain_task" || taskContext.Workflow != creativeCandidateSelectionWorkflow ||
		taskContext.ItemKey != creativeCandidateSelectionItemKey {
		return "candidate selection task completed with invalid task context", nil
	}
	workspaceUUID, workspaceErr := parseCreativeOrchestrationUUID(workspaceID)
	orderID, orderErr := parseCreativeOrchestrationUUID(taskContext.CreativeOrderID)
	itemID, itemErr := parseCreativeOrchestrationUUID(taskContext.CreativeOrderItemID)
	if workspaceErr != nil || orderErr != nil || itemErr != nil || !task.TriggerEvidenceRefID.Valid || task.TriggerEvidenceRefID != itemID {
		return "candidate selection task completed with invalid artifact coordinates", nil
	}
	committed, err := h.creativeCandidateSelectionCommitted(ctx, workspaceUUID, orderID, itemID)
	if err != nil {
		return "", err
	}
	if !committed {
		return "candidate selection task completed without exactly three selected ranks and ordered reserves when available", nil
	}
	return "", nil
}

// queueSelectedCreativeProductionTasks reuses completed primary assets and
// creates one bounded production task only for selected variants with missing
// delivery sizes. Reserve and rejected variants are never enqueued.
func (h *Handler) queueSelectedCreativeProductionTasks(ctx context.Context, orderItemID pgtype.UUID, cause creativeOrchestrationCause, phase string, onlyVariants ...pgtype.UUID) ([]db.AgentTaskQueue, error) {
	if h.TaskService == nil || h.Queries == nil || h.TxStarter == nil || !orderItemID.Valid {
		return nil, nil
	}
	phase = strings.TrimSpace(phase)
	if phase == "" {
		phase = creativeSelectedExpansionPhase
	}
	allowedVariants := make(map[string]struct{}, len(onlyVariants))
	for _, variantID := range onlyVariants {
		if variantID.Valid {
			allowedVariants[uuidToString(variantID)] = struct{}{}
		}
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin selected creative production handoff: %w", err)
	}
	defer tx.Rollback(ctx)
	var orderID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT order_id FROM creative_order_item WHERE id = $1`, orderItemID).Scan(&orderID); errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("resolve selected creative production order: %w", err)
	}
	var orderStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_order WHERE id = $1 FOR UPDATE`, orderID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) || orderStatus == "cancelled" {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("lock selected creative production order: %w", err)
	}
	var itemStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_order_item WHERE id = $1 AND order_id = $2 FOR UPDATE`, orderItemID, orderID).Scan(&itemStatus); errors.Is(err, pgx.ErrNoRows) || itemStatus == "cancelled" {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("lock selected creative production item: %w", err)
	}
	rows, err := tx.Query(ctx, `
SELECT variant.id, variant.id::text, variant.variant_key, variant.revision, variant.primary_size,
       revision.expected_sizes, item.candidate_id::text,
       order_row.id, order_row.id::text, item.id, item.id::text, order_row.workspace_id,
       order_row.issue_id, COALESCE(order_row.issue_id::text, ''), order_row.input_snapshot::text,
       order_row.created_by,
       COALESCE((SELECT asset.id::text FROM creative_order_asset asset
                 WHERE asset.variant_id = variant.id AND asset.revision = variant.revision
                   AND asset.size_key = variant.primary_size AND asset.stage = 'generated'
                   AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
                 ORDER BY asset.updated_at DESC LIMIT 1), ''),
       COALESCE((SELECT asset.attachment_id::text FROM creative_order_asset asset
                 WHERE asset.variant_id = variant.id AND asset.revision = variant.revision
                   AND asset.size_key = variant.primary_size AND asset.stage = 'generated'
                   AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
                 ORDER BY asset.updated_at DESC LIMIT 1), '')
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision
  ON revision.variant_id = variant.id AND revision.revision = variant.revision
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1
  AND variant.candidate_state = 'selected'
  AND variant.selection_rank BETWEEN 1 AND 3
  AND order_row.status <> 'cancelled'
ORDER BY variant.selection_rank, variant.id
FOR UPDATE OF variant, revision
`, orderItemID)
	if err != nil {
		return nil, fmt.Errorf("load selected creative production variants: %w", err)
	}
	selected := make([]creativeSelectedExpansion, 0, 3)
	for rows.Next() {
		var value creativeSelectedExpansion
		var snapshot string
		if err := rows.Scan(
			&value.VariantID,
			&value.VariantIDText,
			&value.VariantKey,
			&value.Revision,
			&value.PrimarySize,
			&value.ExpectedSizes,
			&value.CandidateID,
			&value.OrderID,
			&value.OrderIDText,
			&value.OrderItemID,
			&value.OrderItemIDText,
			&value.WorkspaceID,
			&value.IssueID,
			&value.IssueIDText,
			&snapshot,
			&value.CreatedBy,
			&value.PrimaryAssetID,
			&value.PrimaryAttachmentID,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read selected creative production variant: %w", err)
		}
		value.InputSnapshot = json.RawMessage(snapshot)
		value.ProductionPhase = phase
		selected = append(selected, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("read selected creative production variants: %w", err)
	}
	rows.Close()

	queued := make([]db.AgentTaskQueue, 0, len(selected))
	created := make([]db.AgentTaskQueue, 0, len(selected))
	for _, value := range selected {
		if len(allowedVariants) > 0 {
			if _, allowed := allowedVariants[value.VariantIDText]; !allowed {
				continue
			}
		}
		completedRows, err := tx.Query(ctx, `
SELECT DISTINCT size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'generated'
  AND status = 'completed' AND attachment_id IS NOT NULL
  AND size_key = ANY($3::text[])
`, value.VariantID, value.Revision, value.ExpectedSizes)
		if err != nil {
			return queued, fmt.Errorf("load selected creative generated sizes: %w", err)
		}
		completed := make(map[string]struct{}, len(value.ExpectedSizes))
		for completedRows.Next() {
			var size string
			if err := completedRows.Scan(&size); err != nil {
				completedRows.Close()
				return queued, fmt.Errorf("read selected creative generated size: %w", err)
			}
			completed[size] = struct{}{}
		}
		if err := completedRows.Err(); err != nil {
			completedRows.Close()
			return queued, fmt.Errorf("read selected creative generated sizes: %w", err)
		}
		completedRows.Close()
		for _, size := range value.ExpectedSizes {
			if _, exists := completed[size]; !exists {
				value.MissingSizes = append(value.MissingSizes, size)
			}
		}
		if len(value.MissingSizes) == 0 {
			continue
		}
		var existing bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM agent_task_queue task
  WHERE task.trigger_evidence_kind = 'creative_order_item_production'
    AND task.trigger_evidence_ref_id = $1
    AND task.context->>'workflow' = 'creative_production'
    AND task.context->>'variant_id' = $2::text
	    AND (task.context->>'revision')::integer = $3
    AND task.context->>'production_phase' = $4
)
`, orderItemID, value.VariantID, value.Revision, phase).Scan(&existing); err != nil {
			return queued, fmt.Errorf("check selected creative production task: %w", err)
		}
		if existing {
			continue
		}
		leaderID, _, reviewerID, err := creativeOrderProductionAgentSnapshot(value.InputSnapshot)
		if err != nil {
			return queued, err
		}
		value.LeaderID = leaderID
		value.ReviewerID = reviewerID
		contextValue, err := json.Marshal(map[string]any{
			"type":                          "creative_domain_task",
			"workflow":                      "creative_production",
			"scope":                         "variant",
			"subject_id":                    value.VariantIDText,
			"item_key":                      fmt.Sprintf("%s:r%d", value.VariantIDText, value.Revision),
			"creative_order_id":             value.OrderIDText,
			"creative_order_item_id":        value.OrderItemIDText,
			"candidate_id":                  value.CandidateID,
			"variant_id":                    value.VariantIDText,
			"variant_key":                   value.VariantKey,
			"revision":                      value.Revision,
			"candidate_state":               "selected",
			"primary_size":                  value.PrimarySize,
			"expected_sizes":                value.ExpectedSizes,
			"missing_sizes":                 value.MissingSizes,
			"production_stage":              "selected_missing_sizes",
			"production_phase":              phase,
			"reuse_primary":                 true,
			"primary_generated_asset_id":    value.PrimaryAssetID,
			"primary_attachment_id":         value.PrimaryAttachmentID,
			"issue_id":                      value.IssueIDText,
			"leader_agent_id":               uuidToString(value.LeaderID),
			"reviewer_agent_id":             uuidToString(value.ReviewerID),
			"reserve_source_variant_id":     value.ReserveSourceVariant,
			"rejected_source_variant_id":    value.RejectedSourceVariant,
			"independent_size_generation":   true,
			"primary_is_optional_reference": true,
		})
		if err != nil {
			return queued, fmt.Errorf("encode selected creative production task: %w", err)
		}
		fanoutItem := service.DirectTaskFanoutItem{
			ItemKey: fmt.Sprintf("%s:r%d", value.VariantIDText, value.Revision),
			Context: contextValue,
		}
		if err := validateCreativeTaskFanoutContext("creative_order_item_production", orderItemID, []service.DirectTaskFanoutItem{fanoutItem}); err != nil {
			return queued, fmt.Errorf("validate selected creative production: %w", err)
		}
		if err := h.validateCreativeTaskFanoutExpectedSizes(ctx, value.WorkspaceID, "creative_order_item_production", orderItemID, []service.DirectTaskFanoutItem{fanoutItem}); err != nil {
			return queued, fmt.Errorf("validate selected creative delivery sizes: %w", err)
		}
		attr, requestedBy := creativeOrchestrationAttribution(cause, value.CreatedBy, "creative_order_item_production", orderItemID)
		groups, err := h.prepareCreativeProductionFanout(ctx, value.WorkspaceID, orderItemID, []service.DirectTaskFanoutItem{fanoutItem})
		if err != nil {
			return queued, fmt.Errorf("prepare selected creative production: %w", err)
		}
		tasks, newlyCreated, err := h.enqueueCreativeProductionFanoutTx(ctx, tx, orderItemID, groups, attr, requestedBy)
		if err != nil {
			return queued, fmt.Errorf("queue selected creative production: %w", err)
		}
		queued = append(queued, tasks...)
		created = append(created, newlyCreated...)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit selected creative production handoff: %w", err)
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(ctx, created)
	return queued, nil
}

// maybePromoteCreativeReserve replaces an exhausted initial selected variant
// only when it has never had an active delivery. Existing active revisions are
// immutable from this recovery path.
func (h *Handler) maybePromoteCreativeReserve(ctx context.Context, failedVariantID pgtype.UUID, cause creativeOrchestrationCause) (bool, []db.AgentTaskQueue, error) {
	if h.TxStarter == nil || !failedVariantID.Valid {
		return false, nil, nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, nil, fmt.Errorf("begin creative reserve promotion: %w", err)
	}
	defer tx.Rollback(ctx)

	var itemID, orderID pgtype.UUID
	var failedRevision, failedRank int
	var failedState, failedStatus string
	var previouslyPromoted bool
	var replacedByVariantID string
	var activeRevision pgtype.Int4
	var adoptedVariantID pgtype.UUID
	err = tx.QueryRow(ctx, `
SELECT item.id, item.order_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1
`, failedVariantID).Scan(&itemID, &orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("load failed creative selection: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "creative-reserve-promotion:"+uuidToString(itemID)); err != nil {
		return false, nil, fmt.Errorf("lock creative reserve promotion: %w", err)
	}
	var orderStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_order WHERE id = $1 FOR UPDATE`, orderID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) || orderStatus == "cancelled" {
		return false, nil, nil
	} else if err != nil {
		return false, nil, fmt.Errorf("lock creative reserve promotion order: %w", err)
	}
	var itemStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_order_item WHERE id = $1 AND order_id = $2 FOR UPDATE`, itemID, orderID).Scan(&itemStatus); errors.Is(err, pgx.ErrNoRows) || itemStatus == "cancelled" {
		return false, nil, nil
	} else if err != nil {
		return false, nil, fmt.Errorf("lock creative reserve promotion item: %w", err)
	}
	err = tx.QueryRow(ctx, `
SELECT variant.order_item_id, item.order_id, variant.revision, COALESCE(variant.selection_rank, 0),
       variant.candidate_state, variant.status, variant.active_revision, item.adopted_variant_id,
       COALESCE((variant.brief->>'reserve_promoted_automatically')::boolean, false),
       COALESCE(variant.brief->>'replaced_by_variant_id', '')
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND item.id = $2 AND order_row.id = $3
FOR UPDATE OF variant
`, failedVariantID, itemID, orderID).Scan(
		&itemID,
		&orderID,
		&failedRevision,
		&failedRank,
		&failedState,
		&failedStatus,
		&activeRevision,
		&adoptedVariantID,
		&previouslyPromoted,
		&replacedByVariantID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("reload failed creative selection: %w", err)
	}
	if previouslyPromoted && failedState == "rejected" {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			return false, nil, fmt.Errorf("release creative reserve promotion recovery: %w", err)
		}
		replacementID, parseErr := parseCreativeOrchestrationUUID(replacedByVariantID)
		if parseErr != nil {
			return false, nil, nil
		}
		tasks, queueErr := h.queueSelectedCreativeProductionTasks(ctx, itemID, cause, creativeReservePromotionPhase, replacementID)
		return false, tasks, queueErr
	}
	if failedState != "selected" || failedRank < 1 || failedRank > 3 || activeRevision.Valid || adoptedVariantID.Valid || (failedStatus != "action_required" && failedStatus != "failed") {
		return false, nil, nil
	}

	var activeProduction bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1 FROM agent_task_queue task
  WHERE task.trigger_evidence_kind = 'creative_order_item_production'
    AND task.trigger_evidence_ref_id = $1
    AND task.context->>'workflow' = 'creative_production'
    AND task.context->>'variant_id' = $2::text
	    AND (task.context->>'revision')::integer = $3
    AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
    AND ($4::uuid IS NULL OR task.id <> $4)
)
`, itemID, failedVariantID, failedRevision, creativeCauseParentTaskID(cause)).Scan(&activeProduction); err != nil {
		return false, nil, fmt.Errorf("check failed creative production attempts: %w", err)
	}
	if activeProduction {
		return false, nil, nil
	}

	var reserveID pgtype.UUID
	var reserveRevision, reserveRank int
	var reserveBrief, reservePrimarySize, triggerKind, inputSnapshot string
	err = tx.QueryRow(ctx, `
SELECT reserve.id, reserve.revision, reserve.selection_rank, reserve.brief::text, reserve.primary_size,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text
FROM creative_order_variant reserve
JOIN creative_order_item item ON item.id = reserve.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE reserve.order_item_id = $1
  AND reserve.candidate_state = 'reserve'
  AND reserve.selection_rank IS NOT NULL
  AND reserve.active_revision IS NULL
  AND EXISTS (
    SELECT 1 FROM creative_order_asset generated
    WHERE generated.variant_id = reserve.id AND generated.revision = reserve.revision
      AND generated.size_key = reserve.primary_size AND generated.stage = 'generated'
      AND generated.status = 'completed' AND generated.attachment_id IS NOT NULL
  )
  AND EXISTS (
    SELECT 1 FROM creative_order_asset primed
    WHERE primed.variant_id = reserve.id AND primed.revision = reserve.revision
      AND primed.size_key = reserve.primary_size AND primed.stage = 'primed'
      AND primed.status = 'completed' AND primed.attachment_id IS NOT NULL
  )
ORDER BY reserve.selection_rank, reserve.id
LIMIT 1
FOR UPDATE OF reserve
`, itemID).Scan(&reserveID, &reserveRevision, &reserveRank, &reserveBrief, &reservePrimarySize, &triggerKind, &inputSnapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil, nil
	}
	if err != nil {
		return false, nil, fmt.Errorf("load creative reserve: %w", err)
	}
	fullSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(reserveBrief))
	if err != nil {
		return false, nil, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET candidate_state = 'rejected', selection_rank = NULL,
    brief = brief || jsonb_build_object(
      'rejected_selection_rank', $2::integer,
      'replaced_by_variant_id', $3::text,
      'reserve_promoted_automatically', true
    ),
    updated_at = now()
WHERE id = $1 AND candidate_state = 'selected' AND active_revision IS NULL
`, failedVariantID, failedRank, uuidToString(reserveID)); err != nil {
		return false, nil, fmt.Errorf("reject exhausted creative selection: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET candidate_state = 'selected', selection_rank = $2, status = 'running',
    staging_revision = CASE WHEN active_revision = revision THEN NULL ELSE revision END,
    brief = brief || jsonb_build_object(
      'promoted_from_reserve_rank', $3::integer,
      'replaces_variant_id', $4::text,
      'reserve_promoted_automatically', true
    ),
    updated_at = now()
WHERE id = $1 AND candidate_state = 'reserve' AND active_revision IS NULL
`, reserveID, failedRank, reserveRank, uuidToString(failedVariantID)); err != nil {
		return false, nil, fmt.Errorf("promote creative reserve: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT brief::text FROM creative_order_variant WHERE id = $1`, reserveID).Scan(&reserveBrief); err != nil {
		return false, nil, fmt.Errorf("load promoted creative reserve brief: %w", err)
	}
	if err := upsertCreativeVariantRevision(ctx, tx, reserveID, reserveRevision, json.RawMessage(reserveBrief), "running", fullSizes); err != nil {
		return false, nil, fmt.Errorf("expand promoted creative reserve contract: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		return false, nil, fmt.Errorf("touch creative order reserve promotion: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, nil, fmt.Errorf("commit creative reserve promotion: %w", err)
	}

	tasks, err := h.queueSelectedCreativeProductionTasks(ctx, itemID, cause, creativeReservePromotionPhase, reserveID)
	if err != nil {
		return true, tasks, err
	}
	return true, tasks, nil
}

func creativeCauseParentTaskID(cause creativeOrchestrationCause) pgtype.UUID {
	if cause.ParentTask == nil {
		return pgtype.UUID{}
	}
	return cause.ParentTask.ID
}

// reconcileCreativeCandidateOrchestrationForProductionTask is the post-settle
// hook for daemon task completion. It is intentionally separate from task
// settlement so all reads observe committed terminal state. Candidate selection
// tasks also use this hook to compensate for a post-ranking fanout failure.
func (h *Handler) reconcileCreativeCandidateOrchestrationForProductionTask(ctx context.Context, task db.AgentTaskQueue) error {
	var taskContext struct {
		Type                string `json:"type"`
		Workflow            string `json:"workflow"`
		CreativeOrderItemID string `json:"creative_order_item_id"`
		VariantID           string `json:"variant_id"`
	}
	if json.Unmarshal(task.Context, &taskContext) != nil || taskContext.Type != "creative_domain_task" {
		return nil
	}
	itemID, err := parseCreativeOrchestrationUUID(taskContext.CreativeOrderItemID)
	if err != nil {
		return nil
	}
	cause := creativeOrchestrationCause{ParentTask: &task, RequestedBy: task.RequestingUserID}
	if taskContext.Workflow == creativeCandidateSelectionWorkflow {
		var orderID pgtype.UUID
		var workspaceID pgtype.UUID
		if err := h.DB.QueryRow(ctx, `
SELECT item.order_id, order_row.workspace_id
FROM creative_order_item item
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1
`, itemID).Scan(&orderID, &workspaceID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
		committed, err := h.creativeCandidateSelectionCommitted(ctx, workspaceID, orderID, itemID)
		if err != nil {
			return err
		}
		if committed {
			_, err := h.queueSelectedCreativeProductionTasks(ctx, itemID, cause, creativeSelectedExpansionPhase)
			return err
		}
		if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
			_, err := h.maybeQueueCreativeCandidateSelection(ctx, itemID, cause)
			return err
		}
		return nil
	}
	if taskContext.Workflow != "creative_production" {
		return nil
	}
	variantID, err := parseCreativeOrchestrationUUID(taskContext.VariantID)
	if err != nil {
		return nil
	}
	if _, err := h.maybeQueueCreativeCandidateSelection(ctx, itemID, cause); err != nil {
		return err
	}
	if _, _, err := h.maybePromoteCreativeReserve(ctx, variantID, cause); err != nil {
		return err
	}
	return nil
}

func parseCreativeOrchestrationUUID(value string) (pgtype.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.UUID{}, errors.New("creative orchestration UUID is empty")
	}
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return pgtype.UUID{}, errors.New("creative orchestration UUID is invalid")
	}
	return id, nil
}
