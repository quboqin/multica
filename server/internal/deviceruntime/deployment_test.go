package deviceruntime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestArtifactSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(path, []byte("build-one"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := artifactSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 {
		t.Fatalf("hash length = %d", len(first))
	}
	if err := os.WriteFile(path, []byte("build-two"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := artifactSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("artifact hash did not change")
	}
}

func TestDeploymentStorePublishesProgressAndResult(t *testing.T) {
	store := newDeploymentStore()
	started, err := store.launch("preview-1", "USB-123", "abc", func(_ context.Context, update func(string), _ string) (bool, error) {
		update("installing")
		update("starting")
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot, ok := store.snapshot(started.ID)
		if !ok {
			t.Fatal("deployment disappeared")
		}
		if snapshot.Status == deploymentStatusRunning {
			if snapshot.Phase != "ready" || !snapshot.InstallSkipped {
				t.Fatalf("unexpected snapshot: %#v", snapshot)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("deployment did not finish")
}

func TestDeploymentStoreCancelsActiveJob(t *testing.T) {
	store := newDeploymentStore()
	started, err := store.launch("preview-1", "USB-123", "abc", func(ctx context.Context, _ func(string), _ string) (bool, error) {
		<-ctx.Done()
		return false, errors.New("adb process exited")
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.cancel(started.ID); !ok {
		t.Fatal("cancel did not find job")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot, _ := store.snapshot(started.ID)
		if snapshot.Status == deploymentStatusCanceled {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("deployment did not cancel")
}

func TestDeploymentStoreRejectsDifferentActiveBuild(t *testing.T) {
	store := newDeploymentStore()
	started, err := store.launch("preview-1", "USB-123", "abc", func(ctx context.Context, _ func(string), _ string) (bool, error) {
		<-ctx.Done()
		return false, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.cancel(started.ID)
	if _, err := store.launch("preview-1", "USB-123", "def", func(context.Context, func(string), string) (bool, error) {
		return false, nil
	}); err == nil {
		t.Fatal("different artifact joined an active deployment")
	}
	if _, ok := store.snapshotFor(started.ID, "preview-2"); ok {
		t.Fatal("another preview session read the deployment")
	}
	if _, ok := store.cancelFor(started.ID, "preview-2"); ok {
		t.Fatal("another preview session canceled the deployment")
	}
	if _, ok := store.snapshotFor(started.ID, "preview-1"); !ok {
		t.Fatal("owning preview session could not read deployment")
	}
}

func TestRuntimeAccessRequiresBrowserToken(t *testing.T) {
	server := Server{AccessToken: "host-secret", access: newRuntimeAccessStore()}
	var sessionID string
	handler := server.protectAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionID = runtimeSessionID(r)
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodPost, "/api/input", nil)
	request.Header.Set("Origin", "https://multica.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("browser without token = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/input", nil)
	request.Header.Set(runtimeAccessHeader, server.access.mint("preview-1"))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("console token = %d", response.Code)
	}
	if sessionID != "preview-1" {
		t.Fatalf("console token session = %q", sessionID)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/input", nil)
	request.Header.Set("Authorization", "Bearer host-secret")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("host token = %d", response.Code)
	}
}

func TestRuntimeLeaseRejectsConcurrentSession(t *testing.T) {
	leases := newRuntimeLeaseStore()
	if _, err := leases.acquire("preview-1", "USB-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := leases.acquire("preview-2", "USB-123"); err == nil {
		t.Fatal("second session acquired an active device lease")
	}
	if !leases.validate("preview-1", "USB-123") {
		t.Fatal("owning session could not renew its lease")
	}
	if leases.validate("preview-2", "USB-123") {
		t.Fatal("non-owning session validated another session's lease")
	}
}

func TestRuntimeLeaseRestoresDeviceAfterFailedSwitch(t *testing.T) {
	leases := newRuntimeLeaseStore()
	if _, err := leases.acquire("preview-1", "USB-123"); err != nil {
		t.Fatal(err)
	}
	previous, err := leases.acquire("preview-1", "emulator-5554")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "USB-123" {
		t.Fatalf("previous device = %q", previous)
	}
	leases.restore("preview-1", previous, "emulator-5554")
	if !leases.validate("preview-1", "USB-123") {
		t.Fatal("previous device lease was not restored")
	}
	if leases.validate("preview-1", "emulator-5554") {
		t.Fatal("failed target device lease remained active")
	}
}

func TestParseBatteryHealth(t *testing.T) {
	device := parseBatteryHealth(Device{Serial: "USB-123", State: "device"}, `
AC powered: false
USB powered: false
level: 12
temperature: 463
`)
	if device.BatteryLevel == nil || *device.BatteryLevel != 12 {
		t.Fatalf("battery = %#v", device.BatteryLevel)
	}
	if device.TemperatureC == nil || *device.TemperatureC != 46.3 {
		t.Fatalf("temperature = %#v", device.TemperatureC)
	}
	if device.Health != "warning" || device.Reason != "Device temperature is high" {
		t.Fatalf("health = %q, reason = %q", device.Health, device.Reason)
	}
}

func TestConsoleWebRTCCleanupUsesAuthorizedKeepaliveFetch(t *testing.T) {
	if strings.Contains(consoleWebRTCEnhancements, "sendBeacon") {
		t.Fatal("page cleanup bypasses the authenticated fetch wrapper")
	}
	if !strings.Contains(consoleWebRTCEnhancements, "pagehide") ||
		!strings.Contains(consoleWebRTCEnhancements, "void closeWebRTCSession()") ||
		!strings.Contains(consoleWebRTCEnhancements, "keepalive: true") {
		t.Fatal("page cleanup does not close the WebRTC session with keepalive fetch")
	}
}
