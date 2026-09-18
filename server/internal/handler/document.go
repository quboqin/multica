package handler

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var errDocumentConflict = errors.New("document body version changed")

func writeDocumentConflict(w http.ResponseWriter, current db.Issue, code string) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": "Document changed. Keep your draft, compare with the current body, then retry with its document_revision.",
		"code":  code, "document_revision": current.DocumentRevision, "description": textToPtr(current.Description),
	})
}

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	var project pgtype.UUID
	if raw := r.URL.Query().Get("project_id"); raw != "" {
		project, ok = parseUUIDOrBadRequest(w, raw, "project_id")
		if !ok {
			return
		}
	}
	docs, err := h.Queries.ListDocuments(r.Context(), db.ListDocumentsParams{WorkspaceID: ws, ProjectID: project})
	if err != nil {
		writeError(w, 500, "failed to list documents")
		return
	}
	result := make([]IssueResponse, 0, len(docs))
	prefix := h.getIssuePrefix(r.Context(), ws)
	for _, doc := range docs {
		resp := issueToResponse(doc, prefix)
		h.fillStatusCategory(r.Context(), ws, &resp)
		result = append(result, resp)
	}
	writeJSON(w, 200, result)
}

// Tree moves serialize per workspace, so two simultaneous reparentings cannot
// each validate the other's old ancestry and commit a cycle.
func (h *Handler) MoveDocument(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return
	}
	var req struct {
		ParentID *string `json:"parent_issue_id"`
		BeforeID *string `json:"before_id"`
		Position float64 `json:"position"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil || math.IsNaN(req.Position) || math.IsInf(req.Position, 0) {
		writeError(w, 400, "invalid move")
		return
	}
	var parent pgtype.UUID
	if req.ParentID != nil {
		parent, ok = parseUUIDOrBadRequest(w, *req.ParentID, "parent_issue_id")
		if !ok {
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin move")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if err = q.LockDocumentTree(r.Context(), uuidToString(doc.WorkspaceID)); err != nil {
		writeError(w, 500, "failed to lock tree")
		return
	}
	docs, err := q.ListDocuments(r.Context(), db.ListDocumentsParams{WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read tree")
		return
	}
	nodes := make(map[pgtype.UUID]db.Issue, len(docs))
	for _, d := range docs {
		nodes[d.ID] = d
	}
	current, exists := nodes[doc.ID]
	if !exists {
		writeError(w, 404, "document not found")
		return
	}
	if parent.Valid {
		target, exists := nodes[parent]
		if !exists || target.ProjectID != current.ProjectID {
			writeError(w, 400, "parent must be a document in the same project")
			return
		}
		for ancestor := parent; ancestor.Valid; ancestor = nodes[ancestor].ParentIssueID {
			if ancestor == doc.ID {
				writeError(w, 400, "a document cannot be moved into its own subtree")
				return
			}
		}
	}
	siblings := make([]db.Issue, 0)
	for _, item := range docs {
		if item.ID != current.ID && item.ParentIssueID == parent && item.ProjectID == current.ProjectID {
			siblings = append(siblings, item)
		}
	}
	insertAt := len(siblings)
	if req.BeforeID != nil {
		before, valid := parseUUIDOrBadRequest(w, *req.BeforeID, "before_id")
		if !valid {
			return
		}
		found := false
		for index, sibling := range siblings {
			if sibling.ID == before {
				insertAt = index
				found = true
				break
			}
		}
		if !found {
			writeError(w, 400, "before_id must be a sibling in the destination")
			return
		}
	} else {
		for index, sibling := range siblings {
			if sibling.Position >= req.Position {
				insertAt = index
				break
			}
		}
	}
	siblings = append(siblings, db.Issue{})
	copy(siblings[insertAt+1:], siblings[insertAt:])
	siblings[insertAt] = current
	var moved db.Issue
	for index, sibling := range siblings {
		position := float64((index + 1) * 1024)
		if sibling.ID != current.ID && sibling.Position == position {
			continue
		}
		updated, err := q.MoveDocument(r.Context(), db.MoveDocumentParams{ID: sibling.ID, WorkspaceID: doc.WorkspaceID, ParentIssueID: parent, Position: position})
		if err != nil {
			writeError(w, 500, "failed to move document")
			return
		}
		if sibling.ID == current.ID {
			moved = updated
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit move")
		return
	}
	h.respondDocument(w, r, moved, user)
}

func (h *Handler) TransitionDocument(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "document approval requires a human actor")
		return
	}
	var req struct {
		Action   string `json:"action"`
		Revision int64  `json:"expected_document_revision"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req) != nil {
		writeError(w, 400, "invalid transition")
		return
	}
	status := ""
	switch req.Action {
	case "review":
		status = "reviewing"
	case "publish":
		status = "published"
	case "draft":
		status = "draft"
	default:
		writeError(w, 400, "action must be review, publish, or draft")
		return
	}
	if req.Action == "publish" {
		if _, ok := h.requireWorkspaceRole(w, r, uuidToString(doc.WorkspaceID), "workspace not found", "owner", "admin"); !ok {
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin transition")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if err = assertIssueStatusStillActive(r.Context(), q, doc.WorkspaceID, status); err != nil {
		writeError(w, 409, "document status is unavailable; restore the document status in workspace settings")
		return
	}
	entry, err := q.GetIssueStatusEntryByKey(r.Context(), db.GetIssueStatusEntryByKeyParams{WorkspaceID: doc.WorkspaceID, Key: status})
	category := map[string]string{"draft": "unstarted", "reviewing": "started", "published": "done"}[status]
	if err != nil || entry.Category != category {
		writeError(w, 409, "document status has an incompatible lifecycle category")
		return
	}
	doc, err = q.LockIssueForDescriptionUpdate(r.Context(), db.LockIssueForDescriptionUpdateParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID})
	if err != nil {
		writeError(w, 404, "document not found")
		return
	}
	if req.Revision != doc.DocumentRevision {
		writeDocumentConflict(w, doc, "document_conflict")
		return
	}
	if req.Action == "publish" && doc.Status != "reviewing" {
		writeError(w, 409, "submit this version for review before publishing")
		return
	}
	if req.Action != "publish" {
		if err = q.CancelDocumentIngestion(r.Context(), db.CancelDocumentIngestionParams{WorkspaceID: doc.WorkspaceID, IssueID: doc.ID}); err != nil {
			writeError(w, 500, "failed to withdraw ingestion")
			return
		}
	}
	if err = q.AuthorizeDocumentTransition(r.Context(), uuidToString(doc.ID)); err != nil {
		writeError(w, 500, "failed to authorize transition")
		return
	}
	updated, err := q.TransitionDocument(r.Context(), db.TransitionDocumentParams{ID: doc.ID, WorkspaceID: doc.WorkspaceID, Status: status})
	if err != nil {
		writeError(w, 500, "failed to transition document")
		return
	}
	ingestion := "not_requested"
	if req.Action == "publish" {
		ingestion = "pending"
	}
	err = q.RecordDocumentTransition(r.Context(), db.RecordDocumentTransitionParams{IssueID: doc.ID, WorkspaceID: doc.WorkspaceID, DocumentRevision: doc.DocumentRevision, ActorID: parseUUID(user), Action: req.Action, Body: doc.Description.String, IngestionState: ingestion})
	if err != nil {
		writeError(w, 500, "failed to record approval")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit transition")
		return
	}
	h.respondDocument(w, r, updated, user)
}

func (h *Handler) respondDocument(w http.ResponseWriter, r *http.Request, doc db.Issue, user string) {
	resp := issueToResponse(doc, h.getIssuePrefix(r.Context(), doc.WorkspaceID))
	h.fillStatusCategory(r.Context(), doc.WorkspaceID, &resp)
	actorType, actorID := h.resolveActor(r, user, uuidToString(doc.WorkspaceID))
	h.publish(protocol.EventIssueUpdated, uuidToString(doc.WorkspaceID), actorType, actorID, map[string]any{"issue": resp})
	writeJSON(w, 200, resp)
}
