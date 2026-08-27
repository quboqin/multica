---
name: multica-ad-creative-direct-edit
description: "按用户自然语言对 Creative Order 的无品牌底图做定向修改，并将结果交给平台重新贴片时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告图片直接修改

这是 Creative Order 的精准调整路径，不执行素材爬取、参考分析、三套新创意或标准出图。任务 context 必须给出
`<order-id>`、`<variant-id>`、`revision`、`expected_sizes`、`target_size`、`scope`、`user_request`、`delivery_mode`、
`reviewer_agent_id`、无品牌 `source_asset_id/source_attachment_id`、与本次 `expected_sizes` 对应的 `source_assets` 和可编辑的无品牌 base asset。没有这些字段，或
只有带品牌组件、二维码、Logo、条款、商店徽章的最终图而无法追溯 base asset 时，将 variant 写为 `action_required`；
不得把最终 Prime 图作为可编辑来源。

订单冻结的 `copy_snapshot` 仍是可见文案和业务事实的唯一真值；精准调整不能改写、补造或删除已批准文案。
`raw_user_request`/`user_request` 是审计和意图输入，**不是**最终模型 prompt 的逐字合同。保留原文用于过程证据，但先结合
`annotations`、annotation brief、当前 Prime 成图关系和固定保护约束，编译为可执行的结构化编辑方案；最终 prompt 可以完全重写用户原话。

运行中的 Agent 进程由 daemon 注入 `MULTICA_TASK_ID`，它是当前 direct-edit task 的唯一 task ID。不要从 `issue_id`、
`direct_edit.adjustment_issue_id`、`variant_id` 或 `item_key` 猜 task ID，也不要把 Issue ID 当成 task ID 写入任何领域 JSON。
诊断资产的 `task_id` 只允许使用这个环境变量；它为空时可以省略。若服务端报告 task ownership 不匹配，先修正为环境变量的值；
仍是仅诊断资产写回时才允许去掉 `task_id` 重试。不能因为 task 关联错误重新调用 Image Edit。

`expected_sizes` 是本次调整后要交付的完整尺寸集合。`scope=size` 是默认模式，只把 `target_size` 交给 Image Edit，
其他尺寸沿用上一 revision 的 generated base 和过程证据，不得为未修改尺寸重新调用图像模型。`scope=variant` 是全部交付尺寸模式，
必须按 `edit_sizes`/`expected_sizes` 逐尺寸处理，使用 `source_assets` 中同尺寸的 `asset_id/attachment_id` 作为该尺寸 Input 1，
不能沿用不属于声明 source revision 的 generated base 或过程证据冒充本次调整。source base 永远不可覆盖；输出必须写入平台已经锁定的 `revision`，
并以同尺寸 source asset ID 写入 `derived_from_asset_id`。

先执行 `multica creative order get <order-id> --output json` 确认 source base 属于该 variant、尺寸和 revision。`scope=size`
使用 `multica attachment download <source-attachment-id> --output-dir <work-dir>` 下载目标尺寸的无品牌底图；`scope=variant`
要为每个 `edit_sizes` 找到 `source_assets` 中同尺寸附件并分别下载。

如果 context 给出 `annotation_guide_attachment_id`，再下载该附件。它是用户在最终交付图上的标注 brief，可能包含 Prime 组件、
Logo、二维码、商店徽章、官方条款、红色矩形、编号和评论位置；这些都用于理解用户在最终图上看到的问题，不是可复制广告内容。
模型输入固定为：

1. `Input 1`：当前处理尺寸的同尺寸无品牌 source base，唯一的画面、文字、版式和视觉风格真值。
2. `Input 2`：最终交付图的 annotation brief，只用于读取用户红框、编号、评论位置和固定贴片遮挡关系；不得复制红框、编号、
   引导线、Logo、二维码、商店徽章、官方条款或其他 Prime 组件到输出。

没有 annotation guide 时只传 Input 1。`direct_edit.validation_rework` 存在时，`reference_attachment_id` 是上一 revision 已贴片的失败成图：
可作为 Input 2 读取 `failures` 中所述的真实遮挡关系，但仍绝不可编辑、复制或输出其中任何 Prime 像素。每个编号对应 `direct_edit.annotations` 中同序的 comment；多个红框必须逐一执行，不能合并、
忽略或只按总描述猜测。评论文字出现而红框未覆盖的独立问题也必须成为单独编辑目标，例如同一条反馈同时要求避开顶部二维码和底部条款时，
必须形成“标题组”和“表格组”两个目标，不能只处理红框所在的顶部。`reference_attachment_id` 只用于必要的人工对照，不得作为可编辑输入。若红框覆盖标题、贴片、Logo、
二维码或底部条款，说明用户是在指出最终交付图中的遮挡/关系问题；仍只修改 Input 1 的无品牌底图，让后续固定贴片重新叠加后解决问题，
不得尝试修改、重画或移除 Prime 组件。

