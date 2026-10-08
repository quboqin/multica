package handler

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

type formulaRecord struct {
	ID     string            `json:"id"`
	Fields map[string]any    `json:"fields"`
	Errors map[string]string `json:"formula_errors"`
}

func TestCollectionFormulaLifecycle(t *testing.T) {
	c := newCollection(t, t.Name())
	quantity := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "数量", "type": "number"})
	price := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "name": "单价", "type": "number"})
	add := func(name, expression string) string {
		t.Helper()
		var field struct {
			ID string `json:"id"`
		}
		createField(t, c, map[string]any{"name": name, "type": "formula", "config": map[string]any{"formula": map[string]any{"expression": expression}}}).Want(201).JSON(&field)
		return field.ID
	}
	total := add("总价", "{数量} * {单价}")
	tax := add("含税", "ROUND({总价} * 1.1, 2)")
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "客户", "fields": fmt.Sprintf(`{%q:3,%q:12.5}`, quantity, price)})
	read := func() formulaRecord {
		t.Helper()
		var page struct {
			Records []formulaRecord `json:"records"`
		}
		testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/?record_id="+record, nil), "collectionID", c)).Want(200).JSON(&page)
		if len(page.Records) != 1 {
			t.Fatalf("page=%+v", page)
		}
		return page.Records[0]
	}
	row := read()
	if row.Fields[total] != 37.5 || row.Fields[tax] != 41.25 {
		t.Fatalf("computed=%+v", row)
	}
	if _, stored := recordFields(t, record)[total]; stored {
		t.Fatal("derived value persisted in user data")
	}
	// A committed source edit returns fresh formulas, including chained fields.
	request := withURLParams(newRequest("PUT", "/", map[string]any{"value": 4}), "collectionID", c, "recordID", record, "fieldID", quantity)
	testutil.Call(t, testHandler.SetCollectionRecordField, request).Want(200).JSON(&row)
	if row.Fields[total] != float64(50) || row.Fields[tax] != float64(55) {
		t.Fatalf("write response=%+v", row)
	}
	patchField(t, c, price, map[string]any{"name": "售价"}).Want(200)
	if read().Fields[total] != float64(50) {
		t.Fatal("rename broke the reference")
	}
	var detail struct {
		Fields []struct {
			ID     string         `json:"id"`
			Config PropertyConfig `json:"config"`
		} `json:"fields"`
	}
	testutil.Call(t, testHandler.GetCollection, withURLParam(newRequest("GET", "/", nil), "collectionID", c)).Want(200).JSON(&detail)
	for _, field := range detail.Fields {
		if field.ID == total {
			if field.Config.Formula == nil || field.Config.Formula.Expression != "{数量} * {售价}" || field.Config.Formula.Bindings["售价"] != price {
				t.Fatalf("editor did not show current source labels: %+v", field.Config.Formula)
			}
			patchField(t, c, total, map[string]any{"config": field.Config}).Want(200)
		}
	}
	// Archiving a source exposes an error; restoring it restores the result.
	patchField(t, c, price, map[string]any{"archived": true}).Want(200)
	row = read()
	if row.Errors[total] != "#REF!" || row.Errors[tax] != "#REF!" {
		t.Fatalf("archive=%+v", row)
	}
	testutil.Call(t, testHandler.RestoreCollectionField, withURLParams(newRequest("POST", "/", nil), "collectionID", c, "fieldID", price)).Want(200)
	if read().Fields[tax] != float64(55) {
		t.Fatal("restore did not restore calculation")
	}
	patchField(t, c, total, map[string]any{"config": map[string]any{"formula": map[string]any{"expression": "{含税} + 1"}}}).Want(400)
	patchField(t, c, total, map[string]any{"config": map[string]any{"formula": map[string]any{"expression": "{总价} + 1"}}}).Want(400)
	if read().Fields[total] != float64(50) {
		t.Fatal("rejected expression replaced formula")
	}
	patchField(t, c, total, map[string]any{"config": map[string]any{"formula": map[string]any{"expression": "{数量} / 0"}}}).Want(200)
	if read().Errors[total] != "#DIV/0!" {
		t.Fatal("missing division error")
	}
	// Results are read-only through single writes, null/unset, create, batch and CSV.
	for _, v := range []any{123, nil} {
		testutil.Call(t, testHandler.SetCollectionRecordField, withURLParams(newRequest("PUT", "/", map[string]any{"value": v}), "collectionID", c, "recordID", record, "fieldID", total)).Want(400)
		testutil.Call(t, testHandler.CreateCollectionRecord, withURLParam(newRequest("POST", "/", map[string]any{"title": "bad", "fields": map[string]any{total: v}}), "collectionID", c)).Want(400)
		testutil.Call(t, testHandler.BatchCollectionRecords, withURLParam(newRequest("POST", "/", map[string]any{"action": "update", "record_ids": []string{record}, "fields": map[string]any{total: v}}), "collectionID", c)).Want(400)
	}
	testutil.Call(t, testHandler.ImportCollectionCSV, withURLParam(newRequest("POST", "/", map[string]any{"csv": "title,总价\nx,123\n", "dry_run": true}), "collectionID", c)).Want(400)
	testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/?sort_by="+total, nil), "collectionID", c)).Want(400)
	filter := url.QueryEscape(fmt.Sprintf(`{%q:[1]}`, total))
	testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/?properties="+filter, nil), "collectionID", c)).Want(400)
}

func TestCollectionFormulaValidatesSourcesAndScope(t *testing.T) {
	c := newCollection(t, t.Name())
	other := newCollection(t, t.Name()+" other")
	foreign := dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": other, "name": "Secret", "type": "number"})
	for _, config := range []any{
		map[string]any{"expression": "{Secret}", "bindings": map[string]string{"Secret": foreign}},
		map[string]any{"expression": "{missing}"},
		map[string]any{"expression": "unknown(1)"},
	} {
		createField(t, c, map[string]any{"name": "bad", "type": "formula", "config": map[string]any{"formula": config}}).Want(400)
	}
	createField(t, c, map[string]any{"name": "Empty", "type": "formula"}).Want(400)
	createField(t, c, map[string]any{"name": "Bad type", "type": "number", "config": map[string]any{"formula": map[string]any{"expression": "1"}}}).Want(400)
}
