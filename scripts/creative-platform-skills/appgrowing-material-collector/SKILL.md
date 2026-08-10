---
name: appgrowing-material-collector
description: "当原生 task 要求通过 AppGrowing 创建 Crawl Run、只导入真实图片广告并为本次新增图片委派参考分析时使用。"
allowed-tools: Bash(multica *), Bash(powershell *), Bash(python *)
---

# AppGrowing 素材采集

读取 `references/appgrowing-collection-contract.md`。只处理当前 task context 和 AutoPilot 已明确的筛选条件、
预算及 `analysis_agent_id`；不得从 Agent 名称、Issue 评论或本机配置猜输入。

采集范围固定为图片广告：只导入 `asset_type=image` 的真实素材。视频、非图片资源和无法判定类型的资源不进入 Crawl Run
候选，不占用数量配额，也不参与预分析。

1. 将输入写成结构化参数；新建或编辑的采集参数必须显式包含 `selection_rules.new_materials` 与
   `selection_rules.volume_materials` 的占比、投放天数和曝光阈值。连接器指出缺失字段时，必须仅从当前 task context
   或 AutoPilot 明确筛选条件重建完整规则后重试一次；没有明确依据则写 `action_required`，不得使用默认比例或阈值。
   执行：

   ```text
   multica crawl run --connector appgrowing --capability material_search \
     --params-json <params-json> --analysis-agent-id <analysis-agent-id> \
     --timeout <task-budget> --output json
   ```

2. 响应必须返回 `crawl_run_id`。回读本次 run，确认候选已进入素材库；不得创建 Issue。
3. 执行一次预分析委派脚本：

   ```text
   python <当前 Skill 目录>/references/delegate_preanalysis.py \
     --crawl-run-id <run-id> --assignee-id <analysis-agent-id> \
     [--cli <任务运行时的 multica 可执行文件绝对路径>]
   ```

   脚本只为本次新增、可读、尚无 completed Source Analysis 且无 active/succeeded 同 item task 的图片创建
   `creative_crawl_run_analysis` fanout。脚本优先使用 `--cli`，其次是任务运行时注入的 `MULTICA_CLI`，否则在 Windows 使用
   `PATH` 中的 `multica.com`（其他系统使用 `multica`）；提交前必须确认该 CLI 支持 `multica task fanout`。不支持时任务必须以
   `multica task fanout unavailable; upgrade CLI` 失败，不能把采集结果报告为预分析完成。并发由 Agent、runtime 和 provider 限额控制。
4. fanout 返回后立即结束，不轮询分析任务。Crawl Run 页面从候选、Source Analysis 和 task 派生进度。

凭证失效时把 Crawl Run 标为 `action_required` 并保留平台重新绑定入口。连接器、导入或 fanout 失败时写入
真实阶段、error code/message 和已成功数量；不得以测试数据补齐，也不得输出 Cookie、Token 或请求头。
