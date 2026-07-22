package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/broker"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type credentialProfileResponse struct {
	ID          string  `json:"id"`
	ConnectorID string  `json:"connector_id"`
	Label       string  `json:"label"`
	Status      string  `json:"status"`
	LastUsedAt  *string `json:"last_used_at,omitempty"`
	ExpiresHint *string `json:"expires_hint,omitempty"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type credentialLoginSessionResponse struct {
	ID          string `json:"id"`
	ProfileID   string `json:"profile_id"`
	ConnectorID string `json:"connector_id"`
	BrowserURL  string `json:"browser_url"`
	Status      string `json:"status"`
	ExpiresAt   string `json:"expires_at"`
	CreatedAt   string `json:"created_at"`
}

func (h *Handler) ListCredentialConnectors(w http.ResponseWriter, r *http.Request) {
	if h.CredentialBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "credential broker not configured")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connectors": h.CredentialBroker.ListConnectors()})
}

func (h *Handler) ListCredentialProfiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.credentialWorkspaceScope(w, r, false)
	if !ok {
		return
	}
	profiles, err := h.CredentialBroker.ListProfiles(r.Context(), workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]credentialProfileResponse, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, credentialProfileToResponse(profile))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (h *Handler) GetCredentialProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.credentialWorkspaceScope(w, r, false)
	if !ok {
		return
	}
	profileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "profile_id")
	if !ok {
		return
	}
	profile, err := h.CredentialBroker.GetProfile(r.Context(), workspaceID, profileID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "credential profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, credentialProfileToResponse(profile))
}

func (h *Handler) DeleteCredentialProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.credentialWorkspaceScope(w, r, true)
	if !ok {
		return
	}
	profileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "profile_id")
	if !ok {
		return
	}
	profile, err := h.CredentialBroker.RevokeProfile(r.Context(), workspaceID, profileID)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "credential profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, credentialProfileToResponse(profile))
}

func (h *Handler) CreateCredentialLoginSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.credentialWorkspaceScope(w, r, true)
	if !ok {
		return
	}
	var req struct {
		ConnectorID string `json:"connector_id"`
		ProfileID   string `json:"profile_id"`
		Label       string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var profileID pgtype.UUID
	if strings.TrimSpace(req.ProfileID) != "" {
		var ok bool
		profileID, ok = parseUUIDOrBadRequest(w, req.ProfileID, "profile_id")
		if !ok {
			return
		}
	}
	result, err := h.CredentialBroker.StartLoginSession(r.Context(), broker.StartLoginSessionInput{
		WorkspaceID: workspaceID,
		UserID:      userID,
		ProfileID:   profileID,
		ConnectorID: req.ConnectorID,
		Label:       req.Label,
	})
	if err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"profile": credentialProfileToResponse(result.Profile),
		"session": credentialLoginSessionToResponse(result.Session),
	})
}

func (h *Handler) CompleteCredentialLoginSession(w http.ResponseWriter, r *http.Request) {
	if h.CredentialBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "credential broker not configured")
		return
	}
	var req struct {
		SessionToken string  `json:"session_token"`
		Ciphertext   string  `json:"ciphertext"`
		KeyVersion   string  `json:"key_version"`
		ExpiresHint  *string `json:"expires_hint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.SessionToken) == "" {
		writeError(w, http.StatusBadRequest, "session_token is required")
		return
	}
	ciphertext, err := broker.DecodeCiphertextBase64(req.Ciphertext)
	if err != nil || len(ciphertext) == 0 {
		writeError(w, http.StatusBadRequest, "ciphertext must be non-empty base64")
		return
	}
	var expiresHint pgtype.Timestamptz
	if req.ExpiresHint != nil && strings.TrimSpace(*req.ExpiresHint) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExpiresHint))
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_hint must be RFC3339")
			return
		}
		expiresHint = pgtype.Timestamptz{Time: t, Valid: true}
	}
	result, err := h.CredentialBroker.CompleteLoginSession(r.Context(), broker.CompleteLoginSessionInput{
		SessionToken: req.SessionToken,
		Ciphertext:   ciphertext,
		KeyVersion:   req.KeyVersion,
		ExpiresHint:  expiresHint,
	})
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "login session not found or expired")
			return
		}
		writeCredentialBrokerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"profile": credentialProfileToResponse(result.Profile),
		"session": credentialLoginSessionToResponse(result.Session),
	})
}

