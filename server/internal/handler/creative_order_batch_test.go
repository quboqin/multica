package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

type creativeQueryCounter struct {
	dbExecutor
	queries int
}

func (c *creativeQueryCounter) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.queries++
	return c.dbExecutor.Query(ctx, sql, args...)
}
func (c *creativeQueryCounter) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	c.queries++
	return c.dbExecutor.QueryRow(ctx, sql, args...)
}

func TestCreativeOrderListBatchKeepsAllOrdersAndHistoryInFiveQueries(t *testing.T) {
	fixtures := []creativeCandidateOrchestrationFixture{createCreativeCountFixture(t, 1), createCreativeCountFixture(t, 6), createCreativeCountFixture(t, 10)}
	for _, f := range fixtures {
		seedSixSetCandidatePrimaries(t, f, 3)
	}
	original := testHandler.DB
	counter := &creativeQueryCounter{dbExecutor: original}
	testHandler.DB = counter
	t.Cleanup(func() { testHandler.DB = original })
	w := httptest.NewRecorder()
	r := newRequest(http.MethodGet, "/api/creative/orders", nil)
	testHandler.ListCreativeOrders(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("list=%d %s", w.Code, w.Body.String())
	}
	if counter.queries != 5 {
		t.Fatalf("list queries=%d, want 5 independent of order count", counter.queries)
	}
	var response struct {
		Orders []creativeOrderResponse `json:"orders"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	for _, f := range fixtures {
		found := false
		for _, o := range response.Orders {
			if o.ID == f.OrderID {
				found = true
				if len(o.Items) != 1 || len(o.Items[0].Variants) != 3 || len(o.Items[0].Variants[0].Assets) != 2 {
					t.Fatalf("incomplete list projection: %+v", o)
				}
			}
		}
		if !found {
			t.Fatal("order missing from list")
		}
	}
}

func TestCreativeVariantBatchPreservesAssetsRevisionsAndBlockers(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	ids := seedSixSetCandidatePrimaries(t, f, 8)
	r := newRequest(http.MethodGet, "/api/creative/orders/"+f.OrderID, nil)
	original := testHandler.DB
	counter := &creativeQueryCounter{dbExecutor: original}
	testHandler.DB = counter
	t.Cleanup(func() { testHandler.DB = original })
	variants, err := testHandler.listCreativeOrderVariants(r, parseUUID(f.ItemID))
	if err != nil {
		t.Fatal(err)
	}
	if counter.queries > 10 {
		t.Fatalf("detail queries=%d grow with variant count", counter.queries)
	}
	if len(variants) != 8 {
		t.Fatal("variants lost")
	}
	for i, v := range variants {
		oldAssets, err := testHandler.listCreativeOrderAssets(r, parseUUID(ids[i]))
		if err != nil {
			t.Fatal(err)
		}
		oldRevisions, err := testHandler.listCreativeOrderVariantRevisions(t.Context(), parseUUID(ids[i]))
		if err != nil {
			t.Fatal(err)
		}
		oldBlocker, err := testHandler.loadCreativeOrderVariantBlocker(r, parseUUID(ids[i]))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(v.Assets, oldAssets) || !reflect.DeepEqual(v.Revisions, oldRevisions) {
			t.Fatalf("batch changed variant %s payload", v.ID)
		}
		if v.Status != "completed" && !reflect.DeepEqual(v.ActionRequired, oldBlocker) {
			t.Fatal("blocker changed")
		}
	}
}
