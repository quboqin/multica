package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/imagemodel"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type creativeSourceAnalysisInput struct {
	CandidateID              string          `json:"candidate_id"`
	AnalysisVersion          int             `json:"analysis_version"`
	Status                   string          `json:"status"`
	Summary                  string          `json:"summary"`
	Result                   json.RawMessage `json:"result"`
	ErrorCode                string          `json:"error_code"`
	ErrorMessage             string          `json:"error_message"`
	TriggerEvidenceKind      string          `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string          `json:"trigger_evidence_ref_id"`
}

type creativeSourceAnalysisResponse struct {
	ID                       string          `json:"id"`
	WorkspaceID              string          `json:"workspace_id"`
	CandidateID              string          `json:"candidate_id"`
	AnalysisVersion          int             `json:"analysis_version"`
	Status                   string          `json:"status"`
	Summary                  string          `json:"summary"`
	Result                   json.RawMessage `json:"result"`
	ErrorCode                string          `json:"error_code"`
	ErrorMessage             string          `json:"error_message"`
	TriggerEvidenceKind      string          `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string          `json:"trigger_evidence_ref_id"`
	CreatedAt                string          `json:"created_at"`
	CompletedAt              string          `json:"completed_at"`
}

type creativeOrderItemInput struct {
	SourceKind       string          `json:"source_kind"`
	CopyLibraryID    string          `json:"copy_library_id"`
	CandidateID      string          `json:"candidate_id"`
	SourceAnalysisID string          `json:"source_analysis_id"`
	CopySnapshot     json.RawMessage `json:"copy_snapshot"`
	Direction        string          `json:"direction"`
}

type creativeOrderInput struct {
	IssueID                  string                   `json:"issue_id"`
	SubmissionKey            string                   `json:"submission_key"`
	Status                   string                   `json:"status"`
	InputSnapshot            json.RawMessage          `json:"input_snapshot"`
	TriggerEvidenceKind      string                   `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string                   `json:"trigger_evidence_ref_id"`
	Items                    []creativeOrderItemInput `json:"items"`
}

// Direction is a compact user-facing summary. The production agent authors
// the full per-size model prompt from the frozen copy and visual direction.
const maxCreativeOrderDirectionLength = 16_000

type creativeOrderResponse struct {
	Recoveries               []creativeOrderRecoveryResponse        `json:"recoveries,omitempty"`
	ID                       string                                 `json:"id"`
	WorkspaceID              string                                 `json:"workspace_id"`
	IssueID                  string                                 `json:"issue_id"`
	Status                   string                                 `json:"status"`
	DerivedStatus            string                                 `json:"derived_status"`
	DeliveryStatus           string                                 `json:"delivery_status"`
	ProductionStatus         string                                 `json:"production_status"`
	InputSnapshot            json.RawMessage                        `json:"input_snapshot"`
	TriggerEvidenceKind      string                                 `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string                                 `json:"trigger_evidence_ref_id"`
	CreatedBy                string                                 `json:"created_by"`
	CreatedAt                string                                 `json:"created_at"`
	UpdatedAt                string                                 `json:"updated_at"`
	WorkflowFailures         []creativeOrderWorkflowFailureResponse `json:"workflow_failures"`
	Items                    []creativeOrderItemResponse            `json:"items,omitempty"`
}

type creativeOrderWorkflowFailureResponse struct {
	TaskID                   string `json:"task_id"`
	AgentID                  string `json:"agent_id"`
	Workflow                 string `json:"workflow"`
	Scope                    string `json:"scope"`
	SubjectID                string `json:"subject_id"`
	ItemKey                  string `json:"item_key"`
	TriggerEvidenceKind      string `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string `json:"trigger_evidence_ref_id"`
	FailureReason            string `json:"failure_reason"`
	Error                    string `json:"error"`
	FailedAt                 string `json:"failed_at"`
	Retryable                bool   `json:"retryable"`
}

type creativeOrderWorkflowRetryResponse struct {
	TaskID string `json:"task_id"`
}

// creativeOrderQCRetryResponse describes a recovery run that deliberately
// reuses the already completed Prime assets. New recovery runs are visual-only;
// TechnicalTaskID is kept for older clients and remains empty.
type creativeOrderQCRetryResponse struct {
	VariantID       string `json:"variant_id"`
	Revision        int    `json:"revision"`
	Attempt         int    `json:"attempt"`
	TechnicalTaskID string `json:"technical_task_id"`
	VisualTaskID    string `json:"visual_task_id"`
}

// creativeOrderQCContextResponse is deliberately narrower than an order
// detail response. A visual QC task receives only its frozen target and the
// current Prime package, so sibling variants cannot become an accidental
// selection source during an independent QC lane.
type creativeOrderQCContextResponse struct {
	OrderID       string                       `json:"order_id"`
	OrderItemID   string                       `json:"order_item_id"`
	VariantID     string                       `json:"variant_id"`
	VariantKey    string                       `json:"variant_key"`
	Workflow      string                       `json:"workflow"`
	Revision      int                          `json:"revision"`
	Attempt       int                          `json:"attempt"`
	ExpectedSizes []string                     `json:"expected_sizes"`
	Brief         json.RawMessage              `json:"brief"`
	CopySnapshot  json.RawMessage              `json:"copy_snapshot"`
	PrimeAssets   []creativeOrderAssetResponse `json:"prime_assets"`
}

type creativeOrderItemResponse struct {
	CandidateProgress *creativeCandidateProgress     `json:"candidate_progress,omitempty"`
	SourceKind        string                         `json:"source_kind"`
	CopyLibraryID     string                         `json:"copy_library_id"`
	ID                string                         `json:"id"`
	OrderID           string                         `json:"order_id"`
	CandidateID       string                         `json:"candidate_id"`
	SourceAnalysisID  string                         `json:"source_analysis_id"`
	CopySnapshot      json.RawMessage                `json:"copy_snapshot"`
	Direction         string                         `json:"direction"`
	Status            string                         `json:"status"`
	AdoptedVariantID  string                         `json:"adopted_variant_id"`
	AdoptedAt         string                         `json:"adopted_at"`
	AdoptedBy         string                         `json:"adopted_by"`
	CreatedAt         string                         `json:"created_at"`
	UpdatedAt         string                         `json:"updated_at"`
	Variants          []creativeOrderVariantResponse `json:"variants,omitempty"`
}

type creativeOrderVariantResponse struct {
	ID                  string                           `json:"id"`
	OrderItemID         string                           `json:"order_item_id"`
	VariantKey          string                           `json:"variant_key"`
	Brief               json.RawMessage                  `json:"brief"`
	Revision            int                              `json:"revision"`
	Status              string                           `json:"status"`
	ActiveRevision      int                              `json:"active_revision"`
	StagingRevision     int                              `json:"staging_revision"`
	CandidateState      string                           `json:"candidate_state"`
	SelectionRank       int                              `json:"selection_rank"`
	PrimarySize         string                           `json:"primary_size"`
	QCStatus            string                           `json:"qc_status"`
	QCRecoveryUsed      bool                             `json:"qc_recovery_used"`
	QCRecoveryAvailable bool                             `json:"qc_recovery_available"`
	ActionRequired      *creativeOrderVariantBlocker     `json:"action_required,omitempty"`
	Assets              []creativeOrderAssetResponse     `json:"assets,omitempty"`
	Revisions           []creativeOrderVariantRevision   `json:"revisions,omitempty"`
	ImageOperations     []creativeImageOperationResponse `json:"image_operations,omitempty"`
	DiagnosticAssets    []creativeOrderDiagnosticAsset   `json:"diagnostic_assets,omitempty"`
	QCReports           []creativeOrderQCReportResponse  `json:"qc_reports,omitempty"`
	CreatedAt           string                           `json:"created_at"`
	UpdatedAt           string                           `json:"updated_at"`
}

type creativeOrderVariantBlocker struct {
	TaskID        string `json:"task_id"`
	Workflow      string `json:"workflow"`
	FailureReason string `json:"failure_reason"`
	Detail        string `json:"detail"`
	FailedAt      string `json:"failed_at"`
	Retryable     bool   `json:"retryable"`
}

type creativeOrderDiagnosticAsset struct {
	ID           string          `json:"id"`
	VariantID    string          `json:"variant_id"`
	TaskID       string          `json:"task_id"`
	AttachmentID string          `json:"attachment_id"`
	SizeKey      string          `json:"size_key"`
	Revision     int             `json:"revision"`
	Workflow     string          `json:"workflow"`
	Label        string          `json:"label"`
	Filename     string          `json:"filename"`
	Metadata     json.RawMessage `json:"metadata"`
	URL          string          `json:"url"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
}

type creativeOrderDiagnosticPromotionResponse struct {
	Candidate          creativeOrderDiagnosticAsset `json:"candidate"`
	Generated          creativeOrderAssetResponse   `json:"generated"`
	VariantID          string                       `json:"variant_id"`
	Revision           int                          `json:"revision"`
	SelectedMethod     string                       `json:"selected_method"`
	CompositionStarted bool                         `json:"composition_started"`
}

type creativeOrderAssetResponse struct {
	ID                 string          `json:"id"`
	VariantID          string          `json:"variant_id"`
	AssetFamilyID      string          `json:"asset_family_id"`
	SizeKey            string          `json:"size_key"`
	Revision           int             `json:"revision"`
	Stage              string          `json:"stage"`
	AttachmentID       string          `json:"attachment_id"`
	DerivedFromAssetID string          `json:"derived_from_asset_id"`
	OperationID        string          `json:"operation_id"`
	Metadata           json.RawMessage `json:"metadata"`
	Evidence           json.RawMessage `json:"evidence"`
	Status             string          `json:"status"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type creativeOrderVariantInput struct {
	OrderItemID    string          `json:"order_item_id"`
	VariantKey     string          `json:"variant_key"`
	Brief          json.RawMessage `json:"brief"`
	Revision       int             `json:"revision"`
	Status         string          `json:"status"`
	CandidateState string          `json:"candidate_state"`
	SelectionRank  *int            `json:"selection_rank"`
	PrimarySize    string          `json:"primary_size"`
}

type creativeOrderAssetInput struct {
	VariantID          string          `json:"variant_id"`
	AssetFamilyID      string          `json:"asset_family_id"`
	SizeKey            string          `json:"size_key"`
	Size               string          `json:"size"`
	Revision           int             `json:"revision"`
	Stage              string          `json:"stage"`
	AttachmentID       string          `json:"attachment_id"`
	DerivedFromAssetID string          `json:"derived_from_asset_id"`
	OperationID        string          `json:"operation_id"`
	Metadata           json.RawMessage `json:"metadata"`
	Evidence           json.RawMessage `json:"evidence"`
	Status             string          `json:"status"`
	// These flags are only used while accepting older agent envelopes. They
	// allow the handler to resolve omitted revision/stage/status fields without
	// weakening validation for canonical payloads.
	revisionDefaulted bool
}

type creativeOrderDiagnosticAssetInput struct {
	VariantID    string          `json:"variant_id"`
	TaskID       string          `json:"task_id"`
	AttachmentID string          `json:"attachment_id"`
	SizeKey      string          `json:"size_key"`
	Size         string          `json:"size"`
	Revision     int             `json:"revision"`
	Workflow     string          `json:"workflow"`
	Label        string          `json:"label"`
	Filename     string          `json:"filename"`
	Metadata     json.RawMessage `json:"metadata"`
}

type creativeOrderQCReportResponse struct {
	ID                       string          `json:"id"`
	VariantID                string          `json:"variant_id"`
	Lane                     string          `json:"lane"`
	Revision                 int             `json:"revision"`
	Attempt                  int             `json:"attempt"`
	Status                   string          `json:"status"`
	Findings                 json.RawMessage `json:"findings"`
	TriggerEvidenceKind      string          `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string          `json:"trigger_evidence_ref_id"`
	CreatedAt                string          `json:"created_at"`
	UpdatedAt                string          `json:"updated_at"`
}

type creativeOrderQCInput struct {
	VariantID                string          `json:"variant_id"`
	Lane                     string          `json:"lane"`
	Revision                 int             `json:"revision"`
	Attempt                  int             `json:"attempt"`
	Status                   string          `json:"status"`
	Findings                 json.RawMessage `json:"findings"`
	TriggerEvidenceKind      string          `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string          `json:"trigger_evidence_ref_id"`
}

type creativeOrderQCFinalizeInput struct {
	VariantID string `json:"variant_id"`
	Revision  int    `json:"revision"`
	Attempt   int    `json:"attempt"`
}

type creativeOrderQCFinalizeResponse struct {
	Created              bool   `json:"created"`
	Finalized            bool   `json:"finalized"`
	Outcome              string `json:"outcome"`
	VariantID            string `json:"variant_id"`
	Revision             int    `json:"revision"`
	Attempt              int    `json:"attempt"`
	TechnicalStatus      string `json:"technical_status"`
	VisualStatus         string `json:"visual_status"`
	DeliveredAssetCount  int    `json:"delivered_asset_count"`
	OrderAggregateStatus string `json:"order_aggregate_status"`
	InboxItemID          string `json:"inbox_item_id,omitempty"`
	ReworkTaskID         string `json:"rework_task_id,omitempty"`
	ReworkRevision       int    `json:"rework_revision,omitempty"`
}

type creativeVisualModelReworkFinding struct {
	Code      string `json:"code"`
	SizeKey   string `json:"size_key"`
	Diagnosis string `json:"diagnosis"`
}

const creativeVisualModelReworkMaxAttempts = 2

// This is intentionally stricter than the normal contrast target. Ordinary
// qc_risk evidence still needs visual review, but this severity cannot be
// released on a single model assertion that the Prime text looks readable.
const creativePrimeCriticalRelativeLuminanceContrast = 1.25

const defaultCreativeQCAttempt = 1

const creativeQCOutcomeDeliveredWithRisk = "delivered_with_qc_risk"

type creativeOrderItemAdoptionInput struct {
	VariantID          string `json:"variant_id"`
	QCRiskAcknowledged bool   `json:"qc_risk_acknowledged"`
	QCRiskReason       string `json:"qc_risk_reason"`
}

const creativeCandidatePreviewSize = "1080x1080"

var standardCreativeAssetSizes = []string{creativeCandidatePreviewSize, "1200x628", "800x1000"}

type creativeDeliveryScope struct {
	TargetSize    string   `json:"target_size"`
	ExpectedSizes []string `json:"expected_sizes"`
}

type creativeOrderCapabilityBinding struct {
	Capability        string
	SnapshotField     string
	PoolSnapshotField string
	AllowMultiple     bool
}

var creativeOrderCapabilityBindings = []creativeOrderCapabilityBinding{
	{Capability: "generation_plan", SnapshotField: "planner_agent_id"},
	{Capability: "image_edit", SnapshotField: "producer_agent_id", PoolSnapshotField: "producer_agent_ids", AllowMultiple: true},
	{Capability: "quality_control", SnapshotField: "reviewer_agent_id"},
}

type creativeOrderSquadValidationError struct {
	message string
}

func (e *creativeOrderSquadValidationError) Error() string {
	return e.message
}

func (h *Handler) CreateCreativeSourceAnalysis(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var input creativeSourceAnalysisInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid source analysis")
		return
	}
	input, err := normalizeCreativeSourceAnalysis(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, input.CandidateID, "candidate_id")
	if !ok {
		return
	}
	evidenceID, ok := optionalUUIDOrBadRequest(w, input.TriggerEvidenceReference, "trigger_evidence_ref_id")
	if !ok {
		return
	}
	if evidenceID.Valid {
		var crawlRunCandidateExists bool
		if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM creative_material_crawl_run_candidate rc
  JOIN creative_material_crawl_run cr ON cr.id = rc.run_id
  WHERE rc.run_id = $1 AND rc.candidate_id = $2 AND rc.workspace_id = $3
    AND cr.workspace_id = $3
)`, evidenceID, candidateID, workspaceID).Scan(&crawlRunCandidateExists); err != nil || !crawlRunCandidateExists {
			writeError(w, http.StatusUnprocessableEntity, "trigger_evidence_ref_id must reference this candidate in a crawl run in this workspace")
			return
		}
	}
	var candidateExists bool
	if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2)`, candidateID, workspaceID).Scan(&candidateExists); err != nil || !candidateExists {
		writeError(w, http.StatusUnprocessableEntity, "candidate does not belong to this workspace")
		return
	}
	analysis, err := scanCreativeSourceAnalysis(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_source_analysis (
  workspace_id, candidate_id, analysis_version, status, summary, result, error_code, error_message,
  trigger_evidence_kind, trigger_evidence_ref_id, completed_at
) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,CASE WHEN $4 IN ('completed','failed') THEN now() ELSE NULL END)
ON CONFLICT (candidate_id, analysis_version) DO UPDATE SET
  status = EXCLUDED.status, summary = EXCLUDED.summary, result = EXCLUDED.result,
  error_code = EXCLUDED.error_code, error_message = EXCLUDED.error_message,
  trigger_evidence_kind = EXCLUDED.trigger_evidence_kind,
  trigger_evidence_ref_id = EXCLUDED.trigger_evidence_ref_id,
  completed_at = CASE WHEN EXCLUDED.status IN ('completed','failed') THEN now() ELSE NULL END
RETURNING id::text, workspace_id::text, candidate_id::text, analysis_version, status, summary, result::text,
  error_code, error_message, trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''),
  created_at::text, COALESCE(completed_at::text, '')
`, workspaceID, candidateID, input.AnalysisVersion, input.Status, input.Summary, input.Result,
		input.ErrorCode, input.ErrorMessage, input.TriggerEvidenceKind, evidenceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save source analysis")
		return
	}
	if _, err := h.DB.Exec(r.Context(), `
UPDATE creative_material_issue_candidate
SET analysis_status = $3, analysis_error = $4,
    analyzed_at = CASE WHEN $3 IN ('completed', 'failed') THEN now() ELSE NULL END,
    updated_at = now()
WHERE workspace_id = $1 AND candidate_id = $2
  AND ($5::uuid IS NULL OR source_run_id = $5)
`, workspaceID, candidateID, input.Status, input.ErrorMessage, evidenceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update source analysis state")
		return
	}
	if evidenceID.Valid {
		if _, err := h.DB.Exec(r.Context(), `
UPDATE creative_material_crawl_run_candidate
SET analysis_status = $3, analysis_error = $4,
    analyzed_at = CASE WHEN $3 IN ('completed', 'failed') THEN now() ELSE NULL END,
    updated_at = now()
WHERE run_id = $1 AND candidate_id = $2 AND workspace_id = $5
`, evidenceID, candidateID, input.Status, input.ErrorMessage, workspaceID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update crawl run analysis state")
			return
		}
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "source_analysis", "candidate_id": input.CandidateID, "crawl_run_id": input.TriggerEvidenceReference,
	})
	writeJSON(w, http.StatusCreated, analysis)
}

func (h *Handler) ListCreativeSourceAnalyses(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	candidateRaw := strings.TrimSpace(r.URL.Query().Get("candidate_id"))
	var candidateID any
	if candidateRaw != "" {
		parsed, valid := parseUUIDOrBadRequest(w, candidateRaw, "candidate_id")
		if !valid {
			return
		}
		candidateID = parsed
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, workspace_id::text, candidate_id::text, analysis_version, status, summary, result::text,
  error_code, error_message, trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''),
  created_at::text, COALESCE(completed_at::text, '')
FROM creative_source_analysis
WHERE workspace_id = $1 AND ($2::uuid IS NULL OR candidate_id = $2)
ORDER BY candidate_id, analysis_version DESC
LIMIT 200
`, workspaceID, candidateID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list source analyses")
		return
	}
	defer rows.Close()
	items := []creativeSourceAnalysisResponse{}
	for rows.Next() {
		item, err := scanCreativeSourceAnalysis(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read source analyses")
			return
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"analyses": items})
}

func (h *Handler) CreateCreativeOrder(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var input creativeOrderInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order")
		return
	}
	input, err := normalizeCreativeOrder(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	issueID, ok := optionalUUIDOrBadRequest(w, input.IssueID, "issue_id")
	if !ok {
		return
	}
	evidenceID, ok := optionalUUIDOrBadRequest(w, input.TriggerEvidenceReference, "trigger_evidence_ref_id")
	if !ok {
		return
	}
	if issueID.Valid {
		var issueExists bool
		if err := h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM issue WHERE id = $1 AND workspace_id = $2)`, issueID, workspaceID).Scan(&issueExists); err != nil || !issueExists {
			writeError(w, http.StatusUnprocessableEntity, "issue does not belong to this workspace")
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative order")
		return
	}
	defer tx.Rollback(r.Context())
	if input.SubmissionKey != "" {
		if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, uuidToString(workspaceID)+":"+input.SubmissionKey); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to recover creative order")
			return
		}
		var existingID string
		err := tx.QueryRow(r.Context(), `SELECT id::text FROM creative_order WHERE workspace_id = $1 AND submission_key = $2`, workspaceID, input.SubmissionKey).Scan(&existingID)
		if err == nil {
			existing, loadErr := scanCreativeOrder(tx.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
FROM creative_order WHERE id = $1`, parseUUID(existingID)))
			if loadErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to recover creative order")
				return
			}
			if err := tx.Commit(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to recover creative order")
				return
			}
			if err := h.loadCreativeOrderWorkflowState(r, &existing); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to recover creative order")
				return
			}
			writeJSON(w, http.StatusOK, existing)
			return
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to recover creative order")
			return
		}
	}
	input.InputSnapshot, err = freezeCreativeOrderPipelineVersion(input.InputSnapshot, creativePipelineCandidateV1)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	input.InputSnapshot, err = imagemodel.Freeze(input.InputSnapshot)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	squadIDText, err := creativeOrderSnapshotSquadID(input.InputSnapshot)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, squadIDText, "input_snapshot.squad_snapshot.squad_id")
	if !ok {
		return
	}
	input.InputSnapshot, err = freezeCreativeOrderSquadSnapshot(r.Context(), tx, workspaceID, squadID, input.InputSnapshot, creativeOrderCapabilityBindings)
	if err != nil {
		var validationErr *creativeOrderSquadValidationError
		if errors.As(err, &validationErr) {
			writeError(w, http.StatusUnprocessableEntity, validationErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "failed to resolve creative order squad")
		}
		return
	}
	if err := freezeCreativeCopyLibraryOrder(r.Context(), tx, workspaceID, &input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateCustomCreativeOrderCopyFacts(r.Context(), tx, workspaceID, input.InputSnapshot, input.Items); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	input.InputSnapshot, err = h.freezeCreativeOrderInputAttachments(r.Context(), tx, workspaceID, input.InputSnapshot, input.Items)
	if err != nil {
		var attachmentErr *creativeOrderAttachmentValidationError
		if errors.As(err, &attachmentErr) {
			writeError(w, http.StatusUnprocessableEntity, attachmentErr.Error())
		} else {
			writeError(w, http.StatusInternalServerError, "failed to freeze creative order attachments")
		}
		return
	}
	order, err := scanCreativeOrder(tx.QueryRow(r.Context(), `
INSERT INTO creative_order (workspace_id, issue_id, status, input_snapshot, trigger_evidence_kind, trigger_evidence_ref_id, created_by, submission_key)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7,$8)
RETURNING id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
`, workspaceID, issueID, input.Status, input.InputSnapshot, input.TriggerEvidenceKind, evidenceID, userID, input.SubmissionKey))
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "failed to create creative order")
		return
	}
	for _, item := range input.Items {
		candidateID, valid := optionalUUIDOrBadRequest(w, item.CandidateID, "candidate_id")
		if !valid {
			return
		}
		analysisID, valid := optionalUUIDOrBadRequest(w, item.SourceAnalysisID, "source_analysis_id")
		if !valid {
			return
		}
		copyLibraryID, valid := optionalUUIDOrBadRequest(w, item.CopyLibraryID, "copy_library_id")
		if !valid {
			return
		}
		var referencesValid bool
		if item.SourceKind == "material" {
			if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2)
  AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM creative_source_analysis WHERE id = $3 AND candidate_id = $1 AND workspace_id = $2))
`, candidateID, workspaceID, analysisID).Scan(&referencesValid); err != nil || !referencesValid {
				writeError(w, http.StatusUnprocessableEntity, "order item references do not belong to this workspace")
				return
			}
		}
		if issueID.Valid && candidateID.Valid {
			if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_material_issue_candidate (
  issue_id, candidate_id, workspace_id, status, selected_by, selected_at
) VALUES ($1, $2, $3, 'selected', $4, now())
ON CONFLICT (issue_id, candidate_id) DO UPDATE SET
  status = 'selected',
  selected_by = EXCLUDED.selected_by,
  selected_at = EXCLUDED.selected_at,
  updated_at = now()
`, issueID, candidateID, workspaceID, userID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to link creative material candidate to issue")
				return
			}
		}
		if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, source_analysis_id, copy_snapshot, direction, source_kind, copy_library_id)
VALUES ($1,$2,$3,$4::jsonb,$5,$6,$7)
`, order.ID, candidateID, analysisID, item.CopySnapshot, item.Direction, item.SourceKind, copyLibraryID); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "failed to create creative order item")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create creative order")
		return
	}
	if err := h.loadCreativeOrderWorkflowState(r, &order); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to derive creative order status")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": order.ID})
	writeJSON(w, http.StatusCreated, order)
}

func creativeOrderSnapshotSquadID(raw json.RawMessage) (string, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return "", &creativeOrderSquadValidationError{message: "input_snapshot must be an object"}
	}
	var squadSnapshot map[string]json.RawMessage
	if err := json.Unmarshal(snapshot["squad_snapshot"], &squadSnapshot); err != nil {
		return "", &creativeOrderSquadValidationError{message: "input_snapshot.squad_snapshot is required"}
	}
	var squadID string
	if err := json.Unmarshal(squadSnapshot["squad_id"], &squadID); err != nil || strings.TrimSpace(squadID) == "" {
		return "", &creativeOrderSquadValidationError{message: "input_snapshot.squad_snapshot.squad_id is required"}
	}
	return strings.TrimSpace(squadID), nil
}

func freezeCreativeOrderSquadSnapshot(ctx context.Context, tx pgx.Tx, workspaceID, squadID pgtype.UUID, raw json.RawMessage, bindings []creativeOrderCapabilityBinding) (json.RawMessage, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, &creativeOrderSquadValidationError{message: "input_snapshot must be an object"}
	}
	var squadSnapshot map[string]json.RawMessage
	if err := json.Unmarshal(snapshot["squad_snapshot"], &squadSnapshot); err != nil {
		return nil, &creativeOrderSquadValidationError{message: "input_snapshot.squad_snapshot is required"}
	}
	var leaderID pgtype.UUID
	var leaderCapable bool
	err := tx.QueryRow(ctx, `
SELECT s.leader_id, EXISTS(
  SELECT 1
  FROM agent_skill leader_binding
  JOIN skill leader_skill ON leader_skill.id = leader_binding.skill_id
  WHERE leader_binding.agent_id = s.leader_id
    AND leader_binding.enabled
    AND leader_skill.workspace_id = s.workspace_id
    AND leader_skill.config->>'kind' = 'creative_role'
    AND leader_skill.config->>'capability' = 'creative_leadership'
)
FROM squad s
JOIN agent leader ON leader.id = s.leader_id AND leader.workspace_id = s.workspace_id AND leader.archived_at IS NULL
WHERE s.id = $1 AND s.workspace_id = $2 AND s.archived_at IS NULL
`, squadID, workspaceID).Scan(&leaderID, &leaderCapable)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, &creativeOrderSquadValidationError{message: "squad does not belong to this workspace or is archived"}
	}
	if err != nil {
		return nil, err
	}
	if !leaderCapable {
		return nil, &creativeOrderSquadValidationError{message: "squad leader must have creative_leadership capability"}
	}

	capabilities := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		capabilities = append(capabilities, binding.Capability)
	}
	rows, err := tx.Query(ctx, `
SELECT
  role_skill.config->>'capability',
  sm.member_id::text,
  min(CASE WHEN factory.role_agents->>CASE role_skill.config->>'capability'
    WHEN 'creative_leadership' THEN 'leadership'
    WHEN 'reference_analysis' THEN 'reference_analysis'
    WHEN 'material_collection' THEN 'collection'
    WHEN 'generation_plan' THEN 'generation_plan'
    WHEN 'image_edit' THEN 'image_edit'
    WHEN 'direct_image_edit' THEN 'direct_image_edit'
    WHEN 'quality_control' THEN 'quality_control'
    WHEN 'crawl_diagnosis' THEN 'diagnostics'
    ELSE ''
  END = sm.member_id::text THEN 0 ELSE 1 END)
FROM squad_member sm
JOIN agent role_agent ON role_agent.id = sm.member_id AND role_agent.workspace_id = $2 AND role_agent.archived_at IS NULL
JOIN agent_skill role_binding ON role_binding.agent_id = role_agent.id AND role_binding.enabled
JOIN skill role_skill ON role_skill.id = role_binding.skill_id AND role_skill.workspace_id = $2
LEFT JOIN creative_factory_installation factory
  ON factory.workspace_id = $2
WHERE sm.squad_id = $1
  AND sm.member_type = 'agent'
  AND role_skill.config->>'kind' = 'creative_role'
  AND role_skill.config->>'capability' = ANY($3::text[])
GROUP BY role_skill.config->>'capability', sm.member_id::text
ORDER BY role_skill.config->>'capability', min(CASE WHEN factory.role_agents->>CASE role_skill.config->>'capability'
  WHEN 'creative_leadership' THEN 'leadership'
  WHEN 'reference_analysis' THEN 'reference_analysis'
  WHEN 'material_collection' THEN 'collection'
  WHEN 'generation_plan' THEN 'generation_plan'
  WHEN 'image_edit' THEN 'image_edit'
  WHEN 'direct_image_edit' THEN 'direct_image_edit'
  WHEN 'quality_control' THEN 'quality_control'
  WHEN 'crawl_diagnosis' THEN 'diagnostics'
  ELSE ''
END = sm.member_id::text THEN 0 ELSE 1 END), sm.member_id::text
`, squadID, workspaceID, capabilities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resolved := make(map[string][]string, len(bindings))
	for rows.Next() {
		var capability, agentID string
		var installationPriority int
		if err := rows.Scan(&capability, &agentID, &installationPriority); err != nil {
			return nil, err
		}
		resolved[capability] = append(resolved[capability], agentID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	setSnapshotString := func(field, value string) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		squadSnapshot[field] = encoded
		return nil
	}
	if err := setSnapshotString("squad_id", uuidToString(squadID)); err != nil {
		return nil, err
	}
	if err := setSnapshotString("leader_agent_id", uuidToString(leaderID)); err != nil {
		return nil, err
	}
	for _, binding := range bindings {
		agentIDs := resolved[binding.Capability]
		if len(agentIDs) == 0 {
			return nil, &creativeOrderSquadValidationError{message: "squad is missing " + binding.Capability + " capability"}
		}
		if len(agentIDs) > 1 && !binding.AllowMultiple {
			return nil, &creativeOrderSquadValidationError{message: "squad has multiple agents with " + binding.Capability + " capability"}
		}
		if err := setSnapshotString(binding.SnapshotField, agentIDs[0]); err != nil {
			return nil, err
		}
		if binding.PoolSnapshotField != "" {
			encoded, err := json.Marshal(agentIDs)
			if err != nil {
				return nil, err
			}
			squadSnapshot[binding.PoolSnapshotField] = encoded
		}
	}
	encodedSquad, err := json.Marshal(squadSnapshot)
	if err != nil {
		return nil, err
	}
	snapshot["squad_snapshot"] = encodedSquad
	return json.Marshal(snapshot)
}

var (
	creativeFinancialCurrencyPattern = regexp.MustCompile(`(?i)\b(?:Rp\.?|IDR)\s*\d+(?:[.,]\d+)*`)
	creativeFinancialPercentPattern  = regexp.MustCompile(`\b\d+(?:[.,]\d+)?\s*%`)
	creativeFinancialTermPattern     = regexp.MustCompile(`(?i)\b\d+(?:\s*-\s*\d+)?\s*(?:bulan|hari|tahun)\b`)
	creativeFinancialNumberPattern   = regexp.MustCompile(`(?i)\b(?:limit|pinjaman|dana|jumlah|cicilan|angsuran|tenor|bunga|interest|biaya|fee)\D{0,24}(\d{1,3}(?:[.,]\d{3})+|\d+)(?:\s*(?:juta|ribu|miliar))?`)
	creativeDigitsPattern            = regexp.MustCompile(`\D`)
	creativeNumericFactValuePattern  = regexp.MustCompile(`^\d+(?:[.,]\d+)*$`)
)

type creativeFinancialToken struct {
	key     string
	display string
}

type creativeOrderCopySnapshotFragment struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Role string `json:"role"`
	Text string `json:"text"`
}

type creativeOrderCopySnapshotFact struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	CopyText string `json:"copy_text"`
	Source   string `json:"source"`
}

type creativeOrderCopySnapshotTextReplacement struct {
	BlockID         string `json:"block_id"`
	ReplacementText string `json:"replacement_text"`
	SourceKind      string `json:"source_kind"`
	Status          string `json:"status"`
}

type creativeOrderCopySnapshotPreAdaptation struct {
	SourceAnalysisID string                                     `json:"source_analysis_id"`
	TextReplacements []creativeOrderCopySnapshotTextReplacement `json:"text_replacements"`
	NumericLayouts   []struct {
		RenderInstruction string `json:"render_instruction"`
	} `json:"numeric_layouts"`
}

type creativeOrderCopySnapshot struct {
	SchemaVersion  int                                    `json:"schema_version"`
	ID             string                                 `json:"id"`
	LibraryID      string                                 `json:"library_id"`
	LibraryVersion int                                    `json:"library_version"`
	CompositionID  string                                 `json:"composition_id"`
	CompositionKey string                                 `json:"composition_key"`
	RecipeID       string                                 `json:"recipe_id"`
	RecipeKey      string                                 `json:"recipe_key"`
	CreativeType   string                                 `json:"creative_type"`
	Status         string                                 `json:"status"`
	Headline       string                                 `json:"headline"`
	Subheadline    string                                 `json:"subheadline"`
	Benefit        string                                 `json:"benefit"`
	Supporting     string                                 `json:"supporting"`
	CTA            string                                 `json:"cta"`
	LegalText      string                                 `json:"legal_text"`
	Fragments      []creativeOrderCopySnapshotFragment    `json:"fragments"`
	ProductFacts   []creativeOrderCopySnapshotFact        `json:"product_facts"`
	PreAdaptation  creativeOrderCopySnapshotPreAdaptation `json:"pre_adaptation"`
}

func validateCustomCreativeOrderCopyFacts(ctx context.Context, q dbExecutor, workspaceID pgtype.UUID, inputSnapshot json.RawMessage, items []creativeOrderItemInput) error {
	snapshots := make([]creativeOrderCopySnapshot, 0, len(items))
	requiresLibrary := false
	for index, item := range items {
		if item.SourceKind == "copy_library" {
			continue
		}
		var snapshot creativeOrderCopySnapshot
		if err := json.Unmarshal(item.CopySnapshot, &snapshot); err != nil {
			return errors.New("copy_snapshot must be valid JSON")
		}
		if snapshot.SchemaVersion != 3 {
			return fmt.Errorf("copy_snapshot %d must use schema_version 3", index+1)
		}
		if snapshot.CreativeType != "num" && snapshot.CreativeType != "repayment_plan" {
			return fmt.Errorf("copy_snapshot %d has invalid creative_type", index+1)
		}
		switch snapshot.Status {
		case "approved", "model_pre_adapted":
			requiresLibrary = true
		case "user_custom":
			// A user-authored snapshot is an explicit per-order decision. It can
			// intentionally diverge from the reusable copy library, including
			// financial values, and remains auditable in the frozen snapshot.
		default:
			return fmt.Errorf("copy_snapshot %d has invalid status", index+1)
		}
		snapshots = append(snapshots, snapshot)
	}
	if !requiresLibrary {
		return nil
	}
	libraryID, err := creativeOrderCopyLibraryID(inputSnapshot)
	if err != nil {
		return err
	}
	library, err := loadPublishedCreativeResource(ctx, q, workspaceID, libraryID, "copy_library")
	if err != nil {
		return errors.New("creative order requires the market pack to bind a published copy library")
	}
	var config composableCopyLibraryConfig
	if err := json.Unmarshal(library.Config, &config); err != nil {
		return errors.New("published copy library configuration is invalid")
	}
	if err := validateComposableCopyLibraryConfig(library.Config); err != nil {
		return errors.New("published copy library configuration is invalid: " + err.Error())
	}
	for index, snapshot := range snapshots {
		if snapshot.Status == "approved" {
			if err := validateApprovedCreativeOrderCopySnapshot(snapshot, library, config); err != nil {
				return fmt.Errorf("copy_snapshot %d is not a valid frozen published composition: %w", index+1, err)
			}
		}
		if snapshot.Status == "model_pre_adapted" && (snapshot.LibraryID != library.ID || snapshot.LibraryVersion != library.PublishedVersion) {
			return fmt.Errorf("copy_snapshot %d model pre-adaptation does not match the published copy library", index+1)
		}
	}
	return nil
}

