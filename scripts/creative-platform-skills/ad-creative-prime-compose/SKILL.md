---
name: multica-ad-creative-prime-compose
description: "为一个 Creative Order Variant 的 expected_sizes 底图按冻结市场包确定性合成 schema-v2 Prime 组件，并登记 primed assets 时使用。"
---

# Prime 合成

只处理 task context 指定的 `<order-id>`、`<variant-id>`、revision 和 `expected_sizes`。先运行
`multica creative order get <order-id> --output json`，选择该 variant/revision 中 size_key 属于
`expected_sizes` 的 `stage: "generated"` assets。缺失、重复或混入其他 revision 时进入 `action_required`，
不得补造尺寸。

一次 Prime task 批量处理该 variant/revision 的全部 `expected_sizes`。定向返工可以只包含 1-3 个受影响尺寸；
复用必须按整包判断：每个尺寸都能解析出同一 variant/revision、原 expected_sizes、成功 compose 结果、
尺寸匹配的 `layout_contract` 和独立可复解码 QR 证据时，才复用全部 primed 资产。任一尺寸证据缺失、冲突
或来自旧 revision 时，不得混用新旧 Prime；直接下载同 revision 已完成的 generated 底图，对全部
`expected_sizes` 重新执行本地 Prime 合成。此恢复不调用图片模型，也不重新生成底图。只从订单冻结的已发布市场资源包快照读取附件、
`prime_composition`、`qr_validation`、`naming_rule` 和发布时编译的 `prime_layout_contract`。禁止读取本机
固定路径、未冻结市场包或自选网址。

## Schema v2

`prime_composition.schema_version` 必须为 `2`。没有 v2 配置时进入 `action_required`；生产运行禁止回退到
三张整版 Prime 模板。

组件数组顺序就是叠加顺序，只处理 `enabled=true` 的组件：

- `image`：必须有独立 `source_role`。下载完整附件，保持宽高比，以 contain 方式居中放入该尺寸的
  `destination_rect`，不得裁图或读取 `source_rect`。
- `text`：从组件 `content` 渲染到 `destination_rect`。布局可选 `style`；没有 style 时使用脚本的可读默认值。
- `qr`：由 `qr_mode` 决定；`none` 跳过，`static` 复用唯一 `prime_qr` 独立附件，`dynamic` 现场生成批准 payload。

标准组件 ID 为 `logo`、`terms`、`qr`、`store_badges`、`regulatory`、`afpi`、`pindai_legal`，自定义
组件使用 `custom_*`。三个尺寸只在 `layouts[<size>].components[<id>].destination_rect` 上不同；同一
`source_role` 附件和同一份文本内容必须跨尺寸复用。启停组件、替换附件、修改文本和拖动矩形都属于市场包
配置，不得写死在任务提示中。

QR 规则：

- `none`：不要求 payload、不贴 QR、不解码；
- `static`：必须只有一个 `prime_qr` 附件，`qr_validation.status=passed`、mode 为 `static`，最终每张 PNG
  必须解码为 `approved_payload`；
- `dynamic`：不读取二维码附件，`qr_validation.status=passed`、mode 为 `dynamic`，在目标矩形内生成
  `approved_payload`，最终每张 PNG 必须解码为同一值。

## 批量执行

为任务建立隔离目录，下载无品牌底图和所有启用 image/static QR 组件的附件。相同资源只下载一次并放在
manifest 顶层 `sources`；每个 job 明确声明 variant、size、revision、输入和输出：

