package handler

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestCollectionAtomicFieldsAndStaleSameField(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	fieldA := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Number", "type": "number"})
	fieldB := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Date", "type": "date"})
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Independent fields"})
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for field, value := range map[string]any{fieldA: 42, fieldB: "2026-09-18"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := newRequest("PUT", "/api/collections", map[string]any{"value": value, "expected_value": nil})
			req = withURLParams(req, "collectionID", collection, "recordID", record, "fieldID", field)
			response := httptest.NewRecorder()
			testHandler.SetCollectionRecordField(response, req)
			statuses <- response.Code
		}()
	}
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != 200 {
			t.Fatalf("concurrent field write=%d", status)
		}
	}
	var raw []byte
	dbfx.QueryRow(t, "SELECT fields FROM record WHERE id=$1", record).Scan(&raw)
	var values map[string]any
	if json.Unmarshal(raw, &values) != nil || values[fieldA] != float64(42) || values[fieldB] != "2026-09-18" {
		t.Fatalf("lost concurrent field value: %s", raw)
	}
	req := withURLParams(newRequest("PUT", "/api/collections", map[string]any{"value": 99, "expected_value": nil}), "collectionID", collection, "recordID", record, "fieldID", fieldA)
	testutil.Call(t, testHandler.SetCollectionRecordField, req).Want(409)
}
func TestCollectionPaginationFilterCountsAndCursorScope(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	number := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Value", "type": "number"})
	for i := range 65 {
		raw, _ := json.Marshal(map[string]any{number: i})
		dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": fmt.Sprintf("Row %d", i), "fields": string(raw)})
	}
	type page struct {
		Records []struct {
			ID string `json:"id"`
		} `json:"records"`
		Total int     `json:"total"`
		Next  *string `json:"next_cursor"`
	}
	fetch := func(query string, status int) page {
		t.Helper()
		var out page
		result := testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records?"+query, nil), "collectionID", collection)).Want(status)
		if status == 200 {
			result.JSON(&out)
		}
		return out
	}
	first := fetch("limit=20", 200)
	if first.Total != 65 || len(first.Records) != 20 || first.Next == nil {
		t.Fatalf("first page=%+v", first)
	}
	dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "New head after page one"})
	second := fetch("limit=20&cursor="+url.QueryEscape(*first.Next), 200)
	seen := map[string]bool{}
	for _, record := range first.Records {
		seen[record.ID] = true
	}
	for _, record := range second.Records {
		if seen[record.ID] {
			t.Fatal("cursor page duplicated a row after a concurrent insert")
		}
	}
	focused := fetch("record_id="+first.Records[0].ID, 200)
	if focused.Total != 1 || len(focused.Records) != 1 || focused.Records[0].ID != first.Records[0].ID {
		t.Fatalf("focused row: %+v", focused)
	}
	other := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": "Other source"})
	outside := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": other, "title": "Outside source"})
	if result := fetch("record_id="+outside, 200); result.Total != 0 {
		t.Fatal("focused read crossed collection boundary")
	}
	filter, _ := json.Marshal(map[string]any{number: []any{map[string]any{"op": "gte", "value": "60"}}})
	filtered := fetch("properties="+url.QueryEscape(string(filter)), 200)
	if filtered.Total != 5 || len(filtered.Records) != 5 {
		t.Fatalf("filtered count=%+v", filtered)
	}
	fetch("search=different&cursor="+url.QueryEscape(*first.Next), 400)
	unknown, _ := json.Marshal(map[string]any{number: []any{map[string]any{"op": "mystery", "value": "1"}}})
	fetch("properties="+url.QueryEscape(string(unknown)), 400)
}
func TestCollectionFieldQuotaIsIndependent(t *testing.T) {
	for table := range 3 {
		collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": fmt.Sprintf("Independent %d", table)})
		for i := range 30 {
			dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": fmt.Sprintf("Field %d", i), "type": "text"})
		}
		request := withURLParam(newRequest("POST", "/api/collections/fields", map[string]any{"name": "Another", "type": "checkbox"}), "collectionID", collection)
		var field struct {
			ID string `json:"id"`
		}
		testutil.Call(t, testHandler.CreateCollectionField, request).Want(201).JSON(&field)
		dbfx.Cleanup(t, "DELETE FROM collection_field WHERE id=$1", field.ID)
	}
}

func TestCollectionGroupsHaveIndependentCursorsAndScopedViews(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	group := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Stage", "type": "select", "config": `{"options":[{"id":"a","name":"A","color":"#6b7280"},{"id":"b","name":"B","color":"#6b7280"}]}`})
	for _, key := range []string{"a", "b"} {
		for i := range 23 {
			raw, _ := json.Marshal(map[string]any{group: key})
			dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": fmt.Sprintf("%s %d", key, i), "fields": string(raw)})
		}
	}
	type result struct {
		Records []struct {
			ID     string            `json:"id"`
			Fields map[string]string `json:"fields"`
		} `json:"records"`
		Next   *string `json:"next_cursor"`
		Groups []struct {
			Key   string `json:"key"`
			Count int    `json:"count"`
		} `json:"groups"`
		Total int `json:"total"`
	}
	fetch := func(key, cursor string, status int) result {
		t.Helper()
		var out result
		q := "?group_by=" + group + "&group_key=" + key + "&limit=10"
		if cursor != "" {
			q += "&cursor=" + url.QueryEscape(cursor)
		}
		call := testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records"+q, nil), "collectionID", collection)).Want(status)
		if status == 200 {
			call.JSON(&out)
		}
		return out
	}
	a, b := fetch("a", "", 200), fetch("b", "", 200)
	if a.Total != 46 || len(a.Groups) != 2 || a.Groups[0].Count != 23 || a.Groups[1].Count != 23 || a.Next == nil || b.Next == nil {
		t.Fatalf("incorrect group counts: %+v", a)
	}
	next := fetch("a", *a.Next, 200)
	seen := map[string]bool{}
	for _, row := range a.Records {
		seen[row.ID] = true
	}
	for _, row := range next.Records {
		if seen[row.ID] || row.Fields[group] != "a" {
			t.Fatal("group cursor repeated or crossed groups")
		}
	}
	fetch("b", *a.Next, 400)
	// Collection views cannot leak into the old NULL task-view list.
	var view IssueViewResponse
	testutil.Call(t, testHandler.CreateIssueView, newRequest("POST", "/api/issue-views", map[string]any{"name": "Private collection view", "collection_id": collection, "scope_type": "workspace", "visibility": "private", "query": map[string]any{}, "display": map[string]any{}})).Want(201).JSON(&view)
	dbfx.Cleanup(t, "DELETE FROM issue_view WHERE id=$1", view.ID)
	var taskViews []IssueViewResponse
	testutil.Call(t, testHandler.ListIssueViews, newRequest("GET", "/api/issue-views?scope_type=workspace", nil)).Want(200).JSON(&taskViews)
	for _, item := range taskViews {
		if item.ID == view.ID {
			t.Fatal("collection view appeared in NULL task views")
		}
	}
	_, _, member := privateAgentTestFixture(t)
	testutil.Call(t, testHandler.GetIssueViewByID, withURLParam(newRequestAs(member, "GET", "/api/issue-views/"+view.ID, nil), "id", view.ID)).Want(404)
	testutil.Call(t, testHandler.CreateCollectionField, withURLParam(newRequestAs(member, "POST", "/api/collections/fields", map[string]any{"name": "Forbidden", "type": "text"}), "collectionID", collection)).Want(404)
}
