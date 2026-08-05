package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/broker"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type credentialProfileResponse struct {
	ID          string                             `json:"id"`
	ConnectorID string                             `json:"connector_id"`
	Label       string                             `json:"label"`
	Status      string                             `json:"status"`
	Scope       string                             `json:"scope"`
	CanManage   bool                               `json:"can_manage"`
	Managers    []credentialProfileManagerResponse `json:"managers"`
	LastUsedAt  *string                            `json:"last_used_at,omitempty"`
	ExpiresHint *string                            `json:"expires_hint,omitempty"`
	CreatedAt   string                             `json:"created_at"`
	UpdatedAt   string                             `json:"updated_at"`
}

type credentialProfileManagerResponse struct {
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
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
	workspaceID, userID, ok := h.credentialWorkspaceScope(w, r, false)
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
		response, err := h.credentialProfileResponseForUser(r.Context(), profile, userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out = append(out, response)
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (h *Handler) GetCredentialProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.credentialWorkspaceScope(w, r, false)
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
	response, err := h.credentialProfileResponseForUser(r.Context(), profile, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteCredentialProfile(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.credentialWorkspaceScope(w, r, false)
	if !ok {
		return
	}
	profileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "profile_id")
	if !ok {
		return
	}
	profile, err := h.CredentialBroker.RevokeProfile(r.Context(), workspaceID, userID, profileID)
	if err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credentialProfileToResponse(profile))
}

func (h *Handler) CreateCredentialLoginSession(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.credentialWorkspaceScope(w, r, false)
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
	if !profileID.Valid {
		profiles, err := h.CredentialBroker.ListProfiles(r.Context(), workspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hasConnectorProfile := false
		for _, profile := range profiles {
			if profile.ConnectorID == strings.TrimSpace(req.ConnectorID) {
				hasConnectorProfile = true
				break
			}
		}
		if !hasConnectorProfile {
			workspaceIDRaw := ctxWorkspaceID(r.Context())
			if workspaceIDRaw == "" {
				workspaceIDRaw = h.resolveWorkspaceID(r)
			}
			if _, ok := h.requireWorkspaceRole(w, r, workspaceIDRaw, "workspace not found", "owner", "admin"); !ok {
				return
			}
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

func (h *Handler) AddCredentialProfileManager(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, ok := h.credentialWorkspaceScope(w, r, false)
	if !ok {
		return
	}
	profileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "profile_id")
	if !ok {
		return
	}
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	targetID, ok := parseUUIDOrBadRequest(w, req.UserID, "user_id")
	if !ok {
		return
	}
	if _, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      targetID,
		WorkspaceID: workspaceID,
	}); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusUnprocessableEntity, "credential manager must be a member of the selected workspace")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.CredentialBroker.AddProfileManager(r.Context(), workspaceID, actorID, profileID, targetID); err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	profile, err := h.CredentialBroker.GetProfile(r.Context(), workspaceID, profileID)
	if err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	response, err := h.credentialProfileResponseForUser(r.Context(), profile, actorID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) DeleteCredentialProfileManager(w http.ResponseWriter, r *http.Request) {
	workspaceID, actorID, ok := h.credentialWorkspaceScope(w, r, false)
	if !ok {
		return
	}
	profileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "profile_id")
	if !ok {
		return
	}
	targetID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "userId"), "user_id")
	if !ok {
		return
	}
	if err := h.CredentialBroker.RemoveProfileManager(r.Context(), workspaceID, actorID, profileID, targetID); err != nil {
		writeCredentialBrokerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	var issue db.Issue
	hasIssue := false
	if strings.TrimSpace(req.IssueID) != "" {
		var ok bool
		issue, ok = h.loadIssueForUser(w, r, req.IssueID)
		if !ok {
			return
		}
		if issue.WorkspaceID != workspaceID {
			writeError(w, http.StatusUnprocessableEntity, "issue must belong to the selected workspace")
			return
		}
		hasIssue = true
		req.Params = h.materialSearchParamsWithStrategyMemory(r.Context(), issue, req.ConnectorID, req.Capability, req.Params)
		var noveltyErr error
		req.Params, noveltyErr = h.materialSearchParamsWithNovelty(r.Context(), issue.WorkspaceID, req.ConnectorID, req.Capability, req.Params)
		if noveltyErr != nil {
			slog.Warn("load creative material novelty exclusions failed", append(logger.RequestAttrs(r), "error", noveltyErr, "issue_id", req.IssueID)...)
			writeError(w, http.StatusInternalServerError, "failed to prepare material novelty filter")
			return
		}
	}
	importInput := creativeMaterialImportInput{
		WorkspaceID:    workspaceID,
		AutopilotRunID: h.autopilotRunIDFromCurrentTask(r, actorType),
		ConnectorID:    req.ConnectorID,
		QuerySummary:   req.Capability,
		Params:         req.Params,
		ActorType:      actorType,
		ActorID:        actorID,
		UserID:         requestingUserID,
	}
	if hasIssue {
		importInput.IssueID = issue.ID
		importInput.WorkspaceID = issue.WorkspaceID
	}

	// Keep a run record even when broker-side credential resolution fails.
	// Successful import adopts the same record rather than inserting another.
	crawlRunID := ""
	if strings.TrimSpace(req.Capability) == "material_search" {
		var runErr error
		crawlRunID, runErr = h.startCreativeMaterialCrawlRun(r.Context(), importInput)
		if runErr != nil {
			slog.Warn("create credential crawl run failed", append(logger.RequestAttrs(r), "error", runErr)...)
			writeError(w, http.StatusInternalServerError, "failed to create crawl run")
			return
		}
		importInput.RunID = crawlRunID
	}

	result, err := h.CredentialBroker.RunCrawl(r.Context(), broker.RunCrawlInput{
		WorkspaceID:      workspaceID,
		RequestingUserID: requestingUserID,
		ProfileID:        profileID,
		ConnectorID:      req.ConnectorID,
		Capability:       req.Capability,
		Params:           req.Params,
	})
	if err != nil {
		failureStatus := "failed"
		if credentialCrawlRequiresLogin(err) {
			failureStatus = "action_required"
		}
		h.failCreativeMaterialCrawlRun(r.Context(), workspaceID, crawlRunID, failureStatus, credentialCrawlErrorCode(err), err.Error())
		writeCredentialCrawlError(w, err, crawlRunID)
		return
	}
	if result.Status == broker.StatusNeedReauth {
		message := "credential authorization expired; reconnect and retry this crawl"
		h.failCreativeMaterialCrawlRun(r.Context(), workspaceID, crawlRunID, "action_required", "credential_reauth_required", message)
		writeJSON(w, http.StatusConflict, map[string]any{
			"status":       result.Status,
			"message":      message,
			"crawl_run_id": crawlRunID,
		})
		return
	}
	materials := creativeMaterialsFromCrawlRaw(result.Raw)
	importInput.Materials = materials
	importSummary, err := h.importCreativeMaterials(r.Context(), importInput)
	if err != nil {
		slog.Warn("credential crawl creative import failed", append(logger.RequestAttrs(r), "error", err, "issue_id", req.IssueID)...)
		h.failCreativeMaterialCrawlRun(r.Context(), workspaceID, crawlRunID, "failed", "creative_material_import_failed", "crawl completed but failed to import creative materials")
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":        "crawl completed but failed to import creative materials",
			"crawl_run_id": crawlRunID,
		})
		return
	}
	if hasIssue {
		if err := h.recordCreativeMaterialCrawlStrategyMemory(r.Context(), issue, req.ConnectorID, req.Capability, result.Raw, importSummary.RunID); err != nil {
			slog.Warn("credential crawl strategy memory update failed", append(logger.RequestAttrs(r), "error", err, "issue_id", req.IssueID)...)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":             result.Status,
		"downloaded":         result.Downloaded,
		"output_prefix":      result.OutputPrefix,
		"message":            result.Message,
		"raw":                result.Raw,
		"crawl_run_id":       firstNonEmpty(crawlRunID, importSummary.RunID),
		"analysis_agent_id":  creativeCrawlAnalysisAgentID(req.Params),
		"creative_materials": importSummary,
	})
}

