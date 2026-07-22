package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/creative"
)

type workspaceMCPConnectionResponse struct {
	ID                string   `json:"id"`
	WorkspaceID       string   `json:"workspace_id"`
	Name              string   `json:"name"`
	Capability        string   `json:"capability"`
	Transport         string   `json:"transport"`
	ServerURL         string   `json:"server_url"`
	ToolCreate        string   `json:"tool_create"`
	ToolGet           string   `json:"tool_get"`
	Status            string   `json:"status"`
	IsDefault         bool     `json:"is_default"`
	HasSecretHeaders  bool     `json:"has_secret_headers"`
	SecretHeaderNames []string `json:"secret_header_names"`
	LastVerifiedAt    string   `json:"last_verified_at"`
	LastError         string   `json:"last_error"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type workspaceMCPConnectionInput struct {
	Name               string             `json:"name"`
	ServerURL          string             `json:"server_url"`
	Transport          string             `json:"transport"`
	ToolCreate         string             `json:"tool_create"`
	ToolGet            string             `json:"tool_get"`
	Status             string             `json:"status"`
	IsDefault          *bool              `json:"is_default"`
	SecretHeaders      *map[string]string `json:"secret_headers"`
	ClearSecretHeaders bool               `json:"clear_secret_headers"`
}

func (h *Handler) ListWorkspaceMCPConnections(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	connections, err := h.listWorkspaceMCPConnections(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspace MCP connections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connections": connections})
}

func (h *Handler) CreateWorkspaceMCPConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var input workspaceMCPConnectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	normalized, headers, err := normalizeWorkspaceMCPInput(input, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	encryptedHeaders, headerNames, err := h.encryptWorkspaceMCPHeaders(headers)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if h.TxStarter == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace MCP storage is unavailable")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace MCP connection")
		return
	}
	defer tx.Rollback(r.Context())
	if normalized.IsDefault != nil && *normalized.IsDefault && normalized.Status == "active" {
		if _, err := tx.Exec(r.Context(), `
UPDATE workspace_mcp_connection
SET is_default = false, updated_at = now()
WHERE workspace_id = $1::uuid AND capability = 'creative_edit'
`, workspaceID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update default MCP connection")
			return
		}
	}
	isDefault := normalized.Status == "active" && (normalized.IsDefault == nil || *normalized.IsDefault)
	var connectionID string
	err = tx.QueryRow(r.Context(), `
INSERT INTO workspace_mcp_connection (
  workspace_id, name, capability, transport, server_url, tool_create, tool_get,
  secret_headers_encrypted, secret_header_names, status, is_default, created_by
) VALUES (
  $1::uuid, $2, 'creative_edit', $3, $4, $5, $6,
  $7, $8, $9, $10, $11::uuid
)
RETURNING id::text
`, workspaceID, normalized.Name, normalized.Transport, normalized.ServerURL,
		normalized.ToolCreate, normalized.ToolGet, encryptedHeaders, headerNames,
		normalized.Status, isDefault, userID).Scan(&connectionID)
	if err != nil {
		writeWorkspaceMCPMutationError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace MCP connection")
		return
	}
	connection, err := h.getWorkspaceMCPConnection(r, workspaceID, connectionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load workspace MCP connection")
		return
	}
	writeJSON(w, http.StatusCreated, connection)
}

func (h *Handler) UpdateWorkspaceMCPConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	connectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "connection_id")
	if !ok {
		return
	}
	var input workspaceMCPConnectionInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	normalized, headers, err := normalizeWorkspaceMCPInput(input, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if h.TxStarter == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace MCP storage is unavailable")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace MCP connection")
		return
	}
	defer tx.Rollback(r.Context())
	var exists bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS (
  SELECT 1 FROM workspace_mcp_connection WHERE id = $1 AND workspace_id = $2::uuid
)
`, connectionID, workspaceID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "workspace MCP connection not found")
		return
	}
	if normalized.IsDefault != nil && *normalized.IsDefault && normalized.Status == "active" {
		if _, err := tx.Exec(r.Context(), `
UPDATE workspace_mcp_connection
SET is_default = false, updated_at = now()
WHERE workspace_id = $1::uuid AND capability = 'creative_edit' AND id <> $2
`, workspaceID, connectionID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update default MCP connection")
			return
		}
	}
	var encryptedHeaders any
	var headerNames any
	switch {
	case normalized.ClearSecretHeaders:
		encryptedHeaders = nil
		headerNames = []string{}
	case normalized.SecretHeaders != nil:
		sealed, names, encryptErr := h.encryptWorkspaceMCPHeaders(headers)
		if encryptErr != nil {
			writeError(w, http.StatusServiceUnavailable, encryptErr.Error())
			return
		}
		encryptedHeaders = sealed
		headerNames = names
	}
	_, err = tx.Exec(r.Context(), `
UPDATE workspace_mcp_connection
SET name = $3,
    server_url = $4,
    transport = $5,
    tool_create = $6,
    tool_get = $7,
    status = $8,
    is_default = CASE WHEN $8 = 'disabled' THEN false ELSE COALESCE($9, is_default) END,
    secret_headers_encrypted = CASE
      WHEN $10::boolean THEN NULL
      WHEN $11::boolean THEN $12::bytea
      ELSE secret_headers_encrypted
    END,
    secret_header_names = CASE
      WHEN $10::boolean THEN '{}'::text[]
      WHEN $11::boolean THEN $13::text[]
      ELSE secret_header_names
    END,
    last_verified_at = NULL,
    last_error = '',
    updated_at = now()
WHERE id = $1 AND workspace_id = $2::uuid
`, connectionID, workspaceID, normalized.Name, normalized.ServerURL, normalized.Transport,
		normalized.ToolCreate, normalized.ToolGet, normalized.Status, normalized.IsDefault,
		normalized.ClearSecretHeaders, normalized.SecretHeaders != nil, encryptedHeaders, headerNames)
	if err != nil {
		writeWorkspaceMCPMutationError(w, err)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace MCP connection")
		return
	}
	connection, err := h.getWorkspaceMCPConnection(r, workspaceID, uuidToString(connectionID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load workspace MCP connection")
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (h *Handler) DisableWorkspaceMCPConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	connectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "connection_id")
	if !ok {
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
UPDATE workspace_mcp_connection
SET status = 'disabled', is_default = false, updated_at = now()
WHERE id = $1 AND workspace_id = $2::uuid
`, connectionID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disable workspace MCP connection")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "workspace MCP connection not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) VerifyWorkspaceMCPConnection(w http.ResponseWriter, r *http.Request) {
	workspaceID := ctxWorkspaceID(r.Context())
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	connectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "connection_id")
	if !ok {
		return
	}
	if h.CreativeProviderResolver == nil {
		writeError(w, http.StatusServiceUnavailable, "workspace MCP provider resolver is unavailable")
		return
	}
	resolved, err := h.CreativeProviderResolver.Resolve(r.Context(), workspaceID, uuidToString(connectionID))
	if err != nil {
		h.recordWorkspaceMCPVerification(r, connectionID, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	verifier, ok := resolved.Provider.(interface {
		Verify(context.Context) ([]string, error)
	})
	if !ok {
		writeError(w, http.StatusBadRequest, "selected provider does not support MCP verification")
		return
	}
	tools, err := verifier.Verify(r.Context())
	h.recordWorkspaceMCPVerification(r, connectionID, err)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"provider": resolved.Provider.Name(),
		"tools":    tools,
	})
}

func normalizeWorkspaceMCPInput(input workspaceMCPConnectionInput, update bool) (workspaceMCPConnectionInput, map[string]string, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ServerURL = strings.TrimSpace(input.ServerURL)
	input.Transport = strings.TrimSpace(input.Transport)
	input.ToolCreate = strings.TrimSpace(input.ToolCreate)
	input.ToolGet = strings.TrimSpace(input.ToolGet)
	input.Status = strings.TrimSpace(input.Status)
	if input.Transport == "" {
		input.Transport = "streamable_http"
	}
	if input.ToolCreate == "" {
		input.ToolCreate = creative.CreateJobToolName
	}
	if input.ToolGet == "" {
		input.ToolGet = creative.GetJobToolName
	}
	if input.Status == "" {
		input.Status = "active"
	}
	if len(input.Name) < 1 || len(input.Name) > 80 {
		return input, nil, errors.New("name must contain between 1 and 80 characters")
	}
	parsed, err := url.Parse(input.ServerURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil {
		return input, nil, errors.New("server_url must be an HTTP or HTTPS URL without user info")
	}
	if input.Transport != "streamable_http" {
		return input, nil, errors.New("transport must be streamable_http")
	}
	if input.Status != "active" && input.Status != "disabled" {
		return input, nil, errors.New("status must be active or disabled")
	}
	for field, value := range map[string]string{"tool_create": input.ToolCreate, "tool_get": input.ToolGet} {
		if len(value) < 1 || len(value) > 128 || strings.ContainsAny(value, " \t\r\n") {
			return input, nil, fmt.Errorf("%s must be a non-empty MCP tool name", field)
		}
	}
	if input.ClearSecretHeaders && input.SecretHeaders != nil {
		return input, nil, errors.New("secret_headers and clear_secret_headers cannot be used together")
	}
	headers := map[string]string{}
	if input.SecretHeaders != nil {
		if len(*input.SecretHeaders) > 20 {
			return input, nil, errors.New("secret_headers cannot contain more than 20 entries")
		}
		for key, value := range *input.SecretHeaders {
			canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(key))
			if canonical == "" || len(canonical) > 100 {
				return input, nil, fmt.Errorf("invalid secret header name %q", key)
			}
			switch strings.ToLower(canonical) {
			case "host", "content-length", "transfer-encoding", "connection", "content-type", "accept", "mcp-session-id", "mcp-protocol-version":
				return input, nil, fmt.Errorf("secret header %q is managed by Multica", canonical)
			}
			value = strings.TrimSpace(value)
			if value == "" || len(value) > 8192 || strings.ContainsAny(value, "\r\n") {
				return input, nil, fmt.Errorf("secret header %q has an invalid value", canonical)
			}
			headers[canonical] = value
		}
	}
	if !update && input.IsDefault == nil {
		value := true
		input.IsDefault = &value
	}
	return input, headers, nil
}

func (h *Handler) encryptWorkspaceMCPHeaders(headers map[string]string) ([]byte, []string, error) {
	if len(headers) == 0 {
		return nil, []string{}, nil
	}
	if h.WorkspaceMCPSecretBox == nil {
		return nil, nil, errors.New("MULTICA_WORKSPACE_MCP_KEY is not configured")
	}
	plaintext, err := json.Marshal(headers)
	if err != nil {
		return nil, nil, err
	}
	sealed, err := h.WorkspaceMCPSecretBox.Seal(plaintext)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(headers))
	for key := range headers {
		names = append(names, key)
	}
	sort.Strings(names)
	return sealed, names, nil
}

func (h *Handler) listWorkspaceMCPConnections(r *http.Request, workspaceID string) ([]workspaceMCPConnectionResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, workspace_id::text, name, capability, transport, server_url,
       tool_create, tool_get, status, is_default,
       secret_headers_encrypted IS NOT NULL, secret_header_names,
       COALESCE(last_verified_at::text, ''), last_error,
       created_at::text, updated_at::text
FROM workspace_mcp_connection
WHERE workspace_id = $1::uuid
ORDER BY is_default DESC, status, created_at
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	connections := []workspaceMCPConnectionResponse{}
	for rows.Next() {
		var item workspaceMCPConnectionResponse
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.Name, &item.Capability, &item.Transport,
			&item.ServerURL, &item.ToolCreate, &item.ToolGet, &item.Status, &item.IsDefault,
			&item.HasSecretHeaders, &item.SecretHeaderNames, &item.LastVerifiedAt,
			&item.LastError, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		connections = append(connections, item)
	}
	return connections, rows.Err()
}

func (h *Handler) getWorkspaceMCPConnection(r *http.Request, workspaceID, connectionID string) (workspaceMCPConnectionResponse, error) {
	var item workspaceMCPConnectionResponse
	err := h.DB.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, name, capability, transport, server_url,
       tool_create, tool_get, status, is_default,
       secret_headers_encrypted IS NOT NULL, secret_header_names,
       COALESCE(last_verified_at::text, ''), last_error,
       created_at::text, updated_at::text
FROM workspace_mcp_connection
WHERE id = $1::uuid AND workspace_id = $2::uuid
`, connectionID, workspaceID).Scan(
		&item.ID, &item.WorkspaceID, &item.Name, &item.Capability, &item.Transport,
		&item.ServerURL, &item.ToolCreate, &item.ToolGet, &item.Status, &item.IsDefault,
		&item.HasSecretHeaders, &item.SecretHeaderNames, &item.LastVerifiedAt,
		&item.LastError, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (h *Handler) recordWorkspaceMCPVerification(r *http.Request, connectionID pgtype.UUID, verifyErr error) {
	lastError := ""
	if verifyErr != nil {
		lastError = verifyErr.Error()
	}
	_, _ = h.DB.Exec(r.Context(), `
UPDATE workspace_mcp_connection
SET last_verified_at = now(), last_error = $2, updated_at = now()
WHERE id = $1
`, connectionID, lastError)
}

func writeWorkspaceMCPMutationError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeError(w, http.StatusConflict, "workspace MCP connection name or default already exists")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workspace MCP connection not found")
		return
	}
	writeError(w, http.StatusInternalServerError, "failed to save workspace MCP connection")
}
