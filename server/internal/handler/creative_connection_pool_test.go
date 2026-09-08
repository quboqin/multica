package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func creativeSmallPoolHandler(t *testing.T, connections int32) (*Handler, *pgxpool.Pool) {
	t.Helper()
	cfg := testPool.Config()
	cfg.MaxConns = connections
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	h := New(db.New(pool), pool, testHandler.Hub, testHandler.Bus, testHandler.EmailService, testHandler.Storage, testHandler.CFSigner, testHandler.Analytics, Config{})
	return h, pool
}

func TestCreativeConnectionsSingleConnectionCanReadDashboardAndDispatch(t *testing.T) {
	f := createCreativeCountFixture(t, 1)
	id := createCreativeCandidateOrchestrationVariant(t, f.ItemID, "C01", "selected", 1, "partial", "1080x1080", standardCreativeAssetSizes)
	addCreativeCandidateOrchestrationAsset(t, id, "1080x1080", "generated")
	h, pool := creativeSmallPoolHandler(t, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := h.creativeFeedbackWorkflowDashboard(ctx, parseUUID(testWorkspaceID)); err != nil {
		t.Fatal(err)
	}
	tasks, err := h.queueSelectedCreativeProductionTasks(ctx, parseUUID(f.ItemID), creativeOrchestrationCause{}, creativeSelectedExpansionPhase)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("single-connection dispatch=%d %v", len(tasks), err)
	}
	if pool.Stat().AcquiredConns() != 0 {
		t.Fatal("dispatch retained a connection")
	}
}

