package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type taskFanoutRequest struct {
	TriggerEvidenceKind  string                         `json:"trigger_evidence_kind"`
	TriggerEvidenceRefID string                         `json:"trigger_evidence_ref_id"`
	Items                []service.DirectTaskFanoutItem `json:"items"`
}

type taskFanoutResponse struct {
	Tasks []AgentTaskResponse `json:"tasks"`
}

func (h *Handler) FanoutAgentTasks(w http.ResponseWriter, r *http.Request) {
	agent, ok := h.loadAgentForUser(w, r, chi.URLParam(r, "agentId"))
	if !ok {
		return
	}
	var req taskFanoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	evidenceRefID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(req.TriggerEvidenceRefID), "trigger_evidence_ref_id")
	if !ok {
		return
	}
	evidenceKind := strings.TrimSpace(req.TriggerEvidenceKind)
	if evidenceKind == "" {
		writeError(w, http.StatusBadRequest, "trigger_evidence_kind is required")
		return
	}
	if strings.HasPrefix(evidenceKind, "creative_") && !knownCreativeTaskEvidenceKind(evidenceKind) {
		writeError(w, http.StatusBadRequest, "unsupported creative trigger_evidence_kind")
		return
	}
	if err := validateCreativeTaskFanoutContext(evidenceKind, evidenceRefID, req.Items); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.directTaskEvidenceInWorkspace(w, r, agent.WorkspaceID, evidenceKind, evidenceRefID) {
		return
	}
	if capability := creativeTaskRequiredCapability(evidenceKind); capability != "" {
		hasCapability, err := h.agentHasCreativeCapability(r, agent.ID, agent.WorkspaceID, capability)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate creative task capability")
			return
		}
		if !hasCapability {
			writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("target agent does not provide creative capability %s", capability))
			return
		}
	}

	attr, requestingUserID, ok := h.fanoutAttribution(w, r, agent, evidenceRefID, evidenceKind)
	if !ok {
		return
	}
	tasks, err := h.TaskService.EnqueueDirectTaskFanout(r.Context(), service.DirectTaskFanout{
		Agent:                agent,
		RequestingUserID:     requestingUserID,
		Attribution:          attr,
		TriggerEvidenceKind:  evidenceKind,
		TriggerEvidenceRefID: evidenceRefID,
		Items:                req.Items,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, taskFanoutResponse{Tasks: h.directTaskResponses(r, tasks, uuidToString(agent.WorkspaceID))})
}

func creativeTaskRequiredCapability(kind string) string {
	switch kind {
	case "creative_crawl_run_analysis":
		return "reference_analysis"
	case "creative_order_item_plan":
		return "generation_plan"
	case "creative_order_item_production":
		return "image_edit"
	case "creative_order_variant_prime":
		return "prime_compose"
	case "creative_order_variant_qc":
		return "quality_control"
	case "creative_order_item_direct_edit":
		return "direct_image_edit"
	default:
		return ""
	}
}

// knownCreativeTaskEvidenceKind is intentionally limited to evidence kinds
// that direct fanout can resolve and scope to a workspace.
func knownCreativeTaskEvidenceKind(kind string) bool {
	switch kind {
	case "creative_candidate",
		"creative_crawl_run",
		"creative_crawl_run_analysis",
		"creative_source_analysis",
		"creative_order",
		"creative_order_item",
		"creative_order_item_plan",
		"creative_order_item_production",
		"creative_order_item_direct_edit",
		"creative_variant",
		"creative_order_variant_prime",
		"creative_order_variant_qc",
		"creative_asset",
		"creative_qc_report":
		return true
	default:
		return false
	}
}

