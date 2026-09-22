package handler

import (
	"context"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/documentaccess"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func shareDocument(t *testing.T, id, scope, role string, project *string, people []map[string]string, rev int, status int) {
	t.Helper()
	testutil.Call(t, testHandler.UpdateDocumentAccess, withURLParam(newRequest("PUT", "/api/documents/"+id+"/access", map[string]any{"scope": scope, "scope_role": role, "project_id": project, "collaborators": people, "expected_revision": rev}), "id", id)).Want(status)
}
func TestDocumentPrivateAudienceAndRoles(t *testing.T) {
	doc := createTestDocument(t, "")
	viewer := dbfx.User(t, "Document reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, viewer, "admin")
	other := dbfx.User(t, "Other reader", t.Name()+"other@test.local")
	dbfx.Member(t, testWorkspaceID, other, "member")
	call := func(handler http.HandlerFunc, user, method, path string, body any, status int) {
		t.Helper()
		testutil.Call(t, handler, withURLParam(newRequestAs(user, method, path, body), "id", doc.ID)).Want(status)
	}
	call(testHandler.GetIssue, viewer, "GET", "/api/issues/"+doc.ID, nil, 404)
	// A readable identifier must take the same path as a UUID, including for admins.
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequestAs(viewer, "GET", "/api/issues/"+doc.Identifier, nil), "id", doc.Identifier)).Want(404)
	for _, url := range []string{"/api/issues?open_only=true", "/api/issues/search?q=" + doc.ID + "&kind=doc", "/api/documents"} {
		handler := testHandler.ListDocuments
		if url == "/api/issues?open_only=true" {
			handler = testHandler.ListIssues
		} else if url != "/api/documents" {
			handler = testHandler.SearchIssues
		}
		result := testutil.Call(t, handler, newRequestAs(viewer, "GET", url, nil)).Want(200)
		if containsText(result.Body.String(), doc.ID) {
			t.Fatalf("private document leaked from %s: %s", url, result.Body.String())
		}
	}
	call(testHandler.UpdateDocumentAccess, viewer, "PUT", "/api/documents/"+doc.ID+"/access", map[string]any{"scope": "workspace", "expected_revision": 1}, 404)
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "view"}}, 1, 200)
	call(testHandler.GetIssue, viewer, "GET", "/api/issues/"+doc.ID, nil, 200)
	call(testHandler.GetIssue, other, "GET", "/api/issues/"+doc.ID, nil, 404)
	call(testHandler.UpdateIssue, viewer, "PUT", "/api/issues/"+doc.ID, map[string]any{"title": "forbidden"}, 403)
	call(testHandler.ListDocumentVersions, viewer, "GET", "/api/documents/"+doc.ID+"/versions", nil, 403)
	call(testHandler.CreateComment, viewer, "POST", "/api/issues/"+doc.ID+"/comments", map[string]any{"content": "forbidden"}, 403)
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": viewer, "role": "edit"}}, 2, 200)
	call(testHandler.UpdateIssue, viewer, "PUT", "/api/issues/"+doc.ID, map[string]any{"title": "An editor can save"}, 200)
	call(testHandler.ListDocumentVersions, viewer, "GET", "/api/documents/"+doc.ID+"/versions", nil, 200)
	call(testHandler.UpdateDocumentAccess, viewer, "PUT", "/api/documents/"+doc.ID+"/access", map[string]any{"scope": "workspace", "expected_revision": 3}, 403)
	call(testHandler.DeleteIssue, viewer, "DELETE", "/api/issues/"+doc.ID, nil, 403)
	call(testHandler.BatchDeleteIssues, viewer, "POST", "/api/issues/batch-delete", map[string]any{"issue_ids": []string{doc.ID}}, 403)
	shareDocument(t, doc.ID, "private", "view", nil, nil, 3, 200)
	call(testHandler.GetIssue, viewer, "GET", "/api/issues/"+doc.ID, nil, 404)
	shareDocument(t, doc.ID, "workspace", "edit", nil, nil, 4, 200)
	call(testHandler.UpdateIssue, other, "PUT", "/api/issues/"+doc.ID, map[string]any{"title": "Workspace editor"}, 200)
	shareDocument(t, doc.ID, "workspace", "view", nil, nil, 4, 409)
	project := dbfx.Project(t, "Document project")
	shareDocument(t, doc.ID, "project", "view", &project, nil, 5, 200)
	call(testHandler.GetIssue, other, "GET", "/api/issues/"+doc.ID, nil, 200)
	call(testHandler.UpdateIssue, other, "PUT", "/api/issues/"+doc.ID, map[string]any{"title": "forbidden"}, 403)
	shareDocument(t, doc.ID, "project", "edit", &project, nil, 6, 200)
	call(testHandler.UpdateIssue, other, "PUT", "/api/issues/"+doc.ID, map[string]any{"title": "Project editor"}, 200)
	testutil.Call(t, testHandler.DeleteProject, withURLParam(newRequest("DELETE", "/api/projects/"+project, nil), "id", project)).Want(204)
	call(testHandler.GetIssue, other, "GET", "/api/issues/"+doc.ID, nil, 404)
	var withdrawn IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest("GET", "/api/issues/"+doc.ID, nil), "id", doc.ID)).Want(200).JSON(&withdrawn)
	if withdrawn.Status != "draft" {
		t.Fatal("deleted project retained publication status")
	}

}

