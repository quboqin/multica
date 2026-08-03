package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type creativeResourceResponse struct {
	ID               string          `json:"id"`
	WorkspaceID      string          `json:"workspace_id"`
	Kind             string          `json:"kind"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Status           string          `json:"status"`
	Version          int             `json:"version"`
	PublishedVersion int             `json:"published_version"`
	Config           json.RawMessage `json:"config"`
	CreatedBy        string          `json:"created_by"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

type creativeCopyEntryResponse struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	LibraryID   string          `json:"library_id"`
	ExternalKey string          `json:"external_key"`
	Headline    string          `json:"headline"`
	Subheadline string          `json:"subheadline"`
	Benefit     string          `json:"benefit"`
	CTA         string          `json:"cta"`
	LegalText   string          `json:"legal_text"`
	CopyRole    string          `json:"copy_role"`
	Market      string          `json:"market"`
	Locale      string          `json:"locale"`
	Tags        []string        `json:"tags"`
	Status      string          `json:"status"`
	Version     int             `json:"version"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

type creativeIssueContextResponse struct {
	IssueID      string          `json:"issue_id"`
	WorkspaceID  string          `json:"workspace_id"`
	MarketPackID string          `json:"market_pack_id"`
	SquadID      string          `json:"squad_id"`
	Snapshot     json.RawMessage `json:"snapshot"`
	UpdatedAt    string          `json:"updated_at"`
}

type creativeResourceFileResponse struct {
	ID             string          `json:"id"`
	ResourceID     string          `json:"resource_id"`
	AttachmentID   string          `json:"attachment_id"`
	Role           string          `json:"role"`
	Label          string          `json:"label"`
	Metadata       json.RawMessage `json:"metadata"`
	Filename       string          `json:"filename"`
	URL            string          `json:"url"`
	ContentType    string          `json:"content_type"`
	SizeBytes      int64           `json:"size_bytes"`
	CreatedVersion int             `json:"created_version"`
	CreatedAt      string          `json:"created_at"`
}

type creativeIssueItemResponse struct {
	IssueID       string          `json:"issue_id"`
	CandidateID   string          `json:"candidate_id"`
	CopyEntryID   string          `json:"copy_entry_id"`
	CopySnapshot  json.RawMessage `json:"copy_snapshot"`
	CreativeBrief json.RawMessage `json:"creative_brief"`
	WorkIssueID   string          `json:"work_issue_id"`
	Revision      int             `json:"revision"`
	Status        string          `json:"status"`
	UpdatedAt     string          `json:"updated_at"`
}

type creativeBriefInput struct {
	Theme                string   `json:"theme"`
	ThemeElements        []string `json:"theme_elements"`
	PrimaryBenefit       string   `json:"primary_benefit"`
	SecondaryBenefits    []string `json:"secondary_benefits"`
	BenefitValue         string   `json:"benefit_value"`
	SourceSemantics      string   `json:"source_semantics"`
	InformationMechanism string   `json:"information_mechanism"`
	VisualAnchors        []string `json:"visual_anchors"`
	PaletteAnchors       []string `json:"palette_anchors"`
	MustPreserve         []string `json:"must_preserve"`
	AllowedVariations    []string `json:"allowed_variations"`
	Evidence             []string `json:"evidence"`
	DetectedText         []string `json:"detected_text"`
	VisualType           string   `json:"visual_type"`
	AnalysisSummary      string   `json:"analysis_summary"`
	Status               string   `json:"status"`
	Source               string   `json:"source"`
	Confidence           *float64 `json:"confidence"`
	AnalysisIssueID      string   `json:"analysis_issue_id"`
}

type creativeCopyEntryInput struct {
	ExternalKey string          `json:"external_key"`
	Headline    string          `json:"headline"`
	Subheadline string          `json:"subheadline"`
	Benefit     string          `json:"benefit"`
	CTA         string          `json:"cta"`
	LegalText   string          `json:"legal_text"`
	CopyRole    string          `json:"copy_role"`
	Market      string          `json:"market"`
	Locale      string          `json:"locale"`
	Tags        []string        `json:"tags"`
	Status      string          `json:"status"`
	Metadata    json.RawMessage `json:"metadata"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (h *Handler) ListCreativeResources(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "" && !validCreativeResourceKind(kind) {
		writeError(w, http.StatusBadRequest, "invalid creative resource kind")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, workspace_id::text, kind, name, description, status, version,
       COALESCE(published_version, 0), config::text, created_by::text,
       created_at::text, updated_at::text
FROM creative_resource
WHERE workspace_id = $1 AND status <> 'archived' AND ($2 = '' OR kind = $2)
ORDER BY kind, updated_at DESC
`, workspaceID, kind)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list creative resources")
		return
	}
	defer rows.Close()
	resources := []creativeResourceResponse{}
	for rows.Next() {
		resource, scanErr := scanCreativeResource(rows)
		if scanErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative resources")
			return
		}
		resources = append(resources, resource)
	}
	writeJSON(w, http.StatusOK, map[string]any{"resources": resources})
}

func (h *Handler) ListCreativeMaterialLibrary(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT
  c.id::text, c.workspace_id::text, c.connector_id, COALESCE(c.external_id, ''),
  c.dedupe_key, c.competitor, c.title, c.asset_type,
  c.preview_url, c.resource_url, c.poster_url, c.original_url,
  c.archived_url, c.archive_status, c.archive_error,
  c.duration_days, c.impression_estimate, c.media_names, c.area_names,
  c.language_names, c.platform_names, c.tags || COALESCE(latest.tags, '{}'::text[]),
  COALESCE(NULLIF(latest.note, ''), c.note), COALESCE(latest.selected_at::text, ''),
  c.first_seen_at::text, c.last_seen_at::text, c.created_at::text, c.updated_at::text,
  COALESCE(c.source_attachment_id::text, ''), COALESCE(latest.status, 'new'),
  COALESCE(latest.issue_id::text, ''), COALESCE(latest.source_run_id::text, ''), false
FROM creative_material_candidate c
LEFT JOIN LATERAL (
  SELECT ic.issue_id, ic.source_run_id, ic.status, ic.tags, ic.note, ic.selected_at
  FROM creative_material_issue_candidate ic
  WHERE ic.workspace_id = c.workspace_id AND ic.candidate_id = c.id
  ORDER BY ic.updated_at DESC
  LIMIT 1
) latest ON true
WHERE c.workspace_id = $1
ORDER BY c.last_seen_at DESC
LIMIT 500
`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list creative material library")
		return
	}
	defer rows.Close()
	candidates := []creativeMaterialCandidateResponse{}
	for rows.Next() {
		var item creativeMaterialCandidateResponse
		var duration pgtype.Float8
		var impression pgtype.Int8
		var selectedAt string
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.ConnectorID, &item.ExternalID,
			&item.DedupeKey, &item.Competitor, &item.Title, &item.AssetType,
			&item.PreviewURL, &item.ResourceURL, &item.PosterURL, &item.OriginalURL,
			&item.ArchivedURL, &item.ArchiveStatus, &item.ArchiveError,
			&duration, &impression, &item.MediaNames, &item.AreaNames,
			&item.LanguageNames, &item.PlatformNames, &item.Tags, &item.Note, &selectedAt,
			&item.FirstSeenAt, &item.LastSeenAt, &item.CreatedAt, &item.UpdatedAt,
			&item.SourceAttachmentID, &item.Status, &item.SourceIssueID, &item.SourceRunID, &item.IsNewInRun,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative material library")
			return
		}
		item.DurationDays = float8Ptr(duration)
		item.ImpressionEstimate = int8Ptr(impression)
		if selectedAt != "" {
			item.SelectedAt = &selectedAt
		}
		candidates = append(candidates, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

func (h *Handler) RetryCreativeMaterialArchives(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	var req struct {
		CandidateIDs []string `json:"candidate_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	candidateIDs := make([]pgtype.UUID, 0, len(req.CandidateIDs))
	for _, rawID := range uniqueNonEmptyStrings(req.CandidateIDs) {
		candidateID, parsed := parseUUIDOrBadRequest(w, rawID, "candidate_id")
		if !parsed {
			return
		}
		candidateIDs = append(candidateIDs, candidateID)
	}
	tag, err := h.DB.Exec(r.Context(), `
UPDATE creative_material_candidate
SET archive_status = 'pending', archive_attempts = 0, archive_error = '',
    next_archive_at = now(), updated_at = now()
WHERE workspace_id = $1 AND archived_url = ''
  AND (cardinality($2::uuid[]) = 0 OR id = ANY($2::uuid[]))
  AND (archive_status <> 'running' OR next_archive_at <= now())
`, workspaceID, candidateIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retry creative material archives")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"scheduled_count": tag.RowsAffected()})
}

func (h *Handler) ImportCreativeMaterialLibrary(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	var req struct {
		AttachmentID  string   `json:"attachment_id"`
		SourceURL     string   `json:"source_url"`
		Title         string   `json:"title"`
		Competitor    string   `json:"competitor"`
		AssetType     string   `json:"asset_type"`
		AreaNames     []string `json:"area_names"`
		LanguageNames []string `json:"language_names"`
		PlatformNames []string `json:"platform_names"`
		Tags          []string `json:"tags"`
		Note          string   `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.AttachmentID = strings.TrimSpace(req.AttachmentID)
	req.SourceURL = strings.TrimSpace(req.SourceURL)
	if (req.AttachmentID == "") == (req.SourceURL == "") {
		writeError(w, http.StatusBadRequest, "provide exactly one attachment_id or source_url")
		return
	}
	connectorID := "manual_url"
	externalID := req.SourceURL
	dedupeSource := req.SourceURL
	previewURL := req.SourceURL
	resourceURL := req.SourceURL
	originalURL := req.SourceURL
	archivedURL := ""
	archiveStatus := "pending"
	var sourceAttachmentID any
	assetType := normalizeCreativeAssetType(req.AssetType)
	if req.AttachmentID != "" {
		attachmentID, parsed := parseUUIDOrBadRequest(w, req.AttachmentID, "attachment_id")
		if !parsed {
			return
		}
		attachment, err := h.Queries.GetAttachmentByIDOnly(r.Context(), attachmentID)
		if err != nil || attachment.WorkspaceID != workspaceID {
			writeError(w, http.StatusNotFound, "attachment not found")
			return
		}
		attachmentResponse := h.attachmentToResponse(attachment)
		connectorID = "manual_upload"
		externalID = req.AttachmentID
		dedupeSource = "attachment:" + req.AttachmentID
		previewURL = attachmentResponse.MarkdownURL
		resourceURL = attachmentResponse.MarkdownURL
		originalURL = attachmentResponse.MarkdownURL
		archivedURL = attachmentResponse.MarkdownURL
		archiveStatus = "completed"
		sourceAttachmentID = attachmentID
		if strings.HasPrefix(attachment.ContentType, "image/") {
			assetType = "image"
		} else if strings.HasPrefix(attachment.ContentType, "video/") {
			assetType = "video"
		}
		if strings.TrimSpace(req.Title) == "" {
			req.Title = attachment.Filename
		}
	} else {
		parsed, err := url.Parse(req.SourceURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			writeError(w, http.StatusBadRequest, "source_url must be an absolute HTTP URL")
			return
		}
	}
	digest := sha256.Sum256([]byte(dedupeSource))
	dedupeKey := connectorID + ":" + hex.EncodeToString(digest[:])
	var candidateID string
	err := h.DB.QueryRow(r.Context(), `
INSERT INTO creative_material_candidate (
  workspace_id, connector_id, external_id, dedupe_key, competitor, title, asset_type,
  preview_url, resource_url, original_url, archived_url, archive_status,
  area_names, language_names, platform_names, tags, note, source_attachment_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
ON CONFLICT (workspace_id, connector_id, dedupe_key) DO UPDATE SET
  competitor = EXCLUDED.competitor, title = EXCLUDED.title, asset_type = EXCLUDED.asset_type,
  preview_url = EXCLUDED.preview_url, resource_url = EXCLUDED.resource_url,
  original_url = EXCLUDED.original_url, archived_url = EXCLUDED.archived_url,
  archive_status = EXCLUDED.archive_status, area_names = EXCLUDED.area_names,
  language_names = EXCLUDED.language_names, platform_names = EXCLUDED.platform_names,
  tags = EXCLUDED.tags, note = EXCLUDED.note,
  source_attachment_id = EXCLUDED.source_attachment_id, last_seen_at = now(), updated_at = now()
RETURNING id::text
`, workspaceID, connectorID, externalID, dedupeKey, strings.TrimSpace(req.Competitor), strings.TrimSpace(req.Title), assetType,
		previewURL, resourceURL, originalURL, archivedURL, archiveStatus,
		uniqueNonEmptyStrings(req.AreaNames), uniqueNonEmptyStrings(req.LanguageNames), uniqueNonEmptyStrings(req.PlatformNames),
		uniqueNonEmptyStrings(req.Tags), strings.TrimSpace(req.Note), sourceAttachmentID).Scan(&candidateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to import creative material")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": candidateID})
}

func (h *Handler) CreateCreativeResource(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	var req struct {
		Kind        string          `json:"kind"`
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	req.Name = strings.TrimSpace(req.Name)
	if !validCreativeResourceKind(req.Kind) || req.Name == "" {
		writeError(w, http.StatusBadRequest, "kind and name are required")
		return
	}
	config, err := normalizedJSONObject(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config must be a JSON object")
		return
	}
	config = stripCreativeResourceValidation(config)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative resource")
		return
	}
	defer tx.Rollback(r.Context())
	resource, err := scanCreativeResource(tx.QueryRow(r.Context(), `
INSERT INTO creative_resource (workspace_id, kind, name, description, config, created_by)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, workspaceID, req.Kind, req.Name, strings.TrimSpace(req.Description), string(config), userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative resource")
		return
	}
	if err := insertCreativeResourceRevision(r.Context(), tx, resource, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative resource revision")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative resource")
		return
	}
	writeJSON(w, http.StatusCreated, resource)
}

func (h *Handler) UpdateCreativeResource(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Config      json.RawMessage `json:"config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	config, err := normalizedJSONObject(req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "config must be a JSON object")
		return
	}
	config = stripCreativeResourceValidation(config)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative resource")
		return
	}
	defer tx.Rollback(r.Context())
	resource, err := scanCreativeResource(tx.QueryRow(r.Context(), `
UPDATE creative_resource
SET name = $3, description = $4, config = $5::jsonb, status = 'draft',
    version = version + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND status <> 'archived'
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, resourceID, workspaceID, req.Name, strings.TrimSpace(req.Description), string(config)))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative resource not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative resource")
		return
	}
	if err := insertCreativeResourceRevision(r.Context(), tx, resource, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative resource revision")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative resource")
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (h *Handler) PublishCreativeResource(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish creative resource")
		return
	}
	defer tx.Rollback(r.Context())
	current, err := scanCreativeResource(tx.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, kind, name, description, status, version,
       COALESCE(published_version, 0), config::text, created_by::text,
       created_at::text, updated_at::text
FROM creative_resource
WHERE id = $1 AND workspace_id = $2 AND status <> 'archived'
FOR UPDATE
`, resourceID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative resource not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish creative resource")
		return
	}
	config := current.Config
	if current.Kind == "market_pack" {
		config, err = h.validateMarketPackQRConfig(r.Context(), workspaceID, resourceID, current.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "market resource pack cannot be published: "+err.Error())
			return
		}
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_resource_revision SET config = $3::jsonb
WHERE resource_id = $1 AND version = $2
`, resourceID, current.Version, string(config)); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to record market pack validation")
			return
		}
	}
	resource, err := scanCreativeResource(tx.QueryRow(r.Context(), `
UPDATE creative_resource
SET status = 'published', published_version = version, config = $3::jsonb, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND status <> 'archived'
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, resourceID, workspaceID, string(config)))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish creative resource")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish creative resource")
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

func (h *Handler) ArchiveCreativeResource(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	tag, err := h.DB.Exec(r.Context(), `
UPDATE creative_resource SET status = 'archived', updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND status <> 'archived'
`, resourceID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to archive creative resource")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "creative resource not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListCreativeResourceFiles(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	if _, err := h.requireCreativeResource(r.Context(), workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	files, err := h.loadCreativeResourceFiles(r.Context(), workspaceID, resourceID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list market resource files")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (h *Handler) AddCreativeResourceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	var req struct {
		AttachmentID string          `json:"attachment_id"`
		Role         string          `json:"role"`
		Label        string          `json:"label"`
		Metadata     json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	attachmentID, ok := parseUUIDOrBadRequest(w, req.AttachmentID, "attachment_id")
	if !ok {
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}
	metadata, err := normalizedJSONObject(req.Metadata)
	if err != nil {
		writeError(w, http.StatusBadRequest, "metadata must be a JSON object")
		return
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(r.Context(), attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		writeError(w, http.StatusNotFound, "attachment not found")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add market resource file")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := requireCreativeResourceTx(r.Context(), tx, workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	var exists bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_resource_file
  WHERE resource_id = $1 AND attachment_id = $2 AND role = $3 AND removed_version IS NULL
)
`, resourceID, attachmentID, req.Role).Scan(&exists); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to inspect market resource file")
		return
	}
	if exists {
		writeError(w, http.StatusConflict, "this file already fills the selected resource slot")
		return
	}
	resource, err := bumpCreativeResourceRevision(r.Context(), tx, workspaceID, resourceID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to version market resource pack")
		return
	}
	var fileID string
	if err := tx.QueryRow(r.Context(), `
INSERT INTO creative_resource_file (
  resource_id, workspace_id, attachment_id, role, label, metadata, created_version, created_by
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
RETURNING id::text
`, resourceID, workspaceID, attachmentID, req.Role, strings.TrimSpace(req.Label), string(metadata), resource.Version, userID).Scan(&fileID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add market resource file")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add market resource file")
		return
	}
	files, err := h.loadCreativeResourceFiles(r.Context(), workspaceID, resourceID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read market resource file")
		return
	}
	for _, file := range files {
		if file.ID == fileID {
			writeJSON(w, http.StatusCreated, file)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "market resource file was not persisted")
}

func (h *Handler) RemoveCreativeResourceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	fileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "fileId"), "file_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove market resource file")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := requireCreativeResourceTx(r.Context(), tx, workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	var exists bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_resource_file
  WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND removed_version IS NULL
)
`, fileID, resourceID, workspaceID).Scan(&exists); err != nil || !exists {
		writeError(w, http.StatusNotFound, "market resource file not found")
		return
	}
	resource, err := bumpCreativeResourceRevision(r.Context(), tx, workspaceID, resourceID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to version market resource pack")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_resource_file SET removed_version = $4
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND removed_version IS NULL
`, fileID, resourceID, workspaceID, resource.Version); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove market resource file")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove market resource file")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) UpdateCreativeResourceFile(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	resourceID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "resource_id")
	if !ok {
		return
	}
	fileID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "fileId"), "file_id")
	if !ok {
		return
	}
	var req struct {
		Role     string          `json:"role"`
		Label    string          `json:"label"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	req.Role = strings.TrimSpace(req.Role)
	if req.Role == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}
	metadata, err := normalizedJSONObject(req.Metadata)
	if err != nil {
		writeError(w, http.StatusBadRequest, "metadata must be a JSON object")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update market resource file")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := requireCreativeResourceTx(r.Context(), tx, workspaceID, resourceID, "market_pack"); err != nil {
		writeError(w, http.StatusNotFound, "market resource pack not found")
		return
	}
	var attachmentID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT attachment_id FROM creative_resource_file
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND removed_version IS NULL
FOR UPDATE
`, fileID, resourceID, workspaceID).Scan(&attachmentID); err != nil {
		writeError(w, http.StatusNotFound, "market resource file not found")
		return
	}
	resource, err := bumpCreativeResourceRevision(r.Context(), tx, workspaceID, resourceID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to version market resource pack")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_resource_file SET removed_version = $4
WHERE id = $1 AND resource_id = $2 AND workspace_id = $3 AND removed_version IS NULL
`, fileID, resourceID, workspaceID, resource.Version); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update market resource file")
		return
	}
	var replacementID string
	if err := tx.QueryRow(r.Context(), `
INSERT INTO creative_resource_file (
  resource_id, workspace_id, attachment_id, role, label, metadata, created_version, created_by
) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8)
RETURNING id::text
`, resourceID, workspaceID, attachmentID, req.Role, strings.TrimSpace(req.Label), string(metadata), resource.Version, userID).Scan(&replacementID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update market resource file")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update market resource file")
		return
	}
	files, err := h.loadCreativeResourceFiles(r.Context(), workspaceID, resourceID, 0)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read market resource file")
		return
	}
	for _, file := range files {
		if file.ID == replacementID {
			writeJSON(w, http.StatusOK, file)
			return
		}
	}
	writeError(w, http.StatusInternalServerError, "market resource file was not persisted")
}

func (h *Handler) ListCreativeCopyEntries(w http.ResponseWriter, r *http.Request) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return
	}
	libraryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "library_id")
	if !ok {
		return
	}
	if _, err := h.requireCreativeResource(r.Context(), workspaceID, libraryID, "copy_library"); err != nil {
		writeError(w, http.StatusNotFound, "copy library not found")
		return
	}
	rows, err := h.DB.Query(r.Context(), creativeCopyEntrySelect+`
WHERE workspace_id = $1 AND library_id = $2
ORDER BY CASE status WHEN 'approved' THEN 0 WHEN 'draft' THEN 1 ELSE 2 END, updated_at DESC
`, workspaceID, libraryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list copy entries")
		return
	}
	defer rows.Close()
	entries := []creativeCopyEntryResponse{}
	for rows.Next() {
		entry, scanErr := scanCreativeCopyEntry(rows)
		if scanErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to read copy entries")
			return
		}
		entries = append(entries, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (h *Handler) ImportCreativeCopyEntries(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	libraryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "library_id")
	if !ok {
		return
	}
	var req struct {
		Mode           string                   `json:"mode"`
		SourceFilename string                   `json:"source_filename"`
		Mapping        json.RawMessage          `json:"mapping"`
		Entries        []creativeCopyEntryInput `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Mode == "" {
		req.Mode = "upsert"
	}
	if req.Mode != "append" && req.Mode != "upsert" && req.Mode != "replace" {
		writeError(w, http.StatusBadRequest, "mode must be append, upsert, or replace")
		return
	}
	if len(req.Entries) == 0 || len(req.Entries) > 5000 {
		writeError(w, http.StatusBadRequest, "entries must contain between 1 and 5000 rows")
		return
	}
	mapping, err := normalizedJSONObject(req.Mapping)
	if err != nil {
		writeError(w, http.StatusBadRequest, "mapping must be a JSON object")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to import copy entries")
		return
	}
	defer tx.Rollback(r.Context())
	if _, err := requireCreativeResourceTx(r.Context(), tx, workspaceID, libraryID, "copy_library"); err != nil {
		writeError(w, http.StatusNotFound, "copy library not found")
		return
	}
	if req.Mode == "replace" {
		if _, err := tx.Exec(r.Context(), `UPDATE creative_copy_entry SET status = 'disabled', updated_at = now() WHERE workspace_id = $1 AND library_id = $2`, workspaceID, libraryID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare copy import")
			return
		}
	}
	created, updated, skipped := 0, 0, 0
	for index, input := range req.Entries {
		input = normalizeCreativeCopyEntry(input, index)
		if input.Headline == "" && input.Subheadline == "" && input.Benefit == "" && input.CTA == "" {
			skipped++
			continue
		}
		if req.Mode == "append" {
			var exists bool
			if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM creative_copy_entry WHERE library_id = $1 AND external_key = $2)`, libraryID, input.ExternalKey).Scan(&exists); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to inspect copy import")
				return
			}
			if exists {
				skipped++
				continue
			}
		}
		var inserted bool
		if err := tx.QueryRow(r.Context(), creativeCopyEntryUpsertSQL,
			workspaceID, libraryID, input.ExternalKey, input.Headline, input.Subheadline,
			input.Benefit, input.CTA, input.LegalText, input.CopyRole, input.Market,
			input.Locale, input.Tags, input.Status, string(input.Metadata), userID,
		).Scan(&inserted); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to import copy row %d", index+1))
			return
		}
		if inserted {
			created++
		} else {
			updated++
		}
	}
	resource, err := bumpCreativeLibraryRevision(r.Context(), tx, workspaceID, libraryID, userID, req.SourceFilename, mapping)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to version copy library")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to import copy entries")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created": created, "updated": updated, "skipped": skipped, "resource": resource,
	})
}

