---
name: multica-ad-creative-plan
description: "当 Creative Order Item 已冻结参考分析、业务选择、文案快照和市场资源快照，需要规划 4-5 个主视觉候选并委派候选生产时使用。"
allowed-tools: Bash(multica *)
---

# 广告生成方案

只处理 task context 指定的 Order、Order Item、candidate 和 revision。

```text
multica creative order get <order-id> --output json
```

当 item `source_kind=material`，且订单只保存 `source_analysis_id` 而未内嵌结果时，允许按 candidate 回读一次：

```text
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

除此之外不探测素材命令，不读旧 Issue 流程，不从标题、评论或本机文件补输入。

## 流程合同

先确认 order `input_snapshot.pipeline_version=candidate_v1`，该字段由平台创建订单时冻结，禁止修改。字段缺失或值不符时停止并写真实错误，
不得推断、回填或切换到其他流程。每个标准订单都创建 4-5 个 `C01`-`C05` 候选，先生产各自主尺寸，再由独立质检晋级。

## 输入真值

- item `source_kind=copy_library` 时直接规划原创画面，`candidate_id` 和 `source_analysis_id` 为空是正常状态，不读取或补造竞品分析。
  `copy_snapshot.slots`、`fragments`、顶层文字、`repayment_plan_entries` 和 `repayment_plan_labels` 是完整的已选内容；
  所有文案槽和还款计划均可为空，空槽禁止补写。`visual_only=true` 时只规划视觉，不增加业务卖点、数字、利率、还款表或 CTA。
  图片分类 `creative_type` 仅控制分类，不要求存在数字或还款模块。Prime 保持官方合同。
  从实际输入生成 `creative_intent.input_roles`：没有 source reference，Prime 是第一张视觉输入；后续尺寸仅按身份合同增加主视觉参考。
  `approved_copy` 保留空槽，LayoutPlan 的内容组和验收项只覆盖实际选择的文字及还款行。
- `copy_snapshot`：页面预适配后冻结的唯一可见文案、还款计划和用户视觉方向。`pre_adaptation.text_replacements` 是每个普通
  画面文字区块的权威替换表；`pre_adaptation.numeric_layouts`、`repayment_plan_entries`、
  `repayment_plan_selections.values` 和 `text_replacements[].calculation.result` 都是数值区域的权威版式与数值。
  `pre_adaptation.app_ui_replacement` 是页面冻结的 App UI 参考选择；这里不保存模型提示词，生产 Agent 根据这些结构化事实和当前尺寸自行写短提示词。
- `copy_snapshot` 的顶层非空字段（`headline`、`subheadline`、`benefit`、`supporting`、`cta`、`legal_text`）优先级最高，必须逐字
  进入每个 Variant 的 `approved_copy`。`pre_adaptation.text_replacements` 只负责把源区块映射到这些字段或补充独立结构；当两者冲突时，不能用
  `recommended` 的替换文本覆盖顶层非空字段。只有顶层字段本身为空，或页面明确将该字段置空，才允许按区块状态补入或移除文案。
- Source Analysis：只提供业务语义、信息机制、阅读顺序、区域锚点和通用 App UI 类型；参考图不是底图，
  不得保留竞品文字、品牌、QR、商店徽章、页脚、人脸、服装、手势、道具或背景。
- order item `direction`：由页面 `copy_snapshot.visual_direction` 派生的可追踪摘要。它不替代结构化视觉方向，
  也不是模型提示词；Variant 只能补充视觉执行，不能改写冻结主题、业务事实或文案。若 direction 或 Source Analysis 中含有手机、屏幕、手持手机或
  App 页面描述，但 `pre_adaptation.app_ui_replacement.selected` 不是 true，这些内容只作为源图证据，不得写入 Variant 的 `must_preserve` 或要求生产保留手机界面。
- order `input_snapshot`：冻结的市场资源包 ID/version/config/file IDs 和 squad snapshot。
- 结构化市场 config 是尺寸、Prime、QR、布局和合规规则的真值；附件提供实际品牌与 App UI 文件。

不得读取可变的最新文案库或市场包。冻结输入缺失、版本错配或真实冲突时，将目标 Variant 写成
`action_required`，在 brief 的 `needs_input` 列出字段；不得猜默认市场或业务事实。但不得因为金额、
期限、月供、总利息、总还款、日息或利率只来自冻结还款计划、公式计算或用户确认替换文案而写
`COPY_SNAPSHOT_FINANCIAL_TOKEN_MISMATCH`；这些都是当前订单已冻结的一方批准事实。

## 方案合同

先完整读取 [Creative Intent Contract](references/creative-intent-contract.md)。为每个 item 写入 4-5 个候选 Variant，固定使用
`C01`-`C05` 中连续的 key；默认 5 个，只有无法形成第 5 个真实不同的创意假设时才使用 4 个，不能用同一方向换色凑数。
每个候选必须有独立 `CreativeIntent`，并至少改变两个高显著 `DesignDNA` 维度。候选共享批准文字、我方数值和业务真值，
但不强制共享主色；不得只改文案、数值或局部装饰。

候选阶段只生成一个 `primary_size` 主视觉。为确保 4-5 个候选可公平并排比较，所有候选固定先生成
`1080x1080` 方图；横版、竖版和手机叙事方向仍须在各自三尺寸 LayoutPlan 中预先设计，但不能把候选首轮改成其他比例。每个候选先在
variant-put 顶层写 `candidate_state=candidate` 与 `primary_size=1080x1080`，再用仅包含该方图的 task `expected_sizes`
独立 fanout 主尺寸生产。4-5 个主尺寸 Prime 图形成至少 3 个合格候选后，由 `creative_candidate_selection` 独立比较；终态失败候选由平台标记 rejected，
其余候选继续比较。少于 3 个时不晋级，保留真实失败供有界恢复或人工处理。Planner 不预选三个最终 Variant，不逐条改状态冒充晋级，也不在候选阶段补其他尺寸。

候选比较原子选择恰好 3 个，按 rank 1-3 设为 `selected`，其余为 `reserve`。reserve 的主视觉、提示词、模型回执和附件血缘必须保留，
但不参与订单交付汇总。selected 复用已完成主尺寸，平台把其 `expected_sizes` 扩展为冻结的完整三尺寸，再只补缺失两尺寸。
当前标准尺寸为 `1080x1080`、`1200x628`、`800x1000`。

数值区可以随候选换成另一种表格/卡片排布，但不得改变冻结金额、期限、月供或引入额外金融事实。brief 只保存结构化
决策和必要事实，不粘贴最终模型提示词、还款表副本或来源审计说明；最终提示词由生产 Agent 从结构化合同逐尺寸编译。

参考机制中没有批准对应文本的选项、问题、标签或按钮只能转译为结构关系，写入
`mechanism_adaptation` 和 `omitted_unapproved_copy`，不得生成空选项或重复金融字段；但原图中已有且冻结 snapshot 非空的结构必须保留，不能因为没有
approved fragment 就改成空白。

### App UI 替换合同

每个 brief 必须写 `creative_contract.app_ui_replacement`，但 Source Analysis 检测到手机或 App 页面并不意味着必须替换 UI。
方案 Agent 不看图片像素，也不下载附件；它只消费页面冻结在 `copy_snapshot.pre_adaptation.app_ui_replacement` 中的业务选择，
不得自行从 `input_snapshot.market_pack.files` 中挑选或替换用户选择。只有页面冻结的 `required=true` 时，选择才必须对应
市场资源包中的 `app_ui_reference` 文件，并带有 `resource_file_id` 与 `attachment_id`。

`app_ui_replacement` 必须包含：

- `required`：是否需要把竞品 UI 替换为 AdaKami UI；
- `selected`：仅在 `required=true` 时表示是否已有页面冻结的合适资源；
- `resource_file_id` 与 `attachment_id`：只在 selected=true 时填写，必须逐字来自 `copy_snapshot.pre_adaptation.app_ui_replacement`；
- `source_screen`：记录 `app_ui_type`、屏幕位置、可见度、`app_ui_bounds` 和是否被手/手机边框遮挡；
- `reason`：复用页面冻结选择的 reason，可补充一句执行说明但不得改换资源；
- `constraints`：仅在 `required=true` 时至少说明“只替换手机屏幕内容，保留手机、手、透视、光照和场景；移除竞品 logo、品牌色、按钮文案、QR 和专属页面文案；不得把 AdaKami UI 画到屏幕外”。

当页面冻结的 `required=true` 但 `selected` 不是 true，或缺少 `resource_file_id`/`attachment_id`，该 Variant 写
`action_required`，并在 `needs_input` 中要求用户完成已选择的 App UI 替换。`required=false` 时必须同时写
`selected=false`、清空资源字段和简短 reason；生产不得下载任何 App UI 参考图，也不得把手机、屏幕或 App 页面当作必须保留的 UI。
手机可以被移除，也可以仅作为表达利益点的视觉形式；不得保留竞品屏幕或臆造 AdaKami 页面。不得因为资源包里有 UI 图或 Source Analysis
检测到 UI 就机械选择；替换只来自页面冻结的业务确认。

### 核心利益点

顶层冻结 `benefit` 非空，或 `copy_snapshot.pre_adaptation.additional_copy` 中存在 `role=benefit` 的 `ready` 文案时，
它是批准的核心利益点，不是仅供图标表达的语义提示。每个候选必须把这段文字加入 `creative_intent.locked_set` 和当前尺寸的
`content_groups`，在手机屏幕外规划清晰可读的文本区域；即使有图标、步骤卡或手机形式，也只能辅助，不能替代该文字。每个尺寸的
`acceptance_checks` 必须包含“核心利益点逐字可见且可读”的可观察条件。利益点为空时可使用纯视觉机制，但不得臆造业务 claim。

每个 `variant-put` 对象顶层必须写 `candidate_state` 与 `primary_size`，不能把流程状态重复塞进 brief。
每个 brief 至少包含：candidate/source-analysis/copy/market snapshot identity，完整批准文案，
`creative_contract.creative_intent`、`creative_contract.design_dna`、每个冻结尺寸的 `creative_contract.layout_plans`、
`variant_execution`、`copy_adaptation`、`mechanism_adaptation`、禁用元素和 `app_ui_replacement`。
`CreativeIntent` 必须明确 `input_roles`、`locked_set`、`editable_set`、`change_budget`、优先级和可观察
`acceptance_checks`；`DesignDNA` 必须明确一致性模式、主体、色彩/材质/光线、模型文字层级、视觉母题、空间签名、
跨尺寸不变量和可改编项；每个 `LayoutPlan` 必须写原生重排、裁切容忍、内容密度、表格策略、Prime 承托策略与三尺寸风险。
模型继续负责渲染所有批准业务文字；方案不得安排平台代码排字、空白文字框或后续文字 overlay。

`prime_layout_contract` 是订单快照中的冻结事实，服务端会在写入时覆盖绑定；方案不得自行
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
    "background_polarity": "adaptive",
    "fallback_order": ["local_background_cleanup", "continuous_support_band", "native_layout_regeneration"]
  }
}
```

