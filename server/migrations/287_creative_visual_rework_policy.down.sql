CREATE OR REPLACE FUNCTION creative_task_retry_allowed(task_id uuid) RETURNS boolean
LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS (
   SELECT 1 FROM agent_task_queue t
   JOIN agent a ON a.id=t.agent_id
   JOIN creative_factory_settings s ON s.workspace_id=a.workspace_id
   WHERE t.id=task_id AND NOT s.automatic_retry_enabled
   AND t.context->>'type'='creative_domain_task'
   AND t.rerun_of_task_id IS NULL
   AND COALESCE(t.context->>'manual_retry_task_id','')<>t.id::text
   AND (
     t.retry_of_task_id IS NOT NULL
     OR t.context ? 'creative_recovery'
     OR t.context ? 'automatic_recovery_task_id'
     OR EXISTS (SELECT 1 FROM creative_recovery_attempt r WHERE r.result_task_id=t.id)
     OR (t.context->>'selection_attempt') ~ '^[2-9][0-9]*$'
     OR (t.context->>'selection_attempt') ~ '^1[0-9]+$'
     OR (t.context ? 'qc_recovery_kind' AND t.context->>'qc_recovery_kind'<>'manual_append_rerun')
     OR t.context ? 'qc_visual_rework'
   )
 );
$$;

ALTER TABLE creative_factory_settings DROP COLUMN visual_rework_enabled;
