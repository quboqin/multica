package deviceruntime

import (
	"testing"
	"time"
)

func TestValidateWebViewAction(t *testing.T) {
	valid := webViewActionRequest{
		Serial:   "emulator-5554",
		Action:   "fill",
		Selector: webViewSelector{Role: "textbox", Name: "OTP"},
		Value:    "123456",
	}
	if err := validateWebViewAction(valid); err != nil {
		t.Fatalf("validateWebViewAction: %v", err)
	}
	invalid := valid
	invalid.Action = "script"
	if err := validateWebViewAction(invalid); err == nil {
		t.Fatal("expected unsupported action to fail")
	}
	invalid = valid
	invalid.Selector = webViewSelector{}
	if err := validateWebViewAction(invalid); err == nil {
		t.Fatal("expected empty selector to fail")
	}
}

func TestSelectWebViewTarget(t *testing.T) {
	targets := []webViewTarget{
		{Type: "worker", URL: "http://example.test/worker", WebSocketDebuggerURL: "ws://worker"},
		{Type: "page", URL: "http://10.0.2.2:13000/login", WebSocketDebuggerURL: "ws://page"},
	}
	target, ok := selectWebViewTarget(targets)
	if !ok || target.URL != targets[1].URL {
		t.Fatalf("target = %+v, %v", target, ok)
	}
}

func TestWebViewSessionStoreReusesDeviceSession(t *testing.T) {
	store := newWebViewSessionStore()
	first := store.session("emulator-5554")
	second := store.session("emulator-5554")
	other := store.session("USB-123")
	if first != second {
		t.Fatal("same device did not reuse its WebView session")
	}
	if first == other {
		t.Fatal("different devices shared a WebView session")
	}
}

func TestWebViewSessionStoreReleasesIdleSession(t *testing.T) {
	store := newWebViewSessionStoreWithIdleGrace(10 * time.Millisecond)
	first := store.session("emulator-5554")
	first.mu.Lock()
	first.armIdleClose(store, "emulator-5554")
	first.mu.Unlock()

	time.Sleep(50 * time.Millisecond)
	second := store.session("emulator-5554")
	if first != second {
		t.Fatal("idle WebView session record should be reusable")
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	if first.idleTimer != nil {
		t.Fatal("idle WebView close timer did not fire")
	}
}
