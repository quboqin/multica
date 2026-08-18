package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/capability"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type workspaceCapabilityItem struct {
	Key       string `json:"key"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type workspaceCapabilitiesResponse struct {
	Items     []workspaceCapabilityItem `json:"items"`
	CanManage bool                      `json:"can_manage"`
}

func (h *Handler) ListWorkspaceCapabilities(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "id")
	if _, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id"); !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT capability_key, enabled, updated_at
FROM workspace_capability
WHERE workspace_id = $1::uuid
`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspace capabilities")
		return
	}
	defer rows.Close()

	items := make(map[string]workspaceCapabilityItem)
	for rows.Next() {
		var key string
		var enabled bool
		var updatedAt pgtype.Timestamptz
		if err := rows.Scan(&key, &enabled, &updatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list workspace capabilities")
			return
		}
		items[key] = workspaceCapabilityItem{Key: key, Enabled: enabled, UpdatedAt: timestampToString(updatedAt)}
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspace capabilities")
		return
	}

	definitions := capability.Definitions()
	responseItems := make([]workspaceCapabilityItem, 0, len(definitions))
	for _, definition := range definitions {
		item, ok := items[definition.Key]
		if !ok {
			item = workspaceCapabilityItem{Key: definition.Key, Enabled: definition.DefaultEnable}
		}
		responseItems = append(responseItems, item)
	}
	member, _ := middleware.MemberFromContext(r.Context())
	writeJSON(w, http.StatusOK, workspaceCapabilitiesResponse{
		Items:     responseItems,
		CanManage: member.Role == "owner" || member.Role == "admin",
	})
}

func (h *Handler) UpdateWorkspaceCapability(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "id")
	if _, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id"); !ok {
		return
	}
	key := strings.TrimSpace(chi.URLParam(r, "key"))
	if _, ok := capability.Lookup(key); !ok {
		writeError(w, http.StatusNotFound, "unknown workspace capability")
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "enabled must be a boolean")
		return
	}
	userID, err := util.ParseUUID(requestUserID(r))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not authenticated")
		return
	}
	if key == capability.CreativeFactory && *input.Enabled {
		workspaceUUID, parseErr := util.ParseUUID(workspaceID)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid workspace id")
			return
		}
		if _, initErr := h.initializeCreativeFactory(r.Context(), workspaceUUID, userID); initErr != nil {
			writeError(w, http.StatusUnprocessableEntity, "creative factory initialization failed: "+initErr.Error())
			return
		}
	}
	var enabled bool
	var updatedAt pgtype.Timestamptz
	err = h.DB.QueryRow(r.Context(), `
INSERT INTO workspace_capability (workspace_id, capability_key, enabled, updated_by)
VALUES ($1::uuid, $2, $3, $4)
ON CONFLICT (workspace_id, capability_key) DO UPDATE SET
    enabled = EXCLUDED.enabled,
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING enabled, updated_at
`, workspaceID, key, *input.Enabled, userID).Scan(&enabled, &updatedAt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace capability")
		return
	}
	updatedAtText := timestampToString(updatedAt)
	item := workspaceCapabilityItem{Key: key, Enabled: enabled, UpdatedAt: updatedAtText}
	h.publish(protocol.EventWorkspaceCapabilityUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"capability": item,
	})
	writeJSON(w, http.StatusOK, item)
}
