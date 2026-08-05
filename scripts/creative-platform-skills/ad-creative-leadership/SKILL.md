---
name: coordinate-ad-creative-squad
description: "Coordinate a creative order through native Multica tasks while keeping one user-facing Issue, three coherent variants, independent quality review, and incremental delivery."
allowed-tools: Bash(multica *)
---

# 协作广告素材小队

只做判断、委派、状态收口和发布，不代替专业成员分析或生成图片。读取
`references/collaboration-contract.md` 并严格执行。

## 业务对象

- `Crawl Run` 记录一次采集和素材预分析，不创建 Issue。
- `Creative Order` 记录一次正式生产；每个订单只关联一个用户可见 Issue。
- `Source Analysis`、`Order Item`、`Variant`、`Asset` 和 `QC Report` 是创意领域数据。
- 原生 `agent_task_queue` task 是机器执行单位。不得创建 Task Batch、Work Unit、分析 Issue、变体
  Issue、Prime Issue 或 QC Issue。
- task 通过 `trigger_evidence_kind` 和 `trigger_evidence_ref_id` 关联 Crawl Run、订单项或变体。

Issue 只保留用户目标、补充想法、人工决定、真实阻塞和最终验收。模型输入、调用证据、逐图检查和
中间文件写入对应领域对象及 task，不在 Issue 评论中复制流水账。

## 每次唤醒

1. 从当前 Issue metadata 读取 `creative_order_id`，获取订单及其 items、variants、assets、QC 状态。
   缺少订单 ID 时报告配置错误，不按标题或子 Issue 猜测。
2. 查询本订单关联的原生 task，确认正在运行、已完成、失败和取消的项。委派幂等键固定为
   `target_agent_id + trigger_evidence_kind + trigger_evidence_ref_id + item_key`；其中 `item_key` 必须包含
   领域对象 ID、revision 和 lane/scope。一个 source 可以合法承载多个 item，不能只因 source 下已有一条
   task 就跳过其他就绪项。
3. 一次找出所有已满足依赖且尚无有效 task 的工作，按能力分组，通过
   `multica task fanout --agent <agent-id> --input-file <manifest.json> --output json` 批量委派。
4. 提交后立即结束。不得轮询、休眠、占住执行槽，也不得为等待中的机器步骤创建 Issue。
5. 同一订单项的 V01、V02、V03 独立推进；某个变体通过 QC 后立即登记并展示该 revision 的
   `expected_sizes` 成图，不等待兄弟变体。

标准订单的 Leader 直接委派每个 Order Item 的方案 task。后续使用同一可追溯委派链推进，避免让 Leader 轮询：

- 方案 task 写入 V01-V03 后，一次 fanout 三个 production task；
- 每个 production task 完成自己的 `expected_sizes` 后，fanout 自己 Variant 的 Prime task；
- Prime task 完成全部 `expected_sizes` 后，一次 fanout technical 与 visual 两个 QC task；
- 每条 QC task 写完自己的报告后调用 `multica creative order qc-finalize`；后完成者由服务端事务锁唯一
  收口该 Variant，QC task 不自行复制 delivered asset。

主链所有权是单向且唯一的：Leader 只创建方案 task 并在人工重试或异常恢复时做 reconciliation；Planner
只创建本 Order Item 缺失的 production task；每个 Production task 只创建本 Variant/revision 的 Prime task；
Prime 只创建本 Variant/revision 缺失的 QC lane。任何成员都不得越级重建其他阶段。Leader 被再次唤醒时
只补齐根据上述复合幂等键确实缺失的 task，不与下游成员争抢正常推进权。

下游 agent ID 必须来自订单冻结的 squad snapshot 或当前 task context，不能按名称猜测。每次 fanout 前先
用 `multica task by-source list` 查询目标 agent、evidence kind/ref；存在 active 或成功 task，或领域对象
已达到下一阶段时不再委派。整条链通过 `delegated_from_task_id` 复制最初 Leader task 的人工归因。

## Fanout 契约

manifest 使用以下结构：

```json
{
  "trigger_evidence_kind": "creative_order_item_plan",
  "trigger_evidence_ref_id": "<order-item-id>",
  "items": [
    {
      "item_key": "<order-item-id>:r1",
      "context": {
        "type": "creative_domain_task",
        "workflow": "creative_plan",
        "creative_order_id": "<order-id>",
        "issue_id": "<root-order-issue-id>",
        "leader_agent_id": "<frozen-leader-agent-id>",
        "creative_order_item_id": "<item-id>",
        "candidate_id": "<candidate-id>",
        "revision": 1,
        "scope": "order_item",
        "producer_agent_id": "<frozen-producer-agent-id>",
        "prime_agent_id": "<frozen-prime-agent-id>",
        "reviewer_agent_id": "<frozen-reviewer-agent-id>"
      }
    }
  ]
}
```

