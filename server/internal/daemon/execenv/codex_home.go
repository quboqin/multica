package execenv

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Directories to symlink from the shared ~/.codex/ into the per-task CODEX_HOME.
// The shared directory is created if it doesn't exist, ensuring Codex session
// logs are always written to the global home where users can find them.
var codexSymlinkedDirs = []string{
	"sessions",
}

// Files to symlink from the shared ~/.codex/ into the per-task CODEX_HOME.
// Symlinks share state (e.g. auth tokens) so changes propagate automatically.
var codexSymlinkedFiles = []string{
	"auth.json",
}

// Files to copy from the shared ~/.codex/ into the per-task CODEX_HOME.
// Copies are isolated — changes don't affect the shared home.
var codexCopiedFiles = []string{
	"config.json",
	"config.toml",
	"instructions.md",
}

const CodexStateWarmCacheDirName = ".codex-state-cache"

var codexStateWarmCacheLocks sync.Map

const codexStateWarmCacheMarkerFile = ".multica_codex_state_cache"

// CodexHomeOptions carries optional inputs for prepareCodexHomeWithOpts that
// affect the generated per-task config.toml.
type CodexHomeOptions struct {
	// CodexVersion is the detected Codex CLI version (e.g. "0.121.0"). Empty
	// means unknown; on macOS, unknown is treated as "probably broken" so the
	// daemon falls back to danger-full-access for network access. See
	// codex_sandbox.go for details.
	CodexVersion string
	// StateWarmCacheDir, when set, points at a workspace-scoped cache of
	// Codex state SQLite files used to avoid per-task migration cold starts.
	StateWarmCacheDir string
	// WritableRoots are extra paths the workspace-write sandbox may modify.
	WritableRoots []string
	// GOOS overrides the target platform when deciding the sandbox policy.
	// Empty means use runtime.GOOS. Primarily exists so tests can exercise
	// both macOS and Linux paths deterministically.
	GOOS string
}

// prepareCodexHome is a thin wrapper around prepareCodexHomeWithOpts kept for
// tests that don't care about platform-aware sandbox configuration. It
// assumes a Linux-like environment where workspace-write + network_access
// works correctly.
func prepareCodexHome(codexHome string, logger *slog.Logger) error {
	return prepareCodexHomeWithOpts(codexHome, CodexHomeOptions{GOOS: "linux"}, logger)
}

