---
name: multica-ad-creative-prime-compose
description: "当 Creative Order Variant 的当前 revision 已具备 expected_sizes 无品牌底图，需要按冻结市场包确定性合成 schema-v2 Prime 并委派双路 QC 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# Prime 合成

只处理 task context 指定的 Order、Variant、revision 和 `expected_sizes`。

```text
multica creative order get <order-id> --output json
```

选择同一 variant/revision、size 属于 `expected_sizes` 的 completed `stage=generated` assets。缺失、重复、
revision 错配或夹带其他尺寸时进入 `action_required`，不得补造尺寸。

## 冻结市场合同

只从 order `input_snapshot` 读取已发布市场资源包的 ID/version/config/file IDs、`prime_composition`、
`prime_layout_contract`、`qr_validation` 和 `naming_rule`。不得读取最新市场包、本机固定路径或自选 URL。
`prime_composition.schema_version` 必须为 2；生产不回退整版模板。

按 config 中 `enabled=true` 的组件和数组顺序合成：

- `image`：下载 `source_role` 对应的独立附件，保持比例 contain 到当前尺寸 `destination_rect`；
- `text`：逐字渲染 config 的 `content`，style 缺省时使用脚本默认值；
- `qr`：`none` 跳过，`static` 使用冻结 `prime_qr` 附件，`dynamic` 生成已批准 payload。

组件数量、启停、文案、附件、QR mode 和每个尺寸坐标都来自冻结 config。Skill 不写死品牌组件、业务文字、
坐标或 payload。

## 批量合成

建立隔离目录；相同附件只下载一次。manifest 必须绑定一个 variant/revision、原样的 `expected_sizes`、完整
冻结 composition/layout/QR 合同、sources，以及每个 size 恰好一个 job。job 明确 generated input、final
output、size、revision 和 variant identity。

复用只能按整包判断：每个 expected size 都来自同一 package，且有成功 compose、尺寸匹配 layout contract
和独立 QR evidence。任一证据缺失或冲突时，下载同 revision generated assets 并对全部 expected sizes 重新
本地合成；不得拼接新旧 Prime，也不得调用图像模型。

```text
python <当前 Skill 目录>/references/image_prime_compose.py \
  --manifest <manifest.json> > <compose-result.json>
```

保留脚本原始结果。不得删除或改写 package identity、job status/size/revision、canvas、components、
layout contract、QR decoded/passed。任一 job 失败时整包不送检，但成功 job 和失败 evidence 都保留。

## 资产登记

按冻结 naming rule 命名并上传 final PNG、manifest 和 compose result。每个 `stage=primed` asset 写入：

- variant、asset family、size、revision、attachment 和 generated lineage；
- market pack ID/version 与组件 source file IDs；
- 同一 manifest/result attachment、package contract version、compose 和 QR evidence。

```text
multica attachment upload <file> --output json
multica creative order asset-put <order-id> --input-file <primed-asset.json> --output json
```

## 委派 QC

全部 expected sizes 登记后，查询 reviewer 的 `creative_order_variant_qc` source，逐项比较
`<variant-id>:technical:r<revision>` 和 `<variant-id>:visual:r<revision>`。把缺失 lane 放进同一个 fanout；
source ref 固定为 Variant ID。context 固定 `type=creative_domain_task`，workflow 分别为
`creative_qc_technical` / `creative_qc_visual`，并原样携带 issue/leader/order/item/variant IDs、revision 和
`expected_sizes`。

```text
multica task by-source list --agent <reviewer-agent-id> \
  --kind creative_order_variant_qc --ref <variant-id> --output json
multica task fanout --agent <reviewer-agent-id> --input-file <qc-manifest.json> --output json
```

提交后立即结束，不等待 QC，不创建或修改 Issue。合成、上传、登记或 fanout 失败时保留当前成功资产，
写真实 error code/message 并让 task 失败；重试只补齐同 revision 缺失结果。
