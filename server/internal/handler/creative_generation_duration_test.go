package handler

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestCreativeGenerationDurationConcurrentCompletionAndRollingWindow(t *testing.T) {
	setCreativeMeasurementEpochForTest(t, "-3 days")
	_, before, err := testHandler.creativeInitialGeneratedPackageDuration(t.Context(), parseUUID(testWorkspaceID))
	if err != nil {
		t.Fatal(err)
	}
	old := createCreativeCountFixture(t, 2)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET created_at=now()-interval '25 hours' WHERE id=$1`, old.OrderID); err != nil {
		t.Fatal(err)
	}
	oldVariant := createCreativeCandidateOrchestrationVariant(t, old.ItemID, "C01", "selected", 1, "running", "1080x1080", standardCreativeAssetSizes)
	for _, size := range standardCreativeAssetSizes {
		addCreativeCandidateOrchestrationAsset(t, oldVariant, size, "generated")
	}
	var secondOldVariant string
	if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_variant(order_item_id,variant_key,revision,candidate_state,selection_rank,status) VALUES($1,'C02',1,'selected',2,'running') RETURNING id::text`, old.ItemID).Scan(&secondOldVariant); err != nil {
		t.Fatal(err)
	}
	for _, size := range standardCreativeAssetSizes {
		addCreativeCandidateOrchestrationAsset(t, secondOldVariant, size, "generated")
	}
	_, afterOld, err := testHandler.creativeInitialGeneratedPackageDuration(t.Context(), parseUUID(testWorkspaceID))
	if err != nil || afterOld != before {
		t.Fatalf("old completion entered 24h cohort: %d -> %d %v", before, afterOld, err)
	}
	f := createCreativeCountFixture(t, 1)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order SET created_at=now()-interval '30 minutes' WHERE id=$1`, f.OrderID); err != nil {
		t.Fatal(err)
	}
	variant := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "running", "1080x1080", standardCreativeAssetSizes)
	attachment := addCreativeCandidateOrchestrationAsset(t, variant, "1080x1080", "generated")
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	errs := make(chan error, 2)
	for _, size := range []string{"1200x628", "800x1000"} {
		go func() {
			_, err := testPool.Exec(ctx, `INSERT INTO creative_order_asset(variant_id,size_key,revision,stage,status,attachment_id) VALUES($1,$2,1,'generated','completed',$3)`, variant, size, attachment)
			errs <- err
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	_, after, err := testHandler.creativeInitialGeneratedPackageDuration(t.Context(), parseUUID(testWorkspaceID))
	if err != nil || after != before+1 {
		t.Fatalf("concurrent final sizes lost completion: %d -> %d %v", before, after, err)
	}
}

func setCreativeMeasurementEpochForTest(t *testing.T, offset string) {
	t.Helper()
	if testPool == nil || testHandler == nil {
		t.Skip("database not available")
	}
	var prior pgtype.Timestamptz
	if err := testPool.QueryRow(t.Context(), `SELECT started_at FROM creative_generation_measurement_epoch`).Scan(&prior); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_generation_measurement_epoch SET started_at=now()+$1::interval`, offset); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `UPDATE creative_generation_measurement_epoch SET started_at=$1`, prior)
	})
}

func TestCreativeGenerationDurationFreezesCurrentRevisionAndExcludesOldOrders(t *testing.T) {
	setCreativeMeasurementEpochForTest(t, "-1 day")
	_, candidate := createCreativeFeedbackCandidate(t, "duration frozen cohort")
	attachment := createCreativeFeedbackAsset(t)
	create := func(age string) (string, string, string) {
		t.Helper()
		var order, item, variant string
		if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order(workspace_id,status,input_snapshot,created_by,created_at)
VALUES($1,'running','{"target_variant_count":1}'::jsonb,$2,now()+$3::interval) RETURNING id::text`, testWorkspaceID, testUserID, age).Scan(&order); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM creative_order WHERE id=$1`, order) })
		if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_item(order_id,candidate_id,copy_snapshot) VALUES($1,$2,'{}') RETURNING id::text`, order, candidate).Scan(&item); err != nil {
			t.Fatal(err)
		}
		if err := testPool.QueryRow(t.Context(), `INSERT INTO creative_order_variant(order_item_id,variant_key,revision,candidate_state,status) VALUES($1,'V01',1,'selected','running') RETURNING id::text`, item).Scan(&variant); err != nil {
			t.Fatal(err)
		}
		return order, item, variant
	}
	asset := func(variant, size string, revision int) {
		t.Helper()
		if _, err := testPool.Exec(t.Context(), `INSERT INTO creative_order_asset(variant_id,size_key,revision,stage,status,attachment_id) VALUES($1,$2,$3,'generated','completed',$4)`, variant, size, revision, attachment); err != nil {
			t.Fatal(err)
		}
	}
	_, oldItem, oldVariant := create("-2 days")
	for _, size := range standardCreativeAssetSizes {
		asset(oldVariant, size, 1)
	}
	_, item, variant := create("-20 minutes")
	asset(variant, "1080x1080", 1)
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET revision=2 WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	asset(variant, "1200x628", 2)
	asset(variant, "800x1000", 2)
	var count int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_initial_generated_package WHERE order_item_id IN ($1,$2)`, item, oldItem).Scan(&count); err != nil || count != 0 {
		t.Fatalf("old or mixed-revision package counted: %d %v", count, err)
	}
	asset(variant, "1080x1080", 2)
	var generated pgtype.Timestamptz
	var elapsed float64
	var completeSnapshot bool
	if err := testPool.QueryRow(t.Context(), `SELECT generated_at,extract(epoch FROM(generated_at-submitted_at))::double precision,
jsonb_array_length(assets)=3 AND NOT EXISTS(SELECT 1 FROM jsonb_array_elements(assets) a WHERE a->>'revision'<>'2')
FROM creative_initial_generated_package WHERE order_item_id=$1`, item).Scan(&generated, &elapsed, &completeSnapshot); err != nil {
		t.Fatal(err)
	}
	if elapsed < 1200 || elapsed > 1230 || !completeSnapshot {
		t.Fatalf("wrong first package duration or evidence: %v %v", elapsed, completeSnapshot)
	}
	if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET candidate_state='rejected',revision=3 WHERE id=$1`, variant); err != nil {
		t.Fatal(err)
	}
	var unchanged bool
	if err := testPool.QueryRow(t.Context(), `SELECT generated_at=$2 FROM creative_initial_generated_package WHERE order_item_id=$1`, item, generated).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("later selection rewrote initial duration: %v %v", unchanged, err)
	}
}
