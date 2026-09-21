package main

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func testRow() recordDTO {
	return recordDTO{
		ID: testRowID, CollectionID: testRequestsID, Title: "Embed live views", Revision: 3,
		Fields: map[string]any{
			testStageID: testTriageID,
			testSeatsID: float64(25),
			testTagsID:  []any{testTagIOSID, testTagWebID},
			testOwnerID: "member:" + testMemberAdaID,
			// A value left behind by a field that has since been archived.
			"33333333-dead-4000-8000-000000000000": "old",
		},
		Links: map[string][]recordLinkDTO{
			testTasksID: {
				{ID: "link-1", ToType: "issue", ToID: testIssueID, Identifier: "MUL-31", Title: "Data model", Status: "in_progress"},
				{ID: "link-2", ToType: "issue", ToID: "gone", Missing: true},
			},
			testCustomerID: {{ID: "link-3", ToType: "record", ToID: testACMERowID, Title: "ACME", CollectionID: testCustomersID}},
		},
	}
}

// By default a row prints the way the write commands take it back: by field
// name, with option and member names instead of ids.
func TestBuildRecordOutputSpellsCellsByName(t *testing.T) {
	names := map[string]string{"member:" + testMemberAdaID: "Ada"}
	out := buildRecordOutput(testRequestsDetail().Fields, testRow(), names, false)
	if out.Fields != nil {
		t.Errorf("the default output must not carry detail rows")
	}
	want := map[string]any{
		"Stage": "Triage",
		"Seats": float64(25),
		"Tags":  []string{"iOS", "Web"},
		"Owner": "Ada",
		"Implementation": []map[string]any{
			{"issue": "MUL-31", "title": "Data model", "status": "in_progress"},
			// Nothing but the link's own id can still name a dead link.
			{"deleted": true, "link_id": "link-2"},
		},
		"Customer": []map[string]any{{"row": testACMERowID, "title": "ACME"}},
	}
	if !reflect.DeepEqual(*out.Values, want) {
		t.Errorf("values = %#v\nwant     %#v", *out.Values, want)
	}
}

func TestBuildRecordOutputDetailKeepsIDsAndStoredValues(t *testing.T) {
	out := buildRecordOutput(testRequestsDetail().Fields, testRow(), nil, true)
	if out.Values != nil {
		t.Errorf("--detail must not also print values")
	}
	rows := *out.Fields
	if len(rows) != 6 {
		t.Fatalf("rows = %d, want the six live fields (the archived field's value is left out): %+v", len(rows), rows)
	}
	if rows[0].FieldID != testStageID || rows[0].Value != testTriageID || rows[0].Display != "Triage" {
		t.Errorf("select row = %+v", rows[0])
	}
	// With no member lookup the reference stays raw rather than going blank.
	if rows[3].Display != "member:"+testMemberAdaID {
		t.Errorf("actor row = %+v", rows[3])
	}
	if got := rows[4].DisplayValues; !reflect.DeepEqual(got, []string{"MUL-31 Data model", "(deleted)"}) {
		t.Errorf("relation labels = %v", got)
	}
}

// A row with no cells still says so, instead of dropping the key a parser
// would look for.
func TestRecordOutputAlwaysCarriesItsCells(t *testing.T) {
	for _, detail := range []bool{false, true} {
		buf, err := json.Marshal(buildRecordOutput(testRequestsDetail().Fields, recordDTO{ID: testRowID}, nil, detail))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		key := `"values":{}`
		if detail {
			key = `"fields":[]`
		}
		if !strings.Contains(string(buf), key) {
			t.Errorf("detail=%v: %s should contain %s", detail, buf, key)
		}
	}
}

func recordsPage(records ...recordDTO) map[string]any {
	return map[string]any{"records": records, "total": len(records), "next_cursor": nil}
}

