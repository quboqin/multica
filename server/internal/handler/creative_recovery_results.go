package handler

import (
	"context"
	"github.com/jackc/pgx/v5/pgtype"
)

func (h *Handler) refreshCreativeRecoveryResults(ctx context.Context, orderID pgtype.UUID) error {
	_, err := h.DB.Exec(ctx, `WITH outcomes AS (
 SELECT r.id,a.id AS asset_id,
 CASE WHEN o.status='cancelled' OR i.status='cancelled' OR v.status='cancelled'
 OR (r.variant_id IS NOT NULL AND (v.revision<>r.revision OR v.candidate_state IN ('reserve','rejected'))) THEN 'cancelled'
 WHEN a.id IS NOT NULL OR v.active_revision=r.revision
 OR (r.stage='candidate_selection' AND EXISTS(SELECT 1 FROM creative_order_variant selected WHERE selected.order_item_id=r.order_item_id AND selected.candidate_state='selected' AND selected.selection_rank IS NOT NULL)) THEN 'resolved'
 END AS status
 FROM creative_recovery r JOIN creative_order o ON o.id=r.order_id JOIN creative_order_item i ON i.id=r.order_item_id
 LEFT JOIN creative_order_variant v ON v.id=r.variant_id
 LEFT JOIN creative_order_asset a ON a.variant_id=r.variant_id AND a.revision=r.revision AND a.size_key=r.size_key
 AND a.stage=CASE r.stage WHEN 'production' THEN 'generated' WHEN 'prime' THEN 'primed' WHEN 'qc' THEN 'delivered' END
 AND a.status='completed' AND a.attachment_id IS NOT NULL
 WHERE r.order_id=$1 AND r.status NOT IN ('running','resolved','cancelled')
), changed AS (
 UPDATE creative_recovery r SET status=o.status,resolved_asset_id=o.asset_id,resolved_at=now(),updated_at=now()
 FROM outcomes o WHERE r.id=o.id AND o.status IS NOT NULL RETURNING r.id,r.attempt,r.resolved_asset_id
)
UPDATE creative_recovery_attempt a SET result_asset_id=c.resolved_asset_id FROM changed c
WHERE a.recovery_id=c.id AND a.attempt=c.attempt AND c.resolved_asset_id IS NOT NULL`, orderID)
	return err
}
