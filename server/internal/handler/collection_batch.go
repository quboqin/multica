package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/fields"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func validateCollectionMetadata(w http.ResponseWriter, icon, description string) bool {
	if utf8.RuneCountInString(icon) > 32 || utf8.RuneCountInString(description) > 4000 {
		writeError(w, 400, "icon must be at most 32 characters and description at most 4000")
		return false
	}
	return true
}

func (h *Handler) RestoreCollection(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionID"), "collection_id")
	if !ok {
		return
	}
	c, err := h.Queries.GetArchivedCollection(r.Context(), db.GetArchivedCollectionParams{WorkspaceID: ws, ID: id})
	if err != nil {
		writeError(w, 404, "archived collection not found")
		return
	}
	actorType, actorID, ok := h.requireCollectionManager(w, r, c)
	if !ok {
		return
	}
	if c.ProjectID.Valid {
		if _, err = h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: c.ProjectID, WorkspaceID: ws}); err != nil {
			writeError(w, 409, "restore the project before restoring this collection")
			return
		}
	}
	c, err = h.Queries.RestoreCollection(r.Context(), db.RestoreCollectionParams{WorkspaceID: ws, ID: id})
	if err != nil {
		writeError(w, 409, "collection changed; reload and retry")
		return
	}
	h.publish("collection:updated", uuidToString(ws), actorType, actorID, map[string]any{"collection_id": uuidToString(id)})
	writeJSON(w, 200, c)
}

func (h *Handler) ListArchivedCollectionFields(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	items, err := h.Queries.ListArchivedCollectionFields(r.Context(), db.ListArchivedCollectionFieldsParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID})
	if err != nil {
		writeError(w, 500, "failed to list archived fields")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, f := range items {
		out = append(out, collectionFieldResponse(f))
	}
	writeJSON(w, 200, out)
}

func (h *Handler) RestoreCollectionField(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	actorType, actorID, ok := h.requireCollectionManager(w, r, c)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "fieldID"), "field_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to restore field")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if _, err = q.LockCollection(r.Context(), db.LockCollectionParams{WorkspaceID: c.WorkspaceID, ID: c.ID}); err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	live, err := q.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID})
	if err != nil {
		writeError(w, 500, "failed to read fields")
		return
	}
	if len(live) >= 50 {
		writeError(w, 409, "a collection cannot have more than 50 fields")
		return
	}
	f, err := q.RestoreCollectionField(r.Context(), db.RestoreCollectionFieldParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID, ID: id})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, 409, "a live field has the same name; rename it before restoring")
		} else {
			writeError(w, 404, "archived field not found")
		}
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to restore field")
		return
	}
	h.publish("collection:updated", uuidToString(c.WorkspaceID), actorType, actorID, map[string]any{"collection_id": uuidToString(c.ID)})
	writeJSON(w, 200, collectionFieldResponse(f))
}

