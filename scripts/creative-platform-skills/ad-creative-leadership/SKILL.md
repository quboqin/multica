---
name: coordinate-ad-creative-squad
description: "当 Creative Order 需要通过 Multica 原生 task 协调方案、标准生产或直接改图、视觉质检、异常恢复和最终用户通知时使用。"
allowed-tools: Bash(multica *)
---

# 协作广告素材小队

读取 `references/collaboration-contract.md`。Leader 只判断、委派、恢复和汇总；不执行分析、规划、图片生成、
品牌组件或 QC。

## 每次唤醒

1. 从当前 Issue metadata 读取 `creative_order_id`，执行
   `multica creative order get <order-id> --output json`。缺少或错配时记录配置错误，不按标题猜订单。
   命令返回的冻结小队位于 `input_snapshot.squad_snapshot`，不是订单顶层字段。PowerShell 必须按以下方式读取：

   ```powershell
   $order = multica creative order get <order-id> --output json | ConvertFrom-Json
   $squad = $order.input_snapshot.squad_snapshot
   ```

2. 从 `$squad` 读取冻结 capability Agent IDs。只有 snapshot 明确缺字段且提供 `squad_id` 时，
   允许 `multica squad member list <squad-id> --output json` 补读；不得按 Agent 名称猜角色。
3. 从领域状态和关联 task 找出所有已满足依赖且缺少有效 task 的 item。幂等键为 target Agent + source pair +
   `item_key`；一个 source 下已有兄弟 item 不能阻止缺失项。
4. 按 target/source 分组，使用 `multica task fanout` 一次提交全部就绪项。提交后立即结束，不轮询、休眠或
   创建等待 Issue。

正常主链所有权固定：Leader 创建标准订单方案；初始 direct-edit revision 和 task 由平台建单事务原子创建，Leader 不重复委派；Planner 按冻结数量建立候选并委派主尺寸 production；Production 只完成候选主尺寸和 Prime；
QC 的 `creative_candidate_selection` 按冻结目标原子晋级，平台自动为全部 selected 补排缺失尺寸；selected 三尺寸 Prime 齐备后，QC 做联合视觉终检并调用
`qc-finalize` 完成归档。真实遮挡或官方文字不可读会阻断当前尺寸并触发有界定向返工，关键内容缺失仍然阻断并转人工确认。Leader 只在人工重试或异常恢复时补真正缺失的下一步，
不得与下游重复委派。

## 标准订单

从订单 `input_snapshot.target_variant_count` 读取交付套数 N，`candidate_count` 读取候选数 K=N+2；历史订单未保存时使用 N=3、K=5。
新文案库订单支持 N=1-10，10 套对应 12 个候选、10 个 selected、30 张最终交付图。每套仍独立生产、质检和入库，按现有并发上限排队。
只有 N 套各自完成冻结尺寸后才汇总完整交付；失败只恢复缺失套/尺寸，最多使用两个候补，不把部分成功或候补耗尽当成全单完成。

先读取每个 item 的 `source_kind`。`copy_library` 来源直接进入方案；将真实 `source_kind`、`copy_library_id` 放入 task context，
省略空的 candidate/source-analysis IDs，不采集、不分析、不预适配、不创建占位素材。`material` 来源沿用冻结的素材和分析 ID。
文案来源仍使用相同的 item-plan、候选生产、晋级、三尺寸制作和 QC task；不得另建一条执行流程。

每个就绪 Order Item 单独提交一个 `creative_order_item_plan` fanout；一个 manifest 的 source ref 必须是该 item ID，
不能把不同 item 放入同一个 manifest。item key 固定为 `<item-id>:r1`，context 固定
`type=creative_domain_task`、`workflow=creative_plan`，并携带 issue/leader/order/item/candidate/source-analysis IDs、
`revision: 1`、`scope: item` 及 planner/producer/reviewer IDs。

```text
multica task by-source list --agent <planner-agent-id> \
  --kind creative_order_item_plan --ref <item-id> --output json
multica task fanout --agent <planner-agent-id> --input-file <manifest.json> --output json
```

