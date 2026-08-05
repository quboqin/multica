package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	manualReferenceAnalysisEvidenceKind = "creative_crawl_run_analysis"
	manualReferenceAnalysisSource       = "manual_library_import"
)

type creativeMaterialLibraryImportResponse struct {
	ID       string                                 `json:"id"`
	Analysis creativeMaterialImportAnalysisResponse `json:"analysis"`
}

type creativeMaterialImportAnalysisResponse struct {
	Action          string `json:"action"`
	Status          string `json:"status"`
	Warning         string `json:"warning"`
	CrawlRunID      string `json:"crawl_run_id"`
	AnalysisAgentID string `json:"analysis_agent_id"`
	TaskID          string `json:"task_id"`
}

type manualReferenceAnalysisEvidence struct {
	RunID           pgtype.UUID
	AnalysisAgentID pgtype.UUID
	Status          string
	Error           string
}

func (h *Handler) enqueueManualReferenceAnalysis(
	ctx context.Context,
	workspaceID, userID, candidateID pgtype.UUID,
	connectorID string,
) creativeMaterialImportAnalysisResponse {
	result := creativeMaterialImportAnalysisResponse{Action: "enqueue_failed", Status: "failed"}
	resolvedAgent, resolveErr := h.resolveReferenceAnalysisAgent(ctx, workspaceID, pgtype.UUID{})
	if resolveErr != nil && !errors.Is(resolveErr, pgx.ErrNoRows) {
		result.Warning = "The material was saved, but the reference analysis agent could not be resolved."
		return result
	}
	evidence, err := h.ensureManualReferenceAnalysisEvidence(ctx, workspaceID, userID, candidateID, connectorID, resolvedAgent.ID)
	if err != nil {
		result.Warning = "The material was saved, but its reference analysis evidence could not be prepared."
		return result
	}
	result.CrawlRunID = uuidToString(evidence.RunID)
	result.AnalysisAgentID = uuidToString(evidence.AnalysisAgentID)
	if evidence.Status == "completed" {
		result.Action = "already_completed"
		result.Status = "completed"
		return result
	}

	existingTaskID, existingTaskStatus, existingAgentID, err := h.latestManualReferenceAnalysisTask(ctx, evidence.RunID, candidateID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		result.Warning = "The material was saved, but the existing reference analysis task could not be checked."
		return result
	}
	if err == nil && isActiveDirectTaskStatus(existingTaskStatus) {
		result.Action = "already_queued"
		result.Status = creativeAnalysisStatusFromTask(existingTaskStatus)
		result.TaskID = uuidToString(existingTaskID)
		result.AnalysisAgentID = uuidToString(existingAgentID)
		return result
	}

	agent, err := h.resolveReferenceAnalysisAgent(ctx, workspaceID, evidence.AnalysisAgentID)
	if errors.Is(err, pgx.ErrNoRows) {
		warning := "No enabled agent with reference_analysis capability is configured."
		h.markManualReferenceAnalysisFailed(ctx, workspaceID, evidence.RunID, candidateID, warning)
		result.Action = "unavailable"
		result.Warning = warning
		return result
	}
	if err != nil {
		result.Warning = "The material was saved, but the reference analysis agent could not be loaded."
		return result
	}
	result.AnalysisAgentID = uuidToString(agent.ID)
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_crawl_run
SET params = jsonb_set(params, '{analysis_agent_id}', to_jsonb($3::text), true)
WHERE id = $1 AND workspace_id = $2
`, evidence.RunID, workspaceID, result.AnalysisAgentID); err != nil {
		result.Warning = "The material was saved, but the reference analysis assignment could not be recorded."
		return result
	}
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_crawl_run_candidate
SET analysis_status = 'pending', analysis_error = '', analyzed_at = NULL, updated_at = now()
WHERE run_id = $1 AND candidate_id = $2 AND workspace_id = $3
`, evidence.RunID, candidateID, workspaceID); err != nil {
		result.Warning = "The material was saved, but the reference analysis state could not be reset."
		return result
	}

	itemKey := uuidToString(candidateID) + ":v1"
	taskContext, _ := json.Marshal(map[string]any{
		"type":             "creative_domain_task",
		"workflow":         "creative_reference_analysis",
		"crawl_run_id":     result.CrawlRunID,
		"candidate_id":     uuidToString(candidateID),
		"analysis_version": 1,
	})
	if h.TaskService == nil {
		warning := "Reference analysis task service is unavailable."
		h.markManualReferenceAnalysisFailed(ctx, workspaceID, evidence.RunID, candidateID, warning)
		result.Warning = warning
		return result
	}
	tasks, err := h.TaskService.EnqueueDirectTaskFanout(ctx, service.DirectTaskFanout{
		Agent:                agent,
		RequestingUserID:     userID,
		Attribution:          attribution.DirectHumanRun(userID, attribution.EvidenceKind(manualReferenceAnalysisEvidenceKind), evidence.RunID),
		TriggerEvidenceKind:  manualReferenceAnalysisEvidenceKind,
		TriggerEvidenceRefID: evidence.RunID,
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: itemKey,
			Context: taskContext,
		}},
	})
	if err != nil || len(tasks) != 1 {
		warning := "Reference analysis could not be queued."
		if err != nil {
			warning = fmt.Sprintf("Reference analysis could not be queued: %s", err)
		}
		h.markManualReferenceAnalysisFailed(ctx, workspaceID, evidence.RunID, candidateID, warning)
		result.Warning = warning
		return result
	}
	result.Action = "queued"
	result.Status = creativeAnalysisStatusFromTask(tasks[0].Status)
	result.TaskID = uuidToString(tasks[0].ID)
	return result
}

