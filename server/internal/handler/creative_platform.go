package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
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
	PublishedConfig  json.RawMessage `json:"published_config"`
	CreatedBy        string          `json:"created_by"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
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
	CreativeBrief json.RawMessage `json:"creative_brief"`
	WorkIssueID   string          `json:"work_issue_id"`
	Revision      int             `json:"revision"`
	Status        string          `json:"status"`
	UpdatedAt     string          `json:"updated_at"`
}

type creativeBriefInput struct {
	Theme                    string                             `json:"theme"`
	ThemeElements            []string                           `json:"theme_elements"`
	PrimaryBenefit           string                             `json:"primary_benefit"`
	SecondaryBenefits        []string                           `json:"secondary_benefits"`
	BenefitValue             string                             `json:"benefit_value"`
	SourceSemantics          string                             `json:"source_semantics"`
	InformationMechanism     string                             `json:"information_mechanism"`
	VisualAnchors            []string                           `json:"visual_anchors"`
	PaletteAnchors           []string                           `json:"palette_anchors"`
	MustPreserve             []string                           `json:"must_preserve"`
	AllowedVariations        []string                           `json:"allowed_variations"`
	Evidence                 []string                           `json:"evidence"`
	DetectedText             []string                           `json:"detected_text"`
	VisualType               string                             `json:"visual_type"`
	AnalysisSummary          string                             `json:"analysis_summary"`
	UserDirection            string                             `json:"user_direction"`
	AppUIReplacementRequired bool                               `json:"app_ui_replacement_required"`
	SelectedAppUIReferences  []creativeBriefAppUIReferenceInput `json:"selected_app_ui_references"`
	Status                   string                             `json:"status"`
	Source                   string                             `json:"source"`
	Confidence               *float64                           `json:"confidence"`
	AnalysisIssueID          string                             `json:"analysis_issue_id"`
}

type creativeBriefAppUIReferenceInput struct {
	ResourceFileID string `json:"resource_file_id"`
	AttachmentID   string `json:"attachment_id"`
	Reason         string `json:"reason"`
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
       COALESCE(published_version, 0), config::text,
       COALESCE((SELECT revision.config::text FROM creative_resource_revision revision WHERE revision.resource_id = creative_resource.id AND revision.version = creative_resource.published_version), '{}'),
       created_by::text,
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
		resource, scanErr := scanCreativeResourceWithPublishedConfig(rows)
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
	runID := pgtype.UUID{}
	filterRun := false
	if rawRunID := strings.TrimSpace(r.URL.Query().Get("run_id")); rawRunID != "" {
		var parsed bool
		runID, parsed = parseUUIDOrBadRequest(w, rawRunID, "run_id")
		if !parsed {
			return
		}
		filterRun = true
	}
	limit := 500
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(parsed, 100)
	}
	offset := 0
	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil || parsed < 0 {
			writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsed
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	competitor := strings.TrimSpace(r.URL.Query().Get("competitor"))
	area := strings.TrimSpace(r.URL.Query().Get("area"))
	language := strings.TrimSpace(r.URL.Query().Get("language"))
	media := strings.TrimSpace(r.URL.Query().Get("media"))
	assetType := strings.TrimSpace(r.URL.Query().Get("asset_type"))
	if assetType != "" && assetType != "image" && assetType != "video" && assetType != "unknown" {
		writeError(w, http.StatusBadRequest, "asset_type must be image, video, or unknown")
		return
	}
	sort := strings.TrimSpace(r.URL.Query().Get("sort"))
	if sort == "" {
		sort = "recent"
	}
	if sort != "recent" && sort != "impressions" && sort != "duration" {
		writeError(w, http.StatusBadRequest, "sort must be recent, impressions, or duration")
		return
	}
	view := strings.TrimSpace(r.URL.Query().Get("view"))
	if view == "" {
		view = "all"
	}
	if view != "available" && view != "analyze" && view != "generated" && view != "rejected" && view != "selected" && view != "all" {
		writeError(w, http.StatusBadRequest, "view must be available, analyze, generated, rejected, selected, or all")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
WITH default_market_pack AS (
  SELECT r.id::text AS market_pack_id, revision.version AS market_pack_version, revision.config->>'copy_library_id' AS copy_library_id
  FROM creative_resource r
  JOIN creative_resource_revision revision ON revision.resource_id = r.id AND revision.version = r.published_version
  WHERE r.workspace_id = $1
    AND r.kind = 'market_pack'
    AND r.status <> 'archived'
    AND r.published_version IS NOT NULL
    AND COALESCE(revision.config->>'pre_adaptation_default', 'false') = 'true'
),
default_pre_adaptation_resource AS (
  SELECT market.market_pack_id, market.market_pack_version, library.id::text AS copy_library_id, library_revision.version AS copy_library_version
  FROM default_market_pack market
  JOIN creative_resource library ON library.id::text = market.copy_library_id
    AND library.workspace_id = $1
    AND library.kind = 'copy_library'
    AND library.status <> 'archived'
    AND library.published_version IS NOT NULL
  JOIN creative_resource_revision library_revision ON library_revision.resource_id = library.id AND library_revision.version = library.published_version
  WHERE (SELECT count(*) FROM default_market_pack) = 1
)
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
  COALESCE(latest.issue_id::text, ''), COALESCE(run.run_id::text, ''),
  COALESCE(run.is_new_in_run, false), COALESCE(run.analysis_status, 'pending'),
  COALESCE(run.analysis_error, '')
FROM creative_material_candidate c
LEFT JOIN LATERAL (
  SELECT ic.issue_id, ic.status, ic.tags, ic.note, ic.selected_at
  FROM creative_material_issue_candidate ic
  WHERE ic.workspace_id = c.workspace_id AND ic.candidate_id = c.id
  ORDER BY ic.updated_at DESC
  LIMIT 1
) latest ON true
LEFT JOIN LATERAL (
  SELECT feedback.decision
  FROM creative_feedback_event feedback
  WHERE feedback.workspace_id = c.workspace_id
    AND feedback.subject_type = 'candidate'
    AND feedback.subject_id = c.id
    AND feedback.event_type = 'decision'
    AND NOT EXISTS (
      SELECT 1 FROM creative_feedback_event undo WHERE undo.undo_of_id = feedback.id
    )
  ORDER BY feedback.created_at DESC, feedback.id DESC
  LIMIT 1
) decision ON true
LEFT JOIN LATERAL (
  SELECT analysis.id, analysis.status, analysis.result, analysis.analysis_version
  FROM creative_source_analysis analysis
  WHERE analysis.workspace_id = c.workspace_id AND analysis.candidate_id = c.id
  ORDER BY analysis.analysis_version DESC, COALESCE(analysis.completed_at, analysis.created_at) DESC, analysis.id DESC
  LIMIT 1
) latest_analysis ON true
LEFT JOIN LATERAL (
  SELECT (
    c.asset_type = 'image'
    AND c.archived_url <> ''
    AND COALESCE(decision.decision, '') <> 'rejected'
    AND COALESCE(latest.status, 'new') <> 'rejected'
    AND NOT EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id)
    AND latest_analysis.status = 'completed'
    AND NOT (
      jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'text_blocks') = 'array' THEN latest_analysis.result->'text_blocks' ELSE '[]'::jsonb END) > 0
      AND jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'visual_regions') = 'array' THEN latest_analysis.result->'visual_regions' ELSE '[]'::jsonb END) = 0
    )
    AND EXISTS (
      SELECT 1
      FROM default_pre_adaptation_resource resource
      WHERE latest_analysis.result->'adaptation'->>'status' = 'completed'
        AND latest_analysis.result->'adaptation'->'result'->>'market_pack_id' = resource.market_pack_id
        AND latest_analysis.result->'adaptation'->'result'->>'market_pack_version' = resource.market_pack_version::text
        AND latest_analysis.result->'adaptation'->'result'->>'copy_library_id' = resource.copy_library_id
        AND latest_analysis.result->'adaptation'->'result'->>'copy_library_version' = resource.copy_library_version::text
        AND (
          jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'adaptation'->'result'->'text_replacements') = 'array' THEN latest_analysis.result->'adaptation'->'result'->'text_replacements' ELSE '[]'::jsonb END) > 0
          OR jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'adaptation'->'result'->'numeric_layouts') = 'array' THEN latest_analysis.result->'adaptation'->'result'->'numeric_layouts' ELSE '[]'::jsonb END) > 0
        )
    )
  ) AS is_available
) availability ON true
LEFT JOIN LATERAL (
  SELECT rc.run_id, rc.is_new_in_run,
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
         END AS analysis_status,
         CASE
           WHEN rc.analysis_status = 'completed' THEN rc.analysis_error
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
           ) THEN COALESCE((
             SELECT NULLIF(task.error, '')
             FROM agent_task_queue task
             WHERE task.trigger_evidence_kind = 'creative_crawl_run_analysis'
               AND task.trigger_evidence_ref_id = rc.run_id
               AND task.status = 'failed'
               AND task.context->>'candidate_id' = rc.candidate_id::text
             ORDER BY task.completed_at DESC NULLS LAST, task.created_at DESC
             LIMIT 1
           ), 'analysis task failed before a source analysis was recorded')
           ELSE rc.analysis_error
         END AS analysis_error
  FROM creative_material_crawl_run_candidate rc
  JOIN creative_material_crawl_run cr ON cr.id = rc.run_id
  WHERE rc.workspace_id = c.workspace_id AND rc.candidate_id = c.id
    AND (NOT $2::boolean OR rc.run_id = $3)
  ORDER BY cr.created_at DESC, rc.created_at DESC
  LIMIT 1
) run ON true
 WHERE c.workspace_id = $1
   AND (NOT $2::boolean OR run.run_id IS NOT NULL)
   AND ($4 = '' OR c.title ILIKE '%' || $4 || '%' OR c.competitor ILIKE '%' || $4 || '%'
     OR c.connector_id ILIKE '%' || $4 || '%' OR EXISTS (
       SELECT 1 FROM unnest(c.tags || COALESCE(latest.tags, '{}'::text[])) tag WHERE tag ILIKE '%' || $4 || '%'
     ))
   AND ($5 = '' OR c.competitor = $5)
   AND ($6 = '' OR $6 = ANY(c.area_names))
   AND ($7 = '' OR $7 = ANY(c.language_names))
   AND ($8 = '' OR $8 = ANY(c.media_names))
   AND ($9 = '' OR c.asset_type = $9)
   AND (
     $10 = 'all'
     OR ($10 = 'available' AND COALESCE(availability.is_available, false))
     OR ($10 = 'analyze'
       AND c.asset_type = 'image'
       AND COALESCE(decision.decision, '') <> 'rejected'
       AND COALESCE(latest.status, 'new') <> 'rejected'
       AND NOT EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id)
       AND NOT COALESCE(availability.is_available, false)
     )
     OR ($10 = 'generated' AND EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id))
     OR ($10 = 'rejected' AND (COALESCE(decision.decision, '') = 'rejected' OR COALESCE(latest.status, '') = 'rejected'))
     OR ($10 = 'selected' AND (COALESCE(decision.decision, '') = 'selected' OR COALESCE(latest.status, '') = 'selected'))
   )
 ORDER BY
   CASE WHEN $11 = 'impressions' THEN c.impression_estimate END DESC NULLS LAST,
   CASE WHEN $11 = 'duration' THEN c.duration_days END DESC NULLS LAST,
   c.last_seen_at DESC
 LIMIT $12 OFFSET $13
`, workspaceID, filterRun, nullableUUID(runID, filterRun), query, competitor, area, language, media, assetType, view, sort, limit, offset)
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
			&item.AnalysisStatus, &item.AnalysisError,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative material library")
			return
		}
		item.DurationDays = float8Ptr(duration)
		item.ImpressionEstimate = int8Ptr(impression)
		if selectedAt != "" {
			item.SelectedAt = &selectedAt
		}
		item.ArchivedURL = h.creativeMaterialArchiveResponseURL(item.ID, item.ArchivedURL)
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read creative material library")
		return
	}
	rows.Close()
	var totalCount int
	err = h.DB.QueryRow(r.Context(), `
WITH default_market_pack AS (
  SELECT r.id::text AS market_pack_id, revision.version AS market_pack_version, revision.config->>'copy_library_id' AS copy_library_id
  FROM creative_resource r
  JOIN creative_resource_revision revision ON revision.resource_id = r.id AND revision.version = r.published_version
  WHERE r.workspace_id = $1
    AND r.kind = 'market_pack'
    AND r.status <> 'archived'
    AND r.published_version IS NOT NULL
    AND COALESCE(revision.config->>'pre_adaptation_default', 'false') = 'true'
),
default_pre_adaptation_resource AS (
  SELECT market.market_pack_id, market.market_pack_version, library.id::text AS copy_library_id, library_revision.version AS copy_library_version
  FROM default_market_pack market
  JOIN creative_resource library ON library.id::text = market.copy_library_id
    AND library.workspace_id = $1
    AND library.kind = 'copy_library'
    AND library.status <> 'archived'
    AND library.published_version IS NOT NULL
  JOIN creative_resource_revision library_revision ON library_revision.resource_id = library.id AND library_revision.version = library.published_version
  WHERE (SELECT count(*) FROM default_market_pack) = 1
)
SELECT count(*)
FROM creative_material_candidate c
LEFT JOIN LATERAL (
  SELECT ic.issue_id, ic.status, ic.tags, ic.note, ic.selected_at
  FROM creative_material_issue_candidate ic
  WHERE ic.workspace_id = c.workspace_id AND ic.candidate_id = c.id
  ORDER BY ic.updated_at DESC
  LIMIT 1
) latest ON true
LEFT JOIN LATERAL (
  SELECT feedback.decision
  FROM creative_feedback_event feedback
  WHERE feedback.workspace_id = c.workspace_id
    AND feedback.subject_type = 'candidate'
    AND feedback.subject_id = c.id
    AND feedback.event_type = 'decision'
    AND NOT EXISTS (
      SELECT 1 FROM creative_feedback_event undo WHERE undo.undo_of_id = feedback.id
    )
  ORDER BY feedback.created_at DESC, feedback.id DESC
  LIMIT 1
) decision ON true
LEFT JOIN LATERAL (
  SELECT analysis.id, analysis.status, analysis.result, analysis.analysis_version
  FROM creative_source_analysis analysis
  WHERE analysis.workspace_id = c.workspace_id AND analysis.candidate_id = c.id
  ORDER BY analysis.analysis_version DESC, COALESCE(analysis.completed_at, analysis.created_at) DESC, analysis.id DESC
  LIMIT 1
) latest_analysis ON true
LEFT JOIN LATERAL (
  SELECT (
    c.asset_type = 'image'
    AND c.archived_url <> ''
    AND COALESCE(decision.decision, '') <> 'rejected'
    AND COALESCE(latest.status, 'new') <> 'rejected'
    AND NOT EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id)
    AND latest_analysis.status = 'completed'
    AND NOT (
      jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'text_blocks') = 'array' THEN latest_analysis.result->'text_blocks' ELSE '[]'::jsonb END) > 0
      AND jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'visual_regions') = 'array' THEN latest_analysis.result->'visual_regions' ELSE '[]'::jsonb END) = 0
    )
    AND EXISTS (
      SELECT 1
      FROM default_pre_adaptation_resource resource
      WHERE latest_analysis.result->'adaptation'->>'status' = 'completed'
        AND latest_analysis.result->'adaptation'->'result'->>'market_pack_id' = resource.market_pack_id
        AND latest_analysis.result->'adaptation'->'result'->>'market_pack_version' = resource.market_pack_version::text
        AND latest_analysis.result->'adaptation'->'result'->>'copy_library_id' = resource.copy_library_id
        AND latest_analysis.result->'adaptation'->'result'->>'copy_library_version' = resource.copy_library_version::text
        AND (
          jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'adaptation'->'result'->'text_replacements') = 'array' THEN latest_analysis.result->'adaptation'->'result'->'text_replacements' ELSE '[]'::jsonb END) > 0
          OR jsonb_array_length(CASE WHEN jsonb_typeof(latest_analysis.result->'adaptation'->'result'->'numeric_layouts') = 'array' THEN latest_analysis.result->'adaptation'->'result'->'numeric_layouts' ELSE '[]'::jsonb END) > 0
        )
    )
  ) AS is_available
) availability ON true
WHERE c.workspace_id = $1
  AND (NOT $2::boolean OR EXISTS (
    SELECT 1 FROM creative_material_crawl_run_candidate rc
    WHERE rc.workspace_id = c.workspace_id AND rc.candidate_id = c.id AND rc.run_id = $3
  ))
  AND ($4 = '' OR c.title ILIKE '%' || $4 || '%' OR c.competitor ILIKE '%' || $4 || '%'
    OR c.connector_id ILIKE '%' || $4 || '%' OR EXISTS (
      SELECT 1 FROM unnest(c.tags || COALESCE(latest.tags, '{}'::text[])) tag WHERE tag ILIKE '%' || $4 || '%'
    ))
  AND ($5 = '' OR c.competitor = $5)
  AND ($6 = '' OR $6 = ANY(c.area_names))
  AND ($7 = '' OR $7 = ANY(c.language_names))
  AND ($8 = '' OR $8 = ANY(c.media_names))
  AND ($9 = '' OR c.asset_type = $9)
  AND (
    $10 = 'all'
    OR ($10 = 'available' AND COALESCE(availability.is_available, false))
    OR ($10 = 'analyze'
      AND c.asset_type = 'image'
      AND COALESCE(decision.decision, '') <> 'rejected'
      AND COALESCE(latest.status, 'new') <> 'rejected'
      AND NOT EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id)
      AND NOT COALESCE(availability.is_available, false)
    )
    OR ($10 = 'generated' AND EXISTS (SELECT 1 FROM creative_order_item item WHERE item.candidate_id = c.id))
    OR ($10 = 'rejected' AND (COALESCE(decision.decision, '') = 'rejected' OR COALESCE(latest.status, '') = 'rejected'))
    OR ($10 = 'selected' AND (COALESCE(decision.decision, '') = 'selected' OR COALESCE(latest.status, '') = 'selected'))
  )
`, workspaceID, filterRun, nullableUUID(runID, filterRun), query, competitor, area, language, media, assetType, view).Scan(&totalCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to count creative material library")
		return
	}
	runs, err := h.listCreativeCrawlRunsForWorkspace(r.Context(), workspaceID, pgtype.UUID{}, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list creative crawl runs")
		return
	}
	var nextOffset any
	if offset+len(candidates) < totalCount {
		nextOffset = offset + len(candidates)
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates, "total_count": totalCount, "next_offset": nextOffset, "crawl_runs": runs})
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
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
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
	analysis := h.enqueueManualReferenceAnalysis(r.Context(), workspaceID, userID, parseUUID(candidateID), connectorID, false)
	writeJSON(w, http.StatusCreated, creativeMaterialLibraryImportResponse{ID: candidateID, Analysis: analysis})
}

