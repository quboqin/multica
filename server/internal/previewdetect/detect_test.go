package previewdetect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDevelopmentContextWithMultipleRepositories(t *testing.T) {
	root := t.TempDir()
	h5 := filepath.Join(root, "customer-h5")
	android := filepath.Join(root, "android-shell")
	backend := filepath.Join(root, "api")

	mustWrite(t, filepath.Join(h5, ".git", "config"), "")
	mustWrite(t, filepath.Join(h5, "package.json"), `{
		"scripts":{"dev":"nuxt dev","build":"nuxt build"},
		"dependencies":{"nuxt":"3.0.0","vue":"3.0.0"}
	}`)
	mustWrite(t, filepath.Join(h5, "pnpm-lock.yaml"), "lockfileVersion: 9")

	mustWrite(t, filepath.Join(android, ".git"), "gitdir: somewhere")
	mustWrite(t, filepath.Join(android, "gradlew"), "")
	mustWrite(t, filepath.Join(android, "build.gradle.kts"), `plugins { alias(libs.plugins.android.application) apply false }`)
	mustWrite(t, filepath.Join(android, "app", "build.gradle.kts"), `plugins { alias(libs.plugins.android.application) }`)
	mustWrite(t, filepath.Join(android, "app", "src", "main", "AndroidManifest.xml"), `<manifest />`)

	mustWrite(t, filepath.Join(backend, ".git", "config"), "")
	mustWrite(t, filepath.Join(backend, "go.mod"), "module example.com/api")

	report, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(report.Repositories) != 3 {
		t.Fatalf("repositories = %d, want 3: %#v", len(report.Repositories), report.Repositories)
	}

	byRoot := make(map[string]Repository)
	for _, repo := range report.Repositories {
		byRoot[repo.Root] = repo
	}
	if got := byRoot["customer-h5"].Targets; len(got) != 1 || got[0].Platform != "web" || got[0].Framework != "nuxt" || got[0].DevCommand != "pnpm run dev" {
		t.Errorf("H5 targets = %#v", got)
	}
	if got := byRoot["android-shell"].Targets; len(got) != 1 || got[0].Platform != "android" {
		t.Errorf("Android targets = %#v", got)
	}
	if got := byRoot["api"]; !got.BackendOnly || len(got.Targets) != 0 {
		t.Errorf("backend repository = %#v", got)
	}
}

func TestDetectMonorepoReturnsEveryPreviewTarget(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".git"), "gitdir: elsewhere")
	mustWrite(t, filepath.Join(root, "apps", "web", "package.json"), `{
		"scripts":{"dev":"vite","build":"vite build"},
		"dependencies":{"vite":"1","react":"1","react-dom":"1"}
	}`)
	mustWrite(t, filepath.Join(root, "apps", "desktop", "package.json"), `{
		"scripts":{"dev":"electron ."},"devDependencies":{"electron":"1"}
	}`)
	mustWrite(t, filepath.Join(root, "ios", "Customer.xcodeproj", "project.pbxproj"), "")

	report, err := Detect(root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if len(report.Repositories) != 1 {
		t.Fatalf("repositories = %d, want 1", len(report.Repositories))
	}
	platforms := map[string]bool{}
	for _, target := range report.Repositories[0].Targets {
		platforms[target.Platform] = true
	}
	for _, platform := range []string{"web", "desktop", "ios"} {
		if !platforms[platform] {
			t.Errorf("missing %s target: %#v", platform, report.Repositories[0].Targets)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
