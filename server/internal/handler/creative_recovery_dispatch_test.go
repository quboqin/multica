package handler

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreativeRecoveryDispatchMigrationPreservesRealAttempts(t *testing.T) {
	type sample struct {
		id         pgtype.UUID
		dispatched int
		status     string
	}
	var samples []sample
	for index, dispatched := range []int{0, 2, 3} {
		f := createCreativeCountFixture(t, index+1)
		parent := seedCandidatePlanTask(t, f, "failed")
		target := creativeRecoveryTarget{WorkspaceID: parseUUID(testWorkspaceID), OrderID: parseUUID(f.OrderID), ItemID: parseUUID(f.ItemID), Stage: "planning", Reason: "planning_task_missing"}
		if err := testHandler.recordCreativeRecovery(t.Context(), target); err != nil {
			t.Fatal(err)
		}
		var id pgtype.UUID
		if err := testPool.QueryRow(t.Context(), `UPDATE creative_recovery SET status='manual_required',attempt=3,last_error='recovery retry budget or eligibility: no rows in result set' WHERE order_id=$1 RETURNING id`, f.OrderID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 3; i++ {
			var task pgtype.UUID
			if i <= dispatched {
				task = parent.ID
			}
			if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_recovery_attempt(recovery_id,attempt,lease_token,status,result_task_id,reason_code) VALUES($1,$2,gen_random_uuid(),'failed',$3,'planning_task_missing')`, id, i, task); err != nil {
				t.Fatal(err)
			}
		}
		status := "pending"
		if dispatched == 3 {
			status = "manual_required"
		}
		samples = append(samples, sample{id, dispatched, status})
	}
	migration, err := os.ReadFile("../../migrations/284_creative_recovery_dispatch_budget.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `ALTER TABLE creative_recovery DROP COLUMN dispatch_count, DROP COLUMN dispatch_failures; ALTER TABLE creative_recovery ADD CONSTRAINT creative_recovery_old_limit CHECK(attempt<=max_attempts)`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), string(migration)); err != nil {
		t.Fatal(err)
	}
	for _, s := range samples {
		var status string
		var dispatched, history int
		if err := tx.QueryRow(t.Context(), `SELECT status,dispatch_count,(SELECT count(*) FROM creative_recovery_attempt WHERE recovery_id=r.id) FROM creative_recovery r WHERE id=$1`, s.id).Scan(&status, &dispatched, &history); err != nil {
			t.Fatal(err)
		}
		if status != s.status || dispatched != s.dispatched || history != 3 {
			t.Fatalf("migration: %s %d %d, want %+v", status, dispatched, history, s)
		}
		if s.status == "pending" {
			if _, err := tx.Exec(t.Context(), `UPDATE creative_recovery SET attempt=attempt+1 WHERE id=$1`, s.id); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCreativeRecoveryDispatchIgnoresExhaustedTaskBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 6)
	parent := seedCandidatePlanTask(t, f, "failed")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt=12,max_attempts=12 WHERE id=$1`, parent.ID); err != nil {
		t.Fatal(err)
	}
	runOrderRecoveryForTest(t, f)
	var attempts, dispatches, childAttempt, childLimit, parentLimit int
	if err := testPool.QueryRow(t.Context(), `SELECT r.attempt,r.dispatch_count,c.attempt,c.max_attempts,p.max_attempts
FROM creative_recovery r JOIN agent_task_queue c ON c.id=r.result_task_id JOIN agent_task_queue p ON p.id=c.parent_task_id
WHERE r.order_id=$1 AND r.stage='planning'`, f.OrderID).Scan(&attempts, &dispatches, &childAttempt, &childLimit, &parentLimit); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || dispatches != 1 || childAttempt != 13 || childLimit != 14 || parentLimit != 12 {
		t.Fatalf("budget=%d/%d child=%d/%d original=%d", attempts, dispatches, childAttempt, childLimit, parentLimit)
	}
}

func TestCreativeRecoveryDispatchFencesEnqueueAndRetainsCrashReceipt(t *testing.T) {
	enableCreativeFactoryForTest(t)
	f := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, f, 3)
	if err := testHandler.discoverCreativeOrderRecovery(t.Context(), parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	target, found, err := testHandler.claimCreativeRecovery(t.Context(), parseUUID(f.OrderID))
	if err != nil || !found {
		t.Fatalf("claim=%v %v", found, err)
	}
	invalid := target
	invalid.LeaseToken = pgtype.UUID{}
	ctx := context.WithValue(t.Context(), creativeRecoveryDispatchContextKey{}, invalid)
	if _, err := testHandler.executeCreativeRecovery(ctx, invalid); err == nil {
		t.Fatal("lost lease was allowed to create a task")
	}
	var tasks int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_task_binding WHERE order_id=$1 AND workflow='creative_candidate_selection'`, f.OrderID).Scan(&tasks); err != nil || tasks != 0 {
		t.Fatalf("rolled-back dispatch left %d tasks: %v", tasks, err)
	}
	ctx = context.WithValue(t.Context(), creativeRecoveryDispatchContextKey{}, target)
	if _, err := testHandler.executeCreativeRecovery(ctx, target); err != nil {
		t.Fatal(err)
	}
	// Simulate a worker crash after dispatch commits but before finish.
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_recovery SET lease_expires_at=now()-interval '1 second' WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.expireCreativeRecoveryLeases(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_recovery SET next_retry_at=now() WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	next, found, err := testHandler.claimCreativeRecovery(t.Context(), parseUUID(f.OrderID))
	if err != nil || !found {
		t.Fatalf("reclaim=%v %v", found, err)
	}
	if err := testHandler.processCreativeRecovery(t.Context(), next); err != nil {
		t.Fatal(err)
	}
	var dispatches, receipts int
	if err := testPool.QueryRow(t.Context(), `SELECT r.dispatch_count,(SELECT count(*) FROM creative_recovery_attempt WHERE recovery_id=r.id AND result_task_id IS NOT NULL) FROM creative_recovery r WHERE r.id=$1`, target.ID).Scan(&dispatches, &receipts); err != nil {
		t.Fatal(err)
	}
	if dispatches != 1 || receipts != 1 {
		t.Fatalf("crash lost or duplicated dispatch: %d/%d", dispatches, receipts)
	}
}

func TestCreativeRecoverySelectionUsesItsOwnBoundedBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, f, 3)
	for range 3 {
		queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{})
		if err != nil || !queued {
			t.Fatalf("seed selection: %v %v", queued, err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status='failed',completed_at=now() WHERE trigger_evidence_ref_id=$1 AND trigger_evidence_kind='creative_order_item_candidate_selection'`, f.ItemID); err != nil {
			t.Fatal(err)
		}
	}
	runOrderRecoveryForTest(t, f)
	var dispatches, total int
	if err := testPool.QueryRow(t.Context(), `SELECT dispatch_count,(SELECT count(*) FROM creative_task_binding WHERE order_item_id=$1 AND workflow='creative_candidate_selection') FROM creative_recovery WHERE order_item_id=$1 AND stage='candidate_selection'`, f.ItemID).Scan(&dispatches, &total); err != nil {
		t.Fatal(err)
	}
	if dispatches != 1 || total != 4 {
		t.Fatalf("selection recovery was blocked: dispatched=%d tasks=%d", dispatches, total)
	}
}

