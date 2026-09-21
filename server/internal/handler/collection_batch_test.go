package handler

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/multica-ai/multica/server/internal/testutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCollectionCSV10000AndAtomicBatch500(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), "DELETE FROM record WHERE collection_id=$1", c) })
	kinds := []string{"text", "number", "select", "multi_select", "date", "checkbox", "url", "actor", "multi_actor"}
	header := []string{"title"}
	cells := []string{"Title, with\nnewline"}
	fieldIDs := []string{}
	for i := 0; i < 20; i++ {
		kind := kinds[i%len(kinds)]
		config := "{}"
		if kind == "select" || kind == "multi_select" {
			config = collectionOptionsConfig()
		}
		name := fmt.Sprintf("Field %d", i)
		fieldIDs = append(fieldIDs, dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": name, "type": kind, "config": config}))
		header = append(header, name)
		value := map[string]string{"text": "comma, newline\n中文", "number": "123.5", "select": "Red", "multi_select": "[\"Red\",\"Blue\"]", "date": "2026-09-21", "checkbox": "true", "url": "https://example.com/a,b", "actor": "member:" + testUserID, "multi_actor": "[\"member:" + testUserID + "\"]"}[kind]
		cells = append(cells, value)
	}
	var b bytes.Buffer
	writer := csv.NewWriter(&b)
	_ = writer.Write(header)
	for i := 0; i < 10000; i++ {
		_ = writer.Write(cells)
	}
	writer.Flush()
	call := func(body map[string]any) *testutil.Response {
		return testutil.Call(t, testHandler.ImportCollectionCSV, withURLParam(newRequest("POST", "/", body), "collectionID", c))
	}
	var out struct {
		Count int `json:"count"`
	}
	call(map[string]any{"csv": b.String(), "dry_run": true}).Want(200).JSON(&out)
	if out.Count != 10000 {
		t.Fatal(out)
	}
	var n int
	_ = testPool.QueryRow(t.Context(), "SELECT count(*) FROM record WHERE collection_id=$1", c).Scan(&n)
	if n != 0 {
		t.Fatal("preview wrote records")
	}
	call(map[string]any{"csv": b.String()}).Want(201).JSON(&out)
	if out.Count != 10000 {
		t.Fatal(out)
	}
	rows, err := testPool.Query(t.Context(), "SELECT id::text FROM record WHERE collection_id=$1 ORDER BY id LIMIT 500", c)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	expected := map[string]int64{}
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
		expected[id] = 1
	}
	rows.Close()
	got := recordFields(t, ids[0])
	if len(got) != 20 || got[fieldIDs[0]] != "comma, newline\n中文" || got[fieldIDs[1]] != 123.5 || got[fieldIDs[2]] != optionRed {
		t.Fatalf("lost CSV values: %+v", got)
	}
	batch := func(body map[string]any) *testutil.Response {
		return testutil.Call(t, testHandler.BatchCollectionRecords, withURLParam(newRequest("POST", "/", body), "collectionID", c))
	}
	patch := map[string]any{"action": "update", "record_ids": ids, "expected_revisions": expected, "fields": map[string]any{fieldIDs[0]: "updated", fieldIDs[1]: 456}}
	batch(patch).Want(200).JSON(&out)
	if out.Count != 500 {
		t.Fatal(out)
	}
	patch["fields"] = map[string]any{fieldIDs[0]: "stale"}
	batch(patch).Want(409)
	if recordFields(t, ids[499])[fieldIDs[0]] != "updated" {
		t.Fatal("conflict wrote partial batch")
	}
	batch(map[string]any{"action": "delete", "record_ids": ids}).Want(400)
	batch(map[string]any{"action": "delete", "record_ids": ids, "confirmed": true}).Want(200)
	batch(map[string]any{"action": "restore", "record_ids": ids}).Want(200)
	// One unknown row aborts the entire batch, and failed CSV validation writes nothing.
	batch(map[string]any{"action": "delete", "record_ids": []string{ids[0], "11111111-1111-4111-8111-111111111111"}, "confirmed": true}).Want(404)
	call(map[string]any{"csv": "title,Field 1\nvalid,3\ninvalid,nope\n"}).Want(400)
	_ = testPool.QueryRow(t.Context(), "SELECT count(*) FROM record WHERE collection_id=$1 AND deleted_at IS NULL", c).Scan(&n)
	if n != 10000 {
		t.Fatalf("unexpected count %d", n)
	}
	call(map[string]any{"csv": "title,Field 0,Field 0\na,b,c"}).Want(400)
}