先写入 `intent-plan.json`，至少包含：`raw_user_request`、有权限语义的 `input_roles`、`locked_set`、`editable_set`、
`change_budget`、优先级、逐项 `edit_goals`、每项目标的证据（红框编号或评论文字）、`target_masks`、
`allowed_reflow`、`must_preserve`、`prime_constraints` 和逐目标 `acceptance_checks`。每个红框、评论指向区域或视觉返工失败项必须有稳定
`target_id`，并关联一个逻辑 target mask；没有像素 mask 时使用 annotation 编号和语义区域作为 mask identity，不能把多个目标折叠成一个总目标。
再从该方案生成最终 prompt；不得把原话、默认禁令和
坐标机械拼接。默认的“不要重排未标注区域”只在不妨碍用户目标时生效：若多个关联内容组必须联动移动才能避开固定 Prime，明确授权在
`safe_content_frame` 内重排这些内容组和必要留白。保留的是业务事实、批准文案、人物主体与视觉风格，不是每个原始像素位置。若
`direct_edit.validation_rework.failures` 存在，它们是贴片后验收的最高优先级事实：逐尺寸将每个失败项转为 edit goal 和 acceptance check，
不得用底图目检或“看起来已移动”替代。

最终 prompt 只包含会改变像素的编辑指令，不得包含 order/task/revision、文件路径、哈希、request ID、上传、登记、重试、超时、状态、JSON、
CLI 或附件血缘。必须按以下优先级表达：只编辑 Input 1；Input 2/最终成图仅用于理解固定贴片关系；用户要达成的视觉结果；允许联动调整的
内容组；必须保持的业务事实；Prime 不可生成/不可复制约束；贴片后的验收条件。不要把红框、编号、Prime 组件或官方条款画进无品牌底图。
对于“上移 30px”这类几何要求，使用同尺寸画布和精确的移动方向，但不能只依赖抽象坐标判断完成。
对于“替换人物/换人/换模特”，提示词必须明确这是 replacement，不是微调：现有人物是移除目标，不是身份、五官、发型、服装、
姿势、手势、身形轮廓或构图参考；新人物必须在 1x 预览下肉眼可见地不同，并给出具体不同的年龄段、肤色/发型、服装、姿势和相对关系。
如果用户标注的是单独金额、核心利益点或促销卖点，例如 `Rp100Juta` 这类数值，不要默认把它锁成还款计划；
即使先前识别错了，也要保留后续手动改写空间，不要把流程卡死在还款计划选择。只有 Input 2 明确出现还款表、
分期卡、期限列、月供列或总还款结构时，才把它当成 repayment 结构处理，其他情况保留为可编辑文案。

只调用 `multica image edit`。先保存 `image-operation-put` 的完整响应，从响应的 `id` 和对应 `attempts[].attempt` 读取坐标；
有 annotation guide 时保持 source 在前、guide 在后：

```bash
multica image edit \
  --input <source-base.png> \
  --input <annotation-brief.png> \
  --prompt "<局部编辑提示词>" \
  --size <provider-size> --quality high --output-file <model-output.png> \
  --result-file <workdir>/image-edit-result-<size>.json \
  --operation-id "$(jq -er '.id' <image-operation-response.json>)" \
  --operation-attempt "$(jq -er '.attempts | last | .attempt' <image-operation-response.json>)" \
  --output json
```

`--size` 使用平台允许的 provider 画布，最终文件必须归一化为当前处理尺寸；`scope=size` 当前处理尺寸就是 `target_size`。
保留 CLI 返回的完整 JSON 为
`image-edit-result.json`；其中的 `prompt` 和 `prompt_sha256` 是唯一真值，不能从展示用的 `prompt.txt`、用户原话或重新拼接的
字符串恢复 hash。首次登记时**不得填写** `prompt_sha256`：CLI 可能规范化提示词文件末尾换行，只有原子回执可提供可验的 hash。
完成同一 operation 后，才从该回执读取 `prompt_sha256`、`request_id`、`provider_attempts`、实际宽高和本次 attempt 写回；每个尺寸最多执行一次有明确原因的编辑重试，传输重试不计入编辑次数。