func validateApprovedCreativeOrderCopySnapshot(snapshot creativeOrderCopySnapshot, library creativeResourceResponse, config composableCopyLibraryConfig) error {
	if snapshot.LibraryID != library.ID || snapshot.LibraryVersion != library.PublishedVersion {
		return errors.New("library id or published version does not match")
	}
	if strings.TrimSpace(snapshot.CompositionID) != "" {
		return validateAtomicCreativeOrderCopySnapshot(snapshot, config)
	}
	recipeIndex := -1
	for index := range config.Recipes {
		if config.Recipes[index].Status == "approved" && strings.TrimSpace(config.Recipes[index].ID) == snapshot.RecipeID {
			recipeIndex = index
			break
		}
	}
	if recipeIndex < 0 {
		return errors.New("recipe is missing or not approved")
	}
	recipe := config.Recipes[recipeIndex]
	if snapshot.ID != strings.TrimSpace(recipe.ID) || snapshot.RecipeKey != strings.TrimSpace(recipe.Key) || snapshot.CreativeType != recipe.CreativeType {
		return errors.New("recipe identity or creative type does not match")
	}

	fragmentsByID := make(map[string]struct {
		key  string
		role string
		text string
	}, len(config.Fragments))
	for _, fragment := range config.Fragments {
		if fragment.Status != "approved" {
			continue
		}
		fragmentsByID[strings.TrimSpace(fragment.ID)] = struct {
			key  string
			role string
			text string
		}{key: strings.TrimSpace(fragment.Key), role: fragment.Role, text: strings.TrimSpace(fragment.Text)}
	}
	factsByKey := make(map[string]creativeOrderCopySnapshotFact, len(config.ProductFacts))
	for _, fact := range config.ProductFacts {
		if fact.Status != "approved" {
			continue
		}
		key := strings.TrimSpace(fact.Key)
		factsByKey[key] = creativeOrderCopySnapshotFact{
			Key: key, Label: strings.TrimSpace(fact.Label), Value: strings.TrimSpace(fact.Value),
			CopyText: strings.TrimSpace(fact.CopyText), Source: strings.TrimSpace(fact.Source),
		}
	}

	roles := []string{"headline", "subheadline", "benefit", "supporting", "cta", "legal"}
	expectedText := map[string]string{}
	expectedFragments := make([]creativeOrderCopySnapshotFragment, 0)
	expectedFacts := make([]creativeOrderCopySnapshotFact, 0)
	seenFacts := map[string]bool{}
	for _, role := range roles {
		lines := make([]string, 0)
		for _, fragmentID := range recipe.FragmentIDs[role] {
			fragment, ok := fragmentsByID[strings.TrimSpace(fragmentID)]
			if !ok || fragment.role != role {
				return fmt.Errorf("recipe fragment %q is unavailable", fragmentID)
			}
			missingFact := ""
			resolved := creativeCopyFactReferencePattern.ReplaceAllStringFunc(fragment.text, func(reference string) string {
				match := creativeCopyFactReferencePattern.FindStringSubmatch(reference)
				if len(match) != 3 {
					return ""
				}
				fact, ok := factsByKey[match[1]]
				if !ok {
					missingFact = match[1]
					return ""
				}
				if !seenFacts[fact.Key] {
					seenFacts[fact.Key] = true
					expectedFacts = append(expectedFacts, fact)
				}
				if match[2] == "value" {
					return fact.Value
				}
				return fact.CopyText
			})
			if missingFact != "" {
				return fmt.Errorf("recipe references missing fact %q", missingFact)
			}
			expectedFragments = append(expectedFragments, creativeOrderCopySnapshotFragment{
				ID: strings.TrimSpace(fragmentID), Key: fragment.key, Role: role, Text: resolved,
			})
			if resolved != "" {
				lines = append(lines, resolved)
			}
		}
		expectedText[role] = strings.Join(lines, "\n")
	}
	if snapshot.Headline != expectedText["headline"] || snapshot.Subheadline != expectedText["subheadline"] ||
		snapshot.Benefit != expectedText["benefit"] || snapshot.Supporting != expectedText["supporting"] ||
		snapshot.CTA != expectedText["cta"] || snapshot.LegalText != expectedText["legal"] {
		return errors.New("assembled visible copy does not match the published recipe")
	}
	if !equalCreativeCopySnapshotFragments(snapshot.Fragments, expectedFragments) {
		return errors.New("fragment evidence does not match the published recipe")
	}
	if !equalCreativeCopySnapshotFacts(snapshot.ProductFacts, expectedFacts) {
		return errors.New("product fact evidence does not match the published recipe")
	}
	return nil
}

func validateAtomicCreativeOrderCopySnapshot(snapshot creativeOrderCopySnapshot, config composableCopyLibraryConfig) error {
	if snapshot.ID != strings.TrimSpace(snapshot.CompositionID) || strings.TrimSpace(snapshot.CompositionKey) == "" {
		return errors.New("composition identity is incomplete")
	}
	type publishedFragment struct {
		key   string
		role  string
		text  string
		types map[string]bool
	}
	fragmentsByID := make(map[string]publishedFragment, len(config.Fragments))
	for _, fragment := range config.Fragments {
		if fragment.Status != "approved" {
			continue
		}
		types := map[string]bool{}
		for _, creativeType := range fragment.CreativeTypes {
			types[creativeType] = true
		}
		fragmentsByID[strings.TrimSpace(fragment.ID)] = publishedFragment{
			key: strings.TrimSpace(fragment.Key), role: fragment.Role, text: strings.TrimSpace(fragment.Text), types: types,
		}
	}
	factsByKey := make(map[string]creativeOrderCopySnapshotFact, len(config.ProductFacts))
	for _, fact := range config.ProductFacts {
		if fact.Status != "approved" {
			continue
		}
		key := strings.TrimSpace(fact.Key)
		factsByKey[key] = creativeOrderCopySnapshotFact{
			Key: key, Label: strings.TrimSpace(fact.Label), Value: strings.TrimSpace(fact.Value),
			CopyText: strings.TrimSpace(fact.CopyText), Source: strings.TrimSpace(fact.Source),
		}
	}
	roles := []string{"headline", "subheadline", "benefit", "supporting", "cta", "legal"}
	linesByRole := map[string][]string{}
	expectedFragments := make([]creativeOrderCopySnapshotFragment, 0, len(snapshot.Fragments))
	expectedFacts := make([]creativeOrderCopySnapshotFact, 0)
	seenFragments := map[string]bool{}
	seenFacts := map[string]bool{}
	for _, evidence := range snapshot.Fragments {
		fragmentID := strings.TrimSpace(evidence.ID)
		fragment, ok := fragmentsByID[fragmentID]
		if !ok || seenFragments[fragmentID] {
			return fmt.Errorf("composition fragment %q is unavailable or duplicated", fragmentID)
		}
		seenFragments[fragmentID] = true
		if fragment.role != evidence.Role || fragment.key != evidence.Key || !fragment.types[snapshot.CreativeType] {
			return fmt.Errorf("composition fragment %q role, key, or creative type does not match", fragmentID)
		}
		missingFact := ""
		resolved := creativeCopyFactReferencePattern.ReplaceAllStringFunc(fragment.text, func(reference string) string {
			match := creativeCopyFactReferencePattern.FindStringSubmatch(reference)
			if len(match) != 3 {
				return ""
			}
			fact, exists := factsByKey[match[1]]
			if !exists {
				missingFact = match[1]
				return ""
			}
			if !seenFacts[fact.Key] {
				seenFacts[fact.Key] = true
				expectedFacts = append(expectedFacts, fact)
			}
			if match[2] == "value" {
				return fact.Value
			}
			return fact.CopyText
		})
		if missingFact != "" {
			return fmt.Errorf("composition references missing fact %q", missingFact)
		}
		if resolved != evidence.Text {
			return fmt.Errorf("composition fragment %q resolved text was changed", fragmentID)
		}
		expectedFragments = append(expectedFragments, creativeOrderCopySnapshotFragment{ID: fragmentID, Key: fragment.key, Role: fragment.role, Text: resolved})
		if resolved != "" {
			linesByRole[fragment.role] = append(linesByRole[fragment.role], resolved)
		}
	}
	if len(expectedFragments) == 0 {
		return errors.New("composition must contain approved fragments")
	}
	expectedText := map[string]string{}
	for _, role := range roles {
		expectedText[role] = strings.Join(linesByRole[role], "\n")
	}
	if snapshot.Headline != expectedText["headline"] || snapshot.Subheadline != expectedText["subheadline"] ||
		snapshot.Benefit != expectedText["benefit"] || snapshot.Supporting != expectedText["supporting"] ||
		snapshot.CTA != expectedText["cta"] || snapshot.LegalText != expectedText["legal"] {
		return errors.New("assembled visible copy does not match the published atoms")
	}
	if !equalCreativeCopySnapshotFragments(snapshot.Fragments, expectedFragments) {
		return errors.New("fragment evidence does not match the published atoms")
	}
	if !equalCreativeCopySnapshotFacts(snapshot.ProductFacts, expectedFacts) {
		return errors.New("product fact evidence does not match the published atoms")
	}
	return nil
}

func equalCreativeCopySnapshotFragments(left, right []creativeOrderCopySnapshotFragment) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalCreativeCopySnapshotFacts(left, right []creativeOrderCopySnapshotFact) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func creativeOrderCopyLibraryID(raw json.RawMessage) (pgtype.UUID, error) {
	var snapshot struct {
		MarketPack struct {
			Config struct {
				CopyLibraryID string `json:"copy_library_id"`
			} `json:"config"`
		} `json:"market_pack"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return pgtype.UUID{}, errors.New("input_snapshot must be valid JSON")
	}
	copyLibraryID := strings.TrimSpace(snapshot.MarketPack.Config.CopyLibraryID)
	if copyLibraryID == "" {
		return pgtype.UUID{}, errors.New("creative order requires the market pack to bind a published copy library")
	}
	id, err := parseUUIDString(copyLibraryID)
	if err != nil {
		return pgtype.UUID{}, errors.New("market pack copy library is invalid")
	}
	return id, nil
}

func creativeFinancialTokens(value string) []creativeFinancialToken {
	tokens := []creativeFinancialToken{}
	type tokenRange struct{ start, end int }
	protectedRanges := []tokenRange{}
	add := func(kind, key, display string) {
		if strings.TrimSpace(key) != "" {
			tokens = append(tokens, creativeFinancialToken{key: kind + ":" + strings.TrimSpace(key), display: strings.TrimSpace(display)})
		}
	}
	for _, indexes := range creativeFinancialCurrencyPattern.FindAllStringIndex(value, -1) {
		match := value[indexes[0]:indexes[1]]
		add("currency", creativeDigitsPattern.ReplaceAllString(match, ""), match)
		protectedRanges = append(protectedRanges, tokenRange{start: indexes[0], end: indexes[1]})
	}
	for _, indexes := range creativeFinancialPercentPattern.FindAllStringIndex(value, -1) {
		match := value[indexes[0]:indexes[1]]
		add("percent", strings.ReplaceAll(strings.ReplaceAll(match, " ", ""), ",", "."), match)
		protectedRanges = append(protectedRanges, tokenRange{start: indexes[0], end: indexes[1]})
	}
	for _, indexes := range creativeFinancialTermPattern.FindAllStringIndex(value, -1) {
		match := value[indexes[0]:indexes[1]]
		add("term", strings.ToLower(strings.ReplaceAll(match, " ", "")), match)
		protectedRanges = append(protectedRanges, tokenRange{start: indexes[0], end: indexes[1]})
	}
	for _, indexes := range creativeFinancialNumberPattern.FindAllStringSubmatchIndex(value, -1) {
		if len(indexes) < 4 || indexes[2] < 0 || indexes[3] < 0 {
			continue
		}
		overlapsProtected := false
		for _, protected := range protectedRanges {
			if indexes[0] < protected.end && protected.start < indexes[1] {
				overlapsProtected = true
				break
			}
		}
		if overlapsProtected {
			continue
		}
		add("financial_number", creativeDigitsPattern.ReplaceAllString(value[indexes[2]:indexes[3]], ""), value[indexes[0]:indexes[1]])
	}
	return tokens
}

func (h *Handler) ListCreativeOrders(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
FROM creative_order WHERE workspace_id = $1 ORDER BY created_at DESC, id DESC LIMIT 100
`, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list creative orders")
		return
	}
	defer rows.Close()
	orders := []creativeOrderResponse{}
	for rows.Next() {
		order, err := scanCreativeOrder(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative orders")
			return
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read creative orders")
		return
	}
	rows.Close()
	if err := h.loadCreativeOrderWorkflowStates(r.Context(), orders); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to derive creative order status")
		return
	}
	ids := make([]pgtype.UUID, 0, len(orders))
	for _, order := range orders {
		ids = append(ids, parseUUID(order.ID))
	}
	items, err := h.listCreativeOrderListItemsBatch(r, ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order list summary")
		return
	}
	byOrder := make(map[string][]creativeOrderItemResponse)
	for _, item := range items {
		byOrder[item.OrderID] = append(byOrder[item.OrderID], item)
	}
	for i := range orders {
		orders[i].Items = byOrder[orders[i].ID]
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

func (h *Handler) GetCreativeOrder(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	order, err := scanCreativeOrder(h.DB.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
FROM creative_order WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order")
		return
	}
	if err := h.loadCreativeOrderWorkflowState(r, &order); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to derive creative order status")
		return
	}
	order.Items, err = h.listCreativeOrderItems(r, orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order items")
		return
	}
	order.Recoveries, err = h.listCreativeOrderRecoveries(r.Context(), orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative recovery history")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// GetCreativeOrderQCContext returns the one variant a visual QC task may
// inspect. This is task-token-only because it is also used as the CLI's local
// write preflight for qc-put and qc-finalize.
func (h *Handler) GetCreativeOrderQCContext(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	task, taskContext, ok := h.creativeQCVisualContextTask(w, r)
	if !ok {
		return
	}
	if taskContext.CreativeOrderID != uuidToString(orderID) {
		writeError(w, http.StatusForbidden, "task context is not authorized for this creative order")
		return
	}
	variantID, err := util.ParseUUID(taskContext.VariantID)
	if err != nil {
		writeError(w, http.StatusForbidden, "creative QC task context is invalid")
		return
	}

	var itemID, variantKey, brief, copySnapshot, triggerKind, inputSnapshot string
	var revision int
	err = h.DB.QueryRow(r.Context(), `
SELECT item.id::text, variant.variant_key, variant.brief::text, item.copy_snapshot::text,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.revision
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
`, parseUUID(variantID.String()), orderID, workspaceID).Scan(
		&itemID, &variantKey, &brief, &copySnapshot, &triggerKind, &inputSnapshot, &revision,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "task-bound creative QC target was not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task-bound creative QC target")
		return
	}
	if taskContext.CreativeOrderItemID != "" && taskContext.CreativeOrderItemID != itemID {
		writeError(w, http.StatusForbidden, "creative QC task item does not match the target")
		return
	}
	if revision != taskContext.Revision {
		writeError(w, http.StatusConflict, "creative QC task revision is no longer current")
		return
	}
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	qcSizes, err := creativeQCTaskExpectedSizes(task)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	for _, size := range qcSizes {
		if !creativeSizeIsExpected(size, expectedSizes) {
			writeError(w, http.StatusForbidden, "creative QC task scope is outside the delivery contract")
			return
		}
	}
	assets, err := h.listCreativeOrderAssets(r, parseUUID(variantID.String()))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load task-bound Prime assets")
		return
	}
	expected := make(map[string]struct{}, len(qcSizes))
	for _, size := range qcSizes {
		expected[size] = struct{}{}
	}
	primeAssets := make([]creativeOrderAssetResponse, 0, len(qcSizes))
	for _, asset := range assets {
		if asset.Revision == revision && asset.Stage == "primed" && asset.Status == "completed" && asset.AttachmentID != "" {
			if _, allowed := expected[asset.SizeKey]; allowed {
				primeAssets = append(primeAssets, asset)
			}
		}
	}
	writeJSON(w, http.StatusOK, creativeOrderQCContextResponse{
		OrderID: orderID.String(), OrderItemID: itemID, VariantID: variantID.String(), VariantKey: variantKey,
		Workflow: taskContext.Workflow, Revision: revision, Attempt: creativeQCAttemptFromContext(task),
		ExpectedSizes: qcSizes, Brief: json.RawMessage(brief), CopySnapshot: json.RawMessage(copySnapshot), PrimeAssets: primeAssets,
	})
}

// RetryCreativeOrderWorkflowFailure requeues an eligible failed or
// action-required direct creative task only when its target variant has not
// reached a usable terminal state. This is separate from issue rerun because
// creative domain fanout tasks have their own execution lineage.
func (h *Handler) RetryCreativeOrderWorkflowFailure(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	diagnosticActor, ok := h.requireCreativeDiagnosticRecoveryAuthority(w, r, workspaceID)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	taskID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "taskId"), "task_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start workflow retry")
		return
	}
	defer tx.Rollback(r.Context())
	var orderStatus string
	if err := tx.QueryRow(r.Context(), `SELECT status FROM creative_order WHERE id=$1 AND workspace_id=$2 FOR UPDATE`, orderID, workspaceID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	if diagnosticActor.TaskID.Valid {
		if _, err := tx.Exec(r.Context(), `
UPDATE agent_task_queue task
SET max_attempts = GREATEST(task.max_attempts, $1)
FROM agent assigned_agent
WHERE task.id = $2 AND task.agent_id = assigned_agent.id AND assigned_agent.workspace_id = $3
`, creativeDiagnosticRecoveryMaxAttempts, taskID, workspaceID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to grant diagnostic retry budget")
			return
		}
	}

	// QC action-required results are resolved through the variant-level QC
	// recovery path. A single task retry would leave the previous QC resolution
	// in place while moving the variant back to running, which can strand the
	// order forever. Other failed direct creative tasks can be retried here.
	var workflow, taskStatus string
	err = tx.QueryRow(r.Context(), `
SELECT COALESCE(task.context->>'workflow', ''), task.status
FROM agent_task_queue task
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1
  AND assigned_agent.workspace_id = $2
  AND (
    task.context->>'creative_order_id' = $3::text
    OR (
      task.trigger_evidence_ref_id = $3::uuid
      AND NULLIF(task.context->>'variant_id', '') IS NOT NULL
      AND EXISTS (
        SELECT 1
        FROM creative_order_variant linked_variant
        JOIN creative_order_item linked_item ON linked_item.id = linked_variant.order_item_id
        WHERE linked_variant.id::text = task.context->>'variant_id'
          AND linked_item.order_id = $3::uuid
      )
    )
  )
	`, taskID, workspaceID, orderID).Scan(&workflow, &taskStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate workflow retry")
		return
	}
	if creativeWorkflowRequiresAtomicQCRecovery(workflow) {
		writeError(w, http.StatusConflict, "creative QC failures require visual QC recovery")
		return
	}
	freshProviderAttemptAuthorized := false
	if taskStatus == "failed" && workflow == "creative_direct_edit" {
		var err error
		freshProviderAttemptAuthorized, err = h.reconcileUnrecoverableDirectEditOperationForRetry(r.Context(), tx, taskID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reconcile direct edit provider receipt")
			return
		}
	}

	// This update is both the eligibility fence and the domain-state reset. It
	// locks the target variant before a new task is queued, so a concurrent
	// click cannot enqueue another direct retry after the state has moved on.
	var variantID string
	err = tx.QueryRow(r.Context(), `
UPDATE creative_order_variant variant
SET status = 'running',
    brief = variant.brief - 'error_code' - 'error_message' - 'creative_direct_edit_error',
    updated_at = now()
FROM creative_order_item item
JOIN agent_task_queue task ON TRUE
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1
  AND assigned_agent.workspace_id = $2
  AND item.id = variant.order_item_id
  AND item.order_id = $3
  AND task.context->>'variant_id' = variant.id::text
  AND (
    task.issue_id IS NULL
    OR (
      task.trigger_evidence_kind = 'creative_order_item_production'
      AND task.context->>'workflow' = 'creative_production'
      AND task.context ? 'qc_visual_rework'
    )
    OR (
      task.trigger_evidence_kind = 'creative_order_item_direct_edit'
      AND task.context->>'workflow' = 'creative_direct_edit'
    )
  )
  AND task.status IN ('completed', 'failed')
  AND (
    $4::boolean OR task.attempt < task.max_attempts
    OR (
      task.trigger_evidence_kind = 'creative_order_item_production'
      AND task.context->>'workflow' = 'creative_production'
      AND task.attempt < 5
    )
  )
  AND task.context->>'type' = 'creative_domain_task'
  AND (
    task.context->>'creative_order_id' = $3::text
    OR (
      task.trigger_evidence_ref_id = $3::uuid
      AND NULLIF(task.context->>'variant_id', '') IS NOT NULL
      AND EXISTS (
        SELECT 1
        FROM creative_order_variant linked_variant
        JOIN creative_order_item linked_item ON linked_item.id = linked_variant.order_item_id
        WHERE linked_variant.id::text = task.context->>'variant_id'
          AND linked_item.order_id = $3::uuid
      )
    )
  )
  AND variant.status IN ('queued', 'running', 'partial', 'action_required', 'failed')
RETURNING variant.id::text
`, taskID, workspaceID, orderID, !diagnosticActor.TaskID.Valid).Scan(&variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare workflow retry")
		return
	}
	var child db.AgentTaskQueue
	if !diagnosticActor.TaskID.Valid {
		child, err = h.createManualCreativeTaskRetry(r.Context(), tx, taskID, userID)
	} else if taskStatus == "failed" {
		child, err = h.Queries.WithTx(tx).CreateRetryTask(r.Context(), taskID)
	} else {
		child, err = h.Queries.WithTx(tx).CreateActionRequiredRetryTask(r.Context(), taskID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retry workflow task")
		return
	}
	if taskStatus == "failed" && workflow == "creative_direct_edit" {
		// A failed direct image edit may refer to an expired daemon workspace.
		// Its retry must create new provider state instead of resuming that run.
		if _, err := tx.Exec(r.Context(), `
UPDATE agent_task_queue
SET session_id = NULL, work_dir = NULL, force_fresh_session = TRUE
WHERE id = $1
`, child.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reset direct edit retry session")
			return
		}
		child.SessionID = pgtype.Text{}
		child.WorkDir = pgtype.Text{}
		child.ForceFreshSession = true
	}
	if child.TriggerEvidenceKind.Valid && child.TriggerEvidenceKind.String == "creative_order_item_production" && child.TriggerEvidenceRefID.Valid {
		normalized, err := normalizeCreativeProductionFanoutItem(r.Context(), tx, workspaceID, child.TriggerEvidenceRefID, service.DirectTaskFanoutItem{Context: child.Context})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize workflow retry")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE agent_task_queue SET context = $2::jsonb WHERE id = $1`, child.ID, normalized.Context); err != nil {
			writeError(w, http.StatusConflict, "workflow retry is already queued")
			return
		}
		child.Context = normalized.Context
	} else {
		normalized, err := normalizeManualCreativeProductionFanoutItem(r.Context(), tx, workspaceID, orderID, service.DirectTaskFanoutItem{Context: child.Context})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to normalize workflow retry")
			return
		}
		if string(normalized.Context) != string(child.Context) {
			if _, err := tx.Exec(r.Context(), `UPDATE agent_task_queue SET context = $2::jsonb WHERE id = $1`, child.ID, normalized.Context); err != nil {
				writeError(w, http.StatusConflict, "workflow retry is already queued")
				return
			}
			child.Context = normalized.Context
		}
	}
	if freshProviderAttemptAuthorized {
		recoveryContext, err := annotateDirectEditProviderReceiptRecovery(child.Context)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to annotate direct edit provider recovery")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE agent_task_queue SET context = $2::jsonb WHERE id = $1`, child.ID, recoveryContext); err != nil {
			writeError(w, http.StatusConflict, "workflow retry is already queued")
			return
		}
		child.Context = recoveryContext
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue workflow retry")
		return
	}
	if diagnosticActor.TaskID.Valid {
		details, _ := json.Marshal(map[string]any{
			"diagnostic_task_id": uuidToString(diagnosticActor.TaskID),
			"source_task_id":     uuidToString(taskID),
			"retry_task_id":      uuidToString(child.ID),
			"variant_id":         variantID,
			"retry_budget":       creativeDiagnosticRecoveryMaxAttempts,
		})
		if _, err := h.DB.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, actor_type, actor_id, action, details)
VALUES ($1, 'agent', $2, 'creative_diagnostic_workflow_retry', $3::jsonb)
`, workspaceID, diagnosticActor.AgentID, details); err != nil {
			slog.Warn("record diagnostic workflow retry", "task_id", uuidToString(diagnosticActor.TaskID), "error", err)
		}
	}
	h.TaskService.NotifyTaskEnqueued(r.Context(), child)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": uuidToString(orderID), "task_id": uuidToString(taskID), "variant_id": variantID,
	})
	writeJSON(w, http.StatusOK, creativeOrderWorkflowRetryResponse{TaskID: uuidToString(child.ID)})
}

func creativeWorkflowRequiresAtomicQCRecovery(workflow string) bool {
	switch strings.TrimSpace(workflow) {
	case "creative_qc", "creative_qc_technical", "creative_qc_visual":
		return true
	default:
		return false
	}
}

func annotateDirectEditProviderReceiptRecovery(contextValue json.RawMessage) (json.RawMessage, error) {
	var taskContext map[string]any
	if json.Unmarshal(contextValue, &taskContext) != nil || taskContext["workflow"] != "creative_direct_edit" {
		return nil, errors.New("direct edit recovery task context is invalid")
	}
	taskContext["provider_receipt_reconciliation"] = map[string]any{
		"status":                       "provider_receipt_not_found",
		"allow_fresh_provider_attempt": true,
		"instruction":                  "the prior provider attempt was verified empty; register and invoke exactly one next provider attempt",
	}
	annotated, err := json.Marshal(taskContext)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(annotated), nil
}

// reconcileUnrecoverableDirectEditOperationForRetry closes an unknown direct
// image attempt only after the user explicitly retries an exhausted task and
// the database proves that no provider request, receipt, or output exists.
// This releases the single in-flight operation slot for one fresh retry while
// retaining a durable explanation for why the old attempt was abandoned.
func (h *Handler) reconcileUnrecoverableDirectEditOperationForRetry(ctx context.Context, tx pgx.Tx, taskID pgtype.UUID) (bool, error) {
	var taskAttempt, taskMaxAttempts int
	var failureReason string
	if err := tx.QueryRow(ctx, `
SELECT attempt, max_attempts, COALESCE(failure_reason, '')
FROM agent_task_queue
WHERE id = $1 AND status = 'failed'
FOR UPDATE
`, taskID).Scan(&taskAttempt, &taskMaxAttempts, &failureReason); errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if taskAttempt < taskMaxAttempts || failureReason != "creative_output_missing" {
		return false, nil
	}

	var operationID pgtype.UUID
	var operationStatus string
	err := tx.QueryRow(ctx, `
WITH RECURSIVE task_lineage AS (
  SELECT id, parent_task_id, retry_of_task_id
  FROM agent_task_queue
  WHERE id = $1
  UNION
  SELECT parent.id, parent.parent_task_id, parent.retry_of_task_id
  FROM agent_task_queue parent
  JOIN task_lineage child
    ON parent.id = child.parent_task_id OR parent.id = child.retry_of_task_id
)
SELECT operation.id, operation.status
FROM creative_image_operation operation
JOIN creative_image_operation_attempt attempt
  ON attempt.operation_id = operation.id
WHERE operation.operation_kind = 'direct_edit'
  AND operation.task_id IN (SELECT id FROM task_lineage)
  AND attempt.task_id IN (SELECT id FROM task_lineage)
  AND (
    (
      operation.status = 'unknown'
      AND operation.provider_request_id = ''
      AND operation.result_receipt = '{}'::jsonb
      AND operation.output_attachment_id IS NULL
      AND operation.output_asset_id IS NULL
      AND attempt.status = 'unknown'
      AND attempt.provider_request_id = ''
      AND attempt.result_receipt = '{}'::jsonb
      AND attempt.output_attachment_id IS NULL
    )
    OR (
      operation.status = 'failed'
      AND operation.error_type = 'provider_receipt_not_found'
      AND operation.provider_request_id = ''
      AND operation.result_receipt = '{}'::jsonb
      AND operation.output_attachment_id IS NULL
      AND operation.output_asset_id IS NULL
      AND attempt.status = 'failed'
      AND attempt.error_type = 'provider_receipt_not_found'
      AND attempt.provider_request_id = ''
      AND attempt.result_receipt = '{}'::jsonb
      AND attempt.output_attachment_id IS NULL
      AND NOT EXISTS (
        SELECT 1 FROM creative_image_operation_attempt newer_attempt
        WHERE newer_attempt.operation_id = operation.id AND newer_attempt.attempt > attempt.attempt
      )
    )
  )
ORDER BY operation.started_at
LIMIT 1
FOR UPDATE OF operation, attempt
`, taskID).Scan(&operationID, &operationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	if operationStatus == "unknown" {
		const receiptMissing = "provider receipt was not found during explicit direct-edit retry"
		updatedAttempt, err := tx.Exec(ctx, `
UPDATE creative_image_operation_attempt
SET status = 'failed', error_type = 'provider_receipt_not_found', error_message = $2,
    completed_at = now(), updated_at = now()
WHERE operation_id = $1
  AND status = 'unknown'
  AND provider_request_id = ''
  AND result_receipt = '{}'::jsonb
  AND output_attachment_id IS NULL
`, operationID, receiptMissing)
		if err != nil {
			return false, err
		}
		if updatedAttempt.RowsAffected() != 1 {
			return false, errors.New("unrecoverable direct edit attempt changed during reconciliation")
		}
		updatedOperation, err := tx.Exec(ctx, `
UPDATE creative_image_operation
SET status = 'failed', error_type = 'provider_receipt_not_found', error_message = $2,
    completed_at = now(), updated_at = now()
WHERE id = $1
  AND status = 'unknown'
  AND provider_request_id = ''
  AND result_receipt = '{}'::jsonb
  AND output_attachment_id IS NULL
  AND output_asset_id IS NULL
	`, operationID, receiptMissing)
		if err != nil {
			return false, err
		}
		if updatedOperation.RowsAffected() != 1 {
			return false, errors.New("unrecoverable direct edit operation changed during reconciliation")
		}
	}
	return true, nil
}

// RetryCreativeOrderVariantQC reruns the visual QC lane for a completed Prime
// package. It appends a new QC attempt instead of replacing earlier reports.
func (h *Handler) RetryCreativeOrderVariantQC(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	diagnosticActor, ok := h.requireCreativeDiagnosticRecoveryAuthority(w, r, workspaceID)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "variantId"), "variant_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative QC recovery")
		return
	}
	defer tx.Rollback(r.Context())

	var triggerKind, inputSnapshot, orderStatus string
	var issueID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT issue_id, status, trigger_evidence_kind, input_snapshot::text
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&issueID, &orderStatus, &triggerKind, &inputSnapshot); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	var itemID, brief, variantStatus string
	var revision int
	if err := tx.QueryRow(r.Context(), `
SELECT item.id::text, variant.revision, variant.status, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant
`, variantID, orderID).Scan(&itemID, &revision, &variantStatus, &brief); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative QC recovery target")
		return
	}
	if variantStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative QC recovery requires an active variant")
		return
	}
	if !issueID.Valid {
		writeError(w, http.StatusConflict, "creative QC recovery requires an order issue")
		return
	}

	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	qcSizes, err := creativeDirectEditProcessingSizes(json.RawMessage(brief), expectedSizes)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if len(expectedSizes) == 0 {
		writeError(w, http.StatusConflict, "creative QC recovery requires at least one delivery size")
		return
	}
	primedSizes := map[string]struct{}{}
	rows, err := tx.Query(r.Context(), `
SELECT size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
  AND attachment_id IS NOT NULL
FOR UPDATE
`, variantID, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load completed Prime assets")
		return
	}
	for rows.Next() {
		var size string
		if err := rows.Scan(&size); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read completed Prime assets")
			return
		}
		primedSizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read completed Prime assets")
		return
	}
	rows.Close()
	if !creativeSizesMatchExpected(primedSizes, expectedSizes) {
		writeError(w, http.StatusConflict, "all expected completed Prime assets are required before retrying QC")
		return
	}
	var activeQC bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM agent_task_queue
  WHERE context->>'type' = 'creative_domain_task'
    AND context->>'creative_order_id' = $1::text
    AND context->>'variant_id' = $2::text
	AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $3
    AND context->>'workflow' = 'creative_qc_visual'
    AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, orderID, variantID, revision).Scan(&activeQC); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check active creative QC")
		return
	}
	if activeQC {
		writeError(w, http.StatusConflict, "creative QC is already running for this variant")
		return
	}

	nextAttempt, err := creativeNextQCAttempt(r.Context(), tx, variantID, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to allocate creative QC attempt")
		return
	}
	if diagnosticActor.TaskID.Valid && nextAttempt > creativeDiagnosticRecoveryMaxAttempts {
		writeError(w, http.StatusConflict, "diagnostic creative QC recovery budget is exhausted")
		return
	}
	previousAttempt := nextAttempt - 1
	failedTaskIDs := []string{}
	if previousAttempt > 0 {
		failedTaskIDs, err = creativeRecoverableQCTaskIDs(r.Context(), tx, orderID, variantID, revision, previousAttempt)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to snapshot previous creative QC recovery eligibility")
			return
		}
	}
	if previousAttempt < 1 {
		previousAttempt = defaultCreativeQCAttempt
	}

	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(json.RawMessage(inputSnapshot))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	reviewer, err := h.Queries.WithTx(tx).GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: reviewerID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || reviewer.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "the frozen creative QC reviewer is unavailable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the frozen creative QC reviewer")
		return
	}
	var reviewerCapable bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill skill_row ON skill_row.id = binding.skill_id
  WHERE binding.agent_id = $1 AND binding.enabled
    AND skill_row.workspace_id = $2
    AND skill_row.config->>'kind' = 'creative_role'
    AND skill_row.config->>'capability' = 'quality_control'
)
`, reviewer.ID, workspaceID).Scan(&reviewerCapable); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate creative QC reviewer")
		return
	}
	if !reviewerCapable {
		writeError(w, http.StatusConflict, "the frozen creative QC reviewer no longer provides quality_control")
		return
	}
	if h.TaskService == nil {
		writeError(w, http.StatusServiceUnavailable, "creative QC task service is unavailable")
		return
	}

	var priorReports, priorResolution json.RawMessage
	if err := tx.QueryRow(r.Context(), `
SELECT COALESCE(jsonb_agg(jsonb_build_object('attempt', attempt, 'lane', lane, 'status', status, 'findings', findings) ORDER BY attempt, lane), '[]'::jsonb)::text,
       COALESCE((SELECT jsonb_agg(jsonb_build_object('attempt', attempt, 'outcome', outcome, 'failure_summary', failure_summary, 'finalized_by_task_id', COALESCE(finalized_by_task_id::text, '')) ORDER BY attempt)
                 FROM creative_order_variant_qc_resolution
                 WHERE variant_id = $1 AND revision = $2), '[]'::jsonb)::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2
`, variantID, revision).Scan(&priorReports, &priorResolution); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to snapshot previous creative QC")
		return
	}

	contexts := make(map[string]json.RawMessage, 1)
	items := make([]struct {
		lane string
		key  string
	}, 0, 1)
	for _, lane := range []string{"visual"} {
		itemKey := fmt.Sprintf("%s:%s:r%d", uuidToString(variantID), lane, revision)
		context, err := json.Marshal(map[string]any{
			"type":                        "creative_domain_task",
			"workflow":                    "creative_qc_" + lane,
			"scope":                       "variant",
			"subject_id":                  uuidToString(variantID),
			"item_key":                    itemKey,
			"creative_order_id":           uuidToString(orderID),
			"creative_order_item_id":      itemID,
			"variant_id":                  uuidToString(variantID),
			"revision":                    revision,
			"qc_attempt":                  nextAttempt,
			"qc_rerun_of_attempt":         previousAttempt,
			"expected_sizes":              qcSizes,
			"issue_id":                    uuidToString(issueID),
			"leader_agent_id":             uuidToString(leaderID),
			"qc_recovery_of_task_ids":     failedTaskIDs,
			"qc_recovery_kind":            "manual_append_rerun",
			"qc_recovery_asset_invariant": "reuse_completed_primed_assets_only",
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to prepare creative QC recovery")
			return
		}
		contexts[lane] = context
		items = append(items, struct{ lane, key string }{lane: lane, key: itemKey})
	}
	validationItems := make([]service.DirectTaskFanoutItem, 0, len(items))
	for _, item := range items {
		validationItems = append(validationItems, service.DirectTaskFanoutItem{ItemKey: item.key, Context: contexts[item.lane]})
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", variantID, validationItems); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare canonical creative QC recovery context")
		return
	}

	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_variant_qc"), variantID)
	created := map[string]db.AgentTaskQueue{}
	for _, item := range items {
		task, err := h.Queries.WithTx(tx).CreateAgentTask(r.Context(), db.CreateAgentTaskParams{
			AgentID:              reviewer.ID,
			RuntimeID:            reviewer.RuntimeID,
			Priority:             0,
			ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
			RequestingUserID:     userID,
			OriginatorUserID:     attr.UserID,
			AccountableUserID:    attr.AccountableUserID,
			OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
			TriggerEvidenceKind:  pgtype.Text{String: "creative_order_variant_qc", Valid: true},
			TriggerEvidenceRefID: variantID,
			Context:              contexts[item.lane],
		})
		if err != nil {
			writeError(w, http.StatusConflict, "creative QC recovery could not be queued")
			return
		}
		created[item.lane] = task
	}

	detailsPayload := map[string]any{
		"creative_order_id":      uuidToString(orderID),
		"variant_id":             uuidToString(variantID),
		"revision":               revision,
		"attempt":                nextAttempt,
		"previous_attempt":       previousAttempt,
		"recovery_of_task_ids":   failedTaskIDs,
		"visual_task_id":         uuidToString(created["visual"].ID),
		"asset_invariant":        "reused_completed_primed_assets_only",
		"previous_qc_reports":    json.RawMessage(priorReports),
		"previous_qc_resolution": json.RawMessage(priorResolution),
	}
	actorType := "member"
	actorID := userID
	if diagnosticActor.TaskID.Valid {
		actorType = "agent"
		actorID = diagnosticActor.AgentID
		detailsPayload["diagnostic_task_id"] = uuidToString(diagnosticActor.TaskID)
		detailsPayload["retry_budget"] = creativeDiagnosticRecoveryMaxAttempts
	}
	details, err := json.Marshal(detailsPayload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative QC recovery")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, $3, $4, 'creative_qc_recovery_queued', $5::jsonb)
`, workspaceID, issueID, actorType, actorID, details); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative QC recovery")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order_variant SET status = 'running', updated_at = now() WHERE id = $1`, variantID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resume creative QC")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue creative QC recovery")
		return
	}
	for _, task := range created {
		h.TaskService.NotifyTaskEnqueued(r.Context(), task)
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": uuidToString(orderID), "variant_id": uuidToString(variantID), "revision": revision,
		"attempt": nextAttempt, "recovery": "creative_qc",
	})
	writeJSON(w, http.StatusOK, creativeOrderQCRetryResponse{
		VariantID:    uuidToString(variantID),
		Revision:     revision,
		Attempt:      nextAttempt,
		VisualTaskID: uuidToString(created["visual"].ID),
	})
}