`manifest.json` 必须为以下结构。每个 item 使用自身的冻结 ID，`input_snapshot`、`copy_snapshot` 和 `direction`
不复制进 task context；Planner 以这些 ID 回读同一订单的冻结输入：

```json
{
  "trigger_evidence_kind": "creative_order_item_plan",
  "trigger_evidence_ref_id": "<item-id>",
  "items": [{
    "item_key": "<item-id>:r1",
    "context": {
      "type": "creative_domain_task",
      "workflow": "creative_plan",
      "creative_order_id": "<order-id>",
      "creative_order_item_id": "<item-id>",
      "candidate_id": "<candidate-id>",
      "source_analysis_id": "<source-analysis-id>",
      "revision": 1,
      "scope": "item",
      "issue_id": "<issue-id>",
      "leader_agent_id": "<leader-agent-id>",
      "planner_agent_id": "<planner-agent-id>",
      "producer_agent_id": "<producer-agent-id>",
      "reviewer_agent_id": "<reviewer-agent-id>"
    }
  }]
}
```

提交前用 JSON parser 校验 manifest。CLI 返回的 `tasks` 必须包含新 task ID；否则记录真实错误并让当前任务失败。

文案推荐、编辑和市场资源选择在页面完成并冻结。Leader 只消费 `copy_snapshot` 和 market snapshot，不根据
竞品数字或当前资源草稿改写输入。候选数量、按冻结目标晋级、selected 三尺寸、品牌组件和 QC 的详细合同由对应 Skill 负责。

## 直接改图

`input_snapshot.mode=direct_edit` 的初始 R1 source、R2 staging revision 和 `creative_order_item_direct_edit` task 已由
`CreateCreativeDirectEdit` 在同一事务内创建。Leader 正常情况下不得再创建初始 task，也不得把 Issue 的小队分配当成再次启动信号。
snapshot 必须含 `direct_edit_agent_id`、`reviewer_agent_id`；task context 携带固定 source asset/attachment、用户原话、
target/expected sizes、delivery mode、`revision` 和 `source_revision`。

只有人工重试或异常恢复时，才能补真正缺失的 direct-edit task。恢复前必须同时确认：Variant 当前 `revision` 与
`staging_revision` 一致、`source_revision` 为不小于 1 且严格小于 target `revision` 的当前 active source（正常时为 `revision - 1`；若中间 staging 已废弃则保持 active source）、source asset/attachment 属于该 Variant 的
source revision 和目标尺寸、当前 item key 没有 active/succeeded task、当前 revision 也没有已经登记的有效 generated 输出。
恢复 task 必须沿用当前 revision/source_revision 和现有 source lineage，不能新建 revision、不能从旧 R1 壳或 Issue 文本猜字段，
也不能因通知遗漏而重复调用图像模型。preview 到 generated 后由平台收敛 revision；publish 由后端重新执行确定性品牌组件合成并进入最终视觉 QC，
只有 QC 归档后才 delivered。不得触发采集、参考分析、候选方案或标准生产。

## 恢复与用户留痕

失败项不阻塞兄弟对象。恢复前先检查领域输出、task status 和 item key，只补缺失尺寸/lane/revision；旧
revision 不得覆盖新 revision。平台只按结构化证据自动创建有上限的尺寸续跑、视觉返工和首次交付候补晋级，Leader 不重复创建这些任务；
除此之外只有用户明确操作才 retry failed。已有 active revision 始终继续在线，未通过的 staging revision 不能覆盖它。

Issue 只记录订单启动、可交付 Variant、需要用户决定的真实阻塞和整单完成。过程证据写领域对象与 task。
整单所有 Variant completed 后才将 Issue 设为 done 并通知创建人；否则保持 todo。候选、文案、变体、成图、
QC 和标注的采用/拒绝/编辑必须追加 feedback event，不以评论代替。

任何委派、状态或输入契约失败都写真实 error code/message 和受影响对象；不得伪造进度，不创建分析、
变体、品牌组件或 QC 子 Issue。
