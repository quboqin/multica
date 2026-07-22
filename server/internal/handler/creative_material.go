package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/creative"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type creativeMaterialCandidateResponse struct {
	ID                 string           `json:"id"`
	WorkspaceID        string           `json:"workspace_id"`
	ConnectorID        string           `json:"connector_id"`
	ExternalID         string           `json:"external_id"`
	DedupeKey          string           `json:"dedupe_key"`
	Competitor         string           `json:"competitor"`
	Title              string           `json:"title"`
	AssetType          string           `json:"asset_type"`
	PreviewURL         string           `json:"preview_url"`
	ResourceURL        string           `json:"resource_url"`
	PosterURL          string           `json:"poster_url"`
	OriginalURL        string           `json:"original_url"`
	ArchivedURL        string           `json:"archived_url"`
	ArchiveStatus      string           `json:"archive_status"`
	ArchiveError       string           `json:"archive_error"`
	DurationDays       *float64         `json:"duration_days"`
	ImpressionEstimate *int64           `json:"impression_estimate"`
	MediaNames         []string         `json:"media_names"`
	AreaNames          []string         `json:"area_names"`
	LanguageNames      []string         `json:"language_names"`
	PlatformNames      []string         `json:"platform_names"`
	Status             string           `json:"status"`
	Tags               []string         `json:"tags"`
	Note               string           `json:"note"`
	SelectedAt         *string          `json:"selected_at"`
	FirstSeenAt        string           `json:"first_seen_at"`
	LastSeenAt         string           `json:"last_seen_at"`
	CreatedAt          string           `json:"created_at"`
	UpdatedAt          string           `json:"updated_at"`
	Raw                *json.RawMessage `json:"raw,omitempty"`
}

type creativeMaterialSummaryResponse struct {
	Total      int `json:"total"`
	New        int `json:"new"`
	Selected   int `json:"selected"`
	Rejected   int `json:"rejected"`
	SentToEdit int `json:"sent_to_edit"`
	Edited     int `json:"edited"`
	Approved   int `json:"approved"`
	Archived   int `json:"archived"`
}

type creativeMaterialCrawlRunResponse struct {
	ID            string `json:"id"`
	WorkspaceID   string `json:"workspace_id"`
	IssueID       string `json:"issue_id"`
	ConnectorID   string `json:"connector_id"`
	QuerySummary  string `json:"query_summary"`
	Status        string `json:"status"`
	ImportedCount int    `json:"imported_count"`
	ExistingCount int    `json:"existing_count"`
	TotalCount    int    `json:"total_count"`
	CreatedAt     string `json:"created_at"`
}

type creativeEditAssetResponse struct {
	ID          string `json:"id"`
	VariantID   string `json:"variant_id"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Label       string `json:"label"`
	AssetURL    string `json:"asset_url"`
	ContentType string `json:"content_type"`
	CreatedAt   string `json:"created_at"`
}

type creativeEditVariantResponse struct {
	ID           string                         `json:"id"`
	JobID        string                         `json:"job_id"`
	CandidateID  string                         `json:"candidate_id"`
	VariantIndex int                            `json:"variant_index"`
	Title        string                         `json:"title"`
	Description  string                         `json:"description"`
	QCStatus     string                         `json:"qc_status"`
	CreatedAt    string                         `json:"created_at"`
	Assets       []creativeEditAssetResponse    `json:"assets"`
	Feedback     []creativeEditFeedbackResponse `json:"feedback"`
}

type creativeEditJobResponse struct {
	ID               string                        `json:"id"`
	WorkspaceID      string                        `json:"workspace_id"`
	IssueID          string                        `json:"issue_id"`
	Status           string                        `json:"status"`
	Prompt           string                        `json:"prompt"`
	Rules            json.RawMessage               `json:"rules"`
	ProcessData      json.RawMessage               `json:"process_data"`
	ExternalProvider string                        `json:"external_provider"`
	MCPConnectionID  string                        `json:"mcp_connection_id"`
	ExternalJobID    string                        `json:"external_job_id"`
	ExternalStatus   string                        `json:"external_status"`
	Stage            string                        `json:"stage"`
	Progress         int                           `json:"progress"`
	LastPollAt       string                        `json:"last_poll_at"`
	NextPollAt       string                        `json:"next_poll_at"`
	CompletedAt      string                        `json:"completed_at"`
	ErrorMessage     string                        `json:"error_message"`
	PollAttempts     int                           `json:"poll_attempts"`
	CreatedAt        string                        `json:"created_at"`
	UpdatedAt        string                        `json:"updated_at"`
	CandidateIDs     []string                      `json:"candidate_ids"`
	Variants         []creativeEditVariantResponse `json:"variants"`
}

type creativeMaterialsResponse struct {
	Enabled    bool                                `json:"enabled"`
	Summary    creativeMaterialSummaryResponse     `json:"summary"`
	Candidates []creativeMaterialCandidateResponse `json:"candidates"`
	CrawlRuns  []creativeMaterialCrawlRunResponse  `json:"crawl_runs"`
	EditJobs   []creativeEditJobResponse           `json:"edit_jobs"`
}

type creativeImportSummary struct {
	RunID         string `json:"run_id"`
	ImportedCount int    `json:"imported_count"`
	ExistingCount int    `json:"existing_count"`
	TotalCount    int    `json:"total_count"`
	SkippedCount  int    `json:"skipped_count"`
}

const currentCreativeProcessDataSchemaVersion = 2

type creativeMaterialInput struct {
	ExternalID         string          `json:"external_id"`
	DedupeKey          string          `json:"dedupe_key"`
	Competitor         string          `json:"competitor"`
	Title              string          `json:"title"`
	AssetType          string          `json:"asset_type"`
	PreviewURL         string          `json:"preview_url"`
	ResourceURL        string          `json:"resource_url"`
	PosterURL          string          `json:"poster_url"`
	OriginalURL        string          `json:"original_url"`
	DurationDays       *float64        `json:"duration_days"`
	ImpressionEstimate *int64          `json:"impression_estimate"`
	MediaNames         []string        `json:"media_names"`
	AreaNames          []string        `json:"area_names"`
	LanguageNames      []string        `json:"language_names"`
	PlatformNames      []string        `json:"platform_names"`
	Raw                json.RawMessage `json:"raw"`
}

func (h *Handler) GetCreativeMaterials(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	resp, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		slog.Warn("load creative materials failed", append(logger.RequestAttrs(r), "error", err, "issue_id", chi.URLParam(r, "id"))...)
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) ImportCreativeMaterials(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	if !requestingUserID.Valid {
		writeError(w, http.StatusForbidden, "requesting user not available")
		return
	}

	var req struct {
		ConnectorID  string                  `json:"connector_id"`
		QuerySummary string                  `json:"query_summary"`
		Params       json.RawMessage         `json:"params"`
		Materials    []creativeMaterialInput `json:"materials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	summary, err := h.importCreativeMaterialsForIssue(r.Context(), creativeMaterialImportInput{
		IssueID:      issue.ID,
		WorkspaceID:  issue.WorkspaceID,
		ConnectorID:  req.ConnectorID,
		QuerySummary: req.QuerySummary,
		Params:       req.Params,
		Materials:    req.Materials,
		ActorType:    actorType,
		ActorID:      actorID,
		UserID:       requestingUserID,
	})
	if err != nil {
		slog.Warn("import creative materials failed", append(logger.RequestAttrs(r), "error", err, "issue_id", chi.URLParam(r, "id"))...)
		writeError(w, http.StatusInternalServerError, "failed to import creative materials")
		return
	}
	writeJSON(w, http.StatusCreated, summary)
}

