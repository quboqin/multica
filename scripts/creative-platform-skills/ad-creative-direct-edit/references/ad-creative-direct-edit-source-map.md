# Creative Order Direct Ad Image Edit Source Map

| Contract | Source |
| --- | --- |
| A native direct task carries the order, variant, revision, and task-owned `expected_sizes` context | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/handler/creative_direct_edit.go:CreateCreativeDirectEdit` |
| `multica attachment download` retrieves an authenticated platform attachment | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload` |
| `multica attachment upload <file>` creates a workspace attachment and returns its ID | `server/cmd/multica/cmd_attachment.go:runAttachmentUpload` |
| `multica image edit` uses `gpt-image-2`, the global provider slot limiter, transient retry, and returns the exact provider prompt plus its SHA-256 | `server/cmd/multica/cmd_creative.go:runImageEdit`, `server/cmd/multica/cmd_creative.go:imagePromptSHA256` |
| `multica image edit-batch` applies the same provider slot limiter and returns the exact prompt/hash for each job | `server/cmd/multica/cmd_image_batch.go:imageEditBatchResult`, `server/cmd/multica/cmd_image_batch.go:executeImageEditBatch` |
| `multica creative order asset-put` records the generated base and lineage, validates its prompt trace, and rejects a different overwrite after completion | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderAssetPut`, `server/internal/handler/creative_domain.go:normalizeCreativeOrderAsset`, `server/internal/handler/creative_domain.go:validateCompletedGeneratedAssetTrace`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderAsset` |
| Human direct-edit initialization fixes the source asset at revision 1 and records candidate, request, size, delivery mode, squad, and leader trace | `server/internal/handler/creative_direct_edit.go:CreateCreativeDirectEdit` |
| Active direct tasks are idempotent by target agent, evidence pair, and `item_key` rather than by source alone | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:directTaskItemKey` |
