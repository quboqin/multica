---
name: multica-ad-creative-qc
description: "对 Creative Order Variant 的 Prime 成图执行独立 technical 或 visual QC，并写入 native QC report 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

每个 variant/revision 必须有两个独立 native task，`creative_qc_technical` 和 `creative_qc_visual`。两者都从
task context 读取完全相同且非空的 `expected_sizes`，再用
`multica creative order get <order-id> --output json` 读取同一 variant、revision 中对应的
`stage: "primed"` assets。标准生产检查三个尺寸；direct edit 检查本次发布的 1-3 个受影响尺寸。两 lane
并发运行，不得合并成一个任务或将一个作为另一个的前置条件。每条任务仅写自己的 lane。
QC 不创建 Issue；task、QC Report、机器 evidence 和根订单 Issue 上的去重收口评论共同构成留痕。

开始时校验 task context 的 `issue_id` 与订单 `issue_id` 一致，并保留 `leader_agent_id`。缺失或不一致
属于委派契约错误，写失败 QC 报告后结束；不得按 Issue 标题、当前评论或 Agent 名称猜测。

下载全部 `expected_sizes` Prime 成图，以及这些 asset evidence 共同引用的原始 manifest 与
`compose_result.json`。集合缺失、重复、revision 不一致、引用不同证据包或夹带未声明尺寸时，本 lane
必须失败。Prime 成图、manifest 和 `compose_result.json` 必须逐个顺序下载；禁止后台执行、并发下载或
并发重试。每一次下载都显式使用 2 分钟 HTTP 超时。Windows PowerShell 使用：

```powershell
$env:MULTICA_HTTP_TIMEOUT='2m'; multica attachment download <attachment-id> --output-dir <inspection-dir>
```

Bash 使用等价命令：

```bash
MULTICA_HTTP_TIMEOUT=2m multica attachment download <attachment-id> --output-dir <inspection-dir>
```

等待当前命令成功或失败后才能开始下一个附件。单个附件超时时，可按相同顺序重试同一条命令；不得缩短
超时、删除超时前缀或并发发起多个重试。所有附件成功落盘后才运行批量 QC。

不得从其他来源重建 manifest，也不得临时编写 Pillow/OpenCV 校验脚本。旧包只有在每个 compose item
都能恢复尺寸、revision、成功状态和有效 `layout_contract` 时才兼容：尺寸可从 `V03-800x1000` 末尾恢复，
合同可从 compose item 恢复。缺少这些机器证据时停止 QC，要求 Prime 使用同 revision generated 底图重做
整包；不得调用图像模型。先运行本 Skill 的工具：

```bash
python <当前 Skill 目录>/references/qc_batch.py \
  --manifest <prime-packaging-manifest.json> \
  --compose-result <compose-result.json> \
  --images-dir <final-images-directory> \
  --output <qc-machine-evidence.json> \
  --contact-sheet <qc-contact-sheet.png> \
  --hard-region-sheet <qc-hard-region-sheet.png>
```

该脚本先校验 manifest/compose 是否能组成同一 variant/revision 的完整包，再检查尺寸、命名、Prime 批次
结果、QR 独立复解码、满版边缘和九宫格证据。QR 复解码依次尝试全图、2 倍 nearest、QR hard region
带 padding 裁片和裁片 2 倍 nearest；这一步独立于 Prime 的 `decoded` 结果。原 Prime 模板可高于交付
分辨率，按目标画布缩放不是失败；`edge_white_ratio_needs_visual_review` 只是人工复核提示，浅色满版
设计不得仅因边缘颜色判失败。

`technical` lane 验证文件完整性、目标尺寸、manifest/compose 对应关系、QR 解码、Prime 资产和
`backdrop_rule`。`visual` lane 对照获批 `copy_snapshot`、brief 和对应 source/generated asset，验证可见
文案、金融数值、主题、主体、业务语义、信息层级和图像质量。标准生产使用方形作为横竖版基线；direct
edit 单尺寸检查与来源的保持，多尺寸只在 `expected_sizes` 内检查同内容族。二者都打开
`qc-hard-region-sheet.png` 的原尺寸；只有 red-frame `hard_regions` 是空间硬区，顶部/底部切片是软引导。

