package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func copyOrderTestLibrary() creativeResourceResponse {
	return creativeResourceResponse{ID: "library-1", Name: "Approved copy", PublishedVersion: 2, Config: json.RawMessage(`{
		"schema_version":4,"fragments":[
		{"id":"headline-1","key":"headline","role":"headline","creative_types":["num","repayment_plan"],"text":"Flexible financing","status":"approved"},
		{"id":"draft-1","key":"draft","role":"benefit","creative_types":["num"],"text":"Draft promise","status":"draft"}
		],"repayment_plan":{"entries":[{"id":"plan-1","key":"plan","principal":1000,"tenor_months":3,"monthly_installment":350,"total_interest":50,"total_repayment":1050,"source":"Approved table","status":"approved"}]}
	}`)}
}

func TestCopyLibraryOrderFreezesOnlySelectedCopy(t *testing.T) {
	selection := creativeCopyLibrarySelection{LibraryVersion: 2, CreativeType: "num", Slots: map[string][]string{"subheadline": {"headline-1"}}, RepaymentPlanKeys: []string{"plan"}}
	raw, err := freezeCreativeCopyLibrarySelection(selection, copyOrderTestLibrary())
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["headline"] != "" || snapshot["benefit"] != "" || snapshot["subheadline"] != "Flexible financing" || snapshot["visual_only"] != false {
		t.Fatalf("unexpected copy: %s", raw)
	}
	if len(snapshot["repayment_plan_entries"].([]any)) != 1 {
		t.Fatalf("missing frozen plan: %s", raw)
	}
}

func TestCopyLibraryOrderAllowsExplicitEmptySelection(t *testing.T) {
	selection := creativeCopyLibrarySelection{LibraryVersion: 2, CreativeType: "repayment_plan", VisualOnly: true}
	raw, err := freezeCreativeCopyLibrarySelection(selection, copyOrderTestLibrary())
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot["visual_only"] != true || len(snapshot["fragments"].([]any)) != 0 || len(snapshot["repayment_plan_entries"].([]any)) != 0 {
		t.Fatalf("empty selection was filled: %s", raw)
	}
	selection.VisualOnly = false
	if _, err := freezeCreativeCopyLibrarySelection(selection, copyOrderTestLibrary()); err == nil {
		t.Fatal("empty selection requires explicit visual-only intent")
	}
}

func TestCopyLibraryOrderRejectsStaleOrUnapprovedSelections(t *testing.T) {
	for name, selection := range map[string]creativeCopyLibrarySelection{
		"stale":     {LibraryVersion: 1, CreativeType: "num", VisualOnly: true},
		"draft":     {LibraryVersion: 2, CreativeType: "num", Slots: map[string][]string{"benefit": {"draft-1"}}},
		"unknown":   {LibraryVersion: 2, CreativeType: "num", Slots: map[string][]string{"headline": {"missing"}}},
		"duplicate": {LibraryVersion: 2, CreativeType: "num", Slots: map[string][]string{"headline": {"headline-1"}, "benefit": {"headline-1"}}},
		"plan":      {LibraryVersion: 2, CreativeType: "num", RepaymentPlanKeys: []string{"missing"}},
		"slot":      {LibraryVersion: 2, CreativeType: "num", VisualOnly: true, Slots: map[string][]string{"unrecognized": {}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := freezeCreativeCopyLibrarySelection(selection, copyOrderTestLibrary()); err == nil {
				t.Fatal("invalid selection was accepted")
			}
		})
	}
}

func TestCopyLibraryOrderSourceNormalization(t *testing.T) {
	input := creativeOrderInput{Status: "queued", InputSnapshot: json.RawMessage(`{}`), Items: []creativeOrderItemInput{{SourceKind: "copy_library", CopyLibraryID: "library-1", CopySnapshot: json.RawMessage(`{}`)}}}
	if _, err := normalizeCreativeOrder(input); err != nil {
		t.Fatal(err)
	}
	input.Items[0].CandidateID = "candidate-1"
	if _, err := normalizeCreativeOrder(input); err == nil {
		t.Fatal("copy source must not carry a candidate")
	}
	input.Items[0].SourceKind = "material"
	if _, err := normalizeCreativeOrder(input); err == nil {
		t.Fatal("material source must not carry a copy library relation")
	}
}

func TestCopyLibraryOrderMigrationAndReadback(t *testing.T) {
	if testPool == nil || testHandler == nil {
		t.Skip("database not available")
	}
	tx, err := testPool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	// Temporary tables keep migration verification isolated from workspace data.
	_, err = tx.Exec(t.Context(), `
CREATE TEMP TABLE creative_resource (id UUID PRIMARY KEY) ON COMMIT DROP;
CREATE TEMP TABLE creative_order_item (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(), order_id UUID NOT NULL,
  candidate_id UUID NOT NULL, source_analysis_id UUID, copy_snapshot JSONB NOT NULL DEFAULT '{}',
  direction TEXT NOT NULL DEFAULT '', status TEXT NOT NULL DEFAULT 'ready',
  adopted_variant_id UUID, adopted_at TIMESTAMPTZ, adopted_by UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
) ON COMMIT DROP;`)
	if err != nil {
		t.Fatal(err)
	}
	orderID, candidateID, libraryID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_item (order_id, candidate_id) VALUES ($1, $2)`, orderID, candidateID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_resource (id) VALUES ($1)`, libraryID); err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "278_creative_copy_library_orders.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), string(up)); err != nil {
		t.Fatal(err)
	}
	var copyItemID string
	if err := tx.QueryRow(t.Context(), `INSERT INTO creative_order_item (order_id, source_kind, copy_library_id) VALUES ($1, 'copy_library', $2) RETURNING id::text`, orderID, libraryID).Scan(&copyItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `SAVEPOINT source_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `INSERT INTO creative_order_item (order_id, source_kind, candidate_id, copy_library_id) VALUES ($1, 'copy_library', $2, $3)`, orderID, candidateID, libraryID); err == nil {
		t.Fatal("mixed source references were accepted")
	}
	if _, err := tx.Exec(t.Context(), `ROLLBACK TO SAVEPOINT source_check`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(t.Context(), `
SELECT id::text, order_id::text, COALESCE(candidate_id::text, ''), COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
direction, status, COALESCE(adopted_variant_id::text, ''), COALESCE(adopted_at::text, ''), COALESCE(adopted_by::text, ''),
created_at::text, updated_at::text, source_kind, COALESCE(copy_library_id::text, '')
FROM creative_order_item WHERE order_id = $1`, orderID)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for rows.Next() {
		item, err := scanCreativeOrderItemWithAdoption(rows)
		if err != nil {
			t.Fatal(err)
		}
		if item.ID == copyItemID && (item.SourceKind != "copy_library" || item.CopyLibraryID != libraryID || item.CandidateID != "") {
			t.Fatalf("lost copy source: %+v", item)
		}
		if item.ID != copyItemID && (item.SourceKind != "material" || item.CandidateID != candidateID) {
			t.Fatalf("changed material source: %+v", item)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	if count != 2 {
		t.Fatalf("expected material and copy items, got %d", count)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "278_creative_copy_library_orders.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), string(down)); err == nil {
		t.Fatal("rollback must not discard copy orders")
	}
	if _, err := tx.Exec(t.Context(), `ROLLBACK TO SAVEPOINT source_check`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `DELETE FROM pg_temp.creative_order_item WHERE id = $1`, copyItemID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), string(down)); err != nil {
		t.Fatal(err)
	}
}
