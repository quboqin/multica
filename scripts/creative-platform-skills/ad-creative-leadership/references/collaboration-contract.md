# 创意小队协作契约

## 边界

Multica 的原生智能体通过 `agent_task_queue` 执行工作。创意领域对象保存业务状态，Issue 保存需要
人参与的目标和验收。两者不能互相冒充。

- Crawl Run、Source Analysis、Order Item、Variant、Asset、QC Report 都不是 Issue。
- 不新增 Task Batch 或 Work Unit。
- 一个正式 Creative Order 最多一个用户可见 Issue。
- 采集、逐图分析、方案、生成、Prime、QC 不创建子 Issue。
- task 详情可供排障和审计，但默认不占用业务看板。

## 原生 task 来源

每个 task 都必须有稳定来源：

| 阶段 | `trigger_evidence_kind` | `trigger_evidence_ref_id` |
| --- | --- | --- |
| 参考分析 | `creative_crawl_run_analysis` | Crawl Run UUID；候选在 `item_key` |
| 方案 | `creative_order_item_plan` | Order Item UUID |
| 生成 | `creative_order_item_production` | Order Item UUID；Variant 在 `item_key` |
| Prime | `creative_order_variant_prime` | Variant UUID |
| 技术与视觉 QC | `creative_order_variant_qc` | Variant UUID；lane 在 `item_key` |
| 直接改图 | `creative_order_item_direct_edit` | Order Item UUID |

上表是原生 task 的来源类型。Source Analysis 领域对象写回时使用
`trigger_evidence_kind=crawl_run`，引用同一个 Crawl Run UUID；两类字段不可混用。

委派幂等键是目标 Agent、source pair（`trigger_evidence_kind` + `trigger_evidence_ref_id`）与
`item_key` 的组合。同一 source 可以包含多个并行 item；每次补任务都必须逐项比较 `item_key`，不能用
“source 下已有 task”替代。重试失败项创建带 `retry_of_task_id` 的新 task，取消只作用于未完成 task。
Agent 委派时复制 `delegated_from_task_id`、顶层人工 originator 和 accountable human；
成员直接发起时按工作区权限写入本人归因。

所有订单阶段的 context 必须逐级原样携带根订单 `issue_id` 与冻结的 `leader_agent_id`。后端按 workflow
校验这两个 UUID 及 source 对象；任何一层遗漏都拒绝整批 fanout，不创建部分任务。

方案、生产和 Prime 成员可以按订单冻结的 squad agent ID 委派下一阶段。它们不能改换负责人，也不能
按名称猜 Agent。这些 A2A task 仍属于最初 Leader 的委派链；专业成员只创建契约中的下一阶段，不得
越级创建其他业务。

直接改图订单的 `input_snapshot.mode` 为 `direct_edit`，冻结 snapshot 必须包含
`direct_edit_agent_id`、`prime_agent_id` 和 `reviewer_agent_id`。Leader 只能把
`creative_order_item_direct_edit` 委派给 direct edit Agent；不得复用
生产、方案、分析或采集角色。其 context 还必须携带 source asset/attachment、用户原话、目标尺寸、
`expected_sizes=[target_size]`、delivery mode、Prime Agent ID 和 Reviewer Agent ID。context revision 表示
source revision；source asset 不可覆盖，
编辑输出使用 `output_revision=revision+1` 并以 source asset 写入
`derived_from_asset_id`。preview 不进入 Prime/QC；publish 只进入对应尺寸的 Prime 与双路 QC。

## Fanout

使用 `multica task fanout`，不要用 mention 或批量创建 Issue。manifest 示例：

```json
{
  "trigger_evidence_kind": "creative_crawl_run_analysis",
  "trigger_evidence_ref_id": "<run-id>",
  "items": [
    {
      "item_key": "<candidate-id>:v1",
      "context": {
        "type": "creative_domain_task",
        "workflow": "creative_reference_analysis",
        "crawl_run_id": "<run-id>",
        "candidate_id": "<candidate-id>",
        "analysis_version": 1
      }
    }
  ]
}
```

平台必须原子验证整个 manifest 的工作区、目标 Agent、调用权限、source 和 item key；某一项格式错误
时不创建部分任务。合法任务可在 Agent 的 `max_concurrent_tasks` 与运行时总并发限制内并发认领。
创意图片调用还受独立 provider image slot 限制，不能用提高 Agent task 并发绕过 429 保护。

## 状态聚合

状态由领域对象和关联 task 派生，不由评论文字派生：

- 任一 item queued/running 时，阶段为 running；
- 成功和失败并存时为 partial；
- 所有 item 成功时为 completed；
- 所有可运行项失败时为 failed；
- 需要凭证、配置或用户决定时为 action_required；
- 用户取消时为 cancelled。

失败项不阻塞同批其他对象。已完成的变体即时发布，订单可以长期保持 partial。迟到 task 只能写入
自己声明的 revision；如果当前领域对象已有更高 revision，服务端拒绝旧结果覆盖。

## 生产一致性

标准生产的每个 Variant 是一个内容族，`expected_sizes` 固定为 square、landscape、portrait 三个 Asset；
direct edit 的 `expected_sizes` 固定为本次用户发布的受影响尺寸，可以只有一个：

- square 是内容母版；横竖版保存 `derived_from_asset_id=square`；
- 三张共享批准文案、业务语义、主要人物/产品、信息层级和 `asset_family_id`；
- 尺寸变化通过原生重排完成，不得裁切、拉伸或引入不同场景；
- Prime 只加入版本化四角/底部资产，不修改创意底图内容。

Prime 和 QC 只要求当前 revision 的 `expected_sizes` 完整，不要求 direct edit 生成未受影响尺寸。技术 QC
与视觉 QC 并发。技术 QC 重点检查四角、底部、二维码、尺寸和文件；视觉 QC 重点检查画质、文字、语义，
并在 `expected_sizes` 多于一个时检查跨尺寸一致性。两份报告都无阻断才通过。

## 人工反馈

以下操作必须在发生时追加 feedback event：

- 素材采用、拒绝、恢复、归档；
- 文案推荐曝光、采用、替换、人工编辑及原因；
- 变体接受、调整、放弃；
- 成图接受、下载、报告问题、图片点位或矩形标注；
- QC 漏检、误判和用户接受风险。

事件保存 subject、decision、reason codes、自由说明、相对坐标标注、用户、时间、关联 Crawl Run/
订单/分析/推荐/资源/Skill/修订快照。撤销通过新事件引用原事件，不物理删除历史。

## 用户可见内容

业务页面显示 Crawl Run 进度、候选分析、文案选择、订单状态、原图与成图高清对比、问题标注和下载。
订单 Issue 只显示摘要和跳转。只有需要用户决策的阻塞才通知用户，内部 task 失败保留在执行详情中并
允许按 source 重试。
