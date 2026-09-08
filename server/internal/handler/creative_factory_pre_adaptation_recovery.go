package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func (h *Handler) recoverCreativeFactoryPreAdaptations(ctx context.Context, workspaceID pgtype.UUID) {
	rows, err := h.DB.Query(ctx, `
SELECT task.id
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id AND agent.workspace_id = $1
JOIN creative_source_analysis analysis ON analysis.id = task.trigger_evidence_ref_id
WHERE task.status = 'completed'
  AND task.trigger_evidence_kind = $2
  AND task.context->>'workflow' = 'creative_pre_adaptation'
  AND analysis.workspace_id = $1
  AND analysis.result->'adaptation'->>'status' = 'unavailable'
  AND analysis.result->'adaptation'->>'error_code' = 'manual_confirmation_required'
  AND COALESCE(analysis.result->'adaptation'->>'automatic_repair_attempted', 'false') <> 'true'
  AND task.completed_at > now() - interval '24 hours'
ORDER BY task.completed_at DESC NULLS LAST, task.created_at DESC
LIMIT 10
`, workspaceID, creativePreAdaptationEvidenceKind)
	if err != nil {
		slog.Warn("scan creative pre-adaptation recovery failed", "workspace_id", uuidToString(workspaceID), "error", err)
		return
	}
	defer rows.Close()

	var taskIDs []pgtype.UUID
	for rows.Next() {
		var taskID pgtype.UUID
		if err := rows.Scan(&taskID); err != nil {
			slog.Warn("read creative pre-adaptation recovery failed", "workspace_id", uuidToString(workspaceID), "error", err)
			continue
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("iterate creative pre-adaptation recovery failed", "workspace_id", uuidToString(workspaceID), "error", err)
		return
	}
	rows.Close()
	for _, taskID := range taskIDs {
		task, err := h.Queries.GetAgentTask(ctx, taskID)
		if err != nil {
			slog.Warn("load creative pre-adaptation recovery failed", "task_id", uuidToString(taskID), "error", err)
			continue
		}
		var taskContext creativePreAdaptationTaskContext
		if err := json.Unmarshal(task.Context, &taskContext); err != nil || taskContext.Workflow != "creative_pre_adaptation" {
			continue
		}
		analysisID, err := parseUUIDString(strings.TrimSpace(taskContext.SourceAnalysisID))
		if err != nil {
			continue
		}
		repaired, err := h.repairCreativePreAdaptationAutomatically(ctx, workspaceID, analysisID, taskContext)
		if err != nil {
			slog.Warn("automatic repair of creative pre-adaptation failed", "task_id", uuidToString(taskID), "error", err)
			if markErr := h.markCreativePreAdaptationAutomaticRepairAttempted(ctx, workspaceID, analysisID); markErr != nil {
				slog.Warn("mark creative pre-adaptation repair attempted failed", "task_id", uuidToString(taskID), "error", markErr)
			}
			continue
		}
		if repaired {
			continue
		}
		if markErr := h.markCreativePreAdaptationAutomaticRepairAttempted(ctx, workspaceID, analysisID); markErr != nil {
			slog.Warn("mark creative pre-adaptation repair attempted failed", "task_id", uuidToString(taskID), "error", markErr)
		}
	}
}
