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

// documentTransitionRequiresHuman is the stable code on a lifecycle change
// refused because the caller holds a machine credential.
const documentTransitionRequiresHuman = "document_transition_requires_human"

func writeDocumentConflict(w http.ResponseWriter, current db.Issue, code string) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": "Document changed. Keep your draft, compare with the current body, then retry with its document_revision.",
		"code":  code, "document_revision": current.DocumentRevision, "description": textToPtr(current.Description),
	})
}

func (h *Handler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(w, r); !ok {
		return
	}
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
	docs, err := h.Queries.ListDocuments(r.Context(), db.ListDocumentsParams{WorkspaceID: ws, ProjectID: project, UserID: parseUUID(requestUserID(r))})
	if err != nil {
		writeError(w, 500, "failed to list documents")
		return
	}
	result := make([]IssueResponse, 0, len(docs))
	prefix := h.getIssuePrefix(r.Context(), ws)
	for _, doc := range docs {
		resp := issueToResponse(doc.Issue, prefix)
		resp.DocumentOwnerID = uuidToString(doc.OwnerID)
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
	if !h.requireDocumentOwner(w, r, doc) {
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
	docs, err := q.ListOwnedDocuments(r.Context(), db.ListOwnedDocumentsParams{WorkspaceID: doc.WorkspaceID, OwnerID: parseUUID(user)})
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

// TransitionDocument keeps installed clients from accidentally publishing to an
// implicit audience. Sharing requires an explicit, versioned access update.
func (h *Handler) TransitionDocument(w http.ResponseWriter, r *http.Request) {
	doc, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	if doc.Kind != "doc" {
		writeError(w, 400, "not a document")
		return
	}
	if isMachineCredentialActor(r) {
		writeErrorCode(w, 403, documentTransitionRequiresHuman, "document sharing requires a human owner")
		return
	}
	writeErrorCode(w, 400, "document_sharing_required", "review has been removed; publish with explicit sharing settings")
}

func (h *Handler) respondDocument(w http.ResponseWriter, r *http.Request, doc db.Issue, user string) {
	resp := issueToResponse(doc, h.getIssuePrefix(r.Context(), doc.WorkspaceID))
	h.fillStatusCategory(r.Context(), doc.WorkspaceID, &resp)
	actorType, actorID := h.resolveActor(r, user, uuidToString(doc.WorkspaceID))
	h.publish(protocol.EventIssueUpdated, uuidToString(doc.WorkspaceID), actorType, actorID, map[string]any{"issue": resp})
	writeJSON(w, 200, resp)
}
