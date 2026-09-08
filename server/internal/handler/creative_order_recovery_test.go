package handler

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func runOrderRecoveryForTest(t *testing.T, f creativeCandidateOrchestrationFixture) {
	t.Helper()
	enableCreativeFactoryForTest(t)
	if err := testHandler.discoverCreativeOrderRecovery(t.Context(), parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	for range 40 {
		target, found, err := testHandler.claimCreativeRecovery(t.Context(), parseUUID(f.OrderID))
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			return
		}
		if err := testHandler.processCreativeRecovery(t.Context(), target); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatal("recovery batch failed to settle")
}

func TestCreativeOrderRecoveryQueuesMissingSelectionOnce(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 8)
	runOrderRecoveryForTest(t, f)
	runOrderRecoveryForTest(t, f)
	var tasks, attempts int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_task_binding WHERE order_item_id=$1 AND workflow='creative_candidate_selection'`, f.ItemID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_recovery_attempt a JOIN creative_recovery r ON r.id=a.recovery_id WHERE r.order_id=$1`, f.OrderID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || attempts != 1 {
		t.Fatalf("tasks=%d attempts=%d", tasks, attempts)
	}
}

func TestCreativeOrderRecoveryCreatesMissingPlanAndRecordsHistory(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	runOrderRecoveryForTest(t, f)
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_task_binding WHERE order_id=$1 AND workflow='creative_plan' AND variant_id IS NULL`, f.OrderID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("planning=%d %v", count, err)
	}
	history, err := testHandler.listCreativeOrderRecoveries(t.Context(), parseUUID(f.OrderID))
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ReasonCode != "planning_task_missing" || history[0].ResultTaskID == "" || len(history[0].Attempts) != 1 {
		t.Fatalf("history=%+v", history)
	}
}

func TestCreativeOrderRecoveryPreservesManualBlockAndRetryBudget(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 6)
	task := seedCandidatePlanTask(t, f, "failed")
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET failure_reason='agent_blocked',error='waiting for confirmation' WHERE id=$1`, task.ID); err != nil {
		t.Fatal(err)
	}
	runOrderRecoveryForTest(t, f)
	var status string
	var attempt int
	if err := testPool.QueryRow(t.Context(), `SELECT status,attempt FROM creative_recovery WHERE order_id=$1 AND stage='planning'`, f.OrderID).Scan(&status, &attempt); err != nil {
		t.Fatal(err)
	}
	if status != "manual_required" || attempt != 0 {
		t.Fatalf("manual input was retried: %s %d", status, attempt)
	}
}

