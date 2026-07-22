---
name: multica-working-on-issues
description: "Use when working on a Multica issue after the runtime has provided the trigger context — to apply the product contracts the runtime brief does not encode: how PR linking differs from close intent, how to read a linked PR's real state via the pull-requests CLI, how to register external Web previews, which metadata keys are high-signal, what status changes trigger on the server, and how sub-issue create status (todo vs backlog) controls whether assigned agents start immediately."
user-invocable: false
allowed-tools: Bash(multica *), Bash(git *), Bash(gh *)
---

# Working on Multica issues

Product contracts the runtime brief does not fully encode: PR linking vs close
intent, reading linked-PR state, metadata keys, status side effects, and
sub-issue enqueue behavior.

For building mention links, load `multica-mentioning` instead — not this skill.

Every contract below is traced to source in
`references/working-on-issues-source-map.md`.

## PR linking and close intent are two distinct contracts

The GitHub webhook runs two separate scans over an incoming PR. They are not the
same gate and they read different fields.

**Linking** scans the PR **title, body, OR branch** for a routable issue key
(`PREFIX-NUMBER`, e.g. `MUL-2759`). Each match writes an issue ↔ PR link row.
This is the link that `multica issue pull-requests` reads back.

```text
MUL-2759: add built-in issue working skill        # title prefix → links
agent/matt/mul-2759-working-on-issues             # branch ref   → links
```

**Close intent** is stricter and is a separate scan over **title or body only —
never the branch**. It fires only for a key placed immediately after a closing
keyword (`Closes` / `Fixes` / `Resolves`, optional `:` then whitespace). That
adjacency is what sets the link row's close-intent flag, the gate that
auto-advances the issue to `done` when the PR merges.

```text
Closes MUL-2759                                    # links AND records close intent
Fixes MUL-2759
Resolves MUL-2759
Fix login MUL-2759                                 # links only — keyword not adjacent
```

Consequence: a bare title prefix or a branch reference links the PR but does not
close the issue on merge. A closing keyword immediately adjacent to the issue key
records close intent; on merge, that close intent can move the linked issue to
`done`.

### Default for code-changing issue work

When an issue run changes code in a checked-out GitHub repo, the default handoff
is to open or update a PR before posting the final Multica issue comment, unless
the user explicitly asked for a local-only change or no PR. This is a default, not
an unconditional command: if no code changed, say no PR is needed; if PR creation
is blocked by auth, failing tests, or missing remote state, report that blocker
instead of pretending the run is complete.

Use a routable issue key in the PR title, body, or branch so the webhook can link
the PR back to the issue. If the PR should close the issue on merge, put the key
immediately after a closing keyword in the title or body, for example:

```text
MUL-2759: fix login redirect        # links only
Closes MUL-2759                     # links and records close intent
```

In the final issue comment, include the PR URL when a PR exists. If the task did
not produce a PR because no code changed or the user asked not to create one, say
that explicitly.

## Reading a linked PR's real state

When a step depends on PR state, query Multica's link table — do not infer it
from branch names, GitHub search, memory, or `pr_url` metadata (which can be
stale).

```bash
multica issue pull-requests <issue-id> --output json
```

Returns `{"pull_requests": [...]}`. Each element exposes:

- `number`, `html_url`, `title`
- `state` — the PR lifecycle as a **single enum**, one of `merged`, `closed`,
  `draft`, `open`. There is no separate `draft` or `merged` boolean in the
  response; the server folds them into `state` (merged wins, then closed, then
  draft, else open).
- `merged_at` — non-null once merged; a second confirmation of `state: merged`.
- `mergeable_state` — mirrors GitHub (`clean` / `dirty` surfaced; other values
  round-trip as unknown).
- `checks_conclusion` — aggregated CI: `passed`, `failed`, `pending`, or `null`
  when no check suite has been observed. Backed by `checks_passed`,
  `checks_failed`, `checks_pending` counts.

So "is it merged?" is `state == "merged"` (or `merged_at != null`); "is it still
a draft?" is `state == "draft"`; CI status is `checks_conclusion`.

If the command returns no linked PRs after a PR was opened, the link scanner did
not observe a routable issue key in the PR title/body/branch.

## External Web preview sessions

When issue work produces an already-running HTTP(S) URL that reviewers can
reach, register it as a preview session:

```bash
multica preview create --issue <issue-id> --url <https-url> --title "<title>" --expires-at <RFC3339> --output json
```

`--title` and `--expires-at` are optional. The Phase 0 provider is fixed to
`platform=web` and `provider=external_web`. It records the URL; it does not
start a dev server, expose `localhost`, or create a port tunnel. Do not register
URLs containing credentials, secret query parameters, or services that only
the agent machine can reach.

Read or stop sessions with:

```bash
multica preview list --issue <issue-id> --output json
multica preview stop <session-id> --output json
```

## Local Android device preview sessions

