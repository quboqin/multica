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

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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
	SourceAttachmentID string           `json:"source_attachment_id"`
	SourceIssueID      string           `json:"source_issue_id"`
	SourceRunID        string           `json:"source_run_id"`
	IsNewInRun         bool             `json:"is_new_in_run"`
	AnalysisStatus     string           `json:"analysis_status"`
	AnalysisError      string           `json:"analysis_error"`
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
	ID               string                             `json:"id"`
	WorkspaceID      string                             `json:"workspace_id"`
	IssueID          string                             `json:"issue_id"`
	AutopilotRunID   string                             `json:"autopilot_run_id"`
	RerunOfID        string                             `json:"rerun_of_id"`
	ConnectorID      string                             `json:"connector_id"`
	QuerySummary     string                             `json:"query_summary"`
	AnalysisAgentID  string                             `json:"analysis_agent_id"`
	Status           string                             `json:"status"`
	ErrorCode        string                             `json:"error_code"`
	ErrorMessage     string                             `json:"error_message"`
	ImportedCount    int                                `json:"imported_count"`
	ExistingCount    int                                `json:"existing_count"`
	TotalCount       int                                `json:"total_count"`
	CandidateMetrics creativeMaterialRunMetricsResponse `json:"candidate_metrics"`
	StartedAt        string                             `json:"started_at"`
	FinishedAt       string                             `json:"finished_at"`
	CreatedAt        string                             `json:"created_at"`
}

type creativeMaterialRunMetricsResponse struct {
	Total          int `json:"total"`
	Analyzed       int `json:"analyzed"`
	AnalysisFailed int `json:"analysis_failed"`
	Selected       int `json:"selected"`
	Rejected       int `json:"rejected"`
}

type creativeMaterialsResponse struct {
	Enabled     bool                                `json:"enabled"`
	Summary     creativeMaterialSummaryResponse     `json:"summary"`
	Candidates  []creativeMaterialCandidateResponse `json:"candidates"`
	CrawlRuns   []creativeMaterialCrawlRunResponse  `json:"crawl_runs"`
	Context     *creativeIssueContextResponse       `json:"context,omitempty"`
	Items       []creativeIssueItemResponse         `json:"items"`
	Deliveries  []creativeDeliveryResponse          `json:"deliveries"`
	Adjustments []creativeAdjustmentRequestResponse `json:"adjustments"`
}

type creativeImportSummary struct {
	RunID         string `json:"run_id"`
	ImportedCount int    `json:"imported_count"`
	ExistingCount int    `json:"existing_count"`
	TotalCount    int    `json:"total_count"`
	SkippedCount  int    `json:"skipped_count"`
}

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
	Tags               []string        `json:"tags"`
	Note               string          `json:"note"`
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
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "candidateId"), "candidate_id")
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
	var candidateExists bool
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2
)
`, candidateID, issue.WorkspaceID).Scan(&candidateExists); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative material candidate")
		return
	}
	if !candidateExists {
		writeError(w, http.StatusNotFound, "creative material candidate not found")
		return
	}
	_, err := h.DB.Exec(r.Context(), `
INSERT INTO creative_material_issue_candidate (
  issue_id, candidate_id, workspace_id, status, note, selected_by, selected_at
) VALUES (
  $1, $2, $3, $4, $5,
  CASE WHEN $4 = 'selected' THEN $6::uuid ELSE NULL END,
  CASE WHEN $4 = 'selected' THEN now() ELSE NULL END
)
ON CONFLICT (issue_id, candidate_id) DO UPDATE SET
  status = EXCLUDED.status,
  note = EXCLUDED.note,
  selected_by = EXCLUDED.selected_by,
  selected_at = EXCLUDED.selected_at,
  updated_at = now()
`, issue.ID, candidateID, issue.WorkspaceID, status, strings.TrimSpace(req.Note), selectedBy)
	if err != nil {
		slog.Warn("update creative material candidate failed", append(logger.RequestAttrs(r), "error", err, "candidate_id", uuidToString(candidateID))...)
		writeError(w, http.StatusInternalServerError, "failed to update creative material candidate")
		return
	}
	if status == "selected" {
		if _, err := h.DB.Exec(r.Context(), `
