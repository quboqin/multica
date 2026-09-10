package primecompose

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed image_prime_compose.py
var composerScript []byte

var engineSHA256 = fmt.Sprintf("%x", sha256.Sum256(composerScript))

// Run executes the versioned Prime composition engine bundled with Multica.
func Run(ctx context.Context, manifestPath string, stdout, stderr io.Writer) error {
	manifestPath = strings.TrimSpace(manifestPath)
	if manifestPath == "" {
		return fmt.Errorf("--manifest is required")
	}
	if _, err := os.Stat(manifestPath); err != nil {
		return fmt.Errorf("read Prime compose manifest: %w", err)
	}
	python, err := pythonExecutable()
	if err != nil {
		return err
	}
	temporaryDirectory, err := os.MkdirTemp("", "multica-prime-compose-")
	if err != nil {
		return fmt.Errorf("create Prime composer directory: %w", err)
	}
	defer os.RemoveAll(temporaryDirectory)
	scriptPath := filepath.Join(temporaryDirectory, "image_prime_compose.py")
	if err := os.WriteFile(scriptPath, composerScript, 0o600); err != nil {
		return fmt.Errorf("write bundled Prime composer: %w", err)
	}
	command := exec.CommandContext(ctx, python, scriptPath, "--manifest", manifestPath, "--engine-sha256", engineSHA256)
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run Prime composer engine: %w", err)
	}
	return nil
}

func pythonExecutable() (string, error) {
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python.exe", "python3.exe", "python3", "python"}
	}
	for _, candidate := range candidates {
		if executable, err := exec.LookPath(candidate); err == nil {
			return executable, nil
		}
	}
	return "", fmt.Errorf("Prime composer requires Python with Pillow, OpenCV, and NumPy")
}
