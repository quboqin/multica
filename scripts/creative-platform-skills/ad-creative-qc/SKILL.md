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
  --contact-sheet <qc-contact-sheet.png>
```

脚本路径按运行时实际读取到的 `SKILL.md` 目录解析。原始 Prime 模板可以高于交付分辨率，包装脚本
会按目标画布缩放；QC 不得因为模板原文件尺寸与最终图不同而误报失败。机器工具不替代后续语义、
文案和遮挡人工判断。

机器检查通过后，先用脚本生成的九宫格接触表在原始分辨率做整批语义和构图复核。只有某个单元格
存在文字、遮挡、对比度或数值歧义时，才单独打开对应原图；不得固定逐张再次打开九个文件。候选
锚点优先读取本 Issue 已注入的资源快照 `selected_item.creative_brief`，不要重复下载 30KB 以上的
完整方案附件。机器 evidence、九宫格和逐图评论已经构成完整验收证据，不要再写脚本合并出一份
重复的 `qc_evidence_final.json`。

按正常观看比例检查最终像素：原生尺寸、满版构图、视觉平衡、批准文案可读性、完整
Prime 模板的左上品牌/右上条款/底部合规区是否存在且不被遮挡，以及固定槽位的可解码
动态二维码。拒绝二维码框/占位框、整条空白带、人脸/文案/CTA 碰撞、复制竞品品牌或
没有依据的金融承诺。

同时执行市场包 `prime_layout_contract.backdrop_rule`：Logo、右上条款和底部监管资产虽然存在但因
同色或深色背景而难以正常阅读时仍然 `QC FAIL`。二维码可解码不能替代其余 Prime 资产可见性。

同时把每个变体和原候选图、结构化 brief 对照，逐项检查 `source_semantics`、
`information_mechanism`、`visual_anchors`、`palette_anchors` 和 `must_preserve`。默认必须保持同一
业务场景、同一信息机制和同一主色家族；版式和视觉处理可以变化。若变体把还款计划改成家庭
预算、把表格机制改成无关人物海报、或未经许可改变主色家族，必须 `QC FAIL`，即使文案和 QR
均正确。只有 Issue 中存在用户明确放开该锚点的原话时才允许通过，并在 QC 证据中引用。

重新运行二维码机器解码，并把实际解码内容同时与输入快照的 `approved_payload`、
`qr_payload` 和三个 Prime 模板的 `decoded_payload` 比对。任何一个值不同都必须 `QC FAIL`，
不能只拿市场包里同一个自由文本字段自证。向 Issue
发布以 `QC PASS`、`QC FAIL` 或 `needs_input` 开头的中文评论，并附机器 evidence JSON 和九宫格。
先给整批摘要，再逐变体、逐尺寸
列出来源图、二维码解码证据、可见文案检查、构图问题和每个失败项的精确重跑指令。一个尺寸失败
时只要求重开该变体的该尺寸及其后续包装，不得重做其他变体或已通过尺寸。九张图可以在一次
语义检查中共同读取；批量恢复也必须在一个 QC Issue 中共同读取，但每张必须有独立结论。作用域内
所有图片通过即可置为 `done`；没有成图或
解码器时，必须如实写 `needs_input`，不得声称验收通过。