func (h *Handler) UpdateCreativeCopyEntry(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	entryID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "entryId"), "entry_id")
	if !ok {
		return
	}
	var input creativeCopyEntryInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	input = normalizeCreativeCopyEntry(input, 0)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update copy entry")
		return
	}
	defer tx.Rollback(r.Context())
	entry, err := scanCreativeCopyEntry(tx.QueryRow(r.Context(), `
UPDATE creative_copy_entry
SET external_key = $3, headline = $4, subheadline = $5, benefit = $6,
    cta = $7, legal_text = $8, copy_role = $9, market = $10, locale = $11,
    tags = $12, status = $13, metadata = $14::jsonb, version = version + 1,
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
RETURNING id::text, workspace_id::text, library_id::text, external_key, headline,
          subheadline, benefit, cta, legal_text, copy_role, market, locale, tags,
          status, version, metadata::text, created_by::text, created_at::text, updated_at::text
	`, entryID, workspaceID, input.ExternalKey, input.Headline, input.Subheadline, input.Benefit,
		input.CTA, input.LegalText, input.CopyRole, input.Market, input.Locale, input.Tags,
		input.Status, string(input.Metadata)))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "copy entry not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update copy entry")
		return
	}
	libraryID, err := parseUUIDString(entry.LibraryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "copy entry has invalid library")
		return
	}
	if _, err := bumpCreativeResourceRevision(r.Context(), tx, workspaceID, libraryID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to version copy library")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update copy entry")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) PutCreativeIssueContext(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		MarketPackID string `json:"market_pack_id"`
		SquadID      string `json:"squad_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	marketPackID, ok := parseUUIDOrBadRequest(w, req.MarketPackID, "market_pack_id")
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, req.SquadID, "squad_id")
	if !ok {
		return
	}
	snapshot, err := h.buildCreativeIssueSnapshot(r.Context(), issue.WorkspaceID, marketPackID, squadID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	if !requestingUserID.Valid {
		writeError(w, http.StatusForbidden, "requesting user not available")
		return
	}
	context, err := scanCreativeIssueContext(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_issue_context (
	issue_id, workspace_id, market_pack_id, squad_id, snapshot, updated_by
) VALUES ($1, $2, $3, $4, $5::jsonb, $6)
ON CONFLICT (issue_id) DO UPDATE SET
	market_pack_id = EXCLUDED.market_pack_id,
	squad_id = EXCLUDED.squad_id,
  snapshot = EXCLUDED.snapshot,
  updated_by = EXCLUDED.updated_by,
  updated_at = now()
RETURNING issue_id::text, workspace_id::text, market_pack_id::text,
		  squad_id::text, snapshot::text, updated_at::text
`, issue.ID, issue.WorkspaceID, marketPackID, squadID, string(snapshot), requestingUserID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative context")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, context)
}