调用前先用 `multica creative order image-operation-put <order-id> --input-file <operation.json> --output json` 登记尺寸级持久化调用。
首次 `operation.json` 的必填字段是 `variant_id`、`size_key`、`revision`、`operation_kind`、`idempotency_key`、`status: "running"`、`model`、`input_snapshot` 和 `attempt: 1`；`input_snapshot` 必须是对象，至少写输入资产指纹、输入角色与 target size。首次 JSON 不得带 `prompt_sha256`、request ID、结果回执或输出附件。完成写回时使用同一组坐标，并带入原子 `image-edit-result.json` 的唯一真值。
一般精准修图使用 `operation_kind=direct_edit`，QC 返工使用 `visual_rework`，画布修复使用 `canvas_repair`；稳定幂等键为
`<variant-id>:r<revision>:<size>:<operation-kind>:v1`。只有 `status=running` 的登记返回 `disposition=invoke` 才可发起模型调用；
返回 `reconcile` 时检查同一个原子 `result-file`、订单中的 operation 和迟到回执，禁止重发；返回 `reuse` 时直接复用已完成回图。
保存响应中的 `id` 作为当前尺寸的 `operation_id`，后续 accepted generated asset 必须带同一个值。
两个 operation 参数都来自 `image-operation-put` 响应；CLI 会把 daemon 注入的 task ID、operation 坐标、provider request ID、
prompt hash 和输出 hash 一起写入原子 result-file。缺少其中任一参数都禁止调用 provider。
完成后上传模型原图，并把同一 attempt 更新为 `completed`，保存完整 result receipt、request ID、provider status、耗时和输出附件；
runtime 中断或 provider 状态不明时更新为 `unknown`。unknown 必须先对账原子 result-file、已有模型输出、过程附件和 provider 回执；只有确认都没有有效结果时，才把同一 attempt 更新为 `failed`，并提交 `error_type=provider_receipt_not_found`、`reconcile_confirmed=true`，之后才允许递增 attempt。不得更换幂等键绕过 unknown。其他明确非零退出且没有有效 result-file/回图时，才写带真实错误类型的 `failed`。
若首次 `direct_edit` 已有成功回图但最终视觉验收失败，先把它保留为未采用过程图，绝不能将其写成 canonical asset 或 Prime。只有 task context 的 `direct_edit.visual_rework_budget >= 1` 时，才允许一次后续 `visual_rework`：新建操作使用稳定键 `<variant-id>:r<revision>:<size>:visual_rework:v1`、`operation_kind=visual_rework`、`attempt=1`，输入快照必须引用被拒绝过程图的附件和失败验收项。模型 Input 1 使用该被拒绝回图，prompt 只强化失败的安全区移动与冻结内容，禁止改写已通过的内容。这个返工是独立的受平台预算保护的 operation，不得把已完成的 `direct_edit` operation 改回 running，也不得用其他 key 建立第二次返工；返工仍失败即保留证据并转人工。
task context 含 `late_receipt_recovery` 时，原精准修图已经成功，daemon 也已可信归档。按 `late_receipt_recoveries` 逐项处理所有迟到尺寸
（单项时也保留 `late_receipt_recovery` 作为首项指针），回读每个 operation/attempt 的 `result_receipt`，下载对应
`output_attachment_id`，只补归一化、过程登记、`asset-put` 和 Prime；禁止再次调用 `multica image edit`、
禁止新建 operation/attempt。精准改图的局部编辑、标注图、锁定集合和验收规则全部保持不变。

模型原图只能作为过程图，不能直接写入 canonical generated asset。采用前必须把回图归一化到当前处理尺寸：

```bash
python3 <当前 Skill 目录>/../ad-creative-production/references/normalize_image.py \
  --input <model-output.png> --output <accepted-base.png> --width <w> --height <h> \
  --model-size <provider-width>x<provider-height> --max-aspect-deviation 0.10 --evidence <normalization.json>
```

确认 `accepted-base.png` 的像素尺寸与 `target_size` 完全一致后再继续。精准修图与标准出图共用同一比例策略，不按像素绝对值判断：`1254x1254` 到 `1080x1080` 的比例偏差为 0%，脚本成功后继续使用同一回图，不得重编辑；比例偏差 `<=10%` 直接归一化，`10%-25%` 使用已有回图做一次 canvas repair，`>25%` 才只重生当前失败尺寸。归一化失败时只保留过程图，并按该策略处理。

每一次 Image Edit 实际产生图片后，无论最后采用还是拒绝，都必须先上传并登记过程图：

```bash
multica attachment upload <model-output.png> --output json
multica creative order diagnostic-asset-put <order-id> --input-file <direct-edit-process.json> --output json
```

