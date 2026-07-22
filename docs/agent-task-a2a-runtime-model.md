# Agent Task 的 A2A 运行模型

本文记录 Multica 中 Agent 与 Codex/Claude 等 AI 编程工具的运行方式。核心观点是：Multica 的 Agent 不是一个常驻聊天窗口，而是一份可被调度的配置；真正运行的是每次任务触发时创建的 Agent Task。

## 一句话模型

Multica 不是让 Codex 常驻在一个聊天窗口里一直等人输入，而是每次有一个 Agent Task 要执行时，由 daemon 临时启动一次 Codex 进程。任务跑完后，Codex 进程结束释放；但 Multica 会保存必要的上下文，让下一次同一个 agent 继续处理同一个 issue 时可以恢复。

```text
Agent = 角色配置
Runtime = daemon × AI 编程工具
Agent Task = 一次真实执行

进程不是常驻的，
但任务上下文可以被保存和恢复。
```

## 为什么不是常驻聊天窗口

普通使用 Codex 时，用户通常是在一个终端窗口里持续对话：

```text
打开 Codex
  -> 输入问题
  -> Codex 回答 / 执行
  -> 用户继续追问
  -> 同一个窗口继续上下文
```

Multica 的协作场景不一样。它要服务的是团队看板、Issue 状态、Agent 分派、产物审查和人类确认。因此 Multica 不把“一个终端窗口”当作协作单元，而是把“一次可追踪的任务执行”当作协作单元。

```text
Issue 被分配给 agent / @agent / rerun
  -> 服务端创建 Agent Task
  -> daemon claim 任务
  -> daemon 启动 Codex
  -> Codex 完成这一轮工作
  -> daemon 上报输出、日志、token、session_id、work_dir
  -> Codex 进程退出
```

这样做的好处是，每一次执行都有明确边界、状态、产物和审计记录。

## 一次 Agent Task 一次启动的价值

可以把 Multica 的运行方式近似理解成：每次 `@agent`、分派、rerun 或系统触发，都会创建一个新的 Agent Task；daemon 为这个 Agent Task 启动一次 Codex/Claude 等 AI 编程工具，任务结束后释放进程。

更准确地说，不是“一次聊天一次启动”，而是：

```text
一次 Agent Task，一次启动。
```

这和长窗口模式的差异在于，长窗口把多轮对话、多个阶段和大量历史上下文都放进同一个 AI 会话里；Multica 则把每次执行变成一个可调度的任务，把长期上下文沉淀到 Issue、Comment、Status、Sub Issue、Agent Task result、`session_id` 和 `work_dir` 里。

| 维度 | 常驻长窗口 | Multica 短周期 Agent Task |
| --- | --- | --- |
| 资源占用 | 窗口和进程长期存在，agent 多时成本不可控 | 有任务才启动，跑完释放，daemon 可统一限流 |
| 任务边界 | 多个意图混在同一段对话里，边界容易模糊 | 每个 Agent Task 都有明确输入、目标、产物和状态 |
| 上下文管理 | 主要依赖聊天窗口里的历史消息 | 事实沉淀到 Issue/Comment/Status，执行现场用 `session_id`/`work_dir` 恢复 |
| 失败处理 | 难判断是窗口状态坏了、上下文乱了，还是任务失败 | 单次 task 失败可记录原因、transcript、日志，并支持 rerun |
| 并发能力 | 很难用多个长窗口稳定承载大量需求和 agent | 适合队列、claim、并发、超时、取消、重试 |
| 模型切换 | 会话通常绑定某个工具和模型上下文 | 每次启动都可以按 agent 配置选择模型、参数、MCP 和权限 |
| 人类 review | 人要进入长聊天里找产物和决策点 | 人只看 Issue 产物、状态和 review gate |
| A2A 交接 | 更像 agent 在同一个窗口里互聊 | 通过 Issue / Comment / Status / Sub Issue / Agent Task result 交接 |

示意图：

![长期协作，短周期执行](./diagrams/agent-task-lifecycle-vs-long-window.png)

## 长窗口仍然有价值

短周期 Agent Task 并不是说长窗口没有价值。更准确的定位是：

```text
长窗口 = 个人探索 / 连续对话 / 低协作治理成本
短周期 Agent Task = 团队协作 / 可调度执行 / 可审计交付
```

长窗口适合任务形成前或个人深度工作中的连续探索，例如：

- 需求还很模糊，需要高频来回澄清。
- 一个人处理一个小问题，边想边问边改，追求最低摩擦。
- 需要短时间内保持强上下文连续性，例如 debug 一个复杂问题、追调用链、反复调整方案。
- 不需要 issue、状态、review、审计，只是解释代码、看报错、做技术选型草稿。
- 还没有形成明确产物和负责人，仍处在 brainstorming 阶段。

一旦工作进入团队协作，尤其是需要跨 agent、跨阶段、跨人类 review、跨模型执行时，就应该沉淀为 Project / Issue / Sub Issue / Agent Task。否则后续状态、责任、产物、失败恢复和发布确认都会变得难以管理。

适用边界图：

![长窗口与短周期 Agent Task 的适用边界](./diagrams/agent-task-window-fit-boundary.png)

## 四个核心对象

