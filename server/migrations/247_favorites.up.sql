CREATE TABLE favorite_category (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    name         TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
    is_default   BOOLEAN NOT NULL DEFAULT false,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT favorite_category_owner_key
        UNIQUE (id, workspace_id, user_id)
);

CREATE UNIQUE INDEX idx_favorite_category_name
    ON favorite_category(workspace_id, user_id, lower(name));

CREATE UNIQUE INDEX idx_favorite_category_default
    ON favorite_category(workspace_id, user_id)
    WHERE is_default;

CREATE TABLE favorite_item (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    item_type    TEXT NOT NULL CHECK (item_type IN ('attachment', 'issue', 'project')),
    item_id      UUID NOT NULL,
    category_id  UUID NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_id, item_type, item_id),
    CONSTRAINT favorite_item_category_owner_fk
        FOREIGN KEY (category_id, workspace_id, user_id)
        REFERENCES favorite_category(id, workspace_id, user_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_favorite_item_user_created
    ON favorite_item(workspace_id, user_id, created_at DESC);

CREATE INDEX idx_favorite_item_category_created
    ON favorite_item(category_id, created_at DESC);

CREATE FUNCTION delete_favorite_items_for_source() RETURNS TRIGGER AS $$
BEGIN
    DELETE FROM favorite_item
    WHERE item_type = TG_ARGV[0] AND item_id = OLD.id;
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER delete_attachment_favorite_items
AFTER DELETE ON attachment
FOR EACH ROW EXECUTE FUNCTION delete_favorite_items_for_source('attachment');

CREATE TRIGGER delete_issue_favorite_items
AFTER DELETE ON issue
FOR EACH ROW EXECUTE FUNCTION delete_favorite_items_for_source('issue');

CREATE TRIGGER delete_project_favorite_items
AFTER DELETE ON project
FOR EACH ROW EXECUTE FUNCTION delete_favorite_items_for_source('project');
