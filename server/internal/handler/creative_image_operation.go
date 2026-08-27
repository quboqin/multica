package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var creativeImageOperationSHA256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type creativeImageOperationInput struct {
	VariantID          string          `json:"variant_id"`
	SizeKey            string          `json:"size_key"`
	Revision           int             `json:"revision"`
	OperationKind      string          `json:"operation_kind"`
	IdempotencyKey     string          `json:"idempotency_key"`
	Status             string          `json:"status"`
	Model              string          `json:"model"`
	RuntimeID          string          `json:"runtime_id"`
	PromptSHA256       string          `json:"prompt_sha256"`
	InputSnapshot      json.RawMessage `json:"input_snapshot"`
	Attempt            int             `json:"attempt"`
	ProviderRequestID  string          `json:"provider_request_id"`
	ProviderStatus     string          `json:"provider_status"`
	HTTPStatus         *int            `json:"http_status"`
	ExitCode           *int            `json:"exit_code"`
	ErrorType          string          `json:"error_type"`
	ErrorMessage       string          `json:"error_message"`
	ResultReceipt      json.RawMessage `json:"result_receipt"`
	OutputAttachmentID string          `json:"output_attachment_id"`
	DurationMS         *int64          `json:"duration_ms"`
	ReconcileConfirmed bool            `json:"reconcile_confirmed"`
}