func (h *Handler) UpdateCreativeMaterialCandidate(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	candidateID := strings.TrimSpace(chi.URLParam(r, "candidateId"))
	if candidateID == "" {
		writeError(w, http.StatusBadRequest, "candidate_id is required")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	requestingUserID := h.requestingUserIDFromRequest(r, actorType, actorID)
	if !requestingUserID.Valid {
		writeError(w, http.StatusForbidden, "requesting user not available")
		return
	}

	var req struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	status := strings.TrimSpace(req.Status)
	if !validCreativeCandidateStatus(status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	var selectedBy any
	if status == "selected" {
		selectedBy = requestingUserID
	}
	tag, err := h.DB.Exec(r.Context(), `
UPDATE creative_material_issue_candidate
SET status = $3,
    note = $4,
    selected_by = CASE WHEN $3 = 'selected' THEN $5::uuid ELSE NULL END,
    selected_at = CASE WHEN $3 = 'selected' THEN now() ELSE NULL END,
    updated_at = now()
WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = $6::uuid
`, issue.ID, issue.WorkspaceID, status, strings.TrimSpace(req.Note), selectedBy, candidateID)
	if err != nil {
		slog.Warn("update creative material candidate failed", append(logger.RequestAttrs(r), "error", err, "candidate_id", candidateID)...)
		writeError(w, http.StatusInternalServerError, "failed to update creative material candidate")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "creative material candidate not found")
		return
	}
	resp, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) CreateCreativeEditJob(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))

	var req struct {
		CandidateIDs []string       `json:"candidate_ids"`
		Prompt       string         `json:"prompt"`
		Rules        map[string]any `json:"rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	candidateIDs := uniqueNonEmptyStrings(req.CandidateIDs)
	if len(candidateIDs) == 0 {
		var err error
		candidateIDs, err = h.listSelectedCreativeCandidateIDs(r.Context(), issue.ID, issue.WorkspaceID, 20)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load selected creative materials")
			return
		}
		if len(candidateIDs) == 0 {
			writeError(w, http.StatusBadRequest, "select at least one creative material")
			return
		}
	}
	if len(candidateIDs) > 20 {
		writeError(w, http.StatusBadRequest, "candidate_ids cannot exceed 20")
		return
	}

	rules, err := normalizeCreativeEditRules(req.Rules)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobID, err := h.createAsyncCreativeEditJob(r.Context(), issue.ID, issue.WorkspaceID, actorType, actorID, candidateIDs, strings.TrimSpace(req.Prompt), rules)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "selected creative materials not found")
			return
		}
		slog.Warn("create creative edit job failed", append(logger.RequestAttrs(r), "error", err, "issue_id", chi.URLParam(r, "id"))...)
		writeError(w, http.StatusInternalServerError, "failed to create creative edit job")
		return
	}
	if err := h.reconcileCreativeEditJob(r.Context(), issue.ID, issue.WorkspaceID, parseUUID(jobID)); err != nil {
		slog.Warn("initial creative edit job sync failed", append(logger.RequestAttrs(r), "error", err, "job_id", jobID)...)
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	resp, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) SyncCreativeEditJob(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	jobID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "jobId"), "job_id")
	if !ok {
		return
	}
	if err := h.reconcileCreativeEditJob(r.Context(), issue.ID, issue.WorkspaceID, jobID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "creative edit job not found")
			return
		}
		slog.Warn("sync creative edit job failed", append(logger.RequestAttrs(r), "error", err, "issue_id", chi.URLParam(r, "id"))...)
		writeError(w, http.StatusInternalServerError, "failed to sync creative edit job")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	resp, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type creativeMaterialImportInput struct {
	IssueID      pgtype.UUID
	WorkspaceID  pgtype.UUID
	ConnectorID  string
	QuerySummary string
	Params       json.RawMessage
	Materials    []creativeMaterialInput
	ActorType    string
	ActorID      string
	UserID       pgtype.UUID
}

func (h *Handler) importCreativeMaterialsForIssue(ctx context.Context, in creativeMaterialImportInput) (creativeImportSummary, error) {
	if h.TxStarter == nil {
		return creativeImportSummary{}, errors.New("transaction starter not configured")
	}
	connectorID := strings.TrimSpace(in.ConnectorID)
	if connectorID == "" {
		connectorID = "appgrowing"
	}
	params := strings.TrimSpace(string(in.Params))
	if params == "" {
		params = "{}"
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return creativeImportSummary{}, err
	}
	defer tx.Rollback(ctx)

	var runID string
	err = tx.QueryRow(ctx, `
INSERT INTO creative_material_crawl_run (
  workspace_id, issue_id, connector_id, query_summary, params, status,
  created_by_type, created_by_id
) VALUES ($1, $2, $3, $4, $5::jsonb, 'completed', $6, $7::uuid)
RETURNING id::text
`, in.WorkspaceID, in.IssueID, connectorID, strings.TrimSpace(in.QuerySummary), params, in.ActorType, in.ActorID).Scan(&runID)
	if err != nil {
		return creativeImportSummary{}, err
	}

	summary := creativeImportSummary{RunID: runID, TotalCount: len(in.Materials)}
	for _, material := range in.Materials {
		material = normalizeCreativeMaterialInput(material)
		dedupeKey := creativeMaterialDedupeKey(material)
		if dedupeKey == "" {
			summary.SkippedCount++
			continue
		}
		raw := strings.TrimSpace(string(material.Raw))
		if raw == "" {
			raw = "{}"
		}
		var candidateID string
		var inserted bool
		err := tx.QueryRow(ctx, `
WITH upsert AS (
  INSERT INTO creative_material_candidate (
    workspace_id, connector_id, external_id, dedupe_key, competitor, title,
    asset_type, preview_url, resource_url, poster_url, original_url,
    duration_days, impression_estimate, media_names, area_names,
    language_names, platform_names, raw
  ) VALUES (
    $1, $2, NULLIF($3, ''), $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16, $17, $18::jsonb
  )
  ON CONFLICT (workspace_id, connector_id, dedupe_key) DO UPDATE SET
    external_id = COALESCE(EXCLUDED.external_id, creative_material_candidate.external_id),
    competitor = COALESCE(NULLIF(EXCLUDED.competitor, ''), creative_material_candidate.competitor),
    title = COALESCE(NULLIF(EXCLUDED.title, ''), creative_material_candidate.title),
    asset_type = CASE WHEN EXCLUDED.asset_type = 'unknown' THEN creative_material_candidate.asset_type ELSE EXCLUDED.asset_type END,
    preview_url = COALESCE(NULLIF(EXCLUDED.preview_url, ''), creative_material_candidate.preview_url),
    resource_url = COALESCE(NULLIF(EXCLUDED.resource_url, ''), creative_material_candidate.resource_url),
    poster_url = COALESCE(NULLIF(EXCLUDED.poster_url, ''), creative_material_candidate.poster_url),
    original_url = COALESCE(NULLIF(EXCLUDED.original_url, ''), creative_material_candidate.original_url),
    duration_days = COALESCE(EXCLUDED.duration_days, creative_material_candidate.duration_days),
    impression_estimate = COALESCE(EXCLUDED.impression_estimate, creative_material_candidate.impression_estimate),
    media_names = CASE WHEN cardinality(EXCLUDED.media_names) > 0 THEN EXCLUDED.media_names ELSE creative_material_candidate.media_names END,
    area_names = CASE WHEN cardinality(EXCLUDED.area_names) > 0 THEN EXCLUDED.area_names ELSE creative_material_candidate.area_names END,
    language_names = CASE WHEN cardinality(EXCLUDED.language_names) > 0 THEN EXCLUDED.language_names ELSE creative_material_candidate.language_names END,
    platform_names = CASE WHEN cardinality(EXCLUDED.platform_names) > 0 THEN EXCLUDED.platform_names ELSE creative_material_candidate.platform_names END,
    raw = CASE WHEN EXCLUDED.raw <> '{}'::jsonb THEN EXCLUDED.raw ELSE creative_material_candidate.raw END,
    archive_status = CASE
      WHEN creative_material_candidate.archived_url = '' THEN 'pending'
      ELSE creative_material_candidate.archive_status
    END,
    archive_error = CASE
      WHEN creative_material_candidate.archived_url = '' THEN ''
      ELSE creative_material_candidate.archive_error
    END,
    next_archive_at = CASE
      WHEN creative_material_candidate.archived_url = '' THEN now()
      ELSE creative_material_candidate.next_archive_at
    END,
    last_seen_at = now(),
    updated_at = now()
  RETURNING id, (xmax = 0) AS inserted
)
SELECT id::text, inserted FROM upsert
`, in.WorkspaceID, connectorID, material.ExternalID, dedupeKey, material.Competitor, material.Title,
			material.AssetType, material.PreviewURL, material.ResourceURL, material.PosterURL, material.OriginalURL,
			material.DurationDays, material.ImpressionEstimate, material.MediaNames, material.AreaNames,
			material.LanguageNames, material.PlatformNames, raw).Scan(&candidateID, &inserted)
		if err != nil {
			return creativeImportSummary{}, err
		}
		if inserted {
			summary.ImportedCount++
		} else {
			summary.ExistingCount++
		}
		_, err = tx.Exec(ctx, `
INSERT INTO creative_material_issue_candidate (
  issue_id, candidate_id, workspace_id, source_run_id
) VALUES ($1, $2::uuid, $3, $4::uuid)
ON CONFLICT (issue_id, candidate_id) DO UPDATE SET
  source_run_id = EXCLUDED.source_run_id,
  updated_at = now()
`, in.IssueID, candidateID, in.WorkspaceID, runID)
		if err != nil {
			return creativeImportSummary{}, err
		}
	}

	_, err = tx.Exec(ctx, `
UPDATE creative_material_crawl_run
SET imported_count = $3, existing_count = $4, total_count = $5
WHERE id = $1::uuid AND workspace_id = $2
`, runID, in.WorkspaceID, summary.ImportedCount, summary.ExistingCount, summary.TotalCount)
	if err != nil {
		return creativeImportSummary{}, err
	}
	_, err = tx.Exec(ctx, `
UPDATE issue
SET metadata = jsonb_set(metadata, '{workflow}', '"creative_material"'::jsonb),
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
`, in.IssueID, in.WorkspaceID)
	if err != nil {
		return creativeImportSummary{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return creativeImportSummary{}, err
	}
	h.publishCreativeMaterialsUpdated(in.WorkspaceID, in.IssueID, in.ActorType, in.ActorID)
	return summary, nil
}

func (h *Handler) loadCreativeMaterialsResponse(ctx context.Context, issueID, workspaceID pgtype.UUID, metadata map[string]any) (creativeMaterialsResponse, error) {
	resp := creativeMaterialsResponse{
		Enabled: fmt.Sprint(metadata["workflow"]) == "creative_material",
	}
	candidates, err := h.listCreativeCandidates(ctx, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.Candidates = candidates
	for _, candidate := range candidates {
		resp.Summary.Total++
		switch candidate.Status {
		case "new":
			resp.Summary.New++
		case "selected":
			resp.Summary.Selected++
		case "rejected":
			resp.Summary.Rejected++
		case "sent_to_edit":
			resp.Summary.SentToEdit++
		case "edited":
			resp.Summary.Edited++
		case "approved":
			resp.Summary.Approved++
		case "archived":
			resp.Summary.Archived++
		}
	}
	runs, err := h.listCreativeCrawlRuns(ctx, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.CrawlRuns = runs
	jobs, err := h.listCreativeEditJobs(ctx, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.EditJobs = jobs
	if len(resp.Candidates) > 0 || len(resp.CrawlRuns) > 0 || len(resp.EditJobs) > 0 {
		resp.Enabled = true
	}
	return resp, nil
}

func (h *Handler) listCreativeCandidates(ctx context.Context, issueID, workspaceID pgtype.UUID) ([]creativeMaterialCandidateResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT
  c.id::text, c.workspace_id::text, c.connector_id, COALESCE(c.external_id, ''),
  c.dedupe_key, c.competitor, c.title, c.asset_type,
  c.preview_url, c.resource_url, c.poster_url, c.original_url,
  c.archived_url, c.archive_status, c.archive_error,
  c.duration_days, c.impression_estimate, c.media_names, c.area_names,
  c.language_names, c.platform_names, ic.status, ic.tags, ic.note,
  COALESCE(ic.selected_at::text, ''), c.first_seen_at::text, c.last_seen_at::text,
  c.created_at::text, c.updated_at::text
FROM creative_material_issue_candidate ic
JOIN creative_material_candidate c ON c.id = ic.candidate_id
WHERE ic.issue_id = $1 AND ic.workspace_id = $2
ORDER BY
  CASE ic.status
    WHEN 'selected' THEN 0
    WHEN 'new' THEN 1
    WHEN 'edited' THEN 2
    WHEN 'sent_to_edit' THEN 3
    WHEN 'approved' THEN 4
    WHEN 'rejected' THEN 5
    ELSE 6
  END,
  c.last_seen_at DESC
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creativeMaterialCandidateResponse{}
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
			&item.LanguageNames, &item.PlatformNames, &item.Status, &item.Tags, &item.Note,
			&selectedAt, &item.FirstSeenAt, &item.LastSeenAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.DurationDays = float8Ptr(duration)
		item.ImpressionEstimate = int8Ptr(impression)
		if selectedAt != "" {
			item.SelectedAt = &selectedAt
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) listCreativeCrawlRuns(ctx context.Context, issueID, workspaceID pgtype.UUID) ([]creativeMaterialCrawlRunResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), connector_id,
       query_summary, status, imported_count, existing_count, total_count, created_at::text
FROM creative_material_crawl_run
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at DESC
LIMIT 20
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creativeMaterialCrawlRunResponse{}
	for rows.Next() {
		var item creativeMaterialCrawlRunResponse
		if err := rows.Scan(&item.ID, &item.WorkspaceID, &item.IssueID, &item.ConnectorID, &item.QuerySummary, &item.Status, &item.ImportedCount, &item.ExistingCount, &item.TotalCount, &item.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) listCreativeEditJobs(ctx context.Context, issueID, workspaceID pgtype.UUID) ([]creativeEditJobResponse, error) {
	rows, err := h.DB.Query(ctx, `
SELECT id::text, workspace_id::text, issue_id::text, status, prompt, rules::text, process_data::text,
       external_provider, COALESCE(mcp_connection_id::text, ''),
       external_job_id, external_status, stage, progress,
       COALESCE(last_poll_at::text, ''), COALESCE(next_poll_at::text, ''),
       COALESCE(completed_at::text, ''), error_message, poll_attempts,
       created_at::text, updated_at::text
FROM creative_edit_job
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at DESC
LIMIT 20
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []creativeEditJobResponse{}
	jobByID := map[string]int{}
	for rows.Next() {
		var job creativeEditJobResponse
		var rulesRaw string
		var processDataRaw string
		if err := rows.Scan(
			&job.ID, &job.WorkspaceID, &job.IssueID, &job.Status, &job.Prompt, &rulesRaw, &processDataRaw,
			&job.ExternalProvider, &job.MCPConnectionID, &job.ExternalJobID, &job.ExternalStatus, &job.Stage, &job.Progress,
			&job.LastPollAt, &job.NextPollAt, &job.CompletedAt, &job.ErrorMessage, &job.PollAttempts,
			&job.CreatedAt, &job.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if strings.TrimSpace(rulesRaw) == "" {
			rulesRaw = "{}"
		}
		job.Rules = json.RawMessage(rulesRaw)
		if strings.TrimSpace(processDataRaw) == "" {
			processDataRaw = "{}"
		}
		job.ProcessData = json.RawMessage(processDataRaw)
		job.CandidateIDs = []string{}
		job.Variants = []creativeEditVariantResponse{}
		jobByID[job.ID] = len(jobs)
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return jobs, nil
	}

	candidateRows, err := h.DB.Query(ctx, `
SELECT jc.job_id::text, jc.candidate_id::text
FROM creative_edit_job_candidate jc
JOIN creative_edit_job j ON j.id = jc.job_id
WHERE j.issue_id = $1 AND j.workspace_id = $2
ORDER BY j.created_at DESC, jc.candidate_id::text
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer candidateRows.Close()
	candidateSets := map[string]map[string]struct{}{}
	for candidateRows.Next() {
		var jobID string
		var candidateID string
		if err := candidateRows.Scan(&jobID, &candidateID); err != nil {
			return nil, err
		}
		if _, ok := jobByID[jobID]; !ok {
			continue
		}
		if candidateSets[jobID] == nil {
			candidateSets[jobID] = map[string]struct{}{}
		}
		candidateSets[jobID][candidateID] = struct{}{}
	}
	if err := candidateRows.Err(); err != nil {
		return nil, err
	}

	variantRows, err := h.DB.Query(ctx, `
SELECT v.id::text, v.job_id::text, v.candidate_id::text, v.variant_index,
       v.title, v.description, v.qc_status, v.created_at::text,
       COALESCE(a.id::text, ''), COALESCE(a.width, 0), COALESCE(a.height, 0),
       COALESCE(a.label, ''), COALESCE(a.asset_url, ''), COALESCE(a.content_type, ''),
       COALESCE(a.created_at::text, '')
FROM creative_edit_variant v
JOIN creative_edit_job j ON j.id = v.job_id
LEFT JOIN creative_edit_asset a ON a.variant_id = v.id
WHERE j.issue_id = $1 AND j.workspace_id = $2
ORDER BY j.created_at DESC, v.candidate_id, v.variant_index, a.width DESC, a.height DESC
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer variantRows.Close()
	type variantLocation struct {
		jobIndex     int
		variantIndex int
	}
	variantIndexByKey := map[string]int{}
	variantLocations := map[string]variantLocation{}
	for variantRows.Next() {
		var variant creativeEditVariantResponse
		var asset creativeEditAssetResponse
		if err := variantRows.Scan(
			&variant.ID, &variant.JobID, &variant.CandidateID, &variant.VariantIndex,
			&variant.Title, &variant.Description, &variant.QCStatus, &variant.CreatedAt,
			&asset.ID, &asset.Width, &asset.Height, &asset.Label, &asset.AssetURL, &asset.ContentType, &asset.CreatedAt,
		); err != nil {
			return nil, err
		}
		jobIdx, ok := jobByID[variant.JobID]
		if !ok {
			continue
		}
		key := variant.JobID + ":" + variant.ID
		var vIdx int
		if existing, ok := variantIndexByKey[key]; ok {
			vIdx = existing
		} else {
			variant.Assets = []creativeEditAssetResponse{}
			variant.Feedback = []creativeEditFeedbackResponse{}
			jobs[jobIdx].Variants = append(jobs[jobIdx].Variants, variant)
			vIdx = len(jobs[jobIdx].Variants) - 1
			variantIndexByKey[key] = vIdx
			variantLocations[variant.ID] = variantLocation{jobIndex: jobIdx, variantIndex: vIdx}
		}
		if asset.ID != "" {
			asset.VariantID = variant.ID
			jobs[jobIdx].Variants[vIdx].Assets = append(jobs[jobIdx].Variants[vIdx].Assets, asset)
		}
	}
	if err := variantRows.Err(); err != nil {
		return nil, err
	}

	feedbackRows, err := h.DB.Query(ctx, `
SELECT f.id::text, f.workspace_id::text, f.issue_id::text, f.job_id::text,
       f.candidate_id::text, f.variant_id::text, f.decision, f.reason_codes,
       f.suggestion, f.process_snapshot::text, COALESCE(f.created_by::text, ''),
       f.created_by_name, f.created_at::text
FROM creative_edit_feedback f
JOIN creative_edit_variant v
  ON v.id = f.variant_id AND v.job_id = f.job_id AND v.candidate_id = f.candidate_id
JOIN creative_edit_job j
  ON j.id = f.job_id AND j.issue_id = f.issue_id AND j.workspace_id = f.workspace_id
WHERE f.issue_id = $1 AND f.workspace_id = $2
ORDER BY f.created_at DESC, f.id DESC
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer feedbackRows.Close()
	for feedbackRows.Next() {
		var feedback creativeEditFeedbackResponse
		var processSnapshot string
		if err := feedbackRows.Scan(
			&feedback.ID, &feedback.WorkspaceID, &feedback.IssueID, &feedback.JobID,
			&feedback.CandidateID, &feedback.VariantID, &feedback.Decision, &feedback.ReasonCodes,
			&feedback.Suggestion, &processSnapshot, &feedback.CreatedBy,
			&feedback.CreatedByName, &feedback.CreatedAt,
		); err != nil {
			return nil, err
		}
		feedback.ProcessSnapshot = json.RawMessage(processSnapshot)
		location, ok := variantLocations[feedback.VariantID]
		if !ok {
			continue
		}
		jobs[location.jobIndex].Variants[location.variantIndex].Feedback = append(
			jobs[location.jobIndex].Variants[location.variantIndex].Feedback,
			feedback,
		)
	}
	if err := feedbackRows.Err(); err != nil {
		return nil, err
	}
	for i := range jobs {
		set := candidateSets[jobs[i].ID]
		for id := range set {
			jobs[i].CandidateIDs = append(jobs[i].CandidateIDs, id)
		}
		sort.Strings(jobs[i].CandidateIDs)
	}
	return jobs, nil
}

func (h *Handler) createAsyncCreativeEditJob(
	ctx context.Context,
	issueID, workspaceID pgtype.UUID,
	actorType, actorID string,
	candidateIDs []string,
	prompt string,
	rules map[string]any,
) (string, error) {
	if h.TxStarter == nil {
		return "", errors.New("transaction starter not configured")
	}
	resolved, err := h.resolveCreativeProvider(ctx, uuidToString(workspaceID), "", true)
	if err != nil {
		return "", err
	}
	provider := resolved.Provider
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	for _, candidateID := range candidateIDs {
		var exists bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM creative_material_issue_candidate
  WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = $3::uuid AND status = 'selected'
)
`, issueID, workspaceID, candidateID).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return "", pgx.ErrNoRows
		}
	}

	rules["mode"] = "async"
	rules["mcp"] = map[string]any{
		"provider": provider.Name(),
		"tools": map[string]string{
			"create": creative.CreateJobToolName,
			"get":    creative.GetJobToolName,
		},
	}
	rulesBytes, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}

	var jobID string
	if err := tx.QueryRow(ctx, `
INSERT INTO creative_edit_job (
  workspace_id, issue_id, status, prompt, rules, created_by_type, created_by_id,
  external_provider, mcp_connection_id, external_status, stage, progress, next_poll_at
) VALUES ($1, $2, 'queued', $3, $4::jsonb, $5, $6::uuid, $7, $8::uuid, 'queued', $9, 0, now())
RETURNING id::text
`, workspaceID, issueID, prompt, string(rulesBytes), actorType, actorID, provider.Name(), nullableUUIDString(resolved.ConnectionID), creative.CreateJobToolName).Scan(&jobID); err != nil {
		return "", err
	}

	for _, candidateID := range candidateIDs {
		if _, err := tx.Exec(ctx, `
INSERT INTO creative_edit_job_candidate (job_id, candidate_id)
VALUES ($1::uuid, $2::uuid)
`, jobID, candidateID); err != nil {
			return "", err
		}
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_material_issue_candidate
SET status = 'sent_to_edit', updated_at = now()
WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = ANY($3::uuid[])
`, issueID, workspaceID, candidateIDs); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return jobID, nil
}

func (h *Handler) reconcileCreativeEditJob(ctx context.Context, issueID, workspaceID, jobID pgtype.UUID) error {
	if h.TxStarter == nil {
		return errors.New("transaction starter not configured")
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var localJobID string
	var status string
	var externalJobID string
	var connectionID string
	var prompt string
	var rulesRaw string
	var processDataRaw string
	var createdAt time.Time
	if err := tx.QueryRow(ctx, `
SELECT id::text, status, external_job_id, COALESCE(mcp_connection_id::text, ''),
       prompt, rules::text, process_data::text, created_at
FROM creative_edit_job
WHERE id = $1 AND issue_id = $2 AND workspace_id = $3
FOR UPDATE
`, jobID, issueID, workspaceID).Scan(
		&localJobID, &status, &externalJobID, &connectionID, &prompt, &rulesRaw,
		&processDataRaw, &createdAt,
	); err != nil {
		return err
	}
	terminalProcessUpgrade := status == "completed" || status == "failed"
	if terminalProcessUpgrade && creativeProcessDataSchemaVersion(processDataRaw) >= currentCreativeProcessDataSchemaVersion {
		return tx.Commit(ctx)
	}
	resolved, err := h.resolveCreativeProvider(ctx, uuidToString(workspaceID), connectionID, false)
	if err != nil {
		if terminalProcessUpgrade {
			return fmt.Errorf("upgrade terminal creative edit job %s process_data: resolve provider: %w", localJobID, err)
		}
		if recordErr := recordCreativeProviderError(ctx, tx, jobID, "resolve_workspace_mcp", err); recordErr != nil {
			return recordErr
		}
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return commitErr
		}
		return err
	}
	provider := resolved.Provider

	rules := map[string]any{}
	if err := json.Unmarshal([]byte(rulesRaw), &rules); err != nil {
		return err
	}
	candidates, err := h.listCreativeEditJobCandidates(ctx, tx, localJobID, issueID, workspaceID)
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		return pgx.ErrNoRows
	}
	if terminalProcessUpgrade {
		if strings.TrimSpace(externalJobID) == "" {
			return fmt.Errorf("upgrade terminal creative edit job %s process_data: external job id is empty", localJobID)
		}
		result, getErr := provider.GetJob(ctx, creative.GetJobRequest{
			ExternalJobID: externalJobID,
			Prompt:        prompt,
			Rules:         rules,
			Candidates:    candidates,
			CreatedAt:     createdAt,
		})
		if getErr != nil {
			return fmt.Errorf("upgrade terminal creative edit job %s process_data: %s: %w", localJobID, creative.GetJobToolName, getErr)
		}
		version := creativeProcessDataSchemaVersion(string(result.ProcessData))
		if version < currentCreativeProcessDataSchemaVersion {
			return fmt.Errorf(
				"upgrade terminal creative edit job %s process_data: provider returned schema version %d, want at least %d",
				localJobID, version, currentCreativeProcessDataSchemaVersion,
			)
		}
		if _, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET process_data = $2::jsonb
WHERE id = $1
`, jobID, string(result.ProcessData)); err != nil {
			return fmt.Errorf("upgrade terminal creative edit job %s process_data: persist: %w", localJobID, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		h.publishCreativeMaterialsUpdated(workspaceID, issueID, "system", "")
		return nil
	}

	if strings.TrimSpace(externalJobID) == "" {
		created, createErr := provider.CreateJob(ctx, creative.CreateJobRequest{
			LocalJobID: localJobID,
			Prompt:     prompt,
			Rules:      rules,
			Candidates: candidates,
			CreatedAt:  createdAt,
		})
		if createErr != nil {
			if err := recordCreativeProviderError(ctx, tx, jobID, creative.CreateJobToolName, createErr); err != nil {
				return err
			}
			if err := tx.Commit(ctx); err != nil {
				return err
			}
			return createErr
		}
		nextPollAt := creativeNextPollAt(created.PollAfter)
		processData := creativeProcessDataArg(created.ProcessData)
		if _, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET status = 'running', external_job_id = $2, external_status = $3,
    stage = $4, progress = $5, last_poll_at = now(), next_poll_at = $6,
    process_data = COALESCE($7::jsonb, process_data),
    poll_attempts = poll_attempts + 1, error_message = '', updated_at = now()
WHERE id = $1
`, jobID, created.ExternalJobID, firstNonEmpty(created.Status, "running"), firstNonEmpty(created.Stage, creative.CreateJobToolName), clampCreativeProgress(created.Progress), nextPollAt, processData); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		h.publishCreativeMaterialsUpdated(workspaceID, issueID, "system", "")
		return nil
	}

	result, getErr := provider.GetJob(ctx, creative.GetJobRequest{
		ExternalJobID: externalJobID,
		Prompt:        prompt,
		Rules:         rules,
		Candidates:    candidates,
		CreatedAt:     createdAt,
	})
	if getErr != nil {
		if err := recordCreativeProviderError(ctx, tx, jobID, creative.GetJobToolName, getErr); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return getErr
	}
	processData := creativeProcessDataArg(result.ProcessData)

	switch result.Status {
	case "failed":
		if _, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET status = 'failed', external_status = 'failed', stage = $2, progress = $3,
    error_message = $4, last_poll_at = now(), next_poll_at = NULL,
    process_data = COALESCE($5::jsonb, process_data),
    poll_attempts = poll_attempts + 1, updated_at = now()
WHERE id = $1
`, jobID, firstNonEmpty(result.Stage, creative.GetJobToolName), clampCreativeProgress(result.Progress), result.ErrorMessage, processData); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
UPDATE creative_material_issue_candidate
SET status = 'selected', updated_at = now()
WHERE issue_id = $1 AND workspace_id = $2
  AND candidate_id IN (SELECT candidate_id FROM creative_edit_job_candidate WHERE job_id = $3)
`, issueID, workspaceID, jobID); err != nil {
			return err
		}
	case "completed":
		if err := h.insertCreativeEditResults(ctx, tx, localJobID, issueID, workspaceID, candidates, result.Variants); err != nil {
			return err
		}
		completionStatus := creativeResultCompletionStatus(rules, candidates, result.Variants)
		errorMessage := ""
		if completionStatus == "partial" {
			errorMessage = "部分变体或指定尺寸未完成，已保留可交付成图和全部 QC 过程记录。"
		}
		if _, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET status = $2, external_status = 'completed', stage = $3, progress = 100,
    error_message = $4, last_poll_at = now(), next_poll_at = NULL,
    process_data = COALESCE($5::jsonb, process_data),
    poll_attempts = poll_attempts + 1, completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE id = $1
`, jobID, completionStatus, firstNonEmpty(result.Stage, creative.GetJobToolName), errorMessage, processData); err != nil {
			return err
		}
	default:
		if _, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET status = 'running', external_status = $2, stage = $3, progress = $4,
    error_message = $5, last_poll_at = now(), next_poll_at = $6,
    process_data = COALESCE($7::jsonb, process_data),
    poll_attempts = poll_attempts + 1, updated_at = now()
