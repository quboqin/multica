package daemon

import (
	"encoding/json"
	"testing"
)

func TestApplyUserIntegrationEnvShadowsFallbackCredentials(t *testing.T) {
	env := map[string]string{
		"GITHUB_TOKEN":     "host-token",
		"FEISHU_MCP_TOKEN": "shared-feishu",
		"PAONES_TOKEN":     "shared-paones",
		"JINGWEI_TOKEN":    "shared-jingwei",
	}
	task := Task{
		RequestingUserID:   "user-1",
		RequestingUserName: "Alice",
		IntegrationTokens: IntegrationTokens{
			GitToken: "user-git",
			Extra: map[string]string{
				"github_token": "user-github-extra",
				"notion_token": "user-notion",
			},
		},
	}

	applyUserIntegrationEnv(env, task)

	if env["GITHUB_TOKEN"] != "user-git" || env["GH_TOKEN"] != "user-git" || env["GIT_TOKEN"] != "user-git" {
		t.Fatalf("git env not overridden with user token: %#v", env)
	}
	if env["FEISHU_MCP_TOKEN"] != "" {
		t.Fatalf("missing Feishu token should shadow fallback, got %q", env["FEISHU_MCP_TOKEN"])
	}
	if env["PAONES_TOKEN"] != "" {
		t.Fatalf("missing PAones token should shadow fallback, got %q", env["PAONES_TOKEN"])
	}
	if env["JINGWEI_TOKEN"] != "" {
		t.Fatalf("missing Jingwei token should shadow fallback, got %q", env["JINGWEI_TOKEN"])
	}
	if env["MULTICA_REQUESTING_USER_ID"] != "user-1" {
		t.Fatalf("requesting user id not injected")
	}
	if env["NOTION_TOKEN"] != "user-notion" || env["MULTICA_INTEGRATION_NOTION_TOKEN"] != "user-notion" {
		t.Fatalf("extra token env not injected: %#v", env)
	}
	if env["MULTICA_INTEGRATION_GITHUB_TOKEN"] != "user-github-extra" {
		t.Fatalf("extra github token marker env not injected: %#v", env)
	}
}

func TestIntegrationTokensUnmarshalMapsLegacyPaihubToPaones(t *testing.T) {
	var tokens IntegrationTokens
	if err := json.Unmarshal([]byte(`{"paihub_token":"legacy-paones"}`), &tokens); err != nil {
		t.Fatalf("unmarshal integration tokens: %v", err)
	}
	env := integrationEnvOverrides(tokens)
	if env["PAONES_TOKEN"] != "legacy-paones" {
		t.Fatalf("legacy paihub_token should map to PAONES_TOKEN, got %#v", env["PAONES_TOKEN"])
	}
	if _, ok := env["PAIHUB_TOKEN"]; ok {
		t.Fatalf("legacy paihub_token must not expose PAIHUB_TOKEN: %#v", env)
	}
}

func TestMaterializeIntegrationMcpConfigOverridesKnownEnvKeys(t *testing.T) {
	raw := json.RawMessage(`{
		"mcpServers": {
			"feishu": {
				"command": "feishu-mcp",
				"env": {
					"FEISHU_MCP_TOKEN": "old-token",
					"NOTION_TOKEN": "old-notion",
					"UNCHANGED": "keep"
				}
			}
		}
	}`)

	out := materializeIntegrationMcpConfig(raw, IntegrationTokens{
		FeishuMCPToken: "user-feishu",
		Extra: map[string]string{
			"notion_token": "user-notion",
		},
	})
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	server := decoded["mcpServers"].(map[string]any)["feishu"].(map[string]any)
	env := server["env"].(map[string]any)
	if env["FEISHU_MCP_TOKEN"] != "user-feishu" {
		t.Fatalf("FEISHU_MCP_TOKEN not overridden, got %#v", env["FEISHU_MCP_TOKEN"])
	}
	if env["NOTION_TOKEN"] != "user-notion" {
		t.Fatalf("NOTION_TOKEN not overridden, got %#v", env["NOTION_TOKEN"])
	}
	if env["UNCHANGED"] != "keep" {
		t.Fatalf("unrelated env key changed")
	}
}

func TestMaterializeIntegrationMcpConfigReplacesURLPlaceholders(t *testing.T) {
	raw := json.RawMessage(`{
		"mcpServers": {
			"feishu": {
				"url": "${MULTICA_INTEGRATION_FEISHU_MCP_TOKEN}",
				"headers": {
					"X-Feishu": "${FEISHU_MCP_TOKEN}"
				},
				"args": ["--endpoint", "${MULTICA_INTEGRATION_FEISHU_MCP_TOKEN}"]
			}
		}
	}`)

	out := materializeIntegrationMcpConfig(raw, IntegrationTokens{
		FeishuMCPToken: "https://mcp.feishu.cn/mcp/user-a",
	})
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	server := decoded["mcpServers"].(map[string]any)["feishu"].(map[string]any)
	if server["url"] != "https://mcp.feishu.cn/mcp/user-a" {
		t.Fatalf("url placeholder not replaced: %#v", server["url"])
	}
	headers := server["headers"].(map[string]any)
	if headers["X-Feishu"] != "https://mcp.feishu.cn/mcp/user-a" {
		t.Fatalf("header placeholder not replaced: %#v", headers["X-Feishu"])
	}
	args := server["args"].([]any)
	if args[1] != "https://mcp.feishu.cn/mcp/user-a" {
		t.Fatalf("arg placeholder not replaced: %#v", args)
	}
}

func TestMaterializeIntegrationMcpConfigDisablesServerWithMissingPlaceholderCredential(t *testing.T) {
	raw := json.RawMessage(`{
		"mcpServers": {
			"feishu": {
				"url": "${MULTICA_INTEGRATION_FEISHU_MCP_TOKEN}"
			}
		}
	}`)

	out := materializeIntegrationMcpConfig(raw, IntegrationTokens{})
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	server := decoded["mcpServers"].(map[string]any)["feishu"].(map[string]any)
	if server["url"] != "" {
		t.Fatalf("missing credential should blank placeholder, got %#v", server["url"])
	}
	if server["enabled"] != false {
		t.Fatalf("missing credential should disable server, got %#v", server["enabled"])
	}
}
