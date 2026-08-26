package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCreateCreativeDirectEditAtomicallyInitializesSourceLineage(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := createCreativeDeliveryTestIssue(t, "Direct edit order", "")
	candidateID, sourceAttachmentID := createDirectEditCandidate(t, testWorkspaceID, testUserID)
	squad := createDirectEditSquadFixture(t)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/direct-edits", creativeDirectEditInput{
		IssueID: issueID, CandidateID: candidateID, UserRequest: "保留人物，移除竞品标识", TargetSize: "1080x1080", DeliveryMode: "preview", SquadID: squad.SquadID,
	})
	testHandler.CreateCreativeDirectEdit(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeDirectEdit: %d %s", w.Code, w.Body.String())
	}
	var response creativeDirectEditResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Order.IssueID != issueID || response.Order.TriggerEvidenceKind != "creative_direct_edit" || response.Item.CandidateID != candidateID {
		t.Fatalf("direct edit trace = %#v", response)
	}
	if response.Variant.VariantKey != "direct_edit" || response.Variant.Revision != 1 || response.SourceAsset.AttachmentID != sourceAttachmentID || response.SourceAsset.Revision != 1 || response.SourceAsset.DerivedFromAssetID != "" {
		t.Fatalf("direct edit source lineage = %#v", response)
	}
	var mode, request, deliveryMode string
	if err := testPool.QueryRow(t.Context(), `SELECT input_snapshot->>'mode', input_snapshot->>'user_request', input_snapshot->>'delivery_mode' FROM creative_order WHERE id = $1`, response.Order.ID).Scan(&mode, &request, &deliveryMode); err != nil {
		t.Fatal(err)
	}
	if mode != "direct_edit" || request != "保留人物，移除竞品标识" || deliveryMode != "preview" {
		t.Fatalf("snapshot = mode %q request %q delivery %q", mode, request, deliveryMode)
	}
	var snapshot struct {
		Squad struct {
			LeaderAgentID     string `json:"leader_agent_id"`
			DirectEditAgentID string `json:"direct_edit_agent_id"`
			ReviewerAgentID   string `json:"reviewer_agent_id"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(response.Order.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	wantAgents := map[string]string{
		"leader_agent_id": squad.LeaderAgentID, "direct_edit_agent_id": squad.DirectEditorAgentID, "reviewer_agent_id": squad.ReviewerAgentID,
	}
	gotAgents := map[string]string{
		"leader_agent_id": snapshot.Squad.LeaderAgentID, "direct_edit_agent_id": snapshot.Squad.DirectEditAgentID, "reviewer_agent_id": snapshot.Squad.ReviewerAgentID,
	}
	for field, want := range wantAgents {
		if gotAgents[field] != want {
			t.Errorf("squad snapshot %s = %q, want %q", field, gotAgents[field], want)
		}
	}
}

func TestCreateCreativeDirectEditAllowsMultipleDirectEditors(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	issueID := createCreativeDeliveryTestIssue(t, "Direct edit order pool", "")
	candidateID, _ := createDirectEditCandidate(t, testWorkspaceID, testUserID)
	squad := createDirectEditSquadFixtureWithPool(t, true)
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/direct-edits", creativeDirectEditInput{
		IssueID: issueID, CandidateID: candidateID, UserRequest: "替换背景", TargetSize: "1080x1080", DeliveryMode: "preview", SquadID: squad.SquadID,
	})
	testHandler.CreateCreativeDirectEdit(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreateCreativeDirectEdit: %d %s", w.Code, w.Body.String())
	}
	var response creativeDirectEditResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Squad struct {
			DirectEditAgentID  string   `json:"direct_edit_agent_id"`
			DirectEditAgentIDs []string `json:"direct_edit_agent_ids"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(response.Order.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{}{squad.DirectEditorAgentID: {}, squad.ExtraDirectAgentID: {}}
	for _, id := range snapshot.Squad.DirectEditAgentIDs {
		delete(want, id)
	}
	if snapshot.Squad.DirectEditAgentID == "" || len(snapshot.Squad.DirectEditAgentIDs) != 2 || len(want) != 0 {
		t.Fatalf("direct edit pool snapshot = primary:%q pool:%#v missing:%#v", snapshot.Squad.DirectEditAgentID, snapshot.Squad.DirectEditAgentIDs, want)
	}
}

func TestQueueCreativeOrderAdjustmentAllowsMultipleDirectEditors(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	rootIssueID, candidateID := createCreativeFeedbackCandidate(t, "direct edit adjustment pool")
	squad := createDirectEditSquadFixtureWithPool(t, true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"expected_sizes": standardCreativeAssetSizes,
		"squad_snapshot": map[string]any{
			"squad_id":             squad.SquadID,
			"leader_agent_id":      squad.LeaderAgentID,
			"producer_agent_id":    squad.DirectEditorAgentID,
			"reviewer_agent_id":    squad.ReviewerAgentID,
			"direct_edit_agent_id": squad.DirectEditorAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, created_by)
VALUES ($1, $2, 'completed', $3::jsonb, 'manual', $4)
RETURNING id::text
`, testWorkspaceID, rootIssueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID) })
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot, status)
VALUES ($1, $2, '{}'::jsonb, 'completed')
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status)
VALUES ($1, 'V02', 1, 'completed')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	targetSize := "800x1000"
	var targetAssetID, targetAttachmentID string
	for _, size := range standardCreativeAssetSizes {
		generatedAttachmentID := createCreativeFeedbackAsset(t)
		var generatedAssetID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'generated', $3, 'completed')
RETURNING id::text
`, variantID, size, generatedAttachmentID).Scan(&generatedAssetID); err != nil {
			t.Fatal(err)
		}
		if size != targetSize {
			continue
		}
		targetAttachmentID = createCreativeFeedbackAsset(t)
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, derived_from_asset_id, status)
VALUES ($1, $2, 1, 'primed', $3, $4, 'completed')
RETURNING id::text
`, variantID, size, targetAttachmentID, generatedAssetID).Scan(&targetAssetID); err != nil {
			t.Fatal(err)
		}
	}
	adjustmentIssueID := createCreativeDeliveryTestIssue(t, "Direct edit adjustment pool issue", rootIssueID)
	metadata, err := json.Marshal(map[string]any{
		"workflow":                   "creative_adjustment",
		"creative_adjustment_source": "creative_order",
		"creative_order_id":          orderID,
		"creative_order_item_id":     itemID,
		"creative_variant_id":        variantID,
		"creative_asset_id":          targetAssetID,
		"creative_attachment_id":     targetAttachmentID,
		"creative_scope":             "size",
		"creative_size":              targetSize,
		"creative_source_revision":   1,
		"creative_revision":          2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE issue SET metadata = $2::jsonb WHERE id = $1`, adjustmentIssueID, metadata); err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+orderID+"/adjustments", creativeOrderAdjustmentInput{
		AdjustmentIssueID: adjustmentIssueID,
		AssetID:           targetAssetID,
		SizeKey:           targetSize,
		Scope:             "size",
		SourceRevision:    1,
		Comment:           "把背景换成更明亮的办公室",
		EventType:         "decision",
		ReasonCodes:       []string{"theme_mismatch"},
		ContextSnapshot:   json.RawMessage(`{}`),
	})
	req = withURLParam(req, "id", orderID)
	testHandler.QueueCreativeOrderAdjustment(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("QueueCreativeOrderAdjustment: %d %s", w.Code, w.Body.String())
	}
	var response creativeOrderAdjustmentResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Revision != 2 || response.TaskID == "" {
		t.Fatalf("adjustment response = %#v", response)
	}
	var taskAgentID, taskContext string
	if err := testPool.QueryRow(t.Context(), `
SELECT agent_id::text, context::text
FROM agent_task_queue
WHERE id = $1
`, response.TaskID).Scan(&taskAgentID, &taskContext); err != nil {
		t.Fatal(err)
	}
	if taskAgentID != squad.DirectEditorAgentID && taskAgentID != squad.ExtraDirectAgentID {
		t.Fatalf("task agent = %q, want one of %q or %q", taskAgentID, squad.DirectEditorAgentID, squad.ExtraDirectAgentID)
	}
	var contextValue struct {
		DirectEditAgentID string `json:"direct_edit_agent_id"`
		DirectEdit        struct {
			DirectEditAgentID string `json:"direct_edit_agent_id"`
		} `json:"direct_edit"`
	}
	if err := json.Unmarshal([]byte(taskContext), &contextValue); err != nil {
		t.Fatal(err)
	}
	if contextValue.DirectEditAgentID != taskAgentID || contextValue.DirectEdit.DirectEditAgentID != taskAgentID {
		t.Fatalf("task context direct editor = %#v, task agent %q", contextValue, taskAgentID)
	}
	var frozen struct {
		Squad struct {
			DirectEditAgentIDs []string `json:"direct_edit_agent_ids"`
		} `json:"squad_snapshot"`
	}
	if err := testPool.QueryRow(t.Context(), `SELECT input_snapshot::text FROM creative_order WHERE id = $1`, orderID).Scan(&taskContext); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(taskContext), &frozen); err != nil {
		t.Fatal(err)
	}
	if len(frozen.Squad.DirectEditAgentIDs) != 2 {
		t.Fatalf("frozen direct edit pool = %#v", frozen.Squad.DirectEditAgentIDs)
	}
}

