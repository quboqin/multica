# Cortex P2a 实现与本地验收

日期：2026-09-21。分支：`codex/cortex-g1-ui-rework`，基线 `f02021fea`。本轮改动尚未提交；以下是工作树的验证结果，用户人工验收待确认。

## 实现范围

| 需求 | 本轮补齐 |
| --- | --- |
| FR-021 | 项目详情和建表面板可选择项目；图标、描述编辑；已归档表格列表与恢复；已归档字段列表与恢复。字段恢复保留原值，检查重名和 50 字段上限。 |
| FR-025 / AC-1 | UTF-8 CSV 预校验及最多一万行原子导入；选中最多 500 条匹配记录，批量修改字段、软删除、恢复。回收站保留 30 天。 |
| FR-066 中的 P2a 项 | CLI CSV 预校验/导入、批量修改/删除/恢复、表与字段恢复、图标与描述；数字/日期比较、文本包含、关联过滤和关联目标名称排序。 |

已有表的 CSV 追加使用已有字段，支持 text、number、select、multi_select、date、checkbox、url、actor、multi_actor。选项可以用名称或 ID，多值单元格使用 JSON 数组，成员使用 `member:用户ID`。不自动建字段，不做 upsert；关联边继续使用 `record link`。同一字段的多个过滤条件取 OR，不同字段取 AND；关联排序使用仍可见目标名称中的最小值。

导入会重新校验整份文件；校验失败不写入。批量写入在单个事务中锁定字段目录与所选记录，只合并指定字段，任一错误整批回滚。网页提交每行的预期版本；CLI 可用 `--expect-revision` 覆盖所有选中行以防覆盖。CSV 和批量成功后发布 `collection:updated`，刷新工作区 Query 缓存。

表结构权限仍按创建者或工作区 owner/admin；智能体以运行时主人的权限判定。actor 字段仍只引用成员，文档批准流程不变。无需新增迁移；SQL 查询已通过 `make sqlc` 重新生成。

## 已执行验证

| 检查 | 结果 |
| --- | --- |
| `pnpm typecheck` | 10/10 workspace 任务通过，包含 web、desktop、docs；四种语言 CLI MDX 编译通过。 |
| `pnpm lint` | 7/7 workspace 任务通过；已有 warnings，无错误。 |
| Core API client / schema Vitest | 215 项通过；包含错误批量响应、缺少导入预览标志和工作区隔离的回归测试。 |
| Views collections / cortex / project detail / locale parity | 11 文件、271 项通过；文案增补后另跑 212 项 locale parity，通过。 |
| Go handler `TestCollection\|TestAgentToken` | 通过。CSV 覆盖一万行 × 20 字段、九类字段、中文/逗号/换行、选项和成员；500 行修改、冲突回滚、删除/恢复、恢复权限及关联过滤/排序通过。 |
| Go `./cmd/multica ./internal/cli ./internal/service` | 全部通过；技能契约已更新为可恢复表和字段。 |
| Playwright `e2e/cortex-p2a.spec.ts --workers=1` | Chromium 3/3 通过：一万行导入、500 行修改/删除/恢复；项目内建表；图标/描述和表/字段恢复。 |
| `make build` | server、multica、migrate 构建成功。 |
| 真实本地 API + 新编译 CLI | 25 条命令通过；CSV 预校验/导入、比较过滤、版本保护的批量修改、删除/恢复、按名称恢复表和字段；原值保留。 |
| 验收页面与手册 | 已查看示例表截图；两份 HTML 渲染正常，源文件 ID 与内部锚点检查通过。 |

此次没有运行 `make test` 全量、全仓 `pnpm test`、移动端、原生 Electron 或真实智能体运行。上述自动化结果不替代人工验收，也不表示 P2b、知识库、Workflow、审计和埋点已经完成。FR-066 的 mention 输出降级仍随 FR-031 留在 P2b。

## 本地验收入口

环境为本 checkout 的 `multica-703`：Web `http://localhost:13703`、API `http://localhost:18783`。用 `dev@localhost` 和本地验证码 `888888` 登录 `dev` 工作区。

- [打开 P2a 验收表](http://localhost:13703/dev/collections/55bf76b6-1690-433a-aa58-b64483ab4c0a)：600 行、9 类字段，所属项目为“P2a 手工验收”。
- [打开验收项目](http://localhost:13703/dev/projects/f30169fe-6c99-4e75-88b2-cdceb3ae77c9)：可从项目详情新建表格。
- 本机 CSV 文件：`/tmp/cortex-p2a-import.csv`，一万行，使用该验收表的字段名称和当前测试成员 ID。临时文件只用于本机验收，不随 Git 发布。
- 逐步操作和预期结果见 [LOCAL-MANUAL-TEST.md](../../../LOCAL-MANUAL-TEST.md) 的 P2a 章节。优先确认 CSV 错误整份不写入、500 行批量操作、双窗口版本冲突、表与字段恢复后值不丢失。

服务管理使用 `make status` / `make up` / `make down`；停止保留数据。若要让既有守护进程使用新 CLI 和技能，在没有进行中的任务时运行 `make daemon`。本次只验证新编译 CLI，没有发起智能体任务。

## Excel 导入交互修正（2026-09-21）

用户确认的目标是“上传 Excel，自动转成 Collections 中的新多维表格”。新增列表和空状态入口“导入 Excel / CSV”；上传后读取工作表、表头，预览字段名、类型和示例值，支持选名称列、改字段类型、跳过列和选择项目。确认后 `POST /api/collections/import` 在一个事务里创建 collection、字段和全部记录；预览及失败不留下空表。此前的表内入口改为“追加 CSV 记录”。

Excel 使用服务端 Excelize 解析（[官方文档](https://xuri.me/excelize/en/)），不在浏览器加载整套解析库。支持 .xlsx 和 UTF-8 CSV，24 MiB / 1 万行，解压总量上限 64 MiB。每次选择一个工作表；自动识别保持保守，前导零、超长数字、混合列保留文本。公式只读取缓存结果，无结果时保留公式文本；不执行公式或宏，不保留排版/图表。预览可选择文本、数字、勾选、日期、网址、单选；成员和关联字段在导入后配置。

回归测试包含空首工作表切换、日期与勾选、前导零与长编号、重复表头、公式、CSV 一万行边界、错误转换不留空表、选择单选与跳过列、工作区上传头与错误响应校验。浏览器通过真实 Excel 样例验证创建和刷新；详见 `e2e/collection-import.spec.ts`。

本次修正验证结果：一万行 × 20 字段的真实 Excel 读取通过（纯解析包测试约 2.8 秒）；新导入浏览器测试 2/2、原 P2a 浏览器回归 3/3、相关 handler 测试、122 项 API 测试、268 项共享界面测试通过；typecheck 10/10、lint 7/7 通过。已检查 1440px 和 390px 预览截图，窄屏可滚动字段并操作固定底部按钮。原生 Electron、移动端和真实智能体运行未测。

本地保留了 [Excel 导入验收 — 客户](http://localhost:13703/dev/collections/5fc80a9a-9581-4a37-bc06-a890a7f4daa6)，3 行、6 个字段，已刷新确认保留。样例文件是 `e2e/fixtures/cortex-customers.xlsx`（选择 Customers 工作表）；这是合成测试数据。
