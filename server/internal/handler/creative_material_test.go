package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
