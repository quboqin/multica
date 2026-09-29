-- Per-task access an inviter gives an agent on the document they mention it in.
--
-- A run's task token authenticates as the owner of the runtime it executes on,
-- who is often outside a private document's audience. When a person mentions
-- an agent on such a document, the composer asks them whether the run may
-- read (and, optionally, comment on and edit) that one document. The answer
-- is stored here, keyed by the comment that triggered the run, and consulted
-- only while a task created from that comment is live. It never widens what
-- the runtime owner can do as a person, and it is capped at the granter's own
-- permission at the moment of each request.
CREATE TABLE comment_agent_grant (
    comment_id   UUID NOT NULL REFERENCES comment(id) ON DELETE CASCADE,
    agent_id     UUID NOT NULL REFERENCES agent(id) ON DELETE CASCADE,
    workspace_id UUID NOT NULL,
    issue_id     UUID NOT NULL REFERENCES issue(id) ON DELETE CASCADE,
    permission   TEXT NOT NULL CHECK (permission IN ('view','edit')),
    granted_by   UUID NOT NULL,
    granted_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (comment_id, agent_id)
);
CREATE INDEX idx_comment_agent_grant_issue ON comment_agent_grant(issue_id);
