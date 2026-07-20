package deviceruntime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAPKArtifact(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(apk, []byte("apk"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateAPKArtifact(apk); err != nil {
		t.Fatalf("validateAPKArtifact: %v", err)
	}
	if err := validateAPKArtifact("relative.apk"); err == nil {
		t.Fatal("expected relative artifact to fail")
	}
	notAPK := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(notAPK, []byte("zip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateAPKArtifact(notAPK); err == nil {
		t.Fatal("expected non-APK artifact to fail")
	}
}

func TestValidateWebPreviewURL(t *testing.T) {
	for _, value := range []string{"http://10.0.2.2:13000/login", "https://preview.example.test/task"} {
		got, err := validateWebPreviewURL(value)
		if err != nil || got != value {
			t.Fatalf("validateWebPreviewURL(%q) = %q, %v", value, got, err)
		}
	}
	for _, value := range []string{"file:///tmp/app", "https://user:pass@example.test", "not-a-url"} {
		if _, err := validateWebPreviewURL(value); err == nil {
			t.Fatalf("expected %q to fail", value)
		}
	}
}
