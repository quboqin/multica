package previewdetect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const ReportVersion = 1

var skippedDirectories = map[string]struct{}{
	".agent_context": {}, ".agents": {}, ".claude": {}, ".cursor": {},
	".git": {}, ".github": {}, ".kimi": {}, ".multica": {}, ".next": {},
	".nuxt": {}, ".opencode": {}, ".output": {}, ".pi": {},
	".turbo": {}, ".gradle": {}, ".idea": {}, ".vscode": {}, "build": {},
	"coverage": {}, "dist": {}, "node_modules": {}, "Pods": {}, "target": {},
}

// Report describes repositories and runnable UI targets discovered from a
// task's actual working directory. It is intentionally independent of the
// workspace/project repository registry.
type Report struct {
	Version      int          `json:"version"`
	GeneratedAt  string       `json:"generated_at"`
	Root         string       `json:"root"`
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	Root           string   `json:"root"`
	RemoteURL      string   `json:"remote_url,omitempty"`
	Targets        []Target `json:"targets"`
	BackendOnly    bool     `json:"backend_only"`
	BackendSignals []string `json:"backend_signals,omitempty"`
}

type Target struct {
	ID            string   `json:"id"`
	Platform      string   `json:"platform"`
	Path          string   `json:"path"`
	Framework     string   `json:"framework,omitempty"`
	Confidence    string   `json:"confidence"`
	Evidence      []string `json:"evidence"`
	DevCommand    string   `json:"dev_command,omitempty"`
	BuildCommand  string   `json:"build_command,omitempty"`
	ArtifactHints []string `json:"artifact_hints,omitempty"`
}

type packageManifest struct {
	Scripts      map[string]string `json:"scripts"`
	Dependencies map[string]string `json:"dependencies"`
	DevDeps      map[string]string `json:"devDependencies"`
}

// Detect scans root for Git repositories, then identifies every runnable UI
// target in each repository. When root itself is a Git repository it is the
// sole repository boundary; otherwise immediate/nested checkout roots are
// discovered up to a small bounded depth.
func Detect(root string) (Report, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, fmt.Errorf("resolve scan root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Report{}, fmt.Errorf("stat scan root: %w", err)
	}
	if !info.IsDir() {
		return Report{}, errors.New("preview detection root must be a directory")
	}

	repoRoots, err := findRepositoryRoots(absRoot)
	if err != nil {
		return Report{}, err
	}
	if len(repoRoots) == 0 {
		repoRoots = []string{absRoot}
	}

	report := Report{
		Version:      ReportVersion,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		Root:         ".",
		Repositories: make([]Repository, 0, len(repoRoots)),
	}
	for _, repoRoot := range repoRoots {
		repo, err := inspectRepository(absRoot, repoRoot)
		if err != nil {
			return Report{}, err
		}
		report.Repositories = append(report.Repositories, repo)
	}
	sort.Slice(report.Repositories, func(i, j int) bool {
		return report.Repositories[i].Root < report.Repositories[j].Root
	})
	return report, nil
}

func findRepositoryRoots(root string) ([]string, error) {
	if isGitRoot(root) {
		return []string{root}, nil
	}
	var roots []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		if path != root {
			if _, skip := skippedDirectories[entry.Name()]; skip {
				return filepath.SkipDir
			}
			depth := relativeDepth(root, path)
			if depth > 3 {
				return filepath.SkipDir
			}
		}
		if path != root && isGitRoot(path) {
			roots = append(roots, path)
			return filepath.SkipDir
		}
		return nil
	})
	return roots, err
}

