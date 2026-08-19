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
	if kind != "creative_order_item_production" && kind != "creative_order_variant_qc" && kind != "manual" {
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
	var triggerKind, inputSnapshot, brief string
	var err error
	if kind == "creative_order_item_production" || kind == "manual" {
		err = q.QueryRow(ctx, `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1
  AND item.id = $2
  AND order_row.workspace_id = $3
`, variantID, evidenceRefID, workspaceID).Scan(&triggerKind, &inputSnapshot, &brief)
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
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
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
		taskContext["expected_sizes"], _ = json.Marshal(standardCreativeAssetSizes)
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
		taskContext["expected_sizes"], _ = json.Marshal(standardCreativeAssetSizes)
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
