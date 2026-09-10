package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/middleware"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type daemonCreativeLateReceiptFixture struct {
	Model       string
	Quality     string
	OrderID     string
	ItemID      string
	VariantID   string
	RuntimeID   string
	DaemonID    string
	TaskID      string
	OperationID string
	Attempt     int
	PromptHash  string
	Size        string
	Width       int
	Height      int
}

func TestCreativeLateReceiptRecoverySkipsCanonicalOperation(t *testing.T) {
	queued, err := (&Handler{}).enqueueCreativeLateReceiptRecovery(
		t.Context(), db.AgentTaskQueue{}, daemonCreativeLateReceiptBinding{
			OperationOutputAssetID: pgtype.UUID{Bytes: uuid.New(), Valid: true},
		}, pgtype.UUID{}, 1, pgtype.UUID{}, daemonCreativeLateReceipt{},
	)
	if err != nil || queued {
		t.Fatalf("canonical operation recovery = queued %v, error %v", queued, err)
	}
}

func TestDaemonCreativeLateReceiptAuthPaths(t *testing.T) {
	for _, test := range []struct {
		name     string
		authPath string
		allowed  bool
	}{
		{name: "daemon token", authPath: middleware.DaemonAuthPathDaemonToken, allowed: true},
		{name: "daemon PAT", authPath: middleware.DaemonAuthPathPAT, allowed: true},
		{name: "cloud PAT", authPath: middleware.DaemonAuthPathCloudPAT},
		{name: "JWT", authPath: middleware.DaemonAuthPathJWT},
		{name: "missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := daemonCreativeLateReceiptAuthAllowed(test.authPath); got != test.allowed {
				t.Fatalf("late receipt auth path %q allowed = %v, want %v", test.authPath, got, test.allowed)
			}
		})
	}
}

