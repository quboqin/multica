package handler

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
)

func TestCreativeTaskBindingTracksHierarchyAndSizeScope(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	ids := seedSixSetCandidatePrimaries(t, f, 1)
	var orderID, itemID, variantID string
	var revision, sizes int
	err := testPool.QueryRow(t.Context(), `SELECT b.order_id::text,b.order_item_id::text,b.variant_id::text,b.revision,
 (SELECT count(*) FROM creative_task_size s WHERE s.task_id=b.task_id)
 FROM creative_task_binding b WHERE b.variant_id=$1 AND b.workflow='creative_production'`, ids[0]).Scan(&orderID, &itemID, &variantID, &revision, &sizes)
	if err != nil {
		t.Fatal(err)
	}
	if orderID != f.OrderID || itemID != f.ItemID || variantID != ids[0] || revision != 1 || sizes != 3 {
		t.Fatalf("binding=%s %s %s %d %d", orderID, itemID, variantID, revision, sizes)
	}
	plan := seedCandidatePlanTask(t, f, "completed")
	var variantNull bool
	if err := testPool.QueryRow(t.Context(), `SELECT variant_id IS NULL FROM creative_task_binding WHERE task_id=$1 AND workflow='creative_plan'`, plan.ID).Scan(&variantNull); err != nil || !variantNull {
		t.Fatalf("plan binding: %v %v", variantNull, err)
	}
}

func TestCreativeTaskBindingSizeScopeMatchesImageContract(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	ids := seedSixSetCandidatePrimaries(t, f, 1)
	var taskID string
	var original []byte
	if err := testPool.QueryRow(t.Context(), `SELECT t.id::text,t.context FROM agent_task_queue t JOIN creative_task_binding b ON b.task_id=t.id WHERE b.variant_id=$1 LIMIT 1`, ids[0]).Scan(&taskID, &original); err != nil {
		t.Fatal(err)
	}
	cases := []map[string]any{
		{"workflow": "creative_production"},
		{"workflow": "creative_production", "missing_sizes": []string{"1200x628", "800x1000"}},
		{"workflow": "creative_production", "late_receipt_recovery": map[string]any{"size_key": "800x1000"}},
		{"workflow": "creative_production", "late_receipt_recoveries": []map[string]any{{"size_key": "1200x628"}}},
		{"workflow": "creative_production", "qc_visual_rework": map[string]any{"target_sizes": []string{"1080x1080"}}},
		{"workflow": "creative_direct_edit", "target_size": "800x1000"},
		{"workflow": "creative_direct_edit", "direct_edit": map[string]any{"edit_sizes": []string{"1200x628"}}},
	}
	for _, fields := range cases {
		var payload map[string]any
		if err := json.Unmarshal(original, &payload); err != nil {
			t.Fatal(err)
		}
		for k, v := range fields {
			payload[k] = v
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		expected, err := creativeTaskImageScopeSizes(raw)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(expected)
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=$2::jsonb WHERE id=$1`, taskID, raw); err != nil {
			t.Fatal(err)
		}
		var actual []string
		if err := testPool.QueryRow(t.Context(), `SELECT array_agg(size_key ORDER BY size_key) FROM creative_task_size WHERE task_id=$1 AND is_required`, taskID).Scan(&actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("scope=%v got %v want %v", fields, actual, expected)
		}
	}
}

func TestCreativeTaskBindingIsolatesMismatchedOrder(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	other := createCreativeCountFixture(t, 3)
	ids := seedSixSetCandidatePrimaries(t, f, 1)
	_, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET context=jsonb_set(context,'{creative_order_id}',to_jsonb($2::text)) WHERE id=(SELECT task_id FROM creative_task_binding WHERE variant_id=$1 LIMIT 1)`, ids[0], other.OrderID)
	if err != nil {
		t.Fatal(err)
	}
	var unresolved int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_task_binding b JOIN agent_task_queue q ON q.id=b.task_id WHERE q.context->>'variant_id'=$1 AND b.binding_status='unresolved' AND b.order_id IS NULL AND b.binding_error='scope_mismatch'`, ids[0]).Scan(&unresolved); err != nil || unresolved != 1 {
		t.Fatalf("invalid scope was not isolated: %d %v", unresolved, err)
	}
}
