package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type KpiMetricResponse struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Name           string `json:"name"`
	Owner          string `json:"owner"`
	Target         string `json:"target"`
	Current        string `json:"current"`
	Status         string `json:"status"`
	Note           string `json:"note"`
	LinkType       string `json:"link_type"`
	LinkID         string `json:"link_id"`
	CompletionRate int32  `json:"completion_rate"`
	Position       int32  `json:"position"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type kpiMetricRow struct {
	ID             pgtype.UUID
	WorkspaceID    pgtype.UUID
	Name           string
	Owner          string
	Target         string
	Current        string
	Status         string
	Note           string
	LinkType       string
	LinkID         pgtype.UUID
	CompletionRate int32
	Position       int32
	CreatedBy      pgtype.UUID
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

func kpiMetricToResponse(row kpiMetricRow) KpiMetricResponse {
	createdBy := ""
	if row.CreatedBy.Valid {
		createdBy = uuidToString(row.CreatedBy)
	}
	linkID := ""
	if row.LinkID.Valid {
		linkID = uuidToString(row.LinkID)
	}
	return KpiMetricResponse{
		ID:             uuidToString(row.ID),
		WorkspaceID:    uuidToString(row.WorkspaceID),
		Name:           row.Name,
		Owner:          row.Owner,
		Target:         row.Target,
		Current:        row.Current,
		Status:         row.Status,
		Note:           row.Note,
		LinkType:       row.LinkType,
		LinkID:         linkID,
		CompletionRate: row.CompletionRate,
		Position:       row.Position,
		CreatedBy:      createdBy,
		CreatedAt:      timestampToString(row.CreatedAt),
		UpdatedAt:      timestampToString(row.UpdatedAt),
	}
}

func scanKpiMetric(row pgx.Row) (kpiMetricRow, error) {
	var out kpiMetricRow
	err := row.Scan(
		&out.ID,
		&out.WorkspaceID,
		&out.Name,
		&out.Owner,
		&out.Target,
		&out.Current,
		&out.Status,
		&out.Note,
		&out.LinkType,
		&out.LinkID,
		&out.CompletionRate,
		&out.Position,
		&out.CreatedBy,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	return out, err
}

type CreateKpiMetricRequest struct {
	Name           string `json:"name"`
	Owner          string `json:"owner"`
	Target         string `json:"target"`
	Current        string `json:"current"`
	Status         string `json:"status"`
	Note           string `json:"note"`
	LinkType       string `json:"link_type"`
	LinkID         string `json:"link_id"`
	CompletionRate *int32 `json:"completion_rate"`
	Position       *int32 `json:"position"`
}

type UpdateKpiMetricRequest struct {
	Name           *string `json:"name"`
	Owner          *string `json:"owner"`
	Target         *string `json:"target"`
	Current        *string `json:"current"`
	Status         *string `json:"status"`
	Note           *string `json:"note"`
	LinkType       *string `json:"link_type"`
	LinkID         *string `json:"link_id"`
	CompletionRate *int32  `json:"completion_rate"`
	Position       *int32  `json:"position"`
}

func validateKpiStatus(status string) bool {
	switch status {
	case "on_track", "at_risk", "missed", "pending":
		return true
	default:
		return false
	}
}

func validateKpiLinkType(linkType string) bool {
	switch linkType {
	case "", "none", "milestone", "project", "issue":
		return true
	default:
		return false
	}
}

func validateKpiCompletionRate(rate int32) bool {
	return rate >= 0 && rate <= 100
}

func (h *Handler) ListKpiMetrics(w http.ResponseWriter, r *http.Request) {
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
		SELECT id, workspace_id, name, owner, target, current, status, note, link_type, link_id, completion_rate, position, created_by, created_at, updated_at
		FROM kpi_metric
		WHERE workspace_id = $1
		ORDER BY position ASC, created_at ASC
	`, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list kpi metrics")
		return
	}
	defer rows.Close()
	metrics := []KpiMetricResponse{}
	for rows.Next() {
		var row kpiMetricRow
		if err := rows.Scan(
			&row.ID,
			&row.WorkspaceID,
			&row.Name,
			&row.Owner,
			&row.Target,
			&row.Current,
			&row.Status,
			&row.Note,
			&row.LinkType,
			&row.LinkID,
			&row.CompletionRate,
			&row.Position,
			&row.CreatedBy,
			&row.CreatedAt,
			&row.UpdatedAt,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list kpi metrics")
			return
		}
		metrics = append(metrics, kpiMetricToResponse(row))
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list kpi metrics")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics, "total": len(metrics)})
}

