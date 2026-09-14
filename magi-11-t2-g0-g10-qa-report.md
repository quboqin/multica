## 复验范围

- 候选完整 SHA：`79ff5abe591a007702f3e763adb98a3df889a1a3`
- 冻结 base：`5d182a57f5070365577d7420fb8814dce25f8153`
- 已在独立、干净 detached checkout 上复验；`base` 是 SHA 的祖先，base→SHA 共 102 个文件，未使用旧 SHA 结果。
- 复验日期：2026-09-14（Asia/Shanghai）。

## 总体结论

**不通过（T2 G0～G10 准入未满足）**。本轮未复现已知客户端生命周期缺陷；但真实 Go/PG/HTTP/WS、集合浏览器 E2E、迁移回退、性能、可访问性和 Desktop 交互证据缺失，不能把该 SHA 放行为“完整 T2 通过”。阻断准入的是环境/测试证据缺口，不是本轮已确认的新增代码缺陷。

## 覆盖矩阵

| 门禁 | 结果 | 本 SHA 的可核验证据 / 缺口 |
| --- | --- | --- |
| G0 基线与边界 | 通过（静态） | `git diff --check 5d182a57f5070365577d7420fb8814dce25f8153 79ff5abe591a007702f3e763adb98a3df889a1a3` 通过；共享集合页面实际导入 `useDataViewController`、`TableView`、`DataViewCellEditor`（`packages/views/collections/collection-detail-page.tsx:55`）；共享 `packages/views/data-view` 与 `packages/core/data-source` 无 collection 领域导入；9 个 T2 索引迁移均为 `CREATE [UNIQUE] INDEX CONCURRENTLY`，无 `FOREIGN KEY`/`REFERENCES`。强制无缓存仓库 typecheck 9/9 通过；变更文件 lint 0 error/0 warning。 |
| G1 真实持久闭环 | 未验证 | 目标行为要求两集合、三种字段 set/clear、刷新、新 QueryClient、API 重启后经真实 PG/HTTP/Web 恢复。本环境缺 `go` 与 `psql`，未启动 API/PG；该 SHA 也没有契约所列 `e2e/collection-table.spec.ts`。现有 Core/Views 测试的 API/路由是替身，不能计为本门禁通过。 |
| G2 事务与幂等 | 未验证 | 真实并发创建、字段中途失败回滚、同键同/异内容、双窗口 CAS、响应丢失重试均需 Go+PG；`server/internal/handler/collection_test.go:94` 等真实 DB 用例未能执行。静态实现含事务/CAS/幂等路径（`server/internal/handler/collection.go:302-390`, `:515-590`, `:631-735`），不替代联调证据。 |
| G3 租户/权限 | 未验证 | 静态可见现有 membership 路由、`RequireHumanActor` 和 handler 内 human/feature fence（`server/cmd/server/router.go:1857-1877`, `server/internal/handler/collection.go:81-153`）；但两 workspace、双成员/不同用户、跨 collection/record/field/cursor、`mat_`/`mcn_`、撤权/降级/停用/换号的完整认证 router + 浏览器矩阵未执行。 |
| G4 数据库隔离与清理 | 未验证 | 静态确认三表 `ENABLE/FORCE ROW LEVEL SECURITY` 与 workspace policy（`server/migrations/477_collection_rls.up.sql:1-17`），workspace 删除显式清理 record→field→collection（`server/internal/handler/workspace.go:1124-1187`）。非 super/BYPASSRLS、缺 context、连接池 A→B→空、删除并发与 flag 关闭清理需要真实 PG；未执行。 |
| G5 缓存与编辑生命周期 | 通过（客户端范围） | 该 SHA 独立执行 Core 定向 7 文件 146/146、Views 定向 5 文件 87/87；覆盖撤权后晚到成功/失败、403/404、恢复重读边界、同实例来源隔离、dirty/pending/failed 草稿和 CAS 旧 revision 不覆盖新值。代表用例在 `packages/views/collections/collection-detail-page.test.tsx:782`、`:845`，generation fence 在 `packages/core/collections/queries.test.ts:15`。这只证明 React/Query/页面接线范围，不扩展为真实 HTTP/WS/浏览器虚拟滚动通过。 |
| G6 分页与收敛 | 未验证 | 需真实 ≥201 行、默认 50/最大 200/非法 limit、时间戳相同的 id 兜底、游标错配、双客户端 WS、断线和漏事件轮询；集合 E2E 文件缺失，Go/PG/WS 未启动。前端单测的分页/失效替身不计入真实门禁。 |
| G7 无任务副作用 | 未验证 | 需真实 API 建立 1000 records 后核对 issue 数/编号、`agent_task_queue`、`inbox_item` 增量为 0，并捕获实际 subscriber/run-confirm/task API 调用为 0；未执行。静态 handler 仅写 activity、metrics、collection/record WS（`server/internal/handler/collection.go:363-384`, `:565-589`, `:719-734`），不替代 1000 行实测。 |
| G8 存量与兼容 | 未验证（部分已证） | 该 SHA Core 全量 151 files/1781 tests、Views 全量 431 files/5105 tests 通过；强制无缓存 typecheck 9/9、build 5/5 通过，变更文件 lint 通过。Issue 浏览器 E2E、旧 Web/Desktop client schema、畸形 response 的真实装配未验证。Desktop build 因缺 Go 跳过 CLI bundle，仅使用 fallback，不是 Desktop smoke。 |
| G9 迁移与回退 | 未验证 | 静态确认迁移拆分与并发索引，非空 down 保护存在（`server/migrations/466_collections.down.sql:1-11`）；但新库 up、旧基线 up、中断并发索引恢复、空库 down/up、带数据拒绝 down、兼容回退/重新启用和 cleanup 均需隔离 PG+Go，未执行。 |
| G10 观测与性能 | 未验证 | 静态确认 collection/record analytics catalog、metrics-only 注册（`server/internal/analytics/events.go:32-79`）及业务 counter dispatch（`server/internal/metrics/business_events.go:395-400`）。未取得真实审计/metrics-only 负例、1 万行×20 列首屏 p95<1.5s、单行读写 p95<300ms、键盘/只读可访问性或 Desktop smoke 证据。构建中的 CSS/动态 import 警告不构成准入证据。 |

