CREATE TABLE IF NOT EXISTS milestone (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    start_date DATE,
    end_date DATE,
    status TEXT NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'in_progress', 'paused', 'completed', 'cancelled')),
    position INTEGER NOT NULL DEFAULT 0,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE project
    ADD COLUMN IF NOT EXISTS milestone_id UUID;

ALTER TABLE issue_label
    DROP CONSTRAINT IF EXISTS issue_label_resource_type_check,
    ADD CONSTRAINT issue_label_resource_type_check
        CHECK (resource_type IN ('issue', 'agent', 'skill', 'project'));

CREATE TABLE IF NOT EXISTS project_to_label (
    project_id uuid NOT NULL,
    label_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, label_id)
);

CREATE TABLE IF NOT EXISTS kpi_metric (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL,
    name text NOT NULL,
    owner text NOT NULL DEFAULT '',
    target text NOT NULL DEFAULT '',
    current text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('on_track', 'at_risk', 'missed', 'pending')),
    note text NOT NULL DEFAULT '',
    position integer NOT NULL DEFAULT 0,
    link_type text NOT NULL DEFAULT 'none' CHECK (link_type IN ('none', 'milestone', 'project', 'issue')),
    link_id uuid,
    completion_rate integer NOT NULL DEFAULT 0 CHECK (completion_rate >= 0 AND completion_rate <= 100),
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
