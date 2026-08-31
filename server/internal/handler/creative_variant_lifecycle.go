package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type creativeOrderVariantRevision struct {
	Revision      int             `json:"revision"`
	Brief         json.RawMessage `json:"brief"`
	Status        string          `json:"status"`
	ExpectedSizes []string        `json:"expected_sizes"`
	ActivatedAt   string          `json:"activated_at"`
	CreatedAt     string          `json:"created_at"`
	UpdatedAt     string          `json:"updated_at"`
}

type creativeCandidateSelectionInput struct {
	SelectedIDs []string `json:"selected_ids"`
	ReserveIDs  []string `json:"reserve_ids"`
}

var (
	errCreativeActiveRevisionImmutable = errors.New("activated creative variant revision is immutable")
	errCreativeOrderCancelled          = errors.New("creative order is cancelled")
)

func validCreativeCandidateState(state string) bool {
	return state == "candidate" || state == "selected" || state == "reserve" || state == "rejected"
}

func expectedCreativeVariantProductionSizes(triggerKind string, inputSnapshot, brief json.RawMessage, candidateState, primarySize string) ([]string, error) {
	if candidateState == "candidate" && primarySize != creativeCandidatePreviewSize {
		return nil, errors.New("creative candidate primary_size must be 1080x1080")
	}
	if candidateState != "selected" {
		if !validCreativeAssetSize(primarySize) {
			return nil, errors.New("creative candidate primary_size is invalid")
		}
		return []string{primarySize}, nil
	}
	return expectedCreativeVariantSizes(triggerKind, inputSnapshot, brief)
}

func upsertCreativeVariantRevision(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, brief json.RawMessage, status string, expectedSizes []string) error {
	tag, err := tx.Exec(ctx, `
INSERT INTO creative_order_variant_revision (variant_id, revision, brief, status, expected_sizes)
VALUES ($1, $2, $3::jsonb, $4, $5::text[])
ON CONFLICT (variant_id, revision) DO UPDATE SET
  brief = EXCLUDED.brief,
  status = EXCLUDED.status,
  expected_sizes = EXCLUDED.expected_sizes,
	updated_at = CASE
	  WHEN creative_order_variant_revision.activated_at IS NULL THEN now()
	  ELSE creative_order_variant_revision.updated_at
	END
WHERE creative_order_variant_revision.activated_at IS NULL
   OR (
	 creative_order_variant_revision.brief = EXCLUDED.brief
	 AND creative_order_variant_revision.status = EXCLUDED.status
	 AND creative_order_variant_revision.expected_sizes = EXCLUDED.expected_sizes
	)
`, variantID, revision, brief, status, expectedSizes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errCreativeActiveRevisionImmutable
	}
	return nil
}

