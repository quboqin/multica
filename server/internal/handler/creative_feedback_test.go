package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCreativeFeedbackCandidateRejectsAndUpdatesCurrentStatus(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "candidate rejection")
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		IssueID: issueID, SubjectType: "candidate", SubjectID: candidateID,
		EventType: "decision", Decision: "rejected", ReasonCodes: []string{"irrelevant"},
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeFeedbackEvent: %d %s", w.Code, w.Body.String())
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2`, issueID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Fatalf("candidate status = %q, want rejected", status)
	}
}

func TestCreativeFeedbackRecommendedCopyReplacement(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	copyID := createCreativeFeedbackCopyEntry(t)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "recommended_copy", SubjectID: copyID, EventType: "replacement",
		Decision: "replaced", ReasonCodes: []string{"tone_mismatch"}, Comment: "Use the approved alternative.",
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeFeedbackEvent: %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeFeedbackIdempotencyKeyReusesEvent(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "idempotent candidate rejection")
	input := creativeFeedbackEventInput{
		IdempotencyKey: "feedback-" + uuid.NewString(),
		IssueID:        issueID, SubjectType: "candidate", SubjectID: candidateID,
		EventType: "decision", Decision: "rejected", ReasonCodes: []string{"irrelevant"},
	}
	ids := make([]string, 0, 2)
	for attempt := 0; attempt < 2; attempt++ {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative-feedback-events", input)
		testHandler.CreateCreativeFeedbackEvent(w, req)
		if w.Code != http.StatusCreated && w.Code != http.StatusOK {
			t.Fatalf("attempt %d: %d %s", attempt+1, w.Code, w.Body.String())
		}
		var event creativeFeedbackEventResponse
		if err := json.NewDecoder(w.Body).Decode(&event); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, event.ID)
	}
	if ids[0] != ids[1] {
		t.Fatalf("idempotent event ids differ: %v", ids)
	}
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_feedback_event WHERE workspace_id = $1 AND idempotency_key = $2`, testWorkspaceID, input.IdempotencyKey).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent event count = %d, want 1", count)
	}
}

func TestCreativeFeedbackAssetRectangleAnnotation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	assetID := createCreativeFeedbackAsset(t)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "asset", SubjectID: assetID, EventType: "annotation", Decision: "reported",
		ReasonCodes: []string{"copy_error"}, Annotation: json.RawMessage(`{"kind":"rect","x":0.1,"y":0.2,"width":0.3,"height":0.2}`),
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeFeedbackEvent: %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeFeedbackCandidateDecisionWithoutIssueDoesNotRequireLegacyLink(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, 'Run-only candidate', 'image', 'https://example.test/run-only.png', '{}'::jsonb)
RETURNING id::text`, testWorkspaceID, "run-only-"+uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "candidate", SubjectID: candidateID, EventType: "decision", Decision: "rejected", ReasonCodes: []string{"irrelevant"},
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("run-only candidate feedback: %d %s", w.Code, w.Body.String())
	}
	var created creativeFeedbackEventResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.IssueID != "" {
		t.Fatalf("run-only candidate feedback issue_id = %q, want empty", created.IssueID)
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative-feedback-events/"+created.ID+"/undo", nil)
	req = withURLParam(req, "id", created.ID)
	testHandler.UndoCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("run-only candidate feedback undo: %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeFeedbackAssetPointAnnotation(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	assetID := createCreativeFeedbackAsset(t)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "asset", SubjectID: assetID, EventType: "annotation", Decision: "reported",
		ReasonCodes: []string{"copy_error"}, Annotation: json.RawMessage(`{"kind":"point","x":0.1,"y":0.2}`),
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("point annotation: %d %s", w.Code, w.Body.String())
	}
}

func TestCreativeFeedbackUndoAppendsEventAndResetsCandidate(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "candidate undo")
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		IssueID: issueID, SubjectType: "candidate", SubjectID: candidateID,
		EventType: "decision", Decision: "selected",
	})
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create feedback: %d %s", w.Code, w.Body.String())
	}
	var created creativeFeedbackEventResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative-feedback-events/"+created.ID+"/undo", nil)
	req = withURLParam(req, "id", created.ID)
	testHandler.UndoCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("undo feedback: %d %s", w.Code, w.Body.String())
	}
	var status string
	if err := testPool.QueryRow(t.Context(), `SELECT status FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2`, issueID, candidateID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "new" {
		t.Fatalf("candidate status = %q, want new after undo", status)
	}
	var eventCount int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_feedback_event WHERE issue_id = $1 AND subject_id = $2`, issueID, candidateID).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if eventCount != 2 {
		t.Fatalf("event count = %d, want append-only pair", eventCount)
	}
}