func TestCollectionRestoreMetadataAndFieldValues(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	f := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "Kept", "type": "text"})
	rec := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Keep", "fields": fmt.Sprintf(`{%q:"retained"}`, f)})
	patchCollection(t, c, map[string]any{"icon": "📋", "description": "Description"}).Want(200)
	patchCollection(t, c, map[string]any{"description": strings.Repeat("x", 4001)}).Want(400)
	patchField(t, c, f, map[string]any{"archived": true}).Want(200)
	request := func(method string) *http.Request {
		return withURLParams(newRequest(method, "/", nil), "collectionID", c, "fieldID", f)
	}
	testutil.Call(t, testHandler.ListArchivedCollectionFields, request("GET")).Want(200)
	testutil.Call(t, testHandler.RestoreCollectionField, request("POST")).Want(200)
	if recordFields(t, rec)[f] != "retained" {
		t.Fatal("archiving lost values")
	}
	patchCollection(t, c, map[string]any{"archived": true}).Want(200)
	var listed []struct {
		ID string `json:"id"`
	}
	testutil.Call(t, testHandler.ListCollections, newRequest("GET", "/?archived=true", nil)).Want(200).JSON(&listed)
	found := false
	for _, item := range listed {
		found = found || item.ID == c
	}
	if !found {
		t.Fatal("archived table missing")
	}
	_, _, member := privateAgentTestFixture(t)
	testutil.Call(t, testHandler.RestoreCollection, withURLParam(newRequestAs(member, "POST", "/", nil), "collectionID", c)).Want(403)
	testutil.Call(t, testHandler.RestoreCollection, request("POST")).Want(200)
	var detail struct {
		Collection struct{ Icon, Description string } `json:"collection"`
	}
	testutil.Call(t, testHandler.GetCollection, request("GET")).Want(200).JSON(&detail)
	if detail.Collection.Icon != "📋" || detail.Collection.Description != "Description" {
		t.Fatal(detail)
	}
	patchField(t, c, f, map[string]any{"archived": true}).Want(200)
	dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "Kept", "type": "text"})
	testutil.Call(t, testHandler.RestoreCollectionField, request("POST")).Want(409)
}

func TestCollectionRelationFilteringAndSorting(t *testing.T) {
	c := newCollection(t, t.Name())
	target := newCollection(t, t.Name()+" target")
	f := newRelationField(t, c, "Customer", fmt.Sprintf(`{"relation":{"to_type":"record","collection_id":%q}}`, target))
	a := newRecord(t, c, "First")
	b := newRecord(t, c, "Second")
	empty := newRecord(t, c, "Empty")
	zed := newRecord(t, target, "Zed")
	alpha := newRecord(t, target, "Alpha")
	linkRecord(t, c, a, f, zed).Want(201)
	linkRecord(t, c, b, f, alpha).Want(201)
	var page struct {
		Total   int `json:"total"`
		Records []struct {
			ID string `json:"id"`
		} `json:"records"`
	}
	query := func(params string) *testutil.Response {
		return testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/?"+params, nil), "collectionID", c))
	}
	filter := func(value string) string { return "properties=" + url.QueryEscape(fmt.Sprintf(`{%q:[%q]}`, f, value)) }
	query(filter(alpha)).Want(200).JSON(&page)
	if page.Total != 1 || page.Records[0].ID != b {
		t.Fatal(page)
	}
	query("sort_by=" + f + "&sort_dir=asc").Want(200).JSON(&page)
	if len(page.Records) != 3 || page.Records[0].ID != b || page.Records[2].ID != empty {
		t.Fatal(page)
	}
	patchCollection(t, target, map[string]any{"archived": true}).Want(200)
	query(filter(alpha)).Want(200).JSON(&page)
	if page.Total != 0 {
		t.Fatal("archived target leaked")
	}
	query(filter("__none__")).Want(200).JSON(&page)
	if page.Total != 3 {
		t.Fatal(page)
	}
	query("properties=" + url.QueryEscape(fmt.Sprintf(`{%q:[{"op":"contains","value":"Alpha"}]}`, f))).Want(400)
}

func TestCollectionAgentBatchAndRestoreAuthorization(t *testing.T) {
	agent := newAgentCaller(t)
	c := newCollection(t, t.Name())
	f := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "Notes", "type": "text"})
	r := newRecord(t, c, "Row")
	request := agent.as(withURLParam(newRequest("POST", "/", map[string]any{"action": "update", "record_ids": []string{r}, "fields": map[string]any{f: "agent wrote"}}), "collectionID", c))
	testutil.Call(t, testHandler.BatchCollectionRecords, request).Want(200)
	patchCollection(t, c, map[string]any{"archived": true}).Want(200)
	_, _, member := privateAgentTestFixture(t)
	refused := testutil.Call(t, testHandler.RestoreCollection, agent.onRuntimeOf(member).as(withURLParam(newRequest("POST", "/", nil), "collectionID", c)))
	wantRefusalCode(t, "restore another member's table", refused, collectionSchemaForbidden)
	testutil.Call(t, testHandler.RestoreCollection, agent.as(withURLParam(newRequest("POST", "/", nil), "collectionID", c))).Want(200)
}