func (h *Handler) PutCreativeItemCopy(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "candidateId"), "candidate_id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		CopyEntryID string `json:"copy_entry_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	copyEntryID, ok := parseUUIDOrBadRequest(w, req.CopyEntryID, "copy_entry_id")
	if !ok {
		return
	}
	entry, err := scanCreativeCopyEntry(h.DB.QueryRow(r.Context(), creativeCopyEntrySelect+`
WHERE id = $1 AND workspace_id = $2 AND status <> 'disabled'
`, copyEntryID, issue.WorkspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "copy entry not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load copy entry")
		return
	}
	var candidateExists bool
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_material_issue_candidate
  WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = $3 AND status = 'selected'
)
`, issue.ID, issue.WorkspaceID, candidateID).Scan(&candidateExists); err != nil || !candidateExists {
		writeError(w, http.StatusUnprocessableEntity, "candidate must be selected before assigning copy")
		return
	}
	snapshot, _ := json.Marshal(entry)
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	item, err := scanCreativeIssueItem(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_issue_item (
  issue_id, candidate_id, workspace_id, copy_entry_id, copy_snapshot, updated_by
) VALUES ($1, $2, $3, $4, $5::jsonb, $6)
ON CONFLICT (issue_id, candidate_id) DO UPDATE SET
  copy_entry_id = EXCLUDED.copy_entry_id,
  copy_snapshot = EXCLUDED.copy_snapshot,
  work_issue_id = NULL,
  revision = creative_issue_item.revision + 1,
  status = 'ready',
  updated_by = EXCLUDED.updated_by,
  updated_at = now()
RETURNING issue_id::text, candidate_id::text, COALESCE(copy_entry_id::text, ''),
          copy_snapshot::text, creative_brief::text, COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
`, issue.ID, candidateID, issue.WorkspaceID, copyEntryID, string(snapshot), requestingUserID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to assign copy")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) PutCreativeItemBrief(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "candidateId"), "candidate_id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var input creativeBriefInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative brief")
		return
	}
	brief, err := normalizeCreativeBrief(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var candidateExists bool
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_material_issue_candidate
  WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = $3
)
`, issue.ID, issue.WorkspaceID, candidateID).Scan(&candidateExists); err != nil || !candidateExists {
		writeError(w, http.StatusUnprocessableEntity, "candidate must belong to the issue before saving a creative brief")
		return
	}
	encoded, err := json.Marshal(brief)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode creative brief")
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	if !requestingUserID.Valid {
		writeError(w, http.StatusForbidden, "requesting user not available")
		return
	}
	item, err := scanCreativeIssueItem(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_issue_item (
  issue_id, candidate_id, workspace_id, creative_brief, updated_by
) VALUES ($1, $2, $3, $4::jsonb, $5)
ON CONFLICT (issue_id, candidate_id) DO UPDATE SET
  creative_brief = EXCLUDED.creative_brief,
  work_issue_id = NULL,
  revision = creative_issue_item.revision + 1,
  status = 'ready',
  updated_by = EXCLUDED.updated_by,
  updated_at = now()
RETURNING issue_id::text, candidate_id::text, COALESCE(copy_entry_id::text, ''),
          copy_snapshot::text, creative_brief::text, COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
`, issue.ID, candidateID, issue.WorkspaceID, string(encoded), requestingUserID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative brief")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) PutCreativeItemWorkIssue(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "candidateId"), "candidate_id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		WorkIssueID string `json:"work_issue_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	workIssueID, ok := parseUUIDOrBadRequest(w, req.WorkIssueID, "work_issue_id")
	if !ok {
		return
	}
	var validChild bool
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM issue WHERE id = $1 AND workspace_id = $2 AND parent_issue_id = $3
)
`, workIssueID, issue.WorkspaceID, issue.ID).Scan(&validChild); err != nil || !validChild {
		writeError(w, http.StatusUnprocessableEntity, "work issue must be a direct child of the creative issue")
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	item, err := scanCreativeIssueItem(h.DB.QueryRow(r.Context(), `
UPDATE creative_issue_item
SET work_issue_id = $4, status = 'running', updated_by = $5, updated_at = now()
WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3
RETURNING issue_id::text, candidate_id::text, COALESCE(copy_entry_id::text, ''),
          copy_snapshot::text, creative_brief::text, COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
`, issue.ID, candidateID, issue.WorkspaceID, workIssueID, requestingUserID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "assign copy before starting creative work")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to bind creative work issue")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) loadCreativeIssueState(ctx context.Context, issueID, workspaceID pgtype.UUID) (*creativeIssueContextResponse, []creativeIssueItemResponse, error) {
	creativeContext, err := scanCreativeIssueContext(h.DB.QueryRow(ctx, `
SELECT issue_id::text, workspace_id::text, market_pack_id::text,
	   squad_id::text, snapshot::text, updated_at::text
FROM creative_issue_context
WHERE issue_id = $1 AND workspace_id = $2
`, issueID, workspaceID))
	var contextPtr *creativeIssueContextResponse
	if err == nil {
		contextPtr = &creativeContext
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	rows, err := h.DB.Query(ctx, `
SELECT issue_id::text, candidate_id::text, COALESCE(copy_entry_id::text, ''),
       copy_snapshot::text, creative_brief::text, COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
FROM creative_issue_item
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at
`, issueID, workspaceID)
	if err != nil {
		return contextPtr, nil, err
	}
	defer rows.Close()
	items := []creativeIssueItemResponse{}
	for rows.Next() {
		item, scanErr := scanCreativeIssueItem(rows)
		if scanErr != nil {
			return contextPtr, nil, scanErr
		}
		items = append(items, item)
	}
	return contextPtr, items, rows.Err()
}

func (h *Handler) buildCreativeIssueSnapshot(ctx context.Context, workspaceID, marketPackID, squadID pgtype.UUID) (json.RawMessage, error) {
	marketPack, err := h.loadPublishedCreativeResource(ctx, workspaceID, marketPackID, "market_pack")
	if err != nil {
		return nil, errors.New("market pack must have a published version")
	}
	var squadName string
	var leaderID pgtype.UUID
	if err := h.DB.QueryRow(ctx, `
SELECT name, leader_id FROM squad
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL
`, squadID, workspaceID).Scan(&squadName, &leaderID); err != nil {
		return nil, errors.New("squad is unavailable")
	}
	squadSnapshot, err := h.buildCreativeSquadSnapshot(ctx, squadID, squadName, leaderID)
	if err != nil {
		return nil, err
	}
	marketFiles, err := h.loadCreativeResourceFiles(ctx, workspaceID, marketPackID, marketPack.PublishedVersion)
	if err != nil {
		return nil, errors.New("market pack files are unavailable")
	}
	marketPackSnapshot := map[string]any{
		"id": marketPack.ID, "name": marketPack.Name, "description": marketPack.Description,
		"version": marketPack.PublishedVersion, "config": marketPack.Config, "files": marketFiles,
	}
	snapshot := map[string]any{
		"market_pack": marketPackSnapshot,
		"squad":       squadSnapshot,
	}
	var marketConfig map[string]any
	if json.Unmarshal(marketPack.Config, &marketConfig) == nil {
		if libraryID, _ := marketConfig["copy_library_id"].(string); strings.TrimSpace(libraryID) != "" {
			parsed, parseErr := parseUUIDString(libraryID)
			if parseErr != nil {
				return nil, errors.New("market pack copy library is invalid")
			}
			library, loadErr := h.loadPublishedCreativeResource(ctx, workspaceID, parsed, "copy_library")
			if loadErr != nil {
				return nil, errors.New("market pack copy library must have a published version")
			}
			snapshot["copy_library"] = library
		}
	}
	return json.Marshal(snapshot)
}

func (h *Handler) buildCreativeSquadSnapshot(ctx context.Context, squadID pgtype.UUID, squadName string, leaderID pgtype.UUID) (map[string]any, error) {
	leader, err := h.buildCreativeAgentSnapshot(ctx, leaderID, "leader")
	if err != nil {
		return nil, errors.New("squad leader skills are unavailable")
	}
	members, err := h.Queries.ListSquadMembers(ctx, squadID)
	if err != nil {
		return nil, errors.New("squad members are unavailable")
	}
	memberSnapshots := make([]map[string]any, 0, len(members))
	for _, member := range members {
		if member.MemberType != "agent" || member.MemberID == leaderID {
			continue
		}
		snapshot, snapshotErr := h.buildCreativeAgentSnapshot(ctx, member.MemberID, member.Role)
		if snapshotErr != nil {
			return nil, errors.New("squad member skills are unavailable")
		}
		memberSnapshots = append(memberSnapshots, snapshot)
	}
	return map[string]any{
		"id": uuidToString(squadID), "name": squadName,
		"leader_id": uuidToString(leaderID), "leader": leader, "members": memberSnapshots,
	}, nil
}

func (h *Handler) buildCreativeAgentSnapshot(ctx context.Context, agentID pgtype.UUID, role string) (map[string]any, error) {
	agent, err := h.Queries.GetAgent(ctx, agentID)
	if err != nil || agent.ArchivedAt.Valid {
		return nil, errors.New("agent is unavailable")
	}
	skills, err := h.Queries.ListAgentSkills(ctx, agentID)
	if err != nil {
		return nil, err
	}
	skillSnapshots := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		files, filesErr := h.Queries.ListSkillFiles(ctx, skill.ID)
		if filesErr != nil {
			return nil, filesErr
		}
		references := make([]map[string]any, 0, len(files))
		for _, file := range files {
			references = append(references, map[string]any{"path": file.Path, "content": file.Content})
		}
		var config map[string]any
		if json.Unmarshal(skill.Config, &config) != nil {
			config = map[string]any{}
		}
		skillSnapshots = append(skillSnapshots, map[string]any{
			"id": uuidToString(skill.ID), "name": skill.Name, "description": skill.Description,
			"content": skill.Content, "config": config, "references": references,
		})
	}
	return map[string]any{
		"id": uuidToString(agent.ID), "name": agent.Name, "description": agent.Description,
		"role": strings.TrimSpace(role), "skills": skillSnapshots,
	}, nil
}

func (h *Handler) loadPublishedCreativeResource(ctx context.Context, workspaceID, resourceID pgtype.UUID, kind string) (creativeResourceResponse, error) {
	return scanCreativeResource(h.DB.QueryRow(ctx, `
SELECT r.id::text, r.workspace_id::text, r.kind, rr.name, rr.description, r.status,
       rr.version, rr.version, rr.config::text, r.created_by::text,
       r.created_at::text, rr.created_at::text
FROM creative_resource r
JOIN creative_resource_revision rr
  ON rr.resource_id = r.id AND rr.version = r.published_version
WHERE r.id = $1 AND r.workspace_id = $2 AND r.kind = $3
  AND r.status = 'published' AND r.published_version IS NOT NULL
`, resourceID, workspaceID, kind))
}

func (h *Handler) loadCreativeResourceFiles(ctx context.Context, workspaceID, resourceID pgtype.UUID, version int) ([]creativeResourceFileResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id::text, resource_id::text, attachment_id::text, role, label, metadata::text,
       created_version, created_at::text
FROM creative_resource_file
WHERE resource_id = $1 AND workspace_id = $2
  AND (($3 = 0 AND removed_version IS NULL)
    OR ($3 > 0 AND created_version <= $3 AND (removed_version IS NULL OR removed_version > $3)))
ORDER BY role, created_at
`, resourceID, workspaceID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := []creativeResourceFileResponse{}
	for rows.Next() {
		var file creativeResourceFileResponse
		var metadata string
		if err := rows.Scan(
			&file.ID, &file.ResourceID, &file.AttachmentID, &file.Role, &file.Label,
			&metadata, &file.CreatedVersion, &file.CreatedAt,
		); err != nil {
			return nil, err
		}
		file.Metadata = json.RawMessage(metadata)
		attachmentID, err := parseUUIDString(file.AttachmentID)
		if err != nil {
			return nil, err
		}
		attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
		if err != nil || attachment.WorkspaceID != workspaceID {
			return nil, errors.New("market resource attachment is unavailable")
		}
		attachmentResponse := h.attachmentToResponse(attachment)
		file.Filename = attachmentResponse.Filename
		file.URL = attachmentResponse.MarkdownURL
		file.ContentType = attachmentResponse.ContentType
		file.SizeBytes = attachmentResponse.SizeBytes
		files = append(files, file)
	}
	return files, rows.Err()
}

func (h *Handler) requireCreativeResource(ctx context.Context, workspaceID, resourceID pgtype.UUID, kind string) (string, error) {
	var name string
	err := h.DB.QueryRow(ctx, `SELECT name FROM creative_resource WHERE id = $1 AND workspace_id = $2 AND kind = $3 AND status <> 'archived'`, resourceID, workspaceID, kind).Scan(&name)
	return name, err
}

func requireCreativeResourceTx(ctx context.Context, tx pgx.Tx, workspaceID, resourceID pgtype.UUID, kind string) (string, error) {
	var name string
	err := tx.QueryRow(ctx, `SELECT name FROM creative_resource WHERE id = $1 AND workspace_id = $2 AND kind = $3 AND status <> 'archived' FOR UPDATE`, resourceID, workspaceID, kind).Scan(&name)
	return name, err
}

func bumpCreativeLibraryRevision(ctx context.Context, tx pgx.Tx, workspaceID, libraryID, userID pgtype.UUID, sourceFilename string, mapping json.RawMessage) (creativeResourceResponse, error) {
	resource, err := scanCreativeResource(tx.QueryRow(ctx, `
UPDATE creative_resource
SET config = config || jsonb_build_object(
      'source_filename', $3::text,
      'import_mapping', $4::jsonb
    ),
    status = 'draft', version = version + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND kind = 'copy_library'
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, libraryID, workspaceID, strings.TrimSpace(sourceFilename), string(mapping)))
	if err != nil {
		return creativeResourceResponse{}, err
	}
	if err := insertCreativeResourceRevision(ctx, tx, resource, userID); err != nil {
		return creativeResourceResponse{}, err
	}
	return resource, nil
}

