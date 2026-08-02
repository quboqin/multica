---
name: multica-ad-creative-production
description: "当专业子 Issue 已指定一个创意变体或精准返工作用域，需要在同一原生 Issue 内生成母版及其横竖原生重排并回传完整调用证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告图像编辑

初次生产时，一个专业子 Issue 负责一个完整创意变体：先生成并检查 `1080x1080` 方形母版，
再以该母版为第一输入，同时原生重排 `1200x628` 横版和 `800x1000` 竖版。三个尺寸的提示词、
请求 ID、耗时、附件和返工记录都留在同一个变体 Issue。使用子 Issue 中已发布的原样提示词、
尺寸和获批输入快照；当前运行时只挂载本 Skill 及其 `references/`，不要查找其他 Skill 的本地
文件。不要分析竞品、编写提示词、执行 Prime 包装或做最终验收。

精准返工任务必须读取上层返工 Issue 的结构化 metadata。`creative_scope=size` 只生成指定尺寸；
`creative_scope=variant` 在同一 Issue 内同时处理该变体三个尺寸；
`creative_scope=variant_subset` 只生成同一变体 `affected_sizes` 列出的多个尺寸。输入中把
`creative_base_attachment_ids` 对应的上一版无品牌底图放在第一位，并把用户反馈写成局部修改
要求。带 Prime、Logo、条款、商店徽章和二维码的最终成图只能用于视觉对照，不能作为第一图像
输入，也不能让模型重画这些确定性资产。

若 metadata 含 `creative_adjustment_mode=replan`，这是创意重做而非局部修补。必须把原候选图作为
第一图像输入，上一版底图只能作为负面对照；严格继承 brief 的业务语义、信息机制、视觉锚点和
色系锚点。不得继续以偏题底图为生成起点。

标题含“安全区恢复”时，只修复子 Issue 指定的 Prime 实际覆盖冲突。每个 `affected_sizes` 项都把
该尺寸上一轮无品牌底图作为对应 job 的第一输入，原候选图作为语义参考；保留获批文案、业务语义、
信息机制、人物身份、色系和变体骨架，只把冲突的文案、金额、按钮、主体或关键卡片整体移出 QC
指出的真实品牌覆盖区。不得借恢复任务重做创意，也不得把一个变体的多个失败尺寸拆成多个 Issue。

metadata 的 `scope=dependent_sizes` 表示方形母版已经可复用。必须把 `base_attachment_id` 对应的
方形无品牌底图作为缺失尺寸的第一输入；`missing_sizes` 未提供时一次执行横版和竖版两项
`image edit-batch`，提供时只生成列出的缺失尺寸。不得再次生成方形图，也不得重做
`accepted_size_attachment_ids` 已列出的通过尺寸。

## 1. 读取 Issue 与生成规格

先读取当前 Issue 及全部评论，只能使用人工选中的素材：

```bash
multica creative materials <issue-id> --selected --output json
```

如果 Issue 描述或评论明确把某个附件标成“人工已选参考图”，从评论时间线取得
附件 ID，并用 `multica attachment download <attachment-id>` 下载。记录来源候选或
附件 ID。图片不在本地、不可读，或未明确被选中时，回传 `needs_input`；不能依据
缩略图猜测，也不能把未选素材当作授权。

候选池素材必须使用 `multica creative materials <父 Issue ID> --selected --output json`
返回的 `archived_url`，不得直接下载 `preview_url`、`resource_url` 或 Issue 描述里的
AppGrowing 临时 CDN URL。`archive_status` 不是 `completed` 或 `archived_url` 为空时回传
`needs_input`，由平台重试归档。用平台 CLI 下载归档图：

```bash
multica creative material download <父 Issue ID> <候选 ID> --output-file <reference-image>
```

评论中记录候选 ID 和归档状态，不展示外部临时签名参数。

确认每张输入图都对应生成规格中的来源候选或附件 ID。母版任务以候选图为第一输入；尺寸重排
任务必须先下载该变体已通过的方形母版，并把母版作为第一输入，候选图和 UI 参考只能排在其后。
重排时必须保留同一变体的核心概念、主体身份、文案和风格，只做目标比例的原生空间重组。
参考图只提供构图信号，严禁复用
其品牌、卖点、金额、条款、二维码或 Logo。

生成规格列出 `app_ui_reference` 时，用附件 ID 下载分析成员选中的 AdaKami UI 文件。只使用
规格明确选择的文件，不自行遍历或混入其他市场资源。候选图与 UI 参考图一起传给模型；提示词
必须说明第一张是构图参考、后续图片是必须忠实采用的 AdaKami 产品 UI。候选图含竞品 UI 但
规格没有合格的 UI 参考时回传 `needs_input`。

## 2. 在智能体运行时生成底图

使用 Multica 的 `image edit` 和 `image edit-batch` 受控能力生成底图。它们通过本地 daemon
环境中配置的 GPT Image 编辑接口运行，不依赖 Codex app-server 是否暴露 `image_gen`。

初次生产必须严格执行两个图片波次：