func createDaemonCreativeLateReceiptFixture(t *testing.T) daemonCreativeLateReceiptFixture {
	t.Helper()
	orderID, itemID, issueID := createCreativeLifecycleTestOrder(t, "daemon creative late receipt "+uuid.NewString())
	variant := putCreativeLifecycleVariant(t, orderID, itemID, "C01", 1, "action_required")
	daemonID := "late-receipt-daemon-" + uuid.NewString()
	var runtimeID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_runtime (
  workspace_id, daemon_id, name, runtime_mode, provider, status,
  device_info, metadata, owner_id, last_seen_at
)
VALUES ($1,$2,$2,'local','codex','online','late receipt test','{}'::jsonb,$3,now())
RETURNING id::text
`, testWorkspaceID, daemonID, testUserID).Scan(&runtimeID); err != nil {
		t.Fatal(err)
	}
	var agentID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent (
  workspace_id, name, description, runtime_mode, runtime_config,
  runtime_id, visibility, max_concurrent_tasks, owner_id
)
VALUES ($1,$2,'','local','{}'::jsonb,$3,'workspace',1,$4)
RETURNING id::text
`, testWorkspaceID, "late-receipt-agent-"+uuid.NewString(), runtimeID, testUserID).Scan(&agentID); err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, context, attempt, max_attempts,
  trigger_evidence_kind, trigger_evidence_ref_id, completed_at
)
VALUES ($1,$2,$3,'completed',jsonb_build_object(
  'type','creative_domain_task','workflow','creative_production',
  'creative_order_id',$4::text,'creative_order_item_id',$5::text,
  'variant_id',$6::text,'revision',1,'item_key',$6::text || ':r1',
  'expected_sizes',jsonb_build_array('1080x1080','1200x628','800x1000')
),1,3,'creative_order_item_production',$5::uuid,now())
RETURNING id::text
`, agentID, runtimeID, issueID, orderID, itemID, variant.ID).Scan(&taskID); err != nil {
		t.Fatal(err)
	}
	promptHash := creativePromptSHA256("daemon late receipt prompt")
	var operationID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status,
  model, runtime_id, task_id, prompt_sha256, input_snapshot, started_at
)
VALUES ($1,'1080x1080',1,'generation',$2,'unknown','gpt-image-2',$3,$4,$5,
        '{"source":"late-receipt-test"}'::jsonb,now())
RETURNING id::text
`, variant.ID, "late-receipt:"+taskID, runtimeID, taskID, promptHash).Scan(&operationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (
  operation_id, attempt, status, runtime_id, task_id
)
VALUES ($1,1,'unknown',$2,$3)
`, operationID, runtimeID, taskID); err != nil {
		t.Fatal(err)
	}
	return daemonCreativeLateReceiptFixture{
		OrderID: orderID, ItemID: itemID, VariantID: variant.ID, RuntimeID: runtimeID,
		DaemonID: daemonID, TaskID: taskID, OperationID: operationID, PromptHash: promptHash,
		Attempt: 1, Size: "1080x1080", Width: 1080, Height: 1080,
	}
}

func daemonCreativeLateReceiptRequest(
	t *testing.T,
	fixture daemonCreativeLateReceiptFixture,
	workspaceID, daemonID string,
	providerRequestID string,
	image []byte,
) *http.Request {
	t.Helper()
	attempt := fixture.Attempt
	if attempt < 1 {
		attempt = 1
	}
	digest := sha256.Sum256(image)
	model := fixture.Model
	if model == "" {
		model = "gpt-image-2"
	}
	receipt := map[string]any{
		"operation_id": fixture.OperationID, "operation_attempt": attempt, "task_id": fixture.TaskID,
		"model": model, "size": fixture.Size, "request_id": providerRequestID,
		"prompt_sha256": fixture.PromptHash, "output_sha256": hex.EncodeToString(digest[:]),
		"bytes": len(image), "actual_width": fixture.Width, "actual_height": fixture.Height,
		"provider_elapsed_seconds": 96.2,
		"generated_asset": map[string]any{
			"completed": true, "path": "/task/workdir/late.png", "size": fixture.Size,
			"width": fixture.Width, "height": fixture.Height,
		},
	}
	if fixture.Quality != "" {
		receipt["quality"] = fixture.Quality
	}
	receiptJSON, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("receipt", string(receiptJSON)); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", "late.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(image); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	path := "/api/daemon/runtimes/" + fixture.RuntimeID + "/tasks/" + fixture.TaskID +
		"/creative-image-operations/" + fixture.OperationID + "/attempts/" + strconv.Itoa(attempt) + "/late-success"
	req := httptest.NewRequest(http.MethodPost, path, &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req = req.WithContext(middleware.WithDaemonContext(req.Context(), workspaceID, daemonID))
	return withURLParams(req,
		"runtimeId", fixture.RuntimeID,
		"taskId", fixture.TaskID,
		"operationId", fixture.OperationID,
		"attempt", strconv.Itoa(attempt),
	)
}

func TestDaemonCreativeImageLateSuccessIsScopedIdempotentAndRecoverable(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createDaemonCreativeLateReceiptFixture(t)
	image := creativeTestPNG(t, 1080, 1080)
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	t.Run("cross workspace daemon token rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := daemonCreativeLateReceiptRequest(t, fixture, uuid.NewString(), fixture.DaemonID, "provider-late-96s", image)
		testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("cross-workspace late receipt = %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("member credential rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-late-96s", image)
		req = req.WithContext(t.Context())
		testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
		if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "daemon token") {
			t.Fatalf("member late receipt = %d %s", w.Code, w.Body.String())
		}
	})

	w := httptest.NewRecorder()
	req := daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-late-96s", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"disposition":"late_success"`) || !strings.Contains(w.Body.String(), `"recovery_queued":true`) {
		t.Fatalf("late success = %d %s", w.Code, w.Body.String())
	}
	var operationStatus, attemptStatus, operationRequestID, attemptRequestID, attachmentID, attachmentTaskID string
	if err := testPool.QueryRow(t.Context(), `
SELECT operation.status, attempt.status, operation.provider_request_id, attempt.provider_request_id,
       operation.output_attachment_id::text, attachment.task_id::text
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id AND attempt.attempt = 1
JOIN attachment ON attachment.id = operation.output_attachment_id
WHERE operation.id = $1
`, fixture.OperationID).Scan(
		&operationStatus, &attemptStatus, &operationRequestID, &attemptRequestID, &attachmentID, &attachmentTaskID,
	); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "completed" || attemptStatus != "completed" ||
		operationRequestID != "provider-late-96s" || attemptRequestID != "provider-late-96s" ||
		attachmentID == "" || attachmentTaskID != fixture.TaskID {
		t.Fatalf("late receipt state = op %q attempt %q requests %q/%q attachment %q task %q",
			operationStatus, attemptStatus, operationRequestID, attemptRequestID, attachmentID, attachmentTaskID)
	}
	var recoveryCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE parent_task_id = $1 AND status = 'queued'
  AND context->'late_receipt_recovery'->>'operation_id' = $2
