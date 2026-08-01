package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var creativeDeliverySizes = map[string]struct{}{
	"1080x1080": {},
	"1200x628":  {},
	"800x1000":  {},
}

type creativeDeliveryResponse struct {
	ID                        string `json:"id"`
	IssueID                   string `json:"issue_id"`
	CandidateID               string `json:"candidate_id"`
	WorkIssueID               string `json:"work_issue_id"`
	Variant                   int16  `json:"variant"`
	Size                      string `json:"size"`
	Revision                  int    `json:"revision"`
	BaseAttachmentID          string `json:"base_attachment_id"`
	FinalAttachmentID         string `json:"final_attachment_id"`
	PrimeEvidenceAttachmentID string `json:"prime_evidence_attachment_id"`
	QCIssueID                 string `json:"qc_issue_id"`
	CreatedAt                 string `json:"created_at"`
	UpdatedAt                 string `json:"updated_at"`
}

type creativeDeliveryInput struct {
	CandidateID               string `json:"candidate_id"`
	WorkIssueID               string `json:"work_issue_id"`
	Variant                   int16  `json:"variant"`
	Size                      string `json:"size"`
	Revision                  int    `json:"revision"`
	BaseAttachmentID          string `json:"base_attachment_id"`
	FinalAttachmentID         string `json:"final_attachment_id"`
	PrimeEvidenceAttachmentID string `json:"prime_evidence_attachment_id"`
	QCIssueID                 string `json:"qc_issue_id"`
}

type creativeAdjustmentRequestResponse struct {
	ID                  string   `json:"id"`
	IssueID             string   `json:"issue_id"`
	CandidateID         string   `json:"candidate_id"`
	WorkIssueID         string   `json:"work_issue_id"`
	AdjustmentIssueID   string   `json:"adjustment_issue_id"`
	Variant             int16    `json:"variant"`
	Scope               string   `json:"scope"`
	Size                string   `json:"size"`
	Revision            int      `json:"revision"`
	Instruction         string   `json:"instruction"`
	TargetAttachmentIDs []string `json:"target_attachment_ids"`
	BaseAttachmentIDs   []string `json:"base_attachment_ids"`
	Status              string   `json:"status"`
	CreatedAt           string   `json:"created_at"`
	UpdatedAt           string   `json:"updated_at"`
}

type creativeAdjustmentInput struct {
	Variant             int16    `json:"variant"`
	Scope               string   `json:"scope"`
	Size                string   `json:"size"`
	Instruction         string   `json:"instruction"`
	TargetAttachmentIDs []string `json:"target_attachment_ids"`
	BaseAttachmentIDs   []string `json:"base_attachment_ids"`
}

func validateCreativeDeliveryInput(input creativeDeliveryInput) error {
	if strings.TrimSpace(input.CandidateID) == "" || strings.TrimSpace(input.WorkIssueID) == "" {
		return errors.New("candidate_id and work_issue_id are required")
	}
	if input.Variant < 1 || input.Variant > 3 {
		return errors.New("variant must be between 1 and 3")
	}
	if _, ok := creativeDeliverySizes[input.Size]; !ok {
		return errors.New("size must be 1080x1080, 1200x628, or 800x1000")
	}
	if input.Revision < 1 {
		return errors.New("revision must be greater than zero")
	}
	if strings.TrimSpace(input.FinalAttachmentID) == "" {
		return errors.New("final_attachment_id is required")
	}
	return nil
}

func validateCreativeAdjustmentInput(input creativeAdjustmentInput) error {
	input.Instruction = strings.TrimSpace(input.Instruction)
	if input.Variant < 1 || input.Variant > 3 {
		return errors.New("variant must be between 1 and 3")
	}
	if input.Scope != "size" && input.Scope != "variant" {
		return errors.New("scope must be size or variant")
	}
	if input.Scope == "size" {
		if _, ok := creativeDeliverySizes[input.Size]; !ok {
			return errors.New("size is required for a size adjustment")
		}
		if len(input.TargetAttachmentIDs) != 1 {
			return errors.New("a size adjustment must target exactly one attachment")
		}
	} else {
		if strings.TrimSpace(input.Size) != "" {
			return errors.New("size must be empty for a variant adjustment")
		}
		if len(input.TargetAttachmentIDs) != 3 {
			return errors.New("a variant adjustment must target all three attachments")
		}
	}
	if input.Instruction == "" || len(input.Instruction) > 4000 {
		return errors.New("instruction must be between 1 and 4000 characters")
	}
	return nil
}

