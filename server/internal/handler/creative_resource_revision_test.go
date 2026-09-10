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
  "schema_version":4,
  "locale":"id-ID",
  "fragments":[
    {"id":"fragment-headline","key":"headline","creative_types":["num","repayment_plan"],"role":"headline","text":"Pinjaman Fleksibel","status":"approved"},
    {"id":"fragment-benefit","key":"benefit","creative_types":["num","repayment_plan"],"role":"benefit","text":"Limit hingga Rp80.000.000","status":"approved"}
  ],
  "repayment_plan":{"labels":{"principal":"Jumlah Pinjaman","tenor":"Periode Cicilan","monthly_installment":"Cicilan per Bulan","total_interest":"Total Bunga","total_repayment":"Total Pembayaran"},"entries":[
    {"id":"plan-30m-3","key":"30m-3","principal":30000000,"tenor_months":3,"monthly_installment":10270000,"total_interest":810000,"total_repayment":30810000,"source":"approved sheet","status":"approved"}
  ]},
  "recipes":[
    {"id":"recipe-num","key":"num","creative_type":"num","fragment_ids":{"headline":["fragment-headline"],"benefit":["fragment-benefit"]},"status":"approved"},
    {"id":"recipe-plan","key":"plan","creative_type":"repayment_plan","fragment_ids":{"headline":["fragment-headline"],"benefit":["fragment-benefit"]},"status":"approved"}
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
	withoutHeadline := strings.Replace(validComposableCopyLibraryJSON, `    {"id":"fragment-headline","key":"headline","creative_types":["num","repayment_plan"],"role":"headline","text":"Pinjaman Fleksibel","status":"approved"},
`, "", 1)
	withoutHeadline = strings.ReplaceAll(withoutHeadline, `"headline":["fragment-headline"],`, "")
	if err := validateComposableCopyLibraryConfig(json.RawMessage(withoutHeadline)); err != nil {
		t.Fatalf("headline-optional config rejected: %v", err)
	}
	for _, test := range []struct {
		name        string
		replaceFrom string
		replaceTo   string
		want        string
	}{
		{name: "template variable", replaceFrom: `Limit hingga Rp80.000.000`, replaceTo: `Limit hingga {{fact.limit.copy_text}}`, want: "must contain final copy, not template variables"},
		{name: "fragment role", replaceFrom: `"benefit":["fragment-benefit"]`, replaceTo: `"headline":["fragment-benefit"]`, want: `instead of "benefit"`},
		{name: "duplicate recipe id", replaceFrom: `"id":"recipe-plan"`, replaceTo: `"id":"recipe-num"`, want: `recipe id "recipe-num" is duplicated`},
		{name: "plan labels", replaceFrom: `"principal":"Jumlah Pinjaman"`, replaceTo: `"principal":""`, want: "repayment plan requires all table labels"},
		{name: "duplicate plan key", replaceFrom: `"entries":[`, replaceTo: `"entries":[{"id":"duplicate","key":"30m-3","principal":40000000,"tenor_months":3,"monthly_installment":12000000,"total_interest":1000000,"total_repayment":41000000,"source":"approved sheet","status":"approved"},`, want: `repayment plan key "30m-3" is duplicated`},
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
