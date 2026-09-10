package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateCreativeAdjustmentInput(t *testing.T) {
	tests := []struct {
		name    string
		input   creativeAdjustmentInput
		wantErr bool
	}{
		{
			name: "single size",
			input: creativeAdjustmentInput{
				Variant: 2, Scope: "size", Size: "1200x628", Instruction: "Increase contrast",
				TargetAttachmentIDs: []string{"final"}, BaseAttachmentIDs: []string{"base"},
			},
		},
		{
			name: "whole variant",
			input: creativeAdjustmentInput{
				Variant: 3, Scope: "variant", Instruction: "Use a lighter footer",
				TargetAttachmentIDs: []string{"square", "landscape", "portrait"},
			},
		},
		{
			name: "size scope must target one output",
			input: creativeAdjustmentInput{
				Variant: 1, Scope: "size", Size: "1080x1080", Instruction: "Fix copy",
				TargetAttachmentIDs: []string{"one", "two"},
			},
			wantErr: true,
		},
		{
			name: "variant scope must target three outputs",
			input: creativeAdjustmentInput{
				Variant: 1, Scope: "variant", Instruction: "Fix layout",
				TargetAttachmentIDs: []string{"one"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCreativeAdjustmentInput(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateCreativeAdjustmentInput() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCreativeDeliveryInput(t *testing.T) {
	valid := creativeDeliveryInput{
		CandidateID: "candidate", WorkIssueID: "issue", Variant: 1,
		Size: "1080x1080", Revision: 1, FinalAttachmentID: "final",
	}
	if err := validateCreativeDeliveryInput(valid); err != nil {
		t.Fatalf("valid delivery rejected: %v", err)
	}
	valid.Size = "1024x1024"
	if err := validateCreativeDeliveryInput(valid); err == nil {
		t.Fatal("unsupported delivery size accepted")
	}
}

func TestCreativeAdjustmentTitle(t *testing.T) {
	got := creativeAdjustmentTitle(creativeAdjustmentInput{Variant: 2, Scope: "size", Size: "800x1000"}, 4)
	if got != "V02 / 800x1000 精准调整 · R4" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestCreativeDeliveryRegistrationAndScopedAdjustment(t *testing.T) {
	parentID := createCreativeDeliveryTestIssue(t, "Creative delivery parent", "")
	workIssueID := createCreativeDeliveryTestIssue(t, "Creative delivery work", parentID)

	var candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, dedupe_key, competitor, title, asset_type, archive_status)
VALUES ($1, $2, 'Easycash', 'Delivery fixture', 'image', 'completed')
RETURNING id::text
`, testWorkspaceID, "delivery-fixture-"+workIssueID).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_material_issue_candidate (issue_id, candidate_id, workspace_id, status)
VALUES ($1, $2, $3, 'selected')
`, parentID, candidateID, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_issue_item (issue_id, candidate_id, workspace_id, work_issue_id, updated_by)
VALUES ($1, $2, $3, $4, $5)
`, parentID, candidateID, testWorkspaceID, workIssueID, testUserID); err != nil {
		t.Fatal(err)
	}

	attachment := func(filename string) string {
		t.Helper()
		var id string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, issue_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, $2, 'member', $3, $4, $5, 'image/png', 128)
RETURNING id::text
`, testWorkspaceID, workIssueID, testUserID, filename, "http://example.test/"+filename).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	baseID := attachment("base.png")
	finalID := attachment("AdaKami_V02_1200x628_v1.png")
	evidenceID := attachment("prime-evidence.png")

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+parentID+"/creative-deliveries/register", map[string]any{
		"deliveries": []map[string]any{{
			"candidate_id": candidateID, "work_issue_id": workIssueID,
			"variant": 2, "size": "1200x628", "revision": 1,
			"base_attachment_id": baseID, "final_attachment_id": finalID,
			"prime_evidence_attachment_id": evidenceID,
		}},
	})
	req = withURLParam(req, "id", parentID)
	testHandler.RegisterCreativeDeliveries(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("RegisterCreativeDeliveries: %d %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", fmt.Sprintf("/api/issues/%s/creative-materials/%s/adjustments", parentID, candidateID), map[string]any{
		"variant": 2, "scope": "size", "size": "1200x628", "instruction": "Increase footer contrast",
		"target_attachment_ids": []string{finalID}, "base_attachment_ids": []string{baseID},
	})
	req = withURLParams(req, "id", parentID, "candidateId", candidateID)
	testHandler.CreateCreativeAdjustment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeAdjustment: %d %s", w.Code, w.Body.String())
	}
	var adjustment creativeAdjustmentRequestResponse
	if err := json.NewDecoder(w.Body).Decode(&adjustment); err != nil {
		t.Fatal(err)
	}
	if adjustment.Revision != 2 || adjustment.Scope != "size" || adjustment.Size != "1200x628" {
		t.Fatalf("unexpected adjustment: %#v", adjustment)
	}
	if len(adjustment.TargetAttachmentIDs) != 1 || adjustment.TargetAttachmentIDs[0] != finalID {
		t.Fatalf("target attachments not preserved: %#v", adjustment.TargetAttachmentIDs)
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", fmt.Sprintf("/api/issues/%s/creative-materials/%s/adjustments", parentID, candidateID), map[string]any{
		"variant": 2, "scope": "size", "size": "1080x1080", "instruction": "Wrong requested size",
		"target_attachment_ids": []string{finalID}, "base_attachment_ids": []string{baseID},
	})
	req = withURLParams(req, "id", parentID, "candidateId", candidateID)
	testHandler.CreateCreativeAdjustment(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched target size: got %d %s", w.Code, w.Body.String())
	}

	issue, err := testPool.Query(t.Context(), `SELECT revision, status FROM creative_issue_item WHERE issue_id = $1 AND candidate_id = $2`, parentID, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	defer issue.Close()
	if !issue.Next() {
		t.Fatal("creative issue item missing")
	}
	var revision int
	var status string
	if err := issue.Scan(&revision, &status); err != nil {
		t.Fatal(err)
	}
	if revision != 2 || status != "running" {
		t.Fatalf("item state = revision %d, status %s", revision, status)
	}
}

func createCreativeDeliveryTestIssue(t *testing.T, title, parentID string) string {
	t.Helper()
	body := map[string]any{"title": title, "status": "backlog", "priority": "medium"}
	if parentID != "" {
		body["parent_issue_id"] = parentID
	}
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues?workspace_id="+testWorkspaceID, body)
	testHandler.CreateIssue(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create issue: %d %s", w.Code, w.Body.String())
	}
	var issue IssueResponse
	if err := json.NewDecoder(w.Body).Decode(&issue); err != nil {
		t.Fatal(err)
	}
	return issue.ID
}
