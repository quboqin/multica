---
name: multica-ad-creative-pre-adaptation
description: "当候选图已完成市场中立 Source Analysis，需要在用户选图前按冻结市场包和文案库准备可生产的文字与数值填充时使用。"
allowed-tools: Bash(multica *)
---

# 创意预适配

只使用 task context 的 `source_analysis_id`、`market_pack_id`、`market_pack_version`、
`copy_library_id` 和 `copy_library_version`。这些版本是唯一真值；不得读取当前其他市场包、
Issue、评论、标题或兄弟 task。

```text
multica creative source-analysis pre-adaptation-context <source-analysis-id> \
  --market-pack-id <market-pack-id> --market-pack-version <market-pack-version> \
  --copy-library-id <copy-library-id> --copy-library-version <copy-library-version> --output json
```

Source Analysis 只描述原图事实。预适配负责把原图视觉区域映射到冻结文案库、冻结还款计划和冻结市场包
`calculation_rules`。竞品金额、利率、期限、品牌、二维码和条款只可作为观察证据，绝不可进入最终文案、
数值或 `production_prompt`。

## 输出合同

不要先把整张图归为 NUM、还款计划或文案图。逐个 `visual_regions` 处理：

- `copy` 区域输出 `text_replacements`；
- `numeric` 区域只用于实际展示本金、期限、月供、总利息或总还款的卡片、表格或对照区；
- 利率、日利息条、按钮、问题、标签等即使含数字，只要不是还款计划版式，仍走 `text_replacements`。

`result` 第一层必须包含 `market_pack_id`、`market_pack_version`、`copy_library_id`、
`copy_library_version`、`text_replacements`、`repayment_plan_selections`、`numeric_layouts`、
`analysis_highlights` 和 `production_prompt`。不要写旧字段 `calculation_scenarios`、`product_facts`、
`recipes`、`mapping`、`preferred_fragment_keys`、`decision` 或 `gaps`。

### text_replacements

所有未进入 `numeric_layouts.source_block_ids` 的 Source Analysis `text_blocks` 都必须恰好出现一次。
每项字段：`block_id`、`visual_region_id`、`location`、`role`、`source_text`、`replacement_text`、
`source_keys`、`status`、`note`。`block_id` 是身份字段；`visual_region_id`、`location`、`role` 和
`source_text` 只是便于人审的展示字段，服务端会按 Source Analysis 重新补齐，不要用这些字段表达新的区域身份。

`status` 只能是：

- `ready`：只选择冻结文案库 approved fragment 的一个真实 `source_keys` key；同一个 key 在同一素材中只能绑定
  一个源文字块。`replacement_text` 可以为人审预览写成对应片段文本，但服务端会从冻结文案库按 key 覆盖生成，
  不要把它当来源字段。
- `calculated`：文案库无现成值，但冻结 `calculation_rules` 对同一 `semantic_kind` 有公式和我方输入；
  `source_keys=[]`，写 `calculation.rule_key`、逐字匹配的 `formula`、可读 `inputs` 和与
  `replacement_text` 完全一致的 `result`。
- `recommended`：没有 approved 片段，也不能公式计算，但能依据冻结市场包、通用卖点、订单上下文和原图语义
  给出不含竞品事实的替代；`source_keys=[]`，写非空 `recommendation_basis`，待用户确认。
- `missing`：仅限不可读、纯装饰或无法安全计算/推荐的区域；`replacement_text`、`source_keys`、
  `recommendation_basis` 和 `calculation` 都必须为空。生产会移除原竞品文字。

先读 `semantic_kind`，再匹配内容。金额、期限、月供、总利息、总还款、利率、日利息金额和日利息标签
不可互换；日利息金额不能用利率替换，金额不能用百分比替换。非还款的 `plan_field` 也必须作为普通文字
填充，优先用任何文字空位都成立的 approved 通用补位卖点；不得把品牌、产品条件、人群条件、额度、期限、
利率、月供或行动承诺当兜底。

### repayment_plan_selections 与 numeric_layouts

每个展示的我方金额/期限组合写一条 `repayment_plan_selections`，字段为 `id`、`plan_key`、
`principal`、`tenor_months` 和字符串 `values`。`plan_key` 是身份字段，必须来自冻结 approved repayment plan；
`principal`、`tenor_months` 和 `values` 是展示字段，服务端会按 `plan_key` 从冻结还款计划覆盖生成。你必须理解：
`plan-8000000-6` 是 key，`Rp8.000.000 / 6 Bulan / Rp1.405.333` 是该 key 对应的 value。