func bumpCreativeResourceRevision(ctx context.Context, tx pgx.Tx, workspaceID, resourceID, userID pgtype.UUID) (creativeResourceResponse, error) {
	resource, err := scanCreativeResource(tx.QueryRow(ctx, `
UPDATE creative_resource
SET config = config - 'qr_validation', status = 'draft', version = version + 1, updated_at = now()
WHERE id = $1 AND workspace_id = $2 AND status <> 'archived'
RETURNING id::text, workspace_id::text, kind, name, description, status, version,
          COALESCE(published_version, 0), config::text, created_by::text,
          created_at::text, updated_at::text
`, resourceID, workspaceID))
	if err != nil {
		return creativeResourceResponse{}, err
	}
	if err := insertCreativeResourceRevision(ctx, tx, resource, userID); err != nil {
		return creativeResourceResponse{}, err
	}
	return resource, nil
}

func insertCreativeResourceRevision(ctx context.Context, tx pgx.Tx, resource creativeResourceResponse, userID pgtype.UUID) error {
	_, err := tx.Exec(ctx, `
INSERT INTO creative_resource_revision (resource_id, version, name, description, config, created_by)
VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6)
`, resource.ID, resource.Version, resource.Name, resource.Description, string(resource.Config), userID)
	return err
}

func scanCreativeResource(row rowScanner) (creativeResourceResponse, error) {
	var item creativeResourceResponse
	var config string
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.Kind, &item.Name, &item.Description,
		&item.Status, &item.Version, &item.PublishedVersion, &config, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	)
	item.Config = json.RawMessage(config)
	return item, err
}

