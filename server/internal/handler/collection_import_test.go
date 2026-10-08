package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/internal/collectionimport"
	"github.com/multica-ai/multica/server/internal/testutil"
	"io"
	"mime/multipart"
	"strings"
	"testing"
)

func TestCollectionImportCreatesTableAndFieldsAtomically(t *testing.T) {
	name := t.Name()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), "DELETE FROM record WHERE collection_id IN (SELECT id FROM collection WHERE workspace_id=$1 AND name=$2)", testWorkspaceID, name)
		_, _ = testPool.Exec(context.Background(), "DELETE FROM collection_field WHERE collection_id IN (SELECT id FROM collection WHERE workspace_id=$1 AND name=$2)", testWorkspaceID, name)
		_, _ = testPool.Exec(context.Background(), "DELETE FROM collection WHERE workspace_id=$1 AND name=$2", testWorkspaceID, name)
	})
	call := func(data string, opts map[string]any) *testutil.Response {
		var b bytes.Buffer
		writer := multipart.NewWriter(&b)
		part, _ := writer.CreateFormFile("file", "customers.csv")
		_, _ = io.WriteString(part, data)
		raw, _ := json.Marshal(opts)
		_ = writer.WriteField("options", string(raw))
		_ = writer.Close()
		req := newRequest("POST", "/api/collections/import", nil)
		req.Body = io.NopCloser(&b)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		return testutil.Call(t, testHandler.ImportCollection, req)
	}
	csv := "Customer,Seats,Stage,Internal\nAcme,42,Lead,skip me\n客户,43,Won,also skip\n"
	var p collectionimport.Preview
	call(csv, map[string]any{"dry_run": true}).Want(200).JSON(&p)
	if p.RowCount != 2 || p.Columns[1].Type != "number" {
		t.Fatal(p)
	}
	p.Columns[2].Type = "select"
	p.Columns[3].Skip = true
	opts := map[string]any{"dry_run": false, "name": name, "title_column": 0, "columns": p.Columns}
	// An invalid final value must not leave even an empty collection behind.
	call(strings.Replace(csv, "43", "bad", 1), opts).Want(400)
	var count int
	_ = testPool.QueryRow(t.Context(), "SELECT count(*) FROM collection WHERE workspace_id=$1 AND name=$2", testWorkspaceID, name).Scan(&count)
	if count != 0 {
		t.Fatal("failed import created a table")
	}
	var result struct {
		Collection struct{ ID, Name, TitleName string }
		Count      int
	}
	call(csv, opts).Want(201).JSON(&result)
	if result.Count != 2 || result.Collection.Name != name {
		t.Fatal(result)
	}
	_ = testPool.QueryRow(t.Context(), "SELECT count(*) FROM collection_field WHERE collection_id=$1", result.Collection.ID).Scan(&count)
	if count != 2 {
		t.Fatalf("created %d fields", count)
	}
	var number float64
	err := testPool.QueryRow(t.Context(), "SELECT (r.fields->>f.id::text)::numeric FROM record r JOIN collection_field f ON f.collection_id=r.collection_id AND f.name='Seats' WHERE r.collection_id=$1 AND r.title='Acme'", result.Collection.ID).Scan(&number)
	if err != nil || number != 42 {
		t.Fatalf("lost number: %f %v", number, err)
	}
	p.Columns[1].Index = 0
	opts["columns"] = p.Columns
	call(csv, opts).Want(400)
}
