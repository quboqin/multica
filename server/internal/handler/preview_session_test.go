package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func createPreviewTestIssue(t *testing.T, workspaceID, creatorID, title string) string {
	t.Helper()
	if testHandler == nil {
		t.Skip("database not available")
	}

	var issueID string
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO issue (workspace_id, title, creator_type, creator_id, number)
		VALUES (
			$1, $2, 'member', $3,
			(SELECT COALESCE(MAX(number), 0) + 1 FROM issue WHERE workspace_id = $1)
		)
		RETURNING id
	`, workspaceID, title, creatorID).Scan(&issueID)
	if err != nil {
		t.Fatalf("create preview test issue: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID)
	})
	return issueID
}

func createPreviewTestMember(t *testing.T, role string) string {
	t.Helper()
	email := "preview-member-" + uuid.NewString() + "@multica.test"
	var userID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO "user" (name, email)
		VALUES ('Preview Test Member', $1)
		RETURNING id
	`, email).Scan(&userID); err != nil {
		t.Fatalf("create preview test member: %v", err)
	}
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, $3)
	`, testWorkspaceID, userID, role); err != nil {
		t.Fatalf("join preview test member to workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM "user" WHERE id = $1`, userID)
	})
	return userID
}

func createPreviewSessionForTest(t *testing.T, issueID string, body map[string]any) PreviewSessionResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+issueID+"/preview-sessions", body)
	req = withURLParam(req, "id", issueID)
	testHandler.CreatePreviewSession(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreatePreviewSession: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var response PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode preview session: %v", err)
	}
	return response
}

