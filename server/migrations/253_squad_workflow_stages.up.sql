ALTER TABLE squad_workflow_assignment
    DROP CONSTRAINT IF EXISTS squad_workflow_assignment_stage_id_check;

CREATE TABLE squad_workflow_stage (
    squad_id UUID NOT NULL REFERENCES squad(id) ON DELETE CASCADE,
    id TEXT NOT NULL,
    name TEXT,
    description TEXT,
    position INTEGER NOT NULL,
    updated_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (squad_id, id)
);

CREATE INDEX squad_workflow_stage_order_idx
    ON squad_workflow_stage (squad_id, position, created_at);

INSERT INTO squad_workflow_stage (squad_id, id, position)
SELECT s.id, defaults.id, defaults.position
FROM squad s
CROSS JOIN (VALUES
    ('requirements', 0),
    ('knowledge', 1),
    ('research', 2),
    ('design', 3),
    ('implementation', 4),
    ('review', 5),
    ('test', 6),
    ('delivery', 7),
    ('support', 8)
) AS defaults(id, position)
ON CONFLICT (squad_id, id) DO NOTHING;
