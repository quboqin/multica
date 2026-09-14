## 总体结论

**不通过（D1 已通过；T2b 合入门禁仍有 2 个阻塞缺陷）。**

验收对象为 PR #5 `https://github.com/quboqin/multica/pull/5` 的固定 head
`79a6b732b82ff97d08fb5651d6e3d00d03e07064`，base 为冻结的
`5d182a57f5070365577d7420fb8814dce25f8153`。固定 checkout 的提交和工作树均已核对。

本轮独立执行的 D1 生产 hook 回归通过：服务端实际构造的嵌套
`member.workspace_id` 能在本人撤权后恢复 workspace access，撤权前捕获的旧授权代次仍失效。
Core 相关 7 文件 145/145、集合/编辑 Views 4 文件 58/58、T1/T1b Issue Views 5 文件
41/41 均通过；Core/Views 包级 typecheck、lint（0 error）、串行仓库 build（5/5）和
`git diff --check` 通过。

但固定 head 只新增 D1 的 3 个 Core 文件；上一纵向切片遗留的两条保稿路径仍未修复。源码路径
和此前可复现结果均确认：目录普通后台重读失败会卸载保存编辑状态；首屏刷新裁掉冻结尾行后，
409 的明确重试仍可能使用旧 revision。它们分别导致用户草稿丢失和编辑无法完成，因此不能批准
T2b 合入。完整 Views、真实 Go/PostgreSQL、HTTP/WS、浏览器 E2E、交互式 Desktop smoke、
性能和 CI 仍未独立验证。

## 覆盖矩阵

| 需求/门禁 | 用例与实际结果 | 判定 |
| --- | --- | --- |
| D1 事件契约 | `packages/core/types/events.ts:271` 的 `MemberAddedPayload` 不再声明顶层 `workspace_id`；`server/internal/handler/invitation.go:562` 发布 `member` 对象；生产订阅 `packages/core/realtime/use-realtime-sync.ts:1341` 使用 `member.workspace_id` | 通过 |
| D1 本人撤权→嵌套重新加入 | `packages/core/realtime/use-realtime-sync-ws-instance.test.tsx` 全文件 21/21；单独筛选真实 `useRealtimeSync` 路径 1/1。断言 access 从拒绝恢复允许，撤权前代次保持失效 | 通过 |
| T2b Core / authorization fence / cache | 7 files / 145 tests：collections data source、mutations、queries、schemas、API schema、session cleanup、realtime WS instance | 通过（定向） |
| T2b 集合页面与共享 editor | 4 files / 58 tests：列表、详情、共享 cell editor、搜索回归；包含刷新、失败 CAS 保稿、checkbox 原意图 Retry、number 0/负数/小数/空白与 Clear、可见态恢复 | 通过（happy-path/定向）；下列两条负向路径仍阻塞 |
| T1/T1b Issue 表格回归 | 5 files / 41 tests：编辑、分组、虚拟层级、分组行、Issue surface | 通过（定向） |
| TypeScript | `pnpm --filter @multica/core typecheck`、`pnpm --filter @multica/views typecheck` | 通过（包级）；全仓尝试因本机资源争用中止，见未覆盖项 |
| Lint | Core 0 errors / 2 existing warnings；Views 0 errors / 25 existing warnings | 通过（无 error） |
| Build | `pnpm exec turbo build --filter=!@multica/mobile --concurrency=1`：5 successful / 5 total；Web 编译、类型检查和页面生成完成，Desktop renderer 完成；Desktop CLI 因环境无 `go` 使用 fallback | 通过（环境有 fallback warning） |
| 差异卫生 | `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153..HEAD`；固定 checkout 工作树清洁 | 通过 |
| 服务端集合契约 | Go、PostgreSQL、迁移、RLS、CAS、审计、metrics、事件传输 | 未验证：本机无 `go`/`pg_isready`，`make test` 先因无 `.env` 停止 |
| 真实端到端 | Web API/WS、浏览器 E2E、Desktop interactive smoke、CI、性能阈值 | 未验证：无运行服务、认证账号和测试数据；不把开发自测代替独立结果 |

## 缺陷清单

### 严重（阻塞 T2b 合入）

1. **P1：目录普通后台重读失败会丢失未保存编辑状态。**

   位置：`packages/views/collections/collection-detail-page.tsx:333-346`、
   `packages/views/collections/collection-detail-page.tsx:397-399`。

   复现环境：固定 head `79a6b732b82ff97d08fb5651d6e3d00d03e07064`，真实
   `CollectionDetailPage`，已加载一行集合记录。

   步骤：

   1. 在记录单元格输入 `unsaved before offline`，使 editor 进入 dirty；也应分别覆盖 pending 和 failed 状态。
   2. 让目录 `getCollection` 的后台刷新在已有数据期间返回 503/断网，并耗尽重试。
   3. 观察页面显示错误态，再恢复 API 并点击 Retry。

   期望：普通临时读取失败保留表格、草稿、原 CAS 基线和 pending/failed 状态；恢复后可继续编辑或明确重试。

   实际：`detailQuery.isError` 直接替换整个 `CollectionTableContent` 子树，拥有
   `editorStates` 的 Map 随之卸载；恢复后重新创建空 Map，单元格回到服务器原值，草稿被丢弃。
   明确撤权/不可访问仍应立即清理受保护内容，不能用保留草稿的修正绕过授权边界。

   修复门禁：已有数据的普通重读失败不卸载状态边界，并补 dirty、pending、failed 三态的
   “目录失败→恢复”页面回归；撤权路径继续清理。

### 一般（阻塞 T2b 合入）