func TestCreativeOrderRecoveryStopsAfterBudgetAndFencesExpiredLease(t *testing.T) {
	enableCreativeFactoryForTest(t)
	f := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, f, 3)
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_skill SET enabled=false WHERE agent_id=$1`, f.Squad.ReviewerAgentID); err != nil {
		t.Fatal(err)
	}
	if err := testHandler.discoverCreativeOrderRecovery(t.Context(), parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	var prior creativeRecoveryTarget
	for attempt := 1; attempt <= 3; attempt++ {
		if _, err := testPool.Exec(t.Context(), `UPDATE creative_recovery SET next_retry_at=now() WHERE order_id=$1`, f.OrderID); err != nil {
			t.Fatal(err)
		}
		target, found, err := testHandler.claimCreativeRecovery(t.Context(), parseUUID(f.OrderID))
		if err != nil || !found {
			t.Fatalf("claim=%v %v", found, err)
		}
		prior = target
		if err := testHandler.processCreativeRecovery(t.Context(), target); err == nil {
			t.Fatal("invalid reviewer accepted")
		}
	}
	var status string
	var attempts int
	if err := testPool.QueryRow(t.Context(), `SELECT status,attempt FROM creative_recovery WHERE order_id=$1 AND stage='candidate_selection'`, f.OrderID).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "manual_required" || attempts != 3 {
		t.Fatalf("budget=%s %d", status, attempts)
	}
	if err := testHandler.finishCreativeRecovery(t.Context(), prior, "resolved", pgtype.UUID{}, nil); err == nil {
		t.Fatal("lost lease overwrote terminal recovery")
	}
}

func TestCreativeOrderRecoveryResolvesToExactAsset(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
	target := creativeRecoveryTarget{WorkspaceID: parseUUID(testWorkspaceID), OrderID: parseUUID(f.OrderID), ItemID: parseUUID(f.ItemID), VariantID: parseUUID(id), Revision: 1, Stage: "production", Size: "800x1000", Reason: "generated_size_missing"}
	if err := testHandler.recordCreativeRecovery(t.Context(), target); err != nil {
		t.Fatal(err)
	}
	addCreativeCandidateOrchestrationAsset(t, id, "800x1000", "generated")
	if err := testHandler.refreshCreativeRecoveryResults(t.Context(), parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	var status, size string
	if err := testPool.QueryRow(t.Context(), `SELECT r.status,a.size_key FROM creative_recovery r JOIN creative_order_asset a ON a.id=r.resolved_asset_id WHERE r.order_id=$1`, f.OrderID).Scan(&status, &size); err != nil {
		t.Fatal(err)
	}
	if status != "resolved" || size != "800x1000" {
		t.Fatalf("result=%s %s", status, size)
	}
}

func TestCreativeOrderRecoveryRepairsSixOfEightPlanning(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 6)
	parent := seedCandidatePlanTask(t, f, "completed")
	runOrderRecoveryForTest(t, f)
	var children, assets int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE parent_task_id=$1`, parent.ID).Scan(&children); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_asset a JOIN creative_order_variant v ON v.id=a.variant_id WHERE v.order_item_id=$1`, f.ItemID).Scan(&assets); err != nil {
		t.Fatal(err)
	}
	if children != 1 || assets != 12 {
		t.Fatalf("children=%d preserved assets=%d", children, assets)
	}
}

func TestCreativeOrderRecoveryRepairsSelectedExpansionWithoutRegeneratingPrimary(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
	attachment := addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
	addCreativeCandidateOrchestrationProductionTask(t, f, id, "completed", "candidate_primary")
	runOrderRecoveryForTest(t, f)
	var count int
	var missing string
	if err := testPool.QueryRow(t.Context(), `SELECT count(*),min((t.context->'missing_sizes')::text) FROM creative_task_binding b JOIN agent_task_queue t ON t.id=b.task_id WHERE b.variant_id=$1 AND b.production_phase='selected_missing_sizes'`, id).Scan(&count, &missing); err != nil {
		t.Fatal(err)
	}
	if count != 1 || missing != `["1200x628", "800x1000"]` {
		t.Fatalf("tasks=%d missing=%s", count, missing)
	}
	var current string
	if err := testPool.QueryRow(t.Context(), `SELECT attachment_id::text FROM creative_order_asset WHERE variant_id=$1 AND stage='generated' AND size_key='1080x1080'`, id).Scan(&current); err != nil || current != attachment {
		t.Fatalf("primary changed: %s %v", current, err)
	}
}

func TestCreativeOrderRecoveryLeavesUnknownProviderAndCancelledOrdersAlone(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			f := createCreativeCountFixture(t, 1)
			id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
			addCreativeCandidateOrchestrationProductionTask(t, f, id, "failed", "selected_missing_sizes")
			if cancelled {
				if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET status='cancelled' WHERE id=$1`, f.OrderID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_image_operation(variant_id,revision,size_key,operation_kind,idempotency_key,status,provider_request_id) VALUES($1,1,'1080x1080','generation','unknown-test','unknown','already-submitted')`, id); err != nil {
					t.Fatal(err)
				}
			}
			runOrderRecoveryForTest(t, f)
			var attempts int
			if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_recovery_attempt a JOIN creative_recovery r ON r.id=a.recovery_id WHERE r.order_id=$1`, f.OrderID).Scan(&attempts); err != nil || attempts != 0 {
				t.Fatalf("unexpected retry: %d %v", attempts, err)
			}
		})
	}
}

func TestCreativeOrderRecoveryTwoWorkersClaimDistinctWork(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, f, 3)
	if err := testHandler.discoverCreativeOrderRecovery(t.Context(), parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	claims := make(chan creativeRecoveryTarget, 2)
	errs := make(chan error, 2)
	for range 2 {
		go func() { target, _, err := testHandler.claimCreativeRecovery(ctx); claims <- target; errs <- err }()
	}
	first, second := <-claims, <-claims
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if first.ID.Valid && second.ID.Valid && first.ID == second.ID {
		t.Fatal("two workers claimed the same recovery")
	}
}

func TestCreativeOrderRecoveryFailureBackoffDoesNotBlockNextOrder(t *testing.T) {
	enableCreativeFactoryForTest(t)
	bad := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, bad, 3)
	good := createCreativeCountFixture(t, 3)
	seedSixSetCandidatePrimaries(t, good, 5)
	if _, err := testPool.Exec(t.Context(), `UPDATE agent_skill SET enabled=false WHERE agent_id=$1`, bad.Squad.ReviewerAgentID); err != nil {
		t.Fatal(err)
	}
	for _, f := range []creativeCandidateOrchestrationFixture{bad, good} {
		if err := testHandler.discoverCreativeOrderRecovery(t.Context(), parseUUID(f.OrderID)); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		target, found, err := testHandler.claimCreativeRecovery(t.Context())
		if err != nil || !found {
			t.Fatalf("claim %v %v", found, err)
		}
		_ = testHandler.processCreativeRecovery(t.Context(), target)
	}
	var status string
	var attempt int
	var later bool
	if err := testPool.QueryRow(t.Context(), `SELECT status,attempt,next_retry_at>now() FROM creative_recovery WHERE order_id=$1 AND stage='candidate_selection'`, bad.OrderID).Scan(&status, &attempt, &later); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || attempt != 1 || !later {
		t.Fatalf("backoff=%s %d %v", status, attempt, later)
	}
	var task pgtype.UUID
	if err := testPool.QueryRow(t.Context(), `SELECT task_id FROM creative_task_binding WHERE order_id=$1 AND workflow='creative_candidate_selection'`, good.OrderID).Scan(&task); err != nil {
		t.Fatal(err)
	}
}
