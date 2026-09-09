---
name: multica-ad-creative-production
description: "当 creative_production task 指定候选或已晋级 Creative Order Variant，需要生成主视觉或补齐同 DesignDNA 三尺寸底图并登记完整模型证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告底图生产

只处理 task context 指定的 Order、Order Item、Variant、revision 和 `expected_sizes`；direct edit 不进入本 Skill。
运行中的 Agent 进程由 daemon 注入 `MULTICA_TASK_ID`，它是当前 production task 的唯一 task ID。任何过程证据的
`task_id` 只能使用这个环境变量；不得从 `issue_id`、`candidate_id`、`variant_id`、`item_key` 或任务描述猜测。开始上传
Prime context、模型原图或规范化底图前，先执行 `test -n "$MULTICA_TASK_ID"`；为空时如实报告运行时身份缺失，不能调用模型。
写回因 task ID 或并发状态冲突而失败时，先回读订单和当前 operation，用同一模型回执、附件和 `MULTICA_TASK_ID` 修复写回；
不得重新调用模型。
先执行 `multica creative order get <order-id> --output json`，按 `creative_order_item_id` 与 `variant_id` 精确定位当前
订单项和变体。当前订单项的 `copy_snapshot`、Variant brief 中的 CreativeIntent/DesignDNA/LayoutPlan、冻结 market snapshot、候选素材和 Prime context 是唯一输入。
回读后必须用以下校验器确认精确 ID、revision 和尺寸范围；只有该精确记录仍不匹配 task context 才能报告 stale task，
不能因为读到了同订单的另一个候选而停止：

```bash
python3 <当前 Skill 目录>/references/validate_task_scope.py \
  --order-file <order.json> \
  --order-item-id <creative_order_item_id> \
  --variant-id <variant_id> \
  --revision <revision> \
  --expected-size <size>
```

不得读取最新文案库、历史工作目录、同 candidate 的其他订单项，也不得重新分析竞品素材。

## 流程合同

订单套数来自 `input_snapshot.target_variant_count`（未保存时为 3），文案订单可为 1-10 套、候选最多到 C12。
套数不改变单个 task 的 Variant 和 expected_sizes 范围，不能只执行前三个变体，也不能把其他变体的尺寸纳入当前 task。

先确认 order `input_snapshot.pipeline_version=candidate_v1`，不得修改。字段缺失或值不符时停止并写真实错误，不得推断、回填或切换流程。
candidate 只做主尺寸，selected 复用已合格主尺寸并补缺失尺寸，reserve 不生产。Variant 缺少
`creative_intent`、`design_dna` 或当前尺寸 `layout_plan` 时写 `action_required`，不得从旧字段合成替代合同。

## 冻结输入

- `item.source_kind=copy_library`：没有竞品原图，不下载或补造 source-reference；以下竞品来源与结构继承规则仅适用于 material。
  只渲染冻结的已选顶层文字和 `repayment_plan_entries`，空文案槽和未选还款模块保持不存在；`visual_only=true` 时只制作原创视觉。
  首轮模型输入只有当前尺寸 Prime context，后续按身份锚点增加已选主视觉参考；按真实顺序命名输入，不保留虚假的 Input 1/source 指纹。
  `image-operation-put.input_snapshot.input_roles` 只记录实际输入，保留 Prime 指纹；其余 operation、回执、归一化与贴片规则相同。
  运行 `validate_copy_snapshot.py` 时用 `--order-item-id` 和 `--variant-id`，不传空的 `--candidate-id`。
  先按当前尺寸 LayoutPlan 的自然语言主次与内容组构图，文案多时优先减少装饰、调整主体占比与横竖版组织；不补坐标、百分比框、字号阈值，不代码排字，不删已选文案，不整体内缩。
- `copy_snapshot` 是文案、金融事实、还款计划和用户视觉方向的唯一真值。顶层非空文案字段（`headline`、`subheadline`、`benefit`、`supporting`、`cta`、`legal_text`）逐字优先于
  `pre_adaptation.text_replacements`；后者只提供区块映射和版式，不得覆盖顶层非空字段。`pre_adaptation` 不提供最终模型提示词。