func (h *Handler) GetKpiMetric(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "kpi id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	row, err := scanKpiMetric(h.DB.QueryRow(r.Context(), `
		SELECT id, workspace_id, name, owner, target, current, status, note, link_type, link_id, completion_rate, position, created_by, created_at, updated_at
		FROM kpi_metric
		WHERE id = $1 AND workspace_id = $2
	`, idUUID, wsUUID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "kpi metric not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get kpi metric")
		return
	}
	writeJSON(w, http.StatusOK, kpiMetricToResponse(row))
}

func (h *Handler) CreateKpiMetric(w http.ResponseWriter, r *http.Request) {
	var req CreateKpiMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	status := req.Status
	if status == "" {
		status = "pending"
	}
	if !validateKpiStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	linkType := req.LinkType
	if linkType == "" {
		linkType = "none"
	}
	if !validateKpiLinkType(linkType) {
		writeError(w, http.StatusBadRequest, "invalid link_type")
		return
	}
	var linkID pgtype.UUID
	if strings.TrimSpace(req.LinkID) != "" {
		parsed, ok := parseUUIDOrBadRequest(w, req.LinkID, "link_id")
		if !ok {
			return
		}
		linkID = parsed
	} else {
		linkType = "none"
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
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
	var position int32
	var completionRate int32
	if req.Position != nil {
		position = *req.Position
	}
	if req.CompletionRate != nil {
		if !validateKpiCompletionRate(*req.CompletionRate) {
			writeError(w, http.StatusBadRequest, "completion_rate must be between 0 and 100")
			return
		}
		completionRate = *req.CompletionRate
	}
	row, err := scanKpiMetric(h.DB.QueryRow(r.Context(), `
		INSERT INTO kpi_metric (workspace_id, name, owner, target, current, status, note, link_type, link_id, completion_rate, position, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, workspace_id, name, owner, target, current, status, note, link_type, link_id, completion_rate, position, created_by, created_at, updated_at
	`, wsUUID, name, strings.TrimSpace(req.Owner), strings.TrimSpace(req.Target), strings.TrimSpace(req.Current), status, strings.TrimSpace(req.Note), linkType, linkID, completionRate, position, creator))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create kpi metric")
		return
	}
	writeJSON(w, http.StatusCreated, kpiMetricToResponse(row))
}

func (h *Handler) UpdateKpiMetric(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "kpi id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	var req UpdateKpiMetricRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Status != nil && !validateKpiStatus(*req.Status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	if req.LinkType != nil && !validateKpiLinkType(*req.LinkType) {
		writeError(w, http.StatusBadRequest, "invalid link_type")
		return
	}
	if req.CompletionRate != nil && !validateKpiCompletionRate(*req.CompletionRate) {
		writeError(w, http.StatusBadRequest, "completion_rate must be between 0 and 100")
		return
	}
	var name, owner, target, current, status, note pgtype.Text
	var linkType pgtype.Text
	var linkID pgtype.UUID
	var completionRate pgtype.Int4
	var position pgtype.Int4
	if req.Name != nil {
		name = pgtype.Text{String: strings.TrimSpace(*req.Name), Valid: true}
	}
	if req.Owner != nil {
		owner = pgtype.Text{String: strings.TrimSpace(*req.Owner), Valid: true}
	}
	if req.Target != nil {
		target = pgtype.Text{String: strings.TrimSpace(*req.Target), Valid: true}
	}
	if req.Current != nil {
		current = pgtype.Text{String: strings.TrimSpace(*req.Current), Valid: true}
	}
	if req.Status != nil {
		status = pgtype.Text{String: *req.Status, Valid: true}
	}
	if req.Note != nil {
		note = pgtype.Text{String: strings.TrimSpace(*req.Note), Valid: true}
	}
	if req.LinkType != nil {
		linkType = pgtype.Text{String: *req.LinkType, Valid: true}
	}
	if req.LinkID != nil {
		if strings.TrimSpace(*req.LinkID) != "" {
			parsed, ok := parseUUIDOrBadRequest(w, *req.LinkID, "link_id")
			if !ok {
				return
			}
			linkID = parsed
		}
	}
	if req.CompletionRate != nil {
		completionRate = pgtype.Int4{Int32: *req.CompletionRate, Valid: true}
	}
	if req.Position != nil {
		position = pgtype.Int4{Int32: *req.Position, Valid: true}
	}
	row, err := scanKpiMetric(h.DB.QueryRow(r.Context(), `
		UPDATE kpi_metric SET
			name = COALESCE($3, name),
			owner = COALESCE($4, owner),
			target = COALESCE($5, target),
			current = COALESCE($6, current),
			status = COALESCE($7, status),
			note = COALESCE($8, note),
			link_type = COALESCE($9, link_type),
			link_id = CASE WHEN $10::uuid IS NULL THEN link_id ELSE $10 END,
			completion_rate = COALESCE($11, completion_rate),
			position = COALESCE($12, position),
			updated_at = now()
		WHERE id = $1 AND workspace_id = $2
		RETURNING id, workspace_id, name, owner, target, current, status, note, link_type, link_id, completion_rate, position, created_by, created_at, updated_at
	`, idUUID, wsUUID, name, owner, target, current, status, note, linkType, linkID, completionRate, position))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "kpi metric not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update kpi metric")
		return
	}
	writeJSON(w, http.StatusOK, kpiMetricToResponse(row))
}

func (h *Handler) DeleteKpiMetric(w http.ResponseWriter, r *http.Request) {
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "kpi id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace id")
	if !ok {
		return
	}
	tag, err := h.DB.Exec(r.Context(), `DELETE FROM kpi_metric WHERE id = $1 AND workspace_id = $2`, idUUID, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete kpi metric")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "kpi metric not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
