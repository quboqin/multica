---
name: multica-ad-creative-pre-adaptation
description: "当市场中立的 source_analysis 已完成，需要按冻结市场包和文案库准备可生产的文案、数值与版式映射时使用。"
allowed-tools: Bash(multica *)
---

# 创意预适配

预适配把原图观察结果转换成页面可编辑、可追踪的结构化结果。它不生成图片，
不写生产提示词，不修改 Source Analysis、Market Pack 或 Copy Library。

## 输入真值

只使用 task context 指定的 `source_analysis_id`、`market_pack_id`、
`market_pack_version`、`copy_library_id` 和 `copy_library_version`。这些身份和版本是
唯一真值，不读取其他市场包、评论、标题、兄弟 task 或本机业务文件。

先读取冻结上下文：

```text
multica creative source-analysis pre-adaptation-context <source-analysis-id> \
  --market-pack-id <market-pack-id> --market-pack-version <market-pack-version> \
  --copy-library-id <copy-library-id> --copy-library-version <copy-library-version> --output json
```

`source_analysis.text_blocks` 中的 `id` 是每个源文字块的唯一身份；必须保留其
`location`、`role`、`source_text`、`semantic_kind` 和 `visual_region_id` 关系。
`source_analysis.visual_direction` 只用于理解主题、视觉类型、色彩锚点、必须保留和避免项，
不要把它改写成新的适配字段。

竞品文字、品牌、金额、利率、期限、二维码和条款只能作为观察证据，不能进入我方文案、
还款数值或后续生产提示词。只有画面明确呈现还款结构、分期卡或月供/总还款/期限组合时，才把它们归入
`repayment_plan_selections` 和 `numeric_layouts`；单独金额、核心利益点或促销额度即使含数字，也默认按普通
文案处理，保留人工可编辑空间，不要强行锁成还款计划。只有明显的还款结构才生成 `repayment` 选择和
`numeric layout`；像 `Rp100Juta` 这类核心利益点金额，即使识别错了也要继续允许后续手动改，保留后续可手动改写
空间，不要把流程卡死在还款计划选择上。

## 输出合同

外层 `status` 只能是 `completed`、`unavailable` 或 `failed`。`completed` 必须有非空
`summary` 和完整 `result`；普通文案没有兼容的已审核片段时，必须由模型生成一个安全的
`recommended` 候选并交给用户确认，不要因为一个普通文案空位阻断整张素材。只有无法在当前
市场语言和区块职责下生成安全候选时才写 `missing`；金融数值、利率、法律文字、二维码和
品牌事实不能凭空推荐。只有 Source Analysis 结构无法安全消费时才写 `unavailable` 或 `failed`。

`result` 必须保留以下顶层字段，且字段类型不能改变：

```json
{
  "market_pack_id": "<task context market_pack_id>",
  "market_pack_version": 1,
  "copy_library_id": "<task context copy_library_id>",
  "copy_library_version": 1,
  "text_replacements": [],
  "repayment_plan_selections": [],
  "numeric_layouts": [],
  "analysis_highlights": ["<3 to 5 readable observations>"]
}
```

版本和 UUID 必须逐字复制 task context，不能使用当前最新版本。`analysis_highlights` 必须
有 3-5 条视觉或业务观察，不能粘贴审计日志、错误堆栈或提示词。

### text_replacements

每个未进入 `numeric_layouts.source_block_ids` 的源文字块必须恰好出现一次。
每项保留 `block_id`、`visual_region_id`、`location`、`role`、`source_text`、
`replacement_text`、`source_keys`、`status`、`note`；前三个展示字段必须与 Source Analysis
完全一致，不能创造新的区块身份。

`status` 的字段约束如下：

- `ready`：`replacement_text` 非空，`source_keys` 为只含一个已审核 fragment key 的数组；
  不能带 `recommendation_basis` 或 `calculation`。
- `calculated`：`replacement_text` 非空，`source_keys` 为空数组；必须有
  `calculation.rule_key`、`calculation.formula`、非空字符串数组 `calculation.inputs`，以及与
  `replacement_text` 完全相同的 `calculation.result`。
- `recommended`：`replacement_text` 非空，`source_keys` 为空数组；必须有非空字符串数组
  `recommendation_basis`，说明依据但不能冒充已审核文案。它可以是模型根据当前
  Source Analysis 语义、区块职责、市场语言和布局长度生成的新候选；不要引用不存在的
  fragment key，也不要把竞品品牌、金额、利率、期限、法律文字或二维码写进候选。页面会
  显示“待确认推荐”，用户确认前不能进入订单或生产。
- `missing`：`replacement_text`、`source_keys`、`recommendation_basis` 和 `calculation` 都为空；
  只有普通文案也无法安全生成候选，或区块属于金融事实、法律文字、二维码、品牌事实且
  没有审核来源或冻结计算时才使用；页面会让用户编辑或移除该区域。

先按 `semantic_kind` 匹配。本金、期限、月供、总利息、总还款、利率和日息金额不可互换；
非还款的 `plan_field` 仍作为普通文字处理，不能强行塞进 numeric layout。不要因为一个金额看起来很大、很像卖点，
就要求用户先选还款计划；像 `Rp100Juta` 这种核心利益点金额，应优先保持为可手工修改的文案。

### repayment_plan_selections

每个展示的我方还款组合写一条 selection；只有明显的还款结构才生成 `repayment` 选择和 `numeric layout`，
单独金额和卖点数字不触发锁定：

