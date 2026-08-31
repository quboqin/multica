-- Candidate archives are durable platform objects. Older archive workers wrote
-- their URLs but did not create an attachment identity, leaving the order flow
-- dependent on a submit-time repair. Backfill the immutable references once.
WITH pending AS (
    SELECT
        c.id AS candidate_id,
        c.workspace_id,
        c.archived_url,
        c.asset_type,
        (
            SELECT m.user_id
            FROM member AS m
            WHERE m.workspace_id = c.workspace_id
            ORDER BY CASE m.role WHEN 'owner' THEN 0 WHEN 'admin' THEN 1 ELSE 2 END, m.created_at
            LIMIT 1
        ) AS uploader_id
    FROM creative_material_candidate AS c
    WHERE c.source_attachment_id IS NULL
      AND c.archive_status = 'completed'
      AND c.archived_url <> ''
), inserted AS (
    INSERT INTO attachment (workspace_id, uploader_type, uploader_id, filename, url, content_type, size_bytes)
    SELECT
        p.workspace_id,
        'member',
        p.uploader_id,
        COALESCE(NULLIF(regexp_replace(split_part(p.archived_url, '?', 1), '^.*/', ''), ''), CASE WHEN p.asset_type = 'video' THEN 'source.mp4' ELSE 'source.jpg' END),
        p.archived_url,
        CASE
            WHEN lower(split_part(p.archived_url, '?', 1)) ~ '\.png$' THEN 'image/png'
            WHEN lower(split_part(p.archived_url, '?', 1)) ~ '\.jpe?g$' THEN 'image/jpeg'
            WHEN lower(split_part(p.archived_url, '?', 1)) ~ '\.webp$' THEN 'image/webp'
            WHEN lower(split_part(p.archived_url, '?', 1)) ~ '\.gif$' THEN 'image/gif'
            WHEN lower(split_part(p.archived_url, '?', 1)) ~ '\.mp4$' THEN 'video/mp4'
            WHEN p.asset_type = 'video' THEN 'video/mp4'
            ELSE 'image/jpeg'
        END,
        0
    FROM pending AS p
    WHERE p.uploader_id IS NOT NULL
    RETURNING id, workspace_id, url
)
UPDATE creative_material_candidate AS c
SET source_attachment_id = inserted.id, updated_at = now()
FROM inserted
WHERE c.workspace_id = inserted.workspace_id
  AND c.archived_url = inserted.url
  AND c.source_attachment_id IS NULL;
