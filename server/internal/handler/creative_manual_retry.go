package handler

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/attribution"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// createManualCreativeTaskRetry starts a new human-authorized retry budget.
// The caller must authorize the human and lock the creative scope first.
// Keep the original attempt immutable and retain lineage for output reuse.
func (h *Handler) createManualCreativeTaskRetry(ctx context.Context, tx pgx.Tx, taskID, userID pgtype.UUID) (db.AgentTaskQueue, error) {
	var eligible bool
	if err := tx.QueryRow(ctx, `SELECT status IN ('failed','completed') FROM agent_task_queue WHERE id=$1 FOR UPDATE`, taskID).Scan(&eligible); err != nil {
		return db.AgentTaskQueue{}, err
	}
	if !eligible {
		return db.AgentTaskQueue{}, pgx.ErrNoRows
	}
	parent, err := h.Queries.WithTx(tx).GetAgentTask(ctx, taskID)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	var latestID pgtype.UUID
	var latestStatus string
	if err := tx.QueryRow(ctx, `
 SELECT newer.id, newer.status FROM agent_task_queue newer JOIN agent_task_queue parent ON parent.id=$1
 WHERE newer.agent_id=parent.agent_id
 AND newer.trigger_evidence_kind IS NOT DISTINCT FROM parent.trigger_evidence_kind
 AND newer.trigger_evidence_ref_id IS NOT DISTINCT FROM parent.trigger_evidence_ref_id
 AND newer.context->>'item_key' IS NOT DISTINCT FROM parent.context->>'item_key'
 AND (newer.created_at,newer.id)>=(parent.created_at,parent.id)
 ORDER BY newer.created_at DESC,newer.id DESC LIMIT 1 FOR UPDATE OF newer
`, taskID).Scan(&latestID, &latestStatus); err != nil {
		return db.AgentTaskQueue{}, err
	}
	if latestID != parent.ID {
		if latestStatus != "cancelled" {
			return db.AgentTaskQueue{}, pgx.ErrNoRows
		}
		// The UI reports the last failure even when its continuation was
		// cancelled. A new human retry resumes from that cancelled descendant
		// so image operations still belong to the same continuation chain.
		parent, err = h.Queries.WithTx(tx).GetAgentTask(ctx, latestID)
		if err != nil {
			return db.AgentTaskQueue{}, err
		}
	}
	child, err := h.Queries.WithTx(tx).CreateAgentTask(ctx, db.CreateAgentTaskParams{
		AgentID: parent.AgentID, RuntimeID: parent.RuntimeID, IssueID: parent.IssueID,
		Priority: parent.Priority, TriggerCommentID: parent.TriggerCommentID, TriggerSummary: parent.TriggerSummary,
		ForceFreshSession: pgtype.Bool{Bool: true, Valid: true}, IsLeaderTask: pgtype.Bool{Bool: parent.IsLeaderTask, Valid: true},
		RequestingUserID: userID, OriginatorUserID: userID, AccountableUserID: userID,
		OriginatorSource: pgtype.Text{String: attribution.SourceDirectHuman.String(), Valid: true}, RerunOfTaskID: parent.ID,
		TriggerEvidenceKind: parent.TriggerEvidenceKind, TriggerEvidenceRefID: parent.TriggerEvidenceRefID,
		Context: parent.Context, RuntimeMcpOverlay: parent.RuntimeMcpOverlay, RuntimeConnectedApps: parent.RuntimeConnectedApps,
	})
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	// One explicit attempt plus one infrastructure retry, independent of the
	// exhausted parent's ceiling. Attempt numbers remain monotonic for audit.
	_, err = tx.Exec(ctx, `UPDATE agent_task_queue SET parent_task_id=$2,retry_of_task_id=$2,attempt=$3,max_attempts=$3+1 WHERE id=$1`, child.ID, parent.ID, parent.Attempt+1)
	if err != nil {
		return db.AgentTaskQueue{}, err
	}
	return h.Queries.WithTx(tx).GetAgentTask(ctx, child.ID)
}
