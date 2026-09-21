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

## 2026-09-19：按 UI 原型返工与集合修复

- 文档（S2/S3）：新增文档与表格共用的左侧导航；文档页改为面包屑与状态操作栏、居中正文、右栏“大纲 / 反向引用 / 版本 / 评论”；插入活视图改为选择弹窗（数据源、已保存视图、嵌入方式、Markdown 落盘预览）。`IssueDetail` 增加 `variant="document"`，只隐藏任务页外壳，保存、冲突和评论沿用原逻辑。
- 集合（S4/S5/S6）：视图标签与视图栏（布局、过滤、分组、排序、显示），单元格内编辑（Multi-select 可在表格内连续勾选），表头字段菜单（类型安全转换、编辑选项、排序、分组、过滤、隐藏、归档、字段配额），新建行、看板、日历与画廊样式、行详情侧栏、回收站。
- 修复：Multi-select 在 Table 中无法编辑；Select 选项无法修改。同一单元格的连续写入按顺序提交，冲突时保留用户的值并提供显式覆盖。
- 后端：`PATCH /api/collections/{id}`、`PATCH /api/collections/{id}/fields/{fieldID}`（改名、安全类型转换、选项增删改，删除的选项同步从记录中清除、位置、归档），`DELETE /records/{id}`、`POST /records/{id}/restore`、`GET /trash`（保留 30 天），记录创建可带初始字段值，记录列表支持 `sort_by` / `sort_dir`，集合列表返回 `record_count`。无新迁移。
- 未包含：relation（关联任务 / 关联表）、CSV 导入、“转为任务”、完整版本历史（P1）。

## 2026-09-20：文档与表格各开一列

- 设计：侧栏里的 Documents、Collections 各只有一个入口，不在侧栏内展开；点击后在侧栏右侧新增第二列。原型 S1 增加这条规则，S2–S6 改为“侧栏 + 第二列 + 内容”。
- 实现：原先文档树与表格列表共用的一列拆成两个导航。文档页（`/documents`、`/documents/:id`）的第二列只有文档树；集合页（`/collections`、`/collections/:id`）的第二列只有表格列表，带按名称搜索、记录数和 `+` 新建。
- `/collections` 首页改为“表格列表 + 空状态”，与 `/documents` 一致；表格页面包屑的“表格”可返回首页。
- 侧栏在 `xl` 以下自动收起时第二列保留，列顶的按钮可重新打开侧栏。`md` 以下只容纳一列：首页显示这一列，打开文档或表格后由内容取代。

## 2026-09-20：添加 / 修改字段改为表头下的面板

- 设计：参考飞书多维表格，原型 S4 增加两幅补充画面 —— 新建表后点表头 `+` 添加字段、从表头菜单“修改字段”进入同一个面板。
- 实现：原居中弹窗改为锚在表头格下方的面板（名称、字段类型、单选 / 多选的选项、配额、取消 / 创建字段或保存）。`+` 格占用表格剩余宽度，始终紧跟最后一个字段；表里还没有字段时写成“+ 新建字段”。表头菜单的“字段类型”“编辑选项”合并为“修改字段”。页脚“字段 N / 50”里的“新建字段”打开同一个面板。
- 接口不变：仍是 `POST /api/collections/{id}/fields` 与 `PATCH /api/collections/{id}/fields/{fieldID}`；修改时类型只列安全转换，字段满 50 个时“创建字段”不可用。
- 未包含：默认值、字段捷径、AI 字段、关联 / 查找引用（服务端暂无对应能力）。

## 2026-09-20：首列改名、无表头布局的字段入口、列表里的删除

三个问题，各自的原因与改法：

