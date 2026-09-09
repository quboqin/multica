DELETE FROM squad_workflow_assignment
WHERE stage_id NOT IN (
    'requirements', 'knowledge', 'research', 'design', 'implementation',
    'review', 'test', 'delivery', 'support'
);

DROP TABLE IF EXISTS squad_workflow_stage;

ALTER TABLE squad_workflow_assignment
    ADD CONSTRAINT squad_workflow_assignment_stage_id_check CHECK (stage_id IN (
        'requirements', 'knowledge', 'research', 'design', 'implementation',
        'review', 'test', 'delivery', 'support'
    ));