const creativeCopyEntrySelect = `
SELECT id::text, workspace_id::text, library_id::text, external_key, headline,
       subheadline, benefit, cta, legal_text, copy_role, market, locale, tags,
       status, version, metadata::text, created_by::text, created_at::text, updated_at::text
FROM creative_copy_entry
`

const creativeCopyEntryUpsertSQL = `
WITH upsert AS (
  INSERT INTO creative_copy_entry (
    workspace_id, library_id, external_key, headline, subheadline, benefit,
    cta, legal_text, copy_role, market, locale, tags, status, metadata, created_by
  ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14::jsonb, $15)
  ON CONFLICT (library_id, external_key) DO UPDATE SET
    headline = EXCLUDED.headline,
    subheadline = EXCLUDED.subheadline,
    benefit = EXCLUDED.benefit,
    cta = EXCLUDED.cta,
    legal_text = EXCLUDED.legal_text,
    copy_role = EXCLUDED.copy_role,
    market = EXCLUDED.market,
    locale = EXCLUDED.locale,
    tags = EXCLUDED.tags,
    status = EXCLUDED.status,
    metadata = EXCLUDED.metadata,
    version = creative_copy_entry.version + 1,
    updated_at = now()
  RETURNING xmax = 0 AS inserted
)
SELECT inserted FROM upsert
`

