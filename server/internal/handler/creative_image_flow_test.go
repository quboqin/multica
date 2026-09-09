package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/imagemodel"
)

func TestCreativeImage25SixSetFlowThroughPrimeAndDelivery(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	store := &mockStorage{}
	previous := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = previous })
	t.Setenv("MULTICA_CREATIVE_PRIME_CACHE_DIR", t.TempDir())
	snapshot := imageFlowMarketSnapshot(t, f, store)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot=$2 WHERE id=$1`, f.OrderID, snapshot); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 8)
	primaries := make(map[string]string)
	for index := range ids {
		ids[index] = createCreativeCandidateOrchestrationVariant(t, f.ItemID, fmt.Sprintf("C%02d", index+1), "candidate", nil, "running", "1080x1080", []string{"1080x1080"})
	}
	for _, id := range ids {
		task := addCreativeCandidateOrchestrationProductionTask(t, f, id, "running", "candidate_primary")
		var fields map[string]any
		if err := json.Unmarshal(task.Context, &fields); err != nil {
			t.Fatal(err)
		}
		fields["expected_sizes"] = []string{"1080x1080"}
		fields[imagemodel.SnapshotKey] = imagemodel.Default()
		task.Context, _ = json.Marshal(fields)
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=$2 WHERE id=$1`, task.ID, task.Context); err != nil {
			t.Fatal(err)
		}
		primaries[id] = imageFlowRegisterGenerated(t, f, task, id, "1080x1080", store)
		imageFlowCompleteProduction(t, task)
		imageFlowCompose(t, f, id)
	}
	var selectionTaskID string
	if err := testPool.QueryRow(t.Context(), `SELECT task_id FROM creative_task_binding WHERE order_id=$1 AND workflow='creative_candidate_selection'`, f.OrderID).Scan(&selectionTaskID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='running' WHERE id=$1`, selectionTaskID); err != nil {
		t.Fatal(err)
	}
	selectionTask, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(selectionTaskID))
	if err != nil {
		t.Fatal(err)
	}
	r := withURLParams(newRequest(http.MethodPost, "/", creativeCandidateSelectionInput{SelectedIDs: ids[:6], ReserveIDs: ids[6:]}), "id", f.OrderID, "itemId", f.ItemID)
	imageFlowTaskHeaders(r, selectionTask)
	w := httptest.NewRecorder()
	testHandler.SelectCreativeOrderItemCandidates(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("candidate selection=%d %s", w.Code, w.Body.String())
	}
	for _, id := range ids[:6] {
		var taskID string
		if err := testPool.QueryRow(t.Context(), `SELECT task_id FROM creative_task_binding WHERE variant_id=$1 AND production_phase=$2`, id, creativeSelectedExpansionPhase).Scan(&taskID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='running' WHERE id=$1`, taskID); err != nil {
			t.Fatal(err)
		}
		task, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(taskID))
		if err != nil {
			t.Fatal(err)
		}
		if settings, err := imagemodel.FromSnapshot(task.Context); err != nil || settings != imagemodel.Default() {
			t.Fatalf("expansion settings=%+v %v", settings, err)
		}
		for _, size := range []string{"1200x628", "800x1000"} {
			imageFlowRegisterGenerated(t, f, task, id, size, store)
		}
		imageFlowCompleteProduction(t, task)
		imageFlowCompose(t, f, id)
		var qcTaskID string
		if err := testPool.QueryRow(t.Context(), `SELECT task_id FROM creative_task_binding WHERE variant_id=$1 AND workflow='creative_qc_visual' ORDER BY created_at DESC LIMIT 1`, id).Scan(&qcTaskID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='running' WHERE id=$1`, qcTaskID); err != nil {
			t.Fatal(err)
		}
		qc, err := testHandler.Queries.GetAgentTask(t.Context(), parseUUID(qcTaskID))
		if err != nil {
			t.Fatal(err)
		}
		r := withURLParam(newRequest(http.MethodPut, "/", creativeOrderQCInput{VariantID: id, Revision: 1, Lane: "visual", Status: "passed", Findings: json.RawMessage(`{}`)}), "id", f.OrderID)
		imageFlowTaskHeaders(r, qc)
		w := httptest.NewRecorder()
		testHandler.UpsertCreativeOrderQC(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("QC report=%d %s", w.Code, w.Body.String())
		}
		r = withURLParam(newRequest(http.MethodPost, "/", creativeOrderQCFinalizeInput{VariantID: id, Revision: 1}), "id", f.OrderID)
		imageFlowTaskHeaders(r, qc)
		w = httptest.NewRecorder()
		testHandler.FinalizeCreativeOrderQC(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("QC finalize=%d %s", w.Code, w.Body.String())
		}
		var primary string
		if err := testPool.QueryRow(t.Context(), `SELECT id FROM creative_order_asset WHERE variant_id=$1 AND stage='generated' AND size_key='1080x1080'`, id).Scan(&primary); err != nil || primary != primaries[id] {
			t.Fatalf("primary changed=%s %v", primary, err)
		}
	}
	var delivered, reserveAssets, active, wrongModels int
	if err := testPool.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM creative_order_asset a JOIN creative_order_variant v ON v.id=a.variant_id WHERE v.order_item_id=$1 AND a.stage='delivered' AND a.status='completed'),
 (SELECT count(*) FROM creative_order_asset a JOIN creative_order_variant v ON v.id=a.variant_id WHERE v.order_item_id=$1 AND v.candidate_state='reserve' AND a.size_key<>'1080x1080'),
 (SELECT count(*) FROM creative_order_variant WHERE order_item_id=$1 AND active_revision=1),
 (SELECT count(*) FROM creative_image_operation o JOIN creative_order_variant v ON v.id=o.variant_id WHERE v.order_item_id=$1 AND (o.model<>$2 OR o.result_receipt->>'quality'<>'xhigh'))`, f.ItemID, imagemodel.Sunburst).Scan(&delivered, &reserveAssets, &active, &wrongModels); err != nil {
		t.Fatal(err)
	}
	if delivered != 18 || reserveAssets != 0 || active != 6 || wrongModels != 0 {
		t.Fatalf("delivered=%d reserve expansion=%d active=%d wrong models=%d", delivered, reserveAssets, active, wrongModels)
	}
	t.Log("8 candidate primaries -> 6 selected packages -> 18 delivered images; original primaries preserved; no reserve expansion")
}

