package handler

import (
	"fmt"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

type linkedRecord struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Links    map[string][]struct {
		ID           string `json:"id"`
		ToType       string `json:"to_type"`
		ToID         string `json:"to_id"`
		Title        string `json:"title"`
		Identifier   string `json:"identifier"`
		Status       string `json:"status"`
		CollectionID string `json:"collection_id"`
		Missing      bool   `json:"missing"`
	} `json:"links"`
}

type backlinkList struct {
	Links []struct {
		CollectionID   string `json:"collection_id"`
		CollectionName string `json:"collection_name"`
		RecordID       string `json:"record_id"`
		RecordTitle    string `json:"record_title"`
		FieldID        string `json:"field_id"`
		FieldName      string `json:"field_name"`
	} `json:"links"`
}

func newCollection(t *testing.T, name string) string {
	t.Helper()
	id := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": name})
	// Edges are written by the handlers under test, so no fixture owns them.
	dbfx.Cleanup(t, "DELETE FROM record_link WHERE collection_id=$1", id)
	return id
}

func newRelationField(t *testing.T, collection, name, config string) string {
	t.Helper()
	return dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": name, "type": "relation", "config": config})
}

func newRecord(t *testing.T, collection, title string) string {
	t.Helper()
	return dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": title})
}

func createField(t *testing.T, collection string, body map[string]any) *testutil.Response {
	t.Helper()
	response := testutil.Call(t, testHandler.CreateCollectionField, withURLParam(newRequest("POST", "/api/collections/fields", body), "collectionID", collection))
	if response.Code == 201 {
		var created struct {
			ID string `json:"id"`
		}
		response.JSON(&created)
		dbfx.Cleanup(t, "DELETE FROM collection_field WHERE id=$1", created.ID)
	}
	return response
}

func linkRecord(t *testing.T, collection, record, field, target string) *testutil.Response {
	t.Helper()
	request := withURLParams(newRequest("POST", "/api/collections/records/links", map[string]any{"field_id": field, "to_id": target}), "collectionID", collection, "recordID", record)
	return testutil.Call(t, testHandler.CreateCollectionRecordLink, request)
}

func unlinkRecord(t *testing.T, collection, record, link string) *testutil.Response {
	t.Helper()
	request := withURLParams(newRequest("DELETE", "/api/collections/records/links", nil), "collectionID", collection, "recordID", record, "linkID", link)
	return testutil.Call(t, testHandler.DeleteCollectionRecordLink, request)
}

// readRecord reads one record the way the table does: through the list.
func readRecord(t *testing.T, collection, record string) linkedRecord {
	t.Helper()
	var page struct {
		Records []linkedRecord `json:"records"`
	}
	request := withURLParam(newRequest("GET", "/api/collections/records?record_id="+record, nil), "collectionID", collection)
	testutil.Call(t, testHandler.ListCollectionRecords, request).Want(200).JSON(&page)
	if len(page.Records) != 1 {
		t.Fatalf("record %s not listed: %+v", record, page)
	}
	return page.Records[0]
}

