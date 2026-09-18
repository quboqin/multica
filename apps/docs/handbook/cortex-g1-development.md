# Cortex G1：5.1 / 5.2 P0 开发与验收

分支：`codex/cortex-g1`，基于 `36e789910`。
需求来源：[共享引擎](cortex-prd.html#fr-engine)、[文档](cortex-prd.html#fr-doc)。
原 `apps/docs/public/handbook/` 已迁移到 `apps/docs/handbook/`。

本轮交付范围为 FR-001～005、FR-011～017。代码与本地测试已准备，产品验收等待用户确认。
集合的基础目录、字段、记录和保存视图用于共享引擎的双源验收；不将其等同于 5.3 的全部集合功能。

## 实现与验证对应

| 需求 | 实现 | 验证依据 |
| --- | --- | --- |
| FR-001 | 九类字段共用类型目录、Go 校验和过滤语义；任务字段仍由人类 owner/admin 管理、上限 20；集合创建者或人类管理员管理字段、独立上限 50；单字段 JSONB 原子写入，支持预期值检查 | 既有 property 测试；`collection_test.go`：当前记录读取限定集合、不同字段并发不覆盖、同字段冲突、独立配额、未知操作符报错、权限 |
| FR-002 | `DataSource` 提供工作区/对象身份、字段及布局/编辑/排序/副作用能力；表格、日历、画廊为共享渲染器；Query 持有服务端数据，Zustand 仅持有偏好与草稿 | 引擎依赖边界检查；分页/分组后端测试；草稿与 HTTP/实时事件乱序测试；编辑中的记录独立读取最新值，即使已移出筛选/当前页；任务 1,001 行浏览器回归 |
| FR-003 | `issue_view.collection_id` 可空；NULL 继续指任务视图，私有视图授权和旧偏好保留；新任务视图保留创建倒序、隐藏 cancelled、非 status 分组不手动排序 position 的规则 | schema 默认值、既有 view/store/surface 测试；集合视图不进入任务列表；私有视图拒绝他人读取；任务列宽刷新测试 |
| FR-004 | 任务与集合均支持日历、月/周切换、选择日期字段、拖拽回写；服务端按日期窗口过滤 | 双源 Chromium 拖拽并刷新确认；日期计算单测 |
| FR-005 | 双源画廊共用卡片渲染器，可配置 URL 封面字段和展示字段 | 渲染器测试；双源浏览器切换；从任务卡片进入同一任务 |
| FR-011 | `issue.kind` 默认 task，支持 doc / knowledge / workflow_run；任务列表和任务缓存排除文档 | migration、响应 schema、服务端 payload 一致性、创建文档和任务兼容测试 |
| FR-012 | 文档树支持跨层拖动、同层调序、根节点移动；事务锁与数据库保护防止循环或跨工作区/项目父子关系 | 五层与 50 页后端测试；子树移动、刷新后的面包屑浏览器测试 |
| FR-013 | 工作区/项目范围、树折叠、收藏、最近访问 | 浏览器移动、收藏、最近访问；偏好按工作区保存 |
| FR-014 | Web/Desktop 共用 TipTap 与线程；斜杠插入保存视图，节点只保存引用并重新鉴权；Mermaid/公式/附件链接往返；失败保留草稿；正文 mention 不派发，评论保留已有预览及 /note 规则 | 编辑器序列化、保存竞争与合并测试；真实 handler 的正文/评论 mention 测试；复杂正文打开不产生假草稿的浏览器回归 |
| FR-015 | 既有全文搜索增加 kind 服务端过滤；文档独立分组、“只看文档”开关和文档路由 | 搜索 SQL 排名兼容测试、handler 检索测试、33 个搜索界面测试 |
| FR-016 | draft / reviewing / published 分别映射 unstarted / started / done；保留 assignee、隐藏 priority；人工提交评审，owner/admin 对精确正文版本发布；正文变更回到草稿并取消旧入库请求 | 未授权、未评审、过期版本和通用状态写入拒绝测试；发布审计与 pending 入库请求；浏览器提交评审/发布 |
| FR-017 | 正文版本与任务 revision 分开；所有 API 写入必须带 `expected_document_revision`；数据库防止旧服务无版本覆盖；CLI 显式版本参数；冲突比较、编辑合并、重试 | 双写入者只有一方成功、无版本 409、DB 绕过拒绝、CLI 不自动采用新版本、自回显/迟到附件、断线刷新和迟到 HTTP 响应测试 |

发布会写入带 actor、正文版本及正文快照的持久入库请求。实际知识抽取、索引和检索消费属于 5.4，本轮不提供该消费者；pending 不代表已经完成知识入库。
P1 的段落软锁、版本历史和实时协同编辑也不在本轮。

## 本地测试环境

- 环境：`multica-703`；独立数据库：`multica_multica_703`。
- 登录：`dev@localhost`，本地验证码 `888888`。
- [示例文档](http://localhost:13703/dev/documents/01a0b439-edd0-7ec1-8749-9f3d0c63002f)
- [第五层页面](http://localhost:13703/dev/documents/01a0b439-ee56-7f32-9963-b984d21ce9e1)
- [双源验收集合](http://localhost:13703/dev/collections/fe55369a-8d17-45d0-be2e-3413c88b5135)
- [任务视图](http://localhost:13703/dev/issues)
- API：`http://localhost:18783`。

示例包含五层树、另一个可拖入的父页、含 Mermaid / 公式 / 嵌入保存视图的正文、九类集合字段、12 条记录和带日期的任务。示例没有智能体指派。
服务保留运行以供验收。验证时健康接口中的 commit 为改动提交前的基线 SHA；后端已从工作树重新构建，前端由同一工作树开发服务提供。

```sh
make status
make up      # 环境停止后重新启动
make down    # 停止服务并保留数据
```

### 请按以下步骤确认

1. 打开示例文档，确认已有嵌入表格、Mermaid 和公式。编辑正文，等待“已保存”，刷新确认内容。
2. 展开第五层页面，将子页拖到另一父页，刷新后检查树和面包屑；再试同层调序、收藏、最近访问。
3. 在同一文档的两个窗口编辑：第二个窗口离线后保留草稿，第一个窗口保存；恢复第二个窗口网络并保存，应出现当前版本和本地草稿比较，显式合并重试后才保存。
4. 正文保存完成后提交评审并发布；重新编辑发布后的正文，应回到 draft。普通成员不能执行发布。
5. 在全局搜索中搜索示例标题或正文，确认文档分组、“只看文档”过滤以及点击后的文档路径。
6. 在集合中编辑几种字段，切换 Calendar，选择 Date 字段，拖动记录改期后刷新；切换周/月及 Gallery，配置展示字段。
7. 在任务页切换 Calendar，拖动 `G1 Review — calendar task` 并刷新；切换 Gallery 打开它，再回 Table 检查列宽、排序、过滤和分组。

## 验证记录

2026-09-18，本地执行记录：

- `pnpm test --concurrency=1`：6/6 工作区测试任务通过。后续并发修复后重新运行 core 全套，160 个文件、1,956 个测试通过；views 全套此前为 436 个文件、5,267 个测试通过。
- 编辑器最终修复后，`pnpm --filter @multica/views exec vitest run editor documents data-view locales/parity.test.ts --maxWorkers=2`：80 个文件、1,579 个测试通过。
- `pnpm typecheck`：10/10 通过；`pnpm lint`：7/7 通过，保留原有 warnings，无错误。
- `make test` 的常规 Go 包含 race 检查全部通过，包括 handler、service、CLI、迁移和数据库相关测试；agent 包在并发负载下有 7 个超时/迟到输出用例失败；独立执行 `make env-exec ARGS='-- bash scripts/test-go.sh --race --only agent'` 后，agent 全套通过（185.2 秒）。
- 最后增加筛选外编辑记录读取后，`TestCollection` 后端回归（含 race）通过，覆盖按记录 ID 读取与集合权限隔离。
- Chromium：`make env-exec ARGS='-- pnpm exec playwright test e2e/cortex-g1.spec.ts e2e/issue-table.spec.ts --workers=1'`，最终 11/11 通过（1.4 分钟），包含双源拖拽持久化、文档冲突/发布、复杂正文无假草稿、树移动、筛选外编辑记录冲突恢复、千行分组、游标更新和失败重试。此前一轮文档刷新后的编辑器等待超时，其余 10 个通过；未修改用例或断言，重新执行完整 11 个用例后全部通过。
- 当前 review 示例通过真实 API 创建，页面已截图检查，最终浏览器 smoke 无 page errors。
- `git diff --check` 通过。sqlc 已重新生成；迁移 500～509 已在本环境应用，新增并发索引各用独立文件且已注册失败清理。

此前全量并发运行出现过已有 shell/打包测试超时；限并发后的前端全套通过。Go 默认测试始终使用仓库的 CLI guard 与测试创建的假执行器，没有调用用户安装的真实智能体。

尚未执行：生产打包、原生 Electron 手工 smoke、移动端 UI、跨机器部署与 CI。用户手工确认仍是独立验收步骤。
