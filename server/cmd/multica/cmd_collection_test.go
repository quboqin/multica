package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/multica-ai/multica/server/internal/cli"
)

// Fixture ids shared by the collection, record and document command tests.
const (
	testRequestsID  = "11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testCustomersID = "22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testStageID     = "33333333-0001-4000-8000-000000000001"
	testSeatsID     = "33333333-0002-4000-8000-000000000002"
	testTagsID      = "33333333-0003-4000-8000-000000000003"
	testOwnerID     = "33333333-0004-4000-8000-000000000004"
	testTasksID     = "33333333-0005-4000-8000-000000000005"
	testCustomerID  = "33333333-0006-4000-8000-000000000006"
	testTriageID    = "44444444-0001-4000-8000-000000000001"
	testShippedID   = "44444444-0002-4000-8000-000000000002"
	testTagIOSID    = "44444444-0003-4000-8000-000000000003"
	testTagWebID    = "44444444-0004-4000-8000-000000000004"
	testRowID       = "55555555-0001-4000-8000-000000000001"
	testACMERowID   = "55555555-0002-4000-8000-000000000002"
	testIssueID     = "66666666-0001-4000-8000-000000000001"
)

func testRequestsDetail() collectionDetailDTO {
	field := func(id, name, fieldType string) collectionFieldDTO {
		return collectionFieldDTO{ID: id, Name: name, Type: fieldType}
	}
	stage := field(testStageID, "Stage", "select")
	stage.Config.Options = []propertyOptionDTO{{ID: testTriageID, Name: "Triage", Color: "#2563eb"}, {ID: testShippedID, Name: "Shipped"}}
	tags := field(testTagsID, "Tags", "multi_select")
	tags.Config.Options = []propertyOptionDTO{{ID: testTagIOSID, Name: "iOS"}, {ID: testTagWebID, Name: "Web"}}
	tasks := field(testTasksID, "Implementation", "relation")
	tasks.Config.Relation = &collectionRelationDTO{ToType: "issue"}
	customer := field(testCustomerID, "Customer", "relation")
	customer.Config.Relation = &collectionRelationDTO{ToType: "record", CollectionID: testCustomersID}
	return collectionDetailDTO{
		Collection: collectionDTO{ID: testRequestsID, Name: "Requests"},
		Fields:     []collectionFieldDTO{stage, field(testSeatsID, "Seats", "number"), tags, field(testOwnerID, "Owner", "actor"), tasks, customer},
	}
}

// cortexRequest is one request a command made, as the fake server saw it.
type cortexRequest struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// cortexTestServer records every request and answers from `respond`, which
// returns a status and a JSON-encodable body. Anything it does not recognise
// (status 0) is a 404, so a command that wanders off its contract fails loudly.
type cortexTestServer struct {
	t        *testing.T
	requests []cortexRequest
	respond  func(req cortexRequest) (int, any)
}

func newCortexTestServer(t *testing.T, respond func(req cortexRequest) (int, any)) *cortexTestServer {
	t.Helper()
	s := &cortexTestServer{t: t, respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req := cortexRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &req.Body); err != nil {
				t.Errorf("%s %s sent a body that is not a JSON object: %s", r.Method, r.URL.Path, raw)
			}
		}
		s.requests = append(s.requests, req)
		status, body := s.respond(req)
		if status == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("MULTICA_TOKEN", "test-token")
	return s
}

// writes lists the requests that change something, in order.
func (s *cortexTestServer) writes() []cortexRequest {
	var out []cortexRequest
	for _, req := range s.requests {
		if req.Method != http.MethodGet {
			out = append(out, req)
		}
	}
	return out
}

