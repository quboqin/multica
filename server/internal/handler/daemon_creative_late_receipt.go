package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const maxCreativeLateReceiptFormSize = maxUploadSize + (1 << 20)

type daemonCreativeLateReceipt struct {
	OperationID      string  `json:"operation_id"`
	OperationAttempt int     `json:"operation_attempt"`
	TaskID           string  `json:"task_id"`
	Model            string  `json:"model"`
	Size             string  `json:"size"`
	RequestID        string  `json:"request_id"`
	PromptSHA256     string  `json:"prompt_sha256"`
	OutputSHA256     string  `json:"output_sha256"`
	Bytes            int64   `json:"bytes"`
	ActualWidth      int     `json:"actual_width"`
	ActualHeight     int     `json:"actual_height"`
	ProviderElapsed  float64 `json:"provider_elapsed_seconds"`
	GeneratedAsset   struct {
		Completed bool   `json:"completed"`
		Path      string `json:"path"`
		Size      string `json:"size"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"generated_asset"`
}

type daemonCreativeLateReceiptBinding struct {
	OperationStatus             string
	OperationModel              string
	OperationPromptSHA256       string
	OperationProviderRequestID  string
	OperationResultReceipt      string
	OperationOutputAttachmentID pgtype.UUID
	OperationOutputAssetID      pgtype.UUID
	AttemptStatus               string
	AttemptProviderRequestID    string
	AttemptResultReceipt        string
	AttemptOutputAttachmentID   pgtype.UUID
	VariantID                   pgtype.UUID
	SizeKey                     string
	Revision                    int
	OperationKind               string
	OrderID                     pgtype.UUID
	OrderItemID                 pgtype.UUID
	WorkspaceID                 pgtype.UUID
	IssueID                     pgtype.UUID
	AgentID                     pgtype.UUID
	TaskStatus                  string
	TaskContext                 json.RawMessage
	VariantStatus               string
	VariantCurrentRevision      int
	VariantActiveRevision       pgtype.Int4
}

type creativeLateReceiptRecoveryEnqueue struct {
	Queued  bool
	ChildID pgtype.UUID
}

func validateDaemonCreativeLateReceipt(receipt daemonCreativeLateReceipt, operationID, taskID string, attempt int) error {
	receipt.OperationID = strings.TrimSpace(receipt.OperationID)
	receipt.TaskID = strings.TrimSpace(receipt.TaskID)
	receipt.Model = strings.TrimSpace(receipt.Model)
	receipt.Size = strings.TrimSpace(receipt.Size)
	receipt.RequestID = strings.TrimSpace(receipt.RequestID)
	receipt.PromptSHA256 = strings.ToLower(strings.TrimSpace(receipt.PromptSHA256))
	receipt.OutputSHA256 = strings.ToLower(strings.TrimSpace(receipt.OutputSHA256))
	if receipt.OperationID != operationID || receipt.OperationAttempt != attempt || receipt.TaskID != taskID {
		return errors.New("late image receipt coordinates do not match the daemon route")
	}
	if receipt.Model == "" || !validCreativeAssetSize(receipt.Size) || receipt.RequestID == "" || len(receipt.RequestID) > 512 ||
		!creativeImageOperationSHA256Pattern.MatchString(receipt.PromptSHA256) ||
		!creativeImageOperationSHA256Pattern.MatchString(receipt.OutputSHA256) {
		return errors.New("late image receipt lineage is invalid")
	}
	if !receipt.GeneratedAsset.Completed || strings.TrimSpace(receipt.GeneratedAsset.Path) == "" ||
		receipt.GeneratedAsset.Size != receipt.Size || receipt.ActualWidth < 1 || receipt.ActualHeight < 1 ||
		receipt.GeneratedAsset.Width != receipt.ActualWidth || receipt.GeneratedAsset.Height != receipt.ActualHeight ||
		receipt.Bytes < 1 || receipt.ProviderElapsed < 0 {
		return errors.New("late image receipt does not describe a complete generated asset")
	}
	return nil
}

func decodeDaemonCreativeLateReceiptFile(file multipart.File, receipt daemonCreativeLateReceipt) ([]byte, string, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read late image output: %w", err)
	}
	if len(data) == 0 || len(data) > maxUploadSize || int64(len(data)) != receipt.Bytes {
		return nil, "", errors.New("late image output size does not match its receipt")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != receipt.OutputSHA256 {
		return nil, "", errors.New("late image output digest does not match its receipt")
	}
	contentType := http.DetectContentType(data)
	if contentType != "image/png" && contentType != "image/jpeg" {
		return nil, "", errors.New("late image output must be a PNG or JPEG")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != receipt.ActualWidth || config.Height != receipt.ActualHeight {
		return nil, "", errors.New("late image output dimensions do not match its receipt")
	}
	return data, contentType, nil
}

func scanDaemonCreativeLateReceiptBinding(row rowScanner) (daemonCreativeLateReceiptBinding, error) {
	var binding daemonCreativeLateReceiptBinding
	var taskContext string
	err := row.Scan(
		&binding.OperationStatus, &binding.OperationModel, &binding.OperationPromptSHA256,
		&binding.OperationProviderRequestID, &binding.OperationResultReceipt,
		&binding.OperationOutputAttachmentID, &binding.OperationOutputAssetID,
		&binding.AttemptStatus, &binding.AttemptProviderRequestID, &binding.AttemptResultReceipt,
		&binding.AttemptOutputAttachmentID, &binding.VariantID, &binding.SizeKey,
		&binding.Revision, &binding.OperationKind, &binding.OrderID, &binding.OrderItemID,
		&binding.WorkspaceID, &binding.IssueID, &binding.AgentID, &binding.TaskStatus,
		&taskContext, &binding.VariantStatus, &binding.VariantCurrentRevision,
		&binding.VariantActiveRevision,
	)
	binding.TaskContext = json.RawMessage(taskContext)
	return binding, err
}

const daemonCreativeLateReceiptBindingQuery = `
WITH RECURSIVE task_lineage AS (
  SELECT id, parent_task_id, retry_of_task_id
  FROM agent_task_queue
  WHERE id = $3
  UNION
  SELECT parent.id, parent.parent_task_id, parent.retry_of_task_id
  FROM agent_task_queue parent
  JOIN task_lineage child
    ON parent.id = child.parent_task_id OR parent.id = child.retry_of_task_id
)
SELECT operation.status, operation.model, operation.prompt_sha256,
       operation.provider_request_id, operation.result_receipt::text,
       operation.output_attachment_id, operation.output_asset_id,
       attempt.status, attempt.provider_request_id, attempt.result_receipt::text,
       attempt.output_attachment_id, operation.variant_id, operation.size_key,
       operation.revision, operation.operation_kind, order_row.id, item.id,
       order_row.workspace_id, task.issue_id, task.agent_id, task.status,
       task.context::text, variant.status, variant.revision, variant.active_revision
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt
  ON attempt.operation_id = operation.id AND attempt.attempt = $2
JOIN creative_order_variant variant ON variant.id = operation.variant_id
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
JOIN agent_task_queue task ON task.id = $3
JOIN agent_runtime runtime ON runtime.id = $4
WHERE operation.id = $1
  AND attempt.runtime_id = runtime.id
  AND attempt.task_id = task.id
  AND task.runtime_id = runtime.id
  AND operation.task_id IN (SELECT id FROM task_lineage)
  AND runtime.workspace_id = order_row.workspace_id
  AND task.agent_id IN (SELECT id FROM agent WHERE workspace_id = order_row.workspace_id)
`

func validateDaemonCreativeLateReceiptBinding(binding daemonCreativeLateReceiptBinding, receipt daemonCreativeLateReceipt, workspaceID pgtype.UUID) error {
	if binding.WorkspaceID != workspaceID {
		return errors.New("late image receipt does not belong to this workspace")
	}
	if binding.TaskStatus != "running" && binding.TaskStatus != "completed" && binding.TaskStatus != "failed" {
		return errors.New("late image receipt task is not running or terminal")
	}
	if binding.VariantStatus == "cancelled" {
		return errors.New("cancelled creative work cannot accept a late image receipt")
	}
	if binding.VariantCurrentRevision != binding.Revision ||
		(binding.VariantActiveRevision.Valid && int(binding.VariantActiveRevision.Int32) == binding.Revision) {
		return errors.New("late image receipt revision is no longer mutable")
	}
	if binding.OperationModel != receipt.Model || binding.OperationPromptSHA256 != receipt.PromptSHA256 || binding.SizeKey != receipt.Size {
		return errors.New("late image receipt model, prompt, or size does not match the operation")
	}
	if binding.AttemptProviderRequestID != "" && binding.AttemptProviderRequestID != receipt.RequestID {
		return errors.New("late image receipt provider request does not match the attempt")
	}
	var taskContext struct {
		Type      string `json:"type"`
		Workflow  string `json:"workflow"`
		VariantID string `json:"variant_id"`
		Revision  int    `json:"revision"`
	}
	if json.Unmarshal(binding.TaskContext, &taskContext) != nil || taskContext.Type != "creative_domain_task" ||
		(taskContext.Workflow != "creative_production" && taskContext.Workflow != "creative_direct_edit") ||
		taskContext.VariantID != uuidToString(binding.VariantID) || taskContext.Revision != binding.Revision {
		return errors.New("late image receipt task context does not match the operation")
	}
	sizeBound, scopeErr := creativeTaskAllowsImageSize(binding.TaskContext, binding.SizeKey)
	if scopeErr != nil || !sizeBound {
		return errors.New("late image receipt size is outside the task scope")
	}
	return nil
}

func validDaemonCreativeLateReceiptSourceStatus(status string) bool {
	return status == "running" || status == "unknown" || status == "failed"
}

func daemonCreativeLateReceiptIsExactReplay(binding daemonCreativeLateReceiptBinding, receiptJSON []byte, receipt daemonCreativeLateReceipt) bool {
	if binding.OperationStatus != "completed" || binding.AttemptStatus != "completed" ||
		!binding.OperationOutputAttachmentID.Valid || binding.OperationOutputAttachmentID != binding.AttemptOutputAttachmentID ||
		binding.OperationProviderRequestID != receipt.RequestID || binding.AttemptProviderRequestID != receipt.RequestID {
		return false
	}
	var existingOperation, existingAttempt, incoming any
	if json.Unmarshal([]byte(binding.OperationResultReceipt), &existingOperation) != nil ||
		json.Unmarshal([]byte(binding.AttemptResultReceipt), &existingAttempt) != nil ||
		json.Unmarshal(receiptJSON, &incoming) != nil {
		return false
	}
	return reflect.DeepEqual(existingOperation, incoming) && reflect.DeepEqual(existingAttempt, incoming)
}

// ReportDaemonCreativeImageLateSuccess accepts a provider result after the
// task-scoped token has expired. Only the exact daemon that owns the original
// runtime may call it; the route, receipt, database lineage, and uploaded bytes
// must all agree before the operation is moved to completed.
func (h *Handler) ReportDaemonCreativeImageLateSuccess(w http.ResponseWriter, r *http.Request) {
	if middleware.DaemonAuthPathFromContext(r.Context()) != middleware.DaemonAuthPathDaemonToken {
		writeError(w, http.StatusForbidden, "late image receipts require a daemon token")
		return
	}
	if h.Storage == nil || h.TxStarter == nil {
		writeError(w, http.StatusServiceUnavailable, "late image receipt storage is unavailable")
		return
	}
	runtimeID := chi.URLParam(r, "runtimeId")
	taskID := chi.URLParam(r, "taskId")
	operationID := chi.URLParam(r, "operationId")
	attempt, err := strconv.Atoi(chi.URLParam(r, "attempt"))
	if err != nil || attempt < 1 {
		writeError(w, http.StatusBadRequest, "invalid image operation attempt")
		return
	}
	runtime, ok := h.requireDaemonRuntimeAccess(w, r, runtimeID)
	if !ok {
		return
	}
	if !runtime.DaemonID.Valid || runtime.DaemonID.String != middleware.DaemonIDFromContext(r.Context()) {
		writeError(w, http.StatusNotFound, "runtime not found")
		return
	}
	task, workspaceText, ok := h.requireDaemonTaskAccessWithWorkspace(w, r, taskID)
	if !ok {
		return
	}
	if !task.RuntimeID.Valid || uuidToString(task.RuntimeID) != runtimeID {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	operationUUID, ok := parseUUIDOrBadRequest(w, operationID, "operation_id")
	if !ok {
		return
	}
	workspaceID := parseUUID(workspaceText)

	r.Body = http.MaxBytesReader(w, r.Body, maxCreativeLateReceiptFormSize)
	if err := r.ParseMultipartForm(maxCreativeLateReceiptFormSize); err != nil {
		writeError(w, http.StatusBadRequest, "late image receipt is too large or malformed")
		return
	}
	defer r.MultipartForm.RemoveAll()
	receiptJSON := []byte(r.FormValue("receipt"))
	var receipt daemonCreativeLateReceipt
	if len(receiptJSON) == 0 || json.Unmarshal(receiptJSON, &receipt) != nil {
		writeError(w, http.StatusBadRequest, "invalid late image receipt")
		return
	}
	if err := validateDaemonCreativeLateReceipt(receipt, operationID, taskID, attempt); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "late image receipt file is required")
		return
	}
	defer file.Close()
	data, contentType, err := decodeDaemonCreativeLateReceiptFile(file, receipt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	preflight, err := scanDaemonCreativeLateReceiptBinding(h.DB.QueryRow(r.Context(), daemonCreativeLateReceiptBindingQuery,
		operationUUID, attempt, parseUUID(taskID), parseUUID(runtimeID)))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "late image receipt target not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate late image receipt")
		return
	}
	if err := validateDaemonCreativeLateReceiptBinding(preflight, receipt, workspaceID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if preflight.OperationStatus == "completed" || preflight.AttemptStatus == "completed" {
		if daemonCreativeLateReceiptIsExactReplay(preflight, receiptJSON, receipt) {
			queued, queueErr := h.enqueueCreativeLateReceiptRecovery(
				r.Context(), task, preflight, operationUUID, attempt,
				preflight.OperationOutputAttachmentID, receipt,
			)
			if queueErr != nil {
				slog.Error("queue creative late receipt recovery after replay", "operation_id", operationID, "task_id", taskID, "error", queueErr)
				writeError(w, http.StatusServiceUnavailable, "late image result was saved but recovery could not be queued; retry this receipt")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "completed", "disposition": "reuse", "operation_id": operationID,
				"attempt": attempt, "output_attachment_id": uuidToString(preflight.OperationOutputAttachmentID), "recovery_queued": queued,
			})
			return
		}
		writeError(w, http.StatusConflict, "completed creative image operation is immutable")
		return
	}
	if !validDaemonCreativeLateReceiptSourceStatus(preflight.OperationStatus) ||
		!validDaemonCreativeLateReceiptSourceStatus(preflight.AttemptStatus) {
		writeError(w, http.StatusConflict, "late image receipt can only settle a running, unknown, or failed operation attempt")
		return
	}

	attachmentUUID := pgtype.UUID{Bytes: uuid.Must(uuid.NewV7()), Valid: true}
	extension := ".png"
	if contentType == "image/jpeg" {
		extension = ".jpg"
	}
	filename := "late-image-operation-" + uuidToString(operationUUID) + "-attempt-" + strconv.Itoa(attempt) + extension
	key := "workspaces/" + workspaceText + "/" + uuidToString(attachmentUUID) + extension
	link, err := h.Storage.Upload(r.Context(), key, data, contentType, filename)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to archive late image output")
		return
	}
	keepUpload := false
	defer func() {
		if !keepUpload {
			h.deleteS3Object(context.Background(), link)
		}
	}()

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start late image receipt")
		return
	}
	defer tx.Rollback(r.Context())
	var lockedTaskStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status FROM agent_task_queue WHERE id = $1 FOR UPDATE
