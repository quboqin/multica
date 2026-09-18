package handler

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issueguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestDocumentCreateSaveAndTableIsolation(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.CortexDocs, true)
	var doc IssueResponse
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues", map[string]any{
		"kind": "doc", "title": "Document " + t.Name(), "description": "Initial body", "allow_duplicate": true,
	})).Want(http.StatusCreated).JSON(&doc)
	dbfx.Cleanup(t, `DELETE FROM issue WHERE id = $1`, doc.ID)
	if doc.Kind != "doc" {
		t.Fatalf("kind = %q", doc.Kind)
	}
	update := func(body map[string]any, status int) {
		t.Helper()
		testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+doc.ID, body), "id", doc.ID)).Want(status)
	}
	update(map[string]any{"description": "Unsafe overwrite"}, http.StatusBadRequest)
	update(map[string]any{"title": "Saved document", "description": "Saved body", "expected_revision": doc.Revision}, http.StatusOK)
	update(map[string]any{"description": "Stale overwrite", "expected_revision": doc.Revision}, http.StatusConflict)
	update(map[string]any{"kind": "task"}, http.StatusBadRequest)
	var restored IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest("GET", "/api/issues/"+doc.ID, nil), "id", doc.ID)).Want(http.StatusOK).JSON(&restored)
	if restored.Title != "Saved document" || restored.Description == nil || *restored.Description != "Saved body" {
		t.Fatalf("saved content not restored: %#v", restored)
	}
	var open struct {
		Issues []IssueResponse `json:"issues"`
	}
	testutil.Call(t, testHandler.ListIssues, newRequest("GET", "/api/issues?kind=doc&open_only=true", nil)).Want(http.StatusOK).JSON(&open)
	for _, issue := range open.Issues {
		if issue.Kind != "doc" {
			t.Fatal("open documents included task")
		}
	}
	for _, kind := range []string{"task", "doc"} {
		var page struct {
			Rows []struct {
				Issue IssueResponse `json:"issue"`
			} `json:"rows"`
		}
		testutil.Call(t, testHandler.ListIssueTableRows, newRequest("POST", "/api/issues/table/rows", map[string]any{
			"query": map[string]any{"kind": kind, "scope": map[string]any{"kind": "workspace"}, "filters": map[string]any{}, "search": "Saved document", "sort": map[string]any{"field": "created_at", "direction": "asc"}},
			"group": map[string]any{"kind": "none"}, "hierarchy": map[string]any{"enabled": false},
		})).Want(http.StatusOK).JSON(&page)
		found := false
		for _, row := range page.Rows {
			if row.Issue.ID == doc.ID {
				found = true
				if row.Issue.Description != nil {
					t.Fatal("document list leaked full body")
				}
				if row.Issue.Kind != "doc" {
					t.Fatal("table lost kind")
				}
			}
		}
		if found != (kind == "doc") {
			t.Fatalf("document membership for %s = %v", kind, found)
		}
	}
}

func TestDocumentDisabledReadContract(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.CortexDocs, false)
	id := dbfx.Issue(t, t.Name(), testutil.Cols{"kind": "doc"})
	for _, method := range []string{"GET", "HEAD"} {
		testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest(method, "/api/issues/"+id, nil), "id", id)).Want(http.StatusOK)
	}
	var page struct {
		Issues []IssueResponse `json:"issues"`
	}
	testutil.Call(t, testHandler.ListIssues, newRequest("GET", "/api/issues?kind=doc", nil)).Want(http.StatusOK).JSON(&page)
	found := false
	for _, issue := range page.Issues {
		if issue.ID == id {
			found = true
		}
	}
	if !found {
		t.Fatal("disabled flag must preserve explicit document reads")
	}
	testutil.Call(t, testHandler.CreateIssue, newRequest("POST", "/api/issues", map[string]any{"kind": "doc", "title": "Disabled"})).Want(http.StatusNotFound)
	testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+id, map[string]any{"title": "Disabled", "expected_revision": 1}), "id", id)).Want(http.StatusNotFound)
	testutil.Call(t, testHandler.DeleteIssue, withURLParam(newRequest("DELETE", "/api/issues/"+id, nil), "id", id)).Want(http.StatusNotFound)
	testutil.Call(t, testHandler.ListIssueTableRows, newRequest("POST", "/api/issues/table/rows", map[string]any{
		"query": map[string]any{"kind": "doc", "scope": map[string]any{"kind": "workspace"}, "filters": map[string]any{}, "sort": map[string]any{"field": "created_at", "direction": "asc"}},
		"group": map[string]any{"kind": "none"}, "hierarchy": map[string]any{"enabled": false},
	})).Want(http.StatusNotFound)
}

type documentCountingReader struct {
	io.Reader
	read int
}

func (r *documentCountingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += n
	return n, err
}