// queueCreativeQCAutomaticRecovery reuses a complete Prime package after a
// visual QC delegation/contract failure. It is called from the QC barrier so a
// transient evidence failure never becomes a manual step.
func (h *Handler) queueCreativeQCAutomaticRecovery(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, userID pgtype.UUID,
	orderID, itemID, variantID, issueID pgtype.UUID,
	revision int,
	inputSnapshot json.RawMessage,
	expectedSizes []string,
	failedTaskIDs []string,
	recoveryKind string,
) (map[string]db.AgentTaskQueue, int, error) {
	if recoveryKind == "" {
		recoveryKind = creativeQCAutomaticContractRecoveryKind
	}
	if h.TaskService == nil {
		return nil, 0, errors.New("creative QC task service is unavailable")
	}
	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(inputSnapshot)
	if err != nil {
		return nil, 0, err
	}
	reviewer, err := h.Queries.WithTx(tx).GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: reviewerID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) || reviewer.ArchivedAt.Valid {
		return nil, 0, errors.New("the frozen creative QC reviewer is unavailable")
	}
	if err != nil {
		return nil, 0, errors.New("failed to load the frozen creative QC reviewer")
	}
	var reviewerCapable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill skill_row ON skill_row.id = binding.skill_id
  WHERE binding.agent_id = $1 AND binding.enabled
    AND skill_row.workspace_id = $2
    AND skill_row.config->>'kind' = 'creative_role'
    AND skill_row.config->>'capability' = 'quality_control'
)
`, reviewer.ID, workspaceID).Scan(&reviewerCapable); err != nil {
		return nil, 0, errors.New("failed to validate the frozen creative QC reviewer")
	}
	if !reviewerCapable {
		return nil, 0, errors.New("the frozen creative QC reviewer no longer provides quality_control")
	}
	nextAttempt, err := creativeNextQCAttempt(ctx, tx, variantID, revision)
	if err != nil {
		return nil, 0, errors.New("failed to allocate automatic creative QC attempt")
	}
	previousAttempt := nextAttempt - 1
	if previousAttempt < 1 {
		previousAttempt = defaultCreativeQCAttempt
	}

	contexts := make(map[string]json.RawMessage, 1)
	items := make([]struct {
		lane string
		key  string
	}, 0, 1)
	for _, lane := range []string{"visual"} {
		itemKey := fmt.Sprintf("%s:%s:r%d", uuidToString(variantID), lane, revision)
		contextValue, err := json.Marshal(map[string]any{
			"type":                        "creative_domain_task",
			"workflow":                    "creative_qc_" + lane,
			"scope":                       "variant",
			"subject_id":                  uuidToString(variantID),
			"item_key":                    itemKey,
			"creative_order_id":           uuidToString(orderID),
			"creative_order_item_id":      uuidToString(itemID),
			"variant_id":                  uuidToString(variantID),
			"revision":                    revision,
			"qc_attempt":                  nextAttempt,
			"qc_rerun_of_attempt":         previousAttempt,
			"expected_sizes":              expectedSizes,
			"issue_id":                    uuidToString(issueID),
			"leader_agent_id":             uuidToString(leaderID),
			"qc_recovery_of_task_ids":     failedTaskIDs,
			"qc_recovery_kind":            recoveryKind,
			"qc_recovery_asset_invariant": "reuse_completed_primed_assets_only",
		})
		if err != nil {
			return nil, 0, errors.New("failed to prepare automatic creative QC recovery")
		}
		contexts[lane] = contextValue
		items = append(items, struct {
			lane string
			key  string
		}{lane: lane, key: itemKey})
	}
	validationItems := make([]service.DirectTaskFanoutItem, 0, len(items))
	for _, item := range items {
		validationItems = append(validationItems, service.DirectTaskFanoutItem{ItemKey: item.key, Context: contexts[item.lane]})
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_qc", variantID, validationItems); err != nil {
		return nil, 0, errors.New("failed to prepare canonical automatic creative QC recovery context")
	}

	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_variant_qc"), variantID)
	created := make(map[string]db.AgentTaskQueue, 1)
	for _, item := range items {
		task, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
			AgentID:              reviewer.ID,
			RuntimeID:            reviewer.RuntimeID,
			Priority:             0,
			ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
			RequestingUserID:     userID,
			OriginatorUserID:     attr.UserID,
			AccountableUserID:    attr.AccountableUserID,
			OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
			TriggerEvidenceKind:  pgtype.Text{String: "creative_order_variant_qc", Valid: true},
			TriggerEvidenceRefID: variantID,
			Context:              contexts[item.lane],
		})
		if err != nil {
			return nil, 0, errors.New("automatic creative QC recovery could not be queued")
		}
		created[item.lane] = task
	}
	details, err := json.Marshal(map[string]any{
		"creative_order_id":    uuidToString(orderID),
		"variant_id":           uuidToString(variantID),
		"revision":             revision,
		"attempt":              nextAttempt,
		"previous_attempt":     previousAttempt,
		"recovery_of_task_ids": failedTaskIDs,
		"visual_task_id":       uuidToString(created["visual"].ID),
		"asset_invariant":      "reused_completed_primed_assets_only",
		"automatic":            true,
		"recovery_kind":        recoveryKind,
	})
	if err != nil {
		return nil, 0, errors.New("failed to record automatic creative QC recovery")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_qc_recovery_queued', $4::jsonb)
`, workspaceID, issueID, userID, details); err != nil {
		return nil, 0, errors.New("failed to record automatic creative QC recovery")
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant SET status = 'running', updated_at = now() WHERE id = $1
`, variantID); err != nil {
		return nil, 0, errors.New("failed to resume creative QC")
	}
	return created, nextAttempt, nil
}

func creativeOrderQCAgentSnapshot(raw json.RawMessage) (pgtype.UUID, pgtype.UUID, error) {
	leaderID, _, reviewerID, err := creativeOrderProductionAgentSnapshot(raw)
	return leaderID, reviewerID, err
}

func creativeOrderProductionAgentSnapshot(raw json.RawMessage) (pgtype.UUID, pgtype.UUID, pgtype.UUID, error) {
	var snapshot struct {
		SquadSnapshot struct {
			LeaderAgentID   string `json:"leader_agent_id"`
			ProducerAgentID string `json:"producer_agent_id"`
			ReviewerAgentID string `json:"reviewer_agent_id"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, errors.New("creative order has an invalid frozen squad snapshot")
	}
	parseSnapshotAgent := func(rawID, role string) (pgtype.UUID, error) {
		id, err := uuid.Parse(strings.TrimSpace(rawID))
		if err != nil {
			return pgtype.UUID{}, fmt.Errorf("creative order is missing a frozen %s agent", role)
		}
		return pgtype.UUID{Bytes: id, Valid: true}, nil
	}
	leaderID, err := parseSnapshotAgent(snapshot.SquadSnapshot.LeaderAgentID, "leader")
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	producerID, err := parseSnapshotAgent(snapshot.SquadSnapshot.ProducerAgentID, "production")
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	reviewerID, err := parseSnapshotAgent(snapshot.SquadSnapshot.ReviewerAgentID, "QC reviewer")
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, pgtype.UUID{}, err
	}
	return leaderID, producerID, reviewerID, nil
}

func creativeTaskPreferredProducerID(raw json.RawMessage) pgtype.UUID {
	return creativeTaskPreferredAgentID(raw, "producer_agent_id")
}

func creativeTaskPreferredDirectEditAgentID(raw json.RawMessage) pgtype.UUID {
	return creativeTaskPreferredAgentID(raw, "direct_edit_agent_id")
}

func creativeTaskPreferredAgentID(raw json.RawMessage, field string) pgtype.UUID {
	var values map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return pgtype.UUID{}
	}
	var rawID string
	if json.Unmarshal(values[field], &rawID) != nil {
		return pgtype.UUID{}
	}
	id, err := uuid.Parse(strings.TrimSpace(rawID))
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

func (h *Handler) selectCreativeImageEditAgent(ctx context.Context, q creativeTaskFanoutQuerier, qtx *db.Queries, workspaceID pgtype.UUID, inputSnapshot json.RawMessage, seed string, preferred pgtype.UUID, pinPreferred bool) (db.Agent, error) {
	return h.selectCreativeCapabilityAgent(ctx, q, qtx, workspaceID, inputSnapshot, "image_edit", seed, preferred, pinPreferred, "creative production image_edit agent pool has no online members", "selected creative production agent is unavailable")
}

func (h *Handler) selectCreativeDirectImageEditAgent(ctx context.Context, q creativeTaskFanoutQuerier, qtx *db.Queries, workspaceID pgtype.UUID, inputSnapshot json.RawMessage, seed string, preferred pgtype.UUID, pinPreferred bool) (db.Agent, error) {
	return h.selectCreativeCapabilityAgent(ctx, q, qtx, workspaceID, inputSnapshot, "direct_image_edit", seed, preferred, pinPreferred, "creative direct image edit agent pool has no online members", "selected creative direct image edit agent is unavailable")
}

func (h *Handler) selectCreativeCapabilityAgent(ctx context.Context, q creativeTaskFanoutQuerier, qtx *db.Queries, workspaceID pgtype.UUID, inputSnapshot json.RawMessage, capability, seed string, preferred pgtype.UUID, pinPreferred bool, emptyPoolMessage, unavailableMessage string) (db.Agent, error) {
	squadIDText, err := creativeOrderSnapshotSquadID(inputSnapshot)
	if err != nil {
		return db.Agent{}, err
	}
	squadUUID, err := uuid.Parse(strings.TrimSpace(squadIDText))
	if err != nil {
		return db.Agent{}, errors.New("creative order has an invalid frozen squad")
	}
	squadID := pgtype.UUID{Bytes: squadUUID, Valid: true}
	seed = strings.TrimSpace(seed)
	if seed == "" {
		seed = squadUUID.String()
	}
	capability = strings.TrimSpace(capability)
	if capability == "" {
		return db.Agent{}, errors.New("creative capability is required")
	}

	var agentID pgtype.UUID
	err = q.QueryRow(ctx, `
SELECT agent_row.id
FROM squad_member member
JOIN agent agent_row
  ON agent_row.id = member.member_id
 AND agent_row.workspace_id = $2
 AND agent_row.archived_at IS NULL
JOIN agent_runtime runtime_row
  ON runtime_row.id = agent_row.runtime_id
 AND runtime_row.workspace_id = agent_row.workspace_id
 AND runtime_row.status = 'online'
JOIN agent_skill binding
  ON binding.agent_id = agent_row.id
 AND binding.enabled
JOIN skill role_skill
  ON role_skill.id = binding.skill_id
 AND role_skill.workspace_id = $2
 AND role_skill.config->>'kind' = 'creative_role'
 AND role_skill.config->>'capability' = $5
LEFT JOIN agent_task_queue active_task
  ON active_task.agent_id = agent_row.id
 AND active_task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
WHERE member.squad_id = $1
  AND member.member_type = 'agent'
GROUP BY agent_row.id
ORDER BY
  CASE WHEN $6::boolean AND $4::uuid IS NOT NULL AND agent_row.id = $4::uuid THEN 0 ELSE 1 END ASC,
  count(active_task.id) ASC,
  CASE WHEN NOT $6::boolean AND $4::uuid IS NOT NULL AND agent_row.id = $4::uuid THEN 0 ELSE 1 END ASC,
  md5($3::text || ':' || agent_row.id::text) ASC
LIMIT 1
`, squadID, workspaceID, seed, preferred, capability, pinPreferred).Scan(&agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Agent{}, errors.New(emptyPoolMessage)
	}
	if err != nil {
		return db.Agent{}, err
	}
	agent, err := qtx.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: workspaceID})
	if err != nil {
		return db.Agent{}, err
	}
	if agent.ArchivedAt.Valid || !agent.RuntimeID.Valid {
		return db.Agent{}, errors.New(unavailableMessage)
	}
	return agent, nil
}

func selectedCreativeProductionAgentFromHistory(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, variantID pgtype.UUID, revision int) pgtype.UUID {
	return selectedCreativeAgentFromHistory(ctx, q, workspaceID, variantID, revision, "creative_order_item_production", "creative_production")
}

func selectedCreativeDirectEditAgentFromHistory(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, variantID pgtype.UUID, revision int) pgtype.UUID {
	return selectedCreativeAgentFromHistory(ctx, q, workspaceID, variantID, revision, "creative_order_item_direct_edit", "creative_direct_edit")
}

func selectedCreativeAgentFromHistory(ctx context.Context, q creativeTaskFanoutQuerier, workspaceID, variantID pgtype.UUID, revision int, evidenceKind, workflow string) pgtype.UUID {
	var agentID pgtype.UUID
	err := q.QueryRow(ctx, `
SELECT task.agent_id
FROM agent_task_queue task
JOIN agent agent_row ON agent_row.id = task.agent_id AND agent_row.workspace_id = $1
WHERE task.trigger_evidence_kind = $4
  AND task.context->>'type' = 'creative_domain_task'
  AND task.context->>'workflow' = $5
  AND task.context->>'variant_id' = $2::text
  AND CASE
        WHEN NULLIF(task.context->>'revision', '') ~ '^[0-9]+$'
          THEN (task.context->>'revision')::int
        ELSE 0
      END = $3
ORDER BY
  CASE task.status
    WHEN 'completed' THEN 0
    WHEN 'running' THEN 1
    WHEN 'dispatched' THEN 2
    WHEN 'queued' THEN 3
    WHEN 'waiting_local_directory' THEN 4
    ELSE 5
  END,
  task.created_at ASC,
  task.id ASC
LIMIT 1
`, workspaceID, variantID, revision, evidenceKind, workflow).Scan(&agentID)
	if err != nil {
		return pgtype.UUID{}
	}
	return agentID
}

func creativeOrderDirectEditAgentSnapshot(raw json.RawMessage) (pgtype.UUID, error) {
	var snapshot struct {
		SquadSnapshot struct {
			DirectEditAgentID string `json:"direct_edit_agent_id"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return pgtype.UUID{}, errors.New("creative order has an invalid frozen squad snapshot")
	}
	id, err := uuid.Parse(strings.TrimSpace(snapshot.SquadSnapshot.DirectEditAgentID))
	if err != nil {
		return pgtype.UUID{}, errors.New("creative order is missing a frozen direct image edit agent")
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

func creativeVisualModelReworkFindings(findings json.RawMessage, expectedSizes []string) ([]creativeVisualModelReworkFinding, error) {
	var report struct {
		BlockingFailures []creativeVisualModelReworkFinding `json:"blocking_failures"`
	}
	if err := json.Unmarshal(findings, &report); err != nil {
		return nil, errors.New("visual QC findings must be an object")
	}
	if len(report.BlockingFailures) == 0 {
		return nil, errors.New("visual QC has no model-rework findings")
	}
	expected := make(map[string]struct{}, len(expectedSizes))
	for _, size := range expectedSizes {
		expected[size] = struct{}{}
	}
	// A visual report can contain independent failures for multiple sizes. A
	// non-reworkable problem must keep its own size out of model rework, but it
	// must not suppress a precise Prime fix for another size.
	eligibleBySize := make(map[string]creativeVisualModelReworkFinding, len(report.BlockingFailures))
	blockedSizes := make(map[string]struct{}, len(report.BlockingFailures))
	for index := range report.BlockingFailures {
		finding := &report.BlockingFailures[index]
		finding.Code = strings.TrimSpace(finding.Code)
		finding.SizeKey = strings.TrimSpace(finding.SizeKey)
		finding.Diagnosis = strings.TrimSpace(finding.Diagnosis)
		isEligibleCode := finding.Code == "actual_prime_obstruction" || finding.Code == "official_prime_text_unreadable"
		if !isEligibleCode {
			if _, ok := expected[finding.SizeKey]; ok {
				blockedSizes[finding.SizeKey] = struct{}{}
			}
			continue
		}
		if _, ok := expected[finding.SizeKey]; !ok || !validCreativeVisualModelReworkDiagnosis(finding.Code, finding.SizeKey, finding.Diagnosis) {
			blockedSizes[finding.SizeKey] = struct{}{}
			continue
		}
		if previous, duplicate := eligibleBySize[finding.SizeKey]; duplicate {
			if previous.Code == finding.Code {
				blockedSizes[finding.SizeKey] = struct{}{}
			} else {
				previous.Code = "actual_prime_obstruction"
				previous.Diagnosis += "；" + finding.Diagnosis
				eligibleBySize[finding.SizeKey] = previous
			}
			continue
		}
		eligibleBySize[finding.SizeKey] = *finding
	}

	eligible := make([]creativeVisualModelReworkFinding, 0, len(eligibleBySize))
	for _, finding := range report.BlockingFailures {
		if _, blocked := blockedSizes[finding.SizeKey]; blocked {
			continue
		}
		selected, ok := eligibleBySize[finding.SizeKey]
		if !ok {
			continue
		}
		eligible = append(eligible, selected)
		delete(eligibleBySize, finding.SizeKey)
	}
	if len(eligible) == 0 {
		return nil, errors.New("visual QC has no safely eligible model-rework findings")
	}
	return eligible, nil
}

func validCreativeVisualModelReworkDiagnosis(code, sizeKey, diagnosis string) bool {
	if len([]rune(diagnosis)) > 500 || diagnosis == "" ||
		!(strings.HasPrefix(diagnosis, sizeKey+"：") || strings.HasPrefix(diagnosis, sizeKey+":")) {
		return false
	}
	if code == "actual_prime_obstruction" {
		return strings.Contains(diagnosis, "期望移动到") &&
			(strings.Contains(diagnosis, "safe_content_frame") || strings.Contains(diagnosis, " y")) &&
			(strings.Contains(diagnosis, "冲突") || strings.Contains(diagnosis, "重叠") || strings.Contains(diagnosis, "叠压") || strings.Contains(diagnosis, "遮挡") || strings.Contains(diagnosis, "进入顶部 Prime 禁区") || strings.Contains(diagnosis, "进入底部 Prime 禁区"))
	}
	readsUnclearOfficialText := strings.Contains(diagnosis, "不可读") || strings.Contains(diagnosis, "看不清") ||
		strings.Contains(diagnosis, "不清晰") || strings.Contains(diagnosis, "难以辨认") || strings.Contains(diagnosis, "对比不足")
	requestsBackgroundRework := strings.Contains(diagnosis, "期望调整为") &&
		(strings.Contains(diagnosis, "背景") || strings.Contains(strings.ToLower(diagnosis), "background"))
	legacyStructuredSupport := strings.Contains(diagnosis, "期望承托区") &&
		strings.Contains(diagnosis, "极性") &&
		(strings.Contains(diagnosis, "相对亮度") || strings.Contains(strings.ToLower(diagnosis), "luminance")) &&
		(strings.Contains(diagnosis, "纹理") || strings.Contains(strings.ToLower(diagnosis), "texture"))
	return (requestsBackgroundRework || legacyStructuredSupport) && readsUnclearOfficialText
}

type creativePrimeReadabilityEvidence struct {
	TemplateSelection struct {
		VisualAdequacy struct {
			Status string `json:"status"`
		} `json:"visual_adequacy"`
	} `json:"template_selection"`
	VisibilityAudit struct {
		Blocking          bool `json:"blocking"`
		BackgroundSupport struct {
			RelativeLuminanceContrast struct {
				MinimumLocalP10 float64 `json:"minimum_local_p10"`
			} `json:"relative_luminance_contrast"`
			Texture struct {
				MaximumLocalP90 float64 `json:"maximum_local_p90"`
				Threshold       float64 `json:"threshold"`
			} `json:"texture"`
		} `json:"background_support"`
	} `json:"visibility_audit"`
}

// creativePrimeCriticalReadabilityFinding promotes only the extreme subset of
// composer's qc_risk evidence. This preserves the visual lane for borderline
// anti-aliasing samples while fencing a severe visibility failure.
func creativePrimeCriticalReadabilityFinding(sizeKey string, evidence json.RawMessage) (creativeVisualModelReworkFinding, bool) {
	var parsed creativePrimeReadabilityEvidence
	if err := json.Unmarshal(evidence, &parsed); err != nil || parsed.TemplateSelection.VisualAdequacy.Status != "qc_risk" || !parsed.VisibilityAudit.Blocking {
		return creativeVisualModelReworkFinding{}, false
	}
	contrast := parsed.VisibilityAudit.BackgroundSupport.RelativeLuminanceContrast.MinimumLocalP10
	texture := parsed.VisibilityAudit.BackgroundSupport.Texture
	criticalContrast := contrast > 0 && contrast <= creativePrimeCriticalRelativeLuminanceContrast
	criticalTexture := texture.Threshold > 0 && texture.MaximumLocalP90 > texture.Threshold
	if !criticalContrast && !criticalTexture {
		return creativeVisualModelReworkFinding{}, false
	}
	causes := make([]string, 0, 2)
	if criticalContrast {
		causes = append(causes, fmt.Sprintf("局部相对亮度 %.4f 低于严重下限 %.2f", contrast, creativePrimeCriticalRelativeLuminanceContrast))
	}
	if criticalTexture {
		causes = append(causes, fmt.Sprintf("局部纹理 %.6f 超过阈值 %.6f", texture.MaximumLocalP90, texture.Threshold))
	}
	return creativeVisualModelReworkFinding{
		Code:    "official_prime_text_unreadable",
		SizeKey: sizeKey,
		Diagnosis: fmt.Sprintf(
			"%s：官方 Prime 文字在可见字形 mask 上%s，实际不可读；期望调整为满足 background_support.polarity 的低纹理承托背景，使该官方文字区域达到证据中的相对亮度与纹理门槛",
			sizeKey, strings.Join(causes, "且"),
		),
	}, true
}

func creativePrimeCriticalReadabilityFindings(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string) ([]creativeVisualModelReworkFinding, error) {
	rows, err := tx.Query(ctx, `
SELECT size_key, evidence
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
	  AND stage = 'primed'
	  AND status = 'completed'
	  AND attachment_id IS NOT NULL
	  AND size_key = ANY($3::text[])
ORDER BY size_key
	`, variantID, revision, expectedSizes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	findings := make([]creativeVisualModelReworkFinding, 0)
	for rows.Next() {
		var sizeKey string
		var evidence json.RawMessage
		if err := rows.Scan(&sizeKey, &evidence); err != nil {
			return nil, err
		}
		if finding, critical := creativePrimeCriticalReadabilityFinding(sizeKey, evidence); critical {
			findings = append(findings, finding)
		}
	}
	return findings, rows.Err()
}

func mergeCreativePrimeCriticalReadabilityFailures(input creativeOrderQCInput, critical []creativeVisualModelReworkFinding) (creativeOrderQCInput, error) {
	if input.Lane != "visual" || len(critical) == 0 {
		return input, nil
	}
	var findings map[string]json.RawMessage
	if err := json.Unmarshal(input.Findings, &findings); err != nil || findings == nil {
		return input, errors.New("visual QC findings must be an object")
	}
	var blocking []json.RawMessage
	if raw, exists := findings["blocking_failures"]; exists && string(raw) != "null" {
		if err := json.Unmarshal(raw, &blocking); err != nil {
			return input, errors.New("findings.blocking_failures must be an array")
		}
	}
	criticalBySize := make(map[string]creativeVisualModelReworkFinding, len(critical))
	for _, finding := range critical {
		finding.SizeKey = strings.TrimSpace(finding.SizeKey)
		criticalBySize[finding.SizeKey] = finding
	}
	type criticalSizeState struct {
		seen        bool
		allEligible bool
		obstruction string
	}
	criticalState := make(map[string]criticalSizeState, len(critical))
	retained := make([]json.RawMessage, 0, len(blocking))
	for _, raw := range blocking {
		var existing creativeVisualModelReworkFinding
		if json.Unmarshal(raw, &existing) != nil {
			retained = append(retained, raw)
			continue
		}
		sizeKey := strings.TrimSpace(existing.SizeKey)
		if _, isCritical := criticalBySize[sizeKey]; !isCritical {
			retained = append(retained, raw)
			continue
		}
		state := criticalState[sizeKey]
		if !state.seen {
			state.allEligible = true
		}
		state.seen = true
		existing.Code = strings.TrimSpace(existing.Code)
		existing.Diagnosis = strings.TrimSpace(existing.Diagnosis)
		if existing.Code != "actual_prime_obstruction" && existing.Code != "official_prime_text_unreadable" {
			state.allEligible = false
			retained = append(retained, raw)
		} else if existing.Code == "actual_prime_obstruction" &&
			validCreativeVisualModelReworkDiagnosis(existing.Code, sizeKey, existing.Diagnosis) {
			state.obstruction = existing.Diagnosis
		}
		criticalState[sizeKey] = state
	}
	blocking = retained
	for _, finding := range critical {
		finding.SizeKey = strings.TrimSpace(finding.SizeKey)
		state := criticalState[finding.SizeKey]
		if state.seen && !state.allEligible {
			continue
		}
		if state.obstruction != "" {
			obstruction := strings.TrimPrefix(state.obstruction, finding.SizeKey+"：")
			candidate := finding.Diagnosis + "；同时修正已核实的 Prime 遮挡：" + obstruction
			if len([]rune(candidate)) <= 500 {
				finding.Diagnosis = candidate
			} else {
				finding.Diagnosis += "；同时将被遮挡的正文移回 safe_content_frame"
			}
		}
		encoded, err := json.Marshal(finding)
		if err != nil {
			return input, err
		}
		blocking = append(blocking, encoded)
	}
	encodedBlocking, err := json.Marshal(blocking)
	if err != nil {
		return input, err
	}
	findings["blocking_failures"] = encodedBlocking
	encodedFindings, err := json.Marshal(findings)
	if err != nil {
		return input, err
	}
	input.Findings = encodedFindings
	input.Status = "failed"
	return input, nil
}

type creativeQCPrimeReinspectionConflict struct {
	SizeKey      string
	PreviousCode string
}

// A retry may revise a hard Prime conclusion only after the inspected assets
// changed. Otherwise a prior hard failure remains a delivery fence.
func creativeQCCheckedPrimeAttachments(findings json.RawMessage) (map[string]string, bool) {
	var report struct {
		CheckedAssets []struct {
			SizeKey      string `json:"size_key"`
			AttachmentID string `json:"attachment_id"`
		} `json:"checked_assets"`
	}
	if err := json.Unmarshal(findings, &report); err != nil || len(report.CheckedAssets) == 0 {
		return nil, false
	}
	attachments := make(map[string]string, len(report.CheckedAssets))
	for _, asset := range report.CheckedAssets {
		sizeKey := strings.TrimSpace(asset.SizeKey)
		attachmentID := strings.TrimSpace(asset.AttachmentID)
		if sizeKey == "" || attachmentID == "" {
			return nil, false
		}
		if _, duplicate := attachments[sizeKey]; duplicate {
			return nil, false
		}
		attachments[sizeKey] = attachmentID
	}
	return attachments, true
}

func creativeQCPrimeAttachmentsMatch(left, right map[string]string) bool {
	if len(left) == 0 || len(left) != len(right) {
		return false
	}
	for sizeKey, attachmentID := range left {
		if right[sizeKey] != attachmentID {
			return false
		}
	}
	return true
}

func creativeQCHardPrimeReinspectionConflicts(previous, current json.RawMessage) []creativeQCPrimeReinspectionConflict {
	previousAttachments, previousOK := creativeQCCheckedPrimeAttachments(previous)
	currentAttachments, currentOK := creativeQCCheckedPrimeAttachments(current)
	if !previousOK || !currentOK || !creativeQCPrimeAttachmentsMatch(previousAttachments, currentAttachments) {
		return nil
	}
	var report struct {
		BlockingFailures []struct {
			Code    string `json:"code"`
			SizeKey string `json:"size_key"`
		} `json:"blocking_failures"`
	}
	if json.Unmarshal(previous, &report) != nil {
		return nil
	}
	seen := make(map[string]struct{})
	conflicts := make([]creativeQCPrimeReinspectionConflict, 0)
	for _, failure := range report.BlockingFailures {
		code := strings.TrimSpace(failure.Code)
		sizeKey := strings.TrimSpace(failure.SizeKey)
		if (code != "actual_prime_obstruction" && code != "official_prime_text_unreadable") || sizeKey == "" {
			continue
		}
		if _, duplicate := seen[sizeKey]; duplicate {
			continue
		}
		seen[sizeKey] = struct{}{}
		conflicts = append(conflicts, creativeQCPrimeReinspectionConflict{SizeKey: sizeKey, PreviousCode: code})
	}
	return conflicts
}

func mergeCreativeQCPrimeReinspectionConflicts(findings json.RawMessage, conflicts []creativeQCPrimeReinspectionConflict) (json.RawMessage, error) {
	if len(conflicts) == 0 {
		return findings, nil
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(findings, &report); err != nil || report == nil {
		return nil, errors.New("visual QC findings must be an object")
	}
	var blocking []json.RawMessage
	if raw, exists := report["blocking_failures"]; exists && string(raw) != "null" {
		if err := json.Unmarshal(raw, &blocking); err != nil {
			return nil, errors.New("findings.blocking_failures must be an array")
		}
	}
	alreadyFenced := make(map[string]struct{})
	for _, raw := range blocking {
		var existing struct {
			Code    string `json:"code"`
			SizeKey string `json:"size_key"`
		}
		if json.Unmarshal(raw, &existing) == nil && strings.TrimSpace(existing.Code) == "prime_reinspection_conflict" {
			alreadyFenced[strings.TrimSpace(existing.SizeKey)] = struct{}{}
		}
	}
	for _, conflict := range conflicts {
		if _, exists := alreadyFenced[conflict.SizeKey]; exists {
			continue
		}
		fence := map[string]string{
			"code":      "prime_reinspection_conflict",
			"size_key":  conflict.SizeKey,
			"diagnosis": fmt.Sprintf("%s: the same Prime attachment previously failed %s; unchanged assets cannot be released by a retry", conflict.SizeKey, conflict.PreviousCode),
		}
		encoded, err := json.Marshal(fence)
		if err != nil {
			return nil, err
		}
		blocking = append(blocking, encoded)
		alreadyFenced[conflict.SizeKey] = struct{}{}
	}
	encodedBlocking, err := json.Marshal(blocking)
	if err != nil {
		return nil, err
	}
	report["blocking_failures"] = encodedBlocking
	return json.Marshal(report)
}

func creativeVisualModelReworkAttemptCount(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `
SELECT count(DISTINCT CASE WHEN context->'qc_visual_rework'->>'policy_version' = '2'
 THEN COALESCE(NULLIF(context->>'revision', ''), id::text) ELSE id::text END)
FROM agent_task_queue
WHERE trigger_evidence_kind = ANY(ARRAY['creative_order_item_production', 'creative_order_item_direct_edit']::text[])
  AND context->>'type' = 'creative_domain_task'
  AND context->>'workflow' = ANY(ARRAY['creative_production', 'creative_direct_edit']::text[])
  AND context->>'variant_id' = $1::text
  AND context ? 'qc_visual_rework'
	AND (
	  status NOT IN ('completed', 'failed', 'cancelled')
	  OR EXISTS (
	    SELECT 1
	    FROM creative_image_operation operation
	    WHERE operation.task_id = agent_task_queue.id
	  )
	)
	`, variantID).Scan(&count)
	return count, err
}

func creativeRecoverableQCTaskIDs(ctx context.Context, tx pgx.Tx, orderID, variantID pgtype.UUID, revision int, attempts ...int) ([]string, error) {
	attempt := defaultCreativeQCAttempt
	if len(attempts) > 0 && attempts[0] > 0 {
		attempt = attempts[0]
	}
	rows, err := tx.Query(ctx, `
SELECT id::text
FROM agent_task_queue
WHERE (
    context->>'type' = 'creative_domain_task'
    OR (context->>'type' = 'creative_qc' AND context->>'workflow' = 'creative_qc')
  )
  AND context->>'creative_order_id' = $1::text
  AND context->>'variant_id' = $2::text
  AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $3
  AND COALESCE(
    CASE
      WHEN NULLIF(context->>'qc_attempt', '') ~ '^[0-9]+$'
        THEN (context->>'qc_attempt')::int
    END,
    1
  ) = $4
  AND (
    (
      context->>'workflow' = 'creative_qc_visual'
      AND status = 'failed'
      AND COALESCE(NULLIF(failure_reason, ''), 'agent_error') = ANY($5::text[])
    )
    OR (
      status = 'completed'
      AND EXISTS (
        SELECT 1
        FROM creative_order_qc_report report
        WHERE report.variant_id = $2::uuid
          AND report.revision = $3
          AND report.attempt = $4
          AND report.lane = 'visual'
          AND (
            context->>'workflow' = 'creative_qc_visual'
            OR (context->>'workflow' = 'creative_qc' AND context->>'lane' = 'visual')
          )
          AND report.status = 'failed'
          AND (
            EXISTS (
              SELECT 1
              FROM jsonb_array_elements(
                CASE
                  WHEN jsonb_typeof(report.findings->'blocking_failures') = 'array'
                    THEN report.findings->'blocking_failures'
                  ELSE '[]'::jsonb
                END
              ) AS finding
              WHERE finding->>'code' LIKE 'delegation_contract_%'
                OR finding->>'code' LIKE 'manifest_%'
                OR finding->>'code' LIKE 'prime_layout_contract_%'
                OR finding->>'code' LIKE 'qc_batch_%'
                OR finding->>'code' LIKE 'attachment_download_%'
                OR finding->>'code' LIKE 'visual_inspection_%'
            )
            OR report.findings->>'failure_code' LIKE 'delegation_contract_%'
            OR report.findings->>'failure_code' LIKE 'prompt_contract_%'
            OR report.findings->>'failure_code' LIKE 'manifest_%'
            OR report.findings->>'failure_code' LIKE 'prime_layout_contract_%'
            OR report.findings->>'failure_code' LIKE 'qc_batch_%'
            OR report.findings->>'failure_code' LIKE 'attachment_download_%'
            OR report.findings->>'failure_code' LIKE 'visual_inspection_%'
          )
        )
    )
  )
ORDER BY created_at, id
`, orderID, variantID, revision, attempt, []string{
		"agent_error", "api_invalid_request", "agent_fallback_message", "codex_semantic_inactivity",
		"provider_rate_limited", "queued_expired", "runtime_offline", "runtime_recovery", "timeout",
	})
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

func creativeQCFindingsNeedAutomaticRecovery(findings json.RawMessage) bool {
	var report struct {
		BlockingFailures []struct {
			Code string `json:"code"`
		} `json:"blocking_failures"`
	}
	if err := json.Unmarshal(findings, &report); err != nil {
		return false
	}
	for _, finding := range report.BlockingFailures {
		code := strings.ToLower(strings.TrimSpace(finding.Code))
		if strings.HasPrefix(code, "delegation_contract_") ||
			strings.HasPrefix(code, "prompt_contract_") ||
			strings.HasPrefix(code, "manifest_") ||
			strings.HasPrefix(code, "prime_layout_contract_") ||
			strings.HasPrefix(code, "qc_batch_") ||
			strings.HasPrefix(code, "attachment_download_") {
			return true
		}
	}
	var failureReport struct {
		FailureCode string `json:"failure_code"`
	}
	if err := json.Unmarshal(findings, &failureReport); err == nil {
		code := strings.ToLower(strings.TrimSpace(failureReport.FailureCode))
		if strings.HasPrefix(code, "delegation_contract_") ||
			strings.HasPrefix(code, "prompt_contract_") ||
			strings.HasPrefix(code, "manifest_") ||
			strings.HasPrefix(code, "prime_layout_contract_") ||
			strings.HasPrefix(code, "qc_batch_") ||
			strings.HasPrefix(code, "attachment_download_") {
			return true
		}
	}
	return false
}

// CancelCreativeOrder closes an unfinished order while preserving its assets,
// failures, and collaboration history. Active issue tasks are cancelled after
// the order is fenced so a late worker cannot write new results.
func (h *Handler) CancelCreativeOrder(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative order cancellation")
		return
	}
	defer tx.Rollback(r.Context())

	var issueID pgtype.UUID
	var status, submissionKey string
	var allItemsAdopted bool
	if err := tx.QueryRow(r.Context(), `
SELECT o.issue_id,
  o.status,
  o.submission_key,
  COALESCE((SELECT count(*) > 0 AND bool_and(i.adopted_variant_id IS NOT NULL)
    FROM creative_order_item i WHERE i.order_id = o.id), false)
FROM creative_order o
WHERE o.id = $1 AND o.workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&issueID, &status, &submissionKey, &allItemsAdopted); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order")
		return
	}
	if allItemsAdopted {
		writeError(w, http.StatusConflict, "delivered creative order cannot be cancelled")
		return
	}

	if status != "cancelled" || submissionKey != "" {
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order SET status = 'cancelled', submission_key = '', updated_at = now()
WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel creative order")
			return
		}
		variantRows, err := tx.Query(r.Context(), `
SELECT variant.id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE item.order_id = $1
ORDER BY variant.id
FOR UPDATE OF variant
`, orderID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to lock creative order variants")
			return
		}
		for variantRows.Next() {
			var ignoredVariantID pgtype.UUID
			if err := variantRows.Scan(&ignoredVariantID); err != nil {
				variantRows.Close()
				writeError(w, http.StatusInternalServerError, "failed to read creative order variants")
				return
			}
		}
		if err := variantRows.Err(); err != nil {
			variantRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read creative order variants")
			return
		}
		variantRows.Close()
		if _, err := tx.Exec(r.Context(), `
WITH cancelled_variants AS (
  UPDATE creative_order_variant variant
  SET status = 'cancelled',
      brief = brief - 'prime_composition_pending',
      updated_at = now()
  FROM creative_order_item item
  WHERE variant.order_item_id = item.id AND item.order_id = $1
    AND variant.active_revision IS DISTINCT FROM variant.revision
  RETURNING variant.id, variant.revision, variant.brief
)
UPDATE creative_order_variant_revision revision
SET status = 'cancelled', brief = cancelled.brief, updated_at = now()
FROM cancelled_variants cancelled
WHERE revision.variant_id = cancelled.id AND revision.revision = cancelled.revision
`, orderID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel creative staging revisions")
			return
		}
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_prime_composition_job job
SET status = 'cancelled', lease_token = NULL, lease_expires_at = NULL,
    last_error = 'creative order cancelled', completed_at = now(), updated_at = now()
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE job.variant_id = variant.id
  AND item.order_id = $1 AND job.status IN ('queued', 'running', 'failed')
`, orderID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel creative Prime composition")
			return
		}
		if submissionKey != "" && issueID.Valid {
			if _, err := tx.Exec(r.Context(), `
UPDATE issue
SET metadata = jsonb_set(
  COALESCE(metadata, '{}'::jsonb) - 'creative_submission_key',
  '{creative_submission_history}',
  COALESCE(metadata->'creative_submission_history', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
    'submission_key', $3,
    'creative_order_id', $4::text,
    'terminal_status', 'cancelled',
    'retired_at', now()
  ))
)
WHERE id = $1
  AND workspace_id = $2
  AND metadata->>'creative_submission_key' = $3
