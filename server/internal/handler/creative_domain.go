package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

type creativeOrderItemResponse struct {
	ID               string                         `json:"id"`
	OrderID          string                         `json:"order_id"`
	CandidateID      string                         `json:"candidate_id"`
	SourceAnalysisID string                         `json:"source_analysis_id"`
	CopySnapshot     json.RawMessage                `json:"copy_snapshot"`
	Direction        string                         `json:"direction"`
	Status           string                         `json:"status"`
	CreatedAt        string                         `json:"created_at"`
	UpdatedAt        string                         `json:"updated_at"`
	Variants         []creativeOrderVariantResponse `json:"variants,omitempty"`
}

type creativeOrderVariantResponse struct {
	ID          string                          `json:"id"`
	OrderItemID string                          `json:"order_item_id"`
	VariantKey  string                          `json:"variant_key"`
	Brief       json.RawMessage                 `json:"brief"`
	Revision    int                             `json:"revision"`
	Status      string                          `json:"status"`
	QCStatus    string                          `json:"qc_status"`
	Assets      []creativeOrderAssetResponse    `json:"assets,omitempty"`
	QCReports   []creativeOrderQCReportResponse `json:"qc_reports,omitempty"`
	CreatedAt   string                          `json:"created_at"`
	UpdatedAt   string                          `json:"updated_at"`
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

func (h *Handler) UpsertCreativeOrderVariant(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
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
	var triggerKind string
	if err := h.DB.QueryRow(r.Context(), `
SELECT EXISTS(SELECT 1 FROM creative_order_item i JOIN creative_order o ON o.id = i.order_id WHERE i.id = $1 AND i.order_id = $2 AND o.workspace_id = $3),
  COALESCE((SELECT v.revision FROM creative_order_variant v WHERE v.order_item_id = $1 AND v.variant_key = $4), 0),
  COALESCE((SELECT o.trigger_evidence_kind FROM creative_order_item i JOIN creative_order o ON o.id = i.order_id WHERE i.id = $1), '')
`, itemID, orderID, workspaceID, input.VariantKey).Scan(&itemExists, &currentRevision, &triggerKind); err != nil || !itemExists {
		writeError(w, http.StatusUnprocessableEntity, "order item does not belong to this creative order")
		return
	}
	maxRevision := 2
	if triggerKind == "creative_direct_edit" {
		maxRevision = 3
	}
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
RETURNING id::text, order_item_id::text, variant_key, brief::text, revision, status, created_at::text, updated_at::text
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

func (h *Handler) UpsertCreativeOrderAsset(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	var input creativeOrderAssetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative order asset")
		return
	}
	input, err := normalizeCreativeOrderAsset(input)
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
ON CONFLICT (variant_id, size_key, revision, stage) DO UPDATE SET asset_family_id = EXCLUDED.asset_family_id,
  attachment_id = EXCLUDED.attachment_id, derived_from_asset_id = EXCLUDED.derived_from_asset_id,
  metadata = EXCLUDED.metadata, evidence = EXCLUDED.evidence, status = EXCLUDED.status, updated_at = now()
RETURNING id::text, variant_id::text, asset_family_id::text, size_key, revision, stage, COALESCE(attachment_id::text, ''),
  COALESCE(derived_from_asset_id::text, ''), metadata::text, evidence::text, status, created_at::text, updated_at::text
`, variantID, assetFamilyID, input.SizeKey, input.Revision, input.Stage, attachmentID, derivedFromAssetID, input.Metadata, input.Evidence, input.Status))
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
	task, ok := h.creativeQCFinalizingTask(w, r, variantID)
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

func (h *Handler) creativeQCFinalizingTask(w http.ResponseWriter, r *http.Request, variantID pgtype.UUID) (db.AgentTaskQueue, bool) {
	taskID, ok := parseUUIDOrBadRequest(w, r.Header.Get("X-Task-ID"), "task_id")
	if !ok {
		return db.AgentTaskQueue{}, false
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || uuidToString(task.AgentID) != r.Header.Get("X-Agent-ID") ||
		!task.TriggerEvidenceKind.Valid || task.TriggerEvidenceKind.String != "creative_order_variant_qc" ||
		!task.TriggerEvidenceRefID.Valid || task.TriggerEvidenceRefID != variantID {
		writeError(w, http.StatusForbidden, "task is not authorized to finalize this creative QC")
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
		if item.CandidateID == "" || len(item.Direction) > 4000 {
			return input, errors.New("invalid creative order item")
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

func normalizeCreativeOrderAsset(input creativeOrderAssetInput) (creativeOrderAssetInput, error) {
	input.VariantID = strings.TrimSpace(input.VariantID)
	input.AssetFamilyID = strings.TrimSpace(input.AssetFamilyID)
	input.SizeKey = strings.TrimSpace(input.SizeKey)
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
	return input, nil
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
	err := row.Scan(&variant.ID, &variant.OrderItemID, &variant.VariantKey, &brief, &variant.Revision, &variant.Status, &variant.CreatedAt, &variant.UpdatedAt)
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
    AND q.context->>'creative_order_id' = o.id::text
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
  COALESCE(NULLIF(failure_reason, ''), 'agent_error'),
  COALESCE(error, ''),
  COALESCE(completed_at, created_at)::text,
  attempt < max_attempts
FROM ranked
WHERE row_number = 1 AND status = 'failed'
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
  SELECT count(*) AS total,
    count(*) FILTER (WHERE v.status = 'failed') AS failed,
    count(*) FILTER (WHERE v.status = 'action_required') AS action_required,
    count(*) FILTER (WHERE v.status = 'running') AS running,
    count(*) FILTER (WHERE v.status = 'partial') AS partial,
    count(*) FILTER (WHERE v.status = 'completed') AS completed,
    count(*) FILTER (WHERE v.status = 'cancelled') AS cancelled
  FROM creative_order_item i LEFT JOIN creative_order_variant v ON v.order_item_id = i.id
  WHERE i.order_id = $1
), base AS (
  SELECT aggregate.*,
    CASE
      WHEN total = 0 THEN 'draft'
      WHEN action_required = total THEN 'action_required'
      WHEN failed = total THEN 'failed'
      WHEN action_required > 0 OR failed > 0 THEN 'partial'
      WHEN completed = total THEN 'completed'
      WHEN cancelled = total THEN 'cancelled'
      WHEN partial > 0 OR (completed > 0 AND completed < total) THEN 'partial'
      WHEN running > 0 THEN 'running'
      ELSE 'queued'
    END AS status
  FROM aggregate
)
SELECT CASE
	WHEN NOT $2::boolean THEN status
	WHEN status = 'completed' THEN status
	WHEN completed > 0 OR running > 0 OR partial > 0 THEN 'partial'
	WHEN status IN ('draft', 'queued', 'running') THEN 'action_required'
	ELSE status
END FROM base
`, orderID, hasOpenFailures).Scan(&status)
	return status, err
}

