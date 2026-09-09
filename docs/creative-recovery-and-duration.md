# Creative recovery dispatches and generation duration

Automatic creative recovery has its own bounded dispatch budget. An exhausted
source task does not prevent a continuation. The new task preserves the source
task lineage, attribution, frozen inputs and runtime configuration, starts a
fresh session, and has one infrastructure retry available.

`creative_recovery.dispatch_count` counts committed domain dispatches. Its
increment and `creative_recovery_attempt` receipt are committed in the same
transaction as the new agent task or Prime job. A rejected or rolled-back
dispatch consumes no dispatch budget. Three consecutive dispatch check failures
stop with an actionable error, independently of the dispatch budget. Expired
leases retain already committed dispatch receipts. The original `attempt` field
remains the monotonic audit sequence, including legacy failed dispatches.

Migration 284 counts historical dispatch receipts and re-evaluates only records
stranded by the old task-budget or candidate-selection eligibility rejection.
It preserves actual dispatches and audit history. Cancellation, active tasks,
current revisions, QC visual rework limits and unknown provider requests remain
eligibility gates. An unconfirmed provider operation without an active task for
15 minutes requires receipt reconciliation; it is never blindly repeated.

The UI shows one average generation duration, using the existing API fields
`image_generation_duration_seconds` and
`image_generation_duration_package_count`. The sample is order items submitted after
the measurement epoch and within the last 24 hours that have completed their
first generated package. It excludes cancelled orders and direct edits.
Duration runs from order submission to the first time all selected variants
have all three generated sizes in their current revision. Queueing and retries
are included. Prime composition, QC and subsequent edits are outside this
original-image metric.

Migration 285 establishes the measurement epoch once; service restarts do not
reset it. Database triggers record an immutable completion timestamp and asset
IDs/revisions, serialized per order item to handle concurrent final-size
writes. Later rejection or regeneration does not rewrite the first completion.
Old orders are not backfilled. With no qualifying completions, the UI shows
“待统计” rather than zero or an old average. The tooltip describes the window
and exclusions. This is a completed-sample average, not an estimate for unfinished
orders or a guarantee that generation will return to a previous speed.

Validation covers exhausted source budgets, lost leases and atomic rollback,
post-dispatch worker crashes, preserved operation inputs, legacy migration
counts, concurrent final sizes, revision consistency, immutable first completion,
the rolling 24-hour cohort, and the no-sample UI.