func TestPreviewSessionLifecycle(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Preview session lifecycle")
	expiresAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/issues/"+issueID+"/preview-sessions", map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://preview.example.com/app?branch=main",
		"expires_at":  expiresAt,
	})
	// An untrusted task header on a member request must not be persisted.
	req.Header.Set("X-Task-ID", uuid.NewString())
	req = withURLParam(req, "id", issueID)
	testHandler.CreatePreviewSession(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("CreatePreviewSession: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var created PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Platform != "web" || created.Provider != "external_web" {
		t.Errorf("target = %s/%s, want web/external_web", created.Platform, created.Provider)
	}
	if created.Title != "preview.example.com" {
		t.Errorf("default title = %q, want preview.example.com", created.Title)
	}
	if created.Status != "running" || created.StartedAt == nil {
		t.Errorf("created lifecycle = status %q, started_at %v; want running with timestamp", created.Status, created.StartedAt)
	}
	if created.TaskID != nil {
		t.Errorf("untrusted task header was persisted: %v", created.TaskID)
	}
	if created.ExpiresAt == nil {
		t.Error("expires_at was not persisted")
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/issues/"+issueID+"/preview-sessions", nil)
	req = withURLParam(req, "id", issueID)
	testHandler.ListPreviewSessions(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ListPreviewSessions: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var list struct {
		PreviewSessions []PreviewSessionResponse `json:"preview_sessions"`
		Total           int                      `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if list.Total != 1 || len(list.PreviewSessions) != 1 || list.PreviewSessions[0].ID != created.ID {
		t.Fatalf("list = %#v, want the created preview session", list)
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/preview-sessions/"+created.ID, nil)
	req = withURLParam(req, "sessionId", created.ID)
	testHandler.GetPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetPreviewSession: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+created.ID+"/stop", nil)
	req = withURLParam(req, "sessionId", created.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("StopPreviewSession: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var stopped PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&stopped); err != nil {
		t.Fatalf("decode stop response: %v", err)
	}
	if stopped.Status != "stopped" || stopped.StoppedAt == nil {
		t.Fatalf("stopped lifecycle = status %q, stopped_at %v", stopped.Status, stopped.StoppedAt)
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+created.ID+"/stop", nil)
	req = withURLParam(req, "sessionId", created.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("second StopPreviewSession: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var stoppedAgain PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&stoppedAgain); err != nil {
		t.Fatalf("decode second stop response: %v", err)
	}
	if *stoppedAgain.StoppedAt != *stopped.StoppedAt || stoppedAgain.UpdatedAt != stopped.UpdatedAt {
		t.Errorf("idempotent stop changed timestamps: first=%#v second=%#v", stopped, stoppedAgain)
	}
}

func TestLocalDevicePreviewLeaseSleepAndHandoff(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Device preview lease")
	otherIssueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Device preview lease handoff")
	previewURL := "http://127.0.0.1:18081?serial=emulator-5554"
	session := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "android",
		"provider":    "local_device",
		"preview_url": previewURL,
	})
	if session.ExpiresAt == nil || session.LastActiveAt == nil || session.LeaseExpiresAt == nil {
		t.Fatalf("local device lifecycle timestamps missing: %#v", session)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, *session.ExpiresAt)
	if err != nil || time.Until(expiresAt) < 3*time.Hour+59*time.Minute || time.Until(expiresAt) > 4*time.Hour+time.Minute {
		t.Fatalf("default expiry = %v, want about four hours (parse error %v)", expiresAt, err)
	}

	if _, err := testPool.Exec(context.Background(), `
		UPDATE preview_session
		SET last_active_at = now() - INTERVAL '21 minutes', lease_expires_at = now() - INTERVAL '1 minute'
		WHERE id = $1
	`, session.ID); err != nil {
		t.Fatalf("age preview session: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/preview-sessions/"+session.ID, nil)
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.GetPreviewSession(w, req)
	var sleeping PreviewSessionResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&sleeping) != nil || sleeping.Status != "sleeping" {
		t.Fatalf("aged session = %d %s, want sleeping", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+session.ID+"/touch", nil)
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.TouchPreviewSession(w, req)
	var touched PreviewSessionResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&touched) != nil || touched.Status != "running" || touched.LeaseExpiresAt == nil {
		t.Fatalf("touch session = %d %s, want renewed running session", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+otherIssueID+"/preview-sessions", map[string]any{
		"platform": "android", "provider": "local_device", "preview_url": previewURL,
	})
	req = withURLParam(req, "id", otherIssueID)
	testHandler.CreatePreviewSession(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("concurrent device create = %d %s, want 409", w.Code, w.Body.String())
	}

	if _, err := testPool.Exec(context.Background(), `UPDATE preview_session SET lease_expires_at = now() - INTERVAL '1 second' WHERE id = $1`, session.ID); err != nil {
		t.Fatalf("expire preview lease: %v", err)
	}
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+otherIssueID+"/preview-sessions", map[string]any{
		"platform": "android", "provider": "local_device", "preview_url": previewURL,
	})
	req = withURLParam(req, "id", otherIssueID)
	testHandler.CreatePreviewSession(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("handoff device create = %d %s, want 201", w.Code, w.Body.String())
	}
	var storedStatus string
	if err := testPool.QueryRow(context.Background(), `SELECT status FROM preview_session WHERE id = $1`, session.ID).Scan(&storedStatus); err != nil || storedStatus != "sleeping" {
		t.Fatalf("previous session status = %q, err %v; want sleeping", storedStatus, err)
	}
}

func TestSwitchPreviewSessionDevice(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Switch preview device")
	otherIssueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Conflicting preview device")
	initialSerial := "emulator-" + uuid.NewString()
	targetSerial := "USB-" + uuid.NewString()
	session := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "android",
		"provider":    "local_device",
		"preview_url": "http://127.0.0.1:18081?serial=" + initialSerial,
	})

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/preview-sessions/"+session.ID+"/device", map[string]any{
		"serial": targetSerial, "confirmed": true,
	})
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.SwitchPreviewSessionDevice(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("SwitchPreviewSessionDevice: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var switched PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&switched); err != nil {
		t.Fatalf("decode switched preview: %v", err)
	}
	if switched.Status != "running" || switched.LeaseExpiresAt == nil || !strings.Contains(switched.PreviewURL, "serial="+targetSerial) {
		t.Fatalf("switched preview = %#v", switched)
	}

	conflictSerial := "USB-" + uuid.NewString()
	createPreviewSessionForTest(t, otherIssueID, map[string]any{
		"platform":    "android",
		"provider":    "local_device",
		"preview_url": "http://127.0.0.1:18081?serial=" + conflictSerial,
	})
	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+session.ID+"/device", map[string]any{
		"serial": conflictSerial,
	})
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.SwitchPreviewSessionDevice(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("conflicting switch: expected 409, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("GET", "/api/preview-sessions/"+session.ID, nil)
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.GetPreviewSession(w, req)
	var unchanged PreviewSessionResponse
	if w.Code != http.StatusOK || json.NewDecoder(w.Body).Decode(&unchanged) != nil {
		t.Fatalf("get switched preview: %d %s", w.Code, w.Body.String())
	}
	if unchanged.PreviewURL != switched.PreviewURL {
		t.Fatalf("conflicting switch changed binding from %q to %q", switched.PreviewURL, unchanged.PreviewURL)
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+session.ID+"/device", map[string]any{
		"serial": "bad\nserial",
	})
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.SwitchPreviewSessionDevice(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid serial: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestNormalizeExternalPreviewURL(t *testing.T) {
	oversized := "https://example.com/" + strings.Repeat("a", maxPreviewURLBytes)
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{name: "https", input: "https://example.com/path", valid: true},
		{name: "localhost http", input: "http://localhost:3000", valid: true},
		{name: "missing", input: "", valid: false},
		{name: "relative", input: "/preview", valid: false},
		{name: "missing host", input: "http:///preview", valid: false},
		{name: "unsupported scheme", input: "ftp://example.com/preview", valid: false},
		{name: "credentials", input: "https://user:secret@example.com/preview", valid: false},
		{name: "oversized", input: oversized, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := normalizeExternalPreviewURL(test.input)
			if test.valid && err != nil {
				t.Fatalf("expected valid URL, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected URL validation error")
			}
		})
	}
}

func TestNormalizeLocalDevicePreviewURLRequiresPinnedSerial(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		valid bool
	}{
		{name: "pinned emulator", input: "http://127.0.0.1:18081?serial=emulator-5554", valid: true},
		{name: "pinned physical", input: "http://localhost:18081?serial=USB-123", valid: true},
		{name: "missing serial", input: "http://127.0.0.1:18081", valid: false},
		{name: "blank serial", input: "http://127.0.0.1:18081?serial=", valid: false},
		{name: "multiple serials", input: "http://127.0.0.1:18081?serial=one&serial=two", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := normalizeLocalDevicePreviewURL(test.input)
			if test.valid && err != nil {
				t.Fatalf("expected valid URL, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected pinned serial validation error")
			}
		})
	}
}

func TestCreatePreviewSessionValidatesPhaseZeroRequest(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Preview validation")
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "unsupported platform",
			body: map[string]any{"platform": "android", "provider": "external_web", "preview_url": "https://example.com"},
		},
		{
			name: "unsupported provider",
			body: map[string]any{"platform": "web", "provider": "cloud_avd", "preview_url": "https://example.com"},
		},
		{
			name: "invalid expiry",
			body: map[string]any{"platform": "web", "provider": "external_web", "preview_url": "https://example.com", "expires_at": "tomorrow"},
		},
		{
			name: "expired expiry",
			body: map[string]any{"platform": "web", "provider": "external_web", "preview_url": "https://example.com", "expires_at": time.Now().Add(-time.Minute).Format(time.RFC3339)},
		},
		{
			name: "title too long",
			body: map[string]any{"platform": "web", "provider": "external_web", "preview_url": "https://example.com", "title": strings.Repeat("x", 121)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := newRequest("POST", "/api/issues/"+issueID+"/preview-sessions", test.body)
			req = withURLParam(req, "id", issueID)
			testHandler.CreatePreviewSession(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestPreviewSessionWorkspaceIsolation(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Preview workspace isolation")
	session := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://isolation.example.com",
	})

	otherWorkspaceID := uuid.NewString()
	otherSlug := "preview-isolation-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO workspace (id, name, slug, issue_prefix)
		VALUES ($1, 'Preview Isolation', $2, 'PVI')
	`, otherWorkspaceID, otherSlug)
	if err != nil {
		t.Fatalf("create other workspace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM workspace WHERE id = $1`, otherWorkspaceID)
	})
	if _, err := testPool.Exec(context.Background(), `
		INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')
	`, otherWorkspaceID, testUserID); err != nil {
		t.Fatalf("join other workspace: %v", err)
	}

	for _, method := range []string{"GET", "POST"} {
		w := httptest.NewRecorder()
		req := newRequest(method, "/api/preview-sessions/"+session.ID, nil)
		req.Header.Set("X-Workspace-ID", otherWorkspaceID)
		req = withURLParam(req, "sessionId", session.ID)
		if method == "GET" {
			testHandler.GetPreviewSession(w, req)
		} else {
			testHandler.StopPreviewSession(w, req)
		}
		if w.Code != http.StatusNotFound {
			t.Errorf("%s cross-workspace session: expected 404, got %d: %s", method, w.Code, w.Body.String())
		}
	}

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/issues/"+issueID+"/preview-sessions", nil)
	req.Header.Set("X-Workspace-ID", otherWorkspaceID)
	req = withURLParam(req, "id", issueID)
	testHandler.ListPreviewSessions(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("cross-workspace issue list: expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewSessionExpiryProjection(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Expired preview")
	session := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://expired.example.com",
	})
	if _, err := testPool.Exec(context.Background(), `
		UPDATE preview_session SET expires_at = now() - interval '1 minute' WHERE id = $1
	`, session.ID); err != nil {
		t.Fatalf("expire preview session: %v", err)
	}

	w := httptest.NewRecorder()
	req := newRequest("GET", "/api/preview-sessions/"+session.ID, nil)
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.GetPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GetPreviewSession: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var expired PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&expired); err != nil {
		t.Fatalf("decode expired preview: %v", err)
	}
	if expired.Status != "expired" {
		t.Fatalf("expired preview status = %q, want expired", expired.Status)
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+session.ID+"/stop", nil)
	req = withURLParam(req, "sessionId", session.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("StopPreviewSession: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.NewDecoder(w.Body).Decode(&expired); err != nil {
		t.Fatalf("decode stopped expired preview: %v", err)
	}
	if expired.Status != "expired" {
		t.Fatalf("stop changed expired preview status to %q", expired.Status)
	}
}

func TestPreviewSessionMemberStopAuthorization(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Member preview permissions")
	ownerSession := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://owner-preview.example.com",
	})
	memberID := createPreviewTestMember(t, "member")

	w := httptest.NewRecorder()
	req := newRequest("POST", "/api/preview-sessions/"+ownerSession.ID+"/stop", nil)
	req.Header.Set("X-User-ID", memberID)
	req = withURLParam(req, "sessionId", ownerSession.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("unrelated member stop: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/issues/"+issueID+"/preview-sessions", map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://member-preview.example.com",
	})
	req.Header.Set("X-User-ID", memberID)
	req = withURLParam(req, "id", issueID)
	testHandler.CreatePreviewSession(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("member CreatePreviewSession: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var memberSession PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&memberSession); err != nil {
		t.Fatalf("decode member preview: %v", err)
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+memberSession.ID+"/stop", nil)
	req = withURLParam(req, "sessionId", memberSession.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("workspace owner stop: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPreviewSessionTaskTokenBindingAndStopOwnership(t *testing.T) {
	issueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Task preview")
	otherIssueID := createPreviewTestIssue(t, testWorkspaceID, testUserID, "Other task preview")

	var agentID string
	if err := testPool.QueryRow(context.Background(), `
		SELECT id FROM agent WHERE workspace_id = $1 ORDER BY created_at LIMIT 1
	`, testWorkspaceID).Scan(&agentID); err != nil {
		t.Fatalf("get test agent: %v", err)
	}
	var taskID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority)
		VALUES ($1, $2, $3, 'running', 0)
		RETURNING id
	`, agentID, testRuntimeID, issueID).Scan(&taskID); err != nil {
		t.Fatalf("create test task: %v", err)
	}

	makeAgentRequest := func(issue string) *http.Request {
		req := newRequest("POST", "/api/issues/"+issue+"/preview-sessions", map[string]any{
			"platform":    "web",
			"provider":    "external_web",
			"preview_url": "https://agent-preview.example.com",
		})
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agentID)
		req.Header.Set("X-Task-ID", taskID)
		return withURLParam(req, "id", issue)
	}

	w := httptest.NewRecorder()
	testHandler.CreatePreviewSession(w, makeAgentRequest(otherIssueID))
	if w.Code != http.StatusForbidden {
		t.Fatalf("mismatched task issue: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	testHandler.CreatePreviewSession(w, makeAgentRequest(issueID))
	if w.Code != http.StatusCreated {
		t.Fatalf("agent CreatePreviewSession: expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var agentSession PreviewSessionResponse
	if err := json.NewDecoder(w.Body).Decode(&agentSession); err != nil {
		t.Fatalf("decode agent preview: %v", err)
	}
	if agentSession.CreatorType != "agent" || agentSession.CreatorID != agentID || agentSession.TaskID == nil || *agentSession.TaskID != taskID {
		t.Fatalf("agent identity/task not stamped: %#v", agentSession)
	}

	var otherTaskID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority)
		VALUES ($1, $2, $3, 'running', 0)
		RETURNING id
	`, agentID, testRuntimeID, otherIssueID).Scan(&otherTaskID); err != nil {
		t.Fatalf("create other-issue task: %v", err)
	}

	w = httptest.NewRecorder()
	req := newRequest("POST", "/api/preview-sessions/"+agentSession.ID+"/stop", nil)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", otherTaskID)
	req = withURLParam(req, "sessionId", agentSession.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-issue agent stop: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	humanSession := createPreviewSessionForTest(t, issueID, map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://human-preview.example.com",
	})

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+humanSession.ID+"/stop", nil)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	req = withURLParam(req, "sessionId", humanSession.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("agent stopping member preview: expected 403, got %d: %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = newRequest("POST", "/api/preview-sessions/"+agentSession.ID+"/stop", nil)
	req.Header.Set("X-Actor-Source", "task_token")
	req.Header.Set("X-Agent-ID", agentID)
	req.Header.Set("X-Task-ID", taskID)
	req = withURLParam(req, "sessionId", agentSession.ID)
	testHandler.StopPreviewSession(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("agent stopping own preview: expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
