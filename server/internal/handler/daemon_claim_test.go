package handler

import "testing"

func TestPrepareCreativeDomainTaskClaimUsesDirectEnvelope(t *testing.T) {
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
	}

	prepareCreativeDomainTaskClaim(resp)

	if resp.IssueID != "" || resp.PriorSessionID != "" || resp.PriorWorkDir != "" {
		t.Fatalf("creative claim retained root envelope: %#v", resp)
	}
	if resp.TriggerCommentID != nil || resp.TriggerSummary != nil || resp.TriggerThreadID != "" || resp.TriggerCommentContent != "" || resp.NewCommentCount != 0 {
		t.Fatalf("creative claim retained issue-trigger context: %#v", resp)
	}
	if resp.WorkspaceID != "workspace-1" {
		t.Fatalf("workspace changed: %q", resp.WorkspaceID)
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
