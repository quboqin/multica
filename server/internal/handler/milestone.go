package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type MilestoneResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
	Status      string  `json:"status"`
	Position    int32   `json:"position"`
	CreatedBy   string  `json:"created_by"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

func milestoneToResponse(m db.Milestone) MilestoneResponse {
	return MilestoneResponse{
		ID:          uuidToString(m.ID),
		WorkspaceID: uuidToString(m.WorkspaceID),
		Title:       m.Title,
		Description: m.Description,
		StartDate:   dateToPtr(m.StartDate),
		EndDate:     dateToPtr(m.EndDate),
		Status:      m.Status,
		Position:    m.Position,
		CreatedBy:   uuidToString(m.CreatedBy),
		CreatedAt:   timestampToString(m.CreatedAt),
		UpdatedAt:   timestampToString(m.UpdatedAt),
	}
}

type CreateMilestoneRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
	Status      string  `json:"status"`
	Position    *int32  `json:"position"`
}

type UpdateMilestoneRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	StartDate   *string `json:"start_date"`
	EndDate     *string `json:"end_date"`
	Status      *string `json:"status"`
	Position    *int32  `json:"position"`
}

func parseOptionalDate(w http.ResponseWriter, value *string, fieldName string) (pgtype.Date, bool) {
	if value == nil || *value == "" {
		return pgtype.Date{Valid: false}, true
	}
	d, err := util.ParseCalendarDate(*value)
	if err != nil {
		writeError(w, http.StatusBadRequest, fieldName+" must be YYYY-MM-DD")
		return pgtype.Date{}, false
	}
	return d, true
}

func validateMilestoneStatus(status string) bool {
	switch status {
	case "planned", "in_progress", "paused", "completed", "cancelled":
		return true
	default:
		return false
	}
}

func (h *Handler) ListMilestones(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var statusFilter pgtype.Text
	if s := r.URL.Query().Get("status"); s != "" {
		if !validateMilestoneStatus(s) {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		statusFilter = pgtype.Text{String: s, Valid: true}
	}
	rows, err := h.Queries.ListMilestones(r.Context(), db.ListMilestonesParams{
		WorkspaceID: wsUUID,
		Status:      statusFilter,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list milestones")
		return
	}
	resp := make([]MilestoneResponse, len(rows))
	for i, row := range rows {
		resp[i] = milestoneToResponse(row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"milestones": resp, "total": len(resp)})
}

func (h *Handler) GetMilestone(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "milestone id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	row, err := h.Queries.GetMilestoneInWorkspace(r.Context(), db.GetMilestoneInWorkspaceParams{
		ID:          idUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "milestone not found")
		return
	}
	writeJSON(w, http.StatusOK, milestoneToResponse(row))
}

func (h *Handler) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	var req CreateMilestoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeError(w, http.StatusBadRequest, "title is required")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	creator, ok := parseUUIDOrBadRequest(w, userID, "user_id")
	if !ok {
		return
	}
	status := req.Status
	if status == "" {
		status = "planned"
	}
	if !validateMilestoneStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	startDate, ok := parseOptionalDate(w, req.StartDate, "start_date")
	if !ok {
		return
	}
	endDate, ok := parseOptionalDate(w, req.EndDate, "end_date")
	if !ok {
		return
	}
	var position int32
	if req.Position != nil {
		position = *req.Position
	}
	row, err := h.Queries.CreateMilestone(r.Context(), db.CreateMilestoneParams{
		WorkspaceID: wsUUID,
		Title:       req.Title,
		Description: description,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      status,
		Position:    position,
		CreatedBy:   creator,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create milestone")
		return
	}
	writeJSON(w, http.StatusCreated, milestoneToResponse(row))
}

func (h *Handler) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "milestone id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	prev, err := h.Queries.GetMilestoneInWorkspace(r.Context(), db.GetMilestoneInWorkspaceParams{
		ID:          idUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "milestone not found")
		return
	}
	var req UpdateMilestoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	params := db.UpdateMilestoneParams{
		ID:          prev.ID,
		WorkspaceID: wsUUID,
		StartDate:   prev.StartDate,
		EndDate:     prev.EndDate,
		Description: pgtype.Text{String: prev.Description, Valid: true},
	}
	if req.Title != nil {
		params.Title = pgtype.Text{String: *req.Title, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Status != nil {
		if !validateMilestoneStatus(*req.Status) {
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		params.Status = pgtype.Text{String: *req.Status, Valid: true}
	}
	if req.Position != nil {
		params.Position = pgtype.Int4{Int32: *req.Position, Valid: true}
	}
	if req.StartDate != nil {
		startDate, ok := parseOptionalDate(w, req.StartDate, "start_date")
		if !ok {
			return
		}
		params.StartDate = startDate
	}
	if req.EndDate != nil {
		endDate, ok := parseOptionalDate(w, req.EndDate, "end_date")
		if !ok {
			return
		}
		params.EndDate = endDate
	}
	row, err := h.Queries.UpdateMilestone(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update milestone")
		return
	}
	writeJSON(w, http.StatusOK, milestoneToResponse(row))
}

func (h *Handler) DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "milestone id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	if _, err := h.Queries.GetMilestoneInWorkspace(r.Context(), db.GetMilestoneInWorkspaceParams{
		ID:          idUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusNotFound, "milestone not found")
		return
	}
	if err := h.Queries.DeleteMilestone(r.Context(), db.DeleteMilestoneParams{
		ID:          idUUID,
		WorkspaceID: wsUUID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete milestone")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