- **首列「名称」无法修改。** 首列显示的是记录标题（`record.title`），不是字段目录里的字段，表头原先只是一段文字。现在它和其他列一样是菜单：修改字段（标「文本」）、升序、降序；「修改字段」打开同一个字段面板，只能改名 —— 类型下拉禁用并说明原因，没有配额行，也不提供隐藏、归档、分组。列名存在表上：新增 `collection.title_name`（迁移 `510_collection_title_name`，`TEXT NOT NULL DEFAULT ''`，≤ 32 字符），`PATCH /api/collections/{id}` 接受 `title_name`，校验与字段名一致；留空或改回默认名则存空值，继续按界面语言显示「名称 / Name」。排序菜单里的首列同步用这个名字。
- **之前创建的表无法添加 / 修改字段。** 布局是浏览器里记住的偏好，那张表停在「看板」；看板、日历、画廊没有表头，表头上的 `+` 与列菜单随之消失，而没有单选字段的看板只显示一句提示。现在：① 看板 / 日历的空状态带「新建字段」按钮，打开字段面板并预选缺的类型（单选 / 日期）；② 视图栏「显示」里每个字段右侧有铅笔、底部有「新建字段」，面板锚在「显示」按钮下；日历没有可显隐的内容，管理者看到的是同一入口、名为「字段」的列表。以上入口只对表格管理者（创建者、owner、admin）显示。
- **文档树与表格列表没有删除。** 两个第二列的每一行在悬停 / 键盘聚焦时出现行内操作（平时不占宽度，触屏始终显示）：文档为收藏、添加子页、「···」；表格的记录数让位给「···」。菜单里只有红色的删除项，点击后弹确认框，服务端返回前保持打开。文档走既有的 `DELETE /api/issues/{id}`（子页不会一起删，服务端把它们移到根层级，确认框在有子页时写明）；表格走既有的 `PATCH /api/collections/{id}` `{"archived": true}`，目前没有恢复入口，确认框如实写「不可撤销」，只对管理者显示。删除的是当前打开的那一项时，成功后回到文档 / 表格首页。
- **补充（同日）：首列表头“点了没有菜单”。** 菜单的点击区原先只有表头左侧的图标和文字；首列宽 240px 而列名很短，点在空白处没有任何反应。集合表格的表头改为整格可点（`DataViewColumnHeader` 新增可选的 `fillCell`，任务表格不受影响），悬停时整格变色并在右端显示下拉箭头，拖动手柄与列宽分隔线不变。另外「显示」的字段列表把首列排在第一行（始终显示、不可勾选），管理者可从它右侧的铅笔改名，不依赖表头。
- 兼容：`title_name` 在客户端 schema 里缺省为空字符串，旧服务端不返回该字段时首列照常显示默认名；旧服务端会以 400 拒绝带 `title_name` 的 PATCH（请求体不接受未知字段），面板会原样显示该错误。
- 需要执行迁移并重建后端：`make down && make up`。
- 验证（2026-09-20，隔离环境）：`@multica/views` 全套 445 个文件中 440 个通过；失败的 8 个用例里，5 个在 `issues/surface/`（基线提交上同样失败，与本次无关），另 2 个文件在满载下超时、单独重跑 154 个用例全部通过。`collections/`、`cortex/`、`documents/` 与 locale parity 全部通过；core 的 `collections`、`api` 共 426 个用例通过；core / views 的 `tsc --noEmit` 与 eslint 无错误。Go：`go build ./...`、`go vet`（handler、db）、`go test ./internal/handler -run 'Collection|Record|Document'`、`go test ./cmd/migrate` 通过，数据库已应用迁移 510。新增的三段 e2e 步骤用同样的定位器、经真实页面组件对真实 API 走通（字段创建 201、`title_name` PATCH 200 且刷新后保留、非法列名 400 留在面板里、文档 DELETE 204 且子页 `parent_issue_id` 为空、表格归档后 GET 404）；**Playwright 的 `e2e/cortex-g1.spec.ts` 本身尚未通过 Next 前端执行**，请在本机运行 `make env-exec ARGS='-- pnpm exec playwright test e2e/cortex-g1.spec.ts --workers=1'`。

## 2026-09-20：FR-028 关联字段、`record_link` 与「转为任务」

范围：FR-028（relation 字段、关联边落表、反向引用）、FR-02A 里“关联 / 新建任务出口”、AC-8、7.4 的“目标 issue 被删除”降级、D-3。界面对照原型 S4 的两屏“关联字段”与 S6。

