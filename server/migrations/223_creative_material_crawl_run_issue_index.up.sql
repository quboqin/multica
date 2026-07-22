CREATE INDEX CONCURRENTLY creative_material_crawl_run_issue_idx
    ON creative_material_crawl_run(issue_id, created_at DESC)
    WHERE issue_id IS NOT NULL;
