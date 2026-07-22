package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestResolveDeviceRuntimeURLUsesMacMobilePreviewLabel(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/runtimes" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"status": "online", "metadata": map[string]any{"label": "other", "device_runtime_url": "http://127.0.0.1:19000"}},
			{"id": "codex", "daemon_id": "mac-1", "status": "online", "metadata": map[string]any{"label": "mac-mobile-preview", "device_runtime_url": "http://127.0.0.1:18081"}},
			{"id": "claude", "daemon_id": "mac-1", "status": "online", "metadata": map[string]any{"label": "mac-mobile-preview", "device_runtime_url": "http://127.0.0.1:18081"}},
		})
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	client, err := newAPIClient(cmd)
	if err != nil {
		t.Fatalf("newAPIClient: %v", err)
	}
	got, err := resolveDeviceRuntimeURL(context.Background(), client, cmd)
	if err != nil {
		t.Fatalf("resolveDeviceRuntimeURL: %v", err)
	}
	if got != "http://127.0.0.1:18081" {
		t.Fatalf("runtime URL = %q", got)
	}
}

func androidPreviewTestSession(runtimeURL string) map[string]any {
	return map[string]any{
		"id":          previewTestSessionID,
		"issue_id":    previewTestIssueID,
		"platform":    "android",
		"provider":    "local_device",
		"title":       "Android preview",
		"preview_url": runtimeURL,
		"status":      "running",
	}
}