// requestsCatalog answers the reads nearly every command starts with.
func requestsCatalog(req cortexRequest) (int, any) {
	if req.Method != http.MethodGet {
		return 0, nil
	}
	switch req.Path {
	case "/api/collections":
		return 200, []collectionDTO{{ID: testRequestsID, Name: "Requests"}, {ID: testCustomersID, Name: "Customers"}}
	case "/api/collections/" + testRequestsID:
		return 200, testRequestsDetail()
	case "/api/collections/" + testCustomersID:
		return 200, collectionDetailDTO{Collection: collectionDTO{ID: testCustomersID, Name: "Customers"}}
	case "/api/workspaces/ws-1/members":
		return 200, []map[string]any{{"user_id": testMemberAdaID, "name": "Ada"}}
	}
	return 0, nil
}

// testCmdLike builds a fresh command carrying the same flags as a real one, so
// a test exercises the real flag names and defaults without leaving values
// behind on the package-level command.
func testCmdLike(t *testing.T, source *cobra.Command, flags ...string) *cobra.Command {
	t.Helper()
	if len(flags)%2 != 0 {
		t.Fatalf("flags must come as name, value pairs: %v", flags)
	}
	cmd := &cobra.Command{Use: source.Use}
	source.Flags().VisitAll(func(f *pflag.Flag) {
		switch f.Value.Type() {
		case "string":
			cmd.Flags().String(f.Name, f.DefValue, "")
		case "bool":
			cmd.Flags().Bool(f.Name, f.DefValue == "true", "")
		case "int":
			n, _ := strconv.Atoi(f.DefValue)
			cmd.Flags().Int(f.Name, n, "")
		case "int64":
			n, _ := strconv.ParseInt(f.DefValue, 10, 64)
			cmd.Flags().Int64(f.Name, n, "")
		case "float64":
			n, _ := strconv.ParseFloat(f.DefValue, 64)
			cmd.Flags().Float64(f.Name, n, "")
		case "stringArray":
			cmd.Flags().StringArray(f.Name, nil, "")
		default:
			t.Fatalf("testCmdLike does not know flag type %q of --%s", f.Value.Type(), f.Name)
		}
	})
	for i := 0; i < len(flags); i += 2 {
		if err := cmd.Flags().Set(flags[i], flags[i+1]); err != nil {
			t.Fatalf("set --%s %q: %v", flags[i], flags[i+1], err)
		}
	}
	return cmd
}

func TestMatchCollectionRef(t *testing.T) {
	collections := []collectionDTO{
		{ID: testRequestsID, Name: "Requests"},
		{ID: testCustomersID, Name: "Customers"},
		// A name made of hex characters that is also the start of another id.
		{ID: "77777777-cccc-4ccc-8ccc-cccccccccccc", Name: "2222"},
	}
	for _, tt := range []struct{ name, ref, want string }{
		{"full id", testCustomersID, testCustomersID},
		{"name, any case", "requests", testRequestsID},
		{"id prefix", "1111", testRequestsID},
		{"a name wins over an id prefix", "2222", "77777777-cccc-4ccc-8ccc-cccccccccccc"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchCollectionRef(collections, tt.ref)
			if err != nil || got.ID != tt.want {
				t.Fatalf("matchCollectionRef(%q) = %q, %v; want %q", tt.ref, got.ID, err, tt.want)
			}
		})
	}

	_, err := matchCollectionRef(collections, "Roadmap")
	if err == nil || !strings.Contains(err.Error(), "available: Requests, Customers, 2222") {
		t.Errorf("a miss should list what exists, got %v", err)
	}
	twins := append(collections, collectionDTO{ID: "88888888-dddd-4ddd-8ddd-dddddddddddd", Name: "requests"})
	_, err = matchCollectionRef(twins, "Requests")
	if err == nil || !strings.Contains(err.Error(), testRequestsID) || !strings.Contains(err.Error(), "88888888-dddd") {
		t.Errorf("two tables sharing a name should list both ids, got %v", err)
	}
}