UPDATE creative_material_candidate
SET archive_status = 'pending', archive_attempts = 0, archive_error = '',
    next_archive_at = now(), updated_at = now()
WHERE id = $1::uuid AND workspace_id = $2 AND archived_url = ''
  AND archive_status <> 'running'
`, candidateID, issue.WorkspaceID); err != nil {
			slog.Warn("reset selected creative material archive failed", append(logger.RequestAttrs(r), "error", err, "candidate_id", uuidToString(candidateID))...)
			writeError(w, http.StatusInternalServerError, "failed to schedule selected creative material archive")
			return
		}
	}
	resp, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, resp)
}

type creativeMaterialImportInput struct {
	// RunID is set when the caller has already persisted the lifecycle record
	// before invoking an external crawler. Import then completes that same run.
	RunID          string
	IssueID        pgtype.UUID
	WorkspaceID    pgtype.UUID
	AutopilotRunID pgtype.UUID
	ConnectorID    string
	QuerySummary   string
	Params         json.RawMessage
	Materials      []creativeMaterialInput
	ActorType      string
	ActorID        string
	UserID         pgtype.UUID
}

func (h *Handler) startCreativeMaterialCrawlRun(ctx context.Context, in creativeMaterialImportInput) (string, error) {
	connectorID := strings.TrimSpace(in.ConnectorID)
	if connectorID == "" {
		connectorID = "appgrowing"
	}
	params := strings.TrimSpace(string(in.Params))
	if params == "" {
		params = "{}"
	}
	var runID string
	err := h.DB.QueryRow(ctx, `
INSERT INTO creative_material_crawl_run (
  workspace_id, issue_id, connector_id, query_summary, params, status,
  autopilot_run_id, started_at, created_by_type, created_by_id
) VALUES ($1, $2, $3, $4, $5::jsonb, 'running', $6, now(), $7, $8::uuid)
RETURNING id::text
`, in.WorkspaceID, nullableUUID(in.IssueID, in.IssueID.Valid), connectorID, strings.TrimSpace(in.QuerySummary), params,
		nullableUUID(in.AutopilotRunID, in.AutopilotRunID.Valid), in.ActorType, in.ActorID).Scan(&runID)
	return runID, err
}

func (h *Handler) failCreativeMaterialCrawlRun(ctx context.Context, workspaceID pgtype.UUID, runID, status, errorCode, errorMessage string) {
	if strings.TrimSpace(runID) == "" {
		return
	}
	if status != "action_required" {
		status = "failed"
	}
	if _, err := h.DB.Exec(ctx, `
UPDATE creative_material_crawl_run
SET status = $3, error_code = $4, error_message = $5, finished_at = now()
WHERE id = $1::uuid AND workspace_id = $2
`, runID, workspaceID, status, strings.TrimSpace(errorCode), strings.TrimSpace(errorMessage)); err != nil {
		slog.Warn("mark creative crawl run failed", "crawl_run_id", runID, "error", err)
	}
}

func (h *Handler) importCreativeMaterialsForIssue(ctx context.Context, in creativeMaterialImportInput) (creativeImportSummary, error) {
	if !in.IssueID.Valid {
		return creativeImportSummary{}, errors.New("issue_id is required for issue import")
	}
	return h.importCreativeMaterials(ctx, in)
}

func (h *Handler) importCreativeMaterials(ctx context.Context, in creativeMaterialImportInput) (creativeImportSummary, error) {
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

	runID := strings.TrimSpace(in.RunID)
	if runID == "" {
		err = tx.QueryRow(ctx, `