`, fixture.TaskID, fixture.OperationID).Scan(&recoveryCount); err != nil {
		t.Fatal(err)
	}
	if recoveryCount != 1 {
		t.Fatalf("late receipt recovery tasks = %d, want 1", recoveryCount)
	}
	store.mu.Lock()
	storedBeforeReplay := len(store.files)
	store.mu.Unlock()

	w = httptest.NewRecorder()
	req = daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-late-96s", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"disposition":"reuse"`) {
		t.Fatalf("late success replay = %d %s", w.Code, w.Body.String())
	}
	store.mu.Lock()
	storedAfterReplay := len(store.files)
	store.mu.Unlock()
	if storedAfterReplay != storedBeforeReplay {
		t.Fatalf("late success replay stored %d files, want unchanged %d", storedAfterReplay, storedBeforeReplay)
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue
WHERE parent_task_id = $1 AND context ? 'late_receipt_recovery'
`, fixture.TaskID).Scan(&recoveryCount); err != nil {
		t.Fatal(err)
	}
	if recoveryCount != 1 {
		t.Fatalf("late success replay recovery tasks = %d, want 1", recoveryCount)
	}

	w = httptest.NewRecorder()
	req = daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "different-provider-request", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("completed operation mutation = %d %s", w.Code, w.Body.String())
	}
}

func TestDaemonCreativeImageLateSuccessBackfillsLegacyMissingPromptHash(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createDaemonCreativeLateReceiptFixture(t)
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation SET prompt_sha256 = '' WHERE id = $1
`, fixture.OperationID); err != nil {
		t.Fatal(err)
	}
	image := creativeTestPNG(t, fixture.Width, fixture.Height)
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	w := httptest.NewRecorder()
	req := daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "legacy-missing-prompt", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("legacy missing prompt late success = %d %s", w.Code, w.Body.String())
	}
	var promptHash string
	if err := testPool.QueryRow(t.Context(), `
SELECT prompt_sha256 FROM creative_image_operation WHERE id = $1
`, fixture.OperationID).Scan(&promptHash); err != nil {
		t.Fatal(err)
	}
	if promptHash != fixture.PromptHash {
		t.Fatalf("legacy prompt hash = %q, want %q", promptHash, fixture.PromptHash)
	}
}

func TestDaemonCreativeImageLateSuccessQueuesRecoveryBeforeTaskTerminal(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createDaemonCreativeLateReceiptFixture(t)
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue SET status = 'running', completed_at = NULL WHERE id = $1
`, fixture.TaskID); err != nil {
		t.Fatal(err)
	}
	image := creativeTestPNG(t, 1080, 1080)
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	w := httptest.NewRecorder()
	req := daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-before-terminal", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"recovery_queued":true`) {
		t.Fatalf("pre-terminal late success = %d %s", w.Code, w.Body.String())
	}
	var recoveryCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE parent_task_id = $1
  AND status = 'queued'
  AND context->'late_receipt_recovery'->>'operation_id' = $2
`, fixture.TaskID, fixture.OperationID).Scan(&recoveryCount); err != nil {
		t.Fatal(err)
	}
	if recoveryCount != 1 {
		t.Fatalf("pre-terminal recovery tasks = %d, want 1", recoveryCount)
	}

	w = httptest.NewRecorder()
	req = daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-before-terminal", image)
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"disposition":"reuse"`) {
		t.Fatalf("pre-terminal late success replay = %d %s", w.Code, w.Body.String())
	}
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE parent_task_id = $1
  AND context->'late_receipt_recovery'->>'operation_id' = $2
`, fixture.TaskID, fixture.OperationID).Scan(&recoveryCount); err != nil {
		t.Fatal(err)
	}
	if recoveryCount != 1 {
		t.Fatalf("pre-terminal recovery tasks after replay = %d, want 1", recoveryCount)
	}
}

func TestDaemonCreativeImageLateSuccessBindsEachContinuationAttempt(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	for _, receiptAttempt := range []int{1, 2} {
		t.Run("attempt "+strconv.Itoa(receiptAttempt), func(t *testing.T) {
			fixture := createDaemonCreativeLateReceiptFixture(t)
			var continuationRuntimeID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_runtime (
  workspace_id, daemon_id, name, runtime_mode, provider, status,
  device_info, metadata, owner_id, last_seen_at
)
VALUES ($1,$2,$3,'local',$4,'online','late receipt continuation test','{}'::jsonb,$5,now())
RETURNING id::text
`, testWorkspaceID, fixture.DaemonID, "late-receipt-continuation-"+uuid.NewString(),
				"codex-continuation-"+uuid.NewString(), testUserID).Scan(&continuationRuntimeID); err != nil {
				t.Fatal(err)
			}
			var continuationTaskID string
			if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, context, attempt, max_attempts,
  parent_task_id, retry_of_task_id, trigger_evidence_kind, trigger_evidence_ref_id, completed_at
)
SELECT agent_id, $2, issue_id, 'completed', context, 2, max_attempts,
       id, id, trigger_evidence_kind, trigger_evidence_ref_id, now()