// RetryCreativeMaterialReferenceAnalysis reuses the manual-import analysis path
// for a historical candidate. It only creates/reuses analysis evidence and a
// direct analysis task; it never starts an AppGrowing crawl.
func (h *Handler) RetryCreativeMaterialReferenceAnalysis(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := creativeWorkspaceUser(w, r, h)
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "candidate_id")
	if !ok {
		return
	}
	var connectorID string
	err := h.DB.QueryRow(r.Context(), `
SELECT connector_id
FROM creative_material_candidate
WHERE id = $1 AND workspace_id = $2
`, candidateID, workspaceID).Scan(&connectorID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative material not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative material")
		return
	}
	analysis := h.enqueueManualReferenceAnalysis(r.Context(), workspaceID, userID, candidateID, connectorID, true)
	writeJSON(w, http.StatusOK, creativeMaterialLibraryImportResponse{ID: uuidToString(candidateID), Analysis: analysis})
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
	if err := hydrateCreativeResourcePublishedConfig(r.Context(), tx, &resource); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read published creative resource")
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
	if err := hydrateCreativeResourcePublishedConfig(r.Context(), tx, &resource); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read published creative resource")
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
	if current.Kind == "copy_library" {
		if err := validateComposableCopyLibraryConfig(config); err != nil {
			writeError(w, http.StatusBadRequest, "copy library cannot be published: "+err.Error())
			return
		}
	} else if current.Kind == "market_pack" {
		config, err = h.validateMarketPackPrimeTemplates(r.Context(), workspaceID, resourceID, current.Config)
		if err != nil {
			writeError(w, http.StatusBadRequest, "market resource pack cannot be published: "+err.Error())
			return
		}
		var marketConfig map[string]any
		if json.Unmarshal(config, &marketConfig) == nil && marketConfig["pre_adaptation_default"] == true {
			var anotherDefault bool
			if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_resource resource
  JOIN creative_resource_revision revision ON revision.resource_id = resource.id AND revision.version = resource.published_version
  WHERE resource.workspace_id = $1 AND resource.kind = 'market_pack' AND resource.id <> $2
    AND resource.status <> 'archived' AND resource.published_version IS NOT NULL
    AND COALESCE(revision.config->>'pre_adaptation_default', 'false') = 'true'
)
`, workspaceID, resourceID).Scan(&anotherDefault); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to validate pre-adaptation market pack")
				return
			}
			if anotherDefault {
				writeError(w, http.StatusUnprocessableEntity, "another published market pack is already the pre-adaptation default")
				return
			}
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
	resource.PublishedConfig = resource.Config
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to publish creative resource")
		return
	}
	writeJSON(w, http.StatusOK, resource)
}

type composableCopyLibraryRepaymentPlanLabels struct {
	Principal          string `json:"principal"`
	Tenor              string `json:"tenor"`
	MonthlyInstallment string `json:"monthly_installment"`
	TotalInterest      string `json:"total_interest"`
	TotalRepayment     string `json:"total_repayment"`
}

type composableCopyLibraryRepaymentPlanEntry struct {
	ID                 string `json:"id"`
	Key                string `json:"key"`
	Principal          int64  `json:"principal"`
	TenorMonths        int    `json:"tenor_months"`
	MonthlyInstallment int64  `json:"monthly_installment"`
	TotalInterest      int64  `json:"total_interest"`
	TotalRepayment     int64  `json:"total_repayment"`
	Source             string `json:"source"`
	Status             string `json:"status"`
}

type composableCopyLibraryRepaymentPlan struct {
	Labels  composableCopyLibraryRepaymentPlanLabels  `json:"labels"`
	Entries []composableCopyLibraryRepaymentPlanEntry `json:"entries"`
}

// Kept only to read historical frozen snapshots during the schema transition.
// New v4 libraries neither expose nor validate product facts.
var creativeCopyFactReferencePattern = regexp.MustCompile(`\{\{fact\.([a-z0-9_]+)\.(copy_text|value)\}\}`)

type composableCopyLibraryConfig struct {
	SchemaVersion int    `json:"schema_version"`
	Locale        string `json:"locale"`
	Fragments     []struct {
		ID            string   `json:"id"`
		Key           string   `json:"key"`
		CreativeTypes []string `json:"creative_types"`
		Role          string   `json:"role"`
		SemanticGroup string   `json:"semantic_group"`
		Usage         string   `json:"usage"`
		Text          string   `json:"text"`
		Status        string   `json:"status"`
	} `json:"fragments"`
	RepaymentPlan composableCopyLibraryRepaymentPlan `json:"repayment_plan"`
	Recipes       []struct {
		ID           string              `json:"id"`
		Key          string              `json:"key"`
		CreativeType string              `json:"creative_type"`
		FragmentIDs  map[string][]string `json:"fragment_ids"`
		Status       string              `json:"status"`
	} `json:"recipes"`
	ProductFacts []struct {
		ID       string `json:"id"`
		Key      string `json:"key"`
		Label    string `json:"label"`
		Value    string `json:"value"`
		CopyText string `json:"copy_text"`
		Source   string `json:"source"`
		Status   string `json:"status"`
	} `json:"product_facts"`
}

func validateComposableCopyLibraryConfig(raw json.RawMessage) error {
	var config composableCopyLibraryConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return errors.New("config must be valid JSON")
	}
	if config.SchemaVersion != 4 {
		return errors.New("schema_version must be 4")
	}
	locale := strings.TrimSpace(config.Locale)
	if matched, _ := regexp.MatchString(`^[a-z]{2,3}-[A-Z]{2}$`, locale); !matched {
		return errors.New("locale must be a language-region code such as id-ID or ms-MY")
	}
	validType := func(value string) bool { return value == "num" || value == "repayment_plan" }
	type fragmentContract struct {
		role  string
		types map[string]bool
		text  string
	}
	fragments := make(map[string]fragmentContract, len(config.Fragments))
	approvedHeadlines := 0
	approvedBenefits := 0
	for index, fragment := range config.Fragments {
		id := strings.TrimSpace(fragment.ID)
		if id == "" || strings.TrimSpace(fragment.Key) == "" {
			return fmt.Errorf("fragment %d requires id and key", index+1)
		}
		if _, exists := fragments[id]; exists {
			return fmt.Errorf("fragment id %q is duplicated", id)
		}
		types := map[string]bool{}
		for _, creativeType := range fragment.CreativeTypes {
			if !validType(creativeType) {
				return fmt.Errorf("fragment %q has unsupported creative type %q", fragment.Key, creativeType)
			}
			types[creativeType] = true
		}
		if fragment.Status != "approved" {
			continue
		}
		if !validCopyFragmentRole(fragment.Role) || strings.TrimSpace(fragment.Text) == "" {
			return fmt.Errorf("approved fragment %q requires role and text", fragment.Key)
		}
		if fragment.Usage != "" && fragment.Usage != "core" && fragment.Usage != "fallback" && fragment.Usage != "required" {
			return fmt.Errorf("approved fragment %q has unsupported usage %q", fragment.Key, fragment.Usage)
		}
		if strings.Contains(fragment.Text, "{{") || strings.Contains(fragment.Text, "}}") {
			return fmt.Errorf("approved fragment %q must contain final copy, not template variables", fragment.Key)
		}
		fragments[id] = fragmentContract{role: fragment.Role, types: types, text: fragment.Text}
		if fragment.Role == "headline" {
			approvedHeadlines++
		}
		if fragment.Role == "benefit" {
			approvedBenefits++
		}
	}
	labels := []string{config.RepaymentPlan.Labels.Principal, config.RepaymentPlan.Labels.Tenor, config.RepaymentPlan.Labels.MonthlyInstallment, config.RepaymentPlan.Labels.TotalInterest, config.RepaymentPlan.Labels.TotalRepayment}
	for _, label := range labels {
		if strings.TrimSpace(label) == "" {
			return errors.New("repayment plan requires all table labels")
		}
	}
	planKeys := map[string]bool{}
	planPairs := map[string]bool{}
	approvedPlans := 0
	for index, entry := range config.RepaymentPlan.Entries {
		if entry.Status != "approved" {
			continue
		}
		key := strings.TrimSpace(entry.Key)
		if strings.TrimSpace(entry.ID) == "" || key == "" || strings.TrimSpace(entry.Source) == "" || entry.Principal <= 0 || entry.TenorMonths <= 0 || entry.MonthlyInstallment <= 0 || entry.TotalInterest < 0 || entry.TotalRepayment <= 0 {
			return fmt.Errorf("approved repayment plan row %d requires identity, source, and valid amounts", index+1)
		}
		if planKeys[key] {
			return fmt.Errorf("approved repayment plan key %q is duplicated", key)
		}
		pair := fmt.Sprintf("%d:%d", entry.Principal, entry.TenorMonths)
		if planPairs[pair] {
			return fmt.Errorf("approved repayment plan amount/term %q is duplicated", pair)
		}
		planKeys[key] = true
		planPairs[pair] = true
		approvedPlans++
	}
	if approvedPlans == 0 {
		return errors.New("at least one approved repayment plan row is required")
	}
	approvedRecipeIDs := map[string]bool{}
	for index, recipe := range config.Recipes {
		if !validType(recipe.CreativeType) {
			return fmt.Errorf("recipe %d has unsupported creative type %q", index+1, recipe.CreativeType)
		}
		if recipe.Status != "approved" {
			continue
		}
		recipeID := strings.TrimSpace(recipe.ID)
		if recipeID == "" || strings.TrimSpace(recipe.Key) == "" {
			return fmt.Errorf("approved recipe %d requires id and key", index+1)
		}
		if approvedRecipeIDs[recipeID] {
			return fmt.Errorf("approved recipe id %q is duplicated", recipeID)
		}
		approvedRecipeIDs[recipeID] = true
		used := 0
		for role, ids := range recipe.FragmentIDs {
			if !validCopyFragmentRole(role) {
				return fmt.Errorf("approved recipe %d has unsupported role %q", index+1, role)
			}
			for _, id := range ids {
				fragment, exists := fragments[id]
				if !exists {
					return fmt.Errorf("approved recipe %d references missing or unapproved fragment %q", index+1, id)
				}
				if fragment.role != role {
					return fmt.Errorf("approved recipe %d uses fragment %q in role %q instead of %q", index+1, id, role, fragment.role)
				}
				if !fragment.types[recipe.CreativeType] {
					return fmt.Errorf("approved recipe %d uses fragment %q for incompatible creative type", index+1, id)
				}
				used++
			}
		}
		if used == 0 {
			return fmt.Errorf("approved recipe %d must use at least one fragment", index+1)
		}
	}
	if approvedHeadlines == 0 || approvedBenefits == 0 {
		return errors.New("approved headline and benefit fragments are required")
	}
	return nil
}

func validCopyFragmentRole(value string) bool {
	switch value {
	case "headline", "subheadline", "benefit", "supporting", "cta", "legal":
		return true
	default:
		return false
	}
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
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	if actorType == "agent" {
		if !issue.AssigneeType.Valid || issue.AssigneeType.String != "squad" || issue.AssigneeID != squadID {
			writeError(w, http.StatusForbidden, "agent can only bind the issue's assigned squad")
			return
		}
		squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{
			ID:          squadID,
			WorkspaceID: issue.WorkspaceID,
		})
		if err != nil || !creativeContextAgentCanBind(issue, squadID, actorID, squad) {
			writeError(w, http.StatusForbidden, "only the assigned squad leader can bind creative context")
			return
		}
	}
	snapshot, err := h.buildCreativeIssueSnapshot(r.Context(), issue.WorkspaceID, marketPackID, squadID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
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

func creativeContextAgentCanBind(issue db.Issue, requestedSquadID pgtype.UUID, actorID string, squad db.Squad) bool {
	return issue.AssigneeType.Valid && issue.AssigneeType.String == "squad" &&
		issue.AssigneeID == requestedSquadID && squad.ID == requestedSquadID &&
		squad.WorkspaceID == issue.WorkspaceID && uuidToString(squad.LeaderID) == actorID
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
	input, err := decodeCreativeBrief(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
RETURNING issue_id::text, candidate_id::text, creative_brief::text,
          COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
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
RETURNING issue_id::text, candidate_id::text, creative_brief::text,
          COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
`, issue.ID, candidateID, issue.WorkspaceID, workIssueID, requestingUserID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "save a creative brief before starting creative work")
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
SELECT issue_id::text, candidate_id::text, creative_brief::text,
       COALESCE(work_issue_id::text, ''), revision, status, updated_at::text
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
  AND r.status <> 'archived' AND r.published_version IS NOT NULL
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

