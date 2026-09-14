## 总体结论

**有条件通过（仅针对本次修正的三条目标路径），阻塞缺陷 0 个。**

验收对象为 PR #5 的固定 head `0a17b8b127692b424e4d467e87846da742bcec24`，base 为冻结的
`5d182a57f5070365577d7420fb8814dce25f8153`；独立 checkout 的提交、父提交和工作树均已核对。

目录后台重读失败时，dirty/pending/failed 编辑状态和 CAS 基线继续保留；实际 200+1 分页的尾行
冲突路径按 `revision 1 → 409 → 单条权威重读 revision 2 → 明确重试成功` 通过；本人撤权后收到
真实嵌套 `member.workspace_id` 的 `member:added` 可恢复新授权代次，旧代次仍失效。未发现新的产品
阻塞缺陷。

该结论不等于完整 T2/G-1 或线上发布验收：本机缺 Go 和 PostgreSQL 工具，真实 Go/DB/HTTP/WS、
浏览器 E2E、交互式 Desktop smoke、性能和 CI 仍未验证。

## 覆盖矩阵

| 需求/门禁 | 用例与实际结果 | 判定 |
| --- | --- | --- |
| 目录失败保稿 | `packages/views/collections/collection-detail-page.test.tsx:593` 的真实页面回归覆盖 dirty、pending、failed 三态；失败时保留表格和草稿，Retry 后恢复，CAS 基线未被替换。实现边界在 `packages/views/collections/collection-detail-page.tsx:197-209`、`:416-452`。 | 通过 |
| 200+1 CAS 冲突重试 | `packages/views/collections/collection-detail-page.test.tsx:686` 真实构造 200 条首屏 + 1 条尾行，首次请求断言 `expectedRevision: 1`，收到冲突后调用单条 `getCollectionRecord`，明确第二次 Save 断言 `expectedRevision: 2` 且写入成功、尾行 revision 变为 3。实现位于 `packages/views/collections/collection-detail-page.tsx:248-287`、`packages/core/collections/mutations.ts:169-214`。 | 通过 |
| 失败后保留原编辑基线 | `packages/views/collections/collection-detail-page.test.tsx:816` 断言首次提交仍使用原 revision，重读完成后只有明确再次 Save 才使用新 revision；共享 editor 的基线/失败意图逻辑在 `packages/views/data-view/cell-editor.tsx:189-229`。 | 通过 |
| D1 重新加入授权 | `packages/core/realtime/use-realtime-sync-ws-instance.test.tsx:578` 挂载生产 `useRealtimeSync`，发送服务端形状的嵌套 `member.workspace_id`；断言 access 恢复、旧授权代次不再有效，且本人身份和 workspace 边界生效。生产接线在 `packages/core/realtime/use-realtime-sync.ts:1342-1363`，服务端发布在 `server/internal/handler/invitation.go:562-566`。 | 通过 |
| Core 定向回归 | 7 文件 145/145：collection data source、mutations、queries、schemas、API client、session cleanup、realtime WS instance。另直接筛选三条目标用例为 3/3。 | 通过（定向） |
| Collections/Editor Views 回归 | 4 文件 59/59：详情、列表、共享 cell editor、search；包含上述三条新增路径及既有 checkbox/number/刷新/授权边界回归。 | 通过（定向） |
| T1/T1b Issue Views 回归 | 5 文件 41/41：Table 编辑、分组、虚拟层级、分组行、Issue surface；运行中有既有 `act`/DOM 属性 warning，无测试失败。 | 通过（定向） |
| TypeScript | `pnpm --filter @multica/core typecheck`、`pnpm --filter @multica/views typecheck` 均通过。 | 通过（包级） |
| Lint | Core 0 errors/2 warnings；Views 0 errors/25 warnings，均为既有 hook/disable 警告。 | 通过（无 error） |
| Build | `pnpm exec turbo build --filter=!@multica/mobile --concurrency=1`：5 successful/5 total。Desktop 因无 Go 使用 CLI fallback，未生成本地 Go 二进制。 | 通过（环境降级） |
| 差异卫生 | `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153..0a17b8b127692b424e4d467e87846da742bcec24` 通过；构建生成的 `apps/web/next-env.d.ts` 已恢复，固定 checkout 工作树清洁。 | 通过 |

## 缺陷清单

本次三条目标路径未复现缺陷，阻塞缺陷 0 个。

## 埋点核验结果

- 本次修正没有新增业务事件；D1 仅修正客户端对已有 `member:added` 协议的读取路径，使用嵌套 `member.workspace_id`，未观察到新增敏感字段或顶层 workspace 身份泄漏。
- collection/record 的创建、更新、审计、metrics-only 计数、WS 传输与去重未在真实 Go/DB/WS 环境核验，不能把线上指标和跨客户端事件计数标为通过。

## 未覆盖项与原因

- `go` 与 `pg_isready` 均不在本机 PATH，因此未执行 Go handler、迁移/RLS、PostgreSQL、真实 API 和数据库回归。
- 没有运行中的认证服务、账号和测试数据，未执行真实 Web API/WS、浏览器 E2E 和交互式 Desktop smoke。
- 未执行全量 Views、全仓 typecheck、性能 NFR、10,000 行样本、回退演练或 CI；本报告不以开发者自测替代这些门禁。

## 上线风险提示

三条修正路径已达到定向验收条件，可进入架构/人工复审；在 Go/PostgreSQL 隔离环境补齐服务端、真实联调、全量 Views、E2E、性能和 CI 之前，不建议将本结果视为完整 G-1 发布批准。

## 实际执行命令

- `pnpm --filter @multica/core exec vitest run collections/data-source.test.ts collections/mutations.test.tsx collections/queries.test.ts collections/schemas.test.ts api/client.test.ts platform/session-cleanup.test.ts realtime/use-realtime-sync-ws-instance.test.tsx --reporter=dot`：7 files/145 passed。
- `pnpm --filter @multica/views exec vitest run collections/collections-page.test.tsx collections/collection-detail-page.test.tsx data-view/cell-editor.test.tsx search/search-command.test.tsx --reporter=dot`：4 files/59 passed。
- `pnpm --filter @multica/views exec vitest run issues/components/table-view-editing.test.tsx issues/components/table-view-project-grouping.test.tsx issues/components/table-view-virtualized-hierarchy.test.tsx issues/components/table-group-row.test.tsx issues/surface/issue-surface.test.tsx --reporter=dot`：5 files/41 passed。
- `pnpm --filter @multica/views exec vitest run collections/collection-detail-page.test.tsx -t 'directory refresh failure|actual 200 plus 1|edited revision' --reporter=dot`：3 passed/10 skipped。
- `pnpm --filter @multica/core typecheck`、`pnpm --filter @multica/views typecheck`：通过。
- `pnpm --filter @multica/core lint`、`pnpm --filter @multica/views lint`：0 errors。
- `pnpm exec turbo build --filter=!@multica/mobile --concurrency=1`：5 successful/5 total。
- `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153..0a17b8b127692b424e4d467e87846da742bcec24`：通过。
