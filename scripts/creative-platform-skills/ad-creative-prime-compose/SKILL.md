---
name: multica-ad-creative-prime-compose
description: "当一张素材的三个创意九张底图或精准返工作用域已完成，需要批量叠加 Prime 模板、覆盖动态 QR 并回传机器解码证据时使用。"
allowed-tools: Bash(multica *), Bash(python *)
---

# AdaKami Prime 包装

初次交付时，一个包装 Issue 处理同一素材的 V01-V03 共九张无品牌底图；
`creative_scope=size` 的精准返工只接收 metadata 指定的一张，`creative_scope=variant` 接收该变体
三张；`creative_scope=batch` 接收 metadata 明确列出的本轮 1-9 张变更图。不得要求作用域外尺寸，
也不得重画、改文案、补 Logo 或生成监管/商店资产。

从本次已发布市场资源包快照读取 `prime_square`、`prime_landscape`、`prime_portrait`、
`qr_payload` 和服务端生成的 `qr_validation`。确定性合成工具来自本 Skill 自带的
`references/image_prime_compose.py`，不是市场资源文件。只有 `qr_validation.status=passed`、
三个模板解码记录齐全且 `approved_payload` 与 `qr_payload` 完全一致时才允许执行；否则回传
`needs_input`，不能自行选择网址。用附件 ID 下载，不接受本机固定路径或其他市场的替代文件。
市场资源快照由运行时动态 Skill `creative-issue-resources` 提供。读取该 Skill 的 `SKILL.md` 后，
必须以它所在目录为基准打开同目录下的 `references/issue-resources.json`；不得在任务工作目录中
查找 `references/issue-resources.json`，也不得因为工作目录没有该文件就判定资源缺失。
先使用 Python `Path.mkdir(parents=True, exist_ok=True)` 显式、幂等地创建本 Issue 独立工作目录。
每个变体或 job 必须预先创建独立输入子目录，因为不同附件的原始文件名可能同为
`square.png`、`landscape.png` 或 `portrait.png`；禁止把不同变体下载到同一目录后再靠覆盖结果判断。
确认所有目录存在后，再对作用域内底图、对应模板和校验证据分别执行
`multica attachment download <attachment-id> --output-dir <输入目录>`；运行时支持并行工具调用时，
一次并行发出全部下载，不要让多个下载进程同时负责首次创建同一目录，也不要先查询 attachment
帮助或串行探索命令。
包装 manifest 必须原样写入资源快照中的 `prime_layout_contract`，包括每个尺寸的
`top_key_content_exclusion_end`、`bottom_key_content_exclusion_start`、`hard_regions` 和
`backdrop_rule`；禁止只写资源包 ID 或在本地重新估算坐标。缺少该契约时必须 `needs_input`，因为
后续 QC 无法提供逐图硬区证据。
运行本 Skill 提供的脚本，先叠加完整透明 Prime 资产层，再在模板固有
右上 QR 槽位生成 `qr_validation.approved_payload` 的整数模块二维码，并对最终 PNG 机器解码。
成品解码内容必须同时等于 `approved_payload` 和三个模板的 `decoded_payload`。模板的 Logo、
条款文字、商店徽章、OJK、AFPI、PINDAR 和合规页脚必须原样保留。

把作用域内全部输入写成脚本 batch manifest，并只运行一次
`python references/image_prime_compose.py --manifest <manifest.json>`。初次交付 manifest 必须有九个
job，普通精准返工为一个或三个 job，`creative_scope=batch` 为 metadata 中列出的任意 1-9 个 job。
每个 job 的输出文件名必须按市场包 `naming_rule` 展开并包含 `V01`、`V02` 或 `V03`；执行前必须
确认全部输出路径唯一。规则缺少 `{variant}` 或展开后发生重名时不得运行，应回传配置错误让 Leader
刷新市场包快照，禁止靠顺序覆盖同名文件。
脚本逐图输出状态、QR 坐标、二维码边长和机器解码结果；成功图
必须把标准输出直接保存为 `compose_result.json`。成功图、原始 batch manifest 和
`compose_result.json` 作为同一条评论的附件一次回传；不得只上传一份重新整理的 evidence，也不得
临时编写脚本再次复制这两份 JSON。原生附件记录已经提供最终附件 ID，不需要为了把 ID 抄进另一份
JSON 而增加第二轮加工。评论必须逐图写清变体编号、源图附件 ID、资源包版本、模板文件、最终尺寸、
批准快照版本和解码比对结果。二维码无法解码、与批准快照不一致或尺寸不在契约内时，
只重跑失败 job，已经成功且输入指纹未变化的结果直接复用。全部作用域附件和证据上传成功后把
当前包装 Issue 置为 `done`，让 Leader 创建一个独立 QC Issue；不要停在 `in_review`，包装角色不
代替 QC 审批。