func scanCreativeCopyEntry(row rowScanner) (creativeCopyEntryResponse, error) {
	var item creativeCopyEntryResponse
	var metadata string
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.LibraryID, &item.ExternalKey, &item.Headline,
		&item.Subheadline, &item.Benefit, &item.CTA, &item.LegalText, &item.CopyRole,
		&item.Market, &item.Locale, &item.Tags, &item.Status, &item.Version, &metadata,
		&item.CreatedBy, &item.CreatedAt, &item.UpdatedAt,
	)
	item.Metadata = json.RawMessage(metadata)
	return item, err
}

func scanCreativeIssueContext(row rowScanner) (creativeIssueContextResponse, error) {
	var item creativeIssueContextResponse
	var snapshot string
	err := row.Scan(&item.IssueID, &item.WorkspaceID, &item.MarketPackID, &item.SquadID, &snapshot, &item.UpdatedAt)
	item.Snapshot = json.RawMessage(snapshot)
	return item, err
}

func scanCreativeIssueItem(row rowScanner) (creativeIssueItemResponse, error) {
	var item creativeIssueItemResponse
	var snapshot, brief string
	err := row.Scan(&item.IssueID, &item.CandidateID, &item.CopyEntryID, &snapshot, &brief, &item.WorkIssueID, &item.Revision, &item.Status, &item.UpdatedAt)
	item.CopySnapshot = json.RawMessage(snapshot)
	item.CreativeBrief = json.RawMessage(brief)
	return item, err
}

