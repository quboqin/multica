package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/previewruntime"
)

const (
	maxScenarioBytes = 1 << 20
	maxScenarioSteps = 1000
	maxScenarioWait  = 10 * time.Minute
)

type runtimeDevice struct {
	Serial string `json:"serial"`
	State  string `json:"state"`
	Model  string `json:"model,omitempty"`
	Kind   string `json:"kind"`
}

type deviceScenario struct {
	Name         string               `json:"name"`
	SourceWidth  int                  `json:"source_width,omitempty"`
	SourceHeight int                  `json:"source_height,omitempty"`
	StepDelayMS  int                  `json:"step_delay_ms,omitempty"`
	Steps        []deviceScenarioStep `json:"steps"`
}

type deviceScenarioStep struct {
	Action       string                 `json:"action"`
	X            int                    `json:"x,omitempty"`
	Y            int                    `json:"y,omitempty"`
	EndX         int                    `json:"end_x,omitempty"`
	EndY         int                    `json:"end_y,omitempty"`
	Text         string                 `json:"text,omitempty"`
	Value        string                 `json:"value,omitempty"`
	URL          string                 `json:"url,omitempty"`
	Key          string                 `json:"key,omitempty"`
	DurationMS   int                    `json:"duration_ms,omitempty"`
	TimeoutMS    int                    `json:"timeout_ms,omitempty"`
	SourceWidth  int                    `json:"source_width,omitempty"`
	SourceHeight int                    `json:"source_height,omitempty"`
	Selector     deviceScenarioSelector `json:"selector,omitempty"`
}