func creativeCrawlAnalysisAgentID(params json.RawMessage) string {
	var values struct {
		AnalysisAgentID string `json:"analysis_agent_id"`
	}
	if json.Unmarshal(params, &values) != nil {
		return ""
	}
	return strings.TrimSpace(values.AnalysisAgentID)
}

func credentialCrawlRequiresLogin(err error) bool {
	return errors.Is(err, broker.ErrProfileNotActive) || isNotFound(err)
}

func credentialCrawlErrorCode(err error) string {
	switch {
	case errors.Is(err, broker.ErrProfileNotActive):
		return "credential_not_active"
	case errors.Is(err, broker.ErrProfileRequired):
		return "credential_required"
	case isNotFound(err):
		return "credential_not_found"
	case errors.Is(err, broker.ErrWorkerTimeout):
		return "worker_timeout"
	case errors.Is(err, broker.ErrWorkerBusy):
		return "worker_busy"
	case errors.Is(err, broker.ErrWorkerUnavailable):
		return "worker_unavailable"
	default:
		return "credential_crawl_failed"
	}
}

func writeCredentialCrawlError(w http.ResponseWriter, err error, crawlRunID string) {
	status := credentialBrokerErrorStatus(err)
	message := err.Error()
	if isNotFound(err) {
		message = "credential resource not found"
	}
	if strings.TrimSpace(crawlRunID) == "" {
		writeError(w, status, message)
		return
	}
	writeJSON(w, status, map[string]string{"error": message, "crawl_run_id": crawlRunID})
}