- **数据模型。** 新表 `record_link(id, workspace_id, collection_id, from_record_id, from_field_id, to_type, to_id, created_at)`，`to_type ∈ {issue, record}`，无外键；工作区删除时一并清理（`DeleteWorkspace` 与删除清单测试已登记）。关联的值**只**存在这张表里，不写进 `record.fields`：目标一侧才能反查，rollup 以后也不用改表（D-3）。`collection_field.type` 的 CHECK 加上 `relation`（任务的自定义属性类型表不变，relation 只有表格有）。迁移 511～515：建表、三个 `CONCURRENTLY` 索引（`id` 唯一、边唯一 `(from_record_id, from_field_id, to_type, to_id)`、反查 `(workspace_id, to_type, to_id)`，各自单文件并登记了失败清理）、放宽类型约束（`NOT VALID`，新集合是旧集合的超集）。515 的回滚会先归档 relation 字段，因为 511 的回滚会删掉所有边。
- **字段定义。** `config.relation = {"to_type":"issue"}` 或 `{"to_type":"record","collection_id":"…"}`；目标表必须是本工作区未归档的表，可以是自己。**目标创建后不可改**（PATCH 带 `config` 返回 400），类型也不与其他类型互转；改名、调序、归档照常。
- **读。** 每条记录的响应多一个 `links`：`{ [fieldId]: [{id,to_type,to_id,title,identifier?,status?,collection_id?,missing}] }`。列表接口在同一个只读快照里用一条查询取回整页的边并带上目标当前的标题 / 编号 / 状态，不是逐行查。目标不在了（任务被删、记录进回收站、目标表被归档）→ `missing: true`、标题为空，**边保留**（7.4）。没有关联的记录返回 `links: {}`，旧服务端不返回该字段时客户端 schema 缺省为空。
- **写。** `POST /api/collections/{c}/records/{r}/links {field_id,to_id}`（201；同一条边重复提交返回 200 且不变）与 `DELETE …/links/{linkId}`（重复删除也是 200），都返回更新后的记录并发 `record:updated`。任何能编辑这一行的成员都能关联，不要求字段管理权限。目标按字段自己的定义校验：只能是本工作区的任务（`kind=task`）或目标表里未删除的记录，不能关联自己；一格最多 50 条。写入在记录行锁后串行，判重与上限不会竞争；成功后 `record.revision` 加一。relation 字段不接受值写入（`PUT …/fields/{f}` 与建行时的 `fields` 都返回 400）。
- **反向引用。** `GET /api/issues/{id}/record-links`（id 或编号）与 `GET /api/collections/{c}/records/{r}/backlinks`，返回 `{links:[{collection_id,collection_name,record_id,record_title,field_id,field_name}]}`，只含读者还能打开的来源（行未删除、表未归档、字段未归档），最多 200 条。
- **界面。** 字段面板：类型列表末尾新增「关联」，选中后出现「关联到」（任务 / 各表），并写明创建后不可改；编辑时两项都禁用。单元格：蓝色 chip = 任务（带状态图标和编号），灰色 chip = 记录，删除线「已删除」= 失效；点开是搜索框：未输入时「已关联」在上（点击取消）、可选目标在下（点击关联）；输入后整个列表只剩匹配项，已关联的带勾、再点取消 —— 回车因此总是落在匹配项上，不会误取消排在最前的关联；不自动关闭。搜索结果来自服务端、晚于按键到达，所以共享的 `PropertyPicker` 改为“输入后等到列表里出现匹配项再定位高亮”（在内存里过滤的选择器第一次渲染就有匹配项，行为不变）。表格里关联格只占一行，放不下的 chip 被裁掉，完整列表在行详情。任务需输入关键词才搜（含已关闭），表格先列最新 50 行。看板卡片和画廊同样显示 chip。**关联列暂不支持排序 / 分组 / 过滤**，表头菜单和「排序」「过滤」列表里不出现。
- **行详情与「转为任务」。** 关联字段在行详情里各占一节：每条是可点击的行（任务：状态 + 编号 + 标题），可取消关联，底部「关联已有任务 / 关联记录」。“这一行没有评论区”的说明里多了真正的出口「转为任务」：对话框确认标题 → **先保证有关联到任务的字段，再建任务，最后关联**（任务建好却关联不上是最难发现的失败）。任务走普通的 `POST /api/issues`，建在表格所属项目里；关联写进本表第一个关联到任务的字段，没有时由管理者转换自动新增「关联任务」字段（对话框提前说明），非管理者则禁用并说明找谁。关联失败时提示“已创建但关联失败”。
- **任务侧。** 任务详情右侧栏新增「关联记录」一节（没有时整节不出现），每行“表名 + 行标题”，点击打开 `…/collections/{c}?record={r}` —— 表格页现在读取 `?record=` 并直接展开该行详情。删除任务的确认框在有记录指向它时多一行提示（AC-8）；批量删除的确认框暂未加。任务被删除或状态变化时，实时事件会刷新表格查询，关联格随之更新。
- **未做：** 关联列的排序 / 过滤 / 分组、lookup / rollup、双向关联字段（目标表自动出现反向列）、批量删除任务时的提示、智能体经 CLI 读写关联（FR-036）。
- 需要执行迁移并重建后端：`make down && make up`。
- 验证（2026-09-20，隔离环境）：Go `go build ./...`、`go vet`（handler、fields、migrate、server）、`go test ./internal/handler`（整包）与 `go test ./cmd/migrate` 通过；数据库已应用迁移 515，另在一次性数据库上把 511～515 逐个回滚、再连续执行两遍 up（幂等、回滚后 relation 字段被归档且类型被拒）。新增 `collection_link_test.go` 覆盖：字段定义与不可变目标、AC-8 全流程（关联 → 任务侧反查 → 删任务后 `missing` → 取消关联）、表到表与自关联、回收站 / 归档对正反两个方向的影响、跨工作区 / 文档 / 非 relation 字段 / 不存在的行被拒、普通成员可关联、50 条上限。前端：`@multica/core` 全套 161 个文件 1,964 个用例通过（含 schema 与 `ApiClient` 的畸形响应测试）；`@multica/views` 全套（最后一次改动之后重跑）448 个文件中 445 个、5,329 个用例通过，失败的 5 个用例都在 `issues/surface/`（基线提交上同样失败，与本次无关）。core / views 的 `tsc --noEmit` 与 eslint 无错误。用真实页面组件对真实 API 走通了整条链路（建两个关联字段 201、从格子里关联两个任务和一条客户记录、行详情、转为任务、任务侧「关联记录」点回该行、删除确认框的提示、删除后格子显示「已删除」且可点掉），中英文界面各一遍，无页面错误。**`e2e/cortex-g1.spec.ts` 新增的用例尚未通过 Next 前端执行**，请在本机运行 `make env-exec ARGS='-- pnpm exec playwright test e2e/cortex-g1.spec.ts --workers=1'`。

