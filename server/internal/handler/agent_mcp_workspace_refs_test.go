package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMaterializeAgentWorkspaceMCPRefsMergesCustomAndWorkspaceServers(t *testing.T) {
	connectionID := "11111111-1111-1111-1111-111111111111"
	raw := json.RawMessage(`{
		"mcpServers": {
			"github": {"command": "github-mcp"}
		},
		"workspaceMcpRefs": [
			{"connectionId": "` + connectionID + `", "serverName": "Creative MCP"}
		]
	}`)

	out, err := materializeAgentWorkspaceMCPRefs(
		context.Background(),
		"workspace-1",
		raw,
		func(_ context.Context, workspaceID, gotConnectionID string) (runtimeWorkspaceMCPConnection, error) {
			if workspaceID != "workspace-1" {
				t.Fatalf("workspaceID = %q, want workspace-1", workspaceID)
			}
			if gotConnectionID != connectionID {
				t.Fatalf("connectionID = %q, want %q", gotConnectionID, connectionID)
			}
			return runtimeWorkspaceMCPConnection{
				ID:        connectionID,
				Name:      "Ad Creative FAT",
				Transport: "streamable_http",
				ServerURL: "http://10.0.0.10:19518/mcp",
				Headers:   map[string]string{"Authorization": "Bearer test"},
			}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("materializeAgentWorkspaceMCPRefs: %v", err)
	}

	var parsed struct {
		MCPServers       map[string]map[string]any `json:"mcpServers"`
		WorkspaceMCPRefs any                       `json:"workspaceMcpRefs"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, string(out))
	}
	if parsed.WorkspaceMCPRefs != nil {
		t.Fatalf("workspaceMcpRefs leaked into runtime config: %v", parsed.WorkspaceMCPRefs)
	}
	if _, ok := parsed.MCPServers["github"]; !ok {
		t.Fatalf("custom github server missing from %#v", parsed.MCPServers)
	}
	workspaceServer := parsed.MCPServers["creative-mcp"]
	if workspaceServer == nil {
		t.Fatalf("workspace server missing from %#v", parsed.MCPServers)
	}
	if got := workspaceServer["url"]; got != "http://10.0.0.10:19518/mcp" {
		t.Fatalf("workspace url = %v", got)
	}
	headers, ok := workspaceServer["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers = %T, want object", workspaceServer["headers"])
	}
	if got := headers["Authorization"]; got != "Bearer test" {
		t.Fatalf("Authorization header = %v", got)
	}
}

func TestMaterializeAgentWorkspaceMCPRefsAvoidsCustomServerNameCollision(t *testing.T) {
	connectionID := "22222222-2222-2222-2222-222222222222"
	raw := json.RawMessage(`{
		"mcpServers": {
			"workspace-ad-creative": {"command": "custom"}
		},
		"workspaceMcpRefs": [
			{"connectionId": "` + connectionID + `", "serverName": "workspace-ad-creative"}
		]
	}`)

	out, err := materializeAgentWorkspaceMCPRefs(
		context.Background(),
		"workspace-1",
		raw,
		func(context.Context, string, string) (runtimeWorkspaceMCPConnection, error) {
			return runtimeWorkspaceMCPConnection{
				ID:        connectionID,
				Name:      "Ad Creative",
				Transport: "streamable_http",
				ServerURL: "http://127.0.0.1:19518/mcp",
			}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("materializeAgentWorkspaceMCPRefs: %v", err)
	}
	if !strings.Contains(string(out), `"workspace-ad-creative-22222222"`) {
		t.Fatalf("expected collision-safe workspace server name, got %s", string(out))
	}
}

func TestParseAgentWorkspaceMCPRefsSupportsSnakeCase(t *testing.T) {
	raw := json.RawMessage(`{
		"workspace_mcp_refs": [
			{"connection_id": "33333333-3333-3333-3333-333333333333", "server_name": "workspace_creative", "expose_to_runtime": false}
		]
	}`)

	refs, hasRefs, err := parseAgentWorkspaceMCPRefs(raw)
	if err != nil {
		t.Fatalf("parseAgentWorkspaceMCPRefs: %v", err)
	}
	if !hasRefs || len(refs) != 1 {
		t.Fatalf("hasRefs=%v len=%d", hasRefs, len(refs))
	}
	if refs[0].ConnectionID != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("connection id = %q", refs[0].ConnectionID)
	}
	if refs[0].ServerName != "workspace_creative" {
		t.Fatalf("server name = %q", refs[0].ServerName)
	}
	if refs[0].ExposeToRuntime {
		t.Fatalf("ExposeToRuntime = true, want false")
	}
}
