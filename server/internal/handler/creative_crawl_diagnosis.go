package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const creativeCrawlDiagnosisEvidenceKind = "creative_crawl_diagnosis"

type creativeCrawlDiagnosis struct {
	Diagnosis struct {
		RequiresAgent bool `json:"requires_agent"`
	} `json:"diagnosis"`
}

func (h *Handler) enqueueCreativeCrawlDiagnosis(
	ctx context.Context,
	workspaceID, requestingUserID pgtype.UUID,
	runID, connectorID string,
	diagnostics json.RawMessage,
) string {
	var parsed creativeCrawlDiagnosis
	if json.Unmarshal(diagnostics, &parsed) != nil || !parsed.Diagnosis.RequiresAgent {
		return ""
	}
	if h.TaskService == nil {
		h.setCreativeCrawlDiagnosisTaskState(ctx, workspaceID, runID, "unavailable", "")
		return ""
	}
	agent, err := h.resolveCreativeCrawlDiagnosisAgent(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		h.setCreativeCrawlDiagnosisTaskState(ctx, workspaceID, runID, "unconfigured", "")
		return ""
	}
	if err != nil {
		h.setCreativeCrawlDiagnosisTaskState(ctx, workspaceID, runID, "unavailable", "")
		return ""
	}
	runUUID := parseUUID(strings.TrimSpace(runID))
	contextPayload, err := json.Marshal(map[string]any{
		"type":          "creative_crawl_diagnosis",
		"workflow":      "creative_crawl_diagnosis",
		"crawl_run_id":  strings.TrimSpace(runID),
		"connector_id":  strings.TrimSpace(connectorID),
		"diagnostics":   json.RawMessage(diagnostics),
	})
	if err != nil {
		return ""
	}
	tasks, err := h.TaskService.EnqueueDirectTaskFanout(ctx, service.DirectTaskFanout{
		Agent:                agent,
		RequestingUserID:     requestingUserID,
		Attribution:          attribution.DirectHumanRun(requestingUserID, attribution.EvidenceKind(creativeCrawlDiagnosisEvidenceKind), runUUID),
		TriggerEvidenceKind:  creativeCrawlDiagnosisEvidenceKind,
		TriggerEvidenceRefID: runUUID,
		Items: []service.DirectTaskFanoutItem{{
			ItemKey: "diagnose",
			Context: contextPayload,
		}},
	})
	if err != nil || len(tasks) != 1 {
		h.setCreativeCrawlDiagnosisTaskState(ctx, workspaceID, runID, "enqueue_failed", "")
		return ""
	}
	taskID := uuidToString(tasks[0].ID)
	h.setCreativeCrawlDiagnosisTaskState(ctx, workspaceID, runID, "queued", taskID)
	return taskID
}

func (h *Handler) resolveCreativeCrawlDiagnosisAgent(ctx context.Context, workspaceID pgtype.UUID) (db.Agent, error) {
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
  AND capability.config->>'capability' = 'crawl_diagnosis'
ORDER BY agent.created_at, agent.id
LIMIT 1
`, workspaceID).Scan(&agentID)
	if err != nil {
		return db.Agent{}, err
	}
	return h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
}

func (h *Handler) setCreativeCrawlDiagnosisTaskState(ctx context.Context, workspaceID pgtype.UUID, runID, state, taskID string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	_, _ = h.DB.Exec(ctx, `
UPDATE creative_material_crawl_run
SET diagnostics = diagnostics
  || jsonb_build_object(
    'agent_diagnosis_state', $3,
    'agent_diagnosis_task_id', $4
  )
WHERE id = $1::uuid AND workspace_id = $2
`, runID, workspaceID, strings.TrimSpace(state), strings.TrimSpace(taskID))
}
