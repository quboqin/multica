---
name: multica-ad-creative-analysis
description: "当原生 task 指定一张候选图片，需要读取真实像素并把市场中立的主题、利益点、语义锚点和布局约束写入 Source Analysis 时使用。"
allowed-tools: Bash(multica *)
---

# 广告参考分析

只使用 task context 的 `crawl_run_id`、`candidate_id` 和 `analysis_version`。不得从 Issue、评论、标题或兄弟
task 猜目标，也不得读取市场包、Prime、品牌 App UI 或批准文案。

## 输入

```text
multica creative library download <candidate-id> --output-file <path> --output json
```

必须用运行时视觉能力读取下载文件的真实像素。采集元数据只能辅助定位；图片不可读时仍写 failed Source
Analysis，`error_code=asset_unreadable`，不得用标题或标签补结论。

## 输出

`result` 使用像素证据填写：

- `theme`、`theme_elements`；
- `primary_benefit`、`secondary_benefits`、`benefit_value`；
- `source_semantics`、`information_mechanism`；
- `copy_slots`：只列出原图真实存在的文案职责，可选值为 `headline`、`subheadline`、`benefit`、
  `supporting`、`cta`、`legal`。没有按钮或行动区时不得输出 `cta`；
- `text_blocks`：逐个列出图片中每一块可读文字。每项必须含 `id`、`location`、`role`、`source_text`、
  `purpose`、`semantic_kind`、`visual_bounds`、`confidence`。`visual_bounds` 是相对原图的
  `{ "x": 0-1000, "y": 0-1000, "width": 1-1000, "height": 1-1000 }` 整数矩形，左上角为原点，
  右边和下边不得超过 1000；它只框住该块真实可读文字，用于确认页将原图位置与替换文案逐一核对，
  不是生成时的像素级排版合同。坐标必须根据下载图片的真实像素判断，不得由 `location` 描述反推。
  `semantic_kind` 只能为 `copy`、`principal`、`tenor`、
  `monthly_installment`、`total_interest`、`total_repayment`、`interest_rate`、`daily_interest_amount` 或
  `daily_interest_label`。它表达业务含义，不得根据蓝字、字号或 `role=benefit` 猜测：例如 `Rp10.000`
  的日利息金额是 `daily_interest_amount`，`0,1%` 才是 `interest_rate`。`location` 使用人可读的相对位置，
  例如 `左上第一张还款卡`、`中部横幅`、`底部行动区`；数字区域中的额度、期限、月供必须分别记录为
  `plan_field`，以便预适配识别该区域的完整版式。它们不是逐格替换合同，后续可以用不同数量的我方金额、期限和
  月供重排。`role` 只能使用 `headline`、
  `subheadline`、`benefit`、`supporting`、`cta`、`legal` 或 `plan_field`。看不清时仍记录该区块，
  `source_text` 写空字符串并降低置信度，绝不可根据图外信息补写；
- `visual_regions`：确认页的唯一核对对象。每项必须含 `id`、`location`、`kind`、`source_block_ids` 和
  `visual_bounds`；`kind` 只能是 `copy` 或 `numeric`。一个区域可以包含多块原图文字，例如利率卡的标签与
  数值；所有 `text_blocks.id` 必须恰好出现在一个区域，不能同时属于文案区域和数值区域。`numeric` 只用于
  实际展示本金、期限、月供、总利息或总还款的卡片、表格、对照区；利率卡、日利息条等即使含数值，只要不来自
  还款计划，也属于 `copy` 区域并由后续语义校验决定能否替换。区域 `visual_bounds` 按同一千分比坐标框住
  整个可变组件，供用户一次核对，不得把二维码、品牌贴片或 Prime 固定组件混入；
- `has_repayment_table`：这是唯一需要输出的结构标记。只有画面明确存在月供表、还款明细或逐月金额对照时才为
  `true`；仅出现利率、期限、`tenor`、`bunga` 或首月免息时必须为 `false`。它只决定预适配是否需要一块
  我方数值版式，不把整张图归入互斥的文案类型；
- `visual_anchors`、`palette_anchors`、`must_preserve`、`allowed_variations`；
- `detected_text`、`evidence`、`confidence`、`analysis_summary`；
- `app_ui_detected`、`app_ui_type`、`app_ui_visual_characteristics`；
- `layout_constraints`、`edge_content_density`。

`text_blocks` 只覆盖后续需要重写的正文与计划字段。底部应用商店徽章、合作方标识、监管/二维码贴片、
二维码旁固定条款和其他 Prime 固定组件即使能 OCR 也不得放入 `text_blocks`；它们由 Prime 贴图合同处理，
不属于文案替换或人工补文案范围。

观察到的金额、利率、期限、问题、按钮和标签都是参考证据，不是批准文案或可用金融事实。`must_preserve`
只保留业务语义、信息机制、关键主体和色彩锚点。App UI 只描述通用页面类型与可见结构，不选择品牌附件。
边缘主体只形成布局约束，不映射 Prime hard region，也不自动判失败。

写回 envelope：

```json
{
  "candidate_id": "<candidate-id>",
  "analysis_version": <exact task context value>,
  "status": "completed",
  "summary": "<concise pixel-grounded summary>",
  "result": {},
  "trigger_evidence_kind": "crawl_run",
  "trigger_evidence_ref_id": "<crawl-run-id>"
}
```

```text
multica creative source-analysis put --input-file <result.json> --output json
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

回读必须存在相同 candidate、version 和 status。写入或回读失败时保存 `status=failed` 与真实 error
code/message，并让 task 失败。不得创建或修改 Issue，不生成图片，不调用素材连接器。