type creativeImageOperationAttemptResponse struct {
	ID                 string          `json:"id"`
	Attempt            int             `json:"attempt"`
	Status             string          `json:"status"`
	RuntimeID          string          `json:"runtime_id"`
	TaskID             string          `json:"task_id"`
	ProviderRequestID  string          `json:"provider_request_id"`
	ProviderStatus     string          `json:"provider_status"`
	HTTPStatus         *int            `json:"http_status"`
	ExitCode           *int            `json:"exit_code"`
	ErrorType          string          `json:"error_type"`
	ErrorMessage       string          `json:"error_message"`
	ResultReceipt      json.RawMessage `json:"result_receipt"`
	OutputAttachmentID string          `json:"output_attachment_id"`
	DurationMS         *int64          `json:"duration_ms"`
	StartedAt          string          `json:"started_at"`
	CompletedAt        string          `json:"completed_at"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type creativeImageOperationResponse struct {
	Disposition        string                                  `json:"disposition,omitempty"`
	ID                 string                                  `json:"id"`
	VariantID          string                                  `json:"variant_id"`
	SizeKey            string                                  `json:"size_key"`
	Revision           int                                     `json:"revision"`
	OperationKind      string                                  `json:"operation_kind"`
	IdempotencyKey     string                                  `json:"idempotency_key"`
	Status             string                                  `json:"status"`
	Model              string                                  `json:"model"`
	RuntimeID          string                                  `json:"runtime_id"`
	TaskID             string                                  `json:"task_id"`
	PromptSHA256       string                                  `json:"prompt_sha256"`
	InputSnapshot      json.RawMessage                         `json:"input_snapshot"`
	ProviderRequestID  string                                  `json:"provider_request_id"`
	ResultReceipt      json.RawMessage                         `json:"result_receipt"`
	ErrorType          string                                  `json:"error_type"`
	ErrorMessage       string                                  `json:"error_message"`
	OutputAttachmentID string                                  `json:"output_attachment_id"`
	OutputAssetID      string                                  `json:"output_asset_id"`
	StartedAt          string                                  `json:"started_at"`
	CompletedAt        string                                  `json:"completed_at"`
	CreatedAt          string                                  `json:"created_at"`
	UpdatedAt          string                                  `json:"updated_at"`
	Attempts           []creativeImageOperationAttemptResponse `json:"attempts"`
}

func validCreativeImageOperationKind(kind string) bool {
	return kind == "generation" || kind == "visual_rework" || kind == "direct_edit" || kind == "canvas_repair"
}

func validCreativeImageOperationStatus(status string) bool {
	return status == "queued" || status == "running" || status == "unknown" || status == "completed" || status == "failed" || status == "cancelled"
}

type creativeTaskImageScopeContext struct {
	Type                  string          `json:"type"`
	Workflow              string          `json:"workflow"`
	ExpectedSizes         []string        `json:"expected_sizes"`
	MissingSizes          json.RawMessage `json:"missing_sizes"`
	EditSizes             json.RawMessage `json:"edit_sizes"`
	TargetSize            string          `json:"target_size"`
	LateReceiptRecovery   json.RawMessage `json:"late_receipt_recovery"`
	LateReceiptRecoveries json.RawMessage `json:"late_receipt_recoveries"`
	QCVisualRework        struct {
		TargetSizes json.RawMessage `json:"target_sizes"`
	} `json:"qc_visual_rework"`
	DirectEdit struct {
		EditSizes  json.RawMessage `json:"edit_sizes"`
		TargetSize string          `json:"target_size"`
	} `json:"direct_edit"`
}

func parseCreativeTaskSizeArray(raw json.RawMessage, field string) ([]string, error) {
	var sizes []string
	if err := json.Unmarshal(raw, &sizes); err != nil {
		return nil, fmt.Errorf("creative task %s must be an array of sizes", field)
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("creative task %s cannot be empty", field)
	}
	return sizes, nil
}

func creativeTaskImageScopeSizes(raw json.RawMessage) ([]string, error) {
	var taskContext creativeTaskImageScopeContext
	if json.Unmarshal(raw, &taskContext) != nil || taskContext.Type != "creative_domain_task" {
		return nil, errors.New("creative task image scope is invalid")
	}
	var sizes []string
	var err error
	switch taskContext.Workflow {
	case "creative_production":
		switch {
		case len(taskContext.LateReceiptRecoveries) > 0:
			var recoveries []struct {
				SizeKey string `json:"size_key"`
			}
			if json.Unmarshal(taskContext.LateReceiptRecoveries, &recoveries) != nil || len(recoveries) == 0 {
				return nil, errors.New("creative task late_receipt_recoveries must contain size keys")
			}
			for _, recovery := range recoveries {
				sizes = append(sizes, recovery.SizeKey)
			}
		case len(taskContext.LateReceiptRecovery) > 0:
			var recovery struct {
				SizeKey string `json:"size_key"`
			}
			if json.Unmarshal(taskContext.LateReceiptRecovery, &recovery) != nil || strings.TrimSpace(recovery.SizeKey) == "" {
				return nil, errors.New("creative task late_receipt_recovery must contain a size key")
			}
			sizes = []string{recovery.SizeKey}
		case len(taskContext.MissingSizes) > 0:
			sizes, err = parseCreativeTaskSizeArray(taskContext.MissingSizes, "missing_sizes")
		case len(taskContext.QCVisualRework.TargetSizes) > 0:
			sizes, err = parseCreativeTaskSizeArray(taskContext.QCVisualRework.TargetSizes, "qc_visual_rework.target_sizes")
		default:
			sizes = taskContext.ExpectedSizes
		}
	case "creative_direct_edit":
		switch {
		case len(taskContext.EditSizes) > 0:
			sizes, err = parseCreativeTaskSizeArray(taskContext.EditSizes, "edit_sizes")
		case len(taskContext.DirectEdit.EditSizes) > 0:
			sizes, err = parseCreativeTaskSizeArray(taskContext.DirectEdit.EditSizes, "direct_edit.edit_sizes")
		case strings.TrimSpace(taskContext.TargetSize) != "":
			sizes = []string{taskContext.TargetSize}
		case strings.TrimSpace(taskContext.DirectEdit.TargetSize) != "":
			sizes = []string{taskContext.DirectEdit.TargetSize}
		default:
			sizes = taskContext.ExpectedSizes
		}
	default:
		return nil, errors.New("creative task workflow cannot perform image operations")
	}
	if err != nil {
		return nil, err
	}
	if len(sizes) == 0 {
		return nil, errors.New("creative task image scope is empty")
	}
	normalized := make([]string, 0, len(sizes))
	seen := make(map[string]struct{}, len(sizes))
	for _, value := range sizes {
		size := strings.TrimSpace(value)
		if !validCreativeAssetSize(size) {
			return nil, errors.New("creative task image scope contains an invalid size")
		}
		if _, exists := seen[size]; exists {
			continue
		}
		seen[size] = struct{}{}
		normalized = append(normalized, size)
	}
	return normalized, nil
}

func creativeTaskAllowsImageSize(raw json.RawMessage, size string) (bool, error) {
	sizes, err := creativeTaskImageScopeSizes(raw)
	if err != nil {
		return false, err
	}
	return slices.Contains(sizes, size), nil
}

// reconcileCreativeImageOperationsForTerminalTask preserves provider calls
// whose local task ended before a durable result receipt arrived. Unknown is
// intentionally non-terminal: the reconciler may still accept a late success,
// while callers must not invoke the model again for the same attempt.
func (h *Handler) reconcileCreativeImageOperationsForTerminalTask(ctx context.Context, task db.AgentTaskQueue) error {
	if task.Status != "completed" && task.Status != "failed" && task.Status != "cancelled" {
		return nil
	}
	var taskContext struct {
		Type     string `json:"type"`
		Workflow string `json:"workflow"`
	}
	if json.Unmarshal(task.Context, &taskContext) != nil || taskContext.Type != "creative_domain_task" ||
		(taskContext.Workflow != "creative_production" && taskContext.Workflow != "creative_direct_edit") {
		return nil
	}
	if h.TxStarter == nil {
		return nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin creative image operation reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
WITH unsettled_attempt AS (
  UPDATE creative_image_operation_attempt attempt
  SET status = 'unknown', completed_at = NULL, updated_at = now()
  FROM creative_image_operation operation
  WHERE attempt.operation_id = operation.id
    AND attempt.task_id = $1
    AND attempt.status = 'running'
    AND attempt.attempt = (
      SELECT max(latest.attempt)
      FROM creative_image_operation_attempt latest
      WHERE latest.operation_id = operation.id
    )
    AND operation.status IN ('queued', 'running')
    AND operation.output_attachment_id IS NULL
    AND operation.output_asset_id IS NULL
  RETURNING attempt.operation_id
)
UPDATE creative_image_operation operation
SET status = 'unknown', completed_at = NULL, updated_at = now()
WHERE operation.id IN (SELECT operation_id FROM unsettled_attempt)
  AND operation.status IN ('queued', 'running')
`, task.ID); err != nil {
		return fmt.Errorf("reconcile creative image operations for terminal task: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit creative image operation reconciliation: %w", err)
	}
	return nil
}