`, task.ID).Scan(&lockedTaskStatus); err != nil {
		writeError(w, http.StatusConflict, "late image receipt task is no longer available")
		return
	}
	task.Status = lockedTaskStatus
	var lockedVariantID, lockedOperationID pgtype.UUID
	var lockedAttempt int
	if err := tx.QueryRow(r.Context(), `
SELECT id FROM creative_order_variant WHERE id = $1 FOR UPDATE
`, preflight.VariantID).Scan(&lockedVariantID); err != nil {
		writeError(w, http.StatusConflict, "late image receipt variant is no longer available")
		return
	}
	if err := tx.QueryRow(r.Context(), `
SELECT id FROM creative_image_operation WHERE id = $1 FOR UPDATE
`, operationUUID).Scan(&lockedOperationID); err != nil {
		writeError(w, http.StatusConflict, "late image receipt operation is no longer available")
		return
	}
	if err := tx.QueryRow(r.Context(), `
SELECT attempt FROM creative_image_operation_attempt
WHERE operation_id = $1 AND attempt = $2
FOR UPDATE
`, operationUUID, attempt).Scan(&lockedAttempt); err != nil {
		writeError(w, http.StatusConflict, "late image receipt attempt is no longer available")
		return
	}
	locked, err := scanDaemonCreativeLateReceiptBinding(tx.QueryRow(r.Context(), daemonCreativeLateReceiptBindingQuery, operationUUID, attempt, task.ID, runtime.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "late image receipt target not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock late image receipt")
		return
	}
	if err := validateDaemonCreativeLateReceiptBinding(locked, receipt, workspaceID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if locked.OperationStatus == "completed" || locked.AttemptStatus == "completed" {
		if daemonCreativeLateReceiptIsExactReplay(locked, receiptJSON, receipt) {
			recovery, queueErr := h.enqueueCreativeLateReceiptRecoveryTx(
				r.Context(), tx, task, locked, operationUUID, attempt,
				locked.OperationOutputAttachmentID, receipt,
			)
			if queueErr != nil {
				slog.Error("queue creative late receipt recovery after concurrent replay", "operation_id", operationID, "task_id", taskID, "error", queueErr)
				writeError(w, http.StatusServiceUnavailable, "late image result was saved but recovery could not be queued; retry this receipt")
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to reuse late image receipt")
				return
			}
			h.notifyCreativeLateReceiptRecovery(r.Context(), recovery.ChildID)
			writeJSON(w, http.StatusOK, map[string]any{
				"status": "completed", "disposition": "reuse", "operation_id": operationID,
				"attempt": attempt, "output_attachment_id": uuidToString(locked.OperationOutputAttachmentID), "recovery_queued": recovery.Queued,
			})
			return
		}
		writeError(w, http.StatusConflict, "completed creative image operation is immutable")
		return
	}
	if !validDaemonCreativeLateReceiptSourceStatus(locked.OperationStatus) ||
		!validDaemonCreativeLateReceiptSourceStatus(locked.AttemptStatus) {
		writeError(w, http.StatusConflict, "late image receipt can only settle a running, unknown, or failed operation attempt")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO attachment (
  id, workspace_id, issue_id, uploader_type, uploader_id, filename, url,
  content_type, size_bytes, task_id
)
VALUES ($1,$2,$3,'agent',$4,$5,$6,$7,$8,$9)
`, attachmentUUID, workspaceID, locked.IssueID, locked.AgentID, filename, link, contentType, int64(len(data)), task.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register late image output")
		return
	}
	durationMS := int64(receipt.ProviderElapsed * 1000)
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation_attempt
SET status = 'completed', provider_request_id = $3, provider_status = 'completed',
    http_status = 200, exit_code = 0, error_type = '', error_message = '',
    result_receipt = $4::jsonb, output_attachment_id = $5, duration_ms = $6,
    completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE operation_id = $1 AND attempt = $2 AND status <> 'completed'