## 2026-09-21：FR-027 命令行与智能体接口（PRD 5.7）

起因：文档与表格的 HTTP 接口大多已接受智能体的任务令牌，但 `multica` CLI 没有一条表格命令，文档也只能借道 `issue create --kind doc` / `issue update --expected-document-revision`；内置平台技能和运行时 brief 对这两类对象只字未提。智能体只经 CLI 做事，所以这些能力对它等于不存在。需求写进了 PRD 新增的 5.7（`FR-061`～`FR-066`、`AC-28` / `AC-29`、修订八、`Q-U5`），架构手册新增第 11 章。

- **一条线。** 行和页面是工作材料，结构和批准属于人。任务令牌可以读表、建行、逐格写入、改标题、删行 / 恢复、关联 / 取消关联、反查；可以新建文档、按版本保存、移动、评论。建表、改表名 / 归档、增改字段，以及文档的送审 / 发布 / 退回草稿只有人能做——按凭证类型判断，运行时主人是管理员也不放行。**服务端的放行范围没有变**，本次只是把它固定下来并让拒绝说得清楚。**（同日调整：表结构已向智能体放开，本条关于建表 / 改字段的部分不再成立，见下一节。）**
- **可诊断拒绝。** 两类拒绝从纯文字 403 改为带稳定错误码：`collection_schema_requires_human`（建表、`requireCollectionManager`、建字段）与 `document_transition_requires_human`（`TransitionDocument`）。原因在 CLI：`cli.FormatError` 有意把所有 403 折叠成同一句“无权访问”，命令必须按 `code` 认领才会换成具体的话。响应体多一个 `code` 字段，对 Web / Desktop 无影响。（`collection_schema_requires_human` 已随下一节取消，换成 `collection_schema_forbidden`。）
- **命令。** `server/cmd/multica/` 新增 `cmd_collection.go`、`cmd_record.go`、`cmd_document.go`：
  - `collection list|get|create|update|archive`、`collection field list|add|update|archive`；
  - `record list|get|create|update|delete|restore|trash|link|unlink|backlinks`；
  - `document list|get|create|save|move|status`；
  - `issue records <issue>`（任务侧反查）、`issue search --kind task|doc`。
