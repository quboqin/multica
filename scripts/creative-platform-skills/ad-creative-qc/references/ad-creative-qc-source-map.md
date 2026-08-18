# Creative Order QC Source Map

| Contract | Source |
| --- | --- |
| Deterministic batch QC accepts one or more manifest jobs and emits per-size technical evidence plus an unannotated final-image contact sheet | `references/qc_batch.py` |
| Visual QC judges the frozen copy snapshot, visual direction, actual Prime-composed image and recorded model prompt evidence before judging semantic theme and hierarchy | `scripts/creative-platform-skills/ad-creative-qc/SKILL.md:Lane 合同`, `server/internal/handler/creative_domain.go:validateCompletedGeneratedAssetTrace` |
| QC accepts only the current v6 full-template package, automatic family selection, the unchanged full-canvas alpha compose evidence, and the per-size layout contract; optional QR decoding is recorded as non-blocking evidence when present | `references/qc_batch.py:package_contract_failures`, `references/qc_batch.py:compose_item_succeeded`, `references/qc_batch.py:decode_qr_evidence`, `references/qc_batch.py:resolve_layout_contract` |
| Attachment downloads use the authenticated CLI and honor `MULTICA_HTTP_TIMEOUT`; QC deliberately serializes Prime image, manifest, and compose downloads at the Skill orchestration layer | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload`, `server/internal/cli/client.go:AtLeastAPITimeout` |
| `multica creative order qc-put` writes one report per variant, lane, and revision | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderQC` |
| `multica creative order qc-finalize` waits for both independent reports, then delivers a clean package or creates an action-required resolution | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCFinalize`, `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| A structured final visual finding for actual Prime obstruction or unreadable official Prime text queues one next-revision production task; technical and unstructured failures remain manual | `server/internal/handler/creative_domain.go:creativeVisualModelReworkFindings`, `server/internal/handler/creative_domain.go:queueCreativeVisualModelRework` |
| Native QC tasks are distinct by evidence pair plus lane/revision `item_key` | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:directTaskItemKey` |

Reconfirm these paths before changing the QC release contract.
