package main

import (
	"strings"
	"testing"
)

func TestTaskBySourcePathUsesEvidenceQuery(t *testing.T) {
	cmd := taskBySourceListCmd
	cmd.Flags().Set("agent", "agent id")
	cmd.Flags().Set("kind", "creative_variant")
	cmd.Flags().Set("ref", "019ec09d-6222-722b-bdfa-427b105d80be")
	path, err := taskBySourcePath(cmd)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/api/agents/agent%20id/tasks/by-source", "trigger_evidence_kind=creative_variant", "trigger_evidence_ref_id=019ec09d-6222-722b-bdfa-427b105d80be"} {
		if !strings.Contains(path, want) {
			t.Fatalf("path %q missing %q", path, want)
		}
	}
}

func TestTaskFanoutHelpRequiresTypedContext(t *testing.T) {
	if !strings.Contains(taskFanoutCmd.Long, "context with a non-empty type") {
		t.Fatal("fanout CLI help must document the required context.type discriminator")
	}
}
