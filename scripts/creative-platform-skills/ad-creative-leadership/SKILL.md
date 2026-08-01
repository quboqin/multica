---
name: coordinate-ad-creative-squad
description: "Coordinate an advertising-material squad when each selected image must produce three distinct creative variants in three native sizes, with visible child-Issue delegation, scoped revision, quality review, and final publication."
---

# 协作广告素材小队

只做判断、委派、验收汇总和发布，不替代专业成员执行工作。

每次被唤醒时：

1. 读取当前 Issue、直接子 Issue、评论、附件和创意上下文快照。直接子 Issue 必须使用
   `multica issue children <当前 Issue ID> --output json` 一次获取；不要拉取工作区全量 Issue 后
   本地翻页筛选。
2. 读取小队名册中每个成员的职责、Skill 名称和 Skill 描述。
3. 判断所有已经满足依赖但尚不存在的专业任务，选择能力最匹配的成员，并在本次唤醒中把这些
   任务全部创建为直接子 Issue。禁止一次只派一个已就绪任务。
4. 创建完本轮任务后立即结束执行，由子 Issue 完成事件再次唤醒；不得轮询、休眠或占住执行槽。
   存在未解决风险时只返工受影响的变体和尺寸。人工明确接受的非阻断差异
   已经解决，不得因为评论仍保留历史风险文字而阻断后续。
5. 只有 V01-V03 各三个尺寸全部验收通过，才能把九张交付发布到父 Issue 结果看板。

当前 Issue 的 `metadata.workflow=creative_adjustment` 时，不重新执行完整九图流程。以 metadata 中的
`creative_adjustment_id`、`creative_candidate_id`、`creative_variant`、`creative_scope`、
`creative_size`、`creative_revision`、目标成图附件和无品牌底图附件为唯一作用域：

- `creative_scope=size`：只处理指定变体的指定尺寸；
- `creative_scope=variant`：只处理指定变体的三个尺寸；
- 其他变体和未选尺寸直接沿用，不创建任务、不重新包装、不重新验收。

先判断反馈需要修改无品牌画面，还是只涉及 Prime/QR 确定性包装。需要修改画面时，只为作用域内
尺寸创建图像编辑子 Issue，并要求把上一版无品牌底图作为第一图像输入；不得把带 Prime、Logo、
条款和二维码的最终成图作为模型第一输入。随后只对作用域内一张或三张图执行 Prime 和 QC。

没有数据依赖的任务必须并发委派：多张候选图的素材理解相互独立；多张创意工作 Issue 相互
独立。每张候选图固定交付 `V01`、`V02`、`V03` 三个差异明确的创意变体，每个变体固定交付
`1080x1080`、`1200x628`、`800x1000` 三个原生尺寸，共九张图。方案完成后，一次创建三个
`图像编辑 · V0X · 1080x1080 · rN` 母版子 Issue。某个母版通过后，一次创建该变体的横版与
竖版重排子 Issue；两者都把已通过母版作为第一图像输入。禁止裁切、加边或拉伸母版。

初次生产中，同一变体三张底图齐备后，创建一个 `Prime 包装 · V0X · 三尺寸 · rN` 子 Issue
批量包装；包装完成后创建一个 `广告验收 · V0X · 三尺寸 · rN` 子 Issue 批量验收。精准返工按
metadata 作用域创建单尺寸或三尺寸 Prime/QC Issue。三个变体的包装和 QC 互相
独立，可以并行。只有“简报后选文案”、“方案后三个母版”、“母版后同变体两个重排”、
“同变体三底图后包装”、“包装后终检”和“九张全部通过后发布”保留依赖门槛。图像编辑成员
最多五路并发，其余成员按平台配置执行；Leader 不在单个 Issue 内轮询或等待。

当前 Issue 若是候选池页面发起的“素材理解”请求，只委派参考分析成员读取真实图片并把结构化
创意简报回写目标候选池，不启动图像编辑、包装或 QC。创意简报必须把视觉主题与主利益点分开；
标题、标签和采集元数据不能替代像素证据。

父 Issue 只保留候选池、结果看板和用户必须知道的结论。每张入选素材建立独立创意工作
Issue；专业讨论、提示词、模型调用证据、包装和返工都留在对应子 Issue。多张素材相互独立时
可以并行委派，但不能把不同素材的文案、资源和证据混在一起。

专业任务一律通过“新建直接子 Issue + 分配给目标智能体 + `todo` 状态”触发。禁止在当前创意
工作 Issue 里通过 `@mention` 委派成员，也禁止让专业成员直接在当前创意工作 Issue 上执行。
Leader 只在当前 Issue 留一条不含成员 mention 的子 Issue 链接和必要结论。子 Issue 标题按
“能力 · 对象 · 修订”命名，描述必须带回当前创意工作 Issue、最外层候选池 Issue、候选 ID、
创意简报、文案版本和上下文快照引用。

专业成员完成证据后直接把专业子 Issue 置为 `done`，不要再向父 Issue 发评论或 `@mention`。
平台会自动在直接父 Issue 生成一条最小完成回执并唤醒小队，手工回执会造成 Leader 重复执行。
Leader 被唤醒后先读取所有直接子 Issue，再批量创建所有新近就绪的专业子 Issue、局部返工或发布。
创建前按“候选 ID + 修订 + V01/V02/V03 + 尺寸 + 能力”检查已有标题、描述和附件，保证重复
唤醒不会重复委派、重复调用模型或重复发布。

不要只按 `needs_input`、`approved_with_risk` 等单个词判断证据状态，必须读取该专业子 Issue 的
时间线到最新人工决定。若人工已经接受只涉及连续背景、空白卡片下缘或阴影的 Prime 外围缓冲
区差异，应继续创建 Prime 包装子 Issue，由真实合成图和独立 QC 判断遮挡；这不属于用包装绕过
底图问题。批准文案、金额、按钮、主体或关键卡片内容进入模板实际不透明图文区，才是必须返工
的未解决底图问题。

候选图包含竞品 App 界面时，要求分析成员查看市场资源包中全部 `app_ui_reference` 文件并选择
最匹配的 AdaKami 界面。后续生成不得保留或仿造竞品 App UI。

发布前按市场资源包的 `naming_rule` 统一同一候选的九张文件名。文件名必须包含 `V01`、`V02`
或 `V03`，同一变体的三个尺寸使用相同前缀，并确认附件写入快照中的 `parent_issue_id`。
附件上传后必须生成交付 manifest，并执行
`multica creative delivery register <父 Issue ID> --input-file <manifest.json> --output json`。
结果看板以该登记记录关联竞品原图、变体、尺寸、修订、底图和 QC 证据；评论文案不承担关联
职责。精准返工只登记本轮通过的一张或三张新成图，旧交付记录和附件必须保留。
详细判断和发布边界见 `references/collaboration-contract.md`。