- **按名称寻址。** 表：ID → 名称 → ID 前缀（名称先于前缀）；字段、选项、成员：名称或 ID，值编码复用任务属性的编码器；行：ID 或**完全一致**的标题，同标题多行时列出 ID 并停止，搜索结果被分页截断时也拒绝按标题匹配；任务：编号。文档只接受编号或完整 ID——ID 前缀要靠任务列表解析，而任务列表不含文档。
- **输出。** 行的 JSON 默认把单元格放在 `values` 下：键是字段名，值是选项名 / 成员名 / 原始标量；关联格给出能直接回填 `--to` 的标识（`{"issue": "MUL-31", …}`、`{"row": "<id>", …}`，失效的边是 `{"deleted": true, "link_id": …}`）。`--detail` 才输出字段 ID、存储值与边 ID。同一页数据默认形态约为明细的三分之一大小。`document list` 不带正文。
- **并发语义不放宽。** `document save` 必须带 `--expected-revision`，CLI 不自动补、不重试；409 时回读当前版本再生成提示（冲突响应带整篇正文，超过 CLI 保留的错误体上限，解析不可靠），提示要求“先合并再用新版本保存”。`record update` 先解析完所有参数再开始写；每格一次原子写入，`--expect "字段=值"`（`"字段="` 表示仍为空）变成 `expected_value`；改标题自动带 `title_base`；中途失败时在 stderr 列出已写入的格。关联列的过滤与排序、比较运算符在客户端直接报“尚不支持”，因为服务端会静默退回默认顺序 / 空结果。
- **一个顺手修掉的坑。** `document move --first` 不能用位置 0：新建 issue 的 `position` 是 `MIN(position) - 1`，即负数，位置 0 会排到最后。CLI 用远小于零的位置表示“最前”，远大于现有值的位置表示“最后”。
- **让智能体知道。** 内置技能 `multica-platform` 新增 `references/documents.md`、`references/collections.md`，路由表与描述同步（描述 247 / 300 字符）；运行时 brief 的技能提示点名“documents, tables and their rows”。参考只写可观察行为，不含源码路径（有测试）。
- **文档站。** `apps/docs/content/docs/cli*.mdx` 四种语言的命令总览与参考表补上三组命令、`issue records` 与 `issue search --kind`。
- **未做（PRD `FR-066`）：** 比较运算过滤、按关联过滤 / 排序、批量写入与 CSV 导入的命令、`mention://collection|record` 在 CLI 输出里的形态；是否允许智能体“送审”见 `Q-U5`（批准记录只有 `actor_id`，任务令牌以运行时主人的身份认证，放行会把智能体的动作记在那个人名下）。
- 不需要迁移；需要重新编译后端与 CLI：`make down && make up`，`make build`；让运行中的守护进程用上新 CLI：`make daemon`。
- 验证（2026-09-21，隔离环境）：`go build ./...`、`go vet`（cmd/multica、handler、service、execenv）通过，`gofmt` 无差异；`go test ./internal/handler ./internal/service ./internal/cli ./cmd/multica ./cmd/migrate ./cmd/server` 全部通过。新增测试：`cortex_agent_access_test.go` 两条（表格矩阵、文档矩阵，含“被拒的转换不留批准记录”“人发布后智能体改正文回到 draft”）；`cmd_collection_test.go` / `cmd_record_test.go` / `cmd_document_test.go` 共 35 个测试函数（寻址与歧义、过滤与排序的翻译、逐格写入顺序与 `--expect`、中途失败的提示、关联目标解析、失效边按边 ID 移除、文档树排序、无版本 / 旧版本保存、状态到动作的映射、两种拒绝的提示文案与退出码）；内置技能测试覆盖路由表、描述触发词、两篇参考的契约锚点与“不含源码引用”。`./internal/daemon/execenv` 有 2 个、`./internal/daemon` 有 1 个用例失败（`TestFinalizeKeepsWorktreeWhenCommitFails`、`TestMigrateHermesTaskMemoriesFailureKeepsSource`、`TestValidateLocalPath`）：它们依赖“chmod 后不可读”或“HOME 不是系统根目录”，而隔离环境以 root 运行；撤掉本次改动后同样失败，与本次无关，请在本机用 `make test` 确认。真实走查：用真实的 `multica` 二进制对真实后端执行 60 条命令——先以人的身份建两张表和字段，再以智能体身份（按测试夹具的方式插入运行时、智能体、运行中的 task 与 `mat_` 任务令牌）读表、按名称写格、带 `--expect` 的写入与冲突、关联任务与另一张表的行、任务侧反查、回收站；五种表结构操作被拒（退出码 3，提示说明行仍可写）；文档新建 / 读取 / 按版本保存 / 旧版本保存被拒 / 无版本被拒 / 移动 / 评论 / 按类型搜索，送审被拒，人发布后智能体改正文回到 draft。另外单独验证了：九类字段全部能按名称写入并回读（含带 `=` 和 `,` 的文本），URL / 日期 / 数字的非法值被拒；运行时主人被移出工作区后，同一任务令牌的读请求返回 404。两份 HTML 手册用 Chromium 渲染检查，新增的 Mermaid 图全部渲染、无页面错误；四个 `cli*.mdx` 用 `@mdx-js/mdx` 编译通过。**未执行**：真实智能体运行（需要本机守护进程与模型凭据，步骤见 `LOCAL-MANUAL-TEST.md` 的“命令行与智能体验收”）、`make test` 全量与 race、前端测试（本次没有改前端代码）。

