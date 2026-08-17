package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/broker"
)

type credentialTestWorker struct {
	loginRequests []broker.WorkerLoginSessionRequest
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
	return broker.WorkerCrawlResponse{}, nil
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

func TestCredentialProfileIsSharedPerWorkspaceConnector(t *testing.T) {
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
	service := broker.NewService(testHandler.Queries, worker)
	workspaceID := parseUUID(testWorkspaceID)
	firstAdminID := parseUUID(testUserID)
	secondAdminUUID := parseUUID(secondAdminID)

	first, err := service.StartLoginSession(ctx, broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      firstAdminID,
		ConnectorID: broker.ConnectorAppGrowing,
		Label:       "Shared AppGrowing",
	})
	if err != nil {
		t.Fatalf("start first login session: %v", err)
	}
	if len(worker.loginRequests) != 1 {
		t.Fatalf("first login requests = %d, want 1", len(worker.loginRequests))
	}
	if _, err := service.CompleteLoginSession(ctx, broker.CompleteLoginSessionInput{
		SessionToken: worker.loginRequests[0].SessionToken,
		Ciphertext:   []byte("test-ciphertext"),
		KeyVersion:   "test-v1",
	}); err != nil {
		t.Fatalf("complete first login session: %v", err)
	}

	second, err := service.StartLoginSession(ctx, broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      secondAdminUUID,
		ConnectorID: broker.ConnectorAppGrowing,
		Label:       "Ignored because the shared profile already exists",
	})
	if err != nil {
		t.Fatalf("start second login session: %v", err)
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
