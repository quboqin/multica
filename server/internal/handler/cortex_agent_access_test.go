package handler

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/testutil"
)

// What an agent may do with documents and tables (FR-027, AC-26).
//
// An agent reaches the API with its run's task token, which authenticates as
// the runtime's owner and is stamped as a machine credential. The line drawn
// here is deliberate and the CLI and the built-in platform skill both teach it,
// so it is pinned in one place:
//
//   - A table is working material, rows and structure alike. An agent reads
//     tables, writes rows, cells and links, and also creates tables and reshapes
//     their fields — with exactly the rights of the person whose runtime it runs
//     on, no more and no fewer.
//   - Documents are working material too: an agent creates, saves and moves
//     them as a member does.
//   - Approval belongs to people. Submitting, publishing or withdrawing a
//     document is refused with a stable code, because the approval is recorded
//     against a person and "403" alone tells an agent nothing about what it can
//     do instead.

type agentCaller struct {
	agentID string
	taskID  string
	// owner is the person the task token authenticates as: the owner of the
	// runtime the run is on. Empty means the fixture user, a workspace owner.
	owner string
}

func newAgentCaller(t *testing.T) agentCaller {
	t.Helper()
	agentID := createHandlerTestAgent(t, "cortex-access-agent", nil)
	return agentCaller{agentID: agentID, taskID: createHandlerTestTaskForAgent(t, agentID)}
}

// onRuntimeOf returns the same agent running on a runtime that userID owns.
func (a agentCaller) onRuntimeOf(userID string) agentCaller {
	a.owner = userID
	return a
}

// as stamps a request the way the auth middleware stamps one that arrived with
// this agent's task token.
func (a agentCaller) as(r *http.Request) *http.Request {
	r.Header.Set("X-Actor-Source", "task_token")
	r.Header.Set("X-Agent-ID", a.agentID)
	r.Header.Set("X-Task-ID", a.taskID)
	if a.owner != "" {
		r.Header.Set("X-User-ID", a.owner)
	}
	return r
}

func wantRefusalCode(t *testing.T, what string, response *testutil.Response, code string) {
	t.Helper()
	response.Want(403)
	var body struct {
		Code string `json:"code"`
	}
	response.JSON(&body)
	if body.Code != code {
		t.Errorf("%s: refusal code = %q, want %q (body %s)", what, body.Code, code, response.Body.String())
	}
}

// createdTable is the part of a create-table response these tests read.
type createdTable struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedBy string `json:"created_by"`
}

func createTableAs(t *testing.T, r *http.Request) createdTable {
	t.Helper()
	var table createdTable
	testutil.Call(t, testHandler.CreateCollection, r).Want(201).JSON(&table)
	dbfx.Cleanup(t, "DELETE FROM collection WHERE id=$1", table.ID)
	dbfx.Cleanup(t, "DELETE FROM collection_field WHERE collection_id=$1", table.ID)
	dbfx.Cleanup(t, "DELETE FROM record WHERE collection_id=$1", table.ID)
	return table
}

