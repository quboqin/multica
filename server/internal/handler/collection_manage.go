package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/fields"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// requireCollectionManager admits the collection creator and workspace
// owners/admins. Machine credentials never manage a catalog.
func (h *Handler) requireCollectionManager(w http.ResponseWriter, r *http.Request, collection db.Collection) (string, bool) {
	user, ok := requireUserID(w, r)
	if !ok {
		return "", false
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "field definitions require a human actor")
		return "", false
	}
	if uuidToString(collection.CreatedBy) != user {
		if _, ok := h.requireWorkspaceRole(w, r, uuidToString(collection.WorkspaceID), "workspace not found", "owner", "admin"); !ok {
			return "", false
		}
	}
	return user, true
}

// collectionFieldConversion names the value rewrite a type change needs, or
// reports that the change is unsafe. Unsafe conversions require a new field.
func collectionFieldConversion(from, to string) (string, bool) {
	if from == to {
		return "", true
	}
	switch {
	case from == "select" && to == "multi_select", from == "actor" && to == "multi_actor":
		return "wrap", true
	case from == "multi_select" && to == "select", from == "multi_actor" && to == "actor":
		return "first", true
	case to == "text" && (from == "number" || from == "date" || from == "url" || from == "checkbox"):
		return "text", true
	case from == "text" && to == "url":
		return "", false
	}
	return "", false
}

func (h *Handler) UpdateCollection(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	user, ok := h.requireCollectionManager(w, r, collection)
	if !ok {
		return
	}
	var req struct {
		Name      *string `json:"name"`
		TitleName *string `json:"title_name"`
		Archived  *bool   `json:"archived"`
	}
	if !decodeCollectionBody(w, r, &req) {
		return
	}
	params := db.UpdateCollectionParams{WorkspaceID: collection.WorkspaceID, ID: collection.ID, Archive: req.Archived != nil && *req.Archived}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if len([]rune(name)) < 1 || len([]rune(name)) > 80 {
			writeError(w, 400, "name must be 1 to 80 characters")
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.TitleName != nil {
		// The title column is not a catalog field, so its label lives on the
		// table. Empty returns it to the client's localized default.
		title := strings.TrimSpace(*req.TitleName)
		if title != "" {
			validated, err := validateLabelName(*req.TitleName)
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
			title = validated
		}
		params.TitleName = pgtype.Text{String: title, Valid: true}
	}
	updated, err := h.Queries.UpdateCollection(r.Context(), params)
	if err != nil {
		writeError(w, 404, "collection not found")
		return
	}
	h.publish("collection:updated", uuidToString(collection.WorkspaceID), "member", user, map[string]any{"collection_id": uuidToString(collection.ID)})
	writeJSON(w, 200, updated)
}

func (h *Handler) UpdateCollectionField(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	user, ok := h.requireCollectionManager(w, r, collection)
	if !ok {
		return
	}
	fieldID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "fieldID"), "field_id")
	if !ok {
		return
	}
	var req struct {
		Name     *string         `json:"name"`
		Type     *string         `json:"type"`
		Config   *PropertyConfig `json:"config"`
		Position *float64        `json:"position"`
		Archived *bool           `json:"archived"`
	}
	if !decodeCollectionBody(w, r, &req) {
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
	existing, err := q.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID})
	if err != nil {
		writeError(w, 404, "field not found")
		return
	}
	params := db.UpdateCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID, Archive: req.Archived != nil && *req.Archived}
	if req.Name != nil {
		name, err := validateLabelName(*req.Name)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Position != nil {
		params.Position = pgtype.Float8{Float64: *req.Position, Valid: true}
	}
	targetType := existing.Type
	conversion := ""
	if req.Type != nil && *req.Type != existing.Type {
		if err = validatePropertyType(*req.Type); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		mode, safe := collectionFieldConversion(existing.Type, *req.Type)
		if !safe {
			writeError(w, 400, "changing "+existing.Type+" to "+*req.Type+" could lose values; create a new field instead")
			return
		}
		targetType, conversion = *req.Type, mode
		params.Type = pgtype.Text{String: targetType, Valid: true}
	}
	var nextConfig []byte
	switch {
	case req.Config != nil:
		nextConfig, err = fields.ValidateConfig(targetType, req.Config, validateLabelName, normalizeColor)
	case targetType != existing.Type:
		if propertyTypeHasOptions(targetType) {
			current := parsePropertyConfig(existing.Config)
			nextConfig, err = fields.ValidateConfig(targetType, &current, validateLabelName, normalizeColor)
		} else {
			nextConfig = []byte(`{}`)
		}
	}
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if nextConfig != nil {
		params.Config = nextConfig
		if removed := removedOptionIDs(existing.Config, nextConfig); len(removed) > 0 {
			if _, err = q.StripCollectionFieldOptions(r.Context(), db.StripCollectionFieldOptionsParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, FieldID: uuidToString(fieldID), Removed: removed}); err != nil {
				writeError(w, 500, "failed to clear removed options")
				return
			}
		}
	}
	if conversion != "" {
		if _, err = q.ConvertCollectionFieldValues(r.Context(), db.ConvertCollectionFieldValuesParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, FieldID: uuidToString(fieldID), Mode: conversion}); err != nil {
			writeError(w, 400, "existing values cannot be converted")
			return
		}
	}
	field, err := q.UpdateCollectionField(r.Context(), params)
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, 409, "a field with that name already exists")
		} else {
			writeError(w, 404, "field not found")
		}
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit field")
		return
	}
	h.publish("collection:updated", uuidToString(collection.WorkspaceID), "member", user, map[string]any{"collection_id": uuidToString(collection.ID)})
	writeJSON(w, 200, collectionFieldResponse(field))
}

