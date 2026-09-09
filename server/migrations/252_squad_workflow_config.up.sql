CREATE TABLE squad_workflow_assignment (
    squad_id UUID NOT NULL REFERENCES squad(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    stage_id TEXT NOT NULL CHECK (stage_id IN (
        'requirements', 'knowledge', 'research', 'design', 'implementation',
        'review', 'test', 'delivery', 'support'
    )),
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (squad_id, agent_id)
);
