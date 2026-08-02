---
name: coordinate-ad-creative-squad
description: "Coordinate an advertising-material squad when each selected image must produce three distinct creative variants in three native sizes, with visible child-Issue delegation, scoped revision, quality review, and final publication."
---

# 协作广告素材小队

只做判断、委派、验收汇总和发布，不替代专业成员执行工作。

每次被唤醒时：

1. 读取当前 Issue、直接子 Issue、评论、附件和创意上下文快照。直接子 Issue 必须使用
   `multica issue children <当前 Issue ID> --compact --output json` 一次获取；不要拉取工作区全量 Issue
   后本地翻页筛选。compact 结果足够做依赖和去重判断；只对本轮真正需要交接或验收的专业子 Issue
   再调用 `issue get` 和 `issue comment list`，不要重复读取所有已完成兄弟的长描述和附件。
2. 读取小队名册中每个成员的职责、Skill 名称和 Skill 描述。
3. 判断所有已经满足依赖但尚不存在的专业任务，选择能力最匹配的成员，并在本次唤醒中把这些
   任务全部创建为直接子 Issue。两个及以上任务必须写入 manifest 并使用
   `multica issue create-batch --input-file <manifest.json> --output json` 一次委派；禁止一次只派一个
   已就绪任务。
4. 创建完本轮任务后立即结束执行，由子 Issue 完成事件再次唤醒；不得轮询、休眠或占住执行槽。
   存在未解决风险时只返工受影响的变体和尺寸。人工明确接受的非阻断差异
   已经解决，不得因为评论仍保留历史风险文字而阻断后续。
5. 只有 V01-V03 各三个尺寸全部验收通过，才能把九张交付发布到父 Issue 结果看板。

初次生产的方案依赖是硬门槛。若当前创意工作 Issue 下不存在状态为 `done` 的
`metadata.workflow=creative_plan` 子 Issue，且该子 Issue 没有交付方案附件，则本轮只能创建一个
方案子 Issue并立即结束；不得把工作 Issue 描述、用户文案或 Leader 自己的推断当作已完成方案，
也不得提前创建任何图像编辑 Issue。只有方案附件已经完成，才允许把其附件 ID 和
`plan_issue_id` 写入三个图像编辑 Issue 并批量委派。

批量委派 manifest 使用共享父 Issue、状态和负责人，再给每个任务独立标题、描述与幂等 metadata：

```json
{
  "max_concurrency": 3,
  "defaults": {
    "parent": "ADC-123",
    "status": "todo",
    "priority": "high",
    "assignee": "图像编辑智能体"
  },
  "issues": [
    {
      "key": "v01",
      "title": "图像编辑 · V01 · 三尺寸 · r1",
      "description": "父 Issue、候选、简报、文案与快照引用",
      "metadata": {"workflow": "creative_production", "variant": "V01", "revision": 1}
    },
    {
      "key": "v02",
      "title": "图像编辑 · V02 · 三尺寸 · r1",
      "description": "父 Issue、候选、简报、文案与快照引用",
      "metadata": {"workflow": "creative_production", "variant": "V02", "revision": 1}
    }
  ]
}
```

实际初次生产必须包含 V01、V02、V03 三项。命令返回部分失败时，只针对 `failed` 项补建；已经
返回 `created` 的 Issue 不得重建。

当前 Issue 的 `metadata.workflow=creative_adjustment` 时，不重新执行完整九图流程。以 metadata 中的
`creative_adjustment_id`、`creative_candidate_id`、`creative_variant`、`creative_scope`、
`creative_size`、`creative_revision`、目标成图附件和无品牌底图附件为唯一作用域：

- `creative_scope=size`：只处理指定变体的指定尺寸；
- `creative_scope=variant`：只处理指定变体的三个尺寸；
- `creative_scope=variant_subset`：只处理指定变体的 `affected_sizes`，用于一次 QC 中同一变体有多个
  尺寸失败的批量恢复；
- `creative_scope=batch`：只用于恢复后的 Prime/QC，处理 metadata 明确列出的 1-9 张变更图；
- 其他变体和未选尺寸直接沿用，不创建任务、不重新包装、不重新验收。