```json
{
  "order_item_id": "<item-id>",
  "variant_key": "C01",
  "brief": {
    "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
    "creative_contract": {
      "app_ui_replacement": {
        "required": true,
        "selected": true,
        "resource_file_id": "<frozen-app-ui-resource-file-id>",
        "attachment_id": "<app-ui-reference-attachment-id>",
        "source_screen": {
          "app_ui_type": "<generic type from source analysis>",
          "visibility": "<visible|partial|small|blurred>",
          "bounds": {}
        },
        "reason": "<why this AdaKami UI reference matches>",
        "constraints": ["only replace phone screen content", "preserve hand/phone/perspective/lighting"]
      },
      "creative_intent": {
        "hypothesis_id": "C01-hypothesis",
        "input_roles": [],
        "locked_set": [],
        "editable_set": [],
        "change_budget": "new_concept",
        "priorities": [],
        "acceptance_checks": []
      },
      "design_dna": {
        "consistency_mode": "strict_identity",
        "subject_system": {},
        "visual_system": {},
        "typography_system": {},
        "motif_system": {},
        "spatial_signature": {},
        "invariants": [],
        "adaptable_features": []
      },
      "layout_plans": {
        "1080x1080": {},
        "1200x628": {},
        "800x1000": {}
      },
      "variant_execution": {
        "visual_identity_strategy": "<this Variant's derived visual identity>",
        "must_preserve": ["<parent facts this execution keeps>"],
        "allowed_variations": ["<permitted visual divergence>"],
        "anti_copy_changes": ["<new identity decision>"]
      }
    }
  },
  "candidate_state": "candidate",
  "primary_size": "1080x1080",
  "selection_rank": null,
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

4-5 个候选 Variant 均写入后，从冻结 squad snapshot 或当前 task context 读取 `producer_agent_ids`
（缺失时回退 `producer_agent_id`）和 `reviewer_agent_id`。`producer_agent_ids[0]` 只作为 fanout 入口 Agent；
实际生产 Agent 由服务端按当前 squad 成员、启用的 `image_edit` Skill 和在线 runtime 动态选择，并写回子任务
context。不要在标准生产 manifest 的 item context 里写 `producer_agent_id`，否则会把动态池固定到单个优先 Agent。

先对已知 producer 入口逐个查询
`creative_order_item_production` source，比较 `<variant-id>:r<revision>`；这里的 revision 必须是
`variant-put` 返回或订单当前行里的当前 revision。若返回缺失或为 0，必须重新 `creative order get` 读取当前
Variant 行后再组 manifest；不得发出 `:r0` item key。active/succeeded task 或完整 generated assets 已存在时
跳过该 item。这个查询只是预检；服务端会按历史任务防重复，并会识别运行时新增或移除的出图池成员。

将全部缺失候选放进同一个 manifest：source kind 为 `creative_order_item_production`，ref 为真实
Order Item ID；每项 context 固定 `type=creative_domain_task`、`workflow=creative_production`，并原样携带
`issue_id`、`leader_agent_id`、order/item/variant/candidate IDs、revision、`expected_sizes` 和下一阶段 Agent
IDs；候选阶段的 `expected_sizes` 必须严格为 `[primary_size]`，并带 `candidate_state=candidate`、
`production_stage=candidate_primary`，且候选阶段的唯一 `expected_sizes` 为 `["1080x1080"]`。先用 JSON 解析器校验，再执行：

```text
multica task by-source list --agent <producer-entry-agent-id> \
  --kind creative_order_item_production --ref <item-id> --output json
multica task fanout --agent <producer-entry-agent-id> --input-file <manifest.json> --output json
```

`manifest.json` 必须包含 source 证据字段。一个 item 的 4-5 个候选可放在同一个 manifest，但每个
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
      "expected_sizes": ["1080x1080"],
      "candidate_state": "candidate",
      "production_stage": "candidate_primary",
      "scope": "variant",
      "issue_id": "<issue-id>",
      "leader_agent_id": "<leader-agent-id>",
      "reviewer_agent_id": "<reviewer-agent-id>"
    }
  }]
}
```

CLI 返回的 `tasks` 必须覆盖每个缺失候选；否则记录真实错误并让当前 task 失败，不得宣称已委派。

fanout 成功后立即结束，不轮询。CLI 返回字段错误时修正同一 manifest；没有返回 created task 就不得声称
已入队。写回或委派失败必须让当前 task 失败，并在 Variant/task 保留真实 error code/message，不创建 Issue。
