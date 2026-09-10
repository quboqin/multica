package execenv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	preparationHelperTestMode  = "execenv-preparation-helper"
	preparationHelperBlockMode = "execenv-preparation-helper-block"
)

func preparationHelperTestCommand(mode string) []string {
	return []string{
		os.Args[0],
		"-test.run=^TestPreparationHelperProcess$",
		"--",
		mode,
	}
}

// TestPreparationHelperProcess is the child entry point used by isolation
// tests. It exercises the same stdin/stdout protocol as the real multica
// helper without requiring a built CLI binary.
func TestPreparationHelperProcess(t *testing.T) {
	if len(os.Args) == 0 {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case preparationHelperBlockMode:
		time.Sleep(time.Hour)
	case preparationHelperTestMode:
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		if err := RunPreparationHelper(os.Stdin, os.Stdout, logger); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}
}

func TestPreparationHelperRoundTripsReuse(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := PrepareParams{
		WorkspacesRoot: t.TempDir(),
		WorkspaceID:    "ws-helper-reuse",
		TaskID:         "99999999-8888-7777-6666-555555555555",
		Provider:       "claude",
		Task:           TaskContextForEnv{IssueID: "issue-helper-reuse"},
	}
	env, err := PrepareIsolated(ctx, preparationHelperTestCommand(preparationHelperTestMode), params, logger)
	if err != nil {
		t.Fatalf("PrepareIsolated: %v", err)
	}
	defer env.Cleanup(true)

	reused, err := ReuseIsolated(ctx, preparationHelperTestCommand(preparationHelperTestMode), ReuseParams{
		WorkspacesRoot: params.WorkspacesRoot,
		WorkspaceID:    params.WorkspaceID,
		TaskID:         params.TaskID,
		WorkDir:        env.WorkDir,
		Provider:       params.Provider,
		Task:           TaskContextForEnv{IssueID: "issue-helper-reuse", NewCommentCount: 1},
	}, logger)
	if err != nil {
		t.Fatalf("ReuseIsolated: %v", err)
	}
	if reused == nil || reused.RootDir != env.RootDir || reused.WorkDir != env.WorkDir {
		t.Fatalf("reused environment = %#v, want root %q workdir %q", reused, env.RootDir, env.WorkDir)
	}
}

func TestPreparationHelperRoundTripsProjectResources(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	params := PrepareParams{
		WorkspacesRoot: t.TempDir(),
		WorkspaceID:    "ws-helper-project-resource",
		TaskID:         "88888888-7777-6666-5555-444444444444",
		Provider:       "claude",
		Task: TaskContextForEnv{
			IssueID:   "issue-helper-project-resource",
			ProjectID: "project-helper-project-resource",
			ProjectResources: []ProjectResourceForEnv{
				{
					ID:           "resource-helper-project-resource",
					ResourceType: "github_repo",
					ResourceRef:  json.RawMessage(`{"url":"https://github.com/multica-ai/multica"}`),
					Label:        "Multica",
				},
			},
		},
	}

	env, err := PrepareIsolated(ctx, preparationHelperTestCommand(preparationHelperTestMode), params, logger)
	if err != nil {
		t.Fatalf("PrepareIsolated: %v", err)
	}
	defer env.Cleanup(true)

	data, err := os.ReadFile(filepath.Join(env.WorkDir, ".multica", "project", "resources.json"))
	if err != nil {
		t.Fatalf("read project resources: %v", err)
	}
	var got projectResourceFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode project resources: %v", err)
	}
	if len(got.Resources) != 1 || got.Resources[0].ID != "resource-helper-project-resource" {
		t.Fatalf("project resources = %#v, want one preserved resource", got.Resources)
	}
}

func TestPreparationRequestPreservesOpenclawGatewayForHelper(t *testing.T) {
	want := OpenclawGatewayPin{Host: "gw.internal", Port: 18789, Token: "real-secret", TLS: true}
	request := preparationRequest{
		Action:  preparationActionPrepare,
		Prepare: &PrepareParams{OpenclawGateway: want},
	}

	redacted, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal redacted request: %v", err)
	}
	if bytes.Contains(redacted, []byte(want.Token)) {
		t.Fatalf("ordinary JSON leaked gateway token: %s", redacted)
	}

	payload, err := marshalPreparationRequest(request)
	if err != nil {
		t.Fatalf("marshal preparation request: %v", err)
	}
	got, err := decodePreparationRequest(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode preparation request: %v", err)
	}
	if got.Prepare == nil || got.Prepare.OpenclawGateway != want {
		t.Fatalf("gateway pin did not survive helper protocol: %#v", got.Prepare)
	}
}

func TestPrepareIsolatedCancelsBlockedHelper(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := PrepareIsolated(ctx, preparationHelperTestCommand(preparationHelperBlockMode), PrepareParams{}, slog.Default())
	if err == nil {
		t.Fatal("PrepareIsolated error = nil, want context cancellation")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("PrepareIsolated error = %v, want deadline exceeded", err)
	}
}
