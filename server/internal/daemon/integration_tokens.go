package daemon

import (
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strconv"
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
	applyUserGitIdentityEnv(env, task)
	applyUserGitAuthEnv(env, task)
}

func applyUserGitIdentityEnv(env map[string]string, task Task) {
	name := strings.TrimSpace(task.RequestingUserName)
	email := strings.TrimSpace(task.RequestingUserEmail)
	if name == "" || email == "" {
		return
	}
	env["GIT_AUTHOR_NAME"] = name
	env["GIT_AUTHOR_EMAIL"] = email
	env["GIT_COMMITTER_NAME"] = name
	env["GIT_COMMITTER_EMAIL"] = email
}

// applyUserGitAuthEnv wires the requesting user's git token into the agent's
// git environment so HTTPS clone/fetch/push authenticate as that user, and
// (when available) attributes commits to them. This is what makes per-user
// isolation actually reach git on a shared runtime: the bare cache is cloned
// with the daemon's own credentials (read), but every git operation the agent
// runs in its worktree is rewritten to a token-bearing HTTPS URL for the
// requesting user (write).
//
// We use git's GIT_CONFIG_* env-scoped config (the same mechanism repocache
// uses for safe.directory) so the token never touches disk and lives only for
// the lifetime of the agent process. The url rewrites are host-scoped to the
// task's git hosts so the token is never offered to an unrelated remote.
//
// When the task has remote repo hosts but the user has no git_token, remote
// URL rewrites point those hosts at an invalid Multica-owned sentinel host.
// That lets local git operations such as status/commit keep working while
// fetch/push fail instead of falling through to the daemon machine's SSH key
// or credential helper.
func applyUserGitAuthEnv(env map[string]string, task Task) {
	hosts := gitHostsFromRepos(task.Repos)
	if len(hosts) == 0 {
		return
	}
	token := strings.TrimSpace(task.IntegrationTokens.GitToken)

	type kv struct{ key, value string }
	var entries []kv
	for _, host := range hosts {
		var target string
		if token == "" {
			target = "https://multica-git-token-required.invalid/"
		} else {
			// GitLab/GitHub PATs authenticate over HTTPS as `oauth2:<token>`.
			target = "https://oauth2:" + token + "@" + host + "/"
		}
		// insteadOf is multi-valued; each index below is read by git as a
		// separate rewrite rule for the same target URL. Cover the scp-style
		// SSH form (git@host:group/repo.git), the ssh:// form, and plain
		// HTTPS so any remote shape the workspace stored resolves to the
		// token-bearing URL, or to the missing-token sentinel URL.
		entries = append(entries,
			kv{"url." + target + ".insteadOf", "git@" + host + ":"},
			kv{"url." + target + ".insteadOf", "ssh://git@" + host + "/"},
			kv{"url." + target + ".insteadOf", "https://" + host + "/"},
		)
	}
	// Also set Git config identity for tools that ignore GIT_AUTHOR_* /
	// GIT_COMMITTER_* but still honor env-scoped Git config.
	if name := strings.TrimSpace(task.RequestingUserName); name != "" {
		entries = append(entries, kv{"user.name", name})
	}
	if email := strings.TrimSpace(task.RequestingUserEmail); email != "" {
		entries = append(entries, kv{"user.email", email})
	}

	// Append after any GIT_CONFIG_* the daemon already inherited so we don't
	// clobber inherited env-scoped git config. In normal operation the daemon
	// has no GIT_CONFIG_COUNT (repocache sets it only on its own git
	// subprocess), so base is 0.
	base, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("GIT_CONFIG_COUNT")))
	for i, e := range entries {
		idx := strconv.Itoa(base + i)
		env["GIT_CONFIG_KEY_"+idx] = e.key
		env["GIT_CONFIG_VALUE_"+idx] = e.value
	}
	env["GIT_CONFIG_COUNT"] = strconv.Itoa(base + len(entries))
}

// gitHostsFromRepos returns the unique git hosts across a task's repos,
// preserving first-seen order.
func gitHostsFromRepos(repos []RepoData) []string {
	seen := make(map[string]bool, len(repos))
	hosts := make([]string, 0, len(repos))
	for _, r := range repos {
		host := gitHostFromURL(r.URL)
		if host == "" || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	return hosts
}

// gitHostFromURL extracts the hostname from a git remote URL, handling both
// the scp-like SSH form (git@host:group/repo.git) and scheme URLs
// (https://host/..., ssh://git@host:22/...). Returns "" when no host is
// derivable.
func gitHostFromURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if parsed, err := url.Parse(s); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed.Host
	}

	// Windows local path: C:\repo or C:/repo. This is not an scp-style Git
	// URL and must not force a user git_token.
	if len(s) >= 3 && s[1] == ':' && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && (s[2] == '\\' || s[2] == '/') {
		return ""
	}

	// scp-like: [user@]host:path. A slash-only value is a local path, not a
	// remote URL.
	if !strings.Contains(s, ":") {
		return ""
	}
	if at := strings.Index(s, "@"); at >= 0 {
		s = s[at+1:]
	}
	if colon := strings.Index(s, ":"); colon >= 0 {
		return s[:colon]
	}
	return ""
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
var multicaIntegrationPlaceholder = regexp.MustCompile(`\$\{(MULTICA_INTEGRATION_[A-Z0-9_]+)\}`)

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
		out = multicaIntegrationPlaceholder.ReplaceAllStringFunc(out, func(match string) string {
			name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
			replacement, ok := overrides[name]
			if !ok || strings.TrimSpace(replacement) == "" {
				missingCredential = true
				return ""
			}
			return replacement
		})
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
