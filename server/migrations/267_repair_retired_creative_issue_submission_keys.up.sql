WITH retired_issues AS (
    SELECT i.id, i.workspace_id, i.metadata->>'creative_submission_key' AS submission_key
    FROM issue AS i
    WHERE i.metadata ? 'creative_submission_key'
      AND i.metadata->>'creative_submission_key' <> ''
      AND EXISTS (
          SELECT 1
          FROM creative_order AS o
          WHERE o.issue_id = i.id
            AND o.workspace_id = i.workspace_id
            AND o.status = 'cancelled'
      )
      AND NOT EXISTS (
          SELECT 1
          FROM creative_order AS o
          WHERE o.issue_id = i.id
            AND o.workspace_id = i.workspace_id
            AND o.status <> 'cancelled'
      )
)
UPDATE issue AS i
SET metadata = jsonb_set(
    COALESCE(i.metadata, '{}'::jsonb) - 'creative_submission_key',
    '{creative_submission_history}',
    COALESCE(i.metadata->'creative_submission_history', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
        'submission_key', retired_issues.submission_key,
        'terminal_status', 'cancelled',
        'retired_by_migration', true
    ))
)
FROM retired_issues
WHERE i.id = retired_issues.id
  AND i.workspace_id = retired_issues.workspace_id;
