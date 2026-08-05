package handler

import (
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/storage"
)

func (h *Handler) creativeMaterialArchiveResponseURL(candidateID, rawURL string) string {
	if candidateID == "" || rawURL == "" || h.Storage == nil || h.Storage.CdnDomain() != "" {
		return rawURL
	}
	if h.Storage.KeyFromURL(rawURL) == "" {
		return rawURL
	}
	return "/api/creative/materials/" + url.PathEscape(candidateID) + "/archive"
}

// DownloadCreativeMaterialArchive gives private object-store archives a
// durable browser URL even when old candidates do not have attachment rows.
// The candidate row supplies workspace ownership, so native image/video loads
// need authentication but do not need workspace headers.
func (h *Handler) DownloadCreativeMaterialArchive(w http.ResponseWriter, r *http.Request) {
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "candidate id")
	if !ok {
		return
	}
	var workspaceID pgtype.UUID
	var archivedURL, assetType string
	if err := h.DB.QueryRow(r.Context(), `
SELECT workspace_id, archived_url, asset_type
FROM creative_material_candidate
WHERE id = $1 AND archived_url <> ''
`, candidateID).Scan(&workspaceID, &archivedURL, &assetType); err != nil {
		writeError(w, http.StatusNotFound, "creative material archive not found")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	workspaceIDText := uuidToString(workspaceID)
	if !h.MembershipCache.Get(r.Context(), userID, workspaceIDText) {
		if _, err := h.getWorkspaceMember(r.Context(), userID, workspaceIDText); err != nil {
			writeError(w, http.StatusNotFound, "creative material archive not found")
			return
		}
		h.MembershipCache.Set(r.Context(), userID, workspaceIDText)
	}
	if h.Storage == nil {
		writeError(w, http.StatusServiceUnavailable, "storage not configured")
		return
	}
	key := h.Storage.KeyFromURL(archivedURL)
	if key == "" {
		writeError(w, http.StatusNotFound, "creative material archive not found")
		return
	}
	reader, err := h.Storage.GetReader(r.Context(), key)
	if err != nil {
		writeError(w, http.StatusNotFound, "creative material archive not found")
		return
	}
	defer reader.Close()
	contentType := creativeArchiveContentType(archivedURL, assetType)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", storage.ContentDisposition(contentType, filepath.Base(key)))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = io.Copy(w, reader)
}

func creativeArchiveContentType(rawURL, assetType string) string {
	if parsed, err := url.Parse(rawURL); err == nil {
		if value := mime.TypeByExtension(filepath.Ext(parsed.Path)); value != "" {
			return value
		}
	}
	if strings.EqualFold(assetType, "video") {
		return "video/mp4"
	}
	return "image/jpeg"
}