func TestDocumentWriteBodyLimits(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.CortexDocs, true)
	id := dbfx.Issue(t, t.Name(), testutil.Cols{"kind": "doc"})
	for _, method := range []string{"POST", "PUT"} {
		for _, field := range []string{"description", "unknown"} {
			t.Run(method+"/"+field, func(t *testing.T) {
				body := `{"kind":"doc","title":"Oversized","` + field + `":"` + strings.Repeat("x", 4*1024*1024) + `"}`
				reader := &documentCountingReader{Reader: strings.NewReader(body)}
				req := withURLParam(newRequest(method, "/api/issues/"+id, nil), "id", id)
				req.Body = io.NopCloser(reader)
				handler := testHandler.CreateIssue
				if method == "PUT" {
					handler = testHandler.UpdateIssue
				}
				testutil.Call(t, handler, req).Want(http.StatusRequestEntityTooLarge)
				if reader.read > maxIssueWriteBytes+1 {
					t.Fatalf("consumed %d bytes before rejecting oversized body", reader.read)
				}
			})
		}
		body := map[string]any{"title": "Oversized content", "description": strings.Repeat("x", 1024*1024+1), "expected_revision": 1}
		handler := testHandler.UpdateIssue
		if method == "POST" {
			body["kind"] = "doc"
			handler = testHandler.CreateIssue
		}
		testutil.Call(t, handler, withURLParam(newRequest(method, "/api/issues/"+id, body), "id", id)).Want(http.StatusRequestEntityTooLarge)
	}
}

func TestTaskDuplicateLockCompatibleWithLegacyWriter(t *testing.T) {
	ctx := context.Background()
	legacy, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Rollback(ctx)
	// This is the exact key assembled by backends before issue.kind existed.
	key := "issue-active-duplicate|" + testWorkspaceID + "|||legacy duplicate title"
	if err := db.New(legacy).LockIssueDuplicateKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"", "task", "doc"} {
		t.Run(kind, func(t *testing.T) {
			tx, err := testPool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err := tx.Exec(ctx, "SET LOCAL lock_timeout = '100ms'"); err != nil {
				t.Fatal(err)
			}
			_, _, err = issueguard.LockAndFindActiveDuplicate(ctx, db.New(tx), parseUUID(testWorkspaceID), pgtype.UUID{}, pgtype.UUID{}, " Legacy   Duplicate Title ", true, kind)
			if kind == "doc" {
				if err != nil {
					t.Fatalf("document lock should be independent: %v", err)
				}
				return
			}
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
				t.Fatalf("task must block on legacy writer's lock, got %v", err)
			}
		})
	}
}

func TestDocumentConcurrentSaveDoesNotSchedule(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.CortexDocs, true)
	agent := dbfx.Agent(t, t.Name(), "")
	id := dbfx.Issue(t, t.Name(), testutil.Cols{"kind": "doc", "assignee_type": "agent", "assignee_id": agent})
	var original IssueResponse
	testutil.Call(t, testHandler.GetIssue, withURLParam(newRequest("GET", "/api/issues/"+id, nil), "id", id)).Want(http.StatusOK).JSON(&original)
	start := make(chan struct{})
	results := make(chan int, 2)
	for _, title := range []string{"Concurrent A", "Concurrent B"} {
		go func(title string) {
			<-start
			results <- testutil.Call(t, testHandler.UpdateIssue, withURLParam(newRequest("PUT", "/api/issues/"+id, map[string]any{
				"title": title, "description": "Saved concurrently", "expected_revision": original.Revision,
			}), "id", id)).Code
		}(title)
	}
	close(start)
	counts := map[int]int{}
	counts[<-results]++
	counts[<-results]++
	if counts[http.StatusOK] != 1 || counts[http.StatusConflict] != 1 {
		t.Fatalf("concurrent save statuses: %v", counts)
	}
	if count := dbfx.Count(t, "SELECT count(*) FROM agent_task_queue WHERE issue_id = $1", id); count != 0 {
		t.Fatalf("document save enqueued %d tasks", count)
	}
}

func TestDocumentCrossWorkspaceDenied(t *testing.T) {
	withFeatureFlag(t, testHandler, featureflags.CortexDocs, true)
	ws := dbfx.Workspace(t, t.Name(), "doc-cross-workspace")
	id := dbfx.Issue(t, t.Name(), testutil.Cols{"workspace_id": ws, "kind": "doc"})
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		handler := testHandler.GetIssue
		if method == "PUT" {
			handler = testHandler.UpdateIssue
		}
		if method == "DELETE" {
			handler = testHandler.DeleteIssue
		}
		testutil.Call(t, handler, withURLParam(newRequest(method, "/api/issues/"+id, map[string]any{"title": "Forbidden", "expected_revision": 1}), "id", id)).Want(http.StatusNotFound)
	}
}