type deviceScenarioSelector struct {
	Role        string `json:"role,omitempty"`
	Name        string `json:"name,omitempty"`
	Text        string `json:"text,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	TestID      string `json:"test_id,omitempty"`
	CSS         string `json:"css,omitempty"`
	Contains    bool   `json:"contains,omitempty"`
}

type scenarioRunResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	StepCount  int    `json:"step_count"`
	DurationMS int64  `json:"duration_ms"`
}

type deviceSyncResult struct {
	Status         string             `json:"status"`
	Deployed       bool               `json:"deployed"`
	Reused         bool               `json:"reused"`
	Device         runtimeDevice      `json:"device"`
	PreviewSession map[string]any     `json:"preview_session"`
	Scenario       *scenarioRunResult `json:"scenario,omitempty"`
}

func newPreviewDeviceSyncCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Deploy the configured APK and reuse the issue device preview",
		Args:  cobra.NoArgs,
		RunE:  runPreviewDeviceSync,
	}
	cmd.Flags().String("issue", "", "Issue identifier or ID (required)")
	cmd.Flags().String("url", "", "Loopback Device Runtime URL (defaults to the online mac-mobile-preview Runtime)")
	cmd.Flags().String("serial", "", "ADB device serial (defaults to an online emulator)")
	cmd.Flags().String("artifact", "", "APK built at this checkpoint (defaults to the Device Runtime configured artifact)")
	cmd.Flags().String("web-url", "", "HTTP(S) H5 URL for a preview-enabled application shell")
	cmd.Flags().String("title", "Local Android device", "Preview title when creating a session")
	cmd.Flags().String("expires-at", "", "Optional expiration timestamp when creating a session (RFC3339)")
	cmd.Flags().String("scenario", "", "Optional device scenario JSON to run after deployment")
	cmd.Flags().String("output", "json", "Output format: table or json")
	return cmd
}

func newPreviewDeviceRunCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run an interaction scenario on the Android preview attached to an issue",
		Args:  cobra.NoArgs,
		RunE:  runPreviewDeviceRun,
	}
	cmd.Flags().String("issue", "", "Issue identifier or ID whose active device preview will be controlled (required)")
	cmd.Flags().String("scenario", "", "Device scenario JSON (required)")
	cmd.Flags().String("output", "json", "Output format: table or json")
	return cmd
}

func newPreviewDeviceSnapshotCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Inspect the visible WebView semantics of the Android preview attached to an issue",
		Args:  cobra.NoArgs,
		RunE:  runPreviewDeviceSnapshot,
	}
	cmd.Flags().String("issue", "", "Issue identifier or ID whose active device preview will be inspected (required)")
	cmd.Flags().String("output", "json", "Output format: json")
	return cmd
}

func runPreviewDeviceSync(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	commandTimeout := cli.AtLeastAPITimeout(2 * time.Minute)
	if scenarioPath, _ := cmd.Flags().GetString("scenario"); strings.TrimSpace(scenarioPath) != "" {
		commandTimeout += maxScenarioWait
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	runtimeURL, err := resolveDeviceRuntimeURL(ctx, client, cmd)
	if err != nil {
		return err
	}
	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	var sessions previewSessionListResponse
	if err := client.GetJSON(ctx, path, &sessions); err != nil {
		return fmt.Errorf("list preview sessions: %w", err)
	}

	runtimeClient := &http.Client{Timeout: 50 * time.Second}
	requestedSerial, _ := cmd.Flags().GetString("serial")
	requestedSerial = strings.TrimSpace(requestedSerial)
	explicitSerial := requestedSerial != ""
	if !explicitSerial {
		requestedSerial = activeDeviceSessionSerial(sessions.PreviewSessions, runtimeURL)
	}
	device, err := selectRuntimeDevice(ctx, runtimeClient, runtimeURL, requestedSerial)
	if err != nil && !explicitSerial && requestedSerial != "" {
		device, err = selectRuntimeDevice(ctx, runtimeClient, runtimeURL, "")
	}
	if err != nil {
		return err
	}
	sessionURL := deviceSessionURL(runtimeURL, device.Serial)
	session, reused := findRunningDeviceSession(sessions.PreviewSessions, sessionURL)
	if reused {
		if err := touchPreviewDeviceSession(ctx, client, session); err != nil {
			return err
		}
	} else {
		title, _ := cmd.Flags().GetString("title")
		body := map[string]any{
			"platform":    "android",
			"provider":    "local_device",
			"preview_url": sessionURL,
			"title":       strings.TrimSpace(title),
		}
		if expiresAt, _ := cmd.Flags().GetString("expires-at"); strings.TrimSpace(expiresAt) != "" {
			body["expires_at"] = strings.TrimSpace(expiresAt)
		}
		if err := client.PostJSON(ctx, path, body, &session); err != nil {
			return fmt.Errorf("create device preview session: %w", err)
		}
	}
	ctx = withRuntimeSession(ctx, strVal(session, "id"))
	deployRequest := map[string]string{"serial": device.Serial, "session_id": strVal(session, "id")}
	if artifact, err := deviceArtifactPath(cmd); err != nil {
		return err
	} else if artifact != "" {
		deployRequest["artifact"] = artifact
	}
	if webURL, err := deviceWebPreviewURL(cmd); err != nil {
		return err
	} else if webURL != "" {
		deployRequest["web_url"] = webURL
	}
	if err := postRuntimeJSON(ctx, runtimeClient, runtimeURL+"/api/deploy", deployRequest, nil); err != nil {
		if !reused {
			_ = stopPreviewDeviceSession(ctx, client, strVal(session, "id"))
		}
		return fmt.Errorf("deploy APK to %s: %w", device.Serial, err)
	}
	if err := stopOtherDeviceSessions(ctx, client, sessions.PreviewSessions, runtimeURL, strVal(session, "id")); err != nil {
		return err
	}

	result := deviceSyncResult{
		Status:         "running",
		Deployed:       true,
		Reused:         reused,
		Device:         device,
		PreviewSession: session,
	}
	if scenarioPath, _ := cmd.Flags().GetString("scenario"); strings.TrimSpace(scenarioPath) != "" {
		scenario, err := loadDeviceScenario(strings.TrimSpace(scenarioPath))
		if err != nil {
			return err
		}
		run, err := runDeviceScenario(ctx, runtimeClient, runtimeURL, device.Serial, scenario)
		if err != nil {
			return err
		}
		result.Scenario = &run
	}
	return printDeviceSyncResult(cmd, result)
}

func deviceWebPreviewURL(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("web-url")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.ParseRequestURI(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", fmt.Errorf("--web-url must be an HTTP(S) URL without credentials")
	}
	return parsed.String(), nil
}

func deviceArtifactPath(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("artifact")
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve --artifact: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("read --artifact: %w", err)
	}
	if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(absolute), ".apk") {
		return "", fmt.Errorf("--artifact must be a regular .apk file")
	}
	return absolute, nil
}

func runPreviewDeviceRun(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}
	scenarioPath, _ := cmd.Flags().GetString("scenario")
	scenarioPath = strings.TrimSpace(scenarioPath)
	if scenarioPath == "" {
		return fmt.Errorf("--scenario is required")
	}
	scenario, err := loadDeviceScenario(scenarioPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(2*time.Minute)+maxScenarioWait)
	defer cancel()
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	var sessions previewSessionListResponse
	if err := client.GetJSON(ctx, path, &sessions); err != nil {
		return fmt.Errorf("list preview sessions: %w", err)
	}
	runtimeURL, serial, session, err := activeIssueDeviceTarget(sessions.PreviewSessions)
	if err != nil {
		return err
	}
	ctx = withRuntimeSession(ctx, strVal(session, "id"))
	if err := touchPreviewDeviceSession(ctx, client, session); err != nil {
		return err
	}
	runtimeClient := &http.Client{Timeout: 50 * time.Second}
	device, err := selectRuntimeDevice(ctx, runtimeClient, runtimeURL, serial)
	if err != nil {
		return fmt.Errorf("active Issue preview is unavailable: %w", err)
	}
	result, err := runDeviceScenario(ctx, runtimeClient, runtimeURL, device.Serial, scenario)
	if err != nil {
		return err
	}
	return cli.PrintJSON(os.Stdout, map[string]any{
		"device":          device,
		"preview_session": session,
		"scenario":        result,
	})
}

func runPreviewDeviceSnapshot(cmd *cobra.Command, _ []string) error {
	issueInput, _ := cmd.Flags().GetString("issue")
	issueInput = strings.TrimSpace(issueInput)
	if issueInput == "" {
		return fmt.Errorf("--issue is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), cli.AtLeastAPITimeout(30*time.Second))
	defer cancel()
	client, err := newAPIClient(cmd)
	if err != nil {
		return err
	}
	issueRef, err := resolveIssueRef(ctx, client, issueInput)
	if err != nil {
		return fmt.Errorf("resolve issue: %w", err)
	}
	path := "/api/issues/" + url.PathEscape(issueRef.ID) + "/preview-sessions"
	var sessions previewSessionListResponse
	if err := client.GetJSON(ctx, path, &sessions); err != nil {
		return fmt.Errorf("list preview sessions: %w", err)
	}
	runtimeURL, serial, session, err := activeIssueDeviceTarget(sessions.PreviewSessions)
	if err != nil {
		return err
	}
	ctx = withRuntimeSession(ctx, strVal(session, "id"))
	if err := touchPreviewDeviceSession(ctx, client, session); err != nil {
		return err
	}
	runtimeClient := &http.Client{Timeout: 15 * time.Second}
	device, err := selectRuntimeDevice(ctx, runtimeClient, runtimeURL, serial)
	if err != nil {
		return fmt.Errorf("active Issue preview is unavailable: %w", err)
	}
	var snapshot any
	endpoint := runtimeURL + "/api/webview/snapshot?serial=" + url.QueryEscape(serial)
	if err := getRuntimeJSON(ctx, runtimeClient, endpoint, &snapshot); err != nil {
		return fmt.Errorf("inspect active Issue preview: %w", err)
	}
	return cli.PrintJSON(os.Stdout, map[string]any{
		"device":          device,
		"preview_session": session,
		"snapshot":        snapshot,
	})
}

func activeIssueDeviceTarget(sessions []map[string]any) (string, string, map[string]any, error) {
	var active []map[string]any
	for _, session := range sessions {
		if isUsableDeviceSessionStatus(strVal(session, "status")) && strVal(session, "platform") == "android" && strVal(session, "provider") == "local_device" {
			active = append(active, session)
		}
	}
	if len(active) == 0 {
		return "", "", nil, fmt.Errorf("issue has no active Android device preview; deploy one with 'multica preview device sync --issue <issue>'")
	}
	if len(active) > 1 {
		return "", "", nil, fmt.Errorf("issue has %d running Android device previews; keep exactly one active before running a scenario", len(active))
	}
	previewURL, err := url.Parse(strVal(active[0], "preview_url"))
	if err != nil || previewURL.Scheme != "http" || previewURL.Hostname() == "" || previewURL.User != nil {
		return "", "", nil, fmt.Errorf("active Android preview has an invalid Device Runtime URL")
	}
	serialValues, ok := previewURL.Query()["serial"]
	if !ok || len(serialValues) != 1 || strings.TrimSpace(serialValues[0]) == "" {
		return "", "", nil, fmt.Errorf("active Android preview is not pinned to exactly one device")
	}
	serial := strings.TrimSpace(serialValues[0])
	previewURL.RawQuery = ""
	previewURL.Fragment = ""
	if previewURL.Path != "" && previewURL.Path != "/" {
		return "", "", nil, fmt.Errorf("active Android preview has an invalid Device Runtime path")
	}
	runtimeURL := strings.TrimRight(previewURL.String(), "/")
	host := previewURL.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return "", "", nil, fmt.Errorf("active Android preview must use a loopback Device Runtime URL")
	}
	return runtimeURL, serial, active[0], nil
}

func deviceRuntimeURL(cmd *cobra.Command) (string, error) {
	raw, _ := cmd.Flags().GetString("url")
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("--url is required when no online %s Runtime is registered", previewruntime.MacMobilePreviewLabel)
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("--url must be a loopback HTTP Device Runtime URL")
	}
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return "", fmt.Errorf("--url must use localhost or a loopback IP")
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("--url must not include a path")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

type registeredDeviceRuntime struct {
	ID       string `json:"id"`
	DaemonID string `json:"daemon_id"`
	Status   string `json:"status"`
	Metadata struct {
		Label            string `json:"label"`
		DeviceRuntimeURL string `json:"device_runtime_url"`
	} `json:"metadata"`
}

func resolveDeviceRuntimeURL(ctx context.Context, client *cli.APIClient, cmd *cobra.Command) (string, error) {
	if raw, _ := cmd.Flags().GetString("url"); strings.TrimSpace(raw) != "" {
		return deviceRuntimeURL(cmd)
	}
	var runtimes []registeredDeviceRuntime
	if err := client.GetJSON(ctx, "/api/runtimes", &runtimes); err != nil {
		return "", fmt.Errorf("list runtimes for %s: %w", previewruntime.MacMobilePreviewLabel, err)
	}
	hosts := make(map[string]string)
	for _, runtime := range runtimes {
		if runtime.Status == "online" && runtime.Metadata.Label == previewruntime.MacMobilePreviewLabel && strings.TrimSpace(runtime.Metadata.DeviceRuntimeURL) != "" {
			hostID := strings.TrimSpace(runtime.DaemonID)
			if hostID == "" {
				hostID = runtime.ID
			}
			if existing, ok := hosts[hostID]; ok && existing != runtime.Metadata.DeviceRuntimeURL {
				return "", fmt.Errorf("Runtime host %s advertises conflicting Device Runtime URLs", hostID)
			}
			hosts[hostID] = runtime.Metadata.DeviceRuntimeURL
		}
	}
	if len(hosts) == 0 {
		return "", fmt.Errorf("no online Runtime labeled %s", previewruntime.MacMobilePreviewLabel)
	}
	if len(hosts) > 1 {
		return "", fmt.Errorf("multiple online Runtimes labeled %s; keep exactly one online", previewruntime.MacMobilePreviewLabel)
	}
	var runtimeURL string
	for _, candidate := range hosts {
		runtimeURL = candidate
	}
	if err := cmd.Flags().Set("url", runtimeURL); err != nil {
		return "", err
	}
	return deviceRuntimeURL(cmd)
}

func selectRuntimeDevice(ctx context.Context, client *http.Client, runtimeURL, requestedSerial string) (runtimeDevice, error) {
	var response struct {
		Devices []runtimeDevice `json:"devices"`
	}
	if err := getRuntimeJSON(ctx, client, runtimeURL+"/api/devices", &response); err != nil {
		return runtimeDevice{}, fmt.Errorf("list Device Runtime devices: %w", err)
	}
	if requestedSerial != "" {
		for _, device := range response.Devices {
			if device.Serial == requestedSerial {
				if device.State != "device" {
					return runtimeDevice{}, fmt.Errorf("device %s is not online (state %s)", requestedSerial, device.State)
				}
				return device, nil
			}
		}
		return runtimeDevice{}, fmt.Errorf("device %s was not found", requestedSerial)
	}
	for _, kind := range []string{"emulator", "physical"} {
		for _, device := range response.Devices {
			if device.State == "device" && device.Kind == kind {
				return device, nil
			}
		}
	}
	return runtimeDevice{}, fmt.Errorf("Device Runtime has no online Android device")
}

func findRunningDeviceSession(sessions []map[string]any, runtimeURL string) (map[string]any, bool) {
	for _, session := range sessions {
		if isUsableDeviceSessionStatus(strVal(session, "status")) && strVal(session, "platform") == "android" &&
			strVal(session, "provider") == "local_device" && strings.TrimRight(strVal(session, "preview_url"), "/") == runtimeURL {
			return session, true
		}
	}
	return nil, false
}

func activeDeviceSessionSerial(sessions []map[string]any, runtimeURL string) string {
	for _, session := range sessions {
		if !isRunningDeviceSessionForRuntime(session, runtimeURL) {
			continue
		}
		parsed, err := url.Parse(strVal(session, "preview_url"))
		if err == nil {
			if serial := strings.TrimSpace(parsed.Query().Get("serial")); serial != "" {
				return serial
			}
		}
		// The newest matching session is the active intent. Legacy unpinned
		// sessions fall back to normal device selection instead of inheriting a
		// serial from an older card.
		return ""
	}
	return ""
}

func isRunningDeviceSessionForRuntime(session map[string]any, runtimeURL string) bool {
	if !isUsableDeviceSessionStatus(strVal(session, "status")) || strVal(session, "platform") != "android" || strVal(session, "provider") != "local_device" {
		return false
	}
	sessionURL, err := url.Parse(strVal(session, "preview_url"))
	if err != nil {
		return false
	}
	baseURL, err := url.Parse(runtimeURL)
	if err != nil {
		return false
	}
	sessionURL.RawQuery, sessionURL.Fragment = "", ""
	baseURL.RawQuery, baseURL.Fragment = "", ""
	return strings.TrimRight(sessionURL.String(), "/") == strings.TrimRight(baseURL.String(), "/")
}

func isUsableDeviceSessionStatus(status string) bool {
	return status == "running" || status == "sleeping"
}

func touchPreviewDeviceSession(ctx context.Context, client *cli.APIClient, session map[string]any) error {
	sessionID := strVal(session, "id")
	if sessionID == "" {
		return fmt.Errorf("active Android preview has no session id")
	}
	path := "/api/preview-sessions/" + url.PathEscape(sessionID) + "/touch"
	var touched map[string]any
	if err := client.PostJSON(ctx, path, nil, &touched); err != nil {
		return fmt.Errorf("acquire Android preview device lease: %w", err)
	}
	for key := range session {
		delete(session, key)
	}
	for key, value := range touched {
		session[key] = value
	}
	return nil
}

func stopPreviewDeviceSession(ctx context.Context, client *cli.APIClient, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	path := "/api/preview-sessions/" + url.PathEscape(sessionID) + "/stop"
	var stopped map[string]any
	return client.PostJSON(ctx, path, nil, &stopped)
}

func stopOtherDeviceSessions(ctx context.Context, client *cli.APIClient, sessions []map[string]any, runtimeURL, keepSessionID string) error {
	for _, session := range sessions {
		if !isRunningDeviceSessionForRuntime(session, runtimeURL) {
			continue
		}
		sessionID := strVal(session, "id")
		if sessionID == "" || sessionID == keepSessionID {
			continue
		}
		path := "/api/preview-sessions/" + url.PathEscape(sessionID) + "/stop"
		var stopped map[string]any
		if err := client.PostJSON(ctx, path, nil, &stopped); err != nil {
			return fmt.Errorf("stop superseded device preview %s: %w", sessionID, err)
		}
	}
	return nil
}

func deviceSessionURL(runtimeURL, serial string) string {
	parsed, err := url.Parse(runtimeURL)
	if err != nil {
		return runtimeURL
	}
	query := parsed.Query()
	query.Set("serial", serial)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func loadDeviceScenario(path string) (deviceScenario, error) {
	file, err := os.Open(path)
	if err != nil {
		return deviceScenario{}, fmt.Errorf("open device scenario: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxScenarioBytes+1))
	if err != nil {
		return deviceScenario{}, fmt.Errorf("read device scenario: %w", err)
	}
	if len(data) > maxScenarioBytes {
		return deviceScenario{}, fmt.Errorf("device scenario exceeds 1 MiB")
	}
	var scenario deviceScenario
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scenario); err != nil {
		return deviceScenario{}, fmt.Errorf("decode device scenario: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return deviceScenario{}, fmt.Errorf("decode device scenario: %w", err)
	}
	if err := validateDeviceScenario(scenario); err != nil {
		return deviceScenario{}, err
	}
	return scenario, nil
}

func validateDeviceScenario(scenario deviceScenario) error {
	if strings.TrimSpace(scenario.Name) == "" {
		return fmt.Errorf("device scenario name is required")
	}
	if len(scenario.Steps) == 0 || len(scenario.Steps) > maxScenarioSteps {
		return fmt.Errorf("device scenario must contain 1 to %d steps", maxScenarioSteps)
	}
	if (scenario.SourceWidth == 0) != (scenario.SourceHeight == 0) || scenario.SourceWidth < 0 || scenario.SourceHeight < 0 {
		return fmt.Errorf("device scenario source_width and source_height must both be positive or omitted")
	}
	if scenario.StepDelayMS < 0 || scenario.StepDelayMS > 5_000 {
		return fmt.Errorf("device scenario step_delay_ms must be between 0 and 5000")
	}
	var totalWait time.Duration
	for index, step := range scenario.Steps {
		if (step.SourceWidth == 0) != (step.SourceHeight == 0) || step.SourceWidth < 0 || step.SourceHeight < 0 {
			return fmt.Errorf("device scenario step %d has invalid source dimensions", index+1)
		}
		switch step.Action {
		case "tap":
			if selectorEmpty(step.Selector) && (step.X < 0 || step.Y < 0) {
				return fmt.Errorf("device scenario step %d has negative coordinates", index+1)
			}
		case "swipe":
			if step.X < 0 || step.Y < 0 || step.EndX < 0 || step.EndY < 0 {
				return fmt.Errorf("device scenario step %d has negative coordinates", index+1)
			}
		case "fill", "assert", "wait_for":
			if selectorEmpty(step.Selector) {
				return fmt.Errorf("device scenario step %d selector is required", index+1)
			}
			if step.Action == "fill" && step.Text != "" {
				return fmt.Errorf("device scenario step %d fill uses value, not text", index+1)
			}
			if step.TimeoutMS < 0 || step.TimeoutMS > 30_000 {
				return fmt.Errorf("device scenario step %d timeout_ms must be between 0 and 30000", index+1)
			}
		case "text":
			if step.Text == "" || len(step.Text) > 4096 {
				return fmt.Errorf("device scenario step %d text must contain 1 to 4096 bytes", index+1)
			}
		case "key":
			if strings.TrimSpace(step.Key) == "" {
				return fmt.Errorf("device scenario step %d key is required", index+1)
			}
		case "wait":
			if step.DurationMS < 0 || step.DurationMS > int(maxScenarioWait/time.Millisecond) {
				return fmt.Errorf("device scenario step %d has invalid duration_ms", index+1)
			}
			totalWait += time.Duration(step.DurationMS) * time.Millisecond
		default:
			return fmt.Errorf("device scenario step %d has unsupported action %q", index+1, step.Action)
		}
	}
	totalWait += time.Duration(scenario.StepDelayMS*len(scenario.Steps)) * time.Millisecond
	if totalWait > maxScenarioWait {
		return fmt.Errorf("device scenario total wait exceeds %s", maxScenarioWait)
	}
	return nil
}

func selectorEmpty(selector deviceScenarioSelector) bool {
	return selector.Role == "" && selector.Name == "" && selector.Text == "" && selector.Placeholder == "" && selector.TestID == "" && selector.CSS == ""
}

func runDeviceScenario(ctx context.Context, client *http.Client, runtimeURL, serial string, scenario deviceScenario) (scenarioRunResult, error) {
	started := time.Now()
	for index, step := range scenario.Steps {
		if step.Action == "wait" {
			timer := time.NewTimer(time.Duration(step.DurationMS) * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return scenarioRunResult{}, ctx.Err()
			case <-timer.C:
			}
			if err := waitScenarioDelay(ctx, scenario.StepDelayMS); err != nil {
				return scenarioRunResult{}, err
			}
			continue
		}
		if step.Action == "fill" || step.Action == "assert" || step.Action == "wait_for" || (step.Action == "tap" && !selectorEmpty(step.Selector)) {
			body := map[string]any{
				"serial":   serial,
				"action":   step.Action,
				"selector": step.Selector,
			}
			if step.Value != "" {
				body["value"] = step.Value
			}
			if step.Text != "" {
				body["text"] = step.Text
			}
			if step.URL != "" {
				body["url"] = step.URL
			}
			if step.TimeoutMS > 0 {
				body["timeout_ms"] = step.TimeoutMS
			}
			if err := postRuntimeJSON(ctx, client, runtimeURL+"/api/webview/action", body, nil); err != nil {
				return scenarioRunResult{}, fmt.Errorf("run device scenario step %d (%s): %w", index+1, step.Action, err)
			}
			if err := waitScenarioDelay(ctx, scenario.StepDelayMS); err != nil {
				return scenarioRunResult{}, err
			}
			continue
		}
		body := map[string]any{"serial": serial, "type": step.Action}
		switch step.Action {
		case "tap":
			body["x"], body["y"] = step.X, step.Y
		case "swipe":
			body["x"], body["y"], body["end_x"], body["end_y"] = step.X, step.Y, step.EndX, step.EndY
		case "text":
			body["text"] = step.Text
		case "key":
			body["key"] = strings.TrimSpace(step.Key)
		}
		width, height := scenario.SourceWidth, scenario.SourceHeight
		if step.SourceWidth > 0 {
			width, height = step.SourceWidth, step.SourceHeight
		}
		if (step.Action == "tap" || step.Action == "swipe") && width > 0 {
			body["source_width"], body["source_height"] = width, height
		}
		if err := postRuntimeJSON(ctx, client, runtimeURL+"/api/input", body, nil); err != nil {
			return scenarioRunResult{}, fmt.Errorf("run device scenario step %d (%s): %w", index+1, step.Action, err)
		}
		if err := waitScenarioDelay(ctx, scenario.StepDelayMS); err != nil {
			return scenarioRunResult{}, err
		}
	}
	return scenarioRunResult{
		Name:       scenario.Name,
		Status:     "completed",
		StepCount:  len(scenario.Steps),
		DurationMS: time.Since(started).Milliseconds(),
	}, nil
}

func waitScenarioDelay(ctx context.Context, delayMS int) error {
	if delayMS <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(delayMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func getRuntimeJSON(ctx context.Context, client *http.Client, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	authorizeRuntimeRequest(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return runtimeHTTPError(resp)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxScenarioBytes)).Decode(out)
}

func postRuntimeJSON(ctx context.Context, client *http.Client, endpoint string, body, out any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	authorizeRuntimeRequest(req)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		return runtimeHTTPError(resp)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, maxScenarioBytes)).Decode(out)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxScenarioBytes))
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

type runtimeSessionContextKey struct{}

func withRuntimeSession(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, runtimeSessionContextKey{}, strings.TrimSpace(sessionID))
}

func authorizeRuntimeRequest(req *http.Request) {
	if token := strings.TrimSpace(os.Getenv("MULTICA_DEVICE_RUNTIME_TOKEN")); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if sessionID, _ := req.Context().Value(runtimeSessionContextKey{}).(string); sessionID != "" {
		req.Header.Set("X-Multica-Preview-Session", sessionID)
	}
}
func runtimeHTTPError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	var result struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &result) == nil && result.Error != "" {
		return fmt.Errorf("Device Runtime returned %d: %s", resp.StatusCode, result.Error)
	}
	return fmt.Errorf("Device Runtime returned %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

func printDeviceSyncResult(cmd *cobra.Command, result deviceSyncResult) error {
	output, _ := cmd.Flags().GetString("output")
	if output == "json" {
		return cli.PrintJSON(os.Stdout, result)
	}
	printPreviewSessions([]map[string]any{result.PreviewSession})
	return nil
}