WHERE id = $1
`, jobID, firstNonEmpty(result.Status, "running"), firstNonEmpty(result.Stage, creative.GetJobToolName), clampCreativeProgress(result.Progress), result.ErrorMessage, creativeNextPollAt(result.PollAfter), processData); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	h.publishCreativeMaterialsUpdated(workspaceID, issueID, "system", "")
	return nil
}

func creativeProcessDataArg(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return string(raw)
}

func creativeProcessDataSchemaVersion(raw string) int {
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return 0
	}
	switch value := payload["schema_version"].(type) {
	case string:
		version, _ := strconv.Atoi(strings.TrimSpace(value))
		return version
	case float64:
		return int(value)
	default:
		return 0
	}
}

func (h *Handler) ReconcileDueCreativeEditJobs(ctx context.Context, limit int) (int, error) {
	if h.DB == nil {
		return 0, errors.New("database executor not configured")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := h.DB.Query(ctx, `
WITH due AS (
  SELECT id
  FROM creative_edit_job
  WHERE status IN ('queued', 'running')
    AND (next_poll_at IS NULL OR next_poll_at <= now())
  ORDER BY next_poll_at NULLS FIRST, created_at
  FOR UPDATE SKIP LOCKED
  LIMIT $1
)
UPDATE creative_edit_job j
SET next_poll_at = now() + interval '30 seconds', updated_at = now()
FROM due
WHERE j.id = due.id
RETURNING j.id, j.issue_id, j.workspace_id
`, limit)
	if err != nil {
		return 0, err
	}
	type dueJob struct {
		id          pgtype.UUID
		issueID     pgtype.UUID
		workspaceID pgtype.UUID
	}
	jobs := []dueJob{}
	for rows.Next() {
		var job dueJob
		if err := rows.Scan(&job.id, &job.issueID, &job.workspaceID); err != nil {
			rows.Close()
			return len(jobs), err
		}
		jobs = append(jobs, job)
	}
	rows.Close()
	var reconcileErr error
	for _, job := range jobs {
		if err := h.reconcileCreativeEditJob(ctx, job.issueID, job.workspaceID, job.id); err != nil {
			slog.Warn("creative edit reconciler: job sync failed", "job_id", uuidToString(job.id), "error", err)
			reconcileErr = errors.Join(reconcileErr, err)
		}
	}
	return len(jobs), reconcileErr
}

func (h *Handler) listCreativeEditJobCandidates(ctx context.Context, tx pgx.Tx, jobID string, issueID, workspaceID pgtype.UUID) ([]creative.Candidate, error) {
	rows, err := tx.Query(ctx, `
