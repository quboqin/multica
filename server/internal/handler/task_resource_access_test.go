package handler

import (
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

// What a run may do with documents and tables (task_resource_access.go).
//
// A run's task token authenticates as its runtime owner (R). With a human
// originator (A) on the run, the effective permission is the intersection of
// the two; on the document the run was invited onto, a grant A recorded when
// mentioning the agent lifts that floor. Without a human behind the run, R's
// permission alone applies, as before.

// agentOnRuntimeOwnedBy returns a workspace-invocable agent whose runs
// authenticate as ownerID, plus the caller shape for its requests.
func agentOnRuntimeOwnedBy(t *testing.T, ownerID string) (string, agentCaller) {
	t.Helper()
	runtime := dbfx.Runtime(t, t.Name()+" runtime", testutil.Cols{"owner_id": ownerID, "visibility": "public"})
	agentID := dbfx.Agent(t, t.Name()+" agent", runtime, testutil.Cols{
		"visibility": "workspace", "permission_mode": "public_to", "instructions": "",
		"custom_env": testutil.Raw("'{}'::jsonb"), "custom_args": testutil.Raw("'[]'::jsonb"),
	})
	dbfx.Exec(t, `INSERT INTO agent_invocation_target (agent_id, target_type, target_id) VALUES ($1, 'workspace', $2) ON CONFLICT DO NOTHING`, agentID, testWorkspaceID)
	return agentID, agentCaller{agentID: agentID}.onRuntimeOf(ownerID)
}

// runFor seeds a live run of agentID on issueID with originator as the human
// behind it ("" for none) and returns the caller for its task token.
func runFor(t *testing.T, caller agentCaller, issueID, originator string) agentCaller {
	t.Helper()
	cols := testutil.Cols{"issue_id": issueID, "runtime_id": handlerTestRuntimeID(t), "status": "running"}
	if originator != "" {
		cols["originator_user_id"] = originator
		cols["accountable_user_id"] = originator
	}
	caller.taskID = dbfx.Task(t, caller.agentID, cols)
	return caller
}

func newWorkspaceMember(t *testing.T, name string) string {
	t.Helper()
	id := dbfx.User(t, name, t.Name()+"-"+name+"@test.local")
	dbfx.Member(t, testWorkspaceID, id, "member")
	return id
}

func TestRunIsCappedByThePersonBehindIt(t *testing.T) {
	doc := createTestDocument(t, "") // private, owned by the fixture user
	runner := newWorkspaceMember(t, "runner")
	asker := newWorkspaceMember(t, "asker")
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}}, 1, 200)
	elsewhere := dbfx.Issue(t, t.Name()+" issue")
	_, agent := agentOnRuntimeOwnedBy(t, runner)
	on := func(a agentCaller, handler http.HandlerFunc, method, path string, body any) *testutil.Response {
		t.Helper()
		return testutil.Call(t, handler, a.as(withURLParam(newRequestAs(runner, method, path, body), "id", doc.ID)))
	}

	// The runtime owner alone may edit. A run asked for by someone who cannot
	// see the document sees nothing either — the runtime is not a way around
	// its audience.
	askedByOutsider := runFor(t, agent, elsewhere, asker)
	on(askedByOutsider, testHandler.GetIssue, "GET", "/api/issues/"+doc.ID, nil).Want(404)
	on(askedByOutsider, testHandler.CreateComment, "POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "forbidden"}).Want(404)
	search := testutil.Call(t, testHandler.SearchIssues, askedByOutsider.as(newRequestAs(runner, "GET", "/api/issues/search?q="+doc.ID+"&kind=doc", nil))).Want(200)
	if containsText(search.Body.String(), doc.ID) {
		t.Fatal("search leaked a document the person behind the run cannot read")
	}
	list := testutil.Call(t, testHandler.ListDocuments, askedByOutsider.as(newRequestAs(runner, "GET", "/api/documents", nil))).Want(200)
	if containsText(list.Body.String(), doc.ID) {
		t.Fatal("document list leaked a document the person behind the run cannot read")
	}

	// A viewer's run reads but does not write; an editor's run writes.
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}, {"user_id": asker, "role": "view"}}, 2, 200)
	on(askedByOutsider, testHandler.GetIssue, "GET", "/api/issues/"+doc.ID, nil).Want(200)
	on(askedByOutsider, testHandler.CreateComment, "POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "forbidden"}).Want(403)
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}, {"user_id": asker, "role": "edit"}}, 3, 200)
	on(askedByOutsider, testHandler.CreateComment, "POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "allowed"}).Want(201)

	// The owner's own run is still capped by the runtime: owner-only actions
	// need an owner on both sides.
	askedByOwner := runFor(t, agent, elsewhere, testUserID)
	on(askedByOwner, testHandler.DeleteIssue, "DELETE", "/api/issues/"+doc.ID, nil).Want(403)

	// No human behind the run: the runtime owner's permission, as before.
	unattended := runFor(t, agent, elsewhere, "")
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}}, 4, 200)
	on(unattended, testHandler.CreateComment, "POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "status quo"}).Want(201)
}

