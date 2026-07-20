package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPreviewDetectUsesDevelopmentContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: elsewhere"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{
		"scripts":{"dev":"vite"},
		"dependencies":{"vite":"1","vue":"3"}
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newPreviewDetectCommand()
	_ = cmd.Flags().Set("write", "true")
	out, err := captureStdout(t, func() error { return runPreviewDetect(cmd, []string{root}) })
	if err != nil {
		t.Fatalf("runPreviewDetect: %v", err)
	}
	var report struct {
		Repositories []struct {
			Targets []struct {
				Platform string `json:"platform"`
			} `json:"targets"`
		} `json:"repositories"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(report.Repositories) != 1 || len(report.Repositories[0].Targets) != 1 || report.Repositories[0].Targets[0].Platform != "web" {
		t.Fatalf("unexpected report: %#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, ".multica", "preview", "targets.json")); err != nil {
		t.Fatalf("targets.json not written: %v", err)
	}
}

const (
	previewTestIssueID   = "11111111-1111-1111-1111-111111111111"
	previewTestSessionID = "22222222-2222-2222-2222-222222222222"
)

func setPreviewTestEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("MULTICA_SERVER_URL", serverURL)
	t.Setenv("MULTICA_WORKSPACE_ID", "workspace-123")
	t.Setenv("MULTICA_TOKEN", "test-token")
	t.Setenv("MULTICA_AGENT_ID", "")
	t.Setenv("MULTICA_TASK_ID", "")
}

func writePreviewTestIssue(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         previewTestIssueID,
		"identifier": "MUL-42",
	})
}

func previewTestSession(status string) map[string]any {
	return map[string]any{
		"id":          previewTestSessionID,
		"issue_id":    previewTestIssueID,
		"platform":    "web",
		"provider":    "external_web",
		"title":       "Checkout",
		"preview_url": "https://preview.example.test",
		"status":      status,
	}
}

func TestRunPreviewCreateRequestAndJSONOutput(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Fatalf("decode create body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(previewTestSession("running"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	setPreviewTestEnv(t, srv.URL)

	cmd := newPreviewCreateCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", "https://preview.example.test")
	_ = cmd.Flags().Set("title", "Checkout")
	_ = cmd.Flags().Set("expires-at", "2026-07-14T10:00:00Z")
	_ = cmd.Flags().Set("output", "json")

	out, err := captureStdout(t, func() error { return runPreviewCreate(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewCreate: %v", err)
	}
	wantBody := map[string]any{
		"platform":    "web",
		"provider":    "external_web",
		"preview_url": "https://preview.example.test",
		"title":       "Checkout",
		"expires_at":  "2026-07-14T10:00:00Z",
	}
	if len(gotBody) != len(wantBody) {
		t.Fatalf("create body = %#v, want %#v", gotBody, wantBody)
	}
	for key, want := range wantBody {
		if got := gotBody[key]; got != want {
			t.Fatalf("create body[%q] = %#v, want %#v", key, got, want)
		}
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode create output %q: %v", out, err)
	}
	if result["id"] != previewTestSessionID || result["status"] != "running" {
		t.Fatalf("create output = %#v", result)
	}
}

func TestRunPreviewListRequestAndJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preview_sessions": []map[string]any{previewTestSession("running")},
				"total":            1,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	setPreviewTestEnv(t, srv.URL)

	cmd := newPreviewListCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("output", "json")

	out, err := captureStdout(t, func() error { return runPreviewList(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewList: %v", err)
	}
	var result struct {
		PreviewSessions []map[string]any `json:"preview_sessions"`
		Total           int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode list output %q: %v", out, err)
	}
	if result.Total != 1 || len(result.PreviewSessions) != 1 || result.PreviewSessions[0]["id"] != previewTestSessionID {
		t.Fatalf("list output = %#v", result)
	}
}

func TestRunPreviewStopRequestAndJSONOutput(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/preview-sessions/"+previewTestSessionID+"/stop" {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stop body: %v", err)
		}
		if strings.TrimSpace(string(body)) != "null" {
			t.Fatalf("stop body = %q, want null", body)
		}
		_ = json.NewEncoder(w).Encode(previewTestSession("stopped"))
	}))
	defer srv.Close()
	setPreviewTestEnv(t, srv.URL)

	cmd := newPreviewStopCommand()
	_ = cmd.Flags().Set("output", "json")
	out, err := captureStdout(t, func() error {
		return runPreviewStop(cmd, []string{previewTestSessionID})
	})
	if err != nil {
		t.Fatalf("runPreviewStop: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode stop output %q: %v", out, err)
	}
	if result["id"] != previewTestSessionID || result["status"] != "stopped" {
		t.Fatalf("stop output = %#v", result)
	}
}

func TestPreviewRequiredArguments(t *testing.T) {
	t.Run("create requires issue", func(t *testing.T) {
		err := runPreviewCreate(newPreviewCreateCommand(), nil)
		if err == nil || err.Error() != "--issue is required" {
			t.Fatalf("error = %v, want --issue is required", err)
		}
	})

	t.Run("create requires url", func(t *testing.T) {
		cmd := newPreviewCreateCommand()
		_ = cmd.Flags().Set("issue", "MUL-42")
		err := runPreviewCreate(cmd, nil)
		if err == nil || err.Error() != "--url is required" {
			t.Fatalf("error = %v, want --url is required", err)
		}
	})

	t.Run("device create requires pinned serial", func(t *testing.T) {
		device := newPreviewDeviceCommand()
		cmd, _, err := device.Find([]string{"create"})
		if err != nil {
			t.Fatal(err)
		}
		_ = cmd.Flags().Set("issue", "MUL-42")
		_ = cmd.Flags().Set("url", "http://127.0.0.1:18081")
		err = runPreviewDeviceCreate(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "must pin a device") {
			t.Fatalf("error = %v, want pinned device guidance", err)
		}
	})

	t.Run("list requires issue", func(t *testing.T) {
		err := runPreviewList(newPreviewListCommand(), nil)
		if err == nil || err.Error() != "--issue is required" {
			t.Fatalf("error = %v, want --issue is required", err)
		}
	})

	t.Run("stop requires one session id", func(t *testing.T) {
		cmd := newPreviewStopCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Args(cmd, nil); err == nil {
			t.Fatal("expected missing session id to fail")
		}
		if err := cmd.Args(cmd, []string{"one", "two"}); err == nil {
			t.Fatal("expected extra session id to fail")
		}
	})
}
