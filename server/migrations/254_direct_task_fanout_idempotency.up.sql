-- Native direct-task fanout deduplicates only active work. Domain callers
-- distinguish revisions through their evidence or item_key/context; terminal
-- tasks never get reused.
CREATE UNIQUE INDEX idx_one_active_direct_task_per_evidence_item
    ON agent_task_queue (
        agent_id,
        trigger_evidence_kind,
        trigger_evidence_ref_id,
        (context ->> 'item_key')
    )
    WHERE status IN ('queued', 'dispatched', 'running', 'waiting_local_directory')
      AND issue_id IS NULL
      AND chat_session_id IS NULL
      AND autopilot_run_id IS NULL
      AND COALESCE(context ->> 'type', '') <> 'quick_create'
      AND context ? 'item_key';
