package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func createCreativeCountFixture(t *testing.T, target int) creativeCandidateOrchestrationFixture {
	t.Helper()
	f := createCreativeCandidateOrchestrationFixture(t, fmt.Sprintf("%s copy target %d", t.Name(), target))
	var libraryID string
	if err := testPool.QueryRow(t.Context(), `
INSERT INTO creative_resource (workspace_id, kind, name, created_by)
VALUES ($1, 'copy_library', 'Variant count test', $2) RETURNING id::text
`, testWorkspaceID, testUserID).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_order WHERE id = $1`, f.OrderID)
		_, _ = testPool.Exec(t.Context(), `DELETE FROM creative_resource WHERE id = $1`, libraryID)
	})
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order_item SET source_kind = 'copy_library', candidate_id = NULL, source_analysis_id = NULL, copy_library_id = $2 WHERE id = $1
`, f.ItemID, libraryID); err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(t.Context(), `
UPDATE creative_order SET input_snapshot = input_snapshot || jsonb_build_object('target_variant_count', $2::int, 'candidate_count', $2::int + 2, 'reserve_promotion_mode', 'allow') WHERE id = $1
`, f.OrderID, target); err != nil {
		t.Fatal(err)
	}
	return f
}

func selectCreativeCountCandidates(t *testing.T, f creativeCandidateOrchestrationFixture, ids []string, target, wantStatus int) {
	t.Helper()
	w := httptest.NewRecorder()
	req := newRequest(http.MethodPost, "/api/creative/orders/"+f.OrderID+"/items/"+f.ItemID+"/candidate-selection", creativeCandidateSelectionInput{SelectedIDs: ids[:target], ReserveIDs: ids[target:]})
	req = withURLParams(req, "id", f.OrderID, "itemId", f.ItemID)
	testHandler.SelectCreativeOrderItemCandidates(w, req)
	if w.Code != wantStatus {
		t.Fatalf("selection = %d %s, want %d", w.Code, w.Body.String(), wantStatus)
	}
}

func TestCreativeVariantCountsCandidateSelectionAndFanout(t *testing.T) {
	for _, target := range []int{1, 3, 10} {
		t.Run(fmt.Sprint(target), func(t *testing.T) {
			f := createCreativeCountFixture(t, target)
			ids := make([]string, 0, target+2)
			for index := 1; index <= target+2; index++ {
				w := httptest.NewRecorder()
				req := withURLParam(newRequest(http.MethodPut, "/api/creative/orders/"+f.OrderID+"/variants", creativeOrderVariantInput{
					OrderItemID: f.ItemID, VariantKey: fmt.Sprintf("C%02d", index), Brief: json.RawMessage(`{}`), Revision: 1,
					Status: "completed", CandidateState: "candidate", PrimarySize: "1080x1080",
				}), "id", f.OrderID)
				testHandler.UpsertCreativeOrderVariant(w, req)
				if w.Code != http.StatusOK {
					t.Fatalf("candidate put = %d %s", w.Code, w.Body.String())
				}
				var v creativeOrderVariantResponse
				if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, v.ID)
				addCreativeCandidateOrchestrationAsset(t, v.ID, "1080x1080", "generated")
				addCreativeCandidateOrchestrationAsset(t, v.ID, "1080x1080", "primed")
				addCreativeCandidateOrchestrationProductionTask(t, f, v.ID, "completed", "candidate_primary")
				if index == target+1 {
					queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{})
					if err != nil || queued {
						t.Fatalf("unfinished planning queued selection: %v, %v", queued, err)
					}
				}
			}
			queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)})
			if err != nil || !queued {
				t.Fatalf("candidate selection = %v, %v", queued, err)
			}
			var frozenTarget, compared int
			if err := testPool.QueryRow(t.Context(), `SELECT (context->>'target_variant_count')::int, jsonb_array_length(context->'candidates') FROM agent_task_queue WHERE trigger_evidence_kind = $1 AND trigger_evidence_ref_id = $2`, creativeCandidateSelectionEvidenceKind, f.ItemID).Scan(&frozenTarget, &compared); err != nil {
				t.Fatal(err)
			}
			if frozenTarget != target || compared != target+2 {
				t.Fatalf("selection context: %d/%d", frozenTarget, compared)
			}
			selectCreativeCountCandidates(t, f, ids[:target+1], target-1, http.StatusBadRequest)
			selectCreativeCountCandidates(t, f, ids, target, http.StatusOK)
			selectCreativeCountCandidates(t, f, ids, target, http.StatusOK)
			committed, err := testHandler.creativeCandidateSelectionCommitted(t.Context(), parseUUID(testWorkspaceID), parseUUID(f.OrderID), parseUUID(f.ItemID))
			if err != nil || !committed {
				t.Fatalf("committed selection = %v, %v", committed, err)
			}
			rows, err := testPool.Query(t.Context(), `SELECT context::text FROM agent_task_queue WHERE trigger_evidence_ref_id = $1 AND context->>'production_phase' = $2`, f.ItemID, creativeSelectedExpansionPhase)
			if err != nil {
				t.Fatal(err)
			}
			var queuedIDs []string
			for rows.Next() {
				var raw string
				if err := rows.Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var task struct {
					VariantID   string   `json:"variant_id"`
					SourceKind  string   `json:"source_kind"`
					LibraryID   string   `json:"copy_library_id"`
					CandidateID string   `json:"candidate_id"`
					Missing     []string `json:"missing_sizes"`
					Expected    []string `json:"expected_sizes"`
					Reuse       bool     `json:"reuse_primary"`
				}
				if err := json.Unmarshal([]byte(raw), &task); err != nil {
					t.Fatal(err)
				}
				if task.SourceKind != "copy_library" || task.LibraryID == "" || task.CandidateID != "" || !task.Reuse || !slices.Equal(task.Missing, []string{"1200x628", "800x1000"}) || !slices.Equal(task.Expected, standardCreativeAssetSizes) {
					t.Fatalf("invalid expansion context: %s", raw)
				}
				queuedIDs = append(queuedIDs, task.VariantID)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			slices.Sort(queuedIDs)
			selected := slices.Clone(ids[:target])
			slices.Sort(selected)
			if !slices.Equal(queuedIDs, selected) {
				t.Fatalf("queued %v, want every selected variant %v", queuedIDs, selected)
			}
		})
	}
}

