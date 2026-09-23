package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/documentaccess"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func shareCollection(t *testing.T, id, scope, role string, project *string, people []map[string]string, rev, status int) {
	t.Helper()
	testutil.Call(t, testHandler.UpdateCollectionAccess, withURLParam(newRequest("PUT", "/api/collections/"+id+"/access", map[string]any{"scope": scope, "scope_role": role, "project_id": project, "collaborators": people, "expected_revision": rev}), "collectionID", id)).Want(status)
}

func TestCollectionSharingAudienceAndRoles(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	viewer := dbfx.User(t, "Table reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, viewer, "admin")
	other := dbfx.User(t, "Other reader", t.Name()+"other@test.local")
	dbfx.Member(t, testWorkspaceID, other, "member")
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Private row"})
	field := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "Text", "type": "text"})
	call := func(h http.HandlerFunc, user, method string, body any, status int) *testutil.Response {
		t.Helper()
		return testutil.Call(t, h, withURLParams(newRequestAs(user, method, "/api/collections/"+c, body), "collectionID", c, "recordID", record, "fieldID", field)).Want(status)
	}
	call(testHandler.GetCollection, viewer, "GET", nil, 404)
	call(testHandler.ListCollectionRecords, viewer, "GET", nil, 404)
	for _, suffix := range []string{"", "?archived=true"} {
		response := testutil.Call(t, testHandler.ListCollections, newRequestAs(viewer, "GET", "/api/collections"+suffix, nil)).Want(200)
		if strings.Contains(response.Body.String(), c) {
			t.Fatal("private collection leaked in list")
		}
	}
	shareCollection(t, c, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 1, 200)
	var detail map[string]any
	call(testHandler.GetCollection, viewer, "GET", nil, 200).JSON(&detail)
	access := detail["access"].(map[string]any)
	if access["can_edit"] != false || access["can_manage"] != false {
		t.Fatalf("reader permissions: %+v", access)
	}
	call(testHandler.GetCollection, other, "GET", nil, 404)
	call(testHandler.ListCollectionRecords, viewer, "GET", nil, 200)
	for _, op := range []struct {
		h      http.HandlerFunc
		method string
		body   any
	}{
		{testHandler.CreateCollectionRecord, "POST", map[string]any{"title": "Forbidden"}},
		{testHandler.SetCollectionRecordField, "PUT", map[string]any{"value": "Forbidden", "expected_value": nil}},
		{testHandler.DeleteCollectionRecord, "DELETE", nil},
		{testHandler.CreateCollectionField, "POST", map[string]any{"name": "Forbidden", "type": "text"}},
		{testHandler.UpdateCollection, "PATCH", map[string]any{"name": "Forbidden"}},
		{testHandler.UpdateCollectionAccess, "PUT", map[string]any{"scope": "workspace", "expected_revision": 2}},
	} {
		call(op.h, viewer, op.method, op.body, 403)
	}
	shareCollection(t, c, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "edit"}}, 2, 200)
	call(testHandler.SetCollectionRecordField, viewer, "PUT", map[string]any{"value": "Allowed", "expected_value": nil}, 200)
	call(testHandler.UpdateCollectionField, viewer, "PATCH", map[string]any{"name": "Editor field"}, 200)
	call(testHandler.UpdateCollection, viewer, "PATCH", map[string]any{"name": "Editor table"}, 200)
	call(testHandler.UpdateCollectionAccess, viewer, "PUT", map[string]any{"scope": "workspace", "expected_revision": 3}, 403)
	call(testHandler.UpdateCollection, viewer, "PATCH", map[string]any{"archived": true}, 403)
	shareCollection(t, c, "workspace", "view", nil, []map[string]string{{"user_id": viewer, "role": "edit"}}, 3, 200)
	call(testHandler.UpdateCollection, viewer, "PATCH", map[string]any{"name": "Highest role"}, 200)
	call(testHandler.UpdateCollection, other, "PATCH", map[string]any{"name": "Forbidden"}, 403)
	shareCollection(t, c, "workspace", "edit", nil, nil, 4, 200)
	call(testHandler.UpdateCollection, other, "PATCH", map[string]any{"name": "Workspace editor"}, 200)
	shareCollection(t, c, "workspace", "view", nil, nil, 4, 409)
	project := dbfx.Project(t, "Table project")
	shareCollection(t, c, "project", "view", &project, nil, 5, 200)
	call(testHandler.GetCollection, other, "GET", nil, 200)
	call(testHandler.UpdateCollection, other, "PATCH", map[string]any{"name": "Forbidden"}, 403)
	shareCollection(t, c, "project", "edit", &project, nil, 6, 200)
	call(testHandler.UpdateCollection, other, "PATCH", map[string]any{"name": "Project editor"}, 200)
	testutil.Call(t, testHandler.DeleteProject, withURLParam(newRequest("DELETE", "/api/projects/"+project, nil), "id", project)).Want(204)
	call(testHandler.GetCollection, other, "GET", nil, 404)
	call(testHandler.GetCollection, testUserID, "GET", nil, 200)
}