`, issueID, workspaceID, submissionKey, orderID); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to retire creative order submission")
				return
			}
		}
	}
	if status != "cancelled" {
		details, err := json.Marshal(map[string]any{
			"creative_order_id": uuidToString(orderID),
			"previous_status":   status,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to record creative order cancellation")
			return
		}
		if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_order_cancelled', $4::jsonb)
`, workspaceID, issueID, userID, details); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to record creative order cancellation")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel creative order")
		return
	}

	if h.TaskService == nil {
		writeError(w, http.StatusInternalServerError, "creative order was closed but task cancellation is unavailable")
		return
	}
	rows, err := h.DB.Query(r.Context(), `
SELECT id
FROM agent_task_queue
WHERE context->>'creative_order_id' = $1::text
  AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
ORDER BY created_at, id
`, orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creative order was closed but active tasks could not be loaded")
		return
	}
	taskIDs := make([]pgtype.UUID, 0)
	for rows.Next() {
		var taskID pgtype.UUID
		if err := rows.Scan(&taskID); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read active creative order tasks")
			return
		}
		taskIDs = append(taskIDs, taskID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read active creative order tasks")
		return
	}
	rows.Close()
	for _, taskID := range taskIDs {
		if _, err := h.TaskService.CancelTask(r.Context(), taskID); err != nil {
			writeError(w, http.StatusInternalServerError, "creative order was closed but active tasks could not be cancelled")
			return
		}
	}

	order, err := scanCreativeOrder(h.DB.QueryRow(r.Context(), `
SELECT id::text, workspace_id::text, COALESCE(issue_id::text, ''), status, input_snapshot::text,
  trigger_evidence_kind, COALESCE(trigger_evidence_ref_id::text, ''), created_by::text, created_at::text, updated_at::text
FROM creative_order WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load cancelled creative order")
		return
	}
	if err := h.loadCreativeOrderWorkflowState(r, &order); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to derive creative order status")
		return
	}
	order.Items, err = h.listCreativeOrderItems(r, orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order items")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": chi.URLParam(r, "id"), "status": "cancelled",
	})
	writeJSON(w, http.StatusOK, order)
}

// DeleteCreativeOrder removes a stopped order and its order-domain records.
// Attached source and rendered files stay available because they can be shared
// with materials or other issue history.
func (h *Handler) DeleteCreativeOrder(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative order deletion")
		return
	}
	defer tx.Rollback(r.Context())

	var issueID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT issue_id
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&issueID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}

	var hasActiveTasks bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1
  FROM agent_task_queue
  WHERE context->>'creative_order_id' = $1::text
    AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, orderID).Scan(&hasActiveTasks); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check active creative order tasks")
		return
	}
	if hasActiveTasks {
		writeError(w, http.StatusConflict, "end the creative order before deleting it")
		return
	}

	if err := h.deleteCreativeOrderFeedback(r.Context(), tx, workspaceID, orderID); err != nil {
		slog.Error("failed to remove creative order feedback", "order_id", uuidToString(orderID), "workspace_id", uuidToString(workspaceID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to remove creative order feedback")
		return
	}
	if _, err := tx.Exec(r.Context(), `
DELETE FROM activity_log
WHERE workspace_id = $1
  AND details->>'creative_order_id' = $2::text
`, workspaceID, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove creative order activity")
		return
	}
	if _, err := tx.Exec(r.Context(), `
DELETE FROM creative_order
WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete creative order")
		return
	}
	if issueID.Valid {
		if _, err := tx.Exec(r.Context(), `
UPDATE issue
SET metadata = COALESCE(metadata, '{}'::jsonb) - 'workflow' - 'creative_order_id' - 'creative_submission_key',
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND metadata->>'creative_order_id' = $3::text
`, issueID, workspaceID, orderID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to unlink creative order issue")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete creative order")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": chi.URLParam(r, "id"), "deleted": true,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteCreativeOrderFeedback(ctx context.Context, tx pgx.Tx, workspaceID, orderID pgtype.UUID) error {
	// Undo events retain a restrictive self-reference to their original feedback.
	// Detach those references in the same statement before deleting the order's
	// feedback graph, including malformed legacy undo children without a copied
	// order id in their snapshot.
	_, err := tx.Exec(ctx, `
WITH target_feedback AS MATERIALIZED (
  SELECT feedback.id
  FROM creative_feedback_event AS feedback
  WHERE feedback.workspace_id = $1
    AND (
      feedback.context_snapshot->>'creative_order_id' = $2::text
      OR feedback.subject_id = $2::uuid
      OR feedback.subject_id IN (
        SELECT variant.id
        FROM creative_order_variant AS variant
        JOIN creative_order_item AS item ON item.id = variant.order_item_id
        WHERE item.order_id = $2::uuid
      )
      OR feedback.subject_id IN (
        SELECT asset.id
        FROM creative_order_asset AS asset
        JOIN creative_order_variant AS variant ON variant.id = asset.variant_id
        JOIN creative_order_item AS item ON item.id = variant.order_item_id
        WHERE item.order_id = $2::uuid
      )
    )
), detached_undo AS (
  UPDATE creative_feedback_event AS undo
  SET undo_of_id = NULL
  WHERE undo.undo_of_id IN (SELECT id FROM target_feedback)
)
DELETE FROM creative_feedback_event AS feedback
WHERE feedback.id IN (SELECT id FROM target_feedback)
`, workspaceID, orderID)
	return err
}

// AdoptCreativeOrderItemVariant selects the single delivery package for an
// order item. The item lock serializes competing human decisions, while the
// deferred composite foreign key also prevents cross-item adoption at commit.
func (h *Handler) AdoptCreativeOrderItemVariant(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	itemID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "itemId"), "order_item_id")
	if !ok {
		return
	}
	var input creativeOrderItemAdoptionInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order item adoption")
		return
	}
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.QCRiskReason = strings.TrimSpace(input.QCRiskReason)
	if len(input.QCRiskReason) > 1000 {
		writeError(w, http.StatusBadRequest, "QC risk adoption reason is too long")
		return
	}
	if input.QCRiskAcknowledged != (input.QCRiskReason != "") {
		writeError(w, http.StatusBadRequest, "QC risk acknowledgement and reason must be provided together")
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative variant adoption")
		return
	}
	defer tx.Rollback(r.Context())

	var issueID pgtype.UUID
	var orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT issue_id, status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&issueID, &orderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	var previousVariantID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT adopted_variant_id
FROM creative_order_item
WHERE id = $1 AND order_id = $2
FOR UPDATE
`, itemID, orderID).Scan(&previousVariantID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order item")
		return
	}

	var variantKey, variantStatus, variantBrief string
	var revision int
	var activeRevision pgtype.Int4
	if err := tx.QueryRow(r.Context(), `
SELECT variant_key, revision, status, brief::text, active_revision
FROM creative_order_variant
WHERE id = $1 AND order_item_id = $2
FOR UPDATE
`, variantID, itemID).Scan(&variantKey, &revision, &variantStatus, &variantBrief, &activeRevision); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order item")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order variant")
		return
	}
	if activeRevision.Valid {
		revision = int(activeRevision.Int32)
	}
	var revisionExpectedSizes []string
	revisionErr := tx.QueryRow(r.Context(), `
SELECT brief::text, status, expected_sizes
FROM creative_order_variant_revision
WHERE variant_id = $1 AND revision = $2
`, variantID, revision).Scan(&variantBrief, &variantStatus, &revisionExpectedSizes)
	if errors.Is(revisionErr, pgx.ErrNoRows) && activeRevision.Valid {
		writeError(w, http.StatusInternalServerError, "active creative variant revision is missing")
		return
	}
	if revisionErr != nil && !errors.Is(revisionErr, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load creative variant revision")
		return
	}
	var triggerKind, inputSnapshot string
	if err := tx.QueryRow(r.Context(), `
SELECT order_row.trigger_evidence_kind, order_row.input_snapshot::text
FROM creative_order_item item
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1 AND order_row.id = $2
`, itemID, orderID).Scan(&triggerKind, &inputSnapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative delivery scope")
		return
	}
	expectedSizes := revisionExpectedSizes
	if len(expectedSizes) == 0 {
		expectedSizes, err = expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(variantBrief))
	} else {
		expectedSizes, err = normalizeCreativeExpectedSizes(expectedSizes)
	}
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	var visualStatus, qcOutcome string
	if err := tx.QueryRow(r.Context(), `
WITH latest_resolution AS (
  SELECT attempt, outcome
  FROM creative_order_variant_qc_resolution
  WHERE variant_id = $1 AND revision = $2
  ORDER BY attempt DESC, created_at DESC
  LIMIT 1
)
SELECT
  COALESCE((SELECT status FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2 AND lane = 'visual' AND attempt = (SELECT attempt FROM latest_resolution)), ''),
  COALESCE((SELECT outcome FROM latest_resolution), '')
`, variantID, revision).Scan(&visualStatus, &qcOutcome); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative variant QC state")
		return
	}
	standardAdoption := creativeQCStatusAllowsAdoption(visualStatus) && qcOutcome == "delivered"
	riskAdoption := visualStatus == "failed" && creativeQCOutcomeAllowsRiskAdoption(qcOutcome)
	if standardAdoption {
		if input.QCRiskAcknowledged {
			writeError(w, http.StatusBadRequest, "QC risk acknowledgement is only valid for a failed QC result")
			return
		}
		if variantStatus != "completed" {
			writeError(w, http.StatusConflict, "creative variant is not completed")
			return
		}
	} else if riskAdoption {
		if !input.QCRiskAcknowledged {
			writeError(w, http.StatusConflict, "failed creative QC requires explicit risk acknowledgement and an adoption reason")
			return
		}
		if qcOutcome == "action_required" && variantStatus != "action_required" && !(activeRevision.Valid && variantStatus == "completed") {
			writeError(w, http.StatusConflict, "failed creative variant is not awaiting manual action")
			return
		}
		if qcOutcome == creativeQCOutcomeDeliveredWithRisk && variantStatus != "action_required" && variantStatus != "completed" {
			writeError(w, http.StatusConflict, "creative variant with QC risk is not awaiting adoption")
			return
		}
	} else {
		writeError(w, http.StatusConflict, "creative variant QC is not finalized for adoption")
		return
	}

	var qcReportsJSON, qcResolutionJSON string
	if err := tx.QueryRow(r.Context(), `
WITH latest_resolution AS (
  SELECT attempt, outcome, failure_summary, finalized_by_task_id
  FROM creative_order_variant_qc_resolution
  WHERE variant_id = $1 AND revision = $2
  ORDER BY attempt DESC, created_at DESC
  LIMIT 1
), latest_attempt AS (
  SELECT COALESCE(
    (SELECT attempt FROM latest_resolution),
    (SELECT max(attempt) FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2),
    1
  ) AS attempt
)
SELECT
  COALESCE((SELECT jsonb_agg(jsonb_build_object(
    'id', report.id::text, 'lane', report.lane, 'revision', report.revision, 'attempt', report.attempt,
    'status', report.status, 'findings', report.findings,
    'created_at', report.created_at, 'updated_at', report.updated_at
  ) ORDER BY lane)
  FROM creative_order_qc_report report, latest_attempt
  WHERE report.variant_id = $1 AND report.revision = $2 AND report.attempt = latest_attempt.attempt), '[]'::jsonb)::text,
  COALESCE((SELECT jsonb_build_object(
    'attempt', attempt, 'outcome', outcome, 'failure_summary', failure_summary,
    'finalized_by_task_id', COALESCE(finalized_by_task_id::text, '')
  ) FROM latest_resolution), '{}'::jsonb)::text
`, variantID, revision).Scan(&qcReportsJSON, &qcResolutionJSON); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to snapshot creative variant QC state")
		return
	}

	var missingPrimeSizes []string
	if err := tx.QueryRow(r.Context(), `
SELECT COALESCE(array_agg(required.size_key ORDER BY required.ordinality)
  FILTER (WHERE prime_asset.id IS NULL), '{}'::text[])
FROM unnest($3::text[]) WITH ORDINALITY AS required(size_key, ordinality)
LEFT JOIN LATERAL (
  SELECT asset.id
  FROM creative_order_asset asset
  WHERE asset.variant_id = $1
    AND asset.revision = $2
    AND asset.size_key = required.size_key
		AND asset.stage = 'primed'
		AND asset.status = 'completed'
		AND asset.attachment_id IS NOT NULL
  ORDER BY asset.updated_at DESC
  LIMIT 1
) prime_asset ON true
`, variantID, revision, expectedSizes).Scan(&missingPrimeSizes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative variant Prime package")
		return
	}
	if len(missingPrimeSizes) > 0 {
		writeError(w, http.StatusConflict, "creative variant Prime package is incomplete; missing primed assets for: "+strings.Join(missingPrimeSizes, ", "))
		return
	}

	if riskAdoption && !activeRevision.Valid {
		if _, err := copyCreativePrimedAssetsToDelivered(r.Context(), tx, variantID, revision, expectedSizes); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		if err := activateCreativeVariantRevision(r.Context(), tx, variantID, revision, expectedSizes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to activate acknowledged creative variant revision")
			return
		}
	} else {
		var missingSizes []string
		if err := tx.QueryRow(r.Context(), `
SELECT COALESCE(array_agg(required.size_key ORDER BY required.ordinality)
  FILTER (WHERE final_asset.id IS NULL), '{}'::text[])
FROM unnest($3::text[]) WITH ORDINALITY AS required(size_key, ordinality)
LEFT JOIN LATERAL (
  SELECT asset.id
  FROM creative_order_asset asset
  WHERE asset.variant_id = $1
    AND asset.revision = $2
    AND asset.size_key = required.size_key
		AND asset.stage = 'delivered'
		AND asset.status = 'completed'
		AND asset.attachment_id IS NOT NULL
  ORDER BY asset.updated_at DESC
  LIMIT 1
) final_asset ON true
`, variantID, revision, expectedSizes).Scan(&missingSizes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load creative variant delivery package")
			return
		}
		if len(missingSizes) > 0 {
			writeError(w, http.StatusConflict, "creative variant delivery package is incomplete; missing final assets for: "+strings.Join(missingSizes, ", "))
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order adoption time")
		return
	}

	if previousVariantID.Valid && previousVariantID == variantID {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to confirm creative variant adoption")
			return
		}
		item, err := h.loadCreativeOrderItem(r, itemID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load adopted creative order item")
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_item
SET adopted_variant_id = $2, adopted_at = now(), adopted_by = $3, updated_at = now()
WHERE id = $1
`, itemID, variantID, userID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to adopt creative variant")
		return
	}
	adoptionMode := "standard"
	reasonCodes := []string{}
	feedbackComment := ""
	if riskAdoption {
		adoptionMode = "qc_risk_accepted"
		reasonCodes = []string{"qc_risk_accepted"}
		feedbackComment = input.QCRiskReason
	}
	contextSnapshot, err := json.Marshal(map[string]any{
		"creative_order_id":           uuidToString(orderID),
		"creative_order_item_id":      uuidToString(itemID),
		"variant_id":                  input.VariantID,
		"variant_key":                 variantKey,
		"revision":                    revision,
		"previous_adopted_variant_id": uuidToString(previousVariantID),
		"delivery_package_sizes":      expectedSizes,
		"adoption_mode":               adoptionMode,
		"qc_risk_acknowledged":        riskAdoption,
		"qc_risk_reason":              feedbackComment,
		"qc_snapshot": map[string]any{
			"visual_status": visualStatus,
			"outcome":       qcOutcome,
			"reports":       json.RawMessage(qcReportsJSON),
			"resolution":    json.RawMessage(qcResolutionJSON),
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant adoption")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, decision, reason_codes, comment, context_snapshot
) VALUES ($1, $2, 'member', $3, 'variant', $4, 'decision', 'accepted', $5, $6, $7::jsonb)
`, workspaceID, issueID, userID, variantID, reasonCodes, feedbackComment, contextSnapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant feedback")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_variant_adopted', $4::jsonb)
`, workspaceID, issueID, userID, contextSnapshot); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant activity")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to adopt creative variant")
		return
	}

	item, err := h.loadCreativeOrderItem(r, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load adopted creative order item")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": chi.URLParam(r, "id"), "order_item_id": chi.URLParam(r, "itemId"),
	})
	writeJSON(w, http.StatusOK, item)
}

// UnadoptCreativeOrderItemVariant returns an item to review without removing
// its delivered assets, so a different ready variant can be selected later.
func (h *Handler) UnadoptCreativeOrderItemVariant(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	itemID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "itemId"), "order_item_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative variant unadoption")
		return
	}
	defer tx.Rollback(r.Context())

	var issueID, previousVariantID pgtype.UUID
	var orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT order_row.issue_id, order_row.status, item.adopted_variant_id
FROM creative_order_item item
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1
  AND item.order_id = $2
  AND order_row.workspace_id = $3
FOR UPDATE OF item, order_row
`, itemID, orderID, workspaceID).Scan(&issueID, &orderStatus, &previousVariantID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order item")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	if !previousVariantID.Valid {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to confirm creative variant unadoption")
			return
		}
		item, err := h.loadCreativeOrderItem(r, itemID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load creative order item")
			return
		}
		writeJSON(w, http.StatusOK, item)
		return
	}

	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_item
SET adopted_variant_id = NULL, adopted_at = NULL, adopted_by = NULL, updated_at = now()
WHERE id = $1
`, itemID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unadopt creative variant")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order")
		return
	}
	details, err := json.Marshal(map[string]any{
		"creative_order_id":      uuidToString(orderID),
		"creative_order_item_id": uuidToString(itemID),
		"previous_variant_id":    uuidToString(previousVariantID),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant unadoption")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, decision, context_snapshot
) VALUES ($1, $2, 'member', $3, 'variant', $4, 'decision', 'replaced', $5::jsonb)
`, workspaceID, issueID, userID, previousVariantID, details); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant feedback")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_variant_unadopted', $4::jsonb)
`, workspaceID, issueID, userID, details); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative variant activity")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to unadopt creative variant")
		return
	}
	item, err := h.loadCreativeOrderItem(r, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order item")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": chi.URLParam(r, "id"), "order_item_id": chi.URLParam(r, "itemId"),
	})
	writeJSON(w, http.StatusOK, item)
}

func creativeQCStatusAllowsAdoption(status string) bool {
	return status == "passed" || status == "warning"
}

func (h *Handler) UpsertCreativeOrderVariant(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativeOrderVariantInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order variant")
		return
	}
	input, err := normalizeCreativeOrderVariant(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	itemID, ok := parseUUIDOrBadRequest(w, input.OrderItemID, "order_item_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative order variant update")
		return
	}
	defer tx.Rollback(r.Context())
	taskActor := r.Header.Get("X-Actor-Source") == "task_token"
	var actorWorkflow, actorVariantID string
	var actorRevision int
	if taskActor {
		actorTaskID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
		if !ok {
			return
		}
		actorAgentID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Agent-ID"), "agent_id")
		if !ok {
			return
		}
		var actorEvidenceKind string
		var actorEvidenceRefID pgtype.UUID
		err := tx.QueryRow(r.Context(), `
SELECT COALESCE(task.context->>'workflow', ''), COALESCE(task.context->>'variant_id', ''),
       COALESCE(NULLIF(task.context->>'revision', '')::integer, 0),
       COALESCE(task.trigger_evidence_kind, ''), task.trigger_evidence_ref_id
FROM agent_task_queue task
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1 AND task.agent_id = $2
  AND assigned_agent.workspace_id = $3
  AND task.status IN ('dispatched', 'running')
  AND task.context->>'type' = 'creative_domain_task'
  AND task.context->>'creative_order_id' = $4::text
  AND task.context->>'creative_order_item_id' = $5::text
FOR UPDATE OF task
`, actorTaskID, actorAgentID, workspaceID, orderID, itemID).Scan(
			&actorWorkflow, &actorVariantID, &actorRevision, &actorEvidenceKind, &actorEvidenceRefID,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "creative task is cancelled or does not match this order item")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to lock creative task")
			return
		}
		validPlanningTask := actorWorkflow == "creative_plan" && actorEvidenceKind == "creative_order_item_plan" && actorEvidenceRefID == itemID && input.Revision == 1
		validProductionTask := actorWorkflow == "creative_production" && actorEvidenceKind == "creative_order_item_production" && actorEvidenceRefID == itemID && actorRevision == input.Revision
		if !validPlanningTask && !validProductionTask {
			writeError(w, http.StatusForbidden, "creative task cannot write this variant revision")
			return
		}
	}
	var orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&orderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	var itemDirection, inputSnapshot, triggerKind string
	if err := tx.QueryRow(r.Context(), `
SELECT item.direction, order_row.input_snapshot::text, order_row.trigger_evidence_kind
FROM creative_order_item item
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE item.id = $1 AND order_row.id = $2 AND order_row.workspace_id = $3
FOR UPDATE OF item
`, itemID, orderID, workspaceID).Scan(&itemDirection, &inputSnapshot, &triggerKind); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order item")
		return
	}
	var currentVariantID pgtype.UUID
	var currentRevision int
	var currentStatus, currentCandidateState, currentPrimarySize string
	var currentSelectionRank pgtype.Int4
	err = tx.QueryRow(r.Context(), `
SELECT id, revision, status, candidate_state, selection_rank, primary_size
FROM creative_order_variant
WHERE order_item_id = $1 AND variant_key = $2
FOR UPDATE
`, itemID, input.VariantKey).Scan(&currentVariantID, &currentRevision, &currentStatus, &currentCandidateState, &currentSelectionRank, &currentPrimarySize)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order variant")
		return
	}
	newVariant := errors.Is(err, pgx.ErrNoRows)
	if newVariant {
		currentRevision = 0
		currentStatus = ""
		currentCandidateState = "selected"
		currentPrimarySize = "1080x1080"
	}
	if taskActor && actorWorkflow == "creative_production" {
		if newVariant || actorVariantID != uuidToString(currentVariantID) {
			writeError(w, http.StatusForbidden, "creative production task does not match this variant")
			return
		}
	}
	input.Brief, err = bindCreativeOrderVariantFrozenContract(input.Brief, itemDirection, json.RawMessage(inputSnapshot))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if currentRevision == 0 && input.Revision != 1 {
		writeError(w, http.StatusConflict, "creative order variant rework limit exceeded")
		return
	}
	if currentRevision > 0 && input.Revision > currentRevision {
		maxRevision := creativeOrderVariantMaxRevision(triggerKind, currentRevision, currentStatus)
		if input.Revision > maxRevision || input.Revision > currentRevision+1 {
			writeError(w, http.StatusConflict, "creative order variant rework limit exceeded")
			return
		}
	}
	candidateState := input.CandidateState
	if candidateState == "" {
		candidateState = currentCandidateState
	}
	primarySize := input.PrimarySize
	if primarySize == "" {
		primarySize = currentPrimarySize
	}
	var selectionRank any
	if input.SelectionRank != nil {
		selectionRank = *input.SelectionRank
	} else if currentSelectionRank.Valid {
		selectionRank = currentSelectionRank.Int32
	}
	if triggerKind != "creative_direct_edit" {
		pipelineVersion := creativeOrderPipelineVersion(json.RawMessage(inputSnapshot))
		if pipelineVersion != creativePipelineCandidateV1 {
			writeError(w, http.StatusConflict, "standard creative orders require pipeline_version candidate_v1")
			return
		}
		counts, err := creativeOrderVariantCounts(json.RawMessage(inputSnapshot))
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		var variantCount, incompatibleCount int
		if err := tx.QueryRow(r.Context(), `
SELECT count(*), count(*) FILTER (
  WHERE candidate_state = 'selected' AND selection_rank IS NULL
)
FROM creative_order_variant
WHERE order_item_id = $1
`, itemID).Scan(&variantCount, &incompatibleCount); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate creative candidate pipeline")
			return
		}
		if incompatibleCount > 0 || !counts.allowsKey(input.VariantKey) {
			writeError(w, http.StatusConflict, "candidate key exceeds the order candidate count")
			return
		}
		if newVariant {
			if variantCount >= counts.Candidates || candidateState != "candidate" || input.SelectionRank != nil {
				writeError(w, http.StatusConflict, "new variants must be unranked candidates within the frozen candidate count")
				return
			}
		} else {
			if (input.CandidateState != "" && input.CandidateState != currentCandidateState) ||
				(input.SelectionRank != nil && (!currentSelectionRank.Valid || int(currentSelectionRank.Int32) != *input.SelectionRank)) {
				writeError(w, http.StatusConflict, "candidate state and ranking are managed by candidate selection")
				return
			}
			candidateState = currentCandidateState
			if currentSelectionRank.Valid {
				selectionRank = currentSelectionRank.Int32
			} else {
				selectionRank = nil
			}
		}
		rank := 0
		if currentSelectionRank.Valid {
			rank = int(currentSelectionRank.Int32)
		}
		if input.SelectionRank != nil {
			rank = *input.SelectionRank
		}
		if err := counts.validateRank(candidateState, rank); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
	}
	expectedSizes, err := expectedCreativeVariantProductionSizes(triggerKind, json.RawMessage(inputSnapshot), input.Brief, candidateState, primarySize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	variant, err := scanCreativeOrderVariant(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_variant (
  order_item_id, variant_key, brief, revision, status, candidate_state, selection_rank, primary_size, staging_revision
)
VALUES ($1,$2,$3::jsonb,$4,$5,$6,$7,$8,$4)
ON CONFLICT (order_item_id, variant_key) DO UPDATE SET
  brief = EXCLUDED.brief,
  revision = EXCLUDED.revision,
  status = EXCLUDED.status,
  candidate_state = EXCLUDED.candidate_state,
  selection_rank = EXCLUDED.selection_rank,
  primary_size = EXCLUDED.primary_size,
  staging_revision = CASE
    WHEN creative_order_variant.active_revision = EXCLUDED.revision AND EXCLUDED.status = 'completed' THEN NULL
    ELSE EXCLUDED.revision
  END,
  updated_at = now()
  WHERE creative_order_variant.revision < EXCLUDED.revision
     OR (
       creative_order_variant.revision = EXCLUDED.revision
       AND (
         creative_order_variant.status = EXCLUDED.status
         OR CASE creative_order_variant.status
              WHEN 'queued' THEN 0 WHEN 'running' THEN 1 WHEN 'partial' THEN 2 ELSE 3
            END
            < CASE EXCLUDED.status
                WHEN 'queued' THEN 0 WHEN 'running' THEN 1 WHEN 'partial' THEN 2 ELSE 3
              END
       )
     )
RETURNING id::text, order_item_id::text, variant_key, brief::text, revision, status,
  COALESCE(active_revision, 0), COALESCE(staging_revision, 0), candidate_state,
  COALESCE(selection_rank, 0), primary_size,
  EXISTS (
    SELECT 1
    FROM activity_log recovery
    JOIN creative_order_item recovery_item ON recovery_item.id = creative_order_variant.order_item_id
    JOIN creative_order recovery_order ON recovery_order.id = recovery_item.order_id
    WHERE recovery.workspace_id = recovery_order.workspace_id
      AND recovery.issue_id = recovery_order.issue_id
      AND recovery.action = 'creative_qc_recovery_queued'
      AND recovery.details->>'variant_id' = creative_order_variant.id::text
      AND recovery.details->>'revision' = creative_order_variant.revision::text
  ),
  false,
  created_at::text, updated_at::text
`, itemID, input.VariantKey, input.Brief, input.Revision, input.Status, candidateState, selectionRank, primarySize))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "creative order variant revision is stale")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order variant")
		return
	}
	if err := upsertCreativeVariantRevision(r.Context(), tx, parseUUID(variant.ID), input.Revision, input.Brief, input.Status, expectedSizes); err != nil {
		if errors.Is(err, errCreativeActiveRevisionImmutable) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to save creative order variant revision")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order variant")
		return
	}
	variant.QCStatus = "pending"
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, variant)
}

func creativeOrderVariantMaxRevision(triggerKind string, currentRevision int, currentStatus string) int {
	if triggerKind == "creative_direct_edit" {
		return 3
	}
	if currentRevision == 2 && (currentStatus == "action_required" || currentStatus == "failed") {
		return 3
	}
	return 2
}

func (h *Handler) UpsertCreativeOrderAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	input, err := decodeCreativeOrderAssetInput(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order asset")
		return
	}
	input, err = normalizeCreativeOrderAsset(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Stage != "generated" {
		writeError(w, http.StatusForbidden, "primed and delivered creative assets are registered by the platform")
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	attachmentID, ok := optionalUUIDOrBadRequest(w, input.AttachmentID, "attachment_id")
	if !ok {
		return
	}
	assetFamilyID, ok := optionalUUIDOrBadRequest(w, input.AssetFamilyID, "asset_family_id")
	if !ok {
		return
	}
	derivedFromAssetID, ok := optionalUUIDOrBadRequest(w, input.DerivedFromAssetID, "derived_from_asset_id")
	if !ok {
		return
	}
	operationID, ok := optionalUUIDOrBadRequest(w, input.OperationID, "operation_id")
	if !ok {
		return
	}
	taskActor := r.Header.Get("X-Actor-Source") == "task_token"
	var taskID pgtype.UUID
	if taskActor {
		taskID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
		if !ok {
			return
		}
	}
	var variantRevision int
	var triggerKind, inputSnapshot, brief, candidateState, primarySize string
	if err := h.DB.QueryRow(r.Context(), `
SELECT v.revision, o.trigger_evidence_kind, o.input_snapshot::text, v.brief::text, v.candidate_state, v.primary_size
FROM creative_order_variant v
JOIN creative_order_item i ON i.id = v.order_item_id
JOIN creative_order o ON o.id = i.order_id
WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
`, variantID, orderID, workspaceID).Scan(&variantRevision, &triggerKind, &inputSnapshot, &brief, &candidateState, &primarySize); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order variant")
		return
	}
	if input.Revision == 0 && input.revisionDefaulted {
		input.Revision = variantRevision
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative order asset revision is stale")
		return
	}
	expectedSizes, err := expectedCreativeVariantProductionSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief), candidateState, primarySize)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if !creativeSizeIsExpected(input.SizeKey, expectedSizes) {
		writeError(w, http.StatusUnprocessableEntity, "asset size is outside this creative variant's delivery scope")
		return
	}
	if input.Stage == "generated" && !operationID.Valid {
		writeError(w, http.StatusUnprocessableEntity, "generated creative assets require operation_id")
		return
	}
	var referencesValid bool
	err = h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_order_variant v
  JOIN creative_order_item i ON i.id = v.order_item_id
  JOIN creative_order o ON o.id = i.order_id
  WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
) AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM attachment WHERE id = $4 AND workspace_id = $3))
  AND ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM creative_order_asset WHERE id = $5 AND variant_id = $1))
  AND ($6::uuid IS NULL OR EXISTS(
    SELECT 1 FROM creative_image_operation operation
    WHERE operation.id = $6 AND operation.variant_id = $1
      AND operation.revision = $7 AND operation.size_key = $8
  ))
`, variantID, orderID, workspaceID, attachmentID, derivedFromAssetID, operationID, input.Revision, input.SizeKey).Scan(&referencesValid)
	if err != nil || !referencesValid {
		writeError(w, http.StatusUnprocessableEntity, "asset references do not belong to this creative order")
		return
	}
	if err := h.validateCreativeGeneratedAssetAttachmentDimensions(r.Context(), workspaceID, attachmentID, input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := validateCreativeGeneratedAssetNormalizationTarget(input); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative order asset update")
		return
	}
	defer tx.Rollback(r.Context())
	// Task-backed writes lock their authenticated queue row first. Order
	// cancellation and task cancellation therefore form a transaction boundary
	// around every canonical asset registration.
	if taskActor {
		var taskStatus, taskContextRaw string
		err := tx.QueryRow(r.Context(), `
SELECT task.status, task.context::text
FROM agent_task_queue task
JOIN agent ON agent.id = task.agent_id
WHERE task.id = $1
  AND agent.workspace_id = $2
  AND task.context->>'creative_order_id' = $3::uuid::text
  AND task.context->>'variant_id' = $4::uuid::text
  AND task.context->>'workflow' IN ('creative_production', 'creative_direct_edit')
  AND task.context->>'revision' = $5::int::text
FOR UPDATE OF task
`, taskID, workspaceID, orderID, variantID, input.Revision).Scan(&taskStatus, &taskContextRaw)
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnprocessableEntity, "creative order asset task does not match this operation scope")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate creative order asset task")
			return
		}
		if taskStatus != "dispatched" && taskStatus != "running" {
			writeError(w, http.StatusConflict, "creative order asset task is no longer active")
			return
		}
		if allowed, scopeErr := creativeTaskAllowsImageSize(json.RawMessage(taskContextRaw), input.SizeKey); scopeErr != nil || !allowed {
			writeError(w, http.StatusUnprocessableEntity, "creative order asset size is outside the task's current execution scope")
			return
		}
	}
	var lockedOrderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&lockedOrderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if lockedOrderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	var lockedRevision, lockedActiveRevision int
	var lockedVariantStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT variant.revision, COALESCE(variant.active_revision, 0), variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
FOR UPDATE OF variant
`, variantID, orderID).Scan(&lockedRevision, &lockedActiveRevision, &lockedVariantStatus); err != nil || lockedRevision != input.Revision {
		writeError(w, http.StatusConflict, "creative order asset revision is stale")
		return
	}
	if lockedVariantStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order variant is cancelled")
		return
	}
	if operationID.Valid && input.Stage == "generated" && input.Status == "completed" {
		var assetLineage struct {
			Quality         string `json:"quality"`
			Model           string `json:"model"`
			PromptSHA256    string `json:"prompt_sha256"`
			ProviderRequest string `json:"request_id"`
		}
		if err := json.Unmarshal(input.Metadata, &assetLineage); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "generated asset metadata lineage is invalid")
			return
		}
		var evidenceLineage struct {
			ModelResult  imagemodel.Settings `json:"model_result"`
			PromptSHA256 string              `json:"prompt_sha256"`
			RequestID    string              `json:"request_id"`
		}
		if err := json.Unmarshal(input.Evidence, &evidenceLineage); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "generated asset evidence lineage is invalid")
			return
		}
		assetLineage.PromptSHA256 = evidenceLineage.PromptSHA256
		assetLineage.ProviderRequest = evidenceLineage.RequestID
		if assetLineage.Quality == "" {
			assetLineage.Quality = evidenceLineage.ModelResult.Quality
		}

		var operationStatus, operationModel, operationPromptSHA256, providerRequestID, resultReceipt string
		var rawOutputAttachmentID, operationTaskID pgtype.UUID
		err := tx.QueryRow(r.Context(), `
