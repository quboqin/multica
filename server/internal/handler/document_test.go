package handler

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/channelmedia"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func prepareDocumentStatuses(t *testing.T) {
	t.Helper()
	for _, key := range []string{"draft", "reviewing", "published"} {
		var exists bool
		dbfx.QueryRow(t, "SELECT EXISTS(SELECT 1 FROM issue_status WHERE workspace_id=$1 AND key=$2)", testWorkspaceID, key).Scan(&exists)
		if !exists {
			t.Cleanup(func() {
				if _, err := testPool.Exec(context.Background(), "DELETE FROM issue_status WHERE workspace_id=$1 AND key=$2", testWorkspaceID, key); err != nil {
					t.Errorf("cleanup document status %s: %v", key, err)
				}
			})
		}
	}
}
func createTestDocument(t *testing.T, parent string) IssueResponse {
	t.Helper()
	prepareDocumentStatuses(t)
	req := map[string]any{"kind": "doc", "title": t.Name() + parent, "description": "Original body", "allow_duplicate": true}
	if parent != "" {
		req["parent_issue_id"] = parent
	}
	var doc IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues", req)).Want(201).JSON(&doc)
	dbfx.Cleanup(t, "DELETE FROM issue WHERE id=$1", doc.ID)
	dbfx.Cleanup(t, "DELETE FROM document_publication WHERE issue_id=$1", doc.ID)
	if doc.Kind != "doc" || doc.DocumentRevision != 1 || doc.Status != "draft" {
		t.Fatalf("document defaults: %+v", doc)
	}
	return doc
}
func TestDocumentBodyCASAndTaskCompatibility(t *testing.T) {
	doc := createTestDocument(t, "")
	save := func(body any, status int) IssueResponse {
		t.Helper()
		var out IssueResponse
		result := testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, body), "id", doc.ID)).Want(status)
		if status == 200 {
			result.JSON(&out)
		}
		return out
	}
	save(map[string]any{"description": "Legacy overwrite"}, 409)
	changed := save(map[string]any{"description": "Writer one", "expected_document_revision": 1}, 200)
	if changed.DocumentRevision != 2 {
		t.Fatalf("body version=%d", changed.DocumentRevision)
	}
	save(map[string]any{"description": "Writer two", "expected_document_revision": 1}, 409)
	title := save(map[string]any{"title": "Unrelated edit"}, 200)
	if title.DocumentRevision != 2 {
		t.Fatal("title changed the body version")
	}
	echoed := save(map[string]any{"description": "Writer one", "expected_document_revision": 2}, 200)
	if echoed.DocumentRevision != 2 {
		t.Fatal("own echo changed version")
	}
	save(map[string]any{"description": "Writer next", "expected_document_revision": 2}, 200)
	if _, err := testPool.Exec(context.Background(), "UPDATE issue SET description='bypass' WHERE id=$1", doc.ID); err == nil {
		t.Fatal("database allowed an unversioned legacy writer")
	}
	task := dbfx.Issue(t, "Task compatibility")
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+task, map[string]any{"description": "Task remains compatible"}), "id", task)).Want(200)
}
func TestDocumentConcurrentSavesOneWinner(t *testing.T) {
	doc := createTestDocument(t, "")
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := httptest.NewRecorder()
			testHandler.UpdateIssue(response, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"description": fmt.Sprintf("writer %d", i), "expected_document_revision": 1}), "id", doc.ID))
			statuses <- response.Code
		}()
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("save statuses=%v", counts)
	}
}
func TestDocumentTreeMovesWholeSubtreeAndRejectsCycles(t *testing.T) {
	root := createTestDocument(t, "")
	parent := root
	for range 4 {
		parent = createTestDocument(t, parent.ID)
	}
	other := createTestDocument(t, "")
	testutil.Call(t, testHandler.MoveDocument, withURLParam(newRequest("POST", "/api/documents/"+root.ID+"/move", map[string]any{"parent_issue_id": parent.ID, "position": 0}), "id", root.ID)).Want(400)
	testutil.Call(t, testHandler.MoveDocument, withURLParam(newRequest("POST", "/api/documents/"+root.ID+"/move", map[string]any{"parent_issue_id": other.ID, "position": 0}), "id", root.ID)).Want(200)
	var actualParent string
	dbfx.QueryRow(t, "SELECT parent_issue_id::text FROM issue WHERE id=$1", root.ID).Scan(&actualParent)
	if actualParent != other.ID {
		t.Fatal("root was not reparented")
	}
	var depth int
	dbfx.QueryRow(t, "WITH RECURSIVE path AS (SELECT id,parent_issue_id,1 depth FROM issue WHERE id=$1 UNION ALL SELECT i.id,i.parent_issue_id,p.depth+1 FROM issue i JOIN path p ON i.id=p.parent_issue_id) SELECT max(depth) FROM path", parent.ID).Scan(&depth)
	if depth != 6 {
		t.Fatalf("subtree path depth=%d", depth)
	}

}
func TestDocumentAuthorizedVersionedPublication(t *testing.T) {
	doc := createTestDocument(t, "")
	transition := func(action string, status int) *http.Request {
		return withURLParam(newRequest("POST", "/api/documents/"+doc.ID+"/transition", map[string]any{"action": action, "expected_document_revision": 1}), "id", doc.ID)
	}
	testutil.Call(t, testHandler.TransitionDocument, transition("publish", 409)).Want(409)
	testutil.Call(t, testHandler.TransitionDocument, transition("review", 200)).Want(200)
	machine := transition("publish", 403)
	machine.Header.Set("X-Actor-Source", "task_token")
	testutil.Call(t, testHandler.TransitionDocument, machine).Want(403)
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"status": "published"}), "id", doc.ID)).Want(400)
	testutil.Call(t, testHandler.TransitionDocument, transition("publish", 200)).Want(200)
	var count int
	dbfx.QueryRow(t, "SELECT count(*) FROM document_publication WHERE issue_id=$1 AND document_revision=1 AND actor_id=$2 AND action='publish' AND ingestion_state='pending'", doc.ID, testUserID).Scan(&count)
	if count != 1 {
		t.Fatalf("publication audit/outbox count=%d", count)
	}
	var updated IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "New revision after approval", "expected_document_revision": 1}), "id", doc.ID)).Want(200).JSON(&updated)
	if updated.Status != "draft" || updated.DocumentRevision != 2 {
		t.Fatalf("editing approved content did not invalidate approval: %+v", updated)
	}
}

