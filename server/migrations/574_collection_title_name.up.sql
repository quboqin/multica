-- The first column of a table shows each record's title. Its label is per
-- table; an empty value keeps the client's localized default ("Name").
ALTER TABLE collection ADD COLUMN IF NOT EXISTS title_name TEXT NOT NULL DEFAULT '' CHECK (length(title_name) <= 32);
