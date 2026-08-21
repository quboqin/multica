---
name: multica-ad-creative-production
description: "当 creative_production task 指定一个 Creative Order Variant，需要生成同内容族的方形母版、横版和竖版底图并登记完整模型证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告底图生产

只处理 task context 指定的 Order、Order Item、Variant、revision 和 `expected_sizes`；direct edit 不进入本 Skill。
先执行 `multica creative order get <order-id> --output json`，按 `creative_order_item_id` 与 `variant_id` 精确定位当前
订单项和变体。当前订单项的 `copy_snapshot`、Variant brief、冻结 market snapshot、候选素材和 Prime context 是唯一输入。
不得读取最新文案库、历史工作目录、同 candidate 的其他订单项，也不得重新分析竞品素材。

## 冻结输入

- `copy_snapshot` 是文案、金融事实、还款计划和用户视觉方向的唯一真值。顶层非空文案字段（`headline`、`subheadline`、`benefit`、`supporting`、`cta`、`legal_text`）逐字优先于
  `pre_adaptation.text_replacements`；后者只提供区块映射和版式，不得覆盖顶层非空字段。`pre_adaptation` 不提供最终模型提示词。
- `item.direction` 是由 `copy_snapshot.visual_direction` 派生的可追踪摘要，仅用于合同校验；不要把它当成可直接发送给模型的长提示词。
- 竞品图的全部非空业务结构默认继承：标题区、金额区、期限卡、表格行列、辅助信息区和阅读顺序都必须保留。继承的是结构和我方冻结文案，不是竞品品牌、Logo、二维码、官方模板文字或原金融事实。
- 用户把一个文案槽位清空时才删除对应文字；一行的必需槽位全部为空才删除该行；整个模块没有剩余可见内容才删除模块。不能为了适配横版主动删掉表格、卡片、底部图标或金融事实。
- Prime context 必须来自当前尺寸的官方 Prime 组件预览或合成上下文，包含真实色彩、材质、光照和组件节奏。透明中性轮廓只能作为结构审计证据，不能作为模型的唯一视觉输入。

参考图的权威入口是任务上下文中的 `candidate_id`。订单响应没有展开 `reference_assets` 时，使用
`multica creative material download <issue-id> <candidate-id> --output-file <source-reference.png> --output json`
受控下载当前候选；下载失败才算缺少参考图，不能把 `reference_assets` 为空本身当作失败。不得使用同 candidate 的其他订单、历史工作目录或摘要替代该下载。

Prime context 必须以当前任务的 `revision` 登记在当前 Variant 下，并且每个目标尺寸各有一份；旧 revision 的同名附件不能代替当前 revision。
如果受控候选下载失败、当前尺寸的 Prime context 无法生成、冻结文案缺失或 Variant execution 缺失，才停止并写 `action_required`，不要凭摘要补齐。

从订单 `input_snapshot.market_pack.files` 下载与当前尺寸匹配的官方模板，再生成低透明度的实际视觉上下文（只保留 Prime 保护区的组件内容）：

```text
python <当前 Skill 目录>/references/render_prime_guide.py \
  --layout-file <layout-<size>.json> --template-image <official-prime-template.png> \
  --width <model-width> --height <model-height> --output <prime-context-<size>.png> \
  --evidence <prime-context-<size>.json>
```

`prime-context` 的证据必须是 `render_style=official_prime_visual_context`；如果只能生成
`transparent_neutral_outlines`，不能把它作为唯一模型输入，应停止当前尺寸并写 `action_required`。

## 最终提示词由出图智能体负责

出图智能体根据冻结输入，为每个尺寸和每次实际调用独立写出 `prompt-<size>.txt`。前端不提供提示词编辑器，预适配也不生成
`production_prompt`；模型提示词的质量、长度和尺寸适配由本 Skill 负责。

提示词应保持 1800-2800 个字符，硬上限 3200。只保留能改变画面的信息，禁止粘贴审计日志、JSON、哈希、QC 结论、坐标、矩形框、
像素值、重复的金融事实或同一句文案的多种写法。提示词必须按下面的固定模板写，先给构图闸门，再声明两张输入图的角色：

