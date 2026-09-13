package handler

import (
	"context"
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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	collectiondomain "github.com/multica-ai/multica/server/internal/collection"
	"github.com/multica-ai/multica/server/internal/featureflags"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

const collectionBodyMaxBytes = 128 * 1024

type CollectionResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	Name        string  `json:"name"`
	Revision    int64   `json:"revision"`
	ArchivedAt  *string `json:"archived_at"`
	CreatedAt   string  `json:"created_at"`
	UpdatedAt   string  `json:"updated_at"`
}

type CollectionFieldResponse struct {
	ID           string `json:"id"`
	WorkspaceID  string `json:"workspace_id"`
	CollectionID string `json:"collection_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Position     int32  `json:"position"`
	Revision     int64  `json:"revision"`
}

type CollectionRecordResponse struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	CollectionID string          `json:"collection_id"`
	Title        string          `json:"title"`
	Fields       json.RawMessage `json:"fields"`
	Position     float64         `json:"position"`
	Revision     int64           `json:"revision"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

type CollectionCapabilitiesResponse struct {
	Layouts     []string `json:"layouts"`
	Grouping    bool     `json:"grouping"`
	Hierarchy   bool     `json:"hierarchy"`
	Writable    bool     `json:"writable"`
	MaxPageSize int      `json:"max_page_size"`
}

type collectionTx struct {
	tx        pgx.Tx
	queries   *db.Queries
	workspace pgtype.UUID
	user      pgtype.UUID
	member    db.Member
}

func (h *Handler) collectionsEnabled(ctx context.Context) bool {
	return featureflags.CortexCollectionsEnabled(ctx, h.FeatureFlags)
}

func (h *Handler) beginCollectionTx(w http.ResponseWriter, r *http.Request, write bool) (*collectionTx, bool) {
	if !h.collectionsEnabled(r.Context()) {
		writeErrorCode(w, http.StatusNotFound, "feature_disabled", "collections are not enabled")
		return nil, false
	}
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "this endpoint is only available to human actors")
		return nil, false
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return nil, false
	}
	workspaceID := h.resolveWorkspaceID(r)
	workspaceUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return nil, false
	}
	userUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return nil, false
	}
	if h.TxStarter == nil {
		writeError(w, http.StatusInternalServerError, "collections require transaction support")
		return nil, false
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to begin collection transaction")
		return nil, false
	}
	qtx := h.Queries.WithTx(tx)
	if !write {
		if _, err := tx.Exec(r.Context(), "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
			_ = tx.Rollback(r.Context())
			writeError(w, http.StatusInternalServerError, "failed to configure collection transaction")
			return nil, false
		}
		if _, err := qtx.GetWorkspace(r.Context(), workspaceUUID); err != nil {
			_ = tx.Rollback(r.Context())
			writeError(w, http.StatusNotFound, "workspace not found")
			return nil, false
		}
		member, err := qtx.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: userUUID, WorkspaceID: workspaceUUID})
		if err != nil {
			_ = tx.Rollback(r.Context())
			writeError(w, http.StatusNotFound, "workspace not found")
			return nil, false
		}
		if err := qtx.SetCollectionWorkspaceContext(r.Context(), workspaceID); err != nil {
			_ = tx.Rollback(r.Context())
			writeError(w, http.StatusInternalServerError, "failed to bind collection workspace")
			return nil, false
		}
		return &collectionTx{tx: tx, queries: qtx, workspace: workspaceUUID, user: userUUID, member: member}, true
	}
	if _, err := qtx.LockCollectionWorkspace(r.Context(), workspaceUUID); err != nil {
		_ = tx.Rollback(r.Context())
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, false
	}
	member, err := qtx.LockCollectionMember(r.Context(), db.LockCollectionMemberParams{WorkspaceID: workspaceUUID, UserID: userUUID})
	if err != nil {
		_ = tx.Rollback(r.Context())
		writeError(w, http.StatusNotFound, "workspace not found")
		return nil, false
	}
	if err := qtx.SetCollectionWorkspaceContext(r.Context(), workspaceID); err != nil {
		_ = tx.Rollback(r.Context())
		writeError(w, http.StatusInternalServerError, "failed to bind collection workspace")
		return nil, false
	}
	return &collectionTx{tx: tx, queries: qtx, workspace: workspaceUUID, user: userUUID, member: member}, true
}

func decodeCollectionBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, collectionBodyMaxBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "payload_too_large", "request body exceeds 128 KiB")
		} else {
			writeError(w, http.StatusBadRequest, "invalid request body")
		}
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func collectionToResponse(value db.Collection) CollectionResponse {
	return CollectionResponse{
		ID: uuidToString(value.ID), WorkspaceID: uuidToString(value.WorkspaceID), Name: value.Name,
		Revision: value.Revision, ArchivedAt: timestampToPtr(value.ArchivedAt),
		CreatedAt: timestampToString(value.CreatedAt), UpdatedAt: timestampToString(value.UpdatedAt),
	}
}

func collectionFieldToResponse(value db.CollectionField) CollectionFieldResponse {
	return CollectionFieldResponse{
		ID: uuidToString(value.ID), WorkspaceID: uuidToString(value.WorkspaceID), CollectionID: uuidToString(value.CollectionID),
		Name: value.Name, Type: value.Type, Position: value.Position, Revision: value.Revision,
	}
}

func collectionRecordToResponse(value db.Record) CollectionRecordResponse {
	fields := json.RawMessage(value.Fields)
	if !json.Valid(fields) {
		fields = json.RawMessage("{}")
	}
	return CollectionRecordResponse{
		ID: uuidToString(value.ID), WorkspaceID: uuidToString(value.WorkspaceID), CollectionID: uuidToString(value.CollectionID),
		Title: value.Title, Fields: fields, Position: value.Position, Revision: value.Revision,
		CreatedAt: timestampToString(value.CreatedAt), UpdatedAt: timestampToString(value.UpdatedAt),
	}
}

func parseCollectionPageSize(value string) (int, error) {
	if value == "" {
		return collectiondomain.DefaultPageSize, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > collectiondomain.MaxPageSize {
		return 0, fmt.Errorf("limit must be between 1 and %d", collectiondomain.MaxPageSize)
	}
	return limit, nil
}

func collectionCursor(lastCreatedAt pgtype.Timestamptz, lastID pgtype.UUID, workspaceID, collectionID string, limit int) string {
	return collectiondomain.EncodeCursor(collectiondomain.PageCursor{
		Version: 1, WorkspaceID: workspaceID, CollectionID: collectionID, Query: "created_at_asc", Limit: limit,
		LastCreatedAt: lastCreatedAt.Time.UTC().Format(time.RFC3339Nano), LastID: uuidToString(lastID),
	})
}

func decodeCollectionCursor(value, workspaceID, collectionID string, limit int) (pgtype.Timestamptz, pgtype.UUID, error) {
	cursor, err := collectiondomain.DecodeCursor(value, workspaceID, collectionID, limit)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, cursor.LastCreatedAt)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	id, err := parseUUIDValue(cursor.LastID)
	if err != nil {
		return pgtype.Timestamptz{}, pgtype.UUID{}, errors.New("invalid cursor")
	}
	return pgtype.Timestamptz{Time: createdAt, Valid: true}, id, nil
}

func parseUUIDValue(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil || !id.Valid {
		return pgtype.UUID{}, errors.New("invalid uuid")
	}
	return id, nil
}

