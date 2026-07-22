CREATE INDEX CONCURRENTLY creative_material_candidate_workspace_seen_idx
    ON creative_material_candidate(workspace_id, last_seen_at DESC);
