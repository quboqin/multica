CREATE INDEX IF NOT EXISTS creative_order_workspace_created_idx
    ON creative_order(workspace_id, created_at DESC, id DESC);
