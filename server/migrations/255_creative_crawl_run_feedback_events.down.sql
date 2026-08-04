DROP TABLE IF EXISTS creative_order_qc_report;
DROP TABLE IF EXISTS creative_order_asset;
DROP TABLE IF EXISTS creative_order_variant;
DROP TABLE IF EXISTS creative_order_item;
DROP TABLE IF EXISTS creative_order;
DROP TABLE IF EXISTS creative_source_analysis;
DROP TABLE IF EXISTS creative_feedback_event;

DROP INDEX IF EXISTS creative_material_crawl_run_candidate_analysis_idx;
DROP TABLE IF EXISTS creative_material_crawl_run_candidate;

DROP INDEX IF EXISTS creative_material_issue_candidate_run_analysis_idx;
ALTER TABLE creative_material_issue_candidate
    DROP COLUMN IF EXISTS analyzed_at,
    DROP COLUMN IF EXISTS analysis_error,
    DROP COLUMN IF EXISTS analysis_status;

DROP INDEX IF EXISTS creative_material_crawl_run_autopilot_idx;
DROP INDEX IF EXISTS creative_material_crawl_run_workspace_created_idx;
ALTER TABLE creative_material_crawl_run
    DROP CONSTRAINT IF EXISTS creative_material_crawl_run_finished_after_started_check;
ALTER TABLE creative_material_crawl_run
    DROP CONSTRAINT IF EXISTS creative_material_crawl_run_status_check;
UPDATE creative_material_crawl_run
SET status = 'failed'
WHERE status IN ('partial', 'action_required', 'cancelled');
ALTER TABLE creative_material_crawl_run
    ADD CONSTRAINT creative_material_crawl_run_status_check
    CHECK (status IN ('queued', 'running', 'completed', 'failed'));
ALTER TABLE creative_material_crawl_run
    DROP COLUMN IF EXISTS error_message,
    DROP COLUMN IF EXISTS error_code,
    DROP COLUMN IF EXISTS finished_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS rerun_of_id,
    DROP COLUMN IF EXISTS autopilot_run_id;