SELECT c.id::text, c.competitor, c.title, c.asset_type, c.preview_url,
       c.resource_url, c.poster_url, c.archived_url
FROM creative_edit_job_candidate jc
JOIN creative_material_candidate c ON c.id = jc.candidate_id
JOIN creative_material_issue_candidate ic ON ic.issue_id = $2 AND ic.candidate_id = c.id
WHERE jc.job_id = $1::uuid AND ic.issue_id = $2 AND ic.workspace_id = $3
ORDER BY jc.candidate_id::text
`, jobID, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := []creative.Candidate{}
	for rows.Next() {
		var candidate creative.Candidate
		if err := rows.Scan(
			&candidate.ID, &candidate.Competitor, &candidate.Title, &candidate.AssetType,
			&candidate.PreviewURL, &candidate.ResourceURL, &candidate.PosterURL, &candidate.ArchivedURL,
		); err != nil {
			return nil, err
		}
		candidate.ArchivedURL = h.absoluteCreativeSourceURL(candidate.ArchivedURL)
		candidate.PreviewURL = h.absoluteCreativeSourceURL(candidate.PreviewURL)
		candidate.PosterURL = h.absoluteCreativeSourceURL(candidate.PosterURL)
		candidate.ResourceURL = h.absoluteCreativeSourceURL(candidate.ResourceURL)
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (h *Handler) absoluteCreativeSourceURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err == nil && parsed.Scheme != "" {
		return value
	}
	if strings.HasPrefix(value, "/uploads/") {
		publicURL := strings.TrimRight(strings.TrimSpace(h.cfg.PublicURL), "/")
		if publicURL != "" {
			return publicURL + value
		}
	}
	return value
}

func (h *Handler) insertCreativeEditResults(
	ctx context.Context,
	tx pgx.Tx,
	jobID string,
	issueID, workspaceID pgtype.UUID,
	candidates []creative.Candidate,
	variants []creative.Variant,
) error {
	allowedCandidates := make(map[string]struct{}, len(candidates))
	candidateIDs := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		allowedCandidates[candidate.ID] = struct{}{}
		candidateIDs = append(candidateIDs, candidate.ID)
	}
	for _, variant := range variants {
		if _, ok := allowedCandidates[variant.CandidateID]; !ok {
			return fmt.Errorf("creative provider returned unknown candidate %q", variant.CandidateID)
		}
		if variant.Index < 1 || variant.Index > 100 {
			return fmt.Errorf("creative provider returned invalid variant index %d", variant.Index)
		}
		qcStatus := normalizeCreativeQCStatus(variant.QCStatus)
		var variantID string
		if err := tx.QueryRow(ctx, `