func TestRunRecordListTranslatesFiltersAndSort(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == recordsPath(testRequestsID) {
			return 200, recordsPage(testRow())
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, recordListCmd,
		"filter", "stage=Triage", "filter", "Stage=shipped", "filter", "Seats=__none__", "filter", "Owner=Ada",
		"sort", "seats", "desc", "true", "limit", "20", "cursor", "abc", "search", "embed", "output", "json")
	out, err := captureStdout(t, func() error { return runRecordList(cmd, []string{"Requests"}) })
	if err != nil {
		t.Fatalf("runRecordList: %v", err)
	}
	var query url.Values
	for _, req := range srv.requests {
		if req.Path == recordsPath(testRequestsID) {
			query = req.Query
		}
	}
	var filter map[string][]string
	if err := json.Unmarshal([]byte(query.Get("properties")), &filter); err != nil {
		t.Fatalf("properties = %q: %v", query.Get("properties"), err)
	}
	wantFilter := map[string][]string{
		// The same field twice ORs its values, keyed once by id however it was spelled.
		testStageID: {testTriageID, testShippedID},
		testSeatsID: {propertyNoValueSentinel},
		testOwnerID: {"member:" + testMemberAdaID},
	}
	if !reflect.DeepEqual(filter, wantFilter) {
		t.Errorf("filter = %v\nwant     %v", filter, wantFilter)
	}
	for key, want := range map[string]string{"sort_by": testSeatsID, "sort_dir": "desc", "limit": "20", "cursor": "abc", "search": "embed"} {
		if got := query.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	var page recordListOutput
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, out)
	}
	if len(page.Records) != 1 || (*page.Records[0].Values)["Owner"] != "Ada" {
		t.Errorf("output = %s", out)
	}
}

func TestRunRecordListRefusesWhatTheServerWouldQuietlyIgnore(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"invalid relation comparison", []string{"filter", "Customer>=ACME"}, "equality only"},

		{"invalid comparison", []string{"filter", "Stage>Shipped"}, "comparison requires"},
		{"an empty value", []string{"filter", "Stage="}, propertyNoValueSentinel},
		{"an unknown option", []string{"filter", "Stage=Doing"}, "valid options: Triage, Shipped"},
		{"desc without a sort", []string{"desc", "true"}, "--desc needs --sort"},
		{"a limit past the page size", []string{"limit", "500"}, "--limit must be 1 to 100"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newCortexTestServer(t, requestsCatalog)
			err := runRecordList(testCmdLike(t, recordListCmd, tt.flags...), []string{"Requests"})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			for _, req := range srv.requests {
				if req.Path == recordsPath(testRequestsID) {
					t.Errorf("the rows were requested anyway")
				}
			}
		})
	}
}

func TestMatchRecordTitle(t *testing.T) {
	rows := []recordDTO{{ID: "a", Title: "ACME"}, {ID: "b", Title: "ACME Holdings"}, {ID: "c", Title: " acme "}}
	if _, err := matchRecordTitle(rows, "Customers", "ACME", false); err == nil || !strings.Contains(err.Error(), "2 rows") {
		t.Errorf("two rows sharing a title must be listed, not guessed between: %v", err)
	}
	if id, err := matchRecordTitle(rows, "Customers", "acme holdings", false); err != nil || id != "b" {
		t.Errorf("exact title = %q, %v", id, err)
	}
	if _, err := matchRecordTitle(rows, "Customers", "Globex", false); err == nil || !strings.Contains(err.Error(), "no row titled") {
		t.Errorf("miss = %v", err)
	}
	// One exact match on a page that did not hold every candidate proves nothing.
	if _, err := matchRecordTitle(rows[1:2], "Customers", "ACME Holdings", true); err == nil {
		t.Errorf("a truncated search must not be trusted for a unique match")
	}
}