// markCreativeVariantRevisionActionRequired preserves the historical activation
// record while allowing a later blocking QC result to withdraw this revision
// from delivery. Content and expected-size snapshots remain immutable.
func markCreativeVariantRevisionActionRequired(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, brief json.RawMessage, expectedSizes []string) error {
	tag, err := tx.Exec(ctx, `
UPDATE creative_order_variant_revision
SET status = 'action_required', updated_at = now()
WHERE variant_id = $1 AND revision = $2
`, variantID, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	return upsertCreativeVariantRevision(ctx, tx, variantID, revision, brief, "action_required", expectedSizes)
}

func syncCreativeVariantRevisionFromVariant(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string) error {
	var brief string
	var status string
	if err := tx.QueryRow(ctx, `
SELECT brief::text, status
FROM creative_order_variant
WHERE id = $1 AND revision = $2
FOR UPDATE
`, variantID, revision).Scan(&brief, &status); err != nil {
		return err
	}
	if err := upsertCreativeVariantRevision(ctx, tx, variantID, revision, json.RawMessage(brief), status, expectedSizes); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET staging_revision = CASE
      WHEN active_revision = $2 AND status = 'completed' THEN NULL
      ELSE $2
    END,
    updated_at = now()
WHERE id = $1 AND revision = $2
`, variantID, revision)
	return err
}

func activateCreativeVariantRevision(ctx context.Context, tx pgx.Tx, variantID pgtype.UUID, revision int, expectedSizes []string) error {
	var orderID pgtype.UUID
	if err := tx.QueryRow(ctx, `
SELECT item.order_id
FROM creative_order_variant variant
JOIN creative_order_item item ON item.id = variant.order_item_id
WHERE variant.id = $1
	`, variantID).Scan(&orderID); err != nil {
		return err
	}
	var orderStatus string
	if err := tx.QueryRow(ctx, `
SELECT status
FROM creative_order
WHERE id = $1
FOR UPDATE
	`, orderID).Scan(&orderStatus); err != nil {
		return err
	}
	if orderStatus == "cancelled" {
		return errCreativeOrderCancelled
	}

	var brief string
	var status string
	var revisionExpectedSizes []string
	var itemID, orderCreatedBy pgtype.UUID
	var triggerKind, pipelineVersion string
	var variantCount int
	if err := tx.QueryRow(ctx, `
SELECT variant.brief::text, variant.status, revision_row.expected_sizes,
       item.id, order_row.created_by, order_row.trigger_evidence_kind,
       COALESCE(order_row.input_snapshot->>'pipeline_version', ''),
       (SELECT count(*) FROM creative_order_variant sibling WHERE sibling.order_item_id = item.id)
FROM creative_order_variant variant
JOIN creative_order_variant_revision revision_row
  ON revision_row.variant_id = variant.id AND revision_row.revision = $2
JOIN creative_order_item item ON item.id = variant.order_item_id
JOIN creative_order order_row ON order_row.id = item.order_id
WHERE variant.id = $1 AND variant.revision = $2
FOR UPDATE OF variant, revision_row, item
	`, variantID, revision).Scan(
		&brief, &status, &revisionExpectedSizes, &itemID, &orderCreatedBy,
		&triggerKind, &pipelineVersion, &variantCount,
	); err != nil {
		return err
	}
	revisionExpectedSizes, err := normalizeCreativeExpectedSizes(revisionExpectedSizes)
	if err != nil {
		return errors.New("creative variant revision has an invalid delivery contract")
	}
	requestedSizes, err := normalizeCreativeExpectedSizes(expectedSizes)
	if err != nil || !slices.Equal(requestedSizes, revisionExpectedSizes) {
		return errors.New("activation sizes do not match the creative variant revision contract")
	}
	expectedSizes = revisionExpectedSizes
	if err := upsertCreativeVariantRevision(ctx, tx, variantID, revision, json.RawMessage(brief), status, expectedSizes); err != nil {
		return err
	}
	var deliveredCount int
	if err := tx.QueryRow(ctx, `
SELECT count(DISTINCT asset.size_key)
FROM creative_order_asset asset
WHERE asset.variant_id = $1
  AND asset.revision = $2
  AND asset.stage = 'delivered'
  AND asset.status = 'completed'
  AND asset.attachment_id IS NOT NULL
  AND asset.size_key = ANY($3::text[])
`, variantID, revision, expectedSizes).Scan(&deliveredCount); err != nil {
		return err
	}
	if deliveredCount != len(expectedSizes) {
		return errors.New("completed delivered assets do not match the revision delivery contract")
	}
	if _, err := tx.Exec(ctx, `
UPDATE creative_order_variant_revision
SET status = 'completed', activated_at = COALESCE(activated_at, now()), updated_at = now()
WHERE variant_id = $1 AND revision = $2
`, variantID, revision); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
UPDATE creative_order_variant
SET status = 'completed', active_revision = $2, staging_revision = NULL, updated_at = now()
WHERE id = $1 AND revision = $2
`, variantID, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("creative variant revision changed before activation")
	}
	if triggerKind == "creative_direct_edit" && pipelineVersion == creativePipelineDirectEditV1 {
		if variantCount != 1 {
			return errors.New("direct image edit activation requires exactly one variant")
		}
		tag, err := tx.Exec(ctx, `
UPDATE creative_order_item
SET adopted_variant_id = $2, adopted_at = now(), adopted_by = $3, updated_at = now()
WHERE id = $1
  AND (adopted_variant_id IS NULL OR adopted_variant_id = $2)
`, itemID, variantID, orderCreatedBy)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("direct image edit item has a conflicting adopted variant")
		}
	}
	return nil
}