func seedCreativeCountSelection(t *testing.T, f creativeCandidateOrchestrationFixture, target int) []string {
	t.Helper()
	ids := make([]string, 0, target+2)
	for index := 1; index <= target+2; index++ {
		state, sizes := "selected", standardCreativeAssetSizes
		if index > target {
			state, sizes = "reserve", []string{"1080x1080"}
		}
		id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, fmt.Sprintf("C%02d", index), state, index, "completed", "1080x1080", sizes)
		ids = append(ids, id)
		addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
		addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "primed")
	}
	return ids
}

func TestCreativeVariantCountsCandidateFailuresNeverReduceTarget(t *testing.T) {
	for _, ready := range []int{9, 10, 11} {
		t.Run(fmt.Sprint(ready), func(t *testing.T) {
			f := createCreativeCountFixture(t, 10)
			var readyIDs []string
			for index := 1; index <= 12; index++ {
				status, taskStatus := "action_required", "failed"
				if index <= ready {
					status, taskStatus = "completed", "completed"
				}
				id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, fmt.Sprintf("C%02d", index), "candidate", nil, status, "1080x1080", []string{"1080x1080"})
				if index <= ready {
					addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
					addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "primed")
					readyIDs = append(readyIDs, id)
				}
				addCreativeCandidateOrchestrationProductionTask(t, f, id, taskStatus, "candidate_primary")
			}
			queued, err := testHandler.maybeQueueCreativeCandidateSelection(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)})
			if err != nil || queued != (ready >= 10) {
				t.Fatalf("%d ready selection = %v, %v", ready, queued, err)
			}
			if ready < 10 {
				return
			}
			selectCreativeCountCandidates(t, f, readyIDs, 10, http.StatusOK)
			committed, err := testHandler.creativeCandidateSelectionCommitted(t.Context(), parseUUID(testWorkspaceID), parseUUID(f.OrderID), parseUUID(f.ItemID))
			if err != nil || !committed {
				t.Fatalf("failed candidates blocked complete selection: %v, %v", committed, err)
			}
		})
	}
}

func TestCreativeVariantCountsRankTenReserveRecovery(t *testing.T) {
	f := createCreativeCountFixture(t, 10)
	ids := seedCreativeCountSelection(t, f, 10)
	for attempt := 0; attempt < 3; attempt++ {
		failedID := ids[9+attempt]
		if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET status = 'action_required' WHERE id = $1`, failedID); err != nil {
			t.Fatal(err)
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE agent_task_queue SET status = 'failed', completed_at = now() WHERE trigger_evidence_ref_id = $1 AND context->>'variant_id' = $2`, f.ItemID, failedID); err != nil {
			t.Fatal(err)
		}
		promoted, tasks, err := testHandler.maybePromoteCreativeReserve(t.Context(), parseUUID(failedID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)})
		if err != nil || promoted != (attempt < 2) {
			t.Fatalf("promotion %d = %v, %v", attempt, promoted, err)
		}
		if attempt == 2 {
			if len(tasks) != 0 {
				t.Fatal("exhausted reserves queued another task")
			}
			break
		}
		if len(tasks) != 1 {
			t.Fatalf("promotion queued %d tasks", len(tasks))
		}
		var rank int
		if err := testPool.QueryRow(t.Context(), `SELECT selection_rank FROM creative_order_variant WHERE id = $1 AND candidate_state = 'selected'`, ids[10+attempt]).Scan(&rank); err != nil || rank != 10 {
			t.Fatalf("replacement rank = %d, %v", rank, err)
		}
		committed, err := testHandler.creativeCandidateSelectionCommitted(t.Context(), parseUUID(testWorkspaceID), parseUUID(f.OrderID), parseUUID(f.ItemID))
		if err != nil || !committed {
			t.Fatalf("promotion broke ranking: %v, %v", committed, err)
		}
		promoted, tasks, err = testHandler.maybePromoteCreativeReserve(t.Context(), parseUUID(failedID), creativeOrchestrationCause{})
		if err != nil || promoted || len(tasks) != 0 {
			t.Fatalf("promotion replay = %v/%d, %v", promoted, len(tasks), err)
		}
	}
	var stable, promotionTasks int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_order_variant WHERE order_item_id = $1 AND selection_rank BETWEEN 1 AND 9 AND candidate_state = 'selected' AND status = 'completed' AND revision = 1`, f.ItemID).Scan(&stable); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM agent_task_queue WHERE trigger_evidence_ref_id = $1 AND context->>'production_phase' = $2`, f.ItemID, creativeReservePromotionPhase).Scan(&promotionTasks); err != nil {
		t.Fatal(err)
	}
	if stable != 9 || promotionTasks != 2 {
		t.Fatalf("stable=%d, promotion tasks=%d", stable, promotionTasks)
	}
}

