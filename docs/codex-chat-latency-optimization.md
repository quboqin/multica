# Codex Chat 耗时优化记录

日期：2026-07-01

## 背景

问题：Multica 平台里发送一条 `你好`，Codex chat task 约 30 秒完成；本地 Codex CLI 约 2 秒回复。

测量对象：

- local-debug Multica daemon
- Codex provider
- `Local Codex Debug` agent
- chat task 输入：`你好`

本文把 chat task 专属改动和 Codex 通用改动分开记录。

## 结果

可比 chat task 耗时从 `29.653s` 降到 `18.136s`。

总收益约 `11.5s`。

| 步骤 | 影响范围 | 优化前 | 优化后 | 收益 |
| --- | --- | ---: | ---: | ---: |
| 修正 chat 输出规则 | 只影响 chat task | `29.653s` | `22.060s` | `~7.6s` |
| 缩短成功 Codex shutdown | 影响所有成功 Codex task | `22.060s` | `18.136s` | `~3.9s` |
| 合计 | chat task `你好` | `29.653s` | `18.136s` | `~11.5s` |

最终实测任务：

| 字段 | 值 |
| --- | --- |
| Task ID | `b90fc35a-8316-4982-934d-9c0ebaf45168` |
| Chat session ID | `118fb1c8-61a5-410d-b55f-6030aaddc007` |
| 输入 | `你好` |
| 输出 | `你好，有什么我可以帮你处理的？` |
| 排队耗时 | `0.910s` |
| 执行耗时 | `17.226s` |
| 总耗时 | `18.136s` |
| 工具调用 | `0` |

最终 daemon timing：

| 阶段 | 耗时 |
| --- | ---: |
| 进程启动 | `2ms` |
| initialize | `763ms` |
| thread start | `2850ms` |
| 进程启动到 thread ready | `3649ms` |
| turn start 到首字 | `12176ms` |
| turn start 到完成 | `12316ms` |
| shutdown | `1000ms` |
| backend 总耗时 | `16968ms` |

## 改动

### 1. Chat task 输出规则

文件：

- `server/internal/daemon/execenv/runtime_config.go`
- `server/internal/daemon/execenv/runtime_config_test.go`

改动前：

chat task 走了默认 issue task 输出规则：

```text
Final results MUST be delivered via multica issue comment add ...
```

这条规则适合 issue task，不适合 chat task。普通 chat 回复应该由 chat pipeline 写回页面，不应该要求 agent 调 `multica issue comment add`。

改动后：

chat task 使用单独输出规则：

```text
This is a chat task. Your final assistant response is delivered to the chat automatically.
Do NOT call `multica issue comment add` for ordinary chat replies.
```

影响：

- 只影响 chat task。
- 不移除 chat 里的 issue 工具。
- 用户问具体 issue 时，agent 仍可调用 `multica issue list` 或 `multica issue get`。
- 用户只发 `你好` 时，agent 没有理由查 issue 数据。

实测收益：

- `29.653s -> 22.060s`
- 约 `7.6s`

### 2. Codex final_answer 处理

文件：

- `server/pkg/agent/codex.go`
- `server/pkg/agent/codex_test.go`

改动前：

`agentMessage` 带 `phase=final_answer` 时，daemon 直接把它当成 turn 完成。

风险：

- Codex 还没发 `turn/completed`，daemon 就可能结束任务。
- usage、状态、终态错误信息可能丢失。

改动后：

`final_answer` 只表示最终文本已出现，不等于 turn 完成。

backend 会短暂等待真正的 `turn/completed`：

- 正常路径：收到 `turn/completed` 后结束任务，保留完整元数据。
- 兜底路径：grace window 内没等到 `turn/completed`，用已收到的 final answer 完成任务。

影响：

- 影响所有 Codex task。
- 主要解决正确性，不单独计入提速。
- chat 不再因为 final answer 过早结束，同时也不会在异常 session 上无限等。

### 3. 成功 Codex shutdown 缩短

文件：

- `server/pkg/agent/codex.go`
- `server/pkg/agent/codex_test.go`

改动前：

成功任务拿到输出后，还会在 Codex app-server shutdown 上多等。实测里这段约 `4s`。

改动后：

当 `finalStatus == "completed"`：