func (h *Handler) listCreativeOrderVariantRevisions(ctx context.Context, variantID pgtype.UUID) ([]creativeOrderVariantRevision, error) {
	rows, err := h.DB.Query(ctx, `
SELECT revision, brief::text, status, expected_sizes, COALESCE(activated_at::text, ''), created_at::text, updated_at::text
FROM creative_order_variant_revision
WHERE variant_id = $1
ORDER BY revision
`, variantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	revisions := []creativeOrderVariantRevision{}
	for rows.Next() {
		var revision creativeOrderVariantRevision
		var brief string
		if err := rows.Scan(&revision.Revision, &brief, &revision.Status, &revision.ExpectedSizes, &revision.ActivatedAt, &revision.CreatedAt, &revision.UpdatedAt); err != nil {
			return nil, err
		}
		revision.Brief = json.RawMessage(brief)
		revisions = append(revisions, revision)
	}
	return revisions, rows.Err()
}

func (h *Handler) SelectCreativeOrderItemCandidates(w http.ResponseWriter, r *http.Request) {
	workspaceID, userID, ok := h.creativeFeedbackWorkspaceUser(w, r)
	if !ok {
		return
	}
	orderID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "order_id")
	if !ok {
		return
	}
	itemID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "itemId"), "order_item_id")
	if !ok {
		return
	}
	if !h.requireCreativeOrderWritable(w, r, orderID, workspaceID) {
		return
	}
	if !h.requireCreativeCandidateSelectionTask(w, r, orderID, itemID) {
		return
	}
	taskActor := r.Header.Get("X-Actor-Source") == "task_token"
	var taskID pgtype.UUID
	if taskActor {
		taskID = parseUUID(r.Header.Get("X-Task-ID"))
	}
	var input creativeCandidateSelectionInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid creative candidate selection")
		return
	}
	if len(input.SelectedIDs) != 3 || len(input.ReserveIDs) > 2 {
		writeError(w, http.StatusBadRequest, "candidate selection requires three selected variants and up to two ordered reserves")
		return
	}
	orderedIDs := append(append([]string{}, input.SelectedIDs...), input.ReserveIDs...)
	seen := make(map[string]struct{}, len(orderedIDs))
	parsed := make([]pgtype.UUID, 0, len(orderedIDs))
	for _, rawID := range orderedIDs {
		id, parseOK := parseUUIDOrBadRequest(w, strings.TrimSpace(rawID), "variant_id")
		if !parseOK {
			return
		}
		canonical := uuidToString(id)
		if _, duplicate := seen[canonical]; duplicate {
			writeError(w, http.StatusBadRequest, "candidate selection contains duplicate variants")
			return
		}
		seen[canonical] = struct{}{}
		parsed = append(parsed, id)
	}
	respondAfterCommit := func() {
		if _, err := h.queueSelectedCreativeProductionTasks(
			r.Context(),
			itemID,
			creativeOrchestrationCause{RequestedBy: userID},
			creativeSelectedExpansionPhase,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "creative candidate selection was saved but missing-size production could not be queued")
			return
		}
		item, err := h.loadCreativeOrderItem(r, itemID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load selected creative candidates")
			return
		}
		slices.SortFunc(item.Variants, func(left, right creativeOrderVariantResponse) int {
			if left.SelectionRank == 0 {
				return 1
			}
			if right.SelectionRank == 0 {
				return -1
			}
			return left.SelectionRank - right.SelectionRank
		})
		h.publishCreativeDomainUpdated(r, workspaceID, userID, map[string]any{
			"scope": "order", "order_id": chi.URLParam(r, "id"), "order_item_id": chi.URLParam(r, "itemId"),
		})
		writeJSON(w, http.StatusOK, item)
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start creative candidate selection")
		return
	}
	defer tx.Rollback(r.Context())

	if taskActor {
		var taskStatus string
		if err := tx.QueryRow(r.Context(), `
SELECT status FROM agent_task_queue WHERE id = $1 FOR UPDATE
`, taskID).Scan(&taskStatus); err != nil {
			writeError(w, http.StatusForbidden, "candidate selection task is no longer available")
			return
		}
		if taskStatus != "running" {
			writeError(w, http.StatusConflict, "candidate selection task is no longer active")
			return
		}
	}
	var triggerKind, inputSnapshot, orderStatus string
	if err := tx.QueryRow(r.Context(), `
SELECT trigger_evidence_kind, input_snapshot::text, status
FROM creative_order
WHERE id = $1 AND workspace_id = $2
FOR UPDATE
`, orderID, workspaceID).Scan(&triggerKind, &inputSnapshot, &orderStatus); errors.Is(err, pgx.ErrNoRows) {
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
	var lockedItemID pgtype.UUID
	if err := tx.QueryRow(r.Context(), `
SELECT id FROM creative_order_item
WHERE id = $1 AND order_id = $2
FOR UPDATE
`, itemID, orderID).Scan(&lockedItemID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "creative order item not found")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative order item")
		return
	}
	if triggerKind == "creative_direct_edit" {
		writeError(w, http.StatusConflict, "direct image edits do not support candidate selection")
		return
	}
	if creativeOrderPipelineVersion(json.RawMessage(inputSnapshot)) != creativePipelineCandidateV1 {
		writeError(w, http.StatusConflict, "standard creative orders require pipeline_version candidate_v1")
		return
	}

	type candidate struct {
		id          pgtype.UUID
		variantKey  string
		revision    int
		brief       json.RawMessage
		primarySize string
		state       string
		rank        pgtype.Int4
	}
	rows, err := tx.Query(r.Context(), `
SELECT id, variant_key, revision, brief::text, primary_size, candidate_state, selection_rank
FROM creative_order_variant
WHERE order_item_id = $1 AND candidate_state <> 'rejected'
ORDER BY created_at, id
FOR UPDATE
`, itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to lock creative candidates")
		return
	}
	candidates := make(map[string]candidate, len(orderedIDs))
	for rows.Next() {
		var value candidate
		var brief string
		if err := rows.Scan(&value.id, &value.variantKey, &value.revision, &brief, &value.primarySize, &value.state, &value.rank); err != nil {
			rows.Close()
			writeError(w, http.StatusInternalServerError, "failed to read creative candidates")
			return
		}
		if !isCreativeCandidateVariantKey(value.variantKey) {
			rows.Close()
			writeError(w, http.StatusConflict, "candidate creative orders only accept C01-C05 candidate variants")
			return
		}
		value.brief = json.RawMessage(brief)
		candidates[uuidToString(value.id)] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		writeError(w, http.StatusInternalServerError, "failed to read creative candidates")
		return
	}
	rows.Close()
	if len(candidates) != len(orderedIDs) || len(candidates) < 3 || len(candidates) > 5 {
		writeError(w, http.StatusConflict, "candidate selection must rank every non-rejected candidate in this order item")
		return
	}
	for id := range seen {
		if _, exists := candidates[id]; !exists {
			writeError(w, http.StatusUnprocessableEntity, "candidate variant does not belong to this creative order item")
			return
		}
	}
	rankingPresent := false
	rankingMatches := true
	for index, id := range parsed {
		value := candidates[uuidToString(id)]
		expectedState := "selected"
		if index >= len(input.SelectedIDs) {
			expectedState = "reserve"
		}
		if value.state != "candidate" || value.rank.Valid {
			rankingPresent = true
		}
		if value.state != expectedState || !value.rank.Valid || int(value.rank.Int32) != index+1 {
			rankingMatches = false
		}
	}
	if rankingPresent {
		if !rankingMatches {
			writeError(w, http.StatusConflict, "creative candidate selection is already finalized")
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read saved creative candidate selection")
			return
		}
		respondAfterCommit()
		return
	}

	var readyCount int
	if err := tx.QueryRow(r.Context(), `
SELECT count(*)
FROM creative_order_variant variant
WHERE variant.id = ANY($1::uuid[])
  AND EXISTS (
    SELECT 1
    FROM creative_order_asset asset
    WHERE asset.variant_id = variant.id
      AND asset.revision = variant.revision
      AND asset.size_key = variant.primary_size
      AND asset.stage = 'generated'
      AND asset.status = 'completed'
      AND asset.attachment_id IS NOT NULL
  )
  AND EXISTS (
    SELECT 1
    FROM creative_order_asset asset
    WHERE asset.variant_id = variant.id
      AND asset.revision = variant.revision
      AND asset.size_key = variant.primary_size
      AND asset.stage = 'primed'
      AND asset.status = 'completed'
      AND asset.attachment_id IS NOT NULL
  )
`, parsed).Scan(&readyCount); err != nil || readyCount != len(parsed) {
		writeError(w, http.StatusConflict, "all candidate primary images and Prime compositions must complete before selection")
		return
	}

	if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET selection_rank = NULL, updated_at = now()
WHERE order_item_id = $1 AND candidate_state <> 'rejected'
`, itemID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to reset creative candidate ranking")
		return
	}
	fullSizes, err := expectedCreativeVariantSizes(triggerKind, json.RawMessage(inputSnapshot), json.RawMessage(`{}`))
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	for index, id := range parsed {
		value := candidates[uuidToString(id)]
		state := "selected"
		expectedSizes := fullSizes
		status := "running"
		if index >= len(input.SelectedIDs) {
			state = "reserve"
			expectedSizes = []string{value.primarySize}
			status = "completed"
		}
		if _, err := tx.Exec(r.Context(), `
UPDATE creative_order_variant
SET candidate_state = $2,
    selection_rank = $3,
    status = $4,
    staging_revision = CASE WHEN active_revision = revision THEN NULL ELSE revision END,
    updated_at = now()
WHERE id = $1
`, id, state, index+1, status); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to rank creative candidates")
			return
		}
		if err := upsertCreativeVariantRevision(r.Context(), tx, id, value.revision, value.brief, status, expectedSizes); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update creative candidate delivery contract")
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE creative_order SET updated_at = now() WHERE id = $1`, orderID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update creative order after candidate selection")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save creative candidate selection")
		return
	}
	respondAfterCommit()
}