// reconcileCreativeLifecycleForCancelledTask closes the mutable revision for
// every cancellation entry point. Explicit cancellation never queues another
// model call or promotes a reserve; it leaves an active revision untouched and
// exposes an unfinished staging revision for deliberate recovery.
func (h *Handler) reconcileCreativeLifecycleForCancelledTask(ctx context.Context, task db.AgentTaskQueue) error {
	var taskContext struct {
		Type      string `json:"type"`
		Workflow  string `json:"workflow"`
		VariantID string `json:"variant_id"`
		Revision  int    `json:"revision"`
	}
	if json.Unmarshal(task.Context, &taskContext) != nil || taskContext.Type != "creative_domain_task" ||
		(taskContext.Workflow != "creative_production" && taskContext.Workflow != "creative_direct_edit") || taskContext.Revision < 1 {
		return nil
	}
	variantID, err := parseUUIDString(strings.TrimSpace(taskContext.VariantID))
	if err != nil || h.TxStarter == nil {
		return nil
	}
	var orderID pgtype.UUID
	if err := h.DB.QueryRow(ctx, `
SELECT item.order_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1
`, variantID).Scan(&orderID); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("resolve cancelled creative order: %w", err)
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin cancelled creative lifecycle reconciliation: %w", err)
	}
	defer tx.Rollback(ctx)
	var triggerKind, inputSnapshot, orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status, trigger_evidence_kind, input_snapshot::text
FROM creative_order
WHERE id = $1
FOR UPDATE
`, orderID).Scan(&orderStatus, &triggerKind, &inputSnapshot); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("lock cancelled creative order: %w", err)
	}
	var currentRevision int
	var brief, candidateState, primarySize, variantStatus string
	var activeRevision, stagingRevision pgtype.Int4
	err = tx.QueryRow(ctx, `
SELECT variant.revision, variant.status, variant.active_revision, variant.staging_revision,
       variant.brief::text, variant.candidate_state, variant.primary_size
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant
`, variantID, orderID).Scan(
		&currentRevision, &variantStatus, &activeRevision, &stagingRevision,
		&brief, &candidateState, &primarySize,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load cancelled creative revision: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_prime_composition_job
SET status = 'cancelled', lease_token = NULL, lease_expires_at = NULL,
    last_error = 'creative production task cancelled', completed_at = now(), updated_at = now()
WHERE variant_id = $1 AND revision = $2 AND status IN ('queued', 'running', 'failed')
`, variantID, taskContext.Revision); err != nil {
		return fmt.Errorf("cancel creative Prime composition job: %w", err)
	}
	if currentRevision != taskContext.Revision {
		return tx.Commit(ctx)
	}
	if activeRevision.Valid && int(activeRevision.Int32) == currentRevision && !stagingRevision.Valid {
		return tx.Commit(ctx)
	}
	expectedSizes, err := expectedCreativeVariantProductionSizes(
		triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief), candidateState, primarySize,
	)
	if err != nil {
		return fmt.Errorf("resolve cancelled creative revision scope: %w", err)
	}
	targetStatus := "action_required"
	if orderStatus == "cancelled" || variantStatus == "cancelled" {
		targetStatus = "cancelled"
	}
	if err := tx.QueryRow(ctx, `
UPDATE creative_order_variant
SET status = $3,
    brief = brief - 'prime_composition_pending',
    staging_revision = CASE WHEN active_revision = $2 THEN staging_revision ELSE $2 END,
    updated_at = now()
WHERE id = $1 AND revision = $2
RETURNING brief::text
`, variantID, currentRevision, targetStatus).Scan(&brief); err != nil {
		return fmt.Errorf("settle cancelled creative variant: %w", err)
	}
	if err := upsertCreativeVariantRevision(
		ctx, tx, variantID, currentRevision, json.RawMessage(brief), targetStatus, expectedSizes,
	); err != nil {
		return fmt.Errorf("settle cancelled creative revision: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit cancelled creative lifecycle reconciliation: %w", err)
	}
	return nil
}

// reconcileCreativeLifecycleForFailedTask is shared by every bulk failure
// path, including timeout and runtime-offline sweepers. The generic task retry
// is created before this hook runs, so production settlement observes that
// active continuation and cannot enqueue a second model task.
func (h *Handler) reconcileCreativeLifecycleForFailedTask(ctx context.Context, task db.AgentTaskQueue) error {
	var reconciliationErrors []error
	if err := h.reconcileCreativeImageOperationsForTerminalTask(ctx, task); err != nil {
		reconciliationErrors = append(reconciliationErrors, err)
	}
	if err := h.settleCreativeProductionVariantTask(ctx, task); err != nil {
		reconciliationErrors = append(reconciliationErrors, err)
	} else if err := h.reconcileCreativeCandidateOrchestrationForProductionTask(ctx, task); err != nil {
		reconciliationErrors = append(reconciliationErrors, err)
	}
	if err := h.settleCreativeDirectEditTask(ctx, task); err != nil {
		reconciliationErrors = append(reconciliationErrors, err)
	}
	return errors.Join(reconciliationErrors...)
}