// recordWriteServer answers the reads plus every row write with the row.
func recordWriteServer(t *testing.T, failOn string) *cortexTestServer {
	return newCortexTestServer(t, func(req cortexRequest) (int, any) {
		base := recordsPath(testRequestsID)
		switch {
		case req.Method == http.MethodGet && req.Path == base:
			return 200, recordsPage(testRow())
		case req.Method == http.MethodGet && req.Path == recordsPath(testCustomersID):
			return 200, recordsPage(recordDTO{ID: testACMERowID, CollectionID: testCustomersID, Title: "ACME"})
		case req.Method == http.MethodGet && req.Path == "/api/issues/MUL-31":
			return 200, map[string]any{"id": testIssueID, "identifier": "MUL-31"}
		case req.Method == http.MethodGet:
			return requestsCatalog(req)
		case failOn != "" && strings.HasSuffix(req.Path, failOn):
			return 409, map[string]string{"error": "field changed; reload and retry"}
		case strings.HasPrefix(req.Path, base):
			return 200, testRow()
		}
		return 0, nil
	})
}

func TestRunRecordUpdateWritesEachCellInOrder(t *testing.T) {
	srv := recordWriteServer(t, "")
	cmd := testCmdLike(t, recordUpdateCmd,
		"title", "Embed live views in documents",
		"set", "Stage=Shipped", "set", "Tags=web, ios", "unset", "seats",
		"expect", "Stage=Triage", "expect", "Seats=")
	if _, err := captureStdout(t, func() error { return runRecordUpdate(cmd, []string{"Requests", "embed live views"}) }); err != nil {
		t.Fatalf("runRecordUpdate: %v", err)
	}
	row := recordsPath(testRequestsID) + "/" + testRowID
	want := []cortexRequest{
		// The rename names the title it replaces, so a concurrent rename is refused.
		{Method: "PUT", Path: row, Body: map[string]any{"title": "Embed live views in documents", "title_base": "Embed live views"}},
		{Method: "PUT", Path: row + "/fields/" + testStageID, Body: map[string]any{"value": testShippedID, "expected_value": testTriageID}},
		{Method: "PUT", Path: row + "/fields/" + testTagsID, Body: map[string]any{"value": []any{testTagWebID, testTagIOSID}}},
		// "Seats=" expects the cell to be empty still; clearing sends a null value.
		{Method: "PUT", Path: row + "/fields/" + testSeatsID, Body: map[string]any{"value": nil, "expected_value": nil}},
	}
	got := srv.writes()
	if len(got) != len(want) {
		t.Fatalf("writes = %+v", got)
	}
	for i := range want {
		if got[i].Method != want[i].Method || got[i].Path != want[i].Path || !reflect.DeepEqual(got[i].Body, want[i].Body) {
			t.Errorf("write %d = %s %s %v\nwant      %s %s %v", i, got[i].Method, got[i].Path, got[i].Body, want[i].Method, want[i].Path, want[i].Body)
		}
	}
}

// Every flag is resolved before the first write: a typo in the last one must
// not leave the first ones applied.
func TestRunRecordUpdateResolvesEverythingBeforeWriting(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags []string
		want  string
	}{
		{"an unknown option in the last flag", []string{"set", "Seats=3", "set", "Stage=Doing"}, "valid options"},
		{"a relation written as a value", []string{"set", "Seats=3", "set", "Customer=ACME"}, "multica record link"},
		{"a relation cleared as a value", []string{"set", "Seats=3", "unset", "Implementation"}, "multica record unlink"},
		{"a guard with nothing to guard", []string{"set", "Seats=3", "expect", "Stage=Triage"}, "no --set or --unset targets that field"},
		{"an empty value", []string{"set", "Stage="}, "--unset"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := recordWriteServer(t, "")
			err := runRecordUpdate(testCmdLike(t, recordUpdateCmd, tt.flags...), []string{"Requests", testRowID})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
			if writes := srv.writes(); len(writes) != 0 {
				t.Errorf("writes before the refusal: %+v", writes)
			}
		})
	}
}

