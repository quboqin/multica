# Creative Order execution and recovery

The order owns frozen business input. Its items own candidate directions; a
variant owns its version history. Tasks execute a declared scope, image
operations record provider calls, and assets record the resulting files.

## Record ownership

| Table | One row represents | Authoritative facts |
| --- | --- | --- |
| `creative_order` | One confirmed order | Workspace, frozen resources and target set count |
| `creative_order_item` | One material or selected copy composition | Source, approved copy and adoption |
| `creative_order_variant` | One candidate direction | Candidate/selected/reserve/rejected state and active/staging version pointers |
| `creative_order_variant_revision` | One version of a direction | Version brief, required delivery sizes and activation |
| `agent_task_queue` | One agent execution | Agent/runtime, execution status, session and task retry lineage |
| `creative_task_binding` | One task's business scope | Workspace, order, item, variant, requested revision, workflow and production phase |
| `creative_task_size` | One size in a task's declared scope | `is_expected`: package size; `is_required`: size this execution is asked to process |
| `creative_image_operation` | One logical image operation at a variant/version/size | Idempotency, provider request state and canonical result |
| `creative_image_operation_attempt` | One provider attempt | Request ID, result receipt, errors, timing and attachment |
| `creative_prime_composition_job` | One version's deterministic composition | Package sizes, input fingerprint, lease and composition handoff |
| `creative_order_asset` | One version/size/stage result | Attachment and source lineage for generated, primed or delivered output |
| `creative_order_qc_report` / `creative_order_variant_qc_resolution` | One QC attempt's evidence / decision | Findings and atomic delivery decision |
| `creative_recovery` | One missing stage at item/variant/version/size scope | Reason, bounded retry budget, next retry, lease and resulting task/asset |
| `creative_recovery_attempt` | One attempt to resume that stage | Source task, dispatched task, resulting asset, error and timestamps |
| `creative_order_recovery_scan` | One order's reconciliation cursor | Last inspection, next inspection and discovery error |
| `sys_cron_executions` | One scheduled job execution | Cross-Pod ownership, heartbeat and scheduler-level execution history |

Task state is read from `agent_task_queue`; the binding does not copy it. Model
request state stays in the image operation tables. A recovery attempt marked
`queued` means dispatch succeeded, not that its image has been delivered.

## Task relationships

The task-context write trigger is the single ingestion boundary for typed
relationships. It covers native fanout, retries, late receipts and existing CLI
clients. Runtime JSON remains an execution input snapshot; order readers and
recovery discovery use the relational ownership records.

Order/workspace, item/order and variant/item relationships have composite foreign
keys. A requested task revision may precede its version record; actual image
operations require the version foreign key. Planning and candidate selection
are item-scoped and have no variant or image size. One production task may own
multiple sizes, including a complete package with only two missing sizes.

Unresolvable or inconsistent input creates an `unresolved` binding with a reason
and no fabricated business links. The original task remains intact. Such rows
do not count as valid production work. Changes to a task's declared size scope
replace its current scope rows; actual provider attempts and their output
lineage remain in the operation/attempt tables.

## Order-first recovery

The server registers `creative_order_recovery` with the existing scheduler.
It ticks every minute, inspects at most 25 due orders and processes at most 25
due recovery records. Each order is normally checked again after two minutes.
Orders without mutable candidate/selected versions get a daily check; order
inserts and updates wake their cursor immediately. Disabled Creative Factory
workspaces are paused.

The scanner walks order → item → current version → required sizes. It checks
the actual package and selection state rather than treating the persisted
order `status` as a complete production summary.

| Condition | Action |
| --- | --- |
| Missing plan and no active root execution | Dispatch the frozen planner with the existing item ID |
| Incomplete candidate registration/delegation after planning ended | Retry planning; keep existing candidates and images |
| Settled candidate primaries, no selection task | Queue one comparison under the order/item lock |
| Selected version missing generated sizes | Dispatch missing-size expansion or normalize a bounded producer retry |
| Generated package complete, composition handoff missing | Enqueue deterministic Prime work; preserve model output |
| Primed package complete, QC dispatch/finalization missing | Use the existing QC handoff/recovery helpers |
| Active task or queued/running/unknown image operation | Wait; never submit another provider request |
| Cancelled object, obsolete version, reserve or rejected candidate | Do not resume production |
| User input, credentials, authorization, quota or exhausted budget | Record `manual_required` |
| Complete active package awaiting adoption | No production compensation |

An explicit failed Prime job or resolved QC decision is not automatically
overridden. A later unresolved QC attempt can still be recovered. Direct image
editing does not automatically submit another model call; its deterministic
handoff can be resumed from already registered outputs.

Recovery leases last two minutes and are fenced on completion. Expired attempts
retain their failure history. Object-level retries survive scheduler restarts
and use increasing delays up to 15 minutes, with three attempts by default;
existing task and QC budgets remain additional constraints. An error on one
object does not stop inspecting or processing its siblings.

## Read model and API

Order lists use five business-data queries independent of the number of orders
on the page. They retain the existing fields and historical assets. Detail
loads batch variant assets, versions, operations, attempts, QC and blockers;
candidate progress is grouped by item. Output ordering and the active-version
delivery rules remain unchanged.

Readers drain and close result sets before loading related records. Queries
inside order creation and production/QC handoffs reuse the transaction's
connection. Resource attachments use a joined read. Workflow metrics aggregate
tasks and assets separately to avoid multiplying their counts. Input attachment
storage validation still holds the order transaction while reading its files;
slow object storage can therefore extend submission time.

The detail page stays open after submission and follows WebSocket updates.
Notifications are grouped in 250 ms windows. Each active query has at most one
refresh in flight; notifications received during that request schedule a
subsequent read so its final state is not lost. Order-scoped updates refresh the
affected order and aggregate summaries. Hidden lists and dashboards stay
inactive, and leaving a page cancels its HTTP reads through AbortSignal.
Reconnect and task lifecycle events also refresh the data; the detail page does
not run a separate polling timer.

`GET /api/creative/orders/{id}` additionally returns optional `recoveries`,
including per-attempt history. The route's existing workspace authorization
applies. Lease tokens are not exposed. Older clients can ignore the field;
malformed optional recovery history does not discard the rest of the order.

## Upgrade and acceptance

Apply migrations 280–282 before serving this source. They add and backfill task
bindings, create recovery records/cursors, and add the task evidence-history
index. They do not delete or regenerate existing images. The index migration is
separate so `CREATE INDEX CONCURRENTLY` can run outside a transaction.

Managed workspace templates are version 31: planning 43, production 117,
direct edit 34, leadership 56, diagnosis 9. Managed Skill content follows the
platform template; agent model, thinking-level and concurrency preferences are
preserved. See [image model settings](creative-image-model-settings.md) for
Sunburst/xhigh execution and the deployment dependency on gateway support.

Acceptance should include a six-set copy order (eight planned candidates, six
selected sets, eighteen final images), optional empty copy slots, dense copy,
an existing stalled order, cancellation, and a competitor-material order.
Verify every size after official Prime composition, keep completed primary
asset IDs through recovery, and confirm that reserves are not expanded as
additional deliverables. Real model image quality must be inspected in the
test environment; backend tests establish dispatch, state and data invariants.

For connection pressure, submit a batch and remain on its detail page. Check
that progress continues through comparison and delivery, hidden dashboards do
not issue requests, and detail requests do not overlap during notification
bursts. Observe database pool acquisition waits, request latency and timeout
counts together. The isolated regression uses two connections for fifty
concurrent list/detail/dashboard reads and comparison handoffs; a one-connection
case covers submission validation and selected-size dispatch.
