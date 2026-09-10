package handler

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// RecoverCreativeReferenceAnalyses repairs imports interrupted before dispatch.
// Execution failures and cancelled tasks require the existing explicit retry action.
func (h *Handler) RecoverCreativeReferenceAnalyses(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := h.DB.Query(ctx, `
SELECT DISTINCT ON (candidate.id) candidate.workspace_id, candidate.id,
       membership.user_id, candidate.connector_id
FROM creative_material_crawl_run_candidate rc
JOIN creative_material_candidate candidate ON candidate.id = rc.candidate_id
JOIN creative_material_crawl_run run ON run.id = rc.run_id
LEFT JOIN autopilot_run autopilot ON autopilot.id = run.autopilot_run_id
LEFT JOIN agent_task_queue collector ON collector.id = autopilot.task_id
JOIN member membership ON membership.workspace_id = candidate.workspace_id
  AND membership.user_id = CASE WHEN run.created_by_type = 'member' THEN run.created_by_id
    ELSE COALESCE(collector.originator_user_id, collector.requesting_user_id) END
LEFT JOIN LATERAL (
  SELECT status, error_code, analysis_version FROM creative_source_analysis
  WHERE candidate_id = candidate.id ORDER BY analysis_version DESC LIMIT 1
) analysis ON true
WHERE rc.is_new_in_run AND candidate.asset_type = 'image'
  AND run.query_summary = 'material_search' AND run.status IN ('completed', 'partial')
  AND run.created_at > now() - interval '7 days'
  AND rc.updated_at < now() - interval '1 minute'
  AND (analysis.status IS NULL OR analysis.status = 'pending'
       OR analysis.error_code = 'REFERENCE_ANALYSIS_QUEUE_FAILED')
  AND NOT EXISTS (
    SELECT 1 FROM agent_task_queue task
    WHERE task.trigger_evidence_kind = 'creative_crawl_run_analysis'
      AND task.context->>'candidate_id' = candidate.id::text
      AND (analysis.analysis_version IS NULL
           OR task.context->>'analysis_version' = analysis.analysis_version::text)
  )
ORDER BY candidate.id, run.created_at DESC LIMIT $1
`, limit)
	if err != nil {
		return 0, fmt.Errorf("list undispatched reference analyses: %w", err)
	}
	type pending struct {
		workspaceID, candidateID, userID pgtype.UUID
		connectorID                      string
	}
	var candidates []pending
	for rows.Next() {
		var item pending
		if err := rows.Scan(&item.workspaceID, &item.candidateID, &item.userID, &item.connectorID); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range candidates {
		result := h.enqueueManualReferenceAnalysis(ctx, item.workspaceID, item.userID, item.candidateID, item.connectorID, false)
		if result.Action == "queued" {
			count++
		}
	}
	return count, nil
}