func (h *Handler) autopilotRunIDFromCurrentTask(r *http.Request, actorType string) pgtype.UUID {
	if actorType != "agent" {
		return pgtype.UUID{}
	}
	taskID, err := uuid.Parse(strings.TrimSpace(r.Header.Get("X-Task-ID")))
	if err != nil {
		return pgtype.UUID{}
	}
	task, err := h.Queries.GetAgentTask(r.Context(), parseUUID(taskID.String()))
	if err != nil {
		return pgtype.UUID{}
	}
	return task.AutopilotRunID
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
	status := credentialBrokerErrorStatus(err)
	if isNotFound(err) {
		writeError(w, status, "credential resource not found")
		return
	}
	writeError(w, status, err.Error())
}

func credentialBrokerErrorStatus(err error) int {
	switch {
	case errors.Is(err, broker.ErrProfileRequired):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrConnectorUnknown):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrProfileConnectorMismatch):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrUnsafeParams):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrProfileNotActive):
		return http.StatusConflict
	case errors.Is(err, broker.ErrDeploymentProfileBindingConflict):
		return http.StatusConflict
	case errors.Is(err, broker.ErrProfileManageForbidden):
		return http.StatusForbidden
	case errors.Is(err, broker.ErrLastProfileManager):
		return http.StatusConflict
	case errors.Is(err, broker.ErrWorkerNotConfigured):
		return http.StatusServiceUnavailable
	case errors.Is(err, broker.ErrWorkerRequestInvalid):
		return http.StatusBadRequest
	case errors.Is(err, broker.ErrWorkerBusy):
		return http.StatusTooManyRequests
	case errors.Is(err, broker.ErrWorkerTimeout):
		return http.StatusGatewayTimeout
	case errors.Is(err, broker.ErrWorkerUnavailable):
		return http.StatusServiceUnavailable
	case isNotFound(err):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

func credentialProfileToResponse(profile db.CredentialProfile) credentialProfileResponse {
	return credentialProfileResponse{
		ID:          uuidToString(profile.ID),
		ConnectorID: profile.ConnectorID,
		Label:       profile.Label,
		Status:      profile.Status,
		Scope:       profile.Scope,
		Managers:    []credentialProfileManagerResponse{},
		LastUsedAt:  timestampToPtr(profile.LastUsedAt),
		ExpiresHint: timestampToPtr(profile.ExpiresHint),
		CreatedAt:   timestampToString(profile.CreatedAt),
		UpdatedAt:   timestampToString(profile.UpdatedAt),
	}
}

func (h *Handler) credentialProfileResponseForUser(ctx context.Context, profile db.CredentialProfile, userID pgtype.UUID) (credentialProfileResponse, error) {
	response := credentialProfileToResponse(profile)
	canManage, err := h.CredentialBroker.CanManageProfile(ctx, profile.ID, userID)
	if err != nil {
		return credentialProfileResponse{}, err
	}
	response.CanManage = canManage
	// Deployment-scoped profiles are visible in every workspace. Manager names
	// and email addresses are cross-workspace identity data, so return them
	// only to an explicit profile manager.
	if !canManage {
		return response, nil
	}
	managers, err := h.CredentialBroker.ListProfileManagers(ctx, profile.ID)
	if err != nil {
		return credentialProfileResponse{}, err
	}
	for _, manager := range managers {
		response.Managers = append(response.Managers, credentialProfileManagerResponse{
			UserID:    uuidToString(manager.UserID),
			Name:      manager.Name,
			Email:     manager.Email,
			CreatedAt: timestampToString(manager.CreatedAt),
		})
	}
	return response, nil
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