```json
{
  "id": "<selection id>",
  "plan_key": "<approved repayment plan key>",
  "principal": 0,
  "tenor_months": 0,
  "values": {
    "principal": "<approved display value>",
    "tenor": "<approved display value>",
    "total_interest": "<approved display value>",
    "total_repayment": "<approved display value>",
    "monthly_installment": "<approved display value>"
  }
}
```

`plan_key` 必须来自冻结 Copy Library 的已审核还款计划。`principal` 和 `tenor_months` 是整数，
`values.*` 全部是字符串；不能把数字写成 JSON number，也不能自行计算出不在冻结计划中的行。
示例中的 0 只是类型占位，正式结果必须替换为冻结计划的真实整数。

### numeric_layouts

每个真实的本金、期限、月供、总利息或总还款区域写一条 layout：

```json
{
  "id": "<layout id>",
  "visual_region_id": "<numeric visual region id>",
  "source_block_ids": ["<real repayment block id>"],
  "location": "<source location>",
  "layout_kind": "table",
  "scenario_ids": ["<selection id>"],
  "target_columns": ["principal", "tenor"],
  "render_instruction": "<instruction containing every approved display value for every scenario and target column>"
}
```

`layout_kind` 只能是 `table`、`card_grid`、`comparison`、`single_card`、`single_value`、
`option_buttons` 或 `table_row`。`target_columns` 只能是 `principal`、`tenor`、
`monthly_installment`、`total_interest` 或 `total_repayment`。

`source_block_ids` 只能引用同一 numeric visual region 内真实的还款字段；姓名、职业、资格、
按钮、标签、利率和普通提示语必须留在 `text_replacements`。同一张表的期限顺序要整体保留。
冻结计划没有某个原图期限时，不能拿另一个期限的金额填入该列；要么整体重构为冻结计划支持的
期限列，要么让不支持的源区块留空移除。

`render_instruction` 是服务端可审计的值清单，不是泛化描述：对每个 `scenario_id` 和每个
`target_columns`，必须逐字写入对应 `repayment_plan_selection.values[column]` 的完整展示字符串，
例如 `Rp4.000.000`、`3 Bulan`、`Rp1.369.333`。只写“按行展示七组已审核方案”或只依赖
`scenario_ids` 不合格；如果一个 layout 的目标列有多个，每个方案在每个目标列的值都必须写入
同一条 `render_instruction`，必要时拆分 layout。

多行数值表采用“我方优先、数量取小”的规则，不要求原图行数与我方方案数量相等：

1. 先按表格语义、目标列和期限筛选冻结 Copy Library 中状态为 `approved` 的方案。
2. 每个可渲染区域实际使用的行数为 `min(原图可渲染行数, 我方兼容已审核方案数)`。
3. 每个实际渲染行都写入一条 `repayment_plan_selection`，并由 `numeric_layouts` 引用；展示值只能来自该
   selection 对应的冻结方案，且 `render_instruction` 必须逐字列出这些展示值。
4. 原图多出来的区块保留原始 `block_id`、位置、职责、原文和 `visual_region_id`，各自写成空内容的
   `missing` replacement。它们是默认移除项，不是等待继续匹配的普通文案，也不能阻断整张素材。

每个 layout 只放它实际能展示的 `source_block_ids`，并且服务端的容量上限是：

```text
len(source_block_ids) <= len(scenario_ids) * len(target_columns) * 2
```

当原图行数超过我方可用方案数时，不能把所有行硬塞进一个 layout，也不能把竞品数值改写成我方
数值。只写入我方实际可用的行；多出来的本金、期限、月供、总利息或总还款块必须各自写成
`missing` replacement，确认页保持可追踪并允许留空移除。`missing` 数值块不会阻断整张素材，也不会
触发新的分析循环。

提交前按以下顺序自检，不要靠连续 PUT 试错：

1. 每个 Source Analysis `text_blocks` 恰好出现在一个 replacement 或一个 numeric layout。
2. replacement 的 `block_id`、`location`、`role`、`source_text`、`visual_region_id` 与原分析逐字一致。
3. 未进入 layout 的数值块使用空内容的 `missing`，不能携带 `source_keys`、推荐或计算结果。
4. 每个 layout 的列、方案、渲染说明只包含已审核冻结值；每条 selection 都被 layout 引用，且每个
   `scenario_id` 的每个目标列值都出现在 `render_instruction` 中。
5. 只在上述检查完成后调用一次 `pre-adaptation-put`。收到 400 时修正同一个结果文件并最多再提交一次，
   不更换冻结资源、版本、区块 ID 或 plan key。

## 写回与恢复

写回前逐项检查：每个源文字块恰好进入一个 replacement 或一个 numeric layout；每个 approved
fragment key 只使用一次；无库匹配的普通文案已经生成 `recommended` 且 `source_keys` 为空；每条
selection 都被 layout 引用；公式输入、结果和冻结版本完整。

```text
multica creative source-analysis pre-adaptation-put <source-analysis-id> --input-file <result.json> --output json
```

如果 PUT 返回 400，保留同一个 `result.json`，根据错误修正字段类型或映射后再提交一次；不要换
market pack、copy library、block id 或 plan key，也不要用最新版本重建上下文。平台会在任务层自动
恢复失败的预适配任务；无法完成时要写真实的 `error_code` 和 `error_message`，不要伪造 completed。
平台重试耗尽后会保存人工确认状态；只有确认 Source Analysis 本身损坏时才重新分析，普通的未匹配
数值应以 `missing` 完成并交给用户选择。

预适配完成后，页面会把结构化文案、数值和 `visual_direction` 冻结到订单 `copy_snapshot`；最终
提示词由生产 Skill 在每个尺寸运行时自行组织。
