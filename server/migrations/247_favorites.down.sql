DROP TRIGGER IF EXISTS delete_project_favorite_items ON project;
DROP TRIGGER IF EXISTS delete_issue_favorite_items ON issue;
DROP TRIGGER IF EXISTS delete_attachment_favorite_items ON attachment;
DROP FUNCTION IF EXISTS delete_favorite_items_for_source();
DROP TABLE IF EXISTS favorite_item;
DROP TABLE IF EXISTS favorite_category;
