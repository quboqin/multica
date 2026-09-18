# Document rollout and rollback

Applies to MAGI-27 migrations 478–480 and `cortex_docs`.

## Rollout

1. Keep `cortex_docs` disabled. Apply migrations before replacing backends.
   Migration 478 adds the defaulted column and a `NOT VALID` check: new writes
   are checked immediately, without scanning old rows under its exclusive lock.
   Migration 480 validates existing rows separately, using PostgreSQL's
   `SHARE UPDATE EXCLUSIVE` lock; ordinary reads and writes can continue, but
   concurrent schema maintenance can conflict. 479 builds its index concurrently.
2. Use migration connections with `lock_timeout=5s` and
   `statement_timeout=15min` (PostgreSQL connection parameters; append to the
   existing `DATABASE_URL` query). These are deployment settings, not global
   application defaults. If a lock times out, stop and inspect blockers before
   retrying the migration runner. Do not mark a failed migration as applied.
   For a failed concurrent index build, inspect index validity and follow the
   runner's invalid-index recovery instead of assuming its presence is success.
3. Upgrade **all** API servers and scheduling workers to kind-aware binaries.
   Verify old processes have drained and task queries/scheduling exclude docs.
   Only then enable `cortex_docs`. The legacy task duplicate lock key remains
   unchanged while old and new task writers overlap.
4. In Stage, use a production-sized copy with representative concurrent writes.
   Record migration duration and lock waits; exercise lock timeout/retry, query
   isolation, creation/save/refresh, and rollback with documents present before
   enabling the feature in production. Local functional tests do not replace
   this Stage rehearsal.

## Disabled feature contract

Disabling `cortex_docs` stops document creation, resource-loaded writes, deletes,
execution actions, and the document Table query. It does **not** hide existing
content: authorized GET/HEAD detail reads and `GET /api/issues?kind=doc` remain
available. Workspace access checks continue to apply. Default task lists and
scheduler queries still exclude documents. Keep a kind-aware backend running.

POST issue creation has a 2 MiB raw JSON envelope limit for all kinds, since kind
is inside the body. Document PUT has the same envelope limit. Decoded document
content is limited to 1 MiB on both endpoints. Oversized bodies return 413 and
are read only up to the envelope limit plus one byte. Existing task PUT semantics
are unchanged.

## Rollback

The normal rollback is to disable `cortex_docs` and retain the kind-aware binary,
column, index and document data. **Do not simply deploy a pre-MAGI-27 binary**:
old task lists and workers can treat preserved documents as runnable tasks.

If an old binary is mandatory, first stop writes and workers, take a verified
backup, and export/migrate every document plus related comments, attachments,
labels, activity and other dependent data using an explicitly reviewed cleanup
plan. Verify there are no non-task rows, and rehearse task query/scheduling
isolation before returning traffic. Only then roll back schema in reverse order:
480 retains the validated constraint (validation has no semantic inverse),
479 drops its index concurrently, and 478 refuses to drop kind while non-task
rows exist. The down scripts alone are not a safe data evacuation plan.

If 478 was already applied before its NOT VALID refinement, no rewrite is
needed: its constraint is already valid and 480 is harmless. Do not rerun 478
manually or change the migration ledger.
