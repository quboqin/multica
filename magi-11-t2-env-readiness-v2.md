环境准备就绪清单（脱敏）

## 总体结论

部分就绪，不能开始 T2 G0～G10 正式准入验收。新授权目录此前不存在，父级可由当前普通用户创建；已在该目录建立独立 checkout，未读取、覆盖或删除原先占用目录。API、隔离 PostgreSQL 17、Chromium 和 Linux Desktop renderer 已具备可复核条件；Web 仍未就绪，测试账号/Stage、性能规格和完整门禁仍未验证。

环境准备本身不计作 G0～G10 通过；T2 未通过前不启动 T3。

## 固定版本与目录

- 环境目录：`/home/openclaw/multica-test-envs/magi11-t2-b1954bf-01a09f65`
- checkout：`feature/cortex-g1-collections` 克隆后 detached
- 固定完整 SHA：`b1954bf42cb6a095eaea337b3ab5921312477e54`
- `git status`：仅用户级 `.tools/` 未跟踪目录；未改业务代码、未提交或推送

## 依赖与资源

| 项目 | 实际结果 | 判定 |
| --- | --- | --- |
| Node | v22.22.0 | 已就绪 |
| pnpm | 10.28.2 | 已就绪 |
| Go | 用户目录内 go1.26.6 linux/amd64 | 已就绪 |
| 项目依赖 | `pnpm install --frozen-lockfile` 成功，11 个 workspace 完成安装 | 已就绪 |
| Playwright | 1.58.2；Chromium/Headless Shell 145.0.7632.6 已下载 | 已就绪 |
| Docker | Engine 29.7.2；Compose v5.5.0 | 已就绪 |
| 主机 | Linux；2 logical CPU；3.8 GiB RAM；3.8 GiB swap | 低于性能基准 4 vCPU/8 GiB |

## 服务与数据

| 项目 | 实际结果 | 判定 |
| --- | --- | --- |
| PostgreSQL | Docker PostgreSQL 17.11；独立数据库 `multica_magi11_t2_b1954bf_01a09f65_169` | 已就绪 |
| API 连接角色 | 实际连接为专用 `magi11_api`，`rolsuper=false`、`rolbypassrls=false` | 环境条件已满足；不是 G4 通过 |
| Cortex 表 | `collection`、`collection_field`、`record` 均 `RLS=true`、`FORCE RLS=true` | 结构预检完成；不是 G4 通过 |
| 数据库迁移 | 候选 SHA 全部迁移完成（最高记录 477） | 仅环境准备证据 |
| API | `http://localhost:18249`；`/health` 返回 200，commit 短 SHA `b1954bf42`，pid `1205196` | 已就绪 |
| Web | `http://localhost:13169` 未持续监听 | 未就绪 |
| Desktop renderer | `http://localhost:5343` 返回 200；launcher `1206961`、renderer `1207142` | renderer 已就绪；未做 Desktop smoke |
| WS、真实登录、Stage | 未执行连接/账号/Stage 验收 | 未验证 |

## Web 阻断证据

1. 无额外堆配置执行 `make up C=api,web` 时，首页首次编译触发 Node 原生 OOM，环境脚本停止 Web。
2. 使用用户级 `NODE_OPTIONS=--max-old-space-size=2560` 重试仍未通过环境脚本的 Web 启动检查。
3. 使用 `--max-old-space-size=3072` 重试时首页实际返回 `GET / 200`，但环境脚本无法证明监听进程属于本环境，随后按安全策略停止该进程；当前 Web 仍为 stopped。

这不是产品缺陷判定，也不是 Web 功能通过；恢复条件是提供更高资源的主机，或由实现/环境负责人修复该启动进程归属问题并重新交回同 SHA 的 Web 证据。当前主机规格不满足既定性能基准。

## 持久日志与停止方式

- 环境登记：在 checkout 根目录运行 `make status ARGS=--json`，以输出的 `logs` 字段为准。
- 已生成的日志文件：`migrate.log`、`api.log`、`web.log`、`desktop.log`。
- 当前环境保持运行，供后续复核；停止且保留数据库：`make down`。
- 不要执行 `make destroy`，除非明确授权删除本隔离环境及数据库。

## 未覆盖项与恢复入口

以下均保持“未验证”，不能计入 G0～G10：真实浏览器 collection/record 全流程、Go/PG/HTTP/WS 并发与恢复、跨租户权限、API 重启恢复、record 零任务副作用、迁移回退、审计/metrics、可访问性、性能、共享 Stage 兼容性，以及完整 T2 报告所需的账号夹具。

恢复 Web 后，先在对应 checkout 执行：

```bash
PATH=/home/openclaw/multica-test-envs/magi11-t2-b1954bf-01a09f65/.tools/go/bin:$PATH GOTOOLCHAIN=local make status ARGS=--json
make env-exec ARGS="-- pnpm exec playwright test e2e/collection-table.spec.ts e2e/issue-table.spec.ts --project=chromium --trace=retain-on-failure"
```

上述命令只是恢复入口；每个 G0～G10 仍需按 T2 契约分别给出真实证据，不能因服务就绪而自动通过。