func (h *Handler) ensureManualReferenceAnalysisEvidence(
	ctx context.Context,
	workspaceID, userID, candidateID pgtype.UUID,
	connectorID string,
	resolvedAgentID pgtype.UUID,
) (manualReferenceAnalysisEvidence, error) {
	if h.TxStarter == nil {
		return manualReferenceAnalysisEvidence{}, errors.New("transaction service is unavailable")
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return manualReferenceAnalysisEvidence{}, err
	}
	defer tx.Rollback(ctx)
	lockKey := uuidToString(workspaceID) + ":manual-reference-analysis:" + uuidToString(candidateID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return manualReferenceAnalysisEvidence{}, err
	}

	var evidence manualReferenceAnalysisEvidence
	err = tx.QueryRow(ctx, `
SELECT cr.id, NULLIF(cr.params->>'analysis_agent_id', '')::uuid,
       rc.analysis_status, rc.analysis_error
FROM creative_material_crawl_run_candidate rc
JOIN creative_material_crawl_run cr ON cr.id = rc.run_id AND cr.workspace_id = rc.workspace_id
WHERE rc.workspace_id = $1 AND rc.candidate_id = $2
  AND cr.params->>'source' = $3
ORDER BY cr.created_at DESC
LIMIT 1
`, workspaceID, candidateID, manualReferenceAnalysisSource).Scan(
		&evidence.RunID,
		&evidence.AnalysisAgentID,
		&evidence.Status,
		&evidence.Error,
	)
	if err == nil {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return manualReferenceAnalysisEvidence{}, commitErr
		}
		return evidence, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return manualReferenceAnalysisEvidence{}, err
	}

	params, _ := json.Marshal(map[string]any{
		"source":            manualReferenceAnalysisSource,
		"analysis_agent_id": uuidToString(resolvedAgentID),
	})
	status := "pending"
	analysisError := ""
	if !resolvedAgentID.Valid {
		status = "failed"
		analysisError = "No enabled agent with reference_analysis capability is configured."
	}
	err = tx.QueryRow(ctx, `
INSERT INTO creative_material_crawl_run (
  workspace_id, connector_id, query_summary, params, status,
  imported_count, existing_count, total_count, started_at, finished_at,
  created_by_type, created_by_id
) VALUES ($1, $2, 'manual material import', $3::jsonb, 'completed',
          1, 0, 1, now(), now(), 'member', $4)
RETURNING id
`, workspaceID, strings.TrimSpace(connectorID), params, userID).Scan(&evidence.RunID)
	if err != nil {
		return manualReferenceAnalysisEvidence{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_material_crawl_run_candidate (
  run_id, candidate_id, workspace_id, is_new_in_run, analysis_status, analysis_error,
  analyzed_at
) VALUES ($1, $2, $3, true, $4, $5,
          CASE WHEN $4 = 'failed' THEN now() ELSE NULL END)
`, evidence.RunID, candidateID, workspaceID, status, analysisError); err != nil {
		return manualReferenceAnalysisEvidence{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return manualReferenceAnalysisEvidence{}, err
	}
	evidence.AnalysisAgentID = resolvedAgentID
	evidence.Status = status
	evidence.Error = analysisError
	return evidence, nil
}

func (h *Handler) resolveReferenceAnalysisAgent(ctx context.Context, workspaceID, preferredAgentID pgtype.UUID) (db.Agent, error) {
	var agentID pgtype.UUID
	err := h.DB.QueryRow(ctx, `
SELECT agent.id
FROM agent
JOIN agent_skill binding ON binding.agent_id = agent.id AND binding.enabled
JOIN skill capability ON capability.id = binding.skill_id
  AND capability.workspace_id = agent.workspace_id
WHERE agent.workspace_id = $1
  AND agent.archived_at IS NULL
  AND capability.config->>'kind' = 'creative_role'
  AND capability.config->>'capability' = 'reference_analysis'
ORDER BY CASE WHEN agent.id = $2::uuid THEN 0 ELSE 1 END, agent.created_at, agent.id
LIMIT 1
`, workspaceID, preferredAgentID).Scan(&agentID)
	if err != nil {
		return db.Agent{}, err
	}
	return h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
}

func (h *Handler) latestManualReferenceAnalysisTask(ctx context.Context, runID, candidateID pgtype.UUID) (pgtype.UUID, string, pgtype.UUID, error) {
	var taskID, agentID pgtype.UUID
	var status string
	err := h.DB.QueryRow(ctx, `
SELECT id, status, agent_id
FROM agent_task_queue
WHERE trigger_evidence_kind = $1
  AND trigger_evidence_ref_id = $2
  AND context->>'candidate_id' = $3
ORDER BY CASE WHEN status IN ('queued', 'dispatched', 'running', 'waiting_local_directory') THEN 0 ELSE 1 END,
         created_at DESC
LIMIT 1
`, manualReferenceAnalysisEvidenceKind, runID, uuidToString(candidateID)).Scan(&taskID, &status, &agentID)
	return taskID, status, agentID, err
}

func (h *Handler) markManualReferenceAnalysisFailed(ctx context.Context, workspaceID, runID, candidateID pgtype.UUID, message string) {
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_material_crawl_run_candidate
SET analysis_status = 'failed', analysis_error = $4, analyzed_at = now(), updated_at = now()
WHERE run_id = $1 AND candidate_id = $2 AND workspace_id = $3
`, runID, candidateID, workspaceID, strings.TrimSpace(message))
}

func isActiveDirectTaskStatus(status string) bool {
	return status == "queued" || status == "dispatched" || status == "running" || status == "waiting_local_directory"
}

func creativeAnalysisStatusFromTask(status string) string {
	switch status {
	case "running":
		return "running"
	case "completed":
		return "completed"
	case "failed", "cancelled":
		return "failed"
	default:
		return "pending"
	}
}
