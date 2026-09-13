package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/featureflags"
)

type createCollectionTestResponse struct {
	Collection CollectionResponse        `json:"collection"`
	Fields     []CollectionFieldResponse `json:"fields"`
	Replayed   bool                      `json:"replayed"`
}

type createCollectionRecordTestResponse struct {
	Record   CollectionRecordResponse `json:"record"`
	Replayed bool                     `json:"replayed"`
}

func collectionRequest(method, path string, body any, params map[string]string) *http.Request {
	req := newRequest(method, path, body)
	route := chi.NewRouteContext()
	for key, value := range params {
		route.URLParams.Add(key, value)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, route))
}

func cleanupTestCollections(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		tx, err := testPool.Begin(context.Background())
		if err != nil {
			return
		}
		defer tx.Rollback(context.Background())
		if _, err = tx.Exec(context.Background(), `SELECT set_config('app.workspace_id', $1, true)`, testWorkspaceID); err != nil {
			return
		}
		_, _ = tx.Exec(context.Background(), `DELETE FROM record WHERE workspace_id = $1`, testWorkspaceID)
		_, _ = tx.Exec(context.Background(), `DELETE FROM collection_field WHERE workspace_id = $1`, testWorkspaceID)
		_, _ = tx.Exec(context.Background(), `DELETE FROM collection WHERE workspace_id = $1`, testWorkspaceID)
		_ = tx.Commit(context.Background())
	})
}

func createCollectionForTest(t *testing.T, requestID string) createCollectionTestResponse {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.CreateCollection(w, newRequest(http.MethodPost, "/api/collections", map[string]any{
		"client_request_id": requestID,
		"name":              "Orders",
		"fields": []map[string]any{
			{"name": "Note", "type": "text"},
			{"name": "Quantity", "type": "number"},
			{"name": "Checked", "type": "checkbox"},
		},
	}))
	if w.Code != http.StatusCreated {
		t.Fatalf("create collection: got %d: %s", w.Code, w.Body.String())
	}
	var response createCollectionTestResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode collection: %v", err)
	}
	return response
}

func TestCollectionT2a_CreateReplayRecordCASAndIsolation(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	withFeatureFlag(t, testHandler, featureflags.CortexCollections, true)
	cleanupTestCollections(t)

	var issuesBefore, tasksBefore, inboxBefore int
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM issue WHERE workspace_id = $1`, testWorkspaceID).Scan(&issuesBefore); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM agent_task_queue`).Scan(&tasksBefore); err != nil {
		t.Fatal(err)
	}
	if err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM inbox_item WHERE workspace_id = $1`, testWorkspaceID).Scan(&inboxBefore); err != nil {
		t.Fatal(err)
	}

	collection := createCollectionForTest(t, "11111111-1111-4111-8111-111111111111")
	replay := httptest.NewRecorder()
	testHandler.CreateCollection(replay, newRequest(http.MethodPost, "/api/collections", map[string]any{
		"client_request_id": "11111111-1111-4111-8111-111111111111",
		"name":              "Orders",
		"fields": []map[string]any{
			{"name": "Note", "type": "text"},
			{"name": "Quantity", "type": "number"},
			{"name": "Checked", "type": "checkbox"},
		},
	}))
	if replay.Code != http.StatusOK {
		t.Fatalf("replay collection: got %d: %s", replay.Code, replay.Body.String())
	}
	var replayed createCollectionTestResponse
	if err := json.NewDecoder(replay.Body).Decode(&replayed); err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Collection.ID != collection.Collection.ID {
		t.Fatalf("unexpected replay: %+v", replayed)
	}

	fields := map[string]any{
		collection.Fields[0].ID: "",
		collection.Fields[1].ID: 0,
		collection.Fields[2].ID: false,
	}
	createRecord := httptest.NewRecorder()
	testHandler.CreateCollectionRecord(createRecord, collectionRequest(http.MethodPost, "/api/collections/records", map[string]any{
		"client_request_id": "22222222-2222-4222-8222-222222222222",
		"title":             "First",
		"fields":            fields,
	}, map[string]string{"collectionId": collection.Collection.ID}))
	if createRecord.Code != http.StatusCreated {
		t.Fatalf("create record: got %d: %s", createRecord.Code, createRecord.Body.String())
	}
	var created createCollectionRecordTestResponse
	if err := json.NewDecoder(createRecord.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(created.Record.Fields, &stored); err != nil {
		t.Fatal(err)
	}
	if stored[collection.Fields[0].ID] != "" || stored[collection.Fields[1].ID] != float64(0) || stored[collection.Fields[2].ID] != false {
		t.Fatalf("falsy values changed: %#v", stored)
	}

	update := httptest.NewRecorder()
	testHandler.UpdateCollectionRecord(update, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
		"expected_revision": int64(1),
		"change":            map[string]any{"field_id": "title", "op": "set", "value": "Changed"},
	}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
	if update.Code != http.StatusOK {
		t.Fatalf("update record: got %d: %s", update.Code, update.Body.String())
	}
	conflict := httptest.NewRecorder()
	testHandler.UpdateCollectionRecord(conflict, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
		"expected_revision": int64(1),
		"change":            map[string]any{"field_id": "title", "op": "set", "value": "Stale"},
	}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
	if conflict.Code != http.StatusConflict {
		t.Fatalf("stale update: got %d: %s", conflict.Code, conflict.Body.String())
	}

	otherWorkspace := dbfx.Workspace(t, "Collection Other Workspace", "collection-other")
	crossTenant := httptest.NewRecorder()
	req := collectionRequest(http.MethodGet, "/api/collections/id", nil, map[string]string{"collectionId": collection.Collection.ID})
	req.Header.Set("X-Workspace-ID", otherWorkspace)
	testHandler.GetCollection(crossTenant, req)
	if crossTenant.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant read: got %d: %s", crossTenant.Code, crossTenant.Body.String())
	}

	var issuesAfter, tasksAfter, inboxAfter int
	_ = testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM issue WHERE workspace_id = $1`, testWorkspaceID).Scan(&issuesAfter)
	_ = testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM agent_task_queue`).Scan(&tasksAfter)
	_ = testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM inbox_item WHERE workspace_id = $1`, testWorkspaceID).Scan(&inboxAfter)
	if issuesAfter != issuesBefore || tasksAfter != tasksBefore || inboxAfter != inboxBefore {
		t.Fatalf("record leaked into issue/run/inbox: before=%d/%d/%d after=%d/%d/%d", issuesBefore, tasksBefore, inboxBefore, issuesAfter, tasksAfter, inboxAfter)
	}
}

