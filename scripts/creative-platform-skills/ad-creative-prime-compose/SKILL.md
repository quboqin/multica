---
name: multica-ad-creative-prime-compose
description: "对固定尺寸的 AdaKami 无品牌底图确定性叠加完整 Prime 模板、覆盖动态 QR 并回传机器解码证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# AdaKami Prime 包装

读取当前 Issue 的运行快照和图像编辑任务附件。只接受
`1080x1080`、`1200x628` 或 `800x1000` 的无品牌底图；不得重画、改文案、补 Logo
或生成监管/商店资产。

从本次已发布市场资源包快照读取 `prime_square`、`prime_landscape`、`prime_portrait`、
`qr_payload` 和服务端生成的 `qr_validation`。确定性合成工具来自本 Skill 自带的
`references/image_prime_compose.py`，不是市场资源文件。只有 `qr_validation.status=passed`、
三个模板解码记录齐全且 `approved_payload` 与 `qr_payload` 完全一致时才允许执行；否则回传
`needs_input`，不能自行选择网址。用附件 ID 下载，不接受本机固定路径或其他市场的替代文件。
运行本 Skill 提供的脚本，先叠加完整透明 Prime 资产层，再在模板固有
右上 QR 槽位生成 `qr_validation.approved_payload` 的整数模块二维码，并对最终 PNG 机器解码。
成品解码内容必须同时等于 `approved_payload` 和三个模板的 `decoded_payload`。模板的 Logo、
条款文字、商店徽章、OJK、AFPI、PINDAR 和合规页脚必须原样保留。

把最终 PNG 与脚本 JSON 输出作为附件回传；评论必须写清源图附件 ID、资源包版本、模板文件、最终
尺寸、QR 坐标、二维码边长、批准快照版本和解码比对结果。二维码无法解码、与批准快照不一致
或尺寸不在契约内时，只重跑此阶段。附件和证据上传成功后把当前包装 Issue 置为 `done`，让
Leader 获得子任务完成事件并创建独立 QC Issue；不要停在 `in_review`，包装角色不代替 QC 审批。