```json
{
  "package_contract_version": 1,
  "variant_id": "<variant-id>",
  "variant_key": "V01",
  "revision": 1,
  "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
  "prime_composition": {
    "schema_version": 2,
    "qr_mode": "static",
    "components": [
      {"id":"logo","label":"Logo","kind":"image","enabled":true,"source_role":"prime_logo","content":"","backdrop_rule":"none"},
      {"id":"terms","label":"Terms","kind":"text","enabled":true,"source_role":"","content":"Syarat dan ketentuan berlaku","backdrop_rule":"quiet"},
      {"id":"qr","label":"QR","kind":"qr","enabled":true,"source_role":"prime_qr","content":"","backdrop_rule":"light"}
    ],
    "layouts": {
      "1080x1080":{"components":{"logo":{"destination_rect":[30,28,314,100]},"terms":{"destination_rect":[777,32,982,95]},"qr":{"destination_rect":[983,29,1053,99]}}},
      "1200x628":{"components":{"logo":{"destination_rect":[21,22,214,70]},"terms":{"destination_rect":[989,24,1130,68]},"qr":{"destination_rect":[1133,20,1185,73]}}},
      "800x1000":{"components":{"logo":{"destination_rect":[25,23,234,76]},"terms":{"destination_rect":[573,26,724,73]},"qr":{"destination_rect":[724,22,779,76]}}}
    }
  },
  "prime_layout_contract": {
    "guide_policy": "hard_regions_compiled_from_prime_composition",
    "layouts": {
      "1080x1080":{"hard_regions":[{"id":"logo","kind":"image","x1":30,"y1":28,"x2":314,"y2":100},{"id":"terms","kind":"text","x1":777,"y1":32,"x2":982,"y2":95,"backdrop_rule":"quiet"},{"id":"qr","kind":"qr","x1":983,"y1":29,"x2":1053,"y2":99,"backdrop_rule":"light"}],"top_key_content_exclusion_end":100,"bottom_key_content_exclusion_start":1080},
      "1200x628":{"hard_regions":[{"id":"logo","kind":"image","x1":21,"y1":22,"x2":214,"y2":70},{"id":"terms","kind":"text","x1":989,"y1":24,"x2":1130,"y2":68,"backdrop_rule":"quiet"},{"id":"qr","kind":"qr","x1":1133,"y1":20,"x2":1185,"y2":73,"backdrop_rule":"light"}],"top_key_content_exclusion_end":73,"bottom_key_content_exclusion_start":628},
      "800x1000":{"hard_regions":[{"id":"logo","kind":"image","x1":25,"y1":23,"x2":234,"y2":76},{"id":"terms","kind":"text","x1":573,"y1":26,"x2":724,"y2":73,"backdrop_rule":"quiet"},{"id":"qr","kind":"qr","x1":724,"y1":22,"x2":779,"y2":76,"backdrop_rule":"light"}],"top_key_content_exclusion_end":76,"bottom_key_content_exclusion_start":1000}
    }
  },
  "qr_validation":{"status":"passed","mode":"static","approved_payload":"https://example.com/terms"},
  "sources":{"prime_logo":"shared/logo.png","prime_qr":"shared/qr.png"},
  "jobs":[
    {"id":"V01-1080x1080","variant_id":"<variant-id>","variant_key":"V01","size":"1080x1080","revision":1,"input":"V01/1080x1080/generated.png","output":"V01/1080x1080/final.png"},
    {"id":"V01-1200x628","variant_id":"<variant-id>","variant_key":"V01","size":"1200x628","revision":1,"input":"V01/1200x628/generated.png","output":"V01/1200x628/final.png"},
    {"id":"V01-800x1000","variant_id":"<variant-id>","variant_key":"V01","size":"800x1000","revision":1,"input":"V01/800x1000/generated.png","output":"V01/800x1000/final.png"}
  ]
}
```

执行并原样保存标准输出：

```bash
python <当前 Skill 目录>/references/image_prime_compose.py --manifest <manifest.json> > <compose_result.json>
```

`compose_result.json` 是机器证据，不得摘录、改名或删除整包身份字段及逐项 `status`、`size`、`revision`、
`passed`、`decoded`、`canvas`、`components` 或 `layout_contract`。脚本失败的 job 保留失败证据；该包不得
与成功旧资产拼接后送检。

## 登记与 QC

输出文件名按冻结 `naming_rule` 展开，使用订单真实 `variant_key`。上传每张 final PNG、manifest 和未经改写的
`compose_result.json`，再调用 `multica creative order asset-put` 登记 `stage: "primed"` asset。每张记录
`variant_id`、`asset_family_id`、`size_key`、revision、`attachment_id`、`derived_from_asset_id`、市场包版本、
组件 source file IDs 与 compose/QR evidence。
三张 asset 的 evidence 必须引用同一份 manifest attachment 与 compose-result attachment，并记录
`package_contract_version: 1`；否则不属于可复用 Prime 包。

全部 `expected_sizes` primed assets 完整后，查询 reviewer 已有 QC task，补齐缺失的 technical 与 visual lane，
在同一个 `multica task fanout` 中并发提交。两个 item key 分别为
`<variant-id>:technical:r<revision>` 和 `<variant-id>:visual:r<revision>`；不得串行等待。提交后立即结束。
Prime 不放行 QC，不创建或修改 Issue；执行证据保留在 Prime task 和领域 Asset。

QC fanout 必须使用 Variant 作为证据来源，两个 lane 的完整 envelope 如下。所有 UUID、revision 与
`expected_sizes` 从当前 Prime task context 原样复制；不得自行生成或省略字段。`workflow` 是 lane 标识，
每条 task 只写自己的 QC Report：

```json
{
  "trigger_evidence_kind": "creative_order_variant_qc",
  "trigger_evidence_ref_id": "<variant-id>",
  "items": [
    {
      "item_key": "<variant-id>:technical:r<revision>",
      "context": {
        "type": "creative_domain_task",
        "workflow": "creative_qc_technical",
        "issue_id": "<issue-id>",
        "leader_agent_id": "<leader-agent-id>",
        "creative_order_id": "<order-id>",
        "creative_order_item_id": "<order-item-id>",
        "variant_id": "<variant-id>",
        "revision": 1,
        "expected_sizes": ["1080x1080", "1200x628", "800x1000"]
      }
    },
    {
      "item_key": "<variant-id>:visual:r<revision>",
      "context": {
        "type": "creative_domain_task",
        "workflow": "creative_qc_visual",
        "issue_id": "<issue-id>",
        "leader_agent_id": "<leader-agent-id>",
        "creative_order_id": "<order-id>",
        "creative_order_item_id": "<order-item-id>",
        "variant_id": "<variant-id>",
        "revision": 1,
        "expected_sizes": ["1080x1080", "1200x628", "800x1000"]
      }
    }
  ]
}
```

写文件后先用 JSON 解析器校验，再调用：

```bash
multica task fanout --agent <reviewer-agent-id> --input-file <qc-manifest.json> --output json
```
