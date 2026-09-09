# 素材流程的人工继续与自动恢复

人工点击「继续」「重试」代表一次新的执行授权，不消耗已经结束的自动恢复额度。
历史失败、任务关联及已生成图片保留；自动补偿和诊断任务继续遵守各自的次数限制。

| 入口 | 人工行为 | 实现位置 |
| --- | --- | --- |
| 继续规划候选 | 追加新任务和本轮预算，保留父任务次数 | `creative_candidate_progress.go`, `creative_manual_retry.go` |
| 继续筛选候选 | 不受此前三次自动筛选限制；复用执行中任务 | `creative_candidate_orchestration.go`, `creative_candidate_progress.go` |
| 出图／改图工作流重试 | 追加新任务，恢复按钮不被历史次数隐藏 | `creative_domain.go`, `creative_order_batch.go`, `creative_variant_batch.go` |
| 创意任务按来源重试 | 真人追加新任务；诊断智能体仍走有界恢复 | `task_fanout.go` |
| 再次质检 | 历史 `qc_recovery_used` 不隐藏按钮；完整贴片、当前版本和执行中检查仍有效 | `creative-order-delivery.tsx`, `RetryCreativeOrderVariantQC` |
| 重试贴片 | 显式发送 `force: true`；运行中的合成不被覆盖 | `creative-studio-page.tsx`, `resetCreativePrimeCompositionJob` |
| 重新分析／预适配 | 已有实现会新建任务；继续保留防重和权限检查 | `creative_manual_analysis.go`, `creative_pre_adaptation.go` |
| 重新归档 | 已有实现重置本轮归档尝试 | `creative_domain.go` |

新人工任务的 `attempt` 沿历史递增，`max_attempts` 为新 attempt 加一，允许本轮一次基础设施重试；
原任务的次数和上限不修改。`parent_task_id`、`retry_of_task_id`、`rerun_of_task_id` 关联原任务，
发起者记录为当前真人，并使用新会话。候选筛选使用独立的新筛选任务和其已有队列预算。

人工入口仍拒绝已取消对象、过期任务／版本、未齐备的输入和越权请求。重复点击不追加并行任务。
人工重试不会自动授权重复调用结果未知的图片操作；模型调用回执、视觉返工次数和版本激活规则保持有效。

回归覆盖六套／八候选、自动三次耗尽后继续、规划及来源任务十二次耗尽后继续、旧任务防重、
取消订单、原始预算不变、图片保留和再次质检。测试使用独立数据库，不点击真实订单恢复按钮。
