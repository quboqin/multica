package main

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/cli"
)

const (
	testDocID      = "77777777-0001-4000-8000-000000000001"
	testChildDocID = "77777777-0002-4000-8000-000000000002"
	testProjectID  = "88888888-0001-4000-8000-000000000001"
)

func testDocument(id, key string, revision int, over map[string]any) map[string]any {
	doc := map[string]any{
		"id": id, "identifier": key, "kind": "doc", "title": "Design notes", "status": "draft",
		"description": "# Goal", "document_revision": revision, "parent_issue_id": nil, "project_id": nil,
	}
	for k, v := range over {
		doc[k] = v
	}
	return doc
}

// documentServer serves MUL-7 (a document at the given revision), MUL-8 (its
// child), MUL-9 (a task) and answers writes through `write`.
func documentServer(t *testing.T, revision int, write func(req cortexRequest) (int, any)) *cortexTestServer {
	return newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method != http.MethodGet {
			return write(req)
		}
		switch req.Path {
		case "/api/issues/MUL-7", "/api/issues/" + testDocID:
			return 200, testDocument(testDocID, "MUL-7", revision, map[string]any{"project_id": testProjectID})
		case "/api/issues/MUL-8", "/api/issues/" + testChildDocID:
			return 200, testDocument(testChildDocID, "MUL-8", 1, map[string]any{"parent_issue_id": testDocID, "project_id": testProjectID})
		case "/api/issues/MUL-9":
			return 200, map[string]any{"id": testIssueID, "identifier": "MUL-9", "kind": "task", "title": "A task"}
		}
		return 0, nil
	})
}

func TestOrderDocumentTree(t *testing.T) {
	doc := func(id, parent string) map[string]any {
		out := map[string]any{"id": id}
		if parent != "" {
			out["parent_issue_id"] = parent
		}
		return out
	}
	// As the API returns them: ordered by position, parents and children mixed.
	ordered := orderDocumentTree([]map[string]any{
		doc("a", ""), doc("a2", "a"), doc("b", ""), doc("a1", "a"), doc("a2x", "a2"),
		// Its parent is outside this listing (another project's tree).
		doc("orphan", "elsewhere"),
	})
	var got []string
	for _, d := range ordered {
		got = append(got, strings.Repeat(">", d["depth"].(int))+d["id"].(string))
	}
	want := []string{"a", ">a2", ">>a2x", ">a1", "b", "orphan"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree = %v, want %v", got, want)
	}
}

// Listing the tree is for finding a page. Bodies stay out so one call is cheap
// however long the pages are.
func TestRunDocumentListLeavesBodiesOut(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == "/api/documents" {
			return 200, []map[string]any{
				testDocument(testDocID, "MUL-7", 3, nil),
				testDocument(testChildDocID, "MUL-8", 1, map[string]any{"parent_issue_id": testDocID}),
			}
		}
		if req.Path == "/api/projects" {
			return 200, map[string]any{"projects": []map[string]any{{"id": testProjectID, "title": "Cortex"}}}
		}
		return 0, nil
	})
	cmd := testCmdLike(t, documentListCmd, "output", "json", "project", testProjectID)
	out, err := captureStdout(t, func() error { return runDocumentList(cmd, nil) })
	if err != nil {
		t.Fatalf("runDocumentList: %v", err)
	}
	if got := srv.requests[len(srv.requests)-1].Query.Get("project_id"); got != testProjectID {
		t.Errorf("project_id = %q", got)
	}
	var docs []map[string]any
	if err := json.Unmarshal([]byte(out), &docs); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(docs) != 2 || docs[1]["depth"] != float64(1) {
		t.Errorf("docs = %v", docs)
	}
	for _, doc := range docs {
		if _, present := doc["description"]; present {
			t.Errorf("%v carries its body", doc["identifier"])
		}
	}
}

func TestFetchDocumentRefusesAnythingThatIsNotOne(t *testing.T) {
	documentServer(t, 1, nil)
	cmd := testCmdLike(t, documentGetCmd)
	err := runDocumentGet(cmd, []string{"MUL-9"})
	if err == nil || !strings.Contains(err.Error(), "MUL-9 is a task, not a document") {
		t.Errorf("error = %v", err)
	}
}

func TestRunDocumentGetMarkdownPrintsTheBodyAlone(t *testing.T) {
	documentServer(t, 4, nil)
	stderr := captureStderr(t)
	defer stderr.restore()
	out, err := captureStdout(t, func() error {
		return runDocumentGet(testCmdLike(t, documentGetCmd, "output", "markdown"), []string{"MUL-7"})
	})
	if err != nil {
		t.Fatalf("runDocumentGet: %v", err)
	}
	if out != "# Goal\n" {
		t.Errorf("stdout = %q, want only the body", out)
	}
	// The revision a save needs goes where a redirect does not capture it.
	if got := stderr.read(); !strings.Contains(got, "revision 4") {
		t.Errorf("stderr = %q", got)
	}
}

