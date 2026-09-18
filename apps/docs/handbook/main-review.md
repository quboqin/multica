# Handbook review against main

Reviewed on 2026-09-18 in branch `docs/handbook-main-review`.

Baseline: freshly fetched `origin/main` at **`9e7e529b7fcba26ff4de10b089faa5a368b5a297`**, `fix(editor): release keys from empty mention picker (#8517)`, committed 2026-09-17 18:21:45 +0800. The review branch starts at that commit. Source links below refer to files at this baseline.

## Revision follow-up

The user subsequently requested updates to the three HTML artifacts. The assessment, findings, artifact line numbers, and original verification results below describe the **pre-edit review snapshot**, not the revised files. They remain as the rationale for the changes; the source-code baseline is unchanged.

The revised handbook updates lifecycle and execution boundaries, sessions, comment deletion, telemetry, and migration/authorization compatibility. The PRD consolidates requirements and adds explicit proposed acceptance gates for concurrent saves, data ownership, actor visibility, and schema administration. The prototype updates navigation and tokens, distinguishes illustrative controls from working demonstrations, and applies the UI review guidelines to keyboard focus, accessible names, and reduced motion. Phase-one Web/Desktop scope remains a planning assumption; these document edits do not implement Cortex or establish production acceptance.

Post-edit verification on 2026-09-18:

- All 169 authored IDs are unique within their pages; local fragments and companion links resolve. Pinned source-file paths and line targets resolve in local Git history; external sites were not HTTP-validated. PRD requirement references have corresponding definitions.
- All eight inline scripts pass `node --check`; the embedded Mermaid bundles are unchanged from the supplied files. Offline Chromium renders all 22 diagrams without uncaught page errors or HTTP(S) asset requests.
- All three documents pass document-level overflow checks at 1440px, 1024px, and 390px. All ten prototype screens were checked at each width, with light/dark visual inspection. Nested tables and navigation remain intentionally scrollable.
- The S4 input-arrow regression no longer changes screens or hides focus. Tab arrows, Home/End, roving focus, panel associations, and narrow-screen focused-tab visibility pass. Buttons and inputs have accessible names; reduced-motion mode leaves no running animations. Sort and source-review/promotion demonstrations pass. Static product illustrations are not working dialogs or application flows; this is not a full accessibility conformance audit.
- Whitespace checks against the pre-edit copies and credential-pattern checks pass. The initial full staged-file check reports 58 trailing-whitespace warnings, all inside byte-for-byte unchanged embedded Mermaid bundles; authored content is clean after removing six inherited whitespace-only issues from the prototype. Bundled template strings are preserved rather than reformatted.
- At validation time, remote `main` matched the fixed baseline. No application code, database, E2E, mobile, or real-agent tests were run; no migration or docs-site publishing was performed. Git commit/push delivery is separate from this document validation.

## Assessment

These documents are useful product and architecture drafts, but need revision before they can serve as an implementation contract or current operations handbook. The largest risks are the changed status model, migration compatibility, and the meaning of legacy automatic-run authorization. The original documents correctly disclose their historical baseline; this review identifies the changes needed to bring them forward.

| Artifact | Assessment | Main revision needed |
| --- | --- | --- |
| [Cortex PRD](cortex-prd.html) | Product direction remains coherent; several facts and acceptance boundaries are stale or inconsistent. | Status behavior, mobile scope, mention execution, shared data ownership, and consolidation of the incremental amendments. |
| [Architecture handbook](multica-architecture-handbook.html) | Broad source coverage, with material drift in security, execution, schema, and deployment details. | Migrations 467/468, session renewal, four lifecycle categories, Triage, comment tombstones, telemetry, and Redis configuration. |
| [UI prototype](cortex-ui-prototype.html) | Useful visual review artifact with working screen navigation and demonstrations. | Current sidebar and tokens, document-state semantics, keyboard focus, and accessible implementation contracts. |

The documents retain the original `dcc2d5825` baseline and a partial `3dcfdaea4` update. Current main is 301 commits after the original baseline and 117 after that update. It contains 528 up-migration files, with highest prefix 499; these are repository counts, not a claim about any deployed database.

