package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// All group counts and branch rows use the same expression and snapshot.
func (h *Handler) collectionGroupExpression(w http.ResponseWriter, r *http.Request, tx pgx.Tx, collection db.Collection, where []string, args *[]any) (string, map[string]any, bool) {
	group := r.URL.Query().Get("group_by")
	values := map[string]any{}
	if group == "" {
		return "''::text", values, true
	}
	if group == "title" {
		return "CASE WHEN r.title = '' THEN '__none__' ELSE 'value:' || to_jsonb(r.title)::text END", values, true
	}
	id, ok := parseUUIDOrBadRequest(w, group, "group_by")
	if !ok {
		return "", nil, false
	}
	q := h.Queries.WithTx(tx)
	def, err := q.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id})
	if err != nil {
		writeError(w, 400, "group_by must name an active field")
		return "", nil, false
	}
	key := "'" + uuidToString(id) + "'"
	if def.Type == "select" {
		return "COALESCE(NULLIF(r.fields->>" + key + ",''),'__none__')", values, true
	}
	if def.Type != "formula" && def.Type != "relation" {
		expr := "r.fields->" + key
		if def.Type == "multi_select" || def.Type == "multi_actor" {
			expr = "(SELECT jsonb_agg(v ORDER BY v) FROM jsonb_array_elements_text(CASE WHEN jsonb_typeof(" + expr + ") = 'array' THEN " + expr + " ELSE '[]'::jsonb END) v)"
		}
		return "CASE WHEN " + expr + " IS NULL OR " + expr + " IN ('null'::jsonb, '\"\"'::jsonb, '[]'::jsonb) THEN '__none__' ELSE 'value:' || (" + expr + ")::text END", values, true
	}
	// Computed cells live outside record.fields. Evaluate the filtered set
	// before pagination, using the existing formula and permission-aware link
	// readers. Only this explicit grouping path pays the full-set read cost.
	rows, err := tx.Query(r.Context(), "SELECT r.id,r.title,r.fields FROM record r WHERE "+strings.Join(where, " AND "), (*args)...)
	if err != nil {
		writeError(w, 500, "failed to read grouped values")
		return "", nil, false
	}
	records := []map[string]any{}
	ids := []pgtype.UUID{}
	for rows.Next() {
		var id pgtype.UUID
		var title string
		var raw []byte
		if err = rows.Scan(&id, &title, &raw); err != nil {
			break
		}
		ids = append(ids, id)
		records = append(records, map[string]any{"id": uuidToString(id), "title": title, "fields": json.RawMessage(raw)})
	}
	rowErr := rows.Err()
	rows.Close()
	if err != nil || rowErr != nil {
		writeError(w, 500, "failed to read grouped values")
		return "", nil, false
	}
	if def.Type == "formula" {
		err = h.attachFormulaValues(r.Context(), q, collection.WorkspaceID, collection.ID, records)
	} else {
		err = h.attachRecordLinks(r.Context(), q, requestUserID(r), collection.WorkspaceID, ids, records)
	}
	if err != nil {
		writeError(w, 500, "failed to resolve grouped values")
		return "", nil, false
	}
	keys := map[string]string{}
	for _, record := range records {
		var value any
		if def.Type == "formula" {
			value = record["fields"].(map[string]any)[group]
		} else {
			links := record["links"].(map[string][]recordLinkResponse)[group]
			sort.Slice(links, func(i, j int) bool { return links[i].ToID < links[j].ToID })
			targets := []string{}
			for _, link := range links {
				targets = append(targets, link.ToType+":"+link.ToID)
			}
			if len(targets) > 0 {
				value = targets
			}
			raw, _ := json.Marshal(value)
			values["value:"+string(raw)] = links
		}
		groupKey := "__none__"
		if value != nil && value != "" {
			raw, _ := json.Marshal(value)
			groupKey = "value:" + string(raw)
		}
		keys[record["id"].(string)] = groupKey
	}
	raw, _ := json.Marshal(keys)
	*args = append(*args, string(raw))
	return fmt.Sprintf("($%d::jsonb ->> r.id::text)", len(*args)), values, true
}