- `item.direction` 是由 `copy_snapshot.visual_direction` 派生的可追踪摘要，仅用于合同校验；不要把它当成可直接发送给模型的长提示词。
- 竞品图的全部非空业务结构默认继承：标题区、金额区、期限卡、表格行列、辅助信息区和阅读顺序都必须保留。继承的是结构和我方冻结文案，不是竞品品牌、Logo、二维码、官方模板文字或原金融事实。
- 若 `brief.creative_contract.app_ui_replacement.selected` 不是 true，Source Analysis 或 `item.direction` 中的手机、手持手机、屏幕、App 页面描述只作为源图证据，不是必须保留结构；不得追加 Input 3，不得仿造竞品 UI 或臆造 AdaKami App 页面，可将该区域重构为普通产品利益点、人物场景或留白。
- 用户把一个文案槽位清空时才删除对应文字；一行的必需槽位全部为空才删除该行；整个模块没有剩余可见内容才删除模块。不能为了适配横版主动删掉表格、卡片、底部图标或金融事实。
- Prime context 必须来自当前尺寸的官方 Prime 组件预览或合成上下文，包含真实色彩、材质、光照和组件节奏。透明中性轮廓只能作为结构审计证据，不能作为模型的唯一视觉输入。
- `brief.creative_contract.app_ui_replacement` 是是否替换手机 App 屏幕的唯一合同。若 `required=true` 且
  `selected=true`，必须使用其中的 `attachment_id` 下载所选 AdaKami App UI 参考图，并把它作为 Image Edit
  的额外输入；若 required=true 但没有 selected/attachment_id，当前 Variant 必须写 `action_required`。

material 来源参考图的权威入口是任务上下文中的 `candidate_id`。订单响应没有展开 `reference_assets` 时，使用
订单的 `input_snapshot.attachment_snapshot.candidate_sources` 是候选原图的唯一运行时来源：取 `candidate_id` 等于当前
Order Item candidate 的 `attachment_id`，再执行
`mkdir -p <source-dir> && multica attachment download <attachment-id> -o <source-dir>`。该 ID 在建单时已验证归属、发布版本和
对象存储可读性；不得改用 `archived_url`、`preview_url`、`resource_url`、`original_url`、浏览器会话或历史工作目录。下载得到的唯一图片文件才是
`source-reference`；`reference_assets` 为空本身不是失败。

下载失败时把真实 stderr 连同 `attachment_id`、`size_key` 写入当前 `image-operation-put` 的 `error_message`，并使用唯一
`error_type`：登录态失效、401 或 403 为 `auth_expired`；超时、连接重置或临时存储不可用为 `storage_timeout`；不存在、无对象或无下载 URL
为 `attachment_not_found`；不支持的 CLI 参数或命令合同不匹配为 `cli_contract_mismatch`。不得把这些错误写成笼统的
`attachment_download_failed`，不得换 URL、猜附件或以旧本地文件继续出图。

Prime context 必须以当前任务的 `revision` 登记在当前 Variant 下，并且每个目标尺寸各有一份；旧 revision 的同名附件不能代替当前 revision。
如果受控候选下载失败、当前尺寸的 Prime context 无法生成、冻结文案缺失或 Variant execution 缺失，才停止并写 `action_required`，不要凭摘要补齐。

## 两阶段生产

先以订单回读的 `candidate_state`、`primary_size` 和 `expected_sizes` 判断阶段，不从 Variant key 猜：

- `candidate_state=candidate` / `production_stage=candidate_primary`：只生成 `primary_size=1080x1080`，且 `expected_sizes` 必须恰好为
  `["1080x1080"]`。这是所有候选统一的方形比较图，不得因横版、竖版或手机叙事方向改用其他首轮比例。
  写回 generated 后调用绑定贴片 Skill 生成该主尺寸 Prime 图，供独立 `creative_candidate_selection` 比较；不补另外两尺寸，
  不自行选择候选，也不触发完整交付 QC。
- `candidate_state=selected`：回读平台原子晋级后的完整 `expected_sizes`，复用候选阶段已经完成的主尺寸 canonical asset、
  模型回执、归一化和 Prime 证据，只生成缺失尺寸；不得为了统一流程重做主尺寸。
- `candidate_state=reserve`：保留已有主视觉与全部血缘，立即结束，不生成、不贴片、不进入交付汇总。

若 task context 与订单当前候选状态或尺寸范围不一致，以订单当前结构化状态为准并报告 stale task；不得把候选 task 擅自扩成三尺寸，
也不得把 reserve 改回 selected。候选与晋级生产都使用同一套金融文案、App UI、归一化、证据登记和 Prime 规则。

## App UI 参考输入

若 brief 中 `creative_contract.app_ui_replacement.selected=true`，在写 prompt 前把
`app_ui_replacement.attachment_id` 下载到当前任务的受控临时目录：

