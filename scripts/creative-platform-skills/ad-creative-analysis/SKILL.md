---
name: multica-ad-creative-analysis
description: "当原生 task 指定一张候选图，需要读取真实像素并把主题、利益点、语义锚点和构图风险写入 Source Analysis 时使用。"
allowed-tools: Bash(multica *)
---

# 广告参考与创意理解

只读取 task context 中的 `crawl_run_id`、`candidate_id` 和 `analysis_version`。不得从 Issue 标题、评论
或其他 task 猜测候选。候选可以尚未被用户选择。本阶段必须市场中立：不得读取或选择品牌市场包、
Prime 模板、品牌 App UI 附件和批准文案。

执行：

```text
multica creative library download <candidate-id> --output-file <path> --output json
```

必须以运行时原生视觉能力读取下载文件的真实像素。标题、标签和采集元数据只能弱辅助；图片不可读
时提交失败 Source Analysis，写明 `error_code=asset_unreadable`，不得用元数据补结论。

分别识别：

- `theme` 与 `theme_elements`：赛事、节日、生活场景及可见元素；
- `primary_benefit`、`secondary_benefits`、`benefit_value`：金融利益点和可见金额/比例/期限；
- `source_semantics`：原图具体业务场景；
- `information_mechanism`：表格、卡片、步骤、对比等解释结构；
- `visual_anchors`、`palette_anchors`：关键主体、信息层级、主色家族；
- `must_preserve`、`allowed_variations`：后续三变体的固定项和可变项；
- `detected_text`、`evidence`、`confidence`：像素证据和置信度；
- `app_ui_detected`、`app_ui_type`、`app_ui_visual_characteristics`：只描述是否存在 App UI、通用页面类型
  （如首页、额度页、申请步骤、还款页）及可见结构；不选择任何品牌附件；
- `layout_constraints`、`edge_content_density`：记录画面的一般布局约束和边缘关键内容密度，不映射任何
  市场的 Prime 槽位或硬区。

竞品图上的金额、利率和期限只是观察证据，不是最终可用文案，不写入 `must_preserve`。默认保留原图
业务语义、信息机制、关键主体和主色家族。不能为了做三套变体建议换成无关场景。赛事只记录通用
视觉信号，除非资源包提供已批准资产，不建议官方 Logo、奖杯仿制、球队徽章或合作关系。

`result` 示例：

```json
{
  "theme": "世界杯 / 足球赛事",
  "theme_elements": ["足球", "球场", "欢呼人群"],
  "primary_benefit": "费用减免",
  "secondary_benefits": ["低利率"],
  "benefit_value": "Biaya turun 25%",
  "source_semantics": "以足球赛事氛围表达费用减免活动",
  "information_mechanism": "赛事主视觉加醒目的降费信息",
  "visual_anchors": ["足球", "球场", "主标题利益点"],
  "palette_anchors": ["绿色主色家族", "高对比浅色文字"],
  "must_preserve": ["足球赛事语义", "费用减免为第一信息", "绿色主色家族"],
  "allowed_variations": ["主视觉位置", "信息层级", "卡片布局"],
  "detected_text": ["Potongan biaya 25%"],
  "evidence": ["画面主标题明确出现 biaya 与 25%"],
  "visual_type": "主题活动海报",
  "analysis_summary": "足球赛事氛围承载降费主张",
  "layout_constraints": ["主标题需要保持第一视觉层级", "足球和人物需要完整可见"],
  "edge_content_density": {
    "top_left": "low",
    "top_right": "medium",
    "bottom": "low"
  },
  "app_ui_detected": false,
  "app_ui_type": null,
  "app_ui_visual_characteristics": [],
  "confidence": 0.9
}
```

用完整 envelope 写回：

```json
{
  "candidate_id": "<candidate-id>",
  "analysis_version": 1,
  "status": "completed",
  "summary": "足球赛事氛围承载降费主张",
  "result": {},
  "trigger_evidence_kind": "crawl_run",
  "trigger_evidence_ref_id": "<run-id>"
}
```

执行：

```text
multica creative source-analysis put --input-file <JSON文件> --output json
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

回读必须存在相同 `candidate_id`、`analysis_version` 和 `status=completed` 才成功结束 task。写入失败时
以 `status=failed` 保存真实 error code/message，并让 task 失败。不得新建或修改 Issue，也不得只在
评论中留下分析。

原参考图的主体靠近画布边缘只作为通用布局证据，不映射品牌模板，也不自动构成失败。市场适配、
品牌 App UI 选择和 Prime 硬区映射由 Planner 使用订单冻结的市场快照完成，最终遮挡以 Prime 成图及
独立 QC 的实际可读性为准。本角色不生成图片、不调用外部素材连接器。
