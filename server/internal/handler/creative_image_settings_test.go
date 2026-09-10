package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/pkg/imagemodel"
)

func TestCreativeImageSettingsPersistAcrossProductionAndExpansion(t *testing.T) {
	for _, current := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing order", true: "new order"}[current], func(t *testing.T) {
			f := createCreativeCountFixture(t, 1)
			want := imagemodel.Settings{Model: imagemodel.Image2, Quality: "high"}
			if current {
				want = imagemodel.Default()
				encoded, _ := json.Marshal(want)
				if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot=input_snapshot||jsonb_build_object('image_generation',$2::jsonb) WHERE id=$1`, f.OrderID, encoded); err != nil {
					t.Fatal(err)
				}
			}
			id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
			addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
			raw, _ := json.Marshal(map[string]any{"type": "creative_domain_task", "workflow": "creative_production", "variant_id": id, "image_generation": map[string]string{"model": "wrong", "quality": "wrong"}})
			normalized, err := normalizeCreativeProductionFanoutItem(t.Context(), testPool, parseUUID(testWorkspaceID), parseUUID(f.ItemID), service.DirectTaskFanoutItem{Context: raw})
			if err != nil {
				t.Fatal(err)
			}
			if settings, err := imagemodel.FromSnapshot(normalized.Context); err != nil || settings != want {
				t.Fatalf("production=%+v %v", settings, err)
			}
			tasks, err := testHandler.queueSelectedCreativeProductionTasks(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}, creativeSelectedExpansionPhase)
			if err != nil || len(tasks) != 1 {
				t.Fatalf("expansion=%d %v", len(tasks), err)
			}
			if settings, err := imagemodel.FromSnapshot(tasks[0].Context); err != nil || settings != want {
				t.Fatalf("expansion settings=%+v %v", settings, err)
			}
			settingsJSON, _ := json.Marshal(map[string]any{"image_generation": want, "target_size": "1200x628"})
			input := creativeImageOperationInput{VariantID: id, SizeKey: "1200x628", Revision: 1, OperationKind: "generation", IdempotencyKey: id + ":landscape", Status: "running", Model: want.Model, InputSnapshot: settingsJSON, Attempt: 1}
			put := func(input creativeImageOperationInput) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				r := withURLParam(newRequest(http.MethodPut, "/api/creative/orders/"+f.OrderID+"/image-operations", input), "id", f.OrderID)
				r.Header.Set("X-Actor-Source", "task_token")
				r.Header.Set("X-Agent-ID", uuidToString(tasks[0].AgentID))
				r.Header.Set("X-Task-ID", uuidToString(tasks[0].ID))
				testHandler.UpsertCreativeImageOperation(w, r)
				return w
			}
			if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='running' WHERE id=$1`, tasks[0].ID); err != nil {
				t.Fatal(err)
			}
			wrong := input
			wrong.Model = imagemodel.Flare
			if w := put(wrong); w.Code != http.StatusConflict {
				t.Fatalf("changed model=%d %s", w.Code, w.Body.String())
			}
			if w := put(input); w.Code != http.StatusOK && w.Code != http.StatusCreated {
				t.Fatalf("operation=%d %s", w.Code, w.Body.String())
			}
			input.Status = "completed"
			input.ProviderRequestID = "image-settings-request"
			input.PromptSHA256 = creativePromptSHA256("Frozen settings")
			input.OutputAttachmentID = createCreativeOrderAssetAttachment(t, "model-result.png")
			receipt := map[string]any{"model": want.Model, "quality": want.Quality, "prompt_sha256": input.PromptSHA256, "request_id": input.ProviderRequestID}
			if current {
				receipt["quality"] = "high"
				input.ResultReceipt, _ = json.Marshal(receipt)
				if w := put(input); w.Code != http.StatusConflict {
					t.Fatalf("mismatched quality=%d %s", w.Code, w.Body.String())
				}
			}
			receipt["quality"] = want.Quality
			input.ResultReceipt, _ = json.Marshal(receipt)
			if w := put(input); w.Code != http.StatusOK {
				t.Fatalf("completed operation=%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestCreativeImage25CompletedAssetTrace(t *testing.T) {
	oldMetadata, oldEvidence := completedGeneratedAssetTrace("Older CLI", "request-old", 1)
	var oldRecord map[string]any
	_ = json.Unmarshal(oldEvidence, &oldRecord)
	oldRecord["model_result"].(map[string]any)["quality"] = "high"
	oldEvidence, _ = json.Marshal(oldRecord)
	if err := validateCompletedGeneratedAssetTrace(oldMetadata, oldEvidence); err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{imagemodel.Sunburst, imagemodel.Flare} {
		metadata, evidence := completedGeneratedAssetTrace("Exact approved copy", "request-25", 1)
		var m, e map[string]any
		_ = json.Unmarshal(metadata, &m)
		_ = json.Unmarshal(evidence, &e)
		m["model"], m["quality"] = model, "xhigh"
		r := e["model_result"].(map[string]any)
		r["model"], r["quality"] = model, "xhigh"
		metadata, _ = json.Marshal(m)
		evidence, _ = json.Marshal(e)
		if err := validateCompletedGeneratedAssetTrace(metadata, evidence); err != nil {
			t.Fatal(err)
		}
		r["quality"] = "high"
		evidence, _ = json.Marshal(e)
		if err := validateCompletedGeneratedAssetTrace(metadata, evidence); err == nil {
			t.Fatal("quality mismatch accepted")
		}
	}
}

func TestCreativeImage25LateReceiptPreservesQualityAndQueuesOneRecovery(t *testing.T) {
	f := createDaemonCreativeLateReceiptFixture(t)
	f.Model = imagemodel.Sunburst
	f.Quality = "xhigh"
	snapshot, _ := json.Marshal(map[string]any{imagemodel.SnapshotKey: imagemodel.Default()})
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_image_operation SET model=$2,input_snapshot=$3 WHERE id=$1`, f.OperationID, f.Model, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot=input_snapshot||$2::jsonb WHERE id=$1`, f.OrderID, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=context||$2::jsonb WHERE id=$1`, f.TaskID, snapshot); err != nil {
		t.Fatal(err)
	}
	prior := testHandler.Storage
	testHandler.Storage = &mockStorage{}
	t.Cleanup(func() { testHandler.Storage = prior })
	data := creativeTestPNG(t, 1080, 1080)
	bad := f
	bad.Quality = "high"
	w := httptest.NewRecorder()
	testHandler.ReportDaemonCreativeImageLateSuccess(w, daemonCreativeLateReceiptRequest(t, bad, testWorkspaceID, f.DaemonID, "late-25", data))
	if w.Code != http.StatusConflict {
		t.Fatalf("wrong quality=%d %s", w.Code, w.Body.String())
	}
	for range 2 {
		w = httptest.NewRecorder()
		testHandler.ReportDaemonCreativeImageLateSuccess(w, daemonCreativeLateReceiptRequest(t, f, testWorkspaceID, f.DaemonID, "late-25", data))
		if w.Code != http.StatusOK {
			t.Fatalf("late result=%d %s", w.Code, w.Body.String())
		}
	}
	var quality string
	var children int
	var context []byte
	if err := testPool.QueryRow(t.Context(), `SELECT result_receipt->>'quality' FROM creative_image_operation WHERE id=$1`, f.OperationID).Scan(&quality); err != nil || quality != "xhigh" {
		t.Fatalf("saved quality=%s %v", quality, err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE parent_task_id=$1`, f.TaskID).Scan(&children); err != nil || children != 1 {
		t.Fatalf("recovery children=%d %v", children, err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT context FROM agent_task_queue WHERE parent_task_id=$1`, f.TaskID).Scan(&context); err != nil {
		t.Fatal(err)
	}
	if settings, err := imagemodel.FromSnapshot(context); err != nil || settings != imagemodel.Default() {
		t.Fatalf("recovery settings=%+v %v", settings, err)
	}
}
