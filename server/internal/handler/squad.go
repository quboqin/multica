package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ── Response types ──────────────────────────────────────────────────────────

type SquadResponse struct {
	ID            string                       `json:"id"`
	WorkspaceID   string                       `json:"workspace_id"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description"`
	Instructions  string                       `json:"instructions"`
	AvatarURL     *string                      `json:"avatar_url"`
	LeaderID      string                       `json:"leader_id"`
	CreatorID     string                       `json:"creator_id"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	ArchivedAt    *string                      `json:"archived_at"`
	ArchivedBy    *string                      `json:"archived_by"`
	MemberCount   int                          `json:"member_count"`
	MemberPreview []SquadMemberPreviewResponse `json:"member_preview"`
}

type SquadMemberPreviewResponse struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
	Role       string `json:"role"`
}

type squadMemberSummary struct {
	count   int
	preview []SquadMemberPreviewResponse
}

type SquadMemberResponse struct {
	ID         string `json:"id"`
	SquadID    string `json:"squad_id"`
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
	Role       string `json:"role"`
	CreatedAt  string `json:"created_at"`
}

// ── Converters ──────────────────────────────────────────────────────────────

func squadToResponse(s db.Squad) SquadResponse {
	return SquadResponse{
		ID:            uuidToString(s.ID),
		WorkspaceID:   uuidToString(s.WorkspaceID),
		Name:          s.Name,
		Description:   s.Description,
		Instructions:  s.Instructions,
		AvatarURL:     textToPtr(s.AvatarUrl),
		LeaderID:      uuidToString(s.LeaderID),
		CreatorID:     uuidToString(s.CreatorID),
		CreatedAt:     timestampToString(s.CreatedAt),
		UpdatedAt:     timestampToString(s.UpdatedAt),
		ArchivedAt:    timestampToPtr(s.ArchivedAt),
		ArchivedBy:    uuidToPtr(s.ArchivedBy),
		MemberPreview: []SquadMemberPreviewResponse{},
	}
}

func squadMemberToResponse(m db.SquadMember) SquadMemberResponse {
	return SquadMemberResponse{
		ID:         uuidToString(m.ID),
		SquadID:    uuidToString(m.SquadID),
		MemberType: m.MemberType,
		MemberID:   uuidToString(m.MemberID),
		Role:       m.Role,
		CreatedAt:  timestampToString(m.CreatedAt),
	}
}

func addSquadMemberPreview(summary *squadMemberSummary, memberType string, memberID pgtype.UUID, role string) {
	summary.count++
	if len(summary.preview) >= 3 {
		return
	}
	summary.preview = append(summary.preview, SquadMemberPreviewResponse{
		MemberType: memberType,
		MemberID:   uuidToString(memberID),
		Role:       role,
	})
}

func applySquadMemberSummary(resp *SquadResponse, summary *squadMemberSummary) {
	if summary == nil {
		return
	}
	resp.MemberCount = summary.count
	resp.MemberPreview = summary.preview
}

// ── Helpers ─────────────────────────────────────────────────────────────────

// loadSquadInWorkspace loads a squad scoped to the current workspace.
func (h *Handler) loadSquadInWorkspace(w http.ResponseWriter, r *http.Request) (db.Squad, string, bool) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	squadID := chi.URLParam(r, "id")
	squadUUID, ok := parseUUIDOrBadRequest(w, squadID, "squad id")
	if !ok {
		return db.Squad{}, "", false
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return db.Squad{}, "", false
	}
	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{
		ID:          squadUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "squad not found")
		return db.Squad{}, "", false
	}
	return squad, workspaceID, true
}

func (h *Handler) loadSquadMemberSummary(ctx context.Context, squadID pgtype.UUID) (*squadMemberSummary, error) {
	rows, err := h.Queries.ListSquadMemberPreviewRowsBySquad(ctx, squadID)
	if err != nil {
		return nil, err
	}
	summary := &squadMemberSummary{}
	for _, row := range rows {
		addSquadMemberPreview(summary, row.MemberType, row.MemberID, row.Role)
	}
	return summary, nil
}

func (h *Handler) squadToResponseWithPreview(ctx context.Context, squad db.Squad) (SquadResponse, error) {
	resp := squadToResponse(squad)
	summary, err := h.loadSquadMemberSummary(ctx, squad.ID)
	if err != nil {
		return resp, err
	}
	applySquadMemberSummary(&resp, summary)
	return resp, nil
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (h *Handler) ListSquads(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	squads, err := h.Queries.ListSquads(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list squads")
		return
	}

	previewRows, err := h.Queries.ListSquadMemberPreviewRows(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list squad member preview")
		return
	}
	summaries := make(map[string]*squadMemberSummary, len(squads))
	for _, row := range previewRows {
		squadID := uuidToString(row.SquadID)
		summary := summaries[squadID]
		if summary == nil {
			summary = &squadMemberSummary{}
			summaries[squadID] = summary
		}
		addSquadMemberPreview(summary, row.MemberType, row.MemberID, row.Role)
	}

	resp := make([]SquadResponse, len(squads))
	for i, s := range squads {
		resp[i] = squadToResponse(s)
		applySquadMemberSummary(&resp[i], summaries[uuidToString(s.ID)])
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateSquad(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}

	var req struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		LeaderID    string  `json:"leader_id"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.LeaderID == "" {
		writeError(w, http.StatusBadRequest, "leader_id is required")
		return
	}

	leaderUUID, ok := parseUUIDOrBadRequest(w, req.LeaderID, "leader_id")
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	// Validate leader is an agent in this workspace.
	_, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          leaderUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "leader must be a valid agent in this workspace")
		return
	}

	avatarURL := pgtype.Text{}
	if req.AvatarURL != nil {
		avatarURL = pgtype.Text{String: *req.AvatarURL, Valid: true}
	}

	squad, err := h.Queries.CreateSquad(r.Context(), db.CreateSquadParams{
		WorkspaceID: wsUUID,
		Name:        req.Name,
		Description: req.Description,
		LeaderID:    leaderUUID,
		CreatorID:   member.UserID,
		AvatarUrl:   avatarURL,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create squad")
		return
	}

	// Auto-add leader as a member with role "leader".
	h.Queries.AddSquadMember(r.Context(), db.AddSquadMemberParams{
		SquadID:    squad.ID,
		MemberType: "agent",
		MemberID:   leaderUUID,
		Role:       "leader",
	})

	resp, err := h.squadToResponseWithPreview(r.Context(), squad)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad member preview")
		return
	}
	h.publish(protocol.EventSquadCreated, workspaceID, "member", uuidToString(member.UserID), map[string]any{"squad": resp})
	obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.SquadCreated(
		uuidToString(member.UserID),
		workspaceID,
		uuidToString(squad.ID),
		1,
	))
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) GetSquad(w http.ResponseWriter, r *http.Request) {
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	resp, err := h.squadToResponseWithPreview(r.Context(), squad)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad member preview")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) UpdateSquad(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	var req struct {
		Name         *string `json:"name"`
		Description  *string `json:"description"`
		Instructions *string `json:"instructions"`
		LeaderID     *string `json:"leader_id"`
		AvatarURL    *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateSquadParams{ID: squad.ID}
	if req.Name != nil {
		params.Name = pgtype.Text{String: *req.Name, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Instructions != nil {
		params.Instructions = pgtype.Text{String: *req.Instructions, Valid: true}
	}
	if req.AvatarURL != nil {
		params.AvatarUrl = pgtype.Text{String: *req.AvatarURL, Valid: true}
	}
	if req.LeaderID != nil {
		lid, ok := parseUUIDOrBadRequest(w, *req.LeaderID, "leader_id")
		if !ok {
			return
		}
		// Validate new leader is an agent in workspace.
		if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
			ID: lid, WorkspaceID: wsUUID,
		}); err != nil {
			writeError(w, http.StatusBadRequest, "leader must be a valid agent in this workspace")
			return
		}
		// Ensure new leader is a squad member; auto-add if not.
		isMember, _ := h.Queries.IsSquadMember(r.Context(), db.IsSquadMemberParams{
			SquadID: squad.ID, MemberType: "agent", MemberID: lid,
		})
		if !isMember {
			h.Queries.AddSquadMember(r.Context(), db.AddSquadMemberParams{
				SquadID: squad.ID, MemberType: "agent", MemberID: lid, Role: "leader",
			})
		}
		params.LeaderID = lid
	}

	updated, err := h.Queries.UpdateSquad(r.Context(), params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update squad")
		return
	}

	resp, err := h.squadToResponseWithPreview(r.Context(), updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad member preview")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{"squad": resp})
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteSquad(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}

	if squad.ArchivedAt.Valid {
		writeError(w, http.StatusBadRequest, "squad is already archived")
		return
	}

	// Transfer issues assigned to this squad to the leader agent.
	if err := h.Queries.TransferSquadAssignees(r.Context(), db.TransferSquadAssigneesParams{
		AssigneeID:   squad.ID,
		AssigneeID_2: squad.LeaderID,
	}); err != nil {
		slog.Warn("transfer squad assignees failed", "squad_id", uuidToString(squad.ID), "error", err)
	}

	// Mirror the issue-assignee transfer for autopilots that target this
	// squad. Without this, autopilot.assignee_id would still point at the
	// archived squad row and every subsequent dispatch would skip with
	// "assignee squad is archived" — visible to ops but useless to the
	// owner. Rewriting to the leader keeps the autopilot semantics
	// unchanged (Path A from MUL-2429 is leader-only execution anyway).
	if err := h.Queries.TransferSquadAutopilotsToLeader(r.Context(), db.TransferSquadAutopilotsToLeaderParams{
		AssigneeID:   squad.ID,
		AssigneeID_2: squad.LeaderID,
	}); err != nil {
		slog.Warn("transfer squad autopilots failed", "squad_id", uuidToString(squad.ID), "error", err)
	}

	userID := requestUserID(r)
	userUUID, _ := parseUUIDOrBadRequest(w, userID, "user_id")

	if _, err := h.Queries.ArchiveSquad(r.Context(), db.ArchiveSquadParams{
		ID:         squad.ID,
		ArchivedBy: userUUID,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to archive squad")
		return
	}

	h.publish(protocol.EventSquadDeleted, workspaceID, "member", userID, map[string]any{
		"squad_id":  uuidToString(squad.ID),
		"leader_id": uuidToString(squad.LeaderID),
	})
	w.WriteHeader(http.StatusNoContent)
}

// ── Squad Members ───────────────────────────────────────────────────────────

func (h *Handler) ListSquadMembers(w http.ResponseWriter, r *http.Request) {
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	members, err := h.Queries.ListSquadMembers(r.Context(), squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list squad members")
		return
	}
	resp := make([]SquadMemberResponse, len(members))
	for i, m := range members {
		resp[i] = squadMemberToResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ── Squad Member Status ────────────────────────────────────────────────────

// SquadMemberStatus is the per-member entry in the squad member status
// response. Agent members carry a derived working/idle/offline/unstable
// status plus any active issues; human members are returned with member_type
// only so the front-end can render them in the same list without
// reordering.
type SquadMemberStatusResponse struct {
	MemberType   string                  `json:"member_type"`
	MemberID     string                  `json:"member_id"`
	Status       *string                 `json:"status"`
	ActiveIssues []SquadActiveIssueBrief `json:"active_issues"`
	LastActiveAt *string                 `json:"last_active_at"`
}

type SquadActiveIssueBrief struct {
	IssueID     string `json:"issue_id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	IssueStatus string `json:"issue_status"`
}

type SquadMemberStatusListResponse struct {
	Members []SquadMemberStatusResponse `json:"members"`
}

// deriveSquadMemberStatus collapses runtime + task signals into the five
// status buckets used by the squad UI. Mirrors the workload+availability
// split in packages/core/agents/derive-presence.ts: working wins over
// runtime health (an agent that is in the middle of dispatched/running
// work counts as working even if the runtime briefly drops), then
// availability buckets decide between idle / unstable / offline.
//
// Thresholds match deriveRuntimeHealth: any offline runtime whose
// last_seen_at is within the last 5 minutes is reported as "unstable" so
// the squad UI surfaces transient drops the same way the agent dot does.
//
// Archived agents always report `archived` regardless of any leftover
// runtime row or task — they should appear in the list but never look
// like they're still working or merely offline (a leftover online
// runtime row would otherwise read as "offline" and hide the fact that
// the agent has been archived). Per the RFC decision (see MUL-2319), we
// surface archived agents in this endpoint rather than filtering them
// out in the SQL.
func deriveSquadMemberStatus(
	archived bool,
	runtimeStatus pgtype.Text,
	lastSeen pgtype.Timestamptz,
	hasActiveTask bool,
	now time.Time,
) string {
	if archived {
		return "archived"
	}
	if hasActiveTask {
		return "working"
	}
	if !runtimeStatus.Valid {
		return "offline"
	}
	if runtimeStatus.String == "online" {
		return "idle"
	}
	if !lastSeen.Valid {
		return "offline"
	}
	if now.Sub(lastSeen.Time) < 5*time.Minute {
		return "unstable"
	}
	return "offline"
}

// ListSquadMemberStatus returns one entry per squad member with derived
// status, the issues each agent member is currently running, and the last
// observed runtime activity. The endpoint is read-only and inherits the
// workspace-membership guard from the route middleware — any member of the
// workspace can read it.
func (h *Handler) ListSquadMemberStatus(w http.ResponseWriter, r *http.Request) {
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}

	rows, err := h.Queries.ListSquadMemberStatusRows(r.Context(), squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list squad member status")
		return
	}

	prefix := h.getIssuePrefix(r.Context(), squad.WorkspaceID)
	now := time.Now()

	// Group rows by member_id while preserving the SQL ORDER BY (squad_member
	// insertion order). One member may appear in multiple rows when they have
	// more than one active task.
	type memberAcc struct {
		response       SquadMemberStatusResponse
		archived       bool
		hasActiveTask  bool
		runtimeStatus  pgtype.Text
		runtimeSeenAt  pgtype.Timestamptz
		latestActiveAt pgtype.Timestamptz
	}
	order := make([]string, 0, len(rows))
	acc := make(map[string]*memberAcc, len(rows))

	for _, row := range rows {
		memberID := uuidToString(row.MemberID)
		entry, exists := acc[memberID]
		if !exists {
			entry = &memberAcc{
				response: SquadMemberStatusResponse{
					MemberType:   row.MemberType,
					MemberID:     memberID,
					ActiveIssues: []SquadActiveIssueBrief{},
				},
				archived:      row.AgentArchivedAt.Valid,
				runtimeStatus: row.RuntimeStatus,
				runtimeSeenAt: row.RuntimeLastSeenAt,
			}
			acc[memberID] = entry
			order = append(order, memberID)
		}

		if row.MemberType != "agent" {
			continue
		}

		// A dispatched/running task occupies an agent slot even when it
		// has no associated issue (chat / quick-create tasks set
		// agent_task_queue.issue_id = NULL). The `working` bucket is
		// defined by task presence, not by whether we can render an
		// issue link, so flag the agent here regardless of issue_id.
		if row.TaskID.Valid {
			entry.hasActiveTask = true

			if row.TaskIssueID.Valid {
				brief := SquadActiveIssueBrief{
					IssueID:    uuidToString(row.TaskIssueID),
					Identifier: prefix + "-" + strconv.Itoa(int(row.IssueNumber.Int32)),
					Title:      row.IssueTitle.String,
					IssueStatus: func() string {
						if row.IssueStatus.Valid {
							return row.IssueStatus.String
						}
						return ""
					}(),
				}
				entry.response.ActiveIssues = append(entry.response.ActiveIssues, brief)
			}

			if row.TaskDispatchedAt.Valid && (!entry.latestActiveAt.Valid ||
				row.TaskDispatchedAt.Time.After(entry.latestActiveAt.Time)) {
				entry.latestActiveAt = row.TaskDispatchedAt
			}
		}
	}

	resp := SquadMemberStatusListResponse{
		Members: make([]SquadMemberStatusResponse, 0, len(order)),
	}
	for _, id := range order {
		entry := acc[id]
		if entry.response.MemberType == "agent" {
			status := deriveSquadMemberStatus(
				entry.archived,
				entry.runtimeStatus,
				entry.runtimeSeenAt,
				entry.hasActiveTask,
				now,
			)
			entry.response.Status = &status
			// last_active_at prefers the freshest active-task dispatch
			// over the runtime heartbeat: a working agent should not
			// look stale because the runtime heartbeat is a few seconds
			// behind. Falls back to runtime last_seen_at otherwise.
			if entry.latestActiveAt.Valid {
				entry.response.LastActiveAt = timestampToPtr(entry.latestActiveAt)
			} else if entry.runtimeSeenAt.Valid {
				entry.response.LastActiveAt = timestampToPtr(entry.runtimeSeenAt)
			}
		}
		resp.Members = append(resp.Members, entry.response)
	}

	writeJSON(w, http.StatusOK, resp)
}

type squadWorkflowStageResponse struct {
	ID          string   `json:"id"`
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Position    int32    `json:"position"`
	Keywords    []string `json:"keywords"`
}

type squadWorkflowCanvasPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type squadWorkflowCanvasLayout struct {
	Leader *squadWorkflowCanvasPoint           `json:"leader,omitempty"`
	Stages map[string]squadWorkflowCanvasPoint `json:"stages"`
	Agents map[string]squadWorkflowCanvasPoint `json:"agents"`
}

func validSquadWorkflowCanvasPoint(point squadWorkflowCanvasPoint) bool {
	return !math.IsNaN(point.X) && !math.IsInf(point.X, 0) &&
		!math.IsNaN(point.Y) && !math.IsInf(point.Y, 0) &&
		point.X >= 0 && point.X <= 10000 && point.Y >= 0 && point.Y <= 10000
}

func (h *Handler) ensureSquadWorkflowStages(ctx context.Context, squadID pgtype.UUID) error {
	_, err := h.DB.Exec(ctx, `
		INSERT INTO squad_workflow_stage (squad_id, id, position, baseline_position)
		SELECT $1, defaults.id, defaults.position, defaults.position
		FROM (VALUES
			('requirements', 0), ('knowledge', 1), ('research', 2),
			('design', 3), ('implementation', 4), ('review', 5),
			('test', 6), ('delivery', 7), ('support', 8)
		) AS defaults(id, position)
		WHERE NOT EXISTS (
			SELECT 1 FROM squad_workflow_stage WHERE squad_id = $1
		)
		ON CONFLICT (squad_id, id) DO NOTHING
	`, squadID)
	return err
}

func (h *Handler) shouldGenerateLegacySquadWorkflow(ctx context.Context, squadID pgtype.UUID) (bool, error) {
	var shouldGenerate bool
	err := h.DB.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT source = 'legacy_default' FROM squad_workflow_config WHERE squad_id = $1), true)
			AND (SELECT count(*) FROM squad_workflow_assignment WHERE squad_id = $1) = 0
			AND (SELECT count(*) FROM squad_workflow_stage WHERE squad_id = $1) = 9
			AND NOT EXISTS (
				SELECT 1
				FROM squad_workflow_stage
				WHERE squad_id = $1
					AND (
						id NOT IN ('requirements', 'knowledge', 'research', 'design', 'implementation', 'review', 'test', 'delivery', 'support')
						OR name IS NOT NULL OR description IS NOT NULL OR position <> baseline_position
					)
			)
	`, squadID).Scan(&shouldGenerate)
	return shouldGenerate, err
}

func validateSquadWorkflowStageInput(name, description string) (string, string, string) {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" {
		return "", "", "stage name is required"
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", "", "stage name must be 80 characters or fewer"
	}
	if utf8.RuneCountInString(description) > 500 {
		return "", "", "stage description must be 500 characters or fewer"
	}
	return name, description, ""
}

// ListSquadWorkflowAssignments returns the editable stage definition and the
// manual agent overrides. Agents without an override use deterministic role
// inference in the UI.
func (h *Handler) ListSquadWorkflowAssignments(w http.ResponseWriter, r *http.Request) {
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.ensureSquadWorkflowStages(r.Context(), squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize squad workflow")
		return
	}
	shouldGenerate, err := h.shouldGenerateLegacySquadWorkflow(r.Context(), squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect squad workflow")
		return
	}
	if shouldGenerate {
		if _, err := h.generateSquadWorkflow(r.Context(), squad, squad.CreatorID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to generate squad workflow")
			return
		}
	}

	stageRows, err := h.DB.Query(r.Context(), `
		SELECT id, name, description, position, keywords
		FROM squad_workflow_stage
		WHERE squad_id = $1
		ORDER BY position, created_at, id
	`, squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow stages")
		return
	}
	stages := make([]squadWorkflowStageResponse, 0)
	for stageRows.Next() {
		var stage squadWorkflowStageResponse
		var name, description pgtype.Text
		if err := stageRows.Scan(&stage.ID, &name, &description, &stage.Position, &stage.Keywords); err != nil {
			stageRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to load squad workflow stages")
			return
		}
		stage.Name = textToPtr(name)
		stage.Description = textToPtr(description)
		stages = append(stages, stage)
	}
	if err := stageRows.Err(); err != nil {
		stageRows.Close()
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow stages")
		return
	}
	stageRows.Close()

	rows, err := h.DB.Query(r.Context(), `
		SELECT agent_id, stage_id, source
		FROM squad_workflow_assignment
		WHERE squad_id = $1
	`, squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow assignments")
		return
	}
	defer rows.Close()

	assignments := make(map[string]string)
	assignmentSources := make(map[string]string)
	for rows.Next() {
		var agentID pgtype.UUID
		var stageID, source string
		if err := rows.Scan(&agentID, &stageID, &source); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load squad workflow assignments")
			return
		}
		assignments[uuidToString(agentID)] = stageID
		assignmentSources[uuidToString(agentID)] = source
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow assignments")
		return
	}

	var isDefaultOrder bool
	if err := h.DB.QueryRow(r.Context(), `
		SELECT COALESCE(bool_and(position = baseline_position), true)
		FROM squad_workflow_stage
		WHERE squad_id = $1
	`, squad.ID).Scan(&isDefaultOrder); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect squad workflow order")
		return
	}

	var generationSource, generatedProfile pgtype.Text
	var canvasLayout json.RawMessage
	if err := h.DB.QueryRow(r.Context(), `
		SELECT source, profile, canvas_layout FROM squad_workflow_config WHERE squad_id = $1
	`, squad.ID).Scan(&generationSource, &generatedProfile, &canvasLayout); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow profile")
		return
	}
	if len(canvasLayout) == 0 {
		canvasLayout = json.RawMessage(`{"stages":{},"agents":{}}`)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stages":             stages,
		"assignments":        assignments,
		"assignment_sources": assignmentSources,
		"is_default_order":   isDefaultOrder,
		"generation_source":  generationSource.String,
		"generated_profile":  textToPtr(generatedProfile),
		"canvas_layout":      canvasLayout,
	})
}

