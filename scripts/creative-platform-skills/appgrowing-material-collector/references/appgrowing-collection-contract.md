# AppGrowing 采集契约

## 输入

仅接受当前 task context、AutoPilot 配置和平台注入的连接器上下文。业务筛选可以包含竞品、市场、语言、
设备、媒体、日期、分页预算、数量、选材规则及市场资源引用；`analysis_agent_id` 必须是明确 UUID。缺少连接器
要求的字段时写 `action_required`，不得用某个市场、媒体或 Agent 的默认值替代。唯一的兼容例外是历史 AutoPilot
完全未提供 `selection_rules`：连接器会归一化为标准周采集规则（新素材 40%，投放少于 7 天且曝光估算大于 1K；
跑量素材 60%，投放超过 30 天且曝光估算不低于 10M）。新建或编辑的采集参数必须显式写入完整对象；显式但不完整
的规则会返回精确缺失字段，Agent 只能从当前 task context 或 AutoPilot 的明确筛选条件重建后重试一次。没有明确
依据时必须写 `action_required`，不得以默认值填充。类型错误、越界值或矛盾的规则继续报错。

连接器会无损归一化已提供的常见字段别名，例如 `ratio_pct`/`share_pct`、`duration_max_days`/`duration_min_days`、
`estimated_impressions_gt`/`estimated_impressions_gte` 及等价的嵌套边界对象。该归一化只重命名或换算已明确给出的
值；规范字段优先于别名。历史 AutoPilot 没有结构化规则时，连接器只使用上述标准周采集规则作为兼容行为。

`params-json` 保持业务配置的原始结构；只做连接器 API 明确要求的类型和枚举转换。不确定的筛选值保留在
run 说明中，不猜枚举。连接器分页、去重和浏览器 fallback 使用 task 明确值或平台配置，不在 Skill 中写死。

## 素材类型

本采集只入库 `asset_type=image` 的图片广告。视频、非图片资源和无法确认类型的资源必须在选材前排除，不能占用
目标数量、不能写入 Crawl Run 候选，也不能进入后续归档或参考分析。诊断可记录被排除数量，但不能把它们描述成已采集素材。

## Crawl Run

`multica crawl run` 使用工作区可用的 AppGrowing 凭证，先创建稳定 Crawl Run，再调用 broker 并把候选关联到
同一个 run。响应没有 `crawl_run_id` 属于服务契约错误。

每个采集 task 只能提交一次 `multica crawl run`。Agent 必须把 exec/Bash 工具的外层等待时间设置为长于 CLI
`--timeout`，并等待同一个前台命令返回。空 body、pending session、外层超时或无法确认命令状态时不能重新发起 crawl；
只能继续等待同一命令/会话，无法继续等待则写 `crawl_command_wait_incomplete` / `action_required`。唯一允许的重试是
第一次命令在创建浏览器 run 之前因参数校验错误失败，且缺失字段可由当前 task context 或 AutoPilot 明确筛选条件
确定修复。

HTTP 429、`worker_busy`、`credential broker worker is busy` 或 `crawler worker is busy` 是平台内部 crawler-worker
容量信号，不是 AppGrowing 限流；当前 task 必须保留真实 error code/message 后停止。`worker_unavailable` 或
`context canceled` 表示本次命令被取消、超时或 worker 不可用，同一 task 内不得补发第二次 crawl。

以返回的 `selection_summary`、逐页 evidence、archive summary 和 errors 为真值。目标数量或比例不能反推为
实际结果。单个查询失败不能覆盖其他查询的成功；run 可以是 `partial`。授权问题写业务可读的重新绑定入口，
不得记录或输出凭证内容。

## 入库后的参考分析

后端为本次新增图片统一派发参考分析。采集 Agent 回读 Crawl Run 与响应中的 `analysis` 排队汇总后结束，
不执行委派脚本、不调用分析 fanout、不指定分析版本。采集数量、筛选、分页和预算仍完全由采集参数控制。

同一素材已有活动任务时复用该任务；普通请求复用当前版本，明确重新分析才分配新版本。
入库后未派发的任务由平台在最近七天的采集记录中定时补派；已执行失败或取消的任务需要明确重新分析。
分析 task 写回匹配 candidate、version 和 run 的 Source Analysis，并回读确认。排队数量不等于分析完成数量。
