---
name: appgrowing-material-collector
description: "当 AutoPilot 或用户要求从 AppGrowing 真实采集广告素材、创建 Crawl Run，并对新增图片并发发起预分析时使用。"
allowed-tools: Bash(multica *), Bash(powershell *), Bash(python *)
---

# AppGrowing 素材采集

读取 `references/appgrowing-collection-contract.md` 并严格执行。输入来自当前原生 task 的
`autopilot_run_id`、AutoPilot 描述和 task context，不创建或查找父 Issue。

1. 把业务筛选条件和配置中明确给出的 `analysis_agent_id` 转换成结构化参数，执行真实
   `multica crawl run --analysis-agent-id <analysis_agent_id>`，不要传 `--issue-id`，不得按智能体名称猜 ID。
2. 从响应读取 `crawl_run_id`，确认素材已经进入工作区素材库并关联本次 run。
3. 对本次 `is_new_in_run=true`、`asset_type=image` 且尚无 Source Analysis
   的候选，运行：

   ```text
   python <当前 Skill 目录>/references/delegate_preanalysis.py \
     --crawl-run-id <run-id> --assignee-id <广告参考分析智能体ID>
   ```

4. 脚本通过 `multica task fanout` 创建直接分析 task；实际并发由分析智能体的
   `max_concurrent_tasks`、运行时总并发和 provider 限流共同控制，不在脚本中重复配置。不得创建分析
   Issue、Task Batch 或 Work Unit。
5. 提交完立即结束，不轮询。分析任务优先读取平台归档；归档尚未完成时，下载命令读取本次采集的
   真实源文件，归档任务继续独立完成。Crawl Run 页面根据素材、Source Analysis 和关联 task 派生进度。

授权失效时将 Crawl Run 置为 `action_required` 并记录重新绑定入口；真实抓取失败时写入失败阶段和
可操作原因。不得输出 Cookie、Token、Header 或调试请求，也不得用测试图片或虚构结果替代。