func (h *Handler) SetSquadWorkflowCanvasLayout(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var layout squadWorkflowCanvasLayout
	if err := decoder.Decode(&layout); err != nil {
		writeError(w, http.StatusBadRequest, "invalid canvas layout")
		return
	}
	if layout.Stages == nil {
		layout.Stages = make(map[string]squadWorkflowCanvasPoint)
	}
	if layout.Agents == nil {
		layout.Agents = make(map[string]squadWorkflowCanvasPoint)
	}
	if len(layout.Stages) > 30 || len(layout.Agents) > 500 {
		writeError(w, http.StatusBadRequest, "canvas layout contains too many nodes")
		return
	}
	if layout.Leader != nil && !validSquadWorkflowCanvasPoint(*layout.Leader) {
		writeError(w, http.StatusBadRequest, "canvas layout contains an invalid leader position")
		return
	}
	for stageID, point := range layout.Stages {
		if strings.TrimSpace(stageID) == "" || !validSquadWorkflowCanvasPoint(point) {
			writeError(w, http.StatusBadRequest, "canvas layout contains an invalid stage position")
			return
		}
	}
	for agentID, point := range layout.Agents {
		if strings.TrimSpace(agentID) == "" || !validSquadWorkflowCanvasPoint(point) {
			writeError(w, http.StatusBadRequest, "canvas layout contains an invalid agent position")
			return
		}
	}

	payload, err := json.Marshal(layout)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid canvas layout")
		return
	}
	if _, err := h.DB.Exec(r.Context(), `
		INSERT INTO squad_workflow_config (squad_id, canvas_layout, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (squad_id) DO UPDATE SET
			canvas_layout = EXCLUDED.canvas_layout,
			updated_at = EXCLUDED.updated_at
	`, squad.ID, string(payload)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save canvas layout")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) GenerateSquadWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	profile, err := h.generateSquadWorkflow(r.Context(), squad, member.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate squad workflow")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	writeJSON(w, http.StatusOK, map[string]string{"profile": profile})
}

