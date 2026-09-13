## 总体结论

有条件通过 PR #4（`0e16e6845d783e4be606d09cbe08dd04f40899d6`，base `53f49f0f9f120fd67e1f025039dd6a14db504ea5`）。本次四项修正及 T1b 关键路径独立复验通过，未发现已证实的生产功能缺陷；但有 1 项测试门禁阻塞：`packages/views/issues/surface/issue-surface.test.tsx:391`、`:495` 的旧单参数 mock 断言在新 read API 传入 `{ signal, workspaceSlug }` 后失败。修正该测试契约并重新跑完整 Views 回归后，才可解除条件。浏览器 E2E、真实 Web API/WS 联调和交互式 Desktop smoke 因本机缺少 Go 未验证。

## 覆盖矩阵

| 需求/门禁 | 用例与证据 | 结果 |
| --- | --- | --- |
| G3 编辑会话身份 | 生产 TableView 覆盖 A→B 成功/失败、同单元格关闭后重开成功/失败；共享 controller/editor 定向回归 | 通过 |
| G3 失败保稿 | `packages/views/data-view/cell-editor.test.tsx` 覆盖 dirty/pending/failed 与等价 DTO；`packages/views/issues/components/pickers/custom-property-picker.mutation.test.tsx` 覆盖真实 QueryClient、乐观更新→失败→权威重读→原草稿重试；标题等待最终结果 | 通过 |
| G4 工作区与写权限 | `packages/core/issues/table-data-source.test.ts`、`packages/core/properties/mutations.test.tsx`、`packages/views/issues/actions/table-command-executor.test.ts` 覆盖切工作区/撤权后的零新请求及已发请求结果；临时 API smoke 实测 groups/rows/update/property/labels 全部带捕获 slug | 通过 |
| G6 键与缓存隔离 | `packages/core/data-source/query-keys.test.ts`、`packages/views/data-view/branch-model.test.ts`、`controller.branch-identity.test.tsx` 覆盖 null/空串/root/分隔符；样本源覆盖旧 join 碰撞、子分支独立重试、切源清理和新 QueryClient 重挂 | 通过 |
| T1b 生产共享路径 | 生产 Issue TableView 与第四类非 Issue SampleRow 使用同一 controller/TableView；读、分页、分组、编辑、清空、只读和重试 | 通过（定向） |
| T1 共享包回归 | Core 全量 146 文件/1,754 项通过；Views T1b 定向 9 文件/66 项通过；属性过滤 9/9、ListView 6/6 通过 | 通过（定向） |
| T1 IssueSurface 分页基线 | `packages/views/issues/surface/issue-surface.test.tsx` 独立 11/13 通过，2 项断在旧 mock 参数断言 | 不通过（见 D1） |
| 静态与产物门禁 | `pnpm typecheck` 9/9；`pnpm build` 5/5；`pnpm lint` 0 errors（既有 warnings）；`git diff --check 53f49f0...HEAD` 通过 | 通过 |

## 缺陷清单

### 阻塞（测试门禁）

**D1：IssueSurface 分页测试未跟随 read API 的 AbortSignal/workspace 参数更新**

- 环境：Node 22.22.0、Vitest 4.1.0、head `0e16e6845d783e4be606d09cbe08dd04f40899d6`。
- 重现：在 `packages/views` 执行 `pnpm exec vitest run issues/surface/issue-surface.test.tsx`。
- 期望：分页测试校验 cursor 的同时接受 read callback 的第二个 options 参数。
- 实际：测试在 `packages/views/issues/surface/issue-surface.test.tsx:391`、`:495` 失败；生产调用实际为 `(request, { signal: AbortSignal, workspaceSlug: undefined })`，旧断言只匹配一个参数。文件结果 2 failed / 11 passed。
- 影响：不代表已证实用户侧分页错误，但会阻断该回归文件/完整 Views 测试门禁。
- 建议负责人：kiki 开发更新测试 mock 断言（或明确恢复兼容调用形状），再重跑完整 Views 回归。

## 埋点核验结果

T1b 合同声明无新增 analytics 事件；本次未发现新增埋点，因此无新增事件可核验。

## 未覆盖项与原因

- 浏览器 E2E：未运行；`go version` 返回 command not found，`make up C=api,web` 在 prerequisite 阶段因缺少 `go` 退出。
- 真实 Web API/WS 联调：未运行；本地 Go-backed API 栈无法启动。
- 交互式 Desktop smoke：未运行；同一 Go 缺口，build 使用了仓库既有 CLI runtime fallback。
- Views 全量 `pnpm test`：未作为通过结论；全量进程因耗时过长主动停止，关键未改动失败文件已单独复跑，其中 IssueSurface 的 2 项 D1 可稳定复现。未把未完成全量标为通过。

## 上线风险提示

- 合并前需处理 D1，否则 Views 回归门禁仍红；该项是测试契约更新，不是本次已确认的生产行为缺陷。
- 真实服务端权限、工作区路由、WS 失效和 Desktop 运行时仍需在具备 Go 与隔离数据库的环境完成验收；静态测试和 API mock 不能替代这些门禁。
- 本 PR 仍只完成 FR-002 的 T1b 部分，不代表 Cortex G-1 全部完成；collection/doc 服务端模型在后续 T2/T3。
