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

以返回的 `selection_summary`、逐页 evidence、archive summary 和 errors 为真值。目标数量或比例不能反推为
实际结果。单个查询失败不能覆盖其他查询的成功；run 可以是 `partial`。授权问题写业务可读的重新绑定入口，
不得记录或输出凭证内容。

## 参考分析 fanout

只处理本次 run 中满足以下条件的候选：

- `is_new_in_run=true`；
- `asset_type=image` 且存在可下载归档或真实源；
- 当前 analysis version 没有 completed Source Analysis；
- 目标 Agent、`creative_crawl_run_analysis` source 和 `<candidate-id>:v<version>` item key 下没有
  active/succeeded task。

每个 item context 固定为 `type=creative_domain_task`、`workflow=creative_reference_analysis`，并携带
`crawl_run_id`、`candidate_id` 和 `analysis_version`。不注入品牌市场包、Prime、App UI 或文案库。

委派脚本使用任务运行时的 `multica task fanout` 原生命令。可用 `--cli` 显式传入任务运行时可执行文件，或由
`MULTICA_CLI` 提供；两者都没有时，Windows 优先从 `PATH` 解析 `multica.com`，其他系统解析 `multica`。非 dry-run 必须先验证该命令存在。旧 CLI 缺少
该子命令时，必须以 `multica task fanout unavailable; upgrade CLI` 失败，不能把本次采集报告为预分析完成。

分析 task 必须写回并回读匹配的 Source Analysis。平台完成任务时再次校验候选、版本、run 和领域产物；
缺失产物以 `creative_output_missing` 失败关闭。fanout 某项失败不取消兄弟项。
