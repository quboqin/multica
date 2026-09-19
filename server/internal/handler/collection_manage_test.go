package handler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

const (
	optionRed   = "11111111-1111-4111-8111-111111111111"
	optionGreen = "22222222-2222-4222-8222-222222222222"
	optionBlue  = "33333333-3333-4333-8333-333333333333"
)

func collectionOptionsConfig() string {
	return fmt.Sprintf(`{"options":[{"id":%q,"name":"Red","color":"#dc2626"},{"id":%q,"name":"Green","color":"#16a34a"},{"id":%q,"name":"Blue","color":"#2563eb"}]}`, optionRed, optionGreen, optionBlue)
}

func recordFields(t *testing.T, id string) map[string]any {
	t.Helper()
	var raw []byte
	dbfx.QueryRow(t, "SELECT fields FROM record WHERE id=$1", id).Scan(&raw)
	values := map[string]any{}
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	return values
}

func patchField(t *testing.T, collection, field string, body map[string]any) *testutil.Response {
	t.Helper()
	request := withURLParams(newRequest("PATCH", "/api/collections/fields", body), "collectionID", collection, "fieldID", field)
	return testutil.Call(t, testHandler.UpdateCollectionField, request)
}

func TestCollectionFieldOptionsCanBeEditedAndRemoved(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	single := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Single", "type": "select", "config": collectionOptionsConfig()})
	multi := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Multi", "type": "multi_select", "config": collectionOptionsConfig()})
	raw, _ := json.Marshal(map[string]any{single: optionBlue, multi: []string{optionRed, optionBlue}})
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Row", "fields": string(raw)})
	onlyBlue, _ := json.Marshal(map[string]any{multi: []string{optionBlue}})
	emptied := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Only blue", "fields": string(onlyBlue)})

	options := []map[string]any{
		{"id": optionRed, "name": "Crimson", "color": "#b91c1c"},
		{"id": optionGreen, "name": "Green", "color": "#16a34a"},
		{"name": "Yellow", "color": "#ca8a04"},
	}
	var updated struct {
		Config struct {
			Options []struct {
				ID, Name, Color string
			} `json:"options"`
		} `json:"config"`
	}
	patchField(t, collection, single, map[string]any{"config": map[string]any{"options": options}}).Want(200).JSON(&updated)
	if len(updated.Config.Options) != 3 || updated.Config.Options[0].Name != "Crimson" || updated.Config.Options[2].ID == "" {
		t.Fatalf("options not updated: %+v", updated)
	}
	patchField(t, collection, multi, map[string]any{"config": map[string]any{"options": options}}).Want(200)

	values := recordFields(t, record)
	if _, ok := values[single]; ok {
		t.Fatalf("removed select option still stored: %v", values)
	}
	if list, _ := values[multi].([]any); len(list) != 1 || list[0] != optionRed {
		t.Fatalf("removed multi-select option not filtered: %v", values)
	}
	if _, ok := recordFields(t, emptied)[multi]; ok {
		t.Fatal("multi-select with no remaining options must be unset")
	}
	patchField(t, collection, single, map[string]any{"name": "Renamed", "position": 5}).Want(200)
	patchField(t, collection, single, map[string]any{"config": map[string]any{"options": []any{}}}).Want(400)

	_, _, member := privateAgentTestFixture(t)
	request := withURLParams(newRequestAs(member, "PATCH", "/api/collections/fields", map[string]any{"name": "Nope"}), "collectionID", collection, "fieldID", single)
	testutil.Call(t, testHandler.UpdateCollectionField, request).Want(403)
}

func TestCollectionFieldTypeChangesOnlyAlongSafePaths(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	status := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Status", "type": "select", "config": collectionOptionsConfig()})
	count := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Count", "type": "number"})
	note := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Note", "type": "text"})
	raw, _ := json.Marshal(map[string]any{status: optionGreen, count: 7})
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Row", "fields": string(raw)})

	patchField(t, collection, status, map[string]any{"type": "multi_select"}).Want(200)
	patchField(t, collection, count, map[string]any{"type": "text"}).Want(200)
	values := recordFields(t, record)
	if list, _ := values[status].([]any); len(list) != 1 || list[0] != optionGreen {
		t.Fatalf("select was not wrapped: %v", values)
	}
	if values[count] != "7" {
		t.Fatalf("number was not kept as text: %v", values)
	}
	patchField(t, collection, note, map[string]any{"type": "url"}).Want(400)
	patchField(t, collection, note, map[string]any{"type": "date"}).Want(400)

	patchField(t, collection, note, map[string]any{"archived": true}).Want(200)
	var detail struct {
		Fields []struct {
			ID string `json:"id"`
		} `json:"fields"`
	}
	testutil.Call(t, testHandler.GetCollection, withURLParam(newRequest("GET", "/api/collections", nil), "collectionID", collection)).Want(200).JSON(&detail)
	for _, field := range detail.Fields {
		if field.ID == note {
			t.Fatal("archived field still listed")
		}
	}
}