// Cells are written one at a time, so a failure part-way leaves real state
// behind. The caller has to be told which writes landed.
func TestRunRecordUpdateSaysWhatLandedBeforeAFailure(t *testing.T) {
	srv := recordWriteServer(t, "/fields/"+testStageID)
	stderr := captureStderr(t)
	defer stderr.restore()
	cmd := testCmdLike(t, recordUpdateCmd, "set", "Seats=3", "set", "Stage=Shipped", "set", "Tags=iOS", "expect", "Stage=Triage")
	err := runRecordUpdate(cmd, []string{"Requests", testRowID})
	if err == nil || !strings.Contains(err.Error(), "write Stage") {
		t.Fatalf("error = %v", err)
	}
	if got := stderr.read(); !strings.Contains(got, "Already written before this failure: Seats") {
		t.Errorf("stderr = %q", got)
	}
	if writes := srv.writes(); len(writes) != 2 {
		t.Errorf("writes after the failure: %+v", writes)
	}
}

func TestRunRecordCreateSendsCellsInOneRequest(t *testing.T) {
	srv := recordWriteServer(t, "")
	cmd := testCmdLike(t, recordCreateCmd, "title", "Calendar by week", "set", "Stage=Triage", "set", "Seats=12.5", "set", "Owner=Ada")
	if _, err := captureStdout(t, func() error { return runRecordCreate(cmd, []string{testRequestsID}) }); err != nil {
		t.Fatalf("runRecordCreate: %v", err)
	}
	writes := srv.writes()
	want := map[string]any{"title": "Calendar by week", "fields": map[string]any{
		testStageID: testTriageID, testSeatsID: 12.5, testOwnerID: "member:" + testMemberAdaID,
	}}
	if len(writes) != 1 || writes[0].Method != "POST" || writes[0].Path != recordsPath(testRequestsID) || !reflect.DeepEqual(writes[0].Body, want) {
		t.Errorf("writes = %+v\nwant body %v", writes, want)
	}
	if err := runRecordCreate(testCmdLike(t, recordCreateCmd), []string{"Requests"}); err == nil {
		t.Errorf("an empty row should be refused")
	}
}

// What --to means follows the field: an issue key for a relation to issues, a
// row of the target table otherwise.
func TestRunRecordLinkResolvesTargetsByWhatTheFieldPointsAt(t *testing.T) {
	links := recordsPath(testRequestsID) + "/" + testRowID + "/links"
	for _, tt := range []struct{ field, to, fieldID, toID string }{
		{"Implementation", "MUL-31", testTasksID, testIssueID},
		{"customer", "acme", testCustomerID, testACMERowID},
	} {
		srv := recordWriteServer(t, "")
		cmd := testCmdLike(t, recordLinkCmd, "field", tt.field, "to", tt.to)
		if _, err := captureStdout(t, func() error { return runRecordLink(cmd, []string{"Requests", testRowID}) }); err != nil {
			t.Fatalf("link %s: %v", tt.field, err)
		}
		want := map[string]any{"field_id": tt.fieldID, "to_id": tt.toID}
		if writes := srv.writes(); len(writes) != 1 || writes[0].Path != links || !reflect.DeepEqual(writes[0].Body, want) {
			t.Errorf("link %s: writes = %+v, want POST %s %v", tt.field, writes, links, want)
		}
	}

	srv := recordWriteServer(t, "")
	err := runRecordLink(testCmdLike(t, recordLinkCmd, "field", "Stage", "to", "MUL-31"), []string{"Requests", testRowID})
	if err == nil || !strings.Contains(err.Error(), "not a relation") || len(srv.writes()) != 0 {
		t.Errorf("a value field must be refused without a write: %v, %+v", err, srv.writes())
	}
}