func normalizeCreativeBrief(input creativeBriefInput) (creativeBriefInput, error) {
	input.Theme = strings.TrimSpace(input.Theme)
	input.PrimaryBenefit = strings.TrimSpace(input.PrimaryBenefit)
	input.BenefitValue = strings.TrimSpace(input.BenefitValue)
	input.SourceSemantics = strings.TrimSpace(input.SourceSemantics)
	input.InformationMechanism = strings.TrimSpace(input.InformationMechanism)
	input.VisualType = strings.TrimSpace(input.VisualType)
	input.AnalysisSummary = strings.TrimSpace(input.AnalysisSummary)
	input.AnalysisIssueID = strings.TrimSpace(input.AnalysisIssueID)
	input.ThemeElements = normalizedCreativeBriefList(input.ThemeElements, 12)
	input.SecondaryBenefits = normalizedCreativeBriefList(input.SecondaryBenefits, 12)
	input.VisualAnchors = normalizedCreativeBriefList(input.VisualAnchors, 12)
	input.PaletteAnchors = normalizedCreativeBriefList(input.PaletteAnchors, 8)
	input.MustPreserve = normalizedCreativeBriefList(input.MustPreserve, 16)
	input.AllowedVariations = normalizedCreativeBriefList(input.AllowedVariations, 16)
	input.Evidence = normalizedCreativeBriefList(input.Evidence, 16)
	input.DetectedText = normalizedCreativeBriefList(input.DetectedText, 32)
	if len([]rune(input.Theme)) > 120 || len([]rune(input.PrimaryBenefit)) > 120 || len([]rune(input.BenefitValue)) > 160 || len([]rune(input.SourceSemantics)) > 300 || len([]rune(input.InformationMechanism)) > 300 || len([]rune(input.VisualType)) > 120 || len([]rune(input.AnalysisSummary)) > 2000 {
		return creativeBriefInput{}, errors.New("creative brief field is too long")
	}
	switch input.Status {
	case "requested", "draft", "confirmed":
	default:
		return creativeBriefInput{}, errors.New("creative brief status must be requested, draft, or confirmed")
	}
	switch input.Source {
	case "ai", "user", "mixed":
	default:
		return creativeBriefInput{}, errors.New("creative brief source must be ai, user, or mixed")
	}
	if input.Confidence != nil && (*input.Confidence < 0 || *input.Confidence > 1) {
		return creativeBriefInput{}, errors.New("creative brief confidence must be between 0 and 1")
	}
	return input, nil
}

