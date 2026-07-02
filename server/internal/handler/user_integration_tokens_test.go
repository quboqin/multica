package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateMePreservesUnknownIntegrationTokenKeys(t *testing.T) {
	userID := newLanguageTestUser(t, "integration-token-preserve@multica.ai")
	ctx := context.Background()
	if _, err := testPool.Exec(ctx,
		`UPDATE "user" SET integration_tokens = '{"git_token":"old-git","notion_token":"old-notion","paihub_token":"old-wrong"}'::jsonb WHERE id = $1`,
		userID,
	); err != nil {
		t.Fatalf("preset integration tokens: %v", err)
	}

	w := httptest.NewRecorder()
	req := newPatchMeRequest(userID, `{"integration_tokens":{"git_token":"new-git"}}`)
	testHandler.UpdateMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stored []byte
	if err := testPool.QueryRow(ctx,
		`SELECT integration_tokens FROM "user" WHERE id = $1`,
		userID,
	).Scan(&stored); err != nil {
		t.Fatalf("lookup user: %v", err)
	}
	var tokens map[string]string
	if err := json.Unmarshal(stored, &tokens); err != nil {
		t.Fatalf("decode stored tokens: %v", err)
	}
	if tokens["git_token"] != "new-git" || tokens["notion_token"] != "old-notion" || tokens["paones_token"] != "old-wrong" {
		t.Fatalf("tokens not merged correctly: %#v", tokens)
	}
	if _, ok := tokens["paihub_token"]; ok {
		t.Fatalf("legacy paihub_token should be removed on save: %#v", tokens)
	}
}

func TestUpdateMeDeletesIntegrationTokenOnBlankValue(t *testing.T) {
	userID := newLanguageTestUser(t, "integration-token-delete@multica.ai")
	ctx := context.Background()
	if _, err := testPool.Exec(ctx,
		`UPDATE "user" SET integration_tokens = '{"git_token":"old-git","notion_token":"old-notion"}'::jsonb WHERE id = $1`,
		userID,
	); err != nil {
		t.Fatalf("preset integration tokens: %v", err)
	}

	w := httptest.NewRecorder()
	req := newPatchMeRequest(userID, `{"integration_tokens":{"notion_token":""}}`)
	testHandler.UpdateMe(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stored []byte
	if err := testPool.QueryRow(ctx,
		`SELECT integration_tokens FROM "user" WHERE id = $1`,
		userID,
	).Scan(&stored); err != nil {
		t.Fatalf("lookup user: %v", err)
	}
	var tokens map[string]string
	if err := json.Unmarshal(stored, &tokens); err != nil {
		t.Fatalf("decode stored tokens: %v", err)
	}
	if _, ok := tokens["notion_token"]; ok {
		t.Fatalf("blank token value should delete the key: %#v", tokens)
	}
	if tokens["git_token"] != "old-git" {
		t.Fatalf("unrelated token not preserved: %#v", tokens)
	}
}
