DROP TABLE IF EXISTS squad_workflow_config;

ALTER TABLE squad_workflow_assignment
    DROP COLUMN IF EXISTS source;

ALTER TABLE squad_workflow_stage
    DROP COLUMN IF EXISTS baseline_position,
    DROP COLUMN IF EXISTS keywords;
