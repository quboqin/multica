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
| `creative_leadership` | 标准订单首次委派、异常恢复、用户汇总；不重复创建平台已原子初始化的 direct edit | plan 或缺失任务恢复 |
| `generation_plan` | 写 4-5 个候选 brief 与主尺寸计划 | candidate primary production |
| `image_edit` | 写候选主尺寸或 selected 缺失尺寸 generated assets 和过程证据 | 调用 `prime_compose` Skill |
| `direct_image_edit` | 写下一 revision generated assets 和底图过程证据 | 调用 `prime_compose` Skill并进入最终 visual QC |
| `quality_control` | 候选原子晋级 3 个，或按实际 expected sizes 写联合 visual 报告 | candidate selection 后由平台补排尺寸；finalize 后归档 |

成员不得越级创建其他阶段。下游 Agent ID 来自冻结 squad snapshot 或上游 context，不按名称猜测。

## task 来源与幂等

| 阶段 | `trigger_evidence_kind` | ref | item key |
| --- | --- | --- | --- |
| 参考分析 | `creative_crawl_run_analysis` | Crawl Run | candidate + analysis version |
| 方案 | `creative_order_item_plan` | Order Item | item + revision |
| 候选/晋级生产 | `creative_order_item_production` | Order Item | variant + revision + production stage |
| 候选晋级 | `creative_order_item_candidate_selection` | Order Item | `candidate-selection:v1` |
| QC | `creative_order_variant_qc` | Variant | variant + lane + revision |
| 直接改图 | `creative_order_item_direct_edit` | Order Item | variant + target revision；source revision 指向当前 active revision，通常为 target revision - 1；中间 staging 已废弃时可跨过已废弃版本 |

Source Analysis 写回的 `trigger_evidence_kind=crawl_run` 是领域来源，不是 task source kind。

fanout 幂等键是 target Agent、source kind/ref 和 item key。同一 source 可包含多个并行 item；查询后逐项比较。
active/succeeded item 或领域结果已到下一阶段时不再创建。重试引用失败 task，取消只作用于未完成 task。
所有 Order context 原样透传根 `issue_id`、`leader_agent_id`、order/item/variant IDs、revision、scope 和输入快照
identity；不得只传自然语言。

方案阶段的 source ref 是单个 Order Item，故每个 item 单独提交一个 manifest。context 的 `creative_order_item_id`
必须与 ref 相同，并固定 `type=creative_domain_task`、`workflow=creative_plan`、`revision=1`、`scope=item`，同时携带
candidate/source-analysis 及下游 Agent IDs；Planner 以这些 ID 回读冻结订单输入，不从 context 复制可变数据。

fanout 原子校验整个 manifest。任一 item 非法时不创建部分任务。成功提交后调用方结束；并发由 Agent、
runtime 和 provider 限额控制。

## 输入真值

- Source Analysis 只描述参考图，不提供可投放金融事实。
- `copy_snapshot` 是批准文案与产品事实唯一真值；订单创建后不读取文案库最新版本替换。
- order `input_snapshot` 是冻结输入唯一真值：其中 `market_pack` 决定 Prime、QR、品牌文件、App UI、尺寸和合规规则，
  `squad_snapshot` 决定 capability Agent IDs。运行时不读取市场包最新版本或固定本机文件。
- 业务用户通过页面维护并发布资源；Agent 消费冻结版本，不把任何市场的文案、坐标或组件写进指令。

## 状态与隔离

状态从领域对象和关联 task 派生：queued/running、partial、completed、failed、action_required、cancelled。
失败项不取消兄弟项；每个 Variant 独立推进并即时展示已交付结果。迟到 task 只能写声明 revision，服务端
拒绝覆盖更高 revision。

每个 item 先有 4-5 个 candidate Variant，各自只生成由方向选择的 `primary_size`；候选原子晋级恰好 3 个，reserve 保留但不参与交付汇总。
终态失败候选由平台标记 rejected；至少 3 个合格候选即可进入原子比较，少于 3 个则保留真实失败供有界恢复或人工处理。
selected 的 expected sizes 共享批准文案、业务语义、DesignDNA、信息层级和 `asset_family_id`；各尺寸按 LayoutPlan 从同一参考独立生成，
主尺寸只可作为一致性参考，不是方形硬依赖。后端只按冻结 config 原样叠加完整品牌模板。`prime_compose` Skill 只调用后端确定性合成；visual QC 写独立报告；`qc-finalize` 在视觉报告归档后
按报告与返工策略登记 delivered assets、Variant completion 和 Inbox；阻断 finding 不能由 Agent 自报通过。

初始 direct edit 的 R1 source、R2 staging revision 和执行 task 由平台建单事务原子创建；Leader 只恢复经领域状态确认真正缺失的当前 revision task。
direct edit 只处理 context 的 source asset 和 expected sizes；source 不可覆盖，输出 revision 加一并记录
lineage。preview 不进入品牌组件/QC，publish 调用 `prime_compose` Skill并进入最终 visual QC，不能直接登记 delivered。

## 失败与人工反馈

凭证、输入、工具或写回失败保留 stage、error code/message、已成功对象和可执行下一步。内部失败留 task；
只有需要用户决定时进入 Issue/Inbox。不得用评论文字派生领域状态。

以下用户操作追加 feedback event：素材采用/拒绝，文案推荐曝光/采用/替换/编辑，Variant 接受/调整/放弃，
成图接受/下载/报告问题/区域标注，QC 误判/漏检/接受风险。撤销写新事件，不删除历史。

composer 的结构化 Prime 承托失败只允许对失败尺寸做一次背景定向修复；最终 visual QC 的真实 Prime 遮挡或官方文字不可读按服务端有界轮次只返工失败尺寸。
失败修复写 staging revision；通过全部硬门后才原子切换 active。已有 active 时 staging 失败不影响线上素材；首次交付的 selected 耗尽有界修复时，
平台按排名晋级 reserve 并补齐缺失尺寸。没有 active 且没有 reserve 才进入人工处理，不能自动带风险发布。
预测遮挡、关键内容缺失和其他建议由用户在高清对比中决定局部调整、重做或放弃。