func (h *Handler) ListCollections(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, false)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	limit, err := parseCollectionPageSize(r.URL.Query().Get("limit"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspaceID := uuidToString(ctx.workspace)
	cursor := r.URL.Query().Get("cursor")
	var rows []db.Collection
	if cursor == "" {
		rows, err = ctx.queries.ListCollectionsFirstPage(r.Context(), db.ListCollectionsFirstPageParams{WorkspaceID: ctx.workspace, Limit: int32(limit + 1)})
	} else {
		createdAt, id, cursorErr := decodeCollectionCursor(cursor, workspaceID, "", limit)
		if cursorErr != nil {
			writeError(w, http.StatusBadRequest, cursorErr.Error())
			return
		}
		rows, err = ctx.queries.ListCollectionsPage(r.Context(), db.ListCollectionsPageParams{WorkspaceID: ctx.workspace, AfterCreatedAt: createdAt, AfterID: id, Limit: int32(limit + 1)})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list collections")
		return
	}
	total, err := ctx.queries.CountCollections(r.Context(), ctx.workspace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count collections")
		return
	}
	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		value := collectionCursor(last.CreatedAt, last.ID, workspaceID, "", limit)
		nextCursor = &value
	}
	responses := make([]CollectionResponse, len(rows))
	for index, row := range rows {
		responses[index] = collectionToResponse(row)
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish collection query")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"collections": responses, "total": total, "next_cursor": nextCursor})
}

type createCollectionRequest struct {
	ClientRequestID string                        `json:"client_request_id"`
	Name            string                        `json:"name"`
	Fields          []collectiondomain.FieldInput `json:"fields"`
}

func (h *Handler) CreateCollection(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, true)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	if !roleAllowed(ctx.member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}
	var request createCollectionRequest
	if !decodeCollectionBody(w, r, &request) {
		return
	}
	requestID, ok := parseUUIDOrBadRequest(w, request.ClientRequestID, "client_request_id")
	if !ok {
		return
	}
	spec, err := collectiondomain.NormalizeCollection(request.Name, request.Fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := ctx.queries.CreateCollectionIfAbsent(r.Context(), db.CreateCollectionIfAbsentParams{
		ID: dbid.NewV7(), WorkspaceID: ctx.workspace, Name: spec.Name, CreatedBy: ctx.user,
		CreateRequestID: requestID, CreateFingerprint: spec.Fingerprint,
	})
	replayed := false
	if errors.Is(err, pgx.ErrNoRows) {
		replayed = true
		created, err = ctx.queries.GetCollectionByCreateRequest(r.Context(), db.GetCollectionByCreateRequestParams{WorkspaceID: ctx.workspace, CreatedBy: ctx.user, CreateRequestID: requestID})
		if err == nil && (created.CreateFingerprint != spec.Fingerprint || created.ArchivedAt.Valid) {
			writeErrorCode(w, http.StatusConflict, "idempotency_conflict", "client_request_id was already used for a different collection")
			return
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create collection")
		return
	}
	var fields []db.CollectionField
	if replayed {
		fields, err = ctx.queries.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: ctx.workspace, CollectionID: created.ID})
	} else {
		fields = make([]db.CollectionField, 0, len(spec.Fields))
		for index, field := range spec.Fields {
			createdField, createErr := ctx.queries.CreateCollectionField(r.Context(), db.CreateCollectionFieldParams{
				ID: dbid.NewV7(), WorkspaceID: ctx.workspace, CollectionID: created.ID,
				Name: field.Name, Type: field.Type, Position: int32(index),
			})
			if createErr != nil {
				err = createErr
				break
			}
			fields = append(fields, createdField)
		}
		if err == nil {
			fieldIDs := make([]string, len(fields))
			for index, field := range fields {
				fieldIDs[index] = uuidToString(field.ID)
			}
			details, _ := json.Marshal(map[string]any{"collection_id": uuidToString(created.ID), "field_ids": fieldIDs, "revision": created.Revision, "request_id": request.ClientRequestID})
			_, err = ctx.queries.CreateActivity(r.Context(), db.CreateActivityParams{WorkspaceID: ctx.workspace, ActorType: pgtype.Text{String: "member", Valid: true}, ActorID: ctx.user, Action: "collection_created", Details: details, ID: dbid.NewV7()})
		}
	}
	if err != nil {
		writeCollectionDatabaseError(w, err, "failed to create collection")
		return
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit collection")
		return
	}
	fieldResponses := make([]CollectionFieldResponse, len(fields))
	for index, field := range fields {
		fieldResponses[index] = collectionFieldToResponse(field)
	}
	if !replayed {
		obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.CollectionCreated(uuidToString(ctx.user), uuidToString(ctx.workspace), uuidToString(created.ID)))
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"collection": collectionToResponse(created), "fields": fieldResponses, "replayed": replayed})
}

