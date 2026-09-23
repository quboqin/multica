package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (h *Handler) publishCollectionAccessChanged(collection db.Collection, previous []pgtype.UUID, actorType, actorID string) {
	readers, err := h.Queries.ListCollectionReaders(context.Background(), db.ListCollectionReadersParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID})
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
	h.Bus.Publish(events.Event{Type: "collection:access_changed", WorkspaceID: uuidToString(collection.WorkspaceID), ActorType: actorType, ActorID: actorID,
		RecipientUserIDs: recipients, Payload: map[string]any{"collection_id": uuidToString(collection.ID)}})
}

func (h *Handler) collectionPermission(ctx context.Context, q *db.Queries, collection db.Collection, userID string) string {
	if userID == "" {
		return ""
	}
	permission, err := q.GetCollectionPermission(ctx, db.GetCollectionPermissionParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID, UserID: parseUUID(userID)})
	if err != nil {
		return ""
	}
	return permission
}

// Collection authorization is independent of workspace administrative roles.
// Reads and writes of records and fields pass through the collection loader.
func (h *Handler) checkCollectionAccess(w http.ResponseWriter, r *http.Request, collection db.Collection) bool {
	userID := requestUserID(r)
	if userID == "" {
		writeError(w, 404, "collection not found")
		return false
	}
	permission := h.collectionPermission(r.Context(), h.Queries, collection, userID)
	if permission == "" {
		writeError(w, 404, "collection not found")
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && permission == "view" {
		writeErrorCode(w, 403, "collection_read_only", "collection is read-only; ask its owner for edit access")
		return false
	}

	return true
}

func (h *Handler) requireCollectionOwner(w http.ResponseWriter, r *http.Request, collection db.Collection) bool {
	if h.collectionPermission(r.Context(), h.Queries, collection, requestUserID(r)) != "owner" {
		writeError(w, 403, "only the collection owner can manage sharing or archive the collection")
		return false
	}
	return true
}

type collectionCollaboratorInput struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *Handler) GetCollectionAccess(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	h.respondCollectionAccess(w, r, collection)
}

func (h *Handler) respondCollectionAccess(w http.ResponseWriter, r *http.Request, collection db.Collection) {
	access, err := h.collectionAccessResponse(r, collection)
	if err != nil {
		writeError(w, 500, "failed to read collection sharing")
		return
	}
	writeJSON(w, 200, access)
}
func (h *Handler) collectionAccessResponse(r *http.Request, collection db.Collection) (map[string]any, error) {
	access, err := h.Queries.GetCollectionAccess(r.Context(), db.GetCollectionAccessParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID})
	if err != nil {
		return nil, err
	}
	permission := h.collectionPermission(r.Context(), h.Queries, collection, requestUserID(r))
	people := []collectionCollaboratorInput{}
	if permission == "owner" {
		rows, err := h.Queries.ListCollectionCollaborators(r.Context(), db.ListCollectionCollaboratorsParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			people = append(people, collectionCollaboratorInput{uuidToString(row.UserID), row.Role})
		}
	}
	return map[string]any{"owner_id": uuidToString(access.OwnerID), "scope": access.Scope, "scope_role": access.ScopeRole, "project_id": uuidToPtr(access.ProjectID), "revision": access.Revision,
		"can_edit": permission == "owner" || permission == "edit", "can_manage": permission == "owner" && !isMachineCredentialActor(r), "collaborators": people}, nil
}

// Sharing changes are independent of table contents and schema.
func (h *Handler) UpdateCollectionAccess(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	if !h.requireCollectionOwner(w, r, collection) {
		return
	}
	if isMachineCredentialActor(r) {
		writeErrorCode(w, 403, "collection_sharing_requires_human", "collection sharing requires a human owner")
		return
	}
	var req struct {
		Scope         string                        `json:"scope"`
		ScopeRole     string                        `json:"scope_role"`
		ProjectID     *string                       `json:"project_id"`
		Collaborators []collectionCollaboratorInput `json:"collaborators"`
		Revision      int64                         `json:"expected_revision"`
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
	collection, err = q.LockCollection(r.Context(), db.LockCollectionParams{ID: collection.ID, WorkspaceID: collection.WorkspaceID})
	if err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	access, err := q.GetCollectionAccess(r.Context(), db.GetCollectionAccessParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read sharing settings")
		return
	}
	if access.Revision != req.Revision {
		writeErrorCode(w, 409, "collection_sharing_conflict", "sharing settings changed; reload before saving")
		return
	}
	if project.Valid {
		if _, err = q.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: project, WorkspaceID: collection.WorkspaceID}); err != nil {
			writeError(w, 400, "project not found")
			return
		}
	}
	for _, id := range ids {
		if id == access.OwnerID {
			writeError(w, 400, "the owner is already a collaborator")
			return
		}
		if _, err = q.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: id, WorkspaceID: collection.WorkspaceID}); err != nil {
			writeError(w, 400, "collaborator must belong to this workspace")
			return
		}
	}
	previous, err := q.ListCollectionReaders(r.Context(), db.ListCollectionReadersParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID})
	if err != nil {
		writeError(w, 500, "failed to read collection audience")
		return
	}
	if err = q.ClearCollectionCollaborators(r.Context(), db.ClearCollectionCollaboratorsParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID}); err != nil {
		writeError(w, 500, "failed to replace collaborators")
		return
	}
	for index, c := range req.Collaborators {
		if err = q.AddCollectionCollaborator(r.Context(), db.AddCollectionCollaboratorParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID, UserID: ids[index], Role: c.Role}); err != nil {
			writeError(w, 500, "failed to add collaborator")
			return
		}
	}
	if _, err = q.UpdateCollectionAccess(r.Context(), db.UpdateCollectionAccessParams{CollectionID: collection.ID, WorkspaceID: collection.WorkspaceID, Scope: req.Scope, ProjectID: project, ScopeRole: req.ScopeRole}); err != nil {
		writeError(w, 500, "failed to update sharing")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit sharing")
		return
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), uuidToString(collection.WorkspaceID))
	// Previous readers must invalidate cached bodies too, including readers
	// whose access was revoked. This event carries no collection content.
	h.publishCollectionAccessChanged(collection, previous, actorType, actorID)
	h.respondCollectionAccess(w, r, collection)
}

func (h *Handler) canReadCollection(ctx context.Context, id, ws pgtype.UUID, user string) bool {
	if user == "" {
		return false
	}
	c, err := h.Queries.GetCollection(ctx, db.GetCollectionParams{WorkspaceID: ws, ID: id})
	return err == nil && h.collectionPermission(ctx, h.Queries, c, user) != ""
}
func (h *Handler) checkCollectionResource(w http.ResponseWriter, r *http.Request, id, ws pgtype.UUID) bool {
	c, err := h.Queries.GetCollection(r.Context(), db.GetCollectionParams{WorkspaceID: ws, ID: id})
	if err != nil {
		writeError(w, 404, "collection not found")
		return false
	}
	return h.checkCollectionAccess(w, r, c)
}