func TestRelationFieldNamesItsTargetOnce(t *testing.T) {
	collection := newCollection(t, t.Name())
	customers := newCollection(t, t.Name()+" customers")
	text := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Notes", "type": "text"})

	var field struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Config struct {
			Relation struct {
				ToType       string `json:"to_type"`
				CollectionID string `json:"collection_id"`
			} `json:"relation"`
		} `json:"config"`
	}
	createField(t, collection, map[string]any{"name": "Tasks", "type": "relation", "config": map[string]any{"relation": map[string]any{"to_type": "issue"}}}).Want(201).JSON(&field)
	if field.Type != "relation" || field.Config.Relation.ToType != "issue" || field.Config.Relation.CollectionID != "" {
		t.Fatalf("task relation=%+v", field)
	}
	tasks := field.ID
	// The stored id is canonical whatever spelling the caller used.
	createField(t, collection, map[string]any{"name": "Customer", "type": "relation", "config": map[string]any{"relation": map[string]any{"to_type": "record", "collection_id": strings.ToUpper(customers)}}}).Want(201).JSON(&field)
	if field.Config.Relation.ToType != "record" || field.Config.Relation.CollectionID != customers {
		t.Fatalf("record relation=%+v", field)
	}

	for name, config := range map[string]any{
		"no target":          nil,
		"unknown target":     map[string]any{"relation": map[string]any{"to_type": "document"}},
		"record without id":  map[string]any{"relation": map[string]any{"to_type": "record"}},
		"missing collection": map[string]any{"relation": map[string]any{"to_type": "record", "collection_id": "99999999-9999-4999-8999-999999999999"}},
		"task with table":    map[string]any{"relation": map[string]any{"to_type": "issue", "collection_id": customers}},
		"with options":       map[string]any{"relation": map[string]any{"to_type": "issue"}, "options": []map[string]any{{"name": "A", "color": "#dc2626"}}},
	} {
		body := map[string]any{"name": "Bad " + name, "type": "relation"}
		if config != nil {
			body["config"] = config
		}
		createField(t, collection, body).Want(400)
	}
	// A target belongs to relation fields only.
	createField(t, collection, map[string]any{"name": "Plain", "type": "text", "config": map[string]any{"relation": map[string]any{"to_type": "issue"}}}).Want(400)

	// The type neither converts in nor out, and the target never moves.
	patchField(t, collection, text, map[string]any{"type": "relation"}).Want(400)
	patchField(t, collection, tasks, map[string]any{"type": "text"}).Want(400)
	patchField(t, collection, tasks, map[string]any{"config": map[string]any{"relation": map[string]any{"to_type": "record", "collection_id": customers}}}).Want(400)
	patchField(t, collection, tasks, map[string]any{"name": "Work"}).Want(200).JSON(&field)
	if field.Name != "Work" || field.Config.Relation.ToType != "issue" {
		t.Fatalf("rename lost the target: %+v", field)
	}

	// Its cells are not values.
	record := newRecord(t, collection, "Row")
	issue := dbfx.Issue(t, "Some task")
	write := withURLParams(newRequest("PUT", "/api/collections/records/fields", map[string]any{"value": issue}), "collectionID", collection, "recordID", record, "fieldID", tasks)
	testutil.Call(t, testHandler.SetCollectionRecordField, write).Want(400)
	clear := withURLParams(newRequest("PUT", "/api/collections/records/fields", map[string]any{"value": nil}), "collectionID", collection, "recordID", record, "fieldID", tasks)
	testutil.Call(t, testHandler.SetCollectionRecordField, clear).Want(400)
	create := withURLParam(newRequest("POST", "/api/collections/records", map[string]any{"title": "New", "fields": map[string]any{tasks: issue}}), "collectionID", collection)
	testutil.Call(t, testHandler.CreateCollectionRecord, create).Want(400)
}

