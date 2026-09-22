package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var errDocumentAccessRevoked = errors.New("document edit access was revoked")

func (h *Handler) publishDocumentAccessChanged(doc db.Issue, previous []pgtype.UUID, actorType, actorID string) {
	readers, err := h.Queries.ListDocumentReaders(context.Background(), db.ListDocumentReadersParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		return
	}
	seen := map[string]bool{}
	recipients := []string{}
	for _, id := range append(previous, readers...) {
		key := uuidToString(id)
		if !seen[key] {
			seen[key] = true
			recipients = append(recipients, key)
		}
	}
	h.Bus.Publish(events.Event{Type: "document:access_changed", WorkspaceID: uuidToString(doc.WorkspaceID), ActorType: actorType, ActorID: actorID,
		RecipientUserIDs: recipients, Payload: map[string]any{"document_id": uuidToString(doc.ID)}})
}

func (h *Handler) documentPermission(ctx context.Context, q *db.Queries, doc db.Issue, userID string) string {
	if userID == "" {
		return ""
	}
	permission, err := q.GetDocumentPermission(ctx, db.GetDocumentPermissionParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, UserID: parseUUID(userID)})
	if err != nil {
		return ""
	}
	return permission
}

// Document authorization is independent of workspace administrative roles.
// The shared issue loader also covers comments, timelines and task-shaped URLs.
func (h *Handler) checkDocumentAccess(w http.ResponseWriter, r *http.Request, doc db.Issue) bool {
	if doc.Kind != "doc" {
		return true
	}
	userID := requestUserID(r)
	if userID == "" {
		writeError(w, 404, "document not found")
		return false
	}
	permission := h.documentPermission(r.Context(), h.Queries, doc, userID)
	if permission == "" {
		writeError(w, 404, "document not found")
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && permission == "view" {
		writeError(w, 403, "document is read-only")
		return false
	}

	return true
}

func (h *Handler) requireDocumentOwner(w http.ResponseWriter, r *http.Request, doc db.Issue) bool {
	if h.documentPermission(r.Context(), h.Queries, doc, requestUserID(r)) != "owner" {
		writeError(w, 403, "only the document owner can manage sharing or move the document")
		return false
	}
	return true
}

type documentActorKey struct{}
type documentActor struct{ UserID, Type, ID string }

func (h *Handler) documentActorRequest(r *http.Request, workspaceID string) *http.Request {
	user := requestUserID(r)
	kind, id := h.resolveActor(r, user, workspaceID)
	return r.WithContext(context.WithValue(r.Context(), documentActorKey{}, documentActor{user, kind, id}))
}

type documentCollaboratorInput struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *Handler) GetDocumentAccess(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return
	}
	h.respondDocumentAccess(w, r, doc)
}

func (h *Handler) respondDocumentAccess(w http.ResponseWriter, r *http.Request, doc db.Issue) {
	access, err := h.Queries.GetDocumentAccess(r.Context(), db.GetDocumentAccessParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read document sharing")
		return
	}
	permission := h.documentPermission(r.Context(), h.Queries, doc, requestUserID(r))
	people := []documentCollaboratorInput{}
	if permission == "owner" {
		rows, err := h.Queries.ListDocumentCollaborators(r.Context(), db.ListDocumentCollaboratorsParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID})
		if err != nil {
			writeError(w, 500, "failed to read collaborators")
			return
		}
		for _, row := range rows {
			people = append(people, documentCollaboratorInput{uuidToString(row.UserID), row.Role})
		}
	}
	writeJSON(w, 200, map[string]any{"owner_id": uuidToString(access.OwnerID), "scope": access.Scope, "scope_role": access.ScopeRole, "project_id": uuidToPtr(access.ProjectID), "revision": access.Revision,
		"can_edit": permission == "owner" || permission == "edit", "can_manage": permission == "owner" && !isMachineCredentialActor(r), "collaborators": people})
}

