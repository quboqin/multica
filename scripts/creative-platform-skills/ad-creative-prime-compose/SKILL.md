---
name: multica-ad-creative-prime-compose
description: "当一个创意变体的一张或三张无品牌底图已完成，需要按明确返工作用域叠加对应 Prime 模板、覆盖动态 QR 并回传机器解码证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# AdaKami Prime 包装

一个包装 Issue 只处理一个创意变体。初次交付接收 `1080x1080`、`1200x628`、`800x1000`
三张无品牌底图；`creative_scope=size` 的精准返工只接收 metadata 指定的一张；
`creative_scope=variant` 接收该变体三张。不得要求作用域外尺寸，也不得重画、改文案、补 Logo
或生成监管/商店资产。

从本次已发布市场资源包快照读取 `prime_square`、`prime_landscape`、`prime_portrait`、
`qr_payload` 和服务端生成的 `qr_validation`。确定性合成工具来自本 Skill 自带的
`references/image_prime_compose.py`，不是市场资源文件。只有 `qr_validation.status=passed`、
三个模板解码记录齐全且 `approved_payload` 与 `qr_payload` 完全一致时才允许执行；否则回传
`needs_input`，不能自行选择网址。用附件 ID 下载，不接受本机固定路径或其他市场的替代文件。
先创建本 Issue 独立工作目录，再对作用域内底图、对应模板和校验证据分别执行
`multica attachment download <attachment-id> --output-dir <输入目录>`；运行时支持并行工具调用时，
一次并行发出全部下载，不要先查询 attachment 帮助或串行探索命令。
运行本 Skill 提供的脚本，先叠加完整透明 Prime 资产层，再在模板固有
右上 QR 槽位生成 `qr_validation.approved_payload` 的整数模块二维码，并对最终 PNG 机器解码。
成品解码内容必须同时等于 `approved_payload` 和三个模板的 `decoded_payload`。模板的 Logo、
条款文字、商店徽章、OJK、AFPI、PINDAR 和合规页脚必须原样保留。

对作用域内一个或三个尺寸分别运行脚本。把最终 PNG 与对应脚本 JSON 输出作为附件一次回传；评论必须写清
变体编号、每张源图附件 ID、资源包版本、模板文件、最终尺寸、QR 坐标、二维码边长、批准快照
版本和解码比对结果。二维码无法解码、与批准快照不一致
或尺寸不在契约内时，只重跑此阶段。不得重新包装作用域外已经通过的尺寸。附件和证据上传成功后把当前包装 Issue 置为 `done`，让
Leader 获得子任务完成事件并创建独立 QC Issue；不要停在 `in_review`，包装角色不代替 QC 审批。