func normalizeCreativeImageOperation(input creativeImageOperationInput) (creativeImageOperationInput, error) {
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.SizeKey = strings.TrimSpace(input.SizeKey)
	input.OperationKind = strings.TrimSpace(input.OperationKind)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.Status = strings.TrimSpace(input.Status)
	input.Model = strings.TrimSpace(input.Model)
	input.RuntimeID = strings.TrimSpace(input.RuntimeID)
	input.PromptSHA256 = strings.TrimSpace(strings.ToLower(input.PromptSHA256))
	input.ProviderRequestID = strings.TrimSpace(input.ProviderRequestID)
	input.ProviderStatus = strings.TrimSpace(input.ProviderStatus)
	input.ErrorType = strings.TrimSpace(input.ErrorType)
	input.ErrorMessage = strings.TrimSpace(input.ErrorMessage)
	input.OutputAttachmentID = strings.TrimSpace(input.OutputAttachmentID)
	if input.VariantID == "" || !validCreativeAssetSize(input.SizeKey) || input.Revision < 1 ||
		!validCreativeImageOperationKind(input.OperationKind) || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 240 ||
		!validCreativeImageOperationStatus(input.Status) || input.Attempt < 1 || len(input.ErrorMessage) > 4000 {
		return input, errors.New("invalid creative image operation")
	}
	if input.PromptSHA256 != "" && !creativeImageOperationSHA256Pattern.MatchString(input.PromptSHA256) {
		return input, errors.New("creative image operation prompt_sha256 is invalid")
	}
	var err error
	input.InputSnapshot, err = normalizedOptionalJSONObject(input.InputSnapshot)
	if err != nil {
		return input, errors.New("creative image operation input_snapshot must be an object")
	}
	input.ResultReceipt, err = normalizedOptionalJSONObject(input.ResultReceipt)
	if err != nil {
		return input, errors.New("creative image operation result_receipt must be an object")
	}
	if input.HTTPStatus != nil && (*input.HTTPStatus < 100 || *input.HTTPStatus > 599) {
		return input, errors.New("creative image operation http_status is invalid")
	}
	if input.DurationMS != nil && *input.DurationMS < 0 {
		return input, errors.New("creative image operation duration_ms is invalid")
	}
	if input.Status == "failed" && input.ErrorType == "" {
		return input, errors.New("failed creative image operation error_type is required")
	}
	if input.Status == "completed" && (input.OutputAttachmentID == "" || input.ProviderRequestID == "" || input.PromptSHA256 == "" || string(input.ResultReceipt) == "{}") {
		return input, errors.New("completed creative image operation requires prompt_sha256, provider_request_id, result_receipt, and output_attachment_id")
	}
	if input.ReconcileConfirmed && input.Status != "failed" {
		return input, errors.New("reconcile_confirmed is only valid for a failed creative image operation")
	}
	return input, nil
}

func (h *Handler) UpsertCreativeImageOperation(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	taskActor := r.Header.Get("X-Actor-Source") == "task_token"
	if !taskActor {
		writeError(w, http.StatusForbidden, "image operation mutation requires a task token")
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativeImageOperationInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative image operation")
		return
	}
	input, err := normalizeCreativeImageOperation(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	runtimeID, ok := optionalUUIDOrBadRequest(w, input.RuntimeID, "runtime_id")
	if !ok {
		return
	}
	outputAttachmentID, ok := optionalUUIDOrBadRequest(w, input.OutputAttachmentID, "output_attachment_id")
	if !ok {
		return
	}
	taskID := pgtype.UUID{}
	if taskActor {
		taskID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
		if !ok {
			return
		}
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative image operation")
		return
	}
	defer tx.Rollback(r.Context())
	// Lock the authenticated task before any creative rows. Task cancellation
	// uses the same row lock, so a request that loses that race observes the
	// terminal status and cannot persist an operation or attempt afterward.
	var taskRuntimeID pgtype.UUID
	var taskStatus, taskContextRaw string
	err = tx.QueryRow(r.Context(), `
SELECT task.runtime_id, task.status, task.context::text
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id
WHERE task.id = $1
  AND agent.workspace_id = $2
  AND task.context->>'variant_id' = $3::uuid::text
  AND task.context->>'workflow' IN ('creative_production', 'creative_direct_edit')
  AND task.context->>'revision' = $4::int::text
FOR UPDATE OF task
`, taskID, workspaceID, variantID, input.Revision).Scan(&taskRuntimeID, &taskStatus, &taskContextRaw)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to validate creative image operation task runtime")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) || !taskRuntimeID.Valid {
		writeError(w, http.StatusUnprocessableEntity, "image operation task is not bound to an active runtime or operation scope")
		return
	}
	if taskStatus != "dispatched" && taskStatus != "running" {
		writeError(w, http.StatusConflict, "image operation task is no longer active")
		return
	}
	if allowed, scopeErr := creativeTaskAllowsImageSize(json.RawMessage(taskContextRaw), input.SizeKey); scopeErr != nil || !allowed {
		writeError(w, http.StatusUnprocessableEntity, "image operation size is outside the task's current execution scope")
		return
	}
	if runtimeID.Valid && runtimeID != taskRuntimeID {
		writeError(w, http.StatusConflict, "image operation runtime_id does not match the task runtime")
		return
	}
	runtimeID = taskRuntimeID

	var orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative image operation order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}

	var variantRevision int
	var triggerKind, inputSnapshot, brief, candidateState, primarySize, variantStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT variant.revision, order_row.trigger_evidence_kind, order_row.input_snapshot::text,
       variant.brief::text, variant.candidate_state, variant.primary_size, variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