`, operationUUID, attempt, receipt.RequestID, receiptJSON, attachmentUUID, durationMS); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete late image attempt")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation
SET status = 'completed', provider_request_id = $2, result_receipt = $3::jsonb,
    error_type = '', error_message = '', output_attachment_id = $4,
    started_at = COALESCE(started_at, now()), completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE id = $1 AND status <> 'completed'
`, operationUUID, receipt.RequestID, receiptJSON, attachmentUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to complete late image operation")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation_attempt
SET status = 'cancelled',
    error_type = CASE WHEN error_type = '' THEN 'superseded_by_completed_attempt' ELSE error_type END,
    completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE operation_id = $1 AND attempt <> $2 AND status IN ('running','unknown')
`, operationUUID, attempt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to settle superseded image attempts")
		return
	}
	recovery, queueErr := h.enqueueCreativeLateReceiptRecoveryTx(r.Context(), tx, task, locked, operationUUID, attempt, attachmentUUID, receipt)
	if queueErr != nil {
		slog.Error("queue creative late receipt recovery", "operation_id", operationID, "task_id", taskID, "error", queueErr)
		writeError(w, http.StatusServiceUnavailable, "late image result could not be saved atomically; retry this receipt")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit late image receipt")
		return
	}
	keepUpload = true
	h.notifyCreativeLateReceiptRecovery(r.Context(), recovery.ChildID)
	if h.Bus != nil {
		h.publish(protocol.EventCreativeMaterialsUpdated, workspaceText, "system", "", map[string]any{
			"scope": "order", "order_id": uuidToString(locked.OrderID), "variant_id": uuidToString(locked.VariantID),
			"revision": locked.Revision, "size_key": locked.SizeKey, "image_operation_id": operationID,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "completed", "disposition": "late_success", "operation_id": operationID,
		"attempt": attempt, "output_attachment_id": uuidToString(attachmentUUID), "recovery_queued": recovery.Queued,
	})
}

func (h *Handler) enqueueCreativeLateReceiptRecovery(
	ctx context.Context,
	task db.AgentTaskQueue,
	binding daemonCreativeLateReceiptBinding,
	operationID pgtype.UUID,
	attempt int,
	attachmentID pgtype.UUID,
	receipt daemonCreativeLateReceipt,
) (bool, error) {
	if binding.OperationOutputAssetID.Valid {
		return false, nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin late receipt recovery: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := tx.QueryRow(ctx, `