func TestRunOnATableIsCappedByThePersonBehindIt(t *testing.T) {
	table := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	runner := newWorkspaceMember(t, "runner")
	asker := newWorkspaceMember(t, "asker")
	shareCollection(t, table, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}}, 1, 200)
	elsewhere := dbfx.Issue(t, t.Name()+" issue")
	_, agent := agentOnRuntimeOwnedBy(t, runner)
	on := func(a agentCaller, handler http.HandlerFunc, method string, body any) *testutil.Response {
		t.Helper()
		return testutil.Call(t, handler, a.as(withURLParams(newRequestAs(runner, method, "/api/collections/"+table, body), "collectionID", table)))
	}
	run := runFor(t, agent, elsewhere, asker)
	on(run, testHandler.GetCollection, "GET", nil).Want(404)
	on(run, testHandler.CreateCollectionRecord, "POST", map[string]any{"title": "forbidden"}).Want(404)
	shareCollection(t, table, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}, {"user_id": asker, "role": "view"}}, 2, 200)
	on(run, testHandler.GetCollection, "GET", nil).Want(200)
	on(run, testHandler.CreateCollectionRecord, "POST", map[string]any{"title": "forbidden"}).Want(403)
	on(runFor(t, agent, elsewhere, ""), testHandler.CreateCollectionRecord, "POST", map[string]any{"title": "status quo"}).Want(201)
}

// mentionWithGrants posts a comment on doc as the fixture user that mentions
// agentID with the given grants and returns the task the mention enqueued.
func mentionWithGrants(t *testing.T, doc IssueResponse, agentID string, grants []map[string]string, status int) string {
	t.Helper()
	body := map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") please review this"}
	if grants != nil {
		body["agent_grants"] = grants
	}
	response := testutil.Call(t, testHandler.CreateComment, withURLParam(newRequest("POST", "/api/issues/"+doc.ID+"/comments", body), "id", doc.ID)).Want(status)
	if status != 201 {
		return ""
	}
	var comment CommentResponse
	response.JSON(&comment)
	var taskID string
	dbfx.QueryRow(t, `SELECT id FROM agent_task_queue WHERE trigger_comment_id=$1 AND agent_id=$2`, comment.ID, agentID).Scan(&taskID)
	if taskID == "" {
		t.Fatalf("mentioning the agent enqueued no run: %s", response.Body.String())
	}
	dbfx.Cleanup(t, "DELETE FROM agent_task_queue WHERE id=$1", taskID)
	return taskID
}

