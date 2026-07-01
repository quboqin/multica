package main

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
)

func TestValidateKpiStatusCLI(t *testing.T) {
	for _, s := range validKpiStatuses {
		if err := validateKpiStatusCLI(s); err != nil {
			t.Errorf("status %q should be valid, got: %v", s, err)
		}
	}
	err := validateKpiStatusCLI("green")
	if err == nil {
		t.Fatal("status \"green\" should be rejected")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("error should list valid statuses, got: %v", err)
	}
}

func newKpiTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "kpi"}
	cmd.Flags().String("link-type", "", "")
	cmd.Flags().String("link-id", "", "")
	cmd.Flags().String("plan", "", "")
	cmd.Flags().String("project", "", "")
	cmd.Flags().String("issue", "", "")
	return cmd
}

func TestApplyKpiLinkFlagsRejectsMixedModes(t *testing.T) {
	cmd := newKpiTestCmd()
	_ = cmd.Flags().Set("plan", "abc")
	_ = cmd.Flags().Set("link-type", "project")

	err := applyKpiLinkFlags(context.Background(), &cli.APIClient{}, cmd, map[string]any{}, nil)
	if err == nil {
		t.Fatal("expected mixed link mode error")
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestResolveKpiLinkTargetCanonicalUUIDs(t *testing.T) {
	client := &cli.APIClient{}
	ctx := context.Background()

	got, err := resolveKpiLinkTarget(ctx, client, "milestone", "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("resolve plan uuid: %v", err)
	}
	if got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("got %q", got)
	}

	got, err = resolveKpiLinkTarget(ctx, client, "project", "22222222-2222-2222-2222-222222222222")
	if err != nil {
		t.Fatalf("resolve project uuid: %v", err)
	}
	if got != "22222222-2222-2222-2222-222222222222" {
		t.Fatalf("got %q", got)
	}
}

func TestApplyKpiLinkFlagsUsesExistingLinkTypeForLinkID(t *testing.T) {
	cmd := newKpiTestCmd()
	_ = cmd.Flags().Set("link-id", "33333333-3333-3333-3333-333333333333")

	body := map[string]any{}
	existing := map[string]any{"link_type": "project"}
	err := applyKpiLinkFlags(context.Background(), &cli.APIClient{}, cmd, body, existing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if body["link_id"] != "33333333-3333-3333-3333-333333333333" {
		t.Fatalf("got %v", body["link_id"])
	}
}
