package selfexec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRemovedExecutableAfterReplacement(t *testing.T) {
	directory := t.TempDir()
	stable := filepath.Join(directory, "multica")
	removed := filepath.Join(directory, "multica.next")
	if err := os.WriteFile(stable, []byte("replacement"), 0755); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveWith(func() (string, error) { return removed, nil }, []string{stable})
	if err != nil || resolved != stable {
		t.Fatalf("resolve replaced executable = %q, %v", resolved, err)
	}
	if _, err := resolveWith(func() (string, error) { return removed, nil }, nil); err == nil {
		t.Fatal("missing OS path and argv must fail before launching a helper")
	}
}

func TestResolvePrefersExistingOSExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveWith(func() (string, error) { return executable, nil }, []string{"missing"})
	if err != nil || resolved != executable {
		t.Fatalf("resolve existing executable = %q, %v", resolved, err)
	}
}