// A save names the revision it was written against. The CLI never fills one in:
// doing so would turn every save into "overwrite whatever is there".
func TestRunDocumentSaveNeedsTheRevisionYouRead(t *testing.T) {
	srv := documentServer(t, 3, nil)
	err := runDocumentSave(testCmdLike(t, documentSaveCmd, "content", "new body"), []string{"MUL-7"})
	if err == nil || !strings.Contains(err.Error(), "--expected-revision is required") {
		t.Fatalf("error = %v", err)
	}
	if len(srv.requests) != 0 {
		t.Errorf("a save without a revision made %d requests", len(srv.requests))
	}
}

func TestRunDocumentSaveSendsBodyTitleAndRevision(t *testing.T) {
	srv := documentServer(t, 3, func(req cortexRequest) (int, any) {
		return 200, testDocument(testDocID, "MUL-7", 4, nil)
	})
	cmd := testCmdLike(t, documentSaveCmd, "expected-revision", "3", "content", "line one\\nline two", "title", "Design notes v2", "output", "table")
	out, err := captureStdout(t, func() error { return runDocumentSave(cmd, []string{"MUL-7"}) })
	if err != nil {
		t.Fatalf("runDocumentSave: %v", err)
	}
	want := map[string]any{"expected_document_revision": float64(3), "description": "line one\nline two", "title": "Design notes v2"}
	writes := srv.writes()
	if len(writes) != 1 || writes[0].Method != "PUT" || writes[0].Path != "/api/issues/"+testDocID || !reflect.DeepEqual(writes[0].Body, want) {
		t.Errorf("writes = %+v\nwant body %v", writes, want)
	}
	if !strings.Contains(out, "MUL-7") || !strings.Contains(out, "4") {
		t.Errorf("output = %q, want the new revision", out)
	}
}

// The conflict response carries the whole current body and is routinely cut off
// by the client's error-body cap, so the current revision is read back rather
// than parsed out of it. The message must send the caller to merge, not to
// retry with the new number.
func TestRunDocumentSaveConflictNamesTheRevisionToMergeAgainst(t *testing.T) {
	documentServer(t, 6, func(req cortexRequest) (int, any) {
		return 409, map[string]any{"code": "document_conflict", "description": strings.Repeat("long body ", 1000), "document_revision": 6, "error": "Document changed."}
	})
	err := runDocumentSave(testCmdLike(t, documentSaveCmd, "expected-revision", "4", "content", "mine"), []string{"MUL-7"})
	if err == nil {
		t.Fatal("a stale save must fail")
	}
	got := cli.FormatError(err, false)
	for _, want := range []string{"nothing was written", "revision 6", "named revision 4", "multica document get MUL-7", "merge", "--expected-revision 6"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q should contain %q", got, want)
		}
	}
}

// A 409 that is not about the body revision keeps the server's own words.
func TestRunDocumentSaveLeavesOtherConflictsAlone(t *testing.T) {
	documentServer(t, 4, func(req cortexRequest) (int, any) {
		return 409, map[string]string{"error": "title changed; reload and retry"}
	})
	err := runDocumentSave(testCmdLike(t, documentSaveCmd, "expected-revision", "4", "title", "Renamed"), []string{"MUL-7"})
	if got := cli.FormatError(err, false); !strings.Contains(got, "title changed") || strings.Contains(got, "nothing was written") {
		t.Errorf("message = %q", got)
	}
}

func TestRunDocumentStatusMapsTheStatusOntoItsTransition(t *testing.T) {
	for status, action := range map[string]string{"reviewing": "review", "Published": "publish", "draft": "draft"} {
		srv := documentServer(t, 2, func(req cortexRequest) (int, any) {
			return 200, testDocument(testDocID, "MUL-7", 2, map[string]any{"status": strings.ToLower(status)})
		})
		cmd := testCmdLike(t, documentStatusCmd, "expected-revision", "2")
		if _, err := captureStdout(t, func() error { return runDocumentStatus(cmd, []string{"MUL-7", status}) }); err != nil {
			t.Fatalf("status %s: %v", status, err)
		}
		want := map[string]any{"action": action, "expected_document_revision": float64(2)}
		writes := srv.writes()
		if len(writes) != 1 || writes[0].Path != "/api/documents/"+testDocID+"/transition" || !reflect.DeepEqual(writes[0].Body, want) {
			t.Errorf("status %s: writes = %+v, want %v", status, writes, want)
		}
	}

	srv := documentServer(t, 2, nil)
	err := runDocumentStatus(testCmdLike(t, documentStatusCmd, "expected-revision", "2"), []string{"MUL-7", "done"})
	if err == nil || !strings.Contains(err.Error(), "draft, reviewing, or published") || len(srv.requests) != 0 {
		t.Errorf("an unknown status must be refused before any request: %v", err)
	}
}

