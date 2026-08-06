---
name: multica-ad-creative-market-pack-extraction
description: "当原生 task 指定一张完整成图，需要读取真实像素并发现数量不定的可复用品牌、二维码、商店和合规组件候选时使用。"
allowed-tools: Bash(multica *)
---

# 市场包组件识别

只使用 task context 的 `resource_id`、`extraction_id`、`source_attachment_id`、`source_width` 和
`source_height`。不得读取 Issue，也不得修改或发布市场包。

```text
multica creative market-pack component-extraction get <resource-id> <extraction-id> --output json
multica attachment download <source-attachment-id> --output-dir <isolated-dir>
```

必须读取真实像素。发现所有能脱离创意底图复用的边缘品牌与合规组件，数量不设上限；不要为满足预设槽位
编造候选。主标题、CTA、金额卡片、人物、产品 UI 和装饰不属于 Prime 组件。

每个候选给出原图像素坐标 `[left, top, right, bottom]`，完整包含组件且不超出来源尺寸。可使用平台标准
建议：`logo/prime_logo`、`qr/prime_qr`、`store_badges/prime_store_badges`、监管组织、法律标识、条款文字
和监管说明。未知组件将 suggested ID/role 留空；文字组件写可见原文；不确定时使用中性标签并降低
`confidence`。`evidence` 只写像素依据。

```json
{
  "status": "completed",
  "summary": "<发现数量和类型>",
  "candidates": [
    {
      "id": "candidate_1",
      "label": "<visible component>",
      "kind": "image",
      "suggested_component_id": "",
      "suggested_role": "",
      "content": "",
      "rect": [0, 0, 1, 1],
      "confidence": 0.9,
      "evidence": ["<pixel evidence>"]
    }
  ],
  "error_message": ""
}
```

```text
multica creative market-pack component-extraction put <resource-id> <extraction-id> \
  --input-file <result.json> --output json
multica creative market-pack component-extraction get <resource-id> <extraction-id> --output json
```

回读必须为同一 extraction 且 `status=completed`。图片不可读或写回失败时保存 `status=failed` 与真实
`error_message`，然后让 task 失败。候选仅供用户确认；不得自行裁图、上传组件、调整布局或发布版本。