func (h *Handler) SetSquadWorkflowAssignment(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.ensureSquadWorkflowStages(r.Context(), squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize squad workflow")
		return
	}

	agentID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "agentId"), "agent id")
	if !ok {
		return
	}
	if agentID == squad.LeaderID {
		writeError(w, http.StatusBadRequest, "squad leader is fixed at workflow intake")
		return
	}
	isMember, err := h.Queries.IsSquadMember(r.Context(), db.IsSquadMemberParams{
		SquadID: squad.ID, MemberType: "agent", MemberID: agentID,
	})
	if err != nil || !isMember {
		writeError(w, http.StatusBadRequest, "agent must be a member of this squad")
		return
	}

	var req struct {
		StageID string `json:"stage_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.StageID = strings.TrimSpace(req.StageID)
	var stageExists bool
	if err := h.DB.QueryRow(r.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM squad_workflow_stage WHERE squad_id = $1 AND id = $2
		)
	`, squad.ID, req.StageID).Scan(&stageExists); err != nil || !stageExists {
		writeError(w, http.StatusBadRequest, "invalid workflow stage")
		return
	}

	_, err = h.DB.Exec(r.Context(), `
		INSERT INTO squad_workflow_assignment (squad_id, agent_id, stage_id, updated_by, source)
		VALUES ($1, $2, $3, $4, 'manual')
		ON CONFLICT (squad_id, agent_id) DO UPDATE SET
			stage_id = EXCLUDED.stage_id,
			updated_by = EXCLUDED.updated_by,
			source = 'manual',
			updated_at = now()
	`, squad.ID, agentID, req.StageID, member.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update squad workflow assignment")
		return
	}

	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CreateSquadWorkflowStage(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.ensureSquadWorkflowStages(r.Context(), squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize squad workflow")
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, description, validationError := validateSquadWorkflowStageInput(req.Name, req.Description)
	if validationError != "" {
		writeError(w, http.StatusBadRequest, validationError)
		return
	}

	var stageCount int
	if err := h.DB.QueryRow(r.Context(), `SELECT count(*) FROM squad_workflow_stage WHERE squad_id = $1`, squad.ID).Scan(&stageCount); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workflow stage")
		return
	}
	if stageCount >= 30 {
		writeError(w, http.StatusConflict, "a workflow can contain at most 30 stages")
		return
	}

	stage := squadWorkflowStageResponse{ID: "custom_" + randomID()}
	var stageName, stageDescription pgtype.Text
	err := h.DB.QueryRow(r.Context(), `
		INSERT INTO squad_workflow_stage
			(squad_id, id, name, description, position, baseline_position, updated_by)
		VALUES (
			$1, $2, $3, $4,
			COALESCE((SELECT max(position) + 1 FROM squad_workflow_stage WHERE squad_id = $1), 0),
			COALESCE((SELECT max(baseline_position) + 1 FROM squad_workflow_stage WHERE squad_id = $1), 0),
			$5
		)
		RETURNING name, description, position, keywords
	`, squad.ID, stage.ID, name, description, member.UserID).Scan(&stageName, &stageDescription, &stage.Position, &stage.Keywords)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workflow stage")
		return
	}
	stage.Name = textToPtr(stageName)
	stage.Description = textToPtr(stageDescription)
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{"squad_id": uuidToString(squad.ID)})
	writeJSON(w, http.StatusCreated, stage)
}