SELECT status, model, prompt_sha256, provider_request_id, result_receipt::text, output_attachment_id, task_id
FROM creative_image_operation
WHERE id = $1 AND variant_id = $2 AND revision = $3 AND size_key = $4
FOR UPDATE
`, operationID, variantID, input.Revision, input.SizeKey).Scan(
			&operationStatus, &operationModel, &operationPromptSHA256, &providerRequestID, &resultReceipt, &rawOutputAttachmentID, &operationTaskID,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusConflict, "generated asset requires a completed image operation with provider receipt and raw output")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to validate creative image operation")
			return
		}
		var recordedSettings imagemodel.Settings
		if json.Unmarshal([]byte(resultReceipt), &recordedSettings) != nil || (recordedSettings.Quality != "" && recordedSettings.Quality != assetLineage.Quality) {
			writeError(w, http.StatusConflict, "generated asset quality does not match the image operation receipt")
			return
		}
		if operationStatus != "completed" || strings.TrimSpace(providerRequestID) == "" || resultReceipt == "{}" || !rawOutputAttachmentID.Valid {
			writeError(w, http.StatusConflict, "generated asset requires a completed image operation with provider receipt and raw output")
			return
		}
		if operationModel != assetLineage.Model || operationPromptSHA256 != assetLineage.PromptSHA256 || providerRequestID != assetLineage.ProviderRequest {
			writeError(w, http.StatusConflict, "generated asset lineage does not match the image operation")
			return
		}
		if taskActor {
			var taskDescendsFromOperation bool
			if !operationTaskID.Valid {
				writeError(w, http.StatusConflict, "generated asset image operation is not bound to a task lineage")
				return
			}
			if err := tx.QueryRow(r.Context(), `
WITH RECURSIVE task_lineage AS (
  SELECT id, parent_task_id, retry_of_task_id
  FROM agent_task_queue
  WHERE id = $1
  UNION
  SELECT parent.id, parent.parent_task_id, parent.retry_of_task_id
  FROM agent_task_queue parent
  JOIN task_lineage child
    ON parent.id = child.parent_task_id OR parent.id = child.retry_of_task_id
)
SELECT EXISTS (SELECT 1 FROM task_lineage WHERE id = $2)
`, taskID, operationTaskID).Scan(&taskDescendsFromOperation); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to validate generated asset task lineage")
				return
			}
			if !taskDescendsFromOperation {
				writeError(w, http.StatusConflict, "generated asset task lineage does not match the image operation")
				return
			}
		}
	}
	var asset creativeOrderAssetResponse
	if lockedActiveRevision == input.Revision {
		asset, err = scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
SELECT id::text, variant_id::text, asset_family_id::text, size_key, revision, stage,
  COALESCE(attachment_id::text, ''), COALESCE(derived_from_asset_id::text, ''),
  COALESCE(operation_id::text, ''), metadata::text, evidence::text, status,
  created_at::text, updated_at::text
FROM creative_order_asset
WHERE variant_id = $1 AND size_key = $3 AND revision = $4 AND stage = $5
  AND ($2::uuid IS NULL OR asset_family_id = $2)
  AND attachment_id IS NOT DISTINCT FROM $6::uuid
  AND derived_from_asset_id IS NOT DISTINCT FROM $7::uuid
  AND operation_id IS NOT DISTINCT FROM COALESCE($8::uuid, operation_id)
  AND metadata = $9::jsonb AND evidence = $10::jsonb AND status = $11
`, variantID, assetFamilyID, input.SizeKey, input.Revision, input.Stage, attachmentID, derivedFromAssetID, operationID, input.Metadata, input.Evidence, input.Status))
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "active creative revision assets are immutable")
			return
		}
	} else {
		asset, err = scanCreativeOrderAsset(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_asset (variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, operation_id, metadata, evidence, status)
VALUES ($1,COALESCE($2::uuid, gen_random_uuid()),$3,$4,$5,$6,$7,$8,$9::jsonb,$10::jsonb,$11)
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET asset_family_id = CASE
    WHEN creative_order_asset.stage = 'generated' AND creative_order_asset.status = 'completed'
      THEN creative_order_asset.asset_family_id
    ELSE EXCLUDED.asset_family_id
  END,
	attachment_id = EXCLUDED.attachment_id, derived_from_asset_id = EXCLUDED.derived_from_asset_id,
	operation_id = COALESCE(EXCLUDED.operation_id, creative_order_asset.operation_id),
	metadata = EXCLUDED.metadata, evidence = EXCLUDED.evidence, status = EXCLUDED.status, updated_at = now()
WHERE creative_order_asset.stage <> 'generated'
   OR creative_order_asset.status <> 'completed'
   OR (
	     creative_order_asset.attachment_id IS NOT DISTINCT FROM EXCLUDED.attachment_id
	     AND creative_order_asset.derived_from_asset_id IS NOT DISTINCT FROM EXCLUDED.derived_from_asset_id
	     AND creative_order_asset.operation_id IS NOT DISTINCT FROM COALESCE(EXCLUDED.operation_id, creative_order_asset.operation_id)
     AND creative_order_asset.metadata = EXCLUDED.metadata
     AND creative_order_asset.evidence = EXCLUDED.evidence
   )
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), COALESCE(operation_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variantID, assetFamilyID, input.SizeKey, input.Revision, input.Stage, attachmentID, derivedFromAssetID, operationID, input.Metadata, input.Evidence, input.Status))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "completed generated asset trace is immutable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order asset")
		return
	}
	if operationID.Valid && input.Stage == "generated" && input.Status == "completed" {
		tag, err := tx.Exec(r.Context(), `
UPDATE creative_image_operation
SET output_asset_id = $2, updated_at = now()
WHERE id = $1 AND variant_id = $3 AND revision = $4 AND size_key = $5
  AND status = 'completed'
  AND output_attachment_id IS NOT NULL
  AND provider_request_id <> ''
  AND result_receipt <> '{}'::jsonb
  AND (output_asset_id IS NULL OR output_asset_id = $2)
`, operationID, parseUUID(asset.ID), variantID, input.Revision, input.SizeKey)
		if err != nil || tag.RowsAffected() != 1 {
			writeError(w, http.StatusConflict, "creative image operation cannot accept this generated asset")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order asset")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, asset)
}

func (h *Handler) UpsertCreativeOrderDiagnosticAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativeOrderDiagnosticAssetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order diagnostic asset")
		return
	}
	input, err := normalizeCreativeOrderDiagnosticAsset(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Workflow == "brand_components" {
		writeError(w, http.StatusForbidden, "Prime composition evidence is registered by the platform")
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	attachmentID, ok := parseUUIDOrBadRequest(w, input.AttachmentID, "attachment_id")
	if !ok {
		return
	}
	taskID, ok := optionalUUIDOrBadRequest(w, input.TaskID, "task_id")
	if !ok {
		return
	}
	if input.Workflow == "creative_production" && !taskID.Valid {
		writeError(w, http.StatusUnprocessableEntity, "creative production process evidence requires task_id")
		return
	}
	productionEvidence := input.Workflow == "creative_production"
	var actorTaskID, actorAgentID pgtype.UUID
	if productionEvidence {
		if r.Header.Get("X-Actor-Source") != "task_token" {
			writeError(w, http.StatusForbidden, "creative production process evidence requires its active task token")
			return
		}
		actorTaskID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
		if !ok {
			return
		}
		actorAgentID, ok = parseUUIDOrBadRequest(w, r.Header.Get("X-Agent-ID"), "agent_id")
		if !ok {
			return
		}
		if actorTaskID != taskID {
			writeError(w, http.StatusForbidden, "creative production process evidence task_id must match the task token")
			return
		}
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative diagnostic asset update")
		return
	}
	defer tx.Rollback(r.Context())
	var itemID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT item.id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND item.order_id = $2
`, variantID, orderID).Scan(&itemID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resolve creative diagnostic variant")
		return
	}
	var issueID string
	if taskID.Valid {
		var taskStatus, taskEvidenceKind string
		var taskContext json.RawMessage
		var taskAgentID, taskEvidenceRefID pgtype.UUID
		if err := tx.QueryRow(r.Context(), `
SELECT task.status, task.context, task.agent_id,
       COALESCE(task.trigger_evidence_kind, ''), task.trigger_evidence_ref_id
FROM agent_task_queue task
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1
  AND assigned_agent.workspace_id = $2
  AND (task.issue_id IS NULL OR task.issue_id = (SELECT issue_id FROM creative_order WHERE id = $3))
  AND task.context->>'type' = 'creative_domain_task'
  AND task.context->>'creative_order_id' = $3::text
  AND task.context->>'creative_order_item_id' = $4::text
  AND task.context->>'variant_id' = $5::text
  AND COALESCE(NULLIF(task.context->>'revision', '')::int, $6) = $6
FOR UPDATE OF task
`, taskID, workspaceID, orderID, itemID, variantID, input.Revision).Scan(
			&taskStatus, &taskContext, &taskAgentID, &taskEvidenceKind, &taskEvidenceRefID,
		); errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnprocessableEntity, "task does not belong to this creative order variant")
			return
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to validate creative diagnostic task")
			return
		}
		if productionEvidence {
			if taskAgentID != actorAgentID || (taskStatus != "dispatched" && taskStatus != "running") {
				writeError(w, http.StatusConflict, "creative production task is no longer active")
				return
			}
			if taskEvidenceKind != "creative_order_item_production" || taskEvidenceRefID != itemID {
				writeError(w, http.StatusForbidden, "task cannot write creative production process evidence")
				return
			}
			var taskWorkflow string
			var taskContextObject map[string]json.RawMessage
			if json.Unmarshal(taskContext, &taskContextObject) != nil || json.Unmarshal(taskContextObject["workflow"], &taskWorkflow) != nil || taskWorkflow != "creative_production" {
				writeError(w, http.StatusForbidden, "task cannot write creative production process evidence")
				return
			}
			allowed, scopeErr := creativeTaskAllowsImageSize(taskContext, input.SizeKey)
			if scopeErr != nil || !allowed {
				writeError(w, http.StatusUnprocessableEntity, "creative production process evidence size is outside the current execution scope")
				return
			}
		}
	}
	var orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status, COALESCE(issue_id::text, '')
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&orderStatus, &issueID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative diagnostic order")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	var itemStatus string
	if err := tx.QueryRow(r.Context(), `SELECT status FROM creative_order_item WHERE id = $1 AND order_id = $2 FOR UPDATE`, itemID, orderID).Scan(&itemStatus); err != nil || itemStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order item is no longer writable")
		return
	}
	var variantRevision int
	var variantStatus string
	if err := tx.QueryRow(r.Context(), `
	SELECT variant.revision, variant.status
	FROM creative_order_variant variant
	JOIN creative_order_item item ON item.id = variant.order_item_id
	WHERE item.order_id = $1 AND variant.id = $2
	FOR UPDATE OF variant
	`, orderID, variantID).Scan(&variantRevision, &variantStatus); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	}
	if variantStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order variant is cancelled")
		return
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative order diagnostic asset revision is stale")
		return
	}
	var attachmentFilename string
	if err := tx.QueryRow(r.Context(), `
SELECT filename
FROM attachment
WHERE id = $1 AND workspace_id = $2
`, attachmentID, workspaceID).Scan(&attachmentFilename); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "attachment does not belong to this workspace")
		return
	}
	if input.Filename == "" {
		input.Filename = attachmentFilename
	}
	asset, err := scanCreativeOrderDiagnosticAsset(tx.QueryRow(r.Context(), `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb)
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO UPDATE SET
  task_id = EXCLUDED.task_id,
  attachment_id = EXCLUDED.attachment_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
RETURNING id::text, variant_id::text, COALESCE(task_id::text, ''), attachment_id::text,
  size_key, revision, workflow, label, filename, metadata::text, created_at::text, updated_at::text
`, variantID, taskID, attachmentID, input.SizeKey, input.Revision, input.Workflow, input.Label, input.Filename, input.Metadata))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative diagnostic asset")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative diagnostic asset")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, asset)
}

// PromoteCreativeOrderDiagnosticAsset adopts the completed Prime composition
// of a direct image edit. The composed asset is already the final result, so
// it replaces the active delivery package without another composition pass.
func (h *Handler) PromoteCreativeOrderDiagnosticAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	diagnosticAssetID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "assetId"), "diagnostic_asset_id")
	if !ok {
		return
	}
	requestedVariantID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "variantId"), "variant_id")
	if !ok {
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start diagnostic candidate promotion")
		return
	}
	defer tx.Rollback(r.Context())

	var candidate creativeOrderDiagnosticAsset
	var metadata string
	var variantID pgtype.UUID
	var variantRevision int
	var variantStatus, triggerKind, inputSnapshot, brief string
	var issueID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT diagnostic.id, diagnostic.variant_id, COALESCE(diagnostic.task_id::text, ''),
       diagnostic.attachment_id::text, diagnostic.size_key, diagnostic.revision,
       diagnostic.workflow, diagnostic.label, diagnostic.filename, diagnostic.metadata::text,
       diagnostic.created_at::text, diagnostic.updated_at::text, variant.revision, variant.status,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text,
       order_row.issue_id
FROM creative_order_diagnostic_asset diagnostic
JOIN creative_order_variant variant ON variant.id = diagnostic.variant_id
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE diagnostic.id = $1
  AND order_row.id = $2
  AND order_row.workspace_id = $3
FOR UPDATE OF diagnostic, variant
`, diagnosticAssetID, orderID, workspaceID).Scan(
		&candidate.ID, &candidate.VariantID, &candidate.TaskID, &candidate.AttachmentID,
		&candidate.SizeKey, &candidate.Revision, &candidate.Workflow, &candidate.Label,
		&candidate.Filename, &metadata, &candidate.CreatedAt, &candidate.UpdatedAt, &variantRevision,
		&variantStatus, &triggerKind, &inputSnapshot, &brief, &issueID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "creative repair candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load creative repair candidate")
		return
	}
	variantID = parseUUID(candidate.VariantID)
	candidate.Metadata = json.RawMessage(metadata)
	candidate.URL = attachmentDownloadPath(candidate.AttachmentID)
	if candidate.VariantID != uuidToString(requestedVariantID) {
		writeError(w, http.StatusUnprocessableEntity, "process image does not belong to this creative variant")
		return
	}
	if candidate.Workflow != "brand_components" || candidate.Label != "Prime 合成成图" {
		writeError(w, http.StatusConflict, "only a composed direct image-edit result can be adopted")
		return
	}
	delivery := parseCreativeDirectEditDeliveryConfig(json.RawMessage(brief))
	if delivery.Scope != "size" || delivery.TargetSize == "" || candidate.SizeKey != delivery.TargetSize {
		writeError(w, http.StatusConflict, "process image is outside the current direct image-edit scope")
		return
	}
	selectedMethod := creativeProcessImageAdoptionMethod(candidate)
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if !creativeSizeIsExpected(candidate.SizeKey, expectedSizes) {
		writeError(w, http.StatusUnprocessableEntity, "repair candidate size is outside this creative variant's delivery scope")
		return
	}

	// A completed direct adjustment can switch between its source and adjusted
	// delivery packages. Both revisions are immutable completed packages; only
	// active_revision changes, so this must not restart composition or QC.
	if variantStatus == "completed" {
		if candidate.Revision != variantRevision && candidate.Revision != delivery.SourceRevision {
			writeError(w, http.StatusConflict, "creative repair result is not available for switching")
			return
		}
		var completedRevisionStatus string
		var completedExpectedSizes []string
		var completedAt pgtype.Timestamptz
		if err := tx.QueryRow(r.Context(), `
SELECT status, expected_sizes, activated_at
FROM creative_order_variant_revision
WHERE variant_id = $1 AND revision = $2
FOR UPDATE
`, variantID, candidate.Revision).Scan(&completedRevisionStatus, &completedExpectedSizes, &completedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusConflict, "creative repair result is not available for switching")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load completed creative repair result")
			return
		}
		normalizedCompletedSizes, normalizeErr := normalizeCreativeExpectedSizes(completedExpectedSizes)
		completedSizeSet := make(map[string]struct{}, len(normalizedCompletedSizes))
		for _, size := range normalizedCompletedSizes {
			completedSizeSet[size] = struct{}{}
		}
		if normalizeErr != nil || completedRevisionStatus != "completed" || !completedAt.Valid || !creativeSizesMatchExpected(completedSizeSet, expectedSizes) {
			writeError(w, http.StatusConflict, "creative repair result does not have a completed delivery package")
			return
		}
		var deliveredCount int
		if err := tx.QueryRow(r.Context(), `
SELECT count(*)
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'delivered'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($3::text[])
`, variantID, candidate.Revision, expectedSizes).Scan(&deliveredCount); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to verify completed creative repair delivery")
			return
		}
		if deliveredCount != len(expectedSizes) {
			writeError(w, http.StatusConflict, "creative repair result does not have a completed delivery package")
			return
		}
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET status = 'completed', active_revision = $2, staging_revision = NULL, updated_at = now()
WHERE id = $1 AND revision = $3 AND status = 'completed'
`, variantID, candidate.Revision, variantRevision); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to switch completed creative repair result")
			return
		}
		if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update creative order after switching repair result")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to commit completed creative repair switch")
			return
		}
		h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
		writeJSON(w, http.StatusOK, creativeOrderDiagnosticPromotionResponse{
			Candidate: candidate, VariantID: uuidToString(variantID), Revision: candidate.Revision,
			SelectedMethod: selectedMethod, CompositionStarted: false,
		})
		return
	}

	var candidateState struct {
		Selected bool `json:"selected"`
	}
	_ = json.Unmarshal(candidate.Metadata, &candidateState)
	if (variantStatus != "action_required" && !(variantStatus == "running" && candidateState.Selected)) || candidate.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative repair candidate is not awaiting a selection")
		return
	}
	// A composed direct-edit result has already passed the deterministic Prime
	// step. It becomes the final delivery package directly, so stale QC state
	// and failed composition recovery cannot reopen this revision afterward.
	if _, err := tx.Exec(r.Context(), `
UPDATE agent_task_queue
SET context = COALESCE(context, '{}'::jsonb) || '{"superseded_by_process_result":true}'::jsonb
WHERE context->>'variant_id' = $1::text
  AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $2
`, variantID, candidate.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to supersede previous creative tasks")
		return
	}
	if _, err := tx.Exec(r.Context(), `
DELETE FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2
`, variantID, candidate.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC reports")
		return
	}
	if _, err := tx.Exec(r.Context(), `
DELETE FROM creative_order_variant_qc_resolution
WHERE variant_id = $1 AND revision = $2
`, variantID, candidate.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC resolution")
		return
	}
	var sourceAssetID, assetFamilyID pgtype.UUID
	var sourceRevision int
	if err := tx.QueryRow(r.Context(), `
SELECT id, asset_family_id, revision
FROM creative_order_asset
WHERE variant_id = $1
  AND size_key = $2
  AND revision = $3
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
FOR UPDATE
`, variantID, candidate.SizeKey, candidate.Revision).Scan(&sourceAssetID, &assetFamilyID, &sourceRevision); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusConflict, "composed adjustment result has no generated base")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load composed adjustment base")
		return
	}
	var generated creativeOrderAssetResponse
	var generatedMetadata, generatedEvidence string
	if err := tx.QueryRow(r.Context(), `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id,
  derived_from_asset_id, metadata, evidence, status
)
VALUES ($1,$2,$3,$4,'primed',$5,$6,$7::jsonb,$8::jsonb,'completed')
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET
  asset_family_id = EXCLUDED.asset_family_id,
  attachment_id = EXCLUDED.attachment_id,
  derived_from_asset_id = EXCLUDED.derived_from_asset_id,
  metadata = EXCLUDED.metadata,
  evidence = EXCLUDED.evidence,
  status = EXCLUDED.status,
  updated_at = now()
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision,
  stage, COALESCE(attachment_id::text, ''), COALESCE(derived_from_asset_id::text, ''),
  metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variantID, assetFamilyID, candidate.SizeKey, candidate.Revision, parseUUID(candidate.AttachmentID), sourceAssetID,
		json.RawMessage(fmt.Sprintf(`{"adopted_process_result":%q,"diagnostic_asset_id":%q}`, selectedMethod, uuidToString(diagnosticAssetID))),
		json.RawMessage(fmt.Sprintf(`{"adopted_process_result":%q,"diagnostic_asset_id":%q,"source_asset_id":%q}`, selectedMethod, uuidToString(diagnosticAssetID), uuidToString(sourceAssetID))),
	).Scan(&generated.ID, &generated.VariantID, &generated.AssetFamilyID, &generated.SizeKey, &generated.Revision,
		&generated.Stage, &generated.AttachmentID, &generated.DerivedFromAssetID, &generatedMetadata,
		&generatedEvidence, &generated.Status, &generated.CreatedAt, &generated.UpdatedAt); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to restore composed adjustment result")
		return
	}
	generated.Metadata = json.RawMessage(generatedMetadata)
	generated.Evidence = json.RawMessage(generatedEvidence)
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_diagnostic_asset
SET metadata = jsonb_set(metadata, '{selected}', 'false'::jsonb, true), updated_at = now()
WHERE variant_id = $1 AND revision = $2 AND size_key = $3
`, variantID, candidate.Revision, candidate.SizeKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to mark repair comparison candidates")
		return
	}
	selectedMetadata := json.RawMessage(fmt.Sprintf(`{"candidate_method":%q,"selected":true,"source_revision":%d,"source_asset_id":%q}`, selectedMethod, sourceRevision, uuidToString(sourceAssetID)))
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_diagnostic_asset
SET metadata = metadata || $2::jsonb, updated_at = now()
WHERE id = $1
	`, diagnosticAssetID, selectedMetadata); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record selected repair candidate")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_prime_composition_job
SET status = 'cancelled', completed_at = COALESCE(completed_at, now()), updated_at = now()
WHERE variant_id = $1 AND revision = $2 AND status <> 'completed'
`, variantID, candidate.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to stop obsolete Prime composition")
		return
	}
	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET brief = brief - 'brand_composition_error' - 'creative_qc_handoff_error' - 'creative_direct_edit_error' - 'prime_composition_pending',
    updated_at = now()
WHERE id = $1 AND revision = $2
`, variantID, candidate.Revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear replaced creative state")
		return
	}
	if _, err := copyCreativePrimedAssetsToDelivered(r.Context(), tx, variantID, candidate.Revision, expectedSizes); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := activateCreativeVariantRevision(r.Context(), tx, variantID, candidate.Revision, expectedSizes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to activate composed adjustment result")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order after candidate promotion")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to commit repair candidate promotion")
		return
	}

	candidate.Metadata = selectedMetadata
	h.notifyCreativeDirectAdjustmentDelivery(context.WithoutCancel(r.Context()), workspaceID, orderID, variantID, candidate.Revision, candidate.SizeKey)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, creativeOrderDiagnosticPromotionResponse{
		Candidate: candidate, Generated: generated, VariantID: uuidToString(variantID),
		Revision: candidate.Revision, SelectedMethod: selectedMethod, CompositionStarted: false,
	})
}

func creativeProcessImageAdoptionMethod(candidate creativeOrderDiagnosticAsset) string {
	return "direct_edit_composed_result"
}

func (h *Handler) UpsertCreativeOrderQC(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var raw json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order QC report")
		return
	}
	input, err := decodeCreativeOrderQCInput(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input, err = normalizeCreativeOrderQC(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	task, ok := h.creativeQCActiveTask(w, r, orderID, variantID, input.Revision, input.Lane)
	if !ok {
		return
	}
	attempt, err := creativeQCAttemptFromTask(task, input.Attempt)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	input.Attempt = attempt
	evidenceID, ok := optionalUUIDOrBadRequest(w, input.TriggerEvidenceReference, "trigger_evidence_ref_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative QC transaction")
		return
	}
	defer tx.Rollback(r.Context())

	var lockedTaskStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status
FROM agent_task_queue
WHERE id = $1 AND agent_id = $2
FOR UPDATE
`, task.ID, task.AgentID).Scan(&lockedTaskStatus); err != nil ||
		(lockedTaskStatus != "dispatched" && lockedTaskStatus != "running") {
		writeError(w, http.StatusForbidden, "task is not authorized to write this creative QC")
		return
	}
	var lockedOrderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&lockedOrderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if lockedOrderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	var variantRevision int
	if err := tx.QueryRow(r.Context(), `
SELECT v.revision
FROM creative_order_variant v
  JOIN creative_order_item i ON i.id = v.order_item_id
  JOIN creative_order o ON o.id = i.order_id
  WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
  FOR UPDATE OF v
`, variantID, orderID, workspaceID).Scan(&variantRevision); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative order QC revision is stale")
		return
	}
	var finalized bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_order_variant_qc_resolution
  WHERE variant_id = $1 AND revision = $2 AND attempt = $3
)
`, variantID, input.Revision, input.Attempt).Scan(&finalized); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check creative QC resolution")
		return
	}
	if finalized {
		writeError(w, http.StatusConflict, "creative order QC attempt is already finalized")
		return
	}
	if input.Lane == "visual" {
		qcSizes, sizeErr := creativeQCTaskExpectedSizes(task)
		if sizeErr != nil {
			writeError(w, http.StatusForbidden, sizeErr.Error())
			return
		}
		criticalFindings, findingsErr := creativePrimeCriticalReadabilityFindings(r.Context(), tx, variantID, input.Revision, qcSizes)
		if findingsErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to evaluate Prime readability evidence")
			return
		}
		input, findingsErr = mergeCreativePrimeCriticalReadabilityFailures(input, criticalFindings)
		if findingsErr != nil {
			writeError(w, http.StatusBadRequest, findingsErr.Error())
			return
		}
	}
	var qcID string
	err = tx.QueryRow(r.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, attempt, status, findings, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,$8)
ON CONFLICT (variant_id, lane, revision, attempt) DO UPDATE SET status = EXCLUDED.status, findings = EXCLUDED.findings,
  trigger_evidence_kind = EXCLUDED.trigger_evidence_kind, trigger_evidence_ref_id = EXCLUDED.trigger_evidence_ref_id, updated_at = now()
RETURNING id::text
`, variantID, input.Lane, input.Revision, input.Attempt, input.Status, input.Findings, input.TriggerEvidenceKind, evidenceID).Scan(&qcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order QC report")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order QC report")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, map[string]any{"id": qcID, "variant_id": input.VariantID, "lane": input.Lane, "revision": input.Revision, "attempt": input.Attempt, "status": input.Status})
}

// decodeCreativeOrderQCInput accepts both the API envelope and the direct
// qc-report.json shape emitted by the QC skill. The latter keeps the report's
// blocking_failures at the top level, so treating the whole report as findings
// preserves the structured rework contract instead of silently storing {}.
func decodeCreativeOrderQCInput(raw json.RawMessage) (creativeOrderQCInput, error) {
	var envelope map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &envelope) != nil || envelope == nil {
		return creativeOrderQCInput{}, errors.New("invalid creative order QC report")
	}
	// The task CLI may wrap qc-report.json in {"report": ...}. Keep the
	// request-scoped identifiers from the outer envelope, but otherwise accept
	// the same direct report shape as the API. qc_status is the read-model name
	// exposed by order responses and is equivalent to the write-side status.
	report := envelope
	if rawReport, hasReport := envelope["report"]; hasReport {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(rawReport, &nested); err != nil || nested == nil {
			return creativeOrderQCInput{}, errors.New("invalid creative order QC report")
		}
		report = nested
		for _, key := range []string{"variant_id", "lane", "revision", "attempt", "status", "qc_status", "trigger_evidence_kind", "trigger_evidence_ref_id"} {
			if _, present := report[key]; !present {
				if value, exists := envelope[key]; exists {
					report[key] = value
				}
			}
		}
	}
	if _, hasStatus := report["status"]; !hasStatus {
		if qcStatus, exists := report["qc_status"]; exists {
			report["status"] = qcStatus
		}
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return creativeOrderQCInput{}, errors.New("invalid creative order QC report")
	}
	var input creativeOrderQCInput
	if err := json.Unmarshal(encoded, &input); err != nil {
		return creativeOrderQCInput{}, errors.New("invalid creative order QC report")
	}
	if _, hasFindings := report["findings"]; !hasFindings {
		input.Findings = encoded
	}
	return input, nil
}

// FinalizeCreativeOrderQC is the only barrier between independently-run QC
// lanes and an externally visible delivery. It locks the task, order, then
// variant so cancellation and concurrent lanes serialize before activation.
func (h *Handler) FinalizeCreativeOrderQC(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	if r.Header.Get("X-Actor-Source") != "task_token" {
		writeError(w, http.StatusForbidden, "creative QC finalization requires a task token")
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	var input creativeOrderQCFinalizeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative QC finalization")
		return
	}
	input.VariantID = strings.TrimSpace(input.VariantID)
	if input.Revision < 1 {
		input.Revision = 1
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	task, ok := h.creativeQCActiveTask(w, r, orderID, variantID, input.Revision, "")
	if !ok {
		return
	}
	attempt, err := creativeQCAttemptFromTask(task, input.Attempt)
	if err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	input.Attempt = attempt

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative QC finalization")
		return
	}
	defer tx.Rollback(r.Context())

	var lockedTaskStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status
FROM agent_task_queue
WHERE id = $1 AND agent_id = $2
FOR UPDATE
`, task.ID, task.AgentID).Scan(&lockedTaskStatus); err != nil ||
		(lockedTaskStatus != "dispatched" && lockedTaskStatus != "running") {
		writeError(w, http.StatusForbidden, "task is not authorized to write this creative QC")
		return
	}
	var lockedOrderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&lockedOrderStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order")
		return
	}
	if lockedOrderStatus == "cancelled" {
		writeError(w, http.StatusConflict, errCreativeOrderCancelled.Error())
		return
	}
	var variantRevision int
	var variantKey, triggerKind, inputSnapshot, brief string
	var itemID, candidateID pgtype.UUID
	var issueID pgtype.UUID
	var createdBy pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT v.revision, v.variant_key, i.id, i.candidate_id, o.issue_id, o.created_by, o.trigger_evidence_kind, o.input_snapshot::text, v.brief::text
FROM creative_order_variant v
JOIN creative_order_item i ON i.id = v.order_item_id
JOIN creative_order o ON o.id = i.order_id
WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
FOR UPDATE OF v
`, variantID, orderID, workspaceID).Scan(&variantRevision, &variantKey, &itemID, &candidateID, &issueID, &createdBy, &triggerKind, &inputSnapshot, &brief); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative order QC revision is stale")
		return
	}
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	qcSizes, err := creativeDirectEditProcessingSizes(json.RawMessage(brief), expectedSizes)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	reportStatuses := map[string]string{}
	failureSummary := map[string]json.RawMessage{}
	visualHasBlockingFailure := false
	rows, err := tx.Query(r.Context(), `
SELECT lane, status, findings::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2 AND attempt = $3
FOR UPDATE
`, variantID, input.Revision, input.Attempt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative QC reports")
		return
	}
	for rows.Next() {
		var lane, status, findings string
		if err := rows.Scan(&lane, &status, &findings); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read creative QC reports")
			return
		}
		hasBlockingFailure, findingsErr := creativeQCFindingsHaveBlockingFailures(json.RawMessage(findings))
		if findingsErr != nil {
			rows.Close()
			writeError(w, http.StatusBadRequest, findingsErr.Error())
			return
		}
		reportStatuses[lane] = status
		failureSummary[lane] = json.RawMessage(findings)
		if lane == "visual" && (hasBlockingFailure || status == "failed") {
			visualHasBlockingFailure = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read creative QC reports")
		return
	}
	rows.Close()

	if visualFindings, ok := failureSummary["visual"]; ok {
		previousRows, queryErr := tx.Query(r.Context(), `
SELECT findings::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2 AND lane = 'visual' AND attempt < $3
ORDER BY attempt DESC
FOR UPDATE
`, variantID, input.Revision, input.Attempt)
		if queryErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load prior visual QC reports")
			return
		}
		conflicts := make([]creativeQCPrimeReinspectionConflict, 0)
		for previousRows.Next() {
			var previousFindings string
			if err := previousRows.Scan(&previousFindings); err != nil {
				previousRows.Close()
				writeError(w, http.StatusInternalServerError, "failed to read prior visual QC reports")
				return
			}
			conflicts = append(conflicts, creativeQCHardPrimeReinspectionConflicts(json.RawMessage(previousFindings), visualFindings)...)
		}
		if err := previousRows.Err(); err != nil {
			previousRows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read prior visual QC reports")
			return
		}
		previousRows.Close()
		if len(conflicts) > 0 {
			fencedFindings, mergeErr := mergeCreativeQCPrimeReinspectionConflicts(visualFindings, conflicts)
			if mergeErr != nil {
				writeError(w, http.StatusBadRequest, mergeErr.Error())
				return
			}
			if _, updateErr := tx.Exec(r.Context(), `