// DeleteCollectionRecord moves a record to the collection trash. Records have
// no workflow status: deleting is how a row leaves the table, and restore is
// available for 30 days.
func (h *Handler) DeleteCollectionRecord(w http.ResponseWriter, r *http.Request) {
	h.changeRecordTrash(w, r, true)
}

func (h *Handler) RestoreCollectionRecord(w http.ResponseWriter, r *http.Request) {
	h.changeRecordTrash(w, r, false)
}

func (h *Handler) changeRecordTrash(w http.ResponseWriter, r *http.Request, remove bool) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordID"), "record_id")
	if !ok {
		return
	}
	var (
		record db.Record
		err    error
	)
	if remove {
		record, err = h.Queries.SoftDeleteCollectionRecord(r.Context(), db.SoftDeleteCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id})
	} else {
		record, err = h.Queries.RestoreCollectionRecord(r.Context(), db.RestoreCollectionRecordParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: id})
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, 404, "record not found")
		} else {
			writeError(w, 500, "failed to change record")
		}
		return
	}
	h.respondCollectionRecord(w, r, record, 200)
}

func (h *Handler) ListCollectionTrash(w http.ResponseWriter, r *http.Request) {
	collection, ok := h.loadCollection(w, r)
	if !ok {
		return
	}
	records, err := h.Queries.ListDeletedCollectionRecords(r.Context(), db.ListDeletedCollectionRecordsParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID})
	if err != nil {
		writeError(w, 500, "failed to list trash")
		return
	}
	count, err := h.Queries.CountDeletedCollectionRecords(r.Context(), db.CountDeletedCollectionRecordsParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID})
	if err != nil {
		writeError(w, 500, "failed to count trash")
		return
	}
	out := make([]map[string]any, 0, len(records))
	for _, record := range records {
		response := collectionRecordResponse(record)
		response["deleted_at"] = timestampToString(record.DeletedAt)
		out = append(out, response)
	}
	writeJSON(w, 200, map[string]any{"records": out, "total": count, "retention_days": 30})
}

// initialRecordFields validates the optional field values sent with a new
// record, such as the board column or calendar day it was created in.
func (h *Handler) initialRecordFields(w http.ResponseWriter, r *http.Request, collection db.Collection, raw map[string]json.RawMessage) ([]byte, bool) {
	values := map[string]json.RawMessage{}
	for key, value := range raw {
		if string(value) == "null" {
			continue
		}
		fieldID, ok := parseUUIDOrBadRequest(w, key, "field_id")
		if !ok {
			return nil, false
		}
		definition, err := h.Queries.GetCollectionField(r.Context(), db.GetCollectionFieldParams{WorkspaceID: collection.WorkspaceID, CollectionID: collection.ID, ID: fieldID})
		if err != nil {
			writeError(w, 400, "field not found")
			return nil, false
		}
		stored, err := fields.ValidateValue(fields.Definition{Type: definition.Type, Config: definition.Config}, value)
		if err != nil {
			writeError(w, 400, err.Error())
			return nil, false
		}
		if propertyTypeIsActor(definition.Type) {
			refs, err := actorRefsInValue(definition.Type, stored)
			if err != nil {
				writeError(w, 400, err.Error())
				return nil, false
			}
			if status, msg := h.resolveActorRefs(r, uuidToString(collection.WorkspaceID), refs); status != 0 {
				writeError(w, status, msg)
				return nil, false
			}
		}
		values[uuidToString(fieldID)] = stored
	}
	encoded, err := json.Marshal(values)
	if err != nil || len(encoded) > 65536 {
		writeError(w, 400, "field values exceed record limits")
		return nil, false
	}
	return encoded, true
}
