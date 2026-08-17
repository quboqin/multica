package handler

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/creative"
)

func TestNormalizeCreativeEditRulesDefaults(t *testing.T) {
	rules, err := normalizeCreativeEditRules(nil)
	if err != nil {
		t.Fatalf("normalizeCreativeEditRules: %v", err)
	}
	if rules["variant_count"] != 3 || rules["market"] != "idn-adakami" {
		t.Fatalf("rules = %#v", rules)
	}
	sizes, ok := rules["sizes"].([]any)
	if !ok || len(sizes) != 3 {
		t.Fatalf("sizes = %#v", rules["sizes"])
	}
}

func TestNormalizeCreativeEditRulesRejectsInvalidSize(t *testing.T) {
	_, err := normalizeCreativeEditRules(map[string]any{
		"sizes": []any{map[string]any{"width": float64(10), "height": float64(10)}},
	})
	if err == nil {
		t.Fatal("expected invalid dimensions to fail")
	}
}

func TestCreativeResultCompletionStatusRequiresEveryVariantAndSize(t *testing.T) {
	rules := map[string]any{
		"variant_count": 3,
		"sizes": []any{
			map[string]any{"width": 1080, "height": 1080},
			map[string]any{"width": 800, "height": 1000},
			map[string]any{"width": 1200, "height": 628},
		},
	}
	candidates := []creative.Candidate{{ID: "candidate-1"}}
	allSizes := func() []creative.Asset {
		return []creative.Asset{
			{Width: 1080, Height: 1080},
			{Width: 800, Height: 1000},
			{Width: 1200, Height: 628},
		}
	}
	complete := []creative.Variant{
		{CandidateID: "candidate-1", Index: 1, Assets: allSizes()},
		{CandidateID: "candidate-1", Index: 2, Assets: allSizes()},
		{CandidateID: "candidate-1", Index: 3, Assets: allSizes()},
	}
	if got := creativeResultCompletionStatus(rules, candidates, complete); got != "completed" {
		t.Fatalf("complete result status = %q", got)
	}
	if got := creativeResultCompletionStatus(rules, candidates, complete[:2]); got != "partial" {
		t.Fatalf("missing variation status = %q", got)
	}
	missingSize := append([]creative.Variant(nil), complete...)
	missingSize[2].Assets = missingSize[2].Assets[:2]
	if got := creativeResultCompletionStatus(rules, candidates, missingSize); got != "partial" {
		t.Fatalf("missing size status = %q", got)
	}
}

func TestCreativeArchiveSourcePrefersVideoResource(t *testing.T) {
	got := creativeArchiveSource(creativeArchiveCandidate{
		AssetType:   "video",
		PreviewURL:  "preview.jpg",
		ResourceURL: "video.mp4",
	})
	if got != "video.mp4" {
		t.Fatalf("source = %q", got)
	}
}

