package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/fields"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Relation fields (FR-028). A relation's values are edges in record_link, not
// entries in record.fields: the target side can then list what points at it,
// and an edge survives its target so a deleted task reads as "deleted" in the
// cell instead of silently vanishing.
//
// Contract highlights:
//   - A relation field names its target once, at creation: workspace tasks, or
//     the records of one collection. The target never changes afterwards.
//   - Edges are written by everyone who may edit the record, through their own
//     endpoints. The value write path rejects relation fields.
//   - Every record payload carries its edges under "links", keyed by field id,
//     each with what its target looks like now.

// maxRecordLinksPerField caps one cell. A relation is a handful of references,
// and every listed record resolves all of its edges.
const maxRecordLinksPerField = 50

type recordLinkResponse struct {
	ID     string `json:"id"`
	ToType string `json:"to_type"`
	ToID   string `json:"to_id"`
	// Title is the task's or the record's current title; empty once missing.
	Title string `json:"title"`
	// Identifier and Status are set for task targets only.
	Identifier string `json:"identifier,omitempty"`
	Status     string `json:"status,omitempty"`
	// CollectionID is the target record's collection.
	CollectionID string `json:"collection_id,omitempty"`
	// Missing marks an edge whose target was deleted, trashed or archived away.
	Missing bool `json:"missing"`
}

type recordBacklinkResponse struct {
	ID             string `json:"id"`
	CollectionID   string `json:"collection_id"`
	CollectionName string `json:"collection_name"`
	RecordID       string `json:"record_id"`
	RecordTitle    string `json:"record_title"`
	FieldID        string `json:"field_id"`
	FieldName      string `json:"field_name"`
}

// validateCollectionFieldType admits the shared property types plus the
// collection-only relation and formula types.
func validateCollectionFieldType(t string) error {
	if t == fields.TypeFormula || t == fields.TypeRelation || validatePropertyType(t) == nil {
		return nil
	}
	return fmt.Errorf("invalid type %q; valid types: %s, %s, %s", t, strings.Join(validPropertyTypes, ", "), fields.TypeRelation, fields.TypeFormula)
}

// relationFieldConfig canonicalizes a new relation field's target and checks
// that a named collection is a live one in this workspace.
func (h *Handler) relationFieldConfig(ctx context.Context, collection db.Collection, cfg *PropertyConfig, userID string) ([]byte, error) {
	relation, err := fields.ValidateRelationConfig(cfg)
	if err != nil {
		return nil, err
	}
	if relation.ToType == fields.RelationToRecord {
		if target, err := h.Queries.GetCollection(ctx, db.GetCollectionParams{WorkspaceID: collection.WorkspaceID, ID: parseUUID(relation.CollectionID)}); err != nil || h.collectionPermission(ctx, h.Queries, target, userID) == "" {
			return nil, errors.New("the collection to link to was not found")
		}
	}
	return json.Marshal(PropertyConfig{Relation: &relation})
}

// attachRecordLinks adds "links" to each record payload in one query for the
// whole page. Records without edges get an empty object, so a reader can tell
// "no links" from a server that does not send them.
func (h *Handler) attachRecordLinks(ctx context.Context, q *db.Queries, userID string, workspaceID pgtype.UUID, ids []pgtype.UUID, responses []map[string]any) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.ListRecordLinks(ctx, db.ListRecordLinksParams{WorkspaceID: workspaceID, RecordIds: ids, UserID: parseUUID(userID)})
	if err != nil {
		return err
	}
	prefix, prefixLoaded := "", false
	grouped := map[string]map[string][]recordLinkResponse{}
	for _, row := range rows {
		link := recordLinkResponse{ID: uuidToString(row.ID), ToType: row.ToType, ToID: uuidToString(row.ToID), Missing: true}
		switch {
		case row.ToType == fields.RelationToIssue && row.IssueNumber.Valid:
			if !prefixLoaded {
				prefix, prefixLoaded = h.getIssuePrefix(ctx, workspaceID), true
			}
			link.Missing = false
			link.Title = row.IssueTitle.String
			link.Status = row.IssueStatus.String
			link.Identifier = prefix + "-" + strconv.Itoa(int(row.IssueNumber.Int32))
		case row.ToType == fields.RelationToRecord && row.RecordCollectionID.Valid:
			link.Missing = false
			link.Title = row.RecordTitle.String
			link.CollectionID = uuidToString(row.RecordCollectionID)
		}
		record, field := uuidToString(row.FromRecordID), uuidToString(row.FromFieldID)
		if grouped[record] == nil {
			grouped[record] = map[string][]recordLinkResponse{}
		}
		grouped[record][field] = append(grouped[record][field], link)
	}
	for _, response := range responses {
		id, _ := response["id"].(string)
		if links, ok := grouped[id]; ok {
			response["links"] = links
		} else {
			response["links"] = map[string][]recordLinkResponse{}
		}
	}
	return nil
}

