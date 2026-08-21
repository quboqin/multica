---
name: multica-ad-creative-direct-edit
description: "按用户自然语言对 Creative Order 的无品牌底图做定向修改，并将结果交给平台重新贴片时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告图片直接修改

这是 Creative Order 的精准调整路径，不执行素材爬取、参考分析、三套新创意、标准出图或 QC。任务 context 必须给出
`<order-id>`、`<variant-id>`、`revision`、`expected_sizes`、`target_size`、`scope`、`user_request`、`delivery_mode`、
`reviewer_agent_id`、无品牌 `source_asset_id/source_attachment_id`、三尺寸 `source_assets` 和可编辑的无品牌 base asset。没有这些字段，或
只有带品牌组件、二维码、Logo、条款、商店徽章的最终图而无法追溯 base asset 时，将 variant 写为 `action_required`；
不得把最终 Prime 图作为可编辑来源。

订单冻结的 `copy_snapshot` 仍是可见文案和业务事实的唯一真值；精准调整只执行 `user_request` 指定的视觉修改，不能改写、补造或删除已批准文案。

运行中的 Agent 进程由 daemon 注入 `MULTICA_TASK_ID`，它是当前 direct-edit task 的唯一 task ID。不要从 `issue_id`、
`direct_edit.adjustment_issue_id`、`variant_id` 或 `item_key` 猜 task ID，也不要把 Issue ID 当成 task ID 写入任何领域 JSON。
诊断资产的 `task_id` 只允许使用这个环境变量；它为空时可以省略。若服务端报告 task ownership 不匹配，先修正为环境变量的值；
仍是仅诊断资产写回时才允许去掉 `task_id` 重试。不能因为 task 关联错误重新调用 Image Edit。

`expected_sizes` 是本次调整后要交付的完整尺寸集合。`scope=size` 是默认模式，只把 `target_size` 交给 Image Edit，
其他尺寸沿用上一 revision 的 generated base 和过程证据，不得为未修改尺寸重新调用图像模型。`scope=variant` 是三尺寸模式，
必须按 `edit_sizes`/`expected_sizes` 逐尺寸处理，使用 `source_assets` 中同尺寸的 `asset_id/attachment_id` 作为该尺寸 Input 1，
不能沿用旧 generated base 或旧过程证据冒充本次三尺寸调整。source base 永远不可覆盖；输出必须写入平台已经锁定的 `revision`，
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

没有 annotation guide 时只传 Input 1。每个编号对应 `direct_edit.annotations` 中同序的 comment；多个红框必须逐一执行，不能合并、
忽略或只按总描述猜测。`reference_attachment_id` 只用于必要的人工对照，不得作为可编辑输入。若红框覆盖标题、贴片、Logo、
二维码或底部条款，说明用户是在指出最终交付图中的遮挡/关系问题；仍只修改 Input 1 的无品牌底图，让后续固定贴片重新叠加后解决问题，
不得尝试修改、重画或移除 Prime 组件。

提示词保留用户原话，并追加执行约束：只编辑 Input 1；Input 2 仅用于理解用户标注和固定贴片位置；最终贴片会由平台重新叠加；
不要把红框、编号、Prime 组件或官方条款画进无品牌底图。明确修改对象、方向或像素量，并明确保留其余文字、金额、表格、主体、
背景、比例和视觉风格。对于“上移 30px”这类几何要求，使用同尺寸画布和精确的移动方向；不要重绘整张广告，不要重排未标注区域。
对于“替换人物/换人/换模特”，提示词必须明确这是 replacement，不是微调：现有人物是移除目标，不是身份、五官、发型、服装、
姿势、手势、身形轮廓或构图参考；新人物必须在 1x 预览下肉眼可见地不同，并给出具体不同的年龄段、肤色/发型、服装、姿势和相对关系。
如果用户标注的是单独金额、核心利益点或促销卖点，例如 `Rp100Juta` 这类数值，不要默认把它锁成还款计划；
即使先前识别错了，也要保留后续手动改写空间，不要把流程卡死在还款计划选择。只有 Input 2 明确出现还款表、
分期卡、期限列、月供列或总还款结构时，才把它当成 repayment 结构处理，其他情况保留为可编辑文案。

只调用 `multica image edit`。有 annotation guide 时保持 source 在前、guide 在后：

```bash
multica image edit \
  --input <source-base.png> \
  --input <annotation-brief.png> \
  --prompt "<局部编辑提示词>" \
  --size <provider-size> --quality high --output-file <model-output.png> --output json
```

