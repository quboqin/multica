package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/attribution"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
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

// A production prompt now carries the frozen visual layout plus every
// first-party text and numeric instruction. Complex multi-region ads exceed
// the legacy 4k limit even though they remain one bounded order item.
const maxCreativeOrderDirectionLength = 16_000

type creativeOrderResponse struct {
	ID                       string                                 `json:"id"`
	WorkspaceID              string                                 `json:"workspace_id"`
	IssueID                  string                                 `json:"issue_id"`
	Status                   string                                 `json:"status"`
	DerivedStatus            string                                 `json:"derived_status"`
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

// creativeOrderPrimePackageRepairResponse describes a fresh Prime composition
// run that reuses the existing generated assets for the current revision.
type creativeOrderPrimePackageRepairResponse struct {
	TaskID string `json:"task_id"`
}

// creativeOrderQCRetryResponse describes a recovery run that deliberately
// reuses the already completed Prime assets. These are two independent tasks,
// not a creative-generation retry.
type creativeOrderQCRetryResponse struct {
	VariantID       string `json:"variant_id"`
	Revision        int    `json:"revision"`
	TechnicalTaskID string `json:"technical_task_id"`
	VisualTaskID    string `json:"visual_task_id"`
}

type creativeOrderItemResponse struct {
	ID               string                         `json:"id"`
	OrderID          string                         `json:"order_id"`
	CandidateID      string                         `json:"candidate_id"`
	SourceAnalysisID string                         `json:"source_analysis_id"`
	CopySnapshot     json.RawMessage                `json:"copy_snapshot"`
	Direction        string                         `json:"direction"`
	Status           string                         `json:"status"`
	AdoptedVariantID string                         `json:"adopted_variant_id"`
	AdoptedAt        string                         `json:"adopted_at"`
	AdoptedBy        string                         `json:"adopted_by"`
	CreatedAt        string                         `json:"created_at"`
	UpdatedAt        string                         `json:"updated_at"`
	Variants         []creativeOrderVariantResponse `json:"variants,omitempty"`
}

type creativeOrderVariantResponse struct {
	ID                   string                          `json:"id"`
	OrderItemID          string                          `json:"order_item_id"`
	VariantKey           string                          `json:"variant_key"`
	Brief                json.RawMessage                 `json:"brief"`
	Revision             int                             `json:"revision"`
	Status               string                          `json:"status"`
	QCStatus             string                          `json:"qc_status"`
	QCRecoveryUsed       bool                            `json:"qc_recovery_used"`
	QCRecoveryAvailable  bool                            `json:"qc_recovery_available"`
	PrimeRepairUsed      bool                            `json:"prime_repair_used"`
	PrimeRepairAvailable bool                            `json:"prime_repair_available"`
	ActionRequired       *creativeOrderVariantBlocker    `json:"action_required,omitempty"`
	Assets               []creativeOrderAssetResponse    `json:"assets,omitempty"`
	DiagnosticAssets     []creativeOrderDiagnosticAsset  `json:"diagnostic_assets,omitempty"`
	QCReports            []creativeOrderQCReportResponse `json:"qc_reports,omitempty"`
	CreatedAt            string                          `json:"created_at"`
	UpdatedAt            string                          `json:"updated_at"`
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
	ID        string `json:"id"`
	VariantID string `json:"variant_id"`
	TaskID    string `json:"task_id"`
	SizeKey   string `json:"size_key"`
	Revision  int    `json:"revision"`
	Label     string `json:"label"`
	Filename  string `json:"filename"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

type creativeOrderDiagnosticAssetFile struct {
	creativeOrderDiagnosticAsset
	Path    string
	ModTime time.Time
}

type creativeOrderDiagnosticTask struct {
	OrderID   string
	VariantID string
	Revision  int
	TaskID    string
	Workflow  string
	WorkDir   string
	CreatedAt string
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
	Metadata           json.RawMessage `json:"metadata"`
	Evidence           json.RawMessage `json:"evidence"`
	Status             string          `json:"status"`
	CreatedAt          string          `json:"created_at"`
	UpdatedAt          string          `json:"updated_at"`
}

type creativeOrderVariantInput struct {
	OrderItemID string          `json:"order_item_id"`
	VariantKey  string          `json:"variant_key"`
	Brief       json.RawMessage `json:"brief"`
	Revision    int             `json:"revision"`
	Status      string          `json:"status"`
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
	Metadata           json.RawMessage `json:"metadata"`
	Evidence           json.RawMessage `json:"evidence"`
	Status             string          `json:"status"`
}

type creativeOrderQCReportResponse struct {
	ID                       string          `json:"id"`
	VariantID                string          `json:"variant_id"`
	Lane                     string          `json:"lane"`
	Revision                 int             `json:"revision"`
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
	Status                   string          `json:"status"`
	Findings                 json.RawMessage `json:"findings"`
	TriggerEvidenceKind      string          `json:"trigger_evidence_kind"`
	TriggerEvidenceReference string          `json:"trigger_evidence_ref_id"`
}

type creativeOrderQCFinalizeInput struct {
	VariantID string `json:"variant_id"`
	Revision  int    `json:"revision"`
}

type creativeOrderQCFinalizeResponse struct {
	Created              bool   `json:"created"`
	Finalized            bool   `json:"finalized"`
	Outcome              string `json:"outcome"`
	VariantID            string `json:"variant_id"`
	Revision             int    `json:"revision"`
	TechnicalStatus      string `json:"technical_status"`
	VisualStatus         string `json:"visual_status"`
	DeliveredAssetCount  int    `json:"delivered_asset_count"`
	OrderAggregateStatus string `json:"order_aggregate_status"`
	InboxItemID          string `json:"inbox_item_id,omitempty"`
}

type creativeOrderItemAdoptionInput struct {
	VariantID          string `json:"variant_id"`
	QCRiskAcknowledged bool   `json:"qc_risk_acknowledged"`
	QCRiskReason       string `json:"qc_risk_reason"`
}

var standardCreativeAssetSizes = []string{"1080x1080", "1200x628", "800x1000"}

type creativeDeliveryScope struct {
	TargetSize    string   `json:"target_size"`
	ExpectedSizes []string `json:"expected_sizes"`
}

type creativeOrderCapabilityBinding struct {
	Capability    string
	SnapshotField string
}

var creativeOrderCapabilityBindings = []creativeOrderCapabilityBinding{
	{Capability: "generation_plan", SnapshotField: "planner_agent_id"},
	{Capability: "image_edit", SnapshotField: "producer_agent_id"},
	{Capability: "prime_compose", SnapshotField: "prime_agent_id"},
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
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to recover creative order")
			return
		}
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
	if err := h.validateCustomCreativeOrderCopyFacts(r.Context(), workspaceID, input.InputSnapshot, input.Items); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
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
		candidateID, valid := parseUUIDOrBadRequest(w, item.CandidateID, "candidate_id")
		if !valid {
			return
		}
		analysisID, valid := optionalUUIDOrBadRequest(w, item.SourceAnalysisID, "source_analysis_id")
		if !valid {
			return
		}
		var referencesValid bool
		if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2)
  AND ($3::uuid IS NULL OR EXISTS(SELECT 1 FROM creative_source_analysis WHERE id = $3 AND candidate_id = $1 AND workspace_id = $2))
`, candidateID, workspaceID, analysisID).Scan(&referencesValid); err != nil || !referencesValid {
			writeError(w, http.StatusUnprocessableEntity, "order item references do not belong to this workspace")
			return
		}
		if _, err := tx.Exec(r.Context(), `
INSERT INTO creative_order_item (order_id, candidate_id, source_analysis_id, copy_snapshot, direction)
VALUES ($1,$2,$3,$4::jsonb,$5)
`, order.ID, candidateID, analysisID, item.CopySnapshot, item.Direction); err != nil {
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
SELECT DISTINCT role_skill.config->>'capability', sm.member_id::text
FROM squad_member sm
JOIN agent role_agent ON role_agent.id = sm.member_id AND role_agent.workspace_id = $2 AND role_agent.archived_at IS NULL
JOIN agent_skill role_binding ON role_binding.agent_id = role_agent.id AND role_binding.enabled
JOIN skill role_skill ON role_skill.id = role_binding.skill_id AND role_skill.workspace_id = $2
WHERE sm.squad_id = $1
  AND sm.member_type = 'agent'
  AND role_skill.config->>'kind' = 'creative_role'
  AND role_skill.config->>'capability' = ANY($3::text[])
ORDER BY role_skill.config->>'capability', sm.member_id::text
`, squadID, workspaceID, capabilities)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	resolved := make(map[string][]string, len(bindings))
	for rows.Next() {
		var capability, agentID string
		if err := rows.Scan(&capability, &agentID); err != nil {
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
		if len(agentIDs) > 1 {
			return nil, &creativeOrderSquadValidationError{message: "squad has multiple agents with " + binding.Capability + " capability"}
		}
		if err := setSnapshotString(binding.SnapshotField, agentIDs[0]); err != nil {
			return nil, err
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

func (h *Handler) validateCustomCreativeOrderCopyFacts(ctx context.Context, workspaceID pgtype.UUID, inputSnapshot json.RawMessage, items []creativeOrderItemInput) error {
	snapshots := make([]creativeOrderCopySnapshot, 0, len(items))
	requiresLibrary := false
	for index, item := range items {
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
	library, err := h.loadPublishedCreativeResource(ctx, workspaceID, libraryID, "copy_library")
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
FROM creative_order WHERE workspace_id = $1 ORDER BY updated_at DESC LIMIT 100
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
		if err := h.loadCreativeOrderWorkflowState(r, &order); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to derive creative order status")
			return
		}
		orders = append(orders, order)
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
	writeJSON(w, http.StatusOK, order)
}

// RetryCreativeOrderWorkflowFailure requeues a completed direct creative task
// only when its target variant has not reached a usable terminal state. This is
// separate from issue rerun because creative domain fanout tasks are
// intentionally unbound from issues.
func (h *Handler) RetryCreativeOrderWorkflowFailure(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
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

	// QC action-required results are resolved as one atomic two-lane unit. A
	// single task retry would leave the previous QC resolution in place while
	// moving the variant back to running, which can strand the order forever.
	var workflow string
	err = tx.QueryRow(r.Context(), `
SELECT COALESCE(task.context->>'workflow', '')
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
`, taskID, workspaceID, orderID).Scan(&workflow)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to validate workflow retry")
		return
	}
	if creativeWorkflowRequiresAtomicQCRecovery(workflow) {
		writeError(w, http.StatusConflict, "creative QC failures require atomic dual-lane recovery")
		return
	}

	// This update is both the eligibility fence and the domain-state reset. It
	// locks the target variant before a new task is queued, so a concurrent
	// click cannot enqueue another direct retry after the state has moved on.
	var variantID string
	err = tx.QueryRow(r.Context(), `
UPDATE creative_order_variant variant
SET status = 'running',
    brief = variant.brief - 'error_code' - 'error_message',
    updated_at = now()
FROM creative_order_item item
JOIN agent_task_queue task ON TRUE
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE task.id = $1
  AND assigned_agent.workspace_id = $2
  AND item.id = variant.order_item_id
  AND item.order_id = $3
  AND task.context->>'variant_id' = variant.id::text
  AND task.issue_id IS NULL
  AND task.status = 'completed'
  AND (
    task.attempt < task.max_attempts
    OR (
      task.trigger_evidence_kind = 'creative_order_item_production'
      AND task.context->>'workflow' = 'creative_production'
      AND task.attempt < 3
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
`, taskID, workspaceID, orderID).Scan(&variantID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare workflow retry")
		return
	}
	child, err := h.Queries.WithTx(tx).CreateActionRequiredRetryTask(r.Context(), taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "workflow failure is no longer eligible for retry")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to retry workflow task")
		return
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
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue workflow retry")
		return
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

// RepairCreativeOrderVariantPrimePackage re-runs only the Prime composition
// for a stopped variant. It deliberately reuses the completed generated
// assets from the current revision and leaves creative generation untouched.
func (h *Handler) RepairCreativeOrderVariantPrimePackage(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
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
	if h.TaskService == nil {
		writeError(w, http.StatusServiceUnavailable, "creative Prime task service is unavailable")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start Prime package repair")
		return
	}
	defer tx.Rollback(r.Context())

	var itemID, triggerKind, inputSnapshot, brief, orderStatus, variantStatus string
	var revision int
	var issueID pgtype.UUID
	err = tx.QueryRow(r.Context(), `
SELECT item.id::text, variant.revision, order_row.issue_id, order_row.status, variant.status,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND item.order_id = $2 AND order_row.workspace_id = $3
FOR UPDATE OF variant, order_row
`, variantID, orderID, workspaceID).Scan(&itemID, &revision, &issueID, &orderStatus, &variantStatus, &triggerKind, &inputSnapshot, &brief)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load Prime package repair target")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	if !issueID.Valid {
		writeError(w, http.StatusConflict, "Prime package repair requires an order issue")
		return
	}

	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if len(expectedSizes) != len(standardCreativeAssetSizes) {
		writeError(w, http.StatusConflict, "Prime package repair requires the three standard delivery sizes")
		return
	}
	generatedSizes := map[string]struct{}{}
	rows, err := tx.Query(r.Context(), `
SELECT size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'generated' AND status = 'completed'
FOR UPDATE
`, variantID, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load completed generated assets")
		return
	}
	for rows.Next() {
		var size string
		if err := rows.Scan(&size); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read completed generated assets")
			return
		}
		generatedSizes[size] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read completed generated assets")
		return
	}
	rows.Close()
	if !creativeSizesMatchExpected(generatedSizes, expectedSizes) {
		writeError(w, http.StatusConflict, "all three completed generated assets are required before repairing Prime")
		return
	}
	primedSizes := map[string]struct{}{}
	rows, err = tx.Query(r.Context(), `
SELECT size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
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
	if variantStatus != "action_required" && creativeSizesMatchExpected(primedSizes, expectedSizes) {
		writeError(w, http.StatusConflict, "Prime package repair requires an incomplete Prime package or a variant awaiting action")
		return
	}

	var repairAlreadyUsed bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM activity_log
  WHERE issue_id = $1 AND action = 'creative_prime_package_repair_queued'
    AND details->>'variant_id' = $2::text
    AND details->>'revision' = $3::text
)
`, issueID, variantID, strconv.Itoa(revision)).Scan(&repairAlreadyUsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check Prime package repair limit")
		return
	}
	if repairAlreadyUsed {
		writeError(w, http.StatusConflict, "Prime package repair has already been used for this revision")
		return
	}

	var activeTask bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM agent_task_queue
  WHERE context->>'creative_order_id' = $1::text
    AND context->>'variant_id' = $2::text
    AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
)
`, orderID, variantID).Scan(&activeTask); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check active creative tasks")
		return
	}
	if activeTask {
		writeError(w, http.StatusConflict, "Prime package repair requires no active task for this variant")
		return
	}

	var sourceTaskID, primeAgentID, primeRuntimeID pgtype.UUID
	var sourceContext json.RawMessage
	err = tx.QueryRow(r.Context(), `
SELECT task.id, task.agent_id, task.runtime_id, task.context
FROM agent_task_queue task
JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
WHERE assigned_agent.workspace_id = $1
  AND assigned_agent.archived_at IS NULL
  AND task.context->>'type' = 'creative_domain_task'
  AND task.context->>'workflow' = 'creative_prime'
  AND task.context->>'creative_order_id' = $2::text
  AND task.context->>'variant_id' = $3::text
  AND COALESCE(NULLIF(task.context->>'revision', '')::int, 1) = $4
ORDER BY task.created_at DESC, task.id DESC
LIMIT 1
`, workspaceID, orderID, variantID, revision).Scan(&sourceTaskID, &primeAgentID, &primeRuntimeID, &sourceContext)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "Prime package repair requires a prior Prime task for this revision")
		return
	}
	if err != nil || !primeRuntimeID.Valid {
		writeError(w, http.StatusConflict, "the prior Prime task can no longer be reused")
		return
	}

	var context map[string]any
	if err := json.Unmarshal(sourceContext, &context); err != nil || context == nil {
		writeError(w, http.StatusConflict, "the prior Prime task has an invalid context")
		return
	}
	context["type"] = "creative_domain_task"
	context["workflow"] = "creative_prime"
	context["scope"] = "variant"
	context["subject_id"] = uuidToString(variantID)
	context["item_key"] = fmt.Sprintf("%s:prime-package-repair:r%d", uuidToString(variantID), revision)
	context["creative_order_id"] = uuidToString(orderID)
	context["creative_order_item_id"] = itemID
	context["variant_id"] = uuidToString(variantID)
	context["revision"] = revision
	context["expected_sizes"] = expectedSizes
	context["issue_id"] = uuidToString(issueID)
	context["prime_package_repair"] = true
	context["repair_of_task_id"] = uuidToString(sourceTaskID)
	context["prime_repair_asset_invariant"] = "reuse_completed_generated_assets_only"
	repairContext, err := json.Marshal(context)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare Prime package repair")
		return
	}
	if err := validateCreativeTaskFanoutContext("creative_order_variant_prime", variantID, []service.DirectTaskFanoutItem{{
		ItemKey: fmt.Sprintf("%s:prime-package-repair:r%d", uuidToString(variantID), revision), Context: repairContext,
	}}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to prepare canonical Prime package repair context")
		return
	}

	attr := attribution.DirectHumanRun(userID, attribution.EvidenceKind("creative_order_variant_prime"), variantID)
	task, err := h.Queries.WithTx(tx).CreateAgentTask(r.Context(), db.CreateAgentTaskParams{
		AgentID:              primeAgentID,
		RuntimeID:            primeRuntimeID,
		Priority:             0,
		ForceFreshSession:    pgtype.Bool{Bool: true, Valid: true},
		RequestingUserID:     userID,
		OriginatorUserID:     attr.UserID,
		AccountableUserID:    attr.AccountableUserID,
		OriginatorSource:     pgtype.Text{String: attr.Source.String(), Valid: true},
		TriggerEvidenceKind:  pgtype.Text{String: "creative_order_variant_prime", Valid: true},
		TriggerEvidenceRefID: variantID,
		Context:              repairContext,
	})
	if err != nil {
		writeError(w, http.StatusConflict, "Prime package repair could not be queued")
		return
	}

	details, err := json.Marshal(map[string]any{
		"creative_order_id": uuidToString(orderID),
		"variant_id":        uuidToString(variantID),
		"revision":          revision,
		"repair_of_task_id": uuidToString(sourceTaskID),
		"prime_task_id":     uuidToString(task.ID),
		"asset_invariant":   "reused_completed_generated_assets_only",
		"cleared_qc_state":  true,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record Prime package repair")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_prime_package_repair_queued', $4::jsonb)
`, workspaceID, issueID, userID, details); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record Prime package repair")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2`, variantID, revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC reports")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2`, variantID, revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC resolution")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order_variant SET status = 'running', updated_at = now() WHERE id = $1`, variantID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to resume Prime package repair")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to queue Prime package repair")
		return
	}
	h.TaskService.NotifyTaskEnqueued(r.Context(), task)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "order", "order_id": uuidToString(orderID), "variant_id": uuidToString(variantID), "revision": revision,
		"recovery": "creative_prime_package",
	})
	writeJSON(w, http.StatusOK, creativeOrderPrimePackageRepairResponse{TaskID: uuidToString(task.ID)})
}

// RetryCreativeOrderVariantQC recovers only the two independent QC lanes for a
// completed Prime package. It is intentionally not a creative rework: no
// generated or primed asset is changed or recreated.
func (h *Handler) RetryCreativeOrderVariantQC(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
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

	var itemID, triggerKind, inputSnapshot, brief, orderStatus, variantStatus string
	var revision int
	var issueID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT item.id::text, variant.revision, order_row.issue_id, order_row.status, variant.status,
       order_row.trigger_evidence_kind, order_row.input_snapshot::text, variant.brief::text
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND item.order_id = $2 AND order_row.workspace_id = $3
FOR UPDATE OF variant, order_row
`, variantID, orderID, workspaceID).Scan(&itemID, &revision, &issueID, &orderStatus, &variantStatus, &triggerKind, &inputSnapshot, &brief); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative QC recovery target")
		return
	}
	if orderStatus == "cancelled" {
		writeError(w, http.StatusConflict, "creative order is cancelled")
		return
	}
	if variantStatus != "action_required" {
		writeError(w, http.StatusConflict, "creative QC recovery requires a variant awaiting action")
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
	if len(expectedSizes) != len(standardCreativeAssetSizes) {
		writeError(w, http.StatusConflict, "creative QC recovery requires the three standard delivery sizes")
		return
	}
	primedSizes := map[string]struct{}{}
	rows, err := tx.Query(r.Context(), `
SELECT size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
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
		writeError(w, http.StatusConflict, "all three completed Prime assets are required before retrying QC")
		return
	}
	var recoveryAlreadyUsed bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM activity_log
  WHERE issue_id = $1 AND action = 'creative_qc_recovery_queued'
    AND details->>'variant_id' = $2::text
    AND details->>'revision' = $3::text
    AND created_at > COALESCE((
      SELECT max(repair.created_at)
      FROM activity_log repair
      WHERE repair.issue_id = $1
        AND repair.action = 'creative_prime_package_repair_queued'
        AND repair.details->>'variant_id' = $2::text
        AND repair.details->>'revision' = $3::text
    ), '-infinity'::timestamptz)
)
`, issueID, variantID, strconv.Itoa(revision)).Scan(&recoveryAlreadyUsed); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check creative QC recovery limit")
		return
	}
	if recoveryAlreadyUsed {
		writeError(w, http.StatusConflict, "creative QC recovery has already been used for this revision")
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
    AND context->>'workflow' IN ('creative_qc_technical', 'creative_qc_visual')
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

	failedTaskIDs, err := creativeRecoverableQCTaskIDs(r.Context(), tx, orderID, variantID, revision)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check creative QC recovery eligibility")
		return
	}
	if len(failedTaskIDs) == 0 {
		writeError(w, http.StatusConflict, "creative QC recovery requires a recoverable system or contract failure")
		return
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
SELECT COALESCE(jsonb_agg(jsonb_build_object('lane', lane, 'status', status, 'findings', findings) ORDER BY lane), '[]'::jsonb)::text,
       COALESCE((SELECT jsonb_build_object('outcome', outcome, 'failure_summary', failure_summary, 'finalized_by_task_id', finalized_by_task_id::text)
                 FROM creative_order_variant_qc_resolution
                 WHERE variant_id = $1 AND revision = $2), '{}'::jsonb)::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2
`, variantID, revision).Scan(&priorReports, &priorResolution); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to snapshot previous creative QC")
		return
	}

	contexts := make(map[string]json.RawMessage, 2)
	items := make([]struct {
		lane string
		key  string
	}, 0, 2)
	for _, lane := range []string{"technical", "visual"} {
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
			"expected_sizes":              expectedSizes,
			"issue_id":                    uuidToString(issueID),
			"leader_agent_id":             uuidToString(leaderID),
			"qc_recovery_of_task_ids":     failedTaskIDs,
			"qc_recovery_kind":            "system_or_contract_failure",
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

	details, err := json.Marshal(map[string]any{
		"creative_order_id":      uuidToString(orderID),
		"variant_id":             uuidToString(variantID),
		"revision":               revision,
		"recovery_of_task_ids":   failedTaskIDs,
		"technical_task_id":      uuidToString(created["technical"].ID),
		"visual_task_id":         uuidToString(created["visual"].ID),
		"asset_invariant":        "reused_completed_primed_assets_only",
		"previous_qc_reports":    json.RawMessage(priorReports),
		"previous_qc_resolution": json.RawMessage(priorResolution),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative QC recovery")
		return
	}
	if _, err := tx.Exec(r.Context(), `
INSERT INTO activity_log (workspace_id, issue_id, actor_type, actor_id, action, details)
VALUES ($1, $2, 'member', $3, 'creative_qc_recovery_queued', $4::jsonb)
`, workspaceID, issueID, userID, details); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative QC recovery")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2`, variantID, revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC reports")
		return
	}
	if _, err := tx.Exec(r.Context(), `DELETE FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2`, variantID, revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset previous creative QC resolution")
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
		"recovery": "creative_qc",
	})
	writeJSON(w, http.StatusOK, creativeOrderQCRetryResponse{
		VariantID:       uuidToString(variantID),
		Revision:        revision,
		TechnicalTaskID: uuidToString(created["technical"].ID),
		VisualTaskID:    uuidToString(created["visual"].ID),
	})
}