func TestDocumentVersionsRestoreAndConflict(t *testing.T) {
	doc := createTestDocument(t, "")
	shareDocument(t, doc.ID, "workspace", "view", nil, nil, 1, 200)
	var saved IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"title": "second", "description": "second body", "expected_document_revision": 1}), "id", doc.ID)).Want(200).JSON(&saved)
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "stale", "expected_document_revision": 1}), "id", doc.ID)).Want(409)
	var versions struct {
		Versions []db.ListDocumentVersionsRow `json:"versions"`
	}
	testutil.Call(t, testHandler.ListDocumentVersions, withURLParam(newRequest("GET", "/api/documents/"+doc.ID+"/versions", nil), "id", doc.ID)).Want(200).JSON(&versions)
	if len(versions.Versions) != 2 || uuidToString(versions.Versions[0].ActorID) != testUserID {
		t.Fatalf("snapshots: %+v", versions)
	}
	restore := func(rev int64, status int) IssueResponse {
		t.Helper()
		var out IssueResponse
		req := withURLParam(newRequest("POST", "/api/documents/"+doc.ID+"/versions/1/restore", map[string]any{"expected_revision": rev}), "id", doc.ID)
		chi.RouteContext(req.Context()).URLParams.Add("version", "1")
		result := testutil.Call(t, testHandler.RestoreDocumentVersion, req).Want(status)
		if status == 200 {
			result.JSON(&out)
		}
		return out
	}
	restore(1, 409)
	restored := restore(saved.Revision, 200)
	if restored.Title != doc.Title || *restored.Description != "Original body" || restored.Status != "published" || restored.DocumentRevision != 3 {
		t.Fatalf("restored: %+v", restored)
	}
	snapshot, err := testHandler.Queries.GetDocumentVersion(context.Background(), db.GetDocumentVersionParams{IssueID: parseUUID(doc.ID), WorkspaceID: parseUUID(testWorkspaceID), Version: 3})
	if err != nil || snapshot.Action != "restore" || snapshot.RestoredFrom.Int64 != 1 {
		t.Fatalf("restore snapshot: %+v %v", snapshot, err)
	}
}

