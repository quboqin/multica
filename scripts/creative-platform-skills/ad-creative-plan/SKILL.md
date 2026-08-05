---
name: multica-ad-creative-plan
description: "为 Creative Order 的候选素材规划 V01-V03 创意变体及三尺寸原生重排，并写入 native variant 领域记录时使用。"
allowed-tools: Bash(multica *)
---

# 广告创意方案

只使用 native Creative Order。开始时执行：

```bash
multica creative order get <order-id> --output json
```

Planner 的读取命令是封闭白名单：除上面的 `creative order get` 外，仅在订单只保存
`source_analysis_id`、未内嵌对应分析结果时执行下文的 `creative source-analysis list`。不得调用不存在的
`multica creative material get`，也不得读取面向旧 Issue 流程的 `creative materials`、`creative material
download` 或 `creative context`。Planner 不读取原始图片像素；后续 Producer 使用 candidate ID 通过
`creative library download` 获取归档原图。若订单或指定 Source Analysis 缺少规划必需字段，直接按下文
写入 `action_required`，不要通过 `--help` 猜测其他素材读取命令。

从目标 `item` 的 `copy_snapshot`、`direction`、关联 source analysis 与 order `input_snapshot` 读取获批文案、
市场资源、用户方向和结构化锚点。`copy_snapshot` 是页面确认阶段已经推荐、选择或人工编辑后冻结的结果，
也是金融数字、期限、利率和可见文案的唯一真值。Planner 不再次搜索文案库、不推荐、不替换、不拼接
文案；竞品 `benefit_value`、`detected_text` 和分析证据只可作为创意观察，绝不成为展示事实。快照缺少表达
主利益点的必要内容时，在变体 `brief` 中写明 `needs_input` 和缺失字段，并用 `variant-put` 写入
`action_required`；不得猜测或补造内容。

当 `copy_snapshot.schema_version=2` 时，逐字使用已组装的 `headline`、`subheadline`、`benefit`、
`supporting`、`cta` 和 `legal_text`。`fragments` 只用于说明组合来源，`product_facts` 用于核对已冻结事实，
`recommendation.reasons` 只解释页面为何推荐；三者都不得触发重新组装。不得读取文案库当前草稿或最新
发布版本替换快照，即使订单创建后业务已发布新版本。

参考素材的信息机制必须先经过“获批文案可表达性”检查。竞品中的问题、选项、标签、按钮或步骤若没有
`copy_snapshot` 中的已批准对应文本，只能作为版式关系和阅读路径的灵感，不能进入 `must_preserve`、
`key_content_preserved` 或可见文案要求。此时把机制转译为当前快照能够承载的结构，例如用获批标题承担
入口、用已批准利益点组织卡片或信息条，并在 brief 记录 `mechanism_adaptation` 和
`omitted_unapproved_copy`。不得为保留竞品机制而生成空选项、重复金融字段冒充选项，或把缺少竞品原文
误判为用户输入不足。只有 `copy_snapshot` 本身缺少其承诺的核心利益事实时才进入 `needs_input`。

市场规则的唯一来源是订单 `input_snapshot` 中冻结的已发布市场资源包 ID、版本、结构化 config 与附件
ID。结构化 config 决定尺寸、文案库、QR、Prime 布局和合规规则；版本化附件提供实际 Prime、App UI 和
品牌文件；`brand_guideline` Markdown 只做补充说明，不能覆盖结构化字段。不得读取本机 profile、固定
路径或未冻结市场包的最新版本。冻结快照内部若互相冲突，写明冲突字段并进入 `action_required`，不自行
选择一套规则。

订单详情只保存 `source_analysis_id`、未内嵌对应分析结果时，按候选读取原始分析结果：

