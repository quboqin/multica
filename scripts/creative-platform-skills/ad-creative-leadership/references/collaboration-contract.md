# 创意小队协作契约

## 原生对象与留痕

- Crawl Run 保存一次采集；Source Analysis 保存逐图理解；Creative Order、Item、Variant、Asset、QC Report
  保存生产状态；这些都不是 Issue。
- `agent_task_queue` 是机器执行单位。task 详情用于排障和审计，不占业务看板。
- 一个正式 Order 只关联一个用户 Issue；不新增 Task Batch、Work Unit 或阶段子 Issue。
- Issue 只保存用户目标、人工决定、真实阻塞和最终验收。中间输入、模型证据、附件和检查写领域对象/task。

## 能力和阶段所有权

| Capability | 唯一职责 | 唯一正常下一跳 |
| --- | --- | --- |
| `material_collection` | 创建 Crawl Run、导入候选 | `reference_analysis` |
| `reference_analysis` | 写 Source Analysis | 无 |
| `market_pack_component_extraction` | 写待确认组件候选 | 无 |
| `creative_leadership` | 首次委派、异常恢复、用户汇总 | plan 或 direct edit |
| `generation_plan` | 写 V01-V03 brief | production |
| `image_edit` | 写 generated assets | Prime |
| `direct_image_edit` | 写下一 revision generated assets | publish 时 Prime |
| `prime_compose` | 写 primed package | technical + visual QC |
| `quality_control` | 写一个 lane report 并 finalize | 无 |

成员不得越级创建其他阶段。下游 Agent ID 来自冻结 squad snapshot 或上游 context，不按名称猜测。

## task 来源与幂等

| 阶段 | `trigger_evidence_kind` | ref | item key |
| --- | --- | --- | --- |
| 参考分析 | `creative_crawl_run_analysis` | Crawl Run | candidate + analysis version |
| 方案 | `creative_order_item_plan` | Order Item | item + revision |
| 生产 | `creative_order_item_production` | Order Item | variant + revision |
| Prime | `creative_order_variant_prime` | Variant | variant + revision |
| QC | `creative_order_variant_qc` | Variant | variant + lane + revision |
| 直接改图 | `creative_order_item_direct_edit` | Order Item | variant + size + source revision |

Source Analysis 写回的 `trigger_evidence_kind=crawl_run` 是领域来源，不是 task source kind。

fanout 幂等键是 target Agent、source kind/ref 和 item key。同一 source 可包含多个并行 item；查询后逐项比较。
active/succeeded item 或领域结果已到下一阶段时不再创建。重试引用失败 task，取消只作用于未完成 task。
所有 Order context 原样透传根 `issue_id`、`leader_agent_id`、order/item/variant IDs、revision、scope 和输入快照
identity；不得只传自然语言。

fanout 原子校验整个 manifest。任一 item 非法时不创建部分任务。成功提交后调用方结束；并发由 Agent、
runtime 和 provider 限额控制。

## 输入真值

- Source Analysis 只描述参考图，不提供可投放金融事实。
- `copy_snapshot` 是批准文案与产品事实唯一真值；订单创建后不读取文案库最新版本替换。
- order `input_snapshot` 的 market snapshot 是 Prime、QR、品牌文件、App UI、尺寸和合规规则唯一真值；运行时
  不读取市场包最新版本或固定本机文件。
- 业务用户通过页面维护并发布资源；Agent 消费冻结版本，不把任何市场的文案、坐标或组件写进指令。

## 状态与隔离

状态从领域对象和关联 task 派生：queued/running、partial、completed、failed、action_required、cancelled。
失败项不取消兄弟项；每个 Variant 独立推进并即时展示已交付结果。迟到 task 只能写声明 revision，服务端
拒绝覆盖更高 revision。

标准 Variant 的 expected sizes 共享批准文案、业务语义、主体、信息层级和 `asset_family_id`；方形是横竖版
重排基线。Prime 只按冻结 config 叠加组件。technical/visual QC 并发并写独立报告；`qc-finalize` 是 delivered
assets、Variant resolution 和 Inbox 的唯一事务 barrier。

direct edit 只处理 context 的 source asset 和 expected sizes；source 不可覆盖，输出 revision 加一并记录
lineage。preview 不进入 Prime/QC，publish 才进入同 expected sizes 的 Prime 与双路 QC。

## 失败与人工反馈

凭证、输入、工具或写回失败保留 stage、error code/message、已成功对象和可执行下一步。内部失败留 task；
只有需要用户决定时进入 Issue/Inbox。不得用评论文字派生领域状态。

以下用户操作追加 feedback event：素材采用/拒绝，文案推荐曝光/采用/替换/编辑，Variant 接受/调整/放弃，
成图接受/下载/报告问题/区域标注，QC 误判/漏检/接受风险。撤销写新事件，不删除历史。

QC 不自动返工。用户看高清对比后选择接受风险、局部调整、重做或放弃；同 Variant 最多一轮模型返工，
revision 加一。第二轮仍失败时停止调用并等待决定。