INSERT INTO creative_edit_variant (
  job_id, candidate_id, variant_index, title, description, qc_status
) VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6)
ON CONFLICT (job_id, candidate_id, variant_index) DO UPDATE SET
  title = EXCLUDED.title,
  description = EXCLUDED.description,
  qc_status = EXCLUDED.qc_status
RETURNING id::text
`, jobID, variant.CandidateID, variant.Index, variant.Title, variant.Description, qcStatus).Scan(&variantID); err != nil {
			return err
		}
		for _, asset := range variant.Assets {
			if asset.Width <= 0 || asset.Height <= 0 {
				return fmt.Errorf("creative provider returned invalid asset size %dx%d", asset.Width, asset.Height)
			}
			contentType := firstNonEmpty(asset.ContentType, "image/png")
			publicAssetURL := h.publicCreativeAssetURL(asset.URL)
			if _, err := tx.Exec(ctx, `
INSERT INTO creative_edit_asset (
  variant_id, width, height, label, asset_url, source_asset_url, content_type, storage_key
) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (variant_id, width, height) DO UPDATE SET
  label = EXCLUDED.label,
  asset_url = EXCLUDED.asset_url,
  source_asset_url = EXCLUDED.source_asset_url,
  content_type = EXCLUDED.content_type,
  storage_key = EXCLUDED.storage_key
