package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreativeMaterialsFromCrawlRawPreservesAppGrowingBusinessMetadata(t *testing.T) {
	materials := creativeMaterialsFromCrawlRaw(json.RawMessage(`{
		"selected_materials": [{
			"materialId": "material-metadata-1",
			"brandName": "AdaKami",
			"title": "Flexible loan",
			"assetType": "image",
			"resourceUrl": "https://cdn.example.com/material-metadata-1.jpg",
			"duration_days": "",
			"deliveryDays": "35.5",
			"impressionEstimate": null,
			"impression_inc_2y": "12,000,000",
			"media_names": [],
			"mediaIds": [4, 9],
			"language_names": [],
			"languageCodes": ["id"],
			"platformNames": [],
			"platform": [{"id": 2, "name": "Android"}],
			"tags": [],
			"tagNames": ["finance", "installment"],
			"note": " ",
			"remarks": "Keep the original disclaimer"
		}]
	}`))
	if len(materials) != 1 {
		t.Fatalf("materials = %d, want 1", len(materials))
	}
	material := materials[0]
	if material.ExternalID != "material-metadata-1" || material.Competitor != "AdaKami" {
		t.Fatalf("identity metadata = %#v", material)
	}
	if material.DurationDays == nil || *material.DurationDays != 35.5 {
		t.Fatalf("duration_days = %v, want 35.5", material.DurationDays)
	}
	if material.ImpressionEstimate == nil || *material.ImpressionEstimate != 12_000_000 {
		t.Fatalf("impression_estimate = %v, want 12000000", material.ImpressionEstimate)
	}
	if !reflect.DeepEqual(material.MediaNames, []string{"4", "9"}) {
		t.Fatalf("media_names = %#v, want ID fallback", material.MediaNames)
	}
	if !reflect.DeepEqual(material.LanguageNames, []string{"id"}) {
		t.Fatalf("language_names = %#v, want code fallback", material.LanguageNames)
	}
	if !reflect.DeepEqual(material.PlatformNames, []string{"Android"}) {
		t.Fatalf("platform_names = %#v", material.PlatformNames)
	}
	if !reflect.DeepEqual(material.Tags, []string{"finance", "installment"}) {
		t.Fatalf("tags = %#v", material.Tags)
	}
	if material.Note != "Keep the original disclaimer" {
		t.Fatalf("note = %q", material.Note)
	}
}

func TestCreativeMaterialDedupeUsesStableAssetURL(t *testing.T) {
	first := creativeMaterialDedupeKey(creativeMaterialInput{
		ExternalID:  "material-1",
		ResourceURL: "https://cdn.example.com/a.jpg?auth_key=first",
	})
	second := creativeMaterialDedupeKey(creativeMaterialInput{
		ExternalID:  "material-1",
		ResourceURL: "https://cdn.example.com/a.jpg?auth_key=second",
	})
	sibling := creativeMaterialDedupeKey(creativeMaterialInput{
		ExternalID:  "material-1",
		ResourceURL: "https://cdn.example.com/b.jpg?auth_key=first",
	})
	if first != second {
		t.Fatalf("temporary auth parameters changed dedupe key: %q != %q", first, second)
	}
	if first == sibling {
		t.Fatal("different assets under one AppGrowing material were collapsed")
	}
}

func TestCreativeMaterialPrivateArchiveUsesAuthenticatedProxyRoute(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	store := &mockStorageNoCdn{}
	store.put("creative-materials/private/source.png", []byte("image"))
	handler := *testHandler
	handler.Storage = store

	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, dedupe_key, title, asset_type, archived_url, archive_status, raw
) VALUES ($1, 'test', $2, 'Private archive', 'image', $3, 'completed', '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, "private-archive-"+testWorkspaceID,
		"https://cdn.example.com/creative-materials/private/source.png").Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})

	proxyURL := handler.creativeMaterialArchiveResponseURL(candidateID, "https://cdn.example.com/creative-materials/private/source.png")
	if proxyURL != "/api/creative/materials/"+candidateID+"/archive" {
		t.Fatalf("private archive URL = %q", proxyURL)
	}
	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodGet, proxyURL, nil), "id", candidateID)
	handler.DownloadCreativeMaterialArchive(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("private archive download = %d %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != "image" {
		t.Fatalf("proxied archive body = %q", body)
	}
}