func TestCreativeFeedbackRejectsCrossWorkspaceSubject(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	var otherWorkspaceID, candidateID string
	slug := "creative-feedback-other-" + uuid.NewString()[:8]
	if err := testPool.QueryRow(t.Context(), `INSERT INTO workspace (name, slug, issue_prefix) VALUES ($1, $2, 'CFB') RETURNING id`, "Creative Feedback Other", slug).Scan(&otherWorkspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM workspace WHERE id = $1`, otherWorkspaceID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, 'Other workspace candidate', 'image', 'https://example.test/other.png', '{}'::jsonb)
RETURNING id::text`, otherWorkspaceID, "other-"+uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "candidate", SubjectID: candidateID, EventType: "viewed",
	})
	req.Header.Set("X-Workspace-ID", otherWorkspaceID)
	testHandler.CreateCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace feedback = %d %s, want 404", w.Code, w.Body.String())
	}
}

func TestCreativeFeedbackMetricsAggregateExplicitDecisions(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "metrics")
	for _, input := range []creativeFeedbackEventInput{
		{IssueID: issueID, SubjectType: "candidate", SubjectID: candidateID, EventType: "decision", Decision: "rejected", ReasonCodes: []string{"duplicate"}},
		{SubjectType: "recommended_copy", SubjectID: createCreativeFeedbackCopyEntry(t), EventType: "decision", Decision: "accepted"},
	} {
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative-feedback-events", input)
		testHandler.CreateCreativeFeedbackEvent(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("create feedback event: %d %s", w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	testHandler.GetCreativeFeedbackMetrics(w, newRequest(http.MethodGet, "/api/creative-feedback-events/metrics", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GetCreativeFeedbackMetrics: %d %s", w.Code, w.Body.String())
	}
	var metrics creativeFeedbackMetricsResponse
	if err := json.NewDecoder(w.Body).Decode(&metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.CandidateRejected < 1 || metrics.CopyAccepted < 1 {
		t.Fatalf("metrics = %#v, expected rejected candidate and accepted copy", metrics)
	}
}

func createCreativeFeedbackCandidate(t *testing.T, title string) (string, string) {
	t.Helper()
	issueID := createCreativeDeliveryTestIssue(t, title, "")
	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, preview_url, raw)
VALUES ($1, 'test', $2, $3, 'image', 'https://example.test/creative.png', '{}'::jsonb)
RETURNING id::text`, testWorkspaceID, "feedback-"+uuid.NewString(), title).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id) VALUES ($1, $2, $3)`, issueID, candidateID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	return issueID, candidateID
}

func createCreativeFeedbackCopyEntry(t *testing.T) string {
	t.Helper()
	var libraryID, copyID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_resource (workspace_id, kind, name, created_by)
VALUES ($1, 'copy_library', $2, $3) RETURNING id::text`, testWorkspaceID, "Feedback copy library "+uuid.NewString(), testUserID).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_copy_entry (workspace_id, library_id, external_key, headline, created_by)
VALUES ($1, $2, $3, 'Feedback headline', $4) RETURNING id::text`, testWorkspaceID, libraryID, uuid.NewString(), testUserID).Scan(&copyID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_resource WHERE id = $1`, libraryID) })
	return copyID
}

func createCreativeFeedbackAsset(t *testing.T) string {
	t.Helper()
	var assetID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, 'member', $2, 'feedback.png', '/uploads/feedback.png', 'image/png', 100)
RETURNING id::text`, testWorkspaceID, testUserID).Scan(&assetID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM attachment WHERE id = $1`, assetID) })
	return assetID
}