func TestInvitedRunUsesTheGrantOnItsDocument(t *testing.T) {
	doc := createTestDocument(t, "") // private, owned by the fixture user
	child := createTestDocument(t, doc.ID)
	runner := newWorkspaceMember(t, "runner")
	agentID, agent := agentOnRuntimeOwnedBy(t, runner)
	on := func(a agentCaller, handler http.HandlerFunc, method, id, path string, body any) *testutil.Response {
		t.Helper()
		return testutil.Call(t, handler, a.as(withURLParam(newRequestAs(runner, method, path, body), "id", id)))
	}

	// Mentioned with no grant: the run gets the runtime owner's (no) access.
	agent.taskID = mentionWithGrants(t, doc, agentID, nil, 201)
	on(agent, testHandler.GetIssue, "GET", doc.ID, "/api/issues/"+doc.ID, nil).Want(404)
	dbfx.Exec(t, "DELETE FROM agent_task_queue WHERE id=$1", agent.taskID)

	// Granted edit: read, reply and save on that document; nothing an owner
	// does, and nothing on any other document.
	agent.taskID = mentionWithGrants(t, doc, agentID, []map[string]string{{"agent_id": agentID, "permission": "edit"}}, 201)
	var triggerCommentID string
	dbfx.QueryRow(t, `SELECT trigger_comment_id FROM agent_task_queue WHERE id=$1`, agent.taskID).Scan(&triggerCommentID)
	on(agent, testHandler.GetIssue, "GET", doc.ID, "/api/issues/"+doc.ID, nil).Want(200)
	on(agent, testHandler.ListComments, "GET", doc.ID, "/api/issues/"+doc.ID+"/comments", nil).Want(200)
	on(agent, testHandler.CreateComment, "POST", doc.ID, "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "Assessment attached.", "parent_id": triggerCommentID}).Want(201)
	on(agent, testHandler.UpdateIssue, "PUT", doc.ID, "/api/issues/"+doc.ID, map[string]any{"description": "edited by the invited run", "expected_document_revision": 1}).Want(200)
	on(agent, testHandler.UpdateDocumentAccess, "PUT", doc.ID, "/api/documents/"+doc.ID+"/access", map[string]any{"scope": "workspace", "expected_revision": 1}).Want(403)
	on(agent, testHandler.DeleteIssue, "DELETE", doc.ID, "/api/issues/"+doc.ID, nil).Want(403)
	on(agent, testHandler.GetIssue, "GET", child.ID, "/api/issues/"+child.ID, nil).Want(404)
	var access struct {
		CanEdit   bool `json:"can_edit"`
		CanManage bool `json:"can_manage"`
	}
	on(agent, testHandler.GetDocumentAccess, "GET", doc.ID, "/api/documents/"+doc.ID+"/access", nil).Want(200).JSON(&access)
	if !access.CanEdit || access.CanManage {
		t.Fatalf("access as seen by the invited run = %+v, want edit without manage", access)
	}

	// The grant is a floor, not a ceiling: sharing the document with the
	// runtime owner as a viewer takes nothing away from the invited run.
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "view"}}, 1, 200)
	on(agent, testHandler.CreateComment, "POST", doc.ID, "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "still allowed", "parent_id": triggerCommentID}).Want(201)

	// Granted view only: read, not reply.
	dbfx.Exec(t, "DELETE FROM agent_task_queue WHERE id=$1", agent.taskID)
	shareDocument(t, doc.ID, "private", "view", nil, nil, 2, 200)
	agent.taskID = mentionWithGrants(t, doc, agentID, []map[string]string{{"agent_id": agentID, "permission": "view"}}, 201)
	on(agent, testHandler.GetIssue, "GET", doc.ID, "/api/issues/"+doc.ID, nil).Want(200)
	on(agent, testHandler.CreateComment, "POST", doc.ID, "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "forbidden"}).Want(403)

	// A grant dies with the run.
	dbfx.Exec(t, "UPDATE agent_task_queue SET status='completed' WHERE id=$1", agent.taskID)
	on(agent, testHandler.GetIssue, "GET", doc.ID, "/api/issues/"+doc.ID, nil).Want(404)

	// The headers alone, without the task-token stamp, grant nothing.
	spoof := withURLParam(newRequestAs(runner, "GET", "/api/issues/"+doc.ID, nil), "id", doc.ID)
	spoof.Header.Set("X-Task-ID", agent.taskID)
	testutil.Call(t, testHandler.GetIssue, spoof).Want(404)
}

func TestCommentAgentGrantsAreValidated(t *testing.T) {
	doc := createTestDocument(t, "")
	runner := newWorkspaceMember(t, "runner")
	viewer := newWorkspaceMember(t, "viewer")
	agentID, agent := agentOnRuntimeOwnedBy(t, runner)
	post := func(user string, issueID string, grants []map[string]string) *testutil.Response {
		t.Helper()
		body := map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") hi", "agent_grants": grants}
		return testutil.Call(t, testHandler.CreateComment, withURLParam(newRequestAs(user, "POST", "/api/issues/"+issueID+"/comments", body), "id", issueID))
	}
	post(testUserID, doc.ID, []map[string]string{{"agent_id": agentID, "permission": "owner"}}).Want(400)
	post(testUserID, doc.ID, []map[string]string{{"agent_id": "not-a-uuid", "permission": "edit"}}).Want(400)
	issue := dbfx.Issue(t, t.Name()+" issue")
	post(testUserID, issue, []map[string]string{{"agent_id": agentID, "permission": "edit"}}).Want(400)
	// A viewer may pass on reading, not editing.
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 1, 200)
	post(viewer, doc.ID, []map[string]string{{"agent_id": agentID, "permission": "edit"}}).Want(403)
	// A run cannot grant.
	run := runFor(t, agent, doc.ID, testUserID)
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}, {"user_id": runner, "role": "edit"}}, 2, 200)
	body := map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") hi", "agent_grants": []map[string]string{{"agent_id": agentID, "permission": "edit"}}}
	testutil.Call(t, testHandler.CreateComment, run.as(withURLParam(newRequestAs(runner, "POST", "/api/issues/"+doc.ID+"/comments", body), "id", doc.ID))).Want(403)
}

