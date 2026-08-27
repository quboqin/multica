package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type creativeFanoutScope struct {
	orderID    pgtype.UUID
	itemID     pgtype.UUID
	variantIDs []pgtype.UUID
}

func creativeFanoutContextVariantIDs(items []service.DirectTaskFanoutItem) ([]pgtype.UUID, error) {
	seen := make(map[string]struct{}, len(items))
	ids := make([]pgtype.UUID, 0, len(items))
	for _, item := range items {
		var taskContext struct {
			VariantID string `json:"variant_id"`
		}
		if json.Unmarshal(item.Context, &taskContext) != nil || strings.TrimSpace(taskContext.VariantID) == "" {
			continue
		}
		parsed, err := uuid.Parse(strings.TrimSpace(taskContext.VariantID))
		if err != nil {
			return nil, errors.New("creative task context variant_id must be a UUID")
		}
		id := parseUUID(parsed.String())
		key := uuidToString(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func resolveCreativeFanoutScope(ctx context.Context, tx pgx.Tx, kind string, evidenceID pgtype.UUID, items []service.DirectTaskFanoutItem) (creativeFanoutScope, error) {
	var scope creativeFanoutScope
	var err error
	switch kind {
	case "creative_order":
		scope.orderID = evidenceID
	case "creative_order_item", "creative_order_item_plan", "creative_order_item_production", "creative_order_item_direct_edit":
		scope.itemID = evidenceID
		err = tx.QueryRow(ctx, `SELECT order_id FROM creative_order_item WHERE id = $1`, evidenceID).Scan(&scope.orderID)
	case "creative_variant", "creative_order_variant_qc":
		scope.variantIDs = []pgtype.UUID{evidenceID}
		err = tx.QueryRow(ctx, `
SELECT item.id, item.order_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1
`, evidenceID).Scan(&scope.itemID, &scope.orderID)
	case "creative_asset":
		scope.variantIDs = make([]pgtype.UUID, 1)
		err = tx.QueryRow(ctx, `
SELECT item.id, item.order_id, variant.id
FROM creative_order_asset asset
JOIN creative_order_variant variant ON variant.id = asset.variant_id
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE asset.id = $1
`, evidenceID).Scan(&scope.itemID, &scope.orderID, &scope.variantIDs[0])
	case "creative_qc_report":
		scope.variantIDs = make([]pgtype.UUID, 1)
		err = tx.QueryRow(ctx, `
SELECT item.id, item.order_id, variant.id
FROM creative_order_qc_report report
JOIN creative_order_variant variant ON variant.id = report.variant_id
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE report.id = $1
`, evidenceID).Scan(&scope.itemID, &scope.orderID, &scope.variantIDs[0])
	default:
		return scope, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return creativeFanoutScope{}, errors.New("creative trigger evidence is unavailable")
	}
	if err != nil {
		return creativeFanoutScope{}, fmt.Errorf("resolve creative task scope: %w", err)
	}
	contextVariantIDs, err := creativeFanoutContextVariantIDs(items)
	if err != nil {
		return creativeFanoutScope{}, err
	}
	seen := make(map[string]struct{}, len(scope.variantIDs)+len(contextVariantIDs))
	merged := make([]pgtype.UUID, 0, len(scope.variantIDs)+len(contextVariantIDs))
	for _, id := range append(scope.variantIDs, contextVariantIDs...) {
		key := uuidToString(id)
		if !id.Valid || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, id)
	}
	scope.variantIDs = merged
	return scope, nil
}

// lockCreativeFanoutFence serializes task-created children with task/order
// cancellation. Callers resolve coordinates without locks, then always take
// task -> order -> item -> variant row locks in that order.
func lockCreativeFanoutFence(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID, kind string, evidenceID pgtype.UUID, items []service.DirectTaskFanoutItem, actorTaskID, actorAgentID pgtype.UUID) error {
	scope, err := resolveCreativeFanoutScope(ctx, tx, kind, evidenceID, items)
	if err != nil {
		return err
	}
	if actorTaskID.Valid {
		var taskOrderID string
		err := tx.QueryRow(ctx, `
SELECT COALESCE(task.context->>'creative_order_id', '')
FROM agent_task_queue task
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1 AND task.agent_id = $2
  AND assigned_agent.workspace_id = $3
  AND task.status IN ('dispatched', 'running')
FOR UPDATE OF task
`, actorTaskID, actorAgentID, workspaceID).Scan(&taskOrderID)
		if errors.Is(err, pgx.ErrNoRows) {
			return errors.New("delegating task is no longer active")
		}
		if err != nil {
			return fmt.Errorf("lock delegating task: %w", err)
		}
		if scope.orderID.Valid && taskOrderID != uuidToString(scope.orderID) {
			return errors.New("delegating task does not match the creative order")
		}
	}
	if !scope.orderID.Valid {
		return nil
	}
	var orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, scope.orderID, workspaceID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) {
		return errors.New("creative order is unavailable")
	} else if err != nil {
		return fmt.Errorf("lock creative order: %w", err)
	}
	if orderStatus == "cancelled" || orderStatus == "completed" {
		return fmt.Errorf("creative order is %s", orderStatus)
	}
	if scope.itemID.Valid {
		var itemStatus string
		if err := tx.QueryRow(ctx, `
SELECT status FROM creative_order_item
WHERE id = $1 AND order_id = $2
FOR UPDATE
`, scope.itemID, scope.orderID).Scan(&itemStatus); errors.Is(err, pgx.ErrNoRows) {
			return errors.New("creative order item is unavailable")
		} else if err != nil {
			return fmt.Errorf("lock creative order item: %w", err)
		}
		if itemStatus == "cancelled" {
			return errors.New("creative order item is cancelled")
		}
	}
	if len(scope.variantIDs) == 0 {
		return nil
	}
	variantTexts := make([]string, 0, len(scope.variantIDs))
	for _, id := range scope.variantIDs {
		variantTexts = append(variantTexts, uuidToString(id))
	}
	rows, err := tx.Query(ctx, `
SELECT variant.id::text, variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = ANY($1::uuid[])
  AND item.order_id = $2
  AND ($3::uuid IS NULL OR item.id = $3)
ORDER BY variant.id
FOR UPDATE OF variant
`, variantTexts, scope.orderID, scope.itemID)
	if err != nil {
		return fmt.Errorf("lock creative task variants: %w", err)
	}
	defer rows.Close()
	locked := 0
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return fmt.Errorf("read creative task variant: %w", err)
		}
		locked++
		if status == "cancelled" {
			return errors.New("creative order variant is cancelled")
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read creative task variants: %w", err)
	}
	if locked != len(scope.variantIDs) {
		return errors.New("creative task variant does not belong to the trigger scope")
	}
	return nil
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
	normalizedItems, err := h.normalizeCreativeTaskFanoutItems(r.Context(), agent.WorkspaceID, evidenceKind, evidenceRefID, req.Items)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	req.Items = normalizedItems
	if err := validateCreativeTaskFanoutContext(evidenceKind, evidenceRefID, req.Items); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateCreativeTaskFanoutExpectedSizes(r.Context(), agent.WorkspaceID, evidenceKind, evidenceRefID, req.Items); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.directTaskEvidenceInWorkspace(w, r, agent.WorkspaceID, evidenceKind, evidenceRefID) {
		return
	}
	attr, requestingUserID, ok := h.fanoutAttribution(w, r, agent, evidenceRefID, evidenceKind)
	if !ok {
		return
	}
	var actorTaskID, actorAgentID pgtype.UUID
	if r.Header.Get("X-Actor-Source") == "task_token" {
		actorTaskID = attr.DelegatedFromTaskID
		actorAgentID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Agent-ID"), "agent_id")
		if !ok {
			return
		}
	}
	if evidenceKind == "creative_order_item_production" {
		tasks, err := h.enqueueCreativeProductionFanout(r.Context(), agent.WorkspaceID, evidenceRefID, req.Items, attr, requestingUserID, actorTaskID, actorAgentID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, taskFanoutResponse{Tasks: h.directTaskResponses(r, tasks, uuidToString(agent.WorkspaceID))})
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

	fanout := service.DirectTaskFanout{
		Agent:                agent,
		RequestingUserID:     requestingUserID,
		Attribution:          attr,
		TriggerEvidenceKind:  evidenceKind,
		TriggerEvidenceRefID: evidenceRefID,
		Items:                req.Items,
	}
	var tasks []db.AgentTaskQueue
	if strings.HasPrefix(evidenceKind, "creative_") {
		tasks, err = h.enqueueDirectTaskFanoutWithCreativeFence(r.Context(), agent.WorkspaceID, fanout, actorTaskID, actorAgentID)
	} else {
		tasks, err = h.TaskService.EnqueueDirectTaskFanout(r.Context(), fanout)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, taskFanoutResponse{Tasks: h.directTaskResponses(r, tasks, uuidToString(agent.WorkspaceID))})
}

type creativeProductionFanoutGroup struct {
	agent db.Agent
	items []service.DirectTaskFanoutItem
}

func (h *Handler) prepareCreativeProductionFanout(ctx context.Context, workspaceID, orderItemID pgtype.UUID, items []service.DirectTaskFanoutItem) ([]creativeProductionFanoutGroup, error) {
	groups := map[string]creativeProductionFanoutGroup{}
	order := make([]string, 0, len(items))
	for _, item := range items {
		agent, normalized, err := h.selectCreativeProductionAgentForFanoutItem(ctx, workspaceID, orderItemID, item)
		if err != nil {
			return nil, err
		}
		key := uuidToString(agent.ID)
		group, exists := groups[key]
		if !exists {
			group.agent = agent
			order = append(order, key)
		}
		group.items = append(group.items, normalized)
		groups[key] = group
	}
	prepared := make([]creativeProductionFanoutGroup, 0, len(order))
	for _, key := range order {
		prepared = append(prepared, groups[key])
	}
	return prepared, nil
}

func (h *Handler) enqueueCreativeProductionFanoutTx(ctx context.Context, tx pgx.Tx, orderItemID pgtype.UUID, groups []creativeProductionFanoutGroup, attr attribution.Result, requestingUserID pgtype.UUID) ([]db.AgentTaskQueue, []db.AgentTaskQueue, error) {
	tasks := make([]db.AgentTaskQueue, 0)
	created := make([]db.AgentTaskQueue, 0)
	for _, group := range groups {
		groupTasks, groupCreated, err := h.TaskService.EnqueueDirectTaskFanoutTx(ctx, tx, service.DirectTaskFanout{
			Agent:                group.agent,
			RequestingUserID:     requestingUserID,
			Attribution:          attr,
			TriggerEvidenceKind:  "creative_order_item_production",
			TriggerEvidenceRefID: orderItemID,
			Items:                group.items,
		})
		if err != nil {
			return nil, nil, err
		}
		tasks = append(tasks, groupTasks...)
		created = append(created, groupCreated...)
	}
	return tasks, created, nil
}

func (h *Handler) enqueueCreativeProductionFanout(ctx context.Context, workspaceID, orderItemID pgtype.UUID, items []service.DirectTaskFanoutItem, attr attribution.Result, requestingUserID, actorTaskID, actorAgentID pgtype.UUID) ([]db.AgentTaskQueue, error) {
	if h.TaskService == nil {
		return nil, errors.New("creative production task service is unavailable")
	}
	groups, err := h.prepareCreativeProductionFanout(ctx, workspaceID, orderItemID, items)
	if err != nil {
		return nil, err
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin creative production fanout: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCreativeFanoutFence(ctx, tx, workspaceID, "creative_order_item_production", orderItemID, items, actorTaskID, actorAgentID); err != nil {
		return nil, err
	}
	tasks, created, err := h.enqueueCreativeProductionFanoutTx(ctx, tx, orderItemID, groups, attr, requestingUserID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit creative production fanout: %w", err)
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(ctx, created)
	return tasks, nil
}

func (h *Handler) enqueueDirectTaskFanoutWithCreativeFence(ctx context.Context, workspaceID pgtype.UUID, fanout service.DirectTaskFanout, actorTaskID, actorAgentID pgtype.UUID) ([]db.AgentTaskQueue, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin creative task fanout: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := lockCreativeFanoutFence(ctx, tx, workspaceID, fanout.TriggerEvidenceKind, fanout.TriggerEvidenceRefID, fanout.Items, actorTaskID, actorAgentID); err != nil {
		return nil, err
	}
	tasks, created, err := h.TaskService.EnqueueDirectTaskFanoutTx(ctx, tx, fanout)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit creative task fanout: %w", err)
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(ctx, created)
	return tasks, nil
}

func (h *Handler) selectCreativeProductionAgentForFanoutItem(ctx context.Context, workspaceID, orderItemID pgtype.UUID, item service.DirectTaskFanoutItem) (db.Agent, service.DirectTaskFanoutItem, error) {
	var taskContext map[string]json.RawMessage
	if len(item.Context) == 0 || json.Unmarshal(item.Context, &taskContext) != nil || taskContext == nil {
		return db.Agent{}, item, errors.New("creative production task context must be a JSON object")
	}
	var variantIDText string
	if raw := taskContext["variant_id"]; raw == nil || json.Unmarshal(raw, &variantIDText) != nil || strings.TrimSpace(variantIDText) == "" {
		return db.Agent{}, item, errors.New("creative production task context variant_id must be a UUID")
	}
	variantUUID, err := uuid.Parse(strings.TrimSpace(variantIDText))
	if err != nil {
		return db.Agent{}, item, errors.New("creative production task context variant_id must be a UUID")
	}
	variantID := pgtype.UUID{Bytes: variantUUID, Valid: true}

	var inputSnapshot []byte
	if err := h.DB.QueryRow(ctx, `
SELECT order_row.input_snapshot
FROM creative_order_item item
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1 AND order_row.workspace_id = $2
`, orderItemID, workspaceID).Scan(&inputSnapshot); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return db.Agent{}, item, errors.New("creative production trigger order item is unavailable")
		}
		return db.Agent{}, item, errors.New("failed to load creative production trigger order")
	}

	preferred := creativeTaskPreferredProducerID(item.Context)
	pinPreferred := false
	if revision, hasRevision, err := jsonPositiveInt(taskContext["revision"]); err == nil && hasRevision && revision > 0 {
		if historical := selectedCreativeProductionAgentFromHistory(ctx, h.DB, workspaceID, variantID, revision); historical.Valid {
			preferred = historical
			pinPreferred = true
		}
	}
	agent, err := h.selectCreativeImageEditAgent(ctx, h.DB, h.Queries, workspaceID, json.RawMessage(inputSnapshot), strings.TrimSpace(variantIDText), preferred, pinPreferred)
	if err != nil {
		return db.Agent{}, item, err
	}
	taskContext["producer_agent_id"], _ = json.Marshal(uuidToString(agent.ID))
	taskContext["producer_runtime_id"], _ = json.Marshal(uuidToString(agent.RuntimeID))
	encoded, err := json.Marshal(taskContext)
	if err != nil {
		return db.Agent{}, item, errors.New("failed to encode creative production task context")
	}
	item.Context = encoded
	return agent, item, nil
}

type creativeTaskFanoutQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (h *Handler) normalizeCreativeTaskFanoutItems(ctx context.Context, workspaceID pgtype.UUID, kind string, evidenceRefID pgtype.UUID, items []service.DirectTaskFanoutItem) ([]service.DirectTaskFanoutItem, error) {
	switch kind {
	case "creative_order_item_production":
		return normalizeCreativeProductionFanoutItems(ctx, h.DB, workspaceID, evidenceRefID, items)
	case "manual":
		return normalizeManualCreativeProductionFanoutItems(ctx, h.DB, workspaceID, evidenceRefID, items)
	default:
		return items, nil
	}
}

func (h *Handler) validateCreativeTaskFanoutExpectedSizes(ctx context.Context, workspaceID pgtype.UUID, kind string, evidenceRefID pgtype.UUID, items []service.DirectTaskFanoutItem) error {
	if kind != "creative_order_item_production" && kind != "creative_order_item_direct_edit" && kind != "creative_order_variant_qc" && kind != "manual" {
		return nil
	}

	for _, item := range items {
		var taskContext map[string]any
		if err := json.Unmarshal(item.Context, &taskContext); err != nil || taskContext == nil {
			return errors.New("creative task context must be a JSON object")
		}
		if kind == "manual" {
			taskType, _ := taskContext["type"].(string)
			workflow, _ := taskContext["workflow"].(string)
			if taskType != "creative_domain_task" || workflow != "creative_production" {
				continue
			}
		}
		variantID := evidenceRefID
		if kind != "creative_order_variant_qc" {
			variantIDText, _ := taskContext["variant_id"].(string)
			parsedVariantID, err := uuid.Parse(strings.TrimSpace(variantIDText))
			if err != nil {
				return errors.New("creative task context variant_id must be a UUID")
			}
			variantID = parseUUID(parsedVariantID.String())
		}
		expectedSizes, err := creativeFanoutVariantExpectedSizes(ctx, h.DB, workspaceID, kind, evidenceRefID, variantID)
		if err != nil {
			return err
		}
		if !creativeFanoutExpectedSizesMatch(taskContext["expected_sizes"], expectedSizes) {
			if kind == "creative_order_variant_qc" {
				return errors.New("creative QC task context expected_sizes must match the current creative variant delivery sizes")
			}
			return errors.New("creative production task context expected_sizes must match the current creative variant delivery sizes")
		}
	}
	return nil
}

func creativeFanoutVariantExpectedSizes(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID pgtype.UUID, kind string, evidenceRefID, variantID pgtype.UUID) ([]string, error) {
	var triggerKind, inputSnapshot, brief, candidateState, primarySize string
	var err error
	if kind == "creative_order_item_production" {
		err = q.QueryRow(ctx, `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text,
       variant.candidate_state, variant.primary_size
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND item.id = $2
  AND order_row.workspace_id = $3
`, variantID, evidenceRefID, workspaceID).Scan(&triggerKind, &inputSnapshot, &brief, &candidateState, &primarySize)
	} else if kind == "manual" {
		err = q.QueryRow(ctx, `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text,
       variant.candidate_state, variant.primary_size
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND order_row.id = $2
  AND order_row.workspace_id = $3
`, variantID, evidenceRefID, workspaceID).Scan(&triggerKind, &inputSnapshot, &brief, &candidateState, &primarySize)
	} else if kind == "creative_order_item_direct_edit" {
		err = q.QueryRow(ctx, `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text,
       variant.candidate_state, variant.primary_size
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND item.id = $2
  AND order_row.workspace_id = $3
`, variantID, evidenceRefID, workspaceID).Scan(&triggerKind, &inputSnapshot, &brief, &candidateState, &primarySize)
	} else {
		err = q.QueryRow(ctx, `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND order_row.workspace_id = $2
`, variantID, workspaceID).Scan(&triggerKind, &inputSnapshot, &brief)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if kind == "creative_order_variant_qc" {
			return nil, errors.New("creative QC task variant must belong to the trigger workspace")
		}
		return nil, errors.New("creative production task variant must belong to the trigger order item")
	}
	if err != nil {
		if kind == "creative_order_variant_qc" {
			return nil, errors.New("failed to validate creative QC task variant")
		}
		return nil, errors.New("failed to validate creative production task variant")
	}
	var expectedSizes []string
	if kind == "creative_order_item_production" || kind == "manual" {
		expectedSizes, err = expectedCreativeVariantProductionSizes(
			triggerKind,
			json.RawMessage(inputSnapshot),
			json.RawMessage(brief),
			candidateState,
			primarySize,
		)
	} else {
		expectedSizes, err = expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	}
	if err != nil {
		return nil, err
	}
	return expectedSizes, nil
}

func creativeFanoutExpectedSizesMatch(raw any, expected []string) bool {
	sizes, ok := raw.([]any)
	if !ok || len(sizes) != len(expected) {
		return false
	}
	seen := make(map[string]struct{}, len(sizes))
	for _, rawSize := range sizes {
		size, ok := rawSize.(string)
		if !ok {
			return false
		}
		if _, duplicate := seen[size]; duplicate || !creativeSizeIsExpected(size, expected) {
			return false
		}
		seen[size] = struct{}{}
	}
	return len(seen) == len(expected)
}

func normalizeCreativeProductionFanoutItems(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, orderItemID pgtype.UUID, items []service.DirectTaskFanoutItem) ([]service.DirectTaskFanoutItem, error) {
	normalized := make([]service.DirectTaskFanoutItem, 0, len(items))
	for _, item := range items {
		next, err := normalizeCreativeProductionFanoutItem(ctx, q, workspaceID, orderItemID, item)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func normalizeCreativeProductionFanoutItem(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, orderItemID pgtype.UUID, item service.DirectTaskFanoutItem) (service.DirectTaskFanoutItem, error) {
	var taskContext map[string]json.RawMessage
	if len(item.Context) == 0 || json.Unmarshal(item.Context, &taskContext) != nil || taskContext == nil {
		return item, errors.New("creative production task context must be a JSON object")
	}
	var variantIDText string
	if raw := taskContext["variant_id"]; raw == nil || json.Unmarshal(raw, &variantIDText) != nil || strings.TrimSpace(variantIDText) == "" {
		return item, errors.New("creative production task context variant_id must be a UUID")
	}
	variantUUID, err := uuid.Parse(strings.TrimSpace(variantIDText))
	if err != nil {
		return item, errors.New("creative production task context variant_id must be a UUID")
	}
	variantID := parseUUID(variantUUID.String())
	var revision int
	var canonicalOrderItemID, canonicalOrderID, canonicalIssueID string
	err = q.QueryRow(ctx, `
SELECT variant.revision, item.id::text, order_row.id::text, COALESCE(order_row.issue_id::text, '')
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND item.id = $2
  AND order_row.workspace_id = $3
`, variantID, orderItemID, workspaceID).Scan(&revision, &canonicalOrderItemID, &canonicalOrderID, &canonicalIssueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, errors.New("creative production task variant must belong to the trigger order item")
	}
	if err != nil {
		return item, errors.New("failed to validate creative production task variant")
	}
	suppliedRevision, hasRevision, err := jsonPositiveInt(taskContext["revision"])
	if err != nil {
		return item, errors.New("creative production task context revision must be a positive integer")
	}
	if hasRevision && suppliedRevision != 0 && suppliedRevision != revision {
		return item, errors.New("creative production task context revision must match the current creative order variant revision")
	}
	canonicalVariantID := uuidToString(variantID)
	canonicalItemKey := fmt.Sprintf("%s:r%d", canonicalVariantID, revision)
	taskContext["variant_id"], _ = json.Marshal(canonicalVariantID)
	taskContext["creative_order_id"], _ = json.Marshal(canonicalOrderID)
	taskContext["creative_order_item_id"], _ = json.Marshal(canonicalOrderItemID)
	var suppliedIssueID string
	_ = json.Unmarshal(taskContext["issue_id"], &suppliedIssueID)
	if strings.TrimSpace(suppliedIssueID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(suppliedIssueID)); err != nil {
			return item, errors.New("creative production task context issue_id must be a UUID")
		}
	} else if canonicalIssueID != "" {
		taskContext["issue_id"], _ = json.Marshal(canonicalIssueID)
	}
	taskContext["revision"], _ = json.Marshal(revision)
	taskContext["item_key"], _ = json.Marshal(canonicalItemKey)
	if _, ok := taskContext["expected_sizes"]; !ok || jsonArrayLength(taskContext["expected_sizes"]) == 0 {
		expectedSizes, err := creativeFanoutVariantExpectedSizes(ctx, q, workspaceID, "creative_order_item_production", orderItemID, variantID)
		if err != nil {
			return item, err
		}
		taskContext["expected_sizes"], _ = json.Marshal(expectedSizes)
	}
	if _, adjustment := taskContext["order_adjustment"]; adjustment {
		var orderAdjustment struct {
			AdjustmentIssueID string `json:"adjustment_issue_id"`
			SourceRevision    int    `json:"source_revision"`
			TargetSize        string `json:"target_size"`
			SourceAssetID     string `json:"source_asset_id"`
			SourceAttachment  string `json:"source_attachment_id"`
			Request           string `json:"request"`
		}
		if json.Unmarshal(taskContext["order_adjustment"], &orderAdjustment) != nil ||
			strings.TrimSpace(orderAdjustment.AdjustmentIssueID) == "" || strings.TrimSpace(orderAdjustment.SourceAssetID) == "" ||
			strings.TrimSpace(orderAdjustment.SourceAttachment) == "" || strings.TrimSpace(orderAdjustment.Request) == "" ||
			orderAdjustment.SourceRevision != revision-1 || !validCreativeAssetSize(strings.TrimSpace(orderAdjustment.TargetSize)) ||
			strings.TrimSpace(suppliedIssueID) != strings.TrimSpace(orderAdjustment.AdjustmentIssueID) {
			return item, errors.New("creative production order adjustment context is invalid")
		}
		taskContext["scope"], _ = json.Marshal("size")
	} else {
		taskContext["scope"], _ = json.Marshal("variant")
	}
	taskContext["subject_id"], _ = json.Marshal(canonicalVariantID)
	encoded, err := json.Marshal(taskContext)
	if err != nil {
		return item, errors.New("failed to normalize creative production task context")
	}
	return service.DirectTaskFanoutItem{ItemKey: canonicalItemKey, Context: encoded}, nil
}

func normalizeManualCreativeProductionFanoutItems(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, orderID pgtype.UUID, items []service.DirectTaskFanoutItem) ([]service.DirectTaskFanoutItem, error) {
	normalized := make([]service.DirectTaskFanoutItem, 0, len(items))
	for _, item := range items {
		next, err := normalizeManualCreativeProductionFanoutItem(ctx, q, workspaceID, orderID, item)
		if err != nil {
			return nil, err
		}
		normalized = append(normalized, next)
	}
	return normalized, nil
}

func normalizeManualCreativeProductionFanoutItem(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, orderID pgtype.UUID, item service.DirectTaskFanoutItem) (service.DirectTaskFanoutItem, error) {
	var taskContext map[string]json.RawMessage
	if len(item.Context) == 0 || json.Unmarshal(item.Context, &taskContext) != nil || taskContext == nil {
		return item, nil
	}
	var taskType, workflow string
	_ = json.Unmarshal(taskContext["type"], &taskType)
	_ = json.Unmarshal(taskContext["workflow"], &workflow)
	if taskType != "creative_domain_task" || workflow != "creative_production" {
		return item, nil
	}
	var variantIDText string
	if raw := taskContext["variant_id"]; raw == nil || json.Unmarshal(raw, &variantIDText) != nil || strings.TrimSpace(variantIDText) == "" {
		return item, errors.New("manual creative production task context variant_id must be a UUID")
	}
	variantUUID, err := uuid.Parse(strings.TrimSpace(variantIDText))
	if err != nil {
		return item, errors.New("manual creative production task context variant_id must be a UUID")
	}
	variantID := parseUUID(variantUUID.String())
	var revision int
	var canonicalOrderItemID, canonicalOrderID, canonicalIssueID string
	err = q.QueryRow(ctx, `
SELECT variant.revision, item.id::text, order_row.id::text, COALESCE(order_row.issue_id::text, '')
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND order_row.id = $2
  AND order_row.workspace_id = $3
`, variantID, orderID, workspaceID).Scan(&revision, &canonicalOrderItemID, &canonicalOrderID, &canonicalIssueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, errors.New("manual creative production task variant must belong to the trigger order")
	}
	if err != nil {
		return item, errors.New("failed to validate manual creative production task variant")
	}
	suppliedRevision, hasRevision, err := jsonPositiveInt(taskContext["revision"])
	if err != nil {
		return item, errors.New("manual creative production task context revision must be a positive integer")
	}
	if hasRevision && suppliedRevision != 0 && suppliedRevision != revision {
		return item, errors.New("manual creative production task context revision must match the current creative order variant revision")
	}
	canonicalVariantID := uuidToString(variantID)
	canonicalItemKey := fmt.Sprintf("%s:r%d", canonicalVariantID, revision)
	taskContext["variant_id"], _ = json.Marshal(canonicalVariantID)
	taskContext["creative_order_id"], _ = json.Marshal(canonicalOrderID)
	taskContext["creative_order_item_id"], _ = json.Marshal(canonicalOrderItemID)
	var suppliedIssueID string
	_ = json.Unmarshal(taskContext["issue_id"], &suppliedIssueID)
	if strings.TrimSpace(suppliedIssueID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(suppliedIssueID)); err != nil {
			return item, errors.New("manual creative production task context issue_id must be a UUID")
		}
	} else if canonicalIssueID != "" {
		taskContext["issue_id"], _ = json.Marshal(canonicalIssueID)
	}
	taskContext["revision"], _ = json.Marshal(revision)
	taskContext["item_key"], _ = json.Marshal(canonicalItemKey)
	if _, ok := taskContext["expected_sizes"]; !ok || jsonArrayLength(taskContext["expected_sizes"]) == 0 {
		expectedSizes, err := creativeFanoutVariantExpectedSizes(ctx, q, workspaceID, "manual", orderID, variantID)
		if err != nil {
			return item, err
		}
		taskContext["expected_sizes"], _ = json.Marshal(expectedSizes)
	}
	if _, adjustment := taskContext["order_adjustment"]; adjustment {
		var orderAdjustment struct {
			AdjustmentIssueID string `json:"adjustment_issue_id"`
			SourceRevision    int    `json:"source_revision"`
			TargetSize        string `json:"target_size"`
			SourceAssetID     string `json:"source_asset_id"`
			SourceAttachment  string `json:"source_attachment_id"`
			Request           string `json:"request"`
		}
		if json.Unmarshal(taskContext["order_adjustment"], &orderAdjustment) != nil ||
			strings.TrimSpace(orderAdjustment.AdjustmentIssueID) == "" || strings.TrimSpace(orderAdjustment.SourceAssetID) == "" ||
			strings.TrimSpace(orderAdjustment.SourceAttachment) == "" || strings.TrimSpace(orderAdjustment.Request) == "" ||
			orderAdjustment.SourceRevision != revision-1 || !validCreativeAssetSize(strings.TrimSpace(orderAdjustment.TargetSize)) ||
			strings.TrimSpace(suppliedIssueID) != strings.TrimSpace(orderAdjustment.AdjustmentIssueID) {
			return item, errors.New("manual creative production order adjustment context is invalid")
		}
		taskContext["scope"], _ = json.Marshal("size")
	} else {
		taskContext["scope"], _ = json.Marshal("variant")
	}
	taskContext["subject_id"], _ = json.Marshal(canonicalVariantID)
	encoded, err := json.Marshal(taskContext)
	if err != nil {
		return item, errors.New("failed to normalize manual creative production task context")
	}
	return service.DirectTaskFanoutItem{ItemKey: canonicalItemKey, Context: encoded}, nil
}

func jsonPositiveInt(raw json.RawMessage) (int, bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false, nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, true, err
	}
	if value < 0 || value != float64(int(value)) {
		return 0, true, errors.New("not a positive integer")
	}
	return int(value), true, nil
}