func TestCreateCreativeDirectEditRejectsMissingOrCrossWorkspaceSourceWithoutOrder(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	t.Run("missing source", func(t *testing.T) {
		issueID, candidateID := createCreativeFeedbackCandidate(t, "direct edit missing source")
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/direct-edits", creativeDirectEditInput{IssueID: issueID, CandidateID: candidateID, UserRequest: "调整文字", TargetSize: "1080x1080", DeliveryMode: "preview", SquadID: createDirectEditSquad(t)})
		testHandler.CreateCreativeDirectEdit(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("missing source: %d %s", w.Code, w.Body.String())
		}
		assertNoDirectEditOrder(t, issueID)
	})
	t.Run("cross workspace source", func(t *testing.T) {
		issueID := createCreativeDeliveryTestIssue(t, "Direct edit cross workspace", "")
		otherWorkspaceID := createDirectEditWorkspace(t)
		candidateID, _ := createDirectEditCandidate(t, otherWorkspaceID, testUserID)
		var foreignAttachmentID string
		if err := testPool.QueryRow(t.Context(), `SELECT source_attachment_id::text FROM creative_material_candidate WHERE id = $1`, candidateID).Scan(&foreignAttachmentID); err != nil {
			t.Fatal(err)
		}
		var localCandidateID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, source_attachment_id, raw)
VALUES ($1, 'test', $2, 'cross workspace source', 'image', $3, '{}'::jsonb)
RETURNING id::text
`, testWorkspaceID, uuid.NewString(), foreignAttachmentID).Scan(&localCandidateID); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		req := newRequest(http.MethodPost, "/api/creative/direct-edits", creativeDirectEditInput{IssueID: issueID, CandidateID: localCandidateID, UserRequest: "调整文字", TargetSize: "1080x1080", DeliveryMode: "publish", SquadID: createDirectEditSquad(t)})
		testHandler.CreateCreativeDirectEdit(w, req)
		if w.Code != http.StatusConflict {
			t.Fatalf("cross workspace source: %d %s", w.Code, w.Body.String())
		}
		assertNoDirectEditOrder(t, issueID)
	})
}

func createDirectEditCandidate(t *testing.T, workspaceID, uploaderID string) (string, string) {
	t.Helper()
	var attachmentID, candidateID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
VALUES ($1, 'member', $2, 'direct-edit.png', '/uploads/direct-edit.png', 'image/png', 100)
RETURNING id::text
`, workspaceID, uploaderID).Scan(&attachmentID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_material_candidate (workspace_id, connector_id, dedupe_key, title, asset_type, source_attachment_id, raw)
VALUES ($1, 'test', $2, 'direct edit source', 'image', $3, '{}'::jsonb)
RETURNING id::text
`, workspaceID, uuid.NewString(), attachmentID).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_candidate WHERE id = $1`, candidateID)
	})
	return candidateID, attachmentID
}

