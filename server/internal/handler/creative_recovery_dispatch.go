package handler

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type creativeRecoveryDispatchContextKey struct{}

// recordCreativeRecoveryDispatchTx joins the domain dispatch transaction. A
// rolled-back enqueue consumes no budget; a committed enqueue keeps its receipt
// even if the worker dies before finishing the recovery lease.
func recordCreativeRecoveryDispatchTx(ctx context.Context, tx pgx.Tx, taskID pgtype.UUID) error {
	t, ok := ctx.Value(creativeRecoveryDispatchContextKey{}).(creativeRecoveryTarget)
	if !ok {
		return nil
	}
	enabled, err := creativeAutomaticRetryEnabled(ctx, tx, t.WorkspaceID)
	if err != nil {
		return err
	}
	if !enabled {
		return errors.New("creative automatic retry is paused")
	}
	if taskID.Valid {
		if _, err := tx.Exec(ctx, `UPDATE agent_task_queue SET context=context||jsonb_build_object('automatic_recovery_task_id',id::text) WHERE id=$1`, taskID); err != nil {
			return err
		}
	}
	if taskID.Valid && t.Stage == "production" {
		// Supply the original operation inputs to a fresh-session continuation;
		// local file paths or prompts must not be reinvented for an existing key.
		if _, err := tx.Exec(ctx, `UPDATE agent_task_queue SET context=context||jsonb_build_object('creative_recovery',jsonb_build_object(
 'recovery_id',$2::text,'stage','production','size_key',$5::text,
 'instructions','Read the current order first. Reuse completed outputs and receipts. For an existing image operation, reuse its exact idempotency_key, model, input_snapshot and prompt hash; use the next permitted attempt only after a confirmed failure. Never recreate conflicting frozen inputs or repeat an unknown provider request. Generate only missing sizes.',
 'operations',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',op.id,'size_key',op.size_key,
 'operation_kind',op.operation_kind,'idempotency_key',op.idempotency_key,'status',op.status,
 'model',op.model,'prompt_sha256',op.prompt_sha256,'input_snapshot',op.input_snapshot,
 'attempt',COALESCE((SELECT max(a.attempt) FROM creative_image_operation_attempt a WHERE a.operation_id=op.id),0),'result_receipt',op.result_receipt) ORDER BY op.created_at,op.id)
 FROM creative_image_operation op WHERE op.variant_id=$3 AND op.revision=$4),'[]'::jsonb))) WHERE id=$1`, taskID, t.ID, t.VariantID, t.Revision, t.Size); err != nil {
			return err
		}
	}
	var attempt int
	err = tx.QueryRow(ctx, `UPDATE creative_recovery
SET attempt=attempt+1,dispatch_count=dispatch_count+1,dispatch_failures=0,
 result_task_id=$3,source_task_id=$4,updated_at=now()
WHERE id=$1 AND status='running' AND lease_token=$2 AND lease_expires_at>now()
 AND dispatch_count<max_attempts
RETURNING attempt`, t.ID, t.LeaseToken, taskID, t.SourceTaskID).Scan(&attempt)
	if errors.Is(err, pgx.ErrNoRows) {
		return errors.New("creative recovery dispatch budget or lease changed")
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO creative_recovery_attempt
(recovery_id,attempt,lease_token,status,source_task_id,result_task_id,reason_code,completed_at)
VALUES($1,$2,$3,'queued',$4,$5,$6,now())`, t.ID, attempt, t.LeaseToken, t.SourceTaskID, taskID, t.Reason)
	return err
}
