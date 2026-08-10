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

- 标准 AdaKami 组件 `logo/terms/store_badges/regulatory/afpi/pindai_legal` 必须是官方 `image` 附件；
- `image`：下载 `source_role` 对应的独立附件，保持比例 contain 到当前尺寸 `destination_rect`；
- `text`：仅允许 `custom_*` 非品牌组件；不得用于 Logo、条款、商店标、监管说明或合规标识；
- `qr`：`none` 跳过，`static` 使用冻结 `prime_qr` 附件，`dynamic` 生成已批准 payload。

组件数量、启停、文案、附件、QR mode 和每个尺寸坐标都来自冻结 config。Skill 不写死品牌组件、业务文字、
坐标或 payload。

Prime 位置只有一个来源：冻结市场包的 `prime_composition.layouts[<size>].components[*].destination_rect`
以及由它编译出的 `prime_layout_contract.layouts[<size>]`。不得根据生成图临时移动 Logo、QR、条款、商店标识或
底部合规组件；如果底图在固定区域内含文字、按钮、卡片、人物、原品牌或高纹理背景，合成脚本只记录
`body_clearance` warning，仍按冻结坐标合成并交给 QC/用户判断，不能挪组件，也不能把底图质量问题变成
Prime 阻断。

非 QR 的 `image` 组件如果源图带整块边缘背景色，合成脚本可把与边缘连通的背景色转成 alpha，但
`backdrop_rule=none` 不得额外加白底；Logo、条款、商店标识和底部合规条必须保持官方贴片观感。
QR 组件不做透明化，必须保留可解码证据。

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
layout contract、body_clearance warning、固定背景净化/alpha key evidence、QR decoded/passed。只有组件资源缺失、
尺寸合同错误、合成写入失败或 QR 无法解码这类 Prime 执行问题才让 job 失败；底图槽位不干净只作为 warning。
任一 job 失败时整包不送检，但成功 job 和失败 evidence 都保留。

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
