CREATE INDEX CONCURRENTLY IF NOT EXISTS kpi_metric_workspace_link_idx
    ON kpi_metric(workspace_id, link_type, link_id);