func (h *Handler) agentHasCreativeCapability(r *http.Request, agentID, workspaceID pgtype.UUID, capability string) (bool, error) {
	var exists bool
	err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill agent_binding
  JOIN skill s ON s.id = agent_binding.skill_id
  WHERE agent_binding.agent_id = $1
    AND agent_binding.enabled
    AND s.workspace_id = $2
    AND s.config->>'kind' = 'creative_role'
    AND s.config->>'capability' = $3
)
`, agentID, workspaceID, capability).Scan(&exists)
	return exists, err
}

func validateCreativeTaskFanoutContext(kind string, evidenceRefID pgtype.UUID, items []service.DirectTaskFanoutItem) error {
	expectedWorkflow := ""
	referenceField := ""
	requireOrderTrace := false
	switch kind {
	case "creative_crawl_run_analysis":
		expectedWorkflow, referenceField = "creative_reference_analysis", "crawl_run_id"
	case "creative_order_item_plan":
		expectedWorkflow, referenceField, requireOrderTrace = "creative_plan", "creative_order_item_id", true
	case "creative_order_item_production":
		expectedWorkflow, referenceField, requireOrderTrace = "creative_production", "creative_order_item_id", true
	case "creative_order_variant_prime":
		expectedWorkflow, referenceField, requireOrderTrace = "creative_prime", "variant_id", true
	case "creative_order_variant_qc":
		referenceField, requireOrderTrace = "variant_id", true
	case "creative_order_item_direct_edit":
		expectedWorkflow, referenceField, requireOrderTrace = "creative_direct_edit", "creative_order_item_id", true
	default:
		return nil
	}
	expectedRef := uuidToString(evidenceRefID)
	for _, item := range items {
		var context map[string]any
		if json.Unmarshal(item.Context, &context) != nil {
			return errors.New("creative task context must be a JSON object")
		}
		workflow, _ := context["workflow"].(string)
		if kind == "creative_order_variant_qc" {
			if workflow != "creative_qc_technical" && workflow != "creative_qc_visual" {
				return errors.New("creative QC task context has invalid workflow")
			}
			if err := validateCreativeQCTaskContext(context, item.ItemKey, expectedRef, workflow); err != nil {
				return err
			}
		} else if workflow != expectedWorkflow {
			return fmt.Errorf("creative task context workflow must be %s", expectedWorkflow)
		}
		if reference, _ := context[referenceField].(string); reference != expectedRef {
			return fmt.Errorf("creative task context %s must match trigger evidence", referenceField)
		}
		if requireOrderTrace {
			for _, field := range []string{"creative_order_id", "issue_id", "leader_agent_id"} {
				value, _ := context[field].(string)
				if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
					return fmt.Errorf("creative task context %s must be a UUID", field)
				}
			}
		}
	}
	return nil
}

func validateCreativeQCTaskContext(context map[string]any, itemKey, variantID, workflow string) error {
	itemID, _ := context["creative_order_item_id"].(string)
	if _, err := uuid.Parse(strings.TrimSpace(itemID)); err != nil {
		return errors.New("creative QC task context creative_order_item_id must be a UUID")
	}
	revision, ok := context["revision"].(float64)
	if !ok || revision < 1 || revision != float64(int(revision)) {
		return errors.New("creative QC task context revision must be a positive integer")
	}
	expectedSizes, ok := context["expected_sizes"].([]any)
	if !ok || len(expectedSizes) == 0 {
		return errors.New("creative QC task context expected_sizes must be a non-empty array")
	}
	seen := make(map[string]struct{}, len(expectedSizes))
	for _, rawSize := range expectedSizes {
		size, ok := rawSize.(string)
		size = strings.TrimSpace(size)
		if !ok || size == "" {
			return errors.New("creative QC task context expected_sizes must contain non-empty strings")
		}
		if _, duplicate := seen[size]; duplicate {
			return errors.New("creative QC task context expected_sizes must not contain duplicates")
		}
		seen[size] = struct{}{}
	}
	lane := strings.TrimPrefix(workflow, "creative_qc_")
	wantItemKey := fmt.Sprintf("%s:%s:r%d", variantID, lane, int(revision))
	if itemKey != wantItemKey {
		return fmt.Errorf("creative QC task item_key must be %s", wantItemKey)
	}
	return nil
}

// Known Creative evidence kinds must resolve within the task's workspace. Other
// evidence kinds remain generic because direct fanout is not Creative-specific.
func (h *Handler) directTaskEvidenceInWorkspace(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID, kind string, evidenceID pgtype.UUID) bool {
	query := ""
	switch kind {
	case "creative_candidate":
		query = `SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2)`
	case "creative_crawl_run", "crawl_run", "creative_crawl_run_analysis":
		query = `SELECT EXISTS(SELECT 1 FROM creative_material_crawl_run WHERE id = $1 AND workspace_id = $2)`
	case "creative_source_analysis":
		query = `SELECT EXISTS(SELECT 1 FROM creative_source_analysis WHERE id = $1 AND workspace_id = $2)`
	case "creative_order":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order WHERE id = $1 AND workspace_id = $2)`
	case "creative_order_item", "creative_order_item_plan", "creative_order_item_production", "creative_order_item_direct_edit":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_item i JOIN creative_order o ON o.id = i.order_id WHERE i.id = $1 AND o.workspace_id = $2)`
	case "creative_variant", "creative_order_variant_prime", "creative_order_variant_qc":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id = v.order_item_id JOIN creative_order o ON o.id = i.order_id WHERE v.id = $1 AND o.workspace_id = $2)`
	case "creative_asset":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_asset a JOIN creative_order_variant v ON v.id = a.variant_id JOIN creative_order_item i ON i.id = v.order_item_id JOIN creative_order o ON o.id = i.order_id WHERE a.id = $1 AND o.workspace_id = $2)`
	case "creative_qc_report":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_qc_report q JOIN creative_order_variant v ON v.id = q.variant_id JOIN creative_order_item i ON i.id = v.order_item_id JOIN creative_order o ON o.id = i.order_id WHERE q.id = $1 AND o.workspace_id = $2)`
	case "attachment":
		query = `SELECT EXISTS(SELECT 1 FROM attachment WHERE id = $1 AND workspace_id = $2)`
	default:
		return true
	}
	var exists bool
	if err := h.DB.QueryRow(r.Context(), query, evidenceID, workspaceID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusUnprocessableEntity, "trigger evidence does not belong to the agent workspace")
		return false
	}
	return true
}

