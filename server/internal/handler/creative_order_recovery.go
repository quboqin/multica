package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/capability"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"github.com/multica-ai/multica/server/pkg/taskfailure"
)

type creativeRecoveryTarget struct {
	ID, WorkspaceID, OrderID, ItemID, VariantID, SourceTaskID, LeaseToken pgtype.UUID
	Revision, Attempt                                                     int
	Stage, Size, Reason, Status                                           string
}

// RecoverCreativeOrders reconciles bounded order scopes. Model requests are
// never issued here: existing domain dispatchers retain their idempotency gates.
func (h *Handler) RecoverCreativeOrders(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, nil
	}
	limit = min(limit, 50)
	rows, err := h.DB.Query(ctx, `UPDATE creative_order_recovery_scan s SET checked_at=now(),next_check_at=now()+interval '2 minutes'
 FROM (SELECT s.order_id FROM creative_order_recovery_scan s JOIN creative_order o ON o.id=s.order_id
 WHERE s.next_check_at<=now()
 AND EXISTS(SELECT 1 FROM workspace_capability c WHERE c.workspace_id=o.workspace_id AND c.capability_key='creative_factory' AND c.enabled)
 ORDER BY s.next_check_at,s.order_id
 LIMIT $1 FOR UPDATE OF s SKIP LOCKED) due WHERE s.order_id=due.order_id RETURNING s.order_id`, limit)
	if err != nil {
		return 0, err
	}
	var ids []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	var failures []error
	for _, id := range ids {
		scanErr := h.discoverCreativeOrderRecovery(ctx, id)
		message := ""
		if scanErr != nil {
			message = scanErr.Error()
			failures = append(failures, scanErr)
		}
		if _, err := h.DB.Exec(ctx, `UPDATE creative_order_recovery_scan s SET last_error=$2,
next_check_at=CASE WHEN $2='' AND EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id=v.order_item_id WHERE i.order_id=s.order_id)
AND NOT EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id=v.order_item_id WHERE i.order_id=s.order_id AND i.status<>'cancelled' AND v.status<>'cancelled' AND v.candidate_state IN ('candidate','selected') AND v.revision IS DISTINCT FROM v.active_revision)
THEN now()+interval '1 day' ELSE next_check_at END WHERE order_id=$1`, id, message); err != nil {
			failures = append(failures, err)
		}
	}
	if err := h.expireCreativeRecoveryLeases(ctx); err != nil {
		return 0, err
	}
	processed := 0
	for range limit {
		target, found, err := h.claimCreativeRecovery(ctx)
		if err != nil {
			failures = append(failures, err)
			break
		}
		if !found {
			break
		}
		if err := h.processCreativeRecovery(ctx, target); err != nil {
			failures = append(failures, err)
		}
		processed++
	}
	return processed, errors.Join(failures...)
}

