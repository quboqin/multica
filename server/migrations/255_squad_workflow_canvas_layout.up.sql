ALTER TABLE squad_workflow_config
    ADD COLUMN canvas_layout JSONB NOT NULL DEFAULT '{"stages":{},"agents":{}}'::jsonb;