type directEditSquadFixture struct {
	SquadID             string
	LeaderAgentID       string
	DirectEditorAgentID string
	ExtraDirectAgentID  string
	ReviewerAgentID     string
}

func createDirectEditSquad(t *testing.T) string {
	t.Helper()
	return createDirectEditSquadFixture(t).SquadID
}

func createDirectEditSquadFixture(t *testing.T) directEditSquadFixture {
	return createDirectEditSquadFixtureWithPool(t, false)
}

func createDirectEditSquadFixtureWithPool(t *testing.T, extraDirectEditor bool) directEditSquadFixture {
	t.Helper()
	fixture := directEditSquadFixture{
		LeaderAgentID:       createHandlerTestAgent(t, "direct-edit-leader-"+uuid.NewString(), nil),
		DirectEditorAgentID: createHandlerTestAgent(t, "direct-edit-editor-"+uuid.NewString(), nil),
		ReviewerAgentID:     createHandlerTestAgent(t, "direct-edit-reviewer-"+uuid.NewString(), nil),
	}
	if extraDirectEditor {
		fixture.ExtraDirectAgentID = createHandlerTestAgent(t, "direct-edit-editor-extra-"+uuid.NewString(), nil)
	}
	skillIDs := make([]string, 0, 3)
	bindCapability := func(agentID, capability string) {
		var skillID string
		if err := testPool.QueryRow(t.Context(), `
INSERT INTO skill (workspace_id, name, config, created_by)
VALUES ($1, $2, jsonb_build_object('kind', 'creative_role', 'capability', $3::text), $4)
RETURNING id::text
`, testWorkspaceID, "Direct edit "+capability+" "+uuid.NewString(), capability, testUserID).Scan(&skillID); err != nil {
			t.Fatal(err)
		}
		skillIDs = append(skillIDs, skillID)
		if _, err := testPool.Exec(t.Context(), `INSERT INTO agent_skill (agent_id, skill_id, enabled) VALUES ($1, $2, TRUE)`, agentID, skillID); err != nil {
			t.Fatal(err)
		}
	}
	bindCapability(fixture.LeaderAgentID, "creative_leadership")
	bindCapability(fixture.DirectEditorAgentID, "direct_image_edit")
	if fixture.ExtraDirectAgentID != "" {
		bindCapability(fixture.ExtraDirectAgentID, "direct_image_edit")
	}
	bindCapability(fixture.ReviewerAgentID, "quality_control")
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO squad (workspace_id, name, leader_id, creator_id)
VALUES ($1, $2, $3, $4)
RETURNING id::text
`, testWorkspaceID, "Direct edit squad "+uuid.NewString(), fixture.LeaderAgentID, testUserID).Scan(&fixture.SquadID); err != nil {
		t.Fatal(err)
	}
	agentIDs := []string{fixture.LeaderAgentID, fixture.DirectEditorAgentID, fixture.ReviewerAgentID}
	if fixture.ExtraDirectAgentID != "" {
		agentIDs = append(agentIDs, fixture.ExtraDirectAgentID)
	}
	for _, agentID := range agentIDs {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO squad_member (squad_id, member_type, member_id, role)
		VALUES ($1, 'agent', $2, '')
`, fixture.SquadID, agentID); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM squad WHERE id = $1`, fixture.SquadID)
		for _, skillID := range skillIDs {
			_, _ = testPool.Exec(t.Context(), `DELETE FROM skill WHERE id = $1`, skillID)
		}
	})
	return fixture
}

func createDirectEditWorkspace(t *testing.T) string {
	t.Helper()
	var workspaceID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO workspace (name, slug, issue_prefix)
VALUES ($1, $2, 'DEX')
RETURNING id::text
`, "Direct edit foreign workspace", "direct-edit-"+uuid.NewString()).Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM workspace WHERE id = $1`, workspaceID) })
	return workspaceID
}

func assertNoDirectEditOrder(t *testing.T, issueID string) {
	t.Helper()
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order WHERE issue_id = $1`, issueID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected no order for failed initialization, got %d", count)
	}
}