- graceful shutdown wait：`2s -> 500ms`
- `cmd.WaitDelay`：`2s -> 500ms`

失败、超时、取消、中止路径保留长诊断窗口。

影响：

- 影响所有成功的 Codex task，包括 chat、issue、autopilot、quick-create。
- 不改变失败、超时、取消场景。
- 缩短 shutdown 前，文本输出和 `turn/completed` 已被捕获。

实测收益：

- `22.060s -> 18.136s`
- 约 `3.9s`
- shutdown 从 `4.0s` 降到 `1.0s`

### 4. Codex state warm cache

文件：

- `server/internal/daemon/execenv/codex_home.go`
- `server/internal/daemon/execenv/execenv.go`
- `server/internal/daemon/daemon.go`
- `server/internal/daemon/gc.go`
- 相关测试

目的：

Codex per-task home 复用工作区级 warm cache 里的安全状态，减少每个 task 重复冷启动准备。

影响：

- 影响 Codex task。
- 目标是减少重复 setup 成本。
- 这次 `29.653s -> 18.136s` 没把它单独计入收益，因为没有做独立 A/B。

### 5. Windows Codex sandbox 兼容

文件：

- `server/internal/daemon/execenv/codex_sandbox.go`
- 相关测试

目的：

Windows + Codex CLI `0.142.4` 下，workspace-write sandbox 支持不完整。本地 managed Codex task 路径改为 `danger-full-access`，并在日志里记录原因。

影响：

- 影响不支持 sandbox 的平台上的 Codex task。
- 这是可靠性修正，不计入耗时收益。

## Chat Task 和其他 Task 的影响差异

| 改动 | Chat task | Issue task | Autopilot / quick-create | 说明 |
| --- | --- | --- | --- | --- |
| Chat 输出规则 | 是 | 否 | 否 | chat 不再继承 issue comment 输出规则。 |
| final_answer 处理 | 是 | 是 | 是 | Codex backend 通用行为。 |
| 成功 shutdown 缩短 | 是 | 是 | 是 | 只影响成功的 Codex run。 |
| state warm cache | 是 | 是 | 是 | Codex setup 路径。 |
| Windows sandbox 兼容 | 是 | 是 | 是 | Windows Codex setup 路径。 |

## 已排除

最终 `你好` 任务没有工具调用。

本次剩余耗时不是 issue 查询造成的。agent 没有调用 `multica issue list`、`multica issue get` 或其他工具。

之前 chat prompt 也没有注入具体 issue 数据。它注入的是可用命令和工作流规则。问题点是 chat 继承了 issue 输出规则。

## 剩余瓶颈

最终实测：

- 模型首字：约 `12.2s`
- Codex app-server + thread setup：约 `3.6s`
- shutdown：约 `1.0s`
- DB 排队、上报和 daemon 周边逻辑：约 `1.3s`

现在最大剩余项是模型首字。

## 后续优化方向

1. 瘦身 chat runtime brief

   目标：减少 prompt size，降低简单 chat 的模型推理负担。

   候选改动：

   - 保留 chat 专属输出规则。
   - 对 chat-only task 删除或缩短 issue comment formatting 说明。
   - 保留 issue 命令，但要求用户明确问 issue 时才使用。

2. 简单 chat 的 model / thinking 策略

   目标：产品语义允许时，避免简单问候走 `xhigh` 推理。

   风险：

   - 降低 thinking 可能影响真实任务质量。
   - 需要明确分类器或任务模式，否则正常任务可能被降配。

3. 常驻 Codex app-server

   目标：减少重复进程启动和 thread setup 成本。

   当前日志给出的上限：

   - app-server + thread setup 约 `3.6s`。
   - 常驻 server 只能消掉其中一部分，还需要设计 session 隔离和清理规则。

## 验证

测试命令：

```bash
go test ./pkg/agent ./internal/daemon/execenv -count=1
```

结果：

```text
ok github.com/multica-ai/multica/server/pkg/agent
ok github.com/multica-ai/multica/server/internal/daemon/execenv
```

运行时验证：

- 重建 `server/bin/multica.exe`。
- 重启 `local-debug` daemon。
- 通过真实 HTTP chat 路径发送 `你好`。
- 确认 task 完成。
- 确认工具调用数为 `0`。
