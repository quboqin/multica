package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRecoverPendingCreativeQCFinalizationsQueuesReplacementAttempt(t *testing.T) {
	if testHandler == nil || testPool == nil || testHandler.TaskService == nil {
		t.Skip("creative QC recovery test requires the handler task service")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "recover stranded QC finalization")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]string{
			"leader_agent_id": fixture.LeaderAgentID, "producer_agent_id": fixture.ProducerAgentID, "reviewer_agent_id": fixture.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, trigger_evidence_kind, input_snapshot, created_by)
VALUES ($1, $2, 'running', 'manual', $3::jsonb, $4)
RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_qc_recovery_queued'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb)
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status, candidate_state, primary_size)
VALUES ($1, 'C01', 1, 'action_required', 'selected', '1080x1080')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "stranded-qc-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	context := creativeQCTaskContextForTest(t, orderID, variantID, "visual")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id, completed_at)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'failed', $2::jsonb, 'creative_order_variant_qc', $3, now())
`, fixture.ReviewerAgentID, context, variantID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings)
VALUES ($1, 'visual', 1, 1, 'failed', '{"blocking_failures":[{"code":"actual_prime_obstruction"}]}'::jsonb)
`, variantID); err != nil {
		t.Fatal(err)
	}

	processed, err := testHandler.RecoverPendingCreativeQCFinalizations(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("recovered pending QC finalizations = %d, want 1", processed)
	}
	var attempt int
	var recoveryKind string
	if err := testPool.QueryRow(t.Context(), `
SELECT COALESCE(NULLIF(context->>'qc_attempt', '')::int, 0), COALESCE(context->>'qc_recovery_kind', '')
FROM agent_task_queue
WHERE context->>'creative_order_id' = $1
  AND context->>'variant_id' = $2
  AND context->>'workflow' = 'creative_qc_visual'
ORDER BY created_at DESC, id DESC
LIMIT 1
`, orderID, variantID).Scan(&attempt, &recoveryKind); err != nil {
		t.Fatal(err)
	}
	if attempt != 2 || recoveryKind != creativeQCFinalizationRecoveryKind {
		t.Fatalf("recovery task attempt/kind = %d/%q, want 2/%q", attempt, recoveryKind, creativeQCFinalizationRecoveryKind)
	}
}

func TestRecoverPendingCreativeQCFinalizationsQueuesMissingReportAttempt(t *testing.T) {
	if testHandler == nil || testPool == nil || testHandler.TaskService == nil {
		t.Skip("creative QC recovery test requires the handler task service")
	}
	issueID, candidateID := createCreativeFeedbackCandidate(t, "recover QC without report")
	fixture := createCreativeOrderSquadFixture(t, "", "", true)
	inputSnapshot, err := json.Marshal(map[string]any{
		"squad_snapshot": map[string]string{
			"leader_agent_id": fixture.LeaderAgentID, "producer_agent_id": fixture.ProducerAgentID, "reviewer_agent_id": fixture.ReviewerAgentID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var orderID, itemID, variantID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, trigger_evidence_kind, input_snapshot, created_by)
VALUES ($1, $2, 'running', 'manual', $3::jsonb, $4)
RETURNING id::text
`, testWorkspaceID, issueID, inputSnapshot, testUserID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM activity_log WHERE issue_id = $1 AND action = 'creative_qc_recovery_queued'`, issueID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, orderID)
	})
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, copy_snapshot)
VALUES ($1, $2, '{}'::jsonb)
RETURNING id::text
`, orderID, candidateID).Scan(&itemID); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, revision, status, candidate_state, primary_size)
VALUES ($1, 'C01', 1, 'running', 'selected', '1080x1080')
RETURNING id::text
`, itemID).Scan(&variantID); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		attachmentID := createCreativeOrderAssetAttachment(t, "missing-qc-report-prime-"+strings.ReplaceAll(size, "x", "-")+".png")
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_order_asset (variant_id, size_key, revision, stage, attachment_id, status)
VALUES ($1, $2, 1, 'primed', $3, 'completed')
`, variantID, size, attachmentID); err != nil {
			t.Fatal(err)
		}
	}
	context := creativeQCTaskContextForTest(t, orderID, variantID, "visual")
	if _, err := testPool.Exec(t.Context(), `
INSERT INTO agent_task_queue (agent_id, runtime_id, status, context, trigger_evidence_kind, trigger_evidence_ref_id, completed_at)
VALUES ($1, (SELECT runtime_id FROM agent WHERE id = $1), 'completed', $2::jsonb, 'creative_order_variant_qc', $3, now())
`, fixture.ReviewerAgentID, context, variantID); err != nil {
		t.Fatal(err)
	}

	processed, err := testHandler.RecoverPendingCreativeQCFinalizations(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if processed != 1 {
		t.Fatalf("recovered missing QC report = %d, want 1", processed)
	}
	var attempt int
	var recoveryKind string
	if err := testPool.QueryRow(t.Context(), `
SELECT COALESCE(NULLIF(context->>'qc_attempt', '')::int, 0), COALESCE(context->>'qc_recovery_kind', '')
FROM agent_task_queue
WHERE context->>'creative_order_id' = $1
  AND context->>'variant_id' = $2
  AND context->>'workflow' = 'creative_qc_visual'
ORDER BY created_at DESC, id DESC
LIMIT 1
`, orderID, variantID).Scan(&attempt, &recoveryKind); err != nil {
		t.Fatal(err)
	}
	if attempt != 2 || recoveryKind != creativeQCMissingReportRecoveryKind {
		t.Fatalf("missing-report recovery task attempt/kind = %d/%q, want 2/%q", attempt, recoveryKind, creativeQCMissingReportRecoveryKind)
	}
}
