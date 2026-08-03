---
name: multica-ad-creative-qc
description: "当一张素材的三个创意九张成图或精准返工作用域齐备，需要批量进行二维码、可见文案、构图和金融合规终检时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

当前运行时只挂载本 Skill，不要查找其他 Skill 的本地文件。确认 Issue 描述或评论中
存在本轮候选 ID、文案记录及版本、市场资源包及版本、源成图附件 ID、服务端 QR 校验
快照和包装脚本 JSON 证据。QR 校验快照必须为 `passed`，并包含三个 Prime 模板各自的机器
解码值。缺少任一必需证据时，发布 `needs_input` 或 `QC FAIL`，不能
用竞品内容补全。

初次交付时，一个 QC Issue 验收同一素材的 V01-V03 共九张成图；`creative_scope=variant` 验收
指定变体三个尺寸，`creative_scope=size` 只验收 metadata 指定的一张，不等待或重复检查沿用
尺寸；`creative_scope=batch` 只验收 metadata 明确列出的本轮 1-9 张变更图，不重复检查先前已经
通过且附件未变化的尺寸。作用域内图片必须候选 ID、变体编号、文案版本和修订一致。先使用 Python
`Path.mkdir(parents=True, exist_ok=True)` 显式、幂等地创建本 Issue 独立工作目录，确认目录存在后，
再对作用域内 PNG 和
JSON 证据分别执行
`multica attachment download <attachment-id> --output-dir <验收目录>`；运行时支持并行工具调用时，
一次并行发出全部下载，不要让多个下载进程同时负责首次创建同一目录，也不要先查询 attachment
帮助或串行探索命令。
本角色不生成图片。

直接下载 Prime Issue 附件中原样的包装 manifest、`compose_result.json` 和作用域内最终图；如果
Prime 已按契约提供这两份 JSON，不得从包装评论或其他 evidence 重新构造。包装 manifest 使用
Prime 脚本原生的 `id/input/template/output/qr_payload` 作业字段；尺寸、状态和解码结果来自同 ID 的
compose result。QC 工具直接兼容该原生契约，不得要求 Prime 改写成另一套
`output_file/variant/size` 字段。下载后不要临时编写 Pillow/OpenCV 校验脚本。使用当前 Skill 自带
工具一次完成尺寸、文件名、Prime 批次结果、QR 独立复解码、满版边缘和九宫格证据：

```bash
python <当前 Skill 目录>/references/qc_batch.py \
  --manifest <prime-packaging-manifest.json> \
  --compose-result <compose-result.json> \
  --images-dir <final-images-directory> \
  --output <qc-machine-evidence.json> \
  --contact-sheet <qc-contact-sheet.png> \
  --hard-region-sheet <qc-hard-region-sheet.png>
```

脚本路径按运行时实际读取到的 `SKILL.md` 目录解析。原始 Prime 模板可以高于交付分辨率，包装脚本
会按目标画布缩放；QC 不得因为模板原文件尺寸与最终图不同而误报失败。机器工具不替代后续语义、
文案和遮挡人工判断。`edge_white_ratio_needs_visual_review` 只提示人工确认是否存在真实加边；浅色或
白色满版设计不得仅凭边缘颜色判失败。

机器检查通过后，先用脚本生成的九宫格接触表在原始分辨率做整批语义和构图复核。只有某个单元格
存在文字、遮挡、对比度或数值歧义时，才单独打开对应原图；不得固定逐张再次打开九个文件。
同时必须打开 `qc-hard-region-sheet.png` 的原始分辨率版本。红框是市场包 `hard_regions` 的真实 Prime
资产矩形，也是唯一空间硬门槛；顶部和底部上下文切片只帮助放大观察，属于 `soft guide`，不能把
整条切片当成硬区。关键内容进入软引导范围但没有与任何红框相交时不得判失败或要求返工。

逐图检查每个红框：Logo/条款/QR/商店徽章/监管资产下方应是连续、低纹理且具有足够对比度的背景。
Prime 资产不可读，或者 Prime 实际压住标题、批准金融数字、人脸、按钮、表格、关键卡片和正文，
属于阻断问题。仅有装饰纹理、阴影、非关键背景元素进入红框，但 Prime 资产和全部关键内容仍清晰，
记录质量警告，不得扩大成全图返工。

