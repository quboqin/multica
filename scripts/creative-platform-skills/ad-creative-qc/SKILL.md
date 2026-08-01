---
name: multica-ad-creative-qc
description: "当 Multica Issue 含有待验收广告成图附件，需要进行二维码、可见文案、构图和金融合规终检时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# 广告成图终检

当前运行时只挂载本 Skill，不要查找其他 Skill 的本地文件。确认 Issue 描述或评论中
存在本轮候选 ID、文案记录及版本、市场资源包及版本、源成图附件 ID、服务端 QR 校验
快照和包装脚本 JSON 证据。QR 校验快照必须为 `passed`，并包含三个 Prime 模板各自的机器
解码值。缺少任一必需证据时，发布 `needs_input` 或 `QC FAIL`，不能
用竞品内容补全。

读取完整 Issue 与评论，下载时间线中最新的生成成图附件。本角色不生成图片。

按正常观看比例检查最终像素：原生尺寸、满版构图、视觉平衡、批准文案可读性、完整
Prime 模板的左上品牌/右上条款/底部合规区是否存在且不被遮挡，以及固定槽位的可解码
动态二维码。拒绝二维码框/占位框、整条空白带、人脸/文案/CTA 碰撞、复制竞品品牌或
没有依据的金融承诺。

重新运行二维码机器解码，并把实际解码内容同时与输入快照的 `approved_payload`、
`qr_payload` 和三个 Prime 模板的 `decoded_payload` 比对。任何一个值不同都必须 `QC FAIL`，
不能只拿市场包里同一个自由文本字段自证。向 Issue
发布以 `QC PASS`、`QC FAIL` 或 `needs_input` 开头的中文评论，列出尺寸、来源图、二维码
解码证据、可见文案检查、构图问题和每个失败项的精确重跑指令。只有全量通过时置为
`done`；没有成图或解码器时，必须如实写 `needs_input`，不得声称验收通过。