func (h *Handler) CreateCollectionRecordLink(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	recordID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	var req struct {
		FieldID string `json:"field_id"`
		ToID    string `json:"to_id"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	fieldID, ok := parseUUIDOrBadRequest(w, req.FieldID, "field_id")
	if !ok {
		return
	}
	toID, ok := parseUUIDOrBadRequest(w, req.ToID, "to_id")
	if !ok {
		return
	}
	definition, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID})
	if err != nil {
		writeError(w, 404, "field not found")
		return
	}
	relation, isRelation := fields.ParseRelationConfig(definition.Config)
	if definition.Type != fields.TypeRelation || !isRelation {
		writeError(w, 400, "field is not a relation")
		return
	}
	// The target is checked against the field's own definition, never against
	// anything the caller names: a relation to one table cannot be pointed at
	// another, and nothing reaches across workspaces.
	switch relation.ToType {
	case fields.RelationToIssue:
		issue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: toID, WorkspaceID: collection.WorkspaceID})
		if err != nil {
			writeError(w, 400, "task not found")
			return
		}
		if issue.Kind != "task" {
			writeError(w, 400, "only tasks can be linked")
			return
		}
	case fields.RelationToRecord:
		if toID == recordID {
			writeError(w, 400, "a record cannot link to itself")
			return
		}
		target, err := util.ParseUUID(relation.CollectionID)
		if err != nil {
			writeError(w, 400, "relation target is not supported")
			return
		}
		targetCollection, err := h.Queries.GetCollection(r.Context(), db.GetCollectionParams{WorkspaceID: collection.WorkspaceID, ID: target})
		if err != nil || h.collectionPermission(r.Context(), h.Queries, targetCollection, requestUserID(r)) == "" {
			writeError(w, 404, "linked collection not found")
			return
		}
		if _, err := h.Queries.GetLiveCollectionRecord(r.Context(), db.GetLiveCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: target, ID: toID}); err != nil {
			writeError(w, 400, "record not found in the linked collection")
			return
		}
	default:
		writeError(w, 400, "relation target is not supported")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin link change")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	record, err := q.LockCollectionRecord(r.Context(), db.LockCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: recordID})
	if err != nil {
		writeError(w, 404, "record not found")
		return
	}
	// Linking is idempotent: a second request for the same edge answers with
	// the record as it stands.
	_, err = q.GetRecordLinkEdge(r.Context(), db.GetRecordLinkEdgeParams{WorkspaceID: collection.WorkspaceID, FromRecordID: recordID, FromFieldID: fieldID, ToType: relation.ToType, ToID: toID})
	if err == nil {
		tx.Rollback(r.Context())
		h.respondCollectionRecord(w, r, record, 200)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "failed to read links")
		return
	}
	count, err := q.CountRecordFieldLinks(r.Context(), db.CountRecordFieldLinksParams{WorkspaceID: collection.WorkspaceID, FromRecordID: recordID, FromFieldID: fieldID})
	if err != nil {
		writeError(w, 500, "failed to count links")
		return
	}
	if count >= maxRecordLinksPerField {
		writeError(w, 400, fmt.Sprintf("a relation cell holds at most %d links", maxRecordLinksPerField))
		return
	}
	if _, err = q.CreateRecordLink(r.Context(), db.CreateRecordLinkParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, FromRecordID: recordID, FromFieldID: fieldID, ToType: relation.ToType, ToID: toID}); err != nil {
		writeError(w, 500, "failed to create link")
		return
	}
	if record, err = q.TouchCollectionRecord(r.Context(), db.TouchCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: recordID}); err != nil {
		writeError(w, 500, "failed to update record")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit link")
		return
	}
	h.respondCollectionRecord(w, r, record, 201)
}

func (h *Handler) DeleteCollectionRecordLink(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	recordID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	linkID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "linkID"), "link_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin link change")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	record, err := q.LockCollectionRecord(r.Context(), db.LockCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: recordID})
	if err != nil {
		writeError(w, 404, "record not found")
		return
	}
	removed, err := q.DeleteRecordLink(r.Context(), db.DeleteRecordLinkParams{WorkspaceID: collection.WorkspaceID, FromRecordID: recordID, ID: linkID})
	if err != nil {
		writeError(w, 500, "failed to remove link")
		return
	}
	// Unlinking twice is not an error: the second request finds nothing to
	// remove and answers with the record as it stands.
	if removed > 0 {
		if record, err = q.TouchCollectionRecord(r.Context(), db.TouchCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: recordID}); err != nil {
			writeError(w, 500, "failed to update record")
			return
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit link")
		return
	}
	h.respondCollectionRecord(w, r, record, 200)
}

// ListIssueRecordLinks answers "which records point at this task".
func (h *Handler) ListIssueRecordLinks(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	h.respondBacklinks(w, r, issue.WorkspaceID, fields.RelationToIssue, issue.ID)
}

// ListCollectionRecordBacklinks answers "which records point at this record".
func (h *Handler) ListCollectionRecordBacklinks(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	recordID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	if _, err := h.Queries.GetLiveCollectionRecord(r.Context(), db.GetLiveCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: recordID}); err != nil {
		writeError(w, 404, "record not found")
		return
	}
	h.respondBacklinks(w, r, collection.WorkspaceID, fields.RelationToRecord, recordID)
}

func (h *Handler) respondBacklinks(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID, toType string, toID pgtype.UUID) {
	rows, err := h.Queries.ListRecordLinksTo(r.Context(), db.ListRecordLinksToParams{WorkspaceID: workspaceID, ToType: toType, ToID: toID, UserID: parseUUID(requestUserID(r))})
	if err != nil {
		slog.Warn("list record backlinks failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, 500, "failed to list links")
		return
	}
	links := make([]recordBacklinkResponse, 0, len(rows))
	for _, row := range rows {
		links = append(links, recordBacklinkResponse{
			ID: uuidToString(row.ID), CollectionID: uuidToString(row.CollectionID), CollectionName: row.CollectionName,
			RecordID: uuidToString(row.FromRecordID), RecordTitle: row.RecordTitle,
			FieldID: uuidToString(row.FromFieldID), FieldName: row.FieldName,
		})
	}
	writeJSON(w, 200, map[string]any{"links": links})
}