1. 使用 `multica image edit` 生成方形母版；下载结果并按本 Skill 第 3 节做一次肉眼检查。
2. 母版没有底图自身的阻断问题后，写入只含横版和竖版两个 job 的 JSON manifest，然后一次运行：

```bash
multica image edit-batch --input-file <variant-batch.json> --output json
```

两个重排 job 的第一输入都使用已检查的方形母版，第二输入才是候选原图；需要 App UI 时再追加
已锁定的 UI 参考。`max_concurrency` 固定为 `2`。不要让 Leader 为横版和竖版另外创建 Issue，
也不要在 Skill 内启动后台进程。示例 manifest：

```json
{
  "max_concurrency": 2,
  "jobs": [
    {
      "id": "landscape",
      "inputs": [{"path": "square.png"}, {"path": "reference.webp"}],
      "prompt_file": "landscape.txt",
      "size": "1680x880",
      "max_attempts": 3,
      "output_file": "landscape.png"
    },
    {
      "id": "portrait",
      "inputs": [{"path": "square.png"}, {"path": "reference.webp"}],
      "prompt_file": "portrait.txt",
      "size": "1024x1280",
      "max_attempts": 3,
      "output_file": "portrait.png"
    }
  ]
}
```

manifest 内所有 `path`、`prompt_file`、`mask` 和 `output_file` 都以 manifest 文件所在目录为基准，
不是以当前 shell 工作目录为基准。把 manifest、prompt 和本轮图片放在同一个产物目录时，字段只写
`square.png`、`landscape.txt`、`landscape.png` 这样的文件名；禁止再次加上产物目录前缀。命令行的
`--input-file` 优先使用该 manifest 的绝对路径，避免智能体切换工作目录后重复拼接目录。执行前先
确认 manifest 能被读取；路径校验失败不算图片调用，不应改为单张串行执行。

方形母版单次调用示例：

```bash
multica image edit \
  --input <selected-reference.webp> \
  --input <selected-adakami-ui.jpg> \
  --prompt-file <approved-prompt.txt> \
  --size 1088x1088 \
  --max-attempts 3 \
  --output-file <base.png>
```

模型画布尺寸须遵守接口限制（边长为 16 的倍数）。只允许在交付阶段作确定性等比例尺寸适配，
禁止改变构图或加边。
当前平台固定交付尺寸直接使用下列合法模型画布，不要先提交必然失败的目标尺寸：

- `1080x1080` -> `1088x1088`
- `1200x628` -> `1680x880`
- `800x1000` -> `1024x1280`（连续双请求实测均返回 4:5；不要使用会降级成近似 2:3 的
  `832x1040` 或 `1088x1360`）

provider 返回的实际 PNG 像素可能与请求画布略有差异。不要临时编写适配脚本；使用 Skill 自带的
`references/normalize_image.py` 输出精确交付尺寸并附上 evidence JSON：

```bash
python <当前 Skill 目录>/references/normalize_image.py --input <model.png> --output <delivery.png> \
  --width <target-width> --height <target-height> --evidence <resize-evidence.json>
```

工具只允许最多 3% 的等比覆盖裁边，不拉伸、不加边。源图比例偏差超过 3% 时工具会失败，此时说明
provider 没有返回目标比例，不能强行裁掉主体或表格。脚本成功并写出 evidence 时，其比例判定是
权威结果；不得因为模型原始像素与请求像素不完全相等而推翻结果或重做。竖版首轮固定使用
`1024x1280`；若它实际返回近似 2:3，允许只对该竖版用 `1280x1600` 做一次画布级重试。两者都已
实测能返回 4:5，但 provider 偶发忽略首轮尺寸；第二次仍失败则阻断，禁止继续循环。不得使用
`832x1040` 或 `1088x1360`。若两次 4:5 请求都被 provider 降级，允许对第二次模型原图执行一次
`--max-extension-fraction 0.21` 的完整内容背景延展：只在左右或上下把最外侧背景连续淡出，不裁主体、
不拉伸内容、不再次调用模型，并把 `method` 与 `extension_fraction` 写入 evidence。此兜底只用于
provider 尺寸不稳定，QC 仍须按实际成图判断；不得在首轮请求前使用。脚本路径必须以运行时实际
读取到的 `SKILL.md` 所在目录解析，不能假设当前 Issue 工作目录下存在 `references/`。

平台使用跨进程图片槽位把所有 `image edit` / `image edit-batch` 的实际 provider 请求合计限制为
5 路，给六路额度保留一路。智能体任务数不等于图片请求数：一个变体 Issue 在第二波会同时占用
两个图片槽位。没有槽位时 CLI 自己排队，不要自行重试、轮询或另外启动进程。

`creative_scope=variant` 精准返工不需要母版依赖时，把三个尺寸作为三个独立 job 一次提交；
`creative_scope=variant_subset` 和多尺寸安全区恢复把 `affected_sizes` 作为独立 job 一次提交；
`creative_scope=size` 只使用单次 `image edit`；`scope=dependent_sizes` 只提交
`missing_sizes`，两项都缺失时才并发横竖重排。创意重做仍先生成新方形母版，检查后再并发横竖重排。
批量恢复 manifest 的 `max_concurrency` 为受影响尺寸数与 5 的较小值；每个 job 使用自己的底图和
完整提示词，不能让一张底图充当另一尺寸的输入。

