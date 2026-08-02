---
name: multica-ad-creative-production
description: "当专业子 Issue 已指定一个创意母版或一个尺寸重排任务，需要通过平台图像接口生成一张无品牌资产广告底图并回传调用证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告图像编辑

本 Skill 一次只执行一个图像编辑任务：生成一个 `1080x1080` 创意母版，或把一个已通过母版
原生重排为 `1200x628` / `800x1000`。使用子 Issue 中已发布的原样提示词、尺寸和获批输入
快照；当前运行时只挂载本 Skill 及其 `references/`，不要查找其他 Skill 的本地文件。
不要分析竞品、编写提示词、执行 Prime 包装或做最终验收。

精准返工任务必须读取上层返工 Issue 的结构化 metadata。只生成指定的一个尺寸；输入中把
`creative_base_attachment_ids` 对应的上一版无品牌底图放在第一位，并把用户反馈写成局部修改
要求。带 Prime、Logo、条款、商店徽章和二维码的最终成图只能用于视觉对照，不能作为第一图像
输入，也不能让模型重画这些确定性资产。

若 metadata 含 `creative_adjustment_mode=replan`，这是创意重做而非局部修补。必须把原候选图作为
第一图像输入，上一版底图只能作为负面对照；严格继承 brief 的业务语义、信息机制、视觉锚点和
色系锚点。不得继续以偏题底图为生成起点。

标题含“安全区恢复”时，只修复子 Issue 指定的 Prime 实际覆盖冲突。把上一轮无品牌底图作为
第一输入，原候选图作为语义参考；保留获批文案、业务语义、信息机制、人物身份、色系和变体
骨架，只把冲突的文案、金额、按钮、主体或关键卡片整体移入安全区。不得借恢复任务重做创意。

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

使用 Multica 的 `image edit` 受控能力生成底图。它通过本地 daemon 环境中配置的
GPT Image 编辑接口运行，不依赖 Codex app-server 是否暴露 `image_gen`：

```bash
multica image edit \
  --input <selected-reference.webp> \
  --input <selected-adakami-ui.jpg> \
  --prompt-file <approved-prompt.txt> \
  --size 1088x1360 \
  --max-attempts 3 \
  --output-file <base.png>
```

模型画布尺寸须遵守接口限制（边长为 16 的倍数）。4:5 的 1080x1350 交付尺寸对应
1088x1360 原生画布；只允许在交付阶段作确定性等比例尺寸适配，禁止改变构图或加边。
当前平台固定交付尺寸直接使用下列合法模型画布，不要先提交必然失败的目标尺寸：

- `1080x1080` -> `1088x1088`
- `1200x628` -> `1680x880`
- `800x1000` -> `832x1040`

图像编辑智能体的平台并发上限为 5。每个子 Issue 仍只调用一张图；并发由 Multica 任务调度器
执行，Skill 内禁止轮询、等待其他 Issue 或自行启动后台进程。这样给图像 API 的六路额度保留
一路余量，并允许三个创意母版与已就绪的尺寸重排混合执行。

生成时：

- 每种交付比例都必须原生构图，不能把一张图缩放、加边或裁切成其他尺寸；
- 保持参考图的有效信息密度、视觉中心和层级，不要把主体压缩到中间；
- 不留整条顶部/底部空白，不画二维码框、二维码、Logo、法律页脚、空白卡片或不可读 UI；
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

逐张进行肉眼复核：确认原生构图、满版平衡、获批文案可读、右上具有自然低纹理背景，
且没有二维码、二维码框或扫码提示。每张底图用下列方式上传：

`--max-attempts 3` 只对 408、429、5xx 和网络中断做同一请求的短退避重试，不代表创意返工。
单个尺寸最多执行两轮创意调用（初次生成 + 一次有明确原因的定向返工）。第二轮仍有
非阻断性视觉差异时，选择更好的版本并如实记录风险；不得自行继续消耗模型调用。Prime
外围缓冲区内只有连续背景、空白卡片下缘或阴影属于非阻断差异，应将该尺寸置为 `done` 并
交给 Prime 包装后的真实合成图做最终遮挡判断。只有批准文案、金额、按钮、主体或关键卡片
内容进入模板实际不透明图文覆盖区时才回传 `needs_input`。不要把规划阶段为呼吸感预留的
宽泛矩形当成模板的真实不透明边界。

```bash
multica issue comment add <issue-id> --content-file <review.md> --attachment <image.png>
```

评论必须写明来源候选/附件 ID、变体编号、任务类型、完整提示词、模型、尺寸、请求 ID、
CLI 返回的尝试次数、肉眼复核和风险。
成功后交接 Prime 包装阶段；失败时只列出该尺寸或变体的精确重跑建议。人工反馈成为
下一轮输入，不能覆盖历史记录。

## 来源映射

CLI 与 Issue 附件契约见 `references/ad-creative-production-source-map.md`。
