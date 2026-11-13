package handler

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestCollectionHierarchyScopesPaginationAndTrash(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	parent := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Parent"})
	create := func(parentID string, status int) string {
		t.Helper()
		var out struct {
			ID       string `json:"id"`
			ParentID string `json:"parent_id"`
		}
		req := withURLParam(newRequest("POST", "/api/collections/records", map[string]any{"title": "Child", "parent_id": parentID}), "collectionID", c)
		result := testutil.Call(t, testHandler.CreateCollectionRecord, req).Want(status)
		if status == 201 {
			result.JSON(&out)
			dbfx.Cleanup(t, "DELETE FROM record WHERE id=$1", out.ID)
			if out.ParentID != parentID {
				t.Fatalf("parent was not preserved: %+v", out)
			}
		}
		return out.ID
	}
	child := create(parent, 201)
	grandchild := create(child, 201)
	create("invalid", 400)
	testutil.Call(t, testHandler.UpdateCollectionRecord, withURLParams(newRequest("PUT", "/", map[string]any{"parent_id": child}), "collectionID", c, "recordID", parent)).Want(400)
	other := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": "Other"})
	outside := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": other, "title": "Outside"})
	create(outside, 400)
	for i := range 3 {
		dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "parent_id": parent, "title": fmt.Sprint(i)})
	}
	type page struct {
		Records []struct {
			ID         string  `json:"id"`
			ParentID   *string `json:"parent_id"`
			ChildCount int     `json:"child_count"`
		} `json:"records"`
		Total int     `json:"total"`
		Next  *string `json:"next_cursor"`
	}
	fetch := func(query string, status int) page {
		t.Helper()
		var out page
		result := testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records?"+query, nil), "collectionID", c)).Want(status)
		if status == 200 {
			result.JSON(&out)
		}
		return out
	}
	roots := fetch("parent_id=root", 200)
	if roots.Total != 1 || len(roots.Records) != 1 || roots.Records[0].ID != parent || roots.Records[0].ChildCount != 4 {
		t.Fatalf("roots: %+v", roots)
	}
	first := fetch("parent_id="+parent+"&limit=2", 200)
	if first.Total != 4 || len(first.Records) != 2 || first.Next == nil {
		t.Fatalf("child page: %+v", first)
	}
	second := fetch("parent_id="+parent+"&limit=2&cursor="+url.QueryEscape(*first.Next), 200)
	if len(second.Records) != 2 || second.Next != nil {
		t.Fatalf("second child page: %+v", second)
	}
	fetch("parent_id="+child+"&cursor="+url.QueryEscape(*first.Next), 400)
	fetch("parent_id=invalid", 400)
	if all := fetch("search=Child", 200); all.Total != 2 {
		t.Fatalf("search hid a descendant: %+v", all)
	}
	testutil.Call(t, testHandler.DeleteCollectionRecord, withURLParams(newRequest("DELETE", "/", nil), "collectionID", c, "recordID", parent)).Want(200)
	create(parent, 400)
	roots = fetch("parent_id=root", 200)
	if roots.Total != 4 {
		t.Fatalf("children not promoted: %+v", roots)
	}
	for _, record := range roots.Records {
		if record.ParentID != nil {
			t.Fatalf("promoted child still has parent: %+v", record)
		}
	}
	if branch := fetch("parent_id="+child, 200); len(branch.Records) != 1 || branch.Records[0].ID != grandchild {
		t.Fatalf("grandchild lost: %+v", branch)
	}
	testutil.Call(t, testHandler.RestoreCollectionRecord, withURLParams(newRequest("POST", "/", nil), "collectionID", c, "recordID", parent)).Want(200)
	if roots = fetch("parent_id=root", 200); roots.Total != 5 {
		t.Fatalf("restore hid promoted children: %+v", roots)
	}
}

func TestCollectionHierarchyBatchDeleteAndRestore(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	parent := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Parent"})
	child := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Child", "parent_id": parent})
	sibling := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Sibling", "parent_id": parent})
	batch := func(action string) {
		t.Helper()
		testutil.Call(t, testHandler.BatchCollectionRecords, withURLParam(newRequest("POST", "/", map[string]any{"action": action, "record_ids": []string{parent, child}, "confirmed": true}), "collectionID", c)).Want(200)
	}
	batch("delete")
	var promoted bool
	dbfx.QueryRow(t, "SELECT parent_id IS NULL AND deleted_at IS NULL FROM record WHERE id=$1", sibling).Scan(&promoted)
	if !promoted {
		t.Fatal("unselected child was not preserved")
	}
	batch("restore")
	var linked bool
	dbfx.QueryRow(t, "SELECT parent_id=$2::uuid AND deleted_at IS NULL FROM record WHERE id=$1", child, parent).Scan(&linked)
	if !linked {
		t.Fatal("batch restore lost the selected hierarchy")
	}
}

func TestCollectionEmailFieldWrites(t *testing.T) {
	c := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
	var field struct {
		ID string `json:"id"`
	}
	testutil.Call(t, testHandler.CreateCollectionField, withURLParam(newRequest("POST", "/", map[string]any{"name": "Email", "type": "email"}), "collectionID", c)).Want(201).JSON(&field)
	dbfx.Cleanup(t, "DELETE FROM collection_field WHERE id=$1", field.ID)
	record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": c, "title": "Contact"})
	set := func(value any, status int) {
		t.Helper()
		testutil.Call(t, testHandler.SetCollectionRecordField, withURLParams(newRequest("PUT", "/", map[string]any{"value": value}), "collectionID", c, "recordID", record, "fieldID", field.ID)).Want(status)
	}
	set("Person+tag@example.com", 200)
	set("invalid", 400)
	testutil.Call(t, testHandler.BatchCollectionRecords, withURLParam(newRequest("POST", "/", map[string]any{"action": "update", "record_ids": []string{record}, "fields": map[string]any{field.ID: " next@example.com "}}), "collectionID", c)).Want(200)
	var email string
	dbfx.QueryRow(t, "SELECT fields->>$2::text FROM record WHERE id=$1", record, field.ID).Scan(&email)
	if email != "next@example.com" {
		t.Fatalf("email not normalized: %q", email)
	}
	set(nil, 200)
}
