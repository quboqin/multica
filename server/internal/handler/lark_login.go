package handler

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/integrations/lark"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const larkLoginStateTTL = 15 * time.Minute

var errLarkLoginBindingConflict = errors.New("lark user is already bound to another Multica user")

type LarkLoginStateRequest struct {
	InstallationID string `json:"installation_id"`
	Next           string `json:"next"`
	RedirectURI    string `json:"redirect_uri"`
}

type LarkLoginStateResponse struct {
	State        string `json:"state"`
	AuthorizeURL string `json:"authorize_url,omitempty"`
}

type LarkLoginRequest struct {
	Code  string `json:"code"`
	State string `json:"state"`
}

type LarkLoginResponse struct {
	Token         string       `json:"token"`
	User          UserResponse `json:"user"`
	WorkspaceID   string       `json:"workspace_id,omitempty"`
	WorkspaceSlug string       `json:"workspace_slug,omitempty"`
	Next          string       `json:"next,omitempty"`
}

type larkLoginState struct {
	InstallationID string `json:"installation_id"`
	Next           string `json:"next,omitempty"`
	Nonce          string `json:"nonce"`
	IssuedAt       int64  `json:"iat"`
	ExpiresAt      int64  `json:"exp"`
}

func (h *Handler) CreateLarkLoginState(w http.ResponseWriter, r *http.Request) {
	var req LarkLoginStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	instID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(req.InstallationID), "installation_id")
	if !ok {
		return
	}
	if h.LarkInstallations == nil {
		writeError(w, http.StatusServiceUnavailable, "lark integration not configured")
		return
	}
	inst, err := lark.NewChannelStore(h.Queries).GetLarkInstallation(r.Context(), instID)
	if err != nil {
		writeError(w, http.StatusNotFound, "lark installation not found")
		return
	}
	if inst.Status != string(lark.InstallationActive) {
		writeError(w, http.StatusConflict, "lark installation is not active")
		return
	}
	now := time.Now()
	state, err := h.signLarkLoginState(larkLoginState{
		InstallationID: uuidToString(inst.ID),
		Next:           sanitizeLarkLoginNext(req.Next),
		Nonce:          randomHex(16),
		IssuedAt:       now.Unix(),
		ExpiresAt:      now.Add(larkLoginStateTTL).Unix(),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create state")
		return
	}
	resp := LarkLoginStateResponse{State: state}
	if redirectURI := sanitizeLarkLoginRedirectURI(req.RedirectURI, r.Header.Get("Origin")); redirectURI != "" {
		resp.AuthorizeURL = buildLarkLoginAuthorizeURL(lark.RegionOrDefault(inst.Region), inst.AppID, redirectURI, state)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) LarkLogin(w http.ResponseWriter, r *http.Request) {
	var req LarkLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" || strings.TrimSpace(req.State) == "" {
		writeError(w, http.StatusBadRequest, "code and state are required")
		return
	}
	state, err := h.verifyLarkLoginState(req.State)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid or expired state")
		return
	}
	if h.LarkInstallations == nil {
		writeError(w, http.StatusServiceUnavailable, "lark integration not configured")
		return
	}
	instID, err := util.ParseUUID(state.InstallationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid state")
		return
	}
	store := lark.NewChannelStore(h.Queries)
	inst, err := store.GetLarkInstallation(r.Context(), instID)
	if err != nil {
		writeError(w, http.StatusNotFound, "lark installation not found")
		return
	}
	if inst.Status != string(lark.InstallationActive) {
		writeError(w, http.StatusConflict, "lark installation is not active")
		return
	}
	appSecret, err := h.LarkInstallations.DecryptAppSecret(inst)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load lark installation")
		return
	}
	region := lark.RegionOrDefault(inst.Region)
	info, err := lark.NewLoginClient(lark.LoginClientConfig{}, region).ExchangeCode(r.Context(), inst.AppID, appSecret, code)
	if err != nil {
		slog.Warn("lark login failed", append(logger.RequestAttrs(r), "error", err, "installation_id", state.InstallationID)...)
		writeError(w, http.StatusBadGateway, "failed to verify lark login")
		return
	}

	user, isNew, err := h.findOrCreateUserForLarkLogin(r.Context(), store, inst, string(region), info)
	if err != nil {
		var signupErr SignupError
		if errors.As(err, &signupErr) {
			writeError(w, http.StatusForbidden, signupErr.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}
	if isNew {
		evt := analytics.Signup(uuidToString(user.ID), user.Email, signupSourceFromRequest(r))
		evt.Properties["auth_method"] = "lark"
		obsmetrics.RecordEvent(h.Analytics, h.Metrics, evt)
	}
	user = h.updateUserFromLarkProfile(r.Context(), user, info)

	var workspace db.Workspace
	if ws, ok, err := h.ensureLarkLoginMembership(r.Context(), user, inst); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to join workspace")
		return
	} else if ok {
		workspace = ws
		if onboardedUser, err := h.Queries.MarkUserOnboarded(r.Context(), user.ID); err == nil {
			user = onboardedUser
		} else {
			writeError(w, http.StatusInternalServerError, "failed to mark user onboarded")
			return
		}
	}
	if err := h.bindLarkLoginUser(r.Context(), store, user.ID, inst, string(region), info); err != nil {
		if errors.Is(err, errLarkLoginBindingConflict) {
			writeError(w, http.StatusConflict, "lark user is already bound to another Multica user")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to bind lark user")
		return
	}

	tokenString, err := h.issueJWT(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}
	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, cookie)
		}
	}

	resp := LarkLoginResponse{
		Token: tokenString,
		User:  userToResponse(user),
		Next:  state.Next,
	}
	if workspace.ID.Valid {
		resp.WorkspaceID = uuidToString(workspace.ID)
		resp.WorkspaceSlug = workspace.Slug
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) findOrCreateUserForLarkLogin(ctx context.Context, store *lark.ChannelStore, inst lark.Installation, region string, info lark.LoginUserInfo) (db.User, bool, error) {
	if binding, ok, err := h.findLarkLoginBinding(ctx, store, inst.ID, info); err != nil {
		return db.User{}, false, err
	} else if ok {
		user, err := h.Queries.GetUser(ctx, binding.MulticaUserID)
		return user, false, err
	}

	if info.UnionID != "" {
		identity, err := h.Queries.GetLarkLoginIdentityByUnionID(ctx, db.GetLarkLoginIdentityByUnionIDParams{
			Region:  region,
			UnionID: info.UnionID,
		})
		if err == nil {
			user, err := h.Queries.GetUser(ctx, identity.MulticaUserID)
			return user, false, err
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return db.User{}, false, err
		}
	}

	if info.Email != "" {
		return h.findOrCreateUser(ctx, info.Email)
	}
	return db.User{}, false, SignupError{Message: "Lark account email is required for first-time signup"}
}