func TestDocumentLateMediaPreservesDraftVersion(t *testing.T) {
	doc := createTestDocument(t, "")
	attachment := dbfx.Insert(t, "attachment", testutil.Cols{"workspace_id": testWorkspaceID, "issue_id": doc.ID, "uploader_type": "member", "uploader_id": testUserID, "filename": "diagram.png", "url": "https://cdn.example.test/g1-media", "content_type": "image/png", "size_bytes": 3})
	block := channelmedia.Block(attachment, "diagram.png", true)
	materialized, err := testHandler.Queries.MaterializeIssueChannelMediaMarkdown(context.Background(), db.MaterializeIssueChannelMediaMarkdownParams{ID: parseUUID(doc.ID), WorkspaceID: parseUUID(testWorkspaceID), Markdown: pgtype.Text{String: block, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	if materialized.DocumentRevision != 1 {
		t.Fatalf("system attachment advanced author body version: %d", materialized.DocumentRevision)
	}
	var saved IssueResponse
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"description": "Local draft before media arrived", "description_base": "Original body", "expected_document_revision": 1}), "id", doc.ID)).Want(200).JSON(&saved)
	if saved.Description == nil || !strings.Contains(*saved.Description, channelmedia.DownloadPath(attachment)) || !strings.Contains(*saved.Description, "Local draft") || saved.DocumentRevision != 2 {
		t.Fatalf("media/draft merge: %+v", saved)
	}
}
func TestDocumentBodyMentionsAreReferencesAndCommentsKeepTheirRouter(t *testing.T) {
	prepareDocumentStatuses(t)
	agent := seededReadyAgentID(t)
	body := fmt.Sprintf("[@Agent](mention://agent/%s)\n\n```text\n[@Example](mention://agent/%s)\n```", agent, agent)
	doc := createIssueForTest(t, map[string]any{"title": t.Name(), "kind": "doc", "description": body, "assignee_type": "agent", "assignee_id": agent})
	if count := taskCountFor(t, doc.ID, agent); count != 0 {
		t.Fatalf("document creation scheduled %d tasks", count)
	}
	preview := previewIssueTrigger(t, map[string]any{"kind": "doc", "is_create": true, "status": "draft", "assignee_type": "agent", "assignee_id": agent})
	if preview.TotalCount != 0 {
		t.Fatal("document create preview offered execution")
	}
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, map[string]any{"description": body + "\nReference edit", "expected_document_revision": 1}), "id", doc.ID)).Want(200)
	if count := taskCountFor(t, doc.ID, agent); count != 0 {
		t.Fatalf("document save scheduled %d tasks", count)
	}
	postCommentForTriggerPreviewTest(t, doc.ID, map[string]any{"content": "/note " + body})
	if count := taskCountFor(t, doc.ID, agent); count != 0 {
		t.Fatalf("note scheduled %d tasks", count)
	}
	mention := fmt.Sprintf("[@Agent](mention://agent/%s) Please review", agent)
	commentPreview := previewCommentTriggersForTest(t, doc.ID, map[string]any{"content": mention})
	if len(commentPreview.Agents) != 1 {
		t.Fatalf("explicit comment mention preview=%+v", commentPreview)
	}
	postCommentForTriggerPreviewTest(t, doc.ID, map[string]any{"content": mention})
	if count := countQueuedCommentTriggerTasks(t, doc.ID, agent); count != 1 {
		t.Fatalf("explicit comment mention queued %d tasks", count)
	}
}
func TestDocumentLibraryFiftyPagesSearchAndSiblingOrder(t *testing.T) {
	root := createTestDocument(t, "")
	pages := []IssueResponse{root}
	parent := root
	for i := 1; i < 50; i++ {
		if i < 5 {
			parent = createTestDocument(t, parent.ID)
			pages = append(pages, parent)
		} else {
			pages = append(pages, createTestDocument(t, root.ID))
		}
	}
	var library []IssueResponse
	testutil.Call(t, testHandler.ListDocuments, newRequest("GET", "/api/documents", nil)).Want(200).JSON(&library)
	found := map[string]bool{}
	for _, doc := range library {
		found[doc.ID] = true
	}
	for _, doc := range pages {
		if !found[doc.ID] {
			t.Fatalf("missing library page %s", doc.ID)
		}
	}
	first, last := pages[1], pages[49]
	testutil.Call(t, testHandler.MoveDocument, withURLParam(newRequest("POST", "/api/documents/"+last.ID+"/move", map[string]any{"parent_issue_id": root.ID, "before_id": first.ID}), "id", last.ID)).Want(200)
	var before, after float64
	dbfx.QueryRow(t, "SELECT position FROM issue WHERE id=$1", last.ID).Scan(&before)
	dbfx.QueryRow(t, "SELECT position FROM issue WHERE id=$1", first.ID).Scan(&after)
	if before >= after {
		t.Fatalf("sibling order %f >= %f", before, after)
	}
	testutil.Call(t, testHandler.BatchUpdateIssues, newRequest("POST", "/api/issues/batch-update", map[string]any{"issue_ids": []string{root.ID}, "updates": map[string]any{"parent_issue_id": parent.ID}})).Want(400)
	var search struct {
		Issues []SearchIssueResponse `json:"issues"`
	}
	testutil.Call(t, testHandler.SearchIssues, newRequest("GET", "/api/issues/search?q=Original&kind=doc", nil)).Want(200).JSON(&search)
	if len(search.Issues) == 0 {
		t.Fatal("document body was not searchable")
	}
	for _, doc := range search.Issues {
		if doc.Kind != "doc" {
			t.Fatal("kind filter leaked tasks")
		}
	}
}