The proposed Cortex document kinds, independent collections/records, knowledge ingestion, and workflow templates remain proposals in this main snapshot. Targeted schema/domain searches found no corresponding implementation; the generated [Issue model](../../../server/pkg/db/generated/models.go#L777) has no `kind`. The generic `packages/views/layout/collection-page.tsx` is a list-page shell, not the proposed Collection product. No other branch's implementation or earlier acceptance results were treated as evidence for main.

## Findings requiring revision

P1 means resolve before relying on the document for implementation or an upgrade. P2 means a substantive correction to the current reference or design handoff. These priorities describe the documents, not newly introduced production regressions.

### R1 · P1 · Custom statuses no longer inherit all built-in behavior

Locations: `cortex-prd.html:758,881,942`; `cortex-ui-prototype.html:891,1031`; `multica-architecture-handbook.html:1203,2097`.

The documents map `draft / reviewing / published` onto `todo / in_review / done` categories and describe a category as a complete behavioral equivalent. Current storage has four lifecycle categories: `unstarted / started / done / closed`. Custom statuses inherit lifecycle, but do not acquire all built-in parking, review, failure, or recovery behavior. For example, recovery explicitly leaves custom started statuses alone. See [status contract](../../../server/internal/issuestatus/issuestatus.go#L3), [effective status behavior](../../../server/internal/issuestatus/issuestatus.go#L207), [migration 492](../../../server/migrations/492_issue_status_category_contract.up.sql#L6), and [recovery](../../../server/internal/service/task.go#L5951).

Revise FR-016, the state tables, AC-5, S2, and the handbook together. Specify lifecycle mapping separately from desired dispatch, approval, notification, and recovery behavior. A mapping such as `reviewing → started` alone does not specify review behavior. Do not describe legacy category strings as rejected API input: [compatibility parsing and wire adaptation](../../../server/internal/issuestatus/issuestatus.go#L168) still support older clients. Likewise, some [custom-started handoff notifications](../../../server/cmd/server/notification_listeners.go#L89) remain implemented; the issue is the promise of wholesale inheritance.

### R2 · P1 · The upgrade guidance stops before a destructive schema contraction

Location: `multica-architecture-handbook.html:2197`, migration table in §10.

The table says migration 462 retains `reference_only`. [Migration 468](../../../server/migrations/468_drop_reference_only_column.up.sql#L1) subsequently deletes residual reference-only links and drops that column from both PR-link tables. It explicitly requires all previous-release instances to be gone because their inserts still name the removed column.

Extend the upgrade and recovery guidance with this compatibility boundary and the required deployment sequence. An operator following only the existing table could retain an old application instance against a schema where its PR-link writes fail.

### R3 · P1 · Legacy trigger principals are not necessarily historical creators

Locations: `multica-architecture-handbook.html:2141,2184,2197`.

The authorization description treats `created_by` as the immutable trigger creator and covers migration 449's publisher inference. [Migration 467](../../../server/migrations/467_autopilot_trigger_creator_from_autopilot.up.sql#L12) fills remaining eligible rows from the automation object's creator. It expressly describes this as an authorization widening: execution and delegated runs act with that member's rights. Application rollback and the down migration do not undo it.

Describe both backfills, distinguish inferred principal from proven authorship, and add audit/recovery guidance. [The corrected column contract](../../../server/migrations/467_autopilot_trigger_creator_from_autopilot.up.sql#L58) also preserves the still-valid rule that ordinary edits cannot transfer the dispatch principal.

### R4 · P2 · Mobile reuse and phase-one scope contradict the current architecture

Locations: `cortex-prd.html:529,605,1053,1121`.

The PRD says Web, Desktop, and Mobile share the TipTap editor. Mobile instead owns a [native composer](../../../apps/mobile/components/composer/message-composer.tsx#L8) and [Markdown description renderer](../../../apps/mobile/components/issue/issue-description.tsx#L1). NFR-043 includes mobile document editing and record-detail editing in phase one, while Q-U4 recommends excluding mobile.

Limit the existing TipTap reuse claim to Web/Desktop and make a single explicit mobile scope decision. Any included mobile work needs its own design, implementation estimate, and verification; the shared web editor is not evidence that mobile delivery is nearly free.

### R5 · P2 · Mention execution depends on the write path

Locations: `cortex-prd.html:883,892`.

The PRD says an agent mention starts computation wherever it is written and extends the code-example hazard to any document. The [raw Markdown regex](../../../server/internal/util/mention.go#L16) does ignore code boundaries, but agent execution is implemented in [comment routing](../../../server/internal/handler/comment.go#L2672), with `/note` returning before that routing at line 2660. Description creation/update listeners [notify members](../../../server/cmd/server/notification_listeners.go#L697); [their recipient handling](../../../server/cmd/server/notification_listeners.go#L562) does not execute agents.

Keep the executable-comment warning, distinguish notification from execution, and specify the proposed document-body mention policy separately. A description containing an agent link is not, by itself, proof of current automatic dispatch.

### R6 · P2 · Session renewal and CSRF descriptions are obsolete

Locations: `multica-architecture-handbook.html:1666,1822`.

The handbook says there is no refresh flow and describes CSRF as bound only to the current token. Main adds [sliding session renewal and stable per-login `sid`](../../../server/internal/auth/session.go#L13), [renewal on eligible safe Cookie-authenticated requests](../../../server/internal/middleware/session_renewal.go#L24), and [`POST /api/auth/refresh`](../../../server/cmd/server/router.go#L1560) for token-mode clients. The [cookie contract](../../../server/internal/auth/cookie.go#L20) includes `multica_csrf_session` for renewal-safe CSRF handling.

Update the login diagrams and expiry/recovery advice. Preserve the distinction that this remains stateless JWT renewal: it does not add a separate refresh token or server-side session revocation.

### R7 · P2 · Triage is an additional execution gate

Locations: `cortex-prd.html:880`; `multica-architecture-handbook.html:2098`.

The documents state that backlog is the execution gate and that a stage-four `todo` issue runs immediately. Main now has an independent nullable [`triage_state`](../../../server/migrations/483_issue_triage_state.up.sql#L1). The [queue guard](../../../server/internal/service/task_triage_guard.go#L14) blocks executors derived from an issue in Triage, including assignment and retries, while permitting manually named agent conversations.

Include Triage in the scheduling model and proposed workflow acceptance cases. Stage is still not a scheduler by itself, and backlog remains a parking mechanism, but neither statement is a complete dispatch predicate. Do not describe the whole Triage product as finished; the guard explicitly identifies later subfeatures as unbuilt.

### R8 · P2 · The universal hard-delete invariant no longer holds

Location: `multica-architecture-handbook.html:1087`.

The handbook says `deleted_at` does not exist anywhere. [Migration 471](../../../server/migrations/471_comment_deleted_at.up.sql#L1) introduces comment tombstones. The [delete handler](../../../server/internal/handler/comment.go#L3731) preserves a cleared parent when replies still refer to it, and later prunes replyless tombstones.

Update the invariant and comment ER model. Proposed knowledge/context ingestion must distinguish a retained comment identity from available live content, particularly when handling source deletion or revocation.

### R9 · P2 · Self-host deployment and outbound-data guidance needs two updates

Locations: `multica-architecture-handbook.html:1856,1991,1996`.

The data-flow chapter omits default-on anonymous API-server telemetry. The [configuration](../../../server/internal/selfhosttelemetry/config.go#L15), [fixed first-party destination](../../../server/internal/selfhosttelemetry/client.go#L10), and [payload](../../../server/internal/selfhosttelemetry/event.go#L23) define an anonymous instance identifier, release version, bucketed counts, and aggregate run totals. [`.env.example`](../../../.env.example#L663) documents `DO_NOT_TRACK=1` or `true` and a backend restart to disable collection and delivery. Add this flow and opt-out to the self-host/offline checklist without implying that business content is transmitted.

Separately, the handbook says Compose does not forward `REDIS_URL`. Current [Compose configuration](../../../docker-compose.selfhost.yml#L70) forwards both `REDIS_URL` and `REDIS_CLUSTER_MODE`. Retain the accurate statement that the default stack does not include a Redis service, and update the instructions for connecting an external Redis instance.

### R10 · P2 · The UI shell no longer reproduces main's navigation

Locations: `cortex-ui-prototype.html:706,738,748,2098`.

S1 claims to reuse personal/pinned/workspace/configuration groups. Main uses personal/pinned, Work, and AI Team groups with footer utilities; see [sidebar definitions](../../../packages/views/layout/app-sidebar.tsx#L147) and [rendered grouping](../../../packages/views/layout/app-sidebar.tsx#L860). Current [Chinese labels](../../../packages/views/locales/zh-Hans/layout.json#L5) also use 任务 / 自动化 / Skills rather than the prototype's 工作项 / 自动驾驶 / 技能.

Rebase the shell and glossary before deciding where the new destinations belong. This is drift in the claimed existing foundation; the proposed destinations themselves remain valid design work.

### R11 · P2 · The prototype's token-compliance claim is inaccurate

Locations: `cortex-ui-prototype.html:35,40,60,187,227,230,2034,2079`; `cortex-prd.html:846`.

S9 says the prototype copies the current tokens exactly and introduces no new sizes. Its brand and radius differ from [current tokens](../../../packages/ui/styles/tokens.css#L223), and 9px, 10px, and 22px UI text falls outside the [role scale](../../../packages/ui/styles/tokens.css#L91). More consequentially, `--text-3` aliases `--faint-foreground` for meaningful labels and metadata, although the [current contract](../../../packages/ui/styles/tokens.css#L195) reserves faint for non-text marks and sets muted as the text floor.

Update the values and typography roles, use a text-safe muted foreground, and qualify historical visual approximations. The proposed semantic tint pairs should remain explicitly proposed additions, not be represented as existing tokens.

### R12 · P2 · Arrow navigation hides the focused input

Locations: `cortex-ui-prototype.html:1369,2169,2180,2194`.

Browser reproduction: open S4, focus its field-name input, press ArrowRight. S5 becomes active while `document.activeElement` remains the input in hidden S4. The page-level key handler is not scoped to tabs or guarded against editable targets, and screen switching does not move focus.

Scope arrow navigation to the tab list, keep focus and selection synchronized, and associate tabs and panels. The existing [Tabs primitive](../../../packages/ui/components/ui/tabs.tsx#L3) supplies the appropriate production foundation. This is a confirmed bug in the review artifact itself.

### R13 · P2 · Static controls leave accessibility requirements unspecified

Locations: `cortex-ui-prototype.html:1111,1115,1127,1161,1253,1369,1595`.

The insertion dialog is generic markup; radio/switch illustrations are spans, and several icon-only actions have no accessible names. The browser audit found 23 icon-only buttons across the ten screens without text, `aria-label`, or a title. The field-name input also lacks a label. These are static mock controls, not a production regression, but they do not establish accessible interaction behavior.

Add an explicit handoff contract or demonstrate named controls, keyboard choices, focus trapping/restoration, Escape, and visible focus. Use the repository [Button](../../../packages/ui/docs/button.md#L24) and [Dialog](../../../packages/ui/docs/dialog.md#L13) contracts. The [Web Interface Guidelines](https://raw.githubusercontent.com/vercel-labs/web-interface-guidelines/main/command.md) informed this accessibility review; repository copy conventions take precedence over generic punctuation guidance.

## Acceptance and implementation decisions to resolve

These are valid proposed capabilities that need explicit engineering contracts, rather than features expected to be present already.

- **Concurrent document editing:** FR-017 (`cortex-prd.html:759`) and the edge-case table (`:959`) promise conflict comparison and no silent overwrite, but the document milestone makes FR-017/018 optional (`:1137`). Current [description writes deliberately use last-write-wins](../../../server/internal/handler/issue.go#L3370), including writers without a base revision. Choose a minimum release acceptance requirement for lost-update protection, or narrow the promise until that work ships. Reusing the editor does not provide paragraph locking.
- **Actor references and schema administration:** FR-023/027 and Q-U3 (`cortex-prd.html:772,776,1120`) propose agent/squad references and agent-created fields. The current [property contract](../../../server/internal/handler/property.go#L310) requires visibility gates on writes and table-facet reads before adding non-public actors; [definition administration](../../../server/internal/handler/property.go#L625) currently rejects agent actors. Define collection-specific permissions and test private-agent discovery/facet leakage, without silently changing issue-field administration.
- **Shared view ownership:** FR-002 (`cortex-prd.html:740`) still describes decoupling an issue store. Main now has [workspace-scoped Query projections](../../../packages/core/issues/queries.ts#L45), [server-owned table membership/facets/paginated branches](../../../packages/views/issues/surface/use-issue-surface-controller.ts#L87), and [mutation/WebSocket reconciliation](../../../packages/core/issues/mutations.ts#L284). Carry these guarantees into the shared DataSource contract; changing a write endpoint alone is not sufficient acceptance.
- **Canonical requirements:** Fold §14's corrections into FR-045, NFR-011, and the data sketch (`cortex-prd.html:807,1044,1168`). The older sections retain a doc-only revision key while §14 requires workspace/object-kind isolation, monotonic revisions, and withdrawal handling. Likewise reconcile offline promises at `:1045,1050` with §14, and resolve the mobile inconsistency above. Preserve the old review as dated history so implementers have one current requirement for each behavior.

## Placement and maintenance

`apps/docs/handbook` currently provides repository-local artifacts. The docs app indexes [`content/docs`](../source.config.ts#L3), resolves pages through [its MDX loader](../lib/source.ts#L5), and uses [`basePath: /docs`](../next.config.mjs#L8). No route or build-copy integration for the new handbook directory was found. Moving the files here alone does not publish `/docs/handbook/*.html`. If repository-local viewing is the intent, this placement is sufficient; site publishing needs a separate route or asset-copy decision. This assessment is based on configuration inspection, not a running Next.js deployment.

The handbook's 31 pinned source links still resolve to files at `dcc2d5825`, and those paths also exist in current main. They remain useful historical citations, but do not substantiate newer-main claims. Refresh citations as claims are revised. The September 10 merge advice and branch counts are historical and do not describe this new branch.

Lower-priority follow-ups: add reduced-motion treatment to prototype transitions and indefinite cursor/pulse animations (`cortex-ui-prototype.html:183,346,361,538,2184`); fix the S9 narrow-screen overflow; add the newer maintenance-job tables and claim-time title/description snapshot to the handbook inventory. A title/description snapshot is not a full issue revision snapshot.

## Verification and limits

- Fetched `origin/main`, checked the repository state, and created the docs branch at the fetched SHA. The three supplied HTML files were preserved byte-for-byte. This review is the only newly authored repository file; no fixes were applied to the originals.
- Reviewed authored document content and traced material claims to main's source, migrations, configuration, and repository conventions. Still-aligned examples include five issue layouts, nine property types, the 20-definition limit, the 16KB property bag, member-only actor values, per-issue/agent execution serialization, and a runner deciding stage advancement.
- Static checks found no duplicate authored IDs or missing local fragments across 169 IDs. All eight inline scripts passed `node --check`. All 31 pinned source-file links resolved in local Git history, including their line targets. External sites were not HTTP-validated.
- Loaded all three standalone HTML pages in Chromium; observed no uncaught page errors or HTTP(S) asset requests. All 22 Mermaid diagrams rendered: 2 in the PRD and 20 in the handbook. Both document navigation links reached `#main-sync`.
- Exercised all ten prototype screen tabs, its theme switch, sort demonstration, and source-review/promotion demonstration. The input-arrow focus failure in R12 was reproduced. Prototype rendering was also checked with the browser offline.
- Checked desktop at 1440px and narrow layout at 390px, plus light/dark appearance. The PRD and handbook had no document-level horizontal overflow at 390px. Across the prototype's ten screens, S4 had approximately 1px overflow and S9 had 36px; the other eight had none. Nested tables and navigation can still scroll horizontally. This does not establish production-mobile usability or completion of the mobile product scope.
- No application unit, integration, database, E2E, mobile, or real-agent tests were run; no application behavior or migration was executed. This was a documentation/source review plus standalone-document browser validation. Competitor pricing, external licensing, and knowledge-backend product claims were outside the main-branch comparison and were not reverified.

## Recommended revision order

1. Resolve lifecycle/dispatch semantics and the two migration/authorization findings; refresh the shared execution contract in all three artifacts.
2. Correct current architecture facts, settle mobile and editing/permission acceptance decisions, and consolidate §14 into the requirements.
3. Update the prototype shell, tokens, keyboard behavior, and accessibility handoff; make the repository-only versus published-docs choice explicit.
4. Re-run document/link/browser checks after edits, then review the resulting documentation diff before committing or publishing.