同一次调用中的 `trigger_evidence_kind` 和 `trigger_evidence_ref_id` 是一组任务的来源证据。不同对象需
要不同 source 时，生成不同 manifest。重复唤醒先按 source 查询，再逐项比较 `item_key`；同一 source
已有一个 item 不代表其他 item 已创建。只有用户明确重试失败项时才使用 `retry-failed`；取消只影响尚未
完成的 task。

订单 task context 必须从订单和当前 Issue 携带并逐级原样透传 `issue_id`、`leader_agent_id`，以及领域
对象 UUID、修订、作用域和输入快照版本。不得只传自然语言标题，也不得
依赖当前 Issue 评论推断候选、文案、尺寸或附件。

## 阶段一：方案

每个就绪 Order Item 委派一个 `creative_plan` task 给生成方案智能体。输入必须包含：

- Source Analysis ID 和版本；
- 已确认主题、用户补充想法；
- 已选或人工编辑的文案快照；
- 市场资源包、品牌资源和 App UI 引用快照；
- 原图附件与必须保留、允许变化项。

先从订单 `input_snapshot.squad_snapshot` 读取小队和角色 ID；缺失时可用其中的 `squad_id` 调用
`multica squad member list <squad-id> --output json` 补齐一次，并把实际使用的角色映射写回任务 context。
用户选择的小队中缺少方案、图像编辑、完整贴图或质量验收角色时，订单进入 `action_required` 并提示
缺少的角色，不得按智能体名称猜测。对每个 Order Item 查询/创建一个
`creative_order_item_plan` source；manifest 采用上面的结构，目标是冻结的方案智能体 ID。

方案固定创建 V01、V02、V03 三个差异明确的变体。三个变体共享同一业务语义、批准文案和合规
事实，只允许改变版式、信息组织和视觉表达。方案角色把结构化规格写入 Variant，不把 Markdown
附件当成唯一交付物。

文案检索、相似推荐、差异说明、用户选择和人工编辑都发生在页面确认阶段，并在创建订单前冻结为
`copy_snapshot`。Leader 和 Planner 只消费该快照；不得在设计阶段重新推荐、替换、拼接或根据竞品数字
改写文案。快照缺少必要事实是输入错误，才进入 `action_required`。

竞品信息机制只在 `copy_snapshot` 可表达的范围内转译。参考图中的问题、选项、按钮或标签没有已批准
对应文本时，不得要求 Planner 把它们列为硬性必现内容，也不得因此回头阻塞用户；Planner 应把获批标题
和利益点映射成兼容的结构关系，并明确记录被省略的未批准竞品文字。

## 阶段二：三变体生成

三个 Variant 的 `creative_production` task 一次 fanout，允许并发。每个 task 内：

1. 先生成 `1080x1080` 无品牌方形母版；
2. 方形母版可用后，立即以它为第一参考并发生成 `1200x628` 和 `800x1000`；
3. 不要求方形先经过 Prime 或独立 QC；
4. 三尺寸必须引用同一个 `asset_family_id`，保存 `derived_from_asset_id`、提示词版本、模型请求 ID、
   尺寸和修订；
5. 横竖版是原生重排，不得裁切、拉伸、加边或重新发明内容；人物、产品、批准文案、金额、业务
   语义和关键信息必须与方形一致。

某个尺寸失败只把该 Asset 标为失败；另外两个尺寸和兄弟变体继续。恢复时只重提缺失尺寸，禁止
重做已有可用资产。

## 阶段三：Prime

标准 Variant 的三个 `expected_sizes` 无品牌底图齐备后，委派一个 `creative_prime` task。直接改图的
`expected_sizes` 是用户本次发布的受影响尺寸，可以是 1-3 张。Prime 在一次批处理中只处理该 task 明确
给出的尺寸，使用版本化品牌、条款、二维码和商店徽章资产。每张结果都保存原始底图、模板、合成
manifest、输出附件和机器校验证据；不得要求 direct edit 补造未受影响的尺寸。

底图进入 Prime 引导区不构成失败。只有最终合成图实际遮挡获批关键内容、监管资产不可读或二维码
不可解码，才进入 QC 阻断结论。

## 阶段四：独立 QC 并发

当前 revision 的全部 `expected_sizes` Prime 成图齐备后，同时委派两类只读 QC task：

- `creative_qc_technical`：检查尺寸、文件完整性、四角/底部 Prime 资产、二维码可解码、硬区遮挡；
- `creative_qc_visual`：检查清晰度、伪影、文案可读性、原图语义和变体规格保持；多尺寸时再检查内容一致性。

两类 QC 使用相同 Variant、revision、`expected_sizes` 和最终图集合，但写入独立 QC Report。标准生产
检查三尺寸一致性；direct edit 单尺寸只检查该尺寸及其来源保持，多尺寸才做跨尺寸一致性。聚合规则：任一
`blocking_failures` 非空则该变体 `action_required`；只有两份报告都完成且无阻断才通过。工具异常只
标记对应检查失败，不覆盖另一份报告，也不阻塞兄弟变体。

