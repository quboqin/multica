# Documents

A document is a page in the workspace's document tree: a title, a Markdown
body, a place in the tree, and explicit sharing settings. Underneath it is an issue of
kind `doc`, so it has a key like `MUL-12`, comments, attachments and
subscribers — but it never runs anyone, and its body is versioned.

- [Core model](#core-model)
- [CLI](#cli)
- [Saving is versioned](#saving-is-versioned)
- [Sharing and history](#sharing-and-history)
- [The tree](#the-tree)
- [Comments, mentions and search](#comments-mentions-and-search)
- [Side effects](#side-effects)
- [When something looks wrong](#when-something-looks-wrong)

## Core model

| Field               | Meaning                                                                                      |
| ------------------- | -------------------------------------------------------------------------------------------- |
| `identifier`        | The document's key, e.g. `MUL-12`. Every `<document>` argument takes the key or the full id. |
| `description`       | The body, as Markdown.                                                                       |
| `document_revision` | The body's version. Starts at 1 and goes up by one on every body save.                       |
| `status`            | `draft` when private to the owner, `published` when shared. There is no review workflow.     |
| `parent_issue_id`   | The page above it, or null at the top level.                                                 |
| `project_id`        | The project whose tree it lives in, or null for the workspace tree.                          |

Documents do not appear in `multica issue list`, which lists tasks only. Find
them with `multica document list` or `multica issue search --kind doc`.

Assigning a document to an agent does **not** start a run, and changing its
status never does either. A document is material to read and write, not work to
dispatch.

## CLI

```bash
multica document list --output json                      # the tree: keys, titles, status, revision, depth — no bodies
multica document list --project <project-id> --output json
multica document get <document> --output json             # body in "description", version in "document_revision"
multica document get <document> --output markdown > page.md   # body alone; the revision is printed to stderr
multica document create --title "<title>" --content-stdin --output json
multica document create --title "<title>" --parent <document> --content-file ./page.md --output json
multica document save <document> --expected-revision <n> --content-file ./page.md --output json
multica document save <document> --expected-revision <n> --title "<new title>" --output json
multica document move <document> --parent <document> --output json
multica document move <document> --root --first --output json
multica document move <document> --before <sibling-document> --output json
multica document share <document>                         # current sharing
multica document versions <document>                      # editors and owner
multica document version <document> <version>              # historical title and body
multica document restore <document> <version> --expected-revision <revision>
```

`--content`, `--content-stdin` and `--content-file` are mutually exclusive.
Prefer stdin or a file for anything longer than a line: an inline `--content`
decodes `\n` escapes, which mangles a body that contains literal backslashes. A
`--content-file` must sit inside your working directory.

`document create --parent` puts the page in its parent's project on its own; a
page cannot live in a different project than the page above it.

## Saving is versioned

Every body save names the revision it was written against:

1. `multica document get <document>` — note `document_revision`, say 4.
2. Edit the body you just read.
3. `multica document save <document> --expected-revision 4 ...`

The save lands only while revision 4 is still current, and the document is then
at revision 5. There is no way to save without a revision, by design.

**When the save is refused, nothing was written.** Someone else — a person in
the editor, or another run — saved first. The error names the current revision.
Do this, in this order:

1. Read the document again.
2. Merge your change into the body that is there **now**.
3. Save against the new revision.

Never retry the same body with the new number. That is exactly the overwrite the
version check exists to stop, and the other writer's work would be gone with no
trace. If you cannot tell how the two changes fit together, leave a comment on
the document instead of guessing.

A title-only save (`--title` without content) still needs the revision but does
not raise it.

## Sharing and history

Documents start private to their creator/owner, including existing documents
migrated to this model. Workspace administrators have no implicit access to
someone else's private document. Runs use their runtime owner's access; the
history still attributes their writes to the agent.

The human document owner manages sharing in Publish or with `document share`.
A direct collaborator, project audience, or workspace audience can receive
`view` or `edit`; all new grants default to `view`. Grants are additive, so a
wider read grant does not remove a collaborator's edit grant. Projects currently
have the workspace's audience; project sharing is not a private project team.

```bash
multica document share <document> --collaborator <user-uuid>:edit
multica document share <document> --scope workspace --permission view
multica document share <document> --scope project --project <project-id> --permission edit
multica document share <document> --remove-collaborator <user-uuid>
multica document share <document> --scope private
```

`--scope private` removes the broader audience, preserving direct collaborators.
Remove their grants too to return to owner-only visibility. Sharing changes
require the human owner: task tokens cannot grant or revoke access. A mention
or a document-tree move never grants access.

Editors can save, comment, inspect history and restore. Readers can read the
current page and comments. Only the owner moves or deletes a document. Sharing persists through saves and restores. The old `document status` review actions
are removed; do not try alternate credentials or direct issue status writes.

History records accepted title/body changes. Restore creates a new snapshot
without deleting later history or changing sharing. For `restore`, pass the
current **revision** from `document get`; for `save`, continue using
**document_revision**. A stale restore is rejected. Migrated documents start
with a baseline of their current content; past edit snapshots are not invented.

## The tree

`multica document list` prints the accessible tree in order, each entry with a
`depth`. It carries no bodies, so it is safe to run on a large workspace; read a
body with `document get`.

`document move` reparents and reorders in one step:

- `--parent <document>` / `--root` choose where it goes; with neither, it stays
  under its current parent and only its order changes.
- `--first` / `--before <sibling>` choose the position; with neither, it lands
  last.

The new parent must be a document with the same owner and project, and never the page
itself or one of its descendants. Moving a page moves its whole subtree with
it. The CLI does not delete documents; people do that in the app, and deleting a
page does not delete the pages under it — they move to the top level.

## Comments, mentions and search

Comments work as on any issue:

```bash
multica issue comment list <document> --roots-only --summary --compact --output json
multica issue comment add <document> --content-stdin
```

A mention written in a document's **body** never starts a run: an agent or squad
link there is a reference, whatever the body says around it. A member link in a
body does notify that member once, the way it does in an issue description. A
mention in a **comment** on a document follows the normal comment rules in
`mentions.md` — an agent mention there does enqueue a run.

```bash
multica issue search "<words>" --kind doc --output json     # documents only
multica issue search "<words>" --output json                # tasks and documents together; check "kind"
```

## Side effects

- `create`, `save` and `move` refresh editors for authorized readers only.
- A body save preserves sharing. This sharing model does not queue knowledge-base ingestion.
- Nothing here starts a run. The only notification a save can send is to a
  member newly mentioned in the body. Comments do what comments do.

## When something looks wrong

| Symptom                                                                           | Cause                                                                                         |
| --------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| `nothing was written: MUL-12 is at revision 6, and this command named revision 4` | Someone saved in between. Re-read, merge, save against 6.                                     |
| `--expected-revision is required`                                                 | A save always names the revision it read. Read the document first.                            |
| `MUL-9 is a task, not a document`                                                 | The key belongs to a task. Use `multica issue get`.                                           |
| `parent must be a document in the same project`                                   | Move or create under a page of the same project, or use `--root`.                             |
| `a document cannot be moved into its own subtree`                                 | The chosen parent is the page itself or one of its descendants.                               |
| The status will not change from a run                                             | Sharing belongs to the human document owner.                                                  |
| A document disappears                                                             | Its owner revoked access, the shared project was deleted, or your workspace membership ended. |