SELECT status FROM agent_task_queue WHERE id = $1 FOR UPDATE
`, task.ID).Scan(&task.Status); err != nil {
		return false, fmt.Errorf("lock late receipt recovery source task: %w", err)
	}
	binding.TaskStatus = task.Status
	recovery, err := h.enqueueCreativeLateReceiptRecoveryTx(
		ctx, tx, task, binding, operationID, attempt, attachmentID, receipt,
	)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit late receipt recovery: %w", err)
	}
	h.notifyCreativeLateReceiptRecovery(ctx, recovery.ChildID)
	return recovery.Queued, nil
}

func (h *Handler) enqueueCreativeLateReceiptRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	task db.AgentTaskQueue,
	binding daemonCreativeLateReceiptBinding,
	operationID pgtype.UUID,
	attempt int,
	attachmentID pgtype.UUID,
	receipt daemonCreativeLateReceipt,
) (creativeLateReceiptRecoveryEnqueue, error) {
	if binding.OperationOutputAssetID.Valid {
		return creativeLateReceiptRecoveryEnqueue{}, nil
	}
	if task.Status != "running" && task.Status != "completed" && task.Status != "failed" {
		return creativeLateReceiptRecoveryEnqueue{}, nil
	}
	var contextValue map[string]any
	if json.Unmarshal(task.Context, &contextValue) != nil {
		return creativeLateReceiptRecoveryEnqueue{}, errors.New("late receipt task context is invalid")
	}
	recoveryEntry := map[string]any{
		"operation_id": uuidToString(operationID), "operation_attempt": attempt,
		"size_key": binding.SizeKey, "prompt_sha256": receipt.PromptSHA256,
		"provider_request_id": receipt.RequestID, "output_attachment_id": uuidToString(attachmentID),
		"instruction": "reuse this completed provider result; normalize and register the canonical asset without invoking the image provider",
	}
	contextValue["late_receipt_recovery"] = recoveryEntry
	contextValue["late_receipt_recoveries"] = []any{recoveryEntry}
	// One continuation consumes one completed size-level operation. Keep the
	// full delivery expected_sizes contract, but narrow the provider work scope
	// so simultaneous late receipts cannot make one continuation wait for or
	// recreate another size.
	switch workflow, _ := contextValue["workflow"].(string); workflow {
	case "creative_production":
		contextValue["missing_sizes"] = []string{binding.SizeKey}
	case "creative_direct_edit":
		contextValue["edit_sizes"] = []string{binding.SizeKey}
		contextValue["target_size"] = binding.SizeKey
	}
	recoveryContext, err := json.Marshal(contextValue)
	if err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("encode late receipt recovery context: %w", err)
	}
	// Serialize recovery decisions for one operation. Exact daemon replays can
	// arrive concurrently, but only one active continuation may be created.
	var lockedVariantID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT id FROM creative_order_variant WHERE id = $1 AND revision = $2 FOR UPDATE
`, binding.VariantID, binding.Revision).Scan(&lockedVariantID); err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("lock late receipt recovery variant: %w", err)
	}
	var lockedOperationID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT id FROM creative_image_operation WHERE id = $1 FOR UPDATE
