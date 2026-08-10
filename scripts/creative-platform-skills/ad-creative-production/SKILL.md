---
name: multica-ad-creative-production
description: "当 creative_production task 指定一个 Creative Order Variant，需要生成同内容族无品牌三尺寸底图、登记完整模型证据并委派 Prime 时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告底图生产

只处理 context 指定的 Order、Variant、revision 和 `expected_sizes`；direct edit 不进入本 Skill。

```text
multica creative order get <order-id> --output json
multica creative library download <candidate-id> --output-file <reference-image> --output json
```

每次先读取当前订单 JSON，并按 task context 的 `creative_order_item_id` 与 `variant_id` 精确定位同一个
Order Item/Variant。当前订单返回的 item `copy_snapshot` 是唯一文案快照真值；task 摘要、Issue 材料列表、
同一 `candidate_id` 的其他 item 以及历史工作目录都不得作为缺失判断依据。只使用 Variant brief、冻结
`copy_snapshot`、market snapshot 指定附件和无品牌资产。不得读取当前文案库或市场包，不爬取/重分析素材，
不执行 Prime/QC，也不得把 primed/delivered 图作为模型输入。

## 文案与提示词证据

调用模型前验证 schema-v3 copy snapshot 和最终提示词中的金融 token。存在 `pre_adaptation` 时，必须逐项使用
其 `text_replacements`、`numeric_layouts`、`repayment_plan_selections` 和 `production_prompt`；`text_replacements` 中
`status=missing` 的区块必须移除竞品原文，不得翻译、保留或杜撰占位文案。不得从通用
headline/benefit 重新发明、遗漏或替换画面文字与数值。竞品数字只参考版式，实际金额、期限、月供以冻结
数值版式为准。

每个尺寸先把 Variant brief 的 `prime_layout_contract.layouts[<size>]` 写成独立 `layout-<size>.json`，再渲染
一张不可读的 Prime 避让参考图：

```text
python <当前 Skill 目录>/references/render_prime_guide.py \
  --layout-file <layout-<size>.json> --width <model-width> --height <model-height> \
  --output <prime-guide-<size>.jpg> --evidence <prime-guide-<size>.json>
```

`prime-guide` 只作为模型第二输入的避让图，不是素材，不得登记为 Asset。模型最终输出仍是无品牌底图；真实
Prime 组件只由后续 deterministic Prime Compose 合成。

模型提示词不得原样使用或整段粘贴 `pre_adaptation.production_prompt`。最终 prompt 必须短、分段、可执行：
每条文案、还款行和版式规则只出现一次，不写推理、审计、重复表格或来源流水。每个尺寸使用这个结构：

```text
TASK:
Create one unbranded AdaKami base image. Prime will be added later by deterministic post-processing.

INPUTS:
Input 1 is reference structure/hierarchy only.
Input 2 is the Prime reserved-area guide for avoidance only; do not draw input 2, copy input 2, render guide blocks, or create placeholders.

PRIME RESERVED AREAS:
No readable text, CTA, table, amount, icon with meaning, face, hand, business card, logo, QR, footer, legal mark, store badge, OJK, AFPI, or Pindai inside the guide/reserved areas.
Reserved areas may contain only continuous background, lighting, texture, or non-critical decoration.
Also list current hard region coordinates and top/bottom exclusion boundaries from layout-<size>.json.

CONTENT LAYOUT:
safe_content_frame=(x1,y1)-(x2,y2) from prime-guide-<size>.json. CONTENT_RECT=(x1,y1)-(x2,y2). These control only readable/business content; background and decoration stay full-bleed.
HEADLINE_BOX=(...). MAIN_VALUE_BOX=(...). TABLE_BOX=(...). BENEFIT_BOX=(...). CTA_BOX=(...).
All approved text must stay inside its assigned box. CTA_BOX bottom edge must be <= CONTENT_RECT.y2 and leave readable visible clearance before bottom_prime_band_start=<y>.
If crowded, reduce decoration, spacing, and font scale; never move text, cards, CTA, benefits, or icons outside the boxes.

APPROVED TEXT:
List each approved visible text once.

TABLE:
List approved rows once, or state No table.

STYLE:
Use frozen `pre_adaptation.production_prompt` only as a compact style/layout summary. Reference is structure only; remove source identity and apply brief `visual_identity_strategy`/`anti_copy_changes`.

FORBIDDEN:
No extra readable text, numbers, claims, people, app UI, source brand, Prime components, centered inset poster, border, blurred sidebars, crop, stretch, or guide artifacts.

FINAL:
Full-bleed clean base only, all business content readable and outside Prime reserved areas.
```

