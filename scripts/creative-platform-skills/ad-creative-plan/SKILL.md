---
name: multica-ad-creative-plan
description: "当 Creative Order Item 已冻结参考分析、业务选择、文案快照和市场资源快照，需要规划 V01-V03 并委派标准生产时使用。"
allowed-tools: Bash(multica *)
---

# 广告生成方案

只处理 task context 指定的 Order、Order Item、candidate 和 revision。

```text
multica creative order get <order-id> --output json
```

订单只保存 `source_analysis_id` 而未内嵌结果时，允许按 candidate 回读一次：

```text
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

除此之外不探测素材命令，不读旧 Issue 流程，不从标题、评论或本机文件补输入。

## 输入真值

- `copy_snapshot`：页面预适配后冻结的唯一可见文案、还款计划和用户视觉方向。`pre_adaptation.text_replacements` 是每个普通
  画面文字区块的权威替换表；`pre_adaptation.numeric_layouts`、`repayment_plan_entries`、
  `repayment_plan_selections.values` 和 `text_replacements[].calculation.result` 都是数值区域的权威版式与数值。
  这里不保存模型提示词；生产 Agent 根据这些结构化事实和当前尺寸自行写短提示词。
- `copy_snapshot` 的顶层非空字段（`headline`、`subheadline`、`benefit`、`supporting`、`cta`、`legal_text`）优先级最高，必须逐字
  进入每个 Variant 的 `approved_copy`。`pre_adaptation.text_replacements` 只负责把源区块映射到这些字段或补充独立结构；当两者冲突时，不能用
  `recommended` 的替换文本覆盖顶层非空字段。只有顶层字段本身为空，或页面明确将该字段置空，才允许按区块状态补入或移除文案。
- Source Analysis：只提供业务语义、信息机制、阅读顺序、区域锚点和通用 App UI 类型；参考图不是底图，
  不得保留竞品文字、品牌、QR、商店徽章、页脚、人脸、服装、手势、道具或背景。
- order item `direction`：由页面 `copy_snapshot.visual_direction` 派生的可追踪摘要。它不替代结构化视觉方向，
  也不是模型提示词；Variant 只能补充视觉执行，不能改写冻结主题、业务事实或文案。
- order `input_snapshot`：冻结的市场资源包 ID/version/config/file IDs 和 squad snapshot。
- 结构化市场 config 是尺寸、Prime、QR、布局和合规规则的真值；附件提供实际品牌与 App UI 文件。

不得读取可变的最新文案库或市场包。冻结输入缺失、版本错配或真实冲突时，将目标 Variant 写成
`action_required`，在 brief 的 `needs_input` 列出字段；不得猜默认市场或业务事实。但不得因为金额、
期限、月供、总利息、总还款、日息或利率只来自冻结还款计划、公式计算或用户确认替换文案而写
`COPY_SNAPSHOT_FINANCIAL_TOKEN_MISMATCH`；这些都是当前订单已冻结的一方批准事实。

## 方案合同

为每个 item 固定写入 `V01`、`V02`、`V03`。三者共享批准文字、我方数值、业务语义、信息层级和主色家族；
不得只改文案或数值。每个 brief 必须包含 `visual_identity_strategy` 和 3-5 条 `anti_copy_changes`，明确
新人物/场景/材质/图标/背景或模块处理；V01-V03 至少有一个高显著视觉身份差异。数值区可以随变体换成另
一种表格/卡片排布，但不得改变冻结金额、期限、月供或引入额外金融事实。若只是在不同变体中重排这些冻结
数值，直接继续生产，不要额外设阻断状态。生产的 `expected_sizes` 来自冻结订单；没有显式小尺寸验收范围时默认使用
三种标准尺寸，当前标准合同为
`1080x1080`、`1200x628`、`800x1000`；横竖版依赖同变体方形母版，不依赖 Prime/QC，也不跨变体等待。
brief 只保存结构化决策和必要事实，不粘贴最终模型提示词、还款表副本或来源审计说明；最终提示词由生产 Agent 在每个尺寸独立生成。

参考机制中没有批准对应文本的选项、问题、标签或按钮只能转译为结构关系，写入
`mechanism_adaptation` 和 `omitted_unapproved_copy`，不得生成空选项或重复金融字段；但原图中已有且冻结 snapshot 非空的结构必须保留，不能因为没有
approved fragment 就改成空白。需要品牌 App UI 时，
只从冻结 `app_ui_reference` 附件按分析类型和 tags 选择，并记录 attachment ID 与理由。

每个 brief 至少包含：candidate/source-analysis/copy/market snapshot identity，完整批准文案，
`creative_contract.variant_execution`、`copy_adaptation`、`mechanism_adaptation`、禁用元素、App UI 选择、
三个尺寸规格。`prime_layout_contract` 是订单快照中的冻结事实，服务端会在写入时覆盖绑定；方案不得自行
生成、补齐、删减或改写其中任一 hard region。`variant_execution` 至少有
`visual_identity_strategy`、`must_preserve`、`allowed_variations` 和 `anti_copy_changes`；只能表达该 Variant
允许新增的执行决策，不得复制或替代父方向、全量生产提示词、还款表或来源审计。hard region 只指导布局；原图主体
进入几何区域不是方案失败，最终以 Prime 成图 QC 为准。

## Prime 的视觉意义

Prime 不是页角装饰或模型要重绘的业务元素。它承载官方品牌、合规、下载和跳转入口，使创意成为可投放成品；
因此必须独立清晰，但不得抢走标题、利益点、金额、表格或 CTA 的主叙事。

每个 brief 必须写 `prime_integration`：说明 Prime 与该变体主视觉的关系、应由哪种同体系背景承载其可读性，
以及按需修复的优先顺序。默认 `readability_strategy=integrated_background`，保留完整构图；先考虑局部降噪、
对比度或留白。只有 Prime 的多个官方组件确实无法在局部背景上清楚阅读，且连续承载面比零散补丁更符合该
变体视觉时，才允许写 `continuous_support_band`。不得把横条、白底或固定色块作为所有变体的默认方案。

```json
{
  "prime_integration": {
    "purpose": "官方品牌、合规与下载入口清晰可见",
    "visual_role": "与主视觉使用同一光线和材质体系",
    "readability_strategy": "integrated_background",
    "fallback_order": ["local_background_cleanup", "continuous_support_band", "native_layout_regeneration"]
  }
}
```

```json
{
  "order_item_id": "<item-id>",
  "variant_key": "V01",
  "brief": {
    "expected_sizes": ["<size>"],
    "creative_contract": {
      "variant_execution": {
        "visual_identity_strategy": "<this Variant's derived visual identity>",
        "must_preserve": ["<parent facts this execution keeps>"],
        "allowed_variations": ["<permitted visual divergence>"],
        "anti_copy_changes": ["<new identity decision>"]
      }
    }
  },
  "status": "queued"
}
```

```text
multica creative order variant-put <order-id> --input-file <variant.json> --output json
```

每次 `variant-put` 后必须读取返回的 `variant.id`、当前 `revision` 与 `brief.creative_contract.parent_direction_sha256`；
后续生产委派只能使用这个返回值；
不得沿用输入 task context 或本地草稿中的 `revision=0`、旧 Variant ID 或旧 `item_key`。

## 委派生产

三个 Variant 均写入后，从冻结 squad snapshot 或当前 task context 读取 `producer_agent_id`、
`reviewer_agent_id`。先查询 producer 的
`creative_order_item_production` source，逐个比较 `<variant-id>:r<revision>`；这里的 revision 必须是
`variant-put` 返回或订单当前行里的当前 revision。若返回缺失或为 0，必须重新 `creative order get` 读取当前
Variant 行后再组 manifest；不得发出 `:r0` item key。active/succeeded task 或完整 generated assets 已存在时
跳过该 item。一个 source 下的兄弟 item 不能阻止缺失项。

将全部缺失 Variant 放进同一个 manifest：source kind 为 `creative_order_item_production`，ref 为真实
Order Item ID；每项 context 固定 `type=creative_domain_task`、`workflow=creative_production`，并原样携带
`issue_id`、`leader_agent_id`、order/item/variant/candidate IDs、revision、`expected_sizes` 和下一阶段 Agent
IDs。先用 JSON 解析器校验，再执行：

```text
multica task by-source list --agent <producer-agent-id> \
  --kind creative_order_item_production --ref <item-id> --output json
