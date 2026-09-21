package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/collectionimport"
	"github.com/multica-ai/multica/server/internal/fields"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type collectionImportOptions struct {
	DryRun      bool                      `json:"dry_run"`
	Sheet       string                    `json:"sheet"`
	HeaderRow   int                       `json:"header_row"`
	Name        string                    `json:"name"`
	ProjectID   *string                   `json:"project_id"`
	TitleColumn *int                      `json:"title_column"`
	Columns     []collectionimport.Column `json:"columns"`
}

// ImportCollection previews an uploaded worksheet, or creates its table,
// fields and records in one transaction. No upload or draft table is retained.
func (h *Handler) ImportCollection(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 26*1024*1024)
	err := r.ParseMultipartForm(8 * 1024 * 1024)
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if err != nil {
		writeError(w, 400, "upload an Excel or CSV file up to 24 MiB")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, 400, "file is required")
		return
	}
	defer file.Close()
	if header.Size > 24*1024*1024 {
		writeError(w, 400, "file exceeds 24 MiB")
		return
	}
	options := collectionImportOptions{DryRun: true}
	decoder := json.NewDecoder(strings.NewReader(r.FormValue("options")))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&options); err != nil || decoder.Decode(new(any)) != io.EOF {
		writeError(w, 400, "invalid import options")
		return
	}
	p, err := collectionimport.Read(r.Context(), header.Filename, file, options.Sheet, options.HeaderRow)
	if err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if options.DryRun {
		writeJSON(w, 200, p)
		return
	}
	if p.RowCount == 0 {
		writeError(w, 400, "worksheet has no data rows after the header")
		return
	}
	name := strings.TrimSpace(options.Name)
	if utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 80 {
		writeError(w, 400, "table name must be 1 to 80 characters")
		return
	}
	var project pgtype.UUID
	if options.ProjectID != nil {
		project, ok = parseUUIDOrBadRequest(w, *options.ProjectID, "project_id")
		if !ok {
			return
		}
		if _, err = h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{WorkspaceID: ws, ID: project}); err != nil {
			writeError(w, 400, "project not found")
			return
		}
	}
	if options.TitleColumn == nil || *options.TitleColumn < 0 || *options.TitleColumn >= len(p.Columns) || len(options.Columns) != len(p.Columns) {
		writeError(w, 400, "review the source columns and choose a title column")
		return
	}
	titleIndex := *options.TitleColumn
	definitions := map[int]db.CollectionField{}
	usedIndexes := map[int]bool{}
	names := map[string]bool{}
	titleName := ""
	for _, column := range options.Columns {
		if column.Index < 0 || column.Index >= len(p.Columns) || usedIndexes[column.Index] {
			writeError(w, 400, "duplicate or invalid source column")
			return
		}
		usedIndexes[column.Index] = true
		if column.Skip && column.Index != titleIndex {
			continue
		}
		if column.Skip {
			writeError(w, 400, "the title column cannot be skipped")
			return
		}
		fieldName, nameErr := validateLabelName(column.Name)
		if nameErr != nil {
			writeError(w, 400, fmt.Sprintf("column %d: %v", column.Index+1, nameErr))
			return
		}
		if names[strings.ToLower(fieldName)] {
			writeError(w, 400, "field names must be distinct")
			return
		}
		names[strings.ToLower(fieldName)] = true
		if column.Index == titleIndex {
			titleName = fieldName
			continue
		}
		if len(definitions) >= 50 {
			writeError(w, 400, "select at most 50 fields plus the title column")
			return
		}
		switch column.Type {
		case "text", "number", "checkbox", "date", "url", "select":
		default:
			writeError(w, 400, "unsupported import field type: "+column.Type)
			return
		}
		config := []byte("{}")
		if column.Type == "select" {
			cfg := PropertyConfig{}
			seen := map[string]bool{}
			for _, row := range p.Rows {
				if column.Index >= len(row) || row[column.Index] == "" {
					continue
				}
				value := row[column.Index]
				if !seen[value] {
					cfg.Options = append(cfg.Options, PropertyOption{Name: value, Color: "#6b7280"})
					seen[value] = true
				}
			}
			config, err = validatePropertyConfig("select", &cfg)
			if err != nil {
				writeError(w, 400, fieldName+": "+err.Error())
				return
			}
		}
		definitions[column.Index] = db.CollectionField{ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, Name: fieldName, Type: column.Type, Config: config}
	}
	// Validate every selected value before creating the table. A failure cannot
	// leave an empty table, partially created schema, or partially imported rows.
	values := make([][]any, 0, len(p.Rows))
	for n, row := range p.Rows {
		title := ""
		if titleIndex < len(row) {
			title = row[titleIndex]
		}
		if utf8.RuneCountInString(title) > 2048 {
			writeError(w, 400, fmt.Sprintf("row %d: title is too long", p.RowNumbers[n]))
			return
		}
		bag := map[string]json.RawMessage{}
		for index, def := range definitions {
			if index >= len(row) || row[index] == "" {
				continue
			}
			value, err := collectionimport.CellValue(def.Type, row[index])
			if err != nil {
				writeError(w, 400, fmt.Sprintf("row %d, %s: %v", p.RowNumbers[n], def.Name, err))
				return
			}
			var raw []byte
			if def.Type == "select" {
				raw, err = csvFieldValue(def, row[index])
			} else {
				raw, _ = json.Marshal(value)
				raw, err = fields.ValidateValue(fields.Definition{Type: def.Type, Config: def.Config}, raw)
			}
			if err != nil {
				writeError(w, 400, fmt.Sprintf("row %d, %s: %v", p.RowNumbers[n], def.Name, err))
				return
			}
			bag[uuidToString(def.ID)] = raw
		}
		raw, _ := json.Marshal(bag)
		if len(raw) > 60000 {
			writeError(w, 400, fmt.Sprintf("row %d exceeds record limits", p.RowNumbers[n]))
			return
		}
		values = append(values, []any{ws, nil, title, raw})
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "failed to start import")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	c, err := q.CreateCollection(r.Context(), db.CreateCollectionParams{WorkspaceID: ws, ProjectID: project, Name: name, CreatedBy: parseUUID(user)})
	if err != nil {
		writeError(w, 500, "failed to create imported table")
		return
	}
	c, err = q.UpdateCollection(r.Context(), db.UpdateCollectionParams{WorkspaceID: ws, ID: c.ID, TitleName: pgtype.Text{String: titleName, Valid: true}})
	if err != nil {
		writeError(w, 500, "failed to name title column")
		return
	}
	fieldRows := [][]any{}
	for _, column := range options.Columns {
		if def, ok := definitions[column.Index]; ok {
			fieldRows = append(fieldRows, []any{def.ID, ws, c.ID, def.Name, def.Type, def.Config, float64(column.Index)})
		}
	}
	if len(fieldRows) > 0 {
		_, err = tx.CopyFrom(r.Context(), pgx.Identifier{"collection_field"}, []string{"id", "workspace_id", "collection_id", "name", "type", "config", "position"}, pgx.CopyFromRows(fieldRows))
	}
	if err != nil {
		writeError(w, 400, "failed to import fields; nothing created")
		return
	}
	for _, row := range values {
		row[1] = c.ID
	}
	_, err = tx.CopyFrom(r.Context(), pgx.Identifier{"record"}, []string{"workspace_id", "collection_id", "title", "fields"}, pgx.CopyFromRows(values))
	if err != nil {
		writeError(w, 400, "failed to import rows; nothing created")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		writeError(w, 500, "failed to commit import")
		return
	}
	h.publishCollectionBatch(w, r, c)
	writeJSON(w, 201, map[string]any{"collection": c, "count": len(values)})
}
