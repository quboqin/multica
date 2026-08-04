package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type creativeFeedbackEventInput struct {
	IdempotencyKey  string          `json:"idempotency_key"`
	IssueID         string          `json:"issue_id"`
	SubjectType     string          `json:"subject_type"`
	SubjectID       string          `json:"subject_id"`
	EventType       string          `json:"event_type"`
	Decision        string          `json:"decision"`
	ReasonCodes     []string        `json:"reason_codes"`
	Comment         string          `json:"comment"`
	Annotation      json.RawMessage `json:"annotation"`
	ContextSnapshot json.RawMessage `json:"context_snapshot"`
}

type creativeFeedbackEventResponse struct {
	ID              string          `json:"id"`
	IdempotencyKey  string          `json:"idempotency_key"`
	WorkspaceID     string          `json:"workspace_id"`
	IssueID         string          `json:"issue_id"`
	ActorType       string          `json:"actor_type"`
	ActorID         string          `json:"actor_id"`
	SubjectType     string          `json:"subject_type"`
	SubjectID       string          `json:"subject_id"`
	EventType       string          `json:"event_type"`
	Decision        string          `json:"decision"`
	ReasonCodes     []string        `json:"reason_codes"`
	Comment         string          `json:"comment"`
	Annotation      json.RawMessage `json:"annotation"`
	ContextSnapshot json.RawMessage `json:"context_snapshot"`
	UndoOfID        string          `json:"undo_of_id"`
	CreatedAt       string          `json:"created_at"`
}

type creativeFeedbackMetricsResponse struct {
	CandidateSelected int `json:"candidate_selected"`
	CandidateRejected int `json:"candidate_rejected"`
	CopyAccepted      int `json:"copy_accepted"`
	CopyReplaced      int `json:"copy_replaced"`
	VariantAccepted   int `json:"variant_accepted"`
	VariantRevision   int `json:"variant_needs_revision"`
	AssetAccepted     int `json:"asset_accepted"`
	AssetReported     int `json:"asset_reported"`
	QCAccepted        int `json:"qc_accepted"`
	QCMissedIssue     int `json:"qc_missed_issue"`
	QCFalsePositive   int `json:"qc_false_positive"`
}

var creativeFeedbackReasonCodes = map[string]map[string]struct{}{
	"candidate": {
		"duplicate": {}, "irrelevant": {}, "low_quality": {}, "composition_unsuitable": {},
		"copy_unsuitable": {}, "competitor_hard_to_replace": {}, "app_ui_unsuitable": {}, "other": {},
	},
	"recommended_copy": {
		"benefit_mismatch": {}, "facts_inapplicable": {}, "unnatural": {}, "tone_mismatch": {},
		"too_long": {}, "compliance_risk": {}, "translation": {}, "other": {},
	},
	"variant": {
		"visual_direction_mismatch": {}, "benefit_mismatch": {}, "layout_mismatch": {}, "brand_issue": {}, "other": {},
	},
	"asset": {
		"copy_error": {}, "theme_mismatch": {}, "subject_mismatch": {}, "size_inconsistency": {},
		"brand_or_prime": {}, "broken_image": {}, "competitor_residue": {}, "other": {},
	},
	"qc": {
		"missed_issue": {}, "false_positive": {}, "warning_accepted": {}, "other": {},
	},
}

