# Creative Order QC Source Map

| Contract | Source |
| --- | --- |
| Visual QC judges the frozen copy snapshot, visual direction, actual Prime-composed image and recorded model prompt evidence before judging semantic theme and hierarchy | `scripts/creative-platform-skills/ad-creative-qc/SKILL.md:视觉质检合同`, `server/internal/handler/creative_domain.go:validateCompletedGeneratedAssetTrace` |
| Attachment downloads use the authenticated CLI and honor `MULTICA_HTTP_TIMEOUT`; QC deliberately serializes Prime image downloads at the Skill orchestration layer | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload`, `server/internal/cli/client.go:AtLeastAPITimeout` |
| `multica creative order qc-put` writes one report per variant, lane, and revision | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderQC` |
| `multica creative order qc-finalize` waits for the visual report, then delivers a clean package, queues visual rework, or creates an action-required resolution | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCFinalize`, `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| A structured final visual finding for actual Prime obstruction or unreadable official Prime text queues up to two next-revision production tasks; after that the current delivery stays available with QC risk, while unstructured visual failures remain manual | `server/internal/handler/creative_domain.go:creativeVisualModelReworkFindings`, `server/internal/handler/creative_domain.go:queueCreativeVisualModelRework` |
| Native QC tasks are distinct by evidence pair plus visual-lane revision `item_key` | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:directTaskItemKey` |

Reconfirm these paths before changing the QC release contract.
