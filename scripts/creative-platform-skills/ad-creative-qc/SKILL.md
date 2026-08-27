---
name: multica-ad-creative-qc
description: "当 4-5 个候选主视觉需要原子晋级 3 个，或标准/精准改图 Variant 的实际交付尺寸需要视觉终检时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

先按 task context `workflow` 选择唯一分支：`creative_candidate_selection` 或 `creative_qc_visual`。不得在一次 task 中混跑。
技术质检已下线，新流程只接受上述两个 workflow，不创建、不等待、不处理 technical lane；其他 workflow 直接按无效任务失败。

```text
multica creative order get <order-id> --output json
```

## 候选主视觉晋级

`creative_candidate_selection` 只比较同一 Order Item 中已经完成当前 revision、各自 `primary_size` 的 completed
`stage=primed` asset 且 Prime `visual_adequacy.adequate=true` 的 4-5 个候选。不得使用 generated 底图、页面缩略图、旧 revision 或其他 item。
计划 5 个候选时，若其中 1 个在有界恢复后已经是终态失败且没有合格主图，平台会原子标记该候选为 `rejected`，允许其余 4 个进入比较；
少于 4 个合格候选时不得创建或执行比较，也不得由 QC 自行把失败候选改成 reserve、伪造主图或降低 Prime 门槛。
task source 必须是 `trigger_evidence_kind=creative_order_item_candidate_selection`、ref 为当前 item ID，且
`item_key=candidate-selection:v1`；context 必须携带 `creative_order_id`、`creative_order_item_id` 和这 4-5 个候选的主图引用。
context 的 order/item 与回读订单不一致、主图引用无法逐一归属当前候选时停止，不能按候选名称猜。

逐张下载并用 `view_image` 查看当前 4-5 张合格主尺寸 Prime 图，再放在同一比较上下文中独立评分。`candidate-comparison.json` 对每个 Variant
记录 0-100 分、观察证据和以下固定分项：批准文案/金融事实可读性 25、视觉吸引力 25、创意假设清晰度 15、相对其他候选的差异度 15、
三尺寸可扩展性 15、Prime 融合 5。三尺寸可扩展性必须结合 brief 的 `layout_plans`，检查主体裁切容忍、横竖重排、表格密度、App UI 和
Prime 承托风险；不能因为方图本身好看就默认可扩展。

按总分排序并用分项证据处理同分，恰好选择 3 个。选择必须保留不同 CreativeHypothesis，不能让三个近似换色方向同时晋级。
将 rank 1-3 的 Variant ID 按顺序写入 `selected_ids`，其余 1-2 个合格候选按 rank 4-5 顺序写入 `reserve_ids`：

```json
{
  "selected_ids": ["<rank-1-variant-id>", "<rank-2-variant-id>", "<rank-3-variant-id>"],
  "reserve_ids": ["<rank-4-variant-id>", "<rank-5-variant-id>"]
}
```

```text
multica creative order candidate-select <order-id> <item-id> \
  --input-file <selection.json> --output json
```

`candidate-select` 是唯一晋级入口；不得逐条 `variant-put` 改状态。命令原子设置 selected rank 和 reserve，扩展三个 selected 的
三尺寸范围，并由平台排入缺失尺寸生产；已有主图原样复用。调用后回读订单，确认恰好 3 个 selected、其余 reserve、rank 顺序、主图仍在，
以及 selected expansion task 已存在或平台明确返回已齐全。不要再次 fanout，避免重复出图。候选初筛不写 QC Report、不调用
`qc-put`/`qc-finalize`，也不把 reserve 删除或标为失败。

## 完整交付终检

以下仅适用于 `creative_qc_visual`。

校验 context `issue_id` 与订单一致，并只读取同 Variant/revision/expected sizes 的 completed
`stage=primed` assets。缺失、重复、revision 错配或夹带未声明尺寸时，visual lane 失败；不得按
Issue、评论、Variant 展示名或 Agent 名称猜输入。

## Visual 原生看图检查

visual lane 必须从 `creative order get` 返回的当前 Variant 中筛选
`revision=<context.revision>`、`stage=primed`、`status=completed` 且 size 属于 `expected_sizes` 的 assets；
只使用这些 asset 的 `attachment_id`，不得使用历史工作目录、页面预览图、生成前底图、兄弟 Variant 或旧 revision。

为每个 expected size 顺序下载对应的 Prime 成图到当前 task workdir：