FOR UPDATE OF variant
`, variantID, orderID, workspaceID).Scan(&variantRevision, &triggerKind, &inputSnapshot, &brief, &candidateState, &primarySize, &variantStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative image operation variant")
		return
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative image operation revision is stale")
		return
	}
	if variantStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order variant is cancelled")
		return
	}
	if candidateState == "reserve" || candidateState == "rejected" {
		writeError(w, http.StatusConflict, "inactive creative candidates cannot start image operations")
		return
	}
	expectedSizes, err := expectedCreativeVariantProductionSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief), candidateState, primarySize)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if !creativeSizeIsExpected(input.SizeKey, expectedSizes) {
		writeError(w, http.StatusUnprocessableEntity, "image operation size is outside this creative variant's current production scope")
		return
	}
	var referencesValid bool
	if err := tx.QueryRow(r.Context(), `
SELECT ($1::uuid IS NULL OR EXISTS(SELECT 1 FROM agent_runtime runtime WHERE runtime.id = $1 AND runtime.workspace_id = $3))
   AND ($2::uuid IS NULL OR EXISTS(SELECT 1 FROM attachment WHERE id = $2 AND workspace_id = $3))
   AND ($4::uuid IS NULL OR EXISTS(
     SELECT 1 FROM agent_task_queue task JOIN agent ON agent.id = task.agent_id
     WHERE task.id = $4 AND agent.workspace_id = $3
       AND task.context->>'variant_id' = $5::uuid::text
       AND task.context->>'workflow' IN ('creative_production', 'creative_direct_edit')
       AND task.context->>'revision' = $6::int::text
   ))
`, runtimeID, outputAttachmentID, workspaceID, taskID, variantID, input.Revision).Scan(&referencesValid); err != nil || !referencesValid {
		writeError(w, http.StatusUnprocessableEntity, "image operation references do not belong to this workspace or variant")
		return
	}

	var operationID pgtype.UUID
	var existingStatus, existingSize, existingKind, existingIdempotencyKey string
	var existingModel, existingPromptSHA256, existingInputSnapshot, existingProviderRequestID string
	var existingRevision int
	var existingOutputAttachmentID, existingTaskID pgtype.UUID
	err = tx.QueryRow(r.Context(), `
SELECT id, status, size_key, revision, operation_kind, idempotency_key, output_attachment_id,
       model, prompt_sha256, input_snapshot::text, provider_request_id, task_id
FROM creative_image_operation
WHERE variant_id = $1
  AND (
    idempotency_key = $2
    OR (revision = $3 AND size_key = $4 AND operation_kind = $5)
  )
ORDER BY (revision = $3 AND size_key = $4 AND operation_kind = $5) DESC
LIMIT 1
FOR UPDATE
	`, variantID, input.IdempotencyKey, input.Revision, input.SizeKey, input.OperationKind).Scan(
		&operationID, &existingStatus, &existingSize, &existingRevision, &existingKind, &existingIdempotencyKey, &existingOutputAttachmentID,
		&existingModel, &existingPromptSHA256, &existingInputSnapshot, &existingProviderRequestID, &existingTaskID,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to lock creative image operation")
		return
	}
	disposition := ""
	respondExisting := func(value string) {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to recover creative image operation")
			return
		}
		existing, loadErr := h.loadCreativeImageOperation(r.Context(), operationID)
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to recover creative image operation")
			return
		}
		existing.Disposition = value
		writeJSON(w, http.StatusOK, existing)
	}
	if err == nil {
		if existingSize != input.SizeKey || existingRevision != input.Revision || existingKind != input.OperationKind {
			writeError(w, http.StatusConflict, "creative image operation idempotency coordinates are immutable")
			return
		}
		inputSnapshotChanged := false
		if string(input.InputSnapshot) != "{}" {
			if err := tx.QueryRow(r.Context(), `SELECT $1::jsonb <> $2::jsonb`, input.InputSnapshot, existingInputSnapshot).Scan(&inputSnapshotChanged); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to compare creative image operation input snapshot")
				return
			}
		}
		if (input.Model != "" && input.Model != existingModel) ||
			(input.PromptSHA256 != "" && input.PromptSHA256 != existingPromptSHA256) ||
			inputSnapshotChanged {
			writeError(w, http.StatusConflict, "creative image operation model, prompt, and input snapshot are immutable")
			return
		}
		if taskID.Valid && existingTaskID.Valid && taskID != existingTaskID {
			var taskDescendsFromOperation bool
			if err := tx.QueryRow(r.Context(), `