// AC-8: a row links to the task that implements it, the task lists the row,
// and deleting the task leaves a link that says so.
func TestRecordLinksToTasksAndBack(t *testing.T) {
	setWorkspaceIssuePrefixForTest(t, "REL")
	collection := newCollection(t, "Requirements")
	field := newRelationField(t, collection, "Tasks", `{"relation":{"to_type":"issue"}}`)
	record := newRecord(t, collection, "Embed live views")
	other := newRecord(t, collection, "Untouched row")
	issue := dbfx.Issue(t, "Build the embed block", testutil.Cols{"status": "in_progress"})
	var number int
	dbfx.QueryRow(t, "SELECT number FROM issue WHERE id=$1", issue).Scan(&number)

	var linked linkedRecord
	linkRecord(t, collection, record, field, issue).Want(201).JSON(&linked)
	links := linked.Links[field]
	if len(links) != 1 || links[0].ToType != "issue" || links[0].ToID != issue || links[0].Missing ||
		links[0].Title != "Build the embed block" || links[0].Status != "in_progress" || links[0].Identifier != fmt.Sprintf("REL-%d", number) {
		t.Fatalf("link payload=%+v", linked)
	}
	before := linked.Revision
	// Linking the same task again changes nothing.
	var relinked linkedRecord
	linkRecord(t, collection, record, field, issue).Want(200).JSON(&relinked)
	if len(relinked.Links[field]) != 1 || relinked.Revision != before {
		t.Fatalf("relink was not idempotent: %+v", relinked)
	}
	if got := dbfx.Count(t, "SELECT count(*) FROM record_link WHERE from_record_id=$1", record); got != 1 {
		t.Fatalf("edges=%d", got)
	}

	// The table read carries the same links, and only on the row that has them.
	if listed := readRecord(t, collection, record); len(listed.Links[field]) != 1 || listed.Links[field][0].Identifier != links[0].Identifier {
		t.Fatalf("listed=%+v", listed)
	}
	if listed := readRecord(t, collection, other); listed.Links == nil || len(listed.Links) != 0 {
		t.Fatalf("unlinked row links=%+v", listed.Links)
	}

	// The task side: by id and by identifier.
	var back backlinkList
	for _, id := range []string{issue, links[0].Identifier} {
		testutil.Call(t, testHandler.ListIssueRecordLinks, withURLParam(newRequest("GET", "/api/issues/record-links", nil), "id", id)).Want(200).JSON(&back)
		if len(back.Links) != 1 || back.Links[0].RecordID != record || back.Links[0].RecordTitle != "Embed live views" ||
			back.Links[0].CollectionID != collection || back.Links[0].CollectionName != "Requirements" || back.Links[0].FieldID != field || back.Links[0].FieldName != "Tasks" {
			t.Fatalf("backlinks for %s=%+v", id, back)
		}
	}

	// Deleting the task keeps the edge and marks it.
	dbfx.Exec(t, "DELETE FROM issue WHERE id=$1", issue)
	gone := readRecord(t, collection, record).Links[field]
	if len(gone) != 1 || !gone[0].Missing || gone[0].Title != "" || gone[0].Identifier != "" || gone[0].ToID != issue {
		t.Fatalf("link to a deleted task=%+v", gone)
	}

	// A dead link can still be removed, and removing twice is fine.
	var unlinked linkedRecord
	unlinkRecord(t, collection, record, gone[0].ID).Want(200).JSON(&unlinked)
	if unlinked.Links == nil || len(unlinked.Links) != 0 || unlinked.Revision <= before {
		t.Fatalf("after unlink=%+v", unlinked)
	}
	unlinkRecord(t, collection, record, gone[0].ID).Want(200)
}

func TestRecordLinksToRecordsOfOneTable(t *testing.T) {
	requirements := newCollection(t, "Requirements")
	customers := newCollection(t, "Customers")
	vendors := newCollection(t, "Vendors")
	customer := newRelationField(t, requirements, "Customer", fmt.Sprintf(`{"relation":{"to_type":"record","collection_id":%q}}`, customers))
	related := newRelationField(t, requirements, "Related", fmt.Sprintf(`{"relation":{"to_type":"record","collection_id":%q}}`, requirements))
	row := newRecord(t, requirements, "Calendar layout")
	sibling := newRecord(t, requirements, "Gallery layout")
	acme := newRecord(t, customers, "ACME")
	vendor := newRecord(t, vendors, "Vendor")
	trashed := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": customers, "title": "Gone", "deleted_at": testutil.Raw("now()")})

	// Only live records of the field's own table are targets.
	linkRecord(t, requirements, row, customer, vendor).Want(400)
	linkRecord(t, requirements, row, customer, trashed).Want(400)
	linkRecord(t, requirements, row, customer, dbfx.Issue(t, "Not a record")).Want(400)
	linkRecord(t, requirements, row, related, row).Want(400)

	var linked linkedRecord
	linkRecord(t, requirements, row, customer, acme).Want(201).JSON(&linked)
	if got := linked.Links[customer]; len(got) != 1 || got[0].ToType != "record" || got[0].Title != "ACME" || got[0].CollectionID != customers || got[0].Missing {
		t.Fatalf("record link=%+v", linked)
	}
	// A table may point at itself, one row at another.
	var both linkedRecord
	linkRecord(t, requirements, row, related, sibling).Want(201).JSON(&both)
	if len(both.Links) != 2 || both.Links[related][0].ToID != sibling {
		t.Fatalf("self relation=%+v", both)
	}

	backlinks := func(collection, record string) backlinkList {
		var out backlinkList
		request := withURLParams(newRequest("GET", "/api/collections/records/backlinks", nil), "collectionID", collection, "recordID", record)
		testutil.Call(t, testHandler.ListCollectionRecordBacklinks, request).Want(200).JSON(&out)
		return out
	}
	if back := backlinks(customers, acme); len(back.Links) != 1 || back.Links[0].RecordID != row || back.Links[0].FieldName != "Customer" || back.Links[0].CollectionName != "Requirements" {
		t.Fatalf("customer backlinks=%+v", back)
	}

	// A trashed target reads missing; restoring it brings the link back.
	trash := func(collection, record string, remove bool) {
		handler, method := testHandler.RestoreCollectionRecord, "POST"
		if remove {
			handler, method = testHandler.DeleteCollectionRecord, "DELETE"
		}
		testutil.Call(t, handler, withURLParams(newRequest(method, "/api/collections/records", nil), "collectionID", collection, "recordID", record)).Want(200)
	}
	trash(customers, acme, true)
	if got := readRecord(t, requirements, row).Links[customer]; len(got) != 1 || !got[0].Missing || got[0].Title != "" {
		t.Fatalf("link to a trashed record=%+v", got)
	}
	trash(customers, acme, false)
	if got := readRecord(t, requirements, row).Links[customer]; len(got) != 1 || got[0].Missing || got[0].Title != "ACME" {
		t.Fatalf("link after restore=%+v", got)
	}

	// Backlinks only list sources a reader can still open.
	trash(requirements, row, true)
	if back := backlinks(customers, acme); len(back.Links) != 0 {
		t.Fatalf("trashed source still listed: %+v", back)
	}
	trash(requirements, row, false)
	patchField(t, requirements, customer, map[string]any{"archived": true}).Want(200)
	if back := backlinks(customers, acme); len(back.Links) != 0 {
		t.Fatalf("archived field still listed: %+v", back)
	}
	request := withURLParams(newRequest("GET", "/api/collections/records/backlinks", nil), "collectionID", customers, "recordID", trashed)
	testutil.Call(t, testHandler.ListCollectionRecordBacklinks, request).Want(404)
}

