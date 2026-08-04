---
name: multica-ad-creative-production
description: "为一个 Creative Order Variant 生成并登记无品牌三尺寸底图，保留图像编辑、批处理和规范化证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告底图生产

只处理任务指定的 `<order-id>`、`<variant-id>` 和 revision。标准生产 task 的 `expected_sizes` 必须正好是
`1080x1080`、`1200x628`、`800x1000`；direct edit 不得进入本 Skill。先执行
`multica creative order get <order-id> --output json`，读取该 variant 的 `brief`、当前 assets 与同 item 的获批输入。只使用方案指定的素材和附件；需要候选原图时使用
归档库，而不是临时 URL：

```bash
multica creative library download <candidate-id> --output-file <reference-image>
```

已生成的无品牌底图可作为同变体重排的第一输入。不得把带 Prime、二维码、Logo、条款、商店徽章或
监管资产的最终图送入模型；不得爬取、重新分析竞品、重写获批文案、执行 Prime 或替代 QC。

图像调用前，用本 Skill 的校验脚本逐个检查最终提示词中的金融数字是否来自订单冻结的
`copy_snapshot`；校验不通过时不得调用模型，也不得回到文案库重选：

```bash
python <当前 Skill 目录>/references/validate_copy_snapshot.py \
  --materials-json <order-json> --candidate-id <candidate-id> \
  --prompt-file <prompt-file> --evidence <copy-validation.json>
```

使用 `multica image edit` 生成 `1080x1080` 方形母版。母版自身目检通过后，立即用一次
`multica image edit-batch --input-file <variant-batch.json> --output json` 并发生成 `1200x628` 与
`800x1000` 原生重排，`max_concurrency` 固定为 `2`。横竖版只依赖本变体的方形输出；不等待 Prime、
technical QC 或 visual QC，也不等待其他变体。三张必须保持同一内容族、主体身份、获批文字、业务语义、
信息机制和色系，只允许针对画布原生重排。

所有图像调用使用 `gpt-image-2`，记录原样提示词、模型、request ID、attempts、耗时和
`provider_slot_limit`。`--max-attempts 3` 仅对 408、429、5xx 与网络中断进行传输层短退避重试，
不等于创意返工。每个尺寸最多一次有明确原因的定向返工；成功且输入指纹不变的输出直接复用。没有
合法 image-edit 配置时，将 variant 写为 `action_required` 并记录缺失配置。

每张模型输出必须经本 Skill 的规范化脚本变为精确交付尺寸，并保留 JSON evidence：

```bash
python <当前 Skill 目录>/references/normalize_image.py \
  --input <model.png> --output <delivery.png> --width <width> --height <height> \
  --evidence <normalize-evidence.json>
```

优先使用有界 cover-resize。连续两次 provider 比例失败时才使用记录在 evidence 中的有界 full-content
edge-fade extension；不得静默裁切、拉伸或用 Pillow/SVG/Canvas/HTML/图库替代模型生成广告底图。

按 `brief.prime_layout_contract` 生成连续低纹理背景并记录每个 `hard_regions` 的风险。底图阶段只能写
`prime_clearance_status: "pending_prime_qc"`，不能声称 Prime 可读、硬区通过或实际遮挡。原图已有的
人物、手臂、房屋模型、关键卡片或装饰在这些几何区域内不是失败，也不得触发重画或阻断横竖版；只有
合成后的真实 Prime 资产压住关键内容，才由 QC 决定返工。底图自身的错字、遗漏/改写获批内容、竞品
品牌、模型生成二维码或 Logo、严重破图才可触发这一轮定向返工。

上传每张底图与 batch/normalize evidence：

```bash
multica attachment upload <delivery.png> --output json
```

将返回的 `id` 用 `asset-put` 登记。方形的成功响应会返回 `asset_family_id`；横竖版必须带同一
`asset_family_id`，确保同一 variant 的三尺寸属于同一内容族。每张 JSON 至少含：

```json
{
  "variant_id": "<variant-id>",
  "asset_family_id": "<same-family-id>",
  "size_key": "1080x1080",
  "revision": 1,
  "stage": "generated",
  "attachment_id": "<uploaded-image-id>",
  "metadata": { "prompt": "...", "model": "gpt-image-2" },
  "evidence": { "request_id": "...", "attempts": 1, "prime_clearance_status": "pending_prime_qc" },
  "status": "completed"
}
```

执行：

```bash
multica creative order asset-put <order-id> --input-file <generated-asset.json> --output json
```

同样上传并登记 batch 原始结果与规范化 evidence；它们可使用对应尺寸的 `stage: "generated"` asset
metadata/evidence 或领域 API 已返回的附件引用。附件上传能力不可用时，不要转用旧会话附件路径；只使用
已返回的领域附件 ID 或请求平台提供 domain attachment API。三张 `generated` 资产和证据完整后，用
`variant-put` 将该 variant 更新为 `partial`（等待 Prime/QC）或 `completed`（仅当当前状态机定义已完成）。

随后从当前 task context 读取 `prime_agent_id` 和 `reviewer_agent_id`，先查询该 variant 的 Prime task：

```bash
multica task by-source list --agent <prime-agent-id> \
  --kind creative_order_variant_prime --ref <variant-id> --output json
```

按 source 查询后只比较当前 `item_key=<variant-id>:r<revision>`。当且仅当该 item 没有 active/succeeded
Prime task，且当前 revision 尚未有完整 primed assets 时，fanout 一个 Prime task；旧 revision 或同 source
下其他 item 不能阻止本次委派。manifest 的 `trigger_evidence_kind` 为
`creative_order_variant_prime`、ref 为 variant ID，`item_key` 为 `<variant-id>:r<revision>`；context 使用 `type: creative_domain_task`、
`workflow: creative_prime`，并从当前 task 原样复制 `issue_id`、`leader_agent_id`，携带
order/item/variant/revision、`expected_sizes`、reviewer agent ID 和三张 generated asset ID。
提交后立即结束，不轮询 Prime：

```bash
multica task fanout --agent <prime-agent-id> --input-file <prime-manifest.json> --output json
```