| 对象 | 含义 | 生命周期 |
| --- | --- | --- |
| Agent | 一个可被分派的 AI 角色配置，包括 instructions、model、custom_args、custom_env、skills、mcp_config | 长期存在，可编辑、归档、恢复 |
| Runtime | 某台机器上的 daemon 与某款 AI 工具的组合，例如本机 daemon × Codex | daemon 在线时注册，离线时失联，重启后恢复 |
| Agent Task | 某个 agent 在某个 issue/chat/autopilot 上的一次真实执行 | queued -> dispatched -> running -> completed/failed/cancelled |
| AI 进程 | daemon 为某个 Agent Task 启动的 Codex/Claude 子进程 | 每次任务临时启动，结束后释放 |

因此，Agent 和 Codex 进程不是一回事：

```text
Agent 是配置和身份
Codex 进程是执行载体
Agent Task 是这次执行的记录
```

## Codex 是什么时候启动的

Codex 只在 daemon 领到具体任务后启动。大致流程如下：

```text
人类操作 / 系统触发
  -> issue 分配给 agent，或评论 @agent，或点击 rerun
  -> 服务端创建 agent_task_queue 记录，状态为 queued
  -> daemon 发现该 runtime 有任务
  -> daemon claim 任务，状态变为 dispatched
  -> daemon 准备执行环境
  -> daemon 调 start，状态变为 running
  -> daemon 启动 Codex 子进程
```

对于 Codex，启动形态不是普通交互式窗口，而是 daemon 通过 app-server 模式与它通信：

```bash
codex app-server --listen stdio:// [daemon 默认参数] [agent custom_args]
```

其中 `custom_args` 来自 Agent 配置，会在每次启动 Codex 时追加到命令行末尾。它影响的是“下一次任务启动时的 Codex 参数”，不是已经运行中的进程。

## 为什么进程可以释放但上下文还能续上

Multica 不依赖一个永远不关闭的 Codex 窗口保存上下文，而是把上下文拆成几类持久信息：

| 上下文 | 存在哪里 | 用途 |
| --- | --- | --- |
| issue 标题、描述、评论、状态 | Multica 服务端 | agent 每次执行时重新读取协作事实 |
| task transcript / messages | task_message | 展示执行过程，支持追溯 |
| session_id | agent_task_queue | 尝试恢复上一轮 AI 会话 |
| work_dir | agent_task_queue / daemon 本地目录 | 复用代码目录、上下文文件和中间产物 |
| agent instructions / skills / mcp_config | agent 配置 | 每次执行前重新注入角色能力 |
| workspace/project context | workspace/project/issue 资源 | 告诉 agent 当前团队、项目和资源背景 |

所以一次 Codex 进程结束后，下一次同一个 agent 继续处理同一个 issue 时，daemon 可以把之前保存的 `session_id` 和 `work_dir` 传回去，尽量恢复执行上下文。

```text
第 1 次 Agent Task
  -> 启动 Codex
  -> 完成任务
  -> 保存 session_id + work_dir
  -> Codex 退出

第 2 次 Agent Task
  -> claim 同一个 agent + issue
  -> 读取上次 session_id + work_dir
  -> 启动新的 Codex 进程
  -> 尝试续上上一轮上下文
```

这就是“进程不是常驻的，但上下文可以恢复”。

## A2A 角度怎么理解

这里的 A2A 不应该理解成“两个 agent 在同一个聊天窗口里互相长聊”，而应该理解成“一个 agent 通过 issue/comment/task 这些协作对象，把下一步工作交给另一个 agent”。

例如：

```text
Leader Agent
  -> 在父 Issue 上判断下一步
  -> 创建或 @mention 设计子 Issue
  -> Design Agent 获得一个新的 Agent Task
  -> Design Agent 产出设计报告
  -> 人类 review
  -> Leader Agent 再推动 Coding Agent
```

Agent 之间传递的不是内存里的进程对象，而是 Multica 中可审计的协作对象：

```text
Issue
Comment
Status
Sub Issue
Agent Task result
Review decision
```

这更适合团队协作，因为所有交接都留在看板和评论流里，人类可以随时介入。

## 失败和人工确认怎么处理

如果是技术失败，Agent Task 会进入 `failed`。常见原因包括 Codex 启动失败、参数不合法、模型报错、runtime offline、超时、daemon 崩溃等。Multica 会保留失败原因和 transcript，部分失败可以自动重试，不能自动恢复的需要人类修配置或 rerun。

如果是业务上需要人类确认，不建议让 Codex 进程一直阻塞等待。推荐模式是：

```text
Agent 产出阶段结果
  -> 写评论 / 报告 / PR / 测试报告
  -> 子 Issue 进入 in_review 或 blocked
  -> 人类确认、评论或改状态
  -> 再触发下一次 Agent Task
```

也就是说，确认点发生在 Multica 的 Issue 状态和评论流里，而不是发生在 Codex 的临时进程里。

## 这个模型对团队协作的意义

这种设计把 AI 执行从“个人终端会话”变成了“团队可追踪任务”：

- 每次 agent 执行都有任务记录。
- 每次输出都能被 review。
- 每个阶段都可以由不同 agent 接手。
- 失败、重试、取消、发布确认都有明确状态。
- 人类不需要盯着一个 AI 终端窗口，而是在看板上处理确认点。

因此，Multica 的核心不是替用户保留一个 Codex 窗口，而是把 Codex/Claude 这类工具包装成可调度、可审计、可恢复的团队协作执行单元。