func (h *Handler) findLarkLoginBinding(ctx context.Context, store *lark.ChannelStore, installationID pgtype.UUID, info lark.LoginUserInfo) (lark.UserBinding, bool, error) {
	if info.OpenID != "" {
		binding, err := store.GetLarkUserBindingByOpenID(ctx, lark.GetUserBindingByOpenIDParams{
			InstallationID: installationID,
			ChannelUserID:  info.OpenID,
		})
		if err == nil {
			return binding, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return lark.UserBinding{}, false, err
		}
	}
	if info.UnionID != "" {
		binding, err := store.GetLarkUserBindingByUnionID(ctx, lark.GetUserBindingByUnionIDParams{
			InstallationID: installationID,
			UnionID:        info.UnionID,
		})
		if err == nil {
			return binding, true, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return lark.UserBinding{}, false, err
		}
	}
	return lark.UserBinding{}, false, nil
}

func (h *Handler) updateUserFromLarkProfile(ctx context.Context, user db.User, info lark.LoginUserInfo) db.User {
	needsUpdate := false
	name := user.Name
	avatar := user.AvatarUrl
	if info.Name != "" && (user.Name == user.Email || strings.HasPrefix(user.Email, user.Name+"@")) {
		name = info.Name
		needsUpdate = true
	}
	if info.AvatarURL != "" && !user.AvatarUrl.Valid {
		avatar = pgtype.Text{String: info.AvatarURL, Valid: true}
		needsUpdate = true
	}
	if !needsUpdate {
		return user
	}
	updated, err := h.Queries.UpdateUser(ctx, db.UpdateUserParams{
		ID:        user.ID,
		Name:      name,
		AvatarUrl: avatar,
	})
	if err != nil {
		return user
	}
	return updated
}

