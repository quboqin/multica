package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type creativeCandidateProgress struct {
	State            string `json:"state"`
	Target           int    `json:"target"`
	Expected         int    `json:"expected"`
	Planned          int    `json:"planned"`
	Generated        int    `json:"generated"`
	Primed           int    `json:"primed"`
	Settled          int    `json:"settled"`
	PlanTaskID       string `json:"plan_task_id"`
	PlanStatus       string `json:"plan_status"`
	SelectionTaskID  string `json:"selection_task_id"`
	SelectionStatus  string `json:"selection_status"`
	selected         int
	activeProduction int
	activePrime      int
	cancelled        int
	productionTasks  int
	ready            int
}

func loadCreativeCandidateProgress(ctx context.Context, q dbExecutor, orderID, itemID pgtype.UUID) (*creativeCandidateProgress, error) {
	values, err := loadCreativeCandidateProgressBatch(ctx, q, orderID, []pgtype.UUID{itemID})
	return values[uuidToString(itemID)], err
}

func (h *Handler) RecoverCreativeOrderCandidates(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	itemID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "itemId"), "order_item_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin candidate recovery")
		return
	}
	defer tx.Rollback(r.Context())
	var orderStatus string
	err = tx.QueryRow(r.Context(), `SELECT o.status FROM creative_order o JOIN creative_order_item i ON i.order_id=o.id WHERE o.id=$1 AND o.workspace_id=$2 AND i.id=$3 FOR UPDATE OF o,i`, orderID, workspaceID, itemID).Scan(&orderStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock candidate recovery")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	p, err := loadCreativeCandidateProgress(r.Context(), tx, orderID, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load candidate progress")
		return
	}
	if p == nil || p.State == "cancelled" {
		writeError(w, http.StatusConflict, "candidate recovery is no longer applicable")
		return
	}
	var task db.AgentTaskQueue
	queued := false
	switch p.State {
	case "planning_incomplete":
		if p.PlanTaskID != "" && (p.PlanStatus == "queued" || p.PlanStatus == "dispatched" || p.PlanStatus == "running" || p.PlanStatus == "waiting_local_directory") {
			writeJSON(w, http.StatusOK, creativeOrderWorkflowRetryResponse{TaskID: p.PlanTaskID})
			return
		}
		if p.PlanTaskID == "" || (p.PlanStatus != "failed" && p.PlanStatus != "completed") {
			writeError(w, http.StatusConflict, "candidate planning is not ready for recovery")
			return
		}
		parent, loadErr := h.Queries.WithTx(tx).GetAgentTask(r.Context(), parseUUID(p.PlanTaskID))
		if loadErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load candidate planning task")
			return
		}
		var scope map[string]any
		if json.Unmarshal(parent.Context, &scope) != nil || scope["workflow"] != "creative_plan" || scope["creative_order_id"] != uuidToString(orderID) || scope["creative_order_item_id"] != uuidToString(itemID) {
			writeError(w, http.StatusConflict, "candidate planning task context does not match")
			return
		}
		if p.PlanStatus == "failed" {
			task, err = h.Queries.WithTx(tx).CreateRetryTask(r.Context(), parent.ID)
		} else {
			task, err = h.Queries.WithTx(tx).CreateActionRequiredRetryTask(r.Context(), parent.ID)
		}
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "candidate planning retry budget exhausted")
			return
		}
		queued = true
	case "selection_ready", "selection_incomplete":
		queued, err = h.queueCreativeCandidateSelectionTx(r.Context(), tx, itemID, creativeOrchestrationCause{RequestedBy: userID}, nil, &task)
		if err == nil && !queued {
			writeError(w, http.StatusConflict, "candidate selection is not ready or its retry budget is exhausted")
			return
		}
	case "selection_queued", "selecting":
		writeJSON(w, http.StatusOK, creativeOrderWorkflowRetryResponse{TaskID: p.SelectionTaskID})
		return
	default:
		writeError(w, http.StatusConflict, "candidate images or production tasks are still incomplete")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue candidate recovery")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit candidate recovery")
		return
	}
	if queued {
		h.TaskService.NotifyTaskEnqueued(r.Context(), task)
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": uuidToString(orderID)})
	writeJSON(w, http.StatusOK, creativeOrderWorkflowRetryResponse{TaskID: uuidToString(task.ID)})
}

func (h *Handler) creativePlanningCompletionError(ctx context.Context, task db.AgentTaskQueue, workspace string) (string, error) {
	if !task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != "creative_order_item_plan" || isCreativeTaskTerminal(task.Status) || task.Status == "cancelled" {
		return "", nil
	}
	var scope struct {
		Type     string `json:"type"`
		Workflow string `json:"workflow"`
		OrderID  string `json:"creative_order_id"`
		ItemID   string `json:"creative_order_item_id"`
	}
	if json.Unmarshal(task.Context, &scope) != nil || scope.Type != "creative_domain_task" || scope.Workflow != "creative_plan" {
		return "creative planning task has invalid context", nil
	}
	orderID, err := parseCreativeOrchestrationUUID(scope.OrderID)
	if err != nil {
		return "creative planning task has invalid order", nil
	}
	itemID, err := parseCreativeOrchestrationUUID(scope.ItemID)
	if err != nil || itemID != task.TriggerEvidenceRefID {
		return "creative planning task has invalid item", nil
	}
	workspaceID, err := parseCreativeOrchestrationUUID(workspace)
	if err != nil {
		return "creative planning task has invalid workspace", nil
	}
	counts, err := loadCreativeOrderVariantCounts(ctx, h.DB, orderID, workspaceID)
	if err != nil {
		return "", err
	}
	var sourceKind string
	var total, valid, dispatched int
	err = h.DB.QueryRow(ctx, `
SELECT i.source_kind, count(v.id), count(v.id) FILTER (WHERE v.variant_key ~ $3 AND v.primary_size='1080x1080'),
 count(v.id) FILTER (WHERE EXISTS (SELECT 1 FROM agent_task_queue p WHERE p.trigger_evidence_kind='creative_order_item_production' AND p.trigger_evidence_ref_id=i.id AND p.context->>'workflow'='creative_production' AND p.context->>'variant_id'=v.id::text AND p.context->>'revision'=v.revision::text))
FROM creative_order_item i LEFT JOIN creative_order_variant v ON v.order_item_id=i.id WHERE i.id=$1 AND i.order_id=$2 GROUP BY i.id
`, itemID, orderID, counts.keyPattern()).Scan(&sourceKind, &total, &valid, &dispatched)
	if err != nil {
		return "", err
	}
	minimum := counts.Target
	if sourceKind == "copy_library" && counts.Configured {
		minimum = counts.Candidates
	}
	if total < minimum || total > counts.Candidates || valid != total || dispatched != total {
		return fmt.Sprintf("creative planning incomplete: expected %d-%d candidates, registered %d, valid square candidates %d, production tasks %d; preserve existing candidates and enqueue only missing work", minimum, counts.Candidates, total, valid, dispatched), nil
	}
	return "", nil
}