## 缺陷清单

### 阻塞准入条件（环境/证据，非已确认代码缺陷）

1. 本运行环境缺少 Go 1.26.x 与 PostgreSQL 17 客户端/可用隔离数据库，导致 G1–G4、G6–G7、G9 的真实测试及 G10 后端观测无法执行。
2. `e2e/collection-table.spec.ts` 在该 SHA 中不存在；因此集合真实 Web 创建/刷新/WS/分页/副作用场景没有可直接运行的仓库测试入口。
3. 共享 Stage/真实 Desktop、双客户端 WS、压测数据和认证测试账号未提供；G3、G6、G8、G10 仍未验证。

已确认阻塞缺陷数：**0**。未把上述证据缺口升级成代码缺陷，也未把客户端替身或旧 SHA 结果计为通过。

## 埋点核验结果

- 静态通过：事件名 `collection_created`、`record_created`、`record_updated` 已进入 metrics-only catalog；handler 仅在事务 commit 后记录产品 counter / publish，replay 路径不重复记录。
- 未验证：真实 DB `activity_log` 的字段（resource/field/revision/request/actor 且不含值/标题）、回滚/409/replay 计数为 0、Prometheus 标签与结构化日志、实际 subscriber 无 issue/inbox/task 外发；需 G1/G2/G4/G7 的真实环境执行。

## 未覆盖项与原因

未覆盖真实 Go/PG/HTTP/WS、浏览器集合 E2E、Views 全量终态、旧客户端、Desktop 交互、压测/可访问性、迁移中断及带数据回退。原因是本环境缺 `go`、`psql`、隔离运行环境/账号，且候选 SHA 缺少集合 E2E 文件；静态源码与前端单测不能替代这些层级。

## 外部环境最小复验命令

仅在候选 SHA 的隔离 checkout、非生产数据库执行；先满足 `go version` 为 Go 1.26.x、PostgreSQL 服务为 17（CI 拓扑为 `pgvector/pgvector:pg17`）、可用 Docker、两 workspace 的 owner/admin/member 与不同用户凭证、机器凭证 `mat_`/`mcn_`、双浏览器客户端、Desktop 运行环境和 metrics/log 读取权限：

```bash
git rev-parse HEAD
go version
psql --version
make setup-worktree
make up C=api,web
make status
(cd server && go test -race ./internal/collection ./internal/handler ./internal/metrics ./cmd/migrate -count=1)
make test
test -f e2e/collection-table.spec.ts
make env-exec ARGS="-- pnpm exec playwright test e2e/collection-table.spec.ts e2e/issue-table.spec.ts"
make up C=api,web,desktop
```

G9 仅对空的隔离库执行 `make migrate-down && make migrate-up`；含数据的 down 必须验证明确拒绝，再以关闭入口→停止在途→保留三表/RLS/cleanup 兼容构建→重新启用的流程演练。性能需同 SHA/同 PG 拓扑预置 10,000 行×20 列、20 并发并收集至少 1,000 次 API 样本；这些命令和数据条件未在本轮伪造执行。

## 上线风险提示

在 G1–G10 未补齐前，不得进入 T3 后续实现或共享 Stage/生产开放。尤其注意：前端 build/typecheck 通过不能证明 RLS/认证/事务/幂等/无任务副作用；Desktop fallback build 不能证明 Desktop smoke；缺少集合 E2E 不能证明真实刷新、分页、WS 收敛和键盘可访问性。