func (h *Handler) discoverCreativeOrderRecovery(ctx context.Context, orderID pgtype.UUID) error {
	if err := h.refreshCreativeRecoveryResults(ctx, orderID); err != nil {
		return err
	}
	var workspaceID pgtype.UUID
	var status, snapshot, triggerKind string
	if err := h.DB.QueryRow(ctx, `SELECT workspace_id,status,input_snapshot::text,trigger_evidence_kind FROM creative_order WHERE id=$1`, orderID).Scan(&workspaceID, &status, &snapshot, &triggerKind); err != nil {
		return err
	}
	if status == "cancelled" {
		return nil
	}
	rows, err := h.DB.Query(ctx, `SELECT id FROM creative_order_item WHERE order_id=$1 AND status<>'cancelled' ORDER BY created_at,id`, orderID)
	if err != nil {
		return err
	}
	var items []pgtype.UUID
	for rows.Next() {
		var id pgtype.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		items = append(items, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	progress, err := loadCreativeCandidateProgressBatch(ctx, h.DB, orderID, items)
	if err != nil {
		return err
	}
	for _, itemID := range items {
		p := progress[uuidToString(itemID)]
		if p == nil || p.State != "planning_incomplete" {
			if _, err := h.DB.Exec(ctx, `UPDATE creative_recovery SET status='resolved',resolved_at=now(),updated_at=now() WHERE order_item_id=$1 AND stage='planning' AND status NOT IN ('running','resolved','cancelled')`, itemID); err != nil {
				return err
			}
		}
		base := creativeRecoveryTarget{WorkspaceID: workspaceID, OrderID: orderID, ItemID: itemID}
		if p != nil {
			switch p.State {
			case "planning_incomplete":
				if p.PlanStatus == "completed" || p.PlanStatus == "failed" || p.PlanTaskID == "" {
					t := base
					t.Stage = "planning"
					t.Reason = "candidate_plan_incomplete"
					if p.PlanTaskID != "" {
						t.SourceTaskID = parseUUID(p.PlanTaskID)
					} else {
						t.Reason = "planning_task_missing"
					}
					if err := h.recordCreativeRecovery(ctx, t); err != nil {
						return err
					}
				}
			case "selection_ready", "selection_incomplete":
				t := base
				t.Stage = "candidate_selection"
				t.Reason = "candidate_selection_missing"
				if p.SelectionTaskID != "" {
					t.SourceTaskID = parseUUID(p.SelectionTaskID)
				}
				if err := h.recordCreativeRecovery(ctx, t); err != nil {
					return err
				}
			}
		}
		variants, err := h.DB.Query(ctx, `SELECT v.id,v.revision,v.candidate_state,v.primary_size,v.brief::text
 FROM creative_order_variant v WHERE v.order_item_id=$1 AND v.status<>'cancelled'
 AND v.candidate_state IN ('candidate','selected') AND v.revision IS DISTINCT FROM v.active_revision
 ORDER BY v.variant_key`, itemID)
		if err != nil {
			return err
		}
		type variantScope struct {
			id                    pgtype.UUID
			revision              int
			state, primary, brief string
		}
		var scopes []variantScope
		for variants.Next() {
			var v variantScope
			if err := variants.Scan(&v.id, &v.revision, &v.state, &v.primary, &v.brief); err != nil {
				variants.Close()
				return err
			}
			scopes = append(scopes, v)
		}
		err = variants.Err()
		variants.Close()
		if err != nil {
			return err
		}
		for _, v := range scopes {
			expected, err := expectedCreativeVariantProductionSizes(triggerKind, []byte(snapshot), []byte(v.brief), v.state, v.primary)
			if err != nil {
				return err
			}
			for _, size := range expected {
				t := base
				t.VariantID = v.id
				t.Revision = v.revision
				t.Size = size
				var generated, primed, delivered bool
				if err := h.DB.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM creative_order_asset WHERE variant_id=$1 AND revision=$2 AND size_key=$3 AND stage='generated' AND status='completed' AND attachment_id IS NOT NULL),
 EXISTS(SELECT 1 FROM creative_order_asset WHERE variant_id=$1 AND revision=$2 AND size_key=$3 AND stage='primed' AND status='completed' AND attachment_id IS NOT NULL),
 EXISTS(SELECT 1 FROM creative_order_asset WHERE variant_id=$1 AND revision=$2 AND size_key=$3 AND stage='delivered' AND status='completed' AND attachment_id IS NOT NULL)`, v.id, v.revision, size).Scan(&generated, &primed, &delivered); err != nil {
					return err
				}
				switch {
				case !generated:
					if triggerKind == "creative_direct_edit" {
						continue
					}
					t.Stage = "production"
					t.Reason = "generated_size_missing"
				case !primed:
					t.Stage = "prime"
					t.Reason = "prime_handoff_missing"
				case !delivered && v.state == "selected":
					t.Stage = "qc"
					t.Reason = "qc_handoff_incomplete"
				default:
					continue
				}
				workflow := "creative_production"
				if t.Stage == "qc" {
					workflow = "creative_qc_visual"
				}
				err := h.DB.QueryRow(ctx, `SELECT b.task_id FROM creative_task_binding b WHERE b.variant_id=$1 AND b.revision=$2 AND b.workflow=$3 ORDER BY b.created_at DESC,b.task_id DESC LIMIT 1`, v.id, v.revision, workflow).Scan(&t.SourceTaskID)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return err
				}
				if err := h.recordCreativeRecovery(ctx, t); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (h *Handler) recordCreativeRecovery(ctx context.Context, t creativeRecoveryTarget) error {
	disposition, err := h.creativeRecoveryDisposition(ctx, t)
	if err != nil {
		return err
	}
	if disposition == "waiting" || disposition == "cancelled" || disposition == "resolved" {
		return nil
	}
	key := fmt.Sprintf("%s:%s:%d:%s:%s", uuidToString(t.ItemID), uuidToString(t.VariantID), t.Revision, t.Stage, t.Size)
	_, err = h.DB.Exec(ctx, `INSERT INTO creative_recovery(workspace_id,order_id,order_item_id,variant_id,revision,stage,size_key,recovery_key,reason_code,source_task_id)
 VALUES($1,$2,$3,$4,NULLIF($5,0),$6,$7,$8,$9,$10) ON CONFLICT(recovery_key) DO NOTHING`, t.WorkspaceID, t.OrderID, t.ItemID, t.VariantID, t.Revision, t.Stage, t.Size, key, t.Reason, t.SourceTaskID)
	return err
}

func (h *Handler) expireCreativeRecoveryLeases(ctx context.Context) error {
	_, err := h.DB.Exec(ctx, `WITH expired AS (
 UPDATE creative_recovery SET status=CASE WHEN dispatch_count>=max_attempts THEN 'manual_required' ELSE 'pending' END,
 lease_token=NULL,lease_expires_at=NULL,next_retry_at=now()+interval '1 minute',last_error='recovery lease expired',updated_at=now()
 WHERE status='running' AND lease_expires_at<=now() RETURNING id
 ) UPDATE creative_recovery_attempt a SET status='failed',error_message='recovery lease expired',completed_at=now()
 FROM expired WHERE a.recovery_id=expired.id AND a.status='running'`)
	return err
}

func (h *Handler) claimCreativeRecovery(ctx context.Context, orderIDs ...pgtype.UUID) (creativeRecoveryTarget, bool, error) {
	var orderID pgtype.UUID
	if len(orderIDs) > 0 {
		orderID = orderIDs[0]
	}
	var t creativeRecoveryTarget
	err := h.DB.QueryRow(ctx, `WITH due AS (
 SELECT id FROM creative_recovery WHERE status IN ('pending','waiting') AND next_retry_at<=now()
 AND ($2::uuid IS NULL OR order_id=$2)
 ORDER BY next_retry_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
 ) UPDATE creative_recovery r SET status='running',lease_token=$1,lease_expires_at=now()+interval '2 minutes',updated_at=now()
 FROM due WHERE r.id=due.id RETURNING r.id,r.workspace_id,r.order_id,r.order_item_id,r.variant_id,COALESCE(r.revision,0),
 r.stage,r.size_key,r.reason_code,r.source_task_id,r.lease_token,r.attempt`, uuid.NewString(), orderID).Scan(&t.ID, &t.WorkspaceID, &t.OrderID, &t.ItemID, &t.VariantID, &t.Revision, &t.Stage, &t.Size, &t.Reason, &t.SourceTaskID, &t.LeaseToken, &t.Attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, false, nil
	}
	return t, err == nil, err
}

func (h *Handler) creativeRecoveryDisposition(ctx context.Context, t creativeRecoveryTarget) (string, error) {
	var orderStatus, itemStatus string
	if err := h.DB.QueryRow(ctx, `SELECT o.status,i.status FROM creative_order o JOIN creative_order_item i ON i.order_id=o.id WHERE o.id=$1 AND o.workspace_id=$2 AND i.id=$3`, t.OrderID, t.WorkspaceID, t.ItemID).Scan(&orderStatus, &itemStatus); err != nil {
		return "", err
	}
	if orderStatus == "cancelled" || itemStatus == "cancelled" {
		return "cancelled", nil
	}
	if !t.VariantID.Valid {
		p, err := loadCreativeCandidateProgress(ctx, h.DB, t.OrderID, t.ItemID)
		if err != nil {
			return "", err
		}
		if p == nil {
			return "resolved", nil
		}
		if p.State == "cancelled" {
			return "cancelled", nil
		}
		if t.Stage == "planning" && p.State != "planning_incomplete" {
			return "resolved", nil
		}
		if t.Stage == "candidate_selection" && p.State != "selection_ready" && p.State != "selection_incomplete" {
			return "waiting", nil
		}
		if t.Stage == "planning" && (p.PlanStatus == "queued" || p.PlanStatus == "dispatched" || p.PlanStatus == "running" || p.PlanStatus == "waiting_local_directory") {
			return "waiting", nil
		}
		if t.Stage == "planning" && p.PlanTaskID == "" {
			var leaderActive bool
			if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_task_queue q JOIN creative_order o ON o.issue_id=q.issue_id WHERE o.id=$1 AND q.status IN ('queued','dispatched','running','waiting_local_directory'))`, t.OrderID).Scan(&leaderActive); err != nil {
				return "", err
			}
			if leaderActive {
				return "waiting", nil
			}
		}
		return "pending", nil
	}
	var revision, activeRevision int
	var variantStatus, candidateState string
	if err := h.DB.QueryRow(ctx, `SELECT revision,COALESCE(active_revision,0),status,candidate_state FROM creative_order_variant WHERE id=$1 AND order_item_id=$2`, t.VariantID, t.ItemID).Scan(&revision, &activeRevision, &variantStatus, &candidateState); err != nil {
		return "", err
	}
	if revision != t.Revision || variantStatus == "cancelled" || candidateState == "reserve" || candidateState == "rejected" {
		return "cancelled", nil
	}
	if activeRevision == revision {
		return "resolved", nil
	}
	stage := "generated"
	if t.Stage == "prime" {
		stage = "primed"
	}
	if t.Stage == "qc" {
		stage = "delivered"
	}
	var complete, active, unknown bool
	if err := h.DB.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM creative_order_asset WHERE variant_id=$1 AND revision=$2 AND size_key=$3 AND stage=$4 AND status='completed' AND attachment_id IS NOT NULL),
 EXISTS(SELECT 1 FROM creative_task_binding b JOIN agent_task_queue q ON q.id=b.task_id WHERE b.variant_id=$1 AND b.revision=$2 AND q.status IN ('queued','dispatched','running','waiting_local_directory')),
 EXISTS(SELECT 1 FROM creative_image_operation WHERE variant_id=$1 AND revision=$2 AND status IN ('queued','running','unknown'))`, t.VariantID, t.Revision, t.Size, stage).Scan(&complete, &active, &unknown); err != nil {
		return "", err
	}
	if complete {
		return "resolved", nil
	}
	if active {
		return "waiting", nil
	}
	if unknown {
		var stale bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_image_operation
WHERE variant_id=$1 AND revision=$2 AND status IN ('queued','running','unknown')
AND updated_at<now()-interval '15 minutes')`, t.VariantID, t.Revision).Scan(&stale); err != nil {
			return "", err
		}
		if stale {
			return "manual_required", nil
		}
		return "waiting", nil
	}
	if t.Stage == "production" && candidateState == "candidate" {
		var planningActive bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_task_binding b JOIN agent_task_queue q ON q.id=b.task_id WHERE b.order_item_id=$1 AND b.workflow='creative_plan' AND q.status IN ('queued','dispatched','running','waiting_local_directory'))`, t.ItemID).Scan(&planningActive); err != nil {
			return "", err
		}
		if planningActive {
			return "waiting", nil
		}
	}
	if t.Stage == "prime" || t.Stage == "qc" {
		previousStage := "generated"
		if t.Stage == "qc" {
			previousStage = "primed"
		}
		var ready bool
		if err := h.DB.QueryRow(ctx, `SELECT (SELECT count(DISTINCT a.size_key) FROM creative_order_asset a
WHERE a.variant_id=r.variant_id AND a.revision=r.revision AND a.stage=$3 AND a.status='completed'
AND a.attachment_id IS NOT NULL AND a.size_key=ANY(r.expected_sizes))=cardinality(r.expected_sizes)
FROM creative_order_variant_revision r WHERE r.variant_id=$1 AND r.revision=$2`, t.VariantID, t.Revision, previousStage).Scan(&ready); err != nil {
			return "", err
		}
		if !ready {
			return "waiting", nil
		}
	}
	if t.Stage == "prime" {
		var jobStatus string
		err := h.DB.QueryRow(ctx, `SELECT status FROM creative_prime_composition_job WHERE variant_id=$1 AND revision=$2`, t.VariantID, t.Revision).Scan(&jobStatus)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if jobStatus == "queued" || jobStatus == "running" {
			return "waiting", nil
		}
		if jobStatus == "failed" || jobStatus == "cancelled" {
			return "manual_required", nil
		}
	}
	if t.Stage == "qc" {
		var resolved bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_order_variant_qc_resolution r WHERE r.variant_id=$1 AND r.revision=$2
AND r.attempt>=GREATEST(COALESCE((SELECT max(attempt) FROM creative_order_qc_report WHERE variant_id=$1 AND revision=$2),1),
COALESCE((SELECT max(COALESCE(qc_attempt,1)) FROM creative_task_binding WHERE variant_id=$1 AND revision=$2 AND workflow='creative_qc_visual'),1)))`, t.VariantID, t.Revision).Scan(&resolved); err != nil {
			return "", err
		}
		if resolved {
			return "manual_required", nil
		}
	}
	return "pending", nil
}

