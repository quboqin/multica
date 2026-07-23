CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS creative_material_candidate_dedupe_idx
    ON creative_material_candidate(workspace_id, connector_id, dedupe_key);