func creativeOrderQCAgentSnapshot(raw json.RawMessage) (pgtype.UUID, pgtype.UUID, error) {
	var snapshot struct {
		SquadSnapshot struct {
			LeaderAgentID   string `json:"leader_agent_id"`
			ReviewerAgentID string `json:"reviewer_agent_id"`
		} `json:"squad_snapshot"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("creative order has an invalid frozen squad snapshot")
	}
	leaderID, err := uuid.Parse(strings.TrimSpace(snapshot.SquadSnapshot.LeaderAgentID))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("creative order is missing a frozen leader agent")
	}
	reviewerID, err := uuid.Parse(strings.TrimSpace(snapshot.SquadSnapshot.ReviewerAgentID))
	if err != nil {
		return pgtype.UUID{}, pgtype.UUID{}, errors.New("creative order is missing a frozen QC reviewer")
	}
	return pgtype.UUID{Bytes: leaderID, Valid: true}, pgtype.UUID{Bytes: reviewerID, Valid: true}, nil
}

func creativeRecoverableQCTaskIDs(ctx context.Context, tx pgx.Tx, orderID, variantID pgtype.UUID, revision int) ([]string, error) {
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
  AND (
    (
      context->>'workflow' IN ('creative_qc_technical', 'creative_qc_visual')
      AND status = 'failed'
      AND COALESCE(NULLIF(failure_reason, ''), 'agent_error') = ANY($4::text[])
    )
    OR (
      status = 'completed'
      AND EXISTS (
        SELECT 1
        FROM creative_order_qc_report report
        WHERE report.variant_id = $2::uuid
          AND report.revision = $3
          AND report.lane = CASE
            WHEN context->>'workflow' = 'creative_qc_technical' THEN 'technical'
            WHEN context->>'workflow' = 'creative_qc_visual' THEN 'visual'
            WHEN context->>'workflow' = 'creative_qc' THEN context->>'lane'
            ELSE ''
          END
          AND report.status = 'failed'
          AND EXISTS (
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
          )
      )
    )
  )
ORDER BY created_at, id
`, orderID, variantID, revision, []string{
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
	var status string
	var allItemsAdopted bool
	if err := tx.QueryRow(r.Context(), `
SELECT o.issue_id,
  o.status,
  COALESCE((SELECT count(*) > 0 AND bool_and(i.adopted_variant_id IS NOT NULL)
    FROM creative_order_item i WHERE i.order_id = o.id), false)
FROM creative_order o
WHERE o.id = $1 AND o.workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&issueID, &status, &allItemsAdopted); errors.Is(err, pgx.ErrNoRows) {
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

	if status != "cancelled" {
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order SET status = 'cancelled', updated_at = now()
WHERE id = $1 AND workspace_id = $2
`, orderID, workspaceID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to cancel creative order")
			return
		}
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

	if issueID.Valid && h.TaskService != nil {
		if err := h.TaskService.CancelTasksForIssue(r.Context(), issueID); err != nil {
			writeError(w, http.StatusInternalServerError, "creative order was closed but active tasks could not be cancelled")
			return
		}
	}
	rows, err := h.DB.Query(r.Context(), `
UPDATE agent_task_queue
SET status = 'cancelled', completed_at = now()
WHERE issue_id IS NULL
  AND context->>'type' = 'creative_domain_task'
  AND context->>'creative_order_id' = $1::text
  AND status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')

RETURNING agent_id
`, orderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "creative order was closed but direct tasks could not be cancelled")
		return
	}
	for rows.Next() {
		var agentID pgtype.UUID
		if err := rows.Scan(&agentID); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read cancelled direct tasks")
			return
		}
		if h.TaskService != nil {
			h.TaskService.ReconcileAgentStatus(r.Context(), agentID)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to cancel direct tasks")
		return
	}
	rows.Close()

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

	var issueID, previousVariantID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT o.issue_id, i.adopted_variant_id
FROM creative_order_item i
JOIN creative_order o ON o.id = i.order_id
WHERE i.id = $1 AND o.id = $2 AND o.workspace_id = $3
FOR UPDATE OF i
`, itemID, orderID, workspaceID).Scan(&issueID, &previousVariantID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order item")
		return
	}

	var variantKey, variantStatus string
	var revision int
	if err := tx.QueryRow(r.Context(), `
SELECT variant_key, revision, status
FROM creative_order_variant
WHERE id = $1 AND order_item_id = $2
FOR UPDATE
`, variantID, itemID).Scan(&variantKey, &revision, &variantStatus); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "variant does not belong to this creative order item")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order variant")
		return
	}
	var technicalStatus, visualStatus, qcOutcome string
	if err := tx.QueryRow(r.Context(), `
SELECT
  COALESCE((SELECT status FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2 AND lane = 'technical'), ''),
  COALESCE((SELECT status FROM creative_order_qc_report WHERE variant_id = $1 AND revision = $2 AND lane = 'visual'), ''),
  COALESCE((SELECT outcome FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2), '')
`, variantID, revision).Scan(&technicalStatus, &visualStatus, &qcOutcome); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative variant QC state")
		return
	}
	standardAdoption := creativeQCStatusAllowsAdoption(technicalStatus) && creativeQCStatusAllowsAdoption(visualStatus) && qcOutcome == "delivered"
	riskAdoption := (technicalStatus == "failed" || visualStatus == "failed") && qcOutcome == "action_required"
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
		if variantStatus != "action_required" {
			writeError(w, http.StatusConflict, "failed creative variant is not awaiting manual action")
			return
		}
	} else {
		writeError(w, http.StatusConflict, "creative variant QC is not finalized for adoption")
		return
	}

	var qcReportsJSON, qcResolutionJSON string
	if err := tx.QueryRow(r.Context(), `
SELECT
  COALESCE(jsonb_agg(jsonb_build_object(
    'id', id::text, 'lane', lane, 'revision', revision, 'status', status, 'findings', findings,
    'created_at', created_at, 'updated_at', updated_at
  ) ORDER BY lane), '[]'::jsonb)::text,
  COALESCE((SELECT jsonb_build_object(
    'outcome', outcome, 'failure_summary', failure_summary,
    'finalized_by_task_id', COALESCE(finalized_by_task_id::text, '')
  ) FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2), '{}'::jsonb)::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2
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
`, variantID, revision, standardCreativeAssetSizes).Scan(&missingPrimeSizes); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative variant Prime package")
		return
	}
	if len(missingPrimeSizes) > 0 {
		writeError(w, http.StatusConflict, "creative variant Prime package is incomplete; missing primed assets for: "+strings.Join(missingPrimeSizes, ", "))
		return
	}

	if riskAdoption {
		if _, err := copyCreativePrimedAssetsToDelivered(r.Context(), tx, variantID, revision, standardCreativeAssetSizes); err != nil {
			writeError(w, http.StatusConflict, err.Error())
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
`, variantID, revision, standardCreativeAssetSizes).Scan(&missingSizes); err != nil {
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
		"delivery_package_sizes":      standardCreativeAssetSizes,
		"adoption_mode":               adoptionMode,
		"qc_risk_acknowledged":        riskAdoption,
		"qc_risk_reason":              feedbackComment,
		"qc_snapshot": map[string]any{
			"technical_status": technicalStatus,
			"visual_status":    visualStatus,
			"outcome":          qcOutcome,
			"reports":          json.RawMessage(qcReportsJSON),
			"resolution":       json.RawMessage(qcResolutionJSON),
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
	var itemExists bool
	var currentRevision int
	var currentStatus string
	var triggerKind string
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(SELECT 1 FROM creative_order_item i JOIN creative_order o ON o.id = i.order_id WHERE i.id = $1 AND i.order_id = $2 AND o.workspace_id = $3),
  COALESCE((SELECT v.revision FROM creative_order_variant v WHERE v.order_item_id = $1 AND v.variant_key = $4), 0),
  COALESCE((SELECT v.status FROM creative_order_variant v WHERE v.order_item_id = $1 AND v.variant_key = $4), ''),
  COALESCE((SELECT o.trigger_evidence_kind FROM creative_order_item i JOIN creative_order o ON o.id = i.order_id WHERE i.id = $1), '')
`, itemID, orderID, workspaceID, input.VariantKey).Scan(&itemExists, &currentRevision, &currentStatus, &triggerKind); err != nil || !itemExists {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	}
	maxRevision := creativeOrderVariantMaxRevision(triggerKind, currentRevision, currentStatus)
	if input.Revision > maxRevision || (currentRevision == 0 && input.Revision != 1) || (currentRevision > 0 && input.Revision > currentRevision+1) {
		writeError(w, http.StatusConflict, "creative order variant rework limit exceeded")
		return
	}
	variant, err := scanCreativeOrderVariant(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_order_variant (order_item_id, variant_key, brief, revision, status)
VALUES ($1,$2,$3::jsonb,$4,$5)
ON CONFLICT (order_item_id, variant_key) DO UPDATE SET
  brief = EXCLUDED.brief, revision = EXCLUDED.revision, status = EXCLUDED.status, updated_at = now()
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
	false,
	false,
  created_at::text, updated_at::text
`, itemID, input.VariantKey, input.Brief, input.Revision, input.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "creative order variant revision is stale")
		return
	}
	if err != nil {
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
	var referencesValid bool
	err = h.DB.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_order_variant v
  JOIN creative_order_item i ON i.id = v.order_item_id
  JOIN creative_order o ON o.id = i.order_id
  WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
) AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM attachment WHERE id = $4 AND workspace_id = $3))
  AND ($5::uuid IS NULL OR EXISTS(SELECT 1 FROM creative_order_asset WHERE id = $5 AND variant_id = $1))
