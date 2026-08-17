package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFavoriteCategoryLifecycle(t *testing.T) {
	attachmentID := seedAttachmentURL(
		t,
		"https://cdn.example.com/favorite-report.md",
		"favorite-report.md",
		"text/markdown",
		128,
	)

	listCategories := func() []FavoriteCategoryResponse {
		t.Helper()
		w := httptest.NewRecorder()
		testHandler.ListFavoriteCategories(
			w,
			newRequest(http.MethodGet, "/api/favorite-categories", nil),
		)
		if w.Code != http.StatusOK {
			t.Fatalf("ListFavoriteCategories: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var categories []FavoriteCategoryResponse
		if err := json.NewDecoder(w.Body).Decode(&categories); err != nil {
			t.Fatalf("decode categories: %v", err)
		}
		return categories
	}

	if categories := listCategories(); len(categories) > 1 ||
		(len(categories) == 1 &&
			(!categories[0].IsDefault || categories[0].FavoriteCount != 0)) {
		t.Fatalf("categories before first favorite = %#v, want no custom or non-empty categories", categories)
	}

	favoriteW := httptest.NewRecorder()
	favoriteReq := withFavoriteURLParams(
		newRequest(http.MethodPut, "/api/favorites/attachment/"+attachmentID, nil),
		"itemType", "attachment",
		"itemId", attachmentID,
	)
	testHandler.PutFavorite(favoriteW, favoriteReq)
	if favoriteW.Code != http.StatusOK {
		t.Fatalf("PutFavorite: expected 200, got %d: %s", favoriteW.Code, favoriteW.Body.String())
	}
	var favorite FavoriteResponse
	if err := json.NewDecoder(favoriteW.Body).Decode(&favorite); err != nil {
		t.Fatalf("decode favorite: %v", err)
	}
	if favorite.ItemType != "attachment" || favorite.ItemID != attachmentID || favorite.Attachment == nil || favorite.Attachment.ID != attachmentID || !favorite.Category.IsDefault {
		t.Fatalf("favorite = %#v, want attachment in default category", favorite)
	}

	categories := listCategories()
	if len(categories) != 1 || !categories[0].IsDefault || categories[0].FavoriteCount != 1 {
		t.Fatalf("categories after first favorite = %#v, want one default category with one document", categories)
	}

	createW := httptest.NewRecorder()
	testHandler.CreateFavoriteCategory(
		createW,
		newRequest(http.MethodPost, "/api/favorite-categories", map[string]any{"name": "Research"}),
	)
	if createW.Code != http.StatusCreated {
		t.Fatalf("CreateFavoriteCategory: expected 201, got %d: %s", createW.Code, createW.Body.String())
	}
	var research FavoriteCategoryResponse
	if err := json.NewDecoder(createW.Body).Decode(&research); err != nil {
		t.Fatalf("decode category: %v", err)
	}
	if research.Name != "Research" || research.IsDefault {
		t.Fatalf("created category = %#v", research)
	}
	categories = listCategories()
	if len(categories) != 2 || !categories[0].IsDefault || categories[1].IsDefault {
		t.Fatalf("categories after custom category creation = %#v, want default category first", categories)
	}

	renameW := httptest.NewRecorder()
	renameReq := withURLParam(
		newRequest(http.MethodPatch, "/api/favorite-categories/"+research.ID, map[string]any{"name": "Sources"}),
		"id",
		research.ID,
	)
	testHandler.UpdateFavoriteCategory(renameW, renameReq)
	if renameW.Code != http.StatusOK {
		t.Fatalf("UpdateFavoriteCategory: expected 200, got %d: %s", renameW.Code, renameW.Body.String())
	}
	if err := json.NewDecoder(renameW.Body).Decode(&research); err != nil {
		t.Fatalf("decode renamed category: %v", err)
	}
	if research.Name != "Sources" {
		t.Fatalf("renamed category = %#v, want Sources", research)
	}

	moveW := httptest.NewRecorder()
	moveReq := withFavoriteURLParams(
		newRequest(http.MethodPatch, "/api/favorites/attachment/"+attachmentID, map[string]any{"category_id": research.ID}),
		"itemType", "attachment",
		"itemId", attachmentID,
	)
	testHandler.MoveFavorite(moveW, moveReq)
	if moveW.Code != http.StatusOK {
		t.Fatalf("MoveFavorite: expected 200, got %d: %s", moveW.Code, moveW.Body.String())
	}
	var moved FavoriteResponse
	if err := json.NewDecoder(moveW.Body).Decode(&moved); err != nil {
		t.Fatalf("decode moved favorite: %v", err)
	}
	if moved.Category.ID != research.ID {
		t.Fatalf("moved category = %s, want %s", moved.Category.ID, research.ID)
	}

	// Retrying the idempotent favorite request must preserve a category the
	// user already selected instead of moving the document back to default.
	retryW := httptest.NewRecorder()
	retryReq := withFavoriteURLParams(
		newRequest(http.MethodPut, "/api/favorites/attachment/"+attachmentID, nil),
		"itemType", "attachment",
		"itemId", attachmentID,
	)
	testHandler.PutFavorite(retryW, retryReq)
	if retryW.Code != http.StatusOK {
		t.Fatalf("PutFavorite retry: expected 200, got %d: %s", retryW.Code, retryW.Body.String())
	}
	var retried FavoriteResponse
	if err := json.NewDecoder(retryW.Body).Decode(&retried); err != nil {
		t.Fatalf("decode retried favorite: %v", err)
	}
	if retried.Category.ID != research.ID {
		t.Fatalf("retry moved favorite to %s, want preserved category %s", retried.Category.ID, research.ID)
	}

	listW := httptest.NewRecorder()
	testHandler.ListFavorites(
		listW,
		newRequest(http.MethodGet, "/api/favorites", nil),
	)
	if listW.Code != http.StatusOK {
		t.Fatalf("ListFavorites: expected 200, got %d: %s", listW.Code, listW.Body.String())
	}
	var favorites []FavoriteResponse
	if err := json.NewDecoder(listW.Body).Decode(&favorites); err != nil {
		t.Fatalf("decode favorites: %v", err)
	}
	if len(favorites) != 1 || favorites[0].Category.ID != research.ID {
		t.Fatalf("favorites = %#v, want attachment in Research", favorites)
	}

	deleteCategoryW := httptest.NewRecorder()
	deleteCategoryReq := withURLParam(
		newRequest(http.MethodDelete, "/api/favorite-categories/"+research.ID, nil),
		"id",
		research.ID,
	)
	testHandler.DeleteFavoriteCategory(deleteCategoryW, deleteCategoryReq)
	if deleteCategoryW.Code != http.StatusNoContent {
		t.Fatalf("DeleteFavoriteCategory: expected 204, got %d: %s", deleteCategoryW.Code, deleteCategoryW.Body.String())
	}

	listAfterDeleteW := httptest.NewRecorder()
	testHandler.ListFavorites(
		listAfterDeleteW,
		newRequest(http.MethodGet, "/api/favorites", nil),
	)
	var favoritesAfterDelete []FavoriteResponse
	if err := json.NewDecoder(listAfterDeleteW.Body).Decode(&favoritesAfterDelete); err != nil {
		t.Fatalf("decode favorites after category delete: %v", err)
	}
	if len(favoritesAfterDelete) != 0 {
		t.Fatalf("favorites after category delete = %#v, want empty", favoritesAfterDelete)
	}

	categories = listCategories()
	if len(categories) != 1 || !categories[0].IsDefault || categories[0].FavoriteCount != 0 {
		t.Fatalf("categories after custom category delete = %#v, want empty default category", categories)
	}

	renameDefaultW := httptest.NewRecorder()
	renameDefaultReq := withURLParam(
		newRequest(http.MethodPatch, "/api/favorite-categories/"+categories[0].ID, map[string]any{"name": "Renamed"}),
		"id",
		categories[0].ID,
	)
	testHandler.UpdateFavoriteCategory(renameDefaultW, renameDefaultReq)
	if renameDefaultW.Code != http.StatusConflict {
		t.Fatalf("UpdateFavoriteCategory default: expected 409, got %d: %s", renameDefaultW.Code, renameDefaultW.Body.String())
	}

	deleteDefaultW := httptest.NewRecorder()
	deleteDefaultReq := withURLParam(
		newRequest(http.MethodDelete, "/api/favorite-categories/"+categories[0].ID, nil),
		"id",
		categories[0].ID,
	)
	testHandler.DeleteFavoriteCategory(deleteDefaultW, deleteDefaultReq)
	if deleteDefaultW.Code != http.StatusConflict {
		t.Fatalf("DeleteFavoriteCategory default: expected 409, got %d: %s", deleteDefaultW.Code, deleteDefaultW.Body.String())
	}
	if categories := listCategories(); len(categories) != 1 || !categories[0].IsDefault || categories[0].Name != "Default" {
		t.Fatalf("categories after rejected default mutations = %#v, want unchanged default", categories)
	}

	deleteFavoriteW := httptest.NewRecorder()
	deleteFavoriteReq := withFavoriteURLParams(
		newRequest(http.MethodDelete, "/api/favorites/attachment/"+attachmentID, nil),
		"itemType", "attachment",
		"itemId", attachmentID,
	)
	testHandler.DeleteFavorite(deleteFavoriteW, deleteFavoriteReq)
	if deleteFavoriteW.Code != http.StatusNoContent {
		t.Fatalf("DeleteFavorite: expected 204, got %d: %s", deleteFavoriteW.Code, deleteFavoriteW.Body.String())
	}
}

func TestCreateFavoriteCategoryValidatesName(t *testing.T) {
	w := httptest.NewRecorder()
	testHandler.CreateFavoriteCategory(
		w,
		newRequest(http.MethodPost, "/api/favorite-categories", map[string]any{"name": "   "}),
	)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CreateFavoriteCategory: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestPutFavoriteRejectsMalformedID(t *testing.T) {
	w := httptest.NewRecorder()
	req := withFavoriteURLParams(
		newRequest(http.MethodPut, "/api/favorites/attachment/not-a-uuid", nil),
		"itemType", "attachment",
		"itemId", "not-a-uuid",
	)
	testHandler.PutFavorite(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("PutFavorite: expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
