# Creative Leadership Source Map

| Contract | Source |
| --- | --- |
| Native task fanout has no persisted Task Batch or Work Unit and uses caller-owned evidence plus item keys | `server/internal/service/task.go:DirectTaskFanout`, `server/internal/service/task.go:EnqueueDirectTaskFanout` |
| Active fanout idempotency is target Agent plus evidence pair plus item key | `server/internal/service/task.go:isUniqueTaskFanoutViolation`, `server/internal/service/task.go:directTaskItemKey` |
| `multica task by-source list` exposes all tasks for reconciliation at an evidence source | `server/cmd/multica/cmd_task.go`, `server/internal/handler/task_fanout.go` |
| Plan fanout requires `creative_plan`, a matching `creative_order_item_id`, and order trace UUIDs | `server/internal/handler/task_fanout.go:validateCreativeTaskFanoutContext` |
| Planning follows frozen candidate_count; candidate selection atomically promotes target_variant_count directions and platform orchestration queues selected missing sizes without a duplicate Leader fanout | `server/internal/handler/creative_variant_lifecycle.go:SelectCreativeOrderItemCandidates`, `scripts/creative-platform-skills/ad-creative-leadership/SKILL.md:标准订单` |
| `qc-finalize` waits only for the visual lane and can queue at most two bounded size-specific Variant rework attempts; the technical lane is retired | `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC`, `server/internal/handler/creative_domain.go:creativeVisualModelReworkMaxAttempts` |
| Creative Order API returns every frozen order input, including `squad_snapshot`, under `input_snapshot` | `server/internal/handler/creative_domain.go:GetCreativeOrder`, `server/internal/handler/creative_domain.go:creativeOrderResponse` |
| Direct-edit initialization atomically creates the R1 source lineage, R2 staging revision, pooled runtime task, and Issue trace; Leader only repairs a truly missing current-revision task | `server/internal/handler/creative_direct_edit.go:CreateCreativeDirectEdit`, `server/internal/handler/task_fanout.go:validateCreativeDirectEditTaskContext` |

The stage owner and human-visible Issue policy are specified in `collaboration-contract.md`.