## 2026-09-21：放开智能体管理表结构（PRD 修订九，Q-U3 的 schema 部分）

起因：上一节按基线把建表和改字段留给了人。第一次真实运行，用户让智能体“建一张客户名单”，`multica collection create` 被拒，智能体只能回一句“平台把表结构锁给人类了”。用户决定去掉这条限制。

- **规则。** 表格上不再有按凭证类型的拒绝，任务令牌完全沿用运行时主人的身份：建表对成员与智能体开放，`created_by` 记运行时主人（这个人在界面上继续管理它，`canManage` 的判断没有变）；改表名 / 首列名、归档表、增改 / 归档字段与选项仍是“表的创建者或 owner / admin”，对智能体取其运行时主人——管理员名下的运行可以改任何表，普通成员名下的运行只能改该成员建的表（含他名下的运行替他建的）。云节点 PAT 同理。
- **没有放开的。** 任务字段定义（`requirePropertyAdmin`）仍拒绝智能体；`actor` 字段仍只引用成员（Q-U3 的另一半）；文档送审 / 发布 / 退回草稿仍只属于人（Q-U5）。
- **服务端。** `collection.go` 的 `CreateCollection`、`CreateCollectionField` 和 `collection_manage.go` 的 `requireCollectionManager` 去掉 `isMachineCredentialActor` 分支；建字段改为复用 `requireCollectionManager`。非管理者的拒绝从 `requireWorkspaceRole` 的通用 403 换成带稳定错误码的 `collection_schema_forbidden`，对人和智能体相同；`collection_schema_requires_human` 取消。`collection:updated` 的执行者改用 `resolveActor`，智能体的结构变更记在智能体名下；**建表现在也发这条事件**——之前只有界面自己的 mutation 会刷新列表，CLI 或智能体建的表要等客户端下一次重取才出现。
- **CLI。** `collectionSchemaRequestError` 改为认领新错误码，提示“只有表的创建者或 owner/admin 能改结构、智能体的运行算作运行时主人、行仍可写、可以自建一张表”；帮助文案去掉“仅限人”。新增 `collection field update --add-option`：字段配置接口只收整份选项列表，漏掉的选项会被 `StripCollectionFieldOptions` 从所有行（含回收站）清除，让智能体为了加一个选项去重键整份列表是个坑；`--add-option` 把既有选项按 ID 和颜色原样带回再追加。同名选项、非单选 / 多选字段、与 `--option` 同用，都在发请求之前拒绝。
- **技能。** `references/collections.md` 的权限表改为“运行 = 运行时主人”，新增“CLI: tables and fields”一节：增加是安全的；`--option` 整体替换且不能改名、多选转单选只保留第一个值、关联目标不可改、归档的表与字段目前回不来——只在任务明说时才移除、转换、归档，并在评论里写明改了什么。路由表与测试锚点同步。
- **文档。** PRD：5.7 的“一条线”、`FR-064`、新增 `FR-064A`、权限矩阵、时序图、`FR-027`、`US-10`、`AC-26` / `AC-28`、`Q-U3`、修订四的后续说明、新增修订九、§14。架构手册 11.7（矩阵、放开的原因与实现、风险说明）与 11.8。UI 原型文档里两处 Q-U3 的表述、四种语言的 `cli*.mdx`、`LOCAL-MANUAL-TEST.md`（命令行第 8 步；智能体验收改为预期建表 / 加字段 / 加选项成功、提交评审被拒）。
- 不需要迁移；需要重新编译后端与 CLI：`make down && make up`，`make build`，`make daemon`。
- 验证（2026-09-21，隔离环境）：`go build ./...`、`go vet`（cmd/multica、handler、service）通过，改动的 Go 文件 `gofmt` 无差异；`go test ./internal/handler ./internal/service ./internal/cli ./cmd/multica ./cmd/migrate ./cmd/server` 全部通过。`cortex_agent_access_test.go` 的表格部分改为三条：智能体以主人身份建表、加字段、改名、改选项、归档字段与表，七次结构变更的事件全部记在智能体名下；主人为普通成员时四种结构操作被拒（`collection_schema_forbidden`）而本人得到同样的错误码、写行与自建表不受影响、成员能在界面侧继续管理智能体替他建的表，主人为 admin 时放行；行与关联的原有断言保留。`cmd_collection_test.go` 新增 `--add-option` 的请求体与四种拒绝。真实走查：用真实的 `multica` 二进制对真实后端，四个身份（owner 本人、owner 的运行、普通成员本人、普通成员的运行）共 34 条命令——早上被拒的 `collection create --name "客户名单"` 成功，`created_by` 等于运行时主人；加单选 / 数字 / 关联字段、改首列名、`--add-option` 后立刻用新选项写行；成员的运行在别人的表上写行成功、五种结构操作被拒（退出码 3）且本人得到同一句话；owner 的运行能改成员建的表；`--option` 漏写选项后该选项的行值确实被清掉（与文档写的代价一致）；送审仍被拒。另用 WebSocket 以 owner 身份连上实时通道，智能体建表后收到 `collection:updated`，执行者为该智能体。两份 HTML 手册与 UI 原型文档用 Chromium 渲染：Mermaid 图全部渲染、无失效锚点、无页面错误；四个 `cli*.mdx` 编译通过。**未执行**：真实智能体运行、`make test` 全量与 race、前端测试（没有改前端代码）。

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
