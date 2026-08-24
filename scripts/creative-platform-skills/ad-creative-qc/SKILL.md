---
name: multica-ad-creative-qc
description: "当 Creative Order Variant 当前 revision 的品牌组件包齐备，需要独立执行 technical 或 visual lane、写可见的 QC 检测报告时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

每条 task 只执行 context 指定的 `creative_qc_technical` 或 `creative_qc_visual` lane。两 lane 使用相同且非空的
Variant、revision 和 `expected_sizes`，并各写一份 QC Report；不得合并或互相等待。

```text
multica creative order get <order-id> --output json
```

校验 context `issue_id` 与订单一致，并只读取同 Variant/revision/expected sizes 的 completed
`stage=primed` assets。`manifest_attachment_id` 和 `compose_result_attachment_id` 是后端合成生成的共同证据。
缺失、重复、revision 错配、证据包不一致或夹带未声明尺寸时，本 lane 失败；不得按
Issue、评论、Variant 展示名或 Agent 名称猜输入。

## Technical 证据下载与机器检查

仅 `creative_qc_technical` 执行本段。顺序下载完整品牌组件成图、context 给出的 manifest 和 compose result。每次下载显式设置
`MULTICA_HTTP_TIMEOUT=2m`，等待结束后再开始下一项；单项超时只顺序重试该项，不并发下载。

```text
multica attachment download <attachment-id> --output-dir <inspection-dir>
```

不得重建机器证据。只接受当前 v6 整模板包；合同缺失或不一致时记录后端品牌组件合成异常，不调用图像模型。

```text
python <当前 Skill 目录>/references/qc_batch.py \
  --manifest <manifest.json> --compose-result <compose-result.json> \
  --images-dir <final-images-dir> --output <machine-evidence.json> \
  --contact-sheet <contact-sheet.png>
```

## Visual 后端检查

`creative_qc_visual` 不下载图片到 agent workdir，不调用 `view_image`，不把图片转成 base64/stdout，也不运行本地 OCR 或
`qc_batch.py`。visual lane 只调用后端视觉检查入口，让服务端从 OSS 读取当前 revision 的 primed 成图、生成短时签名 URL、
调用视觉模型并上传 `visual-inspection.json` 与诊断联系表。

```text
multica creative order visual-inspect <order-id> \
  --variant <variant-id> --revision <revision> --output json \
  > visual-inspection.json
```

后端返回的 `visual-inspection.json` 就是本 lane 的 QC report：直接写回，不二次改写图片结论，不自行补造 passed。
若返回 `visual_inspection_model_unconfigured`、`visual_inspection_asset_unavailable`、
`visual_inspection_signed_url_unavailable` 或 `visual_inspection_model_error`，按返回内容写 failed，
再调用 `qc-finalize` 归档。

## Lane 合同

- `technical`：文件、目标尺寸、manifest/compose 对应、完整品牌模板、模板布局契约、四角/底部和 `backdrop_rule`。
- `visual`：通过后端 `visual-inspect` 对照冻结 `copy_snapshot`、brief 和当前 primed assets 检查批准文案、
  金融事实、主题、主体、信息层级和画质。generated evidence 的父方向哈希和 compose_result 归属由后端证据包提供；
  多尺寸时检查同内容族一致性。