```text
app_ui_dir="$(mktemp -d)"
trap 'rm -rf "$app_ui_dir"' EXIT
multica attachment download <app-ui-reference-attachment-id> --output-dir "$app_ui_dir" --output json
```

下载后必须确认临时目录里只有一张可读图片，并将其作为同一次 `multica image edit` 调用的额外 `--input`。不要使用历史工作目录、
同市场资源包里未选中的其他 UI 图、网页截图或竞品图裁剪替代。每个尺寸的 Input 1 都是候选参考结构，
Input 2 是当前尺寸 Prime context，Input 3 是选中的 AdaKami App UI reference；若 `design_dna.subject_system` 声明人物、产品或
核心对象身份锚点，selected 主视觉必须作为 Input 4，并且后续尺寸只能用它保持同一身份。没有身份锚点时，晋级主视觉可作为 Input 4 的
DesignDNA 一致性参考。如果该尺寸明确移除了手机屏幕，prompt-contract 必须写明
`app_ui_reference_attachment_id` 与 `used=false` 的原因。

Input 3 只服务于手机屏幕内容替换：保留原画面中的手机机身、手、透视、遮挡、反光、光照和场景；只把屏幕内的竞品 App
页面替换为 AdaKami 自有 UI。必须移除竞品 logo、品牌色、按钮文案、QR、商店元素和专属页面文案。不得把 AdaKami UI
画到屏幕外，不得把整张参考 UI 拉伸硬贴；若参考图透视不匹配，应生成同品牌风格的屏幕内容，而不是扭曲 pasted screenshot。

若 brief 没有 `app_ui_replacement.selected=true`，本节完全不生效：不要下载市场包里的任意 App UI 文件，不要给 Image Edit
追加第三输入，也不要因为源图或 visual direction 提到手机屏幕就要求保留手机界面。

若冻结批准文案的 `benefit` 非空，或 `copy_snapshot.pre_adaptation.additional_copy` 包含 `role=benefit` 的 ready 文案，必须把该利益点
作为成图中清晰、可读的可见文字逐字渲染。图标、步骤卡、手机外形或人物动作只能强化该利益点，不能替代文字；未选择 App UI 时尤其不得
把利益点藏进手机屏幕或以虚构 UI 代替。只有冻结利益点为空时才允许只使用图形表达，不得补写业务 claim。

先读取当前 Variant brief 的 `prime_composition.mode`。缺失时按 `deterministic` 处理；不得按市场名称、历史任务或模板文件名猜测。
`deterministic` 从订单 `input_snapshot.market_pack.files` 下载与当前尺寸匹配的官方模板，再生成低透明度的实际视觉上下文（只保留 Prime 保护区的组件内容）：

```text
python3 <当前 Skill 目录>/references/render_prime_guide.py \
  --layout-file <layout-contract.json> --size-key <canonical-size> --template-image <official-prime-template.png> \
  --width <model-width> --height <model-height> --output <prime-context-<size>.png> \
  --evidence <prime-context-<size>.json>
```

`prime-context` 的证据必须是 `render_style=official_prime_visual_context`；如果只能生成
`transparent_neutral_outlines`，不能把它作为唯一模型输入，应停止当前尺寸并写 `action_required`。

`model_integrated` 只能使用 brief 中 `prime_composition.template_sources[<size>]` 指向的完整官方模板作为 Input 2，不生成低透明度 guide，
不切换 family，也不从未选中的市场文件中挑模板。该模式只会由平台为已验证、无二维码的冻结模板族写入；若当前附件、family 或尺寸不一致，停止并写
`action_required`。模型需要把 Input 2 的可见官方文字、Logo、色彩与大致位置融入当前尺寸成图，业务内容仍须避开这些区域；不得补画二维码、添加其他
官方组件或要求后端二次贴片。

## 模型提示词与三尺寸一致性

每次模型调用前必须完整读取 [GPT Image Model Prompt Contract](references/model-prompt-contract.md)，并从当前 brief 的
`creative_intent`、`design_dna`、当前尺寸 `layout_plan`、批准文案和输入角色编译 `prompt-<size>.txt`。该参考文件是标准生产
提示词结构的唯一真值；不得从 Agent instructions、task context 或旧 prompt 拼接另一套模板。