func (h *Handler) ListAgentTasksBySource(w http.ResponseWriter, r *http.Request) {
	agent, kind, refID, ok := h.directTaskSourceRequest(w, r)
	if !ok {
		return
	}
	if !h.canAccessDirectTaskSource(w, r, agent) {
		return
	}
	status := pgtype.Text{}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		if raw != "active" && raw != "failed" {
			writeError(w, http.StatusBadRequest, "status must be active or failed")
			return
		}
		status = pgtype.Text{String: raw, Valid: true}
	}
	tasks, err := h.TaskService.ListDirectTasksByEvidence(r.Context(), agent.ID, kind, refID, status.String)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list direct tasks")
		return
	}
	writeJSON(w, http.StatusOK, taskFanoutResponse{Tasks: h.directTaskResponses(r, tasks, uuidToString(agent.WorkspaceID))})
}

func (h *Handler) CancelAgentTasksBySource(w http.ResponseWriter, r *http.Request) {
	agent, kind, refID, ok := h.directTaskSourceRequest(w, r)
	if !ok {
		return
	}
	if !h.canAccessDirectTaskSource(w, r, agent) {
		return
	}
	cancelled, err := h.TaskService.CancelDirectTasksByEvidence(r.Context(), agent.ID, kind, refID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel direct tasks")
		return
	}
	writeJSON(w, http.StatusOK, taskFanoutResponse{Tasks: h.directTaskResponses(r, cancelled, uuidToString(agent.WorkspaceID))})
}

func (h *Handler) RetryFailedAgentTasksBySource(w http.ResponseWriter, r *http.Request) {
	agent, kind, refID, ok := h.directTaskSourceRequest(w, r)
	if !ok {
		return
	}
	if !h.canAccessDirectTaskSource(w, r, agent) {
		return
	}
	retried, err := h.TaskService.RetryFailedDirectTasksByEvidence(r.Context(), agent.ID, kind, refID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, taskFanoutResponse{Tasks: h.directTaskResponses(r, retried, uuidToString(agent.WorkspaceID))})
}