FROM agent_task_queue
WHERE id = $1
RETURNING id::text
`, fixture.TaskID, continuationRuntimeID).Scan(&continuationTaskID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation_attempt
SET status = 'failed', provider_request_id = 'provider-continuation-attempt-1',
    error_type = 'transport_timeout', completed_at = now()
WHERE operation_id = $1 AND attempt = 1
`, fixture.OperationID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation
SET provider_request_id = 'provider-continuation-attempt-2'
WHERE id = $1
`, fixture.OperationID); err != nil {
				t.Fatal(err)
			}
			if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (
  operation_id, attempt, status, runtime_id, task_id, provider_request_id
)
VALUES ($1,2,'unknown',$2,$3,'provider-continuation-attempt-2')
`, fixture.OperationID, continuationRuntimeID, continuationTaskID); err != nil {
				t.Fatal(err)
			}

			store := &mockStorage{}
			originalStorage := testHandler.Storage
			testHandler.Storage = store
			t.Cleanup(func() { testHandler.Storage = originalStorage })

			if receiptAttempt == 2 {
				var unrelatedTaskID string
				if err := testPool.QueryRow(t.Context(), `
INSERT INTO agent_task_queue (
  agent_id, runtime_id, issue_id, status, context, attempt, max_attempts,
  trigger_evidence_kind, trigger_evidence_ref_id, completed_at
)
SELECT agent_id, $2, issue_id, 'completed', context, 2, max_attempts,
       trigger_evidence_kind, trigger_evidence_ref_id, now()
FROM agent_task_queue
WHERE id = $1
RETURNING id::text
`, fixture.TaskID, continuationRuntimeID).Scan(&unrelatedTaskID); err != nil {
					t.Fatal(err)
				}
				if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation_attempt SET task_id = $2
WHERE operation_id = $1 AND attempt = 2
`, fixture.OperationID, unrelatedTaskID); err != nil {
					t.Fatal(err)
				}
				crossChain := fixture
				crossChain.RuntimeID = continuationRuntimeID
				crossChain.TaskID = unrelatedTaskID
				crossChain.Attempt = 2
				w := httptest.NewRecorder()
				req := daemonCreativeLateReceiptRequest(
					t, crossChain, testWorkspaceID, fixture.DaemonID,
					"provider-cross-chain", creativeTestPNG(t, fixture.Width, fixture.Height),
				)
				testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
				if w.Code != http.StatusNotFound {
					t.Fatalf("cross-chain attempt receipt = %d %s", w.Code, w.Body.String())
				}
				if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation_attempt SET task_id = $2
WHERE operation_id = $1 AND attempt = 2
`, fixture.OperationID, continuationTaskID); err != nil {
					t.Fatal(err)
				}
			}

			target := fixture
			if receiptAttempt == 2 {
				target.RuntimeID = continuationRuntimeID
				target.TaskID = continuationTaskID
				target.Attempt = 2
			}
			providerRequestID := "provider-continuation-attempt-" + strconv.Itoa(receiptAttempt)
			w := httptest.NewRecorder()
			req := daemonCreativeLateReceiptRequest(
				t, target, testWorkspaceID, fixture.DaemonID, providerRequestID,
				creativeTestPNG(t, fixture.Width, fixture.Height),
			)
			testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"disposition":"late_success"`) {
				t.Fatalf("continuation attempt %d late receipt = %d %s", receiptAttempt, w.Code, w.Body.String())
			}

			var operationTaskID, operationRuntimeID, completedAttemptStatus, supersededAttemptStatus, attachmentTaskID string
			if err := testPool.QueryRow(t.Context(), `
SELECT operation.task_id::text, operation.runtime_id::text,
       completed.status, superseded.status, attachment.task_id::text