```bash
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

只使用与 `source_analysis_id` 和版本一致的 completed 记录，不得退回标题、标签或评论猜测分析结论。

当 Source Analysis 的 `app_ui_detected=true` 时，由 Planner 在冻结市场附件中筛选
`role=app_ui_reference`，根据 `app_ui_type`、附件 tags、页面结构和所需画布选择最匹配的品牌 UI。把附件
ID、匹配依据和替换要求写入 Variant brief；不得沿用竞品 UI，也不得让分析智能体预选品牌附件。若参考
图不含 App UI，则不选；若明确需要替换但冻结市场包没有兼容附件，写 `needs_input`，不从网络或本机补图。

每个目标 item 固定规划 `V01`、`V02`、`V03`。它们必须保留与获批文案兼容的 `source_semantics`、
`information_mechanism`、`visual_anchors`、`palette_anchors`、`must_preserve`、获批文案和合规边界；
差异来自版式骨架、信息组织、关键卡片形态或视觉处理，而不是改换业务场景或主色家族。每个标准生产
变体有同一内容族的三个 `expected_sizes`：`1080x1080`、`1200x628`、`800x1000`。方形是该变体母版，横竖版都是原生重排，
保留同一主体身份、文案和信息层级，不得裁切、加边、拉伸或重新发明另一套创意。

每个 `brief` 至少包含：

- `candidate_id`、来源素材 candidate ID、`copy_snapshot` ID/版本和冻结资源包 ID/版本；
- `copy_adaptation`、完整原样提示词、禁止元素和必保元素；
- `mechanism_adaptation` 和 `omitted_unapproved_copy`，明确哪些参考机制被获批文案承载、哪些竞品文字未进入
  成图；
- V01-V03 各自的 `anchor_retention` 与三个尺寸的生成任务；
- 每个尺寸的模型画布、目标尺寸、第一图像输入、内容一致性要求；
- 原样的 `prime_layout_contract`，包括 `hard_regions` 与 `backdrop_rule`；
- 每个 `hard_regions[].id` 的 `prime_clearance_checklist`，以及需要替换 App UI 时由本角色选择的
  `selected_app_ui_reference_ids` 和选择依据。

`hard_regions` 是后续 Prime 实际资产的坐标合同。布局时让标题、获批金融数字、人脸、按钮、表格、
关键卡片和正文避开真实矩形；矩形内要求连续、低纹理背景且不生成二维码、Logo、商店徽章、监管标识或
合规页脚。顶部/底部上下文范围只是软引导，不能扩大为整条硬带。原图已有的人物、手臂、模型、装饰或
几何位置进入矩形，不是方案失败条件，也不得为了清空矩形删改关键内容；真实遮挡只由合成后的 QC 判断。

方形、横版、竖版的依赖必须写为：横版和竖版仅依赖同变体方形底图完成，方形完成后二者立即并发；
不得等待 Prime 或 QC，也不得跨变体等待。

用每个 item 的真实 `id` 写入三个变体：

```bash
multica creative order variant-put <order-id> --input-file <V01.json> --output json
```

`V01.json`、`V02.json`、`V03.json` 均为 JSON 对象，至少含：

```json
{
  "order_item_id": "<item-id>",
  "variant_key": "V01",
  "brief": { "...": "完整结构化生成规格" },
  "status": "queued"
}
```

写入后以 `creative order get` 返回的 variant ID 为后续生产、Prime 和 QC 的唯一关联键。不要用会话状态、
task context 或外部表保存交付状态。

三个变体写入成功后，从当前 task context 或订单冻结的 squad snapshot 读取 `producer_agent_id`、
`prime_agent_id` 和 `reviewer_agent_id`。先查询该 Order Item 已有的生产 task：

```bash
multica task by-source list --agent <producer-agent-id> \
  --kind creative_order_item_production --ref <order-item-id> --output json
```

逐一按 `source + item_key` 检查三个变体。当某个 `<variant-id>:r<revision>` 没有 active/succeeded task，
且该 revision 还没有完整 generated assets 时，把它加入 manifest；一个 source 下已有其他 variant task
不能阻止缺失项继续委派。每项 `item_key` 为 `<variant-id>:r<revision>`，`context.type` 固定为
`creative_domain_task`，并携带 `workflow: creative_production`、从当前 task 原样复制的 `issue_id`、
`leader_agent_id`、order/item/variant/candidate ID、revision、
`expected_sizes`、`prime_agent_id` 和 `reviewer_agent_id`。执行后立即结束，不等待生产结果：

把 V01-V03 中所有就绪项放入同一个 manifest，一次 fanout；不得按变体串行调用三次，也不得等待某个
production task 完成后才提交兄弟变体。

`production-manifest.json` 的外层来源必须固定引用当前真实 Order Item。不要生成随机 UUID，不要借用
`creative_crawl_run`、`creative_crawl_run_analysis` 或订单自身的 trigger evidence；这些来源无法被后续
生产失败恢复和幂等查询正确识别。manifest 结构如下，其中每个 Variant 仅通过 `item_key` 和 context
区分：

```json
{
  "trigger_evidence_kind": "creative_order_item_production",
  "trigger_evidence_ref_id": "<order-item-id>",
  "items": [
    {
      "item_key": "<variant-id>:r<revision>",
      "context": {
        "type": "creative_domain_task",
        "workflow": "creative_production",
        "issue_id": "<issue-id>",
        "leader_agent_id": "<leader-agent-id>",
        "creative_order_id": "<order-id>",
        "creative_order_item_id": "<order-item-id>",
        "variant_id": "<variant-id>",
        "candidate_id": "<candidate-id>",
        "revision": 1,
        "expected_sizes": ["1080x1080", "1200x628", "800x1000"],
        "prime_agent_id": "<prime-agent-id>",
        "reviewer_agent_id": "<reviewer-agent-id>"
      }
    }
  ]
}
```

写文件后先用 JSON 解析器校验，再调用 fanout。CLI 拒绝时按返回的字段错误修正同一文件；只要没有返回
已创建 task，就不把本轮描述为“已入队”。

```bash
multica task fanout --agent <producer-agent-id> --input-file <production-manifest.json> --output json
```