func (h *Handler) RegisterCreativeDeliveries(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		Deliveries []creativeDeliveryInput `json:"deliveries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Deliveries) == 0 || len(req.Deliveries) > 9 {
		writeError(w, http.StatusBadRequest, "deliveries must contain between 1 and 9 items")
		return
	}
	for _, input := range req.Deliveries {
		if err := validateCreativeDeliveryInput(input); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	createdBy := h.requestingUserIDFromRequest(r, actorType, actorID)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register creative deliveries")
		return
	}
	defer tx.Rollback(r.Context())

	registered := make([]creativeDeliveryResponse, 0, len(req.Deliveries))
	for _, input := range req.Deliveries {
		candidateID, valid := parseUUIDOrBadRequest(w, input.CandidateID, "candidate_id")
		if !valid {
			return
		}
		workIssueID, valid := parseUUIDOrBadRequest(w, input.WorkIssueID, "work_issue_id")
		if !valid {
			return
		}
		finalAttachmentID, valid := parseUUIDOrBadRequest(w, input.FinalAttachmentID, "final_attachment_id")
		if !valid {
			return
		}
		baseAttachmentID, valid := optionalUUIDOrBadRequest(w, input.BaseAttachmentID, "base_attachment_id")
		if !valid {
			return
		}
		primeEvidenceAttachmentID, valid := optionalUUIDOrBadRequest(w, input.PrimeEvidenceAttachmentID, "prime_evidence_attachment_id")
		if !valid {
			return
		}
		qcIssueID, valid := optionalUUIDOrBadRequest(w, input.QCIssueID, "qc_issue_id")
		if !valid {
			return
		}

		var referencesValid bool
		err = tx.QueryRow(r.Context(), `
SELECT
  EXISTS(SELECT 1 FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3)
  AND EXISTS(SELECT 1 FROM issue WHERE id = $4 AND workspace_id = $3 AND parent_issue_id = $1)
  AND EXISTS(SELECT 1 FROM attachment WHERE id = $5 AND workspace_id = $3)
  AND ($6::uuid IS NULL OR EXISTS(SELECT 1 FROM attachment WHERE id = $6 AND workspace_id = $3))
  AND ($7::uuid IS NULL OR EXISTS(SELECT 1 FROM attachment WHERE id = $7 AND workspace_id = $3))
  AND ($8::uuid IS NULL OR EXISTS(SELECT 1 FROM issue WHERE id = $8 AND workspace_id = $3))
`, issue.ID, candidateID, issue.WorkspaceID, workIssueID, finalAttachmentID,
			baseAttachmentID, primeEvidenceAttachmentID, qcIssueID).Scan(&referencesValid)
		if err != nil || !referencesValid {
			writeError(w, http.StatusUnprocessableEntity, "delivery references do not belong to this creative issue")
			return
		}

		delivery, err := scanCreativeDelivery(tx.QueryRow(r.Context(), `
INSERT INTO creative_delivery (
  workspace_id, issue_id, candidate_id, work_issue_id, variant, size, revision,
  base_attachment_id, final_attachment_id, prime_evidence_attachment_id, qc_issue_id, created_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
ON CONFLICT (issue_id, candidate_id, variant, size, revision) DO UPDATE SET
  work_issue_id = EXCLUDED.work_issue_id,
  base_attachment_id = EXCLUDED.base_attachment_id,
  final_attachment_id = EXCLUDED.final_attachment_id,
  prime_evidence_attachment_id = EXCLUDED.prime_evidence_attachment_id,
  qc_issue_id = EXCLUDED.qc_issue_id,
  updated_at = now()
RETURNING id::text, issue_id::text, candidate_id::text, work_issue_id::text,
          variant, size, revision, COALESCE(base_attachment_id::text, ''),
          final_attachment_id::text, COALESCE(prime_evidence_attachment_id::text, ''),
          COALESCE(qc_issue_id::text, ''), created_at::text, updated_at::text
`, issue.WorkspaceID, issue.ID, candidateID, workIssueID, input.Variant, input.Size,
			input.Revision, baseAttachmentID, finalAttachmentID, primeEvidenceAttachmentID, qcIssueID, createdBy))
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, "failed to register creative delivery")
			return
		}
		registered = append(registered, delivery)
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to register creative deliveries")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, map[string]any{"deliveries": registered})
}

func (h *Handler) CreateCreativeAdjustment(w http.ResponseWriter, r *http.Request) {
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
	var input creativeAdjustmentInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input.Instruction = strings.TrimSpace(input.Instruction)
	if err := validateCreativeAdjustmentInput(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	targetIDs, ok := parseUUIDSliceOrBadRequest(w, input.TargetAttachmentIDs, "target_attachment_ids")
	if !ok {
		return
	}
	baseIDs, ok := parseUUIDSliceOrBadRequest(w, input.BaseAttachmentIDs, "base_attachment_ids")
	if !ok {
		return
	}

	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	createdBy := h.requestingUserIDFromRequest(r, actorType, actorID)
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}
	defer tx.Rollback(r.Context())

	var workIssueID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT work_issue_id
FROM creative_issue_item
WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3 AND work_issue_id IS NOT NULL
FOR UPDATE
`, issue.ID, candidateID, issue.WorkspaceID).Scan(&workIssueID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnprocessableEntity, "creative work issue is not available")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}
	var targetsValid bool
	if err := tx.QueryRow(r.Context(), `
WITH selected AS (
  SELECT size, revision, base_attachment_id
  FROM creative_delivery
  WHERE issue_id = $1 AND candidate_id = $2 AND variant = $3
    AND final_attachment_id = ANY($4::uuid[])
)
SELECT
  (SELECT count(*) FROM selected) = cardinality($4::uuid[])
  AND CASE
    WHEN $5 = 'size' THEN
      (SELECT count(DISTINCT size) FROM selected) = 1
      AND (SELECT bool_and(size = $6) FROM selected)
    ELSE (SELECT count(DISTINCT size) FROM selected) = 3
  END
  AND NOT EXISTS (
    SELECT 1
    FROM selected current
    JOIN creative_delivery newer
      ON newer.issue_id = $1
      AND newer.candidate_id = $2
      AND newer.variant = $3
      AND newer.size = current.size
      AND newer.revision > current.revision
  )
  AND (
    cardinality($7::uuid[]) = 0
    OR (SELECT count(*) FROM selected WHERE base_attachment_id = ANY($7::uuid[])) = cardinality($7::uuid[])
  )
`, issue.ID, candidateID, input.Variant, targetIDs, input.Scope, input.Size, baseIDs).Scan(&targetsValid); err != nil || !targetsValid {
		writeError(w, http.StatusUnprocessableEntity, "adjustment attachments do not match the selected creative delivery")
		return
	}

	var revision int
	if err := tx.QueryRow(r.Context(), `
SELECT GREATEST(
  COALESCE((SELECT max(revision) FROM creative_delivery WHERE issue_id = $1 AND candidate_id = $2), 0),
  COALESCE((SELECT max(revision) FROM creative_adjustment_request WHERE issue_id = $1 AND candidate_id = $2), 0)
) + 1
`, issue.ID, candidateID).Scan(&revision); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}

	var size any
	if input.Scope == "size" {
		size = input.Size
	}
	adjustment, err := scanCreativeAdjustment(tx.QueryRow(r.Context(), `
INSERT INTO creative_adjustment_request (
  workspace_id, issue_id, candidate_id, work_issue_id, variant, scope, size, revision,
  instruction, target_attachment_ids, base_attachment_ids, created_by
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
RETURNING id::text, issue_id::text, candidate_id::text, work_issue_id::text,
          COALESCE(adjustment_issue_id::text, ''), variant, scope, COALESCE(size, ''), revision,
          instruction, target_attachment_ids, base_attachment_ids, status, created_at::text, updated_at::text
`, issue.WorkspaceID, issue.ID, candidateID, workIssueID, input.Variant, input.Scope,
		size, revision, input.Instruction, targetIDs, baseIDs, createdBy))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}
	_, err = tx.Exec(r.Context(), `
UPDATE creative_issue_item
SET revision = $4, status = 'running', updated_by = $5, updated_at = now()
WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3
`, issue.ID, candidateID, issue.WorkspaceID, revision, createdBy)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create adjustment request")
		return
	}
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusCreated, adjustment)
}

