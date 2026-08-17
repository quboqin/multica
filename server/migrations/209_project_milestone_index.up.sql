CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_project_milestone
    ON project(milestone_id);
