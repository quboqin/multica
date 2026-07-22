CREATE INDEX CONCURRENTLY creative_material_issue_candidate_issue_idx
    ON creative_material_issue_candidate(issue_id, status, created_at DESC);
