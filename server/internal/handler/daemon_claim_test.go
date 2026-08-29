package handler

import (
	"strings"
	"testing"
)

func TestPrepareCreativeDomainTaskClaimKeepsLegacyPreparationEnvelope(t *testing.T) {
	t.Parallel()

	trigger := "comment-1"
	summary := "review the root issue"
	resp := &AgentTaskResponse{
		IssueID:               "root-issue",
		WorkspaceID:           "workspace-1",
		Context:               []byte(`{"type":"creative_domain_task","workflow":"creative_candidate_selection"}`),
		PriorSessionID:        "session-1",
		PriorWorkDir:          "/work/old",
		TriggerCommentID:      &trigger,
		TriggerThreadID:       "thread-1",
		TriggerCommentContent: "root issue says creative_order",
		TriggerSummary:        &summary,
		TriggerAuthorType:     "member",
		TriggerAuthorName:     "tester",
		NewCommentCount:       2,
		NewCommentsSince:      "2026-08-29T00:00:00Z",
		Agent: &TaskAgentData{
			Instructions: "base instructions",
		},
	}

	prepareCreativeDomainTaskClaim(resp)

	if resp.IssueID != "root-issue" || resp.PriorSessionID != "" || resp.PriorWorkDir != "" {
		t.Fatalf("creative claim did not retain only the legacy preparation envelope: %#v", resp)
	}
	if resp.TriggerCommentID != nil || resp.TriggerSummary != nil || resp.TriggerThreadID != "" || resp.TriggerCommentContent != "" || resp.NewCommentCount != 0 {
		t.Fatalf("creative claim retained issue-trigger context: %#v", resp)
	}
	if resp.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace changed: %q", resp.WorkspaceID)
	}
	if !strings.Contains(resp.Agent.Instructions, "workflow 为 `creative_candidate_selection`") ||
		!strings.Contains(resp.Agent.Instructions, "candidate-select") ||
		!strings.Contains(resp.Agent.Instructions, "根工单仅用于审计和启动兼容") {
		t.Fatalf("creative claim instructions did not pin task workflow: %q", resp.Agent.Instructions)
	}
}

func TestPrepareCreativeDomainTaskClaimRecoversNativeTaskAuditEnvelope(t *testing.T) {
	t.Parallel()

	resp := &AgentTaskResponse{
		Context: []byte(`{"type":"creative_domain_task","workflow":"creative_production","issue_id":"root-issue"}`),
		Agent:   &TaskAgentData{},
	}

	prepareCreativeDomainTaskClaim(resp)

	if resp.IssueID != "root-issue" {
		t.Fatalf("native creative task did not recover audit envelope: %#v", resp)
	}
}

func TestPrepareCreativeDomainTaskClaimPreservesOrdinaryIssueTask(t *testing.T) {
	t.Parallel()

	resp := &AgentTaskResponse{
		IssueID:        "root-issue",
		PriorSessionID: "session-1",
		Context:        []byte(`{"type":"issue_task"}`),
	}

	prepareCreativeDomainTaskClaim(resp)

	if resp.IssueID != "root-issue" || resp.PriorSessionID != "session-1" {
		t.Fatalf("ordinary issue task was converted to direct task: %#v", resp)
	}
}
