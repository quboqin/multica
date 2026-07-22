# Credential Broker：第三方登录态托管框架设计

本文记录 Multica 的一项通用能力设计：**Credential Broker（登录态托管框架）**。它让云端 AI（agent / autopilot）能够操作"需要登录才能访问"的第三方站点，而账号密码、Cookie、登录态**永不进入 AI 上下文**。

首个落地场景是从 AppGrowing Global 抓取竞品广告素材，但框架本身与站点无关——AppGrowing 只是第一个 connector。设计目标是：**以后再接入任意一个"登录后才能取数"的平台，成本≈填一张几行的声明表**。

Autopilot / Issue 如何用自然语言驱动爬虫、Credential Broker 与 Connector 能力库的产品契约，见 [`crawler-agent-connector-contract.md`](./crawler-agent-connector-contract.md)。

> 术语：本文的"登录态"指浏览器登录后的完整会话凭证（Cookie + localStorage，即 Playwright `storageState`）。

## 1. 背景与约束

目标页面需要登录后访问，例如：

```
https://auth.youcloud.com/login?app_id=en_appgrowing&goto=...
```

Multica 的 AI 运行在云端，因此有一组硬约束：

1. AI 不接触账号、密码。
2. AI 不接触 Cookie、storageState、Authorization Header。
3. 登录后的完整 HTML、请求头、浏览器存储不回传给 AI。
4. 用户在受控的远程浏览器中**亲自**登录。
5. 系统以 `profile_id` 引用登录态；自动任务只使用 `profile_id`。
6. 日志、错误、返回结果必须脱敏。

这些约束对任何需要登录的第三方站点都成立，因此值得抽象成一个通用框架，而不是为 AppGrowing 单独实现一次。

## 2. 核心不变量

无论接入多少个站点，以下三条始终成立，是整个框架的安全地基：

- **AI 边界不变量**：AI 上下文里永远只有 `profile_id` + 业务参数；返回给 AI 的只有结果摘要（数量、存储位置、状态码）。
- **加密不变量**：登录态密文用 **Worker 持有的密钥**加密。**server 只存储密文、无法解密**；即使 server DB 整体泄露，拿到的也只是密文。
- **浏览器一次性不变量**：真实浏览器只在"用户登录"这一步出现一次，用完即销毁；后续取数不再依赖浏览器。

## 3. 三层架构

框架分三层，**只有中间一层是每接入一个新站点需要新写的，且非常薄**。

```
┌──────────────────────────────────────────────────────────────┐
│ ① Broker 平台层  (写一次，接新站点时不改)                      │
│   - login session 生命周期：开远程浏览器 / TTL / 销毁          │
│   - profile 状态机：PENDING → ACTIVE → NEED_REAUTH → REVOKED   │
│   - 归属校验、参数脱敏、日志脱敏、探活调度、失效通知           │
│   - 维持 AI 边界不变量                                          │
├──────────────────────────────────────────────────────────────┤
│ ② Connector 适配层  (每个站点一个，只声明 5 件事)              │
│   AppGrowing 为首个实例；见 §5                                  │
├──────────────────────────────────────────────────────────────┤
│ ③ Secret Store  (可插拔：DB 密文列先行，预留 KMS / Vault)      │
│   Worker 持密钥，统一 Seal / Open 接口                         │
└──────────────────────────────────────────────────────────────┘
```

### 3.1 组件与部署

```
[一次性] 用户 ── 远程浏览器登录 ──> Worker 导出 storageState ── 加密 ──> Secret Store
                                                                            │
[周期性] autopilot cron ─> agent 跑 `multica crawl run` ─> server 校验归属并转发
                                                                            │
                                                    Crawler / Broker Worker
                                                    ├─ 从 Secret Store 取密文并本地解密
                                                    ├─ 按 connector 声明注入登录态
                                                    ├─ 执行业务动作（取素材 / 选片 / 下载）
                                                    └─ 回报结果（已脱敏）
                                                                            │
                                                                        对象存储 (S3/OSS)
```