提示词只包含会改变像素的视觉指令。order/task/revision、文件路径、哈希、request ID、上传、登记、重试、超时、状态、JSON、
CLI 和附件血缘只属于模型调用外的工作流，不能发送给图像模型。GPT Image 继续负责在底图中渲染完整批准文案、金额、表格和 CTA；
平台不代码排字，不预留稍后排字的空白框。`deterministic` 的后续唯一确定性 overlay 是官方 Prime；`model_integrated` 则按冻结的 QR-free
完整模板直接生成最终图，后端只登记证据，不会二次叠加。

三尺寸共享 DesignDNA、批准文案、业务结构和 `asset_family_id`，但每个尺寸从同一候选 source reference、当前尺寸 Prime context
和自身 LayoutPlan 原生生成，不把方图 raster 当作不可替代输入。人物、产品或核心对象只要在 `subject_system` 中作为身份锚点出现，
每个后续尺寸必须使用 selected 主视觉作为 Input 4，保持同一身份，不得替换为另一人物、产品或对象；Input 4 只锁身份、材质、色彩和
视觉母题，绝不捐赠布局、裁切或 Prime 像素。`family_consistent` 只允许没有身份锚点的抽象或无主体方向缺少 Input 4；身份锚点是
真实依赖，且不能伪装成方图布局依赖。

横版按 LayoutPlan 原生横向重排；拥挤时依次减少装饰和上下留白、模块间距、行距，最后才小幅降低字号，不删冻结文案、金融事实、
底部图标或表格列。竖版固定为 4:5，不得成为 story、手机截图、长海报、滚动页、9:16 或 9:19。三个尺寸可以并行；任一尺寸迟到或失败
不使其他尺寸重生。完整交付 QC 会把三个 Prime 成图并排做联合一致性验收，发现单一离群尺寸时只返工该尺寸。

## Prime 参与构图

Prime context 是当前尺寸的真实视觉输入，不是黑白遮罩。`deterministic` 中它不可复制，模型输出仍是无品牌底图：不得绘制 Prime Logo、QR、
官方模板文字、商店徽章、OJK/AFPI/Pindai、占位卡片、白块、横条或灰色引导线，后端再原样叠加。`model_integrated` 中 Input 2 是唯一冻结且无二维码的
完整模板，模型可把它融入成图，但必须保留其可见官方文字、Logo、色彩和大致位置；不补画 QR、不增加官方组件、不改写官方文字，也不要求后端二次贴片。
两种模式下模型都负责让业务内容和背景为官方组件留出自然、可读的空间。

### Prime 承托质量

确定性 composer 选择已批准模板中承托评分最高的一个。若 `template_selection.visual_adequacy.status=qc_risk`，仍应登记 Prime 成图并进入最终 visual QC；
`inadequacy_codes` 是需要放大核验的质量证据，不是重出图、换模板或阻断交付的理由。最终 QC 只以真实 Prime 成图中官方文字、条款与业务内容的实际可读性和遮挡为准。

只有 composer 没有任何可评估模板、模板/证据合同错误或进程失败时才失败。失败完整 report 必须从
`brand_composition_error.compose_result_attachment_id` 下载，不能按错误文字猜。遗留订单若仍返回
`prime_no_adequate_template_for_size`，仅在后端创建带 `qc_visual_rework.reflow_strategy=prime_background_support` 的下一 staging revision 后，
才可做一次仅限失败尺寸的承托修复；当前新订单不得因为质量证据单独触发该重绘路径。

## 调用和证据

每次模型调用都保存实际发送的 prompt、`prompt_sha256`、`request_id`、attempts、实际画布尺寸和输入资产指纹。
Codex 的短 exec bridge 会在约 30 秒无 stdout 时提前返回，即使 Image Edit 子进程仍在请求 provider。因此单张
`multica image edit` 一律使用本 Skill 的 `run_image_edit_job.py`：它把唯一模型命令放到独立会话，持久化状态和原始 stdout/stderr，前台只做 20 秒轮询。worker 的真实进程超时固定至少 25 分钟；`wait` 返回 `running` 时立即再次执行同一个 `wait`，直到 state 是 `completed`。不得因任意一次短轮询、空 stdout 或 `waiting` 把 operation 标记为 `unknown`，不得启动第二次模型调用。传输层 408/429/5xx/网络失败最多重试两次。必须把 `result-file` 中 CLI 的完整原子 JSON 原样保存，不能手工只保留 request ID、hash 或 `generated_asset` 摘要；后续 `asset-put` 使用同一份原始 JSON。
写 canonical generated asset 时，`metadata.prompt`、`model_result.prompt` 和 `prompt_sha256` 必须从该原子 JSON 的同一个
JSON string 逐字复制。不得用 `jq -r`、命令替换、shell 变量、`echo` 或展示用的 `prompt-<size>.txt` 重建 prompt：它们会改变末尾换行或
其他空白字节，导致模型已成功生成却被资产谱系校验拒绝。应生成 JSON 对象后把原字段原样嵌入；只有 `prompt_sha256` 可作为单独标量读取。