WITH RECURSIVE task_lineage AS (
  SELECT id, parent_task_id, retry_of_task_id
  FROM agent_task_queue
  WHERE id = $1
  UNION
  SELECT parent.id, parent.parent_task_id, parent.retry_of_task_id
  FROM agent_task_queue parent
  JOIN task_lineage child
    ON parent.id = child.parent_task_id OR parent.id = child.retry_of_task_id
)
SELECT EXISTS (SELECT 1 FROM task_lineage WHERE id = $2)
`, taskID, existingTaskID).Scan(&taskDescendsFromOperation); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to validate creative image operation task lineage")
				return
			}
			if !taskDescendsFromOperation {
				writeError(w, http.StatusConflict, "creative image operation can only be settled by its task continuation chain")
				return
			}
		}
		var latestAttempt int
		if err := tx.QueryRow(r.Context(), `
SELECT COALESCE(max(attempt), 0)
FROM creative_image_operation_attempt
WHERE operation_id = $1
		`, operationID).Scan(&latestAttempt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load creative image operation attempts")
			return
		}
		var attemptProviderRequestID string
		if err := tx.QueryRow(r.Context(), `
SELECT provider_request_id
FROM creative_image_operation_attempt
WHERE operation_id = $1 AND attempt = $2
`, operationID, input.Attempt).Scan(&attemptProviderRequestID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to validate creative image operation attempt lineage")
			return
		} else if err == nil && attemptProviderRequestID != "" && input.ProviderRequestID != "" && attemptProviderRequestID != input.ProviderRequestID {
			writeError(w, http.StatusConflict, "creative image operation attempt provider request is immutable")
			return
		}
		if existingStatus == "completed" {
			if input.Status != "completed" || (outputAttachmentID.Valid && existingOutputAttachmentID != outputAttachmentID) {
				writeError(w, http.StatusConflict, "completed creative image operation is immutable")
				return
			}
			respondExisting("reuse")
			return
		}
		switch existingStatus {
		case "failed":
			switch {
			case input.Status == "completed" && input.Attempt <= latestAttempt:
				disposition = "reuse"
			case input.Status == "failed" && input.Attempt == latestAttempt:
				respondExisting("terminal")
				return
			case input.Status == "running" && input.Attempt == latestAttempt+1:
				disposition = "invoke"
			default:
				writeError(w, http.StatusConflict, "failed creative image operation requires the next attempt before invocation")
				return
			}
		case "cancelled":
			if input.Status == "cancelled" && input.Attempt == latestAttempt {
				respondExisting("terminal")
				return
			}
			writeError(w, http.StatusConflict, "cancelled creative image operation is immutable")
			return
		case "queued":
			if input.Attempt != latestAttempt {
				respondExisting("reconcile")
				return
			}
			if input.Status == "running" {
				disposition = "invoke"
			} else if input.Status == "completed" {
				disposition = "reuse"
			} else if input.Status == "failed" || input.Status == "cancelled" {
				disposition = "terminal"
			} else {
				respondExisting("reconcile")
				return
			}
		case "running", "unknown":
			if existingStatus == "unknown" && input.Status == "failed" &&
				(!input.ReconcileConfirmed || input.ErrorType != "provider_receipt_not_found") {
				writeError(w, http.StatusConflict, "unknown creative image operation requires confirmed provider receipt reconciliation")
				return
			}
			if existingStatus == "unknown" && input.Status == "cancelled" {
				writeError(w, http.StatusConflict, "unknown creative image operation cannot be cancelled before provider receipt reconciliation")
				return
			}
			if input.Status != "completed" && input.Attempt != latestAttempt {
				respondExisting("reconcile")
				return
			}
			if input.Status == "completed" && input.Attempt <= latestAttempt {
				disposition = "reuse"
			} else if input.Status == "failed" || input.Status == "cancelled" {
				disposition = "terminal"
			} else if input.Status == "unknown" {
				disposition = "reconcile"
			} else {
				respondExisting("reconcile")
				return
			}
		}
	} else {
		if !taskActor {
			writeError(w, http.StatusForbidden, "new creative image operations require a task token")
			return
		}
		if input.Attempt != 1 {
			writeError(w, http.StatusConflict, "new creative image operation must start at attempt 1")
			return
		}
		if input.Status != "queued" && input.Status != "running" {
			writeError(w, http.StatusConflict, "new creative image operation must start queued or running")
			return
		}
		if input.Model == "" || input.PromptSHA256 == "" || string(input.InputSnapshot) == "{}" {
			writeError(w, http.StatusBadRequest, "new creative image operation requires model, prompt_sha256, and input_snapshot")
			return
		}
		switch input.Status {
		case "running":
			disposition = "invoke"
		case "completed":
			disposition = "reuse"
		case "failed", "cancelled":
			disposition = "terminal"
		default:
			disposition = "reconcile"
		}
	}
	if !operationID.Valid {
		if err := tx.QueryRow(r.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status, model, runtime_id, task_id,
  prompt_sha256, input_snapshot, provider_request_id, result_receipt, error_type, error_message,
  output_attachment_id, started_at, completed_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::jsonb,$12,$13::jsonb,$14,$15,$16,
  CASE WHEN $6 IN ('running','unknown','completed','failed') THEN now() ELSE NULL END,
  CASE WHEN $6 IN ('completed','failed','cancelled') THEN now() ELSE NULL END)
RETURNING id
`, variantID, input.SizeKey, input.Revision, input.OperationKind, input.IdempotencyKey, input.Status, input.Model,
			runtimeID, taskID, input.PromptSHA256, input.InputSnapshot, input.ProviderRequestID, input.ResultReceipt,
			input.ErrorType, input.ErrorMessage, outputAttachmentID).Scan(&operationID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create creative image operation")
			return
		}
	} else {
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation
SET status = $2,
    model = CASE WHEN model = '' THEN $3 ELSE model END,
		runtime_id = COALESCE($4, runtime_id), task_id = COALESCE(task_id, $5),
    prompt_sha256 = CASE WHEN prompt_sha256 = '' THEN $6 ELSE prompt_sha256 END,
    input_snapshot = CASE WHEN input_snapshot = '{}'::jsonb THEN $7::jsonb ELSE input_snapshot END,
    provider_request_id = CASE WHEN $8 = '' THEN provider_request_id ELSE $8 END,
    result_receipt = CASE WHEN $9::jsonb = '{}'::jsonb THEN result_receipt ELSE $9::jsonb END,
    error_type = CASE WHEN $10 = '' THEN error_type ELSE $10 END,
    error_message = CASE WHEN $11 = '' THEN error_message ELSE $11 END,
    output_attachment_id = COALESCE($12, output_attachment_id),
    started_at = CASE WHEN started_at IS NULL AND $2 IN ('running','unknown','completed','failed') THEN now() ELSE started_at END,
    completed_at = CASE WHEN $2 IN ('completed','failed','cancelled') THEN COALESCE(completed_at, now()) ELSE NULL END,
    updated_at = now()
WHERE id = $1
`, operationID, input.Status, input.Model, runtimeID, taskID, input.PromptSHA256, input.InputSnapshot,
			input.ProviderRequestID, input.ResultReceipt, input.ErrorType, input.ErrorMessage, outputAttachmentID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update creative image operation")
			return
		}
	}
	attemptStatus := input.Status
	if attemptStatus == "queued" {
		attemptStatus = "running"
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_image_operation_attempt (
  operation_id, attempt, status, runtime_id, task_id, provider_request_id, provider_status,
  http_status, exit_code, error_type, error_message, result_receipt, output_attachment_id,
  duration_ms, completed_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$14,
  CASE WHEN $3 IN ('completed','failed','cancelled') THEN now() ELSE NULL END)
ON CONFLICT (operation_id, attempt) DO UPDATE SET
  status = EXCLUDED.status,
	runtime_id = COALESCE(creative_image_operation_attempt.runtime_id, EXCLUDED.runtime_id),
	task_id = COALESCE(creative_image_operation_attempt.task_id, EXCLUDED.task_id),
	provider_request_id = CASE WHEN creative_image_operation_attempt.provider_request_id = '' THEN EXCLUDED.provider_request_id ELSE creative_image_operation_attempt.provider_request_id END,
  provider_status = CASE WHEN EXCLUDED.provider_status = '' THEN creative_image_operation_attempt.provider_status ELSE EXCLUDED.provider_status END,
  http_status = COALESCE(EXCLUDED.http_status, creative_image_operation_attempt.http_status),
  exit_code = COALESCE(EXCLUDED.exit_code, creative_image_operation_attempt.exit_code),
  error_type = CASE WHEN EXCLUDED.error_type = '' THEN creative_image_operation_attempt.error_type ELSE EXCLUDED.error_type END,
  error_message = CASE WHEN EXCLUDED.error_message = '' THEN creative_image_operation_attempt.error_message ELSE EXCLUDED.error_message END,
  result_receipt = CASE WHEN EXCLUDED.result_receipt = '{}'::jsonb THEN creative_image_operation_attempt.result_receipt ELSE EXCLUDED.result_receipt END,
  output_attachment_id = COALESCE(EXCLUDED.output_attachment_id, creative_image_operation_attempt.output_attachment_id),
  duration_ms = COALESCE(EXCLUDED.duration_ms, creative_image_operation_attempt.duration_ms),
  completed_at = CASE WHEN EXCLUDED.status IN ('completed','failed','cancelled') THEN COALESCE(creative_image_operation_attempt.completed_at, now()) ELSE NULL END,
  updated_at = now()
WHERE creative_image_operation_attempt.status <> 'completed' OR EXCLUDED.status = 'completed'
`, operationID, input.Attempt, attemptStatus, runtimeID, taskID, input.ProviderRequestID, input.ProviderStatus,
		input.HTTPStatus, input.ExitCode, input.ErrorType, input.ErrorMessage, input.ResultReceipt, outputAttachmentID, input.DurationMS); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative image operation attempt")
		return
	}
	if input.Status == "completed" {
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation_attempt
SET status = 'cancelled',
    error_type = CASE WHEN error_type = '' THEN 'superseded_by_completed_attempt' ELSE error_type END,
    completed_at = COALESCE(completed_at, now()),
    updated_at = now()
WHERE operation_id = $1
  AND attempt <> $2
  AND status IN ('running', 'unknown')
`, operationID, input.Attempt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to supersede creative image operation attempts")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative image operation")
		return
	}
	operation, err := h.loadCreativeImageOperation(r.Context(), operationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative image operation")
		return
	}
	operation.Disposition = disposition
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": chi.URLParam(r, "id"), "variant_id": input.VariantID,
		"revision": input.Revision, "size_key": input.SizeKey, "image_operation_id": operation.ID,
	})
	writeJSON(w, http.StatusOK, operation)
}

func (h *Handler) loadCreativeImageOperation(ctx context.Context, operationID pgtype.UUID) (creativeImageOperationResponse, error) {
	operation, err := scanCreativeImageOperation(h.DB.QueryRow(ctx, `
SELECT id::text, variant_id::text, size_key, revision, operation_kind, idempotency_key, status, model,
  COALESCE(runtime_id::text, ''), COALESCE(task_id::text, ''), prompt_sha256, input_snapshot::text,
  provider_request_id, result_receipt::text, error_type, error_message,
  COALESCE(output_attachment_id::text, ''), COALESCE(output_asset_id::text, ''),
  COALESCE(started_at::text, ''), COALESCE(completed_at::text, ''), created_at::text, updated_at::text
FROM creative_image_operation WHERE id = $1
`, operationID))
	if err != nil {
		return creativeImageOperationResponse{}, err
	}
	operation.Attempts, err = h.listCreativeImageOperationAttempts(ctx, operationID)
	return operation, err
}

func scanCreativeImageOperation(row rowScanner) (creativeImageOperationResponse, error) {
	var operation creativeImageOperationResponse
	var inputSnapshot, resultReceipt string
	err := row.Scan(&operation.ID, &operation.VariantID, &operation.SizeKey, &operation.Revision, &operation.OperationKind,
		&operation.IdempotencyKey, &operation.Status, &operation.Model, &operation.RuntimeID, &operation.TaskID,
		&operation.PromptSHA256, &inputSnapshot, &operation.ProviderRequestID, &resultReceipt, &operation.ErrorType,
		&operation.ErrorMessage, &operation.OutputAttachmentID, &operation.OutputAssetID, &operation.StartedAt,
		&operation.CompletedAt, &operation.CreatedAt, &operation.UpdatedAt)
	operation.InputSnapshot = json.RawMessage(inputSnapshot)
	operation.ResultReceipt = json.RawMessage(resultReceipt)
	return operation, err
}

func (h *Handler) listCreativeImageOperations(ctx context.Context, variantID pgtype.UUID) ([]creativeImageOperationResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id::text, variant_id::text, size_key, revision, operation_kind, idempotency_key, status, model,
  COALESCE(runtime_id::text, ''), COALESCE(task_id::text, ''), prompt_sha256, input_snapshot::text,
  provider_request_id, result_receipt::text, error_type, error_message,
  COALESCE(output_attachment_id::text, ''), COALESCE(output_asset_id::text, ''),
  COALESCE(started_at::text, ''), COALESCE(completed_at::text, ''), created_at::text, updated_at::text
FROM creative_image_operation
WHERE variant_id = $1
ORDER BY revision, size_key, created_at
`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := []creativeImageOperationResponse{}
	for rows.Next() {
		operation, err := scanCreativeImageOperation(rows)
		if err != nil {
			return nil, err
		}
		operation.Attempts, err = h.listCreativeImageOperationAttempts(ctx, parseUUID(operation.ID))
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (h *Handler) listCreativeImageOperationAttempts(ctx context.Context, operationID pgtype.UUID) ([]creativeImageOperationAttemptResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id::text, attempt, status, COALESCE(runtime_id::text, ''), COALESCE(task_id::text, ''),
  provider_request_id, provider_status, http_status, exit_code, error_type, error_message, result_receipt::text,
  COALESCE(output_attachment_id::text, ''), duration_ms, started_at::text, COALESCE(completed_at::text, ''),
  created_at::text, updated_at::text
FROM creative_image_operation_attempt
WHERE operation_id = $1
ORDER BY attempt
`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	attempts := []creativeImageOperationAttemptResponse{}
	for rows.Next() {
		var attempt creativeImageOperationAttemptResponse
		var resultReceipt string
		if err := rows.Scan(&attempt.ID, &attempt.Attempt, &attempt.Status, &attempt.RuntimeID, &attempt.TaskID,
			&attempt.ProviderRequestID, &attempt.ProviderStatus, &attempt.HTTPStatus, &attempt.ExitCode, &attempt.ErrorType,
			&attempt.ErrorMessage, &resultReceipt, &attempt.OutputAttachmentID, &attempt.DurationMS, &attempt.StartedAt,
			&attempt.CompletedAt, &attempt.CreatedAt, &attempt.UpdatedAt); err != nil {
			return nil, err
		}
		attempt.ResultReceipt = json.RawMessage(resultReceipt)
		attempts = append(attempts, attempt)
	}
	return attempts, rows.Err()
}