func TestAbsoluteCreativeSourceURL(t *testing.T) {
	h := &Handler{cfg: Config{PublicURL: "https://fat-cybertron.adakamicorp.id/"}}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "relative upload",
			raw:  "/uploads/creative-materials/workspace/candidate/source.jpeg",
			want: "https://fat-cybertron.adakamicorp.id/uploads/creative-materials/workspace/candidate/source.jpeg",
		},
		{
			name: "absolute https",
			raw:  "https://cdn.example.com/source.jpeg",
			want: "https://cdn.example.com/source.jpeg",
		},
		{
			name: "local path fallback",
			raw:  "/data/inputs/source.jpeg",
			want: "/data/inputs/source.jpeg",
		},
		{
			name: "blank",
			raw:  " ",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.absoluteCreativeSourceURL(tc.raw); got != tc.want {
				t.Fatalf("absoluteCreativeSourceURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestPublicCreativeAssetURL(t *testing.T) {
	h := &Handler{cfg: Config{CreativeAssetPublicBaseURL: "https://fat-cybertron.adakamicorp.id/"}}
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "absolute lean file",
			raw:  "http://10.114.29.62:8010/files/lean-job/result.png?download=1",
			want: "https://fat-cybertron.adakamicorp.id/files/lean-job/result.png?download=1",
		},
		{
			name: "relative file",
			raw:  "/files/lean-job/result.png",
			want: "https://fat-cybertron.adakamicorp.id/files/lean-job/result.png",
		},
		{
			name: "non file URL remains untouched",
			raw:  "https://cdn.example.com/assets/result.png",
			want: "https://cdn.example.com/assets/result.png",
		},
		{
			name: "blank",
			raw:  " ",
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.publicCreativeAssetURL(tc.raw); got != tc.want {
				t.Fatalf("publicCreativeAssetURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestSafeCreativeZipPart(t *testing.T) {
	if got := safeCreativeZipPart("Kredit Pintar / ID"); got != "Kredit-Pintar-ID" {
		t.Fatalf("safeCreativeZipPart = %q", got)
	}
}

func TestCreativeDeliveryFilenamePreservesBusinessNamingConvention(t *testing.T) {
	asset := creativePackageAsset{
		AssetURL:    "http://127.0.0.1:8010/files/lean-1/07_P_AK_MY_20260716_NUM_AI02_11.png",
		ContentType: "image/png",
		Label:       "primary",
		Width:       1024,
		Height:      1024,
	}

	if got := creativeDeliveryFilename(asset); got != "07_P_AK_MY_20260716_NUM_AI02_11.png" {
		t.Fatalf("creativeDeliveryFilename = %q", got)
	}
}

func TestCreativeDeliveryFilenameKeepsLegacyPackageName(t *testing.T) {
	asset := creativePackageAsset{
		AssetURL:    "http://127.0.0.1:8010/files/lean-1/candidate_001.png",
		ContentType: "image/png",
		Label:       "primary",
		Width:       1024,
		Height:      1024,
	}

	if got := creativeDeliveryFilename(asset); got != "primary.png" {
		t.Fatalf("creativeDeliveryFilename = %q", got)
	}
}

func TestCreativePackageDownloadNameUsesBusinessNamingConvention(t *testing.T) {
	got := creativePackageDownloadName("07_P_AK_ID_20260716_NUM_AI02_11.png")
	if got != "07_P_AK_ID_20260716_NUM_AI_PACKAGE.zip" {
		t.Fatalf("creativePackageDownloadName = %q", got)
	}
}

func TestCreativePackageFallbackAssetNameNeverUsesDimensionsOnly(t *testing.T) {
	naming := creativePackageNamingFromJob(`{"market":"id-adakami"}`, "2026-07-16T19:11:01+08:00")
	got := creativePackageAssetFilename(creativePackageAsset{
		AssetURL:    "https://assets.example.test/1080x1080.png",
		ContentType: "image/png",
		Width:       1080,
		Height:      1080,
		Variant:     2,
	}, naming)
	if got != "07_P_AK_ID_20260716_NUM_AI02_11.png" {
		t.Fatalf("creativePackageAssetFilename = %q", got)
	}
}

func TestCreativeSourceContentTypeSupportsImageAndVideoOriginals(t *testing.T) {
	if got := creativeSourceContentType("image", "https://cdn.example/source.webp?token=redacted"); got != "image/webp" {
		t.Fatalf("image content type = %q", got)
	}
	if got := creativeSourceContentType("video", "https://cdn.example/download"); got != "video/mp4" {
		t.Fatalf("video content type = %q", got)
	}
	if got := creativePackageExtension("https://cdn.example/download", "video/mp4"); got != ".mp4" {
		t.Fatalf("video extension = %q", got)
	}
}

func TestCreativeMaterialsFromCrawlRawSkipsAppGrowingPageSnapshotsWhenExplicitResultsAreEmpty(t *testing.T) {
	raw := json.RawMessage(`{
		"connector_id": "appgrowing",
		"capability": "material_search",
		"selected_materials": [],
		"material_samples": [],
		"captured": [
			{
				"competitor": "Easycash",
				"url": "https://appgrowing-global.youcloud.com/leaflet?keyword=Easycash",
				"materials_found": 0,
				"page_snapshot": {
					"url": "https://appgrowing-global.youcloud.com/leaflet?keyword=Easycash",
					"title": "AppGrowing Global - YouCloud",
					"text": "Dashboard OVERVIEW Overview FAVORITES App Creatives"
				}
			}
		]
	}`)

	if got := creativeMaterialsFromCrawlRaw(raw); len(got) != 0 {
		t.Fatalf("materials length = %d, want 0; first = %#v", len(got), got[0])
	}
}

func TestCreativeMaterialsFromCrawlRawSkipsExplicitMaterialsWithoutUsableAsset(t *testing.T) {
	raw := json.RawMessage(`{
		"connector_id": "appgrowing",
		"capability": "material_search",
		"selected_materials": [
			{
				"material_id": "empty-material-1",
				"competitor": "Easycash",
				"title": "Empty AppGrowing card",
				"asset_type": "image"
			}
		]
	}`)

	if got := creativeMaterialsFromCrawlRaw(raw); len(got) != 0 {
		t.Fatalf("materials length = %d, want 0; first = %#v", len(got), got[0])
	}
}

func TestCrawlStrategyMemoryFromLearnedBrandWhitelistsFields(t *testing.T) {
	scopeKey, memory, ok := crawlStrategyMemoryFromLearnedStrategy(map[string]any{
		"strategy_type": "appgrowing_brand",
		"scope_key":     " Easycash ",
		"competitor":    "Easycash",
		"headers":       "should-not-be-stored",
		"cookie":        "should-not-be-stored",
		"value": map[string]any{
			"brand_id":         "brand-123",
			"brand_name":       "Easycash",
			"source":           "searchApp",
			"preferred_source": "graphql_api",
			"materials_found":  float64(3),
			"token":            "should-not-be-stored",
		},
	})
	if !ok {
		t.Fatalf("strategy was not accepted")
	}
	if scopeKey != "easycash" {
		t.Fatalf("scopeKey = %q", scopeKey)
	}
	if memory["brand_id"] != "brand-123" || memory["brand_name"] != "Easycash" || memory["preferred_source"] != "graphql_api" {
		t.Fatalf("memory = %#v", memory)
	}
	if _, exists := memory["headers"]; exists {
		t.Fatalf("headers leaked into memory: %#v", memory)
	}
	if _, exists := memory["token"]; exists {
		t.Fatalf("token leaked into memory: %#v", memory)
	}
}

func TestCrawlStrategyMemoryRejectsUnknownStrategyTypes(t *testing.T) {
	_, _, ok := crawlStrategyMemoryFromLearnedStrategy(map[string]any{
		"strategy_type": "raw_headers",
		"scope_key":     "easycash",
		"value": map[string]any{
			"brand_id": "brand-123",
		},
	})
	if ok {
		t.Fatalf("unknown strategy type should not be persisted")
	}
}

func TestCreativeMaterialsFromCrawlRawUsesExplicitSelectedMaterials(t *testing.T) {
	raw := json.RawMessage(`{
		"connector_id": "appgrowing",
		"capability": "material_search",
		"selected_materials": [
			{
				"material_id": "easycash-1",
				"title": "Easycash reference",
				"asset_type": "image",
				"preview_url": "https://cdn.example.com/easycash.jpg",
				"resource_url": "https://cdn.example.com/easycash.jpg"
			}
		],
		"captured": [
			{
				"url": "https://appgrowing-global.youcloud.com/leaflet?keyword=Easycash",
				"page_snapshot": {
					"url": "https://appgrowing-global.youcloud.com/leaflet?keyword=Easycash",
					"title": "AppGrowing Global - YouCloud"
				}
			}
		]
	}`)

	got := creativeMaterialsFromCrawlRaw(raw)
	if len(got) != 1 {
		t.Fatalf("materials length = %d, want 1: %#v", len(got), got)
	}
	if got[0].ResourceURL != "https://cdn.example.com/easycash.jpg" {
		t.Fatalf("resource_url = %q", got[0].ResourceURL)
	}
}

func TestCreativeDownloadPackageContainsOriginalThreeByThreeAndAuditManifest(t *testing.T) {
	ctx := context.Background()
	var issueID, candidateID, jobID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO issue (workspace_id, creator_type, creator_id, title)
VALUES ($1, 'member', $2, 'Creative package 3x3 test')
RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&issueID); err != nil {
		t.Fatalf("insert issue: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, dedupe_key, competitor, title, asset_type,
  preview_url, archived_url, archive_status
) VALUES ($1, 'appgrowing', $2, 'Easy Cash', 'Original reference', 'image',
          '/uploads/source/original.webp',
          '/uploads/source/original.webp', 'completed')
RETURNING id::text
`, testWorkspaceID, fmt.Sprintf("creative-package-%d", time.Now().UnixNano())).Scan(&candidateID); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_provider, external_job_id, external_status, stage, progress,
  poll_attempts, completed_at, process_data
) VALUES ($1, $2, 'completed', '保留核心构图，突出额度',
          '{"market":"idn-adakami","variant_count":3}'::jsonb,
          'member', $3, 'workspace_mcp', 'lean-package-test', 'completed',
          'completed', 100, 4, now(),
		  '{"schema_version":"1","quality_summary":{"pass_rate":1},"usage":{"token_usage":"not_available","cost":"not_available"}}'::jsonb)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&jobID); err != nil {
		t.Fatalf("insert creative job: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id)
VALUES ($1, $2)
`, jobID, candidateID); err != nil {
		t.Fatalf("insert job candidate: %v", err)
	}

	storage := &mockStorage{}
	storage.put("/uploads/source/original.webp", []byte("original-reference"))
	for variant := 1; variant <= 3; variant++ {
		var variantID string
		if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_variant (
  job_id, candidate_id, variant_index, title, description, qc_status
) VALUES ($1, $2, $3, $4, $5, 'passed')
RETURNING id::text
`, jobID, candidateID, variant, fmt.Sprintf("变体 %d", variant), "QC 通过").Scan(&variantID); err != nil {
			t.Fatalf("insert variant %d: %v", variant, err)
		}
		for _, size := range []struct {
			width, height int
			code          string
		}{
			{1080, 1080, "11"},
			{800, 1000, "45"},
			{1200, 628, "191"},
		} {
			filename := fmt.Sprintf("07_P_AK_ID_20260716_NUM_AI%02d_%s.png", variant, size.code)
			storageKey := "creative-results/" + filename
			storage.put(storageKey, []byte(filename))
			if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_asset (
  variant_id, width, height, label, asset_url, content_type, storage_key
) VALUES ($1, $2, $3, $4, $5, 'image/png', $6)
`, variantID, size.width, size.height, fmt.Sprintf("%dx%d", size.width, size.height),
				"https://cdn.example.com/"+storageKey, storageKey); err != nil {
				t.Fatalf("insert asset variant=%d size=%s: %v", variant, size.code, err)
			}
		}
	}

	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	originalStorage := testHandler.Storage
	testHandler.Storage = storage
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	recorder := httptest.NewRecorder()
	req := withURLParams(
		newRequest(http.MethodGet, "/api/issues/"+issueID+"/creative-edit-jobs/"+jobID+"/download", nil),
		"id", issueID,
		"jobId", jobID,
	)
	testHandler.DownloadCreativeEditJob(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("download status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/zip" {
		t.Fatalf("download content type = %q", got)
	}

	archive, err := testHandler.buildCreativeEditJobArchive(
		ctx, parseUUID(issueID), parseUUID(testWorkspaceID), parseUUID(jobID), nil, true,
	)
	if err != nil {
		t.Fatalf("build creative archive: %v", err)
	}
	if _, err := testPool.Exec(ctx, `UPDATE creative_edit_job SET status = 'partial' WHERE id = $1`, jobID); err != nil {
		t.Fatalf("mark creative job partial: %v", err)
	}
	archive, err = testHandler.buildCreativeEditJobArchive(
		ctx, parseUUID(issueID), parseUUID(testWorkspaceID), parseUUID(jobID), nil, true,
	)
	if err != nil {
		t.Fatalf("build partial creative archive: %v", err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatalf("open creative archive: %v", err)
	}
	if len(reader.File) != 11 {
		t.Fatalf("zip entries = %d, want original + 9 assets + manifest", len(reader.File))
	}

	entries := map[string]*zip.File{}
	for _, entry := range reader.File {
		entries[entry.Name] = entry
	}
	prefix := "Easy-Cash-" + shortCreativeID(candidateID)
	if _, ok := entries[prefix+"/original/original.webp"]; !ok {
		t.Fatalf("original entry missing: %#v", mapKeys(entries))
	}
	for variant := 1; variant <= 3; variant++ {
		for _, code := range []string{"11", "45", "191"} {
			filename := fmt.Sprintf("07_P_AK_ID_20260716_NUM_AI%02d_%s.png", variant, code)
			if _, ok := entries[fmt.Sprintf("%s/variant-%d/%s", prefix, variant, filename)]; !ok {
				t.Fatalf("generated entry missing for variant=%d code=%s", variant, code)
			}
		}
	}

	manifestEntry := entries["manifest.json"]
	if manifestEntry == nil {
		t.Fatal("manifest.json missing")
	}
	manifestReader, err := manifestEntry.Open()
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	manifestBytes, err := io.ReadAll(manifestReader)
	_ = manifestReader.Close()
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest["prompt"] != "保留核心构图，突出额度" {
		t.Fatalf("manifest prompt = %#v", manifest["prompt"])
	}
	if manifest["source_count"] != float64(1) || manifest["variant_count"] != float64(3) || manifest["asset_count"] != float64(9) {
		t.Fatalf("manifest counts = source:%v variant:%v asset:%v", manifest["source_count"], manifest["variant_count"], manifest["asset_count"])
	}
	process, _ := manifest["process"].(map[string]any)
	if process["stage"] != "completed" || process["poll_attempts"] != float64(4) {
		t.Fatalf("manifest process = %#v", process)
	}
	processData, _ := manifest["process_data"].(map[string]any)
	qualitySummary, _ := processData["quality_summary"].(map[string]any)
	if processData["schema_version"] != "1" || qualitySummary["pass_rate"] != float64(1) {
		t.Fatalf("manifest process_data = %#v", processData)
	}
	variants, _ := manifest["variants"].([]any)
	if len(variants) != 3 {
		t.Fatalf("manifest variants = %#v", variants)
	}
	for _, raw := range variants {
		variant, _ := raw.(map[string]any)
		if variant["qc_status"] != "passed" {
			t.Fatalf("manifest QC = %#v", variant)
		}
	}
	rules, _ := manifest["rules"].(map[string]any)
	if rules["market"] != "idn-adakami" || rules["variant_count"] != float64(3) {
		t.Fatalf("manifest rules = %#v", rules)
	}
}

func TestPreviewCreativeEditAssetStreamsScopedAsset(t *testing.T) {
	ctx := context.Background()
	issueID := createTestIssue(t, "Creative preview "+uuid.NewString(), "done", "medium")
	var candidateID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_material_candidate (
  workspace_id, dedupe_key, competitor, title, asset_type
) VALUES ($1::uuid, $2, 'Easy Cash', 'Reference', 'image')
RETURNING id::text
`, testWorkspaceID, uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}
	var jobID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_provider, external_job_id, external_status, stage, progress,
  poll_attempts, completed_at, process_data
) VALUES (
  $1::uuid, $2::uuid, 'completed', 'preview', '{}'::jsonb, 'member', $3::uuid,
  'workspace_mcp', 'lean-preview-test', 'completed', 'completed', 100, 1, now(), '{}'::jsonb
)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&jobID); err != nil {
		t.Fatalf("insert creative job: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id)
VALUES ($1::uuid, $2::uuid)
`, jobID, candidateID); err != nil {
		t.Fatalf("insert job candidate: %v", err)
	}
	var variantID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_variant (job_id, candidate_id, variant_index, title, qc_status)
VALUES ($1::uuid, $2::uuid, 1, 'Variant 1', 'passed')
RETURNING id::text
`, jobID, candidateID).Scan(&variantID); err != nil {
		t.Fatalf("insert variant: %v", err)
	}
	storageKey := "creative-results/preview.png"
	var assetID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_asset (
  variant_id, width, height, label, asset_url, content_type, storage_key
) VALUES (
  $1::uuid, 1080, 1080, '1080x1080', 'https://cdn.example.com/creative-results/preview.png',
  'image/png', $2
)
RETURNING id::text
`, variantID, storageKey).Scan(&assetID); err != nil {
		t.Fatalf("insert asset: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1::uuid`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_material_candidate WHERE id = $1::uuid`, candidateID)
	})

	storage := &mockStorage{}
	storage.put(storageKey, []byte("preview-png"))
	originalStorage := testHandler.Storage
	testHandler.Storage = storage
	t.Cleanup(func() { testHandler.Storage = originalStorage })

	recorder := httptest.NewRecorder()
	req := withURLParams(
		newRequest(http.MethodGet, "/api/issues/"+issueID+"/creative-edit-assets/"+assetID+"/preview", nil),
		"id", issueID,
		"assetId", assetID,
	)
	testHandler.PreviewCreativeEditAsset(recorder, req)

	resp := recorder.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, recorder.Body.String())
	}
	if got := resp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("content type = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if string(body) != "preview-png" {
		t.Fatalf("body = %q", body)
	}
}