Device sessions are exclusive per serial. Before deployment or any semantic
action, use the Issue-bound `preview device` command; it acquires or renews the
session lease and fails on a conflicting Issue instead of falling back to a
different device. Both `running` and `sleeping` sessions are reusable. The
Issue viewer performs the same lease renewal when opened.
Device Runtime APIs require the Host secret from `MULTICA_DEVICE_RUNTIME_TOKEN`
for native CLI calls and a console-minted, Preview Session-bound token for Issue
browser calls. Never place the Host secret in a Preview URL or scenario file.

Use `multica preview device sync` for normal Android deployment. Do not call
`preview device create` with a bare Runtime URL: every `local_device` Session
must pin one `?serial=<device>`. The server rejects unpinned or multiply pinned
URLs so an unavailable phone cannot silently switch the Issue viewer to an
emulator. Direct `device create` remains a diagnostic command for an
already-running, explicitly pinned device URL.

Repository and preview-target discovery comes from the task's actual development
context, not from Multica repository registration. Read
`.multica/preview/targets.json`, which the runtime generates from Git roots and
project manifests at task start and refreshes after `multica repo checkout`. If
the code was cloned, copied, or switched another way, run `multica preview
detect --write` from the task workdir. A single repository may expose several
Web/H5, Android, iOS, or desktop targets. Project resources, when present, are
only optional hints and policy overrides.

Do not check every repository out. Build the smallest working set from explicit
repository references first, then development-context evidence and optional
resource `role` / `capabilities` metadata. For a hybrid application, ordinary
business UI work normally selects the H5 target; include the Android or iOS
shell only when the task crosses a native build, bridge, permission, or platform
boundary. If multiple candidates remain, ask before editing.

Repository selection does not decide whether the result is previewed. After a
successful runnable checkpoint, every selected previewable frontend target
publishes to the same Issue Preview area by default. With preview policy `auto`,
detect Web/H5, Android, iOS, and desktop targets from the repository structure;
a backend-only repository does not create a visual preview. Project resource
policy `always` or `never` overrides automatic classification.

For a Device Runtime advertised by the one online Runtime labeled
`mac-mobile-preview`, synchronize a runnable Android checkpoint after the
repository-approved APK build succeeds:

```bash
multica preview device sync --issue <issue-id> --artifact <absolute-apk-path> --output json
```

For a hybrid application whose business UI is H5, start the task checkout's H5
development server and pass the URL reachable from the device. Android emulator
uses `10.0.2.2` for the host; a USB device normally uses `adb reverse` and
`127.0.0.1`:

```bash
multica preview device sync --issue <issue-id> --artifact <absolute-apk-path> --web-url http://10.0.2.2:<h5-port> --scenario <scenario.json> --output json
```

For a loopback `--web-url`, Device Runtime creates the matching `adb reverse`
on the selected serial automatically. The resulting Preview Session is pinned
to that serial, so an embedded Issue viewer cannot silently switch back to an
emulator when several devices are online.

Only a preview-enabled stage/debug shell should accept `--web-url` and expose
WebView debugging. Never modify a release shell to accept an arbitrary launch
URL or enable its WebView debugger.

The Mac daemon advertises its loopback Device Runtime endpoint with
`--device-runtime-url`; omitting `--url` resolves that endpoint from the fixed
`mac-mobile-preview` Runtime metadata. Resolution fails when zero or multiple
matching Runtimes are online. An explicit loopback `--url` remains available
for local diagnostics.

Without `--serial`, `sync` first inherits the serial pinned by the newest
running Android Device Preview on the Issue. It selects an online emulator only
when the Issue has no active device binding. Pass `--serial <adb-serial>` to
switch explicitly. After the target Session is running, `sync` stops every
other running Android Device Preview for the same Issue and Runtime, so the
agent control target and the viewer cannot diverge. The URL is deliberately
restricted to loopback, so it represents a local public-computer runtime rather
than an externally reachable web preview.

Always pass the absolute path to the APK produced by the current task checkout.
Omitting `--artifact` uses the Device Runtime startup default and is intended
for manual console use; an agent checkpoint must not risk reinstalling a stale
APK from another working directory.

Do not run `sync` on every source-file save. Run it after a build succeeds at a
runnable checkpoint: the app launches and the changed flow can be exercised.
This makes the updated application visible in the existing Issue viewer without
creating duplicate preview records.
The Runtime reports preparing, installing, starting, connecting, and ready
progress to that viewer. It hashes the APK and skips installation when the same
artifact is still installed on the selected serial. Do not add sleeps or run a
second deploy to guess readiness; wait for the sync command or deployment job.

When the repository contains a checked-in device scenario for the changed flow,
run it in the same checkpoint command:

```bash
multica preview device sync --issue <issue-id> --artifact <absolute-apk-path> --scenario <scenario.json> --output json
```

To run a scenario without reinstalling the APK, target the active device
attached to the Issue:

