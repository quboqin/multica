package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// GetCreativeTaskResourceContext resolves the nearest pinned creative context
// and the per-image creative brief for any descendant task issue.
func (q *Queries) GetCreativeTaskResourceContext(ctx context.Context, issueID pgtype.UUID) ([]byte, error) {
	var raw string
	err := q.db.QueryRow(ctx, `
WITH RECURSIVE lineage AS (
  SELECT id, parent_issue_id, 0 AS depth
  FROM issue
  WHERE id = $1
  UNION ALL
  SELECT parent.id, parent.parent_issue_id, lineage.depth + 1
  FROM issue parent
  JOIN lineage ON parent.id = lineage.parent_issue_id
), context_root AS (
  SELECT context.*
  FROM lineage
  JOIN creative_issue_context context ON context.issue_id = lineage.id
  ORDER BY lineage.depth
  LIMIT 1
), selected_item AS (
  SELECT item.*
  FROM lineage
  JOIN creative_issue_item item ON item.work_issue_id = lineage.id
  WHERE item.issue_id = (SELECT issue_id FROM context_root)
  ORDER BY lineage.depth
  LIMIT 1
)
SELECT jsonb_build_object(
  'parent_issue_id', context_root.issue_id::text,
  'pinned_resources', context_root.snapshot,
  'selected_item', COALESCE((
    SELECT jsonb_build_object(
      'candidate_id', candidate_id::text,
      'creative_brief', creative_brief,
      'work_issue_id', work_issue_id::text,
      'revision', revision,
      'status', status
    )
    FROM selected_item
  ), 'null'::jsonb)
)::text
FROM context_root
`, issueID).Scan(&raw)
	return []byte(raw), err
}
