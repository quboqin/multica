DROP INDEX IF EXISTS kpi_metric_workspace_link_idx;
DROP INDEX IF EXISTS kpi_metric_workspace_position_idx;
DROP TABLE IF EXISTS kpi_metric;

DROP INDEX IF EXISTS project_to_label_label_id_idx;
DROP TABLE IF EXISTS project_to_label;

ALTER TABLE project
    DROP COLUMN IF EXISTS milestone_id;

DROP INDEX IF EXISTS idx_project_milestone;
DROP INDEX IF EXISTS idx_milestone_workspace;
DROP TABLE IF EXISTS milestone;