```text
mkdir -p <visual-inspection-dir>
multica attachment download <primed-attachment-id> --output-dir <visual-inspection-dir> --output json
```

下载后必须用 `view_image` 查看每张本地 Prime 成图；多尺寸时逐张查看，不把图片转成 base64/stdout，不用 OCR 或
`qc_batch.py` 代替视觉判断。下载失败、数量缺失、重复尺寸、revision 不符或非图片文件，按证据/附件合同错误写 failed。
visual lane 自己写 `visual-inspection.json`；报告中的 `checked_assets` 必须列出每个检查过的
`size_key`、`attachment_id`、本地文件名和主要 observations。

## 视觉质检合同

通过 `attachment download` 下载当前 primed assets，并用 `view_image` 对照冻结 `copy_snapshot`、
  brief 和当前成图检查批准文案、金融事实、主题、主体、信息层级和画质。generated evidence 的 CreativeIntent、
  DesignDNA、LayoutPlan 哈希和 compose_result 归属来自订单证据包；多个 `expected_sizes` 必须在同一视觉上下文中联合检查，不得逐张通过后直接相加。

visual lane 必须按五个闸门验收：文案/组件完整、Prime 合成前后遮挡、官方 Prime 局部可读性、交付尺寸间 DesignDNA 一致性、
直接改图多目标完整性。
第一闸门检查冻结标题、利益点、金额、表格、CTA 是否全部出现；有边框但没有文字也算失败。第二闸门对照机器证据中的
`safe_content_frame`、`top_key_content_exclusion_end`、`bottom_key_content_exclusion_start`，确认正文、金额、表格和 CTA 没有进入顶部或底部 Prime 禁区；
正文被 Prime 实际盖住都算失败。第三闸门逐一放大 Logo、条款和底部组件，必须能看清官方文字，不能用整条带平均颜色代替局部判断。
不得把 hard region 框线当成视觉证据，也不得因为正常搭接、背景物体靠近但文字仍清晰而报错。第三闸门必须逐尺寸消费
asset evidence 顶层 `template_selection`：要求 `selection_scope=delivery_size`、`visual_adequacy.adequate=true`，并核对 selected candidate 的
`visible_component_mask`、`foreground_polarity`、`background_support.polarity`、`relative_luminance_contrast` 与 `texture`。
`background_support.relative_luminance_contrast.basis` 必须是
`alpha_composited_template_over_generated_body`；缺失或使用其他 basis 视为 Prime 证据合同错误，不能用背景采样替代。
亮色官方字形通常需要深色承托，深色字形通常需要浅色承托，mixed 必须按 component mask 分区判断；最终目标完全来自结构化极性，
不得硬编码任何背景极性，也不得用整条带平均 RGB 代替实际可见字形 mask。任一尺寸存在
`prime_relative_luminance_contrast_below_threshold`、`prime_background_polarity_mismatch`、`prime_background_too_textured` 或
`prime_visible_component_mask_missing`、`prime_template_dominant_bright_patch` 或 `prime_no_adequate_template_for_size` 时不能人工自报通过。

第四闸门在 `expected_sizes` 多于一个时把全部交付尺寸并排检查：CreativeIntent、DesignDNA 哈希和批准 copy 必须一致；主体身份/类别、场景逻辑、色彩角色、材质、光线、
视觉母题、信息层级和阅读关系应属于同一设计族，同时允许 LayoutPlan 指定的原生重排、裁切与尺度变化。只要有一个尺寸成为另一套创意，写
`cross_size_design_dna_mismatch` 并指出离群尺寸；不能为迁就一个尺寸而让另外两个一起重生。

第五闸门只在 brief/证据包含 direct-edit `intent-plan` 时生效：逐项核对每个 `target_masks[*].target_id` 的 acceptance checks，
并确认 `locked_set` 无回归。必须所有目标在同一最终成图上同时通过；标题修好但表格回退、顶部避让但底部重新遮挡、或只完成部分红框都写
`direct_edit_target_incomplete`，不能平均后通过。