- **Broker 平台层 + Secret Store 接口**：落在 Go server（`server/internal/handler` + 新增 `server/internal/broker` 或类似包）。
- **Worker**：新顶层目录 `services/crawler-worker/`（不受 `apps/` `packages/` 边界规则约束）。持有解密密钥，是唯一能看到明文登录态的进程。
- **对象存储**：复用 `server/internal/storage` 的 `Storage` 接口（S3 / local 双实现）。

## 4. 数据模型

登录态与元数据**分层存储**，这是可扩展与安全边界的关键。

```
credential_connector   -- 静态注册，代码里定义即可，不一定进表
                          (id, login_url, 成功判定规则, 注入方式, 探活方式)

credential_profile     -- id, workspace_id, authorized_by_id, connector_id, label,
                          status (PENDING / ACTIVE / NEED_REAUTH / REVOKED),
                          last_used_at, expires_hint, created_at, updated_at
                          → server 完全可读：用于工作区解析、UI 展示、审计

credential_secret      -- profile_id, ciphertext BYTEA, key_version, updated_at
                          → server 只存不解；单独成表，访问权限比 profile 更严

login_session          -- 以内存对象为主；如需审计可轻量落表
                          (profile_id, expires_at, status)
```

**`connector_id` 是扩展的支点**：今天是 `appgrowing`，明天加 `some-adnetwork`、`internal-bi`，上面三张表和所有接口一行不用改。

**归属：工作区共享。** 一个工作区中，每个 connector 最多保留一个未撤销的 profile。`authorized_by_id` 只记录最近完成登录的人，供审计使用，不参与爬取解析。成员可以查看状态和触发获准的抓取；owner/admin 才能绑定、重新授权、验证和撤销。Cookie 和浏览器状态仍不会返回给任何用户或智能体。

**状态机：**

```
      绑定发起            登录成功            探活失败/401
PENDING ─────> (登录中) ─────────> ACTIVE ────────────────> NEED_REAUTH
   │                                 │                          │
   │ TTL 超时/放弃                    │ 用户解绑                 │ 重新授权(覆盖密文)
   └──────> (丢弃)                    └──────> REVOKED <─────────┘
                                                 (删 profile + 删密文)
```

## 5. Connector 适配层：只声明 5 件事

接入一个站点，只需声明下面 5 项；登录、加密、存取、调度、脱敏、通知全部复用平台层。

| 声明项 | 含义 | AppGrowing 的值 |
|---|---|---|
| `login_url` | 远程浏览器要打开的登录页 | `https://auth.youcloud.com/login?app_id=en_appgrowing` |
| `login_success` | 何时判定登录成功、可导出登录态 | URL 离开 `/login` 且关键 Cookie 出现 |
| `inject` | 使用时如何把登录态变成一次请求 | 从 storageState 取 Cookie 注入 GraphQL 请求头 |
| `health_check` | 如何探活 / 检测失效 | 打一次 `userinfo` query，非 401 即有效 |
| `capabilities` | 这个 connector 支持的业务动作（供 CLI / agent） | `fetch_materials`（取素材+选片+下载） |

除 `capabilities` 涉及站点独有的业务逻辑（可能需要少量代码）外，前 4 项都是纯声明。

## 6. 鉴权流程

### 6.1 首次绑定（login session 时序）

```
用户                 Multica server           Broker Worker            远程浏览器(Chromium)
 │                       │                          │                        │
 │ owner/admin 点"绑定"   │                          │                        │
 ├──────────────────────>│ 建 credential_profile     │                        │
 │                       │ (status=PENDING)         │                        │
 │                       │ POST /login-sessions     │                        │
 │                       ├─────────────────────────>│ 起 Chromium，打开        │
 │                       │                          │ connector.login_url     │
 │                       │                          ├────────────────────────>│
 │                       │                          │ 生成一次性 token         │
 │                       │                          │ TTL=15min，绑当前用户    │
 │                       │  browser_url + expires_in│                        │
 │                       │<─────────────────────────┤                        │
 │  browser_url          │                          │                        │
 │<──────────────────────┤                          │                        │
 │ 打开 browser_url(新窗口)                          │   ← 实时画面 →          │
 ├───────────────────────────────────────────────────────────────────────────>│
 │ 输账号密码 / 过验证码 MFA                                                    │
 ├───────────────────────────────────────────────────────────────────────────>│
 │                       │                          │ 每 2s 轮询 login_success│
 │                       │                          │<────────────────────────┤
 │                       │                          │ ✓ 成功 → storageState()  │
 │                       │                          │ Worker 密钥加密 → 存密文  │
 │                       │  profile ACTIVE          │ 关闭浏览器，token 失效    │
 │                       │<─────────────────────────┤────────────────────────>│ ✗销毁
 │  UI 显示"已连接"       │                          │                        │
 │<──────────────────────┤ (WS 或轮询 profile 状态)  │                        │
```

