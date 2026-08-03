package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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
	req = withURLParam(req, "id", issueID)
	req = withURLParam(req, "candidateId", candidateID)
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
