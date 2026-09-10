package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
)

// A diagnostic task is intentionally allowed more recovery room than an
// ordinary production task, but it remains bounded and fully auditable. The
// capability check below prevents this from becoming a general task-token
// bypass for human-only creative mutations.
const creativeDiagnosticRecoveryMaxAttempts = 12

type creativeDiagnosticRecoveryActor struct {
	AgentID pgtype.UUID
	TaskID  pgtype.UUID
}

// requireCreativeDiagnosticRecoveryAuthority preserves human access while
// allowing only an active diagnostician task with the crawl_diagnosis
// capability to invoke bounded creative recovery. Cloud credentials stay
// excluded just as they are by the normal human-actor guard.
func (h *Handler) requireCreativeDiagnosticRecoveryAuthority(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID) (creativeDiagnosticRecoveryActor, bool) {
	switch r.Header.Get("X-Actor-Source") {
	case "":
		return creativeDiagnosticRecoveryActor{}, true
	case "cloud_pat":
		writeError(w, http.StatusForbidden, "creative recovery requires a human or diagnostic task actor")
		return creativeDiagnosticRecoveryActor{}, false
	case "task_token":
		// Checked below.
	default:
		writeError(w, http.StatusForbidden, "creative recovery requires a human or diagnostic task actor")
		return creativeDiagnosticRecoveryActor{}, false
	}

	taskID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid diagnostic task")
		return creativeDiagnosticRecoveryActor{}, false
	}
	agentID, err := util.ParseUUID(r.Header.Get("X-Agent-ID"))
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid diagnostic task")
		return creativeDiagnosticRecoveryActor{}, false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), parseUUID(taskID.String()))
	if err != nil || task.AgentID != parseUUID(agentID.String()) || (task.Status != "dispatched" && task.Status != "running") {
		writeError(w, http.StatusForbidden, "diagnostic task is not active")
		return creativeDiagnosticRecoveryActor{}, false
	}
	var capable bool
	err = h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM agent agent_row
  JOIN agent_skill binding ON binding.agent_id = agent_row.id AND binding.enabled
  JOIN skill capability ON capability.id = binding.skill_id
  WHERE agent_row.id = $1
    AND agent_row.workspace_id = $2
    AND agent_row.archived_at IS NULL
    AND capability.workspace_id = agent_row.workspace_id
    AND capability.config->>'kind' = 'creative_role'
    AND capability.config->>'capability' = 'crawl_diagnosis'
)
`, parseUUID(agentID.String()), workspaceID).Scan(&capable)
	if err != nil || !capable {
		writeError(w, http.StatusForbidden, "task does not have creative diagnostic recovery authority")
		return creativeDiagnosticRecoveryActor{}, false
	}
	return creativeDiagnosticRecoveryActor{AgentID: parseUUID(agentID.String()), TaskID: parseUUID(taskID.String())}, true
}
