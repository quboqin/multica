ALTER TABLE creative_material_candidate
    DROP COLUMN source_attachment_id,
    DROP COLUMN note,
    DROP COLUMN tags;

ALTER TABLE creative_issue_context
    ADD COLUMN orchestration_skill_id UUID REFERENCES skill(id) ON DELETE RESTRICT;

UPDATE creative_issue_context
SET orchestration_skill_id = NULLIF(snapshot->'orchestration_skill'->>'id', '')::uuid
WHERE snapshot ? 'orchestration_skill';
