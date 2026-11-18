-- Project-scoped shares follow the project audience, including private membership.
CREATE OR REPLACE FUNCTION document_can_read(doc_id UUID, reader_id UUID) RETURNS BOOLEAN
LANGUAGE SQL STABLE AS $$
    SELECT EXISTS (
        SELECT 1 FROM document_access d
        JOIN member m ON m.workspace_id=d.workspace_id AND m.user_id=reader_id
        WHERE d.issue_id=doc_id AND (d.owner_id=reader_id OR d.scope='workspace'
            OR (d.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=d.project_id AND p.workspace_id=d.workspace_id AND (p.visibility='workspace' OR EXISTS (SELECT 1 FROM project_member pm WHERE pm.project_id=p.id AND pm.workspace_id=p.workspace_id AND pm.user_id=reader_id))))
            OR EXISTS (SELECT 1 FROM document_collaborator c WHERE c.issue_id=d.issue_id AND c.workspace_id=d.workspace_id AND c.user_id=reader_id))
    );
$$;

CREATE OR REPLACE FUNCTION collection_can_read(table_id UUID, reader_id UUID) RETURNS BOOLEAN
LANGUAGE SQL STABLE AS $$
 SELECT EXISTS (
  SELECT 1 FROM collection_access a JOIN member m ON m.workspace_id=a.workspace_id AND m.user_id=reader_id
  WHERE a.collection_id=table_id AND (a.owner_id=reader_id OR a.scope='workspace'
   OR (a.scope='project' AND EXISTS (SELECT 1 FROM project p WHERE p.id=a.project_id AND p.workspace_id=a.workspace_id AND (p.visibility='workspace' OR EXISTS (SELECT 1 FROM project_member pm WHERE pm.project_id=p.id AND pm.workspace_id=p.workspace_id AND pm.user_id=reader_id))))
   OR EXISTS (SELECT 1 FROM collection_collaborator c WHERE c.collection_id=a.collection_id AND c.workspace_id=a.workspace_id AND c.user_id=reader_id))
 );
$$;
