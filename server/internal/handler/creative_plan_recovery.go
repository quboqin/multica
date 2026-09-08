package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (h *Handler) queueMissingCreativePlan(ctx context.Context, t creativeRecoveryTarget) (pgtype.UUID, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, err
	}
	defer tx.Rollback(ctx)
	var snapshot, orderStatus, itemStatus, sourceKind, candidate, analysis, library string
	var createdBy, issueID pgtype.UUID
	if err := tx.QueryRow(ctx, `SELECT o.input_snapshot::text,o.status,i.status,o.created_by,o.issue_id,i.source_kind,
 COALESCE(i.candidate_id::text,''),COALESCE(i.source_analysis_id::text,''),COALESCE(i.copy_library_id::text,'')
FROM creative_order o JOIN creative_order_item i ON i.order_id=o.id
WHERE o.id=$1 AND o.workspace_id=$2 AND i.id=$3 FOR UPDATE OF o,i`, t.OrderID, t.WorkspaceID, t.ItemID).Scan(&snapshot, &orderStatus, &itemStatus, &createdBy, &issueID, &sourceKind, &candidate, &analysis, &library); err != nil {
		return pgtype.UUID{}, err
	}
	if orderStatus == "cancelled" || itemStatus == "cancelled" {
		return pgtype.UUID{}, errors.New("creative plan target is cancelled")
	}
	var existing pgtype.UUID
	err = tx.QueryRow(ctx, `SELECT task_id FROM creative_task_binding WHERE order_item_id=$1 AND workflow='creative_plan' ORDER BY created_at DESC,task_id DESC LIMIT 1`, t.ItemID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, err
	}
	var frozen struct {
		Squad struct {
			Planner  string `json:"planner_agent_id"`
			Leader   string `json:"leader_agent_id"`
			Producer string `json:"producer_agent_id"`
			Reviewer string `json:"reviewer_agent_id"`
		} `json:"squad_snapshot"`
	}
	if json.Unmarshal([]byte(snapshot), &frozen) != nil {
		return pgtype.UUID{}, errors.New("invalid planning snapshot")
	}
	plannerID, err := parseCreativeOrchestrationUUID(frozen.Squad.Planner)
	if err != nil {
		return pgtype.UUID{}, errors.New("frozen planner is unavailable")
	}
	planner, err := h.Queries.WithTx(tx).GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: plannerID, WorkspaceID: t.WorkspaceID})
	if err != nil {
		return pgtype.UUID{}, err
	}
	var capable bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_skill b JOIN skill s ON s.id=b.skill_id WHERE b.agent_id=$1 AND b.enabled AND s.workspace_id=$2 AND s.config->>'capability'='generation_plan')`, plannerID, t.WorkspaceID).Scan(&capable); err != nil {
		return pgtype.UUID{}, err
	}
	if !capable {
		return pgtype.UUID{}, errors.New("frozen planner has no generation_plan capability")
	}
	fields, err := creativeProductionSourceFields(sourceKind, candidate, analysis, library)
	if err != nil {
		return pgtype.UUID{}, err
	}
	key := fmt.Sprintf("%s:r1", uuidToString(t.ItemID))
	payload := map[string]any{"type": "creative_domain_task", "workflow": "creative_plan", "scope": "order_item", "subject_id": uuidToString(t.ItemID), "item_key": key,
		"creative_order_id": uuidToString(t.OrderID), "creative_order_item_id": uuidToString(t.ItemID), "revision": 1, "issue_id": uuidToString(issueID),
		"planner_agent_id": frozen.Squad.Planner, "leader_agent_id": frozen.Squad.Leader, "producer_agent_id": frozen.Squad.Producer, "reviewer_agent_id": frozen.Squad.Reviewer}
	for k, v := range fields {
		payload[k] = v
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return pgtype.UUID{}, err
	}
	items := []service.DirectTaskFanoutItem{{ItemKey: key, Context: encoded}}
	if err := validateCreativeTaskFanoutContext("creative_order_item_plan", t.ItemID, items); err != nil {
		return pgtype.UUID{}, err
	}
	tasks, created, err := h.TaskService.EnqueueDirectTaskFanoutTx(ctx, tx, service.DirectTaskFanout{Agent: planner, RequestingUserID: createdBy,
		Attribution: attribution.DirectHumanRun(createdBy, attribution.EvidenceKind("creative_order_item_plan"), t.ItemID), TriggerEvidenceKind: "creative_order_item_plan", TriggerEvidenceRefID: t.ItemID, Items: items})
	if err != nil {
		return pgtype.UUID{}, err
	}
	if len(tasks) != 1 {
		return pgtype.UUID{}, errors.New("planning dispatch returned no task")
	}
	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, err
	}
	h.TaskService.NotifyDirectTaskFanoutEnqueued(ctx, created)
	return tasks[0].ID, nil
}