生成时：

- 每种交付比例都必须原生构图，不能把一张图缩放、加边或裁切成其他尺寸；
- 保持参考图的有效信息密度、视觉中心和层级，不要把主体压缩到中间；
- 不留整条顶部/底部空白，不画二维码框、二维码、Logo、法律页脚、空白卡片或不可读 UI；
- Prime 资源存在顶部和底部固定资产时，把各顶部硬覆盖区的最低边界合并成一条“顶部关键内容
  避让带”，把各底部硬覆盖区的最高起点合并成一条“底部关键内容避让带”。生成提示词必须要求
  两条带优先延续满版自然背景、渐变或低纹理，避免标题、数字、人脸、按钮、表格或关键卡片；
  避让带不是白色留白，也不能画成可见占位框。合并带是首轮提示词护栏，不是底图验收边界；
- 市场包 `prime_layout_contract.backdrop_rule` 是硬约束。固定 Prime 的 Logo、条款和底部监管资产
  必须在合成后清晰可见；若规则要求浅色或中等明度承托，不得在这些区域使用深色同色系背景；
- 右上只需形成自然、连续、低纹理的背景，不画任何“预留框”；
- 未获批准不得编造金额、利率、期限、日期、金融承诺或 CTA。
- 默认保持 brief 中的原图业务语义、信息机制、关键视觉锚点和主色家族；没有用户明确许可时，
  不得为了制造变体差异换成另一业务场景或另一主色系。
- 不得保留、临摹或改写竞品 App UI；需要界面时只采用已选择的 AdaKami UI 参考。

没有 `OPENAI_API_KEY`、合法 `OPENAI_BASE_URL` 或 `image edit` 命令时，明确回传
`needs_input`。严禁用 Pillow、SVG、Canvas、HTML、模板脚本或图库拼接生成广告底图；这些
只可用于批准的确定性包装和验收，不能代替模型。

图片接口配置：`OPENAI_BASE_URL` 是完整网关域名，`OPENAI_IMAGE_EDIT_PATH` 默认 `/images/edits`，
`OPENAI_IMAGE_FILE_FIELD` 默认 `image`。不要自行拼接 `/v1` 或改为 `image[]`。

## 3. 回传无二维码底图

逐张进行肉眼复核：确认原生构图、满版平衡、获批文案可读、右上尽量具有自然低纹理背景，
且没有二维码、二维码框或扫码提示。底图阶段不能观察 Prime 叠加后的真实可见性，所以内容进入
合并避让带只能记录为待 Prime/QC 验证的风险，不能据此调用第二轮模型或阻断横竖版。只有底图自身
存在错文案、不可读、偏题、竞品品牌、模型生成二维码/Logo 或明显破图时，才允许本 Issue 内定向
返工。初次生产把三张底图和 `image edit-batch` JSON 结果作为同一条
评论的附件一次上传：

正常通过路径只发布这一条最终评论，不要把已经存在于方案 Issue 的方案附件重新上传，也不要把
输入、提示词、模型记录和最终交付拆成多条评论。完整提示词、request ID、尝试次数、逐步耗时、
尺寸适配证据和逐图结论写入同一个 evidence JSON，并与三张最终底图、batch 原始结果一起上传。
发生定向返工时允许在返工前额外留一条失败证据评论，附失败图和明确原因；返工完成后仍只用一条
最终交付评论收口。Issue 会话中的工具调用已经保留完整执行轨迹，无需重复上传下载来的方案文件或
每个中间文件来制造审计痕迹。

`--max-attempts 3` 只对 408、429、5xx 和网络中断做同一请求的短退避重试，不代表创意返工。
单个尺寸最多执行两轮创意调用（初次生成 + 一次有明确原因的定向返工）。第二轮仍有
非阻断性视觉差异时，选择更好的版本并如实记录风险；不得自行继续消耗模型调用。Prime
避让带和实际不透明区域都只能作为底图风险提示：图像编辑角色不得在没有真实合成图的情况下
断言品牌资产不可读或关键内容已被遮挡。应将底图置为 `done`，交给 Prime 包装后的真实合成图和
独立 QC 判断。只有底图自身问题才在本 Issue 内阻断，不能把规划阶段的宽泛矩形当成成图失败。

```bash
multica issue comment add <issue-id> --content-file <review.md> \
  --attachment <square.png> --attachment <landscape.png> --attachment <portrait.png> \
  --attachment <batch-result.json>
```

评论必须逐尺寸写明来源候选/附件 ID、变体编号、任务类型、完整提示词、模型、尺寸、请求 ID、
CLI 返回的尝试次数、耗时、肉眼复核和风险。只有三个尺寸附件和批处理证据都已上传，初次生产的
变体 Issue 才能置为 `done`。失败时只在本 Issue 内重做失败尺寸；已经成功且输入指纹未变化的
尺寸直接复用，不能重复调用模型。人工反馈成为下一轮输入，不能覆盖历史记录。

## 来源映射

CLI 与 Issue 附件契约见 `references/ad-creative-production-source-map.md`。