模型调用前必须先用 `multica creative order image-operation-put <order-id> --input-file <operation.json> --output json`
登记当前 Variant、revision、size、operation kind、稳定幂等键、attempt、model 和输入资产指纹。首次登记不填写 `prompt_sha256`：只可在 Image Edit 的原子回执返回后，用其中的实际 `prompt_sha256` 完成同一 operation，避免提示词文件末尾换行等本地表示差异破坏谱系。标准生产使用
`operation_kind=generation`，视觉返工和画布修复分别使用 `visual_rework`、`canvas_repair`；幂等键固定为
`<variant-id>:r<revision>:<size>:<operation-kind>:v1`，同一逻辑调用重跑时不得换键。首次登记使用 `status=running`：只有返回
`disposition=invoke` 才能调用模型；`reconcile` 表示已有 running/unknown 调用，必须等待或对账；`reuse` 表示已有 completed 回图，必须直接复用。
保存响应中的 `id` 作为当前尺寸的 `operation_id`；后续 generated asset 必须带同一个 `operation_id`，不能省略、猜测或改用 task ID。
把 `image-operation-put` 的完整响应保存为文件，并从响应的 `id` 和对应 `attempts[].attempt` 读取本次坐标；不得只从本地
`operation.json` 猜 attempt。所有真正调用 provider 的单尺寸命令必须同时传入这两个返回值：

首次 `operation.json` 的顶层只能使用服务端字段名，输入指纹和角色必须嵌套在 `input_snapshot`，不能写成顶层
`input_asset_fingerprints`、`input_fingerprints`、`input_roles`、`target_size`，也不能把 `operation_kind` 缩写成 `kind`：

```json
{
  "variant_id": "<variant-id>",
  "size_key": "<canonical-size>",
  "revision": 1,
  "operation_kind": "generation",
  "idempotency_key": "<variant-id>:r1:<canonical-size>:generation:v1",
  "status": "running",
  "model": "gpt-image-2",
  "input_snapshot": {
    "input_asset_fingerprints": {
      "source_reference_sha256": "<sha256>",
      "prime_context_sha256": "<sha256>"
    },
    "input_roles": ["source_reference", "prime_visual_context"],
    "target_size": "<canonical-size>"
  },
  "attempt": 1
}
```

每次登记（含续跑）都先回读订单，然后使用以下助手生成最终请求。助手会按当前 revision、尺寸与 operation_kind 查找既有操作：首次使用 attempt=1；已明确 failed 的操作使用最大历史 attempt+1，并逐字复用原幂等键、model、prompt hash 和 input_snapshot。重试序号不得改回 1；服务端不会替客户端自动增加序号。completed 必须复用回执，running/unknown 必须等待或对账，不能再次调用模型。

```bash
multica creative order get <order-id> --output json > current-order.json
python3 <当前 Skill 目录>/references/prepare_image_operation.py \
  --order-json current-order.json --input-file <operation-draft.json> \
  --output-file <operation.json>
```

首次登记的 input_snapshot 还应保存 `input_asset_attachments`：每个输入指纹字段对应其已上传的 attachment_id。Prime context 使用过程登记返回的附件，身份参考使用已选方图附件。续跑先下载这些原附件并核验指纹，不能只保存哈希后靠重新渲染猜原文件；既有操作缺少该映射时只能查找指纹完全一致的原过程附件，不能改写冻结快照。

随后执行本地结构校验；失败时修正 JSON 后再登记，不能靠更换幂等键或猜别名重试：

```bash
python3 <当前 Skill 目录>/references/validate_image_operation.py \
  --input-file <operation.json>
```

`image-operation-put` 返回 409 时读取明确的服务端原因并回读订单，使用同一助手重新准备一次；不能把状态冲突解释成“最多 4/5 次”或“额度耗尽”。任务层重试预算、provider 单命令的 HTTP 重试次数与 operation 的历史序号是不同的计数。收到 400/401/403 的模型、账户或权限错误时停止重复请求并保留准确原因；503/429/网络故障只做有界退避恢复。

