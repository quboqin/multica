# Document table E2E

Use a dedicated checkout with Node 22, Go, PostgreSQL and the repository
dependencies installed (`pnpm install --frozen-lockfile`). Install Chromium with
`pnpm exec playwright install chromium` if it is not already available.

```sh
pnpm test:e2e:documents
```

The runner uses the checkout's isolated `make up` environment and database,
explicitly starts the API with `FF_CORTEX_DOCS=true`, and restarts an existing
API/Web pair so a healthy API with the flag disabled cannot be silently reused.
It stops API/Web on exit, retaining the database. Do not run it against a
checkout whose API/Web another person is using.

The browser test asserts `feature_flags.cortex_docs === true` on the actual
`/api/config` response received by the page before creating a document. If a
local `.env` or `.env.worktree` explicitly overrides `FF_CORTEX_DOCS`, set that
entry to `true` for this dedicated test environment. A failed precondition is
a failure, never a skipped test. API calls and storage are real; no document
responses are mocked.

The test creates a task and a document, edits the document title and body,
saves, reloads and reopens it, then verifies task and document sources through
the shared TableView. Screenshots are written to `test-results/document-table.png`
and `test-results/document-editor.png`.

Run the independent disabled-flag compatibility regression against the same
isolated database (the handler fixture explicitly turns the flag off):

```sh
make env-exec ARGS='-- bash scripts/go-test-with-agent-cli-guard.sh -- go -C server test ./internal/handler -run TestDocumentDisabledReadContract -count=1 -v'
```

That regression keeps authorized GET/HEAD/list reads available and rejects
document create/update/delete/table queries while the flag is off. Browser E2E
does not replace this API compatibility test, native Desktop acceptance, or
the Stage migration rehearsal.
