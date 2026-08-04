---
name: multica-ad-creative-prime-compose
description: "为一个 Creative Order Variant 的 expected_sizes 底图确定性叠加 Prime 和动态 QR，并登记 primed assets 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# Prime 合成

只处理指定 `<order-id>`、`<variant-id>`、revision 和 task context 的 `expected_sizes`。先运行
`multica creative order get <order-id> --output json`，选择该 variant、revision 中 size_key 正好属于
`expected_sizes` 的 `stage: "generated"` assets。标准生产的集合固定为三尺寸；direct edit 可以是 1-3 个
受影响尺寸。集合缺失、重复或混入其他 revision 时进入 `action_required`，不得补造未声明尺寸。

一个 variant/revision 的一次 Prime task 必须批量处理全部 `expected_sizes`，不能拆成多个工作流；定向返工
仅替换受影响尺寸时，仍以相同 `asset_family_id`、variant 与 revision 关联，未变化输入指纹的 primed 资产
直接复用。

只从订单冻结的已发布市场资源包快照读取与 `expected_sizes` 对应的 Prime 附件、`qr_payload`、
`qr_validation`、`naming_rule` 与完整 `prime_layout_contract`。只有 `qr_validation.status=passed`，
`expected_sizes` 对应的每个模板均有机器解码值，且 `approved_payload` 与 `qr_payload` 完全一致时才可执行。
结构化 config 是操作规则，附件 ID 是实际文件，`brand_guideline` 不能覆盖两者。缺失时将 variant 写为
`action_required`，不得读取本机固定路径、未冻结市场包、自选网址或替换素材。

为每个尺寸创建独立目录，使用 `multica attachment download <attachment-id> --output-dir <dir>` 下载无品牌
底图、Prime 模板和校验证据。manifest 原样携带每个尺寸的 `hard_regions`、`backdrop_rule`、
`top_key_content_exclusion_end` 与 `bottom_key_content_exclusion_start`，禁止本地重估矩形。

用本 Skill 自带脚本一次运行全部 `expected_sizes`：

```bash
python <当前 Skill 目录>/references/image_prime_compose.py --manifest <manifest.json>
```

脚本先叠加完整透明 Prime 资产层，再在模板固有右上 QR 槽生成 `approved_payload` 的整数模块二维码，
并对最终 PNG 机器解码。每张成品解码内容必须同时等于 `approved_payload` 和相应模板的
`decoded_payload`；模板中的 Logo、条款、商店徽章、OJK、AFPI、PINDAR 与合规页脚必须原样保留。输出
文件名按 `naming_rule` 展开并包含订单中的真实 `variant_key`：标准生产为 `V01`/`V02`/`V03`，直接改图
为 `direct_edit`。不得为满足命名规则伪造另一种 variant key，执行前确认同订单内唯一。

上传全部成图、原始 manifest 与 `compose_result.json`：

```bash
multica attachment upload <final.png> --output json
```

以返回 attachment ID 对每张调用：

```bash
multica creative order asset-put <order-id> --input-file <primed-asset.json> --output json
```

每个 `primed-asset.json` 写 `variant_id`、原 generated asset 的 `asset_family_id`、`size_key`、`revision`、
`stage: "primed"`、`attachment_id`、`derived_from_asset_id`、资源包版本、模板文件和 QR/compose evidence，
并设 `status: "completed"`。附件上传不受当前 CLI 支持时，只使用领域 API 或已返回附件 ID；不得换用
旧交付接口。

全部 `expected_sizes` primed assets 完整后，从当前 task context 读取 `reviewer_agent_id`，先查询该 variant
已有的 QC task：

```bash
multica task by-source list --agent <reviewer-agent-id> \
  --kind creative_order_variant_qc --ref <variant-id> --output json
```

逐个检查 `<variant-id>:technical:r<revision>` 与 `<variant-id>:visual:r<revision>`：只把没有
active/succeeded task 且当前 revision 尚无对应完整 QC report 的 lane 加入同一个 manifest。一个 lane 已存在
不能阻止另一个 lane 补齐。`trigger_evidence_kind` 为 `creative_order_variant_qc`、ref 为 variant ID；两个
`item_key` 分别为上述值。context 都使用
`type: creative_domain_task`，从当前 task 原样复制 `issue_id`、`leader_agent_id`，并携带
order/item/variant/revision、`expected_sizes` 和对应 primed asset ID，只有 workflow 分别为
`creative_qc_technical` 与 `creative_qc_visual`。不得把它们串行化，也不得等待其中一个完成后再创建另一个：

```bash
multica task fanout --agent <reviewer-agent-id> --input-file <qc-manifest.json> --output json
```

提交后立即结束。Prime 不做 QC 放行，不创建或修改任何 Issue；执行证据保留在 Prime task 和领域 Asset。