func TestRecordLinkTargetsStayInsideTheWorkspaceAndTheFieldType(t *testing.T) {
	collection := newCollection(t, t.Name())
	field := newRelationField(t, collection, "Tasks", `{"relation":{"to_type":"issue"}}`)
	text := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Notes", "type": "text"})
	record := newRecord(t, collection, "Row")
	issue := dbfx.Issue(t, "Local task")

	elsewhere := dbfx.Workspace(t, "Elsewhere", "record-link-elsewhere")
	foreign := dbfx.Issue(t, "Foreign task", testutil.Cols{"workspace_id": elsewhere})
	document := dbfx.Issue(t, "A document", testutil.Cols{"kind": "doc", "status": "draft"})

	linkRecord(t, collection, record, field, foreign).Want(400)
	linkRecord(t, collection, record, field, document).Want(400)
	linkRecord(t, collection, record, text, issue).Want(400)
	linkRecord(t, collection, record, "99999999-9999-4999-8999-999999999999", issue).Want(404)
	linkRecord(t, collection, "99999999-9999-4999-8999-999999999999", field, issue).Want(404)
	linkRecord(t, collection, record, field, "not-a-uuid").Want(400)
	if got := dbfx.Count(t, "SELECT count(*) FROM record_link WHERE collection_id=$1", collection); got != 0 {
		t.Fatalf("rejected links left %d edges", got)
	}

	// Every member who may edit the row may link it; managing fields is not required.
	_, _, member := privateAgentTestFixture(t)
	request := withURLParams(newRequestAs(member, "POST", "/api/collections/records/links", map[string]any{"field_id": field, "to_id": issue}), "collectionID", collection, "recordID", record)
	testutil.Call(t, testHandler.CreateCollectionRecordLink, request).Want(201)
}

func TestRelationCellHoldsABoundedNumberOfLinks(t *testing.T) {
	collection := newCollection(t, t.Name())
	field := newRelationField(t, collection, "Tasks", `{"relation":{"to_type":"issue"}}`)
	record := newRecord(t, collection, "Row")
	dbfx.Exec(t, `INSERT INTO record_link (workspace_id,collection_id,from_record_id,from_field_id,to_type,to_id)
		SELECT $1,$2,$3,$4,'issue',gen_random_uuid() FROM generate_series(1,$5)`, testWorkspaceID, collection, record, field, maxRecordLinksPerField)
	linkRecord(t, collection, record, field, dbfx.Issue(t, "One too many")).Want(400)
	// Dead links count toward the cap but never hide a cell's content.
	if got := readRecord(t, collection, record).Links[field]; len(got) != maxRecordLinksPerField || !got[0].Missing {
		t.Fatalf("links=%d", len(got))
	}
}
