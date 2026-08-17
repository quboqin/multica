package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type FavoriteResponse struct {
	ID          string                   `json:"id"`
	WorkspaceID string                   `json:"workspace_id"`
	UserID      string                   `json:"user_id"`
	ItemType    string                   `json:"item_type"`
	ItemID      string                   `json:"item_id"`
	Attachment  *AttachmentResponse      `json:"attachment,omitempty"`
	Category    FavoriteCategoryResponse `json:"category"`
	CreatedAt   string                   `json:"created_at"`
}

func (h *Handler) favoriteToResponse(
	item db.FavoriteItem,
	category db.FavoriteCategory,
	attachment *db.Attachment,
) FavoriteResponse {
	response := FavoriteResponse{
		ID:          uuidToString(item.ID),
		WorkspaceID: uuidToString(item.WorkspaceID),
		UserID:      uuidToString(item.UserID),
		ItemType:    item.ItemType,
		ItemID:      uuidToString(item.ItemID),
		Category:    favoriteCategoryToResponse(category, 0),
		CreatedAt:   timestampToString(item.CreatedAt),
	}
	if attachment != nil {
		attachmentResponse := h.attachmentToResponse(*attachment)
		response.Attachment = &attachmentResponse
	}
	return response
}

func validateFavoriteType(w http.ResponseWriter, itemType string) bool {
	switch itemType {
	case "attachment", "issue", "project":
		return true
	default:
		writeError(w, http.StatusBadRequest, "item_type must be 'attachment', 'issue', or 'project'")
		return false
	}
}

func favoritePathParams(w http.ResponseWriter, r *http.Request) (string, pgtype.UUID, bool) {
	itemType := chi.URLParam(r, "itemType")
	if !validateFavoriteType(w, itemType) {
		return "", pgtype.UUID{}, false
	}
	itemID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "itemId"), "item id")
	return itemType, itemID, ok
}

func (h *Handler) loadFavoriteSource(
	ctx context.Context,
	itemType string,
	itemID pgtype.UUID,
	workspaceID pgtype.UUID,
) (*db.Attachment, error) {
	switch itemType {
	case "attachment":
		attachment, err := h.Queries.GetAttachment(ctx, db.GetAttachmentParams{
			ID: itemID, WorkspaceID: workspaceID,
		})
		return &attachment, err
	case "issue":
		_, err := h.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{
			ID: itemID, WorkspaceID: workspaceID,
		})
		return nil, err
	case "project":
		_, err := h.Queries.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{
			ID: itemID, WorkspaceID: workspaceID,
		})
		return nil, err
	default:
		return nil, pgx.ErrNoRows
	}
}

func (h *Handler) ListFavorites(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}

	favorites, err := h.Queries.ListFavorites(r.Context(), db.ListFavoritesParams{
		WorkspaceID: workspaceID,
		UserID:      parseUUID(userID),
	})
	if err != nil {
		slog.Error("failed to list favorites", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list favorites")
		return
	}

	response := make([]FavoriteResponse, 0, len(favorites))
	for _, favorite := range favorites {
		var attachment *db.Attachment
		if favorite.FavoriteItem.ItemType == "attachment" {
			loaded, err := h.Queries.GetAttachment(r.Context(), db.GetAttachmentParams{
				ID:          favorite.FavoriteItem.ItemID,
				WorkspaceID: workspaceID,
			})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				slog.Error("failed to load favorite attachment", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to list favorites")
				return
			}
			attachment = &loaded
		}
		response = append(response, h.favoriteToResponse(
			favorite.FavoriteItem,
			favorite.FavoriteCategory,
			attachment,
		))
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *Handler) PutFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	itemType, itemID, ok := favoritePathParams(w, r)
	if !ok {
		return
	}

	attachment, err := h.loadFavoriteSource(r.Context(), itemType, itemID, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, itemType+" not found")
		return
	}
	if err != nil {
		slog.Error("failed to load favorite source", "item_type", itemType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load favorite source")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start transaction")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	qtx := h.Queries.WithTx(tx)

	defaultCategory, err := qtx.GetOrCreateDefaultFavoriteCategory(
		r.Context(),
		db.GetOrCreateDefaultFavoriteCategoryParams{
			WorkspaceID: workspaceID,
			UserID:      parseUUID(userID),
		},
	)
	if err != nil {
		slog.Error("failed to get default favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create default favorite category")
		return
	}

	favorite, err := qtx.PutFavorite(r.Context(), db.PutFavoriteParams{
		WorkspaceID: workspaceID,
		UserID:      parseUUID(userID),
		ItemType:    itemType,
		ItemID:      itemID,
		CategoryID:  defaultCategory.ID,
	})
	if err != nil {
		slog.Error("failed to save favorite", "item_type", itemType, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to save favorite")
		return
	}

	category, err := qtx.GetFavoriteCategoryForUser(
		r.Context(),
		db.GetFavoriteCategoryForUserParams{
			ID:          favorite.CategoryID,
			WorkspaceID: workspaceID,
			UserID:      parseUUID(userID),
		},
	)
	if err != nil {
		slog.Error("failed to load favorite category", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load favorite category")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save favorite")
		return
	}
	writeJSON(w, http.StatusOK, h.favoriteToResponse(favorite, category, attachment))
}

func (h *Handler) MoveFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	itemType, itemID, ok := favoritePathParams(w, r)
	if !ok {
		return
	}

	var request moveFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	categoryID, ok := parseUUIDOrBadRequest(w, request.CategoryID, "category_id")
	if !ok {
		return
	}
	category, err := h.Queries.GetFavoriteCategoryForUser(
		r.Context(),
		db.GetFavoriteCategoryForUserParams{
			ID: categoryID, WorkspaceID: workspaceID, UserID: parseUUID(userID),
		},
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "favorite category not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load favorite category")
		return
	}

	favorite, err := h.Queries.MoveFavorite(r.Context(), db.MoveFavoriteParams{
		WorkspaceID: workspaceID,
		UserID:      parseUUID(userID),
		ItemType:    itemType,
		ItemID:      itemID,
		CategoryID:  category.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "favorite not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to move favorite")
		return
	}

	var attachment *db.Attachment
	if itemType == "attachment" {
		loaded, err := h.Queries.GetAttachment(r.Context(), db.GetAttachmentParams{
			ID: itemID, WorkspaceID: workspaceID,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load favorite attachment")
			return
		}
		attachment = &loaded
	}
	writeJSON(w, http.StatusOK, h.favoriteToResponse(favorite, category, attachment))
}

func (h *Handler) DeleteFavorite(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	itemType, itemID, ok := favoritePathParams(w, r)
	if !ok {
		return
	}

	if err := h.Queries.DeleteFavorite(r.Context(), db.DeleteFavoriteParams{
		WorkspaceID: workspaceID,
		UserID:      parseUUID(userID),
		ItemType:    itemType,
		ItemID:      itemID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete favorite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
