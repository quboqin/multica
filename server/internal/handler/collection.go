package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/fields"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func collectionRecordResponse(record db.Record) map[string]any {
	return map[string]any{"id": uuidToString(record.ID), "workspace_id": uuidToString(record.WorkspaceID), "collection_id": uuidToString(record.CollectionID), "title": record.Title, "fields": json.RawMessage(record.Fields), "revision": record.Revision, "position": record.Position, "created_at": timestampToString(record.CreatedAt), "updated_at": timestampToString(record.UpdatedAt)}
}
func collectionFieldResponse(field db.CollectionField) map[string]any {
	return map[string]any{"id": uuidToString(field.ID), "workspace_id": uuidToString(field.WorkspaceID), "collection_id": uuidToString(field.CollectionID), "name": field.Name, "type": field.Type, "config": json.RawMessage(field.Config), "position": field.Position}
}
func decodeCollectionBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, 400, "invalid collection request: "+err.Error())
		return false
	}
	if decoder.Decode(new(any)) != io.EOF {
		writeError(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}
func (h *Handler) loadCollection(w http.ResponseWriter, r *http.Request) (db.Collection, bool) {
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return db.Collection{}, false
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionID"), "collection_id")
	if !ok {
		return db.Collection{}, false
	}
	collection, err := h.Queries.GetCollection(r.Context(), db.GetCollectionParams{WorkspaceID: ws, ID: id})
	if err != nil {
		writeError(w, 404, "collection not found")
		return db.Collection{}, false
	}
	return collection, true
}
func (h *Handler) ListCollections(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	collections, err := h.Queries.ListCollections(r.Context(), ws)
	if err != nil {
		writeError(w, 500, "failed to list collections")
		return
	}
	writeJSON(w, 200, collections)
}
func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "collection creation requires a human member")
		return
	}
	var req struct {
		Name      string  `json:"name"`
		ProjectID *string `json:"project_id"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if len([]rune(name)) < 1 || len([]rune(name)) > 80 {
		writeError(w, 400, "name must be 1 to 80 characters")
		return
	}
	var project pgtype.UUID
	if req.ProjectID != nil {
		project, ok = parseUUIDOrBadRequest(w, *req.ProjectID, "project_id")
		if !ok {
			return
		}
		if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: project, WorkspaceID: ws}); err != nil {
			writeError(w, 400, "project not found")
			return
		}
	}
	collection, err := h.Queries.CreateCollection(r.Context(), db.CreateCollectionParams{WorkspaceID: ws, ProjectID: project, Name: name, CreatedBy: parseUUID(user)})
	if err != nil {
		writeError(w, 500, "failed to create collection")
		return
	}
	writeJSON(w, 201, collection)
}
func (h *Handler) GetCollection(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	fields, err := h.Queries.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID})
	if err != nil {
		writeError(w, 500, "failed to list fields")
		return
	}
	result := make([]map[string]any, 0, len(fields))
	for _, field := range fields {
		result = append(result, collectionFieldResponse(field))
	}
	writeJSON(w, 200, map[string]any{"collection": collection, "fields": result, "capabilities": map[string]any{"layouts": []string{"table", "calendar", "gallery"}, "side_effects": false, "max_page_size": 100}})
}
func (h *Handler) CreateCollectionField(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "field definitions require a human actor")
		return
	}
	// A collection creator manages its catalog independently of task-property
	// administration. Workspace administrators may also manage it.
	if uuidToString(collection.CreatedBy) != user {
		if _, ok := h.requireWorkspaceRole(w, r, uuidToString(collection.WorkspaceID), "workspace not found", "owner", "admin"); !ok {
			return
		}
	}
	var req struct {
		Name   string          `json:"name"`
		Type   string          `json:"type"`
		Config *PropertyConfig `json:"config"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	name, err := validateLabelName(req.Name)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if err = validatePropertyType(req.Type); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	config, err := fields.ValidateConfig(req.Type, req.Config, validateLabelName, normalizeColor)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to begin field change")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if _, err = q.LockCollection(r.Context(), db.LockCollectionParams{WorkspaceID: collection.WorkspaceID, ID: collection.ID}); err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	catalog, err := q.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID})
	if err != nil {
		writeError(w, 500, "failed to read fields")
		return
	}
	if len(catalog) >= 50 {
		writeError(w, 400, "a collection may have at most 50 fields")
		return
	}
	field, err := q.CreateCollectionField(r.Context(), db.CreateCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, Name: name, Type: req.Type, Config: config, Position: float64(len(catalog))})
	if err != nil {
		writeError(w, 409, "field name already exists or field is invalid")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit field")
		return
	}
	h.publish("collection:updated", uuidToString(collection.WorkspaceID), "member", user, map[string]any{"collection_id": uuidToString(collection.ID)})
	writeJSON(w, 201, collectionFieldResponse(field))
}
func (h *Handler) CreateCollectionRecord(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	var req struct {
		Title  string                     `json:"title"`
		Fields map[string]json.RawMessage `json:"fields"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	if len([]rune(req.Title)) > 2048 {
		writeError(w, 400, "title is too long")
		return
	}
	initial, ok := h.initialRecordFields(w, r, collection, req.Fields)
	if !ok {
		return
	}
	record, err := h.Queries.CreateCollectionRecord(r.Context(), db.CreateCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, Title: req.Title, Fields: initial})
	if err != nil {
		writeError(w, 500, "failed to create record")
		return
	}
	h.respondCollectionRecord(w, r, record, 201)
}
func (h *Handler) UpdateCollectionRecord(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	var req struct {
		Title string `json:"title"`
		Base  string `json:"title_base"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	if len([]rune(req.Title)) > 2048 {
		writeError(w, 400, "title is too long")
		return
	}
	record, err := h.Queries.UpdateCollectionRecordTitle(r.Context(), db.UpdateCollectionRecordTitleParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id, Title: req.Title, TitleBase: req.Base})
	if err != nil {
		writeError(w, 409, "record changed; reload and retry")
		return
	}
	h.respondCollectionRecord(w, r, record, 200)
}
func (h *Handler) SetCollectionRecordField(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	fieldID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "fieldID"), "field_id")
	if !ok {
		return
	}
	definition, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID})
	if err != nil {
		writeError(w, 404, "field not found")
		return
	}
	var req struct {
		Value         json.RawMessage `json:"value"`
		ExpectedValue json.RawMessage `json:"expected_value"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	unset := string(req.Value) == "null"
	value := []byte("null")
	if !unset {
		value, err = fields.ValidateValue(fields.Definition{Type: definition.Type, Config: definition.Config}, req.Value)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
	}
	if propertyTypeIsActor(definition.Type) && !unset {
		refs, err := actorRefsInValue(definition.Type, value)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if status, msg := h.resolveActorRefs(r, uuidToString(collection.WorkspaceID), refs); status != 0 {
			writeError(w, status, msg)
			return
		}
	}
	compare := len(req.ExpectedValue) > 0
	if !compare {
		req.ExpectedValue = json.RawMessage("null")
	}
	record, err := h.Queries.SetCollectionRecordField(r.Context(), db.SetCollectionRecordFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id, FieldID: uuidToString(fieldID), Value: value, Unset: unset, CompareValue: compare, ExpectedValue: req.ExpectedValue})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 409, "field changed; reload and retry")
		} else {
			writeError(w, 400, "field value exceeds record limits")
		}
		return
	}
	h.respondCollectionRecord(w, r, record, 200)
}
func (h *Handler) respondCollectionRecord(w http.ResponseWriter, r *http.Request, record db.Record, status int) {
	response := collectionRecordResponse(record)
	user, _ := requireUserID(w, r)
	actorType, actorID := h.resolveActor(r, user, uuidToString(record.WorkspaceID))
	h.publish("record:updated", uuidToString(record.WorkspaceID), actorType, actorID, map[string]any{"collection_id": uuidToString(record.CollectionID), "record": response})
	writeJSON(w, status, response)
}

func (h *Handler) ListCollectionRecords(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	args := []any{collection.WorkspaceID, collection.ID}
	add := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	where := []string{"r.workspace_id=$1", "r.collection_id=$2", "r.deleted_at IS NULL"}
	if raw := r.URL.Query().Get("record_id"); raw != "" {
		recordID, ok := parseUUIDOrBadRequest(w, raw, "record_id")
		if !ok {
			return
		}
		where = append(where, "r.id = "+add(recordID))
	}
	if search := r.URL.Query().Get("search"); search != "" {
		where = append(where, "r.title ILIKE "+add("%"+escapeLikePattern(search)+"%"))
	}
	if raw := r.URL.Query().Get("properties"); raw != "" {
		filter, ok := parsePropertiesFilterParam(w, raw)
		if !ok {
			return
		}
		if len(filter) > 0 {
			where = append(where, propertiesFilterPredicate(filter, add, "r.fields"))
		}
	}
	if raw := r.URL.Query().Get("date_field"); raw != "" {
		fieldID, ok := parseUUIDOrBadRequest(w, raw, "date_field")
		if !ok {
			return
		}
		def, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID})
		if err != nil || def.Type != "date" {
			writeError(w, 400, "calendar field must be a date")
			return
		}
		start, end := r.URL.Query().Get("date_start"), r.URL.Query().Get("date_end")
		if _, err = fields.ValidateValue(fields.Definition{Type: "date"}, json.RawMessage(strconv.Quote(start))); err != nil {
			writeError(w, 400, "invalid date_start")
			return
		}
		if _, err = fields.ValidateValue(fields.Definition{Type: "date"}, json.RawMessage(strconv.Quote(end))); err != nil || start > end {
			writeError(w, 400, "invalid date_end")
			return
		}
		column := "r.fields->>" + add(uuidToString(fieldID))
		where = append(where, column+">="+add(start)+" AND "+column+"<="+add(end))
	}
	groupExpr := "''::text"
	if group := r.URL.Query().Get("group_by"); group != "" {
		id, ok := parseUUIDOrBadRequest(w, group, "group_by")
		if !ok {
			return
		}
		def, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id})
		if err != nil || def.Type != "select" {
			writeError(w, 400, "group_by must name a select field")
			return
		}
		groupExpr = "COALESCE(r.fields->>'" + uuidToString(id) + "','__none__')"
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to read records")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
		writeError(w, 500, "failed to read snapshot")
		return
	}
	groupRows, err := tx.Query(r.Context(), "SELECT "+groupExpr+",count(*) FROM record r WHERE "+strings.Join(where, " AND ")+" GROUP BY 1 ORDER BY 1", args...)
	if err != nil {
		writeError(w, 500, "failed to count groups")
		return
	}
	groups := []map[string]any{}
	var total int64
	for groupRows.Next() {
		var key string
		var count int64
		if groupRows.Scan(&key, &count) != nil {
			groupRows.Close()
			writeError(w, 500, "failed to read groups")
			return
		}
		groups = append(groups, map[string]any{"key": key, "count": count})
		total += count
	}
	err = groupRows.Err()
	groupRows.Close()
	if err != nil {
		writeError(w, 500, "failed to count records")
		return
	}
	if r.URL.Query().Has("group_key") {
		where = append(where, groupExpr+"="+add(r.URL.Query().Get("group_key")))
	}
	// Sort parameters join the argument list only after the group count,
	// which must not see placeholders it does not use.
	orderBy := "r.created_at DESC,r.id DESC"
	sorted := false
	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		direction := "ASC"
		switch r.URL.Query().Get("sort_dir") {
		case "", "asc":
		case "desc":
			direction = "DESC"
		default:
			writeError(w, 400, "sort_dir must be asc or desc")
			return
		}
		expr, ok := h.collectionSortExpression(w, r, collection, sortBy, add)
		if !ok {
			return
		}
		orderBy = expr + " " + direction + " NULLS LAST,r.created_at DESC,r.id DESC"
		sorted = true
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, e := strconv.Atoi(raw)
		if e != nil || parsed < 1 || parsed > 100 {
			writeError(w, 400, "limit must be 1 to 100")
			return
		}
		limit = parsed
	}
	fingerprintQuery := r.URL.Query()
	fingerprintQuery.Del("cursor")
	fingerprintQuery.Del("limit")
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(uuidToString(collection.WorkspaceID)+":"+uuidToString(collection.ID)+":"+fingerprintQuery.Encode())))
	type recordCursor struct {
		Fingerprint string `json:"fingerprint"`
		CreatedAt   string `json:"created_at,omitempty"`
		ID          string `json:"id,omitempty"`
		// Offset pages a sorted query. Sorting by an arbitrary field value has
		// no stable keyset, so these cursors are positional.
		Offset int `json:"offset,omitempty"`
	}
	offset := 0
	if encoded := r.URL.Query().Get("cursor"); encoded != "" {
		bytes, err := base64.RawURLEncoding.DecodeString(encoded)
		var cursor recordCursor
		if err != nil || json.Unmarshal(bytes, &cursor) != nil || cursor.Fingerprint != fingerprint {
			writeError(w, 400, "cursor does not match this query")
			return
		}
		if sorted {
			if cursor.Offset < 1 {
				writeError(w, 400, "invalid cursor offset")
				return
			}
			offset = cursor.Offset
		}
		at, err := time.Parse(time.RFC3339Nano, cursor.CreatedAt)
		if sorted {
			err = nil
		}
		if err != nil {
			writeError(w, 400, "invalid cursor date")
			return
		}
		if !sorted {
			recordID, ok := parseUUIDOrBadRequest(w, cursor.ID, "cursor.id")
			if !ok {
				return
			}
			where = append(where, "(r.created_at,r.id)<("+add(at)+","+add(recordID)+")")
		}
	}
	rows, err := tx.Query(r.Context(), "SELECT r.id,r.workspace_id,r.collection_id,r.title,r.fields,r.revision,r.position,r.created_at,r.updated_at FROM record r WHERE "+strings.Join(where, " AND ")+" ORDER BY "+orderBy+" LIMIT "+add(limit+1)+" OFFSET "+add(offset), args...)
	if err != nil {
		writeError(w, 500, "failed to list records")
		return
	}
	defer rows.Close()
	records := []map[string]any{}
	cursors := []recordCursor{}
	for rows.Next() {
		var record db.Record
		if err = rows.Scan(&record.ID, &record.WorkspaceID, &record.CollectionID, &record.Title, &record.Fields, &record.Revision, &record.Position, &record.CreatedAt, &record.UpdatedAt); err != nil {
			writeError(w, 500, "failed to decode record")
			return
		}
		records = append(records, collectionRecordResponse(record))
		cursors = append(cursors, recordCursor{Fingerprint: fingerprint, CreatedAt: record.CreatedAt.Time.Format(time.RFC3339Nano), ID: uuidToString(record.ID)})
	}
	if rows.Err() != nil {
		writeError(w, 500, "failed to read records")
		return
	}
	more := len(records) > limit
	if more {
		records = records[:limit]
	}
	var next *string
	if more {
		cursor := cursors[limit-1]
		if sorted {
			cursor = recordCursor{Fingerprint: fingerprint, Offset: offset + limit}
		}
		raw, _ := json.Marshal(cursor)
		encoded := base64.RawURLEncoding.EncodeToString(raw)
		next = &encoded
	}
	writeJSON(w, 200, map[string]any{"records": records, "total": total, "groups": groups, "next_cursor": next})
}

// collectionSortExpression orders by title, creation time, or one field.
// Select options sort in their catalog order; numbers and checkboxes by value;
// every other type by its stored text.
func (h *Handler) collectionSortExpression(w http.ResponseWriter, r *http.Request, collection db.Collection, sortBy string, add func(any) string) (string, bool) {
	switch sortBy {
	case "title":
		return "lower(r.title)", true
	case "created_at":
		return "r.created_at", true
	}
	id, ok := parseUUIDOrBadRequest(w, sortBy, "sort_by")
	if !ok {
		return "", false
	}
	def, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id})
	if err != nil {
		writeError(w, 400, "sort_by must name a field")
		return "", false
	}
	key := add(uuidToString(id))
	switch def.Type {
	case "number":
		return "CASE WHEN jsonb_typeof(r.fields->" + key + ")='number' THEN (r.fields->>" + key + ")::numeric END", true
	case "checkbox":
		return "CASE WHEN jsonb_typeof(r.fields->" + key + ")='boolean' THEN (r.fields->>" + key + ")::boolean END", true
	case "select":
		ids := []string{}
		for _, option := range parsePropertyConfig(def.Config).Options {
			ids = append(ids, option.ID)
		}
		return "array_position(" + add(ids) + "::text[],r.fields->>" + key + ")", true
	case "multi_select", "multi_actor":
		return "(r.fields->" + key + ")->>0", true
	}
	return "r.fields->>" + key, true
}