`safe_content_frame` 必须逐字使用 `prime-guide-<size>.json` evidence 内的 `safe_content_frame`，它由当前
layout contract 和当前画布尺寸推导；不要用 `CONTENT_RECT`、手写边距或旧方形尺寸反推。`CONTENT_RECT` 必须落在
`safe_content_frame` 内，可更紧但不能越界；`HEADLINE_BOX`、`TABLE_BOX`、`BENEFIT_BOX`、`CTA_BOX` 在 `CONTENT_RECT`
内按当前尺寸与信息密度分配。底部区域的验收口径是最终 Prime 图中文字、金额、表格和按钮清晰可读且不被实际 Prime
组件遮挡；不要把一般几何贴近、浅色边缘、背景纹理或视觉平衡当成阻断。横版按 `1200x628` 原生铺开，竖版按
`800x1000` 原生铺开，不得把方形图缩小居中后补边。

如果 prompt 缺少上面的 Prime-safe 护栏，或者护栏未覆盖当前尺寸的 layout contract，必须写 `action_required`，不得调用 `multica image edit`。调用验证脚本时必须带上最终 prompt 和当前尺寸 layout：

```text
python <当前 Skill 目录>/references/validate_copy_snapshot.py \
  --materials-json <order.json> --candidate-id <candidate-id> \
  --order-item-id <item-id> --variant-id <variant-id> \
  --prompt-file <prompt.txt> --prime-layout-file <layout-<size>.json> \
  --require-prime-guard --require-redesign-guard --require-concise-prompt --evidence <copy-validation.json>
```

失败时写 Variant `action_required`，不得重选文案或调用模型。

`multica image edit --output json` 以及 batch 每个 result 返回实际 provider `prompt` 与 `prompt_sha256`。
保存 CLI 原始 JSON；登记 Asset 时逐尺寸原样使用对应 prompt/hash，不得从 brief、文件或兄弟尺寸重建。

图像 CLI 常超过默认 Bash 工具等待窗口。所有 `multica image edit` 与 `multica image edit-batch`
调用都必须在 Bash 工具上显式设置至少 15 分钟的长超时（`timeout_ms >= 900000`），等待 CLI 返回完整 JSON 后再解析。不得因默认工具超时而假定服务端仍在运行、
不得在缺少 CLI 原始 JSON/provider request ID/prompt hash 时登记资产；若长超时后仍失败，写真实错误并让
当前 task 失败。

## 三尺寸生成

初次生产先用 `multica image edit` 在 `1088x1088` 模型画布生成方形，再规范化为 `1080x1080`。方形底图
输入必须包含参考图和 `prime-guide-1080x1080.jpg`；底图必须是新的 AdaKami 广告，不是原图换文案。若有人物，
只保留角色/位置关系，换成新的可信成人与场景细节。可用后，立即以方形底图为第一输入、对应尺寸
`prime-guide` 为第二输入，用一次 `multica image edit-batch` 并发生成横竖原生重排：

```json
{
  "max_concurrency": 2,
  "jobs": [
    {"id":"landscape","inputs":[{"path":"square.png"},{"path":"prime-guide-1200x628.jpg"}],"prompt_file":"landscape.txt","model":"gpt-image-2","size":"1200x624","max_attempts":3,"output_file":"landscape-model.png"},
    {"id":"portrait","inputs":[{"path":"square.png"},{"path":"prime-guide-800x1000.jpg"}],"prompt_file":"portrait.txt","model":"gpt-image-2","size":"800x1008","max_attempts":3,"output_file":"portrait-model.png"}
  ]
}
```

相对路径以 manifest 目录为准。规范化横版到 `1200x628`、竖版到 `800x1000`。横竖版不等待 Prime、QC 或兄弟
Variant。三张共享批准文案、业务语义、信息层级、色系和 `asset_family_id`；必须按尺寸原生 edge-to-edge 重排，
背景、色块、阴影和装饰必须撑满完整画布，关键可读内容避开 `safe_content_frame` 与 hard region；
不得嵌入旧画布、缩小居中、加边、模糊边栏、裁切、拉伸或重新发明业务内容。

