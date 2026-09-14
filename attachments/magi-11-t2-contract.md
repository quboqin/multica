T2 开发前契约：collection/record 与真实共享 TableView
版本：2026-09-14；状态：本次切片实施契约；执行者 kiki，独立测试 momo，架构复核 bubu。

**结论与边界**

按最新触发线程锁定：T2 = collection/record，T3 = doc。早期总方案附件的任务表把这两项写成相反编号，本契约以最新线程为准。T2 在 Web + Desktop 打通“创建集合 → 创建记录 → 同一共享 TableView 编辑/清空 → 服务端保存 → 刷新恢复”，作为 G-1/FR-002、FR-021～024 的部分交付。doc、issue.kind、文档树和正文均不进入这个 PR。

沿用方案 A，新增 collection、collection_field、record 三张业务表；审计复用现有 activity_log。不引入框架、中间件、数据库服务或通用操作流水平台。T2 仅在隔离开发/测试环境启用；T4 的完整智能体授权、共享环境账号与隔离验收仍是开放门禁。本契约给出新表的 RLS 实现与隔离库验证要求，不将它解释为现有全系统已具备 RLS。

| 决策 | 推荐方案 | 可行备选及代价 |
| --- | --- | --- |
| 字段存储 | 独立 collection_field + record.fields JSONB；T2 只支持 text/number/checkbox | 固定几列放 record 表最简单，但不能验证真实独立字段目录，后续还要搬值；不选 |
| 字段配置 | 创建集合时一次提交 1～50 个简单字段，创建后目录在 T2 不可变 | 同时开放字段 CRUD/改类型/选项，会把 T5 的兼容、归档与权限工作提前；不选 |
| 写入冲突 | record 行级 expected_revision 条件更新；冲突显式重读与重试 | 字段级 CAS 可减少无关字段冲突，但多一套基值协议；T2 暂不需要 |
| 租户隔离 | 共享表、显式 workspace SQL、事务局部上下文、新表 RLS；隔离库先验证 | 每租户独立库隔离更强，但多连接池/迁移与运维负担，当前规模和团队不适合 |
| 并行开发基线 | 在冻结 PR #4 head 上堆叠 T2 | 等 #3/#4 合入后开发维护最简单，但有等待；本任务推荐堆叠 |

以上都沿用团队现有 Go/sqlc/PostgreSQL 与 React Query 技术栈，退出路径是关闭新增入口、保留新表和数据。T2 不做自定义筛选/排序/分组、层级、重排、saved view、批量/CSV、relation、单行删除/回收站、完整 CLI 或移动端新 UI。这些属于后续已识别切片；不显示无效按钮，不宣称 FR-025/026/027 完成。集合先限定 workspace 级；project 归属及归档 UI 随后续集合管理切片补齐，不把未授权的 project_id 透传入库。

**已核实的源码基线**

全部源码判断固定于 `5d182a57f5070365577d7420fb8814dce25f8153`。Multica PR 关联表当前返回空，因此又按历史评论给出的 PR URL 直接查询 GitHub；不能把“关联为空”理解为 PR 不存在。

