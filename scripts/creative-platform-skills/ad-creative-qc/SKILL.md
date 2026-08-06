---
name: multica-ad-creative-qc
description: "当 Creative Order Variant 当前 revision 的 Prime 包齐备，需要独立执行 technical 或 visual lane、写 QC Report 并调用事务化 finalize 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

每条 task 只执行 context 指定的 `creative_qc_technical` 或 `creative_qc_visual` lane。两 lane 使用相同且非空的
Variant、revision 和 `expected_sizes`，并各写一份 QC Report；不得合并或互相等待。

```text
multica creative order get <order-id> --output json
```

校验 context `issue_id` 与订单一致，并只读取同 Variant/revision/expected sizes 的 completed
`stage=primed` assets。缺失、重复、revision 错配、证据包不一致或夹带未声明尺寸时，本 lane 失败；不得按
Issue、评论、Variant 展示名或 Agent 名称猜输入。

## 证据下载与机器检查

顺序下载 Prime 图片、共同引用的 manifest 和 `compose_result.json`。每次下载显式设置
`MULTICA_HTTP_TIMEOUT=2m`，等待结束后再开始下一项；单项超时只顺序重试该项，不并发下载。

```text
multica attachment download <attachment-id> --output-dir <inspection-dir>
```

不得重建机器证据。旧包只有在每个 job 的 variant/size/revision/success 和有效 layout contract 都可恢复时
兼容，否则要求 Prime 用同 revision generated assets 重做整包，不调用图像模型。

```text
python <当前 Skill 目录>/references/qc_batch.py \
  --manifest <manifest.json> --compose-result <compose-result.json> \
  --images-dir <final-images-dir> --output <machine-evidence.json> \
  --contact-sheet <contact-sheet.png> --hard-region-sheet <hard-region-sheet.png>
```

## Lane 合同

- `technical`：文件、目标尺寸、manifest/compose 对应、Prime 组件、QR 独立复解码、四角/底部和
  `backdrop_rule`。
- `visual`：对照冻结 `copy_snapshot`、brief、source/generated assets 检查批准文案、金融事实、主题、主体、
  信息层级和画质；多尺寸时检查同内容族一致性。

只以最终 Prime 图实际遮挡或不可读为阻断。原始底图主体进入 hard region 的几何预测、浅色边缘、装饰
纹理和一般视觉平衡仅可形成 warning。brief 的 `mechanism_adaptation` 是结构验收合同；
`omitted_unapproved_copy` 中的竞品文字不得作为必现文本。brief 自相矛盾时记录
`brief_copy_contract_conflict`，建议 replan，不把未批准文字缺失判为图片质量失败。

每条 lane 输出逐尺寸 checked assets/regions、`actual_hard_regions_clear`、`prime_assets_readable`、
`key_content_preserved`、`blocking_failures`、`quality_warnings` 和 evidence attachments。
`blocking_failures` 非空时 status 必须为 `failed`；`passed` / `warning` 都要求空阻断。

```text
multica creative order qc-put <order-id> --input-file <qc-report.json> --output json
multica creative order qc-finalize <order-id> \
  --variant <variant-id> --revision <revision> --output json
```

每条 lane 写回后立即调用 finalize。`outcome=pending` 或 `created=false` 时立即结束；只有 `created=true` 的
winner 写一次去重 Issue 留痕：整单完成用普通评论唤醒 Leader，单 Variant 完成/阻断用 `/note`。结构化
findings 不复制进评论。

QC 不自动返工。失败只进入 `action_required`，让用户在高清对比中选择接受风险、调整、重做或放弃；同
Variant 最多一轮用户授权返工。工具或写回失败只影响当前 lane，保留真实 error code/message，不影响兄弟
Variant，也不创建子 Issue。