func TestResolveCollectionFieldRef(t *testing.T) {
	detail := testRequestsDetail()
	for _, ref := range []string{"seats", testSeatsID} {
		field, err := resolveCollectionFieldRef(detail, ref)
		if err != nil || field.ID != testSeatsID {
			t.Errorf("resolveCollectionFieldRef(%q) = %q, %v", ref, field.ID, err)
		}
	}
	_, err := resolveCollectionFieldRef(detail, "Priority")
	if err == nil || !strings.Contains(err.Error(), "available: Stage, Seats, Tags, Owner, Implementation, Customer") {
		t.Errorf("a miss should list the table's fields, got %v", err)
	}
}

// A 403 is normally flattened into generic "no access" copy. The structure
// refusal opts out by code, because it is the one refusal a caller can act on:
// the table is visible and its rows are still open to them.
func TestCollectionSchemaRefusalSaysWhoMayChangeTheStructure(t *testing.T) {
	refused := &cli.HTTPError{Method: "POST", Path: "/api/collections/x/fields", StatusCode: 403,
		Body: `{"error":"only the table's creator or a workspace owner or admin can change its structure","code":"collection_schema_forbidden"}`}
	got := cli.FormatError(collectionSchemaRequestError("add field", refused), false)
	for _, want := range []string{"edit access", "owner", "owns its runtime"} {
		if !strings.Contains(got, want) {
			t.Errorf("message %q should contain %q", got, want)
		}
	}
	if cli.ExitCodeFor(collectionSchemaRequestError("add field", refused)) != cli.ExitAuth {
		t.Errorf("the refusal must still exit as an authorization failure")
	}

	// Any other 403 keeps the generic copy: it may be about something else
	// entirely, and must not be explained as a structure rule.
	other := &cli.HTTPError{Method: "PATCH", Path: "/api/collections/x", StatusCode: 403, Body: `{"error":"insufficient permissions"}`}
	if got := cli.FormatError(collectionSchemaRequestError("update table", other), false); strings.Contains(got, "creator") {
		t.Errorf("an uncoded 403 must not claim the structure rule: %q", got)
	}
}

func TestRunCollectionFieldAddBuildsTheConfigForItsType(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags []string
		want  map[string]any
	}{
		{
			name:  "select options keep their colors",
			flags: []string{"name", "Tier", "type", "select", "option", "Enterprise:#2563eb", "option", "Free"},
			want: map[string]any{"name": "Tier", "type": "select", "config": map[string]any{"options": []any{
				map[string]any{"name": "Enterprise", "color": "#2563eb"},
				map[string]any{"name": "Free", "color": defaultOptionColor},
			}}},
		},
		{
			name:  "a relation to issues",
			flags: []string{"name", "Implementation", "type", "relation", "relation-to", "issues"},
			want:  map[string]any{"name": "Implementation", "type": "relation", "config": map[string]any{"relation": map[string]any{"to_type": "issue"}}},
		},
		{
			name:  "a relation to a table named by name",
			flags: []string{"name", "Customer", "type", "relation", "relation-to", "customers"},
			want: map[string]any{"name": "Customer", "type": "relation", "config": map[string]any{"relation": map[string]any{
				"to_type": "record", "collection_id": testCustomersID,
			}}},
		},
		{
			name:  "a plain field sends no config",
			flags: []string{"name", "Seats", "type", "number"},
			want:  map[string]any{"name": "Seats", "type": "number"},
		},
		{
			name:  "a formula sends its expression",
			flags: []string{"name", "Double", "type", "formula", "expression", "{Seats} * 2"},
			want:  map[string]any{"name": "Double", "type": "formula", "config": map[string]any{"formula": map[string]any{"expression": "{Seats} * 2"}}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
				if req.Method == http.MethodPost && req.Path == "/api/collections/"+testRequestsID+"/fields" {
					return 201, collectionFieldDTO{ID: testSeatsID, Name: "created", Type: "text"}
				}
				return requestsCatalog(req)
			})
			cmd := testCmdLike(t, collectionFieldAddCmd, append(tt.flags, "output", "json")...)
			if _, err := captureStdout(t, func() error { return runCollectionFieldAdd(cmd, []string{"Requests"}) }); err != nil {
				t.Fatalf("runCollectionFieldAdd: %v", err)
			}
			writes := srv.writes()
			if len(writes) != 1 {
				t.Fatalf("writes = %+v, want exactly one", writes)
			}
			if got, want := jsonValue(t, writes[0].Body), jsonValue(t, tt.want); !jsonEqual(got, want) {
				t.Errorf("body = %v\nwant   %v", got, want)
			}
		})
	}
}

