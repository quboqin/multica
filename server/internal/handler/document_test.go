package handler

import (
	"net/http"
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
