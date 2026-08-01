---
name: coordinate-ad-creative-squad
description: "Coordinate an advertising-material squad when a parent Issue needs candidate collection, per-image editing, revision handling, quality review, and final publication. Use for the squad Leader role; choose agents from the live squad roster and their platform Skills instead of following fixed workflow nodes."
---

# 协作广告素材小队

只做判断、委派、验收汇总和发布，不替代专业成员执行工作。

每次被唤醒时：

1. 读取当前 Issue、子 Issue、评论、附件和创意上下文快照。
2. 读取小队名册中每个成员的职责、Skill 名称和 Skill 描述。
3. 判断当前缺少的最小有效证据，选择能力最匹配的成员，并为该专业任务创建一个直接子 Issue。
4. 等待成员回传；存在未解决风险时只返工受影响的图片或尺寸。人工明确接受的非阻断差异
   已经解决，不得因为评论仍保留历史风险文字而阻断后续。
5. 只有验收通过的交付才能发布到父 Issue 结果看板。

没有数据依赖的任务必须并发委派：多张候选图的素材理解相互独立；多张创意工作 Issue 相互
独立；同一创意的生成规格确定后，三个目标尺寸可分别创建图像编辑子 Issue 并发执行。每个尺寸
底图完成后即可进入该尺寸的 Prime 包装与检查，不必等待其他尺寸。只有“简报后选文案”、
“方案后生成”、“包装后终检”和“三尺寸全部通过后发布”保留依赖门槛。并发量服从平台和成员
上限，不在单个 Issue 内用轮询或等待占住执行窗口。

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
Leader 被唤醒后读取已完成的子 Issue，再决定创建下一个专业子 Issue、局部返工或发布。

不要只按 `needs_input`、`approved_with_risk` 等单个词判断证据状态，必须读取该专业子 Issue 的
时间线到最新人工决定。若人工已经接受只涉及连续背景、空白卡片下缘或阴影的 Prime 外围缓冲
区差异，应继续创建 Prime 包装子 Issue，由真实合成图和独立 QC 判断遮挡；这不属于用包装绕过
底图问题。批准文案、金额、按钮、主体或关键卡片内容进入模板实际不透明图文区，才是必须返工
的未解决底图问题。

候选图包含竞品 App 界面时，要求分析成员查看市场资源包中全部 `app_ui_reference` 文件并选择
最匹配的 AdaKami 界面。后续生成不得保留或仿造竞品 App UI。

发布前按市场资源包的 `naming_rule` 统一同一候选的三个尺寸文件名，并确认附件写入快照中的
`parent_issue_id`。详细判断和发布边界见 `references/collaboration-contract.md`。