```bash
operation_id="$(jq -er '.id' <image-operation-response.json>)"
operation_attempt="$(jq -er '.attempts | last | .attempt' <image-operation-response.json>)"
runner="<当前 Skill 目录>/references/run_image_edit_job.py"
state_file="<workdir>/image-edit-job-<size>.json"
python3 "$runner" start --state "$state_file" \
  --stdout-file "<workdir>/image-edit-<size>.stdout" \
  --stderr-file "<workdir>/image-edit-<size>.stderr" --timeout-seconds 1500 -- \
  multica image edit --input <input.png> --prompt-file <model-prompt.txt> \
  --size <canonical-size> --quality high --output-file <model-output.png> \
  --result-file <workdir>/image-edit-result-<size>.json \
  --operation-id "$operation_id" --operation-attempt "$operation_attempt" --output json
python3 "$runner" wait --state "$state_file" --max-wait-seconds 20 --heartbeat-seconds 5
```

CLI 会把 daemon 注入的 task ID、operation 坐标、provider request ID、prompt hash、输出 hash 和附件路径一起写入原子
`result-file`。缺少 `--operation-id` 或 `--operation-attempt` 的调用没有可信迟到回执能力，禁止执行。

### 单尺寸 in-flight 与迟到回图恢复

每个尺寸在当前 revision 只允许有一个未结算的 Image Edit 调用。调用单张 `multica image edit` 时，给它固定的
`--result-file <workdir>/image-edit-result-<size>.json`；CLI 只会在模型图片已经成功写入 `--output-file` 后原子发布这份完整 JSON 回执。

- 在发起调用前先检查该 `result-file`：若其中的 `generated_asset.completed=true`、`path` 存在且尺寸/`prompt_sha256` 对应当前调用，必须直接复用该回图做归一化、过程登记或写回，**不得再次调用模型**。
- 通过 `start` 后只能对同一个 `state_file` 重复执行 `wait`；每次 `running` 都表示独立 worker 仍活着，不是失败。只有 `wait` 返回 `completed` 才能检查 `exit_code`、`result-file` 和 `output-file`。不得在此之前运行 `find`、`ls`、第二个 `multica image edit`，或写“provider 未返回”。这个持久 worker 替代不可靠的同一个 Bash/exec 阻塞等待，且始终只承载一次模型调用。
- 若 task 被续跑，先执行 `python3 "$runner" inspect --state "$state_file"`，再继续 `wait`。state 为 `running` 时绝不能更新 operation；state 为 `completed` 且有有效 `result-file` 时，它就是成功的原始模型结果，先补过程图、规范化、上传、`asset-put` 或贴片，不能重出图。
- task context 含 `late_receipt_recovery` 时，provider 成功回图已由原 runtime 的 daemon 可信归档。按 `late_receipt_recoveries` 逐项处理所有迟到尺寸（单项时也保留 `late_receipt_recovery` 作为首项指针）：从订单回读每个 `operation_id`/attempt 的完整 `result_receipt`，下载对应 `output_attachment_id`，只补规范化、过程登记、`asset-put` 和后续 Prime；**禁止调用 `multica image edit` 或创建新 operation/attempt**。
- runtime 中断、空返回或 provider 状态不明时，把同一 operation/attempt 更新为 `unknown` 并保存已有 request ID、退出信息和耗时；unknown 只允许 reconcile，不能重发。明确完成后上传模型原图，再把同一 operation/attempt 更新为 `completed`，保存完整 `result_receipt`、request ID、provider status、耗时和 `output_attachment_id`。
- unknown 对账时先检查原子 `result-file`、已有模型输出、过程附件和 provider 回执。找到成功结果就完成原 attempt；只有已经确认这些位置都没有有效回执时，才能把同一 attempt 更新为 `failed`，并同时提交 `error_type=provider_receipt_not_found`、`reconcile_confirmed=true`。之后才允许用递增 attempt 请求下一次 `disposition=invoke`；不得通过更换幂等键绕过 unknown。
- 只有同一个调用已经得到非零退出码，且没有有效 `result-file`、没有有效模型输出、并且错误确属 408/429/5xx/网络传输，才把该 attempt 更新为带真实 `error_type` 的 `failed`，然后用递增 attempt 请求下一次 `disposition=invoke`。连续三次“没有返回”必须保留每次命令结果、等待时间和 `request_id`（如有），先查是否存在迟到回执或未登记资产，不能直接归因于 provider。

