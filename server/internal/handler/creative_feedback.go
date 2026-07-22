package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/logger"
)

const (
	creativeFeedbackBodyLimit       = 32 * 1024
	creativeFeedbackSuggestionLimit = 2000
)

var creativeFeedbackReasons = map[string]map[string]struct{}{
	"accepted": {
		"ready_to_publish": {},
		"copy_accurate":    {},
		"benefit_clear":    {},
		"layout_match":     {},
		"brand_complete":   {},
		"other":            {},
	},
	"rejected": {
		"copy_error":       {},
		"copy_too_long":    {},
		"benefit_mismatch": {},
		"layout_mismatch":  {},
		"missing_content":  {},
		"brand_compliance": {},
		"visual_quality":   {},
		"other":            {},
	},
	"needs_revision": {
		"copy_error":       {},
		"copy_too_long":    {},
		"benefit_mismatch": {},
		"layout_mismatch":  {},
		"missing_content":  {},
		"brand_compliance": {},
		"visual_quality":   {},
		"other":            {},
	},
}

type creativeEditFeedbackResponse struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	IssueID         string          `json:"issue_id"`
	JobID           string          `json:"job_id"`
	CandidateID     string          `json:"candidate_id"`
	VariantID       string          `json:"variant_id"`
	Decision        string          `json:"decision"`
	ReasonCodes     []string        `json:"reason_codes"`
	Suggestion      string          `json:"suggestion"`
	ProcessSnapshot json.RawMessage `json:"process_snapshot"`
	CreatedBy       string          `json:"created_by"`
	CreatedByName   string          `json:"created_by_name"`
	CreatedAt       string          `json:"created_at"`
}

type createCreativeEditFeedbackRequest struct {
	Decision    string   `json:"decision"`
	ReasonCodes []string `json:"reason_codes"`
	Suggestion  string   `json:"suggestion"`
}

func (h *Handler) CreateCreativeEditFeedback(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	jobID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "jobId"), "job_id")
	if !ok {
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "variantId"), "variant_id")
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, creativeFeedbackBodyLimit)
	var request createCreativeEditFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	decision, reasons, suggestion, err := normalizeCreativeEditFeedback(request)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var feedbackID string
	err = h.DB.QueryRow(r.Context(), `
INSERT INTO creative_edit_feedback (
  workspace_id, issue_id, job_id, candidate_id, variant_id,
  decision, reason_codes, suggestion, process_snapshot, created_by, created_by_name
)
SELECT j.workspace_id, j.issue_id, j.id, v.candidate_id, v.id,
       $5, $6, $7,
       jsonb_build_object(
         'job_status', j.status,
         'job_stage', j.stage,
         'job_progress', j.progress,
         'job_prompt', j.prompt,
	     'process_data', COALESCE(j.process_data, '{}'::jsonb),
         'dynamic_rules', jsonb_build_object(
           'market', j.rules->'market',
           'strategy', j.rules->'strategy',
           'variant_count', j.rules->'variant_count',
           'sizes', COALESCE(j.rules->'sizes', '[]'::jsonb)
         ),
         'external_status', j.external_status,
         'external_provider', j.external_provider,
         'poll_attempts', j.poll_attempts,
         'job_created_at', j.created_at,
         'job_updated_at', j.updated_at,
         'job_completed_at', j.completed_at,
         'last_poll_at', j.last_poll_at,
         'source_candidate', jsonb_build_object(
           'competitor', c.competitor,
           'title', c.title,
           'asset_type', c.asset_type
         ),
         'variant_index', v.variant_index,
         'variant_qc_status', v.qc_status,
         'variant_created_at', v.created_at,
         'asset_count', asset_summary.asset_count,
         'assets', asset_summary.assets
       ),
       $8, u.name
FROM creative_edit_variant v
JOIN creative_edit_job j ON j.id = v.job_id
JOIN creative_material_candidate c ON c.id = v.candidate_id
JOIN "user" u ON u.id = $8
LEFT JOIN LATERAL (
  SELECT COUNT(*)::int AS asset_count,
         COALESCE(
           jsonb_agg(
             jsonb_build_object('width', a.width, 'height', a.height, 'label', a.label)
             ORDER BY a.width DESC, a.height DESC, a.label
           ),
           '[]'::jsonb
         ) AS assets
  FROM creative_edit_asset a
  WHERE a.variant_id = v.id
) asset_summary ON true
WHERE j.id = $3 AND v.id = $4 AND j.issue_id = $1 AND j.workspace_id = $2
RETURNING id::text
`, issue.ID, issue.WorkspaceID, jobID, variantID, decision, reasons, suggestion, userID).Scan(&feedbackID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative edit variant not found")
		return
	}
	if err != nil {
		slog.Warn("create creative edit feedback failed", append(logger.RequestAttrs(r), "error", err, "job_id", chi.URLParam(r, "jobId"), "variant_id", chi.URLParam(r, "variantId"))...)
		writeError(w, http.StatusInternalServerError, "failed to save creative feedback")
		return
	}

	response, err := h.loadCreativeMaterialsResponse(r.Context(), issue.ID, issue.WorkspaceID, parseIssueMetadata(issue.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative materials")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, "member", userID)
	slog.Info("creative edit feedback created", append(logger.RequestAttrs(r), "feedback_id", feedbackID, "variant_id", chi.URLParam(r, "variantId"), "decision", decision)...)
	writeJSON(w, http.StatusCreated, response)
}

func normalizeCreativeEditFeedback(request createCreativeEditFeedbackRequest) (string, []string, string, error) {
	decision := strings.TrimSpace(request.Decision)
	allowedReasons, ok := creativeFeedbackReasons[decision]
	if !ok {
		return "", nil, "", errors.New("decision must be accepted, rejected, or needs_revision")
	}
	reasons := make([]string, 0, len(request.ReasonCodes))
	seen := map[string]struct{}{}
	for _, value := range request.ReasonCodes {
		reason := strings.TrimSpace(value)
		if reason == "" {
			continue
		}
		if _, valid := allowedReasons[reason]; !valid {
			return "", nil, "", errors.New("reason_codes contains a reason that does not match the decision")
		}
		if _, exists := seen[reason]; exists {
			continue
		}
		seen[reason] = struct{}{}
		reasons = append(reasons, reason)
	}
	if len(reasons) == 0 {
		return "", nil, "", errors.New("select at least one feedback reason")
	}
	suggestion := strings.TrimSpace(request.Suggestion)
	if utf8.RuneCountInString(suggestion) > creativeFeedbackSuggestionLimit {
		return "", nil, "", errors.New("suggestion cannot exceed 2000 characters")
	}
	return decision, reasons, suggestion, nil
}
