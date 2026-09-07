---
name: multica-ad-creative-qc
description: "当候选主视觉需要按订单冻结套数原子晋级，或标准/精准改图 Variant 的实际交付尺寸需要视觉终检时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

`copy_snapshot.source_kind=copy_library` 时，文案与金融事实验收只覆盖冻结的非空文字和已选 `repayment_plan_entries`。
空主标题、核心利益点、CTA 或还款计划不是缺失错误；纯视觉探索按 `visual_only=true` 验收原创视觉和 Prime。
对任何自行补写的业务声明、数字、利率、还款模块或空槽文字记录真实问题。不得要求竞品图或 source analysis，也不得从最新文案库补文案。

先按 task context `workflow` 选择唯一分支：`creative_candidate_selection` 或 `creative_qc_visual`。不得在一次 task 中混跑。
技术质检已下线，新流程只接受上述两个 workflow，不创建、不等待、不处理 technical lane；其他 workflow 直接按无效任务失败。

`creative_candidate_selection` 可以读取完整订单比较同一 item 的候选；`creative_qc_visual` 不可以。visual lane 必须先读取
`multica creative order qc-context <order-id> --output json`，它返回 task token 绑定的唯一 Variant、revision、expected sizes、brief、copy snapshot 和 Prime 附件。不得再调用 `creative order get`、`issue get`、评论列表或按 C01-C12 标签筛选整单来选择目标。

## 候选主视觉晋级

从订单 `input_snapshot.target_variant_count` 读取目标 N（1-10），`candidate_count` 读取候选上限 K=N+2（最多 12）；
未保存数量的历史订单按 N=3、K=5 处理。任务中的 target_variant_count 仅用于交叉核对，不得覆盖订单快照。

`creative_candidate_selection` 只比较同一 Order Item 中已经完成当前 revision、各自 `primary_size` 的 completed
`stage=primed` asset 完整的 N 至 K 个候选。新文案库订单计划 K 个，素材订单仍计划 4-5 个；终态失败且没有完整主图的候选由平台标记 `rejected` 后，恰好 N 个可用候选仍是合法集合。`visual_adequacy.status=qc_risk` 必须保留给最终 visual QC，不得在候选阶段伪造通过或因指标单独排除。不得使用 generated 底图、页面缩略图、旧 revision 或其他 item。
少于 N 个合格候选时不得晋级，也不得伪造主图、降低 Prime 门槛或把不足的数量标为交付完成。
task source 必须是 `trigger_evidence_kind=creative_order_item_candidate_selection`、ref 为当前 item ID，且
`item_key=candidate-selection:v1`；context 必须携带 `creative_order_id`、`creative_order_item_id` 和这些候选的主图引用。
这项 source 校验只检查当前 task 的 trigger evidence，绝不能拿 `creative_order.trigger_evidence_kind` 的原始建单来源（例如 `creative_crawl_run`）替代或否定它。context 的 order/item 与回读订单不一致、主图引用无法逐一归属当前候选时停止，不能按候选名称猜。

逐张下载并用 `view_image` 查看当前全部合格主尺寸 Prime 图，再在同一比较上下文中统一排名。10 套时最多比较 12 张，不能只检查前五张。`candidate-comparison.json` 对每个 Variant
记录 0-100 分、观察证据和以下固定分项：批准文案/金融事实可读性 25、视觉吸引力 25、创意假设清晰度 15、相对其他候选的差异度 15、
三尺寸可扩展性 15、Prime 融合 5。三尺寸可扩展性必须结合 brief 的 `layout_plans`，检查主体裁切容忍、横竖重排、表格密度、App UI 和
Prime 承托风险；不能因为方图本身好看就默认可扩展。

按总分排序并用分项证据处理同分，恰好选择 N 个不同 CreativeHypothesis，不能仅靠换色凑满数量。
将 rank 1-N 的 Variant ID 按顺序写入 `selected_ids`，其余 0-2 个合格候选按 N+1、N+2 排入 `reserve_ids`。只有 N 个可用候选时提交空数组 `reserve_ids: []`。
下例仅展示 N=3 的 JSON 结构，实际数组长度必须等于订单目标，N=10 时 selected_ids 必须有十项：

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