`wait` 返回完成且原子 `result-file` 有效时，必须先上传模型原图并用该原始 JSON、原图附件 ID、`prompt_sha256`、`request_id` 和同一
`operation_id`/attempt 调用 `image-operation-put` 将 operation 结算为 `completed`；确认服务端返回 `completed` 后，才允许写
`asset-put`。不得以“模型回图已在本地”为由跳过这一步，否则 canonical asset 必然被 running operation 拒绝。该写回冲突只允许补齐
operation 完成对账并重试同一 asset-put，不得重新调用模型。
模型调用画布必须遵守 GPT Image 2 的 16px 边长约束，交付尺寸与模型画布分开记录：`1080x1080` 使用 `1088x1088`，`1200x628` 使用 `1200x624`，`800x1000` 使用 `800x992`。当前 CLI 接受 canonical 交付尺寸并自动映射到上述 provider canvas；完整模型 JSON 必须同时保留请求尺寸和实际 provider canvas。模型输出必须经过规范化到 `1080x1080`、`1200x628`、`800x1000`。比例偏差 `<=10%` 直接接受并规范化；`10%-25%` 且已存在可下载的拒绝回图时，用该回图和同尺寸 Prime context 做一次 canvas repair retry，提示词只要求压回锁定画布并保留全部业务内容；`>25%` 视为真实画布跑偏，只重生当前失败尺寸并使用更强的 CANVAS LOCK 提示词。模型调用只能通过当前 CLI 的映射，不能自行把 canonical 交付尺寸改写成其他 provider 参数。

```text
python3 <当前 Skill 目录>/references/normalize_image.py \
  --input <model.png> --output <normalized.png> --width <w> --height <h> \
  --model-size <requested-model-size> --max-aspect-deviation 0.10 --evidence <normalization.json>
```

当前阶段所有 `expected_sizes` 底图完成后登记 generated assets，再调用绑定的贴片 Skill。候选阶段只合成主尺寸并等待
`creative_candidate_selection`；selected 阶段三尺寸合成后进入完整 QC。最终 QC 只以真实 Prime 合成图为准：实际遮挡、文字不可读、
底部图标被盖住或联合验收确认某尺寸偏离 DesignDNA 才算失败；靠近边界或预估覆盖不是失败。

## 过程证据登记

过程图片不是工作目录里的私有日志，而是订单可追溯证据。每个尺寸的 `Prime context`、模型原图和规范化底图一生成并确认文件存在，
就必须通过仓库内的登记助手上传并写回当前订单、Variant、revision 和 task：

```text
python3 <当前 Skill 目录>/references/register_process_assets.py \
  --order-id <order-id> --variant-id <variant-id> --revision <revision> --task-id <task-id> \
  --cli multica \
  --image 1080x1080 "Prime context" <prime-context-1080x1080.png> \
  --image 1080x1080 "模型原图" <model-1080x1080.png> \
  --image 1080x1080 "规范化底图" <normalized-1080x1080.png>
```

按文件实际生成情况重复调用，当前阶段的每个 expected size 和三个过程阶段都要登记；不要等 task 结束才批量登记。命令必须返回 JSON，且 `registered` 数量与本次输入一致，
再用 `multica creative order get <order-id> --output json` 回读确认归属。底图与过程证据齐全后调用绑定的贴片 Skill；后端会在确定性合成时登记 `Prime 合成成图`，出图智能体不要伪造该阶段。
任何登记失败都要保留真实错误并让当前 task 失败，以便平台创建有界续跑；在当前 revision 的所有 expected size 都有 canonical generated asset、完整过程证据并且贴片 Skill 返回后端合成成功前，禁止调用
`multica task complete`。

每个 `prompt-contract-<size>.json` 除 `prompt_sha256` 外必须写入当前 Variant brief
`creative_contract.parent_direction_sha256`，以及 canonical `creative_intent_sha256`、`design_dna_sha256` 和当前
`layout_plan_sha256`；并保留 `size_key`、`revision`、`variant_id`、`candidate_state`、`input_roles`、`locked_set`、
`editable_set`、`active_content_groups` 和逐项 `acceptance_checks`。若存在身份锚点，还必须写 `identity_anchor`、Input 4 的 input role、
所用 selected-primary asset 及“同一人物/产品/核心对象，不得替换”的约束摘要。若使用 App UI 参考图，还必须写 `app_ui_reference_attachment_id`、
`resource_file_id`、Input 3 的 input role、是否实际用于当前尺寸，以及只替换手机屏幕内容的约束摘要。该父方向哈希是 visual QC
追溯父方向的证据；DesignDNA 与 LayoutPlan 哈希共同用于三尺寸联合验收，不能省略、伪造或从旧 revision 复制。

