# Creative Leadership Source Map

| Contract | Source |
| --- | --- |
| Native task fanout has no persisted Task Batch or Work Unit and uses caller-owned evidence plus item keys | `server/internal/service/task.go:DirectTaskFanout`, `server/internal/service/task.go:EnqueueDirectTaskFanout` |
| Active fanout idempotency is target Agent plus evidence pair plus item key | `server/internal/service/task.go:isUniqueTaskFanoutViolation`, `server/internal/service/task.go:directTaskItemKey` |
| `multica task by-source list` exposes all tasks for reconciliation at an evidence source | `server/cmd/multica/cmd_task.go`, `server/internal/handler/task_fanout.go` |
| Plan fanout requires `creative_plan`, a matching `creative_order_item_id`, and order trace UUIDs | `server/internal/handler/task_fanout.go:validateCreativeTaskFanoutContext` |
| `qc-finalize` records two lanes, blocks delivery for structured Prime/content defects, and queues at most one bounded Variant rework | `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| Creative Order API returns every frozen order input, including `squad_snapshot`, under `input_snapshot` | `server/internal/handler/creative_domain.go:GetCreativeOrder`, `server/internal/handler/creative_domain.go:creativeOrderResponse` |
| Direct-edit initialization creates one Creative Order/Issue path and freezes the requested source, size, delivery mode, and agent snapshot | `server/internal/handler/creative_direct_edit.go:CreateCreativeDirectEdit` |

The stage owner and human-visible Issue policy are specified in `collaboration-contract.md`.