// Publish is an explicit audience change. Subsequent edits remain shared.
func (h *Handler) UpdateDocumentAccess(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return
	}
	if !h.requireDocumentOwner(w, r, doc) {
		return
	}
	if isMachineCredentialActor(r) {
		writeErrorCode(w, 403, documentTransitionRequiresHuman, "document sharing requires a human owner")
		return
	}
	var req struct {
		Scope         string                      `json:"scope"`
		ScopeRole     string                      `json:"scope_role"`
		ProjectID     *string                     `json:"project_id"`
		Collaborators []documentCollaboratorInput `json:"collaborators"`
		Revision      int64                       `json:"expected_revision"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 65536)).Decode(&req) != nil || !contains([]string{"private", "project", "workspace"}, req.Scope) || len(req.Collaborators) > 200 {
		writeError(w, 400, "invalid sharing settings")
		return
	}
	if req.ScopeRole == "" {
		req.ScopeRole = "view"
	}
	if !contains([]string{"view", "edit"}, req.ScopeRole) {
		writeError(w, 400, "invalid shared permission")
		return
	}
	var project pgtype.UUID
	if req.Scope == "project" {
		if req.ProjectID == nil {
			writeError(w, 400, "project_id is required")
			return
		}
		project, ok = parseUUIDOrBadRequest(w, *req.ProjectID, "project_id")
		if !ok {
			return
		}
	} else if req.ProjectID != nil {
		writeError(w, 400, "project_id only applies to project sharing")
		return
	}
	ids := make([]pgtype.UUID, len(req.Collaborators))
	seen := map[string]bool{}
	for index, c := range req.Collaborators {
		id, valid := parseUUIDOrBadRequest(w, c.UserID, "user_id")
		if !valid {
			return
		}
		key := uuidToString(id)
		if seen[key] || !contains([]string{"view", "edit"}, c.Role) {
			writeError(w, 400, "invalid or duplicate collaborator")
			return
		}
		seen[key] = true
		ids[index] = id
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin sharing change")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	doc, err = q.LockIssueForDescriptionUpdate(r.Context(), db.LockIssueForDescriptionUpdateParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 404, "document not found")
		return
	}
	access, err := q.GetDocumentAccess(r.Context(), db.GetDocumentAccessParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read sharing settings")
		return
	}
	if access.Revision != req.Revision {
		writeErrorCode(w, 409, "document_sharing_conflict", "sharing settings changed; reload before saving")
		return
	}
	if project.Valid {
		if _, err = q.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: project, WorkspaceID: doc.WorkspaceID}); err != nil {
			writeError(w, 400, "project not found")
			return
		}
	}
	for _, id := range ids {
		if id == access.OwnerID {
			writeError(w, 400, "the owner is already a collaborator")
			return
		}
		if _, err = q.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: id, WorkspaceID: doc.WorkspaceID}); err != nil {
			writeError(w, 400, "collaborator must belong to this workspace")
			return
		}
	}
	previous, err := q.ListDocumentReaders(r.Context(), db.ListDocumentReadersParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read document audience")
		return
	}
	if err = q.ClearDocumentCollaborators(r.Context(), db.ClearDocumentCollaboratorsParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID}); err != nil {
		writeError(w, 500, "failed to replace collaborators")
		return
	}
	for index, c := range req.Collaborators {
		if err = q.AddDocumentCollaborator(r.Context(), db.AddDocumentCollaboratorParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, UserID: ids[index], Role: c.Role}); err != nil {
			writeError(w, 500, "failed to add collaborator")
			return
		}
	}
	if _, err = q.UpdateDocumentAccess(r.Context(), db.UpdateDocumentAccessParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, Scope: req.Scope, ProjectID: project, ScopeRole: req.ScopeRole}); err != nil {
		writeError(w, 500, "failed to update sharing")
		return
	}
	status := "published"
	if req.Scope == "private" && len(ids) == 0 {
		status = "draft"
	}
	if err = q.AuthorizeDocumentTransition(r.Context(), uuidToString(doc.ID)); err != nil {
		writeError(w, 500, "failed to authorize publication")
		return
	}
	doc, err = q.TransitionDocument(r.Context(), db.TransitionDocumentParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID, Status: status})
	if err != nil {
		writeError(w, 500, "failed to publish document")
		return
	}
	if err = q.CancelDocumentIngestion(r.Context(), db.CancelDocumentIngestionParams{WorkspaceID: doc.WorkspaceID, IssueID: doc.ID}); err != nil {
		writeError(w, 500, "failed to update ingestion")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit sharing")
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(doc.WorkspaceID))
	// Previous readers must invalidate cached bodies too, including readers
	// whose access was revoked. This event carries no document content.
	h.publishDocumentAccessChanged(doc, previous, actorType, actorID)
	h.respondDocumentAccess(w, r, doc)
}

func (h *Handler) ListDocumentVersions(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadDocumentForHistory(w, r)
	if !ok {
		return
	}
	before := int64(0)
	if raw := r.URL.Query().Get("before"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 1 {
			writeError(w, 400, "invalid version cursor")
			return
		}
		before = n
	}
	rows, err := h.Queries.ListDocumentVersions(r.Context(), db.ListDocumentVersionsParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, BeforeVersion: before})
	if err != nil {
		writeError(w, 500, "failed to read version history")
		return
	}
	if rows == nil {
		rows = []db.ListDocumentVersionsRow{}
	}
	var next *int64
	if len(rows) == 50 {
		n := rows[len(rows)-1].Version
		next = &n
	}
	writeJSON(w, 200, map[string]any{"versions": rows, "next_cursor": next})
}

func (h *Handler) loadDocumentForHistory(w http.ResponseWriter, r *http.Request) (db.Issue, bool) {
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return doc, false
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return doc, false
	}
	permission := h.documentPermission(r.Context(), h.Queries, doc, requestUserID(r))
	if permission != "owner" && permission != "edit" {
		writeError(w, 403, "version history requires edit access")
		return doc, false
	}
	return doc, true
}

func (h *Handler) GetDocumentVersion(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadDocumentForHistory(w, r)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil || version < 1 {
		writeError(w, 400, "invalid version")
		return
	}
	row, err := h.Queries.GetDocumentVersion(r.Context(), db.GetDocumentVersionParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, Version: version})
	if err != nil {
		writeError(w, 404, "version not found")
		return
	}
	writeJSON(w, 200, row)
}

func (h *Handler) RestoreDocumentVersion(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadDocumentForHistory(w, r)
	if !ok {
		return
	}
	version, err := strconv.ParseInt(chi.URLParam(r, "version"), 10, 64)
	if err != nil || version < 1 {
		writeError(w, 400, "invalid version")
		return
	}
	var req struct {
		Revision int64 `json:"expected_revision"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeError(w, 400, "invalid restore request")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin restore")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	doc, err = q.LockIssueForDescriptionUpdate(r.Context(), db.LockIssueForDescriptionUpdateParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 404, "document not found")
		return
	}
	permission := h.documentPermission(r.Context(), q, doc, requestUserID(r))
	if permission != "owner" && permission != "edit" {
		writeError(w, 403, "document is read-only")
		return
	}
	if doc.Revision != req.Revision {
		writeDocumentConflict(w, doc, "document_conflict")
		return
	}
	snapshot, err := q.GetDocumentVersion(r.Context(), db.GetDocumentVersionParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, Version: version})
	if err != nil {
		writeError(w, 404, "version not found")
		return
	}
	kind, actor := h.resolveActor(r, requestUserID(r), uuidToString(doc.WorkspaceID))
	if err = q.SetDocumentAuditActor(r.Context(), db.SetDocumentAuditActorParams{ActorType: kind, ActorID: actor, Action: "restore", RestoredFrom: strconv.FormatInt(version, 10)}); err != nil {
		writeError(w, 500, "failed to record restore actor")
		return
	}
	if err = q.AuthorizeDocumentWrite(r.Context(), uuidToString(doc.ID)+":"+strconv.FormatInt(doc.DocumentRevision, 10)); err != nil {
		writeError(w, 500, "failed to authorize restore")
		return
	}
	updated, err := q.RestoreDocumentVersion(r.Context(), db.RestoreDocumentVersionParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID, Title: snapshot.Title, Description: pgtype.Text{String: snapshot.Body, Valid: true}})
	if err != nil {
		writeError(w, 500, "failed to restore version")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit restore")
		return
	}
	h.respondDocument(w, r, updated, requestUserID(r))
}