```text
COMPOSITION GATE
INPUTS
TASK
VISUAL INHERITANCE
APPROVED COPY AND TABLE
VARIANT DIRECTION
PRIME INTEGRATION
FINAL CHECK
```

模板中的花括号占位符必须由出图智能体替换，不能原样发送给模型：

```text
COMPOSITION GATE
Create an unbranded base layer for deterministic official Prime composition, not a standalone branded ad.
Keep every non-empty approved_copy field and every non-empty source structure exactly once. Do not delete a title, benefit,
product label, amount, tenor option, table row, icon, or supporting module to make the layout fit.
Keep all business content in the middle content area between the protected Prime bands. The protected top and bottom bands are
reserved for the later official overlay and must stay free of business copy, tables, buttons, icons, decorative marks, and shadows.
If the middle business area feels crowded, compress vertically along the Y axis: reduce vertical whitespace, module gaps, and line
spacing before reducing type. Preserve every module and keep it out of the protected bands. Never crop, delete, or push content into them.

INPUTS
{input_one_role}. Input 2 is the current-size official Prime visual context. Do not swap input roles.
Use Input 2 to understand official color, material, lighting, edge rhythm, and the quiet background needed below the overlay.
Do not copy any Prime logo, QR, store badge, legal text, template wording, or component geometry from Input 2.

TASK
{task_for_this_size}

VISUAL INHERITANCE
{shared_visual_identity_and_native_reflow_direction}

APPROVED COPY AND TABLE
Render every non-empty approved_copy field and frozen repayment row exactly once: {approved_copy_and_table}

VARIANT DIRECTION
{size_specific_layout_direction}. For landscape, keep the headline and benefit below the protected top band and the complete table
above the protected bottom band; use native horizontal reflow, not a scaled or cropped square.

PRIME INTEGRATION
Input 2 is visual context only. Continue a calm, low-detail background through the protected bands so the official overlay remains
readable. Never draw Prime or any placeholder for it.

FINAL CHECK
All required copy, amount, tenor options, table rows, and source structures are present and legible. No business content enters the
protected bands. No brand element, QR, store badge, legal text, invented CTA, crop, or duplicate copy is present.
```

`{input_one_role}` 必须按调用阶段明确写成以下之一：方形母版使用“Input 1 is the downloaded candidate reference; use it only for business structure,
reading order, and visual anchors”；横版或竖版使用“Input 1 is the approved square base from this Variant”；视觉返工使用“Input 1 is the failed
unbranded base for this size”。三种场景的 Input 2 都必须是当前尺寸、当前 revision 的官方 Prime context。`APPROVED COPY AND TABLE` 必须逐字列出
Variant brief `approved_copy` 中每一个非空字段（包括 `product_category`、supporting、borrowing prompt、term prompt、所有按钮、summary labels
和表格数值）以及冻结还款行，每项只出现一次；不能因为顶层 snapshot 没有单独字段就漏掉 brief 中仍然非空且属于原图结构的文案。`VARIANT DIRECTION`
只描述视觉主线、主体关系、材质、色彩和当前尺寸的重排意图。不要在提示词中写 `x/y`、`safe_content_frame`、`hard region` 或其他坐标语法；空间关系用
“上方保护带下方、主体中部、下方官方组件上方的连续背景”等自然语言表达。

## 视觉继承和三尺寸顺序

三尺寸共享同一内容族、业务事实、色彩系统和 `asset_family_id`，但不是同一张图缩放：

1. **方形母版（1080x1080）**：Input 1 为竞品参考结构，Input 2 为当前方形 Prime context。创建新的无品牌广告底图，继承信息机制、阅读顺序和可识别视觉锚点，重新设计背景、主体和装饰。
2. **横版重排（1200x628）**：Input 1 为已批准的方形母版，Input 2 为当前横版 Prime context。横版优先处理，原生铺满画布；保留主体、标题、卖点、数值表和图标的内容关系，重新分配宽度和间距，不把方形图缩小居中、不裁切、不加边。
3. **竖版重排（800x1000）**：Input 1 为已批准的方形母版，Input 2 为当前竖版 Prime context。按移动端阅读顺序原生重排，保持与方形母版相同的视觉身份和冻结文案。