`, variantID, orderID, workspaceID, attachmentID, derivedFromAssetID).Scan(&referencesValid)
	if err != nil || !referencesValid {
		writeError(w, http.StatusUnprocessableEntity, "asset references do not belong to this creative order")
		return
	}
	var variantRevision int
	var triggerKind, inputSnapshot, brief string
	if err := h.DB.QueryRow(r.Context(), `
SELECT v.revision, o.trigger_evidence_kind, o.input_snapshot::text, v.brief::text
FROM creative_order_variant v
JOIN creative_order_item i ON i.id = v.order_item_id
JOIN creative_order o ON o.id = i.order_id
WHERE v.id = $1
`, variantID).Scan(&variantRevision, &triggerKind, &inputSnapshot, &brief); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative order variant")
		return
	}
	if input.Revision != variantRevision {
		writeError(w, http.StatusConflict, "creative order asset revision is stale")
		return
	}
	expectedSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(brief))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if !creativeSizeIsExpected(input.SizeKey, expectedSizes) {
		writeError(w, http.StatusUnprocessableEntity, "asset size is outside this creative variant's delivery scope")
		return
	}
	asset, err := scanCreativeOrderAsset(h.DB.QueryRow(r.Context(), `
INSERT INTO creative_order_asset (variant_id, asset_family_id, size_key, revision, stage, attachment_id, derived_from_asset_id, metadata, evidence, status)
VALUES ($1,COALESCE($2::uuid, gen_random_uuid()),$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10)
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET asset_family_id = CASE
    WHEN creative_order_asset.stage = 'generated' AND creative_order_asset.status = 'completed'
      THEN creative_order_asset.asset_family_id
    ELSE EXCLUDED.asset_family_id
  END,
  attachment_id = EXCLUDED.attachment_id, derived_from_asset_id = EXCLUDED.derived_from_asset_id,
  metadata = EXCLUDED.metadata, evidence = EXCLUDED.evidence, status = EXCLUDED.status, updated_at = now()
