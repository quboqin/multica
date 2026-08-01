# AppGrowing 采集契约

## 读取业务条件

阅读全文，识别目标候选池 Issue ID、竞品、优先竞品、地区、语言、设备、媒体、时间范围、
选材比例和最大数量。当前市场识别 Indonesia/印尼/印度尼西亚/ID、
Indonesian/印尼语/印度尼西亚语、Android、iOS。未来 Issue 绑定其他市场资源包时，按该包
明确声明的地区和语言执行。

把条件写入临时 JSON 文件。`selection_rules` 必须是包含 `new_materials` 和
`volume_materials` 的对象，不能写成数组：

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
  "browser_capture_fallback": true,
  "limit": 25,
  "selection_rules": {
    "new_materials": {"ratio": 0.4, "duration_days_lt": 7, "impression_gt": 1000},
    "volume_materials": {"ratio": 0.6, "duration_days_gt": 30, "impression_gte": 10000000}
  }
}
```

媒体只有在确认 AppGrowing 枚举 ID 时才下推；不确定时不要猜，在结果中说明“已理解但未
下推筛选”及原因。先确认连接器对日期范围的语义能返回持续投放超过 30 天的素材；不能满足时
如实说明规则冲突。

## 执行

在 PowerShell 中把 JSON 读取成一个完整字符串：

```powershell
$params = Get-Content -Raw -LiteralPath <JSON 文件>
multica crawl run --connector appgrowing --capability material_search --issue-id <目标候选池 Issue ID> --params-json $params --timeout 15m --output json
```

不要把结构化 JSON 放进 `--intent-file`，也不要在 shell 中手工拼接或反复转义 JSON。
`--issue-id` 必须是目标父 Issue 的真实 ID，不能省略。

使用平台已授权的 AppGrowing 登录态。不得向用户索要或输出账号、密码、验证码、Cookie、
Token、Header、GraphQL 或浏览器调试信息。不要另做 HTML 看板，也不要只输出 URL。

每家普通竞品至少核对 3 页，优先竞品至少核对 5 页。某一家接口页为空或失败时，只对该竞品
启动 Playwright 浏览器补查，并继续核对其后续页；其他竞品已经命中的结果不能掩盖这家竞品的
失败。执行证据中保留逐竞品、逐页状态，但不要把内部请求或调试字段展示给业务用户。

授权失效时，告诉用户前往“设置 - 集成”重新绑定 AppGrowing。抓取失败时说明失败阶段和可操作
原因，不能用测试图片或虚构结果代替。

抓取完成后必须读取响应顶层的 `selection_summary`。最终评论中的新增素材、跑量素材、实际比例、
缺口和比例是否达标只能引用 `selection_summary.actual`、`selection_summary.actual_ratio`、
`selection_summary.shortfall` 和 `selection_summary.ratio_target_met`，不能用配置的目标比例反推实际数量。
当目标比例未达成时，必须明确写出实际数量和缺口，同时说明候选池是否仍补足到最大数量。

## 用户评论

最终评论只使用自然中文：

```text
本次理解的查询条件

竞品：……
优先竞品：……
投放地区：……
语言：……
设备：……
媒体：……（没有限定就写“未限定”）
时间范围：……
选材规则：……
最多输出：……条

抓取结果

共找到 N 条素材，其中新增 X 条、已有 Y 条。
实际选材：新增素材 A 条（P%），跑量素材 B 条（Q%）。目标比例……（已达成/未达成，缺口……）。
素材已放入父 Issue 候选池。请逐图选择参考素材并选择或编辑文案，然后开始修图。
```

不要展示命令、JSON、内部字段或调试过程。