候选
锚点优先读取本 Issue 已注入的资源快照 `selected_item.creative_brief`，不要重复下载 30KB 以上的
完整方案附件。机器 evidence、九宫格和逐图评论已经构成完整验收证据，不要再写脚本合并出一份
重复的 `qc_evidence_final.json`。

按正常观看比例检查最终像素，并把结论分成 `blocking_failures` 和 `quality_warnings`。只有下列问题
可以进入 `blocking_failures`：文件缺失或损坏、尺寸/包装/二维码机器校验失败；任一 Prime Logo、
条款二维码、商店徽章或监管标识缺失、损坏或无法正常辨认；竞品品牌、竞品二维码、竞品法律文字
或竞品 App UI 残留；获批文案、金额、期限、利率或 `must_preserve` 关键内容缺失、被改写、事实错误
或被实际遮挡到无法理解；严重破图。以上任一项存在才发布 `QC FAIL`。

视觉平衡、局部拥挤、装饰元素接近 Prime、一般性对比度不足、非关键视觉锚点偏差但主信息仍完整，
都写入 `quality_warnings`。这些问题不得触发自动重做；整批没有阻断问题但存在警告时发布
`QC PASS WITH WARNINGS`，让结果正常进入看板并留待用户用自然语言决定是否调整。

同时执行市场包 `prime_layout_contract.backdrop_rule`。同色或深色背景只有在导致 Logo、右上条款、
二维码或底部监管资产无法正常辨认时才是阻断问题；资产仍清晰时只记录警告。二维码可解码不能
替代其余 Prime 资产可见性。

同时把每个变体和原候选图、结构化 brief 对照，逐项检查 `source_semantics`、
`information_mechanism`、`visual_anchors`、`palette_anchors` 和 `must_preserve`。获批文案、金融事实、
`information_mechanism` 和 `must_preserve` 是不可变的关键内容：缺失、改写或换成另一件事必须
`QC FAIL`。`visual_anchors` 和 `palette_anchors` 中未被列入 `must_preserve` 的一般风格偏差只记警告；
只有 Issue 中存在用户明确原话时才能放开关键内容，并在 QC 证据中引用。

重新运行二维码机器解码，并把实际解码内容同时与输入快照的 `approved_payload`、
`qr_payload` 和三个 Prime 模板的 `decoded_payload` 比对。任何一个值不同都必须 `QC FAIL`，
不能只拿市场包里同一个自由文本字段自证。向 Issue 发布以 `QC PASS`、
`QC PASS WITH WARNINGS`、`QC FAIL` 或 `needs_input` 开头的中文评论，并附机器 evidence JSON、九宫格和
`qc-hard-region-sheet.png`。为作用域内每张图列出 `checked_region_ids`、`actual_hard_regions_clear`、
`prime_assets_readable`、`key_content_preserved`、`blocking_failures` 和 `quality_warnings`。
`actual_hard_regions_clear` 只判断真实红框，不得包含顶部/底部软引导。缺一张、缺一个字段或只写
“整体无问题”都不得发布 PASS；这些字段是语义验收结论，不得直接复制机器 evidence 的 `passed`。
先给整批摘要，再逐变体、逐尺寸
列出来源图、二维码解码证据、关键内容检查、阻断问题和质量警告。只有阻断问题才给出精确重跑
指令；一个尺寸失败时只要求重开该变体的该尺寸及其后续包装，不得重做其他变体或已通过尺寸。
九张图可以在一次
语义检查中共同读取；批量恢复也必须在一个 QC Issue 中共同读取，但每张必须有独立结论。作用域内
所有图片通过即可置为 `done`；没有成图或
解码器时，必须如实写 `needs_input`，不得声称验收通过。

硬区返工措辞必须限定为空间操作：写“把表格、人物或标题移出/缩放到对应 region ID 的矩形之外，
并在该矩形内改为连续低纹理背景”，不得简写成“移除表格/人物”。返工指令必须明确获批文案、金额、
期限、利率、`information_mechanism`、`must_preserve`、表格、人物和关键卡片继续保留；不得为了清空
硬区删掉关键内容、改变业务语义，或把整张图退化为只剩利益点标题的泛海报。
