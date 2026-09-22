CREATE TABLE document_access (
    issue_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    scope TEXT NOT NULL DEFAULT 'private' CHECK (scope IN ('private','project','workspace')),
    project_id UUID,
    scope_role TEXT NOT NULL DEFAULT 'view' CHECK (scope_role IN ('view','edit')),
    revision BIGINT NOT NULL DEFAULT 1,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((scope = 'project') = (project_id IS NOT NULL))
);

CREATE TABLE document_collaborator (
    issue_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('view','edit'))
);

CREATE TABLE document_version (
    issue_id UUID NOT NULL,
    workspace_id UUID NOT NULL,
    version BIGINT NOT NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id UUID,
    action TEXT NOT NULL CHECK (action IN ('create','edit','restore','baseline')),
    restored_from BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Existing documents start private too. Agent-authored documents belong to
-- their runtime owner; the agent owner is used if that runtime no longer exists.
INSERT INTO document_access (issue_id, workspace_id, owner_id)
SELECT i.id, i.workspace_id, CASE WHEN i.creator_type = 'member' THEN i.creator_id
    ELSE COALESCE(r.owner_id, a.owner_id, i.creator_id) END
FROM issue i LEFT JOIN agent a ON a.id=i.creator_id AND a.workspace_id=i.workspace_id
LEFT JOIN agent_runtime r ON r.id=a.runtime_id AND r.workspace_id=i.workspace_id
WHERE i.kind='doc';

-- The old publication records remain intact. Full history starts with the
-- current snapshot; do not invent past titles or attribute unknown edits.
INSERT INTO document_version (issue_id, workspace_id, version, title, body, actor_type, action)
SELECT id, workspace_id, 1, title, COALESCE(description,''), 'system', 'baseline'
FROM issue WHERE kind='doc';

CREATE FUNCTION document_can_read(doc_id UUID, reader_id UUID) RETURNS BOOLEAN
LANGUAGE SQL STABLE AS $$
    SELECT EXISTS (
        SELECT 1 FROM document_access d
        JOIN member m ON m.workspace_id=d.workspace_id AND m.user_id=reader_id
        WHERE d.issue_id=doc_id AND (d.owner_id=reader_id OR d.scope='workspace'
            OR (d.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=d.project_id AND p.workspace_id=d.workspace_id))
            OR EXISTS (SELECT 1 FROM document_collaborator c WHERE c.issue_id=d.issue_id AND c.workspace_id=d.workspace_id AND c.user_id=reader_id))
    );
$$;

CREATE FUNCTION capture_document_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    owner UUID;
    actor UUID;
    actor_kind TEXT;
    change_action TEXT;
    next_version BIGINT;
BEGIN
    IF NEW.kind <> 'doc' THEN RETURN NEW; END IF;
    IF TG_OP='INSERT' THEN
        owner := NULLIF(current_setting('multica.document_owner',true),'')::uuid;
        IF owner IS NULL AND NEW.creator_type='member' THEN owner:=NEW.creator_id; END IF;
        IF owner IS NULL THEN
            SELECT COALESCE(r.owner_id,a.owner_id) INTO owner FROM agent a
            LEFT JOIN agent_runtime r ON r.id=a.runtime_id AND r.workspace_id=a.workspace_id
            WHERE a.id=NEW.creator_id AND a.workspace_id=NEW.workspace_id;
        END IF;
        INSERT INTO document_access(issue_id,workspace_id,owner_id)
        VALUES(NEW.id,NEW.workspace_id,COALESCE(owner,NEW.creator_id));
    END IF;
    change_action := COALESCE(NULLIF(current_setting('multica.document_action',true),''),'edit');
    IF TG_OP='UPDATE' AND NEW.title IS NOT DISTINCT FROM OLD.title
       AND NEW.description IS NOT DISTINCT FROM OLD.description AND change_action <> 'restore' THEN
        RETURN NEW;
    END IF;
    actor_kind := COALESCE(NULLIF(current_setting('multica.document_actor_type',true),''),'system');
    actor := NULLIF(current_setting('multica.document_actor_id',true),'')::uuid;
    IF TG_OP='INSERT' THEN
        change_action := 'create'; actor_kind := NEW.creator_type; actor := NEW.creator_id;
    END IF;
    SELECT COALESCE(MAX(version),0)+1 INTO next_version FROM document_version WHERE issue_id=NEW.id;
    INSERT INTO document_version(issue_id,workspace_id,version,title,body,actor_type,actor_id,action,restored_from)
    VALUES(NEW.id,NEW.workspace_id,next_version,NEW.title,COALESCE(NEW.description,''),actor_kind,actor,change_action,
        NULLIF(current_setting('multica.document_restored_from',true),'')::bigint);
    RETURN NEW;
END $$;
CREATE TRIGGER document_version_capture AFTER INSERT OR UPDATE ON issue FOR EACH ROW EXECUTE FUNCTION capture_document_version();