func TestInsertCreativeEditResultsPersistsPublicAndSourceAssetURLs(t *testing.T) {
	ctx := context.Background()
	issueID := createTestIssue(t, "Creative asset public URL "+uuid.NewString(), "done", "medium")
	var candidateID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_material_candidate (
  workspace_id, dedupe_key, competitor, title, asset_type
) VALUES ($1::uuid, $2, 'Easy Cash', 'Reference', 'image')
RETURNING id::text
`, testWorkspaceID, uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatalf("insert candidate: %v", err)
	}
	var jobID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_provider, external_job_id, external_status, stage, progress,
  poll_attempts, process_data
) VALUES (
  $1::uuid, $2::uuid, 'running', 'public urls', '{}'::jsonb, 'member', $3::uuid,
  'workspace_mcp', 'lean-public-url-test', 'running', 'poll', 50, 0, '{}'::jsonb
)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&jobID); err != nil {
		t.Fatalf("insert creative job: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id)
VALUES ($1::uuid, $2::uuid)
`, jobID, candidateID); err != nil {
		t.Fatalf("insert job candidate: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, status)
VALUES ($1::uuid, $2::uuid, $3::uuid, 'sent_to_edit')
`, issueID, candidateID, testWorkspaceID); err != nil {
		t.Fatalf("insert issue candidate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1::uuid`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_material_candidate WHERE id = $1::uuid`, candidateID)
	})

	originalCfg := testHandler.cfg
	testHandler.cfg.CreativeAssetPublicBaseURL = "https://fat-cybertron.adakamicorp.id"
	t.Cleanup(func() { testHandler.cfg = originalCfg })

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback(ctx)
	sourceURL := "http://10.114.29.62:8010/files/lean-job/07_P_AK_ID_20260720_NUM_AI01_11.png"
	if err := testHandler.insertCreativeEditResults(
		ctx,
		tx,
		jobID,
		parseUUID(issueID),
		parseUUID(testWorkspaceID),
		[]creative.Candidate{{ID: candidateID}},
		[]creative.Variant{{
			CandidateID: candidateID,
			Index:       1,
			Title:       "Variant 1",
			QCStatus:    "passed",
			Assets: []creative.Asset{{
				Width:       1080,
				Height:      1080,
				Label:       "1080x1080",
				URL:         sourceURL,
				ContentType: "image/png",
			}},
		}},
	); err != nil {
		t.Fatalf("insert results: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	var assetURL string
	var sourceAssetURL string
	if err := testPool.QueryRow(ctx, `
SELECT a.asset_url, a.source_asset_url
FROM creative_edit_asset a
JOIN creative_edit_variant v ON v.id = a.variant_id
WHERE v.job_id = $1::uuid
`, jobID).Scan(&assetURL, &sourceAssetURL); err != nil {
		t.Fatalf("load asset URLs: %v", err)
	}
	if assetURL != "https://fat-cybertron.adakamicorp.id/files/lean-job/07_P_AK_ID_20260720_NUM_AI01_11.png" {
		t.Fatalf("asset_url = %q", assetURL)
	}
	if sourceAssetURL != sourceURL {
		t.Fatalf("source_asset_url = %q", sourceAssetURL)
	}
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

type terminalProcessUpgradeProvider struct {
	result      creative.GetJobResult
	err         error
	getCalls    int
	lastRequest creative.GetJobRequest
}

func (p *terminalProcessUpgradeProvider) Name() string { return "terminal-upgrade-test" }

func (p *terminalProcessUpgradeProvider) CreateJob(context.Context, creative.CreateJobRequest) (creative.CreateJobResult, error) {
	return creative.CreateJobResult{}, errors.New("CreateJob must not run for a terminal process_data upgrade")
}

func (p *terminalProcessUpgradeProvider) GetJob(_ context.Context, request creative.GetJobRequest) (creative.GetJobResult, error) {
	p.getCalls++
	p.lastRequest = request
	return p.result, p.err
}

type terminalProcessUpgradeResolver struct {
	provider     creative.Provider
	resolveCalls int
}

func (r *terminalProcessUpgradeResolver) ResolveDefault(context.Context, string) (creative.ResolvedProvider, error) {
	return creative.ResolvedProvider{}, errors.New("ResolveDefault must not run")
}

func (r *terminalProcessUpgradeResolver) Resolve(_ context.Context, _, _ string) (creative.ResolvedProvider, error) {
	r.resolveCalls++
	if r.provider == nil {
		return creative.ResolvedProvider{}, errors.New("provider unavailable")
	}
	return creative.ResolvedProvider{Provider: r.provider}, nil
}

type terminalCreativeJobFixture struct {
	issueID     string
	jobID       string
	candidateID string
	variantID   string
	completedAt time.Time
}

func seedTerminalCreativeJob(t *testing.T, status, schemaVersion string) terminalCreativeJobFixture {
	t.Helper()
	ctx := context.Background()
	issueID := createTestIssue(t, "Terminal creative telemetry "+uuid.NewString(), "done", "medium")
	var candidateID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_material_candidate (workspace_id, dedupe_key, competitor, title, asset_type)
VALUES ($1::uuid, $2, 'Legacy competitor', 'Legacy reference', 'image')
RETURNING id::text
`, testWorkspaceID, uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatalf("create terminal candidate: %v", err)
	}
	processData := fmt.Sprintf(`{"schema_version":%q,"legacy_marker":"keep-until-upgraded"}`, schemaVersion)
	var jobID string
	var completedAt time.Time
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_provider, external_job_id, external_status, stage, progress,
  last_poll_at, next_poll_at, completed_at, error_message, poll_attempts, process_data
) VALUES (
  $1::uuid, $2::uuid, $3, 'original prompt', '{"market":"idn-adakami"}'::jsonb,
  'member', $4::uuid, 'workspace_mcp', 'legacy-external-job', $3,
  'original-terminal-stage', 100, now() - interval '2 minutes', NULL,
  now() - interval '1 minute', 'original terminal message', 7, $5::jsonb
)
RETURNING id::text, completed_at
`, testWorkspaceID, issueID, status, testUserID, processData).Scan(&jobID, &completedAt); err != nil {
		t.Fatalf("create terminal creative job: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id) VALUES ($1::uuid, $2::uuid)
`, jobID, candidateID); err != nil {
		t.Fatalf("link terminal creative job candidate: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, status)
VALUES ($1::uuid, $2::uuid, $3::uuid, 'approved')
`, issueID, candidateID, testWorkspaceID); err != nil {
		t.Fatalf("link terminal creative issue candidate: %v", err)
	}
	var variantID string
	if err := testPool.QueryRow(ctx, `
INSERT INTO creative_edit_variant (job_id, candidate_id, variant_index, title, qc_status)
VALUES ($1::uuid, $2::uuid, 1, 'Original variant', 'passed')
RETURNING id::text
`, jobID, candidateID).Scan(&variantID); err != nil {
		t.Fatalf("create terminal variant: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_asset (variant_id, width, height, label, asset_url, content_type)
VALUES ($1::uuid, 1080, 1080, '1080x1080', 'https://assets.example.test/original.png', 'image/png')
`, variantID); err != nil {
		t.Fatalf("create terminal asset: %v", err)
	}
	if _, err := testPool.Exec(ctx, `
INSERT INTO creative_edit_feedback (
  workspace_id, issue_id, job_id, candidate_id, variant_id, decision,
  reason_codes, suggestion, created_by, created_by_name
) VALUES (
  $2::uuid, $3::uuid, $4::uuid, $5::uuid, $1::uuid, 'accepted',
  ARRAY['ready_to_publish'], 'Original feedback', $6::uuid, 'Test User'
)
`, variantID, testWorkspaceID, issueID, jobID, candidateID, testUserID); err != nil {
		t.Fatalf("create terminal feedback: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1::uuid`, issueID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM creative_material_candidate WHERE id = $1::uuid`, candidateID)
	})
	return terminalCreativeJobFixture{
		issueID: issueID, jobID: jobID, candidateID: candidateID,
		variantID: variantID, completedAt: completedAt,
	}
}