视觉 QC 以 `copy_snapshot` 为可见文字真值。brief 的 `mechanism_adaptation` 是参考机制到获批内容的验收
合同，`omitted_unapproved_copy` 明确列出的竞品问题、选项、按钮或标签不得再作为必现文本或阻断项。
检查结构关系、阅读路径和已批准利益层级是否按 adaptation 保留，不要求生成空选项，也不要求用批准利益
字段重复冒充竞品选项。若旧 brief 同时要求某段参考文案必须出现又禁止添加该文案，记录
`brief_copy_contract_conflict`，不得把模型未生成该未批准文字误判为图片质量失败；该冲突应由用户选择
`replan`，只修订目标变体。

真实 Prime 资产不可读，或实际压住标题、获批金融数字、人脸、按钮、表格、关键卡片或正文时是阻断。
原始底图中的人物、手臂、模型、装饰或几何位置进入硬区本身不是阻断，不能在未看到合成图前误拦；
仅装饰纹理、阴影、一般视觉平衡或非关键锚点偏差记 `quality_warnings`，不扩展为自动返工。

每条 lane 写出 `blocking_failures`、`quality_warnings`、逐尺寸 `checked_region_ids`、
`actual_hard_regions_clear`、`prime_assets_readable`、`key_content_preserved`、机器 evidence 与人工结论。
`blocking_failures` 非空时 `status` 必须是 `failed`，绝不能写成 `warning` 或 `passed`；`passed` 和
`warning` 都要求 `blocking_failures=[]`。状态只能是 `passed`、`warning`、`failed` 或 `pending`。上传机器 evidence、九宫格和硬区图，取得 attachment
ID 后将其放入 findings/evidence 引用；上传不支持时仅使用已有领域附件 ID 或请求 domain attachment API。
写入：

```bash
multica creative order qc-put <order-id> --input-file <qc-report.json> --output json
```

`qc-report.json` 至少含：

```json
{
  "variant_id": "<variant-id>",
  "lane": "technical",
  "revision": 1,
  "status": "passed",
  "findings": {
    "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
    "checked_assets": ["..."],
    "blocking_failures": [],
    "quality_warnings": []
  }
}
```

写入当前 lane 后必须立即调用事务化 barrier，不再由 QC task 自己登记 delivered asset 或修改 variant：

```bash
multica creative order qc-finalize <order-id> \
  --variant <variant-id> --revision <revision> --output json
```

每条 lane 都调用一次。响应 `outcome=pending` 表示另一 lane 尚未完成，当前 task 立即结束，不轮询、不休眠；
`created=false, finalized=true` 表示另一条 task 已完成收口，幂等退出。只有 `created=true` 的 winner 处理
Issue 留痕：

- `outcome=delivered`：服务已在同一事务把 `expected_sizes` Prime 资产登记为 delivered、完成 variant 并发送 Inbox；
- `outcome=action_required`：服务只把 variant 标为需要处理并发送 Inbox，不自动创建返工 task；
- `order_aggregate_status=completed` 时，用 UTF-8 `--content-file` 向 context 的 `issue_id` 写一条非 `/note`
  收口评论，说明订单领域状态已全部完成并请 Leader 汇总；这会且只会唤醒一次 Leader；
- 单个 variant 完成或阻断只写 `/note` 进度评论，不唤醒 Leader。结构化 findings 和建议作用域已经保存在
  QC Report，评论不得复制模型日志。

QC 不得自动返工。用户在高清对比工作区查看原图、成图和实际问题区域后，选择接受风险、局部调整、
重做该变体或放弃。若选择调整，Leader 最多委派一轮，revision 加一；第二轮仍失败时停止模型调用。
返工指令只能把关键内容移出对应矩形并在其中补连续低纹理背景，不能删除人物、表格、获批文案或改变
业务语义。

两 lane 的 `expected_sizes` 或其对应资产集合不一致也属于阻断，不得由任一 QC task 自行缩小检查范围。
