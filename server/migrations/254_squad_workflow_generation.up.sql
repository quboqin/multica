ALTER TABLE squad_workflow_stage
    ADD COLUMN keywords TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN baseline_position INTEGER;

UPDATE squad_workflow_stage
SET baseline_position = position
WHERE baseline_position IS NULL;

ALTER TABLE squad_workflow_stage
    ALTER COLUMN baseline_position SET NOT NULL;

ALTER TABLE squad_workflow_assignment
    ADD COLUMN source TEXT NOT NULL DEFAULT 'manual'
        CHECK (source IN ('generated', 'manual'));

CREATE TABLE squad_workflow_config (
    squad_id UUID PRIMARY KEY REFERENCES squad(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT 'legacy_default'
        CHECK (source IN ('legacy_default', 'domain_generated')),
    profile TEXT,
    generated_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO squad_workflow_config (squad_id, source)
SELECT id, 'legacy_default'
FROM squad
ON CONFLICT (squad_id) DO NOTHING;
