package handler

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

func loadCreativeCandidateProgressBatch(ctx context.Context, q dbExecutor, orderID pgtype.UUID, itemIDs []pgtype.UUID) (map[string]*creativeCandidateProgress, error) {
	result := make(map[string]*creativeCandidateProgress, len(itemIDs))
	if len(itemIDs) == 0 {
		return result, nil
	}
	var snapshot, orderStatus string
	if err := q.QueryRow(ctx, `SELECT input_snapshot::text,status FROM creative_order WHERE id=$1`, orderID).Scan(&snapshot, &orderStatus); err != nil {
		return nil, err
	}
	if creativeOrderPipelineVersion(json.RawMessage(snapshot)) != creativePipelineCandidateV1 {
		return result, nil
	}
	counts, err := creativeOrderVariantCounts(json.RawMessage(snapshot))
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `
WITH scoped_tasks AS (
 SELECT b.*,t.status FROM creative_task_binding b JOIN agent_task_queue t ON t.id=b.task_id
 WHERE b.order_id=$1 AND b.order_item_id=ANY($2::uuid[])
), production AS (
 SELECT variant_id,revision,bool_or(status IN ('queued','dispatched','running','waiting_local_directory')) AS active,
 (array_agg(status ORDER BY created_at DESC,task_id DESC))[1] AS status
 FROM scoped_tasks WHERE workflow='creative_production' GROUP BY variant_id,revision
), latest AS (
 SELECT DISTINCT ON (order_item_id,workflow) order_item_id,workflow,task_id,status
 FROM scoped_tasks WHERE workflow IN ('creative_plan','creative_candidate_selection')
 ORDER BY order_item_id,workflow,created_at DESC,task_id DESC
), candidates AS (
 SELECT v.order_item_id,v.candidate_state,v.status,
 EXISTS(SELECT 1 FROM creative_order_asset a WHERE a.variant_id=v.id AND a.revision=v.revision AND a.size_key=v.primary_size AND a.stage='generated' AND a.status='completed' AND a.attachment_id IS NOT NULL) AS generated,
 EXISTS(SELECT 1 FROM creative_order_asset a WHERE a.variant_id=v.id AND a.revision=v.revision AND a.size_key=v.primary_size AND a.stage='primed' AND a.status='completed' AND a.attachment_id IS NOT NULL) AS primed,
 COALESCE(p.active,false) AS active,COALESCE(p.status,'') AS task_status,
 EXISTS(SELECT 1 FROM creative_prime_composition_job j WHERE j.variant_id=v.id AND j.revision=v.revision AND j.status IN ('queued','running')) AS prime_active
 FROM creative_order_variant v LEFT JOIN production p ON p.variant_id=v.id AND p.revision=v.revision
 WHERE v.order_item_id=ANY($2::uuid[])
), totals AS (
 SELECT order_item_id,count(*) AS planned,count(*) FILTER(WHERE generated) AS generated,count(*) FILTER(WHERE primed) AS primed,
 count(*) FILTER(WHERE candidate_state='rejected' OR (NOT active AND NOT prime_active AND task_status IN ('completed','failed'))) AS settled,
 count(*) FILTER(WHERE candidate_state='selected') AS selected,count(*) FILTER(WHERE active) AS active_production,
 count(*) FILTER(WHERE prime_active) AS active_prime,count(*) FILTER(WHERE status='cancelled' OR task_status='cancelled') AS cancelled,
 count(*) FILTER(WHERE task_status<>'') AS production_tasks,
 count(*) FILTER(WHERE candidate_state='candidate' AND generated AND primed AND NOT active AND NOT prime_active AND task_status IN ('completed','failed')) AS ready
 FROM candidates GROUP BY order_item_id
)
SELECT i.id::text,i.source_kind,i.status,COALESCE(t.planned,0),COALESCE(t.generated,0),COALESCE(t.primed,0),COALESCE(t.settled,0),
 COALESCE(t.selected,0),COALESCE(t.active_production,0),COALESCE(t.active_prime,0),COALESCE(t.cancelled,0),COALESCE(t.production_tasks,0),COALESCE(t.ready,0),
 COALESCE(p.task_id::text,''),COALESCE(p.status,''),COALESCE(s.task_id::text,''),COALESCE(s.status,'')
FROM creative_order_item i LEFT JOIN totals t ON t.order_item_id=i.id
LEFT JOIN latest p ON p.order_item_id=i.id AND p.workflow='creative_plan'
LEFT JOIN latest s ON s.order_item_id=i.id AND s.workflow='creative_candidate_selection'
WHERE i.order_id=$1 AND i.id=ANY($2::uuid[])
`, orderID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, sourceKind, itemStatus string
		p := &creativeCandidateProgress{Target: counts.Target, Expected: counts.Candidates}
		if err := rows.Scan(&id, &sourceKind, &itemStatus, &p.Planned, &p.Generated, &p.Primed, &p.Settled, &p.selected, &p.activeProduction, &p.activePrime, &p.cancelled, &p.productionTasks, &p.ready, &p.PlanTaskID, &p.PlanStatus, &p.SelectionTaskID, &p.SelectionStatus); err != nil {
			return nil, err
		}
		if p.selected > 0 {
			continue
		}
		if sourceKind != "copy_library" || !counts.Configured {
			p.Expected = max(counts.Target, p.Planned)
		}
		switch {
		case orderStatus == "cancelled" || itemStatus == "cancelled" || p.cancelled > 0 || p.PlanStatus == "cancelled" || p.SelectionStatus == "cancelled":
			p.State = "cancelled"
		case p.Planned < p.Expected || p.productionTasks < p.Planned:
			p.State = "planning_incomplete"
		case p.Planned > counts.Candidates:
			p.State = "planning_invalid"
		case p.activeProduction > 0 && p.Generated < p.Planned:
			p.State = "generating"
		case p.activePrime > 0:
			p.State = "priming"
		case p.activeProduction > 0:
			p.State = "settling"
		case p.Generated < p.Target || p.Primed < p.Target:
			p.State = "primary_incomplete"
		case p.Settled < p.Planned:
			p.State = "settling"
		case p.ready < p.Target:
			p.State = "primary_incomplete"
		case p.SelectionStatus == "queued" || p.SelectionStatus == "dispatched" || p.SelectionStatus == "waiting_local_directory":
			p.State = "selection_queued"
		case p.SelectionStatus == "running":
			p.State = "selecting"
		case p.SelectionStatus == "failed" || p.SelectionStatus == "completed":
			p.State = "selection_incomplete"
		default:
			p.State = "selection_ready"
		}
		result[id] = p
	}
	return result, rows.Err()
}