`qc-finalize` 是唯一发布 barrier：两 lane 未齐返回 `pending`；通过时按 `expected_sizes` 原子登记 delivered asset；
失败时只进入 `action_required`，不得自动返工。每个 Variant/revision 只有一个 resolution 和一条 Inbox。

四角检查以实际 Prime 成图为准，不以原参考图人物或装饰进入模板矩形为失败。原图本来就有的构图
不能仅因几何预测被拦截。

## 返工

返工不是自动硬门槛。QC 阻断后先向用户展示原图/成图、问题区域和建议作用域，由用户选择接受、
局部调整或放弃。每个 Variant 最多一轮返工：

- `size`：只处理一张尺寸；
- `variant_subset`：同一变体的一组尺寸；
- `variant`：标准生产为同一变体三张，direct edit 为本次 `expected_sizes`；
- `replan`：只重做指定变体方案，再生产该变体。

当 QC 证据表明失败来自 brief 同时“要求出现某参考文字”又“禁止新增该文字”时，用户选择调整后必须
使用 `replan`，由 Planner 先修订 `mechanism_adaptation`，不能把相同冲突提示词直接交给 Producer 重试。

页面调整评论必须同时提供 `creative_order_id`、`creative_order_item_id`、`variant_id`、`asset_id`、
`size_key` 和 `revision`。先用 `creative order get` 核对这些 ID 属于同一订单链路，再创建返工 task；任一
字段缺失或不一致时停止并请求补全。`V01`/`V02`/`V03` 只用于展示，同一订单的不同 item 会重复，绝不能
按 `variant_key`、评论顺序或第一个同名变体猜目标。

普通返工以对应尺寸上一版无品牌底图为第一输入；`replan` 以原候选图为第一输入。不得重做无关变体
和已接受尺寸。返工后的 Prime 与两类 QC 仍使用 task，revision 加一。第二轮仍失败时停止模型调用，
保留已通过资产并等待用户决定。

## 发布与用户状态

某个 Variant 的两份 QC 报告通过后，立即把该 revision 的 `expected_sizes` Asset 登记为可交付并更新订单聚合状态。结果页必须
能从每张成图回到候选原图、Source Analysis、Variant 规格、方形母版和 Prime/QC 证据。

只有以下事件写入订单 Issue 评论，且每个阶段按订单项、变体、修订去重：

1. 订单开始生产；
2. 某个变体已交付该 revision 的 `expected_sizes` 成图；
3. 需要用户决定的真实阻塞；
4. 整个订单完成。

QC 的事务化 finalize 会给订单创建人发送原生 Inbox 提醒。收到整单完成或真实阻塞的唯一收口评论后，
Leader 只汇总一次订单状态；完成时在最终评论中明确 @订单创建人并附结果看板入口，阻塞时只列需要
业务选择的变体和作用域。不得因单条 QC task 完成重复唤醒或逐条提醒。

Leader 只有在回读确认全部 Variant 均为 `completed` 时才把订单 Issue 更新为 `done`。存在
`action_required`、运行中或缺失结果时保持 `todo`；不得因收到一条成功通知提前关闭 Issue。

排队、上下文准备、单个 task 完成、内部重试和工具日志不写评论。页面从领域状态展示实时进度，不
要求业务用户进入 task 详情或子 Issue。

用户对候选、文案推荐、变体、成图和 QC 的采用、拒绝、替换、编辑、误判、漏检以及图片区域标注
必须写入追加式反馈事件；当前状态可以更新，但历史事件不得覆盖或删除。用户在高清对比工作区确认
后，订单才进入最终 `accepted`。

## 直接改图

当订单 `input_snapshot.mode=direct_edit` 时，冻结的 squad snapshot 必须同时包含 `direct_edit_agent_id`、
`prime_agent_id` 和 `reviewer_agent_id`。Leader 只能把首次任务委派给 direct edit 角色，后两者仅供它续链；
不得选择图像编辑、方案、分析或采集角色，也不得按名称猜 Agent。缺少任一 ID 时进入 `action_required`，
不能创建一个注定无法完成 Prime/QC 的 direct task。

使用一个 `creative_order_item_direct_edit` task，context 固定携带 `creative_order_id`、`issue_id`、
`leader_agent_id`、`creative_order_item_id`、`variant_id`、`source_asset_id`、`source_attachment_id`、
`user_request`、`target_size`、`expected_sizes: [target_size]`、`delivery_mode`、`prime_agent_id`、
`reviewer_agent_id` 与 `revision`。这里的 `revision`
是 source revision，直接改图只生成一个指定尺寸的 `output_revision=revision+1`：
源资产保持不可变，输出必须以源资产为 `derived_from_asset_id`，不得改写 revision N 的 source asset。

`delivery_mode=preview` 只保留 generated asset，不委派 Prime 或 QC，也不标记正式交付。`delivery_mode=publish`
在 output revision 的 generated asset 完成后，只为该 variant 的 `expected_sizes` 委派一个 Prime task；Prime 完成后并发委派
technical 与 visual QC，并按普通 QC finalize 收口。不得触发采集、参考分析、三变体规划或普通生产。