func (h *Handler) RunCredentialCrawl(w http.ResponseWriter, r *http.Request) {
	userIDRaw, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceIDRaw := ctxWorkspaceID(r.Context())
	if workspaceIDRaw == "" {
		workspaceIDRaw = h.resolveWorkspaceID(r)
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, workspaceIDRaw, "workspace_id")
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userIDRaw, workspaceIDRaw)
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	if !requestingUserID.Valid {
		writeError(w, http.StatusForbidden, "requesting user not available")
		return
	}
	var req struct {
		ProfileID   string          `json:"profile_id"`
		IssueID     string          `json:"issue_id"`
		ConnectorID string          `json:"connector_id"`
		Capability  string          `json:"capability"`
		Params      json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	var profileID pgtype.UUID
	if strings.TrimSpace(req.ProfileID) != "" {
		var ok bool
		profileID, ok = parseUUIDOrBadRequest(w, req.ProfileID, "profile_id")
		if !ok {
			return
		}
	}
	if len(req.Params) == 0 {
		req.Params = json.RawMessage(`{}`)
	}
	result, err := h.CredentialBroker.RunCrawl(r.Context(), broker.RunCrawlInput{
		WorkspaceID: workspaceID,
		ProfileID:   profileID,
		ConnectorID: req.ConnectorID,
		Capability:  req.Capability,
		Params:      req.Params,
	})
	if err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	if strings.TrimSpace(req.IssueID) != "" {
		issue, ok := h.loadIssueForUser(w, r, req.IssueID)
		if !ok {
			return
		}
		materials := creativeMaterialsFromCrawlRaw(result.Raw)
		importSummary, err := h.importCreativeMaterialsForIssue(r.Context(), creativeMaterialImportInput{
			IssueID:      issue.ID,
			WorkspaceID:  issue.WorkspaceID,
			ConnectorID:  req.ConnectorID,
			QuerySummary: req.Capability,
			Params:       req.Params,
			Materials:    materials,
			ActorType:    actorType,
			ActorID:      actorID,
			UserID:       requestingUserID,
		})
		if err != nil {
			slog.Warn("credential crawl creative import failed", append(logger.RequestAttrs(r), "error", err, "issue_id", req.IssueID)...)
			writeError(w, http.StatusInternalServerError, "crawl completed but failed to import creative materials")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":             result.Status,
			"downloaded":         result.Downloaded,
			"output_prefix":      result.OutputPrefix,
			"message":            result.Message,
			"raw":                result.Raw,
			"creative_materials": importSummary,
		})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) credentialWorkspaceScope(w http.ResponseWriter, r *http.Request, manage bool) (pgtype.UUID, pgtype.UUID, bool) {
	if h.CredentialBroker == nil {
		writeError(w, http.StatusServiceUnavailable, "credential broker not configured")
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	userIDRaw, ok := requireUserID(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	userID, ok := parseUUIDOrBadRequest(w, userIDRaw, "user_id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	workspaceIDRaw := ctxWorkspaceID(r.Context())
	if workspaceIDRaw == "" {
		workspaceIDRaw = h.resolveWorkspaceID(r)
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, workspaceIDRaw, "workspace_id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	roles := []string{"owner", "admin", "member"}
	if manage {
		roles = []string{"owner", "admin"}
	}
	if _, ok := h.requireWorkspaceRole(w, r, workspaceIDRaw, "workspace not found", roles...); !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	return workspaceID, userID, true
}

func writeCredentialBrokerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, broker.ErrProfileRequired):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, broker.ErrConnectorUnknown):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, broker.ErrProfileConnectorMismatch):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, broker.ErrUnsafeParams):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, broker.ErrProfileNotActive):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, broker.ErrWorkerNotConfigured):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case isNotFound(err):
		writeError(w, http.StatusNotFound, "credential resource not found")
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func credentialProfileToResponse(profile db.CredentialProfile) credentialProfileResponse {
	return credentialProfileResponse{
		ID:          uuidToString(profile.ID),
		ConnectorID: profile.ConnectorID,
		Label:       profile.Label,
		Status:      profile.Status,
		LastUsedAt:  timestampToPtr(profile.LastUsedAt),
		ExpiresHint: timestampToPtr(profile.ExpiresHint),
		CreatedAt:   timestampToString(profile.CreatedAt),
		UpdatedAt:   timestampToString(profile.UpdatedAt),
	}
}

func credentialLoginSessionToResponse(session db.CredentialLoginSession) credentialLoginSessionResponse {
	return credentialLoginSessionResponse{
		ID:          uuidToString(session.ID),
		ProfileID:   uuidToString(session.ProfileID),
		ConnectorID: session.ConnectorID,
		BrowserURL:  session.BrowserUrl,
		Status:      session.Status,
		ExpiresAt:   timestampToString(session.ExpiresAt),
		CreatedAt:   timestampToString(session.CreatedAt),
	}
}
