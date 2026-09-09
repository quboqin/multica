package handler

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type creativeRecoveryAttemptResponse struct {
	ID            string `json:"id"`
	Attempt       int    `json:"attempt"`
	Status        string `json:"status"`
	SourceTaskID  string `json:"source_task_id"`
	ResultTaskID  string `json:"result_task_id"`
	ResultAssetID string `json:"result_asset_id"`
	ReasonCode    string `json:"reason_code"`
	ErrorMessage  string `json:"error_message"`
	StartedAt     string `json:"started_at"`
	CompletedAt   string `json:"completed_at"`
}

type creativeOrderRecoveryResponse struct {
	ID              string                            `json:"id"`
	OrderItemID     string                            `json:"order_item_id"`
	VariantID       string                            `json:"variant_id"`
	Revision        int                               `json:"revision"`
	Stage           string                            `json:"stage"`
	SizeKey         string                            `json:"size_key"`
	Status          string                            `json:"status"`
	ReasonCode      string                            `json:"reason_code"`
	SourceTaskID    string                            `json:"source_task_id"`
	ResultTaskID    string                            `json:"result_task_id"`
	ResolvedAssetID string                            `json:"resolved_asset_id"`
	Attempt         int                               `json:"attempt"`
	MaxAttempts     int                               `json:"max_attempts"`
	NextRetryAt     string                            `json:"next_retry_at"`
	LastError       string                            `json:"last_error"`
	CreatedAt       string                            `json:"created_at"`
	UpdatedAt       string                            `json:"updated_at"`
	ResolvedAt      string                            `json:"resolved_at"`
	Attempts        []creativeRecoveryAttemptResponse `json:"attempts"`
}

func (h *Handler) listCreativeOrderRecoveries(ctx context.Context, orderID pgtype.UUID) ([]creativeOrderRecoveryResponse, error) {
	values, err := creativeRowsByOwner[creativeOrderRecoveryResponse](ctx, h.DB, `SELECT r.order_id::text,
 (to_jsonb(r)-'lease_token'-'lease_expires_at')||jsonb_build_object('attempt',r.dispatch_count,'created_at',r.created_at::text,'updated_at',r.updated_at::text,'resolved_at',COALESCE(r.resolved_at::text,''),'next_retry_at',r.next_retry_at::text,
 'attempts',COALESCE((SELECT jsonb_agg((to_jsonb(a)-'lease_token')||jsonb_build_object('started_at',a.started_at::text,'completed_at',COALESCE(a.completed_at::text,'')) ORDER BY a.attempt) FROM creative_recovery_attempt a WHERE a.recovery_id=r.id),'[]'::jsonb))
FROM creative_recovery r WHERE r.order_id=ANY($1::uuid[]) ORDER BY r.created_at,r.id`, []pgtype.UUID{orderID})
	return creativeOwnerRows(values, orderID), err
}
