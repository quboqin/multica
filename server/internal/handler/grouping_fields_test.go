package handler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestIssueGroupingAllFieldTypes(t *testing.T) {
	cases := []struct {
		kind  string
		value any
	}{
		{"text", "North"}, {"number", 0}, {"date", "2026-10-09"}, {"url", "https://example.com"},
		{"checkbox", false}, {"select", "a"}, {"multi_select", []string{"a", "b"}},
		{"actor", "member:" + testUserID}, {"multi_actor", []string{"member:" + testUserID}},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			project := dbfx.Insert(t, "project", testutil.Cols{"workspace_id": testWorkspaceID, "title": t.Name()})
			property := dbfx.Insert(t, "issue_property", testutil.Cols{"workspace_id": testWorkspaceID, "name": tc.kind, "type": tc.kind, "config": `{"options":[{"id":"a","name":"A","color":"#666666"},{"id":"b","name":"B","color":"#777777"}]}`})
			for i := range 3 {
				value := tc.value
				if tc.kind == "multi_select" && i == 1 {
					value = []string{"b", "a"}
				}
				raw, _ := json.Marshal(map[string]any{property: value})
				dbfx.Issue(t, fmt.Sprint(i), testutil.Cols{"project_id": project, "properties": string(raw)})
			}
			dbfx.Issue(t, "Unset", testutil.Cols{"project_id": project})
			query := statusCategoryQuery(project)
			spec := issueTableGroupSpec{Kind: "property", PropertyID: property, IncludeEmpty: true}
			request := issueTableGroupsRequest{Query: query, Group: spec, Page: issueTablePageRequest{Limit: 1}}
			all := []issueTableGroupDescriptorResponse{}
			for {
				var page issueTableGroupsResponse
				testutil.Call(t, testHandler.ListIssueTableGroups, newRequest("POST", "/api/issues/table/groups", request)).Want(200).JSON(&page)
				if page.Total != 4 {
					t.Fatalf("total=%d", page.Total)
				}
				all = append(all, page.Groups...)
				if page.NextCursor == nil {
					break
				}
				request.Page.Cursor = page.NextCursor
			}
			if len(all) < 2 {
				t.Fatalf("groups=%+v", all)
			}
			for _, group := range all {
				request := issueTableRowsRequest{Query: query, Group: spec, GroupKey: &group.Key, Page: issueTablePageRequest{Limit: 1}}
				seen := map[string]bool{}
				for {
					var page issueTableRowsResponse
					testutil.Call(t, testHandler.ListIssueTableRows, newRequest("POST", "/api/issues/table/rows", request)).Want(200).JSON(&page)
					for _, row := range page.Rows {
						if seen[row.Issue.ID] {
							t.Fatal("duplicate row")
						}
						seen[row.Issue.ID] = true
					}
					if page.NextCursor == nil {
						break
					}
					request.Page.Cursor = page.NextCursor
				}
				if int64(len(seen)) != group.Count {
					t.Fatalf("group count=%d rows=%d", group.Count, len(seen))
				}
			}
		})
	}
}

func TestCollectionGroupingAllFieldTypes(t *testing.T) {
	cases := []struct {
		kind  string
		value any
	}{
		{"text", "__none__"}, {"number", 0}, {"date", "2026-10-09"}, {"url", "https://example.com"},
		{"checkbox", false}, {"select", "a"}, {"multi_select", []string{"a", "b"}},
		{"actor", "member:" + testUserID}, {"multi_actor", []string{"member:" + testUserID}}, {"formula", nil}, {"relation", nil}, {"title", "Same"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			collection := dbfx.Insert(t, "collection", testutil.Cols{"workspace_id": testWorkspaceID, "created_by": testUserID, "name": t.Name()})
			field := "title"
			if tc.kind != "title" {
				config := `{"options":[{"id":"a","name":"A","color":"#666666"},{"id":"b","name":"B","color":"#777777"}]}`
				if tc.kind == "formula" {
					config = `{"formula":{"expression":"LEN({title})","bindings":{"title":"title"}}}`
				}
				if tc.kind == "relation" {
					config = `{"relation":{"to_type":"issue"}}`
				}
				field = dbfx.Insert(t, "collection_field", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "name": tc.kind, "type": tc.kind, "config": config})
			}
			target := dbfx.Issue(t, "Linked task")
			for i := range 4 {
				value := tc.value
				if tc.kind == "multi_select" && i == 1 {
					value = []string{"b", "a"}
				}
				fields := map[string]any{field: value}
				title := "Same"
				if i == 3 {
					fields = map[string]any{}
					title = ""
				}
				raw, _ := json.Marshal(fields)
				record := dbfx.Insert(t, "record", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "title": title, "fields": string(raw)})
				if tc.kind == "relation" && i < 3 {
					dbfx.Insert(t, "record_link", testutil.Cols{"workspace_id": testWorkspaceID, "collection_id": collection, "from_record_id": record, "from_field_id": field, "to_type": "issue", "to_id": target})
				}
			}
			type page struct {
				Records []struct {
					ID string `json:"id"`
				} `json:"records"`
				Total  int `json:"total"`
				Groups []struct {
					Key   string `json:"key"`
					Count int    `json:"count"`
				} `json:"groups"`
				Next *string `json:"next_cursor"`
			}
			fetch := func(query url.Values) page {
				t.Helper()
				var out page
				testutil.Call(t, testHandler.ListCollectionRecords, withURLParam(newRequest("GET", "/api/collections/records?"+query.Encode(), nil), "collectionID", collection)).Want(200).JSON(&out)
				return out
			}
			q := url.Values{"group_by": {field}, "limit": {"1"}}
			head := fetch(q)
			if head.Total != 4 || len(head.Groups) != 2 {
				t.Fatalf("head=%+v", head)
			}
			for _, group := range head.Groups {
				q.Set("group_key", group.Key)
				q.Del("cursor")
				seen := map[string]bool{}
				for {
					result := fetch(q)
					for _, record := range result.Records {
						if seen[record.ID] {
							t.Fatal("duplicate row")
						}
						seen[record.ID] = true
					}
					if result.Next == nil {
						break
					}
					q.Set("cursor", *result.Next)
				}
				if len(seen) != group.Count {
					t.Fatalf("group=%+v rows=%d", group, len(seen))
				}
			}
			// Group catalogs honor the same filter as their paged branches.
			q.Del("group_key")
			q.Del("cursor")
			q.Set("search", "Same")
			filtered := fetch(q)
			if filtered.Total != 3 || len(filtered.Groups) != 1 {
				t.Fatalf("filtered=%+v", filtered)
			}
		})
	}
}
