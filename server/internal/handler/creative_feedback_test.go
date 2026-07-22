package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeCreativeEditFeedback(t *testing.T) {
	decision, reasons, suggestion, err := normalizeCreativeEditFeedback(createCreativeEditFeedbackRequest{
		Decision:    " needs_revision ",
		ReasonCodes: []string{"copy_error", "copy_error", " layout_mismatch "},
		Suggestion:  "  Keep the amount hierarchy.  ",
	})
	if err != nil {
		t.Fatalf("normalizeCreativeEditFeedback: %v", err)
	}
	if decision != "needs_revision" {
		t.Fatalf("decision = %q", decision)
	}
	if len(reasons) != 2 || reasons[0] != "copy_error" || reasons[1] != "layout_mismatch" {
		t.Fatalf("reasons = %v", reasons)
	}
	if suggestion != "Keep the amount hierarchy." {
		t.Fatalf("suggestion = %q", suggestion)
	}
}

func TestNormalizeCreativeEditFeedbackRejectsMismatchedReason(t *testing.T) {
	_, _, _, err := normalizeCreativeEditFeedback(createCreativeEditFeedbackRequest{
		Decision:    "accepted",
		ReasonCodes: []string{"copy_error"},
	})
	if err == nil {
		t.Fatal("expected a decision/reason validation error")
	}
}

func TestNormalizeCreativeEditFeedbackRequiresReasonAndLimitsSuggestion(t *testing.T) {
	if _, _, _, err := normalizeCreativeEditFeedback(createCreativeEditFeedbackRequest{Decision: "rejected"}); err == nil {
		t.Fatal("expected a missing reason validation error")
	}
	_, _, _, err := normalizeCreativeEditFeedback(createCreativeEditFeedbackRequest{
		Decision:    "rejected",
		ReasonCodes: []string{"other"},
		Suggestion:  strings.Repeat("改", creativeFeedbackSuggestionLimit+1),
	})
	if err == nil {
		t.Fatal("expected a suggestion length validation error")
	}
}