// prepareCodexHomeWithOpts creates a per-task CODEX_HOME directory and seeds
// it with config from the shared ~/.codex/ home. Auth is symlinked (shared),
// config files are copied (isolated). The per-task config.toml gets a
// daemon-managed sandbox block picked by codexSandboxPolicyFor.
func prepareCodexHomeWithOpts(codexHome string, opts CodexHomeOptions, logger *slog.Logger) error {
	sharedHome := resolveSharedCodexHome()

	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		return fmt.Errorf("create codex-home dir: %w", err)
	}

	// Symlink shared directories (sessions) so logs stay in the global home.
	for _, name := range codexSymlinkedDirs {
		src := filepath.Join(sharedHome, name)
		dst := filepath.Join(codexHome, name)
		if err := ensureDirSymlink(src, dst); err != nil {
			logger.Warn("execenv: codex-home dir symlink failed", "dir", name, "error", err)
		}
	}

	// Symlink shared files (auth).
	for _, name := range codexSymlinkedFiles {
		src := filepath.Join(sharedHome, name)
		dst := filepath.Join(codexHome, name)
		if err := ensureSymlink(src, dst); err != nil {
			logger.Warn("execenv: codex-home symlink failed", "file", name, "error", err)
		}
	}

	// Surface the resulting auth.json state (file kind only, never contents)
	// so operators diagnosing token-refresh failures can tell whether the
	// per-task home is tracking the shared ~/.codex/auth.json or has drifted
	// into a stale local copy.
	logCodexAuthState(filepath.Join(codexHome, "auth.json"), logger)

	// Sync config files from the shared source (isolated per task).
	for _, name := range codexCopiedFiles {
		src := filepath.Join(sharedHome, name)
		dst := filepath.Join(codexHome, name)
		if err := syncCopiedFile(src, dst); err != nil {
			logger.Warn("execenv: codex-home sync failed", "file", name, "error", err)
		}
	}

	if opts.StateWarmCacheDir != "" {
		if err := seedCodexStateWarmCache(codexHome, opts.StateWarmCacheDir, logger); err != nil {
			logger.Warn("execenv: codex-home state warm cache seed failed", "error", err)
		}
	}

	// Drop `[[skills.config]]` entries inherited from the user's
	// ~/.codex/config.toml. Codex Desktop writes plugin-backed skills with a
	// `name` and no `path`, which the CLI's stricter TOML parser rejects with
	// `missing field path` and bails out of `thread/start`. Multica writes the
	// agent's active skills directly to `codex-home/skills/`, so the
	// user-level registry is redundant here. See codex_skill_strip.go.
	if err := sanitizeCopiedCodexConfig(filepath.Join(codexHome, "config.toml")); err != nil {
		logger.Warn("execenv: codex-home sanitize config failed", "error", err)
	}

	if err := exposeSharedCodexPluginCache(codexHome, sharedHome); err != nil {
		logger.Warn("execenv: codex-home plugin cache exposure failed", "error", err)
	}

	// Write a daemon-managed sandbox block into config.toml. On macOS we may
	// need to fall back to danger-full-access because of openai/codex#10390;
	// see codex_sandbox.go for the full rationale.
	policy := codexSandboxPolicyFor(opts.GOOS, opts.CodexVersion)
	policy.WritableRoots = opts.WritableRoots
	if err := ensureCodexSandboxConfig(filepath.Join(codexHome, "config.toml"), policy, opts.CodexVersion, logger); err != nil {
		logger.Warn("execenv: codex-home ensure sandbox config failed", "error", err)
	}

	// Disable Codex native multi-agent inside daemon-managed task sessions
	// so the parent thread's `turn/completed` is not interpreted as task
	// completion while spawned subagents are still running. See
	// codex_multi_agent.go for the full rationale and escape hatch.
	if err := ensureCodexMultiAgentConfig(filepath.Join(codexHome, "config.toml"), logger); err != nil {
		logger.Warn("execenv: codex-home ensure multi-agent config failed", "error", err)
	}

	// Disable Codex native auto-memory inside daemon-managed task sessions
	// so cross-task and cross-workspace context leaks (multica#3130) cannot
	// happen via `codex-home/memories/` or `~/.codex/memories/`. See
	// codex_memory.go for the full rationale and escape hatch.
	if err := ensureCodexMemoryConfig(filepath.Join(codexHome, "config.toml"), logger); err != nil {
		logger.Warn("execenv: codex-home ensure memory config failed", "error", err)
	}

	return nil
}

func CodexStateWarmCacheDir(workspacesRoot, workspaceID string) string {
	if workspacesRoot == "" || workspaceID == "" {
		return ""
	}
	return filepath.Join(workspacesRoot, workspaceID, CodexStateWarmCacheDirName)
}

func seedCodexStateWarmCache(codexHome, cacheDir string, logger *slog.Logger) error {
	return withCodexStateWarmCacheLock(cacheDir, func() error {
		return seedCodexStateWarmCacheLocked(codexHome, cacheDir, logger)
	})
}

func seedCodexStateWarmCacheLocked(codexHome, cacheDir string, logger *slog.Logger) error {
	if _, err := os.Stat(filepath.Join(cacheDir, codexStateWarmCacheMarkerFile)); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat cache marker: %w", err)
	}
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read cache dir: %w", err)
	}
	copied := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isCodexStateFile(name) {
			continue
		}
		src := filepath.Join(cacheDir, name)
		dst := filepath.Join(codexHome, name)
		if _, err := os.Lstat(dst); err == nil {
			continue
		}
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy cached %s: %w", name, err)
		}
		copied++
	}
	if copied > 0 && logger != nil {
		logger.Info("execenv: codex-home seeded state warm cache", "files", copied, "cache_dir", cacheDir)
	}
	return nil
}

func RefreshCodexStateWarmCache(codexHome, cacheDir string, logger *slog.Logger) error {
	if cacheDir == "" {
		return nil
	}
	return withCodexStateWarmCacheLock(cacheDir, func() error {
		return refreshCodexStateWarmCacheLocked(codexHome, cacheDir, logger)
	})
}

