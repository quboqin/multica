---
name: multica-ad-creative-qc
description: "当一个创意变体的一张或三张最终成图齐备，需要按明确返工作用域进行二维码、可见文案、构图和金融合规终检时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

当前运行时只挂载本 Skill，不要查找其他 Skill 的本地文件。确认 Issue 描述或评论中
存在本轮候选 ID、文案记录及版本、市场资源包及版本、源成图附件 ID、服务端 QR 校验
快照和包装脚本 JSON 证据。QR 校验快照必须为 `passed`，并包含三个 Prime 模板各自的机器
解码值。缺少任一必需证据时，发布 `needs_input` 或 `QC FAIL`，不能
用竞品内容补全。

一个 QC Issue 只验收一个创意变体。初次交付和 `creative_scope=variant` 验收三个尺寸；
`creative_scope=size` 只验收 metadata 指定的一张，不等待或重复检查沿用尺寸。作用域内图片必须
候选 ID、变体编号、文案版本和修订一致。先创建本 Issue 独立工作目录，再对作用域内 PNG 和
JSON 证据分别执行
`multica attachment download <attachment-id> --output-dir <验收目录>`；运行时支持并行工具调用时，
一次并行发出全部下载，不要先查询 attachment 帮助或串行探索命令。
本角色不生成图片。

按正常观看比例检查最终像素：原生尺寸、满版构图、视觉平衡、批准文案可读性、完整
Prime 模板的左上品牌/右上条款/底部合规区是否存在且不被遮挡，以及固定槽位的可解码
动态二维码。拒绝二维码框/占位框、整条空白带、人脸/文案/CTA 碰撞、复制竞品品牌或
没有依据的金融承诺。

重新运行二维码机器解码，并把实际解码内容同时与输入快照的 `approved_payload`、
`qr_payload` 和三个 Prime 模板的 `decoded_payload` 比对。任何一个值不同都必须 `QC FAIL`，
不能只拿市场包里同一个自由文本字段自证。向 Issue
发布以 `QC PASS`、`QC FAIL` 或 `needs_input` 开头的中文评论，逐尺寸列出来源图、二维码
解码证据、可见文案检查、构图问题和每个失败项的精确重跑指令。一个尺寸失败时只要求重开
当前变体的该尺寸及其后续包装，不得重做其他变体或已通过尺寸。作用域内所有图片通过即可置为
`done`；没有成图或解码器时，必须如实写 `needs_input`，不得声称验收通过。