`browser_url` 不是 AppGrowing 的网址，而是 **Worker 那台远程浏览器的实时操作入口**（browserless live view 或 noVNC）。用户与 Worker 操作的是**同一个浏览器会话**，因此用户登录成功后 Worker 天然持有登录态，只需导出加密，无需替用户操作。账号密码只经过 AppGrowing 官方登录页，既不进 AI，也不进 Multica DB。

**边界处理：**

| 边界情况 | 处理 |
|---|---|
| token 到期（15 分钟未登完） | Worker 关浏览器、token 失效，profile 保持 PENDING；用户重新绑定拿新 URL（旧 URL 打开为 410 Gone） |
| 用户中途关掉页面 | 远程浏览器仍在跑，Worker 轮询到超时后按到期处理；未检测到成功则**绝不导出**，无残留登录态 |
| 登录失败（密码/验证码错） | 页面停在 `/login`，Worker 检测不到成功，用户可在同一页面重试，不用换 URL |
| Worker 崩溃 | login session 是内存对象，随之消失；profile 停在 PENDING，用户重新发起即可 |
| 并发/重复点绑定 | 同一 profile 已有 pending session 时，先废弃旧 session 再开新的，避免占资源 |

三条安全约束：token 一次性 + 短 TTL + 仅发起绑定的管理者持有；检测不到成功绝不导出登录态；浏览器用完立即销毁、不复用。

### 6.2 日常使用（autopilot 每次触发）

```
1. agent 执行: multica crawl run --connector appgrowing --timeout 5m --week 2026-W27
2. server: 用 workspace_id + connector_id 解析该工作区唯一 active profile
           + 硬性拒绝参数里出现 cookie/authorization/password/token 键 (400)
3. 转发 Worker → Worker 取密文本地解密 → connector.inject 注入 → 先 health_check 探活
4. 有效: 执行业务动作 → 下载 → 返回 { downloaded, output_prefix }
5. 失效: 返回 NEED_REAUTH，server 置 profile.status = NEED_REAUTH
```

Worker 日志出口统一脱敏：`Cookie / Set-Cookie / Authorization / *_token → [REDACTED]`；错误只回脱敏错误码。

### 6.3 失效与重授权

- profile 置 NEED_REAUTH → 在工作区集成页标记为需要重新授权；agent 侧遇到 NEED_REAUTH 在 issue 留言等待，**不自行重试**。
- 重授权 = 重跑 §6.1，覆盖旧密文；解绑 = 删 profile 行 + 删密文（REVOKED）。

## 7. 加密与 Secret Store

### 7.1 与 Lark 加密的区别

Lark 的 `app_secret` 用 **server 自己的密钥**（`server/internal/util/secretbox`）加密，server 能解密。Credential Broker 的登录态用 **Worker 的密钥**加密，**server 永远解不开**——它只是密文保管员。这是比 Lark 更强的边界：单独拿到 server DB 无用，必须同时拿到 Worker 密钥才失守。这是本能力的默认姿态。

- 算法复用 `secretbox` 的 AES-256-GCM 构造。
- 密钥独立：`BROKER_STATE_KEY`（base64 32 字节），**只配置在 Worker 进程的环境变量**。
- `credential_secret.key_version` 预留密钥轮换。

### 7.2 可插拔 Secret Store 接口

一期**接口先立、实现只做 DB**（密文存 `credential_secret.ciphertext`），预留 KMS / Vault 实现，将来切换不动上层：

```
SecretStore:
  Put(profile_id, plaintext) -> 加密并持久化
  Get(profile_id) -> 解密并返回明文（仅 Worker 侧可成功）
  Delete(profile_id)
```

## 8. UI 位置：Settings → Integrations

绑定入口放在 **Settings → Integrations tab**，与 Lark 并列一个 section。

