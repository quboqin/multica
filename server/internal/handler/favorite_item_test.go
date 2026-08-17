package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func withFavoriteURLParams(req *http.Request, kv ...string) *http.Request {
	rctx := chi.NewRouteContext()
	for i := 0; i+1 < len(kv); i += 2 {
		rctx.URLParams.Add(kv[i], kv[i+1])
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestFavoriteItemLifecycleUsesSharedCategories(t *testing.T) {
	issueID := createIssueForTimeline(t, "Favorite item issue")

	projectW := httptest.NewRecorder()
	testHandler.CreateProject(
		projectW,
		newRequest(http.MethodPost, "/api/projects?workspace_id="+testWorkspaceID, map[string]any{
			"title": "Favorite item project",
		}),
	)
	if projectW.Code != http.StatusCreated {
		t.Fatalf("CreateProject: expected 201, got %d: %s", projectW.Code, projectW.Body.String())
	}
	var project ProjectResponse
	if err := json.NewDecoder(projectW.Body).Decode(&project); err != nil {
		t.Fatalf("decode project: %v", err)
	}
	t.Cleanup(func() {
		w := httptest.NewRecorder()
		req := withURLParam(
			newRequest(http.MethodDelete, "/api/projects/"+project.ID, nil),
			"id",
			project.ID,
		)
		testHandler.DeleteProject(w, req)
	})

	createFavorite := func(itemType, itemID string) FavoriteResponse {
		t.Helper()
		w := httptest.NewRecorder()
		req := withFavoriteURLParams(
			newRequest(http.MethodPut, "/api/favorites/"+itemType+"/"+itemID, nil),
			"itemType", itemType,
			"itemId", itemID,
		)
		testHandler.PutFavorite(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("PutFavorite(%s): expected 200, got %d: %s", itemType, w.Code, w.Body.String())
		}
		var favorite FavoriteResponse
		if err := json.NewDecoder(w.Body).Decode(&favorite); err != nil {
			t.Fatalf("decode %s favorite: %v", itemType, err)
		}
		return favorite
	}

	issueFavorite := createFavorite("issue", issueID)
	projectFavorite := createFavorite("project", project.ID)
	if !issueFavorite.Category.IsDefault || issueFavorite.Category.ID != projectFavorite.Category.ID {
		t.Fatalf("favorites should share the default category: issue=%#v project=%#v", issueFavorite, projectFavorite)
	}

	listW := httptest.NewRecorder()
	testHandler.ListFavorites(listW, newRequest(http.MethodGet, "/api/favorites", nil))
	if listW.Code != http.StatusOK {
		t.Fatalf("ListFavorites: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var favorites []FavoriteResponse
	if err := json.NewDecoder(listW.Body).Decode(&favorites); err != nil {
		t.Fatalf("decode favorites: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("favorites = %#v, want issue and project", favorites)
	}

	categoriesW := httptest.NewRecorder()
	testHandler.ListFavoriteCategories(
		categoriesW,
		newRequest(http.MethodGet, "/api/favorite-categories", nil),
	)
	var categories []FavoriteCategoryResponse
	if err := json.NewDecoder(categoriesW.Body).Decode(&categories); err != nil {
		t.Fatalf("decode categories: %v", err)
	}
	if len(categories) != 1 || categories[0].FavoriteCount != 2 {
		t.Fatalf("categories = %#v, want one shared category with two favorites", categories)
	}

	deleteFavorite := func(itemType, itemID string) {
		t.Helper()
		w := httptest.NewRecorder()
		req := withFavoriteURLParams(
			newRequest(http.MethodDelete, "/api/favorites/"+itemType+"/"+itemID, nil),
			"itemType", itemType,
			"itemId", itemID,
		)
		testHandler.DeleteFavorite(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("DeleteFavorite(%s): expected 204, got %d: %s", itemType, w.Code, w.Body.String())
		}
	}
	deleteFavorite("issue", issueID)
	deleteFavorite("project", project.ID)
}

func TestPutFavoriteRejectsInvalidType(t *testing.T) {
	w := httptest.NewRecorder()
	req := withFavoriteURLParams(
		newRequest(http.MethodPut, "/api/favorites/unknown/"+testWorkspaceID, nil),
		"itemType", "unknown",
		"itemId", testWorkspaceID,
	)
	testHandler.PutFavorite(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PutFavorite: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
