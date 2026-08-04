# Creative Leadership Source Map

| Contract | Source |
| --- | --- |
| Native task fanout has no persisted Task Batch or Work Unit and uses caller-owned evidence plus item keys | `server/internal/service/task.go:DirectTaskFanout`, `server/internal/service/task.go:EnqueueDirectTaskFanout` |
| Active fanout idempotency is target Agent plus evidence pair plus item key | `server/internal/service/task.go:isUniqueTaskFanoutViolation`, `server/internal/service/task.go:directTaskItemKey` |
| `multica task by-source list` exposes all tasks for reconciliation at an evidence source | `server/cmd/multica/cmd_task.go`, `server/internal/handler/task_fanout.go` |
| `qc-finalize` is the transaction boundary for two lanes, delivery registration, variant status, and Inbox creation | `server/internal/handler/creative_domain.go:FinalizeCreativeOrderQC` |
| Direct-edit initialization creates one Creative Order/Issue path and freezes the requested source, size, delivery mode, and agent snapshot | `server/internal/handler/creative_direct_edit.go:CreateCreativeDirectEdit` |

The stage owner and human-visible Issue policy are specified in `collaboration-contract.md`.
