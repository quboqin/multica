package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type creativeRetrySettings struct {
	AutomaticRetryEnabled bool `json:"automatic_retry_enabled"`
	CanManage             bool `json:"can_manage"`
}

type creativeRetryQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func creativeAutomaticRetryEnabled(ctx context.Context, q creativeRetryQuerier, workspaceID pgtype.UUID) (bool, error) {
	var enabled bool
	err := q.QueryRow(ctx, `SELECT COALESCE((SELECT automatic_retry_enabled FROM creative_factory_settings WHERE workspace_id=$1),true)`, workspaceID).Scan(&enabled)
	return enabled, err
}

func (h *Handler) GetCreativeRetrySettings(w http.ResponseWriter, r *http.Request) {
	workspaceID := middleware.WorkspaceIDFromContext(r.Context())
	enabled, err := creativeAutomaticRetryEnabled(r.Context(), h.DB, parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative retry settings")
		return
	}
	member, _ := middleware.MemberFromContext(r.Context())
	writeJSON(w, http.StatusOK, creativeRetrySettings{enabled, member.Role == "owner" || member.Role == "admin"})
}

func (h *Handler) UpdateCreativeRetrySettings(w http.ResponseWriter, r *http.Request) {
	member, _ := middleware.MemberFromContext(r.Context())
	if member.Role != "owner" && member.Role != "admin" {
		writeError(w, http.StatusForbidden, "workspace owner or admin required")
		return
	}
	var input struct {
		Enabled *bool `json:"automatic_retry_enabled"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil || input.Enabled == nil {
		writeError(w, http.StatusBadRequest, "automatic_retry_enabled must be a boolean")
		return
	}
	workspaceID := middleware.WorkspaceIDFromContext(r.Context())
	if _, err := h.DB.Exec(r.Context(), `INSERT INTO creative_factory_settings(workspace_id,automatic_retry_enabled) VALUES($1,$2)
ON CONFLICT(workspace_id) DO UPDATE SET automatic_retry_enabled=excluded.automatic_retry_enabled,updated_at=now()`, parseUUID(workspaceID), *input.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative retry settings")
		return
	}
	h.publish(protocol.EventCreativeMaterialsUpdated, workspaceID, "member", requestUserID(r), map[string]any{"scope": "settings"})
	writeJSON(w, http.StatusOK, creativeRetrySettings{*input.Enabled, true})
}