```bash
multica preview device snapshot --issue <issue-id> --output json
multica preview device run --issue <issue-id> --scenario <scenario.json> --output json
```

`run` derives both the Runtime endpoint and device serial from the Issue's one
running Android Preview Session. It rejects a missing, ambiguous, unpinned, or
offline binding rather than selecting another device. Interaction intent,
business vocabulary, selector policy, and demonstration pacing belong in an
agent-assigned workspace Skill so those policies can evolve without a server
release.

H5 scenarios should use semantic `fill`, selector-based `tap`, `assert`, and
`wait_for` actions. Selectors support `role`, accessible `name`, visible `text`,
`placeholder`, `test_id`, explicit `css`, and optional `contains`. Device Runtime
resolves them through the inspected WebView DOM and fails when a selector is
missing or ambiguous. Native fallback actions remain `tap`, `swipe`, `text`,
`key`, and `wait`; coordinate steps may declare `source_width` and
`source_height`. Use native actions only for platform UI or surfaces without a
structured document.

The local Device Runtime must be started with a matching scrcpy Server binary
and protocol version so its H.264 stream can be published over WebRTC:

```bash
multica device serve --adb <adb> --access-token "$MULTICA_DEVICE_RUNTIME_TOKEN" --artifact <apk> --package <package> --activity <component> --scrcpy-server <scrcpy-server> --scrcpy-version <version>
```

The Issue viewer adds `embed=1` for `local_device` sessions. That mode exposes
only the interactive device video; deployment, cleanup, navigation, and logcat
remain available only in the standalone local console.

Create and stop mutate durable issue state. Listing is read-only. Stopping the
record does not terminate the external process behind its URL.

## Metadata: high-signal keys only

Metadata is durable issue state. Reading metadata is safe. Writing a metadata key
is a state mutation and should be tied to an explicit task requirement to record
that state for later readers or runs.

High-signal keys (reuse these names so queries stay consistent):

- `pr_url`
- `pr_number`
- `pipeline_status`
- `deploy_url`
- `external_issue_url`
- `waiting_on`
- `blocked_reason`
- `decision`

Not metadata: logs, summaries, files touched, timestamps, attempt counts,
investigation notes. Those belong in the result comment.

```bash
multica issue metadata set <issue-id> --key pr_url --value <url>
multica issue metadata delete <issue-id> --key <stale-key>
```

`--value` is JSON-parsed by default (bool/number are sniffed); pass `--type
string|number|bool` to force a type.

## Status changes have server side effects

A status change is not cosmetic — the server enqueues or skips agent work based
on it. These are the contracts, not advice:

- **`backlog`** parks an agent-assigned issue: the assignee is set but no task
  fires. Moving `backlog → todo` (or any non-done/non-cancelled status) enqueues
  the assigned agent then.
- **`in_review`** is an accepted issue status. Some workflows use it while a PR
  is open and awaiting review; moving to it is an explicit mutation.
- **`done`** on a child issue posts a system comment on its parent. If a PR
  carries close intent (`Closes MUL-XXXX`), it advances the issue to `done`
  itself on merge — you do not also need to flip it manually.
- **`cancelled`** stops outstanding work; treat it as a user-driven decision.

## Sub-issues: `todo` starts work now, `backlog` parks it

On an agent-assigned issue, create status decides whether the assignee fires
immediately. A non-backlog status (e.g. `todo`) enqueues the agent at create
time; `backlog` sets the assignee without triggering.

Parallel children — all start now:

```bash
multica issue create --title "..." --parent <issue-id> --assignee <agent> --status todo
```

Strictly serial children — park later steps, promote one at a time:

```bash
multica issue create --title "Step 2: ..." --parent <issue-id> --assignee <agent> --status backlog
multica issue status <child-id> todo   # promote when the previous step is truly done
```

Creating every serial step as `todo` enqueues the whole chain at once.

## Incorrect → correct

PR title (link the issue):

```text
Fix login redirect                  # incorrect — no issue key, won't link
MUL-2759: fix login redirect        # correct — links the PR
```

Serial sub-issues (don't start the whole chain):

```bash
# incorrect — both fire immediately
multica issue create --title "Step 2" --parent <issue-id> --assignee <agent> --status todo
multica issue create --title "Step 3" --parent <issue-id> --assignee <agent> --status todo

# correct — parked, promote in turn
multica issue create --title "Step 2" --parent <issue-id> --assignee <agent> --status backlog
multica issue create --title "Step 3" --parent <issue-id> --assignee <agent> --status backlog
```

## References

`references/working-on-issues-source-map.md` — accurate `file:line` for every
contract above: the `pull-requests` CLI and route, the PR response field list,
`derivePRState`, the two-path link (`extractIdentifiers`) vs close-intent
(`extractClosingIdentifiers`) proof, the backlog enqueue lines, child-done
notify, and the metadata CLI. Re-derive before depending on an exact line.