func (h *Handler) CreateCreativeFeedbackEvent(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var input creativeFeedbackEventInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid feedback event")
		return
	}
	input, err := normalizeCreativeFeedbackEvent(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	subjectID, ok := parseUUIDOrBadRequest(w, input.SubjectID, "subject_id")
	if !ok {
		return
	}
	issueID, ok := optionalUUIDOrBadRequest(w, input.IssueID, "issue_id")
	if !ok {
		return
	}
	if !h.creativeFeedbackSubjectExists(r, workspaceID, issueID, input.SubjectType, subjectID) {
		writeError(w, http.StatusUnprocessableEntity, "feedback subject does not belong to this workspace")
		return
	}

	actorType, actorID := h.resolveActor(r, uuidToString(userID), uuidToString(workspaceID))
	actorUUID, ok := parseUUIDOrBadRequest(w, actorID, "actor_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create feedback event")
		return
	}
	defer tx.Rollback(r.Context())
	if input.IdempotencyKey != "" {
		if _, err := tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, uuidToString(workspaceID)+":"+input.IdempotencyKey); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create feedback event")
			return
		}
		existing, err := scanCreativeFeedbackEvent(tx.QueryRow(r.Context(), creativeFeedbackEventSelect+`
WHERE workspace_id = $1 AND idempotency_key = $2
`, workspaceID, input.IdempotencyKey))
		if err == nil {
			if err := tx.Commit(r.Context()); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to recover feedback event")
				return
			}
			writeJSON(w, http.StatusOK, existing)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to recover feedback event")
			return
		}
	}
	response, err := scanCreativeFeedbackEvent(tx.QueryRow(r.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, idempotency_key, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, decision, reason_codes, comment, annotation, context_snapshot
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13::jsonb)
RETURNING id::text, idempotency_key, workspace_id::text, COALESCE(issue_id::text, ''), actor_type,
  actor_id::text, subject_type, subject_id::text, event_type, decision, reason_codes,
  comment, annotation::text, context_snapshot::text, COALESCE(undo_of_id::text, ''), created_at::text
`, workspaceID, input.IdempotencyKey, issueID, actorType, actorUUID, input.SubjectType, subjectID, input.EventType,
		input.Decision, input.ReasonCodes, input.Comment, input.Annotation, input.ContextSnapshot))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create feedback event")
		return
	}
	if err := h.applyCreativeCandidateDecision(r, tx, workspaceID, issueID, userID, input, subjectID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create feedback event")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "feedback", "subject_type": input.SubjectType, "subject_id": input.SubjectID,
	})
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) UndoCreativeFeedbackEvent(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	eventID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "feedback_event_id")
	if !ok {
		return
	}
	actorType, actorID := h.resolveActor(r, uuidToString(userID), uuidToString(workspaceID))
	actorUUID, ok := parseUUIDOrBadRequest(w, actorID, "actor_id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to undo feedback event")
		return
	}
	defer tx.Rollback(r.Context())
	var original creativeFeedbackEventResponse
	err = tx.QueryRow(r.Context(), creativeFeedbackEventSelect+`
WHERE id = $1 AND workspace_id = $2 FOR UPDATE
`, eventID, workspaceID).Scan(
		&original.ID, &original.IdempotencyKey, &original.WorkspaceID, &original.IssueID, &original.ActorType, &original.ActorID,
		&original.SubjectType, &original.SubjectID, &original.EventType, &original.Decision, &original.ReasonCodes,
		&original.Comment, &original.Annotation, &original.ContextSnapshot, &original.UndoOfID, &original.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "feedback event not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to undo feedback event")
		return
	}
	if original.EventType == "undo" || original.UndoOfID != "" {
		writeError(w, http.StatusUnprocessableEntity, "feedback event cannot be undone")
		return
	}
	var alreadyUndone bool
	if err := tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM creative_feedback_event WHERE undo_of_id = $1)`, eventID).Scan(&alreadyUndone); err != nil || alreadyUndone {
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to undo feedback event")
		} else {
			writeError(w, http.StatusConflict, "feedback event has already been undone")
		}
		return
	}
	response, err := scanCreativeFeedbackEvent(tx.QueryRow(r.Context(), `
INSERT INTO creative_feedback_event (
  workspace_id, issue_id, actor_type, actor_id, subject_type, subject_id,
  event_type, context_snapshot, undo_of_id
) VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6, 'undo', $7::jsonb, $8)
RETURNING id::text, idempotency_key, workspace_id::text, COALESCE(issue_id::text, ''), actor_type,
  actor_id::text, subject_type, subject_id::text, event_type, decision, reason_codes,
  comment, annotation::text, context_snapshot::text, COALESCE(undo_of_id::text, ''), created_at::text
`, workspaceID, original.IssueID, actorType, actorUUID, original.SubjectType, eventIDToUUID(original.SubjectID), original.ContextSnapshot, eventID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to undo feedback event")
		return
	}
	if err := h.undoCreativeCandidateDecision(r, tx, workspaceID, original); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to undo feedback event")
		return
	}
	h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
		"scope": "feedback", "subject_type": original.SubjectType, "subject_id": original.SubjectID,
	})
	writeJSON(w, http.StatusCreated, response)
}

