CREATE INDEX CONCURRENTLY IF NOT EXISTS kpi_metric_workspace_position_idx
    ON kpi_metric(workspace_id, position, created_at);
