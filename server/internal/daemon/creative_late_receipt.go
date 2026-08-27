package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	creativeLateReceiptPollInterval = 5 * time.Second
	creativeLateReceiptWatchWindow  = 4 * time.Minute
	maxCreativeLateReceiptJSON      = 1 << 20
)

type atomicCreativeImageReceipt struct {
	OperationID      string `json:"operation_id"`
	OperationAttempt int    `json:"operation_attempt"`
	TaskID           string `json:"task_id"`
	OutputSHA256     string `json:"output_sha256"`
	GeneratedAsset   struct {
		Completed bool   `json:"completed"`
		Path      string `json:"path"`
	} `json:"generated_asset"`
}

func creativeImageTaskExpectedReceiptCount(raw json.RawMessage) int {
	var taskContext struct {
		Type          string   `json:"type"`
		Workflow      string   `json:"workflow"`
		ExpectedSizes []string `json:"expected_sizes"`
		TargetSize    string   `json:"target_size"`
	}
	if json.Unmarshal(raw, &taskContext) != nil || taskContext.Type != "creative_domain_task" ||
		(taskContext.Workflow != "creative_production" && taskContext.Workflow != "creative_direct_edit") {
		return 0
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return 0
	}
	if recovery, ok := fields["late_receipt_recovery"]; ok && len(recovery) > 0 && string(recovery) != "null" {
		// A recovery task consumes a receipt already accepted by the server; it
		// must never wait for, or imply, another provider invocation.
		return 0
	}
	providerScopePresent := false
	for _, field := range []string{"missing_sizes", "edit_sizes"} {
		value, ok := fields[field]
		if !ok {
			continue
		}
		providerScopePresent = true
		var sizes []string
		if json.Unmarshal(value, &sizes) == nil {
			if count := uniqueCreativeReceiptSizeCount(sizes); count > 0 {
				return count
			}
		}
	}
	if providerScopePresent {
		return 0
	}
	if count := uniqueCreativeReceiptSizeCount(taskContext.ExpectedSizes); count > 0 {
		return count
	}
	if strings.TrimSpace(taskContext.TargetSize) != "" {
		return 1
	}
	return 0
}

func uniqueCreativeReceiptSizeCount(sizes []string) int {
	seen := make(map[string]struct{}, len(sizes))
	for _, size := range sizes {
		size = strings.TrimSpace(size)
		if size != "" {
			seen[size] = struct{}{}
		}
	}
	return len(seen)
}

func pathWithinCreativeReceiptRoot(root, candidate string) (string, bool) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxUploadSizeForDaemonReceipt {
		return "", false
	}
	return resolved, true
}

const maxUploadSizeForDaemonReceipt = 100 << 20

func readAtomicCreativeImageReceipt(path, taskID string) ([]byte, atomicCreativeImageReceipt, error) {
	var receipt atomicCreativeImageReceipt
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxCreativeLateReceiptJSON {
		return nil, receipt, errors.New("invalid atomic image receipt file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, receipt, err
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.TaskID != taskID || strings.TrimSpace(receipt.OperationID) == "" ||
		receipt.OperationAttempt < 1 || !receipt.GeneratedAsset.Completed || strings.TrimSpace(receipt.GeneratedAsset.Path) == "" ||
		len(strings.TrimSpace(receipt.OutputSHA256)) != 64 {
		return nil, receipt, errors.New("atomic image receipt is incomplete or belongs to another task")
	}
	return raw, receipt, nil
}

func (d *Daemon) reportAtomicCreativeImageReceipts(ctx context.Context, task Task, workDir string, taskLog *slog.Logger) int {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return 0
	}
	settled := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "image-edit-result-") {
			continue
		}
		path := filepath.Join(workDir, name)
		if strings.HasSuffix(name, ".json.reported") {
			if _, receipt, err := readAtomicCreativeImageReceipt(path, task.ID); err == nil && receipt.TaskID == task.ID {
				settled++
			}
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		receiptJSON, receipt, err := readAtomicCreativeImageReceipt(path, task.ID)
		if err != nil {
			taskLog.Warn("ignore invalid atomic image receipt", "path", path, "error", err)
			continue
		}
		imagePath, ok := pathWithinCreativeReceiptRoot(workDir, receipt.GeneratedAsset.Path)
		if !ok {
			taskLog.Warn("ignore atomic image receipt with output outside task workdir", "path", path)
			continue
		}
		image, err := os.ReadFile(imagePath)
		if err != nil {
			taskLog.Warn("read atomic image receipt output failed", "path", imagePath, "error", err)
			continue
		}
		err = d.client.ReportCreativeImageLateSuccess(
			ctx, task.RuntimeID, task.ID, receipt.OperationID, receipt.OperationAttempt,
			receiptJSON, image, filepath.Base(imagePath),
		)
		if err != nil {
			taskLog.Warn("report atomic image receipt failed", "operation_id", receipt.OperationID, "attempt", receipt.OperationAttempt, "error", err)
			continue
		}
		reportedPath := path + ".reported"
		if err := os.Rename(path, reportedPath); err != nil {
			taskLog.Warn("mark atomic image receipt reported failed", "path", path, "error", err)
		}
		taskLog.Info("reported atomic image receipt", "operation_id", receipt.OperationID, "attempt", receipt.OperationAttempt)
		settled++
	}
	return settled
}

func (d *Daemon) reconcileAtomicCreativeImageReceipts(ctx context.Context, task Task, workDir, predictedEnvRoot string, taskLog *slog.Logger) {
	expected := creativeImageTaskExpectedReceiptCount(task.Context)
	if expected == 0 {
		return
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" && predictedEnvRoot != "" {
		workDir = filepath.Join(predictedEnvRoot, "workdir")
	}
	if workDir == "" {
		taskLog.Warn("creative image receipt recovery has no task workdir")
		return
	}
	if settled := d.reportAtomicCreativeImageReceipts(ctx, task, workDir, taskLog); settled >= expected {
		return
	}

	// A provider subprocess can finish after the agent turn or server task has
	// already become terminal. Keep the original daemon credential and task
	// coordinates alive long enough to collect that atomic file; the server's
	// CAS remains authoritative and makes every replay harmless.
	if predictedEnvRoot != "" {
		d.markActiveEnvRoot(predictedEnvRoot)
	}
	go func() {
		if predictedEnvRoot != "" {
			defer d.unmarkActiveEnvRoot(predictedEnvRoot)
		}
		watchCtx, cancel := context.WithTimeout(ctx, creativeLateReceiptWatchWindow)
		defer cancel()
		ticker := time.NewTicker(creativeLateReceiptPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				taskLog.Info("atomic image receipt watch ended", "reason", watchCtx.Err())
				return
			case <-ticker.C:
				if settled := d.reportAtomicCreativeImageReceipts(watchCtx, task, workDir, taskLog); settled >= expected {
					taskLog.Info("all atomic image receipts reconciled", "count", settled)
					return
				}
			}
		}
	}()
}

func atomicCreativeImageReceiptName(size string) string {
	return fmt.Sprintf("image-edit-result-%s.json", size)
}
