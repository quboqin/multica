package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestImportCreativeMaterialLibraryQueuesReferenceAnalysisIdempotently(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	agentID := createReferenceAnalysisAgent(t)
	var issuesBefore int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM issue WHERE workspace_id = $1`, testWorkspaceID).Scan(&issuesBefore); err != nil {
		t.Fatal(err)
	}
	sourceURL := "https://example.test/manual-" + uuid.NewString() + ".png"
	importMaterial := func() creativeMaterialLibraryImportResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/materials/import", map[string]any{
			"source_url": sourceURL,
			"title":      "Manual reference analysis",
			"asset_type": "image",
		})
		testHandler.ImportCreativeMaterialLibrary(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("ImportCreativeMaterialLibrary: %d %s", w.Code, w.Body.String())
		}
		var response creativeMaterialLibraryImportResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		return response
	}

	first := importMaterial()
	if first.ID == "" || first.Analysis.Action != "queued" || first.Analysis.Status != "pending" || first.Analysis.CrawlRunID == "" || first.Analysis.TaskID == "" || first.Analysis.AnalysisAgentID != agentID {
		t.Fatalf("first import response = %#v", first)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, first.Analysis.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_crawl_run WHERE id = $1`, first.Analysis.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, first.ID)
	})
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'running', started_at = now() WHERE id = $1`, first.Analysis.TaskID); err != nil {
		t.Fatal(err)
	}

	second := importMaterial()
	if second.ID != first.ID || second.Analysis.CrawlRunID != first.Analysis.CrawlRunID || second.Analysis.TaskID != first.Analysis.TaskID || second.Analysis.Action != "already_queued" || second.Analysis.Status != "running" {
		t.Fatalf("duplicate import response = %#v, first = %#v", second, first)
	}
	var taskCount, issueCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM agent_task_queue
WHERE trigger_evidence_kind = 'creative_crawl_run_analysis'
  AND trigger_evidence_ref_id = $1
  AND context->>'candidate_id' = $2
`, first.Analysis.CrawlRunID, first.ID).Scan(&taskCount); err != nil {
		t.Fatal(err)
	}
	if taskCount != 1 {
		t.Fatalf("analysis task count = %d, want 1", taskCount)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM issue WHERE workspace_id = $1`, testWorkspaceID).Scan(&issueCount); err != nil {
		t.Fatal(err)
	}
	if issueCount != issuesBefore {
		t.Fatalf("issue count = %d, want unchanged %d", issueCount, issuesBefore)
	}

	w := httptest.NewRecorder()
	testHandler.ListCreativeMaterialLibrary(w, newRequest(http.MethodGet, "/api/creative/materials", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ListCreativeMaterialLibrary: %d %s", w.Code, w.Body.String())
	}
	var library struct {
		Candidates []creativeMaterialCandidateResponse `json:"candidates"`
	}
	if err := json.NewDecoder(w.Body).Decode(&library); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range library.Candidates {
		if candidate.ID == first.ID {
			if candidate.AnalysisStatus != "running" {
				t.Fatalf("candidate analysis status = %q, want running", candidate.AnalysisStatus)
			}
			return
		}
	}
	t.Fatal("imported candidate not found in material library")
}

func TestImportCreativeMaterialLibraryKeepsCandidateWhenTaskServiceUnavailable(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	createReferenceAnalysisAgent(t)
	originalTaskService := testHandler.TaskService
	testHandler.TaskService = nil
	defer func() { testHandler.TaskService = originalTaskService }()

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/materials/import", map[string]any{
		"source_url": "https://example.test/manual-failure-" + uuid.NewString() + ".png",
		"title":      "Persist despite queue failure",
		"asset_type": "image",
	})
	testHandler.ImportCreativeMaterialLibrary(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("ImportCreativeMaterialLibrary: %d %s", w.Code, w.Body.String())
	}
	var response creativeMaterialLibraryImportResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID == "" || response.Analysis.Action != "enqueue_failed" || response.Analysis.Status != "failed" || response.Analysis.Warning == "" {
		t.Fatalf("queue failure response = %#v", response)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_crawl_run WHERE id = $1`, response.Analysis.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, response.ID)
	})
	var candidateExists bool
	if err := testPool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1)`, response.ID).Scan(&candidateExists); err != nil {
		t.Fatal(err)
	}
	if !candidateExists {
		t.Fatal("candidate was lost after analysis queue failure")
	}
}

func TestRetryCreativeMaterialReferenceAnalysisRequeuesFailedCandidate(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	createReferenceAnalysisAgent(t)
	originalTaskService := testHandler.TaskService
	testHandler.TaskService = nil
	defer func() { testHandler.TaskService = originalTaskService }()

	imported := httptest.NewRecorder()
	importRequest := newRequest(http.MethodPost, "/api/creative/materials/import", map[string]any{
		"source_url": "https://example.test/retry-analysis-" + uuid.NewString() + ".png",
		"title":      "Retry analysis",
		"asset_type": "image",
	})
	testHandler.ImportCreativeMaterialLibrary(imported, importRequest)
	if imported.Code != http.StatusCreated {
		t.Fatalf("ImportCreativeMaterialLibrary: %d %s", imported.Code, imported.Body.String())
	}
	var failed creativeMaterialLibraryImportResponse
	if err := json.NewDecoder(imported.Body).Decode(&failed); err != nil {
		t.Fatal(err)
	}
	if failed.Analysis.Status != "failed" || failed.Analysis.CrawlRunID == "" {
		t.Fatalf("failed import = %#v", failed)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_crawl_run WHERE id = $1`, failed.Analysis.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, failed.ID)
	})

	testHandler.TaskService = originalTaskService
	retried := httptest.NewRecorder()
	retryRequest := withURLParam(newRequest(http.MethodPost, "/api/creative/materials/"+failed.ID+"/analysis/retry", nil), "id", failed.ID)
	testHandler.RetryCreativeMaterialReferenceAnalysis(retried, retryRequest)
	if retried.Code != http.StatusOK {
		t.Fatalf("RetryCreativeMaterialReferenceAnalysis: %d %s", retried.Code, retried.Body.String())
	}
	var response creativeMaterialLibraryImportResponse
	if err := json.NewDecoder(retried.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.ID != failed.ID || response.Analysis.Action != "queued" || response.Analysis.Status != "pending" || response.Analysis.CrawlRunID != failed.Analysis.CrawlRunID || response.Analysis.TaskID == "" {
		t.Fatalf("retry response = %#v", response)
	}
	var candidateCount, evidenceCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_material_candidate WHERE id = $1`, failed.ID).Scan(&candidateCount); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_material_crawl_run WHERE id = $1`, failed.Analysis.CrawlRunID).Scan(&evidenceCount); err != nil {
		t.Fatal(err)
	}
	if candidateCount != 1 || evidenceCount != 1 {
		t.Fatalf("retry created unexpected rows: candidates=%d evidence=%d", candidateCount, evidenceCount)
	}
}