func TestRunCollectionFieldAddRefusesFlagsThatDoNotFitTheType(t *testing.T) {
	srv := newCortexTestServer(t, requestsCatalog)
	for _, flags := range [][]string{
		{"name", "Link", "type", "relation"},                          // no target
		{"name", "Notes", "type", "text", "relation-to", "Customers"}, // a target on a plain field
		{"type", "text"}, // no name
	} {
		cmd := testCmdLike(t, collectionFieldAddCmd, flags...)
		if err := runCollectionFieldAdd(cmd, []string{"Requests"}); err == nil {
			t.Errorf("flags %v should be refused before any request", flags)
		}
	}
	if len(srv.requests) != 0 {
		t.Errorf("a refused command made %d requests", len(srv.requests))
	}
}

// Replacing the option list matches by name, so an option that stays keeps its
// id — and with it, every row value that points at it.
func TestRunCollectionFieldUpdateKeepsTheIDsOfOptionsThatStay(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method == http.MethodPatch {
			return 200, collectionFieldDTO{ID: testStageID, Name: "Stage", Type: "select"}
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, collectionFieldUpdateCmd, "option", "triage", "option", "Doing", "output", "json")
	if _, err := captureStdout(t, func() error { return runCollectionFieldUpdate(cmd, []string{"Requests", "Stage"}) }); err != nil {
		t.Fatalf("runCollectionFieldUpdate: %v", err)
	}
	writes := srv.writes()
	if len(writes) != 1 || writes[0].Path != "/api/collections/"+testRequestsID+"/fields/"+testStageID {
		t.Fatalf("writes = %+v", writes)
	}
	want := map[string]any{"config": map[string]any{"options": []any{
		map[string]any{"id": testTriageID, "name": "triage", "color": defaultOptionColor},
		map[string]any{"name": "Doing", "color": defaultOptionColor},
	}}}
	if got := jsonValue(t, writes[0].Body); !jsonEqual(got, jsonValue(t, want)) {
		t.Errorf("body = %v\nwant   %v", got, want)
	}
}

// --add-option is the safe way to extend a select field: the API only takes
// whole lists and clears whatever is missing from every row, so the command
// carries the existing options over by id instead of trusting the caller to.
func TestRunCollectionFieldUpdateAddOptionKeepsEveryExistingOption(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method == http.MethodPatch {
			return 200, collectionFieldDTO{ID: testStageID, Name: "Stage", Type: "select"}
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, collectionFieldUpdateCmd, "add-option", "Churned:#ef4444", "add-option", "Paused", "output", "json")
	if _, err := captureStdout(t, func() error { return runCollectionFieldUpdate(cmd, []string{"Requests", "Stage"}) }); err != nil {
		t.Fatalf("runCollectionFieldUpdate: %v", err)
	}
	writes := srv.writes()
	if len(writes) != 1 || writes[0].Path != "/api/collections/"+testRequestsID+"/fields/"+testStageID {
		t.Fatalf("writes = %+v", writes)
	}
	want := map[string]any{"config": map[string]any{"options": []any{
		map[string]any{"id": testTriageID, "name": "Triage", "color": "#2563eb"},
		map[string]any{"id": testShippedID, "name": "Shipped", "color": defaultOptionColor},
		map[string]any{"name": "Churned", "color": "#ef4444"},
		map[string]any{"name": "Paused", "color": defaultOptionColor},
	}}}
	if got := jsonValue(t, writes[0].Body); !jsonEqual(got, jsonValue(t, want)) {
		t.Errorf("body = %v\nwant   %v", got, want)
	}
}