`, variantID, asset.Width, asset.Height, asset.Label, publicAssetURL, asset.URL, contentType, asset.StorageKey); err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(ctx, `
UPDATE creative_material_issue_candidate
SET status = 'edited', updated_at = now()
WHERE issue_id = $1 AND workspace_id = $2 AND candidate_id = ANY($3::uuid[])
`, issueID, workspaceID, candidateIDs)
	return err
}

func (h *Handler) publicCreativeAssetURL(rawURL string) string {
	value := strings.TrimSpace(rawURL)
	if value == "" {
		return ""
	}
	publicBase := strings.TrimRight(strings.TrimSpace(h.cfg.CreativeAssetPublicBaseURL), "/")
	if publicBase == "" {
		return value
	}
	if strings.HasPrefix(value, "/files/") {
		return publicBase + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed == nil || !parsed.IsAbs() {
		return value
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return value
	}
	if !strings.HasPrefix(parsed.EscapedPath(), "/files/") {
		return value
	}
	return publicBase + parsed.RequestURI()
}

// creativeResultCompletionStatus only marks a job complete when every selected
// reference has every requested variation and required output size. Providers
// may report their asynchronous job as finished even when individual variants
// failed QC or a requested size could not be produced.
func creativeResultCompletionStatus(rules map[string]any, candidates []creative.Candidate, variants []creative.Variant) string {
	expectedVariants := intFromAny(rules["variant_count"])
	if expectedVariants < 1 {
		expectedVariants = 3
	}
	expectedSizes := creativeExpectedOutputSizes(rules)
	completed := make(map[string]map[int]map[string]struct{}, len(candidates))
	for _, variant := range variants {
		if variant.Index < 1 || variant.Index > expectedVariants {
			continue
		}
		if completed[variant.CandidateID] == nil {
			completed[variant.CandidateID] = map[int]map[string]struct{}{}
		}
		if completed[variant.CandidateID][variant.Index] == nil {
			completed[variant.CandidateID][variant.Index] = map[string]struct{}{}
		}
		for _, asset := range variant.Assets {
			completed[variant.CandidateID][variant.Index][creativeOutputSizeKey(asset.Width, asset.Height)] = struct{}{}
		}
	}
	for _, candidate := range candidates {
		for index := 1; index <= expectedVariants; index++ {
			assets := completed[candidate.ID][index]
			if len(assets) == 0 {
				return "partial"
			}
			for _, size := range expectedSizes {
				if _, ok := assets[size]; !ok {
					return "partial"
				}
			}
		}
	}
	return "completed"
}

func creativeExpectedOutputSizes(rules map[string]any) []string {
	rawSizes, ok := rules["sizes"].([]any)
	if !ok {
		return nil
	}
	sizes := make([]string, 0, len(rawSizes))
	for _, raw := range rawSizes {
		size, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		width, height := intFromAny(size["width"]), intFromAny(size["height"])
		if width > 0 && height > 0 {
			sizes = append(sizes, creativeOutputSizeKey(width, height))
		}
	}
	return sizes
}

func creativeOutputSizeKey(width, height int) string {
	return fmt.Sprintf("%dx%d", width, height)
}

func recordCreativeProviderError(ctx context.Context, tx pgx.Tx, jobID pgtype.UUID, stage string, providerErr error) error {
	_, err := tx.Exec(ctx, `