生产 task 只有在当前阶段的每个 `expected_sizes` 都存在完整、可验证的 `generated/completed` canonical asset 后才允许调用
`multica task complete`。只生成方形或只生成部分尺寸时不能提前 complete，也不能把缺失尺寸写成成功；应在同一个 task 中继续补齐，或把真实错误交给
`multica task fail`。候选阶段仅有 `primary_size` 时完成该尺寸就是该阶段齐全，不得擅自扩为三尺寸。若 task 因模型、网络或 daemon 中断而先结束，平台会在服务端自动创建有上限的 fresh continuation，并保持 Variant 为 running，直到当前阶段尺寸齐全或达到上限。

## 自适应恢复

失败恢复优先复用已经生成的证据，只有缺少有效回图或真实视觉失败才重新调用模型：

- `asset-put`、上传、协议、prompt/hash 或证据写回失败时，复用同一 normalized 图、同一模型 JSON、同一 prompt-contract 和 normalization evidence 补登记；不得重新出图。
- Prime 的传输/进程类失败由后端 durable job 恢复，不重新生成 generated asset；`qc_risk` 质量证据继续进入真实 Prime 的最终 QC，不单独重调模型。
- 部分尺寸失败时，只补当前 revision 缺失的尺寸；已有 canonical generated asset 的尺寸跳过。
- 比例失败按 `<=10%` 接受、`10%-25%` canvas repair、`>25%` 重生当前尺寸处理。
- recovery 续跑必须先回读订单和过程证据，确认哪些尺寸已经有 canonical、哪些尺寸只有过程图、哪些尺寸没有有效回图，再选择补登记、补贴片或补生成。

### generated asset 写回格式

每个尺寸都必须用同一 Variant、当前 revision 和已上传附件写回一个完整的 canonical JSON 对象，再调用
`multica creative order asset-put <order-id> --input-file <asset.json> --model-result-file <model.json> --prompt-contract-file <prompt-contract.json> --normalization-evidence-file <normalization.json>`：

```json
{
  "variant_id": "<variant-id>",
  "asset_family_id": "<same-family-id-for-this-variant>",
  "size_key": "1200x628",
  "revision": 1,
  "stage": "generated",
  "status": "completed",
  "operation_id": "<image-operation-put-response-id>",
  "attachment_id": "<normalized-attachment-id>"
}
```

候选主尺寸首次写回时使用平台为该 Variant 分配或返回的 `asset_family_id`；selected 扩尺寸必须逐字复用同一个 family ID，不能按尺寸新建 family。
`model-result-file` 必须是该尺寸模型调用返回的完整 JSON，不能只摘录 prompt 或 request ID；模型结果、prompt contract 和 normalization evidence 必须逐尺寸对应。
只接受上面的 canonical 字段：不得使用 `kind`、`asset_type` 代替 `stage`，也不得省略 `status`、`revision`、`operation_id` 或 `attachment_id`，
避免生成成功却没有进入订单资产链路。

## 有界返工

如果视觉 QC 指出 `actual_prime_obstruction` 或 `official_prime_text_unreadable`，服务端最多为当前 Variant 排两轮有证据的定向返工。
返工仍由出图智能体重新写当前尺寸的短提示词。`deterministic` 的 Input 1 必须是上一 revision 的同尺寸无品牌 generated 底图，不是最终成图；
`model_integrated` 的 Input 1 则是上一 revision 的同尺寸模型成图，Input 2 是同一冻结的 QR-free 完整模板。两种模式都只改失败验收目标，
不得更换模板族、添加 QR 或要求后端二次贴片。
只描述实际遮挡和需要压缩/移动的内容，不改文案、金额、期限、表格、
视觉身份或 Prime 规则。两轮返工后仍失败时，当前 staging revision 不得发布：已有 active revision 时继续保留并展示 active；首次交付没有
active revision 时由平台自动晋级最低序号 reserve 并只补缺失尺寸。没有可用 reserve 才进入人工处理，不能由 Agent 自报通过、自动带风险归档，
也不要把过程图转成候选或要求用户选择技术实现。

过程图片只用于排查，固定登记 `Prime context`、`模型原图`、`规范化底图` 和 `Prime 合成成图`。不得把过程图当作最终交付或采用图。