`candidate-select` 是唯一晋级入口；不得逐条 `variant-put` 改状态。命令原子设置 selected rank 和 reserve，扩展全部 N 个 selected 的
三尺寸范围，并由平台排入缺失尺寸生产；已有主图原样复用。调用后回读订单，确认恰好 N 个 selected、0-2 个 reserve、终态失败候选仍为 rejected、rank 顺序、主图仍在，
以及 selected expansion task 已存在或平台明确返回已齐全。不要再次 fanout，避免重复出图。候选初筛不写 QC Report、不调用
`qc-put`/`qc-finalize`，也不把 reserve 删除或标为失败。

## 完整交付终检

以下仅适用于 `creative_qc_visual`。

校验 `qc-context` 的 Variant/revision/attempt 与 task context 一致，并只读取该响应中同 Variant/revision/expected sizes 的 completed
`stage=primed` assets。缺失、重复、revision 错配或夹带未声明尺寸时，visual lane 失败；不得按
Issue、评论、Variant 展示名或 Agent 名称猜输入。

## Visual 原生看图检查

visual lane 必须从 `creative order qc-context` 返回的唯一 target 中使用
`revision=<context.revision>`、`stage=primed`、`status=completed` 且 size 属于 `expected_sizes` 的 assets；
只使用这些 asset 的 `attachment_id`，不得使用历史工作目录、页面预览图、生成前底图、兄弟 Variant 或旧 revision。

为每个 expected size 顺序下载对应的 Prime 成图到当前 task workdir：

```text
mkdir -p <visual-inspection-dir>
timeout --kill-after=10s 90s multica attachment download <primed-attachment-id> --output-dir <visual-inspection-dir>
```

`attachment download` 不支持 `--output`。每个 attachment 必须作为独立的、有界 shell 调用顺序下载；不得把多个下载串成一个无超时命令。任一调用失败时，带 `size_key`、`attachment_id` 和真实 stderr 记录唯一错误码：登录态失效、401 或 403 为 `auth_expired`；超时、连接重置或临时存储不可用为 `storage_timeout`；不存在、无对象或无下载 URL 为 `attachment_not_found`；不支持的 CLI 参数或命令合同不匹配为 `cli_contract_mismatch`。让当前 QC task 失败以便平台复用同一批 completed Prime assets 创建受限重试；不得继续猜测、使用旧本地文件或卡住等待。

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
不得把 hard region 框线当成视觉证据，也不得因为正常搭接、背景物体靠近但文字仍清晰而报错。`composition_mode=deterministic` 时第三闸门必须逐尺寸消费
asset evidence 顶层 `template_selection`：要求 `selection_scope=delivery_size`，并核对 selected candidate 的
`visible_component_mask`、`foreground_polarity`、`background_support.polarity`、`relative_luminance_contrast` 与 `texture`。
`background_support.relative_luminance_contrast.basis` 必须是
`alpha_composited_template_over_generated_body`；缺失或使用其他 basis 视为 Prime 证据合同错误，不能用背景采样替代。
`composition_mode=model_integrated` 时，不要求不存在的 `template_selection`、对比度阈值或 component mask。回读 evidence 的
`template_family_id`、`template_source_role` 与 `template_attachment_id`，确认它们属于同一冻结 QR-free family；然后逐张放大实际最终图，
核对官方文字、Logo、色彩和大致位置确实可见、没有被业务内容遮挡，也没有凭空出现 QR 或第二套官方组件。此模式的 `polarity_evidence` 写
`not_applicable:model_integrated_visual_inspection`，不得以机器阈值替代真实目检。
亮色官方字形通常需要深色承托，深色字形通常需要浅色承托，mixed 必须按 component mask 分区判断；最终目标完全来自结构化极性，
不得硬编码任何背景极性，也不得用整条带平均 RGB 代替实际可见字形 mask。`visual_adequacy.status=qc_risk` 或任一尺寸存在
`prime_relative_luminance_contrast_below_threshold`、`prime_background_polarity_mismatch`、`prime_background_too_textured` 或
`prime_visible_component_mask_missing`、`prime_template_dominant_bright_patch` 时必须放大真实 Prime 成图逐项判断；只有官方文字或条款实际不可读、
或正文实际被盖住时才失败，不能仅凭质量证据自动通过或自动失败。

