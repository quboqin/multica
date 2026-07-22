# working-on-issues source map

Evidence layer for `SKILL.md`. Every contract the skill states is traced to a
current `file:line` here. Lines were re-derived against `feat/builtin-skills`
after the latest `main` merge; the prior skill cited pre-merge lines that have
since moved (see the "drifted" column). Re-confirm with the verification command
at the bottom before relying on an exact line.

## External Web preview sessions

| Behavior | File:line |
|---|---|
| Top-level `multica preview` command with `create`, `list`, and `stop` | `server/cmd/multica/cmd_preview.go:15-64` |
| Create requires `--issue` and `--url`, fixes `platform=web` and `provider=external_web`, and POSTs the issue endpoint | `server/cmd/multica/cmd_preview.go:71-119` |
| `preview device create` fixes `platform=android` and `provider=local_device`, but requires a loopback URL with exactly one `serial`; normal deployment uses `preview device sync` | `server/cmd/multica/cmd_preview.go`, `server/internal/handler/preview_session.go` |
| `preview device sync` resolves the sole online `mac-mobile-preview` host when `--url` is omitted, inherits the Issue's active device serial unless explicitly overridden, deploys the checkpoint, and stops superseded same-Issue Android sessions | `server/cmd/multica/cmd_preview_device.go`, `server/internal/previewruntime/contract.go` |
| A daemon started with `--device-runtime-url` advertises that endpoint and the fixed `mac-mobile-preview` label in runtime metadata | `server/cmd/multica/cmd_daemon.go`, `server/internal/daemon/config.go`, `server/internal/daemon/daemon.go`, `server/internal/handler/daemon.go` |
| Device Runtime accepts a per-deploy APK and HTTP(S) H5 URL, then passes the preview URL to the stage shell as an Activity string extra | `server/internal/deviceruntime/server.go`, `server/internal/deviceruntime/adb.go` |
| Device deployments expose cancellable progress jobs and skip APK installation when the serial already has the same SHA-256 artifact | `server/internal/deviceruntime/deployment.go`, `server/internal/deviceruntime/server.go`, `server/internal/deviceruntime/console_device_bridge.go` |
| Browser calls use a short-lived Preview Session token, native Host calls use `MULTICA_DEVICE_RUNTIME_TOKEN`, and Runtime enforces a serial write lease for the current Session | `server/internal/deviceruntime/access.go`, `server/internal/deviceruntime/lease.go`, `server/cmd/multica/cmd_device.go`, `server/cmd/multica/cmd_preview_device.go` |
| Device inventory reports ADB authorization plus cached battery, charging, and temperature health | `server/internal/deviceruntime/device_health.go`, `server/internal/deviceruntime/adb.go` |
| Loopback H5 deploys create `adb reverse` automatically and device sync pins the Preview Session URL/viewer to the selected serial | `server/internal/deviceruntime/adb.go`, `server/cmd/multica/cmd_preview_device.go`, `server/internal/deviceruntime/console_webrtc.go` |
| Device Runtime discovers the application WebView through ADB forwarding, keeps one CDP connection per device, snapshots semantic DOM elements, and executes semantic actions | `server/internal/deviceruntime/webview.go` |
| Project Git repositories accept normalized `role` and `capabilities` metadata, which the runtime brief uses to require a minimal issue working set | `server/internal/handler/project_resource.go`, `server/internal/daemon/execenv/runtime_config.go` |
| Preview targets are detected from actual Git roots and project manifests, written at task setup, refreshed after daemon-managed checkout, and available manually through `multica preview detect --write` | `server/internal/previewdetect/detect.go`, `server/internal/daemon/execenv/context.go`, `server/internal/daemon/health.go`, `server/cmd/multica/cmd_preview.go` |
| Issue interaction runs derive the one active Android Runtime and serial from the pinned Preview Session and refuse missing, ambiguous, or offline bindings | `server/cmd/multica/cmd_preview_device.go:runPreviewDeviceRun`, `server/cmd/multica/cmd_preview_device.go:activeIssueDeviceTarget` |
| Issue-bound WebView snapshot exposes visible semantic elements before a scenario is generated | `server/cmd/multica/cmd_preview_device.go:runPreviewDeviceSnapshot`, `server/internal/deviceruntime/webview.go:webViewSnapshot` |
| `preview device run` routes semantic fill/tap/assert/wait_for steps to `/api/webview/action` and native tap/swipe/text/key steps to `/api/input` | `server/cmd/multica/cmd_preview_device.go` |
| Device sync and scenario execution behavior is covered by isolated Multica API and Device Runtime HTTP tests | `server/cmd/multica/cmd_preview_device_test.go` |
| `device serve` accepts the matching scrcpy Server path/version used for H.264 WebRTC video | `server/cmd/multica/cmd_device.go` |
| List resolves the issue and GETs its preview sessions | `server/cmd/multica/cmd_preview.go:121-152` |
| Stop POSTs `/api/preview-sessions/{id}/stop` | `server/cmd/multica/cmd_preview.go:154-177` |
| Issue-scoped create/list route registration | `server/cmd/server/router.go:712-713` |
| Session detail/stop route registration | `server/cmd/server/router.go:741-742` |
| Expired active sessions are projected as `expired` in API responses | `server/internal/handler/preview_session.go:43-94` |
| Create accepts external Web URLs and loopback local Android Device Runtime URLs pinned to exactly one serial | `server/internal/handler/preview_session.go` |
| Local device create/touch uses a transactional per-device lease; Issue Viewer and CLI renew it before control, and stale holders become sleeping | `server/internal/handler/preview_session.go`, `server/cmd/multica/cmd_preview_device.go`, `packages/views/issues/components/preview-sessions-section.tsx` |
| WebRTC producers stop after the last viewer grace period and idle WebView debugger connections close after 20 minutes | `server/internal/deviceruntime/webrtc.go`, `server/internal/deviceruntime/webview.go` |
| Create persists a running external URL and binds task-token calls to the current issue/task | `server/internal/handler/preview_session.go:205-249` |
| Stop is creator/admin scoped for members and issue-task scoped for agents; it changes only the Multica record | `server/internal/handler/preview_session.go:263-324` |