func refreshCodexStateWarmCacheLocked(codexHome, cacheDir string, logger *slog.Logger) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, codexStateWarmCacheMarkerFile), []byte("managed by multica\n"), 0o644); err != nil {
		return fmt.Errorf("write cache marker: %w", err)
	}
	entries, err := os.ReadDir(codexHome)
	if err != nil {
		return fmt.Errorf("read codex home: %w", err)
	}
	copied := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isCodexStateFile(name) {
			continue
		}
		src := filepath.Join(codexHome, name)
		dst := filepath.Join(cacheDir, name)
		if err := replaceFileAtomically(src, dst); err != nil {
			return fmt.Errorf("refresh cached %s: %w", name, err)
		}
		copied++
	}
	if copied > 0 && logger != nil {
		logger.Info("execenv: codex state warm cache refreshed", "files", copied, "cache_dir", cacheDir)
	}
	return nil
}

func withCodexStateWarmCacheLock(cacheDir string, fn func() error) error {
	if cacheDir == "" {
		return fn()
	}
	value, _ := codexStateWarmCacheLocks.LoadOrStore(filepath.Clean(cacheDir), &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

func isCodexStateFile(name string) bool {
	if !strings.HasPrefix(name, "state_") {
		return false
	}
	return strings.HasSuffix(name, ".sqlite") ||
		strings.HasSuffix(name, ".sqlite-wal") ||
		strings.HasSuffix(name, ".sqlite-shm")
}

// resolveSharedCodexHome returns the path to the user's shared Codex home.
// Checks $CODEX_HOME first, falls back to ~/.codex.
func resolveSharedCodexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		abs, err := filepath.Abs(v)
		if err == nil {
			return abs
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), ".codex") // last resort fallback
	}
	return filepath.Join(home, ".codex")
}

func exposeSharedCodexPluginCache(codexHome, sharedHome string) error {
	src := filepath.Join(sharedHome, "plugins", "cache")
	dst := filepath.Join(codexHome, "plugins", "cache")
	if err := os.MkdirAll(src, 0o755); err != nil {
		return fmt.Errorf("create shared plugin cache dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create codex plugin dir: %w", err)
	}

	if fi, err := os.Lstat(dst); err == nil {
		isLink := fi.Mode()&os.ModeSymlink != 0
		if isLink {
			if target, readlinkErr := os.Readlink(dst); readlinkErr == nil && target == src {
				return nil
			}
			if err := os.Remove(dst); err != nil {
				return fmt.Errorf("remove stale plugin cache link: %w", err)
			}
		} else {
			if err := os.RemoveAll(dst); err != nil {
				return fmt.Errorf("remove stale plugin cache path: %w", err)
			}
		}
	}

	if err := createDirLink(src, dst); err != nil {
		return fmt.Errorf("expose shared plugin cache: %w", err)
	}
	return nil
}

// ensureDirSymlink creates a symlink dst → src for a directory.
// Unlike ensureSymlink, it creates the source directory if it doesn't exist,
// so Codex can write to it immediately.
func ensureDirSymlink(src, dst string) error {
	if err := os.MkdirAll(src, 0o755); err != nil {
		return fmt.Errorf("create shared dir %s: %w", src, err)
	}

	// Check if dst already exists.
	if fi, err := os.Lstat(dst); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(dst)
			if err == nil && target == src {
				return nil // already correct
			}
			os.Remove(dst)
		} else {
			// Regular file/dir exists — don't overwrite.
			return nil
		}
	}

	return createDirLink(src, dst)
}

// ensureSymlink ensures dst tracks src. If src doesn't exist, it's a no-op.
// If dst is already a symlink pointing at src, it's a no-op. Otherwise — a
// wrong-target symlink, a broken symlink, or a regular file left over from a
// prior createFileLink copy fallback — dst is removed and recreated via
// createFileLink so the per-task home doesn't drift from the shared source.
//
// The "regular file" branch matters on Windows: when os.Symlink fails (no
// Developer Mode / not elevated), createFileLink falls back to copying the
// file. Without this re-creation step, a once-stale auth.json would never
// pick up token refreshes from the shared ~/.codex/auth.json, leaving Codex
// stuck on a revoked refresh token across env reuses (issue #2081).
func ensureSymlink(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return nil // source doesn't exist — skip
	}

	if fi, err := os.Lstat(dst); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(dst); err == nil && target == src {
				return nil // symlink already points to src
			}
		}
		// Wrong-target symlink, broken symlink, or stale regular file —
		// drop it so createFileLink can re-link/re-copy from the current src.
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove stale dst %s: %w", dst, err)
		}
	}

	return createFileLink(src, dst)
}

