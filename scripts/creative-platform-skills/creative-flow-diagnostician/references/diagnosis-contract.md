# Creative Flow Diagnosis Contract

The diagnostician reads current platform state before making any recovery
change. Historical task summaries, Issue comments, old work directories, and
same-candidate neighbor items are evidence, not authority.

## Supported Surfaces

- AppGrowing Crawl Run diagnosis from sanitized run diagnostics.
- Creative Order diagnosis by order, item, variant, revision, workflow failure,
  asset package, QC report, and task source.
- Runtime diagnosis by daemon profile, Agent runtime binding, task queue state,
  and image-provider configuration presence.

## Recovery Authority

The diagnostician may perform only platform-native recovery:

- retry failed tasks by trigger evidence;
- cancel duplicate or stale active tasks after verifying the source;
- fan out missing current-revision creative tasks with complete context;
- advance a Variant revision only when the user explicitly asks to rerun it;
- call existing Prime repair, QC retry, or workflow retry endpoints when allowed.

It must not write the database directly, modify credentials, broaden AppGrowing
filters, edit market/copy resources, or treat diagnostic model outputs as
deliverable assets. If a recovery API requires a human actor, the task ends with
the exact user action needed.

## Image-Generation Diagnosis Playbooks

- `copy_snapshot` missing: use the current order item's schema-v3 snapshot,
  selected by both `creative_order_item_id` and `variant_id`.
- Latest-versus-frozen input: frozen order snapshots are authoritative until the
  user asks for a new revision.
- Variant rerun: compare current revision, current-revision assets,
  workflow_failures, and source tasks before advancing revision and fanout.
- Daemon/account confusion: daemon identity, Agent runtime, Agent env, and image
  provider billing records are separate evidence streams.
- Prime/QC block: generated, primed, delivered, QC, and diagnostic assets must
  all be scoped to the same variant and revision.

## Crawl Diagnosis Contract

The task context carries one Crawl Run and sanitized diagnostics only. The agent
may choose a browser-network retry for the same connector and business filters,
but it must not broaden the date range, lower selection thresholds, disable
deduplication, or switch credentials.

Allowed automatic outcomes are:

- record a verified route preference through a successful retry;
- finish after an existing successful browser-network capture;
- request user reauthentication when the stored session is invalid.

All other outcomes remain failed with their evidence. A diagnosis task is one
task per Crawl Run, not one task per page or competitor.