func TestCollectionRecordTrashAndRestore(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Soon deleted"})
	dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": "Kept"})
	params := func(method string) *testutil.Response {
		request := withURLParams(newRequest(method, "/api/collections/records", nil), "collectionID", collection, "recordID", record)
		if method == "DELETE" {
			return testutil.Call(t, testHandler.DeleteCollectionRecord, request)
		}
		return testutil.Call(t, testHandler.RestoreCollectionRecord, request)
	}
	params("DELETE").Want(200)
	params("DELETE").Want(404)
	var page struct {
		Total int `json:"total"`
	}
	testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records", nil), "collectionID", collection)).Want(200).JSON(&page)
	if page.Total != 1 {
		t.Fatalf("deleted record still listed: %+v", page)
	}
	var trash struct {
		Records []struct {
			ID        string `json:"id"`
			DeletedAt string `json:"deleted_at"`
		} `json:"records"`
		Total int `json:"total"`
	}
	testutil.Call(t, testHandler.ListCollectionTrash, withURLParam(newRequest("GET", "/api/collections/trash", nil), "collectionID", collection)).Want(200).JSON(&trash)
	if trash.Total != 1 || trash.Records[0].ID != record || trash.Records[0].DeletedAt == "" {
		t.Fatalf("trash=%+v", trash)
	}
	params("POST").Want(200)
	params("POST").Want(404)
	testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records", nil), "collectionID", collection)).Want(200).JSON(&page)
	if page.Total != 2 {
		t.Fatalf("restored record missing: %+v", page)
	}
}

func TestCollectionRecordsSortAndInitialFields(t *testing.T) {
	collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	number := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Score", "type": "number"})
	stage := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Stage", "type": "select", "config": collectionOptionsConfig()})
	for _, value := range []int{5, 1, 9, 3, 7} {
		body := map[string]any{"title": fmt.Sprintf("Score %d", value), "fields": map[string]any{number: value}}
		testutil.Call(t, testHandler.CreateCollectionRecord, withURLParam(newRequest("POST", "/api/collections/records", body), "collectionID", collection)).Want(201)
	}
	body := map[string]any{"title": "In a column", "fields": map[string]any{stage: optionBlue}}
	var created struct {
		Fields map[string]any `json:"fields"`
	}
	testutil.Call(t, testHandler.CreateCollectionRecord, withURLParam(newRequest("POST", "/api/collections/records", body), "collectionID", collection)).Want(201).JSON(&created)
	if created.Fields[stage] != optionBlue {
		t.Fatalf("initial field missing: %+v", created)
	}
	bad := map[string]any{"title": "Bad", "fields": map[string]any{stage: "not-an-option"}}
	testutil.Call(t, testHandler.CreateCollectionRecord, withURLParam(newRequest("POST", "/api/collections/records", bad), "collectionID", collection)).Want(400)

	type page struct {
		Records []struct {
			Title string `json:"title"`
		} `json:"records"`
		Next *string `json:"next_cursor"`
	}
	fetch := func(query string) page {
		t.Helper()
		var out page
		testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records?"+query, nil), "collectionID", collection)).Want(200).JSON(&out)
		return out
	}
	first := fetch("sort_by=" + number + "&sort_dir=asc&limit=2")
	if len(first.Records) != 2 || first.Records[0].Title != "Score 1" || first.Records[1].Title != "Score 3" || first.Next == nil {
		t.Fatalf("sorted first page=%+v", first)
	}
	second := fetch("sort_by=" + number + "&sort_dir=asc&limit=2&cursor=" + url.QueryEscape(*first.Next))
	if len(second.Records) != 2 || second.Records[0].Title != "Score 5" || second.Records[1].Title != "Score 7" {
		t.Fatalf("sorted second page=%+v", second)
	}
	last := fetch("sort_by=" + number + "&sort_dir=desc&limit=10")
	if last.Records[0].Title != "Score 9" || last.Records[len(last.Records)-1].Title != "In a column" {
		t.Fatalf("descending order must keep empty values last: %+v", last)
	}
	byTitle := fetch("sort_by=title&limit=1")
	if byTitle.Records[0].Title != "In a column" {
		t.Fatalf("title sort=%+v", byTitle)
	}
}
