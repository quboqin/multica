# Creative Order Planning Source Map

| Contract | Source |
| --- | --- |
| `multica creative order get` returns the item copy snapshot, source-analysis reference, frozen input snapshot, variants, and assets | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderGet`, `server/internal/handler/creative_domain.go:GetCreativeOrder` |
| `multica creative order variant-put` writes the structured V01-V03 briefs | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderVariantPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderVariant` |
| `multica task by-source list` returns all items for an Agent and evidence pair; callers compare item keys individually | `server/cmd/multica/cmd_task.go`, `server/internal/service/task.go:ListDirectTasksByEvidence` |
| Native production fanout is idempotent for active work by target Agent, evidence pair, and variant/revision item key | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:directTaskItemKey` |

Copy recommendation is a page-confirmation concern. This Skill's runtime source is only the order item's frozen `copy_snapshot`.