func TestRunCollectionFieldUpdateAddOptionRefusesBeforeAnyWrite(t *testing.T) {
	for _, tt := range []struct {
		name  string
		field string
		flags []string
		want  string
	}{
		{"an option that is already there", "Stage", []string{"add-option", "shipped"}, "already has an option"},
		{"the same new option twice", "Stage", []string{"add-option", "Paused", "add-option", "paused"}, "already has an option"},
		{"a field without options", "Seats", []string{"add-option", "Many"}, "only applies to select"},
		{"together with a replacement list", "Stage", []string{"add-option", "Paused", "option", "Triage"}, "pass one of them"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := newCortexTestServer(t, requestsCatalog)
			err := runCollectionFieldUpdate(testCmdLike(t, collectionFieldUpdateCmd, tt.flags...), []string{"Requests", tt.field})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want one containing %q", err, tt.want)
			}
			if writes := srv.writes(); len(writes) != 0 {
				t.Errorf("a refused command wrote %+v", writes)
			}
		})
	}
}

func TestRunCollectionListFiltersByProject(t *testing.T) {
	project := "99999999-eeee-4eee-8eee-eeeeeeeeeeee"
	newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == "/api/collections" {
			return 200, []collectionDTO{{ID: testRequestsID, Name: "Requests", ProjectID: &project}, {ID: testCustomersID, Name: "Customers"}}
		}
		return 0, nil
	})
	cmd := testCmdLike(t, collectionListCmd, "project", project, "output", "json")
	out, err := captureStdout(t, func() error { return runCollectionList(cmd, nil) })
	if err != nil {
		t.Fatalf("runCollectionList: %v", err)
	}
	var listed []collectionDTO
	if err := json.Unmarshal([]byte(out), &listed); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if len(listed) != 1 || listed[0].ID != testRequestsID {
		t.Errorf("listed = %+v, want only the project's table", listed)
	}
}

func jsonEqual(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func TestCollectionFormulaCLIUpdateAndErrors(t *testing.T) {
	formula := collectionFieldDTO{ID: testTagsID, Name: "Total", Type: "formula"}
	if err := json.Unmarshal([]byte(`{"expression":"{Seats} * 2","bindings":{"Seats":"`+testSeatsID+`"}}`), &formula.Config.Formula); err != nil {
		t.Fatal(err)
	}
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method == http.MethodGet && req.Path == "/api/collections/"+testRequestsID {
			detail := testRequestsDetail()
			detail.Fields = append(detail.Fields, formula)
			return 200, detail
		}
		if req.Method == http.MethodPatch {
			return 200, formula
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, collectionFieldUpdateCmd, "expression", "ROUND({Seats} / 3, 2)", "output", "json")
	if _, err := captureStdout(t, func() error { return runCollectionFieldUpdate(cmd, []string{"Requests", "Total"}) }); err != nil {
		t.Fatal(err)
	}
	writes := srv.writes()
	if len(writes) != 1 {
		t.Fatal(writes)
	}
	cfg := writes[0].Body["config"].(map[string]any)["formula"].(map[string]any)
	if cfg["expression"] != "ROUND({Seats} / 3, 2)" || cfg["bindings"].(map[string]any)["Seats"] != testSeatsID {
		t.Fatal(cfg)
	}
	rows := buildRecordRows([]collectionFieldDTO{formula}, recordDTO{FormulaErrors: map[string]string{testTagsID: "#REF!"}}, nil)
	if len(rows) != 1 || rows[0].Display != "#REF!" {
		t.Fatal(rows)
	}
}