每个原图 `numeric` 区域写一条 `numeric_layouts`，字段为 `id`、`visual_region_id`、
`source_block_ids`、`location`、`layout_kind`、`scenario_ids`、`target_columns`、`render_instruction`。
`layout_kind` 只能是 `table`、`card_grid`、`comparison`、`single_card`、`single_value`、
`option_buttons` 或 `table_row`。本金区、期限区、还款表这些业务语义写在 `location` 和
`target_columns`，不要写成新的 `layout_kind`。
`source_block_ids` 只能包含真实还款字段；姓名、职业、资格、问题、按钮和结果文案必须留在
`text_replacements`。`target_columns` 只能是 `principal`、`tenor`、`monthly_installment`、
`total_interest`、`total_repayment`。可按我方数据减少/增加行列或转成卡片网格，不得补造金额、期限或月供。
先识别原图同一张数值表的列顺序；如果冻结 approved repayment plan 覆盖了原图期限，就保持原表语义和顺序。
如果某个期限缺失，不能只把缺失列局部替换成另一个期限，导致同表出现 `6 / 3 / 12` 这类语义混乱顺序。
此时只能整体重构为冻结还款计划支持的标准表，并按照原图表格的业务语义排列我方期限；或者不自动推荐该数值区，
把对应源文字留给用户在确认页人工处理。例如原图左到右表达期限递增，新表也保持左到右递进；如果原图表达主推项优先、
高亮项居中或选中项在前，则保留这个排序语义。整体重构时，`location` 和 `render_instruction` 必须写清楚原表列被整体改为哪组
我方期限以及新的语义顺序。
`scenario_ids` 可以引用 `repayment_plan_selections.id`，也可以直接引用 `plan_key`；服务端会归一到 selection id。
`render_instruction` 可以写预览，但服务端会按最终选中的 `plan_key` 和 `target_columns` 重新生成。

### production_prompt

`production_prompt` 是给生产 Skill 的压缩业务提示词，不是推理日志。必须：

- 逐字包含每个非空 `replacement_text`，以及每条 `numeric_layouts.render_instruction` 指向的我方展示值；
- 每条文案、还款行、版式规则只出现一次；去掉重复句、重复表格行、分析理由和来源审计说明；
- 只写可执行画面要求：视觉主线、文案替换、数值版式、可省略区、禁用竞品事实；
- 不粘贴 Source Analysis 全量文字，不保留竞品品牌/金融事实，不输出代码式推荐。

外层只有 Source Analysis 本身无法支撑适配（例如没有可用文字块或视觉区域身份不唯一）时，才写
`status=unavailable` 和具体 `error_message`。普通文字没有 approved replacement、`source_keys` 选错、
`plan_key` 选错或某个数值区无法自动推荐时，仍写 `status=completed`；服务端会把对应块降级为
`missing`，交给用户在确认页选择文案或手动填写，不能卡住素材可用流程。

```json
{
  "status": "completed",
  "summary": "<简短适配结论>",
  "result": {
    "market_pack_id": "<task context market_pack_id>",
    "market_pack_version": <task context market_pack_version>,
    "copy_library_id": "<task context copy_library_id>",
    "copy_library_version": <task context copy_library_version>,
    "text_replacements": [],
    "repayment_plan_selections": [],
    "numeric_layouts": [],
    "analysis_highlights": ["<3 to 5 items>"],
    "production_prompt": "<compact executable prompt>"
  },
  "error_code": "",
  "error_message": ""
}
```

写回前自检：每个源文字块恰好一次进入 `text_replacements` 或 `numeric_layouts.source_block_ids`；
每个 approved fragment key 最多出现一次；`calculated` 必有公式、我方输入和同文结果；`recommended`
必有依据且不冒充已审核；每个选中还款行都被某个 layout 引用；`production_prompt` 不重复。写回后服务端会
再次按冻结 Source Analysis、文案库和还款计划派生展示字段和值；以服务端回读结果为准。

```text
multica creative source-analysis pre-adaptation-put <source-analysis-id> --input-file <result.json> --output json
```

回读前必须确认写入成功。不得修改 Source Analysis 原有事实、文案库、市场包或生成图片。
