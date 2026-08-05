# Creative Order QC Source Map

| Contract | Source |
| --- | --- |
| Deterministic batch QC accepts one or more manifest jobs and emits per-size machine evidence plus visual sheets | `references/qc_batch.py` |
| Archived Prime jobs are reusable only when size/revision and a valid per-size layout contract can be recovered without conflict; QR is independently retried on the padded hard-region crop | `references/qc_batch.py:package_contract_failures`, `references/qc_batch.py:resolve_layout_contract`, `references/qc_batch.py:decode_qr_evidence` |
| Attachment downloads use the authenticated CLI and honor `MULTICA_HTTP_TIMEOUT`; QC deliberately serializes Prime image, manifest, and compose downloads at the Skill orchestration layer | `server/cmd/multica/cmd_attachment.go:runAttachmentDownload`, `server/internal/cli/client.go:AtLeastAPITimeout` |
| `multica creative order qc-put` writes one report per variant, lane, and revision | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCPut`, `server/internal/handler/creative_domain.go:UpsertCreativeOrderQC` |
| `multica creative order qc-finalize` is the transaction boundary for both lanes, delivery assets, variant state, and Inbox | `server/cmd/multica/cmd_creative_domain.go:runCreativeOrderQCFinalize`, `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| Blocking findings are evaluated server-side when finalizing so an invalid warning status cannot release delivery | `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| Native QC tasks are distinct by evidence pair plus lane/revision `item_key` | `server/internal/service/task.go:EnqueueDirectTaskFanout`, `server/internal/service/task.go:directTaskItemKey` |

Reconfirm these paths before changing the QC release contract.
