# AppGrowing 采集契约

## 输入

仅接受当前 task context、AutoPilot 配置和平台注入的连接器上下文。业务筛选可以包含竞品、市场、语言、
设备、媒体、日期、分页预算、数量、选材规则及市场资源引用；`analysis_agent_id` 必须是明确 UUID。缺少连接器
要求的字段时写 `action_required`，不得用某个市场、媒体或 Agent 的默认值替代。

`params-json` 保持业务配置的原始结构；只做连接器 API 明确要求的类型和枚举转换。不确定的筛选值保留在
run 说明中，不猜枚举。连接器分页、去重和浏览器 fallback 使用 task 明确值或平台配置，不在 Skill 中写死。

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

分析 task 必须写回并回读匹配的 Source Analysis。平台完成任务时再次校验候选、版本、run 和领域产物；
缺失产物以 `creative_output_missing` 失败关闭。fanout 某项失败不取消兄弟项。