func TestCreateCreativeEditFeedbackPersistsScopedHistoryAndProcessSnapshot(t *testing.T) {
	issueID, jobID, variantID, candidateID := seedCreativeFeedbackTarget(t)
	req := newRequest("POST", "/api/issues/"+issueID+"/creative-edit-jobs/"+jobID+"/variants/"+variantID+"/feedback", createCreativeEditFeedbackRequest{
		Decision:    "needs_revision",
		ReasonCodes: []string{"benefit_mismatch", "copy_too_long"},
		Suggestion:  "保留原图的信息层级，缩短主文案。",
	})
	req = withURLParams(req, "id", issueID, "jobId", jobID, "variantId", variantID)
	recorder := httptest.NewRecorder()

	testHandler.CreateCreativeEditFeedback(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var response creativeMaterialsResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.EditJobs) != 1 || len(response.EditJobs[0].Variants) != 1 {
		t.Fatalf("unexpected jobs response: %+v", response.EditJobs)
	}
	var jobProcessData map[string]any
	if err := json.Unmarshal(response.EditJobs[0].ProcessData, &jobProcessData); err != nil || jobProcessData["schema_version"] != "1" {
		t.Fatalf("job API process_data = %s, err = %v", response.EditJobs[0].ProcessData, err)
	}
	feedback := response.EditJobs[0].Variants[0].Feedback
	if len(feedback) != 1 {
		t.Fatalf("feedback history length = %d", len(feedback))
	}
	if feedback[0].CandidateID != candidateID || feedback[0].VariantID != variantID || feedback[0].JobID != jobID {
		t.Fatalf("feedback links = %+v", feedback[0])
	}
	if feedback[0].CreatedByName != handlerTestName {
		t.Fatalf("created_by_name = %q", feedback[0].CreatedByName)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(feedback[0].ProcessSnapshot, &snapshot); err != nil {
		t.Fatalf("decode process snapshot: %v", err)
	}
	if snapshot["variant_qc_status"] != "passed" || snapshot["job_status"] != "completed" {
		t.Fatalf("process snapshot = %v", snapshot)
	}
	if snapshot["job_prompt"] != "test prompt" || snapshot["asset_count"] != float64(1) {
		t.Fatalf("prompt/assets snapshot = %v", snapshot)
	}
	processData, ok := snapshot["process_data"].(map[string]any)
	if !ok || processData["schema_version"] != "1" {
		t.Fatalf("process_data snapshot = %v", snapshot["process_data"])
	}
	dynamicRules, ok := snapshot["dynamic_rules"].(map[string]any)
	if !ok || dynamicRules["market"] != "idn-adakami" || dynamicRules["variant_count"] != float64(3) {
		t.Fatalf("dynamic rules snapshot = %v", snapshot["dynamic_rules"])
	}
	sourceCandidate, ok := snapshot["source_candidate"].(map[string]any)
	if !ok || sourceCandidate["competitor"] != "Competitor" || sourceCandidate["asset_type"] != "image" {
		t.Fatalf("source candidate snapshot = %v", snapshot["source_candidate"])
	}
	if strings.Contains(string(feedback[0].ProcessSnapshot), "example.test") {
		t.Fatal("process snapshot must not persist asset URLs")
	}
}

func TestCreateCreativeEditFeedbackRejectsForeignWorkspaceIssue(t *testing.T) {
	issueID, jobID, variantID, _ := seedCreativeFeedbackTarget(t)
	req := newRequest("POST", "/api/issues/"+issueID+"/feedback", createCreativeEditFeedbackRequest{
		Decision:    "accepted",
		ReasonCodes: []string{"ready_to_publish"},
	})
	req.Header.Set("X-Workspace-ID", uuid.NewString())
	req = withURLParams(req, "id", issueID, "jobId", jobID, "variantId", variantID)
	recorder := httptest.NewRecorder()

	testHandler.CreateCreativeEditFeedback(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func seedCreativeFeedbackTarget(t *testing.T) (string, string, string, string) {
	t.Helper()
	issueID := createTestIssue(t, "Creative feedback "+uuid.NewString(), "todo", "medium")
	var candidateID string
	if err := testPool.QueryRow(context.Background(), `
INSERT INTO creative_material_candidate (workspace_id, dedupe_key, competitor, title, asset_type)
VALUES ($1::uuid, $2, 'Competitor', 'Reference ad', 'image')
RETURNING id::text
`, testWorkspaceID, uuid.NewString()).Scan(&candidateID); err != nil {
		t.Fatalf("create creative candidate: %v", err)
	}
	var jobID string
	if err := testPool.QueryRow(context.Background(), `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_status, stage, progress, completed_at, poll_attempts, process_data
)
VALUES ($1::uuid, $2::uuid, 'completed', 'test prompt',
        '{"market":"idn-adakami","strategy":"instruct","variant_count":3,"sizes":[{"width":1024,"height":1024,"label":"1024x1024"}]}'::jsonb,
        'member', $3::uuid,
        'completed', 'quality_check_complete', 100, now(), 3,
		'{"schema_version":"1","usage":{"token_usage":"not_available","cost":"not_available"}}'::jsonb)
RETURNING id::text
`, testWorkspaceID, issueID, testUserID).Scan(&jobID); err != nil {
		t.Fatalf("create creative job: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id) VALUES ($1::uuid, $2::uuid)
`, jobID, candidateID); err != nil {
		t.Fatalf("link creative job candidate: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, status)
VALUES ($1::uuid, $2::uuid, $3::uuid, 'edited')
`, issueID, candidateID, testWorkspaceID); err != nil {
		t.Fatalf("link creative issue candidate: %v", err)
	}
	var variantID string
	if err := testPool.QueryRow(context.Background(), `
INSERT INTO creative_edit_variant (job_id, candidate_id, variant_index, title, qc_status)
VALUES ($1::uuid, $2::uuid, 1, 'Variant 1', 'passed')
RETURNING id::text
`, jobID, candidateID).Scan(&variantID); err != nil {
		t.Fatalf("create creative variant: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
INSERT INTO creative_edit_asset (variant_id, width, height, label, asset_url, content_type)
VALUES ($1::uuid, 1024, 1024, '1024x1024', 'https://assets.example.test/variant.png', 'image/png')
`, variantID); err != nil {
		t.Fatalf("create creative asset: %v", err)
	}
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1::uuid`, issueID)
		testPool.Exec(context.Background(), `DELETE FROM creative_material_candidate WHERE id = $1::uuid`, candidateID)
	})
	return issueID, jobID, variantID, candidateID
}
