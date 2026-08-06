---
name: multica-ad-creative-direct-edit
description: "按用户自然语言对 Creative Order 的无品牌底图做定向修改，并将结果写回 native asset 记录时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告图片直接修改

这是 Creative Order 的快速修改路径，不执行素材爬取、参考分析、三套新创意或模板绘制。任务 context 必须
给出 `<order-id>`、`<variant-id>`、`revision`、`expected_sizes`、`user_request`、`delivery_mode`、
`prime_agent_id`、`reviewer_agent_id` 和可编辑的无品牌 base asset ID。没有这些字段，或只有带
Prime/二维码/Logo/条款/商店徽章的最终图而无法追溯 base
asset 时，将 variant 写为 `action_required` 并请求对应底图；不得把最终图作为模型输入。

`expected_sizes` 是本次直接修改和正式发布的完整尺寸集合，当前单图入口为 `[target_size]`，以后批量入口
可以是 1-3 个已声明尺寸。它必须逐级原样传给 Prime、两路 QC 和 finalize；不得为了满足标准生产的三尺寸
规则补造未修改的尺寸。

先执行 `multica creative order get <order-id> --output json` 确认 source base asset 属于该 variant、尺寸和当前
revision。task context 的 `revision` 是 `source_revision`；本次 `output_revision=source_revision+1`。source base
永远不可覆盖：输出必须把 variant 推进到 `output_revision`，并以 source asset ID 写入
`derived_from_asset_id`。使用 `multica attachment download <base-attachment-id> --output-dir <work-dir>` 下载底图，将用户原话
原样转为局部编辑提示词，明确要改、保留的主体、文本和布局。只调用 `multica image edit`；用户明确列出
多个尺寸时，以独立 jobs 使用一次 `multica image edit-batch`。同一变体多尺寸必须保持同一修改意图和
内容族。订单冻结的 `copy_snapshot` 仍是金融事实唯一真值；用户请求若包含新的金额、利率或期限，先进入
`action_required` 让用户在页面更新文案快照，不得由图片修改角色直接写入。

只检查底图自身：修改落实、要求保留内容仍在、错字、竞品品牌、生成二维码/Logo 与明显破图。实际 Prime
硬区风险只记录为 `pending_prime_qc`；不得因原图人物、手臂、模型、装饰或几何位置进入矩形而阻断。每个
尺寸最多一次有明确原因的定向返工，`--max-attempts` 的传输重试不计入这一轮。

上传修改后的无品牌底图和 evidence：

```bash
multica attachment upload <edited.png> --output json
```

先以 `multica creative order variant-put <order-id>` 把同一 variant 的 revision 推进至
`source_revision + 1`，再以返回 ID 新建同一尺寸下一 revision 的 `stage: "generated"` asset：

```bash
multica creative order asset-put <order-id> --input-file <edited-asset.json> --output json
```

`edited-asset.json` 必须保留原 `asset_family_id`，写入 `variant_id`、`size_key`、`revision: source_revision + 1`、
`derived_from_asset_id: source_asset_id`、`status: "completed"`，并在 metadata/evidence 记录输入附件 ID、用户原话、完整
提示词、模型、request ID、attempts、provider slot limit、尺寸和目检结论。完整提示词必须原样取自
`multica image edit --output json` 或 `image edit-batch` 对应 job 返回的 `prompt`，并把同一结果的
`prompt_sha256` 写入 evidence；不得根据用户原话或本地文件二次重建。CLI 原始 JSON 作为本次生成 evidence
保留。附件上传能力不存在时，仅可
使用平台已返回的附件 ID 或请求领域 attachment API，不得走旧交付渠道。

`delivery_mode: preview` 只保留 generated asset，不创建 Prime/QC。`delivery_mode: publish` 从当前 task
context 读取 `prime_agent_id`、`reviewer_agent_id`、`issue_id` 和 `leader_agent_id`，查询：

```bash
multica task by-source list --agent <prime-agent-id> \
  --kind creative_order_variant_prime --ref <variant-id> --output json
```

按 `source + item_key=<variant-id>:r<output_revision>` 检查；仅当该 item 没有 active/succeeded task 且
`expected_sizes` 尚无完整 primed assets 时，fanout 一个 Prime task。manifest 的 evidence kind 为
`creative_order_variant_prime`、ref 为 variant ID；context 使用 `type: creative_domain_task`、
`workflow: creative_prime`，原样携带 `creative_order_id`、`issue_id`、`leader_agent_id`、order item/variant ID、
`revision: output_revision`、`expected_sizes`、`reviewer_agent_id` 和本次 generated asset ID：

```bash
multica task fanout --agent <prime-agent-id> --input-file <prime-manifest.json> --output json
```

提交后立即结束，不轮询。Prime 完成后由 Prime Skill 为同一 variant/revision 并发委派 technical 与 visual
两个 native QC task。所有 task context 都携带完全相同的 `expected_sizes`。本流程只创建原生 task 和领域
记录，不得创建或修改分析、变体、Prime、QC 子 Issue。
Prime、二维码和合规资产永远由确定性 Prime 流程生成，不由图像模型重绘。