UPDATE creative_order_qc_report
SET status = 'failed', findings = $4::jsonb
WHERE variant_id = $1 AND revision = $2 AND attempt = $3 AND lane = 'visual'
`, variantID, input.Revision, input.Attempt, string(fencedFindings)); updateErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to fence contradictory visual QC")
				return
			}
			failureSummary["visual"] = fencedFindings
			reportStatuses["visual"] = "failed"
			visualHasBlockingFailure = true
		}
	}

	response := creativeOrderQCFinalizeResponse{
		VariantID:    input.VariantID,
		Revision:     input.Revision,
		Attempt:      input.Attempt,
		VisualStatus: reportStatuses["visual"],
		Outcome:      "pending",
	}
	if !creativeQCLanesComplete(reportStatuses) {
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finalize creative QC")
			return
		}
		response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
		writeJSON(w, http.StatusOK, response)
		return
	}

	outcome := "delivered"
	if visualHasBlockingFailure {
		outcome = "action_required"
	}
	var visualReworkFindings []creativeVisualModelReworkFinding
	visualReworkExhausted := false
	if outcome == "action_required" && reportStatuses["visual"] == "failed" {
		reworkAttempts, queuedErr := creativeVisualModelReworkAttemptCount(r.Context(), tx, variantID)
		if queuedErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to check creative visual rework limit")
			return
		}
		eligibleFindings, findingErr := creativeVisualModelReworkFindings(failureSummary["visual"], qcSizes)
		if findingErr == nil {
			reworkEnabled, settingsErr := creativeVisualReworkEnabled(r.Context(), tx, workspaceID)
			if settingsErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to load creative rework settings")
				return
			}
			if reworkAttempts < creativeVisualModelReworkMaxAttempts && reworkEnabled {
				visualReworkFindings = eligibleFindings
			} else if reworkAttempts >= creativeVisualModelReworkMaxAttempts {
				visualReworkExhausted = true
			}
		}
	}
	if outcome == "action_required" && len(visualReworkFindings) == 0 && !visualReworkExhausted {
		recoverableTaskIDs, recoveryErr := creativeRecoverableQCTaskIDs(r.Context(), tx, orderID, variantID, input.Revision, input.Attempt)
		if recoveryErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to check automatic creative QC recovery")
			return
		}
		retryEnabled, retryErr := creativeAutomaticRetryEnabled(r.Context(), tx, workspaceID)
		if retryErr != nil {
			writeError(w, http.StatusInternalServerError, "failed to load creative retry settings")
			return
		}
		automaticRecovery := retryEnabled && (len(recoverableTaskIDs) > 0 || creativeQCFindingsNeedAutomaticRecovery(failureSummary["visual"]))
		if automaticRecovery {
			currentTaskID := uuidToString(task.ID)
			seenTaskIDs := make(map[string]struct{}, len(recoverableTaskIDs)+1)
			for _, taskID := range recoverableTaskIDs {
				seenTaskIDs[taskID] = struct{}{}
			}
			if currentTaskID != "" {
				if _, seen := seenTaskIDs[currentTaskID]; !seen {
					recoverableTaskIDs = append(recoverableTaskIDs, currentTaskID)
				}
			}
			var recoveryAlreadyUsed bool
			if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM activity_log
  WHERE issue_id = $1 AND action = 'creative_qc_recovery_queued'
    AND details->>'variant_id' = $2::text
    AND details->>'revision' = $3::text
    AND COALESCE(details->>'automatic', 'false') = 'true'
)
`, issueID, variantID, strconv.Itoa(input.Revision)).Scan(&recoveryAlreadyUsed); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to check automatic creative QC recovery limit")
				return
			}
			if !recoveryAlreadyUsed && len(recoverableTaskIDs) > 0 {
				created, recoveryAttempt, recoveryErr := h.queueCreativeQCAutomaticRecovery(
					r.Context(), tx, workspaceID, userID, orderID, itemID, variantID, issueID,
					input.Revision, json.RawMessage(inputSnapshot), qcSizes, recoverableTaskIDs, creativeQCAutomaticContractRecoveryKind,
				)
				if recoveryErr != nil {
					writeError(w, http.StatusInternalServerError, recoveryErr.Error())
					return
				}
				if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
					writeError(w, http.StatusInternalServerError, "failed to update creative order after automatic QC recovery")
					return
				}
				if err := tx.Commit(r.Context()); err != nil {
					writeError(w, http.StatusInternalServerError, "failed to commit automatic creative QC recovery")
					return
				}
				for _, recoveryTask := range created {
					h.TaskService.NotifyTaskEnqueued(r.Context(), recoveryTask)
				}
				response.VisualStatus = "pending"
				response.Outcome = "pending"
				response.Attempt = recoveryAttempt
				response.Finalized = false
				response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
				h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
					"scope": "order", "order_id": chi.URLParam(r, "id"), "variant_id": input.VariantID,
					"revision": input.Revision, "attempt": recoveryAttempt, "recovery": "creative_qc_automatic",
				})
				writeJSON(w, http.StatusOK, response)
				return
			}
		}
	}
	failureSummaryJSON, err := json.Marshal(failureSummary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode creative QC findings")
		return
	}
	var resolutionCreated bool
	err = tx.QueryRow(r.Context(), `
INSERT INTO creative_order_variant_qc_resolution (
  variant_id, revision, attempt, outcome, finalized_by_task_id, issue_id, failure_summary
)
VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)
ON CONFLICT (variant_id, revision, attempt) DO NOTHING
RETURNING true
`, variantID, input.Revision, input.Attempt, outcome, task.ID, issueID, failureSummaryJSON).Scan(&resolutionCreated)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to resolve creative QC")
		return
	}
	if !resolutionCreated {
		if err := tx.QueryRow(r.Context(), `
SELECT outcome FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2 AND attempt = $3
`, variantID, input.Revision, input.Attempt).Scan(&response.Outcome); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative QC resolution")
			return
		}
		response.Finalized = true
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finalize creative QC")
			return
		}
		if visualReworkExhausted && response.Outcome == "action_required" {
			if _, _, err := h.maybePromoteCreativeReserve(
				r.Context(), variantID, creativeOrchestrationCause{ParentTask: &task, RequestedBy: userID},
			); err != nil {
				slog.Error("reconcile creative reserve after repeated exhausted QC finalization", "variant_id", input.VariantID, "revision", input.Revision, "error", err)
			}
		}
		response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
		writeJSON(w, http.StatusOK, response)
		return
	}

	response.Created = true
	response.Finalized = true
	response.Outcome = outcome
	var inbox db.InboxItem
	var shouldPublishInbox bool
	var reworkTask db.AgentTaskQueue
	if outcome == "action_required" {
		// A later blocking report can invalidate a revision that an earlier,
		// contradictory report activated. Keep its activation timestamp for
		// audit, but remove it from the active delivery pointer immediately.
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET status = 'action_required',
    active_revision = CASE WHEN active_revision = $2 THEN NULL ELSE active_revision END,
    staging_revision = $2,
    updated_at = now()
WHERE id = $1 AND revision = $2
`, variantID, input.Revision); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to require creative QC action")
			return
		}
		if err := markCreativeVariantRevisionActionRequired(
			r.Context(), tx, variantID, input.Revision, json.RawMessage(brief), expectedSizes,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to preserve creative QC revision state")
			return
		}
	}
	if creativeQCOutcomeCopiesDelivery(outcome) {
		count, err := copyCreativePrimedAssetsToDelivered(r.Context(), tx, variantID, input.Revision, expectedSizes)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		response.DeliveredAssetCount = count
		if outcome == "delivered" {
			if err := activateCreativeVariantRevision(r.Context(), tx, variantID, input.Revision, expectedSizes); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to activate creative variant revision")
				return
			}
		} else {
			if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET status = 'action_required', staging_revision = $2, updated_at = now()
WHERE id = $1 AND revision = $2
`, variantID, input.Revision); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to preserve risky creative revision for review")
				return
			}
			if err := upsertCreativeVariantRevision(
				r.Context(), tx, variantID, input.Revision, json.RawMessage(brief), "action_required", expectedSizes,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to preserve risky creative revision state")
				return
			}
		}
		shouldPublishInbox = true
	} else if len(visualReworkFindings) > 0 {
		reworkTask, err = h.queueCreativeVisualModelRework(
			r.Context(), tx, workspaceID, orderID, itemID, candidateID, variantID, issueID,
			json.RawMessage(inputSnapshot), input.Revision, expectedSizes, task, visualReworkFindings,
		)
		if err != nil {
			slog.Error("queue creative visual rework failed", "order_id", uuidToString(orderID), "variant_id", input.VariantID, "revision", input.Revision, "attempt", input.Attempt, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		response.ReworkTaskID = uuidToString(reworkTask.ID)
		response.ReworkRevision = input.Revision + 1
	} else {
		if visualReworkExhausted {
			exhaustedDetails, _ := json.Marshal(map[string]any{
				"creative_order_id": uuidToString(orderID),
				"variant_id":        uuidToString(variantID),
				"revision":          input.Revision,
				"attempt":           input.Attempt,
				"max_attempts":      creativeVisualModelReworkMaxAttempts,
				"outcome":           "action_required",
			})
			if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'agent', $3, 'creative_visual_rework_exhausted_action_required', $4::jsonb)
`, workspaceID, issueID, task.AgentID, exhaustedDetails); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to record exhausted creative visual rework")
				return
			}
		}
		shouldPublishInbox = true
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order after QC")
		return
	}

	if shouldPublishInbox {
		inboxDetails, _ := json.Marshal(map[string]any{
			"creative_order_id": uuidToString(orderID), "variant_id": input.VariantID,
			"revision": input.Revision, "attempt": input.Attempt, "outcome": outcome,
		})
		inbox, err = h.Queries.WithTx(tx).CreateInboxItem(r.Context(), db.CreateInboxItemParams{
			WorkspaceID:   workspaceID,
			RecipientType: "member",
			RecipientID:   createdBy,
			Type:          "creative_qc_" + outcome,
			Severity:      creativeQCInboxSeverity(outcome),
			IssueID:       issueID,
			Title:         creativeQCInboxTitle(variantKey, outcome),
			Body:          pgtype.Text{String: creativeQCInboxBody(variantKey, outcome, len(expectedSizes)), Valid: true},
			ActorType:     pgtype.Text{String: "agent", Valid: true},
			ActorID:       task.AgentID,
			Details:       inboxDetails,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to notify creative order owner")
			return
		}
		response.InboxItemID = uuidToString(inbox.ID)
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant_qc_resolution SET inbox_item_id = $3
WHERE variant_id = $1 AND revision = $2 AND attempt = $4
`, variantID, input.Revision, inbox.ID, input.Attempt); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to record creative QC notification")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finalize creative QC")
		return
	}
	if visualReworkExhausted {
		if _, _, err := h.maybePromoteCreativeReserve(
			r.Context(), variantID, creativeOrchestrationCause{ParentTask: &task, RequestedBy: userID},
		); err != nil {
			slog.Error("reconcile creative reserve after exhausted QC finalization", "variant_id", input.VariantID, "revision", input.Revision, "error", err)
		}
	}

	response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id"), "variant_id": input.VariantID, "revision": input.Revision, "attempt": input.Attempt})
	if reworkTask.ID.Valid && h.TaskService != nil {
		h.TaskService.NotifyTaskEnqueued(r.Context(), reworkTask)
	}
	if shouldPublishInbox {
		h.publishCreativeQCInbox(workspaceID, task.AgentID, inbox, response.OrderAggregateStatus)
	}
	writeJSON(w, http.StatusOK, response)
}

type creativeQCTaskContext struct {
	Type                string   `json:"type"`
	Workflow            string   `json:"workflow"`
	CreativeOrderID     string   `json:"creative_order_id"`
	CreativeOrderItemID string   `json:"creative_order_item_id"`
	VariantID           string   `json:"variant_id"`
	Revision            int      `json:"revision"`
	QCAttempt           int      `json:"qc_attempt"`
	ExpectedSizes       []string `json:"expected_sizes"`
}

func creativeQCTaskExpectedSizes(task db.AgentTaskQueue) ([]string, error) {
	var context creativeQCTaskContext
	if err := json.Unmarshal(task.Context, &context); err != nil {
		return nil, errors.New("creative QC task context is invalid")
	}
	sizes, err := normalizeCreativeExpectedSizes(context.ExpectedSizes)
	if err != nil {
		return nil, errors.New("creative QC task context expected_sizes is invalid")
	}
	return sizes, nil
}

func creativeQCAttemptFromContext(task db.AgentTaskQueue) int {
	attempt, err := creativeQCAttemptFromTask(task, 0)
	if err != nil {
		return defaultCreativeQCAttempt
	}
	return attempt
}

func (h *Handler) creativeQCVisualContextTask(w http.ResponseWriter, r *http.Request) (db.AgentTaskQueue, creativeQCTaskContext, bool) {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		writeError(w, http.StatusForbidden, "creative QC context requires a task token")
		return db.AgentTaskQueue{}, creativeQCTaskContext{}, false
	}
	taskID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
	if !ok {
		return db.AgentTaskQueue{}, creativeQCTaskContext{}, false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || uuidToString(task.AgentID) != r.Header.Get("X-Agent-ID") ||
		!task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != "creative_order_variant_qc" ||
		(task.Status != "dispatched" && task.Status != "running") {
		writeError(w, http.StatusForbidden, "task is not authorized to read this creative QC context")
		return db.AgentTaskQueue{}, creativeQCTaskContext{}, false
	}
	var taskContext creativeQCTaskContext
	if json.Unmarshal(task.Context, &taskContext) != nil || taskContext.Type != "creative_domain_task" ||
		taskContext.Workflow != "creative_qc_visual" || taskContext.Revision < 1 ||
		strings.TrimSpace(taskContext.CreativeOrderID) == "" || strings.TrimSpace(taskContext.VariantID) == "" {
		writeError(w, http.StatusForbidden, "creative QC task context is invalid")
		return db.AgentTaskQueue{}, creativeQCTaskContext{}, false
	}
	variantID, err := util.ParseUUID(taskContext.VariantID)
	if err != nil || !task.TriggerEvidenceRefID.Valid || task.TriggerEvidenceRefID != parseUUID(variantID.String()) {
		writeError(w, http.StatusForbidden, "task context is not authorized for this creative QC")
		return db.AgentTaskQueue{}, creativeQCTaskContext{}, false
	}
	return task, taskContext, true
}

// creativeQCActiveTask is the write boundary for an independent QC lane.
// A task token alone is insufficient: the queue row must still be active and
// its frozen context must exactly identify this order, variant, revision, and
// (for reports) lane. This prevents a late failed task from writing into a
// recovered QC run that deliberately reuses the same asset revision.
func (h *Handler) creativeQCActiveTask(w http.ResponseWriter, r *http.Request, orderID, variantID pgtype.UUID, revision int, lane string) (db.AgentTaskQueue, bool) {
	if r.Header.Get("X-Actor-Source") != "task_token" {
		writeError(w, http.StatusForbidden, "creative QC writes require a task token")
		return db.AgentTaskQueue{}, false
	}
	taskID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
	if !ok {
		return db.AgentTaskQueue{}, false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || uuidToString(task.AgentID) != r.Header.Get("X-Agent-ID") ||
		!task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != "creative_order_variant_qc" ||
		!task.TriggerEvidenceRefID.Valid || task.TriggerEvidenceRefID != variantID ||
		(task.Status != "dispatched" && task.Status != "running") {
		writeError(w, http.StatusForbidden, "task is not authorized to write this creative QC")
		return db.AgentTaskQueue{}, false
	}
	var taskContext creativeQCTaskContext
	if err := json.Unmarshal(task.Context, &taskContext); err != nil ||
		taskContext.Type != "creative_domain_task" ||
		taskContext.CreativeOrderID != uuidToString(orderID) ||
		taskContext.VariantID != uuidToString(variantID) ||
		taskContext.Revision != revision ||
		(taskContext.Workflow != "creative_qc_technical" && taskContext.Workflow != "creative_qc_visual") ||
		(lane != "" && taskContext.Workflow != "creative_qc_"+lane) {
		writeError(w, http.StatusForbidden, "task context is not authorized for this creative QC")
		return db.AgentTaskQueue{}, false
	}
	return task, true
}

func creativeQCAttemptFromTask(task db.AgentTaskQueue, requested int) (int, error) {
	if requested < 0 {
		return 0, errors.New("creative QC attempt must be a positive integer")
	}
	var taskContext struct {
		QCAttempt int `json:"qc_attempt"`
		Attempt   int `json:"attempt"`
	}
	if len(task.Context) > 0 {
		if err := json.Unmarshal(task.Context, &taskContext); err != nil {
			return 0, errors.New("creative QC task context is invalid")
		}
	}
	attempt := taskContext.QCAttempt
	if attempt < 1 {
		attempt = taskContext.Attempt
	}
	if attempt < 1 {
		attempt = defaultCreativeQCAttempt
	}
	if requested > 0 && requested != attempt {
		return 0, errors.New("creative QC attempt does not match the task context")
	}
	return attempt, nil
}

func creativeNextQCAttempt(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int) (int, error) {
	var nextAttempt int
	err := tx.QueryRow(ctx, `
SELECT GREATEST(
  COALESCE((SELECT max(attempt) FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2), 0),
  COALESCE((SELECT max(attempt) FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2), 0),
  COALESCE((
    SELECT max(
      COALESCE(
        CASE
          WHEN NULLIF(context->>'qc_attempt', '') ~ '^[0-9]+$'
            THEN (context->>'qc_attempt')::int
        END,
        1
      )
    )
    FROM agent_task_queue
    WHERE trigger_evidence_kind = 'creative_order_variant_qc'
      AND (
        trigger_evidence_ref_id = $1
        OR context->>'variant_id' = $1::text
      )
      AND COALESCE(NULLIF(context->>'revision', '')::int, 1) = $2
  ), 0)
) + 1
`, variantID, revision).Scan(&nextAttempt)
	if nextAttempt < 1 {
		nextAttempt = defaultCreativeQCAttempt
	}
	return nextAttempt, err
}

func creativeQCLanesComplete(statuses map[string]string) bool {
	return statuses["visual"] != "" && statuses["visual"] != "pending"
}

func copyCreativePrimedAssetsToDelivered(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string) (int, error) {
	rows, err := tx.Query(ctx, `
SELECT id, size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
  AND attachment_id IS NOT NULL
FOR UPDATE
`, variantID, revision)
	if err != nil {
		return 0, errors.New("failed to load Prime assets")
	}
	defer rows.Close()
	sizes := map[string]struct{}{}
	for rows.Next() {
		var ignoredID, size string
		if err := rows.Scan(&ignoredID, &size); err != nil {
			return 0, errors.New("failed to read Prime assets")
		}
		sizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return 0, errors.New("failed to read Prime assets")
	}
	if !creativeSizesMatchExpected(sizes, expectedSizes) {
		return 0, errors.New("completed Prime assets do not match the expected delivery sizes")
	}
	tag, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, revision, 'delivered', attachment_id, id, metadata, evidence, 'completed'
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
  AND attachment_id IS NOT NULL
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET
  asset_family_id = EXCLUDED.asset_family_id,
  attachment_id = EXCLUDED.attachment_id,
  derived_from_asset_id = EXCLUDED.derived_from_asset_id,
  metadata = EXCLUDED.metadata,
  evidence = EXCLUDED.evidence,
  status = EXCLUDED.status,
  updated_at = now()
`, variantID, revision)
	if err != nil {
		return 0, errors.New("failed to register delivered creative assets")
	}
	return int(tag.RowsAffected()), nil
}

func (h *Handler) queueCreativeVisualModelRework(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, orderID, itemID, candidateID, variantID, issueID pgtype.UUID,
	inputSnapshot json.RawMessage,
	sourceRevision int,
	expectedSizes []string,
	parentTask db.AgentTaskQueue,
	findings []creativeVisualModelReworkFinding,
) (db.AgentTaskQueue, error) {
	if h.TaskService == nil {
		return db.AgentTaskQueue{}, errors.New("creative production runtime is unavailable")
	}
	if !issueID.Valid {
		return db.AgentTaskQueue{}, errors.New("creative order is missing its root issue")
	}
	var variantBrief string
	if err := tx.QueryRow(ctx, `SELECT brief::text FROM creative_order_variant WHERE id = $1 FOR UPDATE`, variantID).Scan(&variantBrief); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to load creative visual rework contract")
	}
	directDelivery := parseCreativeDirectEditDeliveryConfig(json.RawMessage(variantBrief))
	if creativeOrderPipelineVersion(inputSnapshot) == creativePipelineDirectEditV1 && !directDelivery.FinalVisualValidation {
		return db.AgentTaskQueue{}, errors.New("direct image edit revision is missing its final visual validation contract")
	}
	if directDelivery.FinalVisualValidation {
		return h.queueCreativeDirectEditVisualRework(
			ctx, tx, workspaceID, orderID, itemID, candidateID, variantID, issueID,
			inputSnapshot, sourceRevision, expectedSizes, parentTask, findings, directDelivery,
		)
	}
	leaderID, _, reviewerID, err := creativeOrderProductionAgentSnapshot(inputSnapshot)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	preferredProducerID := selectedCreativeProductionAgentFromHistory(ctx, tx, workspaceID, variantID, sourceRevision)
	producer, err := h.selectCreativeImageEditAgent(ctx, tx, h.Queries.WithTx(tx), workspaceID, inputSnapshot, uuidToString(variantID), preferredProducerID, false)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	var producerCapable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill bound_skill ON bound_skill.id = binding.skill_id
  WHERE binding.agent_id = $1
    AND binding.enabled
    AND bound_skill.workspace_id = $2
    AND bound_skill.config->>'kind' = 'creative_role'
    AND bound_skill.config->>'capability' = 'image_edit'
)
	`, producer.ID, workspaceID).Scan(&producerCapable); err != nil {
		return db.AgentTaskQueue{}, err
	}
	if !producerCapable {
		return db.AgentTaskQueue{}, errors.New("the selected creative production agent lacks image_edit capability")
	}

	targetSizes := make([]string, 0, len(findings))
	for _, finding := range findings {
		targetSizes = append(targetSizes, finding.SizeKey)
	}
	rows, err := tx.Query(ctx, `
SELECT id, size_key
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($3::text[])
FOR UPDATE
`, variantID, sourceRevision, expectedSizes)
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to load generated assets for visual rework")
	}
	defer rows.Close()
	generatedSizes := map[string]struct{}{}
	for rows.Next() {
		var assetID, sizeKey string
		if err := rows.Scan(&assetID, &sizeKey); err != nil {
			return db.AgentTaskQueue{}, errors.New("failed to read generated assets for visual rework")
		}
		generatedSizes[sizeKey] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to read generated assets for visual rework")
	}
	if !creativeSizesMatchExpected(generatedSizes, expectedSizes) {
		return db.AgentTaskQueue{}, errors.New("visual rework requires a complete generated base package")
	}

	newRevision := sourceRevision + 1
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, $3, 'generated', attachment_id, id,
  metadata,
  evidence || jsonb_build_object('qc_visual_rework', jsonb_build_object('source_revision', $2::integer, 'reused_generated_base', true)),
  'completed'
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
  AND NOT (size_key = ANY($5::text[]))
ON CONFLICT (variant_id, size_key, revision, stage) DO NOTHING
	`, variantID, sourceRevision, newRevision, expectedSizes, targetSizes); err != nil {
		return db.AgentTaskQueue{}, fmt.Errorf("preserve passed generated assets for visual rework: %w", err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET revision = $2, status = 'running', staging_revision = $2, updated_at = now()
WHERE id = $1
`, variantID, newRevision); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to begin creative visual rework")
	}
	if err := upsertCreativeVariantRevision(ctx, tx, variantID, newRevision, json.RawMessage(variantBrief), "running", expectedSizes); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to create creative visual rework revision")
	}

	var compositionContract struct {
		PrimeComposition struct {
			Mode string `json:"mode"`
		} `json:"prime_composition"`
	}
	_ = json.Unmarshal([]byte(variantBrief), &compositionContract)
	strategies := make(map[string]string, len(findings))
	for _, finding := range findings {
		strategy := "local_reflow"
		if finding.Code == "official_prime_text_unreadable" {
			strategy = "prime_background_support"
		} else if compositionContract.PrimeComposition.Mode != primeCompositionModeModelIntegrated {
			var previouslyReflowed bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
  SELECT 1 FROM agent_task_queue WHERE context->>'variant_id'=$1
  AND context->'qc_visual_rework'->'size_strategies'->>$2 = 'local_reflow'
  AND (context->>'revision')::integer <= $3
)`, uuidToString(variantID), finding.SizeKey, sourceRevision).Scan(&previouslyReflowed); err != nil {
				return db.AgentTaskQueue{}, err
			}
			if previouslyReflowed {
				strategy = "background_expansion"
			}
		}
		strategies[finding.SizeKey] = strategy
	}
	context, err := imagemodel.MarshalTask(inputSnapshot, map[string]any{
		"type":                   "creative_domain_task",
		"workflow":               "creative_production",
		"scope":                  "variant",
		"subject_id":             uuidToString(variantID),
		"item_key":               fmt.Sprintf("%s:r%d", uuidToString(variantID), newRevision),
		"creative_order_id":      uuidToString(orderID),
		"creative_order_item_id": uuidToString(itemID),
		"candidate_id":           uuidToString(candidateID),
		"variant_id":             uuidToString(variantID),
		"revision":               newRevision,
		"expected_sizes":         expectedSizes,
		"issue_id":               uuidToString(issueID),
		"leader_agent_id":        uuidToString(leaderID),
		"producer_agent_id":      uuidToString(producer.ID),
		"producer_runtime_id":    uuidToString(producer.RuntimeID),
		"reviewer_agent_id":      uuidToString(reviewerID),
		"qc_visual_rework": map[string]any{
			"policy_version":  2,
			"source_revision": sourceRevision,
			"target_sizes":    targetSizes,
			"failures":        findings,
			"reflow_strategy": "targeted_rework_v2",
			"size_strategies": strategies,
		},
	})
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to encode creative visual rework task")
	}
	if err := validateCreativeTaskFanoutContext("creative_order_item_production", itemID, []service.DirectTaskFanoutItem{{
		ItemKey: fmt.Sprintf("%s:r%d", uuidToString(variantID), newRevision), Context: context,
	}}); err != nil {
		return db.AgentTaskQueue{}, err
	}
	task, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID:              producer.ID,
		RuntimeID:            producer.RuntimeID,
		Priority:             0,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     parentTask.RequestingUserID,
		OriginatorUserID:     parentTask.OriginatorUserID,
		AccountableUserID:    parentTask.AccountableUserID,
		OriginatorSource:     parentTask.OriginatorSource,
		DelegatedFromTaskID:  parentTask.ID,
		TriggerEvidenceKind:  pgtype.Text{String: "creative_order_item_production", Valid: true},
		TriggerEvidenceRefID: itemID,
		Context:              context,
	})
	if err != nil {
		return db.AgentTaskQueue{}, fmt.Errorf("failed to queue creative visual rework: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
SELECT variant_id, $6, attachment_id, size_key, $3, workflow, label, filename,
  metadata || jsonb_build_object(
    'reused_from_revision', $2::integer,
    'reused_for_qc_visual_rework', true
  )
FROM creative_order_diagnostic_asset
WHERE variant_id = $1
  AND revision = $2
  AND workflow = 'creative_production'
  AND size_key = ANY($4::text[])
  AND NOT (size_key = ANY($5::text[]))
  AND label = ANY(ARRAY['Prime context', '模型原图', '规范化底图']::text[])
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO UPDATE SET
  task_id = EXCLUDED.task_id,
  attachment_id = EXCLUDED.attachment_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
`, variantID, sourceRevision, newRevision, expectedSizes, targetSizes, task.ID); err != nil {
		return db.AgentTaskQueue{}, errors.New("preserve creative visual rework process evidence")
	}
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to encode creative visual rework findings")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
SELECT variant_id, $5, attachment_id, size_key, $3, 'qc_visual_rework', '视觉质检失败触发图',
  'qc-visual-failure-' || replace(size_key, 'x', '-') || '.png',
  jsonb_build_object(
    'source_revision', $2::integer,
    'qc_visual_rework', true,
    'model_input_policy', 'use_unbranded_generated_base_not_this_primed_image',
    'failures', $6::jsonb
  )
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'primed'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO UPDATE SET
  task_id = EXCLUDED.task_id,
  attachment_id = EXCLUDED.attachment_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
`, variantID, sourceRevision, newRevision, targetSizes, task.ID, findingsJSON); err != nil {
		return db.AgentTaskQueue{}, errors.New("record creative visual rework trigger evidence")
	}
	details, err := json.Marshal(map[string]any{
		"creative_order_id": uuidToString(orderID),
		"variant_id":        uuidToString(variantID),
		"source_revision":   sourceRevision,
		"revision":          newRevision,
		"task_id":           uuidToString(task.ID),
		"target_sizes":      targetSizes,
		"failures":          findings,
	})
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to record creative visual rework")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'agent', $3, 'creative_visual_rework_queued', $4::jsonb)
`, workspaceID, issueID, parentTask.AgentID, details); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to record creative visual rework")
	}
	return task, nil
}

// queueCreativeDirectEditVisualRework keeps an annotated adjustment on the
// direct-edit path after final Prime inspection. The model receives only the
// generated base as an editable input; the prior primed asset is a reference
// for the observed obstruction and the fixed template is composed again by
// the backend.
func (h *Handler) queueCreativeDirectEditVisualRework(
	ctx context.Context,
	tx pgx.Tx,
	workspaceID, orderID, itemID, candidateID, variantID, issueID pgtype.UUID,
	inputSnapshot json.RawMessage,
	sourceRevision int,
	expectedSizes []string,
	parentTask db.AgentTaskQueue,
	findings []creativeVisualModelReworkFinding,
	delivery creativeDirectEditDeliveryConfig,
) (db.AgentTaskQueue, error) {
	leaderID, reviewerID, err := creativeOrderQCAgentSnapshot(inputSnapshot)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	preferredEditorID := selectedCreativeDirectEditAgentFromHistory(ctx, tx, workspaceID, variantID, sourceRevision)
	editor, err := h.selectCreativeDirectImageEditAgent(ctx, tx, h.Queries.WithTx(tx), workspaceID, inputSnapshot, uuidToString(variantID), preferredEditorID, false)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}

	var editorCapable bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM agent_skill binding
  JOIN skill bound_skill ON bound_skill.id = binding.skill_id
  WHERE binding.agent_id = $1
    AND binding.enabled
    AND bound_skill.workspace_id = $2
    AND bound_skill.config->>'kind' = 'creative_role'
    AND bound_skill.config->>'capability' = 'direct_image_edit'
)
	`, editor.ID, workspaceID).Scan(&editorCapable); err != nil {
		return db.AgentTaskQueue{}, err
	}
	if !editorCapable {
		return db.AgentTaskQueue{}, errors.New("the selected direct image edit agent lacks direct_image_edit capability")
	}

	targetSizes := make([]string, 0, len(findings))
	for _, finding := range findings {
		targetSizes = append(targetSizes, finding.SizeKey)
	}
	rows, err := tx.Query(ctx, `
SELECT id::text, size_key, attachment_id::text
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($3::text[])
FOR UPDATE
	`, variantID, sourceRevision, expectedSizes)
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to load generated assets for direct visual rework")
	}
	defer rows.Close()
	generatedSizes := map[string]struct{}{}
	sourceAssets := make([]creativeOrderAdjustmentSourceAsset, 0, len(expectedSizes))
	sourceBySize := make(map[string]creativeOrderAdjustmentSourceAsset, len(expectedSizes))
	for rows.Next() {
		var source creativeOrderAdjustmentSourceAsset
		if err := rows.Scan(&source.AssetID, &source.SizeKey, &source.AttachmentID); err != nil {
			return db.AgentTaskQueue{}, errors.New("failed to read generated assets for direct visual rework")
		}
		generatedSizes[source.SizeKey] = struct{}{}
		sourceBySize[source.SizeKey] = source
	}
	if err := rows.Err(); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to read generated assets for direct visual rework")
	}
	if !creativeSizesMatchExpected(generatedSizes, expectedSizes) {
		return db.AgentTaskQueue{}, errors.New("direct visual rework requires a complete generated base package")
	}
	for _, size := range expectedSizes {
		sourceAssets = append(sourceAssets, sourceBySize[size])
	}
	if len(targetSizes) == 0 {
		return db.AgentTaskQueue{}, errors.New("direct visual rework has no failed sizes")
	}
	targetSize := targetSizes[0]
	targetSource := sourceBySize[targetSize]

	var referenceAssetID, referenceAttachmentID string
	if err := tx.QueryRow(ctx, `
SELECT id::text, attachment_id::text
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND size_key = $3
  AND stage = 'primed' AND status = 'completed' AND attachment_id IS NOT NULL
ORDER BY updated_at DESC, id DESC
LIMIT 1
`, variantID, sourceRevision, targetSize).Scan(&referenceAssetID, &referenceAttachmentID); err != nil {
		return db.AgentTaskQueue{}, errors.New("direct visual rework requires the inspected Prime reference")
	}

	newRevision := sourceRevision + 1
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, $3, 'generated', attachment_id, id,
  metadata,
  evidence || jsonb_build_object('qc_visual_rework', jsonb_build_object('source_revision', $2::integer, 'reused_generated_base', true, 'workflow', 'creative_direct_edit')),
  'completed'
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'generated'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
  AND NOT (size_key = ANY($5::text[]))
ON CONFLICT (variant_id, size_key, revision, stage) DO NOTHING
	`, variantID, sourceRevision, newRevision, expectedSizes, targetSizes); err != nil {
		return db.AgentTaskQueue{}, fmt.Errorf("preserve passed generated assets for direct visual rework: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_asset (
  variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status
)
SELECT variant_id, asset_family_id, size_key, $3, 'primed', attachment_id, id,
  metadata || jsonb_build_object('qc_visual_rework', jsonb_build_object('source_revision', $2::integer, 'reused_primed_asset', true, 'workflow', 'creative_direct_edit')),
  evidence || jsonb_build_object('qc_visual_rework', jsonb_build_object('source_revision', $2::integer, 'reused_primed_asset', true, 'workflow', 'creative_direct_edit')),
  'completed'
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'primed'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
  AND NOT (size_key = ANY($5::text[]))
ON CONFLICT (variant_id, size_key, revision, stage) DO NOTHING
	`, variantID, sourceRevision, newRevision, expectedSizes, targetSizes); err != nil {
		return db.AgentTaskQueue{}, fmt.Errorf("preserve passed Prime assets for direct visual rework: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET revision = $2, status = 'running', staging_revision = $2, updated_at = now()
WHERE id = $1
	`, variantID, newRevision); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to begin direct visual rework")
	}
	var newRevisionBrief string
	if err := tx.QueryRow(ctx, `SELECT brief::text FROM creative_order_variant WHERE id = $1`, variantID).Scan(&newRevisionBrief); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to load direct visual rework revision")
	}
	if err := upsertCreativeVariantRevision(ctx, tx, variantID, newRevision, json.RawMessage(newRevisionBrief), "running", expectedSizes); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to create direct visual rework revision")
	}

	scope := "size"
	if len(targetSizes) > 1 {
		scope = "variant"
	}
	rawUserRequest := delivery.RawUserRequest
	if rawUserRequest == "" {
		rawUserRequest = "解决最终 Prime 成图视觉验收发现的遮挡问题"
	}
	annotations := any([]any(nil))
	if len(delivery.Annotations) > 0 && string(delivery.Annotations) != "null" {
		if err := json.Unmarshal(delivery.Annotations, &annotations); err != nil {
			return db.AgentTaskQueue{}, errors.New("direct visual rework has invalid preserved annotations")
		}
	}
	if delivery.AnnotationGuideAttachmentID == "" {
		annotations = []any(nil)
	}
	context, err := imagemodel.MarshalTask(inputSnapshot, map[string]any{
		"type":                           "creative_domain_task",
		"workflow":                       "creative_direct_edit",
		"scope":                          scope,
		"subject_id":                     uuidToString(variantID),
		"item_key":                       fmt.Sprintf("%s:r%d", uuidToString(variantID), newRevision),
		"creative_order_id":              uuidToString(orderID),
		"creative_order_item_id":         uuidToString(itemID),
		"candidate_id":                   uuidToString(candidateID),
		"variant_id":                     uuidToString(variantID),
		"revision":                       newRevision,
		"expected_sizes":                 expectedSizes,
		"issue_id":                       uuidToString(issueID),
		"leader_agent_id":                uuidToString(leaderID),
		"reviewer_agent_id":              uuidToString(reviewerID),
		"direct_edit_agent_id":           uuidToString(editor.ID),
		"direct_edit_runtime_id":         uuidToString(editor.RuntimeID),
		"user_request":                   rawUserRequest,
		"raw_user_request":               rawUserRequest,
		"prompt_compilation":             "intent_normalization_required",
		"final_visual_validation":        true,
		"delivery_mode":                  "publish",
		"target_size":                    targetSize,
		"edit_sizes":                     targetSizes,
		"source_revision":                sourceRevision,
		"source_asset_id":                targetSource.AssetID,
		"source_attachment_id":           targetSource.AttachmentID,
		"source_assets":                  sourceAssets,
		"reference_asset_id":             referenceAssetID,
		"reference_attachment_id":        referenceAttachmentID,
		"annotation_guide_attachment_id": delivery.AnnotationGuideAttachmentID,
		"annotation_guide_source":        "final_reference",
		"qc_visual_rework": map[string]any{
			"policy_version":  2,
			"source_revision": sourceRevision,
			"target_sizes":    targetSizes,
			"failures":        findings,
			"reflow_strategy": "direct_edit_intent_reflow",
		},
		"direct_edit": map[string]any{
			"scope":                          scope,
			"source_revision":                sourceRevision,
			"target_size":                    targetSize,
			"expected_sizes":                 expectedSizes,
			"edit_sizes":                     targetSizes,
			"source_asset_id":                targetSource.AssetID,
			"source_attachment_id":           targetSource.AttachmentID,
			"source_assets":                  sourceAssets,
			"reference_asset_id":             referenceAssetID,
			"reference_attachment_id":        referenceAttachmentID,
			"direct_edit_agent_id":           uuidToString(editor.ID),
			"direct_edit_runtime_id":         uuidToString(editor.RuntimeID),
			"annotation_guide_attachment_id": delivery.AnnotationGuideAttachmentID,
			"annotation_guide_source":        "final_reference",
			"raw_user_request":               rawUserRequest,
			"prompt_compilation":             "intent_normalization_required",
			"final_visual_validation":        true,
			"annotations":                    annotations,
			"validation_rework": map[string]any{
				"source_revision": sourceRevision,
				"target_sizes":    targetSizes,
				"failures":        findings,
			},
		},
	})
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to encode direct visual rework task")
	}
	if err := validateCreativeTaskFanoutContext("creative_order_item_direct_edit", itemID, []service.DirectTaskFanoutItem{{
		ItemKey: fmt.Sprintf("%s:r%d", uuidToString(variantID), newRevision), Context: context,
	}}); err != nil {
		return db.AgentTaskQueue{}, err
	}
	task, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID:              editor.ID,
		RuntimeID:            editor.RuntimeID,
		Priority:             0,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     parentTask.RequestingUserID,
		OriginatorUserID:     parentTask.OriginatorUserID,
		AccountableUserID:    parentTask.AccountableUserID,
		OriginatorSource:     parentTask.OriginatorSource,
		DelegatedFromTaskID:  parentTask.ID,
		TriggerEvidenceKind:  pgtype.Text{String: "creative_order_item_direct_edit", Valid: true},
		TriggerEvidenceRefID: itemID,
		Context:              context,
	})
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to queue direct visual rework")
	}
	findingsJSON, err := json.Marshal(findings)
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to encode direct visual rework findings")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO creative_order_diagnostic_asset (
  variant_id, task_id, attachment_id, size_key, revision, workflow, label, filename, metadata
)
SELECT variant_id, $5, attachment_id, size_key, $3, 'creative_direct_edit', '最终贴片验收失败参考图',
  'direct-visual-failure-' || replace(size_key, 'x', '-') || '.png',
  jsonb_build_object(
    'source_revision', $2::integer,
    'qc_visual_rework', true,
    'model_input_policy', 'use_unbranded_generated_base_not_this_primed_image',
    'failures', $6::jsonb
  )
FROM creative_order_asset
WHERE variant_id = $1
  AND revision = $2
  AND stage = 'primed'
  AND status = 'completed'
  AND attachment_id IS NOT NULL
  AND size_key = ANY($4::text[])
ON CONFLICT (variant_id, revision, workflow, size_key, label, filename) DO UPDATE SET
  task_id = EXCLUDED.task_id,
  attachment_id = EXCLUDED.attachment_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
	`, variantID, sourceRevision, newRevision, targetSizes, task.ID, findingsJSON); err != nil {
		return db.AgentTaskQueue{}, errors.New("record direct visual rework trigger evidence")
	}
	details, err := json.Marshal(map[string]any{
		"creative_order_id": uuidToString(orderID),
		"variant_id":        uuidToString(variantID),
		"source_revision":   sourceRevision,
		"revision":          newRevision,
		"task_id":           uuidToString(task.ID),
		"target_sizes":      targetSizes,
		"failures":          findings,
		"workflow":          "creative_direct_edit",
	})
	if err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to record direct visual rework")
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'agent', $3, 'creative_direct_edit_visual_rework_queued', $4::jsonb)
	`, workspaceID, issueID, parentTask.AgentID, details); err != nil {
		return db.AgentTaskQueue{}, errors.New("failed to record direct visual rework")
	}
	return task, nil
}

