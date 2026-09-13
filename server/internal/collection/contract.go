package collection

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	MaxCollectionNameRunes = 120
	MaxFieldNameRunes      = 80
	MaxFields              = 50
	MaxTitleRunes          = 2048
	MaxTextRunes           = 16384
	MaxFieldsBytes         = 65536
	MaxNumberMagnitude     = 1e15
	MaxPageSize            = 200
	DefaultPageSize        = 50
)

var ErrFieldsTooLarge = errors.New("record fields exceed 64 KiB")

type FieldInput struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type FieldDefinition struct {
	ID   string
	Type string
}

type CollectionSpec struct {
	Name        string       `json:"name"`
	Fields      []FieldInput `json:"fields"`
	Fingerprint string       `json:"-"`
}

type RecordSpec struct {
	Title       string
	Fields      []byte
	Fingerprint string
}

func NormalizeCollection(name string, fields []FieldInput) (CollectionSpec, error) {
	name = strings.TrimSpace(name)
	if count := utf8.RuneCountInString(name); count < 1 || count > MaxCollectionNameRunes {
		return CollectionSpec{}, fmt.Errorf("name must be between 1 and %d characters", MaxCollectionNameRunes)
	}
	if len(fields) < 1 || len(fields) > MaxFields {
		return CollectionSpec{}, fmt.Errorf("fields must contain between 1 and %d items", MaxFields)
	}

	normalized := make([]FieldInput, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for index, field := range fields {
		field.Name = strings.TrimSpace(field.Name)
		if count := utf8.RuneCountInString(field.Name); count < 1 || count > MaxFieldNameRunes {
			return CollectionSpec{}, fmt.Errorf("field %d name must be between 1 and %d characters", index, MaxFieldNameRunes)
		}
		switch field.Type {
		case "text", "number", "checkbox":
		default:
			return CollectionSpec{}, fmt.Errorf("field %d has unsupported type", index)
		}
		key := strings.ToLower(field.Name)
		if _, exists := seen[key]; exists {
			return CollectionSpec{}, fmt.Errorf("field names must be unique")
		}
		seen[key] = struct{}{}
		normalized[index] = field
	}

	spec := CollectionSpec{Name: name, Fields: normalized}
	spec.Fingerprint = fingerprint(struct {
		Name   string       `json:"name"`
		Fields []FieldInput `json:"fields"`
	}{Name: spec.Name, Fields: spec.Fields})
	return spec, nil
}

func NormalizeRecord(title string, values map[string]json.RawMessage, definitions []FieldDefinition) (RecordSpec, error) {
	if utf8.RuneCountInString(title) > MaxTitleRunes {
		return RecordSpec{}, fmt.Errorf("title must not exceed %d characters", MaxTitleRunes)
	}
	byID := make(map[string]string, len(definitions))
	for _, field := range definitions {
		byID[field.ID] = field.Type
	}
	normalized := make(map[string]any, len(values))
	for fieldID, raw := range values {
		fieldType, ok := byID[fieldID]
		if !ok {
			return RecordSpec{}, fmt.Errorf("unknown field %s", fieldID)
		}
		value, err := normalizeValue(fieldType, raw)
		if err != nil {
			return RecordSpec{}, fmt.Errorf("field %s: %w", fieldID, err)
		}
		normalized[fieldID] = value
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return RecordSpec{}, fmt.Errorf("encode fields: %w", err)
	}
	if len(encoded) > MaxFieldsBytes {
		return RecordSpec{}, ErrFieldsTooLarge
	}
	spec := RecordSpec{Title: title, Fields: encoded}
	spec.Fingerprint = fingerprint(struct {
		Title  string         `json:"title"`
		Fields map[string]any `json:"fields"`
	}{Title: title, Fields: normalized})
	return spec, nil
}

func normalizeValue(fieldType string, raw json.RawMessage) (any, error) {
	switch fieldType {
	case "text":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, errors.New("value must be text")
		}
		if utf8.RuneCountInString(value) > MaxTextRunes {
			return nil, fmt.Errorf("text must not exceed %d characters", MaxTextRunes)
		}
		return value, nil
	case "number":
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Abs(value) > MaxNumberMagnitude {
			return nil, errors.New("value must be a finite number with magnitude at most 10^15")
		}
		return value, nil
	case "checkbox":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, errors.New("value must be true or false")
		}
		return value, nil
	default:
		return nil, errors.New("field type is not writable")
	}
}

func fingerprint(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

type PageCursor struct {
	Version       int    `json:"v"`
	WorkspaceID   string `json:"workspace_id"`
	CollectionID  string `json:"collection_id,omitempty"`
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
	LastCreatedAt string `json:"last_created_at"`
	LastID        string `json:"last_id"`
}

func EncodeCursor(cursor PageCursor) string {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func DecodeCursor(encoded, workspaceID, collectionID string, limit int) (PageCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return PageCursor{}, errors.New("invalid cursor")
	}
	var cursor PageCursor
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return PageCursor{}, errors.New("invalid cursor")
	}
	if cursor.Version != 1 || cursor.Query != "created_at_asc" || cursor.WorkspaceID != workspaceID || cursor.CollectionID != collectionID || cursor.Limit != limit || cursor.LastCreatedAt == "" || cursor.LastID == "" {
		return PageCursor{}, errors.New("cursor does not match this query")
	}
	return cursor, nil
}