WHERE creative_order_asset.stage <> 'generated'
   OR creative_order_asset.status <> 'completed'
   OR (
     creative_order_asset.attachment_id IS NOT DISTINCT FROM EXCLUDED.attachment_id
     AND creative_order_asset.derived_from_asset_id IS NOT DISTINCT FROM EXCLUDED.derived_from_asset_id
     AND creative_order_asset.metadata = EXCLUDED.metadata
     AND creative_order_asset.evidence = EXCLUDED.evidence
   )
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variantID, assetFamilyID, input.SizeKey, input.Revision, input.Stage, attachmentID, derivedFromAssetID, input.Metadata, input.Evidence, input.Status))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusConflict, "completed generated asset trace is immutable")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order asset")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, asset)
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
	var input creativeOrderQCInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order QC report")
		return
	}
	input, err := normalizeCreativeOrderQC(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, input.VariantID, "variant_id")
	if !ok {
		return
	}
	if _, ok := h.creativeQCActiveTask(w, r, orderID, variantID, input.Revision, input.Lane); !ok {
		return
	}
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
  WHERE variant_id = $1 AND revision = $2
)
`, variantID, input.Revision).Scan(&finalized); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check creative QC resolution")
		return
	}
	if finalized {
		writeError(w, http.StatusConflict, "creative order QC revision is already finalized")
		return
	}
	var qcID string
	err = tx.QueryRow(r.Context(), `
INSERT INTO creative_order_qc_report (variant_id, lane, revision, status, findings, trigger_evidence_kind, trigger_evidence_ref_id)
VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7)
ON CONFLICT (variant_id, lane, revision) DO UPDATE SET status = EXCLUDED.status, findings = EXCLUDED.findings,
  trigger_evidence_kind = EXCLUDED.trigger_evidence_kind, trigger_evidence_ref_id = EXCLUDED.trigger_evidence_ref_id, updated_at = now()
