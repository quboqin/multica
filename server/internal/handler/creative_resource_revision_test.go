package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

const validComposableCopyLibraryJSON = `{
  "schema_version":2,
  "locale":"id-ID",
  "product_facts":[{"id":"fact-limit","key":"limit","label":"Limit","value":"80000000","copy_text":"Rp80.000.000","source":"approved sheet","status":"approved"}],
  "fragments":[
    {"id":"fragment-num","key":"num","creative_types":["num"],"role":"benefit","text":"Limit hingga {{fact.limit.copy_text}}","status":"approved"},
    {"id":"fragment-plan","key":"plan","creative_types":["repayment_plan"],"role":"benefit","text":"Pilihan Tenor","status":"approved"}
  ],
  "recipes":[
    {"id":"recipe-num","key":"num","creative_type":"num","fragment_ids":{"benefit":["fragment-num"]},"status":"approved"},
    {"id":"recipe-plan","key":"plan","creative_type":"repayment_plan","fragment_ids":{"benefit":["fragment-plan"]},"status":"approved"}
  ]
}`

func TestListCreativeResourcesReturnsPublishedConfigWhileDraftExists(t *testing.T) {
	resourceID := uuid.NewString()
	_, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource (id, workspace_id, kind, name, description, status, version, published_version, config, created_by)
VALUES ($1, $2, 'copy_library', 'Version test', '', 'draft', 3, 2, '{"version":"v3-draft"}'::jsonb, $3)
`, resourceID, testWorkspaceID, testUserID)
	if err != nil {
		t.Fatalf("insert creative resource: %v", err)
	}
	t.Cleanup(func() { _, _ = testPool.Exec(t.Context(), `DELETE FROM creative_resource WHERE id = $1`, resourceID) })
	for version, config := range map[int]string{1: `{"version":"v1"}`, 2: `{"version":"v2-published"}`, 3: `{"version":"v3-draft"}`} {
		if _, err := testPool.Exec(t.Context(), `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1, $2, 'Version test', '', $3::jsonb, $4)
`, resourceID, version, config, testUserID); err != nil {
			t.Fatalf("insert creative resource revision %d: %v", version, err)
		}
	}

	w := httptest.NewRecorder()
	req := newRequest(http.MethodGet, "/api/creative/resources", nil)
	testHandler.ListCreativeResources(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var response struct {
		Resources []creativeResourceResponse `json:"resources"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, resource := range response.Resources {
		if resource.ID != resourceID {
			continue
		}
		assertJSONDocumentEquals(t, resource.Config, `{"version":"v3-draft"}`)
		assertJSONDocumentEquals(t, resource.PublishedConfig, `{"version":"v2-published"}`)
		return
	}
	t.Fatal("created resource not returned")
}

func assertJSONDocumentEquals(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("decode JSON %s: %v", got, err)
	}
	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("decode expected JSON %s: %v", want, err)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("JSON = %s, want %s", gotJSON, wantJSON)
	}
}

func TestValidateComposableCopyLibraryConfig(t *testing.T) {
	if err := validateComposableCopyLibraryConfig(json.RawMessage(validComposableCopyLibraryJSON)); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for _, test := range []struct {
		name        string
		replaceFrom string
		replaceTo   string
		want        string
	}{
		{name: "fact source", replaceFrom: `"source":"approved sheet"`, replaceTo: `"source":""`, want: "requires id, key, copy_text, and source"},
		{name: "fact reference", replaceFrom: `fact.limit.copy_text`, replaceTo: `fact.unknown.copy_text`, want: `references missing or unapproved fact "unknown"`},
		{name: "duplicate fact key", replaceFrom: `"product_facts":[`, replaceTo: `"product_facts":[{"id":"fact-limit-duplicate","key":"limit","copy_text":"Duplicate","source":"approved sheet","status":"approved"},`, want: `fact key "limit" is duplicated`},
		{name: "fragment role", replaceFrom: `"benefit":["fragment-num"]`, replaceTo: `"headline":["fragment-num"]`, want: `instead of "benefit"`},
		{name: "duplicate recipe id", replaceFrom: `"id":"recipe-plan"`, replaceTo: `"id":"recipe-num"`, want: `recipe id "recipe-num" is duplicated`},
		{name: "missing repayment recipe", replaceFrom: `"id":"recipe-plan","key":"plan","creative_type":"repayment_plan","fragment_ids":{"benefit":["fragment-plan"]},"status":"approved"`, replaceTo: `"id":"recipe-plan","key":"plan","creative_type":"repayment_plan","fragment_ids":{"benefit":["fragment-plan"]},"status":"draft"`, want: "approved repayment_plan recipe"},
	} {
		t.Run(test.name, func(t *testing.T) {
			raw := []byte(validComposableCopyLibraryJSON)
			raw = []byte(strings.Replace(string(raw), test.replaceFrom, test.replaceTo, 1))
			err := validateComposableCopyLibraryConfig(raw)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