| PR | 当前状态 | head | base |
| --- | --- | --- | --- |
| [#3](https://github.com/quboqin/multica/pull/3) | OPEN，未合并 | `feature/cortex-g1@53f49f0f9f120fd67e1f025039dd6a14db504ea5` | `docs/architecture-handbook@2ef09f14c7cfe22349d7144ead5195988c363ffc` |
| [#4](https://github.com/quboqin/multica/pull/4) | OPEN，未合并 | `feature/cortex-g1-tableview@5d182a57f5070365577d7420fb8814dce25f8153` | `feature/cortex-g1@53f49f0f9f120fd67e1f025039dd6a14db504ea5` |

可复用与必须接线的位置：

- `packages/core/data-source/types.ts:18` 已定义 workspace/namespace/source 三元身份，`:44` 有字段投影和 canSet/canClear，`:99` 是最终 accepted/cancelled/failed。
- `packages/core/data-source/query-keys.ts:7` 已提供结构化缓存前缀。`packages/views/data-view/query-binding.ts:48` 允许保留领域原始缓存 DTO；`packages/views/data-view/controller.ts:56` 是共享分页/行控制器，`packages/views/data-view/table-view.tsx:78` 是生产共享 TableView。T2 同时复用 controller 和 TableView，不能只复用一个外壳。
- `packages/core/api/client.ts:638` 的 workspaceHeader 依赖明确 slug；只加 workspace_id 参数不会自动覆盖全局 slug。新调用必须显式捕获并传入原工作区请求上下文。
- `server/cmd/server/router.go:1858` 已有 workspace membership 路由组；`server/internal/middleware/workspace.go:113` 解析工作区并优先尊重 task token 绑定。新 handler 使用 middleware context，不另造一套优先级。
- `server/internal/handler/actor_guards.go:96` 已有 RequireHumanActor，能拒绝 mat_ 与 mcn_ 机器凭证。`server/internal/handler/handler.go:840` 的 resolveActor 主要用于归属判定，不能单独充当机器凭证授权门禁。
- `server/internal/handler/property.go:630` 明确禁止智能体管理 issue 字段定义；T2 不修改此规则。通用字段校验不应通过构造 db.IssueProperty 来复用，避免新领域依赖 Issue DTO。
- `server/pkg/db/queries/issue_view.sql:32` 有 workspace + revision 条件更新范例；`server/internal/handler/handler.go:571`、`:575` 有稳定错误码与 revision_conflict 响应。
- `server/pkg/db/queries/activity.sql:29` 支持 issue_id 为空的审计写入；`server/internal/handler/workspace.go:1100` 的工作区删除持有事务与工作区锁，`server/internal/handler/workspace_delete_manifest_test.go:18` 要求每张表明确清理归属。
- `server/internal/featureflags/keys.go:43`、`server/internal/handler/config.go:58` 支持前端安全布尔开关；`server/internal/analytics/events.go:37` 明确服务端产品事件仅进 DB/Prometheus，不再发 PostHog。
- PRD 的独立 record 边界见 `apps/docs/public/handbook/cortex-prd.html:766`；性能目标见 `apps/docs/public/handbook/cortex-prd.html:1034`。本次范围小于整份 G-1。

现有 `packages/views/layout/collection-page.tsx` 的“collection”是布局组件命名，不能据文件名认定 collection 服务端模型已经存在。本次所读基线没有上述新资源的完整实现。

**约束与质量目标**

1. record 不含 issue 编号、status、assignee、评论、运行确认或调度；创建/改行不能修改 issue 编号计数，不能新增 issue、agent_task_queue 或 inbox_item。
2. 成功响应只在数据库事务提交后返回；同一旧 revision 的两个写入至多一个成功。超时/断线允许“结果未知”，不得据此假定数据库未写入。
3. 每次新请求重验身份和成员资格；跨工作区、跨 collection 的对象/字段/游标不能混用。新表应用账号缺租户上下文时无数据可读、无数据可写。
4. 单次记录页默认 50、最大 200；以 10,000 行/20 个简单字段验证首屏 p95 <1.5s，单行读写 p95 <300ms。建议基准 4 vCPU/8GiB、20 并发，报告浏览器/网络、冷/热缓存与至少 1,000 次 API 样本；这些是待验证目标。
5. T2 不实现三条件过滤，因此 NFR-003 的 100,000 行过滤 p95 <800ms 留给查询切片，不能用无过滤测试替代。
6. 在线双客户端收到事件后目标 2s 内完成重读；事件丢失时通过定期/焦点/重连重读恢复，以下定义收敛边界。T2 不提供离线写队列。

**系统与模块边界**

~~~mermaid
flowchart LR
  H[工作区成员] -->|页面操作| W[Web 与桌面端]
  W -->|HTTP JSON| S[现有 Go 服务]
  S -->|事务 SQL| D[现有 PostgreSQL]
  S -->|工作区 WS 失效事件| W
  S -->|本地指标与结构化日志| O[现有观测系统]
~~~

这张图回答进程与数据流边界。新增能力进入现有服务与数据库；不引入后台调度、模型调用或新持久化服务。读写对象仍为集合与记录。

~~~mermaid
flowchart TD
  P[平台路由] -->|导入页面| C[集合页面装配]
  P -->|导入页面| I[任务页面装配]
  C -->|注入绑定与字段| T[共享控制器与 TableView]
  I -->|注入绑定与字段| T
  C -->|调用 hooks| A[core 集合适配器]
  I -->|调用 hooks| B[core 任务适配器]
  T -->|依赖类型与编辑契约| K[core 数据源契约]
  A -->|实现接口| K
  B -->|实现接口| K
  A -->|调用客户端| Q[core API 与 Query]
  T -->|界面原子组件| U[ui 组件]
~~~

箭头表示允许的依赖。禁止共享层 T/K 反向导入集合或任务适配器、Issue DTO、查询/store/gate；禁止 ui → core；core 不引用 views 的 query-binding 类型。views 装配把 core 的查询选项组成现有 DataViewQueryBinding，接口本身无须搬家。检查应包含导入闭包和类型重导出，不能只验证一条正则。

后端按 `router → collection handler → collection 应用操作/事务 → sqlc` 单向依赖；字段值校验是纯逻辑。复用 TxStarter、db.Queries.WithTx、既有错误/事件/日志工具，不为三张表再建一套通用 repository 框架。新路径拟为 `server/internal/handler/collection*.go`、`server/internal/collection/`、`server/pkg/db/queries/collection*.sql`、`packages/core/collections/`、`packages/views/collections/`。纯校验可直接放新领域包；T5 再将已证实共用的字段规则抽到共享 field 包，不能复制完整 issue property 模块。

**最小 API 与字段契约**

路由统一处于现有 /api 认证、CSRF、限流和 workspace membership 链。新子路由以 `/api/collections` 为根；ID 仅接受 UUID，不支持 issue 编号解析。工作区只从验证后的 context 获取，拒绝请求体中的 workspace_id、created_by、revision 等服务端所有字段。

| 方法与路径 | 请求 | 响应/语义 |
| --- | --- | --- |
| GET /api/collections | limit=50（1～200）、cursor | 200 `{collections,total,next_cursor}`；仅活跃集合 |
| POST /api/collections | `{client_request_id,name,fields:[{name,type}]}` | 首次 201 `{collection,fields,replayed:false}`；同键同内容 200，replayed=true |
| GET /api/collections/{collectionId} | 无 | 200 `{collection,fields,capabilities}`；字段目录按 position,id 排序 |
| POST /api/collections/{collectionId}/records/query | `{page:{limit,cursor}}` | 200 `{records,total,next_cursor}`；T2 无筛选/分组/自定义排序参数 |
| GET /api/collections/{collectionId}/records/{recordId} | 无 | 200 `{record}`；用于重读与冲突恢复 |
| POST /api/collections/{collectionId}/records | `{client_request_id,title,fields}` | 首次 201 `{record,replayed:false}`，重复 200，replayed=true |
| PATCH /api/collections/{collectionId}/records/{recordId} | `{expected_revision,change:{field_id,op,value?}}` | 200 `{record}`；一条命令、一次 CAS、一个 revision 增量 |

这是 C/R/U 闭环；T2 不提供 DELETE、恢复或 collection metadata/schema PATCH。保留未来已识别生命周期字段，但不开放未经设计的写参数。UI 创建表默认附“备注 text、数量 number、已核对 checkbox”三个字段；API 可在创建时提交 1～50 个上述类型字段，供实际业务初始结构与 20 列验收使用。创建后不做字段管理页或增删改字段端点。

DTO：

~~~ts
type Collection = {
  id: string; workspace_id: string; name: string;
  revision: number; archived_at: string | null;
  created_at: string; updated_at: string;
};
type CollectionField = {
  id: string; workspace_id: string; collection_id: string;
  name: string; type: string;
  position: number; revision: number;
};
type CollectionRecord = {
  id: string; workspace_id: string; collection_id: string;
  title: string; fields: Record<string, unknown>;
  position: number; revision: number;
  created_at: string; updated_at: string;
};
~~~

这是兼容读取 DTO：T2 服务端只产生 text/number/checkbox，客户端允许未来 type 字符串与未知值保留，但按已知字段 schema 验证后才能编辑。创建与更新请求的值类型严格限制为已声明的三种。wire 的 fields 键是本集合 collection_field.id。通用 UI 的标题列 fieldId 固定为 `title`；自定义列使用 `field:<UUID>`，适配器拆出 UUID 后提交，防止系统列与自定义列撞名。fieldId 不用列名或 position；DTO 不包装 Issue。

- 集合名 trim 后 1～120 Unicode 字符；字段名 trim 后 1～80，集合内按数据库 lower(name) 唯一；标题可为空，最长 2,048 字符。显示用“未命名”不能反写为实际标题。
- text 接受空字符串，最长 16,384 字符；number 为有限 JSON 数字，绝对值不超过 10^15，T2 不承诺财务定点精度；checkbox 只接受布尔值。`""`、`0`、`false` 都是有效 set，不能转为 clear。
- `op=clear` 删除值袋里的键；不用 null，也不写空字符串。标题允许 set("")，不提供 clear。未知字段、字段属于另一集合、未知类型、set 缺 value 或 clear 带 value 均拒绝。
- record.fields 持久化的规范化 JSON 文本 UTF-8 大小 ≤64KiB，以 PostgreSQL `fields::text` 的字节计量为准；创建/编辑原始请求体限 128KiB，集合创建限 128KiB。对超限返回 413，包含数据库大小约束失败；普通类型/结构错误返回 400。应用提前校验不能用紧凑 JSON 序列化字节数替代数据库最终校验，须测临界大小。
- collection 创建在同一事务建集合及完整字段目录；任一字段失败整单回滚。T2 不允许更改字段类型和目录，后续 T5 用扩展迁移增加新类型与管理接口。
- 每次 accepted 的 PATCH 包括逻辑上相同的 set/对缺值 clear，revision 都 +1，并形成一次审计；不依据相等值跳过并发检查。前端可以避免发无意义提交，但服务端语义唯一。

能力只由服务端事实与当前装配共同决定：`layouts:["table"], grouping:false, hierarchy:false, writable:<当前可写>, maxPageSize:200`。所有字段 `sortable=false/groupable=false`；不能把前端未加载全量数据的排序当服务端排序。custom field 的 canSet/canClear 均要求“活动集合 + 当前人类成员有写权 + 当前字段受支持”，title.canClear=false。创建行由集合装配提供受控入口，await create mutation 后再显示权威行；不必扩展 T1 的 cell command 为通用 create DSL。

错误沿用 `{error,code}`，可增 field_id/actual_revision 等可选定位字段：401 未认证；404 不可见/跨租户/跨集合/软删资源；403 可见资源无写权或机器凭证；400 输入/游标不合法；409 `revision_conflict` 或 `idempotency_conflict`；413 超限；504 查询超时。既有 middleware 对 task-token 绑定冲突的 403 保持原语义。服务端不得在尚未证明资源可见前返回 actual_revision 或对象摘要。无匹配 CAS 时在同租户作用域区分 404 与 409，不能查全局 record 后再决定。

新增响应全部经过 zod + parseWithFallback。读响应未知可选字段忽略，未知字段类型保留展示但禁写；创建/修改响应若缺 ID、来源身份或合法 revision，必须显式失败并安排权威重读，不能把 fallback 空 DTO 判为 accepted。安装中的旧 Desktop 忽略新增 config/event 字段；新客户端遇旧服务端 collection 路由 404/flag 缺省时不展示入口、不循环重试。T2 不改变任何 Issue API/缓存 DTO 或旧移动端行为。

**创建重试、并发与分页**

创建幂等以三张表中的持久化创建键完成，不引入第四张操作流水表：

- 客户端首次提交生成随机 UUID `client_request_id`，失败重试复用；主动发起第二次创建才换键。集合键作用域为 (workspace, authenticated user, key)，record 键为 (workspace, collection, authenticated user, key)。
- 服务端对已校验、补默认值后的业务请求计算 SHA-256；集合包含有序字段规格，record 的字段键排序后计算，client_request_id 不进入 hash。
- 唯一索引是并发仲裁；同键同 hash 等待首次事务完成后返回当前同一资源（200/replayed），不能重复建字段、审计或发创建事件。同键不同 hash 返回 409。响应中的内部 hash 不对客户端公开。
- replay 也先验现有权限；不因持有旧键绕过撤权、归档或软删。未来软删后保留键，资源不可用则拒绝，不复活或新建；T2 不清理创建键。

PATCH 用 expected_revision，不自动重试。连接中断后重发同一 revision 至多再成功一次；若第一次已提交，重发返回 409。客户端重读、保留原草稿并让用户确认是否另发新 revision；不能仅凭“当前值相同”证明上次提交成功，也不能自动用新 revision 覆盖别人的更新。此处不承诺“PATCH 原响应精确重放”，也不将网络 failed 解释成业务零写入。CAS 足以满足 T2 的一次变更与不静默覆盖。

列表/记录页固定按 `created_at ASC, id ASC` 排序，不用 UUIDv7 时间推断顺序，不启用 position 重排。游标是有版本的受限编码，包含 wsId、collectionId（集合列表为空）、固定查询摘要、limit、last_created_at、last_id；逐项解析并与请求作用域比较。客户端不能提供 SQL。游标不作为授权凭证；仅 Base64 编码可行，无需为这个固定查询新引入签名密钥。

同一请求的 total 与行页使用只读 REPEATABLE READ 快照，参考既有 issue table snapshot 模式；跨页不承诺静态快照。新建记录可能在后续页出现；删除/字段排序变动尚未开放。每次变更/WS/reconnect 刷新头页并由共享 controller 淘汰旧尾游标；total 不由浏览器已加载行数推算。页查询上限/超时沿用8秒诊断上界，性能验收仍按上述 p95，更高超时不是性能通过。

~~~mermaid
sequenceDiagram
  participant U as 用户
  participant T as 共享表格
  participant A as 集合适配器
  participant S as 集合服务
  participant D as 数据库
  U->>T: 修改单元格
  T->>A: execute(row, fieldId, change)
  A->>A: 捕获来源与编辑会话，检查能力
  A->>S: PATCH + expected_revision
  S->>D: BEGIN，绑定租户，锁定并重验权限
  S->>D: 条件更新 + 审计
  alt revision 匹配
    D-->>S: COMMIT，新 revision
    S-->>A: 200 record
    S-->>A: WS 失效事件
    A-->>T: accepted，协调原源缓存
  else 已被另一窗口修改
    D-->>S: 零行更新，ROLLBACK
    S-->>A: 409 revision_conflict
    A-->>T: failed，保留本次草稿
    U->>T: 重读并确认重试
    T->>A: 新操作，使用确认后的 revision
  end
~~~

这张图定义成功与失败重试。若 COMMIT 后响应丢失，走上文“结果未知→重读”路径；事务回滚时不发事件、不记成功。切格/切源后，旧 Promise 仍结束，但只有原编辑会话可以处理关闭/恢复。record 不调用 run-confirm。

**数据模型与 DDL 草案**

~~~mermaid
erDiagram
  workspace ||--o{ collection : contains
  collection ||--|{ collection_field : defines
  collection ||--o{ record : contains
  workspace ||--o{ activity_log : audits
  collection {
    uuid id PK
    uuid workspace_id
    text name
    uuid created_by
    uuid create_request_id
    text create_fingerprint
    bigint revision
  }
  collection_field {
    uuid id PK
    uuid workspace_id
    uuid collection_id
    text name
    text type
    int position
    bigint revision
  }
  record {
    uuid id PK
    uuid workspace_id
    uuid collection_id
    text title
    jsonb fields
    bigint revision
    timestamptz deleted_at
  }
~~~

这张图描述逻辑基数，不创建外键。字段值通过 JSONB 的字段 UUID 关联目录，由同一事务校验。record/field 冗余 workspace_id 用于直接租户过滤与索引，省去一次仅用于确定租户的 collection join；创建与后续写入仍校验父集合和字段归属，RLS 不能替代同租户集合之间的一致性校验。

以下是未执行的 DDL 草案。编号由实现者在新 head 中分配；CREATE TABLE 不内联 PRIMARY KEY/UNIQUE，避免隐式非并发建索引。

~~~sql
CREATE TABLE collection (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 120),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by UUID NOT NULL,
  create_request_id UUID NOT NULL,
  create_fingerprint TEXT NOT NULL CHECK (create_fingerprint ~ '^[0-9a-f]{64}$'),
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE collection_field (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  name TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 80),
  type TEXT NOT NULL CHECK (type IN ('text', 'number', 'checkbox')),
  position INTEGER NOT NULL CHECK (position >= 0),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  archived_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE record (
  id UUID NOT NULL DEFAULT gen_random_uuid(),
  workspace_id UUID NOT NULL,
  collection_id UUID NOT NULL,
  title TEXT NOT NULL DEFAULT '' CHECK (char_length(title) <= 2048),
  fields JSONB NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(fields) = 'object')
    CHECK (octet_length(fields::text) <= 65536),
  position DOUBLE PRECISION NOT NULL DEFAULT 0
    CHECK (position > '-Infinity'::float8 AND position < 'Infinity'::float8),
  revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
  created_by UUID NOT NULL,
  updated_by UUID NOT NULL,
  create_request_id UUID NOT NULL,
  create_fingerprint TEXT NOT NULL CHECK (create_fingerprint ~ '^[0-9a-f]{64}$'),
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
~~~

created_by/updated_by 是认证用户 ID，与仓库现有 member 作者语义一致，不是 member 行 ID。T2 只允许人类凭证，因此不抢先设计 agent 多态作者列；T4 若启用智能体，必须做新列扩展/历史 member 回填/双读兼容/验证后切换，不得把 agent ID 写进这两列。

应用 ID 沿用 `server/pkg/dbid/dbid.go:1` 的 UUIDv7 + DB UUIDv4 fallback，全部使用 RETURNING 的实际值；create_request_id 是随机 v4 关联键，不使用时间可预测 ID。时间戳统一 TIMESTAMPTZ，API RFC3339 UTC；枚举用 TEXT + CHECK；字段值与目录默认规范化，唯一反范式项为上述租户列。T2 不接大对象或文件值，未来只能引用既有对象存储元数据。

~~~sql
CREATE UNIQUE INDEX CONCURRENTLY collection_id_uidx ON collection (id);
CREATE UNIQUE INDEX CONCURRENTLY collection_field_id_uidx ON collection_field (id);
CREATE UNIQUE INDEX CONCURRENTLY record_id_uidx ON record (id);

CREATE UNIQUE INDEX CONCURRENTLY collection_create_uidx
  ON collection (workspace_id, created_by, create_request_id);
CREATE UNIQUE INDEX CONCURRENTLY record_create_uidx
  ON record (workspace_id, collection_id, created_by, create_request_id);
CREATE UNIQUE INDEX CONCURRENTLY collection_field_name_uidx
  ON collection_field (workspace_id, collection_id, lower(name))
  WHERE archived_at IS NULL;

CREATE INDEX CONCURRENTLY collection_page_idx
  ON collection (workspace_id, created_at, id)
  WHERE archived_at IS NULL;
CREATE INDEX CONCURRENTLY collection_field_page_idx
  ON collection_field (workspace_id, collection_id, position, id)
  WHERE archived_at IS NULL;
CREATE INDEX CONCURRENTLY record_page_idx
  ON record (workspace_id, collection_id, created_at, id)
  WHERE deleted_at IS NULL;
~~~

**上面每一条 CREATE INDEX 各占一个单语句 .up.sql，绝不能把整个代码块放进一个迁移。** 主键另在后续迁移通过已完成的并发唯一索引绑定：

~~~sql
ALTER TABLE collection
  ADD CONSTRAINT collection_pkey PRIMARY KEY USING INDEX collection_id_uidx;
ALTER TABLE collection_field
  ADD CONSTRAINT collection_field_pkey PRIMARY KEY USING INDEX collection_field_id_uidx;
ALTER TABLE record
  ADD CONSTRAINT record_pkey PRIMARY KEY USING INDEX record_id_uidx;
~~~

PK 是全局 ID 点查与冲突仲裁；列表索引按租户/集合等值前缀再按排序键排列；创建幂等索引不使用软删部分条件，避免删后重试另建；字段目录唯一索引独立于 issue 的20字段配额。T2 不加 GIN 或每字段表达式索引，因为没有对应高频查询。每次 INSERT 维护3个左右索引，需计入写入压测。字段总量只在创建事务中确定，T5 开放动态新增时须以集合锁保证50上限。

新字段目录与 record 具备 archived_at/deleted_at，为已识别的归档与回收站变化准备；T2 所有业务查询过滤活动对象，没有自动物理删除任务，保留全部测试业务数据。后续保留期按原方案至少30天、另做恢复与清理验收；删除整个 workspace 是既有显式数据删除语义，三张新表必须一并清理。

索引并发构建不能放在显式事务中；中断可能留下同名无效索引，IF NOT EXISTS 不等于修复。实现者须登记迁移 runner 的无效索引恢复规则，检查 indisvalid/indisready/indislive，再单独清理重建。依据：[PostgreSQL 17 CREATE INDEX](https://www.postgresql.org/docs/17/sql-createindex.html)。

**权限、事务与隔离**

| 调用者/操作 | T2 行为 |
| --- | --- |
| 人类 owner/admin | 新建集合及初始字段；读集合、创建/修改记录 |
| 人类 member | 读活动集合、创建/修改记录；创建集合 403 |
| 非成员/无效 workspace/不可见对象 | 404，不能探测记录、字段和 revision |
| mat_、mcn_ 机器凭证 | 所有 collection 路由 403（已有认证/工作区拒绝可先发生）；不因 runtime owner 是 admin 放行 |
| 未识别的新机器凭证类型 | 新认证分支进入此功能前必须完成 actor guard 审核；不能用客户端自报身份放行 |

这是 T2 的临时受控能力，不覆盖 FR-027。T4 才按已经讨论的链顶人类权限补齐智能体访问：从可信运行链得到授权主体、实时检查成员/角色与资源权限；没有可验证 originator 时拒绝，accountable human / owner_fallback 不能自动成为授权。原 issue 字段规则保持原样。

新路由复用 RequireHumanActor，并在 handler 的公共业务入口做同等校验，防止测试/内部调用绕开路由。人类 JWT/mul_ PAT 均按其真实用户授权，不能据客户端声称“我是人类”提高权限。

写事务固定顺序：BEGIN → 验证并锁定 workspace（FOR KEY SHARE）→ 当前用户的 member 行（FOR SHARE，重查角色）→ 对已存在的 collection 取 FOR SHARE 并检查活动状态 → 校验 field/record 的 workspace+collection → CAS/INSERT → activity_log → COMMIT。所有新建路径参加 workspace 删除锁协议。成员移除/降级已先提交则写入拒绝；写事务先持有共享锁时撤权等待，写入在线性顺序上发生于撤权之前。读请求只承诺撤权后的新请求拒绝，已经开始的快照读可能完成。

没有外键也必须防孤儿：workspace 删除事务同样绑定本 workspace 的 RLS 上下文，按 record → collection_field → collection 清理，先于 workspace 删除；并更新 deletion manifest 与并发建集合/建行测试。开关关闭时也保留该清理路径。collection 父对象不做硬删除；project_id T2 不开放，因而无需猜测项目删除语义。

新表 RLS 草案，对 collection、collection_field、record 逐表执行：

~~~sql
ALTER TABLE record ENABLE ROW LEVEL SECURITY;
ALTER TABLE record FORCE ROW LEVEL SECURITY;
CREATE POLICY record_workspace_policy ON record
  USING (
    workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid
  )
  WITH CHECK (
    workspace_id = nullif(current_setting('app.workspace_id', true), '')::uuid
  );
~~~

每个新资源读/写事务在鉴权后用参数化语句绑定同一连接：

~~~sql
SELECT set_config('app.workspace_id', $1, true);
~~~

$1 必须是经过 membership 验证的 context UUID；此处 true 表示事务局部。所有 sqlc 调用使用该 tx 生成的 queries，不能中途回到 h.Queries/pool、不能使用连接会话级 SET。没设置/事务结束后，新增表 SELECT 无行、写入拒绝；跨租户 UPDATE/DELETE 影响0行，INSERT 或变更 workspace_id 受 WITH CHECK 拒绝。普通 runtime role 必须不是 superuser、没有 BYPASSRLS；即便表 owner 也要 FORCE。隔离测试既用运行角色直连，也覆盖池连接 A→B→无上下文，不能使用 postgres 超级用户跑过测试后声称 RLS 生效。

RLS 防止漏写租户过滤，不替代 handler 权限、字段归属与数据库账号保护；有任意 SQL/凭据控制的主体可以改自定义上下文，它不是抵御数据库账号失陷的隔离边界。当前源码未找到生产新表 RLS 上下文设施，本节是新增要求。参考：[PostgreSQL 17 Row Security](https://www.postgresql.org/docs/17/ddl-rowsecurity.html)。

T2 开关启用前检查运行角色、policy/FORCE、必需表/有效索引；缺条件则新功能不可用，不能以关闭 RLS 恢复。**需用户确认的是将新表 RLS/账号配置推广至共享 Stage/生产的实际部署安排**，在 T4 演练后给出具体配置、授权与回退结果再提交；不阻塞本次隔离库实现。不会修改旧表 RLS 或为所有现有查询强制增加事务。

**缓存身份与共享表格装配**

- source 固定 `{workspaceId: wsId, namespace:"collection", sourceId: collectionId}`。缓存前缀严格复用 `["data-source",wsId,"collection",collectionId]`，详情/目录分支为 `"collection"`，行详情为 `"record",recordId`，分页使用已有 dataSourceRowQueryKey。工作区集合列表单独为 `["collections",wsId,"list",limit,cursor]`。
- T2 Query 是规范化后的固定查询版本，例如 `{version:1,sort:"created_at_asc"}`；key 包含 page size、cursor、groupBy=null、branch 两项均 null、hierarchy=false。没有“同键不同请求”；rowBranchKey 排除 cursor，但保留 source/query/limit 所影响的窗口身份。
- rowId 取 record.id；fields 由 GET collection 目录转换。通用页面映射 records→rows、total→total/branchTotal，不改变 Issue 原缓存 key 或 row.issue DTO。没有组目录的集合装配不触发 group 请求；不能实现一个永远空成功的假 groups API掩盖误调用。
- 请求变量捕获 wsId、非空 workspaceSlug、collectionId、recordId、expected_revision 与登录会话身份；api 方法显式传 slug 和读取 AbortSignal。未提交/排队操作执行前重查当前源与可写能力；已经发送的请求按原身份结束。wsId 进 key 而 slug 进请求不是两套租户，以原 workspace provider 的绑定验证二者一致。
- mutation 使用逐请求 mutateAsync，T2 不做乐观覆盖，保留 pending 草稿；成功只在其原源 detail 中写入更高/相同可信 revision，再失效原源相关列表。HTTP 旧 revision 晚于 WS/refetch 时不能倒灌。失效重新加载服务端窗口，不把创建行简单 append 到当前页。
- 保持 T1b 的编辑 session/generation/source/表实例保护：A提交→B编辑→A成功/失败不得清稿或抢焦点，同格关闭重开也隔离。切源清 selection/冻结行/游标/当前编辑，无跨源 placeholder。客户端已获知撤权/只读后，系统/自定义 set/clear、键盘提交、创建行都零新写请求；尚未收到权限事件的请求仍由服务端即时权限校验拒绝。
- 列宽/显隐偏好使用 dataSourceViewStateKey(identity,"default")；通过既有 StorageAdapter，若持久化再加用户命名空间；不借 issue saved view ID 或全局 collection 名。服务器数据只在 React Query，Zustand/本地存储不能复制记录列表。
- 切工作区后，旧响应可协调原工作区缓存；**成员撤销或退出登录后的清缓存不同**：用登录/权限失效 epoch 阻止已取消的读与晚到写回调重新填回被移除的保护数据。旧 execute 仍结束，当前页面跳转/清理只由已有单一 responder 执行，避免多表重复响应。

WS 新增 `collection:created`、`record:created`、`record:updated`；使用现有 envelope，payload 仅带 collection_id、record_id（适用时）、revision、可用的操作关联 ID，不广播 title/fields。事件只在 COMMIT 后发布。订阅处理器失效精确 source/record 缓存；不要派发 issue:* 或运行事件。核查 SubscribeAll 消费者，证明没有意外调度/通知。

WS 是失效提示，非审计或可靠提交凭证：若提交后进程在 publish 前崩溃，DB 与 activity_log 仍是事实源。reconnect、focus、页面重挂失效集合命名空间；活动且可见页面每30秒重读目录/首屏，不在后台 tab 轮询。正常在线目标2秒内更新；无事件时在下一次可见轮询（30秒+请求耗时）或重连后恢复。无需新增 outbox；不承诺 exactly-once 事件。

**迁移、发布与回退**

沿用现有 featureflag Service，新增建议 key `cortex_collections`，默认 false，通过 /api/config.feature_flags 暴露 UI 安全布尔值；现有环境覆盖变量按规则为 `FF_CORTEX_COLLECTIONS`，只说明变量名，不输出配置值。服务端独立执行相同功能门禁，前端隐藏不充当授权。旧客户端缺此 key 按关闭处理；访问旧服务端新路由的404不应无限重试。

T2 的公共 config flag 只作为整套隔离环境开关，不虚构已有按工作区灰度能力。T4 若按工作区开放，需分别处理 authenticated capability 与后端目标规则，不能在公开 /api/config 泄露工作区名单。

迁移序列：① 建三表，入口关；② 各独立并发索引；③ USING INDEX 主键；④ 新表 policy/FORCE 与运行角色权限验证；⑤ 应用与 workspace cleanup 接线；⑥ 隔离种子/测试；⑦ 仅隔离环境开入口。无旧 issue 回填、无长期双写、无 issue.kind 变更。未来 schema 破坏性变化遵循扩展→双写（确有旧新格式并存时）→回填→切换→收缩，不能将 T2 三种类型直接改写成别的格式。

带数据的正常回退：关 API/UI 新功能 → 停止接收新操作并让在途事务结束 → 保留三表/RLS/索引/审计 → 保留必要 workspace cleanup 的 T2 兼容服务构建，恢复其余界面 → 重新启用验证记录仍在。**不能无条件把服务端退回裸 PR #4**：它不知道三张新表，仍开放 workspace 删除时会留下孤儿。可以退回经过验证、含最小清理逻辑的兼容构建；旧 web/desktop 界面可回退。目标关闭入口/应用回退 ≤10分钟，在隔离 Stage 演练取实际耗时。

空的、可丢弃的开发库才做 down 演练；包含业务数据时 down 明确拒绝，不静默 DROP。若执行空库 down，逆序先解除对应主键约束（否则其索引受约束依赖）、删除其余索引/policy，再删除空表，不能 CASCADE；每个 down 遵循 runner 单语句要求，需要多个步骤就将引入迁移拆得足够细。中途失败通过恢复索引/policy并前向重跑修复，不先削弱隔离。索引是否存在不能仅依赖 schema_migrations 账本。

需单独测试：旧版本浏览器/桌面读任务正常；flag关闭时直连新 API拒绝；有数据回退→删除测试workspace仍清理全部新表；再启用未删除workspace的数据无损。灾难备份恢复与应用回退不同，恢复时限以真实隔离库演练为准。

**埋点与可观测性**

| 类别 | 事件/指标与触发点 | 验收 |
| --- | --- | --- |
| 持久审计 | activity_log，issue_id=NULL，action=collection_created / record_created / record_updated；业务事务内写入 | resource IDs、字段 ID、原/新 revision、request ID、actor 有记录；不存字段值/标题；审计失败整单回滚 |
| 产品成功计数 | server analytics catalog + BusinessMetrics 扩展 collection_created/record_created/record_updated；COMMIT 后首个真实变更记1次 | 幂等 replay、409、回滚均0次；无 issue_created/issue_executed |
| 请求与耗时 | 复用 HTTP 指标的路由模板；必要时新增 collection 操作 histogram/counter | label 只用 operation/outcome/error_code 等有界枚举；无 wsId/cid/rid/URL原文标签 |
| 失败诊断 | logger.RequestAttrs + source IDs、request ID、expected/actual revision、耗时、结果 | 409可定位；认证、超限、RLS拒绝可区分；不含 SQL参数正文、凭据或环境值 |
| 事件收敛 | post-commit publish 与前端对应失效/重连重读 | 双客户端真实 WS可复验；丢事件后轮询恢复 |
| 外发边界 | 所有新增服务端产品事件登记为 metrics-only | mock analytics sink 捕获0次 PostHog；保持现有退出/关闭遥测设置 |

activity_log 只作为审计记录，不能调用会发 issue timeline/inbox 的任务服务。新业务计数走 `metrics.RecordEvent` 时必须同时补 `IsMetricsOnly` 清单；漏登记可能导致外发，是验收项。事务提交与进程指标之间允许崩溃漏计，精确审计查 DB，不声称计数器 exactly-once。一次 replay 不重发创建指标/事件，HTTP 请求计数可以照常计。

建议告警：5分钟窗口且≥100次请求时，5xx率>1%或单行p95>300ms；低流量保留错误日志告警。冲突单独计，不当5xx；隔离负例测试的拒绝率不混入生产告警基线。无需新增客户端点击漏斗或外部观测供应商。

**自验门禁与证据**

| 门禁 | 必须验证的行为 | 主测试层 / 负责人 |
| --- | --- | --- |
| G0 基线与边界 | T2 diff 仅从5d182a5起；Issue与collection用相同controller/TableView；共享导入闭包无领域依赖 | 静态/类型，kiki；bubu复核 |
| G1 真实持久闭环 | 两个collection各自目录；建行→text/number/checkbox set/clear→刷新→新QueryClient重挂→API重启后值仍正确 | Go真实PG + views装配 + Web E2E；kiki/momo |
| G2 事务与幂等 | 集合字段中途失败全回滚；并发同create key仅1资源/审计；异内容409；CAS两窗口仅1成功；响应丢失后重试无重复创建 | Go真实PG，kiki；momo独立复验 |
| G3 租户/权限 | 两workspace、同用户双成员与不同用户；跨cid/rid/field/cursor拒绝；member建集合403；mat_/mcn_绕头测试；撤权/降级/停用/登录切换 | 全认证router集成 + 浏览器，momo |
| G4 数据库隔离与清理 | 非super/BYPASSRLS角色下跨租户读写；缺context默认拒绝；池A→B→空；workspace删除与创建并发无孤儿；flag关仍清理 | PG/迁移测试，kiki/momo |
| G5 缓存与编辑生命周期 | 同query同field名两源；切源旧读/旧写晚到；原ws缓存收口、新ws零污染；撤权清缓存后不复活；新编辑session保稿；HTTP旧revision不覆盖新值 | core矩阵 + 真实QueryClient/views，kiki |
| G6 分页/收敛 | ≥201行跨页；默认50/最大200及非法limit；相同时间戳id兜底；源/limit/query游标错配拒绝；头页更新裁剪尾页；双客户端WS、断线、丢事件轮询 | Go与E2E，momo |
| G7 无任务副作用 | 通过API建立1000record后，issue计数/编号、agent_task_queue、inbox_item增量0；事件订阅器、运行确认、任务API调用0 | 隔离Go集成 + 实际表装配，momo |
| G8 存量与兼容 | T1/T1b最终结果、只读、自定义属性、modal owner及ws回归；Issue Table分页分组、过滤排序、列宽/私有视图；旧client schema；新增畸形response拒绝伪成功 | core/views全量 + Issue E2E，kiki/momo |
| G9 迁移与回退 | 新库up、旧基线up、并发索引中断恢复、空库down/up、带数据回退/重新启用、cleanup兼容 | Go/隔离Stage，开发与环境负责人 |
| G10 观测与性能 | 精确审计与metrics-only负例、延迟/失败指标、1万行20列首屏和单行p95、键盘编辑/焦点与只读可访问性、Desktop smoke | 隔离Stage，momo |

G7 必须使用隔离数据和受控 subscriber/fake agent，默认测试不运行本机已登录智能体CLI。纯校验矩阵放Go/core无DOM测试，views只验证真实组件接线与特定交错；同一行为不无意义重复铺满每层。字段数50与issue旧配额20互不影响必须有测试，但三种简单类型不代表九类型全量字段引擎完成。

验证命令草案（本轮未执行，新增文件由实现者落地）：

~~~bash
pnpm --filter @multica/core test collections data-source issues/mutations.workspace-routing.test.tsx
pnpm --filter @multica/views test collections data-view issues/components/table-view-editing.test.tsx
pnpm typecheck
pnpm --filter @multica/core lint
pnpm --filter @multica/views lint
pnpm --filter @multica/core test
pnpm --filter @multica/views test
pnpm build
make test
git diff --check 5d182a57f5070365577d7420fb8814dce25f8153 HEAD
~~~

Go新增用例使用testutil/dbfx，make test会迁移并执行race测试，必须绑定隔离DB。数据库集成还需用非superuser角色，不能只让超级用户测试通过。sqlc生成与migrations lint是实现PR必备检查。

本地实际联调：

~~~bash
make up C=api,web
make status
make env-exec ARGS="-- pnpm exec playwright test e2e/collection-table.spec.ts e2e/issue-table.spec.ts"
make up C=api,web,desktop
~~~

`e2e/collection-table.spec.ts` 是本切片拟新增文件；配置项使用现有featureflag机制，默认不开，凭据由环境负责者安全注入。make status提供真实URL/commit；/health验证候选SHA，不能固定写3000。Web与Desktop均需能在sidebar进入集合页，路由为 `/{slug}/collections` 与 `/{slug}/collections/{collectionId}`；共享页面通过NavigationAdapter导航，两端同时接线。成功创建后才导航；编辑错误内联显示并保稿，恢复焦点/键盘操作可用。

| 环境 | 数据与配置 | 准入与责任 |
| --- | --- | --- |
| 本地 | 独立库/端口，两个合成workspace及owner/admin/member；API角色无RLS绕过 | kiki完成每条窄链路；make down保留数据，不销毁验收库 |
| CI | PostgreSQL17隔离数据库、随机测试用户、无真实agent/外部账号 | 类型/lint/契约约5分钟；TS/Go/迁移约15～25分钟，按首轮实测调整 |
| 隔离Stage | 同候选构建、同PG主版本/拓扑/迁移顺序/账号权限，合成1万行；仅容量和沙箱配置不同 | momo执行E2E/WS/Desktop约15分钟，压测/回退另计；不得借用共享生产数据 |
| 共享Stage/生产 | T2默认关闭，保留新表；正式账号、开启范围与RLS推广由T4演练后确认 | 用户最终验收与发布授权仍在后续；T2交付不能触发开放 |

测试数据用现有TestApiClient/dbfx建立和清理，每个suite唯一workspace；先测workspace清理，失败保留seed与trace用于诊断，按规则清理。超时/环境缺Go/Views全量中止均记“未验证”，不能把旧head通过数或隔离重跑算作全量通过。PR交接提供CI当前状态即可，不轮询CI；若本地无法跑真实联调，交付PR和明确门禁缺口，不能标T2验收通过。

PR #4 的Views全量与具备Go环境的真实Web API/WS、Desktop门禁继续累计到同一T2候选；已有定向复验不替代这些项。

**纵向落地与堆叠方案**

T2 内部建议拆两个可独立审阅/回退的纵切提交组；第一组即可运行最小闭环，不先交“只有DDL/types”。

| 切片 | 目标、模块与依赖 | 独立验收 | 风险/规模 |
| --- | --- | --- | --- |
| T2a 基础真实闭环 | 三表/新表隔离、auth/事务/cleanup、集合与记录C/R、标题U、Query+共享TableView、双端路由；依赖冻结T1b | 真实PG持久标题，create幂等/CAS/租户/清理/flag关闭均通过；未接线字段在UI只读 | 事务权限与迁移，M（2～3天） |
| T2b 简单字段闭环与证据 | 三种初始字段的set/clear、编辑会话、WS/轮询、审计指标补齐、性能与全量回归；依赖T2a | G0～G10完整证据，text/number/checkbox/空值链路及同源并发通过 | 编辑竞态、真实联调，M（2～3天） |

T2a即包含所有已开放写路径的审计、权限与回退；T2b只补新增字段路径的观测，不能把安全与审计欠账留到后面。如果按一个PR交付，两组完成才具备完整T2验收资格；若拆PR也使用同样冻结堆叠规则。预计4～6开发日，1～2天独立复验可与后半段证据准备交错；不是整个G-1工期承诺。无需新建agent或squad，本轮也不创建子issue。

~~~mermaid
flowchart LR
  B[冻结 T1b] -->|数据源基线| A[T2a 标题持久闭环]
  A -->|真实数据与授权| C[T2b 字段读写闭环]
  B -->|锁定协议| Q[准备独立测试样本]
  Q -->|输入矩阵| V[同 SHA 全部门禁复验]
  C -->|候选构建| V
  V -->|T2证据| N[后续 T3 与 T4]
~~~

关键路径是冻结T1b→T2a→T2b→独立复验；momo可并行准备测试数据，不能用并行提前放行未经实现的协议。T3仍需另锁doc契约，T4负责完整授权开放。

推荐新分支 `feature/cortex-g1-collections` 从完整 `5d182a57f5070365577d7420fb8814dce25f8153` 创建。初始新PR base为 `feature/cortex-g1-tableview`，比较区间仅 `5d182a5..T2_HEAD`；标题例如“MAGI-11: add collection records through shared TableView”。不写 Closes/Fixes MAGI-11。

1. #3/#4未合入前，保持两个父head冻结。T2可开发/评审/测试，不能合入#4的head分支；#4也不能先合入#3的head分支，否则父PR的已复验范围会变化。
2. #3实际合入后，先处理#4：若原提交仍是目标分支祖先，改base并核对diff；若squash/rebase merge，则只移植旧 `53f49f0..5d182a5`，重新取得T1b新head并复验。实际目标目前是docs/architecture-handbook，不自行改为main。
3. #4重写后，T2只移植 `5d182a5..T2_HEAD` 到已复验的新T1b head。若#4已直接合入目标分支，也可以把同一T2增量移植到该目标最新提交。不要以“旧SHA是否可达”单独替代树/diff核验。
4. #4最终合入后，T2再指向它的实际目标分支；squash时按旧冻结T1b边界移植，保留原T2备份ref，检查range-diff和最终PR diff。父base变化或冲突解决均产生新验收SHA，重跑受影响门禁，发布候选跑完整G0～G10。
5. 实现者可采用下列操作思路，尖括号内容必须替换为当时已核实的提交，不直接照抄执行；本轮未执行这些写操作。

~~~bash
git rebase --onto <new-reviewed-T1b-head-or-actual-target> 5d182a57f5070365577d7420fb8814dce25f8153 feature/cortex-g1-collections
git range-diff 5d182a57f5070365577d7420fb8814dce25f8153..<old-T2-head> <new-base>..<new-T2-head>
~~~

`--onto` 能移植给定边界之后的提交，依据：[Git rebase 文档](https://git-scm.com/docs/git-rebase#_transplanting_a_topic_branch_with_onto)。原始冻结边界必须保存在PR描述，不能重复重放T1/T1b。更新远端前协调父子分支，必要时仅对自己的T2分支使用force-with-lease；不改冻结父分支。

**未决项、正式记录与本轮验证范围**

目前没有阻塞本次隔离实现的需求问题。采用的明确假设为：workspace级集合、三个简单字段类型、创建时定目录、人类凭证、在线Web+Desktop、固定顺序分页。它们限制T2的验收范围，不取消G-1后续功能。T4前必须确认共享部署的账号/RLS推广安排，T5前重新核对project/schema管理与九类型扩展；真实数据规模与硬件目标根据首轮测试修订。

容量假设：每workspace每天1,000行、平均2KiB值袋，一年约36.5万行/~0.7GiB值袋，未计索引、tuple、WAL；10倍增长约365万行/~7GiB。T2不分片/分区。单集合50万行、workspace近千万行或索引/延迟持续越标时重新评估归档/分区；不能把64KiB上限当平均值。

ADR草稿建议由实现者或用户记录到 `docs/architecture/adr/NNNN-cortex-t2-collection-record.md`：状态“拟议，T2隔离范围内采用”；上下文为G-1和已通过T1b；决策为三表+JSONB、不可变初始目录、行CAS、创建键幂等、新表RLS、人类限定、冻结堆叠；后果为明确冲突重试和T4开放前门禁；备选为固定列/字段级CAS/等待合并。架构师不直接落盘该仓库ADR。

本轮完成issue/线程、原方案与T1b契约及最新QA附件、固定源码、构建入口和GitHub PR状态的只读核验。multica repo checkout因目标路径已存在而失败，未覆盖或清理该路径；源码改用GitHub按完整SHA下载的归档只读检查，未据当前工作目录旧HEAD下结论。未修改业务代码/实施分支/PR，未执行DDL、启动环境、运行功能/自动化/性能测试；本文门禁均是实施后的要求，不是本轮通过声明。仅交付此契约附件与线程回复，MAGI-11的G-1仍在进行中。
