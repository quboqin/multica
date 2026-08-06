---
name: coordinate-ad-creative-squad
description: "当 Creative Order 需要通过 Multica 原生 task 协调方案、标准生产或直接改图、Prime、双路 QC、异常恢复和最终用户通知时使用。"
allowed-tools: Bash(multica *)
---

# 协作广告素材小队

读取 `references/collaboration-contract.md`。Leader 只判断、委派、恢复和汇总；不执行分析、规划、图片生成、
Prime 或 QC。

## 每次唤醒

1. 从当前 Issue metadata 读取 `creative_order_id`，执行
   `multica creative order get <order-id> --output json`。缺少或错配时记录配置错误，不按标题猜订单。
2. 读取订单冻结 `squad_snapshot` 的 capability Agent IDs。只有 snapshot 明确缺字段且提供 `squad_id` 时，
   允许 `multica squad member list <squad-id> --output json` 补读；不得按 Agent 名称猜角色。
3. 从领域状态和关联 task 找出所有已满足依赖且缺少有效 task 的 item。幂等键为 target Agent + source pair +
   `item_key`；一个 source 下已有兄弟 item 不能阻止缺失项。
4. 按 target/source 分组，使用 `multica task fanout` 一次提交全部就绪项。提交后立即结束，不轮询、休眠或
   创建等待 Issue。

正常主链所有权固定：Leader 创建方案或 direct-edit task；Planner 创建 production；Production 创建 Prime；
Prime 创建 technical/visual QC；QC 调用 `qc-finalize`。Leader 只在人工重试或异常恢复时补真正缺失的下一步，
不得与下游重复委派。

## 标准订单

每个就绪 Order Item 创建一个 `creative_order_item_plan` source，item key 包含 item ID/revision，context 固定
`type=creative_domain_task`、`workflow=creative_plan`，并携带 issue/leader/order/item/candidate IDs、revision、
scope 及 planner/producer/prime/reviewer IDs。

```text
multica task by-source list --agent <planner-agent-id> \
  --kind creative_order_item_plan --ref <item-id> --output json
multica task fanout --agent <planner-agent-id> --input-file <manifest.json> --output json
```

文案推荐、编辑和市场资源选择在页面完成并冻结。Leader 只消费 `copy_snapshot` 和 market snapshot，不根据
竞品数字或当前资源草稿改写输入。V01-V03、三尺寸、Prime 和 QC 的详细合同由对应 Skill 负责。

## 直接改图

`input_snapshot.mode=direct_edit` 时只创建 `creative_order_item_direct_edit` task。snapshot 必须含
`direct_edit_agent_id`、`prime_agent_id`、`reviewer_agent_id`；context 携带固定 source asset/attachment、
用户原话、target/expected sizes、delivery mode 和 source revision。preview 到 generated 结束；publish 才继续
Prime 与双路 QC。不得触发采集、参考分析、三变体方案或标准生产。

## 恢复与用户留痕

失败项不阻塞兄弟对象。恢复前先检查领域输出、task status 和 item key，只补缺失尺寸/lane/revision；旧
revision 不得覆盖新 revision。只有用户明确操作才 retry failed 或创建最多一轮返工。

Issue 只记录订单启动、可交付 Variant、需要用户决定的真实阻塞和整单完成。过程证据写领域对象与 task。
整单所有 Variant completed 后才将 Issue 设为 done 并通知创建人；否则保持 todo。候选、文案、变体、成图、
QC 和标注的采用/拒绝/编辑必须追加 feedback event，不以评论代替。

任何委派、状态或输入契约失败都写真实 error code/message 和受影响对象；不得伪造进度，不创建分析、
变体、Prime 或 QC 子 Issue。