func (h *Handler) ListCreativeFeedbackEvents(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	subjectType := strings.TrimSpace(r.URL.Query().Get("subject_type"))
	if subjectType != "" && !validCreativeFeedbackSubjectType(subjectType) {
		writeError(w, http.StatusBadRequest, "invalid subject_type")
		return
	}
	subjectIDRaw := strings.TrimSpace(r.URL.Query().Get("subject_id"))
	var subjectID pgtype.UUID
	if subjectIDRaw != "" {
		var valid bool
		subjectID, valid = parseUUIDOrBadRequest(w, subjectIDRaw, "subject_id")
		if !valid {
			return
		}
	}
	limit := 100
	rows, err := h.DB.Query(r.Context(), creativeFeedbackEventSelect+`
WHERE workspace_id = $1
  AND ($2 = '' OR subject_type = $2)
  AND ($3::uuid IS NULL OR subject_id = $3)
ORDER BY created_at DESC
LIMIT $4
`, workspaceID, subjectType, nullableUUID(subjectID, subjectIDRaw != ""), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list feedback events")
		return
	}
	defer rows.Close()
	events := []creativeFeedbackEventResponse{}
	for rows.Next() {
		event, err := scanCreativeFeedbackEvent(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read feedback events")
			return
		}
		events = append(events, event)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (h *Handler) GetCreativeFeedbackMetrics(w http.ResponseWriter, r *http.Request) {
	workspaceID, _, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	var metrics creativeFeedbackMetricsResponse
	err := h.DB.QueryRow(r.Context(), `
SELECT
  count(*) FILTER (WHERE subject_type = 'candidate' AND decision = 'selected'),
  count(*) FILTER (WHERE subject_type = 'candidate' AND decision = 'rejected'),
  count(*) FILTER (WHERE subject_type = 'recommended_copy' AND decision = 'accepted'),
  count(*) FILTER (WHERE subject_type = 'recommended_copy' AND decision = 'replaced'),
  count(*) FILTER (WHERE subject_type = 'variant' AND decision = 'accepted'),
  count(*) FILTER (WHERE subject_type = 'variant' AND decision = 'needs_revision'),
  count(*) FILTER (WHERE subject_type = 'asset' AND decision = 'accepted'),
  count(*) FILTER (WHERE subject_type = 'asset' AND decision = 'reported'),
  count(*) FILTER (WHERE subject_type = 'qc' AND decision = 'accepted'),
  count(*) FILTER (WHERE subject_type = 'qc' AND reason_codes @> ARRAY['missed_issue']::text[]),
  count(*) FILTER (WHERE subject_type = 'qc' AND reason_codes @> ARRAY['false_positive']::text[])
FROM creative_feedback_event
WHERE workspace_id = $1 AND event_type <> 'undo'
`, workspaceID).Scan(
		&metrics.CandidateSelected, &metrics.CandidateRejected, &metrics.CopyAccepted, &metrics.CopyReplaced,
		&metrics.VariantAccepted, &metrics.VariantRevision, &metrics.AssetAccepted, &metrics.AssetReported,
		&metrics.QCAccepted, &metrics.QCMissedIssue, &metrics.QCFalsePositive,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to aggregate feedback metrics")
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (h *Handler) creativeFeedbackWorkspaceUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, pgtype.UUID, bool) {
	workspaceRaw := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceRaw); !ok {
		return pgtype.UUID{}, pgtype.UUID{}, false
	}
	return creativeWorkspaceUser(w, r, h)
}

func normalizeCreativeFeedbackEvent(input creativeFeedbackEventInput) (creativeFeedbackEventInput, error) {
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	input.IssueID = strings.TrimSpace(input.IssueID)
	input.SubjectType = strings.TrimSpace(input.SubjectType)
	input.SubjectID = strings.TrimSpace(input.SubjectID)
	input.EventType = strings.TrimSpace(input.EventType)
	input.Decision = strings.TrimSpace(input.Decision)
	input.Comment = strings.TrimSpace(input.Comment)
	if len(input.IdempotencyKey) > 200 {
		return input, errors.New("idempotency_key is too long")
	}
	if !validCreativeFeedbackSubjectType(input.SubjectType) || input.SubjectID == "" {
		return input, errors.New("invalid feedback subject")
	}
	if !validCreativeFeedbackEvent(input.SubjectType, input.EventType, input.Decision) {
		return input, errors.New("invalid feedback event or decision")
	}
	if len(input.Comment) > 4000 {
		return input, errors.New("comment is too long")
	}
	input.ReasonCodes = uniqueNonEmptyStrings(input.ReasonCodes)
	if len(input.ReasonCodes) > 12 {
		return input, errors.New("too many reason_codes")
	}
	for _, code := range input.ReasonCodes {
		if _, ok := creativeFeedbackReasonCodes[input.SubjectType][code]; !ok {
			return input, errors.New("invalid reason_code")
		}
	}
	if creativeFeedbackNeedsReason(input.Decision) && len(input.ReasonCodes) == 0 {
		return input, errors.New("reason_codes are required for this decision")
	}
	var err error
	input.Annotation, err = normalizedOptionalJSONObject(input.Annotation)
	if err != nil {
		return input, errors.New("annotation must be an object")
	}
	input.ContextSnapshot, err = normalizedOptionalJSONObject(input.ContextSnapshot)
	if err != nil {
		return input, errors.New("context_snapshot must be an object")
	}
	if input.EventType == "annotation" {
		if input.SubjectType != "asset" || !validCreativeAnnotation(input.Annotation) {
			return input, errors.New("annotation must be a valid relative asset annotation")
		}
	} else if string(input.Annotation) != "{}" {
		return input, errors.New("annotation is only supported for asset annotation events")
	}
	return input, nil
}

func validCreativeFeedbackSubjectType(subjectType string) bool {
	_, ok := creativeFeedbackReasonCodes[subjectType]
	return ok
}

func validCreativeFeedbackEvent(subjectType, eventType, decision string) bool {
	if eventType == "viewed" {
		return decision == ""
	}
	if eventType == "shortlisted" {
		return subjectType == "candidate" && decision == ""
	}
	if eventType == "annotation" {
		return subjectType == "asset" && (decision == "" || decision == "reported" || decision == "needs_revision")
	}
	if eventType == "replacement" {
		return subjectType == "recommended_copy" && decision == "replaced"
	}
	if eventType != "decision" {
		return false
	}
	switch subjectType {
	case "candidate":
		return decision == "selected" || decision == "rejected"
	case "recommended_copy":
		return decision == "accepted" || decision == "replaced"
	case "variant":
		return decision == "accepted" || decision == "needs_revision" || decision == "abandoned"
	case "asset":
		return decision == "accepted" || decision == "needs_revision" || decision == "reported"
	case "qc":
		return decision == "accepted" || decision == "reported"
	default:
		return false
	}
}

func creativeFeedbackNeedsReason(decision string) bool {
	switch decision {
	case "rejected", "replaced", "needs_revision", "abandoned", "reported":
		return true
	default:
		return false
	}
}

func validCreativeAnnotation(raw json.RawMessage) bool {
	var annotation struct {
		Kind   string  `json:"kind"`
		Type   string  `json:"type"`
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if json.Unmarshal(raw, &annotation) != nil {
		return false
	}
	kind := annotation.Kind
	if kind == "" {
		kind = annotation.Type
	}
	if annotation.X < 0 || annotation.X >= 1 || annotation.Y < 0 || annotation.Y >= 1 {
		return false
	}
	switch kind {
	case "point":
		return annotation.Width == 0 && annotation.Height == 0
	case "rect", "rectangle":
		return annotation.Width > 0 && annotation.Height > 0 && annotation.Width <= 1 && annotation.Height <= 1 &&
			annotation.X+annotation.Width <= 1 && annotation.Y+annotation.Height <= 1
	default:
		return false
	}
}

func (h *Handler) creativeFeedbackSubjectExists(r *http.Request, workspaceID, issueID pgtype.UUID, subjectType string, subjectID pgtype.UUID) bool {
	var exists bool
	query := ""
	switch subjectType {
	case "candidate":
		query = `SELECT EXISTS(SELECT 1 FROM creative_material_candidate WHERE id = $1 AND workspace_id = $2)`
	case "recommended_copy":
		query = `SELECT EXISTS(SELECT 1 FROM creative_copy_entry WHERE id = $1 AND workspace_id = $2)`
	case "variant":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_variant v JOIN creative_order_item i ON i.id = v.order_item_id JOIN creative_order o ON o.id = i.order_id WHERE v.id = $1 AND o.workspace_id = $2)`
	case "asset":
		query = `SELECT EXISTS(
  SELECT 1 FROM creative_order_asset a
  JOIN creative_order_variant v ON v.id = a.variant_id
  JOIN creative_order_item i ON i.id = v.order_item_id
  JOIN creative_order o ON o.id = i.order_id
  WHERE a.id = $1 AND o.workspace_id = $2
) OR EXISTS(SELECT 1 FROM attachment WHERE id = $1 AND workspace_id = $2)`
	case "qc":
		query = `SELECT EXISTS(SELECT 1 FROM creative_order_qc_report q JOIN creative_order_variant v ON v.id = q.variant_id JOIN creative_order_item i ON i.id = v.order_item_id JOIN creative_order o ON o.id = i.order_id WHERE q.id = $1 AND o.workspace_id = $2)`
	}
	if query == "" || h.DB.QueryRow(r.Context(), query, subjectID, workspaceID).Scan(&exists) != nil || !exists {
		return false
	}
	if !issueID.Valid {
		return true
	}
	if h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM issue WHERE id = $1 AND workspace_id = $2)`, issueID, workspaceID).Scan(&exists) != nil || !exists {
		return false
	}
	if subjectType == "candidate" {
		return h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM creative_material_issue_candidate WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3)`, issueID, subjectID, workspaceID).Scan(&exists) == nil && exists
	}
	return true
}

func (h *Handler) applyCreativeCandidateDecision(r *http.Request, tx pgx.Tx, workspaceID, issueID, userID pgtype.UUID, input creativeFeedbackEventInput, subjectID pgtype.UUID) error {
	if input.SubjectType != "candidate" || input.EventType != "decision" || (input.Decision != "selected" && input.Decision != "rejected") {
		return nil
	}
	if !issueID.Valid {
		return nil
	}
	result, err := tx.Exec(r.Context(), `
UPDATE creative_material_issue_candidate
SET status = $4, selected_at = CASE WHEN $4 = 'selected' THEN now() ELSE NULL END,
    selected_by = CASE WHEN $4 = 'selected' THEN $5::uuid ELSE NULL END, updated_at = now()
WHERE issue_id = $1 AND candidate_id = $2 AND workspace_id = $3
`, issueID, subjectID, workspaceID, input.Decision, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("candidate is not linked to this issue")
	}
	return nil
}

func (h *Handler) undoCreativeCandidateDecision(r *http.Request, tx pgx.Tx, workspaceID pgtype.UUID, original creativeFeedbackEventResponse) error {
	if original.SubjectType != "candidate" || original.EventType != "decision" || (original.Decision != "selected" && original.Decision != "rejected") || original.IssueID == "" {
		return nil
	}
	var laterDecision bool
	if err := tx.QueryRow(r.Context(), `
SELECT EXISTS(
  SELECT 1 FROM creative_feedback_event
  WHERE workspace_id = $1 AND issue_id = $2::uuid AND subject_type = 'candidate'
    AND subject_id = $3::uuid AND event_type = 'decision' AND created_at > $4::timestamptz
)
`, workspaceID, original.IssueID, original.SubjectID, original.CreatedAt).Scan(&laterDecision); err != nil {
		return err
	}
	if laterDecision {
		return nil
	}
	_, err := tx.Exec(r.Context(), `
UPDATE creative_material_issue_candidate
SET status = 'new', selected_at = NULL, selected_by = NULL, updated_at = now()
WHERE issue_id = $1::uuid AND candidate_id = $2::uuid AND workspace_id = $3
`, original.IssueID, original.SubjectID, workspaceID)
	return err
}

func scanCreativeFeedbackEvent(row rowScanner) (creativeFeedbackEventResponse, error) {
	var event creativeFeedbackEventResponse
	var annotation, snapshot string
	err := row.Scan(&event.ID, &event.IdempotencyKey, &event.WorkspaceID, &event.IssueID, &event.ActorType, &event.ActorID,
		&event.SubjectType, &event.SubjectID, &event.EventType, &event.Decision, &event.ReasonCodes,
		&event.Comment, &annotation, &snapshot, &event.UndoOfID, &event.CreatedAt)
	event.Annotation = json.RawMessage(annotation)
	event.ContextSnapshot = json.RawMessage(snapshot)
	return event, err
}

const creativeFeedbackEventSelect = `
SELECT id::text, idempotency_key, workspace_id::text, COALESCE(issue_id::text, ''), actor_type,
  actor_id::text, subject_type, subject_id::text, event_type, decision, reason_codes,
  comment, annotation::text, context_snapshot::text, COALESCE(undo_of_id::text, ''), created_at::text
FROM creative_feedback_event
`

func nullableUUID(value pgtype.UUID, valid bool) any {
	if !valid {
		return nil
	}
	return value
}

func normalizedOptionalJSONObject(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return json.RawMessage(`{}`), nil
	}
	return normalizedJSONObject(raw)
}

func eventIDToUUID(value string) pgtype.UUID {
	return parseUUID(value)
}
