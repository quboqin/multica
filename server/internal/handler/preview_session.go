package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const (
	previewPlatformWeb         = "web"
	previewPlatformAndroid     = "android"
	previewProviderExternal    = "external_web"
	previewProviderLocalDevice = "local_device"
	maxPreviewTitleRunes       = 120
	maxPreviewURLBytes         = 4096
	maxPreviewRequestBytes     = 16 << 10
	previewDeviceLeaseDuration = 5 * time.Minute
	previewDeviceIdleDuration  = 20 * time.Minute
	previewDeviceMaxDuration   = 4 * time.Hour
)

// PreviewSessionResponse is the wire shape shared by preview-session endpoints.
type PreviewSessionResponse struct {
	ID             string  `json:"id"`
	WorkspaceID    string  `json:"workspace_id"`
	IssueID        string  `json:"issue_id"`
	TaskID         *string `json:"task_id"`
	Platform       string  `json:"platform"`
	Provider       string  `json:"provider"`
	Title          string  `json:"title"`
	PreviewURL     string  `json:"preview_url"`
	Status         string  `json:"status"`
	CreatorType    string  `json:"creator_type"`
	CreatorID      string  `json:"creator_id"`
	ErrorMessage   *string `json:"error_message"`
	ExpiresAt      *string `json:"expires_at"`
	LastActiveAt   *string `json:"last_active_at"`
	LeaseExpiresAt *string `json:"lease_expires_at"`
	StartedAt      *string `json:"started_at"`
	StoppedAt      *string `json:"stopped_at"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
}

func isActivePreviewSessionStatus(status string) bool {
	switch status {
	case "creating", "starting", "running", "sleeping", "stopping":
		return true
	default:
		return false
	}
}

func previewSessionToResponseAt(session db.PreviewSession, now time.Time) PreviewSessionResponse {
	status := session.Status
	if session.ExpiresAt.Valid && !session.ExpiresAt.Time.After(now) && isActivePreviewSessionStatus(status) {
		status = "expired"
	} else if session.Provider == previewProviderLocalDevice && status == "running" && session.LastActiveAt.Valid && !session.LastActiveAt.Time.After(now.Add(-previewDeviceIdleDuration)) {
		status = "sleeping"
	}

	return PreviewSessionResponse{
		ID:             uuidToString(session.ID),
		WorkspaceID:    uuidToString(session.WorkspaceID),
		IssueID:        uuidToString(session.IssueID),
		TaskID:         uuidToPtr(session.TaskID),
		Platform:       session.Platform,
		Provider:       session.Provider,
		Title:          session.Title,
		PreviewURL:     session.PreviewUrl,
		Status:         status,
		CreatorType:    session.CreatorType,
		CreatorID:      uuidToString(session.CreatorID),
		ErrorMessage:   textToPtr(session.ErrorMessage),
		ExpiresAt:      timestampToPtr(session.ExpiresAt),
		LastActiveAt:   timestampToPtr(session.LastActiveAt),
		LeaseExpiresAt: timestampToPtr(session.LeaseExpiresAt),
		StartedAt:      timestampToPtr(session.StartedAt),
		StoppedAt:      timestampToPtr(session.StoppedAt),
		CreatedAt:      timestampToString(session.CreatedAt),
		UpdatedAt:      timestampToString(session.UpdatedAt),
	}
}

func previewSessionToResponse(session db.PreviewSession) PreviewSessionResponse {
	return previewSessionToResponseAt(session, time.Now())
}

type CreatePreviewSessionRequest struct {
	Platform   string  `json:"platform"`
	Provider   string  `json:"provider"`
	Title      string  `json:"title"`
	PreviewURL string  `json:"preview_url"`
	ExpiresAt  *string `json:"expires_at"`
}

func normalizeExternalPreviewURL(raw string) (normalized, defaultTitle string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("preview_url is required")
	}
	if len(raw) > maxPreviewURLBytes {
		return "", "", errors.New("preview_url must be at most 4096 bytes")
	}

	parsed, err := url.Parse(raw)
	if err != nil || !parsed.IsAbs() || parsed.Hostname() == "" {
		return "", "", errors.New("preview_url must be an absolute URL with a host")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", "", errors.New("preview_url must use http or https")
	}
	if parsed.User != nil {
		return "", "", errors.New("preview_url must not contain user credentials")
	}

	return parsed.String(), parsed.Hostname(), nil
}

func normalizeLocalDevicePreviewURL(raw string) (string, error) {
	normalized, _, err := normalizeExternalPreviewURL(raw)
	if err != nil {
		return "", err
	}
	parsed, _ := url.Parse(normalized)
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return "", errors.New("local device preview_url must use a loopback host")
	}
	serials := parsed.Query()["serial"]
	if len(serials) != 1 || strings.TrimSpace(serials[0]) == "" {
		return "", errors.New("local device preview_url must pin exactly one device serial")
	}
	return normalized, nil
}

func parsePreviewExpiry(raw *string) (pgtype.Timestamptz, error) {
	if raw == nil {
		return pgtype.Timestamptz{}, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return pgtype.Timestamptz{}, errors.New("expires_at must be an RFC3339 timestamp")
	}
	expiresAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return pgtype.Timestamptz{}, errors.New("expires_at must be an RFC3339 timestamp")
	}
	if !expiresAt.After(time.Now()) {
		return pgtype.Timestamptz{}, errors.New("expires_at must be in the future")
	}
	return pgtype.Timestamptz{Time: expiresAt, Valid: true}, nil
}

func applyLocalDevicePreviewExpiry(expiresAt pgtype.Timestamptz, now time.Time) pgtype.Timestamptz {
	maximum := now.Add(previewDeviceMaxDuration)
	if !expiresAt.Valid || expiresAt.Time.After(maximum) {
		return pgtype.Timestamptz{Time: maximum, Valid: true}
	}
	return expiresAt
}

// ListPreviewSessions returns every preview session attached to an issue.
func (h *Handler) ListPreviewSessions(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	sessions, err := h.Queries.ListPreviewSessionsByIssue(r.Context(), db.ListPreviewSessionsByIssueParams{
		IssueID:     issue.ID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		slog.Error("failed to list preview sessions", "issue_id", uuidToString(issue.ID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list preview sessions")
		return
	}

	response := make([]PreviewSessionResponse, len(sessions))
	for i, session := range sessions {
		response[i] = previewSessionToResponse(session)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"preview_sessions": response,
		"total":            len(response),
	})
}

// CreatePreviewSession registers an already-running external Web preview.
func (h *Handler) CreatePreviewSession(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var req CreatePreviewSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPreviewRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	platform := req.Platform
	provider := req.Provider
	var previewURL, defaultTitle string
	var err error
	switch {
	case platform == previewPlatformWeb && (provider == "" || provider == previewProviderExternal):
		platform = previewPlatformWeb
		provider = previewProviderExternal
		previewURL, defaultTitle, err = normalizeExternalPreviewURL(req.PreviewURL)
	case platform == previewPlatformAndroid && provider == previewProviderLocalDevice:
		previewURL, err = normalizeLocalDevicePreviewURL(req.PreviewURL)
		defaultTitle = "Local Android device"
	default:
		writeError(w, http.StatusBadRequest, "unsupported preview platform or provider")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = defaultTitle
	}
	if utf8.RuneCountInString(title) > maxPreviewTitleRunes {
		writeError(w, http.StatusBadRequest, "title must be at most 120 characters")
		return
	}
	expiresAt, err := parsePreviewExpiry(req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := time.Now()
	if provider == previewProviderLocalDevice {
		expiresAt = applyLocalDevicePreviewExpiry(expiresAt, now)
	}

	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := uuidToString(issue.WorkspaceID)
	creatorType, creatorID := h.resolveActor(r, userID, workspaceID)
	creatorUUID := parseUUID(creatorID)

	var taskID pgtype.UUID
	if r.Header.Get("X-Actor-Source") == "task_token" {
		taskIDHeader := strings.TrimSpace(r.Header.Get("X-Task-ID"))
		if taskIDHeader == "" {
			writeError(w, http.StatusBadRequest, "task_id is required for task token requests")
			return
		}
		taskID = parseUUID(taskIDHeader)
		task, err := h.Queries.GetAgentTask(r.Context(), taskID)
		if err != nil || !task.IssueID.Valid || uuidToString(task.IssueID) != uuidToString(issue.ID) || uuidToString(task.AgentID) != creatorID {
			writeError(w, http.StatusForbidden, "task does not belong to this issue")
			return
		}
	}

	params := db.CreatePreviewSessionParams{
		WorkspaceID:  issue.WorkspaceID,
		IssueID:      issue.ID,
		Platform:     platform,
		Provider:     provider,
		Title:        title,
		PreviewUrl:   previewURL,
		CreatorType:  creatorType,
		CreatorID:    creatorUUID,
		TaskID:       taskID,
		ExpiresAt:    expiresAt,
		LastActiveAt: pgtype.Timestamptz{Time: now, Valid: true},
	}
	if provider == previewProviderLocalDevice {
		params.LeaseExpiresAt = pgtype.Timestamptz{Time: now.Add(previewDeviceLeaseDuration), Valid: true}
	}

	var session db.PreviewSession
	if provider == previewProviderLocalDevice {
		tx, txErr := h.TxStarter.Begin(r.Context())
		if txErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to create preview session")
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		qtx := h.Queries.WithTx(tx)
		if err := qtx.LockPreviewDeviceResource(r.Context(), previewDeviceLockKey(workspaceID, previewURL)); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to acquire preview device lease")
			return
		}
		conflict, conflictErr := qtx.GetConflictingPreviewDeviceLease(r.Context(), db.GetConflictingPreviewDeviceLeaseParams{
			WorkspaceID: issue.WorkspaceID,
			PreviewUrl:  previewURL,
			ID:          pgtype.UUID{Valid: true},
		})
		if conflictErr == nil {
			writeError(w, http.StatusConflict, "device is leased by issue "+uuidToString(conflict.IssueID))
			return
		}
		if !errors.Is(conflictErr, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to inspect preview device lease")
			return
		}
		if err := qtx.SleepOtherPreviewDeviceSessions(r.Context(), db.SleepOtherPreviewDeviceSessionsParams{
			WorkspaceID: issue.WorkspaceID,
			PreviewUrl:  previewURL,
			ID:          pgtype.UUID{Valid: true},
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to release stale preview device leases")
			return
		}
		session, err = qtx.CreatePreviewSession(r.Context(), params)
		if err == nil {
			err = tx.Commit(r.Context())
		}
	} else {
		session, err = h.Queries.CreatePreviewSession(r.Context(), params)
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create preview session")
		return
	}

	response := previewSessionToResponse(session)
	h.publish(protocol.EventPreviewSessionCreated, workspaceID, creatorType, creatorID, map[string]any{
		"preview_session": response,
		"issue_id":        response.IssueID,
	})
	writeJSON(w, http.StatusCreated, response)
}

// TouchPreviewSession renews the exclusive writer lease for a local device
// preview and wakes a sleeping session. Preview history remains durable when
// the lease or idle window ends.
func (h *Handler) TouchPreviewSession(w http.ResponseWriter, r *http.Request) {
	session, ok := h.loadPreviewSession(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	if session.Provider != previewProviderLocalDevice {
		writeJSON(w, http.StatusOK, previewSessionToResponse(session))
		return
	}
	actorType, actorID, ok := h.authorizePreviewTouch(w, r, session)
	if !ok {
		return
	}
	current := previewSessionToResponse(session)
	if current.Status == "expired" || current.Status == "stopped" || current.Status == "failed" {
		writeError(w, http.StatusConflict, "preview session is "+current.Status)
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to renew preview device lease")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	qtx := h.Queries.WithTx(tx)
	workspaceID := uuidToString(session.WorkspaceID)
	if err := qtx.LockPreviewDeviceResource(r.Context(), previewDeviceLockKey(workspaceID, session.PreviewUrl)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to renew preview device lease")
		return
	}
	conflict, conflictErr := qtx.GetConflictingPreviewDeviceLease(r.Context(), db.GetConflictingPreviewDeviceLeaseParams{
		WorkspaceID: session.WorkspaceID,
		PreviewUrl:  session.PreviewUrl,
		ID:          session.ID,
	})
	if conflictErr == nil {
		writeError(w, http.StatusConflict, "device is leased by issue "+uuidToString(conflict.IssueID)+" until "+timestampToString(conflict.LeaseExpiresAt))
		return
	}
	if !errors.Is(conflictErr, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to inspect preview device lease")
		return
	}
	if err := qtx.SleepOtherPreviewDeviceSessions(r.Context(), db.SleepOtherPreviewDeviceSessionsParams{
		WorkspaceID: session.WorkspaceID,
		PreviewUrl:  session.PreviewUrl,
		ID:          session.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to release stale preview device leases")
		return
	}
	updated, err := qtx.TouchPreviewDeviceSession(r.Context(), db.TouchPreviewDeviceSessionParams{
		ID:          session.ID,
		WorkspaceID: session.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "preview session is unavailable")
		return
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to renew preview device lease")
		return
	}
	response := previewSessionToResponse(updated)
	if current.Status == "sleeping" {
		h.publish(protocol.EventPreviewSessionUpdated, workspaceID, actorType, actorID, map[string]any{
			"preview_session": response,
			"issue_id":        response.IssueID,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

type SwitchPreviewSessionDeviceRequest struct {
	Serial    string `json:"serial"`
	Confirmed bool   `json:"confirmed"`
}

// SwitchPreviewSessionDevice atomically moves an active local-device session
// to another serial. The caller reserves the target before asking the Device
// Runtime to deploy, so a failed lease never overwrites another Issue's app.
func (h *Handler) SwitchPreviewSessionDevice(w http.ResponseWriter, r *http.Request) {
	session, ok := h.loadPreviewSession(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	if session.Provider != previewProviderLocalDevice || session.Platform != previewPlatformAndroid {
		writeError(w, http.StatusBadRequest, "preview session does not use a local Android device")
		return
	}
	actorType, actorID, ok := h.authorizePreviewTouch(w, r, session)
	if !ok {
		return
	}
	current := previewSessionToResponse(session)
	if current.Status == "expired" || current.Status == "stopped" || current.Status == "failed" {
		writeError(w, http.StatusConflict, "preview session is "+current.Status)
		return
	}

	var request SwitchPreviewSessionDeviceRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPreviewRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	serial := strings.TrimSpace(request.Serial)
	if serial == "" || utf8.RuneCountInString(serial) > 256 || strings.ContainsAny(serial, "\x00\r\n") {
		writeError(w, http.StatusBadRequest, "serial must contain 1 to 256 printable characters")
		return
	}

	parsedURL, err := url.Parse(session.PreviewUrl)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "preview session has an invalid device URL")
		return
	}
	query := parsedURL.Query()
	query.Del("embed")
	query.Set("serial", serial)
	parsedURL.RawQuery = query.Encode()
	targetURL, err := normalizeLocalDevicePreviewURL(parsedURL.String())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to switch preview device")
		return
	}
	defer func() { _ = tx.Rollback(r.Context()) }()
	qtx := h.Queries.WithTx(tx)
	workspaceID := uuidToString(session.WorkspaceID)
	oldLock := previewDeviceLockKey(workspaceID, session.PreviewUrl)
	newLock := previewDeviceLockKey(workspaceID, targetURL)
	if newLock < oldLock {
		oldLock, newLock = newLock, oldLock
	}
	if err := qtx.LockPreviewDeviceResource(r.Context(), oldLock); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to switch preview device")
		return
	}
	if newLock != oldLock {
		if err := qtx.LockPreviewDeviceResource(r.Context(), newLock); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to switch preview device")
			return
		}
	}
	conflict, conflictErr := qtx.GetConflictingPreviewDeviceLease(r.Context(), db.GetConflictingPreviewDeviceLeaseParams{
		WorkspaceID: session.WorkspaceID,
		PreviewUrl:  targetURL,
		ID:          session.ID,
	})
	if conflictErr == nil {
		writeError(w, http.StatusConflict, "device is leased by issue "+uuidToString(conflict.IssueID)+" until "+timestampToString(conflict.LeaseExpiresAt))
		return
	}
	if !errors.Is(conflictErr, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to inspect preview device lease")
		return
	}
	if err := qtx.SleepOtherPreviewDeviceSessions(r.Context(), db.SleepOtherPreviewDeviceSessionsParams{
		WorkspaceID: session.WorkspaceID,
		PreviewUrl:  targetURL,
		ID:          session.ID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to release stale preview device leases")
		return
	}
	updated, err := qtx.SwitchPreviewDeviceSession(r.Context(), db.SwitchPreviewDeviceSessionParams{
		ID:          session.ID,
		WorkspaceID: session.WorkspaceID,
		PreviewUrl:  targetURL,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "preview session is unavailable")
		return
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, http.StatusInternalServerError, "failed to switch preview device")
		return
	}
	response := previewSessionToResponse(updated)
	if request.Confirmed {
		h.publish(protocol.EventPreviewSessionUpdated, workspaceID, actorType, actorID, map[string]any{
			"preview_session": response,
			"issue_id":        response.IssueID,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func previewDeviceLockKey(workspaceID, previewURL string) string {
	return workspaceID + "\n" + previewURL
}

func (h *Handler) authorizePreviewTouch(w http.ResponseWriter, r *http.Request, session db.PreviewSession) (string, string, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return "", "", false
	}
	workspaceID := uuidToString(session.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if actorType == "agent" {
		taskID := strings.TrimSpace(r.Header.Get("X-Task-ID"))
		if taskID == "" {
			writeError(w, http.StatusForbidden, "task does not belong to this issue")
			return "", "", false
		}
		task, err := h.Queries.GetAgentTask(r.Context(), parseUUID(taskID))
		if err != nil || !task.IssueID.Valid || uuidToString(task.IssueID) != uuidToString(session.IssueID) || uuidToString(task.AgentID) != actorID {
			writeError(w, http.StatusForbidden, "task does not belong to this issue")
			return "", "", false
		}
	} else if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return "", "", false
	}
	return actorType, actorID, true
}

// GetPreviewSession returns one workspace-scoped preview session.
func (h *Handler) GetPreviewSession(w http.ResponseWriter, r *http.Request) {
	session, ok := h.loadPreviewSession(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, previewSessionToResponse(session))
}

// StopPreviewSession closes the Multica preview record. The external process
// behind a Phase 0 URL is not managed by this endpoint.
func (h *Handler) StopPreviewSession(w http.ResponseWriter, r *http.Request) {
	session, ok := h.loadPreviewSession(w, r, chi.URLParam(r, "sessionId"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := uuidToString(session.WorkspaceID)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if actorType == "agent" {
		if session.CreatorType != "agent" || uuidToString(session.CreatorID) != actorID {
			writeError(w, http.StatusForbidden, "agents can only stop their own preview sessions")
			return
		}

		taskIDHeader := strings.TrimSpace(r.Header.Get("X-Task-ID"))
		if taskIDHeader == "" {
			writeError(w, http.StatusForbidden, "task does not belong to this issue")
			return
		}
		task, err := h.Queries.GetAgentTask(r.Context(), parseUUID(taskIDHeader))
		if err != nil || !task.IssueID.Valid || uuidToString(task.IssueID) != uuidToString(session.IssueID) || uuidToString(task.AgentID) != actorID {
			writeError(w, http.StatusForbidden, "task does not belong to this issue")
			return
		}
	} else {
		member, ok := h.workspaceMember(w, r, workspaceID)
		if !ok {
			return
		}
		isCreator := session.CreatorType == "member" && uuidToString(session.CreatorID) == actorID
		if !isCreator && !roleAllowed(member.Role, "owner", "admin") {
			writeError(w, http.StatusForbidden, "only the creator or a workspace admin can stop this preview session")
			return
		}
	}

	currentResponse := previewSessionToResponse(session)
	if currentResponse.Status == "expired" {
		writeJSON(w, http.StatusOK, currentResponse)
		return
	}

	updated, err := h.Queries.StopPreviewSession(r.Context(), db.StopPreviewSessionParams{
		ID:          session.ID,
		WorkspaceID: session.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stop preview session")
		return
	}

	response := previewSessionToResponse(updated)
	if updated.Status != session.Status || timestampToString(updated.UpdatedAt) != timestampToString(session.UpdatedAt) {
		h.publish(protocol.EventPreviewSessionUpdated, workspaceID, actorType, actorID, map[string]any{
			"preview_session": response,
			"issue_id":        response.IssueID,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) loadPreviewSession(w http.ResponseWriter, r *http.Request, rawID string) (db.PreviewSession, bool) {
	sessionID, ok := parseUUIDOrBadRequest(w, rawID, "preview session id")
	if !ok {
		return db.PreviewSession{}, false
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return db.PreviewSession{}, false
	}
	session, err := h.Queries.GetPreviewSessionInWorkspace(r.Context(), db.GetPreviewSessionInWorkspaceParams{
		ID:          sessionID,
		WorkspaceID: workspaceID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "preview session not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to get preview session")
		}
		return db.PreviewSession{}, false
	}
	return session, true
}