FROM creative_image_operation operation
JOIN creative_image_operation_attempt completed
  ON completed.operation_id = operation.id AND completed.attempt = $2
JOIN creative_image_operation_attempt superseded
  ON superseded.operation_id = operation.id AND superseded.attempt = CASE WHEN $2 = 1 THEN 2 ELSE 1 END
JOIN attachment ON attachment.id = operation.output_attachment_id
WHERE operation.id = $1
`, fixture.OperationID, receiptAttempt).Scan(
				&operationTaskID, &operationRuntimeID, &completedAttemptStatus, &supersededAttemptStatus, &attachmentTaskID,
			); err != nil {
				t.Fatal(err)
			}
			wantSupersededStatus := "cancelled"
			if receiptAttempt == 2 {
				wantSupersededStatus = "failed"
			}
			if operationTaskID != fixture.TaskID || operationRuntimeID != fixture.RuntimeID ||
				completedAttemptStatus != "completed" || supersededAttemptStatus != wantSupersededStatus || attachmentTaskID != target.TaskID {
				t.Fatalf("attempt %d binding = operation task/runtime %s/%s, statuses %s/%s, attachment task %s",
					receiptAttempt, operationTaskID, operationRuntimeID, completedAttemptStatus, supersededAttemptStatus, attachmentTaskID)
			}
		})
	}
}

func TestDaemonCreativeImageLateSuccessQueuesOneRecoveryPerLateSize(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	first := createDaemonCreativeLateReceiptFixture(t)
	second := first
	second.Size, second.Width, second.Height = "1200x628", 1200, 628
	second.PromptHash = creativePromptSHA256("daemon second late receipt prompt")
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_image_operation (
  variant_id, size_key, revision, operation_kind, idempotency_key, status,
  model, runtime_id, task_id, prompt_sha256, input_snapshot, started_at
)
VALUES ($1,$2,1,'generation',$3,'unknown','gpt-image-2',$4,$5,$6,
        '{"source":"second-late-receipt-test"}'::jsonb,now())
RETURNING id::text
`, second.VariantID, second.Size, "second-late-receipt:"+second.TaskID, second.RuntimeID, second.TaskID, second.PromptHash).Scan(&second.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_image_operation_attempt (operation_id, attempt, status, runtime_id, task_id)
VALUES ($1,1,'unknown',$2,$3)
`, second.OperationID, second.RuntimeID, second.TaskID); err != nil {
		t.Fatal(err)
	}
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	for index, current := range []daemonCreativeLateReceiptFixture{first, second} {
		image := creativeTestPNG(t, current.Width, current.Height)
		w := httptest.NewRecorder()
		req := daemonCreativeLateReceiptRequest(t, current, testWorkspaceID, current.DaemonID, "provider-multi-size-"+strconv.Itoa(index+1), image)
		testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"recovery_queued":true`) {
			t.Fatalf("late size %s = %d %s", current.Size, w.Code, w.Body.String())
		}
	}

	var recoveryContextRaw []byte
	if err := testPool.QueryRow(t.Context(), `
SELECT context
FROM agent_task_queue
WHERE parent_task_id = $1 AND context ? 'late_receipt_recovery'
`, first.TaskID).Scan(&recoveryContextRaw); err != nil {
		t.Fatal(err)
	}
	var recoveryContext struct {
		MissingSizes []string `json:"missing_sizes"`
		Recoveries   []struct {
			OperationID string `json:"operation_id"`
			SizeKey     string `json:"size_key"`
		} `json:"late_receipt_recoveries"`
	}
	if err := json.Unmarshal(recoveryContextRaw, &recoveryContext); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{first.OperationID: first.Size, second.OperationID: second.Size}
	for _, recovery := range recoveryContext.Recoveries {
		if want[recovery.OperationID] != recovery.SizeKey {
			t.Fatalf("recovery %s size = %q, want %q", recovery.OperationID, recovery.SizeKey, want[recovery.OperationID])
		}
	}
	if len(recoveryContext.Recoveries) != 2 || len(recoveryContext.MissingSizes) != 2 {
		t.Fatalf("late size recovery entries/scopes = %d/%d, want 2/2", len(recoveryContext.Recoveries), len(recoveryContext.MissingSizes))
	}
	var recoveryTaskCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM agent_task_queue WHERE parent_task_id = $1 AND context ? 'late_receipt_recovery'