UPDATE creative_edit_job
SET stage = $2, error_message = $3, last_poll_at = now(),
    next_poll_at = now() + interval '10 seconds', poll_attempts = poll_attempts + 1,
    updated_at = now()
WHERE id = $1
`, jobID, stage, providerErr.Error())
	return err
}

func creativeNextPollAt(after time.Duration) time.Time {
	if after <= 0 {
		after = 2 * time.Second
	}
	if after > time.Minute {
		after = time.Minute
	}
	return time.Now().UTC().Add(after)
}

func clampCreativeProgress(progress int) int {
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}
	return progress
}

func normalizeCreativeQCStatus(status string) string {
	switch status {
	case "mock", "pending", "passed", "warning", "failed":
		return status
	default:
		return "pending"
	}
}

func normalizeCreativeEditRules(input map[string]any) (map[string]any, error) {
	rules := map[string]any{}
	for key, value := range input {
		rules[key] = value
	}
	if strings.TrimSpace(stringFromAny(rules["market"])) == "" {
		rules["market"] = "idn-adakami"
	}
	if strings.TrimSpace(stringFromAny(rules["strategy"])) == "" {
		rules["strategy"] = "instruct"
	}
	variantCount := intFromAny(rules["variant_count"])
	if variantCount == 0 {
		variantCount = 3
	}
	if variantCount < 1 || variantCount > 6 {
		return nil, errors.New("variant_count must be between 1 and 6")
	}
	rules["variant_count"] = variantCount
	if _, ok := rules["sizes"]; !ok {
		rules["sizes"] = []any{
			map[string]any{"width": 1080, "height": 1080, "label": "1080x1080"},
			map[string]any{"width": 800, "height": 1000, "label": "800x1000"},
			map[string]any{"width": 1200, "height": 628, "label": "1200x628"},
		}
	}
	sizes, ok := rules["sizes"].([]any)
	if !ok || len(sizes) < 1 || len(sizes) > 6 {
		return nil, errors.New("sizes must contain between 1 and 6 entries")
	}
	for _, raw := range sizes {
		size, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("sizes entries must be objects")
		}
		width := intFromAny(size["width"])
		height := intFromAny(size["height"])
		if width < 100 || height < 100 || width > 4096 || height > 4096 {
			return nil, errors.New("size width and height must be between 100 and 4096")
		}
		if strings.TrimSpace(stringFromAny(size["label"])) == "" {
			size["label"] = fmt.Sprintf("%dx%d", width, height)
		}
	}
	return rules, nil
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := strconv.Atoi(typed.String())
		return parsed
	default:
		return 0
	}
}

func (h *Handler) listSelectedCreativeCandidateIDs(ctx context.Context, issueID, workspaceID pgtype.UUID, limit int) ([]string, error) {
	rows, err := h.DB.Query(ctx, `
