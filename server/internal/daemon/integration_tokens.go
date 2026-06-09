package daemon

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/multica-ai/multica/server/internal/daemon/execenv"
)

func applyUserIntegrationEnv(env map[string]string, task Task) {
	for key, value := range integrationEnvOverrides(task.IntegrationTokens) {
		env[key] = value
	}
	if strings.TrimSpace(task.RequestingUserID) != "" {
		env["MULTICA_REQUESTING_USER_ID"] = strings.TrimSpace(task.RequestingUserID)
	}
	if strings.TrimSpace(task.RequestingUserName) != "" {
		env["MULTICA_REQUESTING_USER_NAME"] = strings.TrimSpace(task.RequestingUserName)
	}
}

func integrationCredentialStatus(tokens IntegrationTokens) execenv.IntegrationCredentialStatus {
	extra := make(map[string]bool, len(tokens.Extra))
	for key, value := range tokens.Extra {
		if strings.TrimSpace(key) == "" {
			continue
		}
		extra[key] = strings.TrimSpace(value) != ""
	}
	return execenv.IntegrationCredentialStatus{
		GitToken:       strings.TrimSpace(tokens.GitToken) != "",
		FeishuMCPToken: strings.TrimSpace(tokens.FeishuMCPToken) != "",
		PaonesToken:    strings.TrimSpace(tokens.PaonesToken) != "",
		JingweiToken:   strings.TrimSpace(tokens.JingweiToken) != "",
		Extra:          extra,
	}
}

func integrationEnvOverrides(tokens IntegrationTokens) map[string]string {
	git := strings.TrimSpace(tokens.GitToken)
	feishu := strings.TrimSpace(tokens.FeishuMCPToken)
	paones := strings.TrimSpace(tokens.PaonesToken)
	jingwei := strings.TrimSpace(tokens.JingweiToken)
	out := map[string]string{
		"MULTICA_INTEGRATION_GIT_TOKEN":        git,
		"MULTICA_INTEGRATION_FEISHU_MCP_TOKEN": feishu,
		"MULTICA_INTEGRATION_PAONES_TOKEN":     paones,
		"MULTICA_INTEGRATION_JINGWEI_TOKEN":    jingwei,
		"GIT_TOKEN":                            git,
		"GITHUB_TOKEN":                         git,
		"GH_TOKEN":                             git,
		"FEISHU_MCP_TOKEN":                     feishu,
		"PAONES_TOKEN":                         paones,
		"JINGWEI_TOKEN":                        jingwei,
	}
	for key, value := range tokens.Extra {
		envKey := integrationTokenEnvKey(key)
		if envKey == "" {
			continue
		}
		trimmedValue := strings.TrimSpace(value)
		if strings.TrimSpace(out[envKey]) == "" {
			out[envKey] = trimmedValue
		}
		if !strings.HasPrefix(envKey, "MULTICA_INTEGRATION_") {
			out["MULTICA_INTEGRATION_"+envKey] = trimmedValue
		}
	}
	return out
}

var nonEnvKeyChars = regexp.MustCompile(`[^A-Z0-9]+`)

func integrationTokenEnvKey(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	envKey := strings.ToUpper(trimmed)
	envKey = nonEnvKeyChars.ReplaceAllString(envKey, "_")
	envKey = strings.Trim(envKey, "_")
	return envKey
}

func materializeIntegrationMcpConfig(raw json.RawMessage, tokens IntegrationTokens) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return raw
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return raw
	}
	overrides := integrationEnvOverrides(tokens)
	changed := applyMcpServerEnvOverrides(root["mcpServers"], overrides)
	if replaceMcpServerPlaceholders(root["mcpServers"], overrides) {
		changed = true
	}
	if changed {
		out, err := json.Marshal(root)
		if err == nil {
			return out
		}
	}
	return raw
}

func applyMcpServerEnvOverrides(mcpServers any, overrides map[string]string) bool {
	servers, ok := mcpServers.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for _, rawServer := range servers {
		server, ok := rawServer.(map[string]any)
		if !ok {
			continue
		}
		rawEnv, ok := server["env"]
		if !ok {
			continue
		}
		env, ok := rawEnv.(map[string]any)
		if !ok {
			continue
		}
		for key, value := range overrides {
			if _, exists := env[key]; exists {
				env[key] = value
				changed = true
			}
		}
	}
	return changed
}

func replaceMcpServerPlaceholders(mcpServers any, overrides map[string]string) bool {
	servers, ok := mcpServers.(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for name, rawServer := range servers {
		updated, didChange, missingCredential := replaceIntegrationPlaceholders(rawServer, overrides)
		if missingCredential {
			if server, ok := updated.(map[string]any); ok {
				server["enabled"] = false
				updated = server
				didChange = true
			}
		}
		if didChange {
			servers[name] = updated
			changed = true
		}
	}
	return changed
}

func replaceIntegrationPlaceholders(value any, overrides map[string]string) (any, bool, bool) {
	switch v := value.(type) {
	case string:
		out := v
		missingCredential := false
		for key, replacement := range overrides {
			placeholder := "${" + key + "}"
			if strings.Contains(out, placeholder) && strings.TrimSpace(replacement) == "" {
				missingCredential = true
			}
			out = strings.ReplaceAll(out, placeholder, replacement)
		}
		return out, out != v, missingCredential
	case []any:
		changed := false
		missingCredential := false
		for i, item := range v {
			updated, didChange, didMissCredential := replaceIntegrationPlaceholders(item, overrides)
			if didChange {
				v[i] = updated
				changed = true
			}
			if didMissCredential {
				missingCredential = true
			}
		}
		return v, changed, missingCredential
	case map[string]any:
		changed := false
		missingCredential := false
		for key, item := range v {
			updated, didChange, didMissCredential := replaceIntegrationPlaceholders(item, overrides)
			if didChange {
				v[key] = updated
				changed = true
			}
			if didMissCredential {
				missingCredential = true
			}
		}
		return v, changed, missingCredential
	default:
		return value, false, false
	}
}