横版拥挤时必须执行垂直方向的 Y 轴压缩：先减少装饰和上下留白，再压缩模块间距、行距和标题/金额/期限/表格之间的垂直节奏，最后才小幅降低字号；
不得删除冻结文案、金融事实、底部图标或表格列。横版标题和利益点整体必须位于官方顶部 Logo/条款组件下方；金额、期限按钮和完整四行表格必须位于官方底部组件上方，
中间内容区要留出连续背景缓冲，不能让任何正文进入上下组件带。底部图标和数值表必须完整出现在官方 Prime 组件上方，不能被组件遮挡。

## Prime 参与构图

Prime context 是当前尺寸的真实视觉输入，不是黑白遮罩或可复制的模板。它用于让模型理解官方组件的真实色系、材质、光照方向和边缘节奏，并让整张底图在组件下方保持连续背景。模型输出仍然是无品牌底图：
不得绘制 Prime Logo、QR、官方模板文字、商店徽章、OJK/AFPI/Pindai、占位卡片、白块、横条或灰色引导线。官方 Prime 由后续 deterministic compose
原样叠加；模型只负责让业务内容和背景为组件留出自然、可读的空间。

## 调用和证据

每次模型调用都保存实际发送的 prompt、`prompt_sha256`、`request_id`、attempts、实际画布尺寸和输入资产指纹。
调用 `multica image edit` 或 `image edit-batch` 时使用显式长超时；传输层 408/429/5xx/网络失败最多重试两次。必须把 CLI 返回的完整 JSON 原样保存，不能手工只保留 request ID、hash 或 `generated_asset` 摘要；后续 `asset-put` 使用同一份原始 JSON。
模型调用画布必须遵守 GPT Image 2 的 16px 边长约束，交付尺寸与模型画布分开记录：`1080x1080` 使用 `1088x1088`，`1200x628` 使用 `1200x624`，`800x1000` 使用 `800x992`。CLI 接受 canonical 交付尺寸并自动映射到上述 provider canvas；完整模型 JSON 必须同时保留请求尺寸和实际 provider canvas。模型输出必须经过规范化到 `1080x1080`、`1200x628`、`800x1000`。比例在允许范围内直接缩放；如果模型连续返回明显错误比例，CLI 最多重试两次重新取图；两次后仍不符合目标比例时，该尺寸直接失败并交给平台续跑，不要保留最后一张图，也不得用 `aspect-compress`、`contain-edge-extend` 或其他内容挤压/拉伸兜底把错误比例强行压成交付尺寸。不要把 `1080x1080`、`1200x628` 或 `800x1000` 直接作为 provider 的 `--size` 值传入旧版 CLI。

```text
python <当前 Skill 目录>/references/normalize_image.py \
  --input <model.png> --output <normalized.png> --width <w> --height <h> \
  --model-size <requested-model-size> --evidence <normalization.json>
```

三尺寸底图完成后登记 generated assets，再调用绑定的贴片 Skill，由后端合成官方 Prime 并进入 QC。最终 QC 只以真实 Prime 合成图为准：实际遮挡、文字不可读、
底部图标被盖住才算失败；靠近边界、背景纹理或预估覆盖不是失败。

## 过程证据登记

过程图片不是工作目录里的私有日志，而是订单可追溯证据。每个尺寸的 `Prime context`、模型原图和规范化底图一生成并确认文件存在，
就必须通过仓库内的登记助手上传并写回当前订单、Variant、revision 和 task：

```text
python <当前 Skill 目录>/references/register_process_assets.py \
  --order-id <order-id> --variant-id <variant-id> --revision <revision> --task-id <task-id> \
  --cli multica --profile direct-image2 \
  --image 1080x1080 "Prime context" <prime-context-1080x1080.png> \
  --image 1080x1080 "模型原图" <model-1080x1080.png> \
  --image 1080x1080 "规范化底图" <normalized-1080x1080.png>
```

