package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/creative"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

func TestNormalizeWorkspaceMCPInputDefaults(t *testing.T) {
	input := workspaceMCPConnectionInput{
		Name:      " Creative service ",
		ServerURL: "https://creative.example.com/mcp",
	}
	normalized, headers, err := normalizeWorkspaceMCPInput(input, false)
	if err != nil {
		t.Fatalf("normalize input: %v", err)
	}
	if normalized.Name != "Creative service" {
		t.Fatalf("name = %q", normalized.Name)
	}
	if normalized.Transport != "streamable_http" {
		t.Fatalf("transport = %q", normalized.Transport)
	}
	if normalized.ToolCreate != creative.CreateJobToolName || normalized.ToolGet != creative.GetJobToolName {
		t.Fatalf("tools = %q, %q", normalized.ToolCreate, normalized.ToolGet)
	}
	if normalized.IsDefault == nil || !*normalized.IsDefault {
		t.Fatal("new connection should default to the workspace default")
	}
	if len(headers) != 0 {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestNormalizeWorkspaceMCPInputRejectsManagedHeaders(t *testing.T) {
	headers := map[string]string{"Content-Type": "text/plain"}
	_, _, err := normalizeWorkspaceMCPInput(workspaceMCPConnectionInput{
		Name:          "Creative service",
		ServerURL:     "https://creative.example.com/mcp",
		SecretHeaders: &headers,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "managed by Multica") {
		t.Fatalf("error = %v", err)
	}
}

func TestEncryptWorkspaceMCPHeaders(t *testing.T) {
	box, err := secretbox.New([]byte("01234567890123456789012345678901"))
	if err != nil {
		t.Fatalf("create secret box: %v", err)
	}
	h := &Handler{WorkspaceMCPSecretBox: box}
	sealed, names, err := h.encryptWorkspaceMCPHeaders(map[string]string{
		"X-Api-Key":     "secret",
		"Authorization": "Bearer token",
	})
	if err != nil {
		t.Fatalf("encrypt headers: %v", err)
	}
	if strings.Contains(string(sealed), "Bearer token") {
		t.Fatal("ciphertext contains the plaintext token")
	}
	if len(names) != 2 || names[0] != "Authorization" || names[1] != "X-Api-Key" {
		t.Fatalf("names = %#v", names)
	}
	plaintext, err := box.Open(sealed)
	if err != nil {
		t.Fatalf("decrypt headers: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(plaintext, &decoded); err != nil {
		t.Fatalf("decode headers: %v", err)
	}
	if decoded["Authorization"] != "Bearer token" || decoded["X-Api-Key"] != "secret" {
		t.Fatalf("decoded = %#v", decoded)
	}
}
