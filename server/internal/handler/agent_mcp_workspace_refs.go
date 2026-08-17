package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/util"
)

const (
	agentMCPWorkspaceRefsKey      = "workspaceMcpRefs"
	agentMCPWorkspaceRefsSnakeKey = "workspace_mcp_refs"
)

type agentWorkspaceMCPRef struct {
	ConnectionID    string
	ServerName      string
	ExposeToRuntime bool
}

type runtimeWorkspaceMCPConnection struct {
	ID        string
	Name      string
	Transport string
	ServerURL string
	Headers   map[string]string
}

type runtimeWorkspaceMCPResolver func(context.Context, string, string) (runtimeWorkspaceMCPConnection, error)

func (h *Handler) validateAgentWorkspaceMCPRefs(ctx context.Context, workspaceID string, raw json.RawMessage) error {
	refs, hasRefs, err := parseAgentWorkspaceMCPRefs(raw)
	if err != nil {
		return err
	}
	if !hasRefs {
		return nil
	}
	if h.DB == nil {
		return errors.New("workspace MCP storage is unavailable")
	}
	for _, ref := range refs {
		if !ref.ExposeToRuntime {
			continue
		}
		connectionID := strings.TrimSpace(ref.ConnectionID)
		if connectionID == "" {
			return errors.New("workspaceMcpRefs[].connectionId is required")
		}
		if _, err := util.ParseUUID(connectionID); err != nil {
			return fmt.Errorf("workspaceMcpRefs[].connectionId %q is not a valid UUID", connectionID)
		}
		var exists bool
		if err := h.DB.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM workspace_mcp_connection
  WHERE id = $1::uuid
    AND workspace_id = $2::uuid
    AND status = 'active'
)
`, connectionID, workspaceID).Scan(&exists); err != nil {
			return fmt.Errorf("validate workspace MCP reference: %w", err)
		}
		if !exists {
			return fmt.Errorf("workspace MCP connection %s is not active in this workspace", connectionID)
		}
	}
	return nil
}

func agentMCPConfigUsesWorkspaceRefs(raw json.RawMessage) (bool, error) {
	refs, hasRefs, err := parseAgentWorkspaceMCPRefs(raw)
	if err != nil || !hasRefs {
		return false, err
	}
	for _, ref := range refs {
		if ref.ExposeToRuntime && strings.TrimSpace(ref.ConnectionID) != "" {
			return true, nil
		}
	}
	return false, nil
}

func (h *Handler) materializeAgentWorkspaceMCPRefs(ctx context.Context, workspaceID string, raw json.RawMessage, logger *slog.Logger) (json.RawMessage, error) {
	return materializeAgentWorkspaceMCPRefs(ctx, workspaceID, raw, h.resolveRuntimeWorkspaceMCPConnection, logger)
}

func (h *Handler) resolveRuntimeWorkspaceMCPConnection(ctx context.Context, workspaceID, connectionID string) (runtimeWorkspaceMCPConnection, error) {
	if h.DB == nil {
		return runtimeWorkspaceMCPConnection{}, errors.New("workspace MCP storage is unavailable")
	}
	var connection runtimeWorkspaceMCPConnection
	var encryptedHeaders []byte
	err := h.DB.QueryRow(ctx, `
SELECT id::text, name, transport, server_url, COALESCE(secret_headers_encrypted, ''::bytea)
FROM workspace_mcp_connection
WHERE id = $1::uuid
  AND workspace_id = $2::uuid
  AND status = 'active'
LIMIT 1
`, connectionID, workspaceID).Scan(
		&connection.ID,
		&connection.Name,
		&connection.Transport,
		&connection.ServerURL,
		&encryptedHeaders,
	)
	if err != nil {
		return runtimeWorkspaceMCPConnection{}, err
	}
	if len(encryptedHeaders) > 0 {
		if h.WorkspaceMCPSecretBox == nil {
			return runtimeWorkspaceMCPConnection{}, errors.New("MULTICA_WORKSPACE_MCP_KEY is not configured")
		}
		plaintext, err := h.WorkspaceMCPSecretBox.Open(encryptedHeaders)
		if err != nil {
			return runtimeWorkspaceMCPConnection{}, fmt.Errorf("decrypt workspace MCP headers: %w", err)
		}
		if err := json.Unmarshal(plaintext, &connection.Headers); err != nil {
			return runtimeWorkspaceMCPConnection{}, fmt.Errorf("decode workspace MCP headers: %w", err)
		}
	}
	if connection.Headers == nil {
		connection.Headers = map[string]string{}
	}
	return connection, nil
}

func parseAgentWorkspaceMCPRefs(raw json.RawMessage) ([]agentWorkspaceMCPRef, bool, error) {
	root, ok, err := parseObjectMCPConfig(raw)
	if err != nil || !ok {
		return nil, false, err
	}
	refsRaw, hasRefs := root[agentMCPWorkspaceRefsKey]
	if !hasRefs {
		refsRaw, hasRefs = root[agentMCPWorkspaceRefsSnakeKey]
	}
	if !hasRefs {
		return nil, false, nil
	}
	trimmed := bytes.TrimSpace(refsRaw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, true, nil
	}
	var wire []struct {
		ConnectionID         string `json:"connectionId"`
		ConnectionIDSnake    string `json:"connection_id"`
		ServerName           string `json:"serverName"`
		ServerNameSnake      string `json:"server_name"`
		ExposeToRuntime      *bool  `json:"exposeToRuntime"`
		ExposeToRuntimeSnake *bool  `json:"expose_to_runtime"`
	}
	if err := json.Unmarshal(trimmed, &wire); err != nil {
		return nil, true, fmt.Errorf("workspaceMcpRefs must be an array: %w", err)
	}
	refs := make([]agentWorkspaceMCPRef, 0, len(wire))
	for _, item := range wire {
		connectionID := firstNonEmptyMCPRefField(item.ConnectionID, item.ConnectionIDSnake)
		serverName := firstNonEmptyMCPRefField(item.ServerName, item.ServerNameSnake)
		exposeToRuntime := true
		if item.ExposeToRuntime != nil {
			exposeToRuntime = *item.ExposeToRuntime
		}
		if item.ExposeToRuntimeSnake != nil {
			exposeToRuntime = *item.ExposeToRuntimeSnake
		}
		refs = append(refs, agentWorkspaceMCPRef{
			ConnectionID:    strings.TrimSpace(connectionID),
			ServerName:      strings.TrimSpace(serverName),
			ExposeToRuntime: exposeToRuntime,
		})
	}
	return refs, true, nil
}

func materializeAgentWorkspaceMCPRefs(
	ctx context.Context,
	workspaceID string,
	raw json.RawMessage,
	resolve runtimeWorkspaceMCPResolver,
	logger *slog.Logger,
) (json.RawMessage, error) {
	root, ok, err := parseObjectMCPConfig(raw)
	if err != nil || !ok {
		return raw, err
	}
	refs, hasRefs, err := parseAgentWorkspaceMCPRefs(raw)
	if err != nil || !hasRefs {
		return raw, err
	}

	delete(root, agentMCPWorkspaceRefsKey)
	delete(root, agentMCPWorkspaceRefsSnakeKey)

	servers := map[string]json.RawMessage{}
	if rawServers, ok := root["mcpServers"]; ok {
		trimmed := bytes.TrimSpace(rawServers)
		if len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
			if err := json.Unmarshal(trimmed, &servers); err != nil {
				return nil, fmt.Errorf("mcpServers must be an object: %w", err)
			}
		}
	}
	if servers == nil {
		servers = map[string]json.RawMessage{}
	}

	for _, ref := range refs {
		if !ref.ExposeToRuntime {
			continue
		}
		connectionID := strings.TrimSpace(ref.ConnectionID)
		if connectionID == "" {
			continue
		}
		connection, err := resolve(ctx, workspaceID, connectionID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				if logger != nil {
					logger.Warn("workspace MCP reference skipped because the connection is unavailable",
						"workspace_id", workspaceID,
						"connection_id", connectionID)
				}
				continue
			}
			return nil, err
		}
		if connection.Transport != "streamable_http" {
			if logger != nil {
				logger.Warn("workspace MCP reference skipped because the transport is unsupported",
					"workspace_id", workspaceID,
					"connection_id", connection.ID,
					"transport", connection.Transport)
			}
			continue
		}
		serverName := uniqueWorkspaceMCPServerName(ref.ServerName, connection, servers)
		entry := map[string]any{
			"url": connection.ServerURL,
		}
		if len(connection.Headers) > 0 {
			entry["headers"] = connection.Headers
		}
		data, err := json.Marshal(entry)
		if err != nil {
			return nil, fmt.Errorf("marshal workspace MCP runtime entry: %w", err)
		}
		servers[serverName] = data
	}

	serversData, err := json.Marshal(servers)
	if err != nil {
		return nil, fmt.Errorf("marshal mcpServers: %w", err)
	}
	root["mcpServers"] = serversData
	data, err := marshalStableRawObject(root)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func parseObjectMCPConfig(raw json.RawMessage) (map[string]json.RawMessage, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &root); err != nil {
		return nil, false, fmt.Errorf("parse mcp_config json: %w", err)
	}
	if root == nil {
		return nil, false, nil
	}
	return root, true, nil
}

func uniqueWorkspaceMCPServerName(refName string, connection runtimeWorkspaceMCPConnection, existing map[string]json.RawMessage) string {
	base := normalizeRuntimeMCPServerName(refName)
	if base == "" {
		base = normalizeRuntimeMCPServerName("workspace-" + connection.Name)
	}
	if base == "" {
		base = "workspace-mcp"
	}
	if _, ok := existing[base]; !ok {
		return base
	}
	shortID := strings.ReplaceAll(connection.ID, "-", "")
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if shortID == "" {
		shortID = "ref"
	}
	candidate := base + "-" + shortID
	if _, ok := existing[candidate]; !ok {
		return candidate
	}
	for i := 2; ; i++ {
		candidate = fmt.Sprintf("%s-%s-%d", base, shortID, i)
		if _, ok := existing[candidate]; !ok {
			return candidate
		}
	}
}

func normalizeRuntimeMCPServerName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '_' || r == '-':
			if !lastDash {
				b.WriteByte(byte(r))
				lastDash = r == '-'
			}
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-_")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-_")
	}
	return out
}

func marshalStableRawObject(root map[string]json.RawMessage) ([]byte, error) {
	keys := make([]string, 0, len(root))
	for key := range root {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		keyData, _ := json.Marshal(key)
		b.Write(keyData)
		b.WriteByte(':')
		if len(root[key]) == 0 {
			b.WriteString("null")
		} else {
			b.Write(bytes.TrimSpace(root[key]))
		}
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}

func firstNonEmptyMCPRefField(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
