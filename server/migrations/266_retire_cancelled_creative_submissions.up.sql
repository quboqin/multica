WITH retired_orders AS (
    UPDATE creative_order AS o
    SET submission_key = ''
    FROM (
        SELECT id, workspace_id, issue_id, submission_key
        FROM creative_order
        WHERE status = 'cancelled'
          AND submission_key <> ''
    ) AS prior
    WHERE o.id = prior.id
    RETURNING prior.workspace_id, prior.issue_id, prior.id, prior.submission_key
)
UPDATE issue AS i
SET metadata = jsonb_set(
    COALESCE(i.metadata, '{}'::jsonb) - 'creative_submission_key',
    '{creative_submission_history}',
    COALESCE(i.metadata->'creative_submission_history', '[]'::jsonb) || jsonb_build_array(jsonb_build_object(
        'submission_key', retired_orders.submission_key,
        'creative_order_id', retired_orders.id::text,
        'terminal_status', 'cancelled',
        'retired_by_migration', true
    ))
)
FROM retired_orders
WHERE i.id = retired_orders.issue_id
  AND i.workspace_id = retired_orders.workspace_id
  AND i.metadata->>'creative_submission_key' = retired_orders.submission_key;