func (h *Handler) BindCreativeAdjustmentIssue(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	candidateID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "candidateId"), "candidate_id")
	if !ok {
		return
	}
	adjustmentID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "adjustmentId"), "adjustment_id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req struct {
		AdjustmentIssueID string `json:"adjustment_issue_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	adjustmentIssueID, ok := parseUUIDOrBadRequest(w, req.AdjustmentIssueID, "adjustment_issue_id")
	if !ok {
		return
	}
	adjustment, err := scanCreativeAdjustment(h.DB.QueryRow(r.Context(), `
UPDATE creative_adjustment_request ar
SET adjustment_issue_id = $5, status = 'running', updated_at = now()
WHERE ar.id = $1 AND ar.issue_id = $2 AND ar.candidate_id = $3 AND ar.workspace_id = $4
  AND EXISTS(
    SELECT 1 FROM issue child
    WHERE child.id = $5 AND child.workspace_id = $4 AND child.parent_issue_id = ar.work_issue_id
  )
RETURNING ar.id::text, ar.issue_id::text, ar.candidate_id::text, ar.work_issue_id::text,
          ar.adjustment_issue_id::text, ar.variant, ar.scope, COALESCE(ar.size, ''), ar.revision,
          ar.instruction, ar.target_attachment_ids, ar.base_attachment_ids, ar.status,
          ar.created_at::text, ar.updated_at::text