`max_attempts=3` 只重试传输层 408/429/5xx/网络错误，不是创意返工。每个尺寸最多一轮有证据的定向返工；
若返工清空/破坏批准业务内容，丢弃该坏图，允许一次更强 Prime 护栏的完整重生；仍失败才写 `action_required`。
进程仍运行时不重复提交。恢复只补当前 revision 的 missing sizes，复用输入指纹一致的成功输出。

每个模型输出确定性规范化并保存 evidence：

```text
python <当前 Skill 目录>/references/normalize_image.py \
  --input <model.png> --output <delivery.png> --width <w> --height <h> \
  --prime-layout-file <layout-<size>.json> --prime-safe-audit \
  --evidence <normalize-evidence.json>
```

优先有界 cover-resize；连续 provider 比例失败后才允许脚本记录的 full-content edge-fade。随后必须执行
`prime-safe-audit`：记录由当前 `prime_layout_contract` 动态推导的 `safe_content_frame`，但不得缩放、
平移或重绘整张模型图以适配该框。若模型输出把关键文字、CTA、金额、表格或主体卡片放入 Prime 固定区，
必须定向返工或完整重生；不得用本地脚本把整图缩小居中、添加边框、淡化补边或改写业务内容。
以规范化后的满版 delivery 图作为登记和 Prime 输入。
hard region 状态记录为 `prime_clearance_status=pending_fixed_prime_compose`。若规范化后的固定 Prime 区域仍残留
竞品品牌、模型生成品牌/QR、可读文字、按钮或关键业务内容，才定向返工或写 `action_required`；Prime Compose 只能
在同一固定坐标内净化背景，不能移动组件。

## Asset 与下一跳
上传 delivery 图、CLI 原始结果和规范化证据，再登记 completed `stage=generated` Asset：

```text
multica attachment upload <file> --output json
multica creative order asset-put <order-id> --input-file <asset.json> --output json
```

Asset 必须包含 variant、family、size、revision、attachment、generated lineage；metadata 保存 CLI 原样 prompt/
model，evidence 保存 request ID、attempts、prompt SHA-256、provider slot、copy/normalize evidence 和 pending Prime
状态。`asset.json` 必须是顶层 Asset 对象，尺寸字段写 `size_key`；不得包成 `{"asset": ...}`，不得写成
`size`。登记前检查：`metadata.prompt` 等于该尺寸 CLI 原始实际 prompt，`metadata.model=gpt-image-2`；
`evidence.request_id` 字段名精确，`evidence.attempts` 为正整数，`evidence.prompt_sha256=sha256(metadata.prompt)`；
provider hash 只能交叉核对，不能覆盖不一致值。方形响应的 family ID 原样传给横竖版。
`variant-put` 如更新状态，必须保留 item/key/brief 完整 upsert。

全部 expected sizes 齐备后，查询 `creative_order_variant_prime` source。只跳过 active task 或三个 expected size
的 completed primed assets 都存在的 item；不得因旧 Prime task succeeded 但包不完整而跳过。Prime fanout 的
`trigger_evidence_kind` 固定为 `creative_order_variant_prime`，`trigger_evidence_ref_id` 固定为当前 Variant ID。
item_key 固定 `<variant-id>:r<revision>`；context 固定 `type=creative_domain_task`、`workflow=creative_prime`，
并原样携带 issue/leader/order/item/variant IDs、revision、expected sizes、reviewer ID 和 generated asset IDs。

```text
multica task by-source list --agent <prime-agent-id> \
  --kind creative_order_variant_prime --ref <variant-id> --output json
multica task fanout --agent <prime-agent-id> --input-file <manifest.json> --output json
```

`manifest.json` 结构必须是：

```json
{
  "trigger_evidence_kind": "creative_order_variant_prime",
  "trigger_evidence_ref_id": "<variant-id>",
  "items": [{
    "item_key": "<variant-id>:r<revision>",
    "context": {
      "type": "creative_domain_task",
      "workflow": "creative_prime",
      "creative_order_id": "<order-id>",
      "creative_order_item_id": "<item-id>",
      "variant_id": "<variant-id>",
      "revision": 1,
      "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
      "issue_id": "<issue-id>",
      "leader_agent_id": "<leader-agent-id>",
      "reviewer_agent_id": "<reviewer-agent-id>",
      "generated_asset_ids": ["<asset-id>"]
    }
  }]
}
```

提交后立即结束。生成、上传、登记或 fanout 失败时保留已成功尺寸，写真实 error code/message 并让 task
失败；不得创建或修改 Issue。
