package lark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginClientExchangeCodeUsesAppAccessToken(t *testing.T) {
	t.Parallel()

	var sawAppToken bool
	var sawUserToken bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/app_access_token/internal":
			sawAppToken = true
			if r.Method != http.MethodPost {
				t.Errorf("app token method = %s, want POST", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Errorf("app token Authorization = %q, want empty", got)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode app token body: %v", err)
			}
			if body["app_id"] != "cli_test" || body["app_secret"] != "secret" {
				t.Errorf("app token body = %+v", body)
			}
			writeJSON(w, map[string]any{
				"code":             0,
				"msg":              "success",
				"app_access_token": "app-token",
			})
		case "/open-apis/authen/v1/access_token":
			sawUserToken = true
			if got := r.Header.Get("Authorization"); got != "Bearer app-token" {
				t.Errorf("user token Authorization = %q, want Bearer app-token", got)
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode user token body: %v", err)
			}
			if body["grant_type"] != "authorization_code" || body["code"] != "auth-code" {
				t.Errorf("user token body = %+v", body)
			}
			writeJSON(w, map[string]any{
				"code": 0,
				"msg":  "success",
				"data": map[string]any{
					"access_token": "user-token",
					"open_id":      "ou_1",
					"union_id":     "on_1",
					"name":         "Feishu User",
					"email":        "USER@EXAMPLE.COM",
					"avatar_url":   "https://example.com/avatar.png",
				},
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client := NewLoginClient(LoginClientConfig{BaseURL: srv.URL}, RegionFeishu)
	info, err := client.ExchangeCode(context.Background(), "cli_test", "secret", "auth-code")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if !sawAppToken || !sawUserToken {
		t.Fatalf("expected both token endpoints to be called, app=%v user=%v", sawAppToken, sawUserToken)
	}
	if info.OpenID != "ou_1" || info.UnionID != "on_1" || info.Name != "Feishu User" {
		t.Fatalf("unexpected info: %+v", info)
	}
	if info.Email != "user@example.com" {
		t.Fatalf("email = %q, want normalized lowercase", info.Email)
	}
}
