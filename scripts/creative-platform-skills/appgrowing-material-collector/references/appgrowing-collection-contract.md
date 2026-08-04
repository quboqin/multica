# AppGrowing 采集契约

## 输入

从当前 AutoPilot 描述和 task context 读取竞品、优先竞品、地区、语言、设备、媒体、时间范围、选材
比例、最大数量、市场资源包 ID 和广告参考分析智能体 ID。缺少必要字段时将运行标为
`action_required`，不要按名称或工作区唯一性猜测。

当前市场识别 Indonesia/印尼/印度尼西亚/ID、Indonesian/印尼语/印度尼西亚语、Android 和 iOS。
未来其他市场按 AutoPilot 绑定资源包的明确配置执行。

结构化参数示例：

```json
{
  "competitors": ["Easycash"],
  "priority_competitors": ["Easycash"],
  "area": ["ID"],
  "language": ["id"],
  "platform": [1, 2],
  "date_range": "-29,0",
  "pages_per_competitor": 3,
  "priority_pages_per_competitor": 5,
  "max_pages_per_competitor": 10,
  "max_priority_pages_per_competitor": 15,
  "novel_only": true,
  "browser_capture_fallback": true,
  "analysis_agent_id": "<广告参考分析智能体ID>",
  "limit": 25,
  "selection_rules": {
    "new_materials": {"ratio": 0.4, "duration_days_lt": 7, "impression_gt": 1000},
    "volume_materials": {"ratio": 0.6, "duration_days_gt": 30, "impression_gte": 10000000}
  }
}
```

`selection_rules` 必须是对象。媒体只有在确认 AppGrowing 枚举 ID 时才下推；不确定时保留在运行摘要
中，不猜枚举。

## 执行

在 PowerShell 中读取完整 JSON：

```powershell
$params = Get-Content -Raw -LiteralPath <JSON文件>
multica crawl run --connector appgrowing --capability material_search --params-json $params \
  --analysis-agent-id <广告参考分析智能体ID> --timeout 15m --output json
```

不得传 `--issue-id`。平台使用工作区已绑定的 AppGrowing 凭证，并创建或更新一个
`creative_material_crawl_run`。响应必须包含稳定 `crawl_run_id`；若没有，视为服务契约错误。

每家普通竞品至少核对 3 页，优先竞品至少 5 页。平台把工作区历史素材身份作为排除集；达到最低
页数后如果仍未凑足 `limit` 条新素材，按竞品轮询继续到最大页数、时间预算或无更多结果。单家接口
为空或失败时只对该竞品启用 Playwright 补查，其他竞品成功不能掩盖该失败。

运行结果从顶层 `selection_summary` 读取实际新增、跑量素材、比例、缺口和是否达标，不用目标比例
反推。逐竞品、逐页证据、归档统计和错误写入 Crawl Run 的结构化详情，不创建评论。

## 预分析

采集写入后立即按 `crawl_run_id` 列出本次候选。只选择：

- `is_new_in_run=true`；
- `asset_type=image`；
- 当前 analysis version 尚无 completed Source Analysis；
- 目标分析 Agent、当前 Crawl Run source 和 `<candidate-id>:v<analysis-version>` item key 下没有
  active/succeeded task。

为每张候选生成一个 fanout item。分析下载优先使用平台归档，归档尚未结束时使用本次采集保存的真实
源 URL；不能因为异步归档仍为 pending 而永久漏掉分析。context 至少包含 `workflow=creative_reference_analysis`、
`crawl_run_id`、`candidate_id` 和 `analysis_version`。参考分析必须市场中立，不向该 task 注入品牌、Prime
或 App UI 资源。分析智能体必须先写入并回读 completed Source Analysis，再结束 task；平台会同时校验
候选、版本、Crawl Run、工作区和 run-candidate 状态，缺少或错配产物时以 `creative_output_missing`
失败关闭，不能显示为完成。fanout 部分失败时 Crawl Run 为
`partial`，成功项继续；只有无任何可交付项时才为 `failed`。

授权失效时只返回业务可读的重新绑定入口。不得向用户索要或输出账号、密码、验证码、Cookie、
Token、Header、GraphQL 或浏览器调试信息。