`, first.TaskID).Scan(&recoveryTaskCount); err != nil {
		t.Fatal(err)
	}
	if recoveryTaskCount != 1 {
		t.Fatalf("aggregated late size recovery tasks = %d, want 1", recoveryTaskCount)
	}
}

func TestDaemonCreativeImageLateSuccessPreservesDirectEditContinuation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createDaemonCreativeLateReceiptFixture(t)
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue
SET context = jsonb_set(
  jsonb_set(context, '{workflow}', '"creative_direct_edit"'::jsonb),
  '{edit_sizes}', jsonb_build_array('1080x1080')
)
WHERE id = $1
`, fixture.TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_image_operation SET operation_kind = 'direct_edit' WHERE id = $1
`, fixture.OperationID); err != nil {
		t.Fatal(err)
	}
	store := &mockStorage{}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	w := httptest.NewRecorder()
	req := daemonCreativeLateReceiptRequest(t, fixture, testWorkspaceID, fixture.DaemonID, "provider-direct-late", creativeTestPNG(t, fixture.Width, fixture.Height))
	testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"recovery_queued":true`) {
		t.Fatalf("direct late success = %d %s", w.Code, w.Body.String())
	}
	var workflow, size, operationID string
	if err := testPool.QueryRow(t.Context(), `
SELECT context->>'workflow', context->'edit_sizes'->>0,
       context->'late_receipt_recoveries'->0->>'operation_id'
FROM agent_task_queue
WHERE parent_task_id = $1 AND context ? 'late_receipt_recovery'
`, fixture.TaskID).Scan(&workflow, &size, &operationID); err != nil {
		t.Fatal(err)
	}
	if workflow != "creative_direct_edit" || size != fixture.Size || operationID != fixture.OperationID {
		t.Fatalf("direct recovery context = workflow %q size %q operation %q", workflow, size, operationID)
	}
}

func TestDaemonCreativeImageLateSuccessDoesNotOutrunCancellation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	fixture := createDaemonCreativeLateReceiptFixture(t)
	if _, err := testPool.Exec(t.Context(), `
UPDATE agent_task_queue SET status = 'running', completed_at = NULL WHERE id = $1
`, fixture.TaskID); err != nil {
		t.Fatal(err)
	}

	cancelTx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer cancelTx.Rollback(t.Context())
	if _, err := cancelTx.Exec(t.Context(), `
UPDATE agent_task_queue SET status = 'cancelled', completed_at = now() WHERE id = $1
`, fixture.TaskID); err != nil {
		t.Fatal(err)
	}

	uploaded := make(chan struct{})
	store := &mockStorage{onUpload: func() { close(uploaded) }}
	originalStorage := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = originalStorage })
	req := daemonCreativeLateReceiptRequest(
		t, fixture, testWorkspaceID, fixture.DaemonID, "provider-cancel-race",
		creativeTestPNG(t, fixture.Width, fixture.Height),
	)
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		w := httptest.NewRecorder()
		testHandler.ReportDaemonCreativeImageLateSuccess(w, req)
		response <- w
	}()

	select {
	case <-uploaded:
	case <-time.After(5 * time.Second):
		t.Fatal("late receipt did not reach storage before cancellation fence")
	}
	if err := cancelTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	var w *httptest.ResponseRecorder
	select {
	case w = <-response:
	case <-time.After(5 * time.Second):
		t.Fatal("late receipt did not settle after cancellation committed")
	}
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "not running or terminal") {
		t.Fatalf("cancelled late success = %d %s", w.Code, w.Body.String())
	}

	var operationStatus, attemptStatus string
	var outputMissing bool
	var recoveryCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT operation.status, attempt.status, operation.output_attachment_id IS NULL,
       (SELECT count(*) FROM agent_task_queue child
        WHERE child.parent_task_id = $1 AND child.context ? 'late_receipt_recovery')
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt
  ON attempt.operation_id = operation.id AND attempt.attempt = 1
WHERE operation.id = $2
`, fixture.TaskID, fixture.OperationID).Scan(&operationStatus, &attemptStatus, &outputMissing, &recoveryCount); err != nil {
		t.Fatal(err)
	}
	if operationStatus != "unknown" || attemptStatus != "unknown" || !outputMissing || recoveryCount != 0 {
		t.Fatalf("cancelled late receipt mutated state: operation=%q attempt=%q output_missing=%v recoveries=%d",
			operationStatus, attemptStatus, outputMissing, recoveryCount)
	}
}