func creativeQCInboxSeverity(outcome string) string {
	if outcome == "action_required" {
		return "action_required"
	}
	return "info"
}

func creativeQCOutcomeCopiesDelivery(outcome string) bool {
	return outcome == "delivered" || outcome == creativeQCOutcomeDeliveredWithRisk
}

func creativeQCOutcomeAllowsRiskAdoption(outcome string) bool {
	return outcome == "action_required" || outcome == creativeQCOutcomeDeliveredWithRisk
}

func creativeQCInboxTitle(variantKey, outcome string) string {
	if outcome == "action_required" {
		return "Creative QC needs a decision: " + variantKey
	}
	if outcome == creativeQCOutcomeDeliveredWithRisk {
		return "Creative variant ready with QC risk: " + variantKey
	}
	return "Creative variant ready: " + variantKey
}

func creativeQCInboxBody(variantKey, outcome string, expectedSizeCount int) string {
	if outcome == "action_required" {
		return "QC found a blocking issue for " + variantKey + ". Review the comparison and choose whether to revise or accept the risk."
	}
	if outcome == creativeQCOutcomeDeliveredWithRisk {
		return "Automatic visual rework reached its limit for " + variantKey + ". The current final assets are available to review or accept with QC risk."
	}
	if expectedSizeCount == 1 {
		return "The final asset for " + variantKey + " passed independent QC and is ready to review."
	}
	return "All three final sizes for " + variantKey + " passed independent QC and are ready to review."
}

func (h *Handler) publishCreativeQCInbox(workspaceID, agentID pgtype.UUID, item db.InboxItem, issueStatus string) {
	if h.Bus == nil {
		return
	}
	h.publish(protocol.EventInboxNew, uuidToString(workspaceID), "agent", uuidToString(agentID), map[string]any{"item": map[string]any{
		"id": uuidToString(item.ID), "workspace_id": uuidToString(item.WorkspaceID),
		"recipient_type": item.RecipientType, "recipient_id": uuidToString(item.RecipientID),
		"type": item.Type, "severity": item.Severity, "issue_id": uuidToString(item.IssueID),
		"title": item.Title, "body": item.Body.String, "read": item.Read, "archived": item.Archived,
		"created_at": item.CreatedAt.Time.Format(time.RFC3339Nano), "actor_type": item.ActorType.String,
		"actor_id": uuidToString(item.ActorID), "details": json.RawMessage(item.Details), "issue_status": issueStatus,
	}})
}

func (h *Handler) publishCreativeDomainUpdated(r *http.Request, workspaceID, userID pgtype.UUID, payload map[string]any) {
	if h.Bus == nil {
		return
	}
	actorType, actorID := h.resolveActor(r, uuidToString(userID), uuidToString(workspaceID))
	h.publish(protocol.EventCreativeMaterialsUpdated, uuidToString(workspaceID), actorType, actorID, payload)
}

func normalizeCreativeSourceAnalysis(input creativeSourceAnalysisInput) (creativeSourceAnalysisInput, error) {
	input.CandidateID = strings.TrimSpace(input.CandidateID)
	input.Status = strings.TrimSpace(input.Status)
	input.Summary = strings.TrimSpace(input.Summary)
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	input.ErrorMessage = strings.TrimSpace(input.ErrorMessage)
	input.TriggerEvidenceKind = strings.TrimSpace(input.TriggerEvidenceKind)
	input.TriggerEvidenceReference = strings.TrimSpace(input.TriggerEvidenceReference)
	if input.CandidateID == "" || input.AnalysisVersion < 1 || !validCreativeAnalysisStatus(input.Status) || len(input.Summary) > 4000 || len(input.ErrorMessage) > 4000 {
		return input, errors.New("invalid source analysis")
	}
	if (input.TriggerEvidenceKind == "") != (input.TriggerEvidenceReference == "") || (input.TriggerEvidenceKind != "" && input.TriggerEvidenceKind != "crawl_run") {
		return input, errors.New("source analysis evidence must be a crawl_run reference")
	}
	var err error
	input.Result, err = normalizedOptionalJSONObject(input.Result)
	if err != nil {
		return input, errors.New("result must be an object")
	}
	return input, nil
}

func normalizeCreativeOrder(input creativeOrderInput) (creativeOrderInput, error) {
	input.IssueID = strings.TrimSpace(input.IssueID)
	input.SubmissionKey = strings.TrimSpace(input.SubmissionKey)
	input.Status = strings.TrimSpace(input.Status)
	input.TriggerEvidenceKind = strings.TrimSpace(input.TriggerEvidenceKind)
	input.TriggerEvidenceReference = strings.TrimSpace(input.TriggerEvidenceReference)
	if !validCreativeOrderStatus(input.Status) || len(input.Items) == 0 || len(input.Items) > 100 || len(input.SubmissionKey) > 200 {
		return input, errors.New("invalid creative order")
	}
	var err error
	input.InputSnapshot, err = normalizedOptionalJSONObject(input.InputSnapshot)
	if err != nil {
		return input, errors.New("input_snapshot must be an object")
	}
	if _, found, err := creativeOrderSnapshotExpectedSizes(input.InputSnapshot); err != nil {
		return input, err
	} else if found {
		input.InputSnapshot, err = setCreativeOrderSnapshotExpectedSizes(input.InputSnapshot)
		if err != nil {
			return input, err
		}
	}
	seen := map[string]struct{}{}
	for index := range input.Items {
		item := &input.Items[index]
		item.SourceKind = strings.TrimSpace(item.SourceKind)
		if item.SourceKind == "" {
			item.SourceKind = "material"
		}
		item.CopyLibraryID = strings.TrimSpace(item.CopyLibraryID)
		item.CandidateID = strings.TrimSpace(item.CandidateID)
		item.SourceAnalysisID = strings.TrimSpace(item.SourceAnalysisID)
		item.Direction = strings.TrimSpace(item.Direction)
		if (item.SourceKind != "material" && item.SourceKind != "copy_library") ||
			(item.SourceKind == "material" && (item.CandidateID == "" || item.CopyLibraryID != "")) ||
			(item.SourceKind == "copy_library" && (item.CandidateID != "" || item.SourceAnalysisID != "" || item.CopyLibraryID == "")) {
			return input, errors.New("invalid creative order item")
		}
		if len(item.Direction) > maxCreativeOrderDirectionLength {
			return input, errors.New("visual direction exceeds the maximum supported length")
		}
		identity := item.SourceKind + ":" + item.CandidateID + item.CopyLibraryID
		if _, exists := seen[identity]; exists {
			return input, errors.New("duplicate candidate_id")
		}
		seen[identity] = struct{}{}
		item.CopySnapshot, err = normalizedOptionalJSONObject(item.CopySnapshot)
		if err != nil {
			return input, errors.New("copy_snapshot must be an object")
		}
	}
	if err := freezeCreativeOrderVariantCount(&input); err != nil {
		return input, err
	}
	return input, nil
}

func normalizeCreativeOrderVariant(input creativeOrderVariantInput) (creativeOrderVariantInput, error) {
	input.OrderItemID = strings.TrimSpace(input.OrderItemID)
	input.VariantKey = strings.TrimSpace(input.VariantKey)
	input.Status = strings.TrimSpace(input.Status)
	input.CandidateState = strings.TrimSpace(input.CandidateState)
	input.PrimarySize = strings.TrimSpace(input.PrimarySize)
	if input.Revision == 0 {
		input.Revision = 1
	}
	if input.OrderItemID == "" || input.VariantKey == "" || input.Revision < 1 || len(input.VariantKey) > 64 || !validCreativeVariantStatus(input.Status) ||
		(input.CandidateState != "" && !validCreativeCandidateState(input.CandidateState)) ||
		(input.PrimarySize != "" && !validCreativeAssetSize(input.PrimarySize)) ||
		(input.SelectionRank != nil && (*input.SelectionRank < 1 || *input.SelectionRank > creativeMaxCandidateCount)) {
		return input, errors.New("invalid creative order variant")
	}
	if input.SelectionRank != nil {
		if input.CandidateState == "candidate" || input.CandidateState == "rejected" {
			return input, errors.New("unranked creative candidate cannot have selection_rank")
		}
	}
	var err error
	input.Brief, err = normalizedOptionalJSONObject(input.Brief)
	if err != nil {
		return input, errors.New("brief must be an object")
	}
	return input, nil
}

func bindCreativeOrderVariantFrozenContract(rawBrief json.RawMessage, direction string, rawInputSnapshot json.RawMessage) (json.RawMessage, error) {
	var brief map[string]json.RawMessage
	if err := json.Unmarshal(rawBrief, &brief); err != nil || brief == nil {
		return nil, errors.New("brief must be an object")
	}
	contract := map[string]any{}
	if rawContract, ok := brief["creative_contract"]; ok && len(rawContract) > 0 && string(rawContract) != "null" {
		if err := json.Unmarshal(rawContract, &contract); err != nil || contract == nil {
			return nil, errors.New("brief creative_contract must be an object")
		}
	}
	direction = strings.TrimSpace(direction)
	contract["version"] = 1
	contract["parent_direction"] = direction
	contract["parent_direction_sha256"] = fmt.Sprintf("%x", sha256.Sum256([]byte(direction)))
	encodedContract, err := json.Marshal(contract)
	if err != nil {
		return nil, fmt.Errorf("encode brief creative_contract: %w", err)
	}
	brief["creative_contract"] = encodedContract
	primeLayout, found, err := frozenCreativePrimeLayoutContract(rawInputSnapshot)
	if err != nil {
		return nil, err
	}
	if found {
		brief["prime_layout_contract"] = primeLayout
		primeComposition, compositionFound, compositionErr := frozenCreativePrimeCompositionContract(rawInputSnapshot)
		if compositionErr != nil {
			return nil, compositionErr
		}
		if compositionFound {
			brief["prime_composition"] = primeComposition
		}
	}
	if creativeOrderPipelineVersion(rawInputSnapshot) == creativePipelineDirectEditV1 {
		var snapshot struct {
			TargetSize   string `json:"target_size"`
			DeliveryMode string `json:"delivery_mode"`
			UserRequest  string `json:"user_request"`
		}
		if err := json.Unmarshal(rawInputSnapshot, &snapshot); err != nil || !validCreativeAssetSize(strings.TrimSpace(snapshot.TargetSize)) {
			return nil, errors.New("direct image edit snapshot has an invalid target size")
		}
		snapshot.DeliveryMode = strings.TrimSpace(snapshot.DeliveryMode)
		if snapshot.DeliveryMode != "preview" && snapshot.DeliveryMode != "publish" {
			return nil, errors.New("direct image edit snapshot has an invalid delivery mode")
		}
		brief["mode"], _ = json.Marshal("direct_edit")
		brief["target_size"], _ = json.Marshal(strings.TrimSpace(snapshot.TargetSize))
		brief["delivery_mode"], _ = json.Marshal(snapshot.DeliveryMode)
		brief["user_request"], _ = json.Marshal(strings.TrimSpace(snapshot.UserRequest))
		delivery, err := json.Marshal(map[string]any{
			"final_visual_validation": true,
			"delivery_mode":           snapshot.DeliveryMode,
			"target_size":             strings.TrimSpace(snapshot.TargetSize),
			"scope":                   "size",
			"expected_sizes":          []string{strings.TrimSpace(snapshot.TargetSize)},
			"raw_user_request":        strings.TrimSpace(snapshot.UserRequest),
		})
		if err != nil {
			return nil, fmt.Errorf("encode direct image edit delivery contract: %w", err)
		}
		brief["creative_direct_edit_delivery"] = delivery
	}
	encodedBrief, err := json.Marshal(brief)
	if err != nil {
		return nil, fmt.Errorf("encode brief: %w", err)
	}
	return encodedBrief, nil
}

func frozenCreativePrimeCompositionContract(rawInputSnapshot json.RawMessage) (json.RawMessage, bool, error) {
	marketPack, templateSet, filesByRole, err := frozenCreativePrimeMarketPack(rawInputSnapshot)
	if err != nil {
		return nil, false, err
	}
	composition, err := parseFrozenPrimeCompositionConfig(marketPack.Config, templateSet)
	if err != nil {
		return nil, false, err
	}
	contract := map[string]any{"mode": composition.Mode}
	if composition.Mode == primeCompositionModeModelIntegrated {
		var selectedFamily *primeTemplateFamilyConfig
		for index := range templateSet.Families {
			if templateSet.Families[index].ID == composition.TemplateFamilyID {
				selectedFamily = &templateSet.Families[index]
				break
			}
		}
		if selectedFamily == nil {
			return nil, false, errors.New("frozen model-integrated Prime template family is unavailable")
		}
		sources := make(map[string]map[string]string, len(primeTemplateSizes))
		for _, size := range primeTemplateSizes {
			template := selectedFamily.Templates[size]
			file, found := filesByRole[template.SourceRole]
			if !found || strings.TrimSpace(file.AttachmentID) == "" {
				return nil, false, fmt.Errorf("frozen model-integrated Prime template %s is unavailable", template.SourceRole)
			}
			sources[size] = map[string]string{"source_role": template.SourceRole, "attachment_id": strings.TrimSpace(file.AttachmentID)}
		}
		contract["template_family_id"] = selectedFamily.ID
		contract["template_sources"] = sources
	}
	encoded, err := json.Marshal(contract)
	if err != nil {
		return nil, false, fmt.Errorf("encode frozen Prime composition contract: %w", err)
	}
	return encoded, true, nil
}

// frozenCreativePrimeLayoutContract returns the immutable per-size Prime layout
// from the order snapshot. Variant planning may choose visual execution, but it
// cannot decide where the fixed brand and compliance components are placed.
func frozenCreativePrimeLayoutContract(rawInputSnapshot json.RawMessage) (json.RawMessage, bool, error) {
	if len(rawInputSnapshot) == 0 || string(rawInputSnapshot) == "null" {
		return nil, false, nil
	}
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(rawInputSnapshot, &snapshot); err != nil || snapshot == nil {
		return nil, false, errors.New("creative order input snapshot must be an object")
	}
	marketPack, ok := snapshot["market_pack"]
	if !ok || len(marketPack) == 0 || string(marketPack) == "null" {
		return nil, false, nil
	}
	var marketPackValue map[string]json.RawMessage
	if err := json.Unmarshal(marketPack, &marketPackValue); err != nil || marketPackValue == nil {
		return nil, false, errors.New("creative order market pack snapshot must be an object")
	}
	config, ok := marketPackValue["config"]
	if !ok || len(config) == 0 || string(config) == "null" {
		return nil, false, errors.New("creative order market pack snapshot is missing config")
	}
	var configValue map[string]json.RawMessage
	if err := json.Unmarshal(config, &configValue); err != nil || configValue == nil {
		return nil, false, errors.New("creative order market pack config must be an object")
	}
	primeLayout, ok := configValue["prime_layout_contract"]
	if !ok || len(primeLayout) == 0 || string(primeLayout) == "null" {
		return nil, false, errors.New("creative order market pack config is missing prime_layout_contract")
	}
	var primeLayoutValue map[string]json.RawMessage
	if err := json.Unmarshal(primeLayout, &primeLayoutValue); err != nil || primeLayoutValue == nil {
		return nil, false, errors.New("creative order prime_layout_contract must be an object")
	}
	if _, ok := primeLayoutValue["layouts"]; !ok {
		return nil, false, errors.New("creative order prime_layout_contract is missing layouts")
	}
	return primeLayout, true, nil
}

func decodeCreativeOrderAssetInput(r *http.Request) (creativeOrderAssetInput, error) {
	var envelope map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil || envelope == nil {
		return creativeOrderAssetInput{}, errors.New("invalid creative order asset")
	}
	payload := json.RawMessage(nil)
	if nested, ok := envelope["asset"]; ok {
		var nestedObject map[string]json.RawMessage
		if err := json.Unmarshal(nested, &nestedObject); err != nil || nestedObject == nil {
			return creativeOrderAssetInput{}, errors.New("invalid creative order asset")
		}
		payload = nested
	} else {
		raw, err := json.Marshal(envelope)
		if err != nil {
			return creativeOrderAssetInput{}, err
		}
		payload = raw
	}
	var input creativeOrderAssetInput
	if err := json.Unmarshal(payload, &input); err != nil {
		return creativeOrderAssetInput{}, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return creativeOrderAssetInput{}, errors.New("invalid creative order asset")
	}
	if _, present := object["revision"]; !present {
		input.revisionDefaulted = true
	}
	// Some production runs were emitted with the semantic alias `kind` or
	// `asset_type` instead of the storage field `stage`. Accept those aliases
	// at the boundary, while all persisted responses remain canonical.
	if strings.TrimSpace(input.Stage) == "" {
		for _, key := range []string{"asset_type", "kind"} {
			raw, present := object[key]
			if !present {
				continue
			}
			var alias string
			if err := json.Unmarshal(raw, &alias); err == nil {
				input.Stage = strings.TrimSpace(alias)
				if input.Stage != "" {
					break
				}
			}
		}
	}
	// Generated attachments are terminal by definition. Older agents omitted
	// the status field after uploading the file; infer it only for that
	// compatibility shape, never for an explicit status value.
	if strings.TrimSpace(input.Status) == "" && strings.TrimSpace(input.Stage) == "generated" && strings.TrimSpace(input.AttachmentID) != "" {
		input.Status = "completed"
	}
	return input, nil
}

func normalizeCreativeOrderAsset(input creativeOrderAssetInput) (creativeOrderAssetInput, error) {
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.AssetFamilyID = strings.TrimSpace(input.AssetFamilyID)
	input.SizeKey = strings.TrimSpace(input.SizeKey)
	input.Size = strings.TrimSpace(input.Size)
	if input.SizeKey == "" {
		input.SizeKey = input.Size
	}
	input.Stage = strings.TrimSpace(input.Stage)
	input.AttachmentID = strings.TrimSpace(input.AttachmentID)
	input.DerivedFromAssetID = strings.TrimSpace(input.DerivedFromAssetID)
	input.OperationID = strings.TrimSpace(input.OperationID)
	input.Status = strings.TrimSpace(input.Status)
	if input.VariantID == "" || (input.Revision < 1 && !input.revisionDefaulted) || !validCreativeAssetSize(input.SizeKey) || !validCreativeAssetStage(input.Stage) || !validCreativeAssetStatus(input.Status) {
		return input, errors.New("invalid creative order asset")
	}
	var err error
	input.Metadata, err = normalizedOptionalJSONObject(input.Metadata)
	if err != nil {
		return input, errors.New("metadata must be an object")
	}
	input.Evidence, err = normalizedOptionalJSONObject(input.Evidence)
	if err != nil {
		return input, errors.New("evidence must be an object")
	}
	if input.Status == "completed" && input.AttachmentID == "" {
		return input, errors.New("completed creative order asset attachment_id is required")
	}
	if input.Stage == "generated" && input.Status == "completed" {
		if err := validateCompletedGeneratedAssetTrace(input.Metadata, input.Evidence); err != nil {
			return input, err
		}
	}
	return input, nil
}

func normalizeCreativeOrderDiagnosticAsset(input creativeOrderDiagnosticAssetInput) (creativeOrderDiagnosticAssetInput, error) {
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.TaskID = strings.TrimSpace(input.TaskID)
	input.AttachmentID = strings.TrimSpace(input.AttachmentID)
	input.SizeKey = strings.TrimSpace(input.SizeKey)
	input.Size = strings.TrimSpace(input.Size)
	if input.SizeKey == "" {
		input.SizeKey = input.Size
	}
	input.Workflow = strings.TrimSpace(input.Workflow)
	input.Label = strings.TrimSpace(input.Label)
	input.Filename = strings.TrimSpace(input.Filename)
	if input.Revision == 0 {
		input.Revision = 1
	}
	if input.VariantID == "" || input.AttachmentID == "" || input.Revision < 1 || !validCreativeAssetSize(input.SizeKey) || input.Workflow == "" || len(input.Workflow) > 80 || input.Label == "" || len(input.Label) > 120 {
		return input, errors.New("invalid creative order diagnostic asset")
	}
	if input.Workflow == "creative_production" && !slices.Contains(creativeProductionProcessLabels, input.Label) {
		return input, errors.New("creative production process label is not supported")
	}
	if input.Workflow == "brand_components" && input.Label != "Prime 合成成图" {
		return input, errors.New("brand component process label is not supported")
	}
	var err error
	input.Metadata, err = normalizedOptionalJSONObject(input.Metadata)
	if err != nil {
		return input, errors.New("metadata must be an object")
	}
	return input, nil
}

func validateCompletedGeneratedAssetTrace(metadata, evidence json.RawMessage) error {
	var metadataTrace struct {
		Quality           string  `json:"quality"`
		Prompt            string  `json:"prompt"`
		Model             string  `json:"model"`
		ActualWidth       int     `json:"actual_width"`
		ActualHeight      int     `json:"actual_height"`
		ActualAspectRatio float64 `json:"actual_aspect_ratio"`
	}
	if err := json.Unmarshal(metadata, &metadataTrace); err != nil {
		return errors.New("generated asset metadata is invalid")
	}
	if strings.TrimSpace(metadataTrace.Prompt) == "" {
		return errors.New("completed generated asset metadata.prompt is required")
	}
	if err := imagemodel.ValidateReceipt(metadataTrace.Model, metadataTrace.Quality); err != nil {
		return err
	}
	if metadataTrace.ActualWidth < 1 || metadataTrace.ActualHeight < 1 || metadataTrace.ActualAspectRatio <= 0 {
		return errors.New("completed generated asset metadata actual canvas is required")
	}
	var evidenceTrace struct {
		RequestID      string          `json:"request_id"`
		Attempts       int             `json:"attempts"`
		PromptSHA256   string          `json:"prompt_sha256"`
		ModelResult    json.RawMessage `json:"model_result"`
		PromptContract json.RawMessage `json:"prompt_contract"`
		Normalization  json.RawMessage `json:"normalization"`
	}
	if err := json.Unmarshal(evidence, &evidenceTrace); err != nil {
		return errors.New("generated asset evidence is invalid")
	}
	if strings.TrimSpace(evidenceTrace.RequestID) == "" {
		return errors.New("completed generated asset evidence.request_id is required")
	}
	if evidenceTrace.Attempts < 1 {
		return errors.New("completed generated asset evidence.attempts must be a positive integer")
	}
	wantHash := creativePromptSHA256(metadataTrace.Prompt)
	if evidenceTrace.PromptSHA256 != wantHash {
		return errors.New("completed generated asset evidence.prompt_sha256 does not match metadata.prompt")
	}
	var modelResult struct {
		Quality           string  `json:"quality"`
		Model             string  `json:"model"`
		Prompt            string  `json:"prompt"`
		PromptSHA256      string  `json:"prompt_sha256"`
		RequestID         string  `json:"request_id"`
		Attempts          int     `json:"attempts"`
		ActualWidth       int     `json:"actual_width"`
		ActualHeight      int     `json:"actual_height"`
		ActualAspectRatio float64 `json:"actual_aspect_ratio"`
	}
	if err := json.Unmarshal(evidenceTrace.ModelResult, &modelResult); err != nil {
		return errors.New("completed generated asset evidence.model_result is required")
	}
	var modelResultObject map[string]json.RawMessage
	if err := json.Unmarshal(evidenceTrace.ModelResult, &modelResultObject); err != nil || modelResultObject == nil {
		return errors.New("completed generated asset evidence.model_result is required")
	}
	if _, exists := modelResultObject["path"]; exists {
		return errors.New("completed generated asset evidence.model_result must not contain a local path")
	}
	if err := imagemodel.ValidateReceipt(modelResult.Model, modelResult.Quality); err != nil {
		return err
	}
	if (metadataTrace.Quality != "" && modelResult.Quality != metadataTrace.Quality) || modelResult.Model != metadataTrace.Model || modelResult.Prompt != metadataTrace.Prompt || modelResult.PromptSHA256 != evidenceTrace.PromptSHA256 || modelResult.RequestID != evidenceTrace.RequestID || modelResult.Attempts != evidenceTrace.Attempts || modelResult.ActualWidth != metadataTrace.ActualWidth || modelResult.ActualHeight != metadataTrace.ActualHeight || modelResult.ActualAspectRatio != metadataTrace.ActualAspectRatio {
		return errors.New("completed generated asset model result does not match metadata and evidence")
	}
	var promptContract struct {
		PromptSHA256 string `json:"prompt_sha256"`
	}
	if err := json.Unmarshal(evidenceTrace.PromptContract, &promptContract); err != nil || promptContract.PromptSHA256 != evidenceTrace.PromptSHA256 {
		return errors.New("completed generated asset evidence.prompt_contract does not match prompt_sha256")
	}
	var normalization struct {
		TargetSize map[string]json.RawMessage `json:"target_size"`
	}
	if err := json.Unmarshal(evidenceTrace.Normalization, &normalization); err != nil || len(normalization.TargetSize) == 0 {
		return errors.New("completed generated asset evidence.normalization.target_size is required")
	}
	return nil
}

func creativePromptSHA256(prompt string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))
}

func normalizeCreativeOrderQC(input creativeOrderQCInput) (creativeOrderQCInput, error) {
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.Lane = strings.TrimSpace(input.Lane)
	input.Status = strings.TrimSpace(input.Status)
	input.TriggerEvidenceKind = strings.TrimSpace(input.TriggerEvidenceKind)
	input.TriggerEvidenceReference = strings.TrimSpace(input.TriggerEvidenceReference)
	if input.Revision == 0 {
		input.Revision = 1
	}
	if input.VariantID == "" || input.Revision < 1 || input.Attempt < 0 || (input.Lane != "technical" && input.Lane != "visual") || !validCreativeQCStatus(input.Status) {
		return input, errors.New("invalid creative order QC report")
	}
	var err error
	input.Findings, err = normalizedOptionalJSONObject(input.Findings)
	if err != nil {
		return input, errors.New("findings must be an object")
	}
	blocking, err := creativeQCFindingsHaveBlockingFailures(input.Findings)
	if err != nil {
		return input, err
	}
	if blocking {
		input.Status = "failed"
	}
	return input, nil
}

func creativeQCFindingsHaveBlockingFailures(findings json.RawMessage) (bool, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(findings, &object); err != nil {
		return false, errors.New("findings must be an object")
	}
	raw, exists := object["blocking_failures"]
	if !exists || string(raw) == "null" {
		return false, nil
	}
	var failures []json.RawMessage
	if err := json.Unmarshal(raw, &failures); err != nil {
		return false, errors.New("findings.blocking_failures must be an array")
	}
	return len(failures) > 0, nil
}

func expectedCreativeVariantSizes(triggerKind string, inputSnapshot, brief json.RawMessage) ([]string, error) {
	if triggerKind != "creative_direct_edit" {
		if sizes, found, err := creativeOrderSnapshotExpectedSizes(inputSnapshot); err != nil {
			return nil, err
		} else if found {
			return sizes, nil
		}
		return append([]string(nil), standardCreativeAssetSizes...), nil
	}
	for _, raw := range []json.RawMessage{inputSnapshot, brief} {
		var scope creativeDeliveryScope
		if err := json.Unmarshal(raw, &scope); err != nil {
			return nil, errors.New("creative direct-edit delivery scope is invalid")
		}
		if len(scope.ExpectedSizes) > 0 {
			return normalizeCreativeExpectedSizes(scope.ExpectedSizes)
		}
		if strings.TrimSpace(scope.TargetSize) != "" {
			return normalizeCreativeExpectedSizes([]string{scope.TargetSize})
		}
	}
	return nil, errors.New("creative direct-edit delivery scope is missing")
}

func creativeOrderSnapshotExpectedSizes(raw json.RawMessage) ([]string, bool, error) {
	var snapshot struct {
		ExpectedSizes []string `json:"expected_sizes"`
		DeliveryScope struct {
			ExpectedSizes []string `json:"expected_sizes"`
		} `json:"delivery_scope"`
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, false, errors.New("creative order expected_sizes must be an array")
	}
	sizes := snapshot.ExpectedSizes
	if len(sizes) == 0 {
		sizes = snapshot.DeliveryScope.ExpectedSizes
	}
	if len(sizes) == 0 {
		return nil, false, nil
	}
	normalized, err := normalizeCreativeExpectedSizes(sizes)
	if err != nil {
		return nil, false, err
	}
	return normalized, true, nil
}

func setCreativeOrderSnapshotExpectedSizes(raw json.RawMessage) (json.RawMessage, error) {
	var snapshot map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snapshot); err != nil || snapshot == nil {
		return nil, errors.New("input_snapshot must be an object")
	}
	sizes, found, err := creativeOrderSnapshotExpectedSizes(raw)
	if err != nil || !found {
		return raw, err
	}
	encoded, err := json.Marshal(sizes)
	if err != nil {
		return nil, errors.New("failed to encode creative order expected_sizes")
	}
	snapshot["expected_sizes"] = encoded
	result, err := json.Marshal(snapshot)
	if err != nil {
		return nil, errors.New("failed to encode input_snapshot")
	}
	return result, nil
}

func normalizeCreativeExpectedSizes(sizes []string) ([]string, error) {
	if len(sizes) == 0 || len(sizes) > len(standardCreativeAssetSizes) {
		return nil, errors.New("creative direct-edit delivery scope is invalid")
	}
	normalized := make([]string, 0, len(sizes))
	seen := make(map[string]struct{}, len(sizes))
	for _, size := range sizes {
		size = strings.TrimSpace(size)
		if !validCreativeAssetSize(size) {
			return nil, errors.New("creative direct-edit delivery scope is invalid")
		}
		if _, exists := seen[size]; exists {
			return nil, errors.New("creative direct-edit delivery scope contains duplicate sizes")
		}
		seen[size] = struct{}{}
		normalized = append(normalized, size)
	}
	return normalized, nil
}

func creativeSizeIsExpected(size string, expectedSizes []string) bool {
	for _, expected := range expectedSizes {
		if size == expected {
			return true
		}
	}
	return false
}

func creativeSizesMatchExpected(actual map[string]struct{}, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	for _, size := range expected {
		if _, exists := actual[size]; !exists {
			return false
		}
	}
	return true
}

func validCreativeAnalysisStatus(status string) bool {
	return status == "pending" || status == "running" || status == "completed" || status == "failed"
}

func (h *Handler) requireCreativeOrderWritable(w http.ResponseWriter, r *http.Request, orderID, workspaceID pgtype.UUID) bool {
	var status string
	err := h.DB.QueryRow(r.Context(), `
SELECT status FROM creative_order WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order not found")
		return false
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order")
		return false
	}
	if status == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return false
	}
	return true
}

func validCreativeOrderStatus(status string) bool {
	return status == "draft" || status == "queued" || status == "running" || status == "partial" || status == "completed" || status == "failed" || status == "action_required" || status == "cancelled"
}

func validCreativeVariantStatus(status string) bool {
	return status == "queued" || status == "running" || status == "partial" || status == "completed" || status == "failed" || status == "action_required" || status == "cancelled"
}

func validCreativeAssetSize(size string) bool {
	return size == "1080x1080" || size == "1200x628" || size == "800x1000"
}

func creativeAssetSizeDimensions(size string) (int, int, bool) {
	switch size {
	case "1080x1080":
		return 1080, 1080, true
	case "1200x628":
		return 1200, 628, true
	case "800x1000":
		return 800, 1000, true
	default:
		return 0, 0, false
	}
}

func (h *Handler) validateCreativeGeneratedAssetAttachmentDimensions(ctx context.Context, workspaceID, attachmentID pgtype.UUID, input creativeOrderAssetInput) error {
	if h.Storage == nil || !attachmentID.Valid || input.Stage != "generated" || input.Status != "completed" {
		return nil
	}
	wantWidth, wantHeight, ok := creativeAssetSizeDimensions(input.SizeKey)
	if !ok {
		return nil
	}
	attachment, err := h.Queries.GetAttachmentByIDOnly(ctx, attachmentID)
	if err != nil || attachment.WorkspaceID != workspaceID {
		return errors.New("completed generated asset attachment is unavailable")
	}
	if !strings.HasPrefix(strings.ToLower(attachment.ContentType), "image/") {
		return errors.New("completed generated asset attachment must be an image")
	}
	key := h.Storage.KeyFromURL(attachment.Url)
	if key == "" {
		return errors.New("completed generated asset attachment storage key is unavailable")
	}
	reader, err := h.Storage.GetReader(ctx, key)
	if err != nil {
		return errors.New("completed generated asset attachment is unavailable")
	}
	defer reader.Close()
	config, _, err := image.DecodeConfig(io.LimitReader(reader, maxCreativePrimeInputBytes+1))
	if err != nil {
		return errors.New("completed generated asset attachment dimensions are unavailable")
	}
	if config.Width != wantWidth || config.Height != wantHeight {
		return fmt.Errorf("completed generated asset attachment dimensions %dx%d do not match %s", config.Width, config.Height, input.SizeKey)
	}
	return nil
}

func validateCreativeGeneratedAssetNormalizationTarget(input creativeOrderAssetInput) error {
	if input.Stage != "generated" || input.Status != "completed" {
		return nil
	}
	wantWidth, wantHeight, ok := creativeAssetSizeDimensions(input.SizeKey)
	if !ok {
		return nil
	}
	var evidence struct {
		Normalization struct {
			TargetSize struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"target_size"`
		} `json:"normalization"`
	}
	if err := json.Unmarshal(input.Evidence, &evidence); err != nil || evidence.Normalization.TargetSize.Width < 1 || evidence.Normalization.TargetSize.Height < 1 {
		return errors.New("completed generated asset normalization target_size is invalid")
	}
	if evidence.Normalization.TargetSize.Width != wantWidth || evidence.Normalization.TargetSize.Height != wantHeight {
		return fmt.Errorf("completed generated asset normalization target_size %dx%d does not match %s", evidence.Normalization.TargetSize.Width, evidence.Normalization.TargetSize.Height, input.SizeKey)
	}
	return nil
}

func validCreativeAssetStage(stage string) bool {
	return stage == "generated" || stage == "primed" || stage == "delivered"
}

func validCreativeAssetStatus(status string) bool {
	return status == "queued" || status == "running" || status == "completed" || status == "failed" || status == "cancelled"
}

