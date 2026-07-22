DROP TABLE IF EXISTS kpi_metric;
DROP TABLE IF EXISTS project_to_label;

DELETE FROM issue_label
WHERE resource_type = 'project';

ALTER TABLE issue_label
    DROP CONSTRAINT IF EXISTS issue_label_resource_type_check,
    ADD CONSTRAINT issue_label_resource_type_check
        CHECK (resource_type IN ('issue', 'agent', 'skill'));

ALTER TABLE project
    DROP COLUMN IF EXISTS milestone_id;

DROP TABLE IF EXISTS milestone;