func TestUniqueCreativeMaterialInputsCountsFinalAssets(t *testing.T) {
	inputs := []creativeMaterialInput{
		{ResourceURL: "https://cdn.example.com/a.jpg?auth_key=first", AssetType: "image"},
		{ResourceURL: "https://cdn.example.com/a.jpg?auth_key=second", AssetType: "image"},
		{ResourceURL: "https://cdn.example.com/b.jpg", AssetType: "image"},
		{},
	}
	materials, skipped := uniqueCreativeMaterialInputs(inputs)
	if len(materials) != 2 || skipped != 2 {
		t.Fatalf("unique inputs = %d skipped = %d, want 2 and 2", len(materials), skipped)
	}
}

func TestImportCreativeMaterialsPersistsCapturedMetadata(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	dedupeKey := "captured-metadata-" + testWorkspaceID
	duration := 35.5
	impression := int64(12_000_000)
	input := creativeMaterialImportInput{
		WorkspaceID:  parseUUID(testWorkspaceID),
		ConnectorID:  "appgrowing",
		QuerySummary: "captured metadata",
		Materials: []creativeMaterialInput{{
			DedupeKey:          dedupeKey,
			Title:              "Captured metadata material",
			AssetType:          "image",
			PreviewURL:         "https://example.test/captured-metadata.png",
			DurationDays:       &duration,
			ImpressionEstimate: &impression,
			MediaNames:         []string{"Google Ads"},
			LanguageNames:      []string{"id"},
			Tags:               []string{"finance", "installment"},
			Note:               "Keep the original disclaimer",
		}},
		ActorType: "member",
		ActorID:   testUserID,
		UserID:    parseUUID(testUserID),
	}
	if _, err := testHandler.importCreativeMaterials(t.Context(), input); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE workspace_id = $1 AND connector_id = 'appgrowing' AND dedupe_key = $2`, testWorkspaceID, dedupeKey)
	})

	// A later page can omit metadata. The upsert must retain values captured earlier.
	input.Materials[0].DurationDays = nil
	input.Materials[0].ImpressionEstimate = nil
	input.Materials[0].MediaNames = nil
	input.Materials[0].LanguageNames = nil
	input.Materials[0].Tags = nil
	input.Materials[0].Note = ""
	if _, err := testHandler.importCreativeMaterials(t.Context(), input); err != nil {
		t.Fatal(err)
	}

	var storedDuration float64
	var storedImpression int64
	var storedMedia, storedLanguages, storedTags []string
	var storedNote string
	if err := testPool.QueryRow(t.Context(), `
SELECT duration_days, impression_estimate, media_names, language_names, tags, note
FROM creative_material_candidate
WHERE workspace_id = $1 AND connector_id = 'appgrowing' AND dedupe_key = $2
`, testWorkspaceID, dedupeKey).Scan(
		&storedDuration, &storedImpression, &storedMedia, &storedLanguages, &storedTags, &storedNote,
	); err != nil {
		t.Fatal(err)
	}
	if storedDuration != duration || storedImpression != impression {
		t.Fatalf("numeric metadata = %v/%v, want %v/%v", storedDuration, storedImpression, duration, impression)
	}
	if !reflect.DeepEqual(storedMedia, []string{"Google Ads"}) || !reflect.DeepEqual(storedLanguages, []string{"id"}) {
		t.Fatalf("source dimensions = media %#v language %#v", storedMedia, storedLanguages)
	}
	if !reflect.DeepEqual(storedTags, []string{"finance", "installment"}) || storedNote != "Keep the original disclaimer" {
		t.Fatalf("user metadata = tags %#v note %q", storedTags, storedNote)
	}
}

func TestUpdateCreativeMaterialCandidateCanRejectLibraryAsset(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := createCreativeDeliveryTestIssue(t, "Reject library asset", "")
	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, dedupe_key, competitor, title, asset_type,
  preview_url, resource_url, raw
) VALUES ($1, 'appgrowing', $2, 'Easycash', 'Historical asset', 'image',
  'https://example.test/history.png', 'https://example.test/history.png', '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, "reject-library-"+issueID).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPatch, "/api/issues/"+issueID+"/creative-materials/"+candidateID, map[string]any{
		"status": "rejected",
	})
	req = withURLParams(req, "id", issueID, "candidateId", candidateID)
	testHandler.UpdateCreativeMaterialCandidate(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("UpdateCreativeMaterialCandidate: %d %s", w.Code, w.Body.String())
	}

	var status string
	if err := testPool.QueryRow(t.Context(), `
SELECT status FROM creative_material_issue_candidate
WHERE issue_id = $1 AND candidate_id = $2
`, issueID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("status = %q, want rejected", status)
	}
}

func TestWorkspaceLibraryImportReusesArchivedCandidate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := createCreativeDeliveryTestIssue(t, "Creative library reuse", "")
	var sourceCandidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, dedupe_key, competitor, title, asset_type,
  preview_url, resource_url, archived_url, archive_status, raw
) VALUES ($1, 'appgrowing', $2, 'Easycash', 'Archived source', 'image',
  'https://example.test/source.png', 'https://example.test/source.png',
  '/uploads/creative-materials/source/source.png', 'completed', '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, "library-source-"+issueID).Scan(&sourceCandidateID); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/issues/"+issueID+"/creative-materials/import", map[string]any{
		"connector_id":  "workspace-library",
		"query_summary": "library reuse test",
		"materials": []map[string]any{{
			"dedupe_key":  "copied-" + issueID,
			"competitor":  "Easycash",
			"title":       "Archived source",
			"asset_type":  "image",
			"preview_url": "http://localhost:8080/uploads/creative-materials/source/source.png",
			"raw":         map[string]any{"source_candidate_id": sourceCandidateID},
		}},
	})
	req = withURLParam(req, "id", issueID)
	testHandler.ImportCreativeMaterials(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("ImportCreativeMaterials: %d %s", w.Code, w.Body.String())
	}
	var summary creativeImportSummary
	if err := json.NewDecoder(w.Body).Decode(&summary); err != nil {
		t.Fatal(err)
	}
	if summary.ExistingCount != 1 || summary.ImportedCount != 0 {
		t.Fatalf("unexpected import summary: %#v", summary)
	}

	var linkedCandidateID, archiveStatus, archivedURL string
	if err := testPool.QueryRow(t.Context(), `
SELECT c.id::text, c.archive_status, c.archived_url
FROM creative_material_issue_candidate ic
JOIN creative_material_candidate c ON c.id = ic.candidate_id
WHERE ic.issue_id = $1 AND ic.workspace_id = $2
`, issueID, testWorkspaceID).Scan(&linkedCandidateID, &archiveStatus, &archivedURL); err != nil {
		t.Fatal(err)
	}
	if linkedCandidateID != sourceCandidateID {
		t.Fatalf("linked candidate = %s, want source %s", linkedCandidateID, sourceCandidateID)
	}
	if archiveStatus != "completed" || archivedURL == "" {
		t.Fatalf("archive state = %s %q, want completed archived source", archiveStatus, archivedURL)
	}
}

func TestWorkspaceLibrarySourceCandidateIDRejectsInvalidValues(t *testing.T) {
	if got := workspaceLibrarySourceCandidateID(json.RawMessage(`{"source_candidate_id":"not-a-uuid"}`)); got != "" {
		t.Fatalf("invalid source candidate accepted: %q", got)
	}
}

func TestCreativeCrawlRunResponseIncludesLifecycleAndDerivedMetrics(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := createCreativeDeliveryTestIssue(t, "Crawl run metrics", "")
	var runID, candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_crawl_run (
  workspace_id, issue_id, connector_id, status, autopilot_run_id, started_at, finished_at,
  error_code, error_message, created_by_type, created_by_id
) VALUES ($1, $2, 'test', 'partial', NULL, now() - interval '1 minute', now(),
  'analysis_partial', 'one analysis failed', 'member', $3)
RETURNING id::text`, testWorkspaceID, issueID, testUserID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, 'Run candidate', 'image', 'https://example.test/run.png', '{}'::jsonb)
RETURNING id::text`, testWorkspaceID, "run-metrics-"+issueID).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, source_run_id, status, analysis_status)
VALUES ($1, $2, $3, $4, 'selected', 'completed')`, issueID, candidateID, testWorkspaceID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_material_crawl_run_candidate (run_id, candidate_id, workspace_id, is_new_in_run, analysis_status)
VALUES ($1, $2, $3, true, 'completed')`, runID, candidateID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	runs, err := testHandler.listCreativeCrawlRuns(t.Context(), parseUUID(issueID), parseUUID(testWorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 0 {
		t.Fatal("crawl run missing")
	}
	var run *creativeMaterialCrawlRunResponse
	for index := range runs {
		if runs[index].ID == runID {
			run = &runs[index]
			break
		}
	}
	if run == nil {
		t.Fatal("created crawl run missing")
	}
	if run.Status != "partial" || run.ErrorCode != "analysis_partial" || run.StartedAt == "" || run.FinishedAt == "" {
		t.Fatalf("lifecycle response = %#v", run)
	}
	if run.CandidateMetrics.Total != 1 || run.CandidateMetrics.Analyzed != 1 || run.CandidateMetrics.Selected != 1 || run.CandidateMetrics.Rejected != 0 {
		t.Fatalf("candidate metrics = %#v", run.CandidateMetrics)
	}
}

func TestCreativeCrawlRunWithoutIssueCreatesRunCandidateRelations(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	summary, err := testHandler.importCreativeMaterials(t.Context(), creativeMaterialImportInput{
		WorkspaceID:  parseUUID(testWorkspaceID),
		ConnectorID:  "test",
		QuerySummary: "run-only crawl",
		Materials: []creativeMaterialInput{{
			DedupeKey:  "run-only-" + testWorkspaceID,
			Title:      "Run-only material",
			AssetType:  "image",
			PreviewURL: "https://example.test/run-only-material.png",
		}},
		ActorType: "member",
		ActorID:   testUserID,
		UserID:    parseUUID(testUserID),
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunID == "" || summary.TotalCount != 1 {
		t.Fatalf("run-only import summary = %#v", summary)
	}
	var issueID string
	if err := testPool.QueryRow(t.Context(), `SELECT COALESCE(issue_id::text, '') FROM creative_material_crawl_run WHERE id = $1`, summary.RunID).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	if issueID != "" {
		t.Fatalf("run-only crawl issue_id = %q, want empty", issueID)
	}
	var linkedCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_material_crawl_run_candidate WHERE run_id = $1`, summary.RunID).Scan(&linkedCount); err != nil {
		t.Fatal(err)
	}
	if linkedCount != 1 {
		t.Fatalf("run candidate links = %d, want 1", linkedCount)
	}
	w := httptest.NewRecorder()
	testHandler.ListCreativeCrawlRuns(w, newRequest(http.MethodGet, "/api/creative/crawl-runs", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeCrawlRuns: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		CrawlRuns []creativeMaterialCrawlRunResponse `json:"crawl_runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, run := range response.CrawlRuns {
		if run.ID == summary.RunID && run.IssueID == "" && run.CandidateMetrics.Total == 1 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("run-only crawl not returned with derived metrics: %#v", response.CrawlRuns)
	}
	w = httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, newRequest(http.MethodGet, "/api/creative/materials", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary: %d %s", w.Code, w.Body.String())
	}
	var library struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
		CrawlRuns  []creativeMaterialCrawlRunResponse  `json:"crawl_runs"`
	}
	if err := json.NewDecoder(w.Body).Decode(&library); err != nil {
		t.Fatal(err)
	}
	runFound := false
	for _, run := range library.CrawlRuns {
		if run.ID == summary.RunID && run.IssueID == "" && run.CandidateMetrics.Total == 1 {
			runFound = true
			break
		}
	}
	if !runFound {
		t.Fatalf("run-only crawl missing from material library: %#v", library.CrawlRuns)
	}
	for _, candidate := range library.Candidates {
		if candidate.SourceRunID != summary.RunID {
			continue
		}
		if !candidate.IsNewInRun || candidate.AnalysisStatus != "pending" || candidate.AnalysisError != "" {
			t.Fatalf("run-only candidate relation = %#v", candidate)
		}
		return
	}
	t.Fatalf("run-only candidate missing from library: %#v", library.Candidates)
}

func TestCreativeMaterialLibraryRunFilterPreservesHistoricalRelation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	dedupeKey := "historical-run-filter-" + testWorkspaceID
	importOnce := func() creativeImportSummary {
		summary, err := testHandler.importCreativeMaterials(t.Context(), creativeMaterialImportInput{
			WorkspaceID: parseUUID(testWorkspaceID), ConnectorID: "test", QuerySummary: "historical run filter",
			Materials: []creativeMaterialInput{{DedupeKey: dedupeKey, Title: "Historical run candidate", AssetType: "image", PreviewURL: "https://example.test/historical-run.png"}},
			ActorType: "member", ActorID: testUserID, UserID: parseUUID(testUserID),
		})
		if err != nil {
			t.Fatal(err)
		}
		return summary
	}
	first := importOnce()
	second := importOnce()
	if first.ImportedCount != 1 || second.ExistingCount != 1 {
		t.Fatalf("run import counts = first %#v second %#v", first, second)
	}

	w := httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, newRequest(http.MethodGet, "/api/creative/materials?run_id="+first.RunID, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary: %d %s", w.Code, w.Body.String())
	}
	var library struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
	}
	if err := json.NewDecoder(w.Body).Decode(&library); err != nil {
		t.Fatal(err)
	}
	if len(library.Candidates) != 1 || library.Candidates[0].SourceRunID != first.RunID || !library.Candidates[0].IsNewInRun {
		t.Fatalf("historical run candidates = %#v", library.Candidates)
	}
}

func TestCreativeMaterialLibraryPaginationAndFilter(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	competitor := "pagination-filter-" + uuid.NewString()
	summary, err := testHandler.importCreativeMaterials(t.Context(), creativeMaterialImportInput{
		WorkspaceID: parseUUID(testWorkspaceID), ConnectorID: "test", QuerySummary: "pagination filter",
		Materials: []creativeMaterialInput{
			{DedupeKey: "pagination-first-" + competitor, Title: "First pagination candidate", Competitor: competitor, AssetType: "image", PreviewURL: "https://example.test/pagination-first.png"},
			{DedupeKey: "pagination-second-" + competitor, Title: "Second pagination candidate", Competitor: competitor, AssetType: "image", PreviewURL: "https://example.test/pagination-second.png"},
		},
		ActorType: "member", ActorID: testUserID, UserID: parseUUID(testUserID),
	})
	if err != nil || summary.ImportedCount != 2 {
		t.Fatalf("import pagination candidates: summary=%#v err=%v", summary, err)
	}

	request := newRequest(http.MethodGet, "/api/creative/materials?competitor="+url.QueryEscape(competitor)+"&limit=1&offset=0", nil)
	w := httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary page 1: %d %s", w.Code, w.Body.String())
	}
	var firstPage struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
		TotalCount int                                 `json:"total_count"`
		NextOffset *int                                `json:"next_offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&firstPage); err != nil {
		t.Fatal(err)
	}
	if len(firstPage.Candidates) != 1 || firstPage.TotalCount != 2 || firstPage.NextOffset == nil || *firstPage.NextOffset != 1 {
		t.Fatalf("first page = %#v", firstPage)
	}

	w = httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, newRequest(http.MethodGet, "/api/creative/materials?competitor="+url.QueryEscape(competitor)+"&limit=1&offset=1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary page 2: %d %s", w.Code, w.Body.String())
	}
	var secondPage struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
		TotalCount int                                 `json:"total_count"`
		NextOffset *int                                `json:"next_offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&secondPage); err != nil {
		t.Fatal(err)
	}
	if len(secondPage.Candidates) != 1 || secondPage.TotalCount != 2 || secondPage.NextOffset != nil {
		t.Fatalf("second page = %#v", secondPage)
	}

	w = httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, newRequest(http.MethodGet, "/api/creative/materials?sort=unrecognized", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid sort status = %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeMaterialLibraryAvailableViewRequiresCurrentPreAdaptation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	workspaceID := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO workspace (id, name, slug, description, issue_prefix)
VALUES ($1, 'Available material filter', $2, '', 'AMF')
`, workspaceID, "available-material-filter-"+workspaceID[:8]); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO member (workspace_id, user_id, role)
VALUES ($1, $2, 'owner')
`, workspaceID, testUserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM workspace WHERE id = $1`, workspaceID) })

	copyLibraryID := uuid.NewString()
	marketPackID := uuid.NewString()
	marketConfig, err := json.Marshal(map[string]any{
		"pre_adaptation_default": true,
		"copy_library_id":        copyLibraryID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource (id, workspace_id, kind, name, description, status, version, published_version, config, created_by)
VALUES ($1, $2, 'copy_library', 'Available copy library', '', 'published', 1, 1, $3::jsonb, $4)
`, copyLibraryID, workspaceID, validComposableCopyLibraryJSON, testUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1, 1, 'Available copy library', '', $2::jsonb, $3)
`, copyLibraryID, validComposableCopyLibraryJSON, testUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource (id, workspace_id, kind, name, description, status, version, published_version, config, created_by)
VALUES ($1, $2, 'market_pack', 'Available market pack', '', 'published', 1, 1, $3::jsonb, $4)
`, marketPackID, workspaceID, marketConfig, testUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1, 1, 'Available market pack', '', $2::jsonb, $3)
`, marketPackID, marketConfig, testUserID); err != nil {
		t.Fatal(err)
	}

	availableCandidateID := uuid.NewString()
	analyzingCandidateID := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_material_candidate (id, workspace_id, connector_id, dedupe_key, title, asset_type, archived_url, archive_status, raw)
VALUES
  ($1, $3, 'test', $4, 'Ready material', 'image', '/uploads/ready.png', 'completed', '{}'::jsonb),
  ($2, $3, 'test', $5, 'Missing copy material', 'image', '/uploads/missing.png', 'completed', '{}'::jsonb)
`, availableCandidateID, analyzingCandidateID, workspaceID, "ready-"+availableCandidateID, "missing-"+analyzingCandidateID); err != nil {
		t.Fatal(err)
	}
	availableAnalysis, err := json.Marshal(map[string]any{
		"text_blocks": []map[string]any{{
			"id": "headline", "location": "Top", "role": "headline", "source_text": "Old",
			"visual_bounds": map[string]any{"x": 10, "y": 10, "width": 100, "height": 40},
		}},
		"visual_regions": []map[string]any{{
			"id": "headline-region", "location": "Top", "kind": "copy", "source_block_ids": []string{"headline"},
			"visual_bounds": map[string]any{"x": 10, "y": 10, "width": 100, "height": 40},
		}},
		"adaptation": map[string]any{
			"status":  "completed",
			"summary": "ready",
			"result": map[string]any{
				"market_pack_id":       marketPackID,
				"market_pack_version":  1,
				"copy_library_id":      copyLibraryID,
				"copy_library_version": 1,
				"text_replacements": []map[string]any{{
					"block_id": "headline", "location": "Top", "replacement_text": "Pinjaman Fleksibel", "status": "ready",
				}},
				"numeric_layouts": []map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	incompleteAnalysis := json.RawMessage(`{"text_blocks":[]}`)
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_source_analysis (workspace_id, candidate_id, analysis_version, status, summary, result, completed_at)
VALUES
  ($1, $2, 1, 'completed', 'ready', $4::jsonb, now()),
  ($1, $3, 1, 'completed', 'missing adaptation', $5::jsonb, now())
`, workspaceID, availableCandidateID, analyzingCandidateID, availableAnalysis, incompleteAnalysis); err != nil {
		t.Fatal(err)
	}

	request := newRequest(http.MethodGet, "/api/creative/materials?view=available&limit=1", nil)
	request.Header.Set("X-Workspace-ID", workspaceID)
	w := httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary available: %d %s", w.Code, w.Body.String())
	}
	var availablePage struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
		TotalCount int                                 `json:"total_count"`
		NextOffset *int                                `json:"next_offset"`
	}
	if err := json.NewDecoder(w.Body).Decode(&availablePage); err != nil {
		t.Fatal(err)
	}
	if len(availablePage.Candidates) != 1 || availablePage.Candidates[0].ID != availableCandidateID || availablePage.TotalCount != 1 || availablePage.NextOffset != nil {
		t.Fatalf("available page = %#v", availablePage)
	}

	request = newRequest(http.MethodGet, "/api/creative/materials?view=analyze&limit=2", nil)
	request.Header.Set("X-Workspace-ID", workspaceID)
	w = httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, request)
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary analyze: %d %s", w.Code, w.Body.String())
	}
	var analyzePage struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
		TotalCount int                                 `json:"total_count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&analyzePage); err != nil {
		t.Fatal(err)
	}
	if len(analyzePage.Candidates) != 1 || analyzePage.Candidates[0].ID != analyzingCandidateID || analyzePage.TotalCount != 1 {
		t.Fatalf("analyze page = %#v", analyzePage)
	}
}

func TestCreativeCrawlRunCreatedBeforeBrokerIsReusedByImport(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	input := creativeMaterialImportInput{
		WorkspaceID:  parseUUID(testWorkspaceID),
		ConnectorID:  "appgrowing",
		QuerySummary: "material_search",
		Params:       json.RawMessage(`{"analysis_agent_id":"analysis-agent-123"}`),
		ActorType:    "member",
		ActorID:      testUserID,
		UserID:       parseUUID(testUserID),
	}
	runID, err := testHandler.startCreativeMaterialCrawlRun(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.RunID = runID
	input.Materials = []creativeMaterialInput{{
		DedupeKey:  "precreated-run-" + runID,
		Title:      "Precreated run material",
		AssetType:  "image",
		PreviewURL: "https://example.test/precreated-run.png",
	}}
	summary, err := testHandler.importCreativeMaterials(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if summary.RunID != runID {
		t.Fatalf("import run = %q, want precreated %q", summary.RunID, runID)
	}
	var runCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_material_crawl_run WHERE id = $1`, runID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("crawl run count = %d, want 1", runCount)
	}
	runs, err := testHandler.listCreativeCrawlRunsForWorkspace(t.Context(), parseUUID(testWorkspaceID), pgtype.UUID{}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range runs {
		if run.ID == runID {
			if run.Status != "completed" || run.AnalysisAgentID != "analysis-agent-123" {
				t.Fatalf("crawl run response = %#v", run)
			}
			return
		}
	}
	t.Fatalf("precreated crawl run not listed: %#v", runs)
}