func jsonArrayLength(raw json.RawMessage) int {
	var values []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return 0
	}
	return len(values)
}

func creativeTaskRequiredCapability(kind string) string {
	switch kind {
	case "creative_crawl_run_analysis":
		return "reference_analysis"
	case "creative_order_item_plan":
		return "generation_plan"
	case "creative_order_item_production":
		return "image_edit"
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
		if kind == "creative_order_item_production" {
			if err := validateCreativeProductionTaskContext(context, item.ItemKey); err != nil {
				return err
			}
		}
		if kind == "creative_order_item_direct_edit" {
			if err := validateCreativeDirectEditTaskContext(context, item.ItemKey); err != nil {
				return err
			}
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

func validateCreativeProductionTaskContext(context map[string]any, itemKey string) error {
	variantID, _ := context["variant_id"].(string)
	if _, err := uuid.Parse(strings.TrimSpace(variantID)); err != nil {
		return errors.New("creative production task context variant_id must be a UUID")
	}
	revision, ok := context["revision"].(float64)
	if !ok || revision < 1 || revision != float64(int(revision)) {
		return errors.New("creative production task context revision must be a positive integer")
	}
	expectedSizes, ok := context["expected_sizes"].([]any)
	if !ok || len(expectedSizes) == 0 {
		return errors.New("creative production task context expected_sizes must be a non-empty array")
	}
	seen := make(map[string]struct{}, len(expectedSizes))
	for _, rawSize := range expectedSizes {
		size, ok := rawSize.(string)
		size = strings.TrimSpace(size)
		if !ok || !validCreativeAssetSize(size) {
			return errors.New("creative production task context expected_sizes contains an invalid size")
		}
		if _, duplicate := seen[size]; duplicate {
			return errors.New("creative production task context expected_sizes must not contain duplicates")
		}
		seen[size] = struct{}{}
	}
	wantItemKey := fmt.Sprintf("%s:r%d", strings.TrimSpace(variantID), int(revision))
	if itemKey != wantItemKey {
		return fmt.Errorf("creative production task item_key must be %s", wantItemKey)
	}
	if rawAdjustment, ok := context["order_adjustment"]; ok {
		adjustment, ok := rawAdjustment.(map[string]any)
		if !ok {
			return errors.New("creative production task context order_adjustment must be an object")
		}
		adjustmentIssueID, _ := adjustment["adjustment_issue_id"].(string)
		sourceAssetID, _ := adjustment["source_asset_id"].(string)
		sourceAttachmentID, _ := adjustment["source_attachment_id"].(string)
		targetSize, _ := adjustment["target_size"].(string)
		request, _ := adjustment["request"].(string)
		sourceRevision, sourceRevisionOK := adjustment["source_revision"].(float64)
		issueID, _ := context["issue_id"].(string)
		scope, _ := context["scope"].(string)
		if _, err := uuid.Parse(strings.TrimSpace(adjustmentIssueID)); err != nil {
			return errors.New("creative production task context order_adjustment is invalid")
		}
		if _, err := uuid.Parse(strings.TrimSpace(sourceAssetID)); err != nil {
			return errors.New("creative production task context order_adjustment is invalid")
		}
		if _, err := uuid.Parse(strings.TrimSpace(sourceAttachmentID)); err != nil {
			return errors.New("creative production task context order_adjustment is invalid")
		}
		if !validCreativeAssetSize(strings.TrimSpace(targetSize)) || strings.TrimSpace(request) == "" ||
			!sourceRevisionOK || int(sourceRevision) != int(revision)-1 ||
			issueID != adjustmentIssueID || scope != "size" {
			return errors.New("creative production task context order_adjustment is invalid")
		}
	}
	return nil
}

func validateCreativeDirectEditTaskContext(context map[string]any, itemKey string) error {
	variantID, _ := context["variant_id"].(string)
	if _, err := uuid.Parse(strings.TrimSpace(variantID)); err != nil {
		return errors.New("creative direct-edit task context variant_id must be a UUID")
	}
	revision, ok := context["revision"].(float64)
	if !ok || revision < 1 || revision != float64(int(revision)) {
		return errors.New("creative direct-edit task context revision must be a positive integer")
	}
	expectedSizes, ok := context["expected_sizes"].([]any)
	if !ok || len(expectedSizes) == 0 {
		return errors.New("creative direct-edit task context expected_sizes must be a non-empty array")
	}
	seen := make(map[string]struct{}, len(expectedSizes))
	for _, rawSize := range expectedSizes {
		size, ok := rawSize.(string)
		size = strings.TrimSpace(size)
		if !ok || !validCreativeAssetSize(size) {
			return errors.New("creative direct-edit task context expected_sizes contains an invalid size")
		}
		if _, duplicate := seen[size]; duplicate {
			return errors.New("creative direct-edit task context expected_sizes must not contain duplicates")
		}
		seen[size] = struct{}{}
	}
	wantItemKey := fmt.Sprintf("%s:r%d", strings.TrimSpace(variantID), int(revision))
	if itemKey != wantItemKey {
		return fmt.Errorf("creative direct-edit task item_key must be %s", wantItemKey)
	}
	targetSize, _ := context["target_size"].(string)
	targetSize = strings.TrimSpace(targetSize)
	if !validCreativeAssetSize(targetSize) {
		return errors.New("creative direct-edit task context target_size is invalid")
	}
	if _, ok := seen[targetSize]; !ok {
		return errors.New("creative direct-edit task context target_size is not expected")
	}
	request, _ := context["user_request"].(string)
	if strings.TrimSpace(request) == "" {
		return errors.New("creative direct-edit task context user_request is required")
	}
	deliveryMode, _ := context["delivery_mode"].(string)
	if deliveryMode != "preview" && deliveryMode != "publish" {
		return errors.New("creative direct-edit task context delivery_mode is invalid")
	}
	if finalVisualValidation, ok := context["final_visual_validation"].(bool); deliveryMode == "publish" && (!ok || !finalVisualValidation) {
		return errors.New("creative direct-edit publish task requires final_visual_validation")
	}
	sourceRevision, ok := context["source_revision"].(float64)
	if !ok || sourceRevision < 1 || sourceRevision != float64(int(sourceRevision)) || int(sourceRevision) != int(revision)-1 {
		return errors.New("creative direct-edit task context source_revision is invalid")
	}
	for _, field := range []string{"source_asset_id", "source_attachment_id", "reviewer_agent_id"} {
		value, _ := context[field].(string)
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return fmt.Errorf("creative direct-edit task context %s must be a UUID", field)
		}
	}
	if value, ok := context["reference_asset_id"].(string); ok && strings.TrimSpace(value) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return errors.New("creative direct-edit task context reference_asset_id must be a UUID")
		}
	}
	if value, ok := context["reference_attachment_id"].(string); ok && strings.TrimSpace(value) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(value)); err != nil {
			return errors.New("creative direct-edit task context reference_attachment_id must be a UUID")
		}
	}
	annotationGuideAttachmentID, _ := context["annotation_guide_attachment_id"].(string)
	if strings.TrimSpace(annotationGuideAttachmentID) != "" {
		if _, err := uuid.Parse(strings.TrimSpace(annotationGuideAttachmentID)); err != nil {
			return errors.New("creative direct-edit task context annotation_guide_attachment_id must be a UUID")
		}
	}
	if rawDirectEdit, ok := context["direct_edit"]; ok {
		directEdit, ok := rawDirectEdit.(map[string]any)
		if !ok {
			return errors.New("creative direct-edit task context direct_edit must be an object")
		}
		if issueID, _ := directEdit["adjustment_issue_id"].(string); strings.TrimSpace(issueID) != "" {
			contextIssueID, _ := context["issue_id"].(string)
			if _, err := uuid.Parse(strings.TrimSpace(issueID)); err != nil || strings.TrimSpace(issueID) != strings.TrimSpace(contextIssueID) {
				return errors.New("creative direct-edit task context direct_edit adjustment_issue_id is invalid")
			}
		}
		if guideID, _ := directEdit["annotation_guide_attachment_id"].(string); strings.TrimSpace(guideID) != "" && strings.TrimSpace(guideID) != strings.TrimSpace(annotationGuideAttachmentID) {
			return errors.New("creative direct-edit task context annotation guide does not match direct_edit")
		}
		if rawAnnotations, ok := directEdit["annotations"].([]any); ok && len(rawAnnotations) > 0 && strings.TrimSpace(annotationGuideAttachmentID) == "" {
			return errors.New("creative direct-edit task context annotations require annotation_guide_attachment_id")
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
	if rawAttempt, exists := context["qc_attempt"]; exists {
		attempt, ok := rawAttempt.(float64)
		if !ok || attempt < 1 || attempt != float64(int(attempt)) {
			return errors.New("creative QC task context qc_attempt must be a positive integer")
		}
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
	case "creative_variant", "creative_order_variant_qc":
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
	if !strings.HasPrefix(kind, "creative_") {
		retried, err := h.TaskService.RetryFailedDirectTasksByEvidence(r.Context(), agent.ID, kind, refID)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, taskFanoutResponse{Tasks: h.directTaskResponses(r, retried, uuidToString(agent.WorkspaceID))})
		return
	}
	priorTasks, err := h.TaskService.ListDirectTasksByEvidence(r.Context(), agent.ID, kind, refID, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative tasks for retry")
		return
	}
	scopeItems := make([]service.DirectTaskFanoutItem, 0, len(priorTasks))
	for _, task := range priorTasks {
		scopeItems = append(scopeItems, service.DirectTaskFanoutItem{Context: task.Context})
	}
	var actorTaskID, actorAgentID pgtype.UUID
	if r.Header.Get("X-Actor-Source") == "task_token" {
		actorTaskID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
		if !ok {
			return
		}
		actorAgentID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Agent-ID"), "agent_id")
		if !ok {
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative task retry")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lockCreativeFanoutFence(r.Context(), tx, agent.WorkspaceID, kind, refID, scopeItems, actorTaskID, actorAgentID); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	retried, err := h.TaskService.RetryFailedDirectTasksByEvidenceTx(r.Context(), tx, agent.ID, kind, refID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit creative task retry")
		return
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(r.Context(), retried)
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