func TestPreviewReportsWhatARunCanReachOnTheDocument(t *testing.T) {
	doc := createTestDocument(t, "")
	runner := newWorkspaceMember(t, "runner")
	agentID, _ := agentOnRuntimeOwnedBy(t, runner)
	preview := func(user string) CommentTriggerPreviewResponse {
		t.Helper()
		var out CommentTriggerPreviewResponse
		body := map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") hi"}
		testutil.Call(t, testHandler.PreviewCommentTriggers, withURLParam(newRequestAs(user, "POST", "/api/issues/"+doc.ID+"/comments/preview-triggers", body), "id", doc.ID)).Want(200).JSON(&out)
		if len(out.Agents) != 1 || out.Agents[0].ID != agentID || out.Agents[0].DocumentAccess == nil {
			t.Fatalf("preview = %+v, want the agent with document access", out)
		}
		return out
	}
	access := preview(testUserID).Agents[0].DocumentAccess
	if access.RuntimeOwnerPermission != "" || access.MaxGrant != "edit" {
		t.Fatalf("owner mentioning an agent whose runtime cannot read: %+v", access)
	}
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "view"}}, 1, 200)
	access = preview(testUserID).Agents[0].DocumentAccess
	if access.RuntimeOwnerPermission != "view" || access.MaxGrant != "edit" {
		t.Fatalf("runtime can read but not write: %+v", access)
	}
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": runner, "role": "edit"}}, 2, 200)
	access = preview(testUserID).Agents[0].DocumentAccess
	if access.RuntimeOwnerPermission != "edit" || access.MaxGrant != "" {
		t.Fatalf("runtime can already write, nothing to grant: %+v", access)
	}
	// A viewer cannot post comments, so the composer preview is also denied.
	viewer := newWorkspaceMember(t, "viewer")
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 3, 200)
	testutil.Call(t, testHandler.PreviewCommentTriggers, withURLParam(newRequestAs(viewer, "POST", "/api/issues/"+doc.ID+"/comments/preview-triggers", map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") hi"}), "id", doc.ID)).Want(403)
	// A task issue carries no document access.
	issue := dbfx.Issue(t, t.Name()+" issue")
	var out CommentTriggerPreviewResponse
	testutil.Call(t, testHandler.PreviewCommentTriggers, withURLParam(newRequest("POST", "/api/issues/"+issue+"/comments/preview-triggers", map[string]any{"content": "[@Agent](mention://agent/" + agentID + ") hi"}), "id", issue)).Want(200).JSON(&out)
	if len(out.Agents) != 1 || out.Agents[0].DocumentAccess != nil {
		t.Fatalf("preview on a task issue = %+v", out)
	}
}

