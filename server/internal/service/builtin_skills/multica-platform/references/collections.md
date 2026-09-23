# Tables (collections) and rows (records)

A table — `collection` in the CLI and API — holds rows of the workspace's own
data: a customer list, a request pool, a release checklist. A row — `record` —
is a title plus one value per field. Rows are **not** issues: no key, no status,
no assignee, no comments, no inbox entry, and writing one never starts a run.

- [Core model](#core-model)
- [What a run may do](#what-a-run-may-do)
- [CLI: read the table first](#cli-read-the-table-first)
- [CLI: tables and fields](#cli-tables-and-fields)
- [CLI: rows](#cli-rows)
- [Cell values by field type](#cell-values-by-field-type)
- [Concurrent writers](#concurrent-writers)
- [Relations: links to issues and to other rows](#relations-links-to-issues-and-to-other-rows)
- [Reading a lot of rows](#reading-a-lot-of-rows)
- [Side effects](#side-effects)
- [When something looks wrong](#when-something-looks-wrong)

## Core model

- A table has its own field catalog, up to 50 fields, separate from the
  workspace's issue properties. Field types: `text`, `number`, `select`,
  `multi_select`, `date`, `checkbox`, `url`, `actor`, `multi_actor`,
  `relation`.
- The first column of every table is the row's **title**. It is not a field, and
  it is written with `--title`.
- A `relation` field links a row to issues, or to the rows of one other table.
  Which one is fixed when the field is created.
- A row needs a discussion, an owner or a run? That is an issue. Create the
  issue and link the row to it; do not try to make the row behave like one.

Everything is addressed by name where a name exists: tables by name, id or id
prefix; fields by name or id; select options and members by name; rows by id or
**exact** title. A row title is not unique — when two rows share one, the
command lists both ids and stops rather than picking.

## What a run may do

A run acts for the person who owns its runtime, and on tables it may do exactly
what that person may do — rows and structure alike.

| | Who |
|---|---|
| List tables, read fields, rows and reverse links | the owner and people with view or edit access |
| Add, change, delete and restore rows; link/unlink; change fields or table metadata | the owner and people with edit access |
| Create a table | every workspace member and every run |
| Archive or restore a table | its owner |
| Change sharing | the human owner only |

Tables start private to their creator. A table a run creates belongs to its
runtime's owner. Sharing can grant view or edit access to named members, a
project, or the workspace. Project audiences currently include workspace
members, just as document sharing does. Workspace administrative roles do not
bypass table sharing. A missing share reads as not found; read-only access
refuses writes. Ask the owner for access; do not try another route.

## CLI: read the table first

```bash
multica collection list --output json
multica collection get <table> --output json      # field names, types, select options, relation targets
multica collection field list <table>
```

`collection get` is the contract for every write that follows: a `--set` names
a field that exists, with a value its type accepts. Guessing a field or an
option name costs a failed command each time; reading costs one.

## CLI: tables and fields

```bash
multica collection create --name "<name>" --output json                   # --project <project> for a project's table
multica collection update <table> --name "<new name>" --title-name "<label of the first column>"
multica collection field add <table> --name "Stage" --type select --option "Lead:#6b7280" --option "Won:#22c55e"
multica collection field add <table> --name "Implementation" --type relation --relation-to issues
multica collection field add <table> --name "Customer" --type relation --relation-to "<other table>"
multica collection field update <table> <field> --add-option "Churned:#ef4444"
multica collection field update <table> <field> --name "<new name>"
multica collection field archive <table> <field>
multica collection archive <table>
```

A new table has one column, the row title. Look before you create: a table the
team already keeps is where the rows belong, and a second one with a similar
name splits the data.

Adding is safe — a table, a field, an option with `--add-option` — and changes
nothing that is already there. Everything else touches other people's data, and
some changes cannot be undone:

- `--option` **replaces** the whole option list. An option left out is removed
  from every row that held it, trashed rows included. To add an option, use
  `--add-option`; keep `--option` for reordering, recoloring, or a removal the
  task asks for. It cannot rename: options are matched by name, so a new name is
  a new option and the old one goes, with its values. Renaming is done in the
  app.
- `--type` converts only along safe paths: `select` ↔ `multi_select`, `actor` ↔
  `multi_actor`, and `number` / `date` / `url` / `checkbox` → `text`. Going from
  a multi type to its single type keeps the **first** value of each cell and
  drops the rest. Any other conversion is refused: add a new field instead.
- A relation's target is fixed when the field is created.
- An archived field disappears everywhere. Its values are kept in storage, but
  restore it with `collection field restore <table> <field>`. A live field with the same name or the 50-field cap can block restoration.
- An archived table leaves every list, its rows can no longer be opened, and
  links pointing at them read as deleted. Use `collection list --archived` and `collection restore <table>` to bring it back.

So remove, convert and archive only when the task says to, in those words — not
to tidy up — and say in your comment exactly what you changed.

## CLI: rows

```bash
multica record list <table> --output json
multica record list <table> --search "<text in title>" --filter "Stage=Triage" --sort Due --limit 100 --output json
multica record get <table> <row> --output json
multica record create <table> --title "<title>" --set "Stage=Triage" --set "Seats=25" --output json
multica record update <table> <row> --set "Stage=Scheduled" --unset "Due" --title "<new title>" --output json
multica record delete <table> <row>               # to the table's trash, restorable for 30 days
multica record trash <table> --output json
multica record restore <table> <row>
```

JSON output prints a row's cells under `values`, keyed by field name and spelled
the way `--set` takes them back — option and member names, not ids:

```json
{"id": "…", "title": "Embed live views", "revision": 7,
 "values": {"Stage": "Scheduled", "Seats": 25, "Tags": ["iOS", "Web"], "Owner": "Bohan",
            "Implementation": [{"issue": "MUL-31", "title": "Data model", "status": "in_progress"}]}}
```

An empty cell is simply absent. Add `--detail` to get `fields` rows instead —
field ids, types, stored values and link ids — when you need an id.

`--filter "Field=Value"` is repeatable: the same field twice matches either
value, different fields must all match, `"Field=__none__"` matches an empty
cell. Comparisons use `>`, `>=`, `<`, `<=` for numbers/dates and `~=` for text containment. Relation equality accepts a target name or ID; relation sorting uses the alphabetically first visible target title. Deleted/archived targets do not match or contribute sort labels.

## Cell values by field type

| Type | `--set` form |
|---|---|
| `text`, `url` | `--set "Notes=any string"` (a url must be http or https) |
| `number` | `--set "Seats=25"` |
| `date` | `--set "Renewal=2026-10-01"` (`YYYY-MM-DD`) |
| `checkbox` | `--set "Signed=true"` or `false` |
| `select` | `--set "Stage=Won"` — an existing option name |
| `multi_select` | `--set "Tags=iOS,Android"` — comma-separated option names; replaces the whole cell |
| `actor` | `--set "Owner=Bohan"` — a workspace member's name, email or id |
| `multi_actor` | `--set "Reviewers=Bohan,Jiayuan"` |
| `relation` | not a value — see [Relations](#relations-links-to-issues-and-to-other-rows) |

A `select` value must already be one of the field's options; an unknown name
fails and lists the valid ones. Prefer an option that fits. When the task needs
a new one, add it first with `collection field update <table> <field>
--add-option "<name>"`, then write the cell.
An `actor` cell holds members only — never an agent or a squad — and writing one
notifies and runs nobody.

`--unset "Field"` clears a cell. `--set "Field="` is refused, so a value lost to
a shell quoting mistake cannot clear a cell by accident.

## Concurrent writers

Each cell is written on its own. `record update` with three `--set` flags is
three writes applied in order; two writers changing **different** cells of one
row never collide.

When a write fails part-way, the earlier ones stay written and the command says
which: `Already written before this failure: Seats, Stage`. Do not re-run the
whole command blindly — re-read the row and send what is still missing.

To change a cell only if nobody else changed it since you read it, name what you
read:

```bash
multica record update Requests <row> --set "Stage=Shipped" --expect "Stage=Scheduled"
multica record update Requests <row> --set "Owner=Bohan" --expect "Owner="     # only while still empty
```

A refused guarded write (`field changed; reload and retry`) means exactly that:
read the row again and decide again. Use `--expect` for claim-style writes —
taking an unowned row, advancing a stage — where two runs could race. A plain
`--set` is last-writer-wins, which is fine for a value only you maintain.

`--title` is guarded automatically: the rename is refused if someone retitled
the row after the command read it.

## Relations: links to issues and to other rows

A relation cell holds links, not a value, and has its own commands:

```bash
multica record link <table> <row> --field "Implementation" --to MUL-31 --to MUL-60
multica record link <table> <row> --field "Customer" --to "ACME"            # a row of the field's target table, by exact title or id
multica record unlink <table> <row> --field "Implementation" --to MUL-60
multica record backlinks <table> <row> --output json     # rows elsewhere that link to this row
multica issue records <issue> --output json              # rows that link to an issue
```

- What `--to` accepts follows the field: an issue key or id when it points at
  issues, a row id or exact title when it points at a table. A link to an issue
  must be a task — not a document.
- Linking twice is a no-op. A cell holds at most 50 links.
- A link survives its target. When the issue is deleted, or the target row is
  trashed or its table archived, the link reads
  `{"deleted": true, "link_id": "…"}` and stays until someone removes it:
  `multica record unlink … --to <link_id>`. Do not clean these up unasked — a
  dead link is a record that something was there.
- Links cannot be filtered or sorted on yet. To find the rows for an issue, ask
  from the issue's side with `multica issue records`.

The usual pattern when a row needs work done: create the issue, then link the
row to it, so the table shows the work's status and the issue shows where it
came from.

```bash
multica issue create --title "<title>" --output json          # note the identifier
multica record link <table> <row> --field "<relation to issues>" --to <identifier>
```

If the table has no relation field that points at issues, add one first:
`multica collection field add <table> --name "<name>" --type relation
--relation-to issues`.

## Reading a lot of rows

`record list` returns at most 100 rows per call, newest first. When more match,
the JSON carries `next_cursor`; pass it to `--cursor` with the **same** filters
and sort to continue. `total` is the exact number of matching rows.

Narrow before you page: `--search` and `--filter` run on the server and cost
nothing extra, while paging a large table through your context does. Read the
few rows the task is about, not the table.

## Side effects

- Row, link and structure writes appear immediately for everyone with the table
  open, and inside every document that embeds a view of it.
- No write here notifies anyone or starts a run — including `actor` cells and
  links to issues. Linking a row to an issue does not touch the issue.
- `record delete` is a soft delete: the row can be restored for 30 days, and not
  after.

## When something looks wrong

| Symptom | Cause |
|---|---|
| `collection is read-only` | Ask the table owner to grant edit access to your runtime owner. |
| `option "X" not found … valid options: …` | Select values must be existing options. Pick one, or add it with `--add-option`. |
| `field "X" already has an option named "Y"` | `--add-option` found it there already. Nothing was changed; write the cell. |
| `a relation cell holds links, not a value` | Use `record link` / `record unlink`, not `--set`. |
| `field changed; reload and retry` | A guarded write lost the race, or the title changed under a rename. Re-read, decide again. |
| `N rows … are titled "X"; pass the id instead` | Titles are not unique. Use the id from `record list`. |
| `row … not found in this table; it may be in the trash` | It was deleted. `multica record trash <table>`, then `record restore`. |
| `a relation cell holds at most 50 links` | The cap is per cell. |
| A table is missing from `collection list` | Check `collection list --archived`; restore with `collection restore <table>` when requested. |

## CSV and atomic batches

Use these commands only for the rows or import the task explicitly requests.
Never broaden a selection just because a batch accepts more rows.

```bash
multica record import Customers --file customers.csv --dry-run
multica record import Customers --file customers.csv
multica record batch-update Customers --record <id1> --record <id2> --set "Stage=Won"
multica record batch-update Customers --record <id1> --set "Seats=25" --expect-revision "<id1>=3"
multica record batch-delete Customers --record <id1> --record <id2> --yes
multica record batch-restore Customers --record <id1> --record <id2>
multica collection list --archived
multica collection restore Customers
multica collection field list Customers --archived
multica collection field restore Customers Notes
multica collection update Customers --icon "📋" --description "Customer follow-up"
multica record list Customers --filter "Seats>=10" --sort Stage
```

CSV uses UTF-8 and at most 10,000 rows per file. Headers are `title` (or the
table's title-column name) and existing field names or IDs. Select values accept
option names/IDs; multi-select and multi-actor cells contain JSON string arrays
with ordinary CSV quote escaping. Actors use `member:USER_ID`; they remain
member-only. Empty cells remain unset. Relation edges use `record link` rather
than value cells. An invalid row aborts the entire import and reports its row
and column; `--dry-run` never writes. Import creates new rows; it is not an upsert.

Batches accept 1–500 explicit rows. Updates preserve unrelated fields; every
row is written or none are. `--expect-revision` must cover all selected rows
when used. A conflict aborts the batch: read again and explicitly decide whether
to retry. A response count confirms the committed number of rows. Delete
requires `--yes` and keeps rows in the trash for 30 days. Restore works only
within that retention window. No batch starts an agent run.

## Formula fields

A formula computes one read-only value per row on the server. Create or edit it
with the same table-manager permission as other fields:

```bash
multica collection field add Orders --name Total --type formula --expression '{Quantity} * {Price}'
multica collection field update Orders Total --expression 'ROUND({Quantity} * {Price}, 2)'
```

Use `{field name}` (or `{field UUID}`), `{title}` for the first column, double-quoted
strings, and `==` for equality. References bind to IDs and survive renaming.
Supported functions: IF, IFERROR, AND, OR, NOT, SUM, AVG, MIN, MAX, ROUND, ABS,
CEIL, FLOOR, CONCAT, LEN, LOWER, UPPER, TRIM, ISBLANK, COALESCE, VALUE. SUM/AVG
operate on arguments from the same row, not a whole column. Blank numeric inputs
count as zero. Use VALUE for explicit text-to-number conversion.

Record reads return formula results with other cells, including #REF!, #DIV/0!,
#VALUE! or #NUM! for failing formulas. Never write or unset a formula cell, or
include it in a CSV import. Change its inputs or its expression instead. Formula
sort/filter/group, cross-record rollup and Excel formula translation are not supported.