func bumpCreativeResourceRevision(ctx context.Context, tx pgx.Tx, workspaceID, resourceID, userID pgtype.UUID) (creativeResourceResponse, error) {
	resource, err := scanCreativeResource(tx.QueryRow(ctx, `
UPDATE creative_resource
SET config = config - 'prime_template_set_validation' - 'prime_layout_contract',
    status = 'draft', version = version + 1, updated_at = now()
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
	if err := hydrateCreativeResourcePublishedConfig(ctx, tx, &resource); err != nil {
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
	item.PublishedConfig = json.RawMessage("{}")
	return item, err
}

func scanCreativeResourceWithPublishedConfig(row rowScanner) (creativeResourceResponse, error) {
	var item creativeResourceResponse
	var config, publishedConfig string
	err := row.Scan(
		&item.ID, &item.WorkspaceID, &item.Kind, &item.Name, &item.Description,
		&item.Status, &item.Version, &item.PublishedVersion, &config, &publishedConfig, &item.CreatedBy,
		&item.CreatedAt, &item.UpdatedAt,
	)
	item.Config = json.RawMessage(config)
	item.PublishedConfig = json.RawMessage(publishedConfig)
	return item, err
}

type creativeResourceConfigQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func hydrateCreativeResourcePublishedConfig(ctx context.Context, querier creativeResourceConfigQuerier, resource *creativeResourceResponse) error {
	resource.PublishedConfig = json.RawMessage("{}")
	if resource.PublishedVersion <= 0 {
		return nil
	}
	var config string
	if err := querier.QueryRow(ctx, `
SELECT config::text
FROM creative_resource_revision
WHERE resource_id = $1::uuid AND version = $2
`, resource.ID, resource.PublishedVersion).Scan(&config); err != nil {
		return err
	}
	resource.PublishedConfig = json.RawMessage(config)
	return nil
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
	var brief string
	err := row.Scan(&item.IssueID, &item.CandidateID, &brief, &item.WorkIssueID, &item.Revision, &item.Status, &item.UpdatedAt)
	item.CreativeBrief = json.RawMessage(brief)
	return item, err
}

func decodeCreativeBrief(reader io.Reader) (creativeBriefInput, error) {
	var input creativeBriefInput
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return creativeBriefInput{}, fmt.Errorf("invalid creative brief: %w", err)
	}
	return input, nil
}

func normalizeCreativeBrief(input creativeBriefInput) (creativeBriefInput, error) {
	input.Theme = strings.TrimSpace(input.Theme)
	input.PrimaryBenefit = strings.TrimSpace(input.PrimaryBenefit)
	input.BenefitValue = strings.TrimSpace(input.BenefitValue)
	input.SourceSemantics = strings.TrimSpace(input.SourceSemantics)
	input.InformationMechanism = strings.TrimSpace(input.InformationMechanism)
	input.VisualType = strings.TrimSpace(input.VisualType)
	input.AnalysisSummary = strings.TrimSpace(input.AnalysisSummary)
	input.UserDirection = strings.TrimSpace(input.UserDirection)
	input.AnalysisIssueID = strings.TrimSpace(input.AnalysisIssueID)
	input.ThemeElements = normalizedCreativeBriefList(input.ThemeElements, 12)
	input.SecondaryBenefits = normalizedCreativeBriefList(input.SecondaryBenefits, 12)
	input.VisualAnchors = normalizedCreativeBriefList(input.VisualAnchors, 12)
	input.PaletteAnchors = normalizedCreativeBriefList(input.PaletteAnchors, 8)
	input.MustPreserve = normalizedCreativeBriefList(input.MustPreserve, 16)
	input.AllowedVariations = normalizedCreativeBriefList(input.AllowedVariations, 16)
	input.Evidence = normalizedCreativeBriefList(input.Evidence, 16)
	input.DetectedText = normalizedCreativeBriefList(input.DetectedText, 32)
	var err error
	input.SelectedAppUIReferences, err = normalizedCreativeBriefAppUIReferences(input.SelectedAppUIReferences, 8)
	if err != nil {
		return creativeBriefInput{}, err
	}
	if len(input.SelectedAppUIReferences) > 0 {
		input.AppUIReplacementRequired = true
	}
	if input.AppUIReplacementRequired && len(input.SelectedAppUIReferences) == 0 {
		return creativeBriefInput{}, errors.New("creative brief requires at least one selected App UI reference")
	}
	if len([]rune(input.Theme)) > 120 || len([]rune(input.PrimaryBenefit)) > 120 || len([]rune(input.BenefitValue)) > 160 || len([]rune(input.SourceSemantics)) > 300 || len([]rune(input.InformationMechanism)) > 300 || len([]rune(input.VisualType)) > 120 || len([]rune(input.AnalysisSummary)) > 2000 || len([]rune(input.UserDirection)) > 2000 {
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

func normalizedCreativeBriefAppUIReferences(values []creativeBriefAppUIReferenceInput, limit int) ([]creativeBriefAppUIReferenceInput, error) {
	result := make([]creativeBriefAppUIReferenceInput, 0, min(len(values), limit))
	seen := map[string]struct{}{}
	for _, value := range values {
		value.ResourceFileID = strings.TrimSpace(value.ResourceFileID)
		value.AttachmentID = strings.TrimSpace(value.AttachmentID)
		value.Reason = strings.TrimSpace(value.Reason)
		if value.ResourceFileID == "" || value.AttachmentID == "" || value.Reason == "" {
			return nil, errors.New("creative brief App UI references require resource_file_id, attachment_id, and reason")
		}
		if len([]rune(value.Reason)) > 300 {
			return nil, errors.New("creative brief App UI reference reason is too long")
		}
		key := strings.ToLower(value.ResourceFileID + "\x00" + value.AttachmentID)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result, nil
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

func parseUUIDString(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	err := id.Scan(strings.TrimSpace(value))
	return id, err
}