func TestCollectionSharingViewsPinsAndEvents(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	viewer := dbfx.User(t, "Table reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, viewer, "member")
	view := dbfx.Insert(t, "issue_view", testutil.Cols{"workspace_id": testWorkspaceID, "owner_id": testUserID, "name": "Private table view", "scope_type": "workspace", "visibility": "workspace", "collection_id": c, "query": "{}", "display": "{}"})
	readView := func(status int) {
		t.Helper()
		testutil.Call(t, testHandler.GetIssueViewByID, withURLParam(newRequestAs(viewer, "GET", "/api/issue-views/"+view, nil), "id", view)).Want(status)
	}
	readView(404)
	pin := func(status int) {
		t.Helper()
		testutil.Call(t, testHandler.CreatePin, newRequestAs(viewer, "POST", "/api/pins", map[string]any{"item_type": "collection", "item_id": c})).Want(status)
	}
	pin(404)
	policy := documentaccess.EventPolicy(testHandler.Queries)
	for _, kind := range []string{"pin:created", "pin:deleted", "pin:reordered"} {
		event, allowed := policy(events.Event{Type: kind, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: viewer, Payload: map[string]any{"item_id": c}})
		if !allowed || len(event.RecipientUserIDs) != 1 || event.RecipientUserIDs[0] != viewer {
			t.Fatal("pin event must be personal")
		}
	}
	for _, payload := range []map[string]any{{"collection_id": c}, {"record": map[string]any{"collection_id": c}}, {"view": map[string]any{"collection_id": c}}} {
		e, ok := policy(events.Event{Type: "collection:updated", WorkspaceID: testWorkspaceID, Payload: payload})
		if !ok || len(e.RecipientUserIDs) != 1 || e.RecipientUserIDs[0] != testUserID {
			t.Fatalf("private event audience: %+v", e)
		}
	}
	shareCollection(t, c, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 1, 200)
	readView(200)
	pin(201)
	dbfx.Cleanup(t, "DELETE FROM pinned_item WHERE item_type='collection' AND item_id=$1", c)
	testutil.Call(t, testHandler.UpdateIssueView, withURLParam(newRequestAs(viewer, "PATCH", "/api/issue-views/"+view, map[string]any{"name": "Forbidden"}), "id", view)).Want(403)
	for _, include := range []string{"", "?include=view", "?include=view,collection"} {
		response := testutil.Call(t, testHandler.ListPins, newRequestAs(viewer, "GET", "/api/pins"+include, nil)).Want(200)
		if strings.Contains(response.Body.String(), c) != (include == "?include=view,collection") {
			t.Fatalf("pin opt-in: %s", response.Body.String())
		}
	}
	shareCollection(t, c, "private", "view", nil, nil, 2, 200)
	readView(404)
	response := testutil.Call(t, testHandler.ListPins, newRequestAs(viewer, "GET", "/api/pins?include=view,collection", nil)).Want(200)
	if strings.Contains(response.Body.String(), c) {
		t.Fatal("revoked pin visible")
	}
}

func TestCollectionSharingHidesRelationTargetsAndSources(t *testing.T) {
	source := newCollection(t, t.Name()+" source")
	target := newCollection(t, t.Name()+" target")
	f := newRelationField(t, source, "Target", `{"relation":{"to_type":"record","collection_id":"`+target+`"}}`)
	from := newRecord(t, source, "Private source title")
	to := newRecord(t, target, "Private target title")
	linkRecord(t, source, from, f, to).Want(201)
	viewer := dbfx.User(t, "Relation reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, viewer, "member")
	shareCollection(t, source, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "edit"}}, 1, 200)
	response := testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequestAs(viewer, "GET", "/", nil), "collectionID", source)).Want(200)
	if strings.Contains(response.Body.String(), "Private target title") {
		t.Fatal("private target title leaked")
	}
	var page struct {
		Records []linkedRecord `json:"records"`
	}
	response.JSON(&page)
	if len(page.Records) != 1 || !page.Records[0].Links[f][0].Missing {
		t.Fatal("private target must appear unavailable")
	}
	testutil.Call(t, testHandler.CreateCollectionRecordLink, withURLParams(newRequestAs(viewer, "POST", "/", map[string]any{"field_id": f, "to_id": to}), "collectionID", source, "recordID", from)).Want(404)
	shareCollection(t, source, "private", "view", nil, nil, 2, 200)
	shareCollection(t, target, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 1, 200)
	response = testutil.Call(t, testHandler.ListCollectionRecordBacklinks, withURLParams(newRequestAs(viewer, "GET", "/", nil), "collectionID", target, "recordID", to)).Want(200)
	if strings.Contains(response.Body.String(), from) || strings.Contains(response.Body.String(), "Private source") {
		t.Fatal("private source appeared in backlinks")
	}
}

func TestCollectionSharingValidatesAudienceAndRejectsMachineChanges(t *testing.T) {
	c := newCollection(t, t.Name())
	outsider := dbfx.User(t, "Outside workspace", t.Name()+"@test.local")
	foreignWs := dbfx.Workspace(t, "Other workspace", "sharing-foreign")
	foreignProject := dbfx.Project(t, "Foreign project", testutil.Cols{"workspace_id": foreignWs})
	for _, people := range [][]map[string]string{
		{{"user_id": "invalid", "role": "view"}},
		{{"user_id": outsider, "role": "view"}},
		{{"user_id": testUserID, "role": "view"}},
		{{"user_id": outsider, "role": "owner"}},
	} {
		shareCollection(t, c, "private", "view", nil, people, 1, 400)
	}
	shareCollection(t, c, "project", "view", &foreignProject, nil, 1, 400)
	shareCollection(t, c, "project", "view", nil, nil, 1, 400)
	shareCollection(t, c, "private", "owner", nil, nil, 1, 400)
	a := newAgentCaller(t)
	request := a.as(withURLParam(newRequest("PUT", "/", map[string]any{"scope": "workspace", "scope_role": "edit", "expected_revision": 1}), "collectionID", c))
	wantRefusalCode(t, "machine sharing", testutil.Call(t, testHandler.UpdateCollectionAccess, request), "collection_sharing_requires_human")
	shareCollection(t, c, "workspace", "view", nil, nil, 1, 200)
	shareCollection(t, c, "private", "view", nil, nil, 1, 409)
}
