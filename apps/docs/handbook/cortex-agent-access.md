# Cortex：智能体对文档 / 多维表格的权限模型与临时授权

分支：`fix/agent-document-access`，基于 `codex/cortex-g1-ui-rework`。
需求来源：[智能体与 CLI 接口](cortex-prd.html#fr-agent)（PRD 5.7）；架构手册第 11 章「Cortex：文档与集合的实现」。

起因：2026-09-28 在一篇仅所有者可见的文档里 @ 智能体，智能体对该文档的读取 / 评论 / 附件全部返回“资源不存在”，只好把报告写到运行时机器的 `/root/multica_workspaces/…`，用户打不开。

## 一、三个角色

| 角色 | 是谁 | 决定什么 |
|---|---|---|
| **发起人 A** | @智能体或给它派任务的人（任务的 `originator_user_id`） | 能否调用这个智能体（`canInvokeAgent`） |
| **智能体所有者 O** | 创建智能体的人 | 智能体是私有还是 `public_to` |
| **运行时所有者 R** | 智能体所在运行时（runtime）的所有者 | **任务令牌以 R 的身份认证**（`daemon.go` 里 `tokenParams.UserID = locked.OwnerID`） |

私有运行时要求 O = R；共享运行时的 R 可能是别人。

## 二、此前的行为（问题所在）

文档和表格各有 owner / edit / view / 无 四级，与 workspace 管理员角色无关。智能体没有自己的权限，@ 它不改变任何东西：

| 操作 | 人需要 | 智能体（此前） |
|---|---|---|
| 读文档 / 表格 | ≥ view | 仅当 R ≥ view，否则 404（装作不存在） |
| 评论、改正文、改行 / 字段 | ≥ edit | 仅当 R ≥ edit，否则 403 |
| 新建文档 / 表格 | 成员 | 可以，但**所有者是 R、私有**，A 看不见 |
| 移动 / 删除文档、归档表格 | owner | R 是 owner 才行 |
| 改分享、送审 / 发布 / 退回 | owner / 人 | 一律拒绝（`…_requires_human`） |

两个缺口：

- **太紧（场景 2、6）**：A 在自己的私有文档里 @智能体，R 不在受众里 → 智能体拿到了文档内容（派发时随任务下发）却写不回去；智能体新建的文档归 R，A 看不见。
- **太松（场景 5）**：A 只有 view 或没有权限，却能让跑在 R 运行时上的智能体去改 R 能改的私有文档 / 表格——借 R 的权限越权。

## 三、机制

### 1. 交集规则（修“太松”）

`server/internal/handler/task_resource_access.go`：任务令牌请求且任务有真人发起人 A 时，**有效权限 = min(R 的权限, A 的权限)**。适用于文档和表格的读、写、owner-only 动作（删除、移动、分享）、文档列表、搜索、运行历史。

- 无真人发起人的任务（autopilot、系统触发）：保持原状，只用 R 的权限。
- 场景 5 的结果：A 没权限 → 智能体一样 404；A 只有 view → 智能体只能读。想要权限走现有的分享流程找所有者，**不弹窗自批**。

### 2. @ 时的临时授权（修“太紧”，场景 2）

在 @ 的那一刻，由 A 本人确认：

1. A 在文档评论里 @智能体，发送前前端调触发预览接口（`PreviewCommentTriggers`）。服务端对每个会被触发的智能体算出 `document_access`：`runtime_owner_permission`（R 在这篇文档上的权限，再按 A 封顶）和 `max_grant`（A 最多能授予的，= min(A, edit)，无需授权时为空）。
2. 有智能体够不到文档时，点发送弹窗（`packages/views/issues/components/agent-access-grant-dialog.tsx`，顶层评论和回复两个 composer 都接了），按智能体选：允许本次任务读取、回复并编辑 / 仅允许读取 / 不授权仍发送 / 取消（保留草稿）。
3. 选择随评论提交（`CreateCommentRequest.agent_grants`），服务端校验后写入表 `comment_agent_grant`（迁移 536），键 = 触发评论 × 智能体，记 `granted_by`、`granted_at`。
4. 任务带 `trigger_comment_id` 创建；智能体之后访问该文档时，服务端查授权记录，把它作为权限**下限**：`effective = max(min(R, A), min(grant, min(A, edit)))`。

边界（全部服务端强制）：

| 约束 | 实现 |
|---|---|
| 只对这一篇文档 | 仅当 `task.issue_id == 文档` 才查授权；子页面、其他文档不算 |
| 只对这一次任务 | 任务进入终态即失效（任务令牌本来也会删除） |
| 不超过 A | 授权时以 A 的权限为上限，运行时再取 A 的当前权限，最高 edit |
| 不给 R 本人 | 授权绑定任务，不是分享；R 作为人打开仍是 404 |
| 人的动作留给人 | 分享、发布、移动、删除不受授权影响 |
| 只有人能授权 | 任务令牌发的评论带 `agent_grants` → 403；非文档 → 400；超出自身权限 → 403 |
| 有审计 | `granted_by`、`granted_at` |

### 3. 新建资源归发起人（场景 6）

任务有真人发起人 A 且 A ≠ R 时，智能体新建的文档 / 表格所有者 = A（`createdResourceOwners`），并把 R 加为 **edit 协作者**——否则按交集规则智能体会立刻失去访问权、没法继续写。A 在自己的列表里能看到并管理分享；R 的协作者身份在分享面板可见。

### 4. 界面

文档分享按钮由「Publish」改为「Share」（`packages/views/documents/document-sharing.tsx`）。

## 四、场景表

| # | 场景 | A | R | 结果 |
|---|---|---|---|---|
| 1 | A 在自己的私有文档 @智能体，A = R | owner | owner | 读、评论、编辑，可删可移；不能改分享 / 发布 |
| 2 | 同上，但 R ≠ A 且 R 不在受众 | owner | 无 | 弹窗：授 edit → 读、评论、编辑；授 view → 只读；不授 → 404 |
| 3 | 文档只分享给 R view | owner | view | 不弹窗也能读；写需授权（弹窗 `max_grant`=edit） |
| 4 | 全 workspace edit | edit | edit | 可编辑，无弹窗 |
| 5 | A 只有 view / 无权限，R 有 edit | view / 无 | edit | 交集：只读 / 404，**不能借 R 越权** |
| 6 | A 让智能体新建文档或表格 | — | — | 所有者 A，R 为 edit 协作者 |
| 7 | 智能体 A 调智能体 B | — | — | 能否调用按链顶真人；B 的权限 = min(B 的 R, 链顶真人) |
| 8 | 表格 | — | — | 无 @ 入口，只有交集规则 |
| 9 | 无真人发起人的任务 | — | R | 原状，只用 R 的权限 |

## 五、尚未覆盖（第二期候选）

- **运行中才碰到的其他私有文档**：仍 404。方案：任务进入 `waiting_access`（仿 `waiting_local_directory`，带 `wait_reason`），通知有权授权的人（通常是 A；A 无权则通知所有者），批准后继续，拒绝 / 超时则任务失败并在评论区说明。要改 daemon、智能体重试、通知链路。
- **编辑旧评论新加 @**：走 `UpdateComment`，未接弹窗。
- **服务端对拿到内容却无权限的机器凭证仍返回 404 而非 403**：智能体应把触发文档上的 404 当“无权限”上报，而不是把结果存到本地。
- 任务家族列表（`ListTasksByIssue` 的 SQL）未按发起人再过滤，只暴露标题。
- 需同步更新：PRD 5.7 的权限表、架构手册第 11 章、内置技能 `multica-platform` 的 `references/documents.md`（“运行 = 运行时主人”一句已不再准确）。

## 六、验证

- 前端：`agent-access-grant-dialog.test.tsx`、`document-sharing.test.tsx`、两个 composer 套件、评论卡片、locale parity、core API/schema 通过；`tsc` 对 `packages/views`、`packages/core` 干净。5 种语言文案 `comment.agent_grant.*`。
- 后端：`server/internal/handler/task_resource_access_test.go` 覆盖文档 / 表格的交集、授权（edit、view、失效、伪造头）、授权校验、预览、新建资源归属。sqlc 生成代码为手写，需 `sqlc generate` 核对。运行：

```
cd server && go build ./... && go test ./internal/handler -run 'TestRunIsCapped|TestRunOnATable|TestInvitedRun|TestCommentAgentGrants|TestPreviewReports|TestWhatARunCreates|TestDocument|TestAgentTokenWrites|TestCollectionSharing'
```