检查 Prime 整体观感：官方组件应清晰但不应在可替换的已批准模板家族中表现为突兀贴片，也不应因与底图过于接近而失去辨识。
QC 的 warning 必须附尺寸、贴片位置、selected family 与结构化极性证据；它不重画二维码或手动修改 Prime 像素。装饰纹理和一般视觉平衡仅可形成 warning。
brief 的 `mechanism_adaptation` 是结构验收合同；
`omitted_unapproved_copy` 中的竞品文字不得作为必现文本。brief 自相矛盾时记录
`brief_copy_contract_conflict`，建议 replan，不把未批准文字缺失判为图片质量失败。以下肉眼可见的最终图缺陷可以阻断：
正文/金额/表格/CTA 被官方模板内容实际盖住而不可读，或官方模板文字与结构化承托极性、相对亮度/纹理门槛不匹配，可以触发一次有界底图返工。
分别写 `actual_prime_obstruction` 或 `official_prime_text_unreadable`。冻结关键内容缺失仍须写入阻断报告，但由人工决定重做或调整，不触发自动模型返工。
其他发现仍写尺寸级 `quality_warnings`。

visual lane 输出逐尺寸 checked assets、`prime_assets_readable`、`key_content_preserved`、`polarity_evidence`、
`joint_size_acceptance`、可选 `direct_edit_target_results`、`quality_warnings` 和 evidence attachments。
`joint_size_acceptance` 必须列出每个 `expected_sizes` 的 DesignDNA/CreativeIntent hash、每个不变量的逐尺寸结论和离群尺寸；不能只写总布尔值。
单尺寸精准改图将该闸门记录为 `not_applicable`，但其他四个闸门和最终视觉质检全部照常执行，不能据此跳过 QC。
没有阻断问题写 `passed`；只有 warning 时写 `warning`。存在上述实际缺陷时 visual lane 写
`failed`，并在 `blocking_failures` 放每个失败尺寸一个对象：

```json
{
  "code": "actual_prime_obstruction",
  "size_key": "1200x628",
  "diagnosis": "1200x628：还款表格 与 bottom Prime legal template content 冲突；期望移动到 safe_content_frame 内 y<=430"
}
```

`official_prime_text_unreadable` 的 diagnosis 使用同一格式，末段必须引用该尺寸 selected template 的
`foreground_polarity`、所需 `background_support.polarity`、失败的 contrast/texture threshold 和 component mask，写成
`期望承托区匹配结构化极性并达到证据中的相对亮度与纹理门槛`；禁止把任何背景极性写成固定常量。
`generated_content_missing` 的 diagnosis 必须写明 `期望补齐冻结文案 copy_snapshot`，但该问题不属于自动模型返工范围。
遮挡诊断的冲突词可以使用 `冲突`、`重叠`、`叠压`、`遮挡` 或明确的 `进入顶部 Prime 禁区`/`进入底部 Prime 禁区`，并始终保留 `期望移动到 safe_content_frame ...` 的目标坐标。
这是对最终图的视觉结论，不是区域脚本推断。模型收到后只改无品牌底图；它不能画横条、白块或品牌组件占位物。

```text
multica creative order qc-put <order-id> --input-file <qc-report.json> --output json
multica creative order qc-finalize <order-id> \
  --variant <variant-id> --revision <revision> --output json
```

写回后立即调用 finalize。finalize 只等待 visual 检测报告归档后登记最终成图；不可自动修复的视觉失败进入人工处理。
上述完整的 visual blocking finding 会新建下一 revision 的生产任务：服务端保留通过尺寸的
无品牌底图，只让模型改失败尺寸，然后重新合成品牌组件与 QC。服务端最多排两轮真实视觉返工；耗尽后不得把未通过的 staging revision
发布为交付资产。已有 active revision 时继续展示原 active，失败的 staging 保留完整证据；首次交付尚无 active 时，平台按候选排名自动晋级
最低序号 reserve 并只生产其缺失尺寸。没有可晋级 reserve 时进入人工处理，只有用户明确接受风险后才能发布。`outcome=pending` 或 `created=false` 时立即结束；只有
`created=true` 的 winner 写一次去重 Issue 留痕。结构化 findings 不复制进评论。

附件下载、查看图片或写回失败仍如实写入结构化错误；这类是平台执行错误，不是用户需要处理的图片质量问题。服务端会自动复用已完成 Prime
资产重跑 visual QC，当前 revision 最多一次；恢复任务只补视觉验收，不重新生图。

完整模板按上传文件原样 alpha 叠加；二维码属于官方 Prime 模板资产，机器检查不再解码二维码，只记录跳过状态且不得因此阻断。QC 不直接改图。它只提交最终视觉结论；符合上述合同的自动返工由服务端创建生产任务。工具或写回失败只影响当前 lane，
保留真实 error code/message，不影响兄弟 Variant，也不创建子 Issue。