// logCodexAuthState records the kind of auth.json the per-task CODEX_HOME
// ended up with — symlink (with target), regular file (with size + mtime),
// or missing — so an operator chasing refresh_token_reused / token_expired
// reports can immediately tell whether the per-task home is tracking the
// shared ~/.codex/auth.json or has drifted into a stale local copy.
//
// Never logs the file contents.
func logCodexAuthState(authPath string, logger *slog.Logger) {
	fi, err := os.Lstat(authPath)
	if err != nil {
		logger.Info("execenv: codex auth.json absent", "path", authPath, "error", err)
		return
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(authPath)
		logger.Info("execenv: codex auth.json is symlink", "path", authPath, "target", target)
		return
	}
	logger.Info("execenv: codex auth.json is regular file",
		"path", authPath,
		"size", fi.Size(),
		"mtime", fi.ModTime().UTC(),
	)
}

// (The daemon used to write a minimal inline config here; the authoritative
// sandbox/network directives now live in a managed block rendered by
// codex_sandbox.go's ensureCodexSandboxConfig so they can be updated
// idempotently without touching user-managed keys.)

// syncCopiedFile mirrors a per-task dst onto the current state of the shared
// src so the per-task copy tracks the shared source across Reuse() runs:
//
//   - src present, dst absent:  copy src → dst
//   - src present, dst present: drop dst and re-copy src → dst (refresh)
//   - src absent,  dst present: drop dst (the shared source has been removed,
//     so the per-task stale copy must not linger)
//   - src absent,  dst absent:  no-op
//
// Regression for MUL-2646: the prior "don't overwrite" guard left per-task
// config.toml / config.json / instructions.md stuck on whatever snapshot they
// were seeded with at first Prepare. A user who edited ~/.codex/config.toml
// between runs — switching the active [model_providers.X] base_url, pointing
// env_key at a freshly rotated API key, or removing the file outright to
// drop a provider — kept hitting the stale per-task copy on session resume,
// with Codex calling the new URL using the old key (or replaying a provider
// the user had since deleted from the shared config).
//
// For config.toml the subsequent ensureCodex{Sandbox,MultiAgent,Memory}Config
// passes recreate the file from scratch when the shared source is gone, so
// the per-task home keeps the daemon-managed defaults but loses every
// user-managed [model_providers.X] / model_provider line that no longer
// exists in the shared config. For config.json / instructions.md there is
// no daemon-managed default, so they simply disappear in lockstep with the
// shared source.
func syncCopiedFile(src, dst string) error {
	_, srcErr := os.Stat(src)
	srcMissing := os.IsNotExist(srcErr)
	if srcErr != nil && !srcMissing {
		return fmt.Errorf("stat src %s: %w", src, srcErr)
	}

	if _, err := os.Lstat(dst); err == nil {
		if err := os.Remove(dst); err != nil {
			return fmt.Errorf("remove stale dst %s: %w", dst, err)
		}
	}

	if srcMissing {
		return nil
	}
	return copyFile(src, dst)
}

// copyFile copies src to dst unconditionally.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return nil
}

func replaceFileAtomically(src, dst string) error {
	if _, err := os.Stat(src); err != nil {
		if os.IsNotExist(err) {
			if removeErr := os.Remove(dst); removeErr != nil && !os.IsNotExist(removeErr) {
				return fmt.Errorf("remove stale dst %s: %w", dst, removeErr)
			}
			return nil
		}
		return fmt.Errorf("stat src %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create dst dir %s: %w", filepath.Dir(dst), err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", dst, err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.Remove(tmpPath)
		}
	}()

	in, err := os.Open(src)
	if err != nil {
		_ = tmp.Close()
		return fmt.Errorf("open %s: %w", src, err)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = in.Close()
		_ = tmp.Close()
		return fmt.Errorf("copy %s -> %s: %w", src, tmpPath, err)
	}
	if err := in.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("close %s: %w", src, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if removeErr := os.Remove(dst); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove old dst %s before replace: %w", dst, removeErr)
		}
		if retryErr := os.Rename(tmpPath, dst); retryErr != nil {
			return fmt.Errorf("replace %s: %w", dst, retryErr)
		}
	}
	cleanupTmp = false
	return nil
}
