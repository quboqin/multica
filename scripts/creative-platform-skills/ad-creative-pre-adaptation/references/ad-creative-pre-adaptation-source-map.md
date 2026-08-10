# 预适配来源映射

| 输出 | 唯一来源 |
| --- | --- |
| 原图机制、视觉锚点、布局限制 | task 指定的 Source Analysis |
| `text_replacements` 的区块 ID、视觉区域、位置、职责、原图文字和语义类型 | `block_id` 来自 Source Analysis `text_blocks`，是唯一身份；`visual_region_id`、`location`、`role` 和 `source_text` 由服务端按 `block_id` 从 Source Analysis 回填。`semantic_kind` 决定金额、利率、期限等金融槽位的兼容性，不能按视觉角色或文案相似度互换；先选 approved fragment 的唯一 `source_keys` key 写 `ready`，最终 `replacement_text` 由服务端从冻结文案库派生；无现成项但有市场包 `calculation_rules` 的同语义公式和我方输入则写带 `calculation.rule_key` 的 `calculated`，有安全上下文依据则写带 `recommendation_basis` 的 `recommended`，两者均待用户确认；不可安全推荐时 `missing` |
| `numeric_layouts` 的原图数字区域、视觉区域、表格/卡片关系 | 仅限 Source Analysis 中实际表示本金、期限、月供、总利息或总还款的标签和值，且所有 `source_block_ids` 必须属于同一个 `numeric` `visual_region_id`；`layout_kind` 使用服务端接受的 `table`、`card_grid`、`comparison`、`single_card`、`single_value`、`option_buttons` 或 `table_row`；本金区、期限区、还款表等语义来自 `location` 和 `target_columns`；同表期限必须先识别原图列顺序及其业务语义，原期限可覆盖时保持原语义顺序，缺失期限不得局部替换成其他期限，只能整体重构为冻结还款计划支持且符合原图排序语义的标准表，或交给用户人工处理 |
| 普通文案、通用补位卖点、还款计划行与最终文案 | task 指定版本的 Copy Library；同一个 approved fragment 只能占用一个源文字块。文案库是 `ready` 的唯一来源，不是计算值或系统推荐的伪造来源。`source_keys` 和 `plan_key` 是 agent 选择的身份字段，服务端从冻结版本派生文案文本、金额、期限、月供、总利息和总还款。通用补位卖点只可填任意文字空位都成立的审核内容，不能含品牌、产品条件或金融事实 |
| 品牌、合规、Prime 和 QR 规则 | task 指定版本的 Market Pack |
| 可计算的日息、总利息、总还款和月还款 | task 指定版本 Market Pack 的 `calculation_rules`；公式是业务规则，还款计划行只是可直接展示的已审核样例，二者不得互相伪造来源 |
| production_prompt | 上述冻结来源与每项 `text_replacements`、`numeric_layouts` 和还款计划选择的可追溯组合；服务端会把已派生的非空文案和数值展示要求补入提示词。最终内容必须是去重后的可执行业务提示词，每条文案、还款行和版式规则只出现一次 |

竞品文本只可作为 Source Analysis 里的观察证据，禁止成为最终文案或还款数据来源。
