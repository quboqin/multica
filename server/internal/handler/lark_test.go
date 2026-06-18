package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/integrations/lark"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Lark-handler unit tests focus on the no-config short-circuits —
// verifying that a self-host deployment without MULTICA_LARK_SECRET_KEY
// does NOT serve revoke / redeem / install, and that list degrades
// gracefully to an empty response so the Integrations tab still
// renders. Happy-path flows (begin device-flow + poll status; token
// mint + redeem) need a real DB and land alongside the WS hub
// integration tests in a follow-up commit.

func TestRevokeLarkInstallation_NotConfigured(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodDelete, "/api/workspaces/x/lark/installations/y", nil)
	w := httptest.NewRecorder()
	h.RevokeLarkInstallation(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestRedeemLarkBindingToken_NotConfigured(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/lark/binding/redeem", strings.NewReader(`{"token":"x"}`))
	w := httptest.NewRecorder()
	h.RedeemLarkBindingToken(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestBeginLarkInstall_NotConfigured(t *testing.T) {
	// When the device-flow registration service is nil (no at-rest
	// key, or the stub APIClient is the only one wired), the begin
	// endpoint must short-circuit to 503 — silently returning a
	// "configured: false" envelope would hide a real misconfiguration
	// from the operator. The UI hides the bind button in that case
	// so this should not be reached through the normal flow.
	h := &Handler{}
	req := httptest.NewRequest(http.MethodPost, "/api/workspaces/x/lark/install/begin?agent_id=y", nil)
	w := httptest.NewRecorder()
	h.BeginLarkInstall(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestGetLarkInstallStatus_NotConfigured(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/x/lark/install/sess_y/status", nil)
	w := httptest.NewRecorder()
	h.GetLarkInstallStatus(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestListLarkInstallations_NotConfiguredReturnsEmpty(t *testing.T) {
	// Listing is intentionally a "soft" endpoint: when lark is not
	// configured we return an empty list + configured:false rather
	// than a 503, so the Integrations tab renders normally with a
	// "not connected" empty state instead of an error banner.
	h := &Handler{}
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/x/lark/installations", nil)
	w := httptest.NewRecorder()
	h.ListLarkInstallations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Installations    []any `json:"installations"`
		Configured       bool  `json:"configured"`
		InstallSupported bool  `json:"install_supported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Configured {
		t.Fatalf("configured should be false when LarkInstallations is nil")
	}
	if resp.InstallSupported {
		t.Fatalf("install_supported should be false when LarkInstallations is nil")
	}
	if len(resp.Installations) != 0 {
		t.Fatalf("expected empty installations list, got %d", len(resp.Installations))
	}
}

// TestListLarkInstallations_StubClientReportsInstallNotSupported pins
// the front-half of the "don't expose a doomed install flow"
// guarantee: even when the at-rest key + registration service are set,
// install_supported flips false if the underlying APIClient is the
// stub. The stub cannot complete the post-poll GetBotInfo call that
// finalizes a device-flow install, so the UI must hide install entry
// points until a real client is wired.
func TestListLarkInstallations_StubClientReportsInstallNotSupported(t *testing.T) {
	stubLogger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	h := &Handler{
		LarkAPIClient: lark.NewStubAPIClient(stubLogger),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/x/lark/installations", nil)
	w := httptest.NewRecorder()
	h.ListLarkInstallations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Configured       bool `json:"configured"`
		InstallSupported bool `json:"install_supported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.InstallSupported {
		t.Fatalf("install_supported must be false while only stub APIClient is wired")
	}
}

// TestListLarkInstallations_NotConfigured_HardCodedInstallSupportedFalse
// pins the invariant for the early-return branch: when
// LarkInstallations is nil (the deployment has no at-rest encryption
// key wired), the response MUST return both configured:false AND
// install_supported:false regardless of what APIClient is in place.
// A real APIClient on a not-configured deployment must not flip
// install_supported via the APIClient path — that path is not
// consulted in the early-return branch.
func TestListLarkInstallations_NotConfigured_HardCodedInstallSupportedFalse(t *testing.T) {
	stubLogger := slog.New(slog.NewTextHandler(httptest.NewRecorder(), nil))
	h := &Handler{
		LarkInstallations: nil, // triggers the not-configured early return.
		LarkAPIClient:     lark.NewStubAPIClient(stubLogger),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/x/lark/installations", nil)
	w := httptest.NewRecorder()
	h.ListLarkInstallations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct {
		Configured       bool `json:"configured"`
		InstallSupported bool `json:"install_supported"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Configured {
		t.Fatalf("configured must be false when LarkInstallations is nil")
	}
	if resp.InstallSupported {
		t.Fatalf("install_supported must be false in the early-return branch even with a non-nil APIClient")
	}
}

func TestFindOrCreateUserForLarkLoginPrefersExistingBinding(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	cleanupLarkLoginBindingPreferenceTest(t, ctx)

	boundEmail := "lark-bound-" + uuid.NewString() + "@multica.ai"
	emailMatchEmail := "lark-email-match-" + uuid.NewString() + "@multica.ai"
	var boundUserID, emailMatchUserID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Bound User', $1)
		RETURNING id
	`, boundEmail).Scan(&boundUserID); err != nil {
		t.Fatalf("create bound user: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ('Email Match User', $1)
		RETURNING id
	`, emailMatchEmail).Scan(&emailMatchUserID); err != nil {
		t.Fatalf("create email-match user: %v", err)
	}
	t.Cleanup(func() {
		cleanupLarkLoginBindingPreferenceTest(t, context.Background())
	})
	if _, err := testPool.Exec(ctx, `
		INSERT INTO member (workspace_id, user_id, role)
		VALUES ($1, $2, 'member'), ($1, $3, 'member')
		ON CONFLICT (workspace_id, user_id) DO NOTHING
	`, testWorkspaceID, boundUserID, emailMatchUserID); err != nil {
		t.Fatalf("create memberships: %v", err)
	}

	var agentID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent (
			workspace_id, name, description, runtime_mode, runtime_config,
			runtime_id, visibility, max_concurrent_tasks, owner_id
		)
		VALUES ($1, $2, '', 'cloud', '{}'::jsonb, $3, 'workspace', 1, $4)
		RETURNING id
	`, testWorkspaceID, "Lark Login Binding Preference", testRuntimeID, boundUserID).Scan(&agentID); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	appID := "cli_lark_login_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	inst, err := testHandler.Queries.CreateLarkInstallation(ctx, db.CreateLarkInstallationParams{
		WorkspaceID:        util.MustParseUUID(testWorkspaceID),
		AgentID:            util.MustParseUUID(agentID),
		AppID:              appID,
		AppSecretEncrypted: []byte("ciphertext"),
		TenantKey:          pgtype.Text{String: "tenant", Valid: true},
		BotOpenID:          "ou_bot_" + uuid.NewString(),
		InstallerUserID:    util.MustParseUUID(boundUserID),
	})
	if err != nil {
		t.Fatalf("create installation: %v", err)
	}

	_, err = testHandler.Queries.CreateLarkUserBinding(ctx, db.CreateLarkUserBindingParams{
		WorkspaceID:    util.MustParseUUID(testWorkspaceID),
		MulticaUserID:  util.MustParseUUID(boundUserID),
		InstallationID: inst.ID,
		LarkOpenID:     "ou_existing",
		UnionID:        pgtype.Text{String: "on_existing", Valid: true},
	})
	if err != nil {
		t.Fatalf("create binding: %v", err)
	}

	user, isNew, err := testHandler.findOrCreateUserForLarkLogin(ctx, inst, "feishu", lark.LoginUserInfo{
		OpenID:  "ou_existing",
		UnionID: "on_existing",
		Email:   emailMatchEmail,
	})
	if err != nil {
		t.Fatalf("findOrCreateUserForLarkLogin: %v", err)
	}
	if isNew {
		t.Fatalf("existing binding login should not create a new user")
	}
	if uuidToString(user.ID) != boundUserID {
		t.Fatalf("user id = %s, want existing binding user %s (email match user was %s)", uuidToString(user.ID), boundUserID, emailMatchUserID)
	}
}

func cleanupLarkLoginBindingPreferenceTest(t *testing.T, ctx context.Context) {
	t.Helper()
	statements := []string{
		`DELETE FROM lark_user_binding WHERE multica_user_id IN (SELECT id FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai')`,
		`DELETE FROM lark_login_identity WHERE multica_user_id IN (SELECT id FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai')`,
		`DELETE FROM lark_installation
		 WHERE installer_user_id IN (SELECT id FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai')
		    OR agent_id IN (
				SELECT a.id
				FROM agent a
				JOIN "user" u ON u.id = a.owner_id
				WHERE (u.email LIKE 'lark-bound-%@multica.ai' OR u.email LIKE 'lark-email-match-%@multica.ai')
				  AND a.name = 'Lark Login Binding Preference'
			)`,
		`DELETE FROM agent
		 WHERE owner_id IN (SELECT id FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai')
		   AND name = 'Lark Login Binding Preference'`,
		`DELETE FROM member WHERE user_id IN (SELECT id FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai')`,
		`DELETE FROM "user" WHERE email LIKE 'lark-bound-%@multica.ai' OR email LIKE 'lark-email-match-%@multica.ai'`,
	}
	for _, stmt := range statements {
		if _, err := testPool.Exec(ctx, stmt); err != nil {
			t.Errorf("cleanup lark login binding preference test: %v", err)
		}
	}
}
