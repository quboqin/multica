---
name: appgrowing-material-collector
description: "当原生 task 要求通过 AppGrowing 创建 Crawl Run、导入真实广告素材并为本次新增图片委派参考分析时使用。"
allowed-tools: Bash(multica *), Bash(powershell *), Bash(python *)
---

# AppGrowing 素材采集

读取 `references/appgrowing-collection-contract.md`。只处理当前 task context 和 AutoPilot 已明确的筛选条件、
预算及 `analysis_agent_id`；不得从 Agent 名称、Issue 评论或本机配置猜输入。

1. 将输入写成结构化参数，执行：

   ```text
   multica crawl run --connector appgrowing --capability material_search \
     --params-json <params-json> --analysis-agent-id <analysis-agent-id> \
     --timeout <task-budget> --output json
   ```

2. 响应必须返回 `crawl_run_id`。回读本次 run，确认候选已进入素材库；不得创建 Issue。
3. 执行一次预分析委派脚本：

   ```text
   python <当前 Skill 目录>/references/delegate_preanalysis.py \
     --crawl-run-id <run-id> --assignee-id <analysis-agent-id>
   ```

   脚本只为本次新增、可读、尚无 completed Source Analysis 且无 active/succeeded 同 item task 的图片创建
   `creative_crawl_run_analysis` fanout。并发由 Agent、runtime 和 provider 限额控制。
4. fanout 返回后立即结束，不轮询分析任务。Crawl Run 页面从候选、Source Analysis 和 task 派生进度。

凭证失效时把 Crawl Run 标为 `action_required` 并保留平台重新绑定入口。连接器、导入或 fanout 失败时写入
真实阶段、error code/message 和已成功数量；不得以测试数据补齐，也不得输出 Cookie、Token 或请求头。