func inspectRepository(scanRoot, repoRoot string) (Repository, error) {
	repo := Repository{
		Root:      relativePath(scanRoot, repoRoot),
		RemoteURL: gitRemoteURL(repoRoot),
		Targets:   []Target{},
	}
	targets := make(map[string]Target)
	backendSignals := make(map[string]struct{})

	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path != repoRoot && entry.IsDir() {
			if _, skip := skippedDirectories[entry.Name()]; skip {
				return filepath.SkipDir
			}
			if relativeDepth(repoRoot, path) > 6 {
				return filepath.SkipDir
			}
		}

		name := entry.Name()
		switch {
		case !entry.IsDir() && name == "package.json":
			detectPackageTarget(repoRoot, path, targets, backendSignals)
		case !entry.IsDir() && (name == "build.gradle" || name == "build.gradle.kts"):
			detectAndroidTarget(repoRoot, path, targets)
		case entry.IsDir() && (strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace")):
			detectIOSTarget(repoRoot, filepath.Dir(path), name, targets)
			return filepath.SkipDir
		case !entry.IsDir() && name == "go.mod":
			backendSignals[relativePath(repoRoot, path)] = struct{}{}
		case !entry.IsDir() && (name == "pom.xml" || name == "Cargo.toml"):
			backendSignals[relativePath(repoRoot, path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return Repository{}, fmt.Errorf("scan repository %s: %w", repoRoot, err)
	}

	for _, target := range targets {
		repo.Targets = append(repo.Targets, target)
	}
	sort.Slice(repo.Targets, func(i, j int) bool {
		if repo.Targets[i].Path == repo.Targets[j].Path {
			return repo.Targets[i].Platform < repo.Targets[j].Platform
		}
		return repo.Targets[i].Path < repo.Targets[j].Path
	})
	for signal := range backendSignals {
		repo.BackendSignals = append(repo.BackendSignals, signal)
	}
	sort.Strings(repo.BackendSignals)
	repo.BackendOnly = len(repo.Targets) == 0 && len(repo.BackendSignals) > 0
	return repo, nil
}

func detectPackageTarget(repoRoot, manifestPath string, targets map[string]Target, backendSignals map[string]struct{}) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil || len(raw) > 2<<20 {
		return
	}
	var manifest packageManifest
	if json.Unmarshal(raw, &manifest) != nil {
		return
	}
	deps := make(map[string]string, len(manifest.Dependencies)+len(manifest.DevDeps))
	for name, version := range manifest.Dependencies {
		deps[name] = version
	}
	for name, version := range manifest.DevDeps {
		deps[name] = version
	}
	projectDir := filepath.Dir(manifestPath)
	relManifest := relativePath(repoRoot, manifestPath)
	relDir := relativePath(repoRoot, projectDir)

	if _, ok := deps["electron"]; ok {
		addTarget(targets, Target{
			Platform: "desktop", Path: relDir, Framework: "electron", Confidence: "high",
			Evidence:     []string{relManifest + ":electron"},
			DevCommand:   packageScriptCommand(projectDir, "dev", manifest.Scripts),
			BuildCommand: packageScriptCommand(projectDir, "build", manifest.Scripts),
		})
		return
	}
	if _, err := os.Stat(filepath.Join(projectDir, "src-tauri", "tauri.conf.json")); err == nil {
		addTarget(targets, Target{
			Platform: "desktop", Path: relDir, Framework: "tauri", Confidence: "high",
			Evidence:     []string{relativePath(repoRoot, filepath.Join(projectDir, "src-tauri", "tauri.conf.json"))},
			DevCommand:   packageScriptCommand(projectDir, "tauri", manifest.Scripts),
			BuildCommand: packageScriptCommand(projectDir, "build", manifest.Scripts),
		})
		return
	}

	framework, evidence := webFramework(deps, projectDir, repoRoot, relManifest)
	if framework != "" {
		devScript := firstScript(manifest.Scripts, "dev", "serve", "start")
		addTarget(targets, Target{
			Platform: "web", Path: relDir, Framework: framework, Confidence: "high",
			Evidence:     evidence,
			DevCommand:   packageScriptCommand(projectDir, devScript, manifest.Scripts),
			BuildCommand: packageScriptCommand(projectDir, firstScript(manifest.Scripts, "build", "generate"), manifest.Scripts),
		})
		return
	}

	if _, hasStart := manifest.Scripts["start"]; hasStart {
		backendSignals[relManifest+":node-service"] = struct{}{}
	}
}

func webFramework(deps map[string]string, projectDir, repoRoot, manifest string) (string, []string) {
	frameworks := []struct {
		name string
		deps []string
	}{
		{name: "nuxt", deps: []string{"nuxt"}},
		{name: "next", deps: []string{"next"}},
		{name: "sveltekit", deps: []string{"@sveltejs/kit"}},
		{name: "angular", deps: []string{"@angular/core"}},
		{name: "vite-vue", deps: []string{"vite", "vue"}},
		{name: "vite-react", deps: []string{"vite", "react"}},
		{name: "vite-svelte", deps: []string{"vite", "svelte"}},
		{name: "react", deps: []string{"react", "react-dom"}},
		{name: "vue", deps: []string{"vue"}},
	}
	for _, candidate := range frameworks {
		matched := true
		for _, dep := range candidate.deps {
			if _, ok := deps[dep]; !ok {
				matched = false
				break
			}
		}
		if matched {
			evidence := []string{manifest + ":" + strings.Join(candidate.deps, "+")}
			return candidate.name, evidence
		}
	}
	if _, err := os.Stat(filepath.Join(projectDir, "index.html")); err == nil {
		return "static-web", []string{relativePath(repoRoot, filepath.Join(projectDir, "index.html"))}
	}
	return "", nil
}

func detectAndroidTarget(repoRoot, gradlePath string, targets map[string]Target) {
	raw, err := os.ReadFile(gradlePath)
	if err != nil || len(raw) > 2<<20 {
		return
	}
	content := string(raw)
	if !hasAndroidApplicationPlugin(content) {
		return
	}
	moduleDir := filepath.Dir(gradlePath)
	evidence := []string{relativePath(repoRoot, gradlePath) + ":com.android.application"}
	manifestPath := filepath.Join(moduleDir, "src", "main", "AndroidManifest.xml")
	if _, err := os.Stat(manifestPath); err == nil {
		evidence = append(evidence, relativePath(repoRoot, manifestPath))
	}
	addTarget(targets, Target{
		Platform: "android", Path: relativePath(repoRoot, moduleDir), Framework: "android-gradle", Confidence: "high",
		Evidence: evidence, BuildCommand: gradleCommand(repoRoot, "assembleDebug"),
		ArtifactHints: []string{"**/build/outputs/apk/**/*.apk"},
	})
}

func hasAndroidApplicationPlugin(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "apply false") {
			continue
		}
		if strings.Contains(line, "com.android.application") || strings.Contains(line, "libs.plugins.android.application") {
			return true
		}
	}
	return false
}