func imageFlowTaskHeaders(r *http.Request, task db.AgentTaskQueue) {
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Agent-ID", uuidToString(task.AgentID))
	r.Header.Set("X-Task-ID", uuidToString(task.ID))
}

func imageFlowRegisterGenerated(t *testing.T, f creativeCandidateOrchestrationFixture, task db.AgentTaskQueue, variantID, size string, store *mockStorage, verifyPrimeContext ...bool) string {
	t.Helper()
	width, height, _ := creativeAssetSizeDimensions(size)
	data := imageFlowPNG(t, width, height, false)
	attachment := seedPreviewAttachment(t, store, "image25/"+uuid.NewString()+".png", size+".png", "image/png", data)
	settings := imagemodel.Default()
	inputSnapshot, _ := json.Marshal(map[string]any{imagemodel.SnapshotKey: settings, "target_size": size})
	checkContext := len(verifyPrimeContext) > 0 && verifyPrimeContext[0]
	if checkContext {
		inputSnapshot, _ = json.Marshal(map[string]any{imagemodel.SnapshotKey: settings, "target_size": size, "input_asset_attachments": map[string]string{"prime_context_sha256": attachment}})
	}
	op := creativeImageOperationInput{VariantID: variantID, SizeKey: size, Revision: 1, OperationKind: "generation", IdempotencyKey: variantID + ":" + size, Status: "running", Model: settings.Model, InputSnapshot: inputSnapshot, Attempt: 1}
	putOperation := func() creativeImageOperationResponse {
		r := withURLParam(newRequest(http.MethodPut, "/", op), "id", f.OrderID)
		imageFlowTaskHeaders(r, task)
		w := httptest.NewRecorder()
		testHandler.UpsertCreativeImageOperation(w, r)
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("operation=%d %s", w.Code, w.Body.String())
		}
		var result creativeImageOperationResponse
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	operation := putOperation()
	prompt := "Frozen image flow " + size
	hash := creativePromptSHA256(prompt)
	requestID := uuid.NewString()
	receipt := map[string]any{"model": settings.Model, "quality": settings.Quality, "prompt": prompt, "prompt_sha256": hash, "request_id": requestID, "attempts": 1, "actual_width": width, "actual_height": height, "actual_aspect_ratio": float64(width) / float64(height)}
	op.Status = "completed"
	op.OutputAttachmentID = attachment
	op.ProviderRequestID = requestID
	op.PromptSHA256 = hash
	op.ResultReceipt, _ = json.Marshal(receipt)
	putOperation()
	metadata, _ := json.Marshal(receipt)
	evidence, _ := json.Marshal(map[string]any{"model_result": receipt, "request_id": requestID, "attempts": 1, "prompt_sha256": hash, "prompt_contract": map[string]string{"prompt_sha256": hash}, "normalization": map[string]any{"target_size": map[string]int{"width": width, "height": height}}})
	asset := creativeOrderAssetInput{VariantID: variantID, OperationID: operation.ID, SizeKey: size, Revision: 1, Stage: "generated", Status: "completed", AttachmentID: attachment, Metadata: metadata, Evidence: evidence}
	if checkContext {
		// A guide for another attachment must not satisfy this operation's input.
		wrongAttachment := seedPreviewAttachment(t, store, "image25/"+uuid.NewString()+".png", "other-context.png", "image/png", data)
		if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_order_diagnostic_asset(variant_id,attachment_id,size_key,revision,workflow,label,filename,metadata) VALUES($1,$2,$3,1,'creative_production','Prime context','guide.png',$4)`, variantID, wrongAttachment, size, fmt.Sprintf(`{"prime_template_source_role":"template_%s"}`, size)); err != nil {
			t.Fatal(err)
		}
		r := withURLParam(newRequest(http.MethodPut, "/", asset), "id", f.OrderID)
		imageFlowTaskHeaders(r, task)
		w := httptest.NewRecorder()
		testHandler.UpsertCreativeOrderAsset(w, r)
		if w.Code != http.StatusUnprocessableEntity || !bytes.Contains(w.Body.Bytes(), []byte("Prime context template evidence is required")) {
			t.Fatalf("missing actual context evidence=%d %s", w.Code, w.Body.String())
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_diagnostic_asset SET attachment_id=$2 WHERE variant_id=$1 AND label='Prime context'`, variantID, attachment); err != nil {
			t.Fatal(err)
		}
	}
	var result creativeOrderAssetResponse
	for range 2 {
		r := withURLParam(newRequest(http.MethodPut, "/", asset), "id", f.OrderID)
		imageFlowTaskHeaders(r, task)
		w := httptest.NewRecorder()
		testHandler.UpsertCreativeOrderAsset(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("asset=%d %s", w.Code, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
	}
	for _, label := range creativeProductionProcessLabels {
		if checkContext && label == "Prime context" {
			continue
		}
		if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_order_diagnostic_asset(variant_id,attachment_id,size_key,revision,workflow,label,filename) VALUES($1,$2,$3,1,'creative_production',$4,$5)`, variantID, attachment, size, label, label+".png"); err != nil {
			t.Fatal(err)
		}
	}
	return result.ID
}

func TestCreativePrimeContextEvidenceBindsActualOperationInput(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	store := &mockStorage{}
	previous := testHandler.Storage
	testHandler.Storage = store
	t.Cleanup(func() { testHandler.Storage = previous })
	t.Setenv("MULTICA_CREATIVE_PRIME_CACHE_DIR", t.TempDir())
	snapshot := imageFlowMarketSnapshot(t, f, store)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET input_snapshot=$2::jsonb || '{"prime_context_policy_version":1}'::jsonb WHERE id=$1`, f.OrderID, snapshot); err != nil {
		t.Fatal(err)
	}
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "candidate", nil, "running", "1080x1080", []string{"1080x1080"})
	task := addCreativeCandidateOrchestrationProductionTask(t, f, id, "running", "candidate_primary")
	var taskContext map[string]any
	if err := json.Unmarshal(task.Context, &taskContext); err != nil {
		t.Fatal(err)
	}
	taskContext["expected_sizes"] = []string{"1080x1080"}
	task.Context, _ = json.Marshal(taskContext)
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=$2 WHERE id=$1`, task.ID, task.Context); err != nil {
		t.Fatal(err)
	}
	assetID := imageFlowRegisterGenerated(t, f, task, id, "1080x1080", store, true)
	var role string
	if err := testPool.QueryRow(t.Context(), `SELECT metadata->>'prime_template_source_role' FROM creative_order_asset WHERE id=$1`, assetID).Scan(&role); err != nil || role != "template_1080x1080" {
		t.Fatalf("bound role=%s %v", role, err)
	}
	imageFlowCompleteProduction(t, task)
	imageFlowCompose(t, f, id)
	if err := testPool.QueryRow(t.Context(), `SELECT metadata->'template'->>'source_role' FROM creative_order_asset WHERE variant_id=$1 AND stage='primed'`, id).Scan(&role); err != nil || role != "template_1080x1080" {
		t.Fatalf("composed role=%s %v", role, err)
	}
}

func imageFlowCompleteProduction(t *testing.T, task db.AgentTaskQueue) {
	t.Helper()
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='completed',completed_at=now() WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	task.Status = "completed"
	if err := testHandler.settleCreativeProductionVariantTask(t.Context(), task); err != nil {
		t.Fatal(err)
	}
}

func imageFlowCompose(t *testing.T, f creativeCandidateOrchestrationFixture, variantID string) {
	t.Helper()
	claim, found, err := testHandler.claimCreativePrimeComposition(t.Context(), parseUUID(variantID), 1)
	if err != nil || !found {
		t.Fatalf("Prime claim=%v %v", found, err)
	}
	if ok, err := testHandler.composeCreativeOrderVariantPrime(t.Context(), parseUUID(testWorkspaceID), parseUUID(f.OrderID), parseUUID(variantID), parseUUID(testUserID), false, &claim); err != nil || !ok {
		t.Fatalf("Prime composition=%v %v", ok, err)
	}
	if err := testHandler.completeCreativePrimeCompositionHandoff(t.Context(), claim); err != nil {
		t.Fatal(err)
	}
}

func imageFlowPNG(t *testing.T, width, height int, template bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	if !template {
		draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	} else {
		for x := 12; x < 120; x += 12 {
			draw.Draw(img, image.Rect(x, 12, x+6, 30), image.NewUniform(color.Black), image.Point{}, draw.Src)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func imageFlowMarketSnapshot(t *testing.T, f creativeCandidateOrchestrationFixture, store *mockStorage) []byte {
	t.Helper()
	var raw []byte
	if err := testPool.QueryRow(t.Context(), `SELECT input_snapshot FROM creative_order WHERE id=$1`, f.OrderID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	templates := map[string]any{}
	layouts := map[string]any{}
	files := []map[string]any{}
	for _, size := range standardCreativeAssetSizes {
		width, height, _ := creativeAssetSizeDimensions(size)
		role := "template_" + size
		attachment := seedPreviewAttachment(t, store, "image25/"+uuid.NewString()+".png", role+".png", "image/png", imageFlowPNG(t, width, height, true))
		templates[size] = map[string]any{"source_role": role}
		layouts[size] = map[string]any{"hard_regions": []any{}, "top_key_content_exclusion_end": 40, "bottom_key_content_exclusion_start": height - 40}
		files = append(files, map[string]any{"id": uuid.NewString(), "role": role, "attachment_id": attachment})
	}
	families := []map[string]any{{"id": "light_background", "label": "Dark glyphs", "templates": templates}}
	snapshot["market_pack"] = map[string]any{"id": uuid.NewString(), "version": 1, "files": files, "config": map[string]any{
		"prime_template_set":            map[string]any{"schema_version": 2, "selection_mode": "automatic_family_contrast", "families": families},
		"prime_template_set_validation": map[string]any{"status": "passed", "families": families}, "prime_layout_contract": map[string]any{"layouts": layouts},
	}}
	snapshot[imagemodel.SnapshotKey] = imagemodel.Default()
	result, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
