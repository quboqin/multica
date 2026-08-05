# Creative Order Ad Production Source Map

| Contract | Source |
| --- | --- |
| `multica creative order get` returns the order, items, variants, assets, and QC reports used as the native workflow snapshot | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderGet` |
| `multica creative library download <candidate-id> --output-file <path>` downloads an archived workspace creative material | `server/cmd/multica/cmd_creative_domain.go:runCreativeLibraryDownload` |
| `multica image edit --max-attempts 3` invokes the daemon image capability, retries transient 408/429/5xx or transport failures, and reports attempts plus `provider_slot_limit` | `server/cmd/multica/cmd_creative.go:runImageEdit`, `server/cmd/multica/cmd_creative.go:requestGPTImageEditWithRetry` |
| `multica image edit-batch` executes a dependency-aware image DAG and returns per-job request IDs, attempts, bytes, paths, durations, and `provider_slot_limit` | `server/cmd/multica/cmd_image_batch.go` |
| Relative batch paths resolve against the manifest directory | `server/cmd/multica/cmd_image_batch.go:loadImageEditBatchManifest` |
| The selected order item must contain a schema-v2 copy snapshot with valid creative type, status, and approved recipe provenance; prompt financial tokens are then checked against its frozen visible copy before model invocation | `references/validate_copy_snapshot.py`, `server/internal/handler/creative_domain.go:validateCustomCreativeOrderCopyFacts` |
| Exact delivery dimensions use bounded deterministic cover-resize; after two provider ratio failures, an explicit bounded full-content edge-fade extension is available and recorded in evidence | `references/normalize_image.py` |
| Direct and batch image requests share a host-wide provider slot limiter, configurable with `MULTICA_IMAGE_MAX_CONCURRENT` | `server/cmd/multica/image_slots.go`, `scripts/start-direct-image-daemon.ps1` |
| `multica attachment upload <file>` returns a workspace attachment ID without linking it to a conversational container | `server/cmd/multica/cmd_attachment.go:runAttachmentUpload` |
| `multica creative order asset-put` upserts a generated, primed, or delivered asset keyed by variant, size, revision, and stage | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderAssetPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderAsset` |
| Runtime resources and attachments use authenticated Multica CLI access | `server/internal/daemon/execenv/runtime_config.go` |
| Direct fanout injects `item_key` into task context and deduplicates active work by target agent, evidence pair, and item key | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:normalizeDirectTaskContext` |

Reconfirm these paths before changing the native delivery contract.
