package main

import (
	"encoding/json"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"testing"
)

func cortexChild(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, cmd := range root.Commands() {
		if cmd.Name() == name {
			return cmd
		}
	}
	t.Fatalf("command %s missing", name)
	return nil
}

func TestRecordBatchWritesOneAtomicRequest(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method == "POST" && req.Path == recordsPath(testRequestsID)+"/batch" {
			return 200, map[string]any{"count": 2}
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, cortexChild(t, recordCmd, "batch-update"), "record", testRowID, "record", testACMERowID, "set", "Seats=42", "expect-revision", testRowID+"=3", "expect-revision", testACMERowID+"=4")
	if err := runRecordBatch("update")(cmd, []string{"Requests"}); err != nil {
		t.Fatal(err)
	}
	writes := srv.writes()
	if len(writes) != 1 {
		t.Fatalf("writes=%+v", writes)
	}
	body := writes[0].Body
	if body["fields"].(map[string]any)[testSeatsID] != float64(42) || len(body["expected_revisions"].(map[string]any)) != 2 {
		t.Fatalf("body=%+v", body)
	}
	denied := testCmdLike(t, cortexChild(t, recordCmd, "batch-delete"), "record", testRowID)
	if err := runRecordBatch("delete")(denied, []string{"Requests"}); err == nil {
		t.Fatal("missing confirmation accepted")
	}
	if len(srv.writes()) != 1 {
		t.Fatal("unconfirmed delete wrote")
	}
}
func TestRecordCSVCommandPreservesBytesAndDryRun(t *testing.T) {
	data := "title,Seats\r\n\"客户, 一\",42\r\n"
	file := filepath.Join(t.TempDir(), "input.csv")
	if err := os.WriteFile(file, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Method == "POST" && req.Path == recordsPath(testRequestsID)+"/import" {
			return 200, map[string]any{"count": 1, "dry_run": true}
		}
		return requestsCatalog(req)
	})
	if err := runRecordImport(testCmdLike(t, cortexChild(t, recordCmd, "import"), "file", file, "dry-run", "true"), []string{"Requests"}); err != nil {
		t.Fatal(err)
	}
	writes := srv.writes()
	if len(writes) != 1 || writes[0].Body["csv"] != data || writes[0].Body["dry_run"] != true {
		t.Fatalf("writes=%+v", writes)
	}
}
func TestRecordComparisonAndRelationFilterCommands(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == recordsPath(testRequestsID) {
			return 200, recordsPage()
		}
		return requestsCatalog(req)
	})
	cmd := testCmdLike(t, recordListCmd, "filter", "Seats>=10", "filter", "Customer="+testACMERowID, "sort", "Implementation")
	if err := runRecordList(cmd, []string{"Requests"}); err != nil {
		t.Fatal(err)
	}
	last := srv.requests[len(srv.requests)-1]
	var props map[string][]any
	if err := json.Unmarshal([]byte(last.Query.Get("properties")), &props); err != nil {
		t.Fatal(err)
	}
	if props[testSeatsID][0].(map[string]any)["op"] != "gte" || props[testCustomerID][0] != testACMERowID || last.Query.Get("sort_by") != testTasksID {
		t.Fatalf("query=%v", last.Query)
	}
	name, value, op, err := splitRecordFilter("Notes=a>=b")
	if err != nil || name != "Notes" || value != "a>=b" || op != "=" {
		t.Fatalf("literal value changed: %s %s %s %v", name, value, op, err)
	}
}
func TestCollectionRestoreByNameAndArchivedField(t *testing.T) {
	srv := newCortexTestServer(t, func(req cortexRequest) (int, any) {
		if req.Path == "/api/collections" && req.Query.Get("archived") == "true" {
			return 200, []collectionDTO{{ID: testRequestsID, Name: "Requests"}}
		}
		if req.Path == "/api/collections/"+testRequestsID+"/restore" {
			return 200, collectionDTO{ID: testRequestsID, Name: "Requests"}
		}
		if req.Path == "/api/collections/"+testRequestsID+"/fields/archived" {
			return 200, []collectionFieldDTO{{ID: testSeatsID, Name: "Seats", Type: "number"}}
		}
		if req.Path == "/api/collections/"+testRequestsID+"/fields/"+testSeatsID+"/restore" {
			return 200, collectionFieldDTO{ID: testSeatsID, Name: "Seats", Type: "number"}
		}
		return requestsCatalog(req)
	})
	if err := runCollectionRestore(testCmdLike(t, cortexChild(t, collectionCmd, "restore")), []string{"Requests"}); err != nil {
		t.Fatal(err)
	}
	if err := runCollectionFieldRestore(testCmdLike(t, cortexChild(t, collectionFieldCmd, "restore")), []string{"Requests", "Seats"}); err != nil {
		t.Fatal(err)
	}
	if len(srv.writes()) != 2 {
		t.Fatal(srv.writes())
	}
}