RETURNING id::text
`, variantID, input.Lane, input.Revision, input.Status, input.Findings, input.TriggerEvidenceKind, evidenceID).Scan(&qcID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order QC report")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative order QC report")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	writeJSON(w, http.StatusOK, map[string]any{"id": qcID, "variant_id": input.VariantID, "lane": input.Lane, "revision": input.Revision, "status": input.Status})
}

// FinalizeCreativeOrderQC is the only barrier between independently-run QC
// lanes and an externally visible delivery. It locks the variant row, so two
// lanes may race to call it but only one can create the resolution and inbox
// notification for a revision.
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

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative QC finalization")
		return
	}
	defer tx.Rollback(r.Context())

	var variantRevision int
	var variantKey, triggerKind, inputSnapshot, brief string
	var issueID pgtype.UUID
	var createdBy pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT v.revision, v.variant_key, o.issue_id, o.created_by, o.trigger_evidence_kind, o.input_snapshot::text, v.brief::text
FROM creative_order_variant v
JOIN creative_order_item i ON i.id = v.order_item_id
JOIN creative_order o ON o.id = i.order_id
WHERE v.id = $1 AND o.id = $2 AND o.workspace_id = $3
FOR UPDATE OF v
`, variantID, orderID, workspaceID).Scan(&variantRevision, &variantKey, &issueID, &createdBy, &triggerKind, &inputSnapshot, &brief); err != nil {
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

	reportStatuses := map[string]string{}
	failureSummary := map[string]json.RawMessage{}
	rows, err := tx.Query(r.Context(), `
SELECT lane, status, findings::text
FROM creative_order_qc_report
WHERE variant_id = $1 AND revision = $2
FOR UPDATE
`, variantID, input.Revision)
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
		blockingFailures, findingsErr := creativeQCFindingsHaveBlockingFailures(json.RawMessage(findings))
		if findingsErr != nil || blockingFailures {
			status = "failed"
		}
		reportStatuses[lane] = status
		failureSummary[lane] = json.RawMessage(findings)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read creative QC reports")
		return
	}
	rows.Close()

	response := creativeOrderQCFinalizeResponse{
		VariantID:       input.VariantID,
		Revision:        input.Revision,
		TechnicalStatus: reportStatuses["technical"],
		VisualStatus:    reportStatuses["visual"],
		Outcome:         "pending",
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
	if reportStatuses["technical"] == "failed" || reportStatuses["visual"] == "failed" {
		outcome = "action_required"
	}
	failureSummaryJSON, err := json.Marshal(failureSummary)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode creative QC findings")
		return
	}
	var resolutionCreated bool
	err = tx.QueryRow(r.Context(), `
INSERT INTO creative_order_variant_qc_resolution (
  variant_id, revision, outcome, finalized_by_task_id, issue_id, failure_summary
)
VALUES ($1, $2, $3, $4, $5, $6::jsonb)
ON CONFLICT (variant_id, revision) DO NOTHING
RETURNING true
`, variantID, input.Revision, outcome, task.ID, issueID, failureSummaryJSON).Scan(&resolutionCreated)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to resolve creative QC")
		return
	}
	if !resolutionCreated {
		if err := tx.QueryRow(r.Context(), `
SELECT outcome FROM creative_order_variant_qc_resolution WHERE variant_id = $1 AND revision = $2
`, variantID, input.Revision).Scan(&response.Outcome); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read creative QC resolution")
			return
		}
		response.Finalized = true
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to finalize creative QC")
			return
		}
		response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
		writeJSON(w, http.StatusOK, response)
		return
	}

	response.Created = true
	response.Finalized = true
	response.Outcome = outcome
	if outcome == "delivered" {
		count, err := copyCreativePrimedAssetsToDelivered(r.Context(), tx, variantID, input.Revision, expectedSizes)
		if err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		response.DeliveredAssetCount = count
		if _, err := tx.Exec(r.Context(), `UPDATE creative_order_variant SET status = 'completed', updated_at = now() WHERE id = $1`, variantID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to complete creative variant")
			return
		}
	} else if _, err := tx.Exec(r.Context(), `UPDATE creative_order_variant SET status = 'action_required', updated_at = now() WHERE id = $1`, variantID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to require creative QC action")
		return
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order after QC")
		return
	}

	inboxDetails, _ := json.Marshal(map[string]any{
		"creative_order_id": uuidToString(orderID), "variant_id": input.VariantID,
		"revision": input.Revision, "outcome": outcome,
	})
	inbox, err := h.Queries.WithTx(tx).CreateInboxItem(r.Context(), db.CreateInboxItemParams{
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
WHERE variant_id = $1 AND revision = $2
`, variantID, input.Revision, inbox.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to record creative QC notification")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to finalize creative QC")
		return
	}

	response.OrderAggregateStatus, _ = h.derivedCreativeOrderStatus(r, orderID)
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{"scope": "order", "order_id": chi.URLParam(r, "id")})
	h.publishCreativeQCInbox(workspaceID, task.AgentID, inbox, response.OrderAggregateStatus)
	writeJSON(w, http.StatusOK, response)
}

type creativeQCTaskContext struct {
	Type            string `json:"type"`
	Workflow        string `json:"workflow"`
	CreativeOrderID string `json:"creative_order_id"`
	VariantID       string `json:"variant_id"`
	Revision        int    `json:"revision"`
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

func creativeQCLanesComplete(statuses map[string]string) bool {
	return statuses["technical"] != "" && statuses["technical"] != "pending" &&
		statuses["visual"] != "" && statuses["visual"] != "pending"
}

func copyCreativePrimedAssetsToDelivered(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string) (int, error) {
	rows, err := tx.Query(ctx, `
SELECT id, size_key
FROM creative_order_asset
WHERE variant_id = $1 AND revision = $2 AND stage = 'primed' AND status = 'completed'
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

func creativeQCInboxSeverity(outcome string) string {
	if outcome == "action_required" {
		return "action_required"
	}
	return "info"
}

func creativeQCInboxTitle(variantKey, outcome string) string {
	if outcome == "action_required" {
		return "Creative QC needs a decision: " + variantKey
	}
	return "Creative variant ready: " + variantKey
}

func creativeQCInboxBody(variantKey, outcome string, expectedSizeCount int) string {
	if outcome == "action_required" {
		return "QC found a blocking issue for " + variantKey + ". Review the comparison and choose whether to revise or accept the risk."
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
	seen := map[string]struct{}{}
	for index := range input.Items {
		item := &input.Items[index]
		item.CandidateID = strings.TrimSpace(item.CandidateID)
		item.SourceAnalysisID = strings.TrimSpace(item.SourceAnalysisID)
		item.Direction = strings.TrimSpace(item.Direction)
		if item.CandidateID == "" {
			return input, errors.New("invalid creative order item")
		}
		if len(item.Direction) > maxCreativeOrderDirectionLength {
			return input, errors.New("production prompt exceeds the maximum supported length")
		}
		if _, exists := seen[item.CandidateID]; exists {
			return input, errors.New("duplicate candidate_id")
		}
		seen[item.CandidateID] = struct{}{}
		item.CopySnapshot, err = normalizedOptionalJSONObject(item.CopySnapshot)
		if err != nil {
			return input, errors.New("copy_snapshot must be an object")
		}
	}
	return input, nil
}

func normalizeCreativeOrderVariant(input creativeOrderVariantInput) (creativeOrderVariantInput, error) {
	input.OrderItemID = strings.TrimSpace(input.OrderItemID)
	input.VariantKey = strings.TrimSpace(input.VariantKey)
	input.Status = strings.TrimSpace(input.Status)
	if input.Revision == 0 {
		input.Revision = 1
	}
	if input.OrderItemID == "" || input.VariantKey == "" || input.Revision < 1 || len(input.VariantKey) > 64 || !validCreativeVariantStatus(input.Status) {
		return input, errors.New("invalid creative order variant")
	}
	var err error
	input.Brief, err = normalizedOptionalJSONObject(input.Brief)
	if err != nil {
		return input, errors.New("brief must be an object")
	}
	return input, nil
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
	input.Status = strings.TrimSpace(input.Status)
	if input.VariantID == "" || input.Revision < 1 || !validCreativeAssetSize(input.SizeKey) || !validCreativeAssetStage(input.Stage) || !validCreativeAssetStatus(input.Status) {
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
	if input.Stage == "generated" && input.Status == "completed" {
		if err := validateCompletedGeneratedAssetTrace(input.Metadata, input.Evidence); err != nil {
			return input, err
		}
	}
	return input, nil
}

func validateCompletedGeneratedAssetTrace(metadata, evidence json.RawMessage) error {
	var metadataTrace struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	if err := json.Unmarshal(metadata, &metadataTrace); err != nil {
		return errors.New("generated asset metadata is invalid")
	}
	if strings.TrimSpace(metadataTrace.Prompt) == "" {
		return errors.New("completed generated asset metadata.prompt is required")
	}
	if strings.TrimSpace(metadataTrace.Model) != "gpt-image-2" {
		return errors.New("completed generated asset metadata.model must be gpt-image-2")
	}
	var evidenceTrace struct {
		RequestID    string `json:"request_id"`
		Attempts     int    `json:"attempts"`
		PromptSHA256 string `json:"prompt_sha256"`
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
	if input.VariantID == "" || input.Revision < 1 || (input.Lane != "technical" && input.Lane != "visual") || !validCreativeQCStatus(input.Status) {
		return input, errors.New("invalid creative order QC report")
	}
	var err error
	input.Findings, err = normalizedOptionalJSONObject(input.Findings)
	if err != nil {
		return input, errors.New("findings must be an object")
	}
	blockingFailures, err := creativeQCFindingsHaveBlockingFailures(input.Findings)
	if err != nil {
		return input, err
	}
	if blockingFailures {
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

func validCreativeAssetStage(stage string) bool {
	return stage == "generated" || stage == "primed" || stage == "delivered"
}

func validCreativeAssetStatus(status string) bool {
	return status == "queued" || status == "running" || status == "completed" || status == "failed" || status == "cancelled"
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
	err := row.Scan(&variant.ID, &variant.OrderItemID, &variant.VariantKey, &brief, &variant.Revision, &variant.Status, &variant.QCRecoveryUsed, &variant.QCRecoveryAvailable, &variant.PrimeRepairUsed, &variant.PrimeRepairAvailable, &variant.CreatedAt, &variant.UpdatedAt)
	variant.Brief = json.RawMessage(brief)
	return variant, err
}

func scanCreativeOrderAsset(row rowScanner) (creativeOrderAssetResponse, error) {
	var asset creativeOrderAssetResponse
	var metadata, evidence string
	err := row.Scan(&asset.ID, &asset.VariantID, &asset.AssetFamilyID, &asset.SizeKey, &asset.Revision, &asset.Stage, &asset.AttachmentID, &asset.DerivedFromAssetID, &metadata, &evidence, &asset.Status, &asset.CreatedAt, &asset.UpdatedAt)
	asset.Metadata = json.RawMessage(metadata)
	asset.Evidence = json.RawMessage(evidence)
	return asset, err
}

func (h *Handler) derivedCreativeOrderStatus(r *http.Request, orderID pgtype.UUID) (string, error) {
	failures, err := h.listCreativeOrderWorkflowFailures(r, orderID)
	if err != nil {
		return "", err
	}
	return h.derivedCreativeOrderStatusWithFailures(r, orderID, len(failures) > 0)
}

func (h *Handler) loadCreativeOrderWorkflowState(r *http.Request, order *creativeOrderResponse) error {
	orderID := parseUUID(order.ID)
	failures, err := h.listCreativeOrderWorkflowFailures(r, orderID)
	if err != nil {
		return err
	}
	status, err := h.derivedCreativeOrderStatusWithFailures(r, orderID, len(failures) > 0)
	if err != nil {
		return err
	}
	order.WorkflowFailures = failures
	order.DerivedStatus = status
	return nil
}

func (h *Handler) listCreativeOrderWorkflowFailures(r *http.Request, orderID pgtype.UUID) ([]creativeOrderWorkflowFailureResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
WITH ranked AS (
  SELECT q.*,
    CASE
      WHEN jsonb_typeof(q.context->'expected_sizes') = 'array'
        AND jsonb_array_length(q.context->'expected_sizes') > 0
      THEN q.context->'expected_sizes'
      ELSE '["1080x1080","1200x628","800x1000"]'::jsonb
    END AS workflow_expected_sizes,
    row_number() OVER (
      PARTITION BY q.agent_id,
        COALESCE(q.trigger_evidence_kind, ''),
        COALESCE(q.trigger_evidence_ref_id::text, ''),
        COALESCE(q.context->>'item_key', '')
      ORDER BY q.created_at DESC, q.id DESC
    ) AS row_number
  FROM agent_task_queue q
  JOIN agent a ON a.id = q.agent_id
  JOIN creative_order o ON o.id = $1 AND o.workspace_id = a.workspace_id
  WHERE q.context->>'type' = 'creative_domain_task'
    AND (
      q.context->>'creative_order_id' = o.id::text
      OR (
        q.trigger_evidence_ref_id = o.id
        AND NULLIF(q.context->>'variant_id', '') IS NOT NULL
        AND EXISTS (
          SELECT 1
          FROM creative_order_variant linked_variant
          JOIN creative_order_item linked_item ON linked_item.id = linked_variant.order_item_id
          WHERE linked_variant.id::text = q.context->>'variant_id'
            AND linked_item.order_id = o.id
        )
      )
    )
)
SELECT id::text,
  agent_id::text,
  COALESCE(context->>'workflow', ''),
  COALESCE(NULLIF(context->>'scope', ''),
    CASE
      WHEN NULLIF(context->>'variant_id', '') IS NOT NULL THEN 'variant'
      WHEN NULLIF(context->>'creative_order_item_id', '') IS NOT NULL THEN 'order_item'
      ELSE 'order'
    END),
  COALESCE(NULLIF(context->>'subject_id', ''),
    CASE COALESCE(NULLIF(context->>'scope', ''),
      CASE
        WHEN NULLIF(context->>'variant_id', '') IS NOT NULL THEN 'variant'
        WHEN NULLIF(context->>'creative_order_item_id', '') IS NOT NULL THEN 'order_item'
        ELSE 'order'
      END)
      WHEN 'variant' THEN NULLIF(context->>'variant_id', '')
      WHEN 'order_item' THEN NULLIF(context->>'creative_order_item_id', '')
      WHEN 'order' THEN NULLIF(context->>'creative_order_id', '')
      ELSE NULL
    END,
    NULLIF(context->>'variant_id', ''),
    NULLIF(context->>'creative_order_item_id', ''),
    NULLIF(context->>'creative_order_id', ''),
    COALESCE(trigger_evidence_ref_id::text, '')),
  COALESCE(context->>'item_key', ''),
  COALESCE(trigger_evidence_kind, ''),
  COALESCE(trigger_evidence_ref_id::text, ''),
  CASE
    WHEN status = 'completed' THEN 'agent_reported_action_required'
    ELSE COALESCE(NULLIF(failure_reason, ''), 'agent_error')
  END,
  CASE
    WHEN status = 'completed' THEN COALESCE((
      SELECT message.content
      FROM task_message message
      WHERE message.task_id = ranked.id
        AND message.type = 'text'
        AND btrim(message.content) <> ''
      ORDER BY message.seq DESC, message.id DESC
      LIMIT 1
    ), '')
    ELSE COALESCE(error, '')
  END,
  COALESCE(completed_at, created_at)::text,
  (
    attempt < max_attempts
    OR (
      trigger_evidence_kind = 'creative_order_item_production'
      AND context->>'workflow' = 'creative_production'
      AND attempt < 3
    )
  )
FROM ranked
WHERE row_number = 1
AND (
  NULLIF(context->>'variant_id', '') IS NULL
  OR EXISTS (
    SELECT 1
    FROM creative_order_variant variant
    JOIN creative_order_item item ON item.id = variant.order_item_id
    WHERE variant.id::text = context->>'variant_id'
      AND item.order_id = $1
      AND COALESCE(NULLIF(NULLIF(context->>'revision', '')::int, 0), variant.revision) = variant.revision
  )
)
AND (
  status = 'failed'
  OR (
    status = 'completed'
    AND NULLIF(context->>'variant_id', '') IS NOT NULL
    AND EXISTS(
      SELECT 1
      FROM creative_order_variant variant
      JOIN creative_order_item item ON item.id = variant.order_item_id
      WHERE variant.id::text = context->>'variant_id'
        AND item.order_id = $1
        AND variant.status NOT IN ('completed', 'cancelled')
        AND (
          COALESCE(context->>'workflow', '') NOT IN ('creative_production', 'creative_prime')
          OR (
            SELECT count(DISTINCT asset.size_key)
            FROM creative_order_asset asset
            WHERE asset.variant_id = variant.id
              AND asset.revision = variant.revision
              AND asset.stage = CASE COALESCE(context->>'workflow', '')
                WHEN 'creative_production' THEN 'generated'
                WHEN 'creative_prime' THEN 'primed'
              END
              AND asset.status = 'completed'
              AND asset.size_key IN (
                SELECT jsonb_array_elements_text(workflow_expected_sizes)
              )
          ) < jsonb_array_length(workflow_expected_sizes)
        )
    )
  )
)
ORDER BY COALESCE(completed_at, created_at), id
`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	failures := []creativeOrderWorkflowFailureResponse{}
	for rows.Next() {
		var failure creativeOrderWorkflowFailureResponse
		if err := rows.Scan(
			&failure.TaskID,
			&failure.AgentID,
			&failure.Workflow,
			&failure.Scope,
			&failure.SubjectID,
			&failure.ItemKey,
			&failure.TriggerEvidenceKind,
			&failure.TriggerEvidenceReference,
			&failure.FailureReason,
			&failure.Error,
			&failure.FailedAt,
			&failure.Retryable,
		); err != nil {
			return nil, err
		}
		failures = append(failures, failure)
	}
	return failures, rows.Err()
}

func (h *Handler) derivedCreativeOrderStatusWithFailures(r *http.Request, orderID pgtype.UUID, hasOpenFailures bool) (string, error) {
	var status string
	err := h.DB.QueryRow(r.Context(), `
WITH aggregate AS (
  SELECT count(v.id) AS total,
    count(DISTINCT i.id) AS item_total,
    count(DISTINCT i.id) FILTER (WHERE i.adopted_variant_id IS NOT NULL) AS adopted_items,
	count(DISTINCT i.id) FILTER (WHERE v.status = 'completed') AS reviewable_items,
	COALESCE(max(o.status), 'draft') AS order_status,
    COALESCE(max(o.trigger_evidence_kind), '') AS trigger_evidence_kind,
    count(*) FILTER (WHERE v.status = 'failed') AS failed,
    count(*) FILTER (WHERE v.status = 'action_required') AS action_required,
    count(*) FILTER (WHERE v.status = 'running') AS running,
    count(*) FILTER (WHERE v.status = 'partial') AS partial,
    count(*) FILTER (WHERE v.status = 'completed') AS completed,
    count(*) FILTER (WHERE v.status = 'cancelled') AS cancelled,
    (SELECT count(*)
      FROM agent_task_queue q
      WHERE q.context->>'type' = 'creative_domain_task'
        AND (
          q.context->>'creative_order_id' = $1::uuid::text
          OR (
            q.trigger_evidence_ref_id = $1::uuid
            AND NULLIF(q.context->>'variant_id', '') IS NOT NULL
            AND EXISTS (
              SELECT 1
              FROM creative_order_variant linked_variant
              JOIN creative_order_item linked_item ON linked_item.id = linked_variant.order_item_id
              WHERE linked_variant.id::text = q.context->>'variant_id'
                AND linked_item.order_id = $1::uuid
            )
          )
        )
        AND q.status IN ('queued', 'dispatched', 'running')) AS active_tasks,
    (SELECT count(*)
      FROM agent_task_queue q
      WHERE q.context->>'type' = 'creative_domain_task'
        AND (
          q.context->>'creative_order_id' = $1::uuid::text
          OR (
            q.trigger_evidence_ref_id = $1::uuid
            AND NULLIF(q.context->>'variant_id', '') IS NOT NULL
            AND EXISTS (
              SELECT 1
              FROM creative_order_variant linked_variant
              JOIN creative_order_item linked_item ON linked_item.id = linked_variant.order_item_id
              WHERE linked_variant.id::text = q.context->>'variant_id'
                AND linked_item.order_id = $1::uuid
            )
          )
        )
        AND q.status IN ('dispatched', 'running')) AS started_tasks
  FROM creative_order o
  LEFT JOIN creative_order_item i ON i.order_id = o.id
  LEFT JOIN creative_order_variant v ON v.order_item_id = i.id
  WHERE o.id = $1
), base AS (
  SELECT aggregate.*,
    CASE
      WHEN order_status = 'cancelled' THEN 'cancelled'
      WHEN item_total > 0 AND adopted_items = item_total THEN 'completed'
      WHEN total > 0 AND completed = total AND trigger_evidence_kind = 'creative_direct_edit' THEN 'completed'
      WHEN item_total > 0 AND reviewable_items = item_total THEN 'awaiting_adoption'
      WHEN active_tasks > 0 AND (completed > 0 OR action_required > 0 OR failed > 0 OR partial > 0) THEN 'partial'
      WHEN active_tasks > 0 AND started_tasks > 0 THEN 'running'
      WHEN active_tasks > 0 THEN 'queued'
      WHEN action_required > 0 OR failed > 0 OR partial > 0 OR completed > 0 OR running > 0 THEN 'action_required'
      WHEN total > 0 AND cancelled = total THEN 'cancelled'
      WHEN total = 0 AND order_status IN ('failed', 'action_required') THEN 'action_required'
      WHEN total = 0 AND order_status = 'draft' THEN 'draft'
      ELSE 'queued'
    END AS status
  FROM aggregate
)
SELECT CASE
	WHEN NOT $2::boolean THEN status
	WHEN status IN ('completed', 'awaiting_adoption', 'cancelled') THEN status
	WHEN active_tasks > 0 THEN 'partial'
	ELSE 'action_required'
END FROM base
`, orderID, hasOpenFailures).Scan(&status)
	return status, err
}

func (h *Handler) listCreativeOrderItems(r *http.Request, orderID pgtype.UUID) ([]creativeOrderItemResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, COALESCE(adopted_variant_id::text, ''), COALESCE(adopted_at::text, ''),
  COALESCE(adopted_by::text, ''), created_at::text, updated_at::text
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
		item.Variants, err = h.listCreativeOrderVariants(r, parseUUID(item.ID))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanCreativeOrderItemWithAdoption(row rowScanner) (creativeOrderItemResponse, error) {
	var item creativeOrderItemResponse
	var snapshot string
	err := row.Scan(&item.ID, &item.OrderID, &item.CandidateID, &item.SourceAnalysisID, &snapshot,
		&item.Direction, &item.Status, &item.AdoptedVariantID, &item.AdoptedAt, &item.AdoptedBy,
		&item.CreatedAt, &item.UpdatedAt)
	item.CopySnapshot = json.RawMessage(snapshot)
	return item, err
}

func (h *Handler) loadCreativeOrderItem(r *http.Request, itemID pgtype.UUID) (creativeOrderItemResponse, error) {
	item, err := scanCreativeOrderItemWithAdoption(h.DB.QueryRow(r.Context(), `
SELECT id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, COALESCE(adopted_variant_id::text, ''), COALESCE(adopted_at::text, ''),
  COALESCE(adopted_by::text, ''), created_at::text, updated_at::text
FROM creative_order_item WHERE id = $1
`, itemID))
	if err != nil {
		return creativeOrderItemResponse{}, err
	}
	item.Variants, err = h.listCreativeOrderVariants(r, itemID)
	return item, err
}

func (h *Handler) listCreativeOrderVariants(r *http.Request, itemID pgtype.UUID) ([]creativeOrderVariantResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT variant.id::text, variant.order_item_id::text, variant.variant_key, variant.brief::text, variant.revision, variant.status,
  EXISTS (
    SELECT 1
    FROM activity_log recovery
    JOIN creative_order_item recovery_item ON recovery_item.id = variant.order_item_id
    JOIN creative_order recovery_order ON recovery_order.id = recovery_item.order_id
    WHERE recovery.workspace_id = recovery_order.workspace_id
      AND recovery.issue_id = recovery_order.issue_id
      AND recovery.action = 'creative_qc_recovery_queued'
      AND recovery.details->>'variant_id' = variant.id::text
      AND recovery.details->>'revision' = variant.revision::text
      AND recovery.created_at > COALESCE((
        SELECT max(repair.created_at)
        FROM activity_log repair
        WHERE repair.workspace_id = recovery_order.workspace_id
          AND repair.issue_id = recovery_order.issue_id
          AND repair.action = 'creative_prime_package_repair_queued'
          AND repair.details->>'variant_id' = variant.id::text
          AND repair.details->>'revision' = variant.revision::text
      ), '-infinity'::timestamptz)
  ) AS qc_recovery_used,
  (
    variant.status = 'action_required'
    AND recovery_order.issue_id IS NOT NULL
    AND NOT EXISTS (
      SELECT 1 FROM activity_log recovery
      WHERE recovery.workspace_id = recovery_order.workspace_id
        AND recovery.issue_id = recovery_order.issue_id
        AND recovery.action = 'creative_qc_recovery_queued'
        AND recovery.details->>'variant_id' = variant.id::text
        AND recovery.details->>'revision' = variant.revision::text
        AND recovery.created_at > COALESCE((
          SELECT max(repair.created_at)
          FROM activity_log repair
          WHERE repair.workspace_id = recovery_order.workspace_id
            AND repair.issue_id = recovery_order.issue_id
            AND repair.action = 'creative_prime_package_repair_queued'
            AND repair.details->>'variant_id' = variant.id::text
            AND repair.details->>'revision' = variant.revision::text
        ), '-infinity'::timestamptz)
    )
    AND (
      SELECT count(DISTINCT asset.size_key)
      FROM creative_order_asset asset
      WHERE asset.variant_id = variant.id
        AND asset.revision = variant.revision
        AND asset.stage = 'primed'
        AND asset.status = 'completed'
        AND asset.size_key IN ('1080x1080', '1200x628', '800x1000')
    ) = 3
    AND NOT EXISTS (
      SELECT 1 FROM agent_task_queue task
      WHERE task.context->>'creative_order_id' = recovery_order.id::text
        AND task.context->>'variant_id' = variant.id::text
        AND COALESCE(NULLIF(task.context->>'revision', '')::int, 1) = variant.revision
        AND task.context->>'workflow' IN ('creative_qc_technical', 'creative_qc_visual')
        AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
    )
    AND EXISTS (
      SELECT 1
      FROM agent_task_queue task
      WHERE (
          (
            task.context->>'type' = 'creative_domain_task'
            AND task.context->>'workflow' IN ('creative_qc_technical', 'creative_qc_visual')
            AND task.status = 'failed'
            AND COALESCE(NULLIF(task.failure_reason, ''), 'agent_error') = ANY(ARRAY[
              'agent_error', 'api_invalid_request', 'agent_fallback_message', 'codex_semantic_inactivity',
              'provider_rate_limited', 'queued_expired', 'runtime_offline', 'runtime_recovery', 'timeout'
            ])
          )
          OR (
            task.status = 'completed'
            AND EXISTS (
              SELECT 1 FROM creative_order_qc_report report
              WHERE report.variant_id = variant.id
                AND report.revision = variant.revision
                AND report.lane = CASE
                  WHEN task.context->>'workflow' = 'creative_qc_technical' THEN 'technical'
                  WHEN task.context->>'workflow' = 'creative_qc_visual' THEN 'visual'
                  WHEN task.context->>'workflow' = 'creative_qc' THEN task.context->>'lane'
                  ELSE ''
                END
                AND report.status = 'failed'
                AND EXISTS (
                  SELECT 1
                  FROM jsonb_array_elements(
                    CASE WHEN jsonb_typeof(report.findings->'blocking_failures') = 'array'
                      THEN report.findings->'blocking_failures' ELSE '[]'::jsonb END
                  ) finding
                  WHERE finding->>'code' LIKE 'delegation_contract_%'
                    OR finding->>'code' LIKE 'manifest_%'
                    OR finding->>'code' LIKE 'prime_layout_contract_%'
                    OR finding->>'code' LIKE 'qc_batch_%'
                    OR finding->>'code' LIKE 'attachment_download_%'
                )
            )
          )
        )
        AND task.context->>'creative_order_id' = recovery_order.id::text
        AND task.context->>'variant_id' = variant.id::text
        AND COALESCE(NULLIF(task.context->>'revision', '')::int, 1) = variant.revision
    )
  ) AS qc_recovery_available,
  EXISTS (
    SELECT 1 FROM activity_log repair
    WHERE repair.workspace_id = recovery_order.workspace_id
      AND repair.issue_id = recovery_order.issue_id
      AND repair.action = 'creative_prime_package_repair_queued'
      AND repair.details->>'variant_id' = variant.id::text
      AND repair.details->>'revision' = variant.revision::text
  ) AS prime_repair_used,
  (
    recovery_order.issue_id IS NOT NULL
    AND (
      variant.status = 'action_required'
      OR (
        SELECT count(DISTINCT asset.size_key)
        FROM creative_order_asset asset
        WHERE asset.variant_id = variant.id
          AND asset.revision = variant.revision
          AND asset.stage = 'primed'
          AND asset.status = 'completed'
          AND asset.size_key IN ('1080x1080', '1200x628', '800x1000')
      ) < 3
    )
    AND NOT EXISTS (
      SELECT 1 FROM activity_log repair
      WHERE repair.workspace_id = recovery_order.workspace_id
        AND repair.issue_id = recovery_order.issue_id
        AND repair.action = 'creative_prime_package_repair_queued'
        AND repair.details->>'variant_id' = variant.id::text
        AND repair.details->>'revision' = variant.revision::text
    )
    AND (
      SELECT count(DISTINCT asset.size_key)
      FROM creative_order_asset asset
      WHERE asset.variant_id = variant.id
        AND asset.revision = variant.revision
        AND asset.stage = 'generated'
        AND asset.status = 'completed'
        AND asset.size_key IN ('1080x1080', '1200x628', '800x1000')
    ) = 3
    AND NOT EXISTS (
      SELECT 1 FROM agent_task_queue task
      WHERE task.context->>'type' = 'creative_domain_task'
        AND task.context->>'creative_order_id' = recovery_order.id::text
        AND task.context->>'variant_id' = variant.id::text
        AND task.status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
    )
    AND EXISTS (
      SELECT 1 FROM agent_task_queue task
      JOIN agent assigned_agent ON assigned_agent.id = task.agent_id
      WHERE assigned_agent.workspace_id = recovery_order.workspace_id
        AND assigned_agent.archived_at IS NULL
        AND task.context->>'type' = 'creative_domain_task'
        AND task.context->>'workflow' = 'creative_prime'
        AND task.context->>'creative_order_id' = recovery_order.id::text
        AND task.context->>'variant_id' = variant.id::text
        AND COALESCE(NULLIF(task.context->>'revision', '')::int, 1) = variant.revision
        AND task.runtime_id IS NOT NULL
    )
  ) AS prime_repair_available,
  variant.created_at::text, variant.updated_at::text
FROM creative_order_variant variant
JOIN creative_order_item recovery_item ON recovery_item.id = variant.order_item_id
JOIN creative_order recovery_order ON recovery_order.id = recovery_item.order_id
WHERE variant.order_item_id = $1 ORDER BY variant.variant_key
`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	variants := []creativeOrderVariantResponse{}
	for rows.Next() {
		variant, err := scanCreativeOrderVariant(rows)
		if err != nil {
			return nil, err
		}
		variant.QCStatus, err = h.derivedCreativeVariantQCStatus(r, parseUUID(variant.ID))
		if err != nil {
			return nil, err
		}
		variant.Assets, err = h.listCreativeOrderAssets(r, parseUUID(variant.ID))
		if err != nil {
			return nil, err
		}
		variant.QCReports, err = h.listCreativeOrderQCReports(r, parseUUID(variant.ID))
		if err != nil {
			return nil, err
		}
		if variant.Status != "completed" && variant.Status != "cancelled" {
			variant.DiagnosticAssets, err = h.listCreativeOrderVariantDiagnosticAssets(r, parseUUID(variant.ID))
			if err != nil {
				return nil, err
			}
			variant.ActionRequired, err = h.loadCreativeOrderVariantBlocker(r, parseUUID(variant.ID))
			if err != nil {
				return nil, err
			}
		}
		variants = append(variants, variant)
	}
	return variants, rows.Err()
}

func (h *Handler) loadCreativeOrderVariantBlocker(r *http.Request, variantID pgtype.UUID) (*creativeOrderVariantBlocker, error) {
	var blocker creativeOrderVariantBlocker
	var rawDetail string
	err := h.DB.QueryRow(r.Context(), `
WITH latest AS (
  SELECT q.id,
    COALESCE(q.context->>'workflow', '') AS workflow,
    CASE
      WHEN q.status = 'completed' THEN 'agent_reported_action_required'
      ELSE COALESCE(NULLIF(q.failure_reason, ''), 'agent_error')
    END AS failure_reason,
    COALESCE(q.error, '') AS error,
    COALESCE(q.result->>'output', '') AS result_output,
    COALESCE(q.completed_at, q.created_at)::text AS failed_at,
    (
      q.attempt < q.max_attempts
      OR (
        q.trigger_evidence_kind = 'creative_order_item_production'
        AND q.context->>'workflow' = 'creative_production'
        AND q.attempt < 3
      )
    ) AS retryable
  FROM agent_task_queue q
  JOIN creative_order_variant variant ON variant.id = $1
  WHERE q.context->>'type' = 'creative_domain_task'
    AND q.context->>'variant_id' = variant.id::text
    AND COALESCE(NULLIF(q.context->>'revision', '')::int, variant.revision) = variant.revision
    AND q.status IN ('failed', 'completed')
  ORDER BY COALESCE(q.completed_at, q.created_at) DESC, q.id DESC
  LIMIT 1
), latest_message AS (
  SELECT message.content
  FROM task_message message
  JOIN latest ON latest.id = message.task_id
  WHERE message.type = 'text'
    AND btrim(message.content) <> ''
  ORDER BY message.seq DESC, message.id DESC
  LIMIT 1
)
SELECT latest.id::text,
  latest.workflow,
  latest.failure_reason,
  COALESCE(NULLIF(latest_message.content, ''), NULLIF(latest.error, ''), NULLIF(latest.result_output, ''), ''),
  latest.failed_at,
  latest.retryable
FROM latest
LEFT JOIN latest_message ON true
`, variantID).Scan(&blocker.TaskID, &blocker.Workflow, &blocker.FailureReason, &rawDetail, &blocker.FailedAt, &blocker.Retryable)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	blocker.Detail = summarizeCreativeOrderVariantBlocker(rawDetail)
	if blocker.Detail == "" {
		blocker.Detail = "该变体已停止自动流程，但任务未返回可读原因。"
	}
	return &blocker, nil
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

func (h *Handler) DownloadCreativeOrderVariantDiagnosticAsset(w http.ResponseWriter, r *http.Request) {
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	variantID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "variantId"), "variant_id")
	if !ok {
		return
	}
	diagnosticID := strings.TrimSpace(chi.URLParam(r, "diagnosticId"))
	if diagnosticID == "" {
		writeError(w, http.StatusBadRequest, "diagnostic_id is required")
		return
	}
	workspaceID, found, err := h.loadCreativeOrderVariantWorkspaceID(r, orderID, variantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative diagnostic asset")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "creative diagnostic asset not found")
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, uuidToString(workspaceID), "creative diagnostic asset not found"); !ok {
		return
	}
	files, err := h.loadCreativeOrderVariantDiagnosticAssetFilesForOrder(r, orderID, variantID, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load creative diagnostic assets")
		return
	}
	var selected *creativeOrderDiagnosticAssetFile
	for index := range files {
		if files[index].ID == diagnosticID {
			selected = &files[index]
			break
		}
	}
	if selected == nil {
		writeError(w, http.StatusNotFound, "creative diagnostic asset not found")
		return
	}
	file, err := os.Open(selected.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "creative diagnostic asset not found")
		return
	}
	defer file.Close()
	if contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(selected.Filename))); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": selected.Filename}))
	http.ServeContent(w, r, selected.Filename, selected.ModTime, file)
}

func (h *Handler) loadCreativeOrderVariantWorkspaceID(r *http.Request, orderID, variantID pgtype.UUID) (pgtype.UUID, bool, error) {
	var workspaceID pgtype.UUID
	err := h.DB.QueryRow(r.Context(), `
SELECT order_row.workspace_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE order_row.id = $1 AND variant.id = $2
`, orderID, variantID).Scan(&workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, false, nil
	}
	return workspaceID, err == nil, err
}

func (h *Handler) listCreativeOrderVariantDiagnosticAssets(r *http.Request, variantID pgtype.UUID) ([]creativeOrderDiagnosticAsset, error) {
	files, err := h.loadCreativeOrderVariantDiagnosticAssetFiles(r, variantID)
	if err != nil {
		return nil, err
	}
	assets := make([]creativeOrderDiagnosticAsset, 0, len(files))
	for _, file := range files {
		file.Path = ""
		file.ModTime = time.Time{}
		assets = append(assets, file.creativeOrderDiagnosticAsset)
	}
	return assets, nil
}

func (h *Handler) loadCreativeOrderVariantDiagnosticAssetFiles(r *http.Request, variantID pgtype.UUID) ([]creativeOrderDiagnosticAssetFile, error) {
	task, ok, err := h.loadCreativeOrderVariantDiagnosticTask(r, variantID)
	if !ok || err != nil {
		return nil, err
	}
	return creativeOrderDiagnosticAssetFiles(task), nil
}

func (h *Handler) loadCreativeOrderVariantDiagnosticAssetFilesForOrder(r *http.Request, orderID, variantID, workspaceID pgtype.UUID) ([]creativeOrderDiagnosticAssetFile, error) {
	task, ok, err := h.loadCreativeOrderVariantDiagnosticTaskForOrder(r, orderID, variantID, workspaceID)
	if !ok || err != nil {
		return nil, err
	}
	return creativeOrderDiagnosticAssetFiles(task), nil
}

func (h *Handler) loadCreativeOrderVariantDiagnosticTask(r *http.Request, variantID pgtype.UUID) (creativeOrderDiagnosticTask, bool, error) {
	var task creativeOrderDiagnosticTask
	err := h.DB.QueryRow(r.Context(), `
SELECT order_row.id::text, variant.id::text, variant.revision,
  latest.id::text, latest.workflow, latest.work_dir, latest.created_at
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
JOIN LATERAL (
  SELECT q.id,
    COALESCE(q.context->>'workflow', '') AS workflow,
    COALESCE(q.work_dir, '') AS work_dir,
    COALESCE(q.completed_at, q.created_at)::text AS created_at
  FROM agent_task_queue q
  WHERE q.context->>'type' = 'creative_domain_task'
    AND q.context->>'variant_id' = variant.id::text
    AND COALESCE(NULLIF(q.context->>'revision', '')::int, variant.revision) = variant.revision
    AND q.status IN ('failed', 'completed')
    AND COALESCE(q.context->>'workflow', '') IN ('creative_production', 'creative_prime')
    AND COALESCE(q.work_dir, '') <> ''
  ORDER BY COALESCE(q.completed_at, q.created_at) DESC, q.id DESC
  LIMIT 1
) latest ON true
WHERE variant.id = $1
`, variantID).Scan(&task.OrderID, &task.VariantID, &task.Revision, &task.TaskID, &task.Workflow, &task.WorkDir, &task.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return creativeOrderDiagnosticTask{}, false, nil
	}
	return task, err == nil, err
}

func (h *Handler) loadCreativeOrderVariantDiagnosticTaskForOrder(r *http.Request, orderID, variantID, workspaceID pgtype.UUID) (creativeOrderDiagnosticTask, bool, error) {
	var task creativeOrderDiagnosticTask
	err := h.DB.QueryRow(r.Context(), `
SELECT order_row.id::text, variant.id::text, variant.revision,
  latest.id::text, latest.workflow, latest.work_dir, latest.created_at
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
JOIN LATERAL (
  SELECT q.id,
    COALESCE(q.context->>'workflow', '') AS workflow,
    COALESCE(q.work_dir, '') AS work_dir,
    COALESCE(q.completed_at, q.created_at)::text AS created_at
  FROM agent_task_queue q
  WHERE q.context->>'type' = 'creative_domain_task'
    AND q.context->>'variant_id' = variant.id::text
    AND COALESCE(NULLIF(q.context->>'revision', '')::int, variant.revision) = variant.revision
    AND q.status IN ('failed', 'completed')
    AND COALESCE(q.context->>'workflow', '') IN ('creative_production', 'creative_prime')
    AND COALESCE(q.work_dir, '') <> ''
  ORDER BY COALESCE(q.completed_at, q.created_at) DESC, q.id DESC
  LIMIT 1
) latest ON true
WHERE order_row.id = $1 AND variant.id = $2 AND order_row.workspace_id = $3
`, orderID, variantID, workspaceID).Scan(&task.OrderID, &task.VariantID, &task.Revision, &task.TaskID, &task.Workflow, &task.WorkDir, &task.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return creativeOrderDiagnosticTask{}, false, nil
	}
	return task, err == nil, err
}

func creativeOrderDiagnosticAssetFiles(task creativeOrderDiagnosticTask) []creativeOrderDiagnosticAssetFile {
	root := filepath.Clean(strings.TrimSpace(task.WorkDir))
	if root == "." || root == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	files := []creativeOrderDiagnosticAssetFile{}
	const maxDiagnosticScanFiles = 300
	seenFiles := 0
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") || strings.Count(rel, string(filepath.Separator)) >= 2 {
				return filepath.SkipDir
			}
			return nil
		}
		seenFiles++
		if seenFiles > maxDiagnosticScanFiles {
			return filepath.SkipAll
		}
		sizeKey, label, keep := classifyCreativeOrderDiagnosticFile(rel, task.Workflow)
		if !keep {
			return nil
		}
		fileInfo, err := entry.Info()
		if err != nil {
			return nil
		}
		id := creativeOrderDiagnosticAssetID(task.TaskID, rel)
		files = append(files, creativeOrderDiagnosticAssetFile{
			creativeOrderDiagnosticAsset: creativeOrderDiagnosticAsset{
				ID:        id,
				VariantID: task.VariantID,
				TaskID:    task.TaskID,
				SizeKey:   sizeKey,
				Revision:  task.Revision,
				Label:     label,
				Filename:  filepath.Base(path),
				URL:       fmt.Sprintf("/api/creative/orders/%s/variants/%s/diagnostic-assets/%s", task.OrderID, task.VariantID, id),
				CreatedAt: task.CreatedAt,
			},
			Path:    path,
			ModTime: fileInfo.ModTime(),
		})
		return nil
	})
	sort.SliceStable(files, func(i, j int) bool {
		leftSize, rightSize := creativeAssetSizeOrder(files[i].SizeKey), creativeAssetSizeOrder(files[j].SizeKey)
		if leftSize != rightSize {
			return leftSize < rightSize
		}
		return files[i].ModTime.After(files[j].ModTime)
	})
	if len(files) > 9 {
		return files[:9]
	}
	return files
}