visual-inspect 必须按三个闸门验收：文案/组件完整、Prime 合成前后遮挡、官方 Prime 局部可读性。
第一闸门检查冻结标题、利益点、金额、表格、CTA 是否全部出现；有边框但没有文字也算失败。第二闸门对照机器证据中的
`safe_content_frame`、`top_key_content_exclusion_end`、`bottom_key_content_exclusion_start`，确认正文、金额、表格和 CTA 没有进入顶部或底部 Prime 禁区；
正文被 Prime 实际盖住都算失败。第三闸门逐一放大 Logo、条款和底部组件，必须能看清官方文字，不能用整条带平均颜色代替局部判断。
不得把 hard region 框线当成视觉证据，也不得因为正常搭接、背景物体靠近但文字仍清晰而报错。浅色边缘、装饰纹理和一般视觉平衡仅可形成 warning。
brief 的 `mechanism_adaptation` 是结构验收合同；
`omitted_unapproved_copy` 中的竞品文字不得作为必现文本。brief 自相矛盾时记录
`brief_copy_contract_conflict`，建议 replan，不把未批准文字缺失判为图片质量失败。以下肉眼可见的最终图缺陷可以阻断：
正文/金额/表格/CTA 被官方模板内容实际盖住而不可读，或官方模板文字因底图深色或高纹理而不可读，可以触发一次有界底图返工。
分别写 `actual_prime_obstruction` 或 `official_prime_text_unreadable`。冻结关键内容缺失仍须写入阻断报告，但由人工决定重做或调整，不触发自动模型返工。
其他发现仍写尺寸级 `quality_warnings`。

每条 lane 输出逐尺寸 checked assets、`prime_assets_readable`、`key_content_preserved`、`quality_warnings` 和
evidence attachments。没有阻断问题写 `passed`；只有 warning 时写 `warning`。存在上述实际缺陷时 visual lane 写
`failed`，并在 `blocking_failures` 放每个失败尺寸一个对象：

```json
{
  "code": "actual_prime_obstruction",
  "size_key": "1200x628",
  "diagnosis": "1200x628：还款表格 与 bottom Prime legal template content 冲突；期望移动到 safe_content_frame 内 y<=430"
}
```

`official_prime_text_unreadable` 的 diagnosis 使用同一格式，末段改为 `期望调整为官方模板文字下方连续、低细节的浅色背景`。
`generated_content_missing` 的 diagnosis 必须写明 `期望补齐冻结文案 copy_snapshot`，但该问题不属于自动模型返工范围。
遮挡诊断的冲突词可以使用 `冲突`、`重叠`、`叠压`、`遮挡` 或明确的 `进入顶部 Prime 禁区`/`进入底部 Prime 禁区`，并始终保留 `期望移动到 safe_content_frame ...` 的目标坐标。
这是对最终图的视觉结论，不是区域脚本推断。模型收到后只改无品牌底图；它不能画横条、白块或品牌组件占位物。

```text
multica creative order qc-put <order-id> --input-file <qc-report.json> --output json
multica creative order qc-finalize <order-id> \
  --variant <variant-id> --revision <revision> --output json
```

每条 lane 写回后立即调用 finalize。finalize 只等待两份检测报告归档后登记最终成图；技术失败或不可自动修复的
视觉失败进入人工处理。上述四类完整的 visual blocking finding 会新建下一 revision 的生产任务：服务端保留通过尺寸的
无品牌底图，只让模型改失败尺寸，然后重新合成品牌组件与 QC。服务端最多排两轮真实视觉返工；两轮后仍失败会保留当前品牌成图和交付资产，
并以 `delivered_with_qc_risk` 归档，让用户继续标注调整或风险采用，不再因为这类视觉返工耗尽阻断输出。`outcome=pending` 或 `created=false` 时立即结束；只有
`created=true` 的 winner 写一次去重 Issue 留痕。结构化 findings 不复制进评论。

如果 generated evidence 缺少 `parent_direction_sha256`、manifest/compose 对应或其他 delegation/manifest/Prime contract，
仍然如实写入对应阻断 finding；这类是平台证据契约错误，不是用户需要处理的图片质量问题。服务端会自动复用已完成 Prime
资产重跑 technical 和 visual 两条 QC，当前 revision 最多一次；恢复任务只补证据，不重新生图。

完整模板按上传文件原样 alpha 叠加；二维码属于官方 Prime 模板资产，机器检查不再解码二维码，只记录跳过状态且不得因此阻断。QC 不直接改图。它只提交最终视觉结论；符合上述合同的自动返工由服务端创建生产任务。工具或写回失败只影响当前 lane，
保留真实 error code/message，不影响兄弟 Variant，也不创建子 Issue。
