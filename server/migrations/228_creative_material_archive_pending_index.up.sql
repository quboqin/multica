CREATE INDEX CONCURRENTLY creative_material_archive_pending_idx
    ON creative_material_candidate(next_archive_at, created_at)
    WHERE archive_status IN ('pending', 'running');
