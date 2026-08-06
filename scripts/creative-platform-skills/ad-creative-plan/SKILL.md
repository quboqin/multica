---
name: multica-ad-creative-plan
description: "当 Creative Order Item 已冻结参考分析、业务选择、文案快照和市场资源快照，需要规划 V01-V03 并委派标准生产时使用。"
allowed-tools: Bash(multica *)
---

# 广告生成方案

只处理 task context 指定的 Order、Order Item、candidate 和 revision。

```text
multica creative order get <order-id> --output json
```

订单只保存 `source_analysis_id` 而未内嵌结果时，允许按 candidate 回读一次：

```text
multica creative source-analysis list --candidate-id <candidate-id> --output json
```

除此之外不探测素材命令，不读旧 Issue 流程，不从标题、评论或本机文件补输入。

## 输入真值

- `copy_snapshot`：页面推荐、选择或人工编辑后冻结的唯一可见文案与金融事实；不得重选、拼接或改写。
- Source Analysis：只提供业务语义、信息机制、视觉锚点和通用 App UI 类型；竞品文字不是事实。
- order `input_snapshot`：冻结的市场资源包 ID/version/config/file IDs、用户方向和 squad snapshot。
- 结构化市场 config 是尺寸、Prime、QR、布局和合规规则的真值；附件提供实际品牌与 App UI 文件。

不得读取可变的最新文案库或市场包。冻结输入缺失、版本错配或冲突时，将目标 Variant 写成
`action_required`，在 brief 的 `needs_input` 列出字段；不得猜默认市场或业务事实。

## 方案合同

为每个 item 固定写入 `V01`、`V02`、`V03`。三者共享批准文案、业务语义、主体、主色家族和信息层级，
只改变版式骨架、信息组织或视觉处理。标准生产的 `expected_sizes` 来自冻结订单，当前合同为
`1080x1080`、`1200x628`、`800x1000`；横竖版依赖同变体方形母版，不依赖 Prime/QC，也不跨变体等待。

参考机制中没有批准对应文本的选项、问题、标签或按钮只能转译为结构关系，写入
`mechanism_adaptation` 和 `omitted_unapproved_copy`，不得生成空选项或重复金融字段。需要品牌 App UI 时，
只从冻结 `app_ui_reference` 附件按分析类型和 tags 选择，并记录 attachment ID 与理由。

每个 brief 至少包含：candidate/source-analysis/copy/market snapshot identity，完整批准文案，
`must_preserve`、`allowed_variations`、`copy_adaptation`、`mechanism_adaptation`、禁用元素、App UI 选择、
三个尺寸规格，以及从冻结市场 config 原样复制的 `prime_layout_contract`。hard region 只指导布局；原图主体
进入几何区域不是方案失败，最终以 Prime 成图 QC 为准。

```json
{
  "order_item_id": "<item-id>",
  "variant_key": "V01",
  "brief": {"expected_sizes": ["<size>"]},
  "status": "queued"
}
```

```text
multica creative order variant-put <order-id> --input-file <variant.json> --output json
```

## 委派生产

三个 Variant 均写入后，从冻结 squad snapshot 或当前 task context 读取 `producer_agent_id`、
`prime_agent_id`、`reviewer_agent_id`。先查询 producer 的
`creative_order_item_production` source，逐个比较 `<variant-id>:r<revision>`；active/succeeded task 或完整
generated assets 已存在时跳过该 item。一个 source 下的兄弟 item 不能阻止缺失项。

将全部缺失 Variant 放进同一个 manifest：source kind 为 `creative_order_item_production`，ref 为真实
Order Item ID；每项 context 固定 `type=creative_domain_task`、`workflow=creative_production`，并原样携带
`issue_id`、`leader_agent_id`、order/item/variant/candidate IDs、revision、`expected_sizes` 和下一阶段 Agent
IDs。先用 JSON 解析器校验，再执行：

```text
multica task by-source list --agent <producer-agent-id> \
  --kind creative_order_item_production --ref <item-id> --output json
multica task fanout --agent <producer-agent-id> --input-file <manifest.json> --output json
```

fanout 成功后立即结束，不轮询。CLI 返回字段错误时修正同一 manifest；没有返回 created task 就不得声称
已入队。写回或委派失败必须让当前 task 失败，并在 Variant/task 保留真实 error code/message，不创建 Issue。