- `packages/views/settings/components/integrations-tab.tsx` 已是"第三方平台连接"的 umbrella host（注释原文：each integration owns its own description and install flow），加 section 即可，IA 不变。
- **不放在 Agent 详情页的 integrations tab**：那是配"单个 agent"的；登录态是 autopilot 触发的任意 agent 都要复用的共享凭证，绑到具体 agent 上语义错位。
- section 内容：工作区共享 profile 列表（状态徽标 ACTIVE / NEED_REAUTH）；所有成员可查看状态，owner/admin 可使用"绑定账号"、重新授权、验证和解绑。
- 由 connector 注册表驱动渲染：新增 connector 时该 tab 自动多出对应 section。

前端遵循仓库既有规则：query hooks 放 `packages/core/`，key 带 `wsId`；响应体走 `parseWithFallback` + zod schema，同 PR 带 malformed-response 测试。

## 9. 工具面（CLI，agent 的接口）

通用命令，不随站点增加：

```
multica creds bind    --connector appgrowing            # 发起绑定，返回 browser_url
multica creds list                                       # human-only，列出工作区 profile 及状态
multica crawl run     --connector appgrowing --timeout 5m [业务参数...] # agent/autopilot 执行业务动作
multica crawl run     --profile-id <id> --timeout 5m [业务参数...]      # human/manual 指定 profile
multica crawl status  --job <id>
```

返回体只含：job 状态、数量、`output_prefix`。绝不返回 Cookie、Header、完整 DOM。

## 10. 首个 Connector：AppGrowing 取素材

### 10.1 数据获取方式：GraphQL 直连，不爬页面

页面背后是稳定的 GraphQL 端点 `https://api-appgrowing-global.youcloud.com/graphql`。取素材用 `appMaterialList`（detailed 版）query，返回体中：

- `creative.resource.path` = 图片/视频真实 CDN 地址（含 `width/height/format/poster`）。
- `duration` = 投放天数，`impression_inc_2y` = 曝光估算 —— 正好是选片要的字段。

Playwright 只用于 §6.1 拿登录态；取数是纯 HTTP GraphQL，连浏览器都不再开。相比"用 Playwright/MCP 读页面"：更稳（schema 变更慢）、更快（一次 POST 拿一页）、不把页面 HTML/网络日志带进 AI。

> 参考实现 `ronaldo123321/appgrowing-cli` 已逆出全部 query，请求层可移植；但其"明文 Cookie 存本地文件 + 从本机浏览器抽 Cookie + 默认跳过 TLS 校验"的登录态模式**不可采用**，登录态注入必须改由 Worker 从 Secret Store 读取，且开启 TLS 校验。

### 10.2 选片策略

**竞品池**（配置化，先各跑一次 `searchApp` 拿 `app_brand_id` 固化）：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO；重点更新前三家。市场默认印尼（ID），purpose=2。

常用直接竞品池示例：Easycash、Kredit Pintar、Adapundi、BantuSaku、Rupiah Cepat、UATAS、JULO。Easycash、Kredit Pintar、Adapundi 是重点竞品。该规则属于 agent / autopilot 业务策略，应由调用方作为 `params` 传给 worker，而不是写死在 worker connector 里。

每周每个竞品跑一次 `appMaterialList`，拉回后在 Worker 侧分两桶（API 不支持按投放天数/曝光过滤，须拉回自筛）：

| 桶 | 占比 | 条件（对应字段） |
|---|---|---|
| 新素材 | 40% | `duration` < 7 天 且 `impression_inc_2y` > 1,000 |
| 跑量素材 | 60% | `duration` > 30 天 且 `impression_inc_2y` > 10,000,000 |

每桶按曝光降序取 top，按 40/60 合并，对选中素材的 `creative.resource.path` 去重下载到对象存储。元数据记 `app_brand`、`duration`、`impression`、`bucket`、`captured_at`。

存储路径示例：`s3://internal-bucket/appgrowing/{app_brand}/{yyyy-Www}/{bucket}/{hash}.jpg`

> ⚠️ **字段语义须先验证**：`duration`、`impression_inc_2y` 的确切语义系从 schema 字段名推断。一期第一件事是跑一次真实 API，把返回值与 AppGrowing 网页显示的"投放天数/曝光估算"对齐，确认后再固化阈值。