func newPreviewRuntimeTestServer(t *testing.T, deployStatus int, inputs *[]map[string]any, mu *sync.Mutex) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []runtimeDevice{
				{Serial: "emulator-5554", State: "device", Kind: "emulator"},
				{Serial: "USB-123", State: "device", Kind: "physical", Model: "Pixel 8"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/deploy":
			if deployStatus >= http.StatusBadRequest {
				w.WriteHeader(deployStatus)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "install failed"})
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["serial"] != "emulator-5554" {
				t.Errorf("deploy serial = %v, want emulator-5554", body["serial"])
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "running"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/input":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			*inputs = append(*inputs, body)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/webview/action":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			*inputs = append(*inputs, body)
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/webview/snapshot":
			if r.URL.Query().Get("serial") != "USB-123" {
				t.Errorf("snapshot serial = %q, want USB-123", r.URL.Query().Get("serial"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"url":      "http://127.0.0.1/register",
				"elements": []map[string]any{{"role": "textbox", "placeholder": "Nomor HP"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRunDeviceScenarioUsesSemanticWebViewActions(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()
	scenario := deviceScenario{
		Name: "OTP login",
		Steps: []deviceScenarioStep{
			{Action: "fill", Selector: deviceScenarioSelector{Role: "textbox", Name: "OTP"}, Value: "123456"},
			{Action: "assert", Selector: deviceScenarioSelector{Role: "textbox", Name: "OTP"}, Value: "123456"},
			{Action: "tap", Selector: deviceScenarioSelector{Role: "button", Name: "Sah"}},
		},
	}
	if err := validateDeviceScenario(scenario); err != nil {
		t.Fatalf("validateDeviceScenario: %v", err)
	}
	result, err := runDeviceScenario(t.Context(), runtimeServer.Client(), runtimeServer.URL, "emulator-5554", scenario)
	if err != nil {
		t.Fatalf("runDeviceScenario: %v", err)
	}
	if result.StepCount != 3 || len(inputs) != 3 {
		t.Fatalf("result = %+v, inputs = %#v", result, inputs)
	}
	if inputs[0]["action"] != "fill" || inputs[0]["value"] != "123456" {
		t.Fatalf("fill input = %#v", inputs[0])
	}
	selector, ok := inputs[0]["selector"].(map[string]any)
	if !ok || selector["role"] != "textbox" || selector["name"] != "OTP" {
		t.Fatalf("selector = %#v", inputs[0]["selector"])
	}
}

func TestValidateDeviceScenarioRejectsTextForFill(t *testing.T) {
	scenario := deviceScenario{
		Name: "invalid fill",
		Steps: []deviceScenarioStep{{
			Action:   "fill",
			Selector: deviceScenarioSelector{Role: "textbox", Name: "Phone"},
			Text:     "123456",
		}},
	}
	if err := validateDeviceScenario(scenario); err == nil || !strings.Contains(err.Error(), "uses value, not text") {
		t.Fatalf("validateDeviceScenario error = %v", err)
	}
}

func TestRunPreviewDeviceSyncReusesRunningSessionAndRunsScenario(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()

	createCalls := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preview_sessions": []map[string]any{androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "emulator-5554"))},
				"total":            1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			createCalls++
			http.Error(w, "must not create", http.StatusInternalServerError)
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/touch":
			_ = json.NewEncoder(w).Encode(androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "emulator-5554")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	scenarioPath := filepath.Join(t.TempDir(), "login.json")
	if err := os.WriteFile(scenarioPath, []byte(`{
		"name":"Login form",
		"source_width":576,
		"source_height":1280,
		"steps":[
			{"action":"tap","x":260,"y":380},
			{"action":"text","text":"24680"},
			{"action":"key","key":"4"}
		]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	_ = cmd.Flags().Set("scenario", scenarioPath)
	out, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewDeviceSync: %v", err)
	}
	if createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", createCalls)
	}
	var result deviceSyncResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("decode sync output: %v", err)
	}
	if !result.Deployed || !result.Reused || result.Device.Serial != "emulator-5554" || result.Scenario == nil || result.Scenario.Status != "completed" {
		t.Fatalf("sync output = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	gotActions := make([]string, 0, len(inputs))
	for _, input := range inputs {
		gotActions = append(gotActions, input["type"].(string))
	}
	if !reflect.DeepEqual(gotActions, []string{"tap", "text", "key"}) {
		t.Fatalf("input actions = %#v", gotActions)
	}
	if inputs[0]["source_width"] != float64(576) || inputs[0]["source_height"] != float64(1280) {
		t.Fatalf("tap dimensions = %#v", inputs[0])
	}
}

func TestRunPreviewDeviceSyncDoesNotDeployWhenLeaseConflicts(t *testing.T) {
	deployCalls := 0
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []runtimeDevice{{Serial: "emulator-5554", State: "device", Kind: "emulator"}}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/deploy":
			deployCalls++
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "running"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer runtimeServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preview_sessions": []map[string]any{androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "emulator-5554"))},
				"total":            1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/touch":
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "device is leased by another issue"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	_, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) })
	if err == nil || !strings.Contains(err.Error(), "device is leased") || deployCalls != 0 {
		t.Fatalf("error = %v, deploy calls = %d", err, deployCalls)
	}
}

func TestRunPreviewDeviceSyncCreatesSessionWhenMissing(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()

	createCalls := 0
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"preview_sessions": []map[string]any{}, "total": 0})
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			createCalls++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["preview_url"] != deviceSessionURL(runtimeServer.URL, "emulator-5554") || body["platform"] != "android" || body["provider"] != "local_device" {
				t.Errorf("create body = %#v", body)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(androidPreviewTestSession(runtimeServer.URL))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	out, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewDeviceSync: %v", err)
	}
	var result deviceSyncResult
	_ = json.Unmarshal([]byte(out), &result)
	if result.Reused || createCalls != 1 {
		t.Fatalf("result reused = %v, create calls = %d", result.Reused, createCalls)
	}
}

func TestRunPreviewDeviceSyncInheritsActiveIssueDevice(t *testing.T) {
	deployedSerial := ""
	runtimeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/devices":
			_ = json.NewEncoder(w).Encode(map[string]any{"devices": []runtimeDevice{
				{Serial: "emulator-5554", State: "device", Kind: "emulator"},
				{Serial: "USB-123", State: "device", Kind: "physical"},
			}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/deploy":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			deployedSerial = body["serial"]
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "running"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer runtimeServer.Close()

	physicalSession := androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123"))
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"preview_sessions": []map[string]any{physicalSession}, "total": 1})
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/touch":
			_ = json.NewEncoder(w).Encode(physicalSession)
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	if _, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) }); err != nil {
		t.Fatalf("runPreviewDeviceSync: %v", err)
	}
	if deployedSerial != "USB-123" {
		t.Fatalf("deployed serial = %q, want active Issue device", deployedSerial)
	}
}

func TestRunPreviewDeviceRunControlsIssueBoundDevice(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preview_sessions": []map[string]any{androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123"))},
				"total":            1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/touch":
			_ = json.NewEncoder(w).Encode(androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	scenarioPath := filepath.Join(t.TempDir(), "otp.json")
	if err := os.WriteFile(scenarioPath, []byte(`{"name":"OTP","steps":[{"action":"text","text":"123456"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newPreviewDeviceRunCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("scenario", scenarioPath)
	out, err := captureStdout(t, func() error { return runPreviewDeviceRun(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewDeviceRun: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	device := result["device"].(map[string]any)
	if device["serial"] != "USB-123" {
		t.Fatalf("controlled device = %#v, want Issue-bound USB-123", device)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(inputs) != 1 || inputs[0]["serial"] != "USB-123" {
		t.Fatalf("inputs = %#v", inputs)
	}
}

func TestRunPreviewDeviceSnapshotInspectsIssueBoundDevice(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"preview_sessions": []map[string]any{androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123"))},
				"total":            1,
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/touch":
			_ = json.NewEncoder(w).Encode(androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123")))
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)
	cmd := newPreviewDeviceSnapshotCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	out, err := captureStdout(t, func() error { return runPreviewDeviceSnapshot(cmd, nil) })
	if err != nil {
		t.Fatalf("runPreviewDeviceSnapshot: %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	snapshot := result["snapshot"].(map[string]any)
	if snapshot["url"] != "http://127.0.0.1/register" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestActiveIssueDeviceTargetRejectsMissingOrAmbiguousBinding(t *testing.T) {
	runtimeURL := "http://127.0.0.1:18081"
	if _, _, _, err := activeIssueDeviceTarget(nil); err == nil {
		t.Fatal("expected missing active preview to fail")
	}
	unpinned := androidPreviewTestSession(runtimeURL)
	if _, _, _, err := activeIssueDeviceTarget([]map[string]any{unpinned}); err == nil {
		t.Fatal("expected unpinned preview to fail")
	}
	one := androidPreviewTestSession(deviceSessionURL(runtimeURL, "USB-123"))
	two := androidPreviewTestSession(deviceSessionURL(runtimeURL, "emulator-5554"))
	if _, _, _, err := activeIssueDeviceTarget([]map[string]any{one, two}); err == nil {
		t.Fatal("expected ambiguous active previews to fail")
	}
}

func TestRunPreviewDeviceSyncStopsSupersededDeviceSession(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusOK, &inputs, &mu)
	defer runtimeServer.Close()

	physicalSession := androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "USB-123"))
	physicalSession["id"] = "physical-session"
	stopped := ""
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42":
			writePreviewTestIssue(w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"preview_sessions": []map[string]any{physicalSession}, "total": 1})
		case r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions":
			created := androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "emulator-5554"))
			created["id"] = "emulator-session"
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/physical-session/stop":
			stopped = "physical-session"
			_ = json.NewEncoder(w).Encode(map[string]any{"id": stopped, "status": "stopped"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	_ = cmd.Flags().Set("serial", "emulator-5554")
	if _, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) }); err != nil {
		t.Fatalf("runPreviewDeviceSync: %v", err)
	}
	if stopped != "physical-session" {
		t.Fatalf("stopped session = %q", stopped)
	}
}

func TestDeviceSessionURLPinsSerial(t *testing.T) {
	got := deviceSessionURL("http://127.0.0.1:18081", "USB 123/ABC")
	if got != "http://127.0.0.1:18081?serial=USB+123%2FABC" {
		t.Fatalf("deviceSessionURL = %q", got)
	}
}

func TestActiveDeviceSessionSerialDoesNotInheritPastNewerUnpinnedSession(t *testing.T) {
	runtimeURL := "http://127.0.0.1:18081"
	newer := androidPreviewTestSession(runtimeURL)
	older := androidPreviewTestSession(deviceSessionURL(runtimeURL, "USB-123"))
	if got := activeDeviceSessionSerial([]map[string]any{newer, older}, runtimeURL); got != "" {
		t.Fatalf("active serial = %q, want default selection for newer unpinned session", got)
	}
}

func TestRunPreviewDeviceSyncReleasesNewLeaseWhenDeployFails(t *testing.T) {
	var inputs []map[string]any
	var mu sync.Mutex
	runtimeServer := newPreviewRuntimeTestServer(t, http.StatusBadRequest, &inputs, &mu)
	defer runtimeServer.Close()

	previewMutations := 0
	stopped := false
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/issues/MUL-42" {
			writePreviewTestIssue(w)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions" {
			_ = json.NewEncoder(w).Encode(map[string]any{"preview_sessions": []map[string]any{}, "total": 0})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/issues/"+previewTestIssueID+"/preview-sessions" {
			previewMutations++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(androidPreviewTestSession(deviceSessionURL(runtimeServer.URL, "emulator-5554")))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/preview-sessions/"+previewTestSessionID+"/stop" {
			previewMutations++
			stopped = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": previewTestSessionID, "status": "stopped"})
			return
		}
		http.NotFound(w, r)
	}))
	defer apiServer.Close()
	setPreviewTestEnv(t, apiServer.URL)

	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("issue", "MUL-42")
	_ = cmd.Flags().Set("url", runtimeServer.URL)
	_, err := captureStdout(t, func() error { return runPreviewDeviceSync(cmd, nil) })
	if err == nil || previewMutations != 2 || !stopped {
		t.Fatalf("error = %v, preview mutations = %d, stopped = %v", err, previewMutations, stopped)
	}
}

func TestDeviceRuntimeURLRejectsNonLoopback(t *testing.T) {
	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("url", "https://device.example.test")
	if _, err := deviceRuntimeURL(cmd); err == nil {
		t.Fatal("expected non-loopback URL to fail")
	}
}

func TestLoadDeviceScenarioRejectsTrailingJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, []byte(`{"name":"one","steps":[{"action":"wait"}]} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadDeviceScenario(path); err == nil {
		t.Fatal("expected trailing JSON to fail")
	}
}

func TestDeviceArtifactPathRequiresAPKFile(t *testing.T) {
	cmd := newPreviewDeviceSyncCommand()
	path := filepath.Join(t.TempDir(), "app-stage.apk")
	if err := os.WriteFile(path, []byte("apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("artifact", path)
	got, err := deviceArtifactPath(cmd)
	if err != nil {
		t.Fatalf("deviceArtifactPath: %v", err)
	}
	want, _ := filepath.Abs(path)
	if got != want {
		t.Fatalf("artifact = %q, want %q", got, want)
	}

	invalid := filepath.Join(t.TempDir(), "not-an-apk.txt")
	if err := os.WriteFile(invalid, []byte("text"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Flags().Set("artifact", invalid)
	if _, err := deviceArtifactPath(cmd); err == nil {
		t.Fatal("expected non-APK artifact to fail")
	}
}

func TestDeviceWebPreviewURL(t *testing.T) {
	cmd := newPreviewDeviceSyncCommand()
	_ = cmd.Flags().Set("web-url", "http://10.0.2.2:13000/login")
	got, err := deviceWebPreviewURL(cmd)
	if err != nil || got != "http://10.0.2.2:13000/login" {
		t.Fatalf("deviceWebPreviewURL = %q, %v", got, err)
	}
	_ = cmd.Flags().Set("web-url", "file:///tmp/app")
	if _, err := deviceWebPreviewURL(cmd); err == nil {
		t.Fatal("expected non-HTTP URL to fail")
	}
}