SELECT candidate_id::text
FROM creative_material_issue_candidate
WHERE issue_id = $1 AND workspace_id = $2 AND status = 'selected'
ORDER BY selected_at NULLS LAST, created_at
LIMIT $3
`, issueID, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (h *Handler) publishCreativeMaterialsUpdated(workspaceID, issueID pgtype.UUID, actorType, actorID string) {
	if h.Bus == nil {
		return
	}
	h.publish(protocol.EventCreativeMaterialsUpdated, uuidToString(workspaceID), actorType, actorID, map[string]any{
		"issue_id": uuidToString(issueID),
	})
}

func validCreativeCandidateStatus(status string) bool {
	switch status {
	case "new", "selected", "rejected", "sent_to_edit", "edited", "approved", "archived":
		return true
	default:
		return false
	}
}

func normalizeCreativeMaterialInput(in creativeMaterialInput) creativeMaterialInput {
	in.ExternalID = strings.TrimSpace(firstNonEmpty(in.ExternalID, ""))
	in.DedupeKey = strings.TrimSpace(in.DedupeKey)
	in.Competitor = strings.TrimSpace(in.Competitor)
	in.Title = strings.TrimSpace(in.Title)
	in.AssetType = normalizeCreativeAssetType(in.AssetType)
	in.PreviewURL = strings.TrimSpace(in.PreviewURL)
	in.ResourceURL = strings.TrimSpace(in.ResourceURL)
	in.PosterURL = strings.TrimSpace(in.PosterURL)
	in.OriginalURL = strings.TrimSpace(in.OriginalURL)
	in.MediaNames = uniqueNonEmptyStrings(in.MediaNames)
	in.AreaNames = uniqueNonEmptyStrings(in.AreaNames)
	in.LanguageNames = uniqueNonEmptyStrings(in.LanguageNames)
	in.PlatformNames = uniqueNonEmptyStrings(in.PlatformNames)
	return in
}

func normalizeCreativeAssetType(assetType string) string {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "image", "video":
		return strings.ToLower(strings.TrimSpace(assetType))
	default:
		return "unknown"
	}
}

func creativeMaterialDedupeKey(in creativeMaterialInput) string {
	if strings.TrimSpace(in.DedupeKey) != "" {
		return strings.TrimSpace(in.DedupeKey)
	}
	base := firstNonEmpty(in.ExternalID, in.ResourceURL, in.PreviewURL, in.PosterURL, in.OriginalURL)
	if base == "" {
		base = strings.TrimSpace(in.Competitor + "|" + in.Title + "|" + in.AssetType)
	}
	if base == "" || base == "||" {
		return ""
	}
	sum := sha256.Sum256([]byte(base))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (h *Handler) resolveCreativeProvider(ctx context.Context, workspaceID, connectionID string, useDefault bool) (creative.ResolvedProvider, error) {
	if h.CreativeProviderResolver != nil {
		if useDefault {
			return h.CreativeProviderResolver.ResolveDefault(ctx, workspaceID)
		}
		return h.CreativeProviderResolver.Resolve(ctx, workspaceID, connectionID)
	}
	if strings.TrimSpace(connectionID) != "" {
		return creative.ResolvedProvider{}, errors.New("creative provider resolver is not configured")
	}
	if h.CreativeEditProvider == nil {
		return creative.ResolvedProvider{}, errors.New("creative edit provider is not configured")
	}
	return creative.ResolvedProvider{Provider: h.CreativeEditProvider}, nil
}

func nullableUUIDString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func float8Ptr(v pgtype.Float8) *float64 {
	if !v.Valid {
		return nil
	}
	out := v.Float64
	return &out
}

func int8Ptr(v pgtype.Int8) *int64 {
	if !v.Valid {
		return nil
	}
	out := v.Int64
	return &out
}

func creativeMaterialsFromCrawlRaw(raw json.RawMessage) []creativeMaterialInput {
	if strings.TrimSpace(string(raw)) == "" {
		return nil
	}
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	keys := []string{"selected_materials", "materials", "material_samples", "selected"}
	for _, key := range keys {
		items := creativeMaterialArrayAtKey(root, key)
		if len(items) > 0 {
			return items
		}
	}
	out := []creativeMaterialInput{}
	creativeCollectMaterialInputs(root, &out, map[string]struct{}{})
	if len(out) > 500 {
		return out[:500]
	}
	return out
}

func creativeMaterialArrayAtKey(value any, key string) []creativeMaterialInput {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	rawItems, ok := obj[key].([]any)
	if !ok {
		return nil
	}
	out := []creativeMaterialInput{}
	for _, rawItem := range rawItems {
		if item, ok := creativeMaterialInputFromAny(rawItem); ok {
			out = append(out, item)
		}
	}
	return out
}

func creativeCollectMaterialInputs(value any, out *[]creativeMaterialInput, seen map[string]struct{}) {
	if len(*out) >= 500 {
		return
	}
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			creativeCollectMaterialInputs(child, out, seen)
		}
	case map[string]any:
		if item, ok := creativeMaterialInputFromMap(v); ok {
			key := creativeMaterialDedupeKey(item)
			if key != "" {
				if _, exists := seen[key]; !exists {
					seen[key] = struct{}{}
					*out = append(*out, item)
				}
			}
		}
		for _, child := range v {
			creativeCollectMaterialInputs(child, out, seen)
		}
	}
}

func creativeMaterialInputFromAny(value any) (creativeMaterialInput, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return creativeMaterialInput{}, false
	}
	return creativeMaterialInputFromMap(obj)
}

func creativeMaterialInputFromMap(obj map[string]any) (creativeMaterialInput, bool) {
	duration := optionalFloatFromAny(firstAnyAtKeys(obj, "duration_days", "duration"))
	impression := optionalInt64FromAny(firstAnyAtKeys(obj, "impression_estimate", "impression", "impressions"))
	raw, _ := json.Marshal(obj)
	input := creativeMaterialInput{
		ExternalID:         stringFromAny(firstAnyAtKeys(obj, "material_id", "external_id", "id", "creative_id")),
		Competitor:         stringFromAny(firstAnyAtKeys(obj, "competitor", "brand", "app_name")),
		Title:              stringFromAny(firstAnyAtKeys(obj, "title", "name", "description")),
		AssetType:          normalizeCreativeAssetType(stringFromAny(firstAnyAtKeys(obj, "asset_type", "type"))),
		PreviewURL:         stringFromAny(firstAnyAtKeys(obj, "preview_url", "preview", "image_url", "cover_url")),
		ResourceURL:        stringFromAny(firstAnyAtKeys(obj, "resource_url", "url", "video_url", "image_url")),
		PosterURL:          stringFromAny(firstAnyAtKeys(obj, "poster_url", "poster", "cover_url")),
		OriginalURL:        stringFromAny(firstAnyAtKeys(obj, "original_url", "landing_url")),
		DurationDays:       duration,
		ImpressionEstimate: impression,
		MediaNames:         stringSliceFromAny(firstAnyAtKeys(obj, "media_names", "media")),
		AreaNames:          stringSliceFromAny(firstAnyAtKeys(obj, "area_names", "areas")),
		LanguageNames:      stringSliceFromAny(firstAnyAtKeys(obj, "language_names", "languages")),
		PlatformNames:      stringSliceFromAny(firstAnyAtKeys(obj, "platform_names", "platforms")),
		Raw:                json.RawMessage(raw),
	}
	input = normalizeCreativeMaterialInput(input)
	if input.ExternalID == "" && input.PreviewURL == "" && input.ResourceURL == "" && input.PosterURL == "" && input.Title == "" {
		return creativeMaterialInput{}, false
	}
	return input, true
}

func firstAnyAtKeys(obj map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			return value
		}
	}
	return nil
}

func stringFromAny(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return ""
	}
}

func stringSliceFromAny(value any) []string {
	switch v := value.(type) {
	case []string:
		return uniqueNonEmptyStrings(v)
	case []any:
		out := []string{}
		for _, item := range v {
			if text := stringFromAny(item); text != "" {
				out = append(out, text)
			} else if obj, ok := item.(map[string]any); ok {
				if name := stringFromAny(firstAnyAtKeys(obj, "name", "label", "title")); name != "" {
					out = append(out, name)
				}
			}
		}
		return uniqueNonEmptyStrings(out)
	case string:
		return uniqueNonEmptyStrings(strings.Split(v, ","))
	default:
		return nil
	}
}

func optionalFloatFromAny(value any) *float64 {
	switch v := value.(type) {
	case float64:
		if v == v {
			return &v
		}
	case int:
		out := float64(v)
		return &out
	case int64:
		out := float64(v)
		return &out
	case string:
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", ""), 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func optionalInt64FromAny(value any) *int64 {
	switch v := value.(type) {
	case float64:
		out := int64(v)
		return &out
	case int:
		out := int64(v)
		return &out
	case int64:
		return &v
	case string:
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(v), ",", ""), 64)
		if err == nil {
			out := int64(parsed)
			return &out
		}
	}
	return nil
}
