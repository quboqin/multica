CREATE TABLE milestone (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
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

CREATE INDEX idx_milestone_workspace ON milestone(workspace_id, position, created_at);

ALTER TABLE project
    ADD COLUMN milestone_id UUID REFERENCES milestone(id) ON DELETE SET NULL;

CREATE INDEX idx_project_milestone ON project(milestone_id);

CREATE TABLE project_to_label (
    project_id uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,
    label_id uuid NOT NULL REFERENCES issue_label(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, label_id)
);

CREATE INDEX project_to_label_label_id_idx ON project_to_label(label_id);

CREATE TABLE kpi_metric (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
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
    created_by uuid REFERENCES "user"(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX kpi_metric_workspace_position_idx ON kpi_metric(workspace_id, position, created_at);
CREATE INDEX kpi_metric_workspace_link_idx ON kpi_metric(workspace_id, link_type, link_id);
