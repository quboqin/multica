package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestClaudePlugin(t *testing.T, home, id, name string, enabled bool) string {
	t.Helper()
	installPath := filepath.Join(home, ".claude", "plugins", "cache", name, "1.0.0")
	if err := os.MkdirAll(filepath.Join(installPath, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"` + name + `","skills":"./skills","mcpServers":"./mcp.json"}`
	if err := os.WriteFile(filepath.Join(installPath, ".claude-plugin", "plugin.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "plugins"), 0o755); err != nil {
		t.Fatal(err)
	}
	installed := `{"version":2,"plugins":{"` + id + `":[{"scope":"user","installPath":"` + installPath + `"}]}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json"), []byte(installed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{"enabledPlugins":{"` + id + `":` + map[bool]string{true: "true", false: "false"}[enabled] + `}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	return installPath
}
