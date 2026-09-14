总体结论

有条件通过（E2E 规格与静态门禁交付）。复审列出的 4 项问题已修正，并补齐 G1/G3/G6/G7 在当前规格内可证明的断言。未发现产品阻塞缺陷；但本次运行环境缺少 Chromium、后端/API 与 PostgreSQL 工具，4 个 Playwright 用例均未进入业务步骤，因此运行态验收仍未完成，不得据此宣称 T2 完成或推进 T3。

覆盖矩阵

| 需求/门禁 | 用例或断言 | 结果 | 说明 |
| --- | --- | --- | --- |
| 复审项 1 | 页面初始化后再监听；区分只读 shell 请求与任务副作用；操作后核对 PG 前后快照 | 静态通过；运行未验证 | `e2e/collection-table.spec.ts` 保留真实页面入口，非读任务端点仍要求零调用 |
| 复审项 2 | 真实 Note Save 触发 409；断言 draft 保留、恢复 GET、expected revision、record/field 与最终 GET | 静态通过；运行未验证 | 仅使用生产实际存在的 Save/Clear 控件，不依赖不存在的 Retry 控件 |
| 复审项 3 | Note/Quantity/Checked 的 set、clear、空值/0/false；每次按精确 collection/record/field/op 绑定 PATCH，并用 GET/刷新核对 | 静态通过；运行未验证 | 分别断言 key 存在且值正确，或 clear 后 key 不存在 |
| 复审项 4 | 独立 member BrowserContext；真实 `/api/me` 与 workspace members 身份/角色核对；跨工作区/权限断言 | 静态通过；运行未验证 | member 会话只注入 member token，未复用 owner context |
| G1 | 两个 collection 的记录/字段隔离；set/clear 持久化；直接 API 写入 | 静态通过；运行未验证 | 真实 Web/API 路径与刷新后的 UI/GET 均有断言；API 重启恢复未验证 |
| G3 | member 创建/编辑拒绝、跨 workspace 访问拒绝与资源隔离 | 静态通过；运行未验证 | 当前规格覆盖 member 子集；撤权/降级/停用/换 session 未覆盖 |
| G6 | 首页 200 条、尾页 1 条、cursor 边界、总数、无重复；尾行 revision conflict 恢复 | 静态通过；运行未验证 | 首尾查询按真实 cursor 精确匹配；双客户端 WS 断线/漏事件轮询未覆盖 |
| G7 | 通过真实 API 创建 1,000 条记录；任务/issue/inbox 副作用为零 | 静态通过；运行未验证 | 断言记录数、唯一 ID、collection 归属，以及 PG issue/issue counter/task/inbox 前后快照相等 |

缺陷清单

- 阻塞：0 个已确认产品缺陷。
- 环境阻塞：Chromium executable 不存在；本地 `make status` 报告没有已注册环境；`go` 与 `pg_isready` 不可用。该项阻止运行态验收，不判定为产品缺陷。

埋点核验结果

本次需求未提供可核对的埋点事件、必填属性、去重规则或看板查询口径。未新增埋点断言，埋点结果为未验证；不能把网络请求监听误作业务埋点核验。

未覆盖项与原因

- Chromium 启动、后端/API、PostgreSQL 前后快照的实际执行：环境不可用。
- API 重启后的 collection/record 恢复：没有可用后端/测试环境。
- G3 撤权、降级、停用、切换登录 session：当前规格只包含 member 权限子集。
- G6 双客户端 WebSocket 断线、漏事件与 polling recovery：当前规格未提供第二客户端/断线控制。
- G7 issue number、task queue、inbox 的独立 API 创建结果：保留并强化了零副作用监听与 PG 快照，但未在运行环境执行。
- Desktop、移动端、性能阈值、CI：不在本次可执行范围内。

验证记录

- 通过：E2E TypeScript 直接编译（`e2e/collection-table.spec.ts`、`e2e/fixtures.ts`）。
- 通过：Playwright `--list`，共 4 个用例。
- 通过：`pnpm exec turbo typecheck --filter=!@multica/mobile --force`，9/9，0 cached。
- 通过：`pnpm --filter @multica/views lint`，0 errors；保留既有 warnings。
- 通过：`git diff --check`。
- 未验证：`pnpm exec playwright test e2e/collection-table.spec.ts`；4 个用例均因缺少 Chromium executable 在启动阶段失败，未使用 mock/skip/response spoof 替代。

上线风险提示

当前只能交付测试规格与静态校验结果。T3 应继续暂停；需在与提交 SHA `b1c761a99b5b6edf89be9adcea34bcdad9500e5e` 匹配的 Web/API/PG 环境中安装 Chromium、准备测试账号与数据库后，重新执行完整 E2E，并补做上述未覆盖项。当前运行分配分支 `agent/momo/magi-11` 与远端 `feature/cortex-g1-collections` 已确认同指该 SHA。