func withTerminalUpgradeResolver(t *testing.T, resolver creative.ProviderResolver) {
	t.Helper()
	original := testHandler.CreativeProviderResolver
	testHandler.CreativeProviderResolver = resolver
	t.Cleanup(func() { testHandler.CreativeProviderResolver = original })
}

func TestReconcileLegacyTerminalCreativeJobOnlyUpgradesProcessData(t *testing.T) {
	for _, localStatus := range []string{"completed", "failed"} {
		t.Run(localStatus, func(t *testing.T) {
			fixture := seedTerminalCreativeJob(t, localStatus, "1")
			provider := &terminalProcessUpgradeProvider{result: creative.GetJobResult{
				Status: "completed",
				Stage:  "provider-stage-must-not-overwrite",
				ProcessData: json.RawMessage(`{
                  "schema_version":"2",
                  "candidates":[{"candidate_id":"candidate","submissions":[]}]
                }`),
				Variants: []creative.Variant{{
					CandidateID: fixture.candidateID,
					Index:       2,
					Title:       "Must not be inserted",
				}},
			}}
			resolver := &terminalProcessUpgradeResolver{provider: provider}
			withTerminalUpgradeResolver(t, resolver)

			for call := 0; call < 2; call++ {
				if err := testHandler.reconcileCreativeEditJob(
					context.Background(), parseUUID(fixture.issueID),
					parseUUID(testWorkspaceID), parseUUID(fixture.jobID),
				); err != nil {
					t.Fatalf("reconcile terminal creative job call %d: %v", call+1, err)
				}
			}
			if provider.getCalls != 1 || resolver.resolveCalls != 1 {
				t.Fatalf("provider calls: get=%d resolve=%d, want one each", provider.getCalls, resolver.resolveCalls)
			}
			if len(provider.lastRequest.Candidates) != 1 || provider.lastRequest.ExternalJobID != "legacy-external-job" {
				t.Fatalf("GetJob request = %#v", provider.lastRequest)
			}

			var gotStatus, externalStatus, stage, errorMessage, processData, candidateStatus string
			var progress, pollAttempts, variants, assets, feedback int
			var completedAt time.Time
			if err := testPool.QueryRow(context.Background(), `
SELECT status, external_status, stage, progress, error_message, poll_attempts,
       completed_at, process_data::text,
       (SELECT count(*) FROM creative_edit_variant WHERE job_id = j.id),
       (SELECT count(*) FROM creative_edit_asset a JOIN creative_edit_variant v ON v.id = a.variant_id WHERE v.job_id = j.id),
       (SELECT count(*) FROM creative_edit_feedback f WHERE f.job_id = j.id),
       (SELECT status FROM creative_material_issue_candidate WHERE issue_id = j.issue_id AND candidate_id = $2::uuid)
FROM creative_edit_job j
WHERE id = $1::uuid
`, fixture.jobID, fixture.candidateID).Scan(
				&gotStatus, &externalStatus, &stage, &progress, &errorMessage, &pollAttempts,
				&completedAt, &processData, &variants, &assets, &feedback, &candidateStatus,
			); err != nil {
				t.Fatalf("load upgraded terminal job: %v", err)
			}
			if gotStatus != localStatus || externalStatus != localStatus || stage != "original-terminal-stage" ||
				progress != 100 || errorMessage != "original terminal message" || pollAttempts != 7 {
				t.Fatalf("terminal fields changed: status=%s external=%s stage=%s progress=%d error=%q polls=%d",
					gotStatus, externalStatus, stage, progress, errorMessage, pollAttempts)
			}
			if !completedAt.Equal(fixture.completedAt) {
				t.Fatalf("completed_at changed: got %s want %s", completedAt, fixture.completedAt)
			}
			if creativeProcessDataSchemaVersion(processData) != 2 {
				t.Fatalf("process_data = %s", processData)
			}
			if variants != 1 || assets != 1 || feedback != 1 || candidateStatus != "approved" {
				t.Fatalf("related state changed: variants=%d assets=%d feedback=%d candidate=%s",
					variants, assets, feedback, candidateStatus)
			}
		})
	}
}

