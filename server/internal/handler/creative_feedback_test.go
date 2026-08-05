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
		Comment: "Not relevant to this campaign.", ContextSnapshot: json.RawMessage(`{"source":"candidate_pool"}`),
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
	var action, detailsJSON string
	if err := testPool.QueryRow(t.Context(), `
SELECT action, details::text
FROM activity_log
WHERE issue_id = $1 AND action = 'creative_feedback_recorded'
ORDER BY created_at DESC
LIMIT 1
`, issueID).Scan(&action, &detailsJSON); err != nil {
		t.Fatal(err)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(detailsJSON), &details); err != nil {
		t.Fatal(err)
	}
	if action != "creative_feedback_recorded" || details["subject_type"] != "candidate" || details["decision"] != "rejected" || details["comment"] != "Not relevant to this campaign." {
		t.Fatalf("activity = %q %#v", action, details)
	}
	snapshot, ok := details["context_snapshot"].(map[string]any)
	if !ok || snapshot["source"] != "candidate_pool" {
		t.Fatalf("activity context_snapshot = %#v", details["context_snapshot"])
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

func TestCreativeFeedbackAcceptsPublishedComposableCopyRecipe(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	resourceID := uuid.NewString()
	recipeID := uuid.NewString()
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource (id, workspace_id, kind, name, status, version, published_version, config, created_by)
VALUES ($1, $2, 'copy_library', 'Composable feedback library', 'draft', 2, 1, '{"recipes":[]}'::jsonb, $3)
`, resourceID, testWorkspaceID, testUserID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource_revision (resource_id, version, name, config, created_by)
VALUES ($1, 1, 'Composable feedback library', jsonb_build_object('recipes', jsonb_build_array(jsonb_build_object('id', $2::text))), $3),
       ($1, 2, 'Composable feedback library', '{"recipes":[]}'::jsonb, $3)
`, resourceID, recipeID, testUserID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_resource WHERE id = $1`, resourceID) })

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative-feedback-events", creativeFeedbackEventInput{
		SubjectType: "recommended_copy", SubjectID: recipeID, EventType: "decision", Decision: "accepted",
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
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'creative_feedback_recorded'`, issueID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent activity count = %d, want 1", count)
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
	var recordedCount, undoneCount int
	if err := testPool.QueryRow(t.Context(), `
SELECT
  count(*) FILTER (WHERE action = 'creative_feedback_recorded'),
  count(*) FILTER (WHERE action = 'creative_feedback_undone')
FROM activity_log
WHERE issue_id = $1
`, issueID).Scan(&recordedCount, &undoneCount); err != nil {
		t.Fatal(err)
	}
	if recordedCount != 1 || undoneCount != 1 {
		t.Fatalf("activity counts = recorded %d undone %d, want 1 each", recordedCount, undoneCount)
	}
}

func TestShouldProjectCreativeFeedback(t *testing.T) {
	tests := []struct {
		name        string
		subjectType string
		eventType   string
		decision    string
		want        bool
	}{
		{name: "candidate selected", subjectType: "candidate", eventType: "decision", decision: "selected", want: true},
		{name: "candidate rejected", subjectType: "candidate", eventType: "decision", decision: "rejected", want: true},
		{name: "candidate viewed", subjectType: "candidate", eventType: "viewed", want: false},
		{name: "copy accepted", subjectType: "recommended_copy", eventType: "decision", decision: "accepted", want: true},
		{name: "copy replaced", subjectType: "recommended_copy", eventType: "replacement", decision: "replaced", want: true},
		{name: "asset annotated", subjectType: "asset", eventType: "annotation", decision: "reported", want: false},
		{name: "asset needs revision", subjectType: "asset", eventType: "decision", decision: "needs_revision", want: false},
		{name: "asset accepted", subjectType: "asset", eventType: "decision", decision: "accepted", want: false},
		{name: "variant accepted", subjectType: "variant", eventType: "decision", decision: "accepted", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldProjectCreativeFeedback(tt.subjectType, tt.eventType, tt.decision); got != tt.want {
				t.Fatalf("shouldProjectCreativeFeedback() = %v, want %v", got, tt.want)
			}
		})
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

func TestCreativeFeedbackMetricsExcludeUndoneEvents(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	readMetrics := func() creativeFeedbackMetricsResponse {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.GetCreativeFeedbackMetrics(w, newRequest(http.MethodGet, "/api/creative-feedback-events/metrics", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GetCreativeFeedbackMetrics: %d %s", w.Code, w.Body.String())
		}
		var metrics creativeFeedbackMetricsResponse
		if err := json.NewDecoder(w.Body).Decode(&metrics); err != nil {
			t.Fatal(err)
		}
		return metrics
	}

	before := readMetrics()
	issueID, candidateID := createCreativeFeedbackCandidate(t, "undone metrics")
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
	active := readMetrics()
	if active.CandidateSelected != before.CandidateSelected+1 {
		t.Fatalf("active selected count = %d, want %d", active.CandidateSelected, before.CandidateSelected+1)
	}

	w = httptest.NewRecorder()
	req = newRequest(http.MethodPost, "/api/creative-feedback-events/"+created.ID+"/undo", nil)
	req = withURLParam(req, "id", created.ID)
	testHandler.UndoCreativeFeedbackEvent(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("undo feedback: %d %s", w.Code, w.Body.String())
	}
	afterUndo := readMetrics()
	if afterUndo.CandidateSelected != before.CandidateSelected {
		t.Fatalf("selected count after undo = %d, want %d", afterUndo.CandidateSelected, before.CandidateSelected)
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