multica task fanout --agent <producer-agent-id> --input-file <manifest.json> --output json
```

`manifest.json` 必须包含 source 证据字段。一个 item 的三个 Variant 可放在同一个 manifest，但每个
`item_key`、`variant_id` 和 `revision` 必须使用 `variant-put` 返回的实际值：

```json
{
  "trigger_evidence_kind": "creative_order_item_production",
  "trigger_evidence_ref_id": "<item-id>",
  "items": [{
    "item_key": "<variant-id>:r<revision>",
    "context": {
      "type": "creative_domain_task",
      "workflow": "creative_production",
      "creative_order_id": "<order-id>",
      "creative_order_item_id": "<item-id>",
      "candidate_id": "<candidate-id>",
      "variant_id": "<variant-id>",
      "revision": 1,
    "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
      "scope": "variant",
      "issue_id": "<issue-id>",
      "leader_agent_id": "<leader-agent-id>",
      "producer_agent_id": "<producer-agent-id>",
      "reviewer_agent_id": "<reviewer-agent-id>"
    }
  }]
}
```

CLI 返回的 `tasks` 必须覆盖每个缺失 Variant；否则记录真实错误并让当前 task 失败，不得宣称已委派。

fanout 成功后立即结束，不轮询。CLI 返回字段错误时修正同一 manifest；没有返回 created task 就不得声称
已入队。写回或委派失败必须让当前 task 失败，并在 Variant/task 保留真实 error code/message，不创建 Issue。
