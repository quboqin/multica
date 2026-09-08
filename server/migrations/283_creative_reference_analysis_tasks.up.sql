CREATE INDEX agent_task_reference_candidate_idx
ON agent_task_queue ((context->>'candidate_id'), created_at DESC)
WHERE trigger_evidence_kind = 'creative_crawl_run_analysis';