func TestRetryCreativeMaterialReferenceAnalysisReusesMaterialSearchRun(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID := createReferenceAnalysisAgent(t)
	summary, err := testHandler.importCreativeMaterials(t.Context(), creativeMaterialImportInput{
		WorkspaceID:  parseUUID(testWorkspaceID),
		ConnectorID:  "appgrowing",
		QuerySummary: "material_search",
		Params:       json.RawMessage(`{"analysis_agent_id":"` + agentID + `"}`),
		Materials: []creativeMaterialInput{{
			DedupeKey:  "retry-material-search-" + uuid.NewString(),
			Title:      "Retry material search evidence",
			AssetType:  "image",
			PreviewURL: "https://example.test/retry-material-search-" + uuid.NewString() + ".png",
		}},
		ActorType: "member", ActorID: testUserID, UserID: parseUUID(testUserID),
	})
	if err != nil || len(summary.ImportedCandidateIDs) != 1 {
		t.Fatalf("import material search summary=%#v err=%v", summary, err)
	}
	candidateID := summary.ImportedCandidateIDs[0]
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, summary.RunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_source_analysis WHERE candidate_id = $1`, candidateID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_crawl_run WHERE id = $1`, summary.RunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})

	w := httptest.NewRecorder()
	req := withURLParam(newRequest(http.MethodPost, "/api/creative/materials/"+candidateID+"/analysis/retry", nil), "id", candidateID)
	testHandler.RetryCreativeMaterialReferenceAnalysis(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("RetryCreativeMaterialReferenceAnalysis: %d %s", w.Code, w.Body.String())
	}
	var response creativeMaterialLibraryImportResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Analysis.Action != "queued" || response.Analysis.CrawlRunID != summary.RunID || response.Analysis.TaskID == "" {
		t.Fatalf("retry response = %#v, want original material search run %s", response, summary.RunID)
	}
	var runCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT count(*)
FROM creative_material_crawl_run_candidate rc
JOIN creative_material_crawl_run cr ON cr.id = rc.run_id
WHERE rc.candidate_id = $1 AND cr.workspace_id = $2
`, candidateID, testWorkspaceID).Scan(&runCount); err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("retry created duplicate crawl evidence: candidate runs=%d", runCount)
	}
}

func TestCreativeAnalysisStatusFromTask(t *testing.T) {
	for status, want := range map[string]string{
		"queued": "pending", "dispatched": "pending", "running": "running",
		"completed": "completed", "failed": "failed", "cancelled": "failed",
	} {
		if got := creativeAnalysisStatusFromTask(status); got != want {
			t.Fatalf("creativeAnalysisStatusFromTask(%q) = %q, want %q", status, got, want)
		}
	}
}

func createReferenceAnalysisAgent(t *testing.T) string {
	t.Helper()
	agentID := createHandlerTestAgent(t, "reference-analysis-"+uuid.NewString(), nil)
	var skillID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO skill (workspace_id, name, config, created_by)
VALUES ($1, $2, jsonb_build_object('kind', 'creative_role', 'capability', 'reference_analysis'), $3)
RETURNING id::text
`, testWorkspaceID, "Reference analysis "+uuid.NewString(), testUserID).Scan(&skillID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `INSERT INTO agent_skill (agent_id, skill_id, enabled) VALUES ($1, $2, TRUE)`, agentID, skillID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM skill WHERE id = $1`, skillID) })
	return agentID
}