// settleCreativeProductionVariantTask is the single terminal-state bridge for
// creative production tasks. A production task is allowed to stop after a
// partial model run, but the user-facing Variant must remain running while the
// bounded server-side continuation creates the next attempt. Only an exhausted
// continuation budget becomes action_required; no page click is needed for a
// transient model/daemon failure or a missing size.
func (h *Handler) settleCreativeProductionVariantTask(ctx context.Context, task db.AgentTaskQueue) error {
	var taskContext struct {
		Type            string   `json:"type"`
		Workflow        string   `json:"workflow"`
		CreativeOrderID string   `json:"creative_order_id"`
		VariantID       string   `json:"variant_id"`
		Revision        int      `json:"revision"`
		ExpectedSizes   []string `json:"expected_sizes"`
		ItemKey         string   `json:"item_key"`
	}
	if json.Unmarshal(task.Context, &taskContext) != nil || taskContext.Type != "creative_domain_task" || taskContext.Workflow != "creative_production" {
		return nil
	}
	variantID, err := uuid.Parse(strings.TrimSpace(taskContext.VariantID))
	if err != nil || taskContext.Revision < 1 {
		return nil
	}
	expectedSizes := taskContext.ExpectedSizes
	if len(expectedSizes) == 0 {
		expectedSizes = []string{"1080x1080", "1200x628", "800x1000"}
	}
	seen := make(map[string]struct{}, len(expectedSizes))
	for _, size := range expectedSizes {
		if !validCreativeAssetSize(size) {
			return nil
		}
		if _, duplicate := seen[size]; duplicate {
			return nil
		}
		seen[size] = struct{}{}
	}
	if h.TxStarter == nil || h.Queries == nil {
		return nil
	}
	variantUUID := pgtype.UUID{Bytes: variantID, Valid: true}
	orderID, orderErr := parseUUIDString(strings.TrimSpace(taskContext.CreativeOrderID))
	if orderErr != nil {
		if err := h.DB.QueryRow(ctx, `
SELECT item.order_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1
`, variantUUID).Scan(&orderID); err != nil {
			return nil
		}
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin creative production settlement: %w", err)
	}
	defer tx.Rollback(ctx)

	var workspaceID pgtype.UUID
	var orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT workspace_id, status
FROM creative_order
WHERE id = $1
FOR UPDATE
`, orderID).Scan(&workspaceID, &orderStatus); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("lock creative production order: %w", err)
	}
	if orderStatus == "cancelled" {
		return nil
	}
	var variantStatus string
	err = tx.QueryRow(ctx, `
SELECT variant.status
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND variant.revision = $2 AND item.order_id = $3
FOR UPDATE OF variant
`, variantUUID, taskContext.Revision, orderID).Scan(&variantStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock creative production variant: %w", err)
	}
	commitSettlement := func() error {
		if err := syncCreativeVariantRevisionFromVariant(ctx, tx, variantUUID, taskContext.Revision, expectedSizes); err != nil {
			return fmt.Errorf("sync creative production revision: %w", err)
		}
		return tx.Commit(ctx)
	}
	if variantStatus == "cancelled" {
		if err := commitSettlement(); err != nil {
			return fmt.Errorf("commit cancelled creative production variant: %w", err)
		}
		return nil
	}

	var completedCount int
	if err := tx.QueryRow(ctx, `
SELECT count(DISTINCT asset.size_key)
FROM creative_order_asset asset
WHERE asset.variant_id = $1
  AND asset.revision = $2
  AND asset.stage = 'generated'
  AND asset.status = 'completed'
  AND asset.size_key = ANY($3::text[])
`, variantUUID, taskContext.Revision, expectedSizes).Scan(&completedCount); err != nil {
		return fmt.Errorf("count creative production assets: %w", err)
	}
	if completedCount >= len(expectedSizes) {
		missingProcess, err := h.creativeProcessEvidenceMissing(ctx, h.DB, variantUUID, taskContext.Revision, expectedSizes, "creative_production", creativeProductionProcessLabels)
		if err != nil {
			return err
		}
		if len(missingProcess) > 0 {
			message := "creative production process evidence is incomplete: " + strings.Join(missingProcess, ", ")
			missingJSON, err := json.Marshal(missingProcess)
			if err != nil {
				return fmt.Errorf("encode missing creative process evidence: %w", err)
			}
			if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'action_required',
    brief = brief || jsonb_build_object(
      'error_code', 'process_evidence_missing',
      'error_message', $2::text,
      'process_evidence_missing', $3::jsonb
    ),
    updated_at = now()
WHERE id = $1 AND revision = $4
`, variantUUID, message, missingJSON, taskContext.Revision); err != nil {
				return fmt.Errorf("mark missing creative process evidence: %w", err)
			}
			if err := commitSettlement(); err != nil {
				return fmt.Errorf("commit missing creative process evidence: %w", err)
			}
			return nil
		}
		var primedCount int
		if err := tx.QueryRow(ctx, `
SELECT count(DISTINCT asset.size_key)
FROM creative_order_asset asset
WHERE asset.variant_id = $1
  AND asset.revision = $2
  AND asset.stage = 'primed'
  AND asset.status = 'completed'
  AND asset.attachment_id IS NOT NULL
  AND asset.size_key = ANY($3::text[])
`, variantUUID, taskContext.Revision, expectedSizes).Scan(&primedCount); err != nil {
			return fmt.Errorf("count creative Prime assets during production settlement: %w", err)
		}
		if primedCount < len(expectedSizes) {
			generated, generatedComplete, err := loadCreativePrimeGeneratedAssetsWithQuerier(
				ctx, tx, variantUUID, taskContext.Revision, expectedSizes,
			)
			if err != nil {
				return err
			}
			if !generatedComplete {
				return errors.New("creative generated package changed during Prime recovery enqueue")
			}
			if _, err := enqueueCreativePrimeCompositionJob(
				ctx, tx, variantUUID, taskContext.Revision, expectedSizes, creativePrimeGeneratedFingerprint(generated),
			); err != nil {
				return err
			}
			if err := commitSettlement(); err != nil {
				return fmt.Errorf("commit pending creative Prime composition: %w", err)
			}
			return nil
		}
		if err := commitSettlement(); err != nil {
			return fmt.Errorf("commit complete creative production variant: %w", err)
		}
		return nil
	}

	var orderItemID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT item.id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1 AND variant.revision = $2 AND item.order_id = $3
`, variantUUID, taskContext.Revision, orderID).Scan(&orderItemID); err != nil {
		return fmt.Errorf("resolve creative production ownership: %w", err)
	}

	itemKey := strings.TrimSpace(taskContext.ItemKey)
	if itemKey == "" {
		itemKey = fmt.Sprintf("%s:r%d", variantID.String(), taskContext.Revision)
	}
	var active bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM agent_task_queue active
  WHERE active.id <> $1
    AND active.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
    AND active.context->>'type' = 'creative_domain_task'
    AND active.context->>'workflow' = 'creative_production'
    AND active.context->>'variant_id' = $2
    AND active.context->>'revision' = $3
    AND active.context->>'item_key' = $4
)
`, task.ID, uuidToString(variantUUID), strconv.Itoa(taskContext.Revision), itemKey).Scan(&active); err != nil {
		return fmt.Errorf("check creative production continuation: %w", err)
	}
	if active {
		if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'running', brief = brief - 'error_code' - 'error_message', updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, variantUUID, taskContext.Revision); err != nil {
			return fmt.Errorf("keep creative production variant running: %w", err)
		}
		if err := commitSettlement(); err != nil {
			return fmt.Errorf("commit active creative production continuation: %w", err)
		}
		return nil
	}

	enabled, retryErr := creativeAutomaticRetryEnabled(ctx, tx, workspaceID)
	if retryErr != nil {
		return retryErr
	}
	if !enabled {
		if _, err := tx.Exec(ctx, "UPDATE creative_order_variant SET status='action_required',updated_at=now() WHERE id=$1 AND revision=$2 AND status<>'cancelled'", variantUUID, taskContext.Revision); err != nil {
			return err
		}
		return commitSettlement()
	}
	var normalized service.DirectTaskFanoutItem
	if task.TriggerEvidenceKind.Valid && task.TriggerEvidenceKind.String == "creative_order_item_production" && task.TriggerEvidenceRefID.Valid {
		normalized, err = normalizeCreativeProductionFanoutItem(ctx, tx, workspaceID, task.TriggerEvidenceRefID, service.DirectTaskFanoutItem{ItemKey: itemKey, Context: task.Context})
	} else {
		normalized, err = normalizeManualCreativeProductionFanoutItem(ctx, tx, workspaceID, orderID, service.DirectTaskFanoutItem{ItemKey: itemKey, Context: task.Context})
	}
	if err != nil {
		return fmt.Errorf("normalize creative production continuation: %w", err)
	}

	// Both a completed partial run and a failed direct run use a fresh session.
	// The production context contains the frozen brief, revision, and source
	// lineage, so resuming the old conversation is less reliable than asking
	// the producer to complete the missing sizes from the canonical context.
	var childID pgtype.UUID
	err = tx.QueryRow(ctx, `
INSERT INTO agent_task_queue (
    agent_id, runtime_id, issue_id, chat_session_id, autopilot_run_id,
    status, priority, trigger_comment_id, trigger_summary, context,
    session_id, work_dir,
    attempt, max_attempts, parent_task_id, force_fresh_session, is_leader_task,
    requesting_user_id, originator_user_id, accountable_user_id,
    originator_source, delegated_from_task_id, rule_version_id,
    retry_of_task_id, trigger_evidence_kind, trigger_evidence_ref_id
)
SELECT
    p.agent_id, p.runtime_id, p.issue_id, p.chat_session_id, p.autopilot_run_id,
    'queued', p.priority, p.trigger_comment_id, p.trigger_summary, $2::jsonb,
    NULL, NULL,
    p.attempt + 1, GREATEST(p.max_attempts, 3), p.id, TRUE, p.is_leader_task,
    p.requesting_user_id, p.originator_user_id, p.accountable_user_id,
    p.originator_source, p.delegated_from_task_id, p.rule_version_id,
    p.id, p.trigger_evidence_kind, p.trigger_evidence_ref_id
FROM agent_task_queue p
WHERE p.id = $1
  AND p.status IN ('completed', 'failed')
  AND p.attempt < GREATEST(p.max_attempts, 3)
RETURNING id
`, task.ID, normalized.Context).Scan(&childID)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, updateErr := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'action_required', updated_at = now()
WHERE id = $1 AND revision = $2 AND status IN ('queued', 'running', 'partial', 'failed', 'action_required')
`, variantUUID, taskContext.Revision); updateErr != nil {
			return fmt.Errorf("mark exhausted creative production variant: %w", updateErr)
		}
		if err := commitSettlement(); err != nil {
			return fmt.Errorf("commit exhausted creative production variant: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("queue creative production continuation: %w", err)
	}
	child, err := h.Queries.WithTx(tx).GetAgentTask(ctx, childID)
	if err != nil {
		return fmt.Errorf("load creative production continuation: %w", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'running', brief = brief - 'error_code' - 'error_message', updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, variantUUID, taskContext.Revision); err != nil {
		return fmt.Errorf("mark creative production continuation running: %w", err)
	}
	if err := commitSettlement(); err != nil {
		return fmt.Errorf("commit creative production continuation: %w", err)
	}
	if h.TaskService != nil {
		h.TaskService.NotifyTaskEnqueued(ctx, child)
	}
	slog.Info("creative production continuation enqueued",
		"parent_task_id", uuidToString(task.ID),
		"child_task_id", uuidToString(child.ID),
		"variant_id", uuidToString(variantUUID),
		"revision", taskContext.Revision,
		"completed_sizes", completedCount,
		"expected_sizes", len(expectedSizes),
		"attempt", child.Attempt,
		"max_attempts", child.MaxAttempts,
	)
	return nil
}

type creativeDirectEditTaskCompletionContext struct {
	Type                  string   `json:"type"`
	Workflow              string   `json:"workflow"`
	CreativeOrderID       string   `json:"creative_order_id"`
	VariantID             string   `json:"variant_id"`
	Revision              int      `json:"revision"`
	TargetSize            string   `json:"target_size"`
	DeliveryMode          string   `json:"delivery_mode"`
	ExpectedSizes         []string `json:"expected_sizes"`
	FinalVisualValidation bool     `json:"final_visual_validation"`
}

type creativeDirectEditArtifactState struct {
	VariantExists   bool
	TargetGenerated bool
	GeneratedCount  int
	PrimedCount     int
	DeliveredCount  int
}

const (
	creativeDirectEditOutputPendingFailureReason = "creative_output_pending"
	creativeDirectEditOutputPendingError         = "direct image edit is awaiting a durable provider receipt"
)

func parseCreativeDirectEditTaskCompletionContext(raw json.RawMessage) (creativeDirectEditTaskCompletionContext, bool, error) {
	var taskContext creativeDirectEditTaskCompletionContext
	if err := json.Unmarshal(raw, &taskContext); err != nil {
		return taskContext, false, err
	}
	if taskContext.Type != "creative_domain_task" || taskContext.Workflow != "creative_direct_edit" {
		return taskContext, false, nil
	}
	taskContext.CreativeOrderID = strings.TrimSpace(taskContext.CreativeOrderID)
	taskContext.VariantID = strings.TrimSpace(taskContext.VariantID)
	taskContext.TargetSize = strings.TrimSpace(taskContext.TargetSize)
	taskContext.DeliveryMode = strings.TrimSpace(taskContext.DeliveryMode)
	if _, err := uuid.Parse(taskContext.VariantID); err != nil || taskContext.Revision < 1 ||
		!validCreativeAssetSize(taskContext.TargetSize) ||
		(taskContext.DeliveryMode != "preview" && taskContext.DeliveryMode != "publish") {
		return taskContext, true, errors.New("invalid creative direct-edit completion context")
	}
	expectedSizes, err := normalizeCreativeExpectedSizes(taskContext.ExpectedSizes)
	if err != nil {
		return taskContext, true, err
	}
	if !creativeSizeIsExpected(taskContext.TargetSize, expectedSizes) {
		return taskContext, true, errors.New("creative direct-edit target size is not expected")
	}
	taskContext.ExpectedSizes = expectedSizes
	return taskContext, true, nil
}

func (h *Handler) loadCreativeDirectEditArtifactState(ctx context.Context, variantID pgtype.UUID, revision int, targetSize string, expectedSizes []string, workspaceID pgtype.UUID) (creativeDirectEditArtifactState, error) {
	var state creativeDirectEditArtifactState
	if err := h.DB.QueryRow(ctx, `
WITH expected(size_key) AS (
  SELECT unnest($4::text[])
)
SELECT EXISTS(
    SELECT 1
    FROM creative_order_variant variant
    JOIN creative_order_item item ON item.id = variant.order_item_id
    JOIN creative_order order_row ON order_row.id = item.order_id
    WHERE variant.id = $1 AND variant.revision = $2
      AND (NOT $5::boolean OR order_row.workspace_id = $6)
  ),
  EXISTS(
    SELECT 1
    FROM creative_order_asset asset
    WHERE asset.variant_id = $1 AND asset.revision = $2 AND asset.size_key = $3
      AND asset.stage = 'generated' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
  ),
  (
    SELECT count(DISTINCT asset.size_key)
    FROM creative_order_asset asset
    JOIN expected ON expected.size_key = asset.size_key
    WHERE asset.variant_id = $1 AND asset.revision = $2
      AND asset.stage = 'generated' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
  ),
  (
    SELECT count(DISTINCT asset.size_key)
    FROM creative_order_asset asset
    JOIN expected ON expected.size_key = asset.size_key
    WHERE asset.variant_id = $1 AND asset.revision = $2
      AND asset.stage = 'primed' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
  ),
  (
    SELECT count(DISTINCT asset.size_key)
    FROM creative_order_asset asset
    JOIN expected ON expected.size_key = asset.size_key
    WHERE asset.variant_id = $1 AND asset.revision = $2
      AND asset.stage = 'delivered' AND asset.status = 'completed' AND asset.attachment_id IS NOT NULL
  )
`, variantID, revision, targetSize, expectedSizes, workspaceID.Valid, workspaceID).Scan(
		&state.VariantExists,
		&state.TargetGenerated,
		&state.GeneratedCount,
		&state.PrimedCount,
		&state.DeliveredCount,
	); err != nil {
		return creativeDirectEditArtifactState{}, err
	}
	return state, nil
}

func creativeDirectEditArtifactError(taskContext creativeDirectEditTaskCompletionContext, state creativeDirectEditArtifactState) string {
	if !state.VariantExists {
		return "direct image edit task completed with invalid artifact coordinates"
	}
	if !state.TargetGenerated {
		return "direct image edit did not register a completed target generated asset"
	}
	if taskContext.DeliveryMode != "publish" {
		return ""
	}
	if !taskContext.FinalVisualValidation {
		return "direct image edit publish task requires final visual validation"
	}
	expectedCount := len(taskContext.ExpectedSizes)
	if state.GeneratedCount < expectedCount {
		return fmt.Sprintf("direct image edit registered %d/%d completed generated assets", state.GeneratedCount, expectedCount)
	}
	if state.PrimedCount < expectedCount {
		return fmt.Sprintf("direct image edit registered %d/%d completed primed assets", state.PrimedCount, expectedCount)
	}
	return ""
}

const creativeDirectEditPreviewRejectedError = "已生成改后预览，但未通过交付验收；请查看预览图片"

// creativeDirectEditPreviewAvailable distinguishes a real missing writeback
// from a model result deliberately retained as an unadopted diagnostic asset.
func (h *Handler) creativeDirectEditPreviewAvailable(ctx context.Context, variantID pgtype.UUID, revision int, targetSize string, rejectedOnly bool) (bool, error) {
	var available bool
	err := h.DB.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM creative_order_diagnostic_asset
  WHERE variant_id = $1
    AND revision = $2
    AND size_key = $3
    AND workflow = 'creative_direct_edit'
    AND attachment_id IS NOT NULL
    AND (NOT $4::boolean OR metadata->>'accepted' = 'false')
)
`, variantID, revision, targetSize, rejectedOnly).Scan(&available)
	return available, err
}

func (h *Handler) creativeDirectEditCompletionError(ctx context.Context, task db.AgentTaskQueue, workspaceID string) (string, error) {
	if task.Status == "completed" || task.Status == "failed" || task.Status == "cancelled" {
		return "", nil
	}
	taskContext, ok, parseErr := parseCreativeDirectEditTaskCompletionContext(task.Context)
	if !ok {
		return "", nil
	}
	if parseErr != nil {
		return "direct image edit task completed with invalid task context", nil
	}
	variantID, _ := parseUUIDString(taskContext.VariantID)
	workspaceUUID, workspaceErr := parseUUIDString(workspaceID)
	if workspaceErr != nil {
		return "", workspaceErr
	}
	state, err := h.loadCreativeDirectEditArtifactState(ctx, variantID, taskContext.Revision, taskContext.TargetSize, taskContext.ExpectedSizes, workspaceUUID)
	if err != nil {
		return "", err
	}
	if !state.TargetGenerated {
		hasRejectedPreview, err := h.creativeDirectEditPreviewAvailable(ctx, variantID, taskContext.Revision, taskContext.TargetSize, true)
		if err != nil {
			return "", err
		}
		if hasRejectedPreview {
			return creativeDirectEditPreviewRejectedError, nil
		}
	}
	return creativeDirectEditArtifactError(taskContext, state), nil
}

// creativeDirectEditTargetOperationPending reports whether the missing target
// asset still has one persisted provider attempt that can deliver a late
// receipt. A terminal task must retain this recoverable state instead of
// turning its variant into action_required.
func (h *Handler) creativeDirectEditTargetOperationPending(ctx context.Context, task db.AgentTaskQueue) (bool, error) {
	taskContext, ok, parseErr := parseCreativeDirectEditTaskCompletionContext(task.Context)
	if !ok {
		return false, nil
	}
	if parseErr != nil {
		return false, parseErr
	}
	variantID, err := parseUUIDString(taskContext.VariantID)
	if err != nil {
		return false, err
	}
	state, err := h.loadCreativeDirectEditArtifactState(ctx, variantID, taskContext.Revision, taskContext.TargetSize, taskContext.ExpectedSizes, pgtype.UUID{})
	if err != nil || !state.VariantExists || state.TargetGenerated {
		return false, err
	}
	var pending bool
	err = h.DB.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM creative_image_operation operation
  JOIN creative_image_operation_attempt attempt ON attempt.operation_id = operation.id
  WHERE operation.task_id = $1
    AND attempt.task_id = $1
    AND operation.variant_id = $2
    AND operation.revision = $3
    AND operation.size_key = $4
    AND operation.operation_kind IN ('direct_edit', 'visual_rework', 'canvas_repair')
    AND operation.status IN ('queued', 'running', 'unknown')
    AND attempt.attempt = (
      SELECT max(latest.attempt)
      FROM creative_image_operation_attempt latest
      WHERE latest.operation_id = operation.id
    )
    AND attempt.status IN ('running', 'unknown')
)
`, task.ID, variantID, taskContext.Revision, taskContext.TargetSize).Scan(&pending)
	if err != nil {
		return false, err
	}
	return pending, nil
}

func (h *Handler) settleCreativeDirectEditTask(ctx context.Context, task db.AgentTaskQueue) error {
	taskContext, ok, parseErr := parseCreativeDirectEditTaskCompletionContext(task.Context)
	if !ok || parseErr != nil {
		return nil
	}
	variantID, err := uuid.Parse(taskContext.VariantID)
	if err != nil {
		return nil
	}
	if h.TxStarter == nil {
		return nil
	}
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin direct image edit settlement: %w", err)
	}
	defer tx.Rollback(ctx)

	variantUUID := pgtype.UUID{Bytes: variantID, Valid: true}
	var variantStatus string
	if err := tx.QueryRow(ctx, `
SELECT status
FROM creative_order_variant
WHERE id = $1 AND revision = $2
FOR UPDATE
`, variantUUID, taskContext.Revision).Scan(&variantStatus); errors.Is(err, pgx.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("lock direct image edit variant: %w", err)
	}
	commitSettlement := func() error {
		if err := syncCreativeVariantRevisionFromVariant(ctx, tx, variantUUID, taskContext.Revision, taskContext.ExpectedSizes); err != nil {
			return fmt.Errorf("sync direct image edit revision: %w", err)
		}
		return tx.Commit(ctx)
	}
	if variantStatus == "cancelled" {
		return commitSettlement()
	}
	if task.FailureReason.Valid && strings.TrimSpace(task.FailureReason.String) == creativeDirectEditOutputPendingFailureReason {
		if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'running',
    brief = brief - 'creative_direct_edit_error' - 'error_code' - 'error_message',
    updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, variantUUID, taskContext.Revision); err != nil {
			return fmt.Errorf("preserve pending direct image edit: %w", err)
		}
		return commitSettlement()
	}

	state, err := h.loadCreativeDirectEditArtifactState(ctx, variantUUID, taskContext.Revision, taskContext.TargetSize, taskContext.ExpectedSizes, pgtype.UUID{})
	if err != nil {
		return fmt.Errorf("check direct image edit output: %w", err)
	}
	if creativeDirectEditArtifactError(taskContext, state) == "" {
		if taskContext.DeliveryMode == "preview" {
			if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'completed',
    brief = brief - 'creative_direct_edit_error' - 'error_code' - 'error_message',
    updated_at = now()
WHERE id = $1 AND revision = $2 AND status <> 'cancelled'
`, variantUUID, taskContext.Revision); err != nil {
				return fmt.Errorf("complete direct image edit preview: %w", err)
			}
		}
		if err := commitSettlement(); err != nil {
			return err
		}
		orderUUID := pgtype.UUID{}
		if parsedOrderID, parseErr := parseUUIDString(taskContext.CreativeOrderID); parseErr == nil {
			orderUUID = parsedOrderID
		}
		h.notifyCreativeDirectAdjustmentDelivery(ctx, pgtype.UUID{}, orderUUID, variantUUID, taskContext.Revision, taskContext.TargetSize)
		return nil
	}

	detail := creativeDirectEditArtifactError(taskContext, state)
	if detail == "" {
		detail = "direct image edit did not register a completed target base"
	}
	if task.Error.Valid && strings.TrimSpace(task.Error.String) != "" {
		detail = strings.TrimSpace(task.Error.String)
	} else if task.FailureReason.Valid && strings.TrimSpace(task.FailureReason.String) != "" {
		detail = strings.TrimSpace(task.FailureReason.String)
	}
	if len(detail) > 1200 {
		detail = detail[:1200]
	}
	hasPreview, err := h.creativeDirectEditPreviewAvailable(ctx, variantUUID, taskContext.Revision, taskContext.TargetSize, true)
	if err != nil {
		return fmt.Errorf("check direct image edit preview: %w", err)
	}
	if hasPreview {
		detail = creativeDirectEditPreviewRejectedError
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'action_required',
    brief = jsonb_set(
      brief,
      '{creative_direct_edit_error}',
      jsonb_build_object('message', $2::text, 'retryable', true, 'updated_at', now()::text),
      true
    ),
    updated_at = now()
WHERE id = $1 AND revision = $3
`, variantUUID, detail, taskContext.Revision); err != nil {
		return fmt.Errorf("mark direct image edit action required: %w", err)
	}
	if err := commitSettlement(); err != nil {
		return err
	}
	if hasPreview {
		orderUUID := pgtype.UUID{}
		if parsedOrderID, parseErr := parseUUIDString(taskContext.CreativeOrderID); parseErr == nil {
			orderUUID = parsedOrderID
		}
		h.notifyCreativeDirectAdjustmentPreview(ctx, pgtype.UUID{}, orderUUID, variantUUID, taskContext.Revision, taskContext.TargetSize)
	}
	return nil
}

func validCreativeQCStatus(status string) bool {
	return status == "pending" || status == "passed" || status == "warning" || status == "failed"
}

func scanCreativeSourceAnalysis(row rowScanner) (creativeSourceAnalysisResponse, error) {
	var item creativeSourceAnalysisResponse
	var result string
	err := row.Scan(&item.ID, &item.WorkspaceID, &item.CandidateID, &item.AnalysisVersion, &item.Status, &item.Summary, &result,
		&item.ErrorCode, &item.ErrorMessage, &item.TriggerEvidenceKind, &item.TriggerEvidenceReference, &item.CreatedAt, &item.CompletedAt)
	item.Result = json.RawMessage(result)
	return item, err
}

func scanCreativeOrder(row rowScanner) (creativeOrderResponse, error) {
	var order creativeOrderResponse
	var snapshot string
	err := row.Scan(&order.ID, &order.WorkspaceID, &order.IssueID, &order.Status, &snapshot,
		&order.TriggerEvidenceKind, &order.TriggerEvidenceReference, &order.CreatedBy, &order.CreatedAt, &order.UpdatedAt)
	order.InputSnapshot = json.RawMessage(snapshot)
	order.WorkflowFailures = []creativeOrderWorkflowFailureResponse{}
	return order, err
}

func scanCreativeOrderVariant(row rowScanner) (creativeOrderVariantResponse, error) {
	var variant creativeOrderVariantResponse
	var brief string
	err := row.Scan(&variant.ID, &variant.OrderItemID, &variant.VariantKey, &brief, &variant.Revision, &variant.Status,
		&variant.ActiveRevision, &variant.StagingRevision, &variant.CandidateState, &variant.SelectionRank, &variant.PrimarySize,
		&variant.QCRecoveryUsed, &variant.QCRecoveryAvailable, &variant.CreatedAt, &variant.UpdatedAt)
	variant.Brief = json.RawMessage(brief)
	return variant, err
}

func scanCreativeOrderAsset(row rowScanner) (creativeOrderAssetResponse, error) {
	var asset creativeOrderAssetResponse
	var metadata, evidence string
	err := row.Scan(&asset.ID, &asset.VariantID, &asset.AssetFamilyID, &asset.SizeKey, &asset.Revision, &asset.Stage, &asset.AttachmentID, &asset.DerivedFromAssetID, &asset.OperationID, &metadata, &evidence, &asset.Status, &asset.CreatedAt, &asset.UpdatedAt)
	asset.Metadata = json.RawMessage(metadata)
	asset.Evidence = json.RawMessage(evidence)
	return asset, err
}

func scanCreativeOrderDiagnosticAsset(row rowScanner) (creativeOrderDiagnosticAsset, error) {
	var asset creativeOrderDiagnosticAsset
	var metadata string
	err := row.Scan(&asset.ID, &asset.VariantID, &asset.TaskID, &asset.AttachmentID, &asset.SizeKey, &asset.Revision, &asset.Workflow, &asset.Label, &asset.Filename, &metadata, &asset.CreatedAt, &asset.UpdatedAt)
	asset.Metadata = json.RawMessage(metadata)
	asset.URL = attachmentDownloadPath(asset.AttachmentID)
	return asset, err
}

func (h *Handler) derivedCreativeOrderStatus(r *http.Request, orderID pgtype.UUID) (string, error) {
	failures, err := h.listCreativeOrderWorkflowFailures(r, orderID)
	if err != nil {
		return "", err
	}
	productionStatus, err := h.derivedCreativeOrderStatusWithFailures(r, orderID, len(failures) > 0)
	if err != nil {
		return "", err
	}
	deliveryStatus, err := h.derivedCreativeOrderDeliveryStatus(r.Context(), orderID)
	if err != nil {
		return "", err
	}
	if deliveryStatus == "completed" || deliveryStatus == "awaiting_adoption" {
		return deliveryStatus, nil
	}
	if deliveryStatus == "partial" && productionStatus == "completed" {
		return "partial", nil
	}
	return productionStatus, nil
}

func (h *Handler) loadCreativeOrderWorkflowState(r *http.Request, order *creativeOrderResponse) error {
	if len(order.InputSnapshot) == 0 {
		var snapshot string
		if err := h.DB.QueryRow(r.Context(), `SELECT input_snapshot::text FROM creative_order WHERE id=$1`, parseUUID(order.ID)).Scan(&snapshot); err != nil {
			return err
		}
		order.InputSnapshot = json.RawMessage(snapshot)
	}
	values := []creativeOrderResponse{*order}
	if err := h.loadCreativeOrderWorkflowStates(r.Context(), values); err != nil {
		return err
	}
	*order = values[0]
	return nil
}

func (h *Handler) derivedCreativeOrderDeliveryStatus(ctx context.Context, orderID pgtype.UUID) (string, error) {
	var snapshot string
	if err := h.DB.QueryRow(ctx, "SELECT input_snapshot::text FROM creative_order WHERE id=$1", orderID).Scan(&snapshot); err != nil {
		return "", err
	}
	values, err := h.creativeOrderDeliveryBatch(ctx, []creativeOrderResponse{{ID: uuidToString(orderID), InputSnapshot: json.RawMessage(snapshot)}})
	return values[uuidToString(orderID)], err
}

func (h *Handler) listCreativeOrderWorkflowFailures(r *http.Request, orderID pgtype.UUID) ([]creativeOrderWorkflowFailureResponse, error) {
	values, err := h.creativeOrderFailuresBatch(r.Context(), []pgtype.UUID{orderID})
	if err != nil {
		return nil, err
	}
	result := values[uuidToString(orderID)]
	if result == nil {
		result = []creativeOrderWorkflowFailureResponse{}
	}
	return result, nil
}

func (h *Handler) derivedCreativeOrderStatusWithFailures(r *http.Request, orderID pgtype.UUID, hasOpenFailures bool) (string, error) {
	failed := []pgtype.UUID{}
	if hasOpenFailures {
		failed = append(failed, orderID)
	}
	values, err := h.creativeOrderProductionBatch(r.Context(), []pgtype.UUID{orderID}, failed)
	return values[uuidToString(orderID)], err
}

func (h *Handler) listCreativeOrderItems(r *http.Request, orderID pgtype.UUID) ([]creativeOrderItemResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, order_id::text, COALESCE(candidate_id::text, ''), COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, COALESCE(adopted_variant_id::text, ''), COALESCE(adopted_at::text, ''),
  COALESCE(adopted_by::text, ''), created_at::text, updated_at::text, source_kind, COALESCE(copy_library_id::text, '')
FROM creative_order_item WHERE order_id = $1 ORDER BY created_at
`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []creativeOrderItemResponse{}
	for rows.Next() {
		item, err := scanCreativeOrderItemWithAdoption(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	ids := make([]pgtype.UUID, 0, len(items))
	for _, item := range items {
		ids = append(ids, parseUUID(item.ID))
	}
	variants, err := h.listCreativeOrderVariantsBatch(r, ids)
	if err != nil {
		return nil, err
	}
	byItem := make(map[string][]creativeOrderVariantResponse)
	for _, variant := range variants {
		byItem[variant.OrderItemID] = append(byItem[variant.OrderItemID], variant)
	}
	progress, err := loadCreativeCandidateProgressBatch(r.Context(), h.DB, orderID, ids)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Variants = byItem[items[i].ID]
		items[i].CandidateProgress = progress[items[i].ID]
	}
	return items, nil
}

// List responses only need enough item, variant, and asset data to render the
// order list. Full QC, diagnostic, and blocker data remains on GetCreativeOrder.
// Keeping this projection separate prevents the list page from issuing the
// detail endpoint's per-variant N+1 query tree for every order.
func (h *Handler) listCreativeOrderListItems(r *http.Request, orderID pgtype.UUID) ([]creativeOrderItemResponse, error) {
	return h.listCreativeOrderListItemsBatch(r, []pgtype.UUID{orderID})
}

func scanCreativeOrderItemWithAdoption(row rowScanner) (creativeOrderItemResponse, error) {
	var item creativeOrderItemResponse
	var snapshot string
	err := row.Scan(&item.ID, &item.OrderID, &item.CandidateID, &item.SourceAnalysisID, &snapshot,
		&item.Direction, &item.Status, &item.AdoptedVariantID, &item.AdoptedAt, &item.AdoptedBy,
		&item.CreatedAt, &item.UpdatedAt, &item.SourceKind, &item.CopyLibraryID)
	item.CopySnapshot = json.RawMessage(snapshot)
	return item, err
}

func (h *Handler) loadCreativeOrderItem(r *http.Request, itemID pgtype.UUID) (creativeOrderItemResponse, error) {
	item, err := scanCreativeOrderItemWithAdoption(h.DB.QueryRow(r.Context(), `
SELECT id::text, order_id::text, COALESCE(candidate_id::text, ''), COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, COALESCE(adopted_variant_id::text, ''), COALESCE(adopted_at::text, ''),
  COALESCE(adopted_by::text, ''), created_at::text, updated_at::text, source_kind, COALESCE(copy_library_id::text, '')
FROM creative_order_item WHERE id = $1
`, itemID))
	if err != nil {
		return creativeOrderItemResponse{}, err
	}
	item.Variants, err = h.listCreativeOrderVariants(r, itemID)
	if err == nil {
		item.CandidateProgress, err = loadCreativeCandidateProgress(r.Context(), h.DB, parseUUID(item.OrderID), itemID)
	}
	return item, err
}

func (h *Handler) listCreativeOrderVariants(r *http.Request, itemID pgtype.UUID) ([]creativeOrderVariantResponse, error) {
	return h.listCreativeOrderVariantsBatch(r, []pgtype.UUID{itemID})
}

func (h *Handler) loadCreativeOrderVariantBlocker(r *http.Request, variantID pgtype.UUID) (*creativeOrderVariantBlocker, error) {
	var brief string
	err := h.DB.QueryRow(r.Context(), `SELECT brief::text FROM creative_order_variant WHERE id=$1`, variantID).Scan(&brief)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if blocker := creativeVariantBriefBlocker(json.RawMessage(brief)); blocker != nil {
		return blocker, nil
	}
	values, err := h.creativeVariantBlockers(r.Context(), []pgtype.UUID{variantID})
	return values[uuidToString(variantID)], err
}

func summarizeCreativeOrderVariantBlocker(value string) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	for _, marker := range []string{"原因：", "原因:", "阻断原因：", "阻断原因:"} {
		if index := strings.LastIndex(text, marker); index >= 0 {
			text = strings.TrimSpace(text[index+len(marker):])
			break
		}
	}
	paragraphs := strings.Split(text, "\n")
	bestScore := 0
	bestLines := []string{}
	for _, paragraph := range paragraphs {
		line := strings.TrimSpace(paragraph)
		if line == "" {
			continue
		}
		score := creativeOrderVariantBlockerLineScore(line)
		if score > bestScore {
			bestScore = score
			bestLines = []string{line}
		} else if score == bestScore && score > 0 {
			bestLines = append(bestLines, line)
		}
	}
	if bestScore > 0 {
		if len(bestLines) > 2 {
			bestLines = bestLines[len(bestLines)-2:]
		}
		text = strings.Join(bestLines, "；")
	} else {
		for i := len(paragraphs) - 1; i >= 0; i-- {
			line := strings.TrimSpace(paragraphs[i])
			if line == "" {
				continue
			}
			text = line
			break
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	return truncateCreativeOrderBlockerDetail(text, 220)
}

func creativeOrderVariantBlockerLineScore(line string) int {
	normalized := strings.ToLower(strings.TrimSpace(line))
	if normalized == "" || creativeOrderVariantBlockerBookkeepingLine(normalized) {
		return 0
	}
	score := 0
	if strings.Contains(normalized, "超时") || strings.Contains(normalized, "timeout") ||
		strings.Contains(normalized, "未返回") || strings.Contains(normalized, "无法") ||
		strings.Contains(normalized, "拒绝") || strings.Contains(normalized, "比例") ||
		strings.Contains(normalized, "尺寸") || strings.Contains(normalized, "像素") ||
		strings.Contains(normalized, "json") || strings.Contains(normalized, "aspect") {
		score = 4
	}
	if strings.Contains(normalized, "失败") || strings.Contains(normalized, "failed") ||
		strings.Contains(normalized, "error") || strings.Contains(normalized, "未注册") ||
		strings.Contains(normalized, "未登记") || strings.Contains(normalized, "未完成") ||
		strings.Contains(normalized, "未通过") || strings.Contains(normalized, "没有") {
		if score < 3 {
			score = 3
		}
	}
	if strings.Contains(normalized, "安全") || strings.Contains(normalized, "prime") {
		if score < 2 {
			score = 2
		}
	}
	if strings.Contains(normalized, "action_required") && score < 1 {
		score = 1
	}
	return score
}

func creativeOrderVariantBlockerBookkeepingLine(line string) bool {
	return (strings.Contains(line, "已标记为") && strings.Contains(line, "action_required")) ||
		strings.Contains(line, "未修改 issue") ||
		strings.Contains(line, "任务已执行并失败")
}

func truncateCreativeOrderBlockerDetail(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	if limit <= 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func (h *Handler) listCreativeOrderVariantDiagnosticAssets(r *http.Request, variantID pgtype.UUID) ([]creativeOrderDiagnosticAsset, error) {
	values, err := creativeRowsByOwner[creativeOrderDiagnosticAsset](r.Context(), h.DB, creativeDiagnosticBatchSQL, []pgtype.UUID{variantID})
	result := creativeOwnerRows(values, variantID)
	for i := range result {
		result[i].URL = attachmentDownloadPath(result[i].AttachmentID)
	}
	return result, err
}

func creativeAssetSizeOrder(size string) int {
	switch size {
	case "1080x1080":
		return 0
	case "1200x628":
		return 1
	case "800x1000":
		return 2
	default:
		return 3
	}
}

func (h *Handler) listCreativeOrderAssets(r *http.Request, variantID pgtype.UUID) ([]creativeOrderAssetResponse, error) {
	values, err := creativeRowsByOwner[creativeOrderAssetResponse](r.Context(), h.DB, creativeAssetBatchSQL, []pgtype.UUID{variantID})
	return creativeOwnerRows(values, variantID), err
}

func (h *Handler) derivedCreativeVariantQCStatus(r *http.Request, variantID pgtype.UUID) (string, error) {
	values, err := h.creativeVariantQCStatuses(r.Context(), []pgtype.UUID{variantID})
	if err != nil {
		return "", err
	}
	value, found := values[uuidToString(variantID)]
	if !found {
		return "", pgx.ErrNoRows
	}
	return value, nil
}

func (h *Handler) listCreativeOrderQCReports(r *http.Request, variantID pgtype.UUID) ([]creativeOrderQCReportResponse, error) {
	values, err := creativeRowsByOwner[creativeOrderQCReportResponse](r.Context(), h.DB, creativeQCReportBatchSQL, []pgtype.UUID{variantID})
	return creativeOwnerRows(values, variantID), err
}