func classifyCreativeOrderDiagnosticFile(relPath string, workflow string) (string, string, bool) {
	normalized := filepath.ToSlash(strings.ToLower(relPath))
	extension := strings.ToLower(filepath.Ext(normalized))
	if extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".webp" {
		return "", "", false
	}
	name := filepath.Base(normalized)
	if strings.Contains(name, "reference") || strings.Contains(name, "source") || strings.Contains(name, "contact") || strings.Contains(name, "hard-zone") {
		return "", "", false
	}
	if !strings.Contains(name, "model") && !strings.Contains(name, "delivery") && !strings.Contains(name, "normal") && !strings.Contains(name, "prime") && !strings.Contains(name, "compose") {
		return "", "", false
	}
	sizeKey := ""
	switch {
	case strings.Contains(name, "1080") || strings.Contains(name, "square"):
		sizeKey = "1080x1080"
	case strings.Contains(name, "1200") || strings.Contains(name, "landscape"):
		sizeKey = "1200x628"
	case strings.Contains(name, "800") || strings.Contains(name, "portrait"):
		sizeKey = "800x1000"
	default:
		return "", "", false
	}
	stage := "模型输出"
	if strings.Contains(name, "rework") || strings.Contains(name, "retry") || strings.Contains(name, "repair") {
		stage = "返工输出"
	}
	if strings.Contains(name, "delivery") || strings.Contains(name, "normal") {
		stage = "规范化输出"
	}
	if workflow == "creative_prime" {
		stage = "Prime 合成输出"
	}
	return sizeKey, stage, true
}