func (h *Handler) processCreativeRecovery(ctx context.Context, t creativeRecoveryTarget) error {
	enabled, err := capability.Enabled(ctx, h.DB, uuidToString(t.WorkspaceID), capability.CreativeFactory)
	if err != nil {
		return err
	}
	if !enabled {
		return h.finishCreativeRecovery(ctx, t, "waiting", pgtype.UUID{}, nil)
	}
	disposition, err := h.creativeRecoveryDisposition(ctx, t)
	if err != nil {
		return h.finishCreativeRecovery(ctx, t, "pending", pgtype.UUID{}, err)
	}
	if disposition != "pending" {
		if disposition == "manual_required" && t.Stage == "production" {
			return h.finishCreativeRecovery(ctx, t, disposition, pgtype.UUID{}, errors.New("image operation result is still unconfirmed after 15 minutes; reconcile its receipt before retrying"))
		}
		return h.finishCreativeRecovery(ctx, t, disposition, pgtype.UUID{}, nil)
	}
	workflow := "creative_production"
	if t.Stage == "planning" {
		workflow = "creative_plan"
	} else if t.Stage == "candidate_selection" {
		workflow = "creative_candidate_selection"
	} else if t.Stage == "qc" {
		workflow = "creative_qc_visual"
	}
	if latest, lookupErr := h.latestCreativeRecoveryTask(ctx, t, workflow); lookupErr == nil {
		t.SourceTaskID = latest
		task, loadErr := h.Queries.GetAgentTask(ctx, latest)
		if loadErr != nil {
			return h.finishCreativeRecovery(ctx, t, "pending", pgtype.UUID{}, loadErr)
		}
		if task.Status == "cancelled" {
			return h.finishCreativeRecovery(ctx, t, "cancelled", pgtype.UUID{}, nil)
		}
		reason := taskfailure.Reason(task.FailureReason.String)
		if reason == "" || reason == "agent_error" || reason == taskfailure.ReasonAgentUnknown {
			reason = taskfailure.Classify(task.Error.String)
		}
		switch reason {
		case taskfailure.ReasonAgentBlocked, taskfailure.ReasonAgentMissingConfig, taskfailure.ReasonAgentProviderAuthOrAccess, taskfailure.ReasonAgentProviderQuotaLimit:
			return h.finishCreativeRecovery(ctx, t, "manual_required", pgtype.UUID{}, fmt.Errorf("manual input required: %s", reason))
		}
	} else if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return h.finishCreativeRecovery(ctx, t, "pending", pgtype.UUID{}, lookupErr)
	}
	var remaining bool
	if err := h.DB.QueryRow(ctx, `SELECT dispatch_count<max_attempts FROM creative_recovery WHERE id=$1 AND status='running' AND lease_token=$2 AND lease_expires_at>now()`, t.ID, t.LeaseToken).Scan(&remaining); err != nil {
		return err
	}
	if !remaining {
		return h.finishCreativeRecovery(ctx, t, "manual_required", pgtype.UUID{}, nil)
	}
	ctx = context.WithValue(ctx, creativeRecoveryDispatchContextKey{}, t)
	taskID, err := h.executeCreativeRecovery(ctx, t)
	status := "waiting"
	if err != nil {
		status = "pending"
	}
	finishErr := h.finishCreativeRecovery(ctx, t, status, taskID, err)
	return errors.Join(err, finishErr)
}