func (h *Handler) fanoutAttribution(w http.ResponseWriter, r *http.Request, target db.Agent, evidenceRefID pgtype.UUID, evidenceKind string) (attribution.Result, pgtype.UUID, bool) {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		if !h.canManageAgent(w, r, target) {
			return attribution.Result{}, pgtype.UUID{}, false
		}
		userID, ok := parseUUIDOrBadRequest(w, requestUserID(r), "user_id")
		if !ok {
			return attribution.Result{}, pgtype.UUID{}, false
		}
		return attribution.DirectHumanRun(userID, attribution.EvidenceKind(strings.TrimSpace(evidenceKind)), evidenceRefID), userID, true
	}
	parentID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
	if !ok {
		return attribution.Result{}, pgtype.UUID{}, false
	}
	parent, err := h.Queries.GetAgentTask(r.Context(), parentID)
	if err != nil || uuidToString(parent.AgentID) != r.Header.Get("X-Agent-ID") {
		writeError(w, http.StatusForbidden, "invalid delegating task")
		return attribution.Result{}, pgtype.UUID{}, false
	}
	delegator, err := h.Queries.GetAgent(r.Context(), parent.AgentID)
	if err != nil || delegator.WorkspaceID != target.WorkspaceID {
		writeError(w, http.StatusForbidden, "agent cannot fan out across workspaces")
		return attribution.Result{}, pgtype.UUID{}, false
	}
	return attribution.DelegatedRun(parent.ID, parent.OriginatorUserID, parent.AccountableUserID, attribution.EvidenceKind(strings.TrimSpace(evidenceKind)), evidenceRefID), parent.RequestingUserID, true
}

func (h *Handler) directTaskSourceRequest(w http.ResponseWriter, r *http.Request) (db.Agent, string, pgtype.UUID, bool) {
	agent, ok := h.loadAgentForUser(w, r, chi.URLParam(r, "agentId"))
	if !ok {
		return db.Agent{}, "", pgtype.UUID{}, false
	}
	kind := strings.TrimSpace(r.URL.Query().Get("trigger_evidence_kind"))
	refID, ok := parseUUIDOrBadRequest(w, strings.TrimSpace(r.URL.Query().Get("trigger_evidence_ref_id")), "trigger_evidence_ref_id")
	if kind == "" {
		writeError(w, http.StatusBadRequest, "trigger_evidence_kind is required")
		return db.Agent{}, "", pgtype.UUID{}, false
	}
	if !ok {
		return db.Agent{}, "", pgtype.UUID{}, false
	}
	return agent, kind, refID, true
}

func (h *Handler) canAccessDirectTaskSource(w http.ResponseWriter, r *http.Request, agent db.Agent) bool {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		return h.canManageAgent(w, r, agent)
	}
	parentID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		writeError(w, http.StatusForbidden, "invalid delegating task")
		return false
	}
	parent, err := h.Queries.GetAgentTask(r.Context(), parentID)
	if err != nil || uuidToString(parent.AgentID) != r.Header.Get("X-Agent-ID") {
		writeError(w, http.StatusForbidden, "invalid delegating task")
		return false
	}
	delegator, err := h.Queries.GetAgent(r.Context(), parent.AgentID)
	if err != nil || delegator.WorkspaceID != agent.WorkspaceID {
		writeError(w, http.StatusForbidden, "agent cannot access tasks across workspaces")
		return false
	}
	return true
}

func (h *Handler) directTaskResponses(r *http.Request, tasks []db.AgentTaskQueue, workspaceID string) []AgentTaskResponse {
	responses := make([]AgentTaskResponse, len(tasks))
	for i, task := range tasks {
		responses[i] = h.hydratedTaskResponse(r.Context(), task, workspaceID)
	}
	return responses
}