func (h *Handler) UpdateSquadWorkflowStage(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	stageID := strings.TrimSpace(chi.URLParam(r, "stageId"))
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, description, validationError := validateSquadWorkflowStageInput(req.Name, req.Description)
	if validationError != "" {
		writeError(w, http.StatusBadRequest, validationError)
		return
	}

	stage := squadWorkflowStageResponse{ID: stageID}
	var stageName, stageDescription pgtype.Text
	err := h.DB.QueryRow(r.Context(), `
		UPDATE squad_workflow_stage
		SET name = $3, description = $4, updated_by = $5, updated_at = now()
		WHERE squad_id = $1 AND id = $2
		RETURNING name, description, position, keywords
	`, squad.ID, stageID, name, description, member.UserID).Scan(&stageName, &stageDescription, &stage.Position, &stage.Keywords)
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow stage not found")
		return
	}
	stage.Name = textToPtr(stageName)
	stage.Description = textToPtr(stageDescription)
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{"squad_id": uuidToString(squad.ID)})
	writeJSON(w, http.StatusOK, stage)
}

func (h *Handler) DeleteSquadWorkflowStage(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	stageID := strings.TrimSpace(chi.URLParam(r, "stageId"))
	var stageCount int
	if err := h.DB.QueryRow(r.Context(), `SELECT count(*) FROM squad_workflow_stage WHERE squad_id = $1`, squad.ID).Scan(&stageCount); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	if stageCount <= 1 {
		writeError(w, http.StatusConflict, "a workflow must contain at least one stage")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	if _, err := tx.Exec(r.Context(), `DELETE FROM squad_workflow_assignment WHERE squad_id = $1 AND stage_id = $2`, squad.ID, stageID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	tag, err := tx.Exec(r.Context(), `DELETE FROM squad_workflow_stage WHERE squad_id = $1 AND id = $2`, squad.ID, stageID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "workflow stage not found")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		WITH current_ordered AS (
			SELECT id, row_number() OVER (ORDER BY position, created_at, id) - 1 AS next_position
			FROM squad_workflow_stage WHERE squad_id = $1
		), baseline_ordered AS (
			SELECT id, row_number() OVER (ORDER BY baseline_position, created_at, id) - 1 AS next_position
			FROM squad_workflow_stage WHERE squad_id = $1
		)
		UPDATE squad_workflow_stage AS stage
		SET position = current_ordered.next_position,
			baseline_position = baseline_ordered.next_position,
			updated_at = now()
		FROM current_ordered, baseline_ordered
		WHERE stage.squad_id = $1
			AND stage.id = current_ordered.id
			AND stage.id = baseline_ordered.id
	`, squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow stage")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{"squad_id": uuidToString(squad.ID)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ReorderSquadWorkflowStages(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	var req struct {
		StageIDs []string `json:"stage_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.StageIDs) == 0 {
		writeError(w, http.StatusBadRequest, "stage_ids is required")
		return
	}
	seen := make(map[string]struct{}, len(req.StageIDs))
	for _, stageID := range req.StageIDs {
		stageID = strings.TrimSpace(stageID)
		if stageID == "" {
			writeError(w, http.StatusBadRequest, "stage_ids contains an empty id")
			return
		}
		if _, duplicate := seen[stageID]; duplicate {
			writeError(w, http.StatusBadRequest, "stage_ids contains duplicates")
			return
		}
		seen[stageID] = struct{}{}
	}
	var existingCount, matchedCount int
	err := h.DB.QueryRow(r.Context(), `
		SELECT
			(SELECT count(*) FROM squad_workflow_stage WHERE squad_id = $1),
			(SELECT count(*) FROM squad_workflow_stage WHERE squad_id = $1 AND id = ANY($2::text[]))
	`, squad.ID, req.StageIDs).Scan(&existingCount, &matchedCount)
	if err != nil || existingCount != len(req.StageIDs) || matchedCount != existingCount {
		writeError(w, http.StatusBadRequest, "stage_ids must contain every workflow stage exactly once")
		return
	}
	if _, err := h.DB.Exec(r.Context(), `
		UPDATE squad_workflow_stage AS stage
		SET position = requested.ordinality - 1, updated_by = $3, updated_at = now()
		FROM unnest($2::text[]) WITH ORDINALITY AS requested(id, ordinality)
		WHERE stage.squad_id = $1 AND stage.id = requested.id
	`, squad.ID, req.StageIDs, member.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reorder workflow stages")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{"squad_id": uuidToString(squad.ID)})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ResetSquadWorkflowAssignments(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin")
	if !ok {
		return
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	if err := h.ensureSquadWorkflowStages(r.Context(), squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to initialize squad workflow")
		return
	}

	stageRows, err := h.DB.Query(r.Context(), `
		SELECT id, keywords
		FROM squad_workflow_stage
		WHERE squad_id = $1
		ORDER BY baseline_position, created_at, id
	`, squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow")
		return
	}
	stages := make([]squadWorkflowTemplateStage, 0)
	for stageRows.Next() {
		var stage squadWorkflowTemplateStage
		if err := stageRows.Scan(&stage.id, &stage.keywords); err != nil {
			stageRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to load squad workflow")
			return
		}
		stages = append(stages, stage)
	}
	if err := stageRows.Err(); err != nil {
		stageRows.Close()
		writeError(w, http.StatusInternalServerError, "failed to load squad workflow")
		return
	}
	stageRows.Close()

	agents, err := h.loadSquadWorkflowAgents(r.Context(), squad.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load squad agents")
		return
	}
	workers := make([]squadWorkflowAgent, 0, len(agents))
	for _, agent := range agents {
		if !squad.LeaderID.Valid || agent.id != squad.LeaderID {
			workers = append(workers, agent)
		}
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset squad workflow")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck
	if _, err := tx.Exec(r.Context(), `DELETE FROM squad_workflow_assignment WHERE squad_id = $1`, squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset squad workflow")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE squad_workflow_stage
		SET position = baseline_position, updated_by = $2, updated_at = now()
		WHERE squad_id = $1
	`, squad.ID, member.UserID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset squad workflow")
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE squad_workflow_config
		SET canvas_layout = '{"stages":{},"agents":{}}'::jsonb, updated_at = now()
		WHERE squad_id = $1
	`, squad.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset squad workflow layout")
		return
	}
	for _, agent := range workers {
		stageIndex := chooseAgentWorkflowStage(agent, stages)
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO squad_workflow_assignment (squad_id, agent_id, stage_id, updated_by, source)
			VALUES ($1, $2, $3, $4, 'generated')
		`, squad.ID, agent.id, stages[stageIndex].id, member.UserID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reset squad workflow")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset squad workflow")
		return
	}
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AddSquadMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}

	var req struct {
		MemberType string `json:"member_type"`
		MemberID   string `json:"member_id"`
		Role       string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MemberType != "agent" && req.MemberType != "member" {
		writeError(w, http.StatusBadRequest, "member_type must be 'agent' or 'member'")
		return
	}
	if req.MemberID == "" {
		writeError(w, http.StatusBadRequest, "member_id is required")
		return
	}

	memberUUID, ok := parseUUIDOrBadRequest(w, req.MemberID, "member_id")
	if !ok {
		return
	}

	// Validate the member belongs to this workspace.
	if req.MemberType == "agent" {
		if _, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
			ID: memberUUID, WorkspaceID: wsUUID,
		}); err != nil {
			writeError(w, http.StatusBadRequest, "agent not found in this workspace")
			return
		}
	} else {
		if _, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
			UserID: memberUUID, WorkspaceID: wsUUID,
		}); err != nil {
			writeError(w, http.StatusBadRequest, "member not found in this workspace")
			return
		}
	}

	sm, err := h.Queries.AddSquadMember(r.Context(), db.AddSquadMemberParams{
		SquadID:    squad.ID,
		MemberType: req.MemberType,
		MemberID:   memberUUID,
		Role:       req.Role,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "member already in squad")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to add squad member")
		return
	}

	writeJSON(w, http.StatusCreated, squadMemberToResponse(sm))
	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
}

func (h *Handler) RemoveSquadMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}

	var req struct {
		MemberType string `json:"member_type"`
		MemberID   string `json:"member_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	memberUUID, ok := parseUUIDOrBadRequest(w, req.MemberID, "member_id")
	if !ok {
		return
	}

	// Prevent removing the leader.
	if req.MemberType == "agent" && uuidToString(squad.LeaderID) == req.MemberID {
		writeError(w, http.StatusBadRequest, "cannot remove the squad leader; change leader first")
		return
	}

	rows, err := h.Queries.RemoveSquadMember(r.Context(), db.RemoveSquadMemberParams{
		SquadID:    squad.ID,
		MemberType: req.MemberType,
		MemberID:   memberUUID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove squad member")
		return
	}
	if rows == 0 {
		writeError(w, http.StatusNotFound, "squad member not found")
		return
	}

	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateSquadMemberRole(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	if _, ok := h.requireWorkspaceRole(w, r, workspaceID, "workspace not found", "owner", "admin"); !ok {
		return
	}

	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return
	}

	var req struct {
		MemberType string `json:"member_type"`
		MemberID   string `json:"member_id"`
		Role       string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	memberUUID, ok := parseUUIDOrBadRequest(w, req.MemberID, "member_id")
	if !ok {
		return
	}

	sm, err := h.Queries.UpdateSquadMemberRole(r.Context(), db.UpdateSquadMemberRoleParams{
		SquadID:    squad.ID,
		MemberType: req.MemberType,
		MemberID:   memberUUID,
		Role:       req.Role,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "squad member not found")
		return
	}

	h.publish(protocol.EventSquadUpdated, workspaceID, "member", requestUserID(r), map[string]any{
		"squad_id": uuidToString(squad.ID),
	})
	writeJSON(w, http.StatusOK, squadMemberToResponse(sm))
}

// ── Squad Leader Evaluation ──────────────────────────────────────────────────

// RecordSquadLeaderEvaluation records a squad leader's evaluation decision
// into the unified activity_log. Called by the leader agent via CLI after
// each trigger to record whether it took action, stayed silent, or failed.
func (h *Handler) RecordSquadLeaderEvaluation(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var req struct {
		Outcome string `json:"outcome"` // action | no_action | failed
		Reason  string `json:"reason"`  // short explanation from leader
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Outcome != "action" && req.Outcome != "no_action" && req.Outcome != "failed" {
		writeError(w, http.StatusBadRequest, "outcome must be 'action', 'no_action', or 'failed'")
		return
	}

	// The issue must be assigned to a squad.
	if !issue.AssigneeType.Valid || issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		writeError(w, http.StatusBadRequest, "issue is not assigned to a squad")
		return
	}

	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{
		ID:          issue.AssigneeID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "squad not found")
		return
	}

	// Security: only the squad leader agent can record evaluations.
	workspaceID := uuidToString(issue.WorkspaceID)
	userID := requestUserID(r)
	actorType, actorID := h.resolveActor(r, userID, workspaceID)
	if actorType != "agent" || actorID != uuidToString(squad.LeaderID) {
		writeError(w, http.StatusForbidden, "only the squad leader agent can record evaluations")
		return
	}

	taskID := r.Header.Get("X-Task-ID")
	taskUUID, ok := parseUUIDOrBadRequest(w, taskID, "task id")
	if !ok {
		return
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskUUID)
	if err != nil || !task.IssueID.Valid || uuidToString(task.IssueID) != uuidToString(issue.ID) {
		writeError(w, http.StatusBadRequest, "task does not belong to issue")
		return
	}

	details, _ := json.Marshal(map[string]string{
		"squad_id": uuidToString(squad.ID),
		"task_id":  util.UUIDToString(taskUUID),
		"outcome":  req.Outcome,
		"reason":   req.Reason,
	})

	activity, err := h.Queries.CreateActivity(r.Context(), db.CreateActivityParams{
		WorkspaceID: issue.WorkspaceID,
		IssueID:     issue.ID,
		ActorType:   pgtype.Text{String: "agent", Valid: true},
		ActorID:     squad.LeaderID,
		Action:      "squad_leader_evaluated",
		Details:     details,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record evaluation")
		return
	}

	h.publish(protocol.EventActivityCreated, uuidToString(issue.WorkspaceID), "agent", actorID, map[string]any{
		"issue_id": uuidToString(issue.ID),
		"entry": map[string]any{
			"type":       "activity",
			"id":         uuidToString(activity.ID),
			"actor_type": "agent",
			"actor_id":   actorID,
			"action":     activity.Action,
			"details":    json.RawMessage(details),
			"created_at": timestampToString(activity.CreatedAt),
		},
	})

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":         uuidToString(activity.ID),
		"action":     activity.Action,
		"created_at": timestampToString(activity.CreatedAt),
	})
}

// ── Squad Trigger Logic ─────────────────────────────────────────────────────

// lastTaskWasLeader returns true when the agent's most recent task on the
// issue was enqueued in the squad-leader role. Used by the self-trigger
// guards to tell apart a comment posted while the agent was acting as
// leader (skip) from one posted while it was acting as a worker (do not
// skip). When the agent has no prior task on this issue the role is
// undetermined and we treat it as non-leader so a brand-new external
// trigger can still reach the leader.
func (h *Handler) lastTaskWasLeader(ctx context.Context, issueID, agentID pgtype.UUID) bool {
	flag, err := h.Queries.GetLatestTaskIsLeaderForIssueAndAgent(ctx, db.GetLatestTaskIsLeaderForIssueAndAgentParams{
		IssueID: issueID,
		AgentID: agentID,
	})
	if err != nil {
		return false
	}
	return flag
}

// commentMentionsAnyone returns true when the comment body contains at least
// one routing-style mention — [@Name](mention://agent|member|squad|all/<id>).
// Issue cross-references (mention://issue/...) are ignored because they are
// not directed at a participant. Only the current comment is inspected —
// parent (thread root) mentions are NOT inherited here.
func commentMentionsAnyone(content string) bool {
	for _, m := range util.ParseMentions(content) {
		switch m.Type {
		case "agent", "member", "squad", "all":
			return true
		}
	}
	return false
}

// shouldEnqueueSquadLeaderOnAssign returns true when assigning an issue to a
// squad (or creating an issue pre-assigned to a squad) should immediately
// trigger the squad leader. Mirrors shouldEnqueueAgentTask: backlog issues
// are skipped (parking lot), and the leader agent must have a runtime and
// not be archived.
func (h *Handler) shouldEnqueueSquadLeaderOnAssign(ctx context.Context, issue db.Issue) bool {
	if issue.Status == "backlog" {
		return false
	}
	return h.isSquadLeaderReady(ctx, issue)
}

// isSquadLeaderReady returns true when the issue is assigned to a squad whose
// leader agent can accept work right now. Readiness criteria (archived,
// runtime bound, runtime online) are shared with the autopilot admission
// gate via service.AgentReadiness — both paths must move together or one
// will start enqueueing tasks the other refuses (MUL-2429 RFC §4.b B4).
func (h *Handler) isSquadLeaderReady(ctx context.Context, issue db.Issue) bool {
	if !issue.AssigneeType.Valid || issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		return false
	}
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
		ID:          issue.AssigneeID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		return false
	}
	agent, err := h.Queries.GetAgent(ctx, squad.LeaderID)
	if err != nil {
		return false
	}
	ready, _, err := service.AgentReadiness(ctx, h.Queries, agent)
	if err != nil {
		// Fail closed when we can't tell — same posture as the rest of
		// this function (any error path returns false).
		return false
	}
	return ready
}

// enqueueSquadLeaderTask triggers the squad leader agent for an issue assigned
// to a squad. Assign and backlog-promotion paths use this directly; comment
// paths go through computeCommentAgentTriggers so preview and create share the
// same trigger set.
func (h *Handler) enqueueSquadLeaderTask(ctx context.Context, issue db.Issue, triggerCommentID pgtype.UUID, authorType, authorID string, requestingUserID pgtype.UUID) {
	squad, err := h.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{
		ID:          issue.AssigneeID,
		WorkspaceID: issue.WorkspaceID,
	})
	if err != nil {
		return
	}

	if !h.canEnqueueSquadLeader(ctx, squad.LeaderID, authorType, authorID, uuidToString(issue.WorkspaceID)) {
		return
	}

	hasPending, err := h.Queries.HasPendingTaskForIssueAndAgent(ctx, db.HasPendingTaskForIssueAndAgentParams{
		IssueID: issue.ID,
		AgentID: squad.LeaderID,
	})
	if err != nil || hasPending {
		return
	}

	if _, err := h.TaskService.EnqueueTaskForSquadLeaderByUser(ctx, issue, squad.LeaderID, triggerCommentID, requestingUserID); err != nil {
		slog.Warn("enqueue squad leader task failed",
			"issue_id", uuidToString(issue.ID),
			"squad_id", uuidToString(squad.ID),
			"leader_id", uuidToString(squad.LeaderID),
			"error", err)
	}
}
