# Issue-Native Ad Creative Production Source Map

| Contract | Source |
| --- | --- |
| `multica creative materials <issue-id> --selected` reads the existing Issue creative-materials endpoint and filters to person-selected candidates | `server/cmd/multica/cmd_creative.go:runCreativeMaterials` |
| `multica creative material download <issue-id> <candidate-id>` downloads only a person-selected candidate whose platform archive is completed | `server/cmd/multica/cmd_creative.go:runCreativeMaterialDownload` |
| `multica image edit --max-attempts 3` invokes the daemon image capability, retries transient 408/429/5xx or transport failures, and reports the actual attempt count | `server/cmd/multica/cmd_creative.go:runImageEdit`, `server/cmd/multica/cmd_creative.go:requestGPTImageEditWithRetry` |
| `multica issue comment add` supports local `--attachment` files | `server/cmd/multica/cmd_issue.go:runIssueCommentAdd` |
| Comment attachments upload before the Issue comment is posted | `server/cmd/multica/cmd_issue.go:runIssueCommentAdd` |
| `multica attachment download` gets attachment metadata through the authenticated API and writes a local file | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload` |
| The runtime brief requires platform attachments and resources to go through the Multica CLI | `server/internal/daemon/execenv/runtime_config.go` |
| An `mention://agent/<uuid>` comment link wakes an eligible agent task | `server/internal/handler/comment.go:computeMentionedAgentCommentTriggers` |
| Candidate selections and per-image copy snapshots are persisted on the Issue before image work is delegated | `server/internal/handler/creative_platform.go` |
| Structured delivery rows map each final attachment to its candidate, variant, size, revision, unbranded base, Prime evidence, and QC Issue | `server/internal/handler/creative_delivery.go`, `server/migrations/252_creative_delivery_adjustment.up.sql` |
| Adjustment requests validate `size` versus `variant` scope and persist target/base attachment IDs before the child Issue is assigned | `server/internal/handler/creative_delivery.go:CreateCreativeAdjustment`, `server/internal/handler/issue.go:CreateIssue` |
| `multica creative delivery register` posts a validated JSON manifest to the Issue delivery endpoint | `server/cmd/multica/cmd_creative.go:runCreativeDeliveryRegister`, `server/cmd/server/router.go` |

Reconfirm these paths before changing the direct-delivery contract.