func TestCreativeRecoveryProductionKeepsFrozenOperationAndRealBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
	addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
	addCreativeCandidateOrchestrationProductionTask(t, f, id, "completed", "selected_missing_sizes")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET attempt=12,max_attempts=12 WHERE context->>'variant_id'=$1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_image_operation(variant_id,revision,size_key,operation_kind,idempotency_key,status,input_snapshot)
VALUES($1,1,'800x1000','generation','frozen-operation','failed','{"source_attachment":"original","roles":["source"]}')`, id); err != nil {
		t.Fatal(err)
	}
	runOrderRecoveryForTest(t, f)
	var frozen bool
	var dispatches, children int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*),bool_and(context->'creative_recovery'->'operations'->0->'input_snapshot'='{"source_attachment":"original","roles":["source"]}'::jsonb)
FROM agent_task_queue WHERE context->>'variant_id'=$1 AND parent_task_id IS NOT NULL`, id).Scan(&children, &frozen); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT sum(dispatch_count) FROM creative_recovery WHERE variant_id=$1`, id).Scan(&dispatches); err != nil {
		t.Fatal(err)
	}
	if !frozen || children != 1 || dispatches != 1 {
		t.Fatalf("recovery lost input or double charged: frozen=%v children=%d dispatches=%d", frozen, children, dispatches)
	}
}

func TestCreativeRecoveryStaleProviderResultRequiresReconciliation(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
	addCreativeCandidateOrchestrationProductionTask(t, f, id, "failed", "selected_missing_sizes")
	if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_image_operation(variant_id,revision,size_key,operation_kind,idempotency_key,status,updated_at)
VALUES($1,1,'1080x1080','generation','unconfirmed-provider','unknown',now()-interval '20 minutes')`, id); err != nil {
		t.Fatal(err)
	}
	runOrderRecoveryForTest(t, f)
	var stopped bool
	var dispatched int
	if err := testPool.QueryRow(t.Context(), `SELECT bool_and(status='manual_required' AND last_error LIKE '%reconcile its receipt%'),sum(dispatch_count) FROM creative_recovery WHERE variant_id=$1 AND stage='production'`, id).Scan(&stopped, &dispatched); err != nil {
		t.Fatal(err)
	}
	if !stopped || dispatched != 0 {
		t.Fatalf("unconfirmed provider was repeated or left waiting forever: stopped=%v dispatched=%d", stopped, dispatched)
	}
}