按文件实际生成情况重复调用，三个尺寸和三个过程阶段都要登记；不要等 task 结束才批量登记。命令必须返回 JSON，且 `registered` 数量与本次输入一致，
再用 `multica creative order get <order-id> --output json` 回读确认归属。底图与过程证据齐全后调用绑定的贴片 Skill；后端会在确定性合成时登记 `Prime 合成成图`，出图智能体不要伪造该阶段。
任何登记失败都要保留真实错误并让当前 task 失败，以便平台创建有界续跑；在当前 revision 的所有 expected size 都有 canonical generated asset、完整过程证据并且贴片 Skill 返回后端合成成功前，禁止调用
`multica task complete`。

文案校验必须与本 Skill 的当前提示词合同一致：当前合同使用自然语言描述 Prime 保护带，禁止把坐标、矩形框或审计重复写进模型提示词。
`--require-prime-guard` 应校验无品牌底图、Input 1/Input 2 角色、官方 Prime 视觉上下文、保护带避让和确定性叠加语义；旧版坐标合同只适用于仍明确使用坐标语法的历史提示词。
期限数值带 `bulan`、`hari` 或 `tahun` 时由期限 token 校验，不得再拆成未批准的裸金融数字。

每个 `prompt-contract-<size>.json` 除 `prompt_sha256` 外必须写入当前 Variant brief
`creative_contract.parent_direction_sha256`，并保留 `size_key`、`revision`、`variant_id`、`input_roles` 和
`active_content_groups`。该父方向哈希是 visual QC 校验三尺寸视觉继承的唯一证据；不能省略、伪造或从旧 revision 复制。

生产 task 只有在当前 revision 的每个 `expected_sizes` 都存在完整、可验证的 `generated/completed` canonical asset 后才允许调用
`multica task complete`。只生成方形或只生成部分尺寸时不能提前 complete，也不能把缺失尺寸写成成功；应在同一个 task 中继续补齐，或把真实错误交给
`multica task fail`。若 task 因模型、网络或 daemon 中断而先结束，平台会在服务端自动创建有上限的 fresh continuation，并保持 Variant 为 running，直到尺寸齐全或达到上限。

### generated asset 写回格式

每个尺寸都必须用同一 Variant、当前 revision 和已上传附件写回一个完整的 canonical JSON 对象，再调用
`multica creative order asset-put <order-id> --input-file <asset.json> --model-result-file <model.json> --prompt-contract-file <prompt-contract.json> --copy-validation-file <copy-validation.json> --normalization-evidence-file <normalization.json>`：

```json
{
  "variant_id": "<variant-id>",
  "size_key": "1200x628",
  "revision": 1,
  "stage": "generated",
  "status": "completed",
  "attachment_id": "<normalized-attachment-id>"
}
```

`model-result-file` 必须是该尺寸模型调用返回的完整 JSON，不能只摘录 prompt 或 request ID；四份证据文件必须逐尺寸对应。
不要使用 `kind`、`asset_type` 代替 `stage`，也不要省略 `status`、`revision` 或 `attachment_id`。平台会兼容旧别名，但新任务必须按上面的 canonical
格式写回，避免生成成功却没有进入订单资产链路。

## 有界返工

如果视觉 QC 指出 `actual_prime_obstruction` 或 `official_prime_text_unreadable`，当前 Variant 每个尺寸最多执行一次有证据的定向返工。
返工仍由出图智能体重新写当前尺寸的短提示词，输入为失败底图和同尺寸 Prime context；只描述实际遮挡和需要压缩/移动的内容，不改文案、金额、期限、表格、
视觉身份或 Prime 规则。一次返工后仍失败就写 `action_required`；不要把过程图转成候选或要求用户选择技术实现。

过程图片只用于排查，固定登记 `Prime context`、`模型原图`、`规范化底图` 和 `Prime 合成成图`。不得把过程图当作最终交付或采用图。