`qc_risk` 不是“默认可读”。逐尺寸读取 `visibility_audit.background_support`：当
`minimum_local_p10 <= 1.25`，或 `maximum_local_p90 > texture.threshold` 时，这是服务端不可绕过的严重可读性失败；visual lane 必须写
`official_prime_text_unreadable`，不能写 `passed` 或把它降为 warning。其余临界样本仍须放大真实字形后独立判断。
第二闸门的 `top_key_content_exclusion_end` 和 `bottom_key_content_exclusion_start` 是正文文字边界，不是只检查 Logo/二维码的不透明像素：
冻结标题、利益点、金额、表格或 CTA 的可见字形只要进入对应禁区，就写 `actual_prime_obstruction`；`checked_assets.observations` 必须记录可见内容边界和所对照的禁区坐标，不能只声明“位于安全区”。

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

`official_prime_text_unreadable` 的 diagnosis 使用同一格式，必须引用该尺寸 selected template 的
`foreground_polarity`、所需 `background_support.polarity`、失败的 contrast/texture threshold 和 component mask，并以
`期望调整为满足 background_support.polarity 的低纹理承托背景，使该官方文字区域达到证据中的相对亮度与纹理门槛` 收尾；禁止把任何背景极性写成固定常量。
`generated_content_missing` 的 diagnosis 必须写明 `期望补齐冻结文案 copy_snapshot`，但该问题不属于自动模型返工范围。
遮挡诊断的冲突词可以使用 `冲突`、`重叠`、`叠压`、`遮挡` 或明确的 `进入顶部 Prime 禁区`/`进入底部 Prime 禁区`，并始终保留 `期望移动到 safe_content_frame ...` 的目标坐标。
这是对最终图的视觉结论，不是区域脚本推断。`deterministic` 的模型只改无品牌底图；`model_integrated` 的模型只在同一冻结 QR-free
模板上下文中调整失败业务内容，不能增加 QR、横条、白块或第二套品牌组件。

```text
multica creative order qc-put <order-id> --input-file <qc-report.json> --output json
multica creative order qc-finalize <order-id> --output json
```

`qc-put` 会把 report 的 variant、lane、revision、attempt 与 `qc-context` 的 task-bound target 做本地校验并补齐；`qc-finalize` 也只使用同一 target。坐标不一致时立即失败，绝不能改用兄弟 Variant 重试。写回后立即调用 finalize。finalize 只等待 visual 检测报告归档后登记最终成图；不可自动修复的视觉失败进入人工处理。若 `qc-finalize` 返回
5xx 或连接中断，不得重复 `qc-put`、重复调用模型、重生图或盲目重试 finalize；保留已成功的 report 与真实响应，写
`qc_finalize_transient_failure` 后结束当前 QC task。服务端会从未决 report 创建新的、只复用当前 Prime assets 的终检尝试。
上述完整的 visual blocking finding 会新建下一 revision 的生产任务：服务端保留通过尺寸的生成资产，只让模型改失败尺寸，然后按冻结模式
重新交接品牌组件与 QC。服务端最多排两轮真实视觉返工；耗尽后不得把未通过的 staging revision
发布为交付资产。已有 active revision 时继续展示原 active，失败的 staging 保留完整证据；首次交付尚无 active 时，平台按候选排名自动晋级
最低序号 reserve 并只生产其缺失尺寸。没有可晋级 reserve 时进入人工处理，只有用户明确接受风险后才能发布。`outcome=pending` 或 `created=false` 时立即结束；只有
`created=true` 的 winner 写一次去重 Issue 留痕。结构化 findings 不复制进评论。

附件下载、查看图片或写回失败仍如实写入结构化错误；这类是平台执行错误，不是用户需要处理的图片质量问题。服务端会自动复用已完成 Prime
资产重跑 visual QC，当前 revision 最多一次；恢复任务只补视觉验收，不重新生图。

`deterministic` 的完整模板按上传文件原样 alpha 叠加；`model_integrated` 只允许已验证的无二维码模板族，由实际目检而非 alpha 证据验收。
二维码属于官方 Prime 模板资产，机器检查不再解码二维码，只记录跳过状态且不得因此阻断。QC 不直接改图。它只提交最终视觉结论；符合上述合同的自动返工由服务端创建生产任务。工具或写回失败只影响当前 lane，
保留真实 error code/message，不影响兄弟 Variant，也不创建子 Issue。