func (h *Handler) finishCreativeRecovery(ctx context.Context, t creativeRecoveryTarget, status string, taskID pgtype.UUID, cause error) error {
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var assetID pgtype.UUID
	if status == "resolved" && t.VariantID.Valid && t.Size != "" {
		stage := "generated"
		if t.Stage == "prime" {
			stage = "primed"
		} else if t.Stage == "qc" {
			stage = "delivered"
		}
		err := tx.QueryRow(ctx, `SELECT id FROM creative_order_asset WHERE variant_id=$1 AND revision=$2 AND size_key=$3 AND stage=$4 AND status='completed' AND attachment_id IS NOT NULL`, t.VariantID, t.Revision, t.Size, stage).Scan(&assetID)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE creative_recovery SET status=CASE WHEN $3='pending' AND (dispatch_count>=max_attempts OR dispatch_failures+1>=3) THEN 'manual_required' ELSE $3 END,
 dispatch_failures=CASE WHEN $3='pending' THEN dispatch_failures+1 ELSE dispatch_failures END,
 result_task_id=COALESCE($4,result_task_id),last_error=$5,lease_token=NULL,lease_expires_at=NULL,
 resolved_asset_id=COALESCE($6,resolved_asset_id),resolved_at=CASE WHEN $3 IN ('resolved','cancelled') THEN now() ELSE resolved_at END,
 next_retry_at=now()+make_interval(secs=>LEAST(900,30*(1<<LEAST(dispatch_count+dispatch_failures,5)))),updated_at=now()
 WHERE id=$1 AND status='running' AND lease_token=$2 AND lease_expires_at>now()`, t.ID, t.LeaseToken, status, taskID, message, assetID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errors.New("creative recovery lease lost")
	}
	result := "queued"
	if cause != nil {
		result = "failed"
	} else if status == "resolved" {
		result = "resolved"
	} else if status == "cancelled" || status == "manual_required" {
		result = "cancelled"
	}
	if _, err := tx.Exec(ctx, `UPDATE creative_recovery_attempt SET status=$3,result_task_id=$4,error_message=$5,completed_at=now()
 WHERE recovery_id=$1 AND lease_token=$2 AND status='running'`, t.ID, t.LeaseToken, result, taskID, message); err != nil {
		return err
	}
	if assetID.Valid {
		if _, err := tx.Exec(ctx, `UPDATE creative_recovery_attempt SET result_asset_id=$3 WHERE recovery_id=$1 AND attempt=$2`, t.ID, t.Attempt, assetID); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if status != "waiting" || taskID.Valid || cause != nil {
		h.publish(protocol.EventCreativeMaterialsUpdated, uuidToString(t.WorkspaceID), "system", "", map[string]any{"scope": "order", "order_id": uuidToString(t.OrderID)})
	}
	return nil
}

func (h *Handler) executeCreativeRecovery(ctx context.Context, t creativeRecoveryTarget) (pgtype.UUID, error) {
	switch t.Stage {
	case "candidate_selection":
		queued, err := h.maybeQueueCreativeCandidateSelection(ctx, t.ItemID, creativeOrchestrationCause{})
		if err != nil {
			return pgtype.UUID{}, err
		}
		if !queued {
			return pgtype.UUID{}, errors.New("candidate selection is not eligible for another dispatch")
		}
		return h.latestCreativeRecoveryTask(ctx, t, "creative_candidate_selection")
	case "planning":
		if !t.SourceTaskID.Valid {
			return h.queueMissingCreativePlan(ctx, t)
		}
		return h.retryCreativeRecoveryTask(ctx, t)
	case "production":
		var state string
		if err := h.DB.QueryRow(ctx, `SELECT candidate_state FROM creative_order_variant WHERE id=$1 AND revision=$2`, t.VariantID, t.Revision).Scan(&state); err != nil {
			return pgtype.UUID{}, err
		}
		if state == "selected" {
			tasks, err := h.queueSelectedCreativeProductionTasks(ctx, t.ItemID, creativeOrchestrationCause{}, creativeSelectedExpansionPhase, t.VariantID)
			if err != nil {
				return pgtype.UUID{}, err
			}
			if len(tasks) > 0 {
				return tasks[0].ID, nil
			}
		}
		return h.retryCreativeRecoveryTask(ctx, t)
	case "prime":
		var restoreCompleted bool
		if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM creative_prime_composition_job WHERE variant_id=$1 AND revision=$2 AND status='completed')`, t.VariantID, t.Revision).Scan(&restoreCompleted); err != nil {
			return pgtype.UUID{}, err
		}
		_, _, complete, err := h.queueCreativePrimeComposition(ctx, t.WorkspaceID, t.OrderID, t.VariantID, restoreCompleted)
		if err == nil && !complete {
			err = errors.New("generated package is incomplete")
		}
		return pgtype.UUID{}, err
	case "qc":
		var requestedBy pgtype.UUID
		if err := h.DB.QueryRow(ctx, `SELECT created_by FROM creative_order WHERE id=$1 AND workspace_id=$2`, t.OrderID, t.WorkspaceID).Scan(&requestedBy); err != nil {
			return pgtype.UUID{}, err
		}
		queued, err := h.recoverPendingCreativeQCFinalization(ctx, t.VariantID)
		if err != nil {
			return pgtype.UUID{}, err
		}
		if !queued {
			if t.SourceTaskID.Valid {
				return pgtype.UUID{}, errors.New("QC recovery budget or eligibility exhausted")
			}
			if err := h.enqueueCreativeVariantQC(ctx, t.WorkspaceID, t.OrderID, t.VariantID, requestedBy, nil); err != nil {
				return pgtype.UUID{}, err
			}
		}
		return h.latestCreativeRecoveryTask(ctx, t, "creative_qc_visual")
	}
	return pgtype.UUID{}, errors.New("unsupported creative recovery stage")
}