func TestCreativeVariantCountsRetryOnlyMissingSizeOfTenthSet(t *testing.T) {
	f := createCreativeCountFixture(t, 10)
	ids := seedCreativeCountSelection(t, f, 10)
	for index, id := range ids[:10] {
		addCreativeCandidateOrchestrationAsset(t, id, "1200x628", "generated")
		status := "failed"
		if index < 9 {
			addCreativeCandidateOrchestrationAsset(t, id, "800x1000", "generated")
			status = "completed"
		}
		addCreativeCandidateOrchestrationProductionTask(t, f, id, status, creativeSelectedExpansionPhase)
	}
	var originalAssets []string
	if err := testPool.QueryRow(t.Context(), `SELECT array_agg(asset.id::text ORDER BY asset.id) FROM creative_order_asset asset JOIN creative_order_variant variant ON variant.id = asset.variant_id WHERE variant.order_item_id = $1`, f.ItemID).Scan(&originalAssets); err != nil {
		t.Fatal(err)
	}
	tasks, err := testHandler.queueSelectedCreativeProductionTasks(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{RequestedBy: parseUUID(testUserID)}, creativeSelectedExpansionPhase)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("missing-size retry = %d tasks, %v", len(tasks), err)
	}
	var task struct {
		VariantID string   `json:"variant_id"`
		Missing   []string `json:"missing_sizes"`
	}
	if err := json.Unmarshal(tasks[0].Context, &task); err != nil {
		t.Fatal(err)
	}
	if task.VariantID != ids[9] || !slices.Equal(task.Missing, []string{"800x1000"}) {
		t.Fatalf("retry expanded scope: %s", tasks[0].Context)
	}
	tasks, err = testHandler.queueSelectedCreativeProductionTasks(t.Context(), parseUUID(f.ItemID), creativeOrchestrationCause{}, creativeSelectedExpansionPhase)
	if err != nil || len(tasks) != 0 {
		t.Fatalf("retry replay = %d tasks, %v", len(tasks), err)
	}
	var unchanged bool
	if err := testPool.QueryRow(t.Context(), `SELECT array_agg(asset.id::text ORDER BY asset.id) = $2::text[] FROM creative_order_asset asset JOIN creative_order_variant variant ON variant.id = asset.variant_id WHERE variant.order_item_id = $1`, f.ItemID, originalAssets).Scan(&unchanged); err != nil || !unchanged {
		t.Fatalf("retry changed successful assets: %v", err)
	}
}

func TestCreativeVariantCountsDeliveryAndMetricsRequireTen(t *testing.T) {
	f := createCreativeCountFixture(t, 10)
	ids := seedCreativeCountSelection(t, f, 10)
	var baselineDuration pgtype.Int8
	var baselinePackages int
	if err := testPool.QueryRow(t.Context(), creativeInitialGeneratedPackageDurationSQL, parseUUID(testWorkspaceID)).Scan(&baselineDuration, &baselinePackages); err != nil {
		t.Fatal(err)
	}
	for index, id := range ids[:10] {
		for _, size := range standardCreativeAssetSizes {
			if size != "1080x1080" {
				addCreativeCandidateOrchestrationAsset(t, id, size, "generated")
			}
			addCreativeCandidateOrchestrationAsset(t, id, size, "delivered")
		}
		if _, err := testPool.Exec(t.Context(), `UPDATE creative_order_variant SET active_revision = 1, staging_revision = NULL WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		if index < 8 {
			continue
		}
		status, err := testHandler.derivedCreativeOrderDeliveryStatus(t.Context(), parseUUID(f.OrderID))
		wantStatus, wantPackages := "partial", baselinePackages
		if index == 9 {
			wantStatus, wantPackages = "awaiting_adoption", baselinePackages+1
		}
		if err != nil || status != wantStatus {
			t.Fatalf("%d sets status = %q, %v", index+1, status, err)
		}
		var duration pgtype.Int8
		var packages int
		if err := testPool.QueryRow(t.Context(), creativeInitialGeneratedPackageDurationSQL, parseUUID(testWorkspaceID)).Scan(&duration, &packages); err != nil {
			t.Fatal(err)
		}
		if packages != wantPackages {
			t.Fatalf("%d sets counted as %d complete packages", index+1, packages)
		}
	}
}