func (h *Handler) GetCollection(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, false)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	collectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionId"), "collection id")
	if !ok {
		return
	}
	value, err := ctx.queries.GetCollectionInWorkspace(r.Context(), db.GetCollectionInWorkspaceParams{ID: collectionID, WorkspaceID: ctx.workspace})
	if err != nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	fields, err := ctx.queries.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: ctx.workspace, CollectionID: collectionID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load collection fields")
		return
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish collection query")
		return
	}
	fieldResponses := make([]CollectionFieldResponse, len(fields))
	for index, field := range fields {
		fieldResponses[index] = collectionFieldToResponse(field)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"collection": collectionToResponse(value), "fields": fieldResponses,
		"capabilities": CollectionCapabilitiesResponse{Layouts: []string{"table"}, Writable: true, MaxPageSize: collectiondomain.MaxPageSize},
	})
}

type collectionPageRequest struct {
	Page struct {
		Limit  int     `json:"limit"`
		Cursor *string `json:"cursor"`
	} `json:"page"`
}

func (h *Handler) QueryCollectionRecords(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, false)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	collectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionId"), "collection id")
	if !ok {
		return
	}
	var request collectionPageRequest
	if !decodeCollectionBody(w, r, &request) {
		return
	}
	limit := request.Page.Limit
	if limit == 0 {
		limit = collectiondomain.DefaultPageSize
	}
	if limit < 1 || limit > collectiondomain.MaxPageSize {
		writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
		return
	}
	if _, err := ctx.queries.GetCollectionInWorkspace(r.Context(), db.GetCollectionInWorkspaceParams{ID: collectionID, WorkspaceID: ctx.workspace}); err != nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	workspaceID := uuidToString(ctx.workspace)
	collectionIDString := uuidToString(collectionID)
	var rows []db.Record
	var err error
	if request.Page.Cursor == nil || *request.Page.Cursor == "" {
		rows, err = ctx.queries.ListRecordsFirstPage(r.Context(), db.ListRecordsFirstPageParams{WorkspaceID: ctx.workspace, CollectionID: collectionID, Limit: int32(limit + 1)})
	} else {
		createdAt, id, cursorErr := decodeCollectionCursor(*request.Page.Cursor, workspaceID, collectionIDString, limit)
		if cursorErr != nil {
			writeError(w, http.StatusBadRequest, cursorErr.Error())
			return
		}
		rows, err = ctx.queries.ListRecordsPage(r.Context(), db.ListRecordsPageParams{WorkspaceID: ctx.workspace, CollectionID: collectionID, AfterCreatedAt: createdAt, AfterID: id, Limit: int32(limit + 1)})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list collection records")
		return
	}
	total, err := ctx.queries.CountRecords(r.Context(), db.CountRecordsParams{WorkspaceID: ctx.workspace, CollectionID: collectionID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count collection records")
		return
	}
	var nextCursor *string
	if len(rows) > limit {
		rows = rows[:limit]
		last := rows[len(rows)-1]
		value := collectionCursor(last.CreatedAt, last.ID, workspaceID, collectionIDString, limit)
		nextCursor = &value
	}
	responses := make([]CollectionRecordResponse, len(rows))
	for index, row := range rows {
		responses[index] = collectionRecordToResponse(row)
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish collection record query")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": responses, "total": total, "next_cursor": nextCursor})
}