// Approval is recorded against a person, so a run's task token is refused. The
// generic 403 copy would send an agent off to ask for access that cannot be
// granted; this one names what it can do instead.
func TestRunDocumentStatusTellsAnAgentToAskAPerson(t *testing.T) {
	documentServer(t, 2, func(req cortexRequest) (int, any) {
		return 403, map[string]string{"error": "document approval requires a human actor", "code": "document_transition_requires_human"}
	})
	err := runDocumentStatus(testCmdLike(t, documentStatusCmd, "expected-revision", "2"), []string{"MUL-7", "reviewing"})
	got := cli.FormatError(err, false)
	for _, want := range []string{"signed by a person", "leave a comment"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q should contain %q", got, want)
		}
	}
	if cli.ExitCodeFor(err) != cli.ExitAuth {
		t.Errorf("the refusal must still exit as an authorization failure")
	}
}

func TestRunDocumentMoveBuildsTheMove(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags []string
		want  map[string]any
	}{
		{
			name:  "to the top level, last",
			flags: []string{"root", "true"},
			want:  map[string]any{"parent_issue_id": nil, "position": float64(documentMoveToEnd)},
		},
		{
			name:  "under another page",
			flags: []string{"parent", "MUL-7"},
			want:  map[string]any{"parent_issue_id": testDocID, "position": float64(documentMoveToEnd)},
		},
		{
			// Reordering only: the endpoint always writes the parent, so the
			// current one goes back unchanged. New pages sit at negative
			// positions, so "first" has to be far below zero.
			name:  "first among its current siblings",
			flags: []string{"first", "true"},
			want:  map[string]any{"parent_issue_id": testDocID, "position": float64(documentMoveToStart)},
		},
		{
			name:  "before a sibling",
			flags: []string{"before", "MUL-7", "root", "true"},
			want:  map[string]any{"parent_issue_id": nil, "position": float64(documentMoveToEnd), "before_id": testDocID},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := documentServer(t, 1, func(req cortexRequest) (int, any) {
				return 200, testDocument(testChildDocID, "MUL-8", 1, nil)
			})
			cmd := testCmdLike(t, documentMoveCmd, tt.flags...)
			if _, err := captureStdout(t, func() error { return runDocumentMove(cmd, []string{"MUL-8"}) }); err != nil {
				t.Fatalf("runDocumentMove: %v", err)
			}
			writes := srv.writes()
			if len(writes) != 1 || writes[0].Path != "/api/documents/"+testChildDocID+"/move" || !reflect.DeepEqual(writes[0].Body, tt.want) {
				t.Errorf("writes = %+v\nwant body %v", writes, tt.want)
			}
		})
	}

	srv := documentServer(t, 1, nil)
	for _, flags := range [][]string{{}, {"parent", "MUL-7", "root", "true"}, {"before", "MUL-7", "first", "true"}} {
		if err := runDocumentMove(testCmdLike(t, documentMoveCmd, flags...), []string{"MUL-8"}); err == nil {
			t.Errorf("flags %v should be refused", flags)
		}
	}
	if len(srv.requests) != 0 {
		t.Errorf("refused moves made %d requests", len(srv.requests))
	}
}

// A page lives in its parent's project, and the server refuses anything else.
func TestRunDocumentCreateFollowsItsParentsProject(t *testing.T) {
	srv := documentServer(t, 1, func(req cortexRequest) (int, any) {
		return 201, testDocument(testChildDocID, "MUL-10", 1, nil)
	})
	cmd := testCmdLike(t, documentCreateCmd, "title", "Open questions", "content", "- Who owns it?", "parent", "MUL-7")
	if _, err := captureStdout(t, func() error { return runDocumentCreate(cmd, nil) }); err != nil {
		t.Fatalf("runDocumentCreate: %v", err)
	}
	want := map[string]any{
		"kind": "doc", "title": "Open questions", "description": "- Who owns it?",
		"parent_issue_id": testDocID, "project_id": testProjectID,
	}
	writes := srv.writes()
	if len(writes) != 1 || writes[0].Path != "/api/issues" || !reflect.DeepEqual(writes[0].Body, want) {
		t.Errorf("writes = %+v\nwant body %v", writes, want)
	}

	err := runDocumentCreate(testCmdLike(t, documentCreateCmd, "title", "Under a task", "parent", "MUL-9"), nil)
	if err == nil || !strings.Contains(err.Error(), "not a document") {
		t.Errorf("a task cannot be a page's parent: %v", err)
	}
}

func TestRunIssueSearchPassesTheKind(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == "/api/issues/search" {
			return 200, map[string]any{"issues": []any{}, "total": 0}
		}
		return 0, nil
	})
	cmd := testCmdLike(t, issueSearchCmd, "kind", "doc")
	if _, err := captureStdout(t, func() error { return runIssueSearch(cmd, []string{"live table"}) }); err != nil {
		t.Fatalf("runIssueSearch: %v", err)
	}
	if got := srv.requests[0].Query.Get("kind"); got != "doc" {
		t.Errorf("kind = %q", got)
	}
}
