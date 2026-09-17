ALTER TABLE issue ADD COLUMN kind TEXT NOT NULL DEFAULT 'task';
ALTER TABLE issue ADD CONSTRAINT issue_kind_check CHECK (kind IN ('task', 'doc', 'knowledge', 'workflow_run')) NOT VALID;
