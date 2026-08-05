---
name: multica-ad-creative-market-pack-extraction
description: "当原生 task 指定一张完整成图，需要读取真实像素并发现其中全部可复用品牌、二维码、商店与合规组件，写回市场包组件候选时使用。"
allowed-tools: Bash(multica *)
---

# 市场包组件识别

只处理 task context 中的 `resource_id`、`extraction_id`、`source_attachment_id`、`source_width` 和
`source_height`。不得读取 Issue，也不得修改或发布市场包。

先读取识别记录并下载精确的来源附件：

```text
multica creative market-pack component-extraction get <resource-id> <extraction-id> --output json
multica attachment download <source-attachment-id> --output-dir <隔离目录>
```

必须使用运行时原生视觉能力读取真实像素。发现所有位于完整成图边缘、能够脱离创意底图复用的品牌与合规
组件，数量不设上限，包括 Logo、二维码、应用商店标识、行业组织标识、监管标识、法律印章、条款文字和
监管说明。主创意标题、CTA、金额卡片、人物、产品 UI 与装饰不属于 Prime 组件。

对每个候选输出原图像素坐标 `[left, top, right, bottom]`，边界必须完整包含组件且不能超出
`source_width × source_height`。分类规则：

- 已知品牌 Logo：`suggested_component_id=logo`、`suggested_role=prime_logo`、`kind=image`；
- 二维码：`suggested_component_id=qr`、`suggested_role=prime_qr`、`kind=qr`；
- 商店标识：`suggested_component_id=store_badges`、`suggested_role=prime_store_badges`；
- AFPI / Pindai：分别使用 `afpi/prime_afpi`、`pindai_legal/prime_pindai_legal`；
- 条款和监管文字：分别建议 `terms` 或 `regulatory`，使用 `kind=text` 并把可见原文写入 `content`；
- 其他可复用标识：建议 ID 与 role 留空，由用户确认时创建自定义组件。

不要为了凑预设数量编造候选。图片中只有 2 个就提交 2 个，发现 8 个就提交 8 个；无法确认用途时使用
中性标签并降低 `confidence`。每项 `evidence` 只写可见像素依据。

写回完整结果：

```json
{
  "status": "completed",
  "summary": "发现 4 个可复用品牌与合规组件",
  "candidates": [
    {
      "id": "candidate_1",
      "label": "品牌 Logo",
      "kind": "image",
      "suggested_component_id": "logo",
      "suggested_role": "prime_logo",
      "content": "",
      "rect": [30, 28, 314, 100],
      "confidence": 0.98,
      "evidence": ["左上角为独立品牌字标"]
    }
  ],
  "error_message": ""
}
```

```text
multica creative market-pack component-extraction put <resource-id> <extraction-id> --input-file <结果.json> --output json
multica creative market-pack component-extraction get <resource-id> <extraction-id> --output json
```

回读必须为同一 `extraction_id` 且 `status=completed` 才能结束 task。图片不可读时写回 `status=failed` 和
真实 `error_message`，然后让 task 失败。候选只是待确认数据；不得自行裁图、上传组件、修改布局或发布版本。