Phase 0 is registration, not runtime management: the request carries an
already-running external URL. `normalizeExternalPreviewURL` rejects relative
URLs, non-HTTP(S) schemes, and userinfo. The provider stored in the database is
the server constant `external_web`, even though the CLI sends the explicit
provider value as part of the public request contract.

## `multica issue pull-requests` — read PR links from Multica

| Behavior | File:line | Drifted from |
|---|---|---|
| CLI command `pull-requests <id>` (alias `prs`) | `server/cmd/multica/cmd_issue.go:105` | `:104` |
| `runIssuePullRequests` handler | `server/cmd/multica/cmd_issue.go:507` | new citation |
| Calls `GET /api/issues/<id>/pull-requests` | `server/cmd/multica/cmd_issue.go:522` | `:522` (unchanged) |
| API route registration | `server/cmd/server/router.go:480` | `:480` (unchanged) |
| Handler `ListPullRequestsForIssue` → `Queries.ListPullRequestsByIssue` | `server/internal/handler/github.go:466,471` | `:466` (unchanged) |
| Row → response mapper `issuePullRequestRowToResponse` | `server/internal/handler/github.go:149` | new citation |

The CLI resolves the issue ref, GETs the endpoint, and (for `--output json`)
prints the raw `{"pull_requests": [...]}` body. Only `--output` is accepted; the
default `table` shows `NUMBER STATE TITLE URL`.

## PR response shape

`GitHubPullRequestResponse` struct: `server/internal/handler/github.go:51`. JSON
fields the agent can read off each element of `pull_requests`:

- `number` (`json:"number"`, line 56)
- `html_url` (`json:"html_url"`, line 59)
- `title` (`json:"title"`, line 57)
- `state` (`json:"state"`, line 58) — the folded lifecycle enum (see below)
- `merged_at` (`json:"merged_at"`, line 63), `closed_at` (line 64)
- `mergeable_state` (`json:"mergeable_state"`, line 70) — mirrors GitHub; UI only
  surfaces `clean`/`dirty`, other values round-trip as unknown
- `checks_conclusion` (`json:"checks_conclusion"`, line 74) — aggregated
  `"passed"`/`"failed"`/`"pending"` or `null` (no observed suite)
- `checks_passed` / `checks_failed` / `checks_pending` (lines 78-80) — per-suite
  counts; `aggregateChecksConclusion` (line 183) folds them into
  `checks_conclusion`

There is **no** standalone `draft` or `merged` boolean in the response. The
PR lifecycle is encoded in the single `state` string by `derivePRState`
(`server/internal/handler/github.go:994`):

```
merged   → if PullRequest.Merged
closed   → else if PullRequest.State == "closed"
draft    → else if PullRequest.Draft
open     → otherwise
```

`derivePRState` is called when the webhook upserts the row
(`server/internal/handler/github.go:682`), so `state` is what the list endpoint
returns. "Is it merged?" = `state == "merged"` (or `merged_at != null`); "is it a
draft?" = `state == "draft"`. Combine with `checks_conclusion` for CI status.

## Two distinct webhook paths: link vs close-intent

Both run inside the `pull_request` webhook handler, gated by the workspace
auto-link flag (`workspaceAutoLinkPRsEnabled`, `github.go:1074`).

### Path 1 — link (title OR body OR branch)