// Bulk changes lock the catalog and rows in a stable order. Validation or a
// conflict aborts the whole transaction; unrelated field values stay intact.
func (h *Handler) BatchCollectionRecords(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	var req struct {
		Action    string                     `json:"action"`
		RecordIDs []string                   `json:"record_ids"`
		Fields    map[string]json.RawMessage `json:"fields"`
		Expected  map[string]int64           `json:"expected_revisions"`
		Confirmed bool                       `json:"confirmed"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	if len(req.RecordIDs) < 1 || len(req.RecordIDs) > 500 {
		writeError(w, 400, "select between 1 and 500 records")
		return
	}
	if req.Action != "update" && req.Action != "delete" && req.Action != "restore" {
		writeError(w, 400, "action must be update, delete or restore")
		return
	}
	if req.Action == "delete" && !req.Confirmed {
		writeErrorCode(w, 400, "batch_confirmation_required", "confirm moving the selected records to trash")
		return
	}
	ids := make([]pgtype.UUID, 0, len(req.RecordIDs))
	seen := map[string]bool{}
	for _, s := range req.RecordIDs {
		id, ok := parseUUIDOrBadRequest(w, s, "record_id")
		if !ok {
			return
		}
		key := uuidToString(id)
		if seen[key] {
			writeError(w, 400, "duplicate record id")
			return
		}
		seen[key] = true
		ids = append(ids, id)
	}
	if len(req.Expected) > 0 {
		if len(req.Expected) != len(ids) {
			writeError(w, 400, "expected_revisions must cover every selected record")
			return
		}
		for id, rev := range req.Expected {
			if !seen[id] || rev < 1 {
				writeError(w, 400, "invalid expected revision")
				return
			}
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to start batch")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if _, err = q.LockCollection(r.Context(), db.LockCollectionParams{WorkspaceID: c.WorkspaceID, ID: c.ID}); err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	catalog, err := q.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID})
	if err != nil {
		writeError(w, 500, "failed to read fields")
		return
	}
	patch := map[string]json.RawMessage{}
	unset := []string{}
	if req.Action == "update" {
		if len(req.Fields) == 0 {
			writeError(w, 400, "fields must not be empty")
			return
		}
		defs := map[string]db.CollectionField{}
		for _, f := range catalog {
			defs[uuidToString(f.ID)] = f
		}
		for id, value := range req.Fields {
			f, exists := defs[id]
			if !exists || f.Type == fields.TypeRelation {
				writeError(w, 400, "field not found or requires record links")
				return
			}
			if string(value) == "null" {
				unset = append(unset, id)
				continue
			}
			v, err := fields.ValidateValue(fields.Definition{Type: f.Type, Config: f.Config}, value)
			if err != nil {
				writeError(w, 400, f.Name+": "+err.Error())
				return
			}
			if propertyTypeIsActor(f.Type) {
				refs, err := actorRefsInValue(f.Type, v)
				if err != nil {
					writeError(w, 400, err.Error())
					return
				}
				if status, msg := h.resolveActorRefs(r, uuidToString(c.WorkspaceID), refs); status != 0 {
					writeError(w, status, msg)
					return
				}
			}
			patch[id] = v
		}
	} else if len(req.Fields) > 0 {
		writeError(w, 400, "fields apply only to update")
		return
	}
	condition := "deleted_at IS NULL"
	if req.Action == "restore" {
		condition = "deleted_at IS NOT NULL AND deleted_at > now()-interval '30 days'"
	}
	rows, err := tx.Query(r.Context(), "SELECT id,revision FROM record WHERE workspace_id=$1 AND collection_id=$2 AND id=ANY($3::uuid[]) AND "+condition+" ORDER BY id FOR UPDATE", c.WorkspaceID, c.ID, ids)
	if err != nil {
		writeError(w, 500, "failed to lock records")
		return
	}
	count := 0
	conflict := false
	for rows.Next() {
		var id pgtype.UUID
		var rev int64
		if err = rows.Scan(&id, &rev); err != nil {
			break
		}
		count++
		if len(req.Expected) > 0 && req.Expected[uuidToString(id)] != rev {
			conflict = true
		}
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil || readErr != nil {
		writeError(w, 500, "failed to read records")
		return
	}
	if count != len(ids) {
		writeError(w, 404, "one or more records are unavailable; nothing changed")
		return
	}
	if conflict {
		writeErrorCode(w, 409, "record_conflict", "one or more records changed; reload and retry; nothing changed")
		return
	}
	switch req.Action {
	case "update":
		raw, _ := json.Marshal(patch)
		_, err = tx.Exec(r.Context(), "UPDATE record SET fields=(fields || $4::jsonb)-$5::text[],revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND collection_id=$2 AND id=ANY($3::uuid[])", c.WorkspaceID, c.ID, ids, raw, unset)
	case "delete":
		_, err = tx.Exec(r.Context(), "UPDATE record SET deleted_at=now(),revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND collection_id=$2 AND id=ANY($3::uuid[])", c.WorkspaceID, c.ID, ids)
	case "restore":
		_, err = tx.Exec(r.Context(), "UPDATE record SET deleted_at=NULL,revision=revision+1,updated_at=now() WHERE workspace_id=$1 AND collection_id=$2 AND id=ANY($3::uuid[])", c.WorkspaceID, c.ID, ids)
	}
	if err != nil {
		writeError(w, 400, "batch exceeds record limits; nothing changed")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit batch")
		return
	}
	h.publishCollectionBatch(w, r, c)
	writeJSON(w, 200, map[string]any{"count": count})
}

func (h *Handler) publishCollectionBatch(w http.ResponseWriter, r *http.Request, c db.Collection) {
	user, _ := requireUserID(w, r)
	typ, id := h.resolveActor(r, user, uuidToString(c.WorkspaceID))
	h.publish("collection:updated", uuidToString(c.WorkspaceID), typ, id, map[string]any{"collection_id": uuidToString(c.ID)})
}

// CSV cells use existing field names (or IDs). Lists are JSON arrays, avoiding
// lossy splitting when a value contains a comma. Empty cells stay unset.
func csvFieldValue(f db.CollectionField, cell string) (json.RawMessage, error) {
	var value any = cell
	switch f.Type {
	case "relation":
		return nil, fmt.Errorf("use record link for relation fields")
	case "number":
		if err := json.Unmarshal([]byte(cell), &value); err != nil {
			return nil, fmt.Errorf("expected a number")
		}
	case "checkbox":
		v, err := strconv.ParseBool(cell)
		if err != nil {
			return nil, fmt.Errorf("expected true or false")
		}
		value = v
	case "multi_select", "multi_actor":
		var list []string
		if err := json.Unmarshal([]byte(cell), &list); err != nil {
			return nil, fmt.Errorf("expected a JSON array of strings")
		}
		value = list
	}
	if propertyTypeHasOptions(f.Type) {
		lookup := func(s string) (string, error) {
			for _, o := range parsePropertyConfig(f.Config).Options {
				if s == o.ID || s == o.Name {
					return o.ID, nil
				}
			}
			return "", fmt.Errorf("unknown option %q", s)
		}
		if f.Type == "select" {
			v, err := lookup(cell)
			if err != nil {
				return nil, err
			}
			value = v
		} else {
			list := value.([]string)
			for i, s := range list {
				v, err := lookup(s)
				if err != nil {
					return nil, err
				}
				list[i] = v
			}
			value = list
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return fields.ValidateValue(fields.Definition{Type: f.Type, Config: f.Config}, raw)
}

func (h *Handler) ImportCollectionCSV(w http.ResponseWriter, r *http.Request) {
	c, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	var req struct {
		CSV    string `json:"csv"`
		DryRun bool   `json:"dry_run"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32*1024*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, 400, "invalid CSV import request (maximum 32 MiB)")
		return
	}
	if !utf8.ValidString(req.CSV) {
		writeError(w, 400, "CSV must be UTF-8")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to start import")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if _, err = q.LockCollection(r.Context(), db.LockCollectionParams{WorkspaceID: c.WorkspaceID, ID: c.ID}); err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	catalog, err := q.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID})
	if err != nil {
		writeError(w, 500, "failed to read fields")
		return
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(req.CSV, "\ufeff")))
	headers, err := reader.Read()
	if err != nil || len(headers) > 51 {
		writeError(w, 400, "CSV needs a header and at most 51 columns")
		return
	}
	columns := make([]*db.CollectionField, len(headers))
	seen := map[string]bool{}
	titleIndex := -1
	for i, header := range headers {
		header = strings.TrimSpace(header)
		if header == "title" || header == c.TitleName && c.TitleName != "" {
			if titleIndex >= 0 {
				writeError(w, 400, "duplicate title column")
				return
			}
			titleIndex = i
			continue
		}
		var def *db.CollectionField
		for j := range catalog {
			f := &catalog[j]
			if header == f.Name || header == uuidToString(f.ID) {
				if def != nil {
					writeError(w, 400, "ambiguous column: "+header)
					return
				}
				def = f
			}
		}
		if def == nil || seen[uuidToString(def.ID)] {
			writeError(w, 400, "unknown or duplicate CSV column: "+header)
			return
		}
		seen[uuidToString(def.ID)] = true
		columns[i] = def
	}
	if titleIndex < 0 {
		writeError(w, 400, "CSV requires a title column")
		return
	}
	values := [][]any{}
	actorChecked := map[string]bool{}
	for line := 2; ; line++ {
		cells, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			writeError(w, 400, fmt.Sprintf("CSV row %d: %v", line, readErr))
			return
		}
		if len(values) >= 10000 {
			writeError(w, 400, "CSV supports at most 10000 rows per import")
			return
		}
		title := cells[titleIndex]
		if utf8.RuneCountInString(title) > 2048 {
			writeError(w, 400, fmt.Sprintf("CSV row %d: title is too long", line))
			return
		}
		bag := map[string]json.RawMessage{}
		for i, f := range columns {
			if f == nil || cells[i] == "" {
				continue
			}
			v, err := csvFieldValue(*f, cells[i])
			if err != nil {
				writeError(w, 400, fmt.Sprintf("CSV row %d, %s: %v", line, f.Name, err))
				return
			}
			if propertyTypeIsActor(f.Type) {
				key := f.Type + string(v)
				if !actorChecked[key] {
					refs, err := actorRefsInValue(f.Type, v)
					if err != nil {
						writeError(w, 400, err.Error())
						return
					}
					if status, msg := h.resolveActorRefs(r, uuidToString(c.WorkspaceID), refs); status != 0 {
						writeError(w, status, msg)
						return
					}
					actorChecked[key] = true
				}
			}
			bag[uuidToString(f.ID)] = v
		}
		raw, _ := json.Marshal(bag)
		if len(raw) > 60000 {
			writeError(w, 400, fmt.Sprintf("CSV row %d exceeds record limits", line))
			return
		}
		values = append(values, []any{c.WorkspaceID, c.ID, title, raw})
	}
	if len(values) == 0 {
		writeError(w, 400, "CSV contains no records")
		return
	}
	if req.DryRun {
		writeJSON(w, 200, map[string]any{"count": len(values), "dry_run": true})
		return
	}
	if _, err = tx.CopyFrom(r.Context(), pgx.Identifier{"record"}, []string{"workspace_id", "collection_id", "title", "fields"}, pgx.CopyFromRows(values)); err != nil {
		writeError(w, 400, "CSV import failed; nothing imported")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit import")
		return
	}
	h.publishCollectionBatch(w, r, c)
	writeJSON(w, 201, map[string]any{"count": len(values), "dry_run": false})
}

