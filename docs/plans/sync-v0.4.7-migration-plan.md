# syncV0.4.7 Migration Plan

## Background

The `syncV0.4.7` branch is based on the upstream GitHub tag `v0.4.7`.
The local `master` branch does not share a merge base with `v0.4.7`, and the
upstream branch contains thousands of commits not present in the local history.

Because of that, this migration must not use a normal merge or blindly
cherry-pick the full local history. The safe approach is to migrate local
features by intent, adapting each feature to the current `v0.4.7` code shape.

Dry-run patch checks showed that all candidate local feature commits conflict
against `v0.4.7`, so the expected workflow is manual, feature-by-feature
application with small commits.

## Migration Progress

- Completed: integration token storage/API/settings, daemon user-token env
  injection, and Git credential injection.
- Completed: Lark/Feishu joint login, identity persistence, API client support,
  and `/lark/start` web entry.
- Completed: operating plans, milestones, KPI metrics, project-plan linkage,
  project labels, related web/views/core wiring, and CLI `plan`/`kpi` commands.
- Completed: product terminology and UI adjustments, including zh-Hans
  issue/task and project/demand copy, demand default icon, sidebar ordering, and
  local/system font stacks instead of `next/font/google`.
- Completed: Codex latency/sandbox audit. The current branch already has newer
  sandbox handling, so only missing warm-cache, final-answer latency, and Next
  file tracing root behavior were migrated.

## Remaining Master Commit Audit

These local `master` commits were identified after the main migration pass and
should keep their original commit IDs for traceability when migrated manually.

- Covered by current originator model with added regression tests:
  `dc15b7bbee381050d1202e3ccd8193f0c313fccf` - `乱@人优化`.
  The current `v0.4.7` branch already resolves agent task-token requests from
  the source task's `originator_user_id` instead of treating the token owner as
  the requester. Added tests pin the behavior and avoid regressing it while
  preserving the newer daemon rerun/session logic.
- Completed: `2d621ed449632a9961a0dd9ae4b8240a5b8dc727` - `项目视图 优化`.
  Migrated the project status bucket / drag-and-drop board view by adapting it
  to the current project page and package dependency layout.
- Completed: `8eca83762220f1a462e160e7f1c1afa0f4eccda4` - `app`.
  Migrated the mobile Expo Web compatibility fallbacks for persistent storage,
  theme preference storage, markdown image lightbox, and Shiki highlighting.
- Completed: `6d0ee4fca469ced1b2017d75fb353d4f0159c70a` (`Feishu config`).
  The final `.env` from local `master` has been restored into the migration
  branch so this config commit is no longer omitted.
- Audited / covered by current `v0.4.7`: `05134cdde93293391543add9d68467d677e01153`
  (`remote-github`). This is a large upstream-sync snapshot, not a focused local
  feature commit. Spot checks confirmed current branch coverage for the
  representative product areas it touched: Lark Bot integration, Lark binding
  page, system notifications, attachment URL helpers, recent chat context,
  editor/comment trigger improvements, CodeBuddy runtime, Cursor MCP sidecars,
  desktop daemon wiring, and built-in skill docs. Where current files still
  differ from `master`, the current branch generally contains newer behavior
  such as blocked @mention warnings, extra attachment URL shapes, Cursor MCP
  auth seeding, and stricter CodeBuddy stream finalization; these should not be
  reverted to the older `master` snapshot.
- Completed: `091d5a800` (`mobile app env`). Restored the final mobile
  staging/production env files from local `master`.
- Completed: `6b891bcee` (`env and docker`). Restored `.env` and the final
  self-host compose file from local `master`, and migrated the Docker mirror
  settings while preserving current `v0.4.7` build additions.
- Completed: `9943e8795` (`private branding`). Migrated Cybertron landing
  metadata/copy and restored the favicon from local `master`.
- No local `master` commit is intentionally omitted after this pass.

Current local verification:

- `corepack pnpm exec tsc -p packages/core/tsconfig.json --noEmit` passes.
- `corepack pnpm exec tsc -p packages/ui/tsconfig.json --noEmit` passes.
- `corepack pnpm exec tsc -p packages/views/tsconfig.json --noEmit` passes.
- `corepack pnpm exec tsc -p apps/web/tsconfig.json --noEmit` passes.
- `corepack pnpm --filter @multica/mobile typecheck` passes.
- `corepack pnpm exec vitest run apps/mobile/lib/markdown/preprocess.test.ts
  apps/mobile/lib/attachment-url.test.ts apps/mobile/lib/attachment-dedup.test.ts`
  passes.
- `corepack pnpm exec vitest run projects/components/projects-page.test.tsx`
  passes from `packages/views`.
- `go test ./internal/handler -run 'TestInvokeOriginatorFromTaskToken'` passes
  from `server/`.
- Go 1.26.5 was found at `D:\dev-tools\go\bin`; migrated Go files were
  formatted with `gofmt`.
- `go test ./... -run '^$'` passes under `server/`, confirming all server
  packages compile.
- `go test ./cmd/multica -run 'Test.*(Plan|Kpi|KPI)'` passes.
- `go test ./internal/integrations/lark ./internal/integrations/channel
  ./internal/integrations/channel/engine` passes.
- `go test ./internal/handler -run 'Test.*(Config|ClientIP|Webhook|File|Contact|Agent)'`
  passes.
- PostgreSQL 17 was found at `D:\dev-tools\postgresql\17\pgsql\bin`.
  A local test database was initialized under `.tmp/postgres-data` and started
  on port 5432 with `multica/multica`.
- `go run ./cmd/migrate up` passes against the local test database and applied
  migrations through `212_kpi_metric_workspace_link_index`.
- `go test ./cmd/migrate` passes.
- `go test ./internal/migrations` passes.
- All locale JSON files parse.
- `git diff --check` reports only CRLF normalization warnings.
- `http://127.0.0.1:3000/` and `http://127.0.0.1:3000/lark/start` return 200
  after Next dev compilation.
- The current branch builds a server binary successfully at
  `.tmp/server-current-sync.exe`.
- The current server binary starts on port 8080 against the local test database.
  `http://127.0.0.1:8080/api/config` and `http://127.0.0.1:8080/health`
  return 200.

Local verification blockers:

- `sqlc`, `make`, `docker`, `docker compose`, and `bash` were not found in PATH
  or under `D:\dev-tools`, so the normal Makefile/Docker Compose flow could not
  be run here.
- A fallback `go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate`
  attempt downloaded the module but did not finish within several minutes and
  was stopped. Generated DB code still compiles and the migrations apply cleanly,
  but SQLC regeneration was not completed in this environment.
- Full `go test ./...` runs but has Windows-environment test failures unrelated
  to the migrated code path, including missing `sh`, long temp paths, and tests
  expecting POSIX-style paths. Use the compile check and targeted tests above as
  the local verification signal on this machine.
- `corepack pnpm typecheck` still fails in Turbo with `Unable to find package
  manager binary`, while direct package-level TypeScript checks pass.

## Non-Goals

- Do not omit local `master` commits. Repository initialization content from
  `c1bf788a0 init` is covered by the `v0.4.7` base repository, and the later
  Cybertron branding part from `9943e8795 init` has been migrated explicitly.
- Do not overwrite `v0.4.7` files with old snapshots from local `master`.
- Do not preserve duplicate early versions of the same local feature.
- Do not copy old migration numbers directly when `v0.4.7` already has newer
  migrations.

## Migration Order

### 1. Base Data And API Capabilities

Candidate commits:

- `e128ff445` - account integration token management
- `1bb4b0d4d` - account integration token follow-up
- `7deeec52e` - token support
- `41c2c9f71` - dynamic integration tokens

Scope:

- User integration token storage and API handling.
- Agent task requesting-user support.
- Frontend account settings token management.
- Runtime/profile token consumption.

Notes:

- Local migrations `113_user_integration_tokens` and
  `114_agent_task_requesting_user` must be renumbered after the current highest
  `v0.4.7` migration.
- At the time this plan was written, `v0.4.7` already had migrations through
  `202_runtime_profile_add_qwen`, so new migrations should start at `203+`.
- Run `make sqlc` after changing SQL queries or migrations that require
  generated DB code.