## 11. 接入新站点要做什么（可扩展性验证）

未来接另一个"登录后才能取数"的平台：

1. 新增一个 connector 声明（§5 的 5 项）—— 唯一必写项。
2. 若有独特业务动作，在 Worker 加一个 handler；取图等通用动作可复用。
3. CLI 无需加命令：`multica creds bind --connector <新站点>` 等全通用。
4. UI 无需改结构：Integrations tab 自动多出一个 section。

数据库、加密、登录会话、autopilot 调度、脱敏、通知 —— **零改动**。

## 12. 最小改动清单

| 改动 | 位置 | 说明 |
|---|---|---|
| 3 张表 | `server/migrations/121_*` | `credential_profile` / `credential_secret` / （可选 `login_session`）。密文列 server 存不解 |
| Broker 平台层 | `server/internal/broker`（新包） | profile 状态机、SecretStore 接口、connector 注册表 |
| Handler | `server/internal/handler/credential.go` | profiles 增删查、`POST /login-sessions`、`POST /crawl`（转发 + 参数脱敏校验）；归属用 `loadProfileForUser` 约定 |
| CLI | `server/cmd/multica/cmd_creds.go` + `cmd_crawl.go` | agent 工具面 |
| Worker | `services/crawler-worker/`（新顶层目录） | login session + 登录态加解密 + connector（AppGrowing GraphQL）+ 选片 + 下载 + 脱敏。**核心全在这** |
| autopilot | 复用现有 | cron → agent task 跑 CLI，无需新调度器 |
| UI | `packages/views/settings/components/` + `packages/core/` | Integrations tab 加 section；query hooks + zod schema |

## 13. 分期

**一期（打通 + 验证）**
1. Worker 骨架：Playwright 一次性登录 + `secretbox` 加密 + SecretStore(DB 实现)。
2. AppGrowing connector：GraphQL 客户端 + **字段语义验证** + 两桶选片 + 下载 S3。
3. `credential_profile/secret` 表 + `POST /login-sessions` + `POST /crawl`。
4. CLI 手动触发单次；暂不做前端页。

**二期（自动化）**
1. 接 autopilot cron → agent task。
2. health_check 探活 + NEED_REAUTH → inbox 通知。
3. `resource.path` 去重 + 元数据落库。

**三期（平台化）**
1. Settings → Integrations 绑定 UI + profile 管理（web/desktop 共享）。
2. builtin skill 教 agent 用法（含 NEED_REAUTH 处理，遵循 CLAUDE.md 对 builtin skill 的源码追踪要求）。
3. 审计日志（复用 activity 体系）+ 配额限制。
4. Secret Store 增加 KMS/Vault 实现（接口一期已立）。

## 14. 关键决策记录

| 决策 | 结论 | 理由 |
|---|---|---|
| 数据获取方式 | GraphQL 直连 | 稳定、快、不把页面内容带进 AI；Playwright 仅用于登录 |
| 调度 | 复用 autopilot | 已有 cron→agent task、initiator user、重试、通知 |
| 归属层级 | 工作区共享（`workspace_id + connector_id`） | 成员可复用已授权连接；`authorized_by_id` 仅用于审计 |
| 登录态存储 | server DB 密文列 | 支持工作区共享 profile + 多 Worker 副本；server 存不解 |
| 加密密钥 | Worker 持有（非 server） | 比 Lark 更强边界：server DB 泄露仅得密文 |
| Secret Store | 接口先立、一期只做 DB | 抽象成本低，省将来大改 |
| 能力命名 | Credential Broker | 长期存在，代码与 UI 统一露出 |

## 15. 待验证 / 未决

1. **AppGrowing 字段语义**（`duration` / `impression_inc_2y`）需一期用真实 API 对齐 —— 阻塞选片阈值固化。
2. 远程浏览器实现选型：自托管 browserless（自带 live view，胶水少，倾向）vs Chromium + noVNC（更可控）。
3. Worker 技术栈：登录用 Node + Playwright；取数逻辑 Node 一体 vs 移植开源 CLI 的 Python —— 倾向 Node 一体。
4. `login_session` 是否落表：一期内存优先，若需审计再落。