// Missing/archived targets do not contribute a name, count or match.
const collectionLiveLinkJoin = `FROM record_link l
LEFT JOIN issue i ON l.to_type='issue' AND i.id=l.to_id AND i.workspace_id=r.workspace_id AND i.kind='task'
LEFT JOIN record t ON l.to_type='record' AND t.id=l.to_id AND t.workspace_id=r.workspace_id AND t.deleted_at IS NULL
LEFT JOIN collection tc ON tc.id=t.collection_id AND tc.workspace_id=r.workspace_id AND tc.archived_at IS NULL
WHERE l.workspace_id=r.workspace_id AND l.collection_id=r.collection_id AND l.from_record_id=r.id
AND (i.id IS NOT NULL OR (t.id IS NOT NULL AND tc.id IS NOT NULL))`

func (h *Handler) collectionFilterPredicate(w http.ResponseWriter, r *http.Request, c db.Collection, raw string, add func(any) string) (string, bool) {
	if len(raw) > 32768 {
		writeError(w, 400, "properties filter is too large")
		return "", false
	}
	var input map[string][]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &input); err != nil || len(input) > 50 {
		writeError(w, 400, "invalid properties filter")
		return "", false
	}
	predicates := []string{}
	plain := map[string][]json.RawMessage{}
	for key, values := range input {
		id, ok := parseUUIDOrBadRequest(w, key, "field_id")
		if !ok {
			return "", false
		}
		def, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: c.WorkspaceID, CollectionID: c.ID, ID: id})
		if err != nil {
			writeError(w, 400, "filter field not found")
			return "", false
		}
		if def.Type != fields.TypeRelation {
			plain[key] = values
			continue
		}
		if len(values) < 1 || len(values) > 50 {
			writeError(w, 400, "relation filter needs 1 to 50 targets")
			return "", false
		}
		alternatives := []string{}
		for _, value := range values {
			var target string
			if json.Unmarshal(value, &target) != nil {
				writeError(w, 400, "relation filters accept target IDs or __none__")
				return "", false
			}
			predicate := "EXISTS (SELECT 1 " + collectionLiveLinkJoin + " AND l.from_field_id=" + add(id) + "::uuid"
			if target == "__none__" {
				predicate = "NOT " + predicate + ")"
			} else {
				to, ok := parseUUIDOrBadRequest(w, target, "relation_target_id")
				if !ok {
					return "", false
				}
				predicate += " AND l.to_id=" + add(to) + "::uuid)"
			}
			alternatives = append(alternatives, predicate)
		}
		predicates = append(predicates, "("+strings.Join(alternatives, " OR ")+")")
	}
	if len(plain) > 0 {
		encoded, _ := json.Marshal(plain)
		filter, ok := parsePropertiesFilterParam(w, string(encoded))
		if !ok {
			return "", false
		}
		if len(filter) > 0 {
			predicates = append(predicates, propertiesFilterPredicate(filter, add, "r.fields"))
		}
	}
	return strings.Join(predicates, " AND "), true
}
