package handler

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

func creativeRowsByOwner[T any](ctx context.Context, q dbExecutor, query string, ids []pgtype.UUID) (map[string][]T, error) {
	result := make(map[string][]T)
	rows, err := q.Query(ctx, query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var owner string
		var raw []byte
		var value T
		if err := rows.Scan(&owner, &raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		result[owner] = append(result[owner], value)
	}
	return result, rows.Err()
}

func creativeOwnerRows[T any](values map[string][]T, id pgtype.UUID) []T {
	result := values[uuidToString(id)]
	if result == nil {
		return []T{}
	}
	return result
}

const creativeAssetBatchSQL = `SELECT a.variant_id::text,to_jsonb(a)||jsonb_build_object('created_at',a.created_at::text,'updated_at',a.updated_at::text)
FROM creative_order_asset a WHERE a.variant_id=ANY($1::uuid[]) ORDER BY a.revision,a.size_key,a.stage`
const creativeRevisionBatchSQL = `SELECT v.variant_id::text,to_jsonb(v)||jsonb_build_object('created_at',v.created_at::text,'updated_at',v.updated_at::text,'activated_at',COALESCE(v.activated_at::text,''))
FROM creative_order_variant_revision v WHERE v.variant_id=ANY($1::uuid[]) ORDER BY v.revision`
const creativeOperationBatchSQL = `SELECT p.variant_id::text,to_jsonb(p)||jsonb_build_object('created_at',p.created_at::text,'updated_at',p.updated_at::text,'started_at',COALESCE(p.started_at::text,''),'completed_at',COALESCE(p.completed_at::text,''))
FROM creative_image_operation p WHERE p.variant_id=ANY($1::uuid[]) ORDER BY p.revision,p.size_key,p.created_at`
const creativeOperationAttemptBatchSQL = `SELECT a.operation_id::text,to_jsonb(a)||jsonb_build_object('created_at',a.created_at::text,'updated_at',a.updated_at::text,'started_at',a.started_at::text,'completed_at',COALESCE(a.completed_at::text,''))
FROM creative_image_operation_attempt a WHERE a.operation_id=ANY($1::uuid[]) ORDER BY a.attempt`
const creativeQCReportBatchSQL = `SELECT q.variant_id::text,to_jsonb(q)||jsonb_build_object('created_at',q.created_at::text,'updated_at',q.updated_at::text)
FROM creative_order_qc_report q WHERE q.variant_id=ANY($1::uuid[]) ORDER BY q.revision,q.attempt,q.lane`
const creativeDiagnosticBatchSQL = `SELECT a.variant_id::text,to_jsonb(a)||jsonb_build_object('created_at',a.created_at::text,'updated_at',a.updated_at::text)
FROM creative_order_diagnostic_asset a WHERE a.variant_id=ANY($1::uuid[]) ORDER BY a.revision,a.size_key,a.updated_at DESC,a.id`

func (h *Handler) hydrateCreativeVariants(ctx context.Context, variants []creativeOrderVariantResponse) error {
	ids := make([]pgtype.UUID, 0, len(variants))
	for _, v := range variants {
		ids = append(ids, parseUUID(v.ID))
	}
	if len(ids) == 0 {
		return nil
	}
	assets, err := creativeRowsByOwner[creativeOrderAssetResponse](ctx, h.DB, creativeAssetBatchSQL, ids)
	if err != nil {
		return err
	}
	revisions, err := creativeRowsByOwner[creativeOrderVariantRevision](ctx, h.DB, creativeRevisionBatchSQL, ids)
	if err != nil {
		return err
	}
	operations, err := h.creativeImageOperationsBatch(ctx, ids)
	if err != nil {
		return err
	}
	reports, err := creativeRowsByOwner[creativeOrderQCReportResponse](ctx, h.DB, creativeQCReportBatchSQL, ids)
	if err != nil {
		return err
	}
	diagnostics, err := creativeRowsByOwner[creativeOrderDiagnosticAsset](ctx, h.DB, creativeDiagnosticBatchSQL, ids)
	if err != nil {
		return err
	}
	qc, err := h.creativeVariantQCStatuses(ctx, ids)
	if err != nil {
		return err
	}
	blockers, err := h.creativeVariantBlockers(ctx, ids)
	if err != nil {
		return err
	}
	for i := range variants {
		v := &variants[i]
		v.Assets = assets[v.ID]
		v.Revisions = revisions[v.ID]
		v.ImageOperations = operations[v.ID]
		v.QCReports = reports[v.ID]
		v.QCStatus = qc[v.ID]
		if v.Status != "cancelled" {
			v.DiagnosticAssets = diagnostics[v.ID]
			for j := range v.DiagnosticAssets {
				v.DiagnosticAssets[j].URL = attachmentDownloadPath(v.DiagnosticAssets[j].AttachmentID)
			}
		}
		if v.Status != "completed" && v.Status != "cancelled" {
			v.ActionRequired = creativeVariantBriefBlocker(v.Brief)
			if v.ActionRequired == nil {
				v.ActionRequired = blockers[v.ID]
			}
		}
	}
	return nil
}

func (h *Handler) creativeImageOperationsBatch(ctx context.Context, ids []pgtype.UUID) (map[string][]creativeImageOperationResponse, error) {
	operations, err := creativeRowsByOwner[creativeImageOperationResponse](ctx, h.DB, creativeOperationBatchSQL, ids)
	if err != nil {
		return nil, err
	}
	operationIDs := []pgtype.UUID{}
	for _, values := range operations {
		for _, p := range values {
			operationIDs = append(operationIDs, parseUUID(p.ID))
		}
	}
	attempts, err := creativeRowsByOwner[creativeImageOperationAttemptResponse](ctx, h.DB, creativeOperationAttemptBatchSQL, operationIDs)
	if err != nil {
		return nil, err
	}
	for _, values := range operations {
		for i := range values {
			values[i].Attempts = attempts[values[i].ID]
			if values[i].Attempts == nil {
				values[i].Attempts = []creativeImageOperationAttemptResponse{}
			}
		}
	}
	return operations, nil
}

func (h *Handler) creativeVariantQCStatuses(ctx context.Context, ids []pgtype.UUID) (map[string]string, error) {
	result := make(map[string]string, len(ids))
	rows, err := h.DB.Query(ctx, creativeVariantQCBatchSQL, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, status string
		if err := rows.Scan(&id, &status); err != nil {
			return nil, err
		}
		result[id] = status
	}
	return result, rows.Err()
}

func (h *Handler) creativeVariantBlockers(ctx context.Context, ids []pgtype.UUID) (map[string]*creativeOrderVariantBlocker, error) {
	result := make(map[string]*creativeOrderVariantBlocker, len(ids))
	rows, err := h.DB.Query(ctx, creativeVariantBlockerBatchSQL, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, detail string
		var b creativeOrderVariantBlocker
		if err := rows.Scan(&id, &b.TaskID, &b.Workflow, &b.FailureReason, &detail, &b.FailedAt, &b.Retryable); err != nil {
			return nil, err
		}
		b.Detail = summarizeCreativeOrderVariantBlocker(detail)
		result[id] = &b
	}
	return result, rows.Err()
}

func creativeVariantBriefBlocker(raw json.RawMessage) *creativeOrderVariantBlocker {
	var brief map[string]json.RawMessage
	if json.Unmarshal(raw, &brief) != nil {
		return nil
	}
	for _, entry := range []struct{ key, workflow, reason string }{
		{"creative_direct_edit_error", "creative_direct_edit", "direct_image_edit_failed"},
		{"creative_qc_handoff_error", "quality_control", "quality_control_queue_failed"},
		{"brand_composition_error", "brand_components", "brand_composition_failed"},
	} {
		var e struct {
			Message   string `json:"message"`
			UpdatedAt string `json:"updated_at"`
		}
		if json.Unmarshal(brief[entry.key], &e) == nil && strings.TrimSpace(e.Message) != "" {
			return &creativeOrderVariantBlocker{Workflow: entry.workflow, FailureReason: entry.reason, Detail: summarizeCreativeOrderVariantBlocker(e.Message), FailedAt: e.UpdatedAt, Retryable: true}
		}
	}
	return nil
}