func detectIOSTarget(repoRoot, projectDir, projectName string, targets map[string]Target) {
	addTarget(targets, Target{
		Platform: "ios", Path: relativePath(repoRoot, projectDir), Framework: "xcode", Confidence: "medium",
		Evidence:      []string{relativePath(repoRoot, filepath.Join(projectDir, projectName))},
		ArtifactHints: []string{"DerivedData/**/Build/Products/**/*.app"},
	})
}

func addTarget(targets map[string]Target, target Target) {
	target.Path = filepath.ToSlash(target.Path)
	if target.Path == "" {
		target.Path = "."
	}
	target.ID = target.Platform + ":" + target.Path
	key := target.ID
	if existing, ok := targets[key]; ok {
		existing.Evidence = appendUnique(existing.Evidence, target.Evidence...)
		targets[key] = existing
		return
	}
	target.Evidence = appendUnique(nil, target.Evidence...)
	targets[key] = target
}

func packageScriptCommand(projectDir, script string, scripts map[string]string) string {
	if script == "" {
		return ""
	}
	if _, ok := scripts[script]; !ok {
		return ""
	}
	manager := "npm"
	if fileExists(filepath.Join(projectDir, "pnpm-lock.yaml")) {
		manager = "pnpm"
	} else if fileExists(filepath.Join(projectDir, "yarn.lock")) {
		manager = "yarn"
	} else if fileExists(filepath.Join(projectDir, "bun.lockb")) || fileExists(filepath.Join(projectDir, "bun.lock")) {
		manager = "bun"
	}
	if manager == "yarn" || manager == "bun" {
		return manager + " " + script
	}
	return manager + " run " + script
}

func gradleCommand(repoRoot, task string) string {
	if fileExists(filepath.Join(repoRoot, "gradlew.bat")) {
		return ".\\gradlew.bat " + task
	}
	if fileExists(filepath.Join(repoRoot, "gradlew")) {
		return "./gradlew " + task
	}
	return "gradle " + task
}

func firstScript(scripts map[string]string, names ...string) string {
	for _, name := range names {
		if _, ok := scripts[name]; ok {
			return name
		}
	}
	return ""
}

func WriteReport(root string, report Report) (string, error) {
	dir := filepath.Join(root, ".multica", "preview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "targets.json")
	temp, err := os.CreateTemp(dir, "targets-*.json")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tempPath, path); err != nil {
		// Windows cannot atomically replace an existing destination.
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return "", err
		}
		if err := os.Rename(tempPath, path); err != nil {
			return "", err
		}
	}
	return path, nil
}

func isGitRoot(path string) bool {
	_, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil
}

func gitRemoteURL(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "config", "--get", "remote.origin.url")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func relativeDepth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(filepath.Clean(rel), string(filepath.Separator)))
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "" {
		return "."
	}
	return filepath.ToSlash(rel)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func appendUnique(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	for _, value := range existing {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		existing = append(existing, value)
	}
	return existing
}
