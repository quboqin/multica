ALTER TABLE creative_issue_context
    DROP COLUMN orchestration_skill_id;

ALTER TABLE creative_material_candidate
    ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN note TEXT NOT NULL DEFAULT '',
    ADD COLUMN source_attachment_id UUID REFERENCES attachment(id) ON DELETE SET NULL;