func TestCreativeConnectionsInputValidationUsesOrderTransaction(t *testing.T) {
	store := &mockStorage{}
	attachmentID := seedPreviewAttachment(t, store, "pool/input.png", "input.png", "image/png", []byte("image body"))
	h, pool := creativeSmallPoolHandler(t, 1)
	h.Storage = store
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	createResource := func(kind string, config json.RawMessage) string {
		t.Helper()
		var id string
		if err := tx.QueryRow(ctx, `INSERT INTO creative_resource(workspace_id,kind,name,status,version,published_version,config,created_by)
VALUES($1,$2,'Single connection input','published',2,2,$3,$4) RETURNING id::text`, testWorkspaceID, kind, config, testUserID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO creative_resource_revision(resource_id,version,name,config,created_by)
VALUES($1,2,'Single connection input',$2,$3)`, id, config, testUserID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	libraryID := createResource("copy_library", copyOrderTestLibrary().Config)
	marketID := createResource("market_pack", json.RawMessage(fmt.Sprintf(`{"copy_library_id":%q}`, libraryID)))
	input := creativeOrderInput{
		InputSnapshot: json.RawMessage(fmt.Sprintf(`{"market_pack":{"id":%q,"version":2}}`, marketID)),
		Items: []creativeOrderItemInput{{SourceKind: "copy_library", CopyLibraryID: libraryID,
			CopySnapshot: json.RawMessage(`{"library_version":2,"creative_type":"num","slots":{"subheadline":["headline-1"]}}`)}},
	}
	if err := freezeCreativeCopyLibraryOrder(ctx, tx, parseUUID(testWorkspaceID), &input); err != nil {
		t.Fatal(err)
	}
	var frozen creativeOrderCopySnapshot
	if err := json.Unmarshal(input.Items[0].CopySnapshot, &frozen); err != nil || frozen.Subheadline != "Flexible financing" {
		t.Fatalf("published copy was not frozen: %s, %v", input.Items[0].CopySnapshot, err)
	}
	var fileID string
	if err := tx.QueryRow(ctx, `INSERT INTO creative_resource_file(resource_id,workspace_id,attachment_id,role,created_version,created_by)
VALUES($1,$2,$3,'prime_template',2,$4) RETURNING id::text`, marketID, testWorkspaceID, attachmentID, testUserID).Scan(&fileID); err != nil {
		t.Fatal(err)
	}
	if err := h.validateCreativeOrderResourceAttachment(ctx, tx, parseUUID(testWorkspaceID), parseUUID(marketID), 2,
		creativeResourceFileResponse{ID: fileID, AttachmentID: attachmentID}, "Prime template"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if pool.Stat().AcquiredConns() != 0 || pool.Stat().CanceledAcquireCount() != 0 {
		t.Fatal("input validation acquired a second connection or retained the transaction")
	}
}

func TestCreativeConnectionsFiftyConcurrentReadsAndHandoffs(t *testing.T) {
	f := createCreativeCountFixture(t, 6)
	seedSixSetCandidatePrimaries(t, f, 8)
	h, pool := creativeSmallPoolHandler(t, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	results := make(chan error, 50)
	for i := range 50 {
		go func(index int) {
			switch index % 4 {
			case 0:
				_, err := h.maybeQueueCreativeCandidateSelection(ctx, parseUUID(f.ItemID), creativeOrchestrationCause{})
				results <- err
			case 1:
				_, err := h.creativeFeedbackWorkflowDashboard(ctx, parseUUID(testWorkspaceID))
				results <- err
			case 2:
				request := newRequest(http.MethodGet, "/api/creative/orders", nil)
				requestCtx, done := context.WithTimeout(request.Context(), 20*time.Second)
				defer done()
				request = request.WithContext(requestCtx)
				w := httptest.NewRecorder()
				h.ListCreativeOrders(w, request)
				if w.Code != http.StatusOK {
					results <- fmt.Errorf("list %d: %s", w.Code, w.Body.String())
				} else {
					results <- nil
				}
			default:
				request := withURLParam(newRequest(http.MethodGet, "/api/creative/orders/"+f.OrderID, nil), "id", f.OrderID)
				requestCtx, done := context.WithTimeout(request.Context(), 20*time.Second)
				defer done()
				request = request.WithContext(requestCtx)
				w := httptest.NewRecorder()
				h.GetCreativeOrder(w, request)
				if w.Code != http.StatusOK {
					results <- fmt.Errorf("detail %d: %s", w.Code, w.Body.String())
				} else {
					results <- nil
				}
			}
		}(i)
	}
	for range 50 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if pool.Stat().AcquiredConns() != 0 || pool.Stat().CanceledAcquireCount() != 0 {
		t.Fatalf("pool did not drain: acquired=%d cancelled=%d", pool.Stat().AcquiredConns(), pool.Stat().CanceledAcquireCount())
	}
	var selections int
	if err := testPool.QueryRow(t.Context(), `SELECT count(*) FROM creative_task_binding WHERE order_id=$1 AND workflow='creative_candidate_selection'`, f.OrderID).Scan(&selections); err != nil {
		t.Fatal(err)
	}
	if selections != 1 {
		t.Fatalf("concurrent handoffs created %d selection tasks", selections)
	}
}

func TestCreativeConnectionsRecoveryBudgetRaceReleasesTransaction(t *testing.T) {
	enableCreativeFactoryForTest(t)
	f := createCreativeCountFixture(t, 1)
	seedSixSetCandidatePrimaries(t, f, 3)
	h, pool := creativeSmallPoolHandler(t, 1)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := h.discoverCreativeOrderRecovery(ctx, parseUUID(f.OrderID)); err != nil {
		t.Fatal(err)
	}
	target, found, err := h.claimCreativeRecovery(ctx)
	if err != nil || !found || target.OrderID != parseUUID(f.OrderID) {
		t.Fatalf("recovery claim=%v %v", found, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE creative_recovery SET attempt=max_attempts WHERE id=$1`, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := h.processCreativeRecovery(ctx, target); err != nil {
		t.Fatal(err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM creative_recovery WHERE id=$1`, target.ID).Scan(&status); err != nil || status != "manual_required" {
		t.Fatalf("exhausted recovery=%s %v", status, err)
	}
	if pool.Stat().AcquiredConns() != 0 || pool.Stat().CanceledAcquireCount() != 0 {
		t.Fatal("recovery attempted to finish while retaining its transaction")
	}
}