func TestReconcileCurrentTerminalCreativeJobUsesFastPath(t *testing.T) {
	for _, localStatus := range []string{"completed", "failed"} {
		t.Run(localStatus, func(t *testing.T) {
			fixture := seedTerminalCreativeJob(t, localStatus, "2")
			resolver := &terminalProcessUpgradeResolver{}
			withTerminalUpgradeResolver(t, resolver)

			if err := testHandler.reconcileCreativeEditJob(
				context.Background(), parseUUID(fixture.issueID),
				parseUUID(testWorkspaceID), parseUUID(fixture.jobID),
			); err != nil {
				t.Fatalf("reconcile current terminal job: %v", err)
			}
			if resolver.resolveCalls != 0 {
				t.Fatalf("current terminal job resolved provider %d times", resolver.resolveCalls)
			}
		})
	}
}

func TestReconcileLegacyTerminalCreativeJobPreservesDataOnProviderFailure(t *testing.T) {
	fixture := seedTerminalCreativeJob(t, "completed", "1")
	provider := &terminalProcessUpgradeProvider{err: errors.New("upstream telemetry unavailable")}
	withTerminalUpgradeResolver(t, &terminalProcessUpgradeResolver{provider: provider})

	err := testHandler.reconcileCreativeEditJob(
		context.Background(), parseUUID(fixture.issueID),
		parseUUID(testWorkspaceID), parseUUID(fixture.jobID),
	)
	if err == nil || !strings.Contains(err.Error(), "upgrade terminal creative edit job") ||
		!strings.Contains(err.Error(), creative.GetJobToolName) {
		t.Fatalf("diagnostic error = %v", err)
	}
	var status, processData string
	var pollAttempts int
	if queryErr := testPool.QueryRow(context.Background(), `
SELECT status, poll_attempts, process_data::text FROM creative_edit_job WHERE id = $1::uuid
`, fixture.jobID).Scan(&status, &pollAttempts, &processData); queryErr != nil {
		t.Fatalf("load failed terminal upgrade: %v", queryErr)
	}
	if status != "completed" || pollAttempts != 7 || creativeProcessDataSchemaVersion(processData) != 1 {
		t.Fatalf("failed upgrade mutated job: status=%s polls=%d process=%s", status, pollAttempts, processData)
	}
}