func TestCollectionT2a_HumanRoleAndMachineCredentialGuards(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	withFeatureFlag(t, testHandler, featureflags.CortexCollections, true)
	cleanupTestCollections(t)
	collection := createCollectionForTest(t, "33333333-3333-4333-8333-333333333333")

	machine := httptest.NewRecorder()
	machineReq := newRequest(http.MethodGet, "/api/collections", nil)
	machineReq.Header.Set("X-Actor-Source", "task_token")
	testHandler.ListCollections(machine, machineReq)
	if machine.Code != http.StatusForbidden {
		t.Fatalf("machine credential: got %d: %s", machine.Code, machine.Body.String())
	}

	memberID := createPermissionTestMember(t, "collection-member@multica.ai")
	createDenied := httptest.NewRecorder()
	memberCreate := newRequest(http.MethodPost, "/api/collections", map[string]any{
		"client_request_id": "44444444-4444-4444-8444-444444444444",
		"name":              "Denied",
		"fields":            []map[string]any{{"name": "Note", "type": "text"}},
	})
	memberCreate.Header.Set("X-User-ID", memberID)
	testHandler.CreateCollection(createDenied, memberCreate)
	if createDenied.Code != http.StatusForbidden {
		t.Fatalf("member create collection: got %d: %s", createDenied.Code, createDenied.Body.String())
	}

	memberRecord := httptest.NewRecorder()
	memberReq := collectionRequest(http.MethodPost, "/api/collections/records", map[string]any{
		"client_request_id": "55555555-5555-4555-8555-555555555555",
		"title":             "Member record",
		"fields":            map[string]any{},
	}, map[string]string{"collectionId": collection.Collection.ID})
	memberReq.Header.Set("X-User-ID", memberID)
	testHandler.CreateCollectionRecord(memberRecord, memberReq)
	if memberRecord.Code != http.StatusCreated {
		t.Fatalf("member create record: got %d: %s", memberRecord.Code, memberRecord.Body.String())
	}
}

func TestCollectionT2a_RLSRejectsCrossWorkspaceReadAndWrite(t *testing.T) {
	if testPool == nil {
		t.Skip("database not available")
	}
	otherWorkspace := dbfx.Workspace(t, "Collection RLS Workspace", "collection-rls")
	var collectionID string
	seed, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer seed.Rollback(context.Background())
	if _, err = seed.Exec(context.Background(), `SELECT set_config('app.workspace_id', $1, true)`, otherWorkspace); err != nil {
		t.Fatal(err)
	}
	if err = seed.QueryRow(context.Background(), `
		INSERT INTO collection (
			workspace_id, name, created_by, create_request_id, create_fingerprint
		) VALUES ($1, 'RLS secret', $2, gen_random_uuid(), repeat('a', 64))
		RETURNING id
	`, otherWorkspace, testUserID).Scan(&collectionID); err != nil {
		t.Fatal(err)
	}
	if err = seed.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = testPool.Exec(context.Background(), `CREATE ROLE multica_collection_rls_test NOLOGIN`); err != nil {
		t.Fatal(err)
	}
	if _, err = testPool.Exec(context.Background(), `
		GRANT USAGE ON SCHEMA public TO multica_collection_rls_test;
		GRANT SELECT ON collection TO multica_collection_rls_test;
		GRANT INSERT ON record TO multica_collection_rls_test;
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DROP OWNED BY multica_collection_rls_test; DROP ROLE multica_collection_rls_test`)
	})
	t.Cleanup(func() {
		tx, beginErr := testPool.Begin(context.Background())
		if beginErr != nil {
			return
		}
		defer tx.Rollback(context.Background())
		_, _ = tx.Exec(context.Background(), `SELECT set_config('app.workspace_id', $1, true)`, otherWorkspace)
		_, _ = tx.Exec(context.Background(), `DELETE FROM collection WHERE workspace_id = $1`, otherWorkspace)
		_ = tx.Commit(context.Background())
	})

	tx, err := testPool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(context.Background(), `SET LOCAL ROLE multica_collection_rls_test`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(context.Background(), `SELECT set_config('app.workspace_id', $1, true)`, testWorkspaceID); err != nil {
		t.Fatal(err)
	}
	var visible int
	if err = tx.QueryRow(context.Background(), `SELECT COUNT(*) FROM collection WHERE id = $1`, collectionID).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 0 {
		t.Fatalf("cross-workspace collection visible under RLS: %d", visible)
	}
	if _, err = tx.Exec(context.Background(), `
		INSERT INTO record (
			workspace_id, collection_id, title, fields, created_by, updated_by,
			create_request_id, create_fingerprint
		) VALUES ($1, $2, 'cross tenant', '{}'::jsonb, $3, $3, gen_random_uuid(), repeat('b', 64))
	`, otherWorkspace, collectionID, testUserID); err == nil {
		t.Fatal("cross-workspace record insert unexpectedly passed RLS")
	}
}