若同时存在 `creative_adjustment_mode=replan`，先委派方案成员只重做 metadata 指定的变体方案，
再委派图像编辑。该模式必须以原候选图和 brief 锚点为起点，不把上一版无品牌底图作为第一输入；
未指定变体保持原修订。普通精准调整仍以上一版无品牌底图为第一输入。用户一次自然语言反馈
命中多个变体时，平台会为每个变体建立独立直接子 Issue；这些 Issue 相互独立，应并发推进。

先判断反馈需要修改无品牌画面，还是只涉及 Prime/QR 确定性包装。需要修改画面时，只为作用域内
尺寸创建图像编辑子 Issue，并要求把上一版无品牌底图作为第一图像输入；不得把带 Prime、Logo、
条款和二维码的最终成图作为模型第一输入。随后只对作用域内一张或三张图执行 Prime 和 QC。

没有数据依赖的任务必须并发委派：多张候选图的素材理解相互独立；多张创意工作 Issue 相互
独立。每张候选图固定交付 `V01`、`V02`、`V03` 三个差异明确的创意变体，每个变体固定交付
`1080x1080`、`1200x628`、`800x1000` 三个原生尺寸，共九张图。方案完成后，用一个 batch manifest
一次创建三个 `图像编辑 · V0X · 三尺寸 · rN` 子 Issue。每个变体 Issue 内部先生成并检查方形
母版，再由同一智能体使用 `image edit-batch` 同时生成横版和竖版；Leader 不再为三个尺寸创建
额外子 Issue，也不介入变体内部两波执行。

初次生产中，V01-V03 三个变体 Issue 全部交付三尺寸底图后，只创建一个
`Prime 包装 · V01-V03 · 九图 · rN` 子 Issue批量包装；包装完成后只创建一个
`广告验收 · V01-V03 · 九图 · rN` 子 Issue 批量验收。精准返工继续按 metadata 作用域创建单尺寸
或单变体三尺寸 Prime/QC Issue；同一轮 QC 的成图缺陷恢复则按下述分组规则创建一个批量 Prime/QC。
只有“简报后选文案”、“方案后三个变体”、“九张底图后包装”、
“包装后终检”和“终检通过后发布”保留依赖门槛。图片实际请求由平台跨进程槽位限制为五路；
Leader 不在单个 Issue 内轮询或等待。

Prime 子 Issue 必须把原始包装 manifest、`compose_result.json` 和作用域内最终图一起交付。创建 QC
Issue 时直接引用这两份附件 ID 和全部最终图附件 ID，并把资源快照中的 `creative_brief` 锚点摘要
写入描述；不得要求 QC 从另一份包装 evidence 重建输入，也不得要求它重新下载完整方案 Markdown。
QC Issue metadata 必须同时写入 `prime_issue_id`、`manifest_attachment_id` 和
`compose_result_attachment_id`。同一 Prime Issue 和同一对证据附件只允许一个未取消的 QC Issue；
无论它是 todo、in_progress、blocked 还是 done，都不得再建第二个。若 QC 失败原因仅为工具或证据
schema 不兼容，且结论明确说明成图无需重做，则修复 Skill 后在原 QC Issue 追加评论触发复验，不能
新建“复验”Issue。只有 QC 已指出具体成图缺陷并且受影响图片完成新修订后，才能创建引用新 Prime
证据的新 QC Issue。

当前 Issue 若是候选池页面发起的“素材理解”请求，只委派参考分析成员读取真实图片并把结构化
创意简报回写目标候选池，不启动图像编辑、包装或 QC。创意简报必须把视觉主题与主利益点分开；
标题、标签和采集元数据不能替代像素证据。分析还必须固化业务语义、信息机制、视觉锚点、色系
锚点、必须保留项和允许变化项，供策划、生成和 QC 使用。

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
创建前按“候选 ID + 修订 + V01/V02/V03 + 作用域 + 能力”检查已有标题、描述和附件，保证重复
唤醒不会重复委派、重复调用模型或重复发布。

状态为 `cancelled` 的子 Issue 不满足任何依赖，也不占用幂等键。特别是没有 `plan_issue_id` 的
已取消图像编辑 Issue 属于无效委派；方案完成后必须使用真实 `plan_issue_id` 创建新的 V01-V03
任务，不能复用、续跑或等待这些无效 Issue。