// Linked resources inherit the document audience, not just workspace membership.
func (h *Handler) checkDocumentResource(w http.ResponseWriter, r *http.Request, issueID, workspaceID pgtype.UUID) bool {
	if !issueID.Valid {
		return true
	}
	issue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: issueID, WorkspaceID: workspaceID})
	if err != nil {
		writeError(w, 404, "resource not found")
		return false
	}
	return h.checkDocumentAccess(w, r, issue)
}
func (h *Handler) readableDocuments(r *http.Request, issues []db.Issue) []db.Issue {
	result := make([]db.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.Kind != "doc" || h.documentPermission(r.Context(), h.Queries, issue, requestUserID(r)) != "" {
			result = append(result, issue)
		}
	}
	return result
}
func (h *Handler) checkDocumentAttachment(w http.ResponseWriter, r *http.Request, att db.Attachment) bool {
	if !h.checkDocumentResource(w, r, att.IssueID, att.WorkspaceID) {
		return false
	}
	if att.CommentID.Valid {
		comment, err := h.Queries.GetCommentInWorkspace(r.Context(), db.GetCommentInWorkspaceParams{ID: att.CommentID, WorkspaceID: att.WorkspaceID})
		if err != nil {
			writeError(w, 404, "attachment not found")
			return false
		}
		return h.checkDocumentResource(w, r, comment.IssueID, att.WorkspaceID)
	}
	return true
}

// Execution history may contain document contents. Agent visibility does not
// grant access to the document that a run was working on.
func (h *Handler) readableDocumentTasks(r *http.Request, workspaceID pgtype.UUID, tasks []db.AgentTaskQueue) ([]db.AgentTaskQueue, error) {
	ids := []pgtype.UUID{}
	for _, task := range tasks {
		if task.IssueID.Valid {
			ids = append(ids, task.IssueID)
		}
	}
	if len(ids) == 0 {
		return tasks, nil
	}
	readable, err := h.Queries.ListReadableIssueIDs(r.Context(), db.ListReadableIssueIDsParams{
		WorkspaceID: workspaceID, IssueIds: ids, UserID: parseUUID(requestUserID(r)),
	})
	if err != nil {
		return nil, err
	}
	allowed := make(map[pgtype.UUID]bool, len(readable))
	for _, id := range readable {
		allowed[id] = true
	}
	result := make([]db.AgentTaskQueue, 0, len(tasks))
	for _, task := range tasks {
		if !task.IssueID.Valid || allowed[task.IssueID] {
			result = append(result, task)
		}
	}
	return result, nil
}
