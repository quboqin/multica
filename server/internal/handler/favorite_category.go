package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const favoriteCategoryNameMaxLength = 80

type FavoriteCategoryResponse struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	IsDefault     bool   `json:"is_default"`
	FavoriteCount int64  `json:"favorite_count"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type createFavoriteCategoryRequest struct {
	Name string `json:"name"`
}

type moveFavoriteRequest struct {
	CategoryID string `json:"category_id"`
}

func favoriteCategoryToResponse(
	category db.FavoriteCategory,
	favoriteCount int64,
) FavoriteCategoryResponse {
	return FavoriteCategoryResponse{
		ID:            uuidToString(category.ID),
		WorkspaceID:   uuidToString(category.WorkspaceID),
		UserID:        uuidToString(category.UserID),
		Name:          category.Name,
		IsDefault:     category.IsDefault,
		FavoriteCount: favoriteCount,
		CreatedAt:     timestampToString(category.CreatedAt),
		UpdatedAt:     timestampToString(category.UpdatedAt),
	}
}

func (h *Handler) ListFavoriteCategories(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}

	categories, err := h.Queries.ListFavoriteCategories(
		r.Context(),
		db.ListFavoriteCategoriesParams{
			UserID:      parseUUID(userID),
			WorkspaceID: wsUUID,
		},
	)
	if err != nil {
		slog.Error("failed to list favorite categories", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list favorite categories")
		return
	}

	resp := make([]FavoriteCategoryResponse, len(categories))
	for i, category := range categories {
		resp[i] = favoriteCategoryToResponse(
			category.FavoriteCategory,
			category.FavoriteCount,
		)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateFavoriteCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}

	var req createFavoriteCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if utf8.RuneCountInString(req.Name) > favoriteCategoryNameMaxLength {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}
	if strings.EqualFold(req.Name, "Default") {
		writeError(w, http.StatusConflict, "category name already exists")
		return
	}

	category, err := h.Queries.CreateFavoriteCategory(
		r.Context(),
		db.CreateFavoriteCategoryParams{
			WorkspaceID: wsUUID,
			UserID:      parseUUID(userID),
			Name:        req.Name,
		},
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "category name already exists")
			return
		}
		slog.Error("failed to create favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create favorite category")
		return
	}

	writeJSON(w, http.StatusCreated, favoriteCategoryToResponse(category, 0))
}

func (h *Handler) UpdateFavoriteCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	categoryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "category id")
	if !ok {
		return
	}

	category, err := h.Queries.GetFavoriteCategoryForUser(
		r.Context(),
		db.GetFavoriteCategoryForUserParams{
			ID:          categoryID,
			WorkspaceID: wsUUID,
			UserID:      parseUUID(userID),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "favorite category not found")
		return
	}
	if err != nil {
		slog.Error("failed to load favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load favorite category")
		return
	}
	if category.IsDefault {
		writeError(w, http.StatusConflict, "default favorite category cannot be renamed")
		return
	}

	var req createFavoriteCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if utf8.RuneCountInString(req.Name) > favoriteCategoryNameMaxLength {
		writeError(w, http.StatusBadRequest, "name is too long")
		return
	}
	if strings.EqualFold(req.Name, "Default") && !category.IsDefault {
		writeError(w, http.StatusConflict, "category name already exists")
		return
	}

	updated, err := h.Queries.UpdateFavoriteCategory(
		r.Context(),
		db.UpdateFavoriteCategoryParams{
			ID:          category.ID,
			WorkspaceID: category.WorkspaceID,
			UserID:      category.UserID,
			Name:        req.Name,
		},
	)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "category name already exists")
			return
		}
		slog.Error("failed to update favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update favorite category")
		return
	}
	favoriteCount, err := h.Queries.CountFavoritesByCategory(
		r.Context(),
		db.CountFavoritesByCategoryParams{
			CategoryID: updated.ID,
			UserID:     updated.UserID,
		},
	)
	if err != nil {
		slog.Error("failed to count favorites by category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update favorite category")
		return
	}

	writeJSON(w, http.StatusOK, favoriteCategoryToResponse(updated, favoriteCount))
}

func (h *Handler) DeleteFavoriteCategory(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	categoryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "category id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	qtx := h.Queries.WithTx(tx)

	category, err := qtx.GetFavoriteCategoryForUser(
		r.Context(),
		db.GetFavoriteCategoryForUserParams{
			ID:          categoryID,
			WorkspaceID: wsUUID,
			UserID:      parseUUID(userID),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "favorite category not found")
		return
	}
	if err != nil {
		slog.Error("failed to load favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load favorite category")
		return
	}
	if category.IsDefault {
		writeError(w, http.StatusConflict, "default favorite category cannot be deleted")
		return
	}

	if _, err := qtx.DeleteFavoriteCategory(
		r.Context(),
		db.DeleteFavoriteCategoryParams{
			ID:          category.ID,
			WorkspaceID: category.WorkspaceID,
			UserID:      category.UserID,
		},
	); err != nil {
		slog.Error("failed to delete favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete favorite category")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Error("failed to commit favorite category deletion", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete favorite category")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
