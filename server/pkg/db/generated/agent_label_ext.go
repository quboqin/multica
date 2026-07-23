package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

type LabelResourceTypeParams struct {
	WorkspaceID  pgtype.UUID `json:"workspace_id"`
	ResourceType string      `json:"resource_type"`
}

type CreateLabelWithResourceTypeParams struct {
	WorkspaceID  pgtype.UUID `json:"workspace_id"`
	Name         string      `json:"name"`
	Color        string      `json:"color"`
	ResourceType string      `json:"resource_type"`
}

func (q *Queries) ListLabelsByResourceType(ctx context.Context, arg LabelResourceTypeParams) ([]IssueLabel, error) {
	rows, err := q.db.Query(ctx, `
SELECT id, workspace_id, name, color, created_at, updated_at
FROM issue_label
WHERE workspace_id = $1 AND resource_type = $2
ORDER BY LOWER(name) ASC
`, arg.WorkspaceID, arg.ResourceType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []IssueLabel{}
	for rows.Next() {
		var i IssueLabel
		if err := rows.Scan(&i.ID, &i.WorkspaceID, &i.Name, &i.Color, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (q *Queries) CreateLabelWithResourceType(ctx context.Context, arg CreateLabelWithResourceTypeParams) (IssueLabel, error) {
	row := q.db.QueryRow(ctx, `
INSERT INTO issue_label (workspace_id, name, color, resource_type)
VALUES ($1, $2, $3, $4)
RETURNING id, workspace_id, name, color, created_at, updated_at
`, arg.WorkspaceID, arg.Name, arg.Color, arg.ResourceType)
	var i IssueLabel
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.Name, &i.Color, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

type GetLabelByResourceTypeParams struct {
	ID           pgtype.UUID `json:"id"`
	WorkspaceID  pgtype.UUID `json:"workspace_id"`
	ResourceType string      `json:"resource_type"`
}

func (q *Queries) GetLabelByResourceType(ctx context.Context, arg GetLabelByResourceTypeParams) (IssueLabel, error) {
	row := q.db.QueryRow(ctx, `
SELECT id, workspace_id, name, color, created_at, updated_at
FROM issue_label
WHERE id = $1 AND workspace_id = $2 AND resource_type = $3
`, arg.ID, arg.WorkspaceID, arg.ResourceType)
	var i IssueLabel
	err := row.Scan(&i.ID, &i.WorkspaceID, &i.Name, &i.Color, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

type AgentLabelParams struct {
	AgentID     pgtype.UUID `json:"agent_id"`
	LabelID     pgtype.UUID `json:"label_id"`
	WorkspaceID pgtype.UUID `json:"workspace_id"`
}

func (q *Queries) AttachLabelToAgent(ctx context.Context, arg AgentLabelParams) error {
	_, err := q.db.Exec(ctx, `
INSERT INTO agent_to_label (agent_id, label_id)
SELECT $1::uuid, $2::uuid
WHERE EXISTS (
    SELECT 1 FROM agent a
    WHERE a.id = $1::uuid
      AND a.workspace_id = $3::uuid
)
AND EXISTS (
    SELECT 1 FROM issue_label l
    WHERE l.id = $2::uuid
      AND l.workspace_id = $3::uuid
      AND l.resource_type = 'agent'
)
ON CONFLICT DO NOTHING
`, arg.AgentID, arg.LabelID, arg.WorkspaceID)
	return err
}

func (q *Queries) DetachLabelFromAgent(ctx context.Context, arg AgentLabelParams) error {
	_, err := q.db.Exec(ctx, `
DELETE FROM agent_to_label
WHERE agent_id = $1::uuid
  AND label_id = $2::uuid
  AND EXISTS (
      SELECT 1 FROM agent a
      WHERE a.id = $1::uuid
        AND a.workspace_id = $3::uuid
  )
`, arg.AgentID, arg.LabelID, arg.WorkspaceID)
	return err
}

type ListLabelsByAgentParams struct {
	AgentID     pgtype.UUID `json:"agent_id"`
	WorkspaceID pgtype.UUID `json:"workspace_id"`
}

func (q *Queries) ListLabelsByAgent(ctx context.Context, arg ListLabelsByAgentParams) ([]IssueLabel, error) {
	rows, err := q.db.Query(ctx, `
SELECT l.id, l.workspace_id, l.name, l.color, l.created_at, l.updated_at
FROM issue_label l
JOIN agent_to_label al ON al.label_id = l.id
WHERE al.agent_id = $1::uuid
  AND l.workspace_id = $2::uuid
  AND l.resource_type = 'agent'
ORDER BY LOWER(l.name) ASC
`, arg.AgentID, arg.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []IssueLabel{}
	for rows.Next() {
		var i IssueLabel
		if err := rows.Scan(&i.ID, &i.WorkspaceID, &i.Name, &i.Color, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

type ListLabelsForAgentsParams struct {
	AgentIds    []pgtype.UUID `json:"agent_ids"`
	WorkspaceID pgtype.UUID   `json:"workspace_id"`
}

type ListLabelsForAgentsRow struct {
	AgentID pgtype.UUID `json:"agent_id"`
	IssueLabel
}

func (q *Queries) ListLabelsForAgents(ctx context.Context, arg ListLabelsForAgentsParams) ([]ListLabelsForAgentsRow, error) {
	rows, err := q.db.Query(ctx, `
SELECT al.agent_id, l.id, l.workspace_id, l.name, l.color, l.created_at, l.updated_at
FROM issue_label l
JOIN agent_to_label al ON al.label_id = l.id
WHERE al.agent_id = ANY($1::uuid[])
  AND l.workspace_id = $2::uuid
  AND l.resource_type = 'agent'
ORDER BY al.agent_id, LOWER(l.name) ASC
`, arg.AgentIds, arg.WorkspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []ListLabelsForAgentsRow{}
	for rows.Next() {
		var i ListLabelsForAgentsRow
		if err := rows.Scan(&i.AgentID, &i.ID, &i.WorkspaceID, &i.Name, &i.Color, &i.CreatedAt, &i.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