### 2. Git Token And Daemon Repository Authentication

Candidate commits:

- `de103917c` - git token
- `a9a9db150` - git-token
- `9564db713` - git-token-user
- Relevant missing pieces from `05134cdde` - remote-github

Scope:

- Host/user-scoped Git token selection.
- Daemon credential injection for repo clone/fetch.
- Repo cache authentication behavior.
- Any CLI or server-side token plumbing still missing from `v0.4.7`.

Notes:

- `v0.4.7` already includes significant repository and daemon repo-cache logic,
  including `multica repo` commands. Do not replace these with older local
  versions.
- Treat `05134cdde` as an audit source, not a commit to apply wholesale.

### 3. Lark/Feishu Joint Login

Candidate commits:

- `20e1b28ab` - Feishu joint login
- `7bec49cee` - Feishu joint login follow-up

Scope:

- Web `/lark/start` login entry.
- Lark login identity persistence.
- Server handler and integration login service.
- Core API client/schema additions.

Notes:

- `v0.4.7` already includes extensive Lark Bot/channel integration. The
  migration should add login/binding behavior without regressing existing Lark
  channel flows.
- Local migration `120_lark_login_identity` must be renumbered to the next
  available migration number.

### 4. Milestones, KPIs, And Operating Plans

Candidate commits:

- `2a873855c` - milestones and KPI board
- `b37e07254` - CLI plan and KPI commands with project plan linking

Scope:

- Operating plan, milestone, and KPI data model.
- Server handlers and routes.
- `packages/core` queries, mutations, and types.
- Shared `packages/views` plan/KPI pages.
- Web and desktop routing.
- CLI `plan` and `kpi` commands.

Notes:

- Local migration `115_operating_plans` must be renumbered after all earlier
  migrated local migrations.
- Run `make sqlc` after adapting SQL.
- Keep package boundaries intact: shared pages live in `packages/views`, state
  and API logic live in `packages/core`, app-specific route wiring stays in the
  apps.

### 5. Product Copy And UI Adjustments

Candidate commits:

- `f7597bd1e` - issue to demand terminology
- `0c433606f` - demand to task terminology
- `a5a0e6207` - project to demand and plan linkage
- `a67a5c948` - remove remote font dependency
- `1ff2bc5a9` - default demand icon

Scope:

- Chinese terminology updates.
- Locale updates for demand/task/project/plan wording.
- Sidebar/search/project page copy and linkage updates.
- Remote font removal.
- Project/demand default icon adjustment.

Notes:

- Do this after feature migration so new plan/KPI locale files and UI surfaces
  can be adjusted consistently.
- Follow the conventions in `apps/docs/content/docs/developers/conventions.mdx`
  and `apps/docs/content/docs/developers/conventions.zh.mdx` for naming,
  translations, and Chinese product voice.

### 6. Codex Latency And Sandbox Changes

Candidate commits:

- `422398538` - Codex chat latency fix
- `e3d1b6299` - codex_sandbox model

Scope:

- Codex daemon/runtime behavior that is still missing from `v0.4.7`.
- `server/pkg/agent/codex.go`.
- `server/internal/daemon/execenv/codex_*`.
- Related daemon runtime config behavior.

Notes:

- This area overlaps heavily with upstream `v0.4.7` Codex work. Do not apply
  either commit wholesale.
- Compare behavior file-by-file and only migrate missing local behavior.

## Suggested Workflow

1. Start from `syncV0.4.7`.
2. Work through the sections above in order.
3. For each section, inspect the candidate commit diff and manually adapt the
   intended behavior to the current `v0.4.7` implementation.
4. Commit each section separately using conventional commit messages.
5. Run targeted checks after each section.
6. Run `make check` after all sections are complete.

## Verification Targets

- Run `make sqlc` whenever SQL query files or DB model migrations require
  regenerated code.
- Run relevant Go tests for touched server packages, for example
  `cd server && go test ./internal/handler/... ./internal/daemon/...`.
- Run relevant frontend checks for touched packages, for example
  `pnpm typecheck` and targeted Vitest commands.
- Finish with `make check` before treating the migration as complete.
