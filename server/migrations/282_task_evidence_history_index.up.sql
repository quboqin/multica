CREATE INDEX CONCURRENTLY IF NOT EXISTS agent_task_queue_evidence_history_idx
    ON agent_task_queue(trigger_evidence_kind,trigger_evidence_ref_id,created_at DESC,id DESC)
    WHERE trigger_evidence_kind IS NOT NULL AND trigger_evidence_ref_id IS NOT NULL;
