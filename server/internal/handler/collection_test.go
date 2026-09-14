package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type collectionCaptureSpy struct{ names []string }

func (s *collectionCaptureSpy) Capture(event analytics.Event) {
	s.names = append(s.names, event.Name)
}
func (s *collectionCaptureSpy) Close() {}

func collectionMetricValue(t *testing.T, metrics *obsmetrics.BusinessMetrics, name string) float64 {
	t.Helper()
	family := obsmetrics.GatherForTest(t, metrics)[name]
	if family == nil || len(family.GetMetric()) != 1 {
		t.Fatalf("metric %s missing or has unexpected labels", name)
	}
	return family.GetMetric()[0].GetCounter().GetValue()
}

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
	previousMetrics, previousAnalytics, previousBus := testHandler.Metrics, testHandler.Analytics, testHandler.Bus
	metrics := obsmetrics.NewBusinessMetrics()
	analyticsSpy := &collectionCaptureSpy{}
	eventBus := events.New()
	published := make([]events.Event, 0)
	eventBus.SubscribeAll(func(event events.Event) { published = append(published, event) })
	testHandler.Metrics, testHandler.Analytics, testHandler.Bus = metrics, analyticsSpy, eventBus
	t.Cleanup(func() {
		testHandler.Metrics, testHandler.Analytics, testHandler.Bus = previousMetrics, previousAnalytics, previousBus
	})

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
	if got := collectionMetricValue(t, metrics, "multica_collection_created_total"); got != 1 {
		t.Fatalf("collection metric after success and replay = %v, want 1", got)
	}
	otherCollection := createCollectionForTest(t, "66666666-6666-4666-8666-666666666666")

	fields := map[string]any{
		collection.Fields[0].ID: "",
		collection.Fields[1].ID: 0,
		collection.Fields[2].ID: false,
	}
	invalidRecord := httptest.NewRecorder()
	testHandler.CreateCollectionRecord(invalidRecord, collectionRequest(http.MethodPost, "/api/collections/records", map[string]any{
		"client_request_id": "22222222-2222-4222-8222-222222222222",
		"title":             "First",
		"fields": map[string]any{
			collection.Fields[0].ID: nil,
		},
	}, map[string]string{"collectionId": collection.Collection.ID}))
	if invalidRecord.Code != http.StatusBadRequest {
		t.Fatalf("null record field: got %d: %s", invalidRecord.Code, invalidRecord.Body.String())
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
	if got := collectionMetricValue(t, metrics, "multica_record_created_total"); got != 1 {
		t.Fatalf("record metric after rollback and success = %v, want 1", got)
	}

	for name, change := range map[string]map[string]any{
		"missing":    {"field_id": "title", "op": "set"},
		"null":       {"field_id": "title", "op": "set", "value": nil},
		"wrong type": {"field_id": "title", "op": "set", "value": false},
	} {
		t.Run("reject title "+name, func(t *testing.T) {
			invalid := httptest.NewRecorder()
			testHandler.UpdateCollectionRecord(invalid, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
				"expected_revision": int64(1),
				"change":            change,
			}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
			if invalid.Code != http.StatusBadRequest {
				t.Fatalf("invalid title: got %d: %s", invalid.Code, invalid.Body.String())
			}
		})
	}

	update := httptest.NewRecorder()
	testHandler.UpdateCollectionRecord(update, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
		"expected_revision": int64(1),
		"change":            map[string]any{"field_id": "title", "op": "set", "value": ""},
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

	for name, change := range map[string]map[string]any{
		"null":        {"field_id": collection.Fields[0].ID, "op": "set", "value": nil},
		"wrong type":  {"field_id": collection.Fields[1].ID, "op": "set", "value": "0"},
		"clear value": {"field_id": collection.Fields[2].ID, "op": "clear", "value": nil},
		"unknown":     {"field_id": "99999999-9999-4999-8999-999999999999", "op": "set", "value": "x"},
		"cross collection": {
			"field_id": otherCollection.Fields[0].ID,
			"op":       "set",
			"value":    "x",
		},
	} {
		t.Run("reject custom field "+name, func(t *testing.T) {
			invalid := httptest.NewRecorder()
			testHandler.UpdateCollectionRecord(invalid, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
				"expected_revision": int64(2),
				"change":            change,
			}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
			if invalid.Code != http.StatusBadRequest {
				t.Fatalf("invalid custom field: got %d: %s", invalid.Code, invalid.Body.String())
			}
		})
	}

	revision := int64(2)
	for _, change := range []map[string]any{
		{"field_id": collection.Fields[0].ID, "op": "set", "value": ""},
		{"field_id": collection.Fields[1].ID, "op": "set", "value": 0},
		{"field_id": collection.Fields[2].ID, "op": "set", "value": false},
		{"field_id": collection.Fields[0].ID, "op": "clear"},
		{"field_id": collection.Fields[1].ID, "op": "clear"},
		{"field_id": collection.Fields[2].ID, "op": "clear"},
	} {
		custom := httptest.NewRecorder()
		testHandler.UpdateCollectionRecord(custom, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
			"expected_revision": revision,
			"change":            change,
		}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
		if custom.Code != http.StatusOK {
			t.Fatalf("custom field update: got %d: %s", custom.Code, custom.Body.String())
		}
		var response struct {
			Record CollectionRecordResponse `json:"record"`
		}
		if err := json.NewDecoder(custom.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}
		revision++
		if response.Record.Revision != revision {
			t.Fatalf("custom field revision = %d, want %d", response.Record.Revision, revision)
		}
		created.Record = response.Record
	}
	stored = nil
	if err := json.Unmarshal(created.Record.Fields, &stored); err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Fatalf("custom set/clear result mismatch: %#v", stored)
	}
	staleCustom := httptest.NewRecorder()
	testHandler.UpdateCollectionRecord(staleCustom, collectionRequest(http.MethodPatch, "/api/collections/records/record", map[string]any{
		"expected_revision": revision - 1,
		"change":            map[string]any{"field_id": collection.Fields[1].ID, "op": "set", "value": 99},
	}, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
	if staleCustom.Code != http.StatusConflict {
		t.Fatalf("stale custom update: got %d: %s", staleCustom.Code, staleCustom.Body.String())
	}
	reloaded := httptest.NewRecorder()
	testHandler.GetCollectionRecord(reloaded, collectionRequest(http.MethodGet, "/api/collections/records/record", nil, map[string]string{"collectionId": collection.Collection.ID, "recordId": created.Record.ID}))
	if reloaded.Code != http.StatusOK {
		t.Fatalf("reload custom fields: got %d: %s", reloaded.Code, reloaded.Body.String())
	}
	var reloadedResponse struct {
		Record CollectionRecordResponse `json:"record"`
	}
	if err := json.NewDecoder(reloaded.Body).Decode(&reloadedResponse); err != nil {
		t.Fatal(err)
	}
	if reloadedResponse.Record.Revision != revision || string(reloadedResponse.Record.Fields) != string(created.Record.Fields) {
		t.Fatalf("reloaded record mismatch: got %+v want revision=%d fields=%s", reloadedResponse.Record, revision, created.Record.Fields)
	}
	if got := collectionMetricValue(t, metrics, "multica_record_updated_total"); got != 7 {
		t.Fatalf("update metric after invalid requests, successes, and conflict = %v, want 7", got)
	}
	if len(analyticsSpy.names) != 0 {
		t.Fatalf("metrics-only collection events reached external analytics: %v", analyticsSpy.names)
	}
	eventCounts := map[string]int{}
	for _, event := range published {
		eventCounts[event.Type]++
		if event.WorkspaceID != testWorkspaceID {
			t.Fatalf("event routed to workspace %q", event.WorkspaceID)
		}
		if event.Type == protocol.EventRecordUpdated {
			payload, ok := event.Payload.(map[string]any)
			if !ok || len(payload) != 3 || payload["collection_id"] != collection.Collection.ID || payload["record_id"] != created.Record.ID || payload["revision"] == nil {
				t.Fatalf("record event leaked or omitted payload fields: %#v", event.Payload)
			}
		}
	}
	if eventCounts[protocol.EventCollectionCreated] != 2 || eventCounts[protocol.EventRecordCreated] != 1 || eventCounts[protocol.EventRecordUpdated] != 7 {
		t.Fatalf("unexpected collection event counts: %#v", eventCounts)
	}
	var auditCount int
	var auditLeakedValue bool
	if err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*), COALESCE(bool_or(details ? 'value'), false)
		FROM activity_log
		WHERE workspace_id = $1
		  AND action = 'record_updated'
		  AND details->>'collection_id' = $2
		  AND details->>'record_id' = $3
	`, testWorkspaceID, collection.Collection.ID, created.Record.ID).Scan(&auditCount, &auditLeakedValue); err != nil {
		t.Fatal(err)
	}
	if auditCount != 7 || auditLeakedValue {
		t.Fatalf("record update audit count/value leakage = %d/%v", auditCount, auditLeakedValue)
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