func TestWhatARunCreatesBelongsToThePersonBehindIt(t *testing.T) {
	prepareDocumentStatuses(t)
	runner := newWorkspaceMember(t, "runner")
	asker := newWorkspaceMember(t, "asker")
	_, agent := agentOnRuntimeOwnedBy(t, runner)
	elsewhere := dbfx.Issue(t, t.Name()+" issue")
	run := runFor(t, agent, elsewhere, asker)

	var doc IssueResponse
	testutil.Call(t, testHandler.CreateIssue, run.as(newRequestAs(runner, "POST", "/api/issues", map[string]any{"kind": "doc", "title": t.Name(), "description": "v1", "allow_duplicate": true}))).Want(201).JSON(&doc)
	dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", doc.ID)
	dbfx.Cleanup(t, "DELETE FROM document_publication WHERE issue_id=$1", doc.ID)
	var access struct {
		OwnerID       string              `json:"owner_id"`
		Collaborators []map[string]string `json:"collaborators"`
	}
	testutil.Call(t, testHandler.GetDocumentAccess, withURLParam(newRequestAs(asker, "GET", "/api/documents/"+doc.ID+"/access", nil), "id", doc.ID)).Want(200).JSON(&access)
	if access.OwnerID != asker || len(access.Collaborators) != 1 || access.Collaborators[0]["user_id"] != runner || access.Collaborators[0]["role"] != "edit" {
		t.Fatalf("document a run created: %+v, want owned by the asker with the runtime owner as editor", access)
	}
	// The asker sees it; the run keeps working on it.
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequestAs(asker, "GET", "/api/issues/"+doc.ID, nil), "id", doc.ID)).Want(200)
	testutil.Call(t, testHandler.UpdateIssue, run.as(withURLParam(newRequestAs(runner, "PUT", "/api/issues/"+doc.ID, map[string]any{"description": "v2", "expected_document_revision": 1}), "id", doc.ID))).Want(200)

	var table struct {
		ID string `json:"id"`
	}
	testutil.Call(t, testHandler.CreateCollection, run.as(newRequestAs(runner, "POST", "/api/collections", map[string]any{"name": t.Name()}))).Want(201).JSON(&table)
	dbfx.Cleanup(t, "DELETE FROM collection WHERE id=$1", table.ID)
	var tableAccess struct {
		OwnerID       string              `json:"owner_id"`
		Collaborators []map[string]string `json:"collaborators"`
	}
	testutil.Call(t, testHandler.GetCollectionAccess, withURLParams(newRequestAs(asker, "GET", "/api/collections/"+table.ID+"/access", nil), "collectionID", table.ID)).Want(200).JSON(&tableAccess)
	if tableAccess.OwnerID != asker || len(tableAccess.Collaborators) != 1 || tableAccess.Collaborators[0]["user_id"] != runner {
		t.Fatalf("table a run created: %+v, want owned by the asker with the runtime owner as editor", tableAccess)
	}
	testutil.Call(t, testHandler.CreateCollectionRecord, run.as(withURLParams(newRequestAs(runner, "POST", "/api/collections/"+table.ID+"/records", map[string]any{"title": "by the run"}), "collectionID", table.ID))).Want(201)
}
