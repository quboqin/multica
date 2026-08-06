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
- `creative_type_hint`：存在月供表或逐月还款明细时为 `repayment_plan`，否则为 `num`；这是跨市场结构标签，
  不根据具体语言或国家品牌猜测；
- `copy_slots`：只列出原图真实存在的文案职责，可选值为 `headline`、`subheadline`、`benefit`、
  `supporting`、`cta`、`legal`。没有按钮或行动区时不得输出 `cta`；
- `has_repayment_table`：只有画面明确存在月供表、还款明细或逐月金额对照时才为 `true`；仅出现利率、期限、
  `tenor`、`bunga` 或首月免息时必须为 `false`；
- `visual_anchors`、`palette_anchors`、`must_preserve`、`allowed_variations`；
- `detected_text`、`evidence`、`confidence`、`analysis_summary`；
- `app_ui_detected`、`app_ui_type`、`app_ui_visual_characteristics`；
- `layout_constraints`、`edge_content_density`。

观察到的金额、利率、期限、问题、按钮和标签都是参考证据，不是批准文案或可用金融事实。`must_preserve`
只保留业务语义、信息机制、关键主体和色彩锚点。App UI 只描述通用页面类型与可见结构，不选择品牌附件。
边缘主体只形成布局约束，不映射 Prime hard region，也不自动判失败。

写回 envelope：

```json
{
  "candidate_id": "<candidate-id>",
  "analysis_version": 1,
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