`, adjustmentID, issue.ID, candidateID, issue.WorkspaceID, adjustmentIssueID))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnprocessableEntity, "adjustment issue must be a direct child of the creative work issue")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to bind adjustment issue")
		return
	}
	actorType, actorID := h.resolveActor(r, userID, uuidToString(issue.WorkspaceID))
	h.publishCreativeMaterialsUpdated(issue.WorkspaceID, issue.ID, actorType, actorID)
	writeJSON(w, http.StatusOK, adjustment)
}

func listCreativeDeliveries(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, issueID, workspaceID pgtype.UUID) ([]creativeDeliveryResponse, error) {
	rows, err := queryer.Query(ctx, `
SELECT id::text, issue_id::text, candidate_id::text, work_issue_id::text,
       variant, size, revision, COALESCE(base_attachment_id::text, ''),
       final_attachment_id::text, COALESCE(prime_evidence_attachment_id::text, ''),
       COALESCE(qc_issue_id::text, ''), created_at::text, updated_at::text
FROM creative_delivery
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY candidate_id, variant, size, revision DESC
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]creativeDeliveryResponse, 0)
	for rows.Next() {
		item, err := scanCreativeDelivery(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func listCreativeAdjustments(ctx context.Context, queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, issueID, workspaceID pgtype.UUID) ([]creativeAdjustmentRequestResponse, error) {
	rows, err := queryer.Query(ctx, `
SELECT id::text, issue_id::text, candidate_id::text, work_issue_id::text,
       COALESCE(adjustment_issue_id::text, ''), variant, scope, COALESCE(size, ''), revision,
       instruction, target_attachment_ids, base_attachment_ids, status, created_at::text, updated_at::text
FROM creative_adjustment_request
WHERE issue_id = $1 AND workspace_id = $2
ORDER BY created_at DESC
`, issueID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]creativeAdjustmentRequestResponse, 0)
	for rows.Next() {
		item, err := scanCreativeAdjustment(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func scanCreativeDelivery(row rowScanner) (creativeDeliveryResponse, error) {
	var item creativeDeliveryResponse
	err := row.Scan(&item.ID, &item.IssueID, &item.CandidateID, &item.WorkIssueID,
		&item.Variant, &item.Size, &item.Revision, &item.BaseAttachmentID,
		&item.FinalAttachmentID, &item.PrimeEvidenceAttachmentID, &item.QCIssueID,
		&item.CreatedAt, &item.UpdatedAt)
	return item, err
}

func scanCreativeAdjustment(row rowScanner) (creativeAdjustmentRequestResponse, error) {
	var item creativeAdjustmentRequestResponse
	var targetIDs, baseIDs []pgtype.UUID
	err := row.Scan(&item.ID, &item.IssueID, &item.CandidateID, &item.WorkIssueID,
		&item.AdjustmentIssueID, &item.Variant, &item.Scope, &item.Size, &item.Revision,
		&item.Instruction, &targetIDs, &baseIDs, &item.Status, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return item, err
	}
	item.TargetAttachmentIDs = uuidStrings(targetIDs)
	item.BaseAttachmentIDs = uuidStrings(baseIDs)
	return item, nil
}

func optionalUUIDOrBadRequest(w http.ResponseWriter, raw, field string) (pgtype.UUID, bool) {
	if strings.TrimSpace(raw) == "" {
		return pgtype.UUID{}, true
	}
	return parseUUIDOrBadRequest(w, raw, field)
}

func uuidStrings(ids []pgtype.UUID) []string {
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			result = append(result, uuidToString(id))
		}
	}
	return result
}

func creativeAdjustmentTitle(input creativeAdjustmentInput, revision int) string {
	target := fmt.Sprintf("V%02d", input.Variant)
	if input.Scope == "size" {
		target += " / " + input.Size
	}
	return fmt.Sprintf("%s 精准调整 · R%d", target, revision)
}