2. **P2：冻结尾行发生 CAS 冲突后，明确重试仍可能反复使用旧 revision。**

   位置：`packages/views/data-view/controller.ts:333-360`、
   `packages/views/collections/collection-detail-page.tsx:461-469`、
   `packages/views/data-view/cell-editor.tsx:189-197`。

   复现环境：固定 head，真实集合页面，201 个实际记录，首屏 page size 200，编辑第 201 行。

   步骤：

   1. 加载首屏和第 201 行，输入草稿但不立即提交。
   2. 在另一客户端把第 201 行从 revision 1 更新到 revision 2。
   3. 触发 focus/可见态刷新，使首屏重读并裁掉旧分页尾行。
   4. 首次 Save 得到 409；随后连续点击明确 Save/Retry。

   期望：保留草稿和首次编辑基线；在活动编辑记录完成按原 workspace/source/record 的权威重读后，
   用户明确重试才采用 revision 2 并成功保存。

   实际：controller 的 head 刷新将分页 cursor 裁到 `[null]`；冻结行刷新只从当前 `liveRows`
   更新，缺失的尾行继续使用冻结旧对象；editor 的 `latestRowRef` 因而仍可能是 revision 1。
   重试请求的 `expectedRevision` 可持续为 `[1, 1, 1]`，每次 409，草稿无法完成保存。

   修复门禁：活动编辑尾行接入按原 workspace/source/record 隔离的权威单条重读，并补真实
   200+1 行的 “刷新→409→重读→明确保存成功” 回归；同时覆盖晚到结果、撤权、虚拟化重挂和
   checkbox 原意图重试。

## 埋点核验结果

- D1 本次只改变客户端事件契约和授权代次恢复，没有新增业务埋点。服务端
  `server/internal/handler/workspace.go:488-497` 定义 `MemberWithUserResponse.workspace_id`，
  `server/internal/handler/invitation.go:562-566` 将其嵌套在 `member` 中；该协议形状与客户端
  `packages/core/types/events.ts:271-274`、`packages/core/realtime/use-realtime-sync.ts:1341-1349`
  一致。
- 定向测试未观察到新增敏感字段或顶层 workspace 身份泄漏；授权恢复仍要求事件 member 属于当前用户，
  旧授权代次不会复活。
- collection/record 的审计、metrics-only 计数、WS 事件去重和隐私属性未通过真实 Go/DB/WS 执行核验；
  不能将线上看板、跨客户端事件计数或真实传输标为通过。

## 未覆盖项与原因

- `pnpm typecheck` 全仓并行尝试运行约 10 分钟后手动中止：本机 3.8GiB 内存一度仅约 100–200MiB 可用，
  Web/Desktop `tsc` 长时间资源争用；退出码 130。包级 Core/Views typecheck 已独立通过，build 也完成 Web
  类型检查。
- Go/PostgreSQL、`make test`、全量 server collection handler/migration/RLS/rollback：本机无 `go` 和
  `pg_isready`；`make test` 在缺少 `.env` 时先退出，未伪造通过。
- 全量 Views suite、浏览器 E2E、真实 HTTP/WS、交互式 Desktop smoke、CI：无可用服务、账号和测试数据；
  未运行即标为未验证。
- 性能 NFR、10,000 行/20 字段样本、回退演练：未具备隔离服务端环境；未验证。
- 负向 P1/P2 的固定 SHA 页面重现未在本轮重新编写或提交测试脚本；结论依据当前固定 SHA 源码路径、
  未变化的增量范围及上一轮在同一 T2b 路径上的可复现记录，故不把它们伪装成新增自动化通过项。

## 上线风险提示

当前不建议合入或开启 `cortex_collections`：D1 已解除，但 P1 会静默丢失用户未保存内容，P2 会让
分页尾行在冲突后无法完成保存；两者都需要开发修正并由 QA 按固定新 SHA 复验。即使修复后，还必须在
具备 Go/PostgreSQL 的隔离环境补跑服务端契约、迁移/RLS、审计/metrics/WS 传输、全量 Views、浏览器
E2E、Desktop smoke、性能和 CI。

## 实际执行命令

- `pnpm --filter @multica/core exec vitest run realtime/use-realtime-sync-ws-instance.test.tsx`：1 file / 21 passed。
- `pnpm --filter @multica/core exec vitest run realtime/use-realtime-sync-ws-instance.test.tsx -t "restores access from the nested workspace id"`：1 passed / 20 skipped。
- `pnpm --filter @multica/core exec vitest run collections/data-source.test.ts collections/mutations.test.tsx collections/queries.test.ts collections/schemas.test.ts api/client.test.ts platform/session-cleanup.test.ts realtime/use-realtime-sync-ws-instance.test.tsx`：7 files / 145 passed。
- `pnpm --filter @multica/views exec vitest run collections/collections-page.test.tsx collections/collection-detail-page.test.tsx data-view/cell-editor.test.tsx search/search-command.test.tsx`：4 files / 58 passed。
- `pnpm --filter @multica/views exec vitest run issues/components/table-view-editing.test.tsx issues/components/table-view-project-grouping.test.tsx issues/components/table-view-virtualized-hierarchy.test.tsx issues/components/table-group-row.test.tsx issues/surface/issue-surface.test.tsx`：5 files / 41 passed。
- `pnpm --filter @multica/core typecheck`、`pnpm --filter @multica/views typecheck`：通过。
- `pnpm --filter @multica/core lint`、`pnpm --filter @multica/views lint`：0 errors；分别 2/25 warnings。
- `pnpm exec turbo build --filter=!@multica/mobile --concurrency=1`：5 successful / 5 total；Desktop CLI 因无 Go 使用 fallback。
- `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153..HEAD`：通过；固定 checkout 工作树清洁。
- `pnpm typecheck`：因本机资源争用手动中止，退出码 130；不计通过。
- `make test`：因固定 checkout 未配置 `.env` 退出；Go/PG 测试不计通过。