- `extractIdentifiers` regex helper: `server/internal/handler/github.go:1028`
- driving regex `identifierRe` (`\b([a-z][a-z0-9]{1,9})-(\d+)\b`, case-insensitive):
  `server/internal/handler/github.go:490`
- call site: `server/internal/handler/github.go:727` —
  `extractIdentifiers(p.PullRequest.Title, p.PullRequest.Body, p.PullRequest.Head.Ref)`

Every `PREFIX-NUMBER` mention in **title, body, or branch** resolves to an issue
in the workspace and writes a link row (`LinkIssueToPullRequest`, ~`github.go:762`).
This is what `multica issue pull-requests` later reads back.

Drifted from the prior skill's `github.go:727` citation, which pointed at the old
call-site location for the link logic.

### Path 2 — close intent (title OR body only, keyword-adjacent)

- `extractClosingIdentifiers` regex helper: `server/internal/handler/github.go:1051`
- driving regex `closingIdentifierRe`
  (`\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)[:\s]+([a-z][a-z0-9]{1,9})-(\d+)\b`):
  `server/internal/handler/github.go:501`
- call site: `server/internal/handler/github.go:736` —
  `extractClosingIdentifiers(p.PullRequest.Title, p.PullRequest.Body)` (no branch arg)

Only a `PREFIX-NUMBER` immediately after a closing keyword
(`Closes`/`Fixes`/`Resolves`, optional `:` then whitespace) sets the link row's
`close_intent` flag — the gate that auto-advances the issue to `done` on merge.
`Fix MUL-1` closes; `Fix login MUL-1` does not (adjacency). Branch names are
deliberately excluded (function doc, `github.go:1044-1050`): a branch like
`mul-1/fix-login` links but must never declare close intent.

Drifted from the prior skill's `github.go:736` citation.

Net: a bare title prefix (`MUL-2759: ...`) or a branch ref links only;
`Closes MUL-2759` links **and** records close intent.

## Status side effects (enqueue contracts)

| Behavior | File:line | Drifted from |
|---|---|---|
| Create-time: agent-assigned, non-backlog issue enqueues immediately | `server/internal/handler/issue.go:2263-2264` | new citation |
| `shouldEnqueueAgentTask` returns false for `backlog` (parking lot) | `server/internal/handler/issue.go:2644-2648` | new citation |
| Backlog → non-backlog (not done/cancelled) enqueues on update | `server/internal/handler/issue.go:2537-2540` | `:2523` |
| Same contract in batch update | `server/internal/handler/issue.go:3021-3024` | new citation |
| Child → `done` posts a system comment on the parent | `server/internal/handler/issue_child_done.go:51` (`notifyParentOfChildDone`; doc comment at `:15`) | func def `:51` |

Creation with `--status todo` (or any non-backlog status) on an agent-assigned
issue fires the agent immediately; `--status backlog` parks it with the assignee
set but no trigger. Promoting `backlog → todo` later fires it then (update path,
line 2537).

## Metadata CLI

| Behavior | File:line |
|---|---|
| `multica issue metadata set <issue-id> --key --value [--type]` | `server/cmd/multica/cmd_issue_metadata.go:80,109-111` |
| `multica issue metadata delete <issue-id> --key` | `server/cmd/multica/cmd_issue_metadata.go:93,113` |
| API routes (PUT/DELETE `/metadata/{key}`) | `server/cmd/server/router.go:478-479` |

`--value` is JSON-parsed by default (bool/number sniff); `--type` forces
`string`/`number`/`bool`.

## Verification command

Re-derive any line above before depending on it:

```bash
cd server
grep -n 'pull-requests <id>'                 cmd/multica/cmd_issue.go
grep -n 'ListPullRequestsForIssue'           cmd/server/router.go internal/handler/github.go
grep -n 'func issuePullRequestRowToResponse\|type GitHubPullRequestResponse struct\|func derivePRState\|func extractIdentifiers\|func extractClosingIdentifiers\|closingIdentifierRe' internal/handler/github.go
grep -n 'extractIdentifiers(\|extractClosingIdentifiers(\|derivePRState(' internal/handler/github.go
grep -n 'prevIssue.Status == "backlog"\|func (h \*Handler) shouldEnqueueAgentTask' internal/handler/issue.go
grep -n 'func notifyParentOfChildDone'       internal/handler/issue_child_done.go
grep -n 'func runPreviewCreate\|func runPreviewList\|func runPreviewStop' cmd/multica/cmd_preview.go
grep -n 'preview-sessions'                   cmd/server/router.go
grep -n 'func (h \*Handler) CreatePreviewSession\|func (h \*Handler) StopPreviewSession\|normalizeExternalPreviewURL' internal/handler/preview_session.go
```