func TestAgentTokenManagesTablesAsItsRuntimeOwner(t *testing.T) {
	agent := newAgentCaller(t)

	// Every structure change tells open clients to refetch, credited to the
	// agent rather than to the person its token authenticates as.
	var mu sync.Mutex
	credited := map[string][]string{}
	testHandler.Bus.Subscribe("collection:updated", func(e events.Event) {
		payload, _ := e.Payload.(map[string]any)
		id, _ := payload["collection_id"].(string)
		mu.Lock()
		credited[id] = append(credited[id], e.ActorType+":"+e.ActorID)
		mu.Unlock()
	})

	// The table belongs to the person the run acts for, so they find it
	// manageable in the app afterwards.
	table := createTableAs(t, agent.as(newRequest("POST", "/api/collections", map[string]any{"name": t.Name()})))
	if table.CreatedBy != testUserID {
		t.Fatalf("created_by = %q, want the runtime owner %q", table.CreatedBy, testUserID)
	}
	at := func(method string, body any, params ...string) *http.Request {
		return agent.as(withURLParams(newRequest(method, "/api/collections", body), append([]string{"collectionID", table.ID}, params...)...))
	}

	var stage struct {
		ID     string `json:"id"`
		Config struct {
			Options []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"options"`
		} `json:"config"`
	}
	testutil.Call(t, testHandler.CreateCollectionField, at("POST", map[string]any{
		"name": "Stage", "type": "select",
		"config": map[string]any{"options": []map[string]string{{"name": "Lead", "color": "#6b7280"}}},
	})).Want(201).JSON(&stage)
	if len(stage.Config.Options) != 1 {
		t.Fatalf("field added by an agent = %+v", stage)
	}
	var notes struct {
		ID string `json:"id"`
	}
	testutil.Call(t, testHandler.CreateCollectionField, at("POST", map[string]any{"name": "Notes", "type": "text"})).Want(201).JSON(&notes)

	testutil.Call(t, testHandler.UpdateCollection, at("PATCH", map[string]any{"name": t.Name() + " renamed", "title_name": "Customer"})).Want(200)
	// Options are part of the structure: a run may extend the list it read.
	testutil.Call(t, testHandler.UpdateCollectionField, at("PATCH", map[string]any{
		"name": "Pipeline stage",
		"config": map[string]any{"options": []map[string]string{
			{"id": stage.Config.Options[0].ID, "name": "Lead", "color": "#6b7280"},
			{"name": "Won", "color": "#22c55e"},
		}},
	}, "fieldID", stage.ID)).Want(200).JSON(&stage)
	if len(stage.Config.Options) != 2 || stage.Config.Options[1].Name != "Won" {
		t.Fatalf("options after an agent's change = %+v", stage.Config.Options)
	}

	// The row written next uses the option the agent has just added.
	var row linkedRecord
	testutil.Call(t, testHandler.CreateCollectionRecord, at("POST", map[string]any{
		"title": "ACME", "fields": map[string]any{stage.ID: stage.Config.Options[1].ID, notes.ID: "from an agent"},
	})).Want(201).JSON(&row)

	testutil.Call(t, testHandler.UpdateCollectionField, at("PATCH", map[string]any{"archived": true}, "fieldID", notes.ID)).Want(200)
	var detail struct {
		Fields []struct {
			Name string `json:"name"`
		} `json:"fields"`
	}
	testutil.Call(t, testHandler.GetCollection, at("GET", nil)).Want(200).JSON(&detail)
	if len(detail.Fields) != 1 || detail.Fields[0].Name != "Pipeline stage" {
		t.Errorf("fields after the agent's changes = %+v", detail.Fields)
	}

	testutil.Call(t, testHandler.UpdateCollection, at("PATCH", map[string]any{"archived": true})).Want(200)
	testutil.Call(t, testHandler.GetCollection, at("GET", nil)).Want(404)

	mu.Lock()
	defer mu.Unlock()
	// create, two fields, rename, field change, field archive, table archive.
	if got := credited[table.ID]; len(got) != 7 {
		t.Errorf("structure changes announced = %d (%v), want 7", len(got), got)
	}
	for _, actor := range credited[table.ID] {
		if actor != "agent:"+agent.agentID {
			t.Errorf("a structure change was credited to %s, want the agent", actor)
		}
	}
}

func TestAgentTokenIsHeldToItsRuntimeOwnersTableRights(t *testing.T) {
	agent := newAgentCaller(t)
	member := dbfx.User(t, "Plain Member", "cortex-plain-member@multica.test")
	dbfx.Member(t, testWorkspaceID, member, "member")
	admin := dbfx.User(t, "Workspace Admin", "cortex-admin@multica.test")
	dbfx.Member(t, testWorkspaceID, admin, "admin")

	// Someone else's table: created by the fixture user.
	theirs := newCollection(t, t.Name())
	notes := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": theirs, "name": "Notes", "type": "text", "config": "{}"})
	on := func(r *http.Request, params ...string) *http.Request {
		return withURLParams(r, append([]string{"collectionID", theirs}, params...)...)
	}

	const code = "collection_schema_forbidden"
	memberAgent := agent.onRuntimeOf(member)
	for what, call := range map[string]func() *testutil.Response{
		"rename table": func() *testutil.Response {
			return testutil.Call(t, testHandler.UpdateCollection, memberAgent.as(on(newRequest("PATCH", "/api/collections", map[string]any{"name": "Renamed"}))))
		},
		"archive table": func() *testutil.Response {
			return testutil.Call(t, testHandler.UpdateCollection, memberAgent.as(on(newRequest("PATCH", "/api/collections", map[string]any{"archived": true}))))
		},
		"add field": func() *testutil.Response {
			return testutil.Call(t, testHandler.CreateCollectionField, memberAgent.as(on(newRequest("POST", "/api/collections", map[string]any{"name": "Stage", "type": "text"}))))
		},
		"change field": func() *testutil.Response {
			return testutil.Call(t, testHandler.UpdateCollectionField, memberAgent.as(on(newRequest("PATCH", "/api/collections", map[string]any{"name": "Renamed"}), "fieldID", notes)))
		},
	} {
		wantRefusalCode(t, "a plain member's agent: "+what, call(), code)
	}
	// The rule is about the person, not about being an agent: the member gets
	// the same answer at the keyboard.
	wantRefusalCode(t, "the plain member", testutil.Call(t, testHandler.CreateCollectionField, on(newRequestAs(member, "POST", "/api/collections", map[string]any{"name": "Stage", "type": "text"}))), code)

	// Rows stay open, and so does a table of the member's own.
	var row linkedRecord
	testutil.Call(t, testHandler.CreateCollectionRecord, memberAgent.as(on(newRequest("POST", "/api/collections", map[string]any{"title": "Written anyway"})))).Want(201).JSON(&row)
	dbfx.Cleanup(t, "DELETE FROM record WHERE id=$1", row.ID)

	own := createTableAs(t, memberAgent.as(newRequest("POST", "/api/collections", map[string]any{"name": t.Name() + " own"})))
	if own.CreatedBy != member {
		t.Fatalf("created_by = %q, want the member the run acts for", own.CreatedBy)
	}
	testutil.Call(t, testHandler.CreateCollectionField, memberAgent.as(withURLParams(newRequest("POST", "/api/collections", map[string]any{"name": "Stage", "type": "text"}), "collectionID", own.ID))).Want(201)
	// ...which the member then manages in the app, never having touched it.
	testutil.Call(t, testHandler.UpdateCollection, withURLParams(newRequestAs(member, "PATCH", "/api/collections", map[string]any{"name": t.Name() + " own, renamed"}), "collectionID", own.ID)).Want(200)

	// An admin reshapes any table, and so does a run on an admin's runtime.
	var added struct {
		ID string `json:"id"`
	}
	testutil.Call(t, testHandler.CreateCollectionField, agent.onRuntimeOf(admin).as(on(newRequest("POST", "/api/collections", map[string]any{"name": "Added by an admin's agent", "type": "text"})))).Want(201).JSON(&added)
	dbfx.Cleanup(t, "DELETE FROM collection_field WHERE id=$1", added.ID)
}

func TestAgentTokenWritesRowsAndLinks(t *testing.T) {
	agent := newAgentCaller(t)
	collection := newCollection(t, t.Name())
	customers := newCollection(t, t.Name()+" customers")
	notes := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": "Notes", "type": "text", "config": "{}"})
	tasks := newRelationField(t, collection, "Implementation", `{"relation":{"to_type":"issue"}}`)
	customer := newRelationField(t, collection, "Customer", `{"relation":{"to_type":"record","collection_id":"`+customers+`"}}`)
	acme := newRecord(t, customers, "ACME")
	issue := dbfx.Issue(t, "Build the embed block")

	at := func(method string, body any, params ...string) *http.Request {
		return agent.as(withURLParams(newRequest(method, "/api/collections", body), append([]string{"collectionID", collection}, params...)...))
	}

	// Reading the structure is how an agent learns what it may write.
	var detail struct {
		Fields []struct {
			Name string `json:"name"`
		} `json:"fields"`
	}
	testutil.Call(t, testHandler.GetCollection, at("GET", nil)).Want(200).JSON(&detail)
	if len(detail.Fields) != 3 {
		t.Fatalf("an agent should read the whole field catalog, got %+v", detail.Fields)
	}
	testutil.Call(t, testHandler.ListCollections, agent.as(newRequest("GET", "/api/collections", nil))).Want(200)

	// Rows, cells, links, trash: all open.
	var row linkedRecord
	testutil.Call(t, testHandler.CreateCollectionRecord, at("POST", map[string]any{"title": "Embed live views", "fields": map[string]any{notes: "from an agent"}})).Want(201).JSON(&row)
	dbfx.Cleanup(t, "DELETE FROM record WHERE id=$1", row.ID)

	testutil.Call(t, testHandler.SetCollectionRecordField, at("PUT", map[string]any{"value": "rewritten", "expected_value": "from an agent"}, "recordID", row.ID, "fieldID", notes)).Want(200)
	// A guarded write whose guard no longer holds is a conflict, not an overwrite.
	testutil.Call(t, testHandler.SetCollectionRecordField, at("PUT", map[string]any{"value": "lost the race", "expected_value": "from an agent"}, "recordID", row.ID, "fieldID", notes)).Want(409)
	testutil.Call(t, testHandler.UpdateCollectionRecord, at("PUT", map[string]any{"title": "Embed live views in documents", "title_base": "Embed live views"}, "recordID", row.ID)).Want(200)

	testutil.Call(t, testHandler.CreateCollectionRecordLink, at("POST", map[string]any{"field_id": tasks, "to_id": issue}, "recordID", row.ID)).Want(201)
	var linked linkedRecord
	testutil.Call(t, testHandler.CreateCollectionRecordLink, at("POST", map[string]any{"field_id": customer, "to_id": acme}, "recordID", row.ID)).Want(201).JSON(&linked)
	if len(linked.Links[tasks]) != 1 || len(linked.Links[customer]) != 1 {
		t.Fatalf("links written by an agent = %+v", linked.Links)
	}

	var back struct {
		Links []struct {
			RecordID string `json:"record_id"`
		} `json:"links"`
	}
	testutil.Call(t, testHandler.ListIssueRecordLinks, agent.as(withURLParam(newRequest("GET", "/api/issues/record-links", nil), "id", issue))).Want(200).JSON(&back)
	if len(back.Links) != 1 || back.Links[0].RecordID != row.ID {
		t.Errorf("rows linked to the issue, as the agent reads them = %+v", back.Links)
	}
	testutil.Call(t, testHandler.ListCollectionRecordBacklinks, agent.as(withURLParams(newRequest("GET", "/api/collections/backlinks", nil), "collectionID", customers, "recordID", acme))).Want(200)

	testutil.Call(t, testHandler.DeleteCollectionRecordLink, at("DELETE", nil, "recordID", row.ID, "linkID", linked.Links[tasks][0].ID)).Want(200)
	testutil.Call(t, testHandler.DeleteCollectionRecord, at("DELETE", nil, "recordID", row.ID)).Want(200)
	testutil.Call(t, testHandler.ListCollectionTrash, at("GET", nil)).Want(200)
	testutil.Call(t, testHandler.RestoreCollectionRecord, at("POST", nil, "recordID", row.ID)).Want(200)
}

func TestAgentTokenWritesDocumentsButCannotApproveThem(t *testing.T) {
	agent := newAgentCaller(t)
	prepareDocumentStatuses(t)

	var doc IssueResponse
	testutil.Call(t, testHandler.CreateIssue, agent.as(newRequest("POST", "/api/issues", map[string]any{
		"kind": "doc", "title": t.Name(), "description": "v1", "allow_duplicate": true,
	}))).Want(201).JSON(&doc)
	dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", doc.ID)
	dbfx.Cleanup(t, "DELETE FROM document_publication WHERE issue_id=$1", doc.ID)
	if doc.Kind != "doc" || doc.Status != "draft" || doc.DocumentRevision != 1 {
		t.Fatalf("a document created by an agent = %+v", doc)
	}
	parent := createTestDocument(t, "")

	on := func(method, path string, body any) *http.Request {
		return agent.as(withURLParam(newRequest(method, path, body), "id", doc.ID))
	}

	// The body is versioned for an agent exactly as for a person: a save names
	// the revision it read, and one that does not is refused, not applied.
	var saved IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, on("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "v2 by the agent", "expected_document_revision": 1})).Want(200).JSON(&saved)
	if saved.DocumentRevision != 2 {
		t.Fatalf("revision after the agent's save = %d, want 2", saved.DocumentRevision)
	}
	var conflict struct {
		Code     string `json:"code"`
		Revision int64  `json:"document_revision"`
	}
	testutil.Call(t, testHandler.UpdateIssue, on("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "no revision named"})).Want(409).JSON(&conflict)
	if conflict.Code != "document_version_required" || conflict.Revision != 2 {
		t.Errorf("unversioned save = %+v", conflict)
	}
	testutil.Call(t, testHandler.UpdateIssue, on("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "stale", "expected_document_revision": 1})).Want(409)

	testutil.Call(t, testHandler.ListDocuments, agent.as(newRequest("GET", "/api/documents", nil))).Want(200)
	testutil.Call(t, testHandler.MoveDocument, on("POST", "/api/documents/"+doc.ID+"/move", map[string]any{"parent_issue_id": parent.ID, "position": 0})).Want(200)

	var comment struct {
		AuthorType string `json:"author_type"`
		AuthorID   string `json:"author_id"`
	}
	testutil.Call(t, testHandler.CreateComment, on("POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "Draft is ready for review."})).Want(201).JSON(&comment)
	if comment.AuthorType != "agent" || comment.AuthorID != agent.agentID {
		t.Errorf("comment author = %+v, want the agent", comment)
	}

	// Approval is recorded against the person who gave it. None of the three
	// transitions is open to a machine credential.
	for _, action := range []string{"review", "publish", "draft"} {
		wantRefusalCode(t, action, testutil.Call(t, testHandler.TransitionDocument, on("POST", "/api/documents/"+doc.ID+"/transition", map[string]any{"action": action, "expected_document_revision": 2})), "document_transition_requires_human")
	}
	var recorded int
	dbfx.QueryRow(t, "SELECT count(*) FROM document_publication WHERE issue_id=$1", doc.ID).Scan(&recorded)
	if recorded != 0 {
		t.Errorf("a refused transition left %d approval records", recorded)
	}

	// The human owner shares explicitly; an agent's later edits stay shared.
	shareDocument(t, doc.ID, "workspace", "view", nil, nil, 1, 200)
	wantRefusalCode(t, "sharing", testutil.Call(t, testHandler.UpdateDocumentAccess, on("PUT", "/api/documents/"+doc.ID+"/access", map[string]any{"scope": "workspace", "expected_revision": 2})), "document_transition_requires_human")
	var edited IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, on("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "v3, after publication", "expected_document_revision": 2})).Want(200).JSON(&edited)
	if edited.Status != "published" || edited.DocumentRevision != 3 {
		t.Errorf("after an agent edits a published document = %s rev %d, want published rev 3", edited.Status, edited.DocumentRevision)
	}

	// The refusal body is what the CLI branches on; keep it parseable.
	var refusal map[string]string
	response := testutil.Call(t, testHandler.TransitionDocument, on("POST", "/api/documents/"+doc.ID+"/transition", map[string]any{"action": "review", "expected_document_revision": 3}))
	if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil || refusal["error"] == "" {
		t.Errorf("refusal body = %s (%v)", response.Body.String(), err)
	}
}
