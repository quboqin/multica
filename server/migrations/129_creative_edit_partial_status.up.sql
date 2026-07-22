ALTER TABLE creative_edit_job
    DROP CONSTRAINT IF EXISTS creative_edit_job_status_check;

ALTER TABLE creative_edit_job
    ADD CONSTRAINT creative_edit_job_status_check
    CHECK (status IN ('queued', 'running', 'partial', 'completed', 'failed'));