INSERT INTO creative_material_crawl_run (
  workspace_id, issue_id, connector_id, query_summary, params, status,
  autopilot_run_id, started_at, created_by_type, created_by_id
) VALUES ($1, $2, $3, $4, $5::jsonb, 'running', $6, now(), $7, $8::uuid)
RETURNING id::text
`, in.WorkspaceID, nullableUUID(in.IssueID, in.IssueID.Valid), connectorID, strings.TrimSpace(in.QuerySummary), params,
			nullableUUID(in.AutopilotRunID, in.AutopilotRunID.Valid), in.ActorType, in.ActorID).Scan(&runID)
		if err != nil {
			return creativeImportSummary{}, err
		}
	} else {
		commandTag, updateErr := tx.Exec(ctx, `
UPDATE creative_material_crawl_run
SET status = 'running', started_at = COALESCE(started_at, now()),
    error_code = '', error_message = ''
WHERE id = $1::uuid AND workspace_id = $2
`, runID, in.WorkspaceID)
		if updateErr != nil {
			return creativeImportSummary{}, updateErr
		}
		if commandTag.RowsAffected() != 1 {
			return creativeImportSummary{}, errors.New("crawl run does not belong to workspace")
		}
	}

	materials, skippedCount := uniqueCreativeMaterialInputs(in.Materials)
	summary := creativeImportSummary{RunID: runID, TotalCount: len(materials), SkippedCount: skippedCount}
	for _, material := range materials {
		dedupeKey := creativeMaterialDedupeKey(material)
		raw := strings.TrimSpace(string(material.Raw))
		if raw == "" {
			raw = "{}"
		}
		var candidateID string
		var inserted bool
		if connectorID == "workspace-library" {
			sourceCandidateID := workspaceLibrarySourceCandidateID(material.Raw)
			if sourceCandidateID != "" {
				err = tx.QueryRow(ctx, `
UPDATE creative_material_candidate
SET last_seen_at = now(), updated_at = now()
WHERE id = $1::uuid AND workspace_id = $2
RETURNING id::text
`, sourceCandidateID, in.WorkspaceID).Scan(&candidateID)
				if errors.Is(err, pgx.ErrNoRows) {
					candidateID = ""
				} else if err != nil {
					return creativeImportSummary{}, err
				}
			}
		}
		if candidateID == "" {
			err = tx.QueryRow(ctx, `