func (h *Handler) listCreativeOrderItems(r *http.Request, orderID pgtype.UUID) ([]creativeOrderItemResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, order_id::text, candidate_id::text, COALESCE(source_analysis_id::text, ''), copy_snapshot::text,
  direction, status, created_at::text, updated_at::text
FROM creative_order_item WHERE order_id = $1 ORDER BY created_at
`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []creativeOrderItemResponse{}
	for rows.Next() {
		var item creativeOrderItemResponse
		var snapshot string
		if err := rows.Scan(&item.ID, &item.OrderID, &item.CandidateID, &item.SourceAnalysisID, &snapshot, &item.Direction, &item.Status, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.CopySnapshot = json.RawMessage(snapshot)
		item.Variants, err = h.listCreativeOrderVariants(r, parseUUID(item.ID))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (h *Handler) listCreativeOrderVariants(r *http.Request, itemID pgtype.UUID) ([]creativeOrderVariantResponse, error) {
	rows, err := h.DB.Query(r.Context(), `
SELECT id::text, order_item_id::text, variant_key, brief::text, revision, status, created_at::text, updated_at::text
FROM creative_order_variant WHERE order_item_id = $1 ORDER BY variant_key
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
		variants = append(variants, variant)
	}
	return variants, rows.Err()
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
WITH latest_revision AS (
  SELECT COALESCE(max(revision), 0) AS revision
  FROM creative_order_qc_report
  WHERE variant_id = $1
)
SELECT CASE
  WHEN count(*) FILTER (WHERE q.status = 'failed') > 0 THEN 'failed'
  WHEN count(*) FILTER (WHERE q.status = 'warning') > 0 THEN 'warning'
  WHEN count(*) FILTER (WHERE q.status = 'passed') = 2 THEN 'passed'
  ELSE 'pending'
END
FROM creative_order_qc_report q
JOIN latest_revision latest ON q.revision = latest.revision
WHERE q.variant_id = $1
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