`, operationID).Scan(&lockedOperationID); err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("lock late receipt recovery operation: %w", err)
	}
	var childID pgtype.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, chat_session_id, autopilot_run_id,
  status, priority, trigger_comment_id, trigger_summary, context,
  attempt, max_attempts, parent_task_id, force_fresh_session, is_leader_task,
  requesting_user_id, originator_user_id, accountable_user_id, originator_source,
  delegated_from_task_id, rule_version_id, retry_of_task_id,
  trigger_evidence_kind, trigger_evidence_ref_id
)
SELECT task.agent_id, task.runtime_id, task.issue_id, task.chat_session_id, task.autopilot_run_id,
       'queued', task.priority, task.trigger_comment_id, task.trigger_summary, $2::jsonb,
       task.attempt + 1, GREATEST(task.max_attempts, task.attempt + 1), task.id, TRUE, task.is_leader_task,
       task.requesting_user_id, task.originator_user_id, task.accountable_user_id, task.originator_source,
       task.delegated_from_task_id, task.rule_version_id, task.id,
       task.trigger_evidence_kind, task.trigger_evidence_ref_id
FROM agent_task_queue task
JOIN creative_order_variant variant ON variant.id = $3 AND variant.revision = $4
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
JOIN creative_image_operation operation ON operation.id = $5
WHERE task.id = $1 AND task.status IN ('running','completed','failed')
  AND order_row.status <> 'cancelled' AND variant.status <> 'cancelled'
  AND (variant.active_revision IS NULL OR variant.active_revision <> variant.revision)
  AND operation.status = 'completed' AND operation.output_asset_id IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM agent_task_queue active
    WHERE active.id <> task.id
      AND active.status IN ('queued','dispatched','running','waiting_local_directory')
      AND active.context->>'type' = 'creative_domain_task'
      AND active.context->>'workflow' = task.context->>'workflow'
      AND active.context->>'variant_id' = $3::uuid::text
      AND active.context->>'revision' = $4::int::text
	  AND active.context->'late_receipt_recovery'->>'operation_id' = $5::uuid::text
  )
ON CONFLICT DO NOTHING
RETURNING id
	`, task.ID, recoveryContext, binding.VariantID, binding.Revision, operationID).Scan(&childID)
	if errors.Is(err, pgx.ErrNoRows) {
		// One issue/agent can have only one queued task. If another late
		// receipt reaches us before the first continuation is claimed, fold
		// the new size-level operation into that queued continuation instead
		// of acknowledging it without durable work.
		var queuedID pgtype.UUID
		var queuedContextRaw string
		mergeErr := tx.QueryRow(ctx, `
SELECT active.id, active.context::text
FROM agent_task_queue active
WHERE active.agent_id = $1
  AND active.issue_id IS NOT DISTINCT FROM $2
  AND active.status = 'queued'
  AND active.context->>'type' = 'creative_domain_task'
  AND active.context->>'workflow' = $3
  AND active.context->>'variant_id' = $4::uuid::text
  AND active.context->>'revision' = $5::int::text
  AND active.context ? 'late_receipt_recovery'
ORDER BY active.created_at
LIMIT 1
FOR UPDATE
`, task.AgentID, task.IssueID, contextValue["workflow"], binding.VariantID, binding.Revision).Scan(&queuedID, &queuedContextRaw)
		if mergeErr == nil {
			var queuedContext map[string]any
			if json.Unmarshal([]byte(queuedContextRaw), &queuedContext) != nil {
				return creativeLateReceiptRecoveryEnqueue{}, errors.New("queued late receipt recovery context is invalid")
			}
			entries, _ := queuedContext["late_receipt_recoveries"].([]any)
			if len(entries) == 0 {
				if first, ok := queuedContext["late_receipt_recovery"].(map[string]any); ok {
					entries = append(entries, first)
				}
			}
			operationText := uuidToString(operationID)
			alreadyPresent := false
			for _, rawEntry := range entries {
				entry, _ := rawEntry.(map[string]any)
				if strings.TrimSpace(fmt.Sprint(entry["operation_id"])) == operationText {
					alreadyPresent = true
					break
				}
			}
			if !alreadyPresent {
				queuedContext["late_receipt_recoveries"] = append(entries, recoveryEntry)
				mergeCreativeLateReceiptRecoverySize(queuedContext, binding.SizeKey)
				mergedContext, marshalErr := json.Marshal(queuedContext)
				if marshalErr != nil {
					return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("encode merged late receipt recovery: %w", marshalErr)
				}
				if _, updateErr := tx.Exec(ctx, `
UPDATE agent_task_queue SET context = $2::jsonb
WHERE id = $1 AND status = 'queued'
`, queuedID, mergedContext); updateErr != nil {
					return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("merge queued late receipt recovery: %w", updateErr)
				}
			}
			return creativeLateReceiptRecoveryEnqueue{Queued: true}, nil
		}
		if !errors.Is(mergeErr, pgx.ErrNoRows) {
			return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("load queued late receipt recovery: %w", mergeErr)
		}
		var recoveryNeeded bool
		if checkErr := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM creative_image_operation operation
  JOIN creative_order_variant variant ON variant.id = operation.variant_id
  JOIN creative_order_item item ON item.id = variant.order_item_id
  JOIN creative_order order_row ON order_row.id = item.order_id
  JOIN agent_task_queue source_task ON source_task.id = operation.task_id
  WHERE operation.id = $1
    AND operation.status = 'completed'
    AND operation.output_asset_id IS NULL
    AND source_task.status IN ('running','completed','failed')
    AND order_row.status <> 'cancelled'
    AND variant.status <> 'cancelled'
    AND (variant.active_revision IS NULL OR variant.active_revision <> variant.revision)
)
`, operationID).Scan(&recoveryNeeded); checkErr != nil {
			return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("check late receipt recovery need: %w", checkErr)
		}
		if !recoveryNeeded {
			return creativeLateReceiptRecoveryEnqueue{}, nil
		}
		return creativeLateReceiptRecoveryEnqueue{}, errors.New("late receipt recovery is blocked by another pending task; retry the receipt")
	}
	if err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("queue late receipt recovery task: %w", err)
	}
	tag, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'running',
    brief = (brief - 'error_code' - 'error_message' - 'creative_direct_edit_error') ||
      jsonb_build_object('late_receipt_recovery', jsonb_build_object(
        'operation_id', $3::uuid::text, 'attempt', $4::int, 'size_key', $5::text,
        'queued_at', now()::text
      )),
    updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, binding.VariantID, binding.Revision, operationID, attempt, binding.SizeKey)
	if err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("mark late receipt recovery running: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return creativeLateReceiptRecoveryEnqueue{}, errors.New("late receipt recovery target is no longer mutable")
	}
	var recoveryScope struct {
		ExpectedSizes []string `json:"expected_sizes"`
		TargetSize    string   `json:"target_size"`
	}
	if err := json.Unmarshal(recoveryContext, &recoveryScope); err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("decode late receipt recovery scope: %w", err)
	}
	expectedSizes := recoveryScope.ExpectedSizes
	if len(expectedSizes) == 0 && validCreativeAssetSize(recoveryScope.TargetSize) {
		expectedSizes = []string{recoveryScope.TargetSize}
	}
	if len(expectedSizes) == 0 {
		return creativeLateReceiptRecoveryEnqueue{}, errors.New("late receipt recovery has no expected size scope")
	}
	if err := syncCreativeVariantRevisionFromVariant(ctx, tx, binding.VariantID, binding.Revision, expectedSizes); err != nil {
		return creativeLateReceiptRecoveryEnqueue{}, fmt.Errorf("sync late receipt recovery revision: %w", err)
	}
	return creativeLateReceiptRecoveryEnqueue{Queued: true, ChildID: childID}, nil
}

func (h *Handler) notifyCreativeLateReceiptRecovery(ctx context.Context, childID pgtype.UUID) {
	if h.TaskService == nil || h.Queries == nil || !childID.Valid {
		return
	}
	child, err := h.Queries.GetAgentTask(ctx, childID)
	if err != nil {
		slog.Error("load queued creative late receipt recovery", "task_id", uuidToString(childID), "error", err)
		return
	}
	h.TaskService.NotifyTaskEnqueued(ctx, child)
}

func mergeCreativeLateReceiptRecoverySize(taskContext map[string]any, size string) {
	workflow, _ := taskContext["workflow"].(string)
	field := "missing_sizes"
	if workflow == "creative_direct_edit" {
		field = "edit_sizes"
		taskContext["target_size"] = size
	}
	values, _ := taskContext[field].([]any)
	for _, value := range values {
		if strings.TrimSpace(fmt.Sprint(value)) == size {
			return
		}
	}
	taskContext[field] = append(values, size)
}
