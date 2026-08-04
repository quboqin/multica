# Creative Order Prime Compose Source Map

| Contract | Source |
| --- | --- |
| Prime composition accepts a manifest batch and validates each contractual delivery canvas plus QR payload | `references/image_prime_compose.py` |
| `multica creative order get` returns the frozen order, variant, assets, and market input snapshot | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderGet` |
| `multica attachment download` and `upload` use authenticated workspace attachments | `server/cmd/multica/cmd_attachment.go` |
| `multica creative order asset-put` records each primed asset and its generated lineage | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderAssetPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderAsset` |
| Native QC fanout uses a shared evidence source and distinct lane/revision item keys | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:normalizeDirectTaskContext` |

Reconfirm these paths before changing the Prime delivery contract.