func collectionDefinitions(fields []db.CollectionField) []collectiondomain.FieldDefinition {
	definitions := make([]collectiondomain.FieldDefinition, len(fields))
	for index, field := range fields {
		definitions[index] = collectiondomain.FieldDefinition{ID: uuidToString(field.ID), Type: field.Type}
	}
	return definitions
}

type createCollectionRecordRequest struct {
	ClientRequestID string                     `json:"client_request_id"`
	Title           string                     `json:"title"`
	Fields          map[string]json.RawMessage `json:"fields"`
}

func (h *Handler) CreateCollectionRecord(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, true)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	collectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionId"), "collection id")
	if !ok {
		return
	}
	requestID, requestIDOK := pgtype.UUID{}, false
	var request createCollectionRecordRequest
	if !decodeCollectionBody(w, r, &request) {
		return
	}
	requestID, requestIDOK = parseUUIDOrBadRequest(w, request.ClientRequestID, "client_request_id")
	if !requestIDOK {
		return
	}
	if _, err := ctx.queries.LockCollectionInWorkspace(r.Context(), db.LockCollectionInWorkspaceParams{ID: collectionID, WorkspaceID: ctx.workspace}); err != nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	fields, err := ctx.queries.ListCollectionFields(r.Context(), db.ListCollectionFieldsParams{WorkspaceID: ctx.workspace, CollectionID: collectionID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load collection fields")
		return
	}
	spec, err := collectiondomain.NormalizeRecord(request.Title, request.Fields, collectionDefinitions(fields))
	if err != nil {
		if errors.Is(err, collectiondomain.ErrFieldsTooLarge) {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "fields_too_large", err.Error())
		} else {
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	created, err := ctx.queries.CreateRecordIfAbsent(r.Context(), db.CreateRecordIfAbsentParams{
		ID: dbid.NewV7(), WorkspaceID: ctx.workspace, CollectionID: collectionID, Title: spec.Title, Fields: spec.Fields,
		CreatedBy: ctx.user, UpdatedBy: ctx.user, CreateRequestID: requestID, CreateFingerprint: spec.Fingerprint,
	})
	replayed := false
	if errors.Is(err, pgx.ErrNoRows) {
		replayed = true
		created, err = ctx.queries.GetRecordByCreateRequest(r.Context(), db.GetRecordByCreateRequestParams{WorkspaceID: ctx.workspace, CollectionID: collectionID, CreatedBy: ctx.user, CreateRequestID: requestID})
		if err == nil && (created.CreateFingerprint != spec.Fingerprint || created.DeletedAt.Valid) {
			writeErrorCode(w, http.StatusConflict, "idempotency_conflict", "client_request_id was already used for a different record")
			return
		}
	}
	if err == nil && !replayed {
		details, _ := json.Marshal(map[string]any{"collection_id": uuidToString(collectionID), "record_id": uuidToString(created.ID), "revision": created.Revision, "request_id": request.ClientRequestID})
		_, err = ctx.queries.CreateActivity(r.Context(), db.CreateActivityParams{WorkspaceID: ctx.workspace, ActorType: pgtype.Text{String: "member", Valid: true}, ActorID: ctx.user, Action: "record_created", Details: details, ID: dbid.NewV7()})
	}
	if err != nil {
		writeCollectionDatabaseError(w, err, "failed to create record")
		return
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit record")
		return
	}
	if !replayed {
		obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.RecordCreated(uuidToString(ctx.user), uuidToString(ctx.workspace), uuidToString(collectionID), uuidToString(created.ID)))
	}
	status := http.StatusCreated
	if replayed {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{"record": collectionRecordToResponse(created), "replayed": replayed})
}

func (h *Handler) GetCollectionRecord(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, false)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	collectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionId"), "collection id")
	if !ok {
		return
	}
	recordID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordId"), "record id")
	if !ok {
		return
	}
	if _, err := ctx.queries.GetCollectionInWorkspace(r.Context(), db.GetCollectionInWorkspaceParams{ID: collectionID, WorkspaceID: ctx.workspace}); err != nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	record, err := ctx.queries.GetRecordInCollection(r.Context(), db.GetRecordInCollectionParams{ID: recordID, WorkspaceID: ctx.workspace, CollectionID: collectionID})
	if err != nil {
		writeError(w, http.StatusNotFound, "record not found")
		return
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finish record query")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"record": collectionRecordToResponse(record)})
}

type updateCollectionRecordRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
	Change           struct {
		FieldID string          `json:"field_id"`
		Op      string          `json:"op"`
		Value   json.RawMessage `json:"value,omitempty"`
	} `json:"change"`
}

func (h *Handler) UpdateCollectionRecord(w http.ResponseWriter, r *http.Request) {
	ctx, ok := h.beginCollectionTx(w, r, true)
	if !ok {
		return
	}
	defer ctx.tx.Rollback(r.Context())
	collectionID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "collectionId"), "collection id")
	if !ok {
		return
	}
	recordID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "recordId"), "record id")
	if !ok {
		return
	}
	var request updateCollectionRecordRequest
	if !decodeCollectionBody(w, r, &request) {
		return
	}
	if request.ExpectedRevision < 1 {
		writeError(w, http.StatusBadRequest, "expected_revision must be positive")
		return
	}
	if request.Change.FieldID != "title" || request.Change.Op != "set" || len(request.Change.Value) == 0 {
		writeError(w, http.StatusBadRequest, "T2a only supports setting the title field")
		return
	}
	var title string
	if err := json.Unmarshal(request.Change.Value, &title); err != nil || len([]rune(title)) > collectiondomain.MaxTitleRunes {
		writeError(w, http.StatusBadRequest, "title must be text with at most 2048 characters")
		return
	}
	if _, err := ctx.queries.LockCollectionInWorkspace(r.Context(), db.LockCollectionInWorkspaceParams{ID: collectionID, WorkspaceID: ctx.workspace}); err != nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	updated, err := ctx.queries.UpdateRecordTitleCAS(r.Context(), db.UpdateRecordTitleCASParams{
		ID: recordID, WorkspaceID: ctx.workspace, CollectionID: collectionID, UpdatedBy: ctx.user,
		Title: title, Revision: request.ExpectedRevision,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		current, currentErr := ctx.queries.GetRecordInCollection(r.Context(), db.GetRecordInCollectionParams{ID: recordID, WorkspaceID: ctx.workspace, CollectionID: collectionID})
		if errors.Is(currentErr, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "record not found")
			return
		}
		if currentErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to resolve record conflict")
			return
		}
		writeRevisionConflict(w, "record", recordID, request.ExpectedRevision, current.Revision)
		return
	}
	if err != nil {
		writeCollectionDatabaseError(w, err, "failed to update record")
		return
	}
	details, _ := json.Marshal(map[string]any{"collection_id": uuidToString(collectionID), "record_id": uuidToString(recordID), "field_id": "title", "from_revision": request.ExpectedRevision, "to_revision": updated.Revision})
	if _, err := ctx.queries.CreateActivity(r.Context(), db.CreateActivityParams{WorkspaceID: ctx.workspace, ActorType: pgtype.Text{String: "member", Valid: true}, ActorID: ctx.user, Action: "record_updated", Details: details, ID: dbid.NewV7()}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to audit record update")
		return
	}
	if err := ctx.tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit record update")
		return
	}
	obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.RecordUpdated(uuidToString(ctx.user), uuidToString(ctx.workspace), uuidToString(collectionID), uuidToString(recordID)))
	writeJSON(w, http.StatusOK, map[string]any{"record": collectionRecordToResponse(updated)})
}

func writeCollectionDatabaseError(w http.ResponseWriter, err error, fallback string) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23514" && strings.Contains(pgErr.ConstraintName, "fields_size") {
		writeErrorCode(w, http.StatusRequestEntityTooLarge, "fields_too_large", "record fields exceed 64 KiB")
		return
	}
	writeError(w, http.StatusInternalServerError, fallback)
}