采用的回图使用 `workflow: "creative_direct_edit"`、`label: "直接改图结果"`；被拒绝的回图使用同一 workflow、当前尺寸和 revision，
label 写为 `直接改图尝试 <attempt> · 未采用`。metadata 必须保留 `accepted`、`attempt`、拒绝原因、source revision、source asset、
annotation guide attachment、模型、request ID、prompt_sha256 和目标尺寸。回读订单，确认附件属于当前 variant、task、revision 和 size。

只有被采用的回图才能写入 `stage: "generated"` 的 canonical asset。先上传同一张已采用回图，再准备三份与该尺寸完全对应的必需证据文件：

canonical 上传必须使用归一化后的 `accepted-base.png`，不得复用 `model-output.png` 的诊断附件 ID。`model-output.png` 只属于
`creative_direct_edit` 过程图；`asset-put` 的 `attachment_id` 必须来自 `accepted-base.png` 的上传结果。

- `image-edit-result.json`：Image Edit 返回的完整 JSON，必须包含原始 `prompt`、匹配的 `prompt_sha256`、model、request ID、attempt 和实际画布；不得保留本地 `path`。
- `prompt-contract.json`：至少包含与模型结果完全相同的 `prompt_sha256`。
- `normalization.json`：包含当前处理尺寸的归一化证据。

三份必需证据必须一起传给 CLI：

```bash
multica attachment upload <accepted-base.png> --output json
multica creative order asset-put <order-id> --input-file <edited-asset.json> \
  --model-result-file <image-edit-result.json> \
  --prompt-contract-file <prompt-contract.json> \
  --normalization-evidence-file <normalization.json> --output json
```

`edited-asset.json` 必须保留原 `asset_family_id`，写入当前 task context 的 `variant_id`、当前处理尺寸 `size_key`、`revision`、
`operation_id: <image-operation-put-response-id>`、
`derived_from_asset_id: <当前尺寸 source asset id>`、`status: "completed"`。使用上述证据参数时，`edited-asset.json` 不得包含
`metadata` 或 `evidence` 字段；CLI 会从证据生成它们，并校验 prompt/hash、模型结果和 target size。用户原话、输入附件、
annotation guide、目检结论放在过程诊断资产的 metadata 或任务错误中，不要塞入 canonical asset 的自动生成字段。不得再次调用
`variant-put` 或把 revision 再加一。

所有需要编辑的 canonical generated base 写回后，调用绑定的 `素材_技能_贴片`；它只调用后端唯一的确定性 Prime composer，由后端为所有 expected sizes
重新贴回官方透明组件并登记 `primed` 和 Prime 合成过程图。精准调整不创建贴片 task；贴片后的最终图由平台创建 visual QC，不能把
无品牌底图自检当成交付验收。贴片 Skill 是唯一的官方组件交接来源。

如果回图已经存在，后续失败按协议层处理：task/variant 归属、JSON 字段、三份必需证据未同时提供、prompt/hash、
normalization 或本地 path 错误，都只修复对应 JSON、参数或 task_id，再用同一张上传附件和同一份模型结果重试；不要重新调用 Image Edit。
只有没有有效模型回图、Provider 明确返回图片失败，或目检确认修改未完成时，才按每尺寸最多一次限制重新编辑。上传失败重试上传，
不得重复生成。每次最终写回后都要回读订单，确认所有 `expected_sizes` 都有当前 revision 的 generated 资产、正确 source lineage 和过程图。

每次回图必须逐项核对 `target_masks[*].acceptance_checks`，并确认 `locked_set` 没有回归。只有所有 target mask 在同一张回图上同时通过，
该尺寸才能采用；标题已修复但表格回退、上方已避让但下方又遮挡、或只完成部分红框都必须拒绝并登记逐目标结果，不能取平均分或由 Agent 自报通过。

如果模型回图不满足要求，仍先登记该回图和失败原因，再按最多一次的限制重试。没有可采用回图时，写结构化 `error_code/error_message`
并让当前 task 失败；平台会把 variant 标为可重试，已登记的方图和其他过程图片必须保留。不得用文字结果冒充图片，不得删除失败过程图。

绑定的 `素材_技能_贴片` 返回后再次执行 `multica creative order get <order-id> --output json`，确认每个 expected size 都有当前 revision
的 `primed` 资产和 Prime 合成过程图；`final_visual_validation=true` 时不等待 delivered 资产，交由平台最终图视觉验收。该验收若发现
真实 Prime 遮挡，只会把失败尺寸创建为有上限的 `creative_direct_edit` 定向续调，继续使用无品牌底图，绝不编辑二维码、Logo 或条款。

只处理 task context 指定的对象、revision、target_size、edit_sizes 和 scope。不得创建或修改 Issue，不得用评论代替领域数据，不得触发采集、分析、
方案、标准生产、Prime agent 或 QC。
