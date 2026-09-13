## 总体结论

有条件通过（仅针对 PR #5 `a802a7817f33933e981e5e16a35a381d67fdd1dd` 的定向客户端门禁）。P1 阻塞缺陷 0 个；Core/Views 定向回归、类型检查、lint（0 error）和 `git diff --check` 通过。

页面生命周期仍有 1 个已复现 P2，以及 1 个源码确认但本轮未用独立 DOM 测试复现的 P2 风险，合入前需补齐授权代次和同实例 collection 切换隔离。Go/PostgreSQL、真实 API/WS、浏览器 E2E、Desktop smoke、Views 全量和 CI 未验证。

## 覆盖矩阵

| 需求/门禁 | 用例 | 结果 |
| --- | --- | --- |
| P1 撤权缓存隔离 | Core collection queries/mutations/realtime/session cleanup：撤权后拒绝新读写，旧成功/失败/settled 不回填或失效缓存 | 通过；Core 相关 7 文件 142/142 |
| P1 标题 CAS 基线 | 首次 dirty 冻结 revision 1；refetch 到 revision 2 后首次 Save 仍发 1；409 保留草稿；明确再次 Save 才发 2 | 通过；Views detail/cell-editor 8/8，Core mutations 相关用例通过 |
| P2 列表页卸载 | 创建请求挂起，页面卸载，晚到成功不导航 | 通过；`collections-page.test.tsx` 1/1 |
| P2 创建完成生命周期 | 撤权代次推进后，旧创建成功/失败不得导航、清稿、页面重读 | 不通过；列表页旧成功仍导航，见 P2-1 |
| P2 详情页同实例切源 | A 创建挂起后切换到 B；A 的成功/失败不影响 B；A→B→A 不复用旧意图 | 未验证；源码显示缺少 source generation/key，见 P2-2 |
| Search 兼容回归 | Collections flag-off 时不出现搜索入口，并保持已有命令行为 | 通过；`search-command.test.tsx` 纳入 42/42 |

实际命令与结果：

- `pnpm exec vitest run ...`（在 `packages/core` 配置范围）：7 files / 142 tests passed。
- `pnpm exec vitest run ...`（在 `packages/views` 配置范围）：4 files / 42 tests passed。
- `pnpm --filter @multica/core typecheck`：passed。
- `pnpm --filter @multica/views typecheck`：passed。
- `pnpm --filter @multica/core lint`：0 errors，2 existing warnings。
- `pnpm --filter @multica/views lint`：0 errors，25 existing warnings。
- `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153 a802a7817f33933e981e5e16a35a381d67fdd1dd`：passed。

## 缺陷清单

### 一般（P2）

1. **撤权后的创建页面完成回调仍可导航/清稿/重读。**

   - 环境：目标 SHA，`packages/views` Vitest + jsdom，真实 React Query/页面组件；API 创建请求使用 deferred promise。
   - 步骤：打开 Collections；提交创建使请求挂起；调用生产授权边界 `revokeClientWorkspaceAccess(queryClient, "ws-1")`；返回旧请求成功。
   - 期望：旧完成结果不产生页面副作用，导航、清稿和失败重读均为 0。
   - 实际：`CollectionsPage` 调用 `push("/alpha/collections/collection-1")` 一次。
   - 依据：`packages/views/collections/collections-page.tsx:90` 只检查 operation、挂载状态和 workspace id/slug；`packages/views/collections/collection-detail-page.tsx:234` 与 `:245` 同样没有校验 `isClientWorkspaceAccessGenerationCurrent`。mutation cache 协调虽已受代次保护，但 `mutateAsync` 的页面副作用仍未受保护。
   - 责任建议：开发；让页面捕获创建时的 workspace access generation，并在成功、失败、settled 分支统一校验 generation，或将页面完成副作用收敛到同一受保护协调器。

2. **详情页同一组件实例切换 collectionId 时未隔离旧创建意图。**

   - 环境：目标 SHA，源码审阅；本轮未执行独立 A→B DOM 回归。
   - 步骤：详情页 A 提交创建并保持请求挂起；同一 tab/同一 React 组件位置切换到 B；让 A 成功或失败。
   - 期望：B 不显示 A 的 pending/error，不清除 B 的草稿；A→B→A 不复用已失效的创建意图。
   - 风险依据：`packages/views/collections/collection-detail-page.tsx:119-127` 的 `pendingCreateRequest` 与 `pageActive` 不随 `collectionId` 重置；`useEffect` 依赖为空；`submitRecord` 的完成分支 `:234-257` 只有页面挂载和 workspace 检查，没有 source generation/key。该项标为未验证，不把它计为通过。
   - 责任建议：开发；按 `(workspaceId, collectionId)` 为有状态内容设置 key，或引入 source generation 并让所有旧完成分支失效；补 A→B、A→B→A 成功/失败用例。

## 埋点核验结果

本增量未涉及新增或修改业务埋点；没有可核验事件。服务端 analytics/metrics、真实 API/WS 事件流本轮未运行，不能据此宣称线上埋点通过。

## 未覆盖项与原因

- Views 全量测试未运行；本轮只执行与 PR5 变更相关的定向文件。
- 浏览器 E2E、真实 HTTP/API/WS、Go/PostgreSQL migration/handler 联调未运行。
- Desktop 交互 smoke 未运行；仅完成共享 Views 的静态检查和 Vitest。
- `pnpm build` 与 CI 未作为本轮独立门禁执行。
- 详情页 A→B 同实例切换只完成源码风险核对，尚未完成独立 DOM 复现。

## 上线风险提示

P1 的授权代次和 CAS 保护已通过定向回归；但在撤权列表收敛延迟窗口内，旧创建结果仍可能把用户导航回失权 workspace，或在详情页展示旧请求错误。若支持同 tab 详情切换，旧 collection 请求还可能污染新 collection 的页面状态。修复并补齐上述 P2 回归前，不建议将页面生命周期门禁标为完全通过。