func TestRunRecordUnlinkFindsTheLinkByTargetOrByItsOwnID(t *testing.T) {
	base := recordsPath(testRequestsID) + "/" + testRowID + "/links/"
	for _, tt := range []struct{ name, to, want string }{
		{"by issue key", "MUL-31", "link-1"},
		{"by target id", testIssueID, "link-1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := recordWriteServer(t, "")
			cmd := testCmdLike(t, recordUnlinkCmd, "field", "Implementation", "to", tt.to)
			if _, err := captureStdout(t, func() error { return runRecordUnlink(cmd, []string{"Requests", testRowID}) }); err != nil {
				t.Fatalf("runRecordUnlink: %v", err)
			}
			if writes := srv.writes(); len(writes) != 1 || writes[0].Method != "DELETE" || writes[0].Path != base+tt.want {
				t.Errorf("writes = %+v, want DELETE %s", writes, base+tt.want)
			}
		})
	}

	srv := recordWriteServer(t, "")
	err := runRecordUnlink(testCmdLike(t, recordUnlinkCmd, "field", "Customer", "to", testIssueID), []string{"Requests", testRowID})
	if err == nil || !strings.Contains(err.Error(), "is not linked") || len(srv.writes()) != 0 {
		t.Errorf("an unlinked target must be refused without a write: %v, %+v", err, srv.writes())
	}
}

// A link whose target is gone can no longer be named by key or title; its own
// id still works, which is the only way to clear it.
func TestMatchRecordLinkByLinkID(t *testing.T) {
	field := testRequestsDetail().Fields[4]
	links := []recordLinkDTO{{ID: "aaaaaaaa-0000-4000-8000-000000000001", ToID: "bbbbbbbb-0000-4000-8000-000000000002", Missing: true}}
	for _, ref := range []string{"aaaaaaaa-0000-4000-8000-000000000001", "BBBBBBBB-0000-4000-8000-000000000002"} {
		link, err := matchRecordLink(t.Context(), nil, field, links, ref)
		if err != nil || link.ID != links[0].ID {
			t.Errorf("matchRecordLink(%q) = %+v, %v", ref, link, err)
		}
	}
}

func TestRunIssueRecordsListsTheRowsThatPointAtAnIssue(t *testing.T) {
	newCortexTestServer(t, func(req cortexRequest) (int, any) {
		switch req.Path {
		case "/api/issues/MUL-31":
			return 200, map[string]any{"id": testIssueID, "identifier": "MUL-31"}
		case "/api/issues/" + testIssueID + "/record-links":
			return 200, map[string]any{"links": []recordBacklinkDTO{{CollectionName: "Requests", RecordID: testRowID, RecordTitle: "Embed live views", FieldName: "Implementation"}}}
		}
		return 0, nil
	})
	out, err := captureStdout(t, func() error { return runIssueRecords(testCmdLike(t, issueRecordsCmd), []string{"MUL-31"}) })
	if err != nil {
		t.Fatalf("runIssueRecords: %v", err)
	}
	for _, want := range []string{"Requests", "Embed live views", "Implementation", testRowID} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q should contain %q", out, want)
		}
	}
}

func TestRunRecordRestoreLooksInTheTrash(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		switch {
		case req.Path == "/api/collections/"+testRequestsID+"/trash":
			return 200, map[string]any{"records": []recordDTO{{ID: testRowID, Title: "Embed live views", DeletedAt: "2026-09-21T10:00:00Z"}}, "total": 1, "retention_days": 30}
		case req.Method == http.MethodPost && strings.HasSuffix(req.Path, "/restore"):
			return 200, testRow()
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, recordRestoreCmd, "output", "table")
	out, err := captureStdout(t, func() error { return makeRecordTrashRun(false)(cmd, []string{"Requests", "embed live views"}) })
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	want := recordsPath(testRequestsID) + "/" + testRowID + "/restore"
	if writes := srv.writes(); len(writes) != 1 || writes[0].Path != want {
		t.Errorf("writes = %+v, want POST %s", writes, want)
	}
	if !strings.Contains(out, "restored") {
		t.Errorf("output = %q", out)
	}
}
