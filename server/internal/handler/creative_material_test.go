package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

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