只有 Prime 包装后的真实合成图或独立 QC 明确证明品牌、条款、二维码、商店徽章、监管资产不可读，
或批准文案/关键内容被实际遮挡时，才触发同尺寸“安全区恢复”。不得根据无品牌底图进入顶部或底部
合并避让带的坐标推断成图失败。恢复任务沿用原候选、获批文案、变体骨架和无品牌底图，精确修复
已证实的真实冲突，不改变业务语义、信息机制、色系或文案。原 Issue 和附件保留用于审计。每个
“变体 + 尺寸 + 修订”最多自动恢复一次；恢复仍失败才向用户报告精确冲突并等待决定。

同一个 QC Issue 报告多张真实成图缺陷时，先按变体聚合，禁止逐尺寸创建一串恢复 Issue。使用一次
`issue create-batch`，每个受影响变体最多创建一个图像编辑恢复 Issue：该变体三个尺寸全失败时使用
`creative_scope=variant`；只失败部分尺寸时使用 `creative_scope=variant_subset` 并写入
`affected_sizes`。每个尺寸都必须写入上一版无品牌底图附件 ID 和 QC 的实际碰撞证据。所有恢复
Issue 相互独立并发执行；全部完成后只创建一个 `creative_scope=batch` 的恢复 Prime Issue，统一包装
本轮 1-9 张变更图，再只创建一个引用同一 manifest/compose result 的恢复 QC Issue。先前 QC 已通过
且未变化的尺寸直接沿用，不重新包装、不重新验收。恢复 QC 通过后，一次登记新修订与沿用结果，
保证结果看板仍是一套完整九图。

若旧版图像编辑或安全区恢复 Issue 已产生可读、满版、无竞品品牌和无模型二维码的方形底图，但只因
后续尺寸缺失而阻断，Leader 应复用最佳方形附件，创建一个
`metadata.workflow=creative_production_continuation`、`scope=dependent_sizes` 的续跑 Issue。metadata
必须写入 `base_attachment_id`、`missing_sizes` 和 `accepted_size_attachment_ids`：只缺一项就只补
该尺寸，两项都缺才并发生成横版和竖版；不得重做方形或已通过尺寸。三张底图齐备后照常进入九图
Prime。只有机器规范化脚本失败或画面自身存在实际问题才算尺寸失败；原始 PNG 像素与请求值不完全
相等不能覆盖脚本已经通过的比例结论。

不要只按 `needs_input`、`approved_with_risk` 等单个词判断证据状态，必须读取该专业子 Issue 的
时间线到最新人工决定。无品牌底图中的避让带差异应继续进入 Prime 包装，由真实合成图和独立 QC
判断遮挡；这不属于用包装绕过底图问题。只有真实 Prime 成图已经证明资产不可读或关键内容被遮挡，
才是必须返工的未解决问题。

候选图包含竞品 App 界面时，要求分析成员查看市场资源包中全部 `app_ui_reference` 文件并选择
最匹配的 AdaKami 界面。后续生成不得保留或仿造竞品 App UI。

发布前按市场资源包的 `naming_rule` 统一同一候选的九张文件名。文件名必须包含 `V01`、`V02`
或 `V03`，同一变体的三个尺寸使用相同前缀，并确认附件写入快照中的 `parent_issue_id`。
附件上传后必须生成交付 manifest，并执行
`multica creative delivery register <父 Issue ID> --input-file <manifest.json> --output json`。
结果看板以该登记记录关联竞品原图、变体、尺寸、修订、底图和 QC 证据；评论文案不承担关联
职责。精准返工只登记本轮通过的一张或三张新成图，旧交付记录和附件必须保留。

恢复批次发布前必须读取 `multica creative materials <父 Issue ID> --output json` 的 `deliveries`，构造
当前应交付的完整“候选 + V01-V03 + 三尺寸”九键集合。所有变更图必须登记；所有沿用图若已有
对应交付记录则复用，若仅在早期 QC 中通过但从未发布登记，必须把本次发布到父 Issue 的新附件 ID、
原底图、原 Prime 证据和原 QC Issue 一并加入同一个登记 manifest，不能假设历史记录存在。登记后
重新读取并验证九键全部可追溯，缺一项都不能宣称发布完成。最后执行工作 Issue `status done`，再用
`issue get` 验证实际状态；文字回复不能代替落库结果。
详细判断和发布边界见 `references/collaboration-contract.md`。
