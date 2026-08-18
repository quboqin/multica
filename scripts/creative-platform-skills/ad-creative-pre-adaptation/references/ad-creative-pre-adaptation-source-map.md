# 预适配来源映射

| 输出 | 唯一来源 |
| --- | --- |
| 原图事实、文字块、视觉区域和 `visual_direction` | task 指定版本的 Source Analysis |
| `text_replacements` 的区块身份、位置、职责和原文 | Source Analysis `text_blocks`；`block_id` 是唯一身份，其他展示字段必须保持一致 |
| `ready` 文案 | task 指定版本 Copy Library 中状态为 `approved` 的 fragment；`source_keys` 只记录已审核来源 |
| `calculated` 数值 | task 指定版本 Market Pack 的 `calculation_rules` 和冻结输入；公式、输入和结果必须可追溯 |
| `recommended` 文案 | 没有兼容的已审核片段时，由模型按 Source Analysis 语义、区块职责、市场语言和版面长度生成；`source_keys` 为空，必须提供 `recommendation_basis`，页面标记待确认，不能伪装成已审核文案 |
| `missing` 文案 | 只有普通文案也无法安全生成候选，或金融事实、法律文字、二维码、品牌事实缺少审核来源/冻结计算时使用；由确认页编辑或移除，不阻断素材 |
| `repayment_plan_selections` | task 指定版本 Copy Library 的已审核 repayment plan；`plan_key` 是唯一身份，展示 values 必须与其一致 |
| `numeric_layouts` | Source Analysis 中真实本金、期限、月供、总利息或总还款区域，以及被 layout 引用的冻结 repayment selections；使用 `min(原图可渲染行数, 我方兼容已审核方案数)`，不要求行数相等 |
| layout 的 `render_instruction` | 必须逐字列出每个 `scenario_id` 对应 selection 在每个 `target_columns` 中的完整 approved 展示值；不能只写“按行展示”或只依赖 scenario id |
| 原图多出的数值块 | 保留原始区块身份的空 `missing` replacement，作为默认移除项；不改写竞品数字，不阻断整张素材 |
| `analysis_highlights` | Source Analysis 的可读视觉和业务观察，3-5 条，不包含提示词或审计日志 |
| 订单 `copy_snapshot` 的适配结构 | 服务端校验通过后的预适配结果；生产 Skill 不从竞品文本或当前最新资源补事实 |

竞品文本、竞品金融数字和竞品品牌只能作为观察证据，不能成为最终文案、数值或生产提示词。

服务端会校验每个 block 的来源锚点、视觉区域、冻结版本、已审核 fragment 和还款计划值。
`numeric_layouts` 的容量约束是 `source_block_ids <= scenario_ids * target_columns * 2`；超过时拆成多个
layout。没有我方兼容方案的期限列不能借用其他期限金额，直接写 `missing` 并在确认页移除，不能用一个
大 layout 或泛化文案绕过校验。每个 layout 的 `render_instruction` 都必须包含它实际引用的每个方案、
每个目标列的完整冻结展示字符串，服务端会按这些字符串核对写回结果。