func normalizedCreativeBriefList(values []string, limit int) []string {
	result := make([]string, 0, min(len(values), limit))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > 300 {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func creativeWorkspaceUser(w http.ResponseWriter, r *http.Request, h *Handler) (pgtype.UUID, pgtype.UUID, bool) {
	workspaceID, ok := parseUUIDOrBadRequest(w, h.resolveWorkspaceID(r), "workspace_id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	userIDRaw, ok := requireUserID(w, r)
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	userID, ok := parseUUIDOrBadRequest(w, userIDRaw, "user_id")
	if !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	return workspaceID, userID, true
}

func validCreativeResourceKind(kind string) bool {
	return kind == "copy_library" || kind == "market_pack"
}

func normalizedJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return json.RawMessage(`{}`), nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return nil, errors.New("not an object")
	}
	return json.Marshal(value)
}

func normalizeCreativeCopyEntry(input creativeCopyEntryInput, index int) creativeCopyEntryInput {
	input.ExternalKey = strings.TrimSpace(input.ExternalKey)
	input.Headline = strings.TrimSpace(input.Headline)
	input.Subheadline = strings.TrimSpace(input.Subheadline)
	input.Benefit = strings.TrimSpace(input.Benefit)
	input.CTA = strings.TrimSpace(input.CTA)
	input.LegalText = strings.TrimSpace(input.LegalText)
	input.CopyRole = strings.TrimSpace(input.CopyRole)
	input.Market = strings.TrimSpace(input.Market)
	input.Locale = strings.TrimSpace(input.Locale)
	input.Tags = uniqueNonEmptyStrings(input.Tags)
	if input.Status != "approved" && input.Status != "disabled" {
		input.Status = "draft"
	}
	metadata, err := normalizedJSONObject(input.Metadata)
	if err != nil {
		metadata = json.RawMessage(`{}`)
	}
	input.Metadata = metadata
	if input.ExternalKey == "" {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%s|%s|%s", index, input.Headline, input.Subheadline, input.Benefit, input.CTA)))
		input.ExternalKey = "copy:" + hex.EncodeToString(sum[:8])
	}
	return input
}

func parseUUIDString(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(strings.TrimSpace(value))
	return id, err
}