func (h *Handler) latestCreativeRecoveryTask(ctx context.Context, t creativeRecoveryTarget, workflow string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := h.DB.QueryRow(ctx, `SELECT b.task_id FROM creative_task_binding b WHERE b.order_item_id=$1 AND b.workflow=$2
 AND ($3::uuid IS NULL OR (b.variant_id=$3 AND b.revision=$4)) ORDER BY b.created_at DESC,b.task_id DESC LIMIT 1`, t.ItemID, workflow, t.VariantID, t.Revision).Scan(&id)
	return id, err
}

func (h *Handler) retryCreativeRecoveryTask(ctx context.Context, t creativeRecoveryTarget) (pgtype.UUID, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return pgtype.UUID{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM creative_order WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, t.OrderID, t.WorkspaceID).Scan(&status); err != nil {
		return pgtype.UUID{}, err
	}
	if status == "cancelled" {
		return pgtype.UUID{}, errors.New("creative order cancelled")
	}
	var taskID pgtype.UUID
	workflow := "creative_production"
	if t.Stage == "planning" {
		workflow = "creative_plan"
	}
	err = tx.QueryRow(ctx, `SELECT b.task_id FROM creative_task_binding b WHERE b.order_item_id=$1 AND b.workflow=$2
 AND ($3::uuid IS NULL OR (b.variant_id=$3 AND b.revision=$4)) ORDER BY b.created_at DESC,b.task_id DESC LIMIT 1`, t.ItemID, workflow, t.VariantID, t.Revision).Scan(&taskID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("no recoverable source task: %w", err)
	}
	parent, err := h.Queries.WithTx(tx).GetAgentTask(ctx, taskID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if parent.Status == "queued" || parent.Status == "dispatched" || parent.Status == "running" || parent.Status == "waiting_local_directory" {
		return parent.ID, nil
	}
	if !isCreativeTaskTerminal(parent.Status) {
		return pgtype.UUID{}, errors.New("source task requires manual recovery")
	}
	if t.VariantID.Valid {
		var eligible bool
		if err := tx.QueryRow(ctx, `SELECT v.revision=$2 AND v.status<>'cancelled' AND i.status<>'cancelled'
 AND v.candidate_state IN ('candidate','selected') AND v.active_revision IS DISTINCT FROM v.revision
 AND NOT EXISTS(SELECT 1 FROM creative_image_operation p WHERE p.variant_id=v.id AND p.revision=v.revision AND p.status IN ('queued','running','unknown'))
 FROM creative_order_variant v JOIN creative_order_item i ON i.id=v.order_item_id WHERE v.id=$1 FOR UPDATE OF v,i`, t.VariantID, t.Revision).Scan(&eligible); err != nil {
			return pgtype.UUID{}, err
		}
		if !eligible {
			return pgtype.UUID{}, errors.New("variant no longer permits automatic recovery")
		}
	}
	// The recovery owns a bounded dispatch budget. Preserve the parent's
	// attribution and frozen inputs without reusing its exhausted queue limit.
	child, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID: parent.AgentID, RuntimeID: parent.RuntimeID, IssueID: parent.IssueID,
		Priority: parent.Priority, TriggerCommentID: parent.TriggerCommentID, TriggerSummary: parent.TriggerSummary,
		ForceFreshSession: pgtype.Bool{Bool: true, Valid: true}, IsLeaderTask: pgtype.Bool{Bool: parent.IsLeaderTask, Valid: true},
		RequestingUserID: parent.RequestingUserID, OriginatorUserID: parent.OriginatorUserID, AccountableUserID: parent.AccountableUserID,
		OriginatorSource: parent.OriginatorSource, DelegatedFromTaskID: parent.DelegatedFromTaskID,
		TriggerEvidenceKind: parent.TriggerEvidenceKind, TriggerEvidenceRefID: parent.TriggerEvidenceRefID,
		Context: parent.Context, RuntimeMcpOverlay: parent.RuntimeMcpOverlay, RuntimeConnectedApps: parent.RuntimeConnectedApps,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("recovery retry budget or eligibility: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE agent_task_queue SET parent_task_id=$2,retry_of_task_id=$2,attempt=$3,max_attempts=$3+1 WHERE id=$1`, child.ID, parent.ID, parent.Attempt+1); err != nil {
		return pgtype.UUID{}, err
	}
	if t.Stage == "production" {
		normalized, err := normalizeCreativeProductionFanoutItem(ctx, tx, t.WorkspaceID, t.ItemID, service.DirectTaskFanoutItem{Context: child.Context})
		if err != nil {
			return pgtype.UUID{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE agent_task_queue SET context=$2::jsonb WHERE id=$1`, child.ID, normalized.Context); err != nil {
			return pgtype.UUID{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE creative_order_variant SET status='running',updated_at=now() WHERE id=$1 AND revision=$2`, t.VariantID, t.Revision); err != nil {
			return pgtype.UUID{}, err
		}
	}
	if err := recordCreativeRecoveryDispatchTx(ctx, tx, child.ID); err != nil {
		return pgtype.UUID{}, err
	}
	child, err = h.Queries.WithTx(tx).GetAgentTask(ctx, child.ID)
	if err != nil {
		return pgtype.UUID{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return pgtype.UUID{}, err
	}
	h.TaskService.NotifyTaskEnqueued(ctx, child)
	return child.ID, nil
}