func creativeOrderDiagnosticAssetID(taskID, relPath string) string {
	sum := sha256.Sum256([]byte(taskID + "\n" + filepath.ToSlash(relPath)))
	return fmt.Sprintf("%x", sum[:12])
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
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
FROM creative_order_asset WHERE variant_id = $1 ORDER BY revision, size_key, stage
`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := []creativeOrderAssetResponse{}
	for rows.Next() {
		asset, err := scanCreativeOrderAsset(rows)
		if err != nil {
			return nil, err
		}
		assets = append(assets, asset)
	}
	return assets, rows.Err()
}

func (h *Handler) derivedCreativeVariantQCStatus(r *http.Request, variantID pgtype.UUID) (string, error) {
	var status string
	err := h.DB.QueryRow(r.Context(), `
SELECT CASE
  WHEN count(*) FILTER (WHERE q.status = 'failed') > 0 THEN 'failed'
  WHEN count(*) FILTER (WHERE q.status = 'warning') > 0 THEN 'warning'
  WHEN count(*) FILTER (WHERE q.status = 'passed') = 2 THEN 'passed'
  ELSE 'pending'
END
FROM creative_order_variant v
LEFT JOIN creative_order_qc_report q ON q.variant_id = v.id AND q.revision = v.revision
WHERE v.id = $1
GROUP BY v.id
`, variantID).Scan(&status)
	return status, err
}

func (h *Handler) listCreativeOrderQCReports(r *http.Request, variantID pgtype.UUID) ([]creativeOrderQCReportResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, variant_id::text, lane, revision, status, findings::text, trigger_evidence_kind,
  COALESCE(trigger_evidence_ref_id::text, ''), created_at::text, updated_at::text
FROM creative_order_qc_report WHERE variant_id = $1 ORDER BY revision, lane
`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := []creativeOrderQCReportResponse{}
	for rows.Next() {
		var report creativeOrderQCReportResponse
		var findings string
		if err := rows.Scan(&report.ID, &report.VariantID, &report.Lane, &report.Revision, &report.Status, &findings,
			&report.TriggerEvidenceKind, &report.TriggerEvidenceReference, &report.CreatedAt, &report.UpdatedAt); err != nil {
			return nil, err
		}
		report.Findings = json.RawMessage(findings)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}