func TestDocumentEventsNotificationsAndAttachments(t *testing.T) {
	doc := createTestDocument(t, "")
	other := dbfx.User(t, "Blocked", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, other, "member")
	policy := documentaccess.EventPolicy(testHandler.Queries)
	event, allowed := policy(events.Event{Type: "issue:updated", WorkspaceID: testWorkspaceID, Payload: map[string]any{"issue": doc}})
	if !allowed || len(event.RecipientUserIDs) != 1 || event.RecipientUserIDs[0] != testUserID {
		t.Fatalf("private audience: %+v %v", event, allowed)
	}
	attachment := dbfx.Insert(t, "attachment", testutil.Cols{"workspace_id": testWorkspaceID, "issue_id": doc.ID, "uploader_type": "member", "uploader_id": testUserID, "filename": "private.txt", "url": "http://localhost/uploads/doc-private.txt", "content_type": "text/plain", "size_bytes": 3})
	testutil.Call(t, testHandler.GetAttachmentByID, withURLParam(newRequestAs(other, "GET", "/api/attachments/"+attachment, nil), "id", attachment)).Want(404)
	testutil.Call(t, testHandler.DownloadAttachment, withURLParam(newRequestAs(other, "GET", "/api/attachments/"+attachment+"/download", nil), "id", attachment)).Want(404)
	localHandler := *testHandler
	t.Setenv("LOCAL_UPLOAD_DIR", t.TempDir())
	t.Setenv("LOCAL_UPLOAD_BASE_URL", "http://localhost")
	localHandler.Storage = storage.NewLocalStorageFromEnv()
	for _, path := range []string{"/uploads/doc-private.txt", "/uploads/nested/../doc-private.txt"} {
		response := testutil.Call(t, localHandler.ServeLocalUpload, newRequestAs(other, "GET", path, nil)).Want(http.StatusTemporaryRedirect)
		if response.Header().Get("Location") != "/api/attachments/"+attachment+"/download" {
			t.Fatal("raw document upload bypassed authorization")
		}
	}
	item := dbfx.Insert(t, "inbox_item", testutil.Cols{"workspace_id": testWorkspaceID, "recipient_type": "member", "recipient_id": other, "type": "mention", "title": "private title", "body": "private body", "issue_id": doc.ID})
	testutil.Call(t, inboxWorkspaceHandler(testHandler.ListInbox), withURLParam(newRequestAs(other, "GET", "/api/inbox", nil), "workspaceId", testWorkspaceID)).Want(200)
	if _, err := testHandler.Queries.GetInboxItem(context.Background(), parseUUID(item)); err == nil {
		t.Fatal("old revoked notification remained readable")
	}
	shareDocument(t, doc.ID, "workspace", "view", nil, nil, 1, 200)
	if _, err := testHandler.Queries.GetInboxItem(context.Background(), parseUUID(item)); err != nil {
		t.Fatal(err)
	}
	event, allowed = policy(events.Event{Type: "comment:created", WorkspaceID: testWorkspaceID, Payload: map[string]any{"comment": map[string]any{"issue_id": doc.ID, "content": "shared"}}})
	if !allowed || !contains(event.RecipientUserIDs, other) {
		t.Fatalf("shared audience: %+v", event)
	}
}
func containsText(value, part string) bool { return len(part) > 0 && strings.Contains(value, part) }

func TestDocumentRunSurfacesRespectAudience(t *testing.T) {
	doc := createTestDocument(t, "")
	reader := dbfx.User(t, "Run reader", t.Name()+"@test.local")
	dbfx.Member(t, testWorkspaceID, reader, "member")
	member, err := testHandler.Queries.GetMemberByUserAndWorkspace(context.Background(), db.GetMemberByUserAndWorkspaceParams{UserID: parseUUID(reader), WorkspaceID: parseUUID(testWorkspaceID)})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, key, id string) *http.Request {
		req := withURLParam(newRequestAs(reader, method, path, nil), key, id)
		return req.WithContext(middleware.SetMemberContext(req.Context(), testWorkspaceID, member))
	}

	agent := createHandlerTestAgent(t, "Document run agent", []byte("[]"))
	run := dbfx.Task(t, agent, testutil.Cols{"issue_id": doc.ID, "runtime_id": handlerTestRuntimeID(t), "status": "running"})
	testutil.Call(t, testHandler.ListTaskMessagesByUser, request("GET", "/api/tasks/"+run+"/messages", "taskId", run)).Want(404)
	testutil.Call(t, testHandler.CancelTaskByUser, request("POST", "/api/tasks/"+run+"/cancel", "taskId", run)).Want(404)
	for _, handler := range []http.HandlerFunc{testHandler.ListAgentTasks, testHandler.ListWorkspaceAgentTaskSnapshot} {
		result := testutil.Call(t, handler, request("GET", "/api/agents/"+agent+"/tasks", "id", agent)).Want(200)
		if strings.Contains(result.Body.String(), run) {
			t.Fatal("private document run leaked through agent history")
		}
	}
	shareDocument(t, doc.ID, "private", "view", nil, []map[string]string{{"user_id": reader, "role": "view"}}, 1, 200)
	testutil.Call(t, testHandler.ListTaskMessagesByUser, request("GET", "/api/tasks/"+run+"/messages", "taskId", run)).Want(200)
	testutil.Call(t, testHandler.CancelTaskByUser, request("POST", "/api/tasks/"+run+"/cancel", "taskId", run)).Want(403)
	child := createTestDocument(t, doc.ID)
	hiddenRun := dbfx.Task(t, agent, testutil.Cols{"issue_id": child.ID, "runtime_id": handlerTestRuntimeID(t), "status": "running"})
	family := testutil.Call(t, testHandler.ListTasksByIssue, request("GET", "/api/issues/"+doc.ID+"/tasks?scope=family", "id", doc.ID)).Want(200)
	if strings.Contains(family.Body.String(), hiddenRun) || !strings.Contains(family.Body.String(), run) {
		t.Fatal("document family runs did not apply each page's audience")
	}
}
