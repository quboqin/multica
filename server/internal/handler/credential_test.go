package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/broker"
)

type credentialTestWorker struct {
	loginRequests []broker.WorkerLoginSessionRequest
	crawlResponse broker.WorkerCrawlResponse
	crawlErr      error
}

func (w *credentialTestWorker) Configured() bool { return true }

func (w *credentialTestWorker) StartLoginSession(_ context.Context, request broker.WorkerLoginSessionRequest) (broker.WorkerLoginSessionResponse, error) {
	w.loginRequests = append(w.loginRequests, request)
	return broker.WorkerLoginSessionResponse{
		BrowserURL:       "https://worker.example.test/login",
		ExpiresInSeconds: 60,
	}, nil
}

func (w *credentialTestWorker) RunCrawl(context.Context, broker.WorkerCrawlRequest) (broker.WorkerCrawlResponse, error) {
	if w.crawlErr != nil {
		return broker.WorkerCrawlResponse{}, w.crawlErr
	}
	if w.crawlResponse.Status == "" {
		return broker.WorkerCrawlResponse{Status: "completed"}, nil
	}
	return w.crawlResponse, nil
}

func TestRunCredentialCrawlQueuesReferenceAnalysisForNewMaterials(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	agentID := createReferenceAnalysisAgent(t)
	worker := &credentialTestWorker{crawlResponse: broker.WorkerCrawlResponse{
		Status: "completed",
		Raw: json.RawMessage(`{"selected_materials":[
			{"materialId":"crawl-fanout-` + uuid.NewString() + `","assetType":"image","resourceUrl":"https://example.test/crawl-fanout-one-` + uuid.NewString() + `.png"},
			{"materialId":"crawl-fanout-` + uuid.NewString() + `","assetType":"image","resourceUrl":"https://example.test/crawl-fanout-two-` + uuid.NewString() + `.png"}
		]}`),
	}}
	registry, err := broker.RegistryWithJSON(`[{"id":"fixture-crawl","display_name":"Fixture Crawl","login_url":"https://example.test/login","capabilities":["material_search"],"scope":"deployment"}]`)
	if err != nil {
		t.Fatal(err)
	}
	service := broker.NewServiceWithRegistry(testHandler.Queries, worker, registry)
	workspaceID := parseUUID(testWorkspaceID)
	userID := parseUUID(testUserID)
	started, err := service.StartLoginSession(t.Context(), broker.StartLoginSessionInput{
		WorkspaceID: workspaceID, UserID: userID, ConnectorID: "fixture-crawl", Label: "Fixture crawl profile",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM credential_profile WHERE id = $1`, started.Profile.ID)
	})
	if _, err := service.CompleteLoginSession(t.Context(), broker.CompleteLoginSessionInput{
		SessionToken: worker.loginRequests[0].SessionToken, Ciphertext: []byte("test-ciphertext"), KeyVersion: "test-v1",
	}); err != nil {
		t.Fatal(err)
	}
	handler := *testHandler
	handler.CredentialBroker = service
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/credential-crawl", map[string]any{
		"profile_id": uuidToString(started.Profile.ID), "connector_id": "fixture-crawl", "capability": "material_search",
		"params": map[string]any{"analysis_agent_id": agentID},
	})
	handler.RunCredentialCrawl(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("RunCredentialCrawl: %d %s", w.Code, w.Body.String())
	}
	var response struct {
		CrawlRunID        string                       `json:"crawl_run_id"`
		Analysis          creativeCrawlAnalysisSummary `json:"analysis"`
		CreativeMaterials creativeImportSummary        `json:"creative_materials"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.CrawlRunID == "" || response.CreativeMaterials.ImportedCount != 2 || response.Analysis != (creativeCrawlAnalysisSummary{Requested: 2, Queued: 2}) {
		t.Fatalf("crawl response = %#v", response)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM agent_task_queue WHERE trigger_evidence_ref_id = $1`, response.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_source_analysis WHERE trigger_evidence_ref_id = $1`, response.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `
WITH removed_candidates AS (
  DELETE FROM creative_material_crawl_run_candidate
  WHERE run_id = $1
  RETURNING candidate_id
)
DELETE FROM creative_material_candidate
WHERE id IN (SELECT candidate_id FROM removed_candidates)
`, response.CrawlRunID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_material_crawl_run WHERE id = $1`, response.CrawlRunID)
	})
	var queued, pending int
	if err := testPool.QueryRow(t.Context(), `
SELECT
  count(*) FILTER (WHERE task.status IN ('queued', 'dispatched', 'running')),
  count(*) FILTER (WHERE rc.analysis_status = 'pending')
FROM creative_material_crawl_run_candidate rc
LEFT JOIN agent_task_queue task
  ON task.trigger_evidence_kind = 'creative_crawl_run_analysis'
 AND task.trigger_evidence_ref_id = rc.run_id
 AND task.context->>'candidate_id' = rc.candidate_id::text
WHERE rc.run_id = $1
`, response.CrawlRunID).Scan(&queued, &pending); err != nil {
		t.Fatal(err)
	}
	if queued != 2 || pending != 2 {
		t.Fatalf("crawl fanout state queued=%d pending=%d", queued, pending)
	}
}

func TestWriteCredentialBrokerErrorClassifiesWorkerFailures(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"invalid", broker.ErrWorkerRequestInvalid, http.StatusBadRequest},
		{"busy", broker.ErrWorkerBusy, http.StatusTooManyRequests},
		{"timeout", broker.ErrWorkerTimeout, http.StatusGatewayTimeout},
		{"unavailable", broker.ErrWorkerUnavailable, http.StatusServiceUnavailable},
		{"deployment binding race", broker.ErrDeploymentProfileBindingConflict, http.StatusConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeCredentialBrokerError(recorder, tc.err)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, tc.want, recorder.Body.String())
			}
		})
	}
}

func TestCredentialCrawlFailureKeepsActionRequiredSeparateFromRealFailure(t *testing.T) {
	if !credentialCrawlRequiresLogin(broker.ErrProfileNotActive) {
		t.Fatal("inactive profile should require login")
	}
	if credentialCrawlRequiresLogin(broker.ErrWorkerTimeout) {
		t.Fatal("worker timeout must remain a real crawl failure")
	}
	if got := credentialCrawlErrorCode(broker.ErrWorkerTimeout); got != "worker_timeout" {
		t.Fatalf("worker timeout code = %q", got)
	}

	recorder := httptest.NewRecorder()
	writeCredentialCrawlError(recorder, broker.ErrProfileNotActive, "run-123")
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	var payload map[string]string
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["crawl_run_id"] != "run-123" {
		t.Fatalf("response missing stable crawl run: %#v", payload)
	}
}

func TestCreativeCrawlAnalysisAgentIDReadsOnlyValidParams(t *testing.T) {
	if got := creativeCrawlAnalysisAgentID(json.RawMessage(`{"analysis_agent_id":" analyst-1 "}`)); got != "analyst-1" {
		t.Fatalf("analysis agent id = %q", got)
	}
	if got := creativeCrawlAnalysisAgentID(json.RawMessage(`not-json`)); got != "" {
		t.Fatalf("invalid params yielded %q", got)
	}
}

func TestCredentialWorkspaceScopeAllowsMembersToViewButNotManage(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	email := "credential-member-" + uuid.NewString() + "@multica.ai"
	var memberID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Credential Member', $1)
		RETURNING id
	`, email).Scan(&memberID); err != nil {
		t.Fatalf("create member user: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, memberID) })
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'member')
	`, testWorkspaceID, memberID); err != nil {
		t.Fatalf("add workspace member: %v", err)
	}

	h := &Handler{
		Queries:          testHandler.Queries,
		CredentialBroker: broker.NewService(testHandler.Queries, nil),
	}
	request := httptest.NewRequest(http.MethodGet, "/api/credential-profiles", nil)
	request.Header.Set("X-User-ID", memberID)
	request.Header.Set("X-Workspace-ID", testWorkspaceID)

	viewerRecorder := httptest.NewRecorder()
	_, _, ok := h.credentialWorkspaceScope(viewerRecorder, request, false)
	if !ok {
		t.Fatalf("member should be able to view credential profiles: status=%d body=%s", viewerRecorder.Code, viewerRecorder.Body.String())
	}

	managerRecorder := httptest.NewRecorder()
	_, _, ok = h.credentialWorkspaceScope(managerRecorder, request, true)
	if ok {
		t.Fatal("member should not be able to manage credential profiles")
	}
	if managerRecorder.Code != http.StatusForbidden {
		t.Fatalf("manage status = %d, want %d", managerRecorder.Code, http.StatusForbidden)
	}
}

func TestCredentialProfileIsSharedAcrossWorkspacesWithExplicitManagers(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}

	ctx := context.Background()
	email := "credential-admin-" + uuid.NewString() + "@multica.ai"
	var secondAdminID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Credential Admin', $1)
		RETURNING id
	`, email).Scan(&secondAdminID); err != nil {
		t.Fatalf("create second admin: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, secondAdminID) })
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'admin')
	`, testWorkspaceID, secondAdminID); err != nil {
		t.Fatalf("add second admin: %v", err)
	}

	worker := &credentialTestWorker{}
	connectorID := "appgrowing-test-shared"
	registry, err := broker.RegistryWithJSON(`[{"id":"appgrowing-test-shared","display_name":"AppGrowing Test","login_url":"https://example.test/login","capabilities":["material_search"],"scope":"deployment"}]`)
	if err != nil {
		t.Fatalf("create test connector registry: %v", err)
	}
	service := broker.NewServiceWithRegistry(testHandler.Queries, worker, registry)
	workspaceID := parseUUID(testWorkspaceID)
	firstAdminID := parseUUID(testUserID)
	secondAdminUUID := parseUUID(secondAdminID)

	first, err := service.StartLoginSession(ctx, broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      firstAdminID,
		ConnectorID: connectorID,
		Label:       "Shared AppGrowing",
	})
	if err != nil {
		t.Fatalf("start first login session: %v", err)
	}
	if len(worker.loginRequests) != 1 {
		t.Fatalf("first login requests = %d, want 1", len(worker.loginRequests))
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM credential_profile WHERE id = $1`, first.Profile.ID)
	})
	if first.Profile.Scope != broker.ScopeDeployment {
		t.Fatalf("profile scope = %q, want deployment", first.Profile.Scope)
	}
	if _, err := service.CompleteLoginSession(ctx, broker.CompleteLoginSessionInput{
		SessionToken: worker.loginRequests[0].SessionToken,
		Ciphertext:   []byte("test-ciphertext"),
		KeyVersion:   "test-v1",
	}); err != nil {
		t.Fatalf("complete first login session: %v", err)
	}

	_, err = service.StartLoginSession(ctx, broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      secondAdminUUID,
		ConnectorID: connectorID,
		Label:       "Ignored because the shared profile already exists",
	})
	if !errors.Is(err, broker.ErrProfileManageForbidden) {
		t.Fatalf("unlisted workspace admin rebind error = %v, want manage forbidden", err)
	}
	if err := service.AddProfileManager(ctx, workspaceID, firstAdminID, first.Profile.ID, secondAdminUUID); err != nil {
		t.Fatalf("grant explicit manager: %v", err)
	}
	second, err := service.StartLoginSession(ctx, broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      secondAdminUUID,
		ConnectorID: connectorID,
		Label:       "Ignored because the shared profile already exists",
	})
	if err != nil {
		t.Fatalf("explicit manager starts second login session: %v", err)
	}
	if second.Profile.ID != first.Profile.ID {
		t.Fatalf("second profile id = %s, want existing shared profile %s", uuidToString(second.Profile.ID), uuidToString(first.Profile.ID))
	}
	if len(worker.loginRequests) != 2 {
		t.Fatalf("second login requests = %d, want 2", len(worker.loginRequests))
	}
	if _, err := service.CompleteLoginSession(ctx, broker.CompleteLoginSessionInput{
		SessionToken: worker.loginRequests[1].SessionToken,
		Ciphertext:   []byte("test-ciphertext"),
		KeyVersion:   "test-v1",
	}); err != nil {
		t.Fatalf("complete second login session: %v", err)
	}

	profile, err := service.GetProfile(ctx, workspaceID, first.Profile.ID)
	if err != nil {
		t.Fatalf("get shared profile: %v", err)
	}
	if uuidToString(profile.AuthorizedByID) != secondAdminID {
		t.Fatalf("authorized_by_id = %q, want latest completing admin %q", uuidToString(profile.AuthorizedByID), secondAdminID)
	}

	otherWorkspaceSlug := "credential-shared-" + uuid.NewString()
	var otherWorkspaceID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, issue_prefix)
		VALUES ('Credential Shared Other', $1, 'CSO')
		RETURNING id
	`, otherWorkspaceSlug).Scan(&otherWorkspaceID); err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, otherWorkspaceID)
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'member')
	`, otherWorkspaceID, secondAdminID); err != nil {
		t.Fatalf("add member to other workspace: %v", err)
	}

	result, err := service.RunCrawl(ctx, broker.RunCrawlInput{
		WorkspaceID:      parseUUID(otherWorkspaceID),
		RequestingUserID: secondAdminUUID,
		ConnectorID:      connectorID,
		Capability:       "material_search",
		Params:           json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatalf("other workspace resolves shared profile: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("other workspace crawl status = %q", result.Status)
	}
	var auditCount int
	if err := testPool.QueryRow(ctx, `
		SELECT count(*) FROM credential_usage_audit
		WHERE profile_id = $1 AND workspace_id = $2 AND outcome = 'completed'
	`, first.Profile.ID, otherWorkspaceID).Scan(&auditCount); err != nil {
		t.Fatalf("read cross-workspace usage audit: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("cross-workspace audit count = %d, want 1", auditCount)
	}

	responseJSON, err := json.Marshal(credentialProfileToResponse(profile))
	if err != nil {
		t.Fatalf("marshal profile response: %v", err)
	}
	if containsJSONKey(responseJSON, "authorized_by_id") {
		t.Fatalf("profile response exposes audit identity: %s", responseJSON)
	}
}

func containsJSONKey(raw []byte, key string) bool {
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil {
		return false
	}
	_, ok := value[key]
	return ok
}