`--size` 使用平台允许的 provider 画布，最终文件必须归一化为当前处理尺寸；`scope=size` 当前处理尺寸就是 `target_size`。
保留 CLI 返回的完整 JSON 为
`image-edit-result.json`；其中的 `prompt` 和 `prompt_sha256` 是唯一真值，不能从展示用的 `prompt.txt`、用户原话或重新拼接的
字符串恢复 hash。若需要 `prompt.txt`，只用于人读并确保末尾换行不进入提交内容。同时保留 model、request ID、实际宽高和本次
attempt。每个尺寸最多执行一次有明确原因的编辑重试；传输重试不计入编辑次数。

每一次 Image Edit 实际产生图片后，无论最后采用还是拒绝，都必须先上传并登记过程图：

```bash
multica attachment upload <model-output.png> --output json
multica creative order diagnostic-asset-put <order-id> --input-file <direct-edit-process.json> --output json
```

采用的回图使用 `workflow: "creative_direct_edit"`、`label: "直接改图结果"`；被拒绝的回图使用同一 workflow、当前尺寸和 revision，
label 写为 `直接改图尝试 <attempt> · 未采用`。metadata 必须保留 `accepted`、`attempt`、拒绝原因、source revision、source asset、
annotation guide attachment、模型、request ID、prompt_sha256 和目标尺寸。回读订单，确认附件属于当前 variant、task、revision 和 size。

只有被采用的回图才能写入 `stage: "generated"` 的 canonical asset。先上传同一张已采用回图，再准备四份与该尺寸完全对应的证据文件：

- `image-edit-result.json`：Image Edit 返回的完整 JSON，必须包含原始 `prompt`、匹配的 `prompt_sha256`、model、request ID、attempt 和实际画布；不得保留本地 `path`。
- `prompt-contract.json`：至少包含与模型结果完全相同的 `prompt_sha256`。
- `copy-validation.json`：`{"passed": true}` 的通过证据。
- `normalization.json`：包含当前处理尺寸的归一化证据。

四份证据必须一起传给 CLI：

```bash
multica attachment upload <accepted-base.png> --output json
multica creative order asset-put <order-id> --input-file <edited-asset.json> \
  --model-result-file <image-edit-result.json> \
  --prompt-contract-file <prompt-contract.json> \
  --copy-validation-file <copy-validation.json> \
  --normalization-evidence-file <normalization.json> --output json
```

`edited-asset.json` 必须保留原 `asset_family_id`，写入当前 task context 的 `variant_id`、当前处理尺寸 `size_key`、`revision`、
`derived_from_asset_id: <当前尺寸 source asset id>`、`status: "completed"`。使用上述四个证据参数时，`edited-asset.json` 不得包含
`metadata` 或 `evidence` 字段；CLI 会从证据生成它们，并校验 prompt/hash、模型结果、复制校验和 target size。用户原话、输入附件、
annotation guide、目检结论放在过程诊断资产的 metadata 或任务错误中，不要塞入 canonical asset 的自动生成字段。不得再次调用
`variant-put` 或把 revision 再加一。

所有需要编辑的 canonical generated base 写回后，调用绑定的 `素材_技能_贴片`；它只调用后端唯一的确定性 Prime composer，由后端为所有 expected sizes
重新贴回官方透明组件并直接登记 `primed`、`delivered` 和 Prime 合成过程图。精准调整不创建贴片 task、不调用 QC、不创建 QC 子 Issue；
贴片 Skill 是唯一的官方组件交接来源。

如果回图已经存在，后续失败按协议层处理：task/variant 归属、JSON 字段、四份证据未同时提供、prompt/hash、copy validation、
normalization 或本地 path 错误，都只修复对应 JSON、参数或 task_id，再用同一张上传附件和同一份模型结果重试；不要重新调用 Image Edit。
只有没有有效模型回图、Provider 明确返回图片失败，或目检确认修改未完成时，才按每尺寸最多一次限制重新编辑。上传失败重试上传，
不得重复生成。每次最终写回后都要回读订单，确认所有 `expected_sizes` 都有当前 revision 的 generated 资产、正确 source lineage 和过程图。

如果模型回图不满足要求，仍先登记该回图和失败原因，再按最多一次的限制重试。没有可采用回图时，写结构化 `error_code/error_message`
并让当前 task 失败；平台会把 variant 标为可重试，已登记的方图和其他过程图片必须保留。不得用文字结果冒充图片，不得删除失败过程图。

绑定的 `素材_技能_贴片` 返回后再次执行 `multica creative order get <order-id> --output json`，确认每个 expected size 都有当前 revision
的 `primed` 与 `delivered` 资产和 Prime 合成过程图；缺任何尺寸都不能报告完成。精准调整不执行 QC。

只处理 task context 指定的对象、revision、target_size、edit_sizes 和 scope。不得创建或修改 Issue，不得用评论代替领域数据，不得触发采集、分析、
方案、标准生产、Prime agent 或 QC。