func (h *Handler) ensureLarkLoginMembership(ctx context.Context, user db.User, inst lark.Installation) (db.Workspace, bool, error) {
	var ws db.Workspace
	var err error
	if h.cfg.LarkLoginJoinBotWorkspace {
		ws, err = h.Queries.GetWorkspace(ctx, inst.WorkspaceID)
	} else if h.cfg.LarkLoginDefaultWorkspace != "" {
		ws, err = h.resolveWorkspaceByIDOrSlug(ctx, h.cfg.LarkLoginDefaultWorkspace)
	} else {
		return db.Workspace{}, false, nil
	}
	if err != nil {
		return db.Workspace{}, false, err
	}
	member, err := h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{
		UserID:      user.ID,
		WorkspaceID: ws.ID,
	})
	if err == nil {
		return ws, true, nil
	}
	if !isNotFound(err) {
		return db.Workspace{}, false, err
	}
	member, err = h.Queries.CreateMember(ctx, db.CreateMemberParams{
		WorkspaceID: ws.ID,
		UserID:      user.ID,
		Role:        "member",
	})
	if err != nil {
		if isUniqueViolation(err) {
			return ws, true, nil
		}
		return db.Workspace{}, false, err
	}
	h.publish(protocol.EventMemberAdded, uuidToString(ws.ID), "member", uuidToString(user.ID), map[string]any{
		"member":         memberWithUserResponse(member, user),
		"workspace_name": ws.Name,
	})
	return ws, true, nil
}

func (h *Handler) bindLarkLoginUser(ctx context.Context, store *lark.ChannelStore, userID pgtype.UUID, inst lark.Installation, region string, info lark.LoginUserInfo) error {
	_, err := store.CreateLarkUserBinding(ctx, lark.CreateUserBindingParams{
		WorkspaceID:    inst.WorkspaceID,
		MulticaUserID:  userID,
		InstallationID: inst.ID,
		ChannelUserID:  info.OpenID,
		UnionID:        pgtype.Text{String: info.UnionID, Valid: info.UnionID != ""},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errLarkLoginBindingConflict
		}
		return err
	}

	if info.UnionID == "" {
		return nil
	}
	if _, err := h.Queries.UpsertLarkLoginIdentity(ctx, db.UpsertLarkLoginIdentityParams{
		Region:        region,
		UnionID:       info.UnionID,
		OpenID:        pgtype.Text{String: info.OpenID, Valid: info.OpenID != ""},
		MulticaUserID: userID,
		Name:          pgtype.Text{String: info.Name, Valid: info.Name != ""},
		Email:         pgtype.Text{String: info.Email, Valid: info.Email != ""},
		AvatarUrl:     pgtype.Text{String: info.AvatarURL, Valid: info.AvatarURL != ""},
	}); err != nil {
		return err
	}
	return nil
}

func (h *Handler) resolveWorkspaceByIDOrSlug(ctx context.Context, value string) (db.Workspace, error) {
	value = strings.TrimSpace(value)
	if id, err := util.ParseUUID(value); err == nil {
		return h.Queries.GetWorkspace(ctx, id)
	}
	return h.Queries.GetWorkspaceBySlug(ctx, strings.ToLower(value))
}

func (h *Handler) signLarkLoginState(state larkLoginState) (string, error) {
	body, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	bodyB64 := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, h.larkLoginStateSecret())
	mac.Write([]byte(bodyB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return bodyB64 + "." + sig, nil
}

func (h *Handler) verifyLarkLoginState(raw string) (larkLoginState, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return larkLoginState{}, errors.New("bad state")
	}
	mac := hmac.New(sha256.New, h.larkLoginStateSecret())
	mac.Write([]byte(parts[0]))
	want := mac.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || subtle.ConstantTimeCompare(want, got) != 1 {
		return larkLoginState{}, errors.New("bad state signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return larkLoginState{}, err
	}
	var state larkLoginState
	if err := json.Unmarshal(body, &state); err != nil {
		return larkLoginState{}, err
	}
	now := time.Now().Unix()
	if state.ExpiresAt < now || state.IssuedAt > now+60 || state.InstallationID == "" {
		return larkLoginState{}, errors.New("expired state")
	}
	return state, nil
}

func (h *Handler) larkLoginStateSecret() []byte {
	if h.cfg.LarkLoginStateSecret != "" {
		return []byte(h.cfg.LarkLoginStateSecret)
	}
	return auth.JWTSecret()
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	}
	return hex.EncodeToString(buf)
}

func sanitizeLarkLoginNext(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	return raw
}

func sanitizeLarkLoginRedirectURI(raw, origin string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || !u.IsAbs() || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	if u.Path != "/lark/start" {
		return ""
	}
	origin = strings.TrimSpace(origin)
	if origin != "" {
		originURL, err := url.Parse(origin)
		if err != nil || originURL.Scheme != u.Scheme || originURL.Host != u.Host {
			return ""
		}
	}
	return u.String()
}

func buildLarkLoginAuthorizeURL(region lark.Region, appID, redirectURI, state string) string {
	u, _ := url.Parse(region.OpenPlatformBaseURL() + "/open-apis/authen/v1/index")
	q := u.Query()
	q.Set("app_id", appID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	return u.String()
}