WITH upsert AS (
  INSERT INTO creative_material_candidate (
    workspace_id, connector_id, external_id, dedupe_key, competitor, title,
    asset_type, preview_url, resource_url, poster_url, original_url,
    duration_days, impression_estimate, media_names, area_names,
    language_names, platform_names, tags, note, raw
  ) VALUES (
    $1, $2, NULLIF($3, ''), $4, $5, $6,
    $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16, $17, $18, $19, $20::jsonb
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
    tags = CASE WHEN cardinality(EXCLUDED.tags) > 0 THEN EXCLUDED.tags ELSE creative_material_candidate.tags END,
    note = COALESCE(NULLIF(EXCLUDED.note, ''), creative_material_candidate.note),
    raw = CASE WHEN EXCLUDED.raw <> '{}'::jsonb THEN EXCLUDED.raw ELSE creative_material_candidate.raw END,
    archive_status = CASE
      WHEN creative_material_candidate.archived_url = '' THEN 'pending'
      ELSE creative_material_candidate.archive_status
    END,
    archive_attempts = CASE
      WHEN creative_material_candidate.archived_url = '' THEN 0
      ELSE creative_material_candidate.archive_attempts
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
				material.LanguageNames, material.PlatformNames, material.Tags, material.Note, raw).Scan(&candidateID, &inserted)
			if err != nil {
				return creativeImportSummary{}, err
			}
		}
		if inserted {
			summary.ImportedCount++
		} else {
			summary.ExistingCount++
		}
		_, err = tx.Exec(ctx, `
INSERT INTO creative_material_crawl_run_candidate (
  run_id, candidate_id, workspace_id, is_new_in_run
) VALUES ($1::uuid, $2::uuid, $3, $4)
ON CONFLICT (run_id, candidate_id) DO UPDATE SET
  is_new_in_run = EXCLUDED.is_new_in_run,
  updated_at = now()
`, runID, candidateID, in.WorkspaceID, inserted)
		if err != nil {
			return creativeImportSummary{}, err
		}
		if !in.IssueID.Valid {
			continue
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
SET imported_count = $3, existing_count = $4, total_count = $5,
    status = 'completed', finished_at = now()
WHERE id = $1::uuid AND workspace_id = $2
`, runID, in.WorkspaceID, summary.ImportedCount, summary.ExistingCount, summary.TotalCount)
	if err != nil {
		return creativeImportSummary{}, err
	}
	if in.IssueID.Valid {
		_, err = tx.Exec(ctx, `
UPDATE issue
SET metadata = jsonb_set(metadata, '{workflow}', '"creative_material"'::jsonb),
    updated_at = now()
WHERE id = $1 AND workspace_id = $2
`, in.IssueID, in.WorkspaceID)
		if err != nil {
			return creativeImportSummary{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return creativeImportSummary{}, err
	}
	h.publishCreativeMaterialsUpdated(in.WorkspaceID, in.IssueID, in.ActorType, in.ActorID)
	return summary, nil
}

func workspaceLibrarySourceCandidateID(raw json.RawMessage) string {
	var metadata struct {
		SourceCandidateID string `json:"source_candidate_id"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &metadata) != nil {
		return ""
	}
	candidateID := strings.TrimSpace(metadata.SourceCandidateID)
	if _, err := uuid.Parse(candidateID); err != nil {
		return ""
	}
	return candidateID
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
	creativeContext, items, err := h.loadCreativeIssueState(ctx, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.Context = creativeContext
	resp.Items = items
	deliveries, err := listCreativeDeliveries(ctx, h.DB, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.Deliveries = deliveries
	adjustments, err := listCreativeAdjustments(ctx, h.DB, issueID, workspaceID)
	if err != nil {
		return resp, err
	}
	resp.Adjustments = adjustments
	if len(resp.Candidates) > 0 || len(resp.CrawlRuns) > 0 {
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
  c.language_names, c.platform_names, ic.status, c.tags || ic.tags,
  COALESCE(NULLIF(ic.note, ''), c.note),
  COALESCE(ic.selected_at::text, ''), c.first_seen_at::text, c.last_seen_at::text,
  c.created_at::text, c.updated_at::text
  , COALESCE(ic.issue_id::text, ''), COALESCE(ic.source_run_id::text, ''),
  COALESCE(c.created_at >= cr.created_at, false)
FROM creative_material_issue_candidate ic
JOIN creative_material_candidate c ON c.id = ic.candidate_id
LEFT JOIN creative_material_crawl_run cr ON cr.id = ic.source_run_id
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
			&item.SourceIssueID, &item.SourceRunID, &item.IsNewInRun,
		); err != nil {
			return nil, err
		}
		item.DurationDays = float8Ptr(duration)
		item.ImpressionEstimate = int8Ptr(impression)
		if selectedAt != "" {
			item.SelectedAt = &selectedAt
		}
		item.ArchivedURL = h.creativeMaterialArchiveResponseURL(item.ID, item.ArchivedURL)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (h *Handler) listCreativeCrawlRuns(ctx context.Context, issueID, workspaceID pgtype.UUID) ([]creativeMaterialCrawlRunResponse, error) {
	return h.listCreativeCrawlRunsForWorkspace(ctx, workspaceID, issueID, true)
}

func (h *Handler) ListCreativeCrawlRuns(w http.ResponseWriter, r *http.Request) {
	workspaceIDRaw := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceIDRaw); !ok {
		return
	}
	workspaceID, ok := parseUUIDOrBadRequest(w, workspaceIDRaw, "workspace_id")
	if !ok {
		return
	}
	runs, err := h.listCreativeCrawlRunsForWorkspace(r.Context(), workspaceID, pgtype.UUID{}, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list creative crawl runs")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"crawl_runs": runs})
}

func (h *Handler) listCreativeCrawlRunsForWorkspace(ctx context.Context, workspaceID, issueID pgtype.UUID, filterIssue bool) ([]creativeMaterialCrawlRunResponse, error) {
	rows, err := h.DB.Query(ctx, `
WITH crawl_candidate_analysis AS (
  SELECT rc.run_id, rc.candidate_id, rc.workspace_id,
         CASE
           WHEN rc.analysis_status = 'completed' THEN rc.analysis_status
           WHEN EXISTS(
             SELECT 1
             FROM agent_task_queue task
             WHERE task.trigger_evidence_kind = 'creative_crawl_run_analysis'
               AND task.trigger_evidence_ref_id = rc.run_id
               AND task.status = 'running'
               AND task.context->>'candidate_id' = rc.candidate_id::text
           ) THEN 'running'
           WHEN EXISTS(
             SELECT 1
             FROM agent_task_queue task
             WHERE task.trigger_evidence_kind = 'creative_crawl_run_analysis'
               AND task.trigger_evidence_ref_id = rc.run_id
               AND task.status = 'failed'
               AND task.context->>'candidate_id' = rc.candidate_id::text
           ) AND NOT EXISTS(
             SELECT 1
             FROM creative_source_analysis analysis
             WHERE analysis.candidate_id = rc.candidate_id
               AND analysis.trigger_evidence_kind = 'crawl_run'
               AND analysis.trigger_evidence_ref_id = rc.run_id
               AND analysis.status = 'completed'
           ) THEN 'failed'
           ELSE rc.analysis_status
         END AS analysis_status
  FROM creative_material_crawl_run_candidate rc
)
SELECT cr.id::text, cr.workspace_id::text, COALESCE(cr.issue_id::text, ''),
       COALESCE(cr.autopilot_run_id::text, ''), COALESCE(cr.rerun_of_id::text, ''),
       cr.connector_id, cr.query_summary, COALESCE(cr.params->>'analysis_agent_id', ''),
       cr.status, cr.error_code, cr.error_message,
       cr.imported_count, cr.existing_count, cr.total_count,
       count(rc.candidate_id)::int,
       count(*) FILTER (WHERE rc.analysis_status = 'completed')::int,
       count(*) FILTER (WHERE rc.analysis_status = 'failed')::int,
       count(*) FILTER (WHERE EXISTS(SELECT 1 FROM creative_material_issue_candidate ic WHERE ic.candidate_id = rc.candidate_id AND ic.workspace_id = cr.workspace_id AND ic.status = 'selected'))::int,
       count(*) FILTER (WHERE EXISTS(SELECT 1 FROM creative_material_issue_candidate ic WHERE ic.candidate_id = rc.candidate_id AND ic.workspace_id = cr.workspace_id AND ic.status = 'rejected'))::int,
       COALESCE(cr.started_at::text, ''), COALESCE(cr.finished_at::text, ''), cr.created_at::text
FROM creative_material_crawl_run cr
LEFT JOIN crawl_candidate_analysis rc
  ON rc.run_id = cr.id AND rc.workspace_id = cr.workspace_id
WHERE cr.workspace_id = $1 AND (NOT $2::boolean OR cr.issue_id = $3)
GROUP BY cr.id
ORDER BY cr.created_at DESC
LIMIT 20
`, workspaceID, filterIssue, nullableUUID(issueID, filterIssue))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []creativeMaterialCrawlRunResponse{}
	for rows.Next() {
		var item creativeMaterialCrawlRunResponse
		if err := rows.Scan(
			&item.ID, &item.WorkspaceID, &item.IssueID, &item.AutopilotRunID, &item.RerunOfID,
			&item.ConnectorID, &item.QuerySummary, &item.AnalysisAgentID,
			&item.Status, &item.ErrorCode, &item.ErrorMessage,
			&item.ImportedCount, &item.ExistingCount, &item.TotalCount,
			&item.CandidateMetrics.Total, &item.CandidateMetrics.Analyzed, &item.CandidateMetrics.AnalysisFailed,
			&item.CandidateMetrics.Selected, &item.CandidateMetrics.Rejected,
			&item.StartedAt, &item.FinishedAt, &item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
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
	in.Tags = uniqueNonEmptyStrings(in.Tags)
	in.Note = strings.TrimSpace(in.Note)
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

func creativeMaterialHasUsableAsset(in creativeMaterialInput) bool {
	return firstNonEmpty(in.PreviewURL, in.ResourceURL, in.PosterURL) != ""
}

func creativeMaterialDedupeKey(in creativeMaterialInput) string {
	if strings.TrimSpace(in.DedupeKey) != "" {
		return strings.TrimSpace(in.DedupeKey)
	}
	base := creativeMaterialAssetIdentity(in)
	if base == "" {
		base = strings.TrimSpace(in.ExternalID)
	}
	if base == "" {
		base = strings.TrimSpace(in.Competitor + "|" + in.Title + "|" + in.AssetType)
	}
	if base == "" || base == "||" {
		return ""
	}
	sum := sha256.Sum256([]byte(base))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func creativeMaterialAssetIdentity(in creativeMaterialInput) string {
	raw := firstNonEmpty(in.ResourceURL, in.PreviewURL, in.PosterURL, in.OriginalURL)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return raw
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

func uniqueCreativeMaterialInputs(inputs []creativeMaterialInput) ([]creativeMaterialInput, int) {
	seen := make(map[string]struct{}, len(inputs))
	out := make([]creativeMaterialInput, 0, len(inputs))
	skipped := 0
	for _, material := range inputs {
		material = normalizeCreativeMaterialInput(material)
		if !creativeMaterialHasUsableAsset(material) {
			skipped++
			continue
		}
		key := creativeMaterialDedupeKey(material)
		if key == "" {
			skipped++
			continue
		}
		if _, exists := seen[key]; exists {
			skipped++
			continue
		}
		seen[key] = struct{}{}
		material.DedupeKey = key
		out = append(out, material)
	}
	return out, skipped
}

func (h *Handler) materialSearchParamsWithNovelty(ctx context.Context, workspaceID pgtype.UUID, connectorID, capability string, raw json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(connectorID) != "appgrowing" || strings.TrimSpace(capability) != "material_search" {
		return raw, nil
	}
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, err
		}
	}
	if novelOnly, exists := params["novel_only"].(bool); exists && !novelOnly {
		return raw, nil
	}
	params["novel_only"] = true
	keys := map[string]struct{}{}
	if values, ok := params["exclude_dedupe_keys"].([]any); ok {
		for _, value := range values {
			key := strings.TrimSpace(fmt.Sprint(value))
			if key != "" {
				keys[key] = struct{}{}
			}
		}
	}
	rows, err := h.DB.Query(ctx, `
SELECT resource_url, preview_url, poster_url, original_url
FROM creative_material_candidate
WHERE workspace_id = $1 AND connector_id = 'appgrowing'
`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var material creativeMaterialInput
		if err := rows.Scan(&material.ResourceURL, &material.PreviewURL, &material.PosterURL, &material.OriginalURL); err != nil {
			return nil, err
		}
		if key := creativeMaterialDedupeKey(material); key != "" {
			keys[key] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	exclusions := make([]string, 0, len(keys))
	for key := range keys {
		exclusions = append(exclusions, key)
	}
	sort.Strings(exclusions)
	params["exclude_dedupe_keys"] = exclusions
	return json.Marshal(params)
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
	hasExplicitMaterialArrays := creativeMaterialRootHasAnyKey(root, keys)
	for _, key := range keys {
		items := creativeMaterialArrayAtKey(root, key)
		if len(items) > 0 {
			return items
		}
	}
	if hasExplicitMaterialArrays {
		return nil
	}
	out := []creativeMaterialInput{}
	creativeCollectMaterialInputs(root, &out, map[string]struct{}{})
	if len(out) > 500 {
		return out[:500]
	}
	return out
}

func creativeMaterialRootHasAnyKey(value any, keys []string) bool {
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for _, key := range keys {
		if _, ok := obj[key]; ok {
			return true
		}
	}
	return false
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
	duration := firstOptionalFloatAtKeys(obj,
		"duration_days", "durationDays", "duration", "delivery_days", "deliveryDays",
		"put_days", "putDays", "running_days", "runningDays", "online_days", "onlineDays",
		"days_count", "daysCount",
	)
	impression := firstOptionalInt64AtKeys(obj,
		"impression_estimate", "impressionEstimate", "impression_inc_2y", "impressionInc2y",
		"impression", "impressions", "impression_count", "impressionCount", "impression_num",
		"impressionNum", "show_count", "showCount", "exposure", "exposure_count", "exposureCount",
		"estimated_impression", "estimatedImpression", "estimated_exposure", "estimatedExposure",
	)
	raw, _ := json.Marshal(obj)
	input := creativeMaterialInput{
		ExternalID:         firstStringAtKeys(obj, "material_id", "materialId", "external_id", "externalId", "id", "creative_id", "creativeId", "ad_id", "adId"),
		DedupeKey:          firstStringAtKeys(obj, "dedupe_key", "dedupeKey"),
		Competitor:         firstStringAtKeys(obj, "competitor", "brand", "app_name", "appName", "brand_name", "brandName"),
		Title:              firstStringAtKeys(obj, "title", "name", "description", "slogan"),
		AssetType:          normalizeCreativeAssetType(firstStringAtKeys(obj, "asset_type", "assetType", "type")),
		PreviewURL:         firstStringAtKeys(obj, "preview_url", "previewUrl", "preview", "image_url", "imageUrl", "cover_url", "coverUrl"),
		ResourceURL:        firstStringAtKeys(obj, "resource_url", "resourceUrl", "url", "video_url", "videoUrl", "image_url", "imageUrl"),
		PosterURL:          firstStringAtKeys(obj, "poster_url", "posterUrl", "poster", "cover_url", "coverUrl"),
		OriginalURL:        firstStringAtKeys(obj, "original_url", "originalUrl", "landing_url", "landingUrl", "redirect_url", "redirectUrl", "target_url", "targetUrl"),
		DurationDays:       duration,
		ImpressionEstimate: impression,
		MediaNames:         firstStringSliceAtKeys(obj, "media_names", "mediaNames", "media", "media_ids", "mediaIds"),
		AreaNames:          firstStringSliceAtKeys(obj, "area_names", "areaNames", "areas", "area", "area_codes", "areaCodes"),
		LanguageNames:      firstStringSliceAtKeys(obj, "language_names", "languageNames", "languages", "language", "language_codes", "languageCodes"),
		PlatformNames:      firstStringSliceAtKeys(obj, "platform_names", "platformNames", "platforms", "platform", "platform_ids", "platformIds"),
		Tags:               firstStringSliceAtKeys(obj, "tags", "tag_names", "tagNames", "labels"),
		Note:               firstStringAtKeys(obj, "note", "notes", "remark", "remarks"),
		Raw:                json.RawMessage(raw),
	}
	input = normalizeCreativeMaterialInput(input)
	if input.ExternalID == "" && input.PreviewURL == "" && input.ResourceURL == "" && input.PosterURL == "" && input.Title == "" {
		return creativeMaterialInput{}, false
	}
	if !creativeMaterialHasUsableAsset(input) {
		return creativeMaterialInput{}, false
	}
	return input, true
}

func firstStringAtKeys(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if text := stringFromAny(value); text != "" {
				return text
			}
		}
	}
	return ""
}

func firstStringSliceAtKeys(obj map[string]any, keys ...string) []string {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if values := stringSliceFromAny(value); len(values) > 0 {
				return values
			}
		}
	}
	return nil
}

func firstOptionalFloatAtKeys(obj map[string]any, keys ...string) *float64 {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if parsed := optionalFloatFromAny(value); parsed != nil {
				return parsed
			}
		}
	}
	return nil
}

func firstOptionalInt64AtKeys(obj map[string]any, keys ...string) *int64 {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			if parsed := optionalInt64FromAny(value); parsed != nil {
				return parsed
			}
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
				if name := firstStringAtKeys(obj, "name", "label", "title", "code", "id"); name != "" {
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
