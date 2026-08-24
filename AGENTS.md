# Repository Guidelines

This file provides guidance to AI agents when working with code in this repository.

> **Single source of truth:** This file is a concise pointer document.
> All authoritative architecture, coding rules, commands, and conventions
> live in **CLAUDE.md** at the project root. Read that file first.

## Quick Reference

### Architecture

Go backend + monorepo frontend (pnpm workspaces + Turborepo) with shared packages.

- `server/` — Go backend (Chi router, sqlc, gorilla/websocket)
- `apps/web/` — Next.js frontend (App Router)
- `apps/desktop/` — Electron desktop app
- `packages/core/` — Headless business logic (Zustand stores, React Query hooks, API client)
- `packages/ui/` — Atomic UI components (shadcn/Base UI, zero business logic)
- `packages/views/` — Shared business pages/components
- `packages/tsconfig/` — Shared TypeScript config

### State Management (critical)

- **React Query** owns all server state (issues, members, agents, inbox, workspace list)
- **Zustand** owns all client state (current workspace selection, view filters, drafts, modals)
- All Zustand stores live in `packages/core/` — never in `packages/views/` or app directories
- WS events invalidate React Query — never write directly to stores

### Package Boundaries (hard rules)

- `packages/core/` — zero react-dom, zero localStorage, zero process.env
- `packages/ui/` — zero `@multica/core` imports
- `packages/views/` — zero `next/*`, zero `react-router-dom`, use `NavigationAdapter` for routing
- `apps/web/platform/` — only place for Next.js APIs

### Commands

```bash
make dev              # Auto-setup + start everything
pnpm typecheck        # TypeScript check
pnpm test             # TS unit tests (Vitest)
make test             # Go tests
make check            # Full verification pipeline
```

### Local Go Toolchain

Go is intentionally not added to the Windows `PATH`. For this checkout, use
the complete portable Go 1.26.1 toolchain at
`tmp\\tools\\go1.26.1-full\\go\\bin\\go.exe` (and the adjacent `gofmt.exe`).
The similarly named `tmp\\tools\\go1.26.1` and `tmp\\go-portable` copies do
not contain a complete standard library and must not be used for builds.
For example:

```powershell
& '.\\tmp\\tools\\go1.26.1-full\\go\\bin\\go.exe' test ./internal/handler -run CreativePreAdaptation -count=1
```

Use this native Windows toolchain for local backend tests and the native API
binary build. Do not fall back to a fresh Linux container download when its
proxy path is unavailable.

### Self-Hosted Build And Deployment

For the packaged Docker Compose deployment, use the repository targets below.
This is a separate environment from the local direct-Podman acceptance stack
described below.

```bash
make selfhost          # Pull official images, create .env/secrets if needed, deploy
make selfhost-build    # Build backend and web from this checkout, then deploy
make selfhost-deploy   # Redeploy configured official images without rebuilding
make selfhost-verify   # Verify API health/migrations, web login, and API proxy
make selfhost-stop     # Stop the self-hosted Compose stack
```

- After source changes that must be accepted on the self-hosted stack, run
  `make selfhost-build`, then `make selfhost-verify`.
- Default acceptance endpoints are `http://localhost:3000` (web) and
  `http://localhost:8080/healthz` (API). Their ports can be changed through
  `FRONTEND_PORT` and `BACKEND_PORT`/`API_PORT`/`SERVER_PORT`/`PORT` in `.env`.
- `make dev` is the checkout development workflow, not the packaged
  self-host deployment workflow.
- Keep the existing `.env`, particularly `BROKER_STATE_KEY`, across upgrades:
  changing it makes previously encrypted credential/browser state unreadable.

### Current Local Hybrid Acceptance Stack

The current local acceptance topology keeps stateful services in Podman but
runs the API as a native Windows process so OSS traffic uses the Windows host
network rather than the Podman/WSL egress path:

- `server.exe` from `tmp/native-api-oss-network/` owns `localhost:8080`.
  Confirm the owner before any backend operation with
  `Get-NetTCPConnection -LocalPort 8080 -State Listen`.
- For browser acceptance with Chrome DevTools MCP, keep using
  `http://localhost:3000` so the existing auth cookies apply. If the MCP browser
  shows a blank page or spinner while normal Chrome works, reload with
  `ignoreCache=true`; this is usually stale MCP Chrome cache interacting with
  Next/Turbopack dev chunks, not an app outage. See CLAUDE.md for the MCP
  config.
  If Chrome DevTools MCP reports `Browser.setContentsSize` or repeatedly says
  the `chrome-profile` is already running, check for stale `chrome-devtools-mcp`
  processes using `C:\Users\zhangzhenyu\.cache\chrome-devtools-mcp\chrome-profile`.
  The usual project Playwright probe is not the cause unless it is explicitly
  launched with that same `userDataDir`; `services/crawler-worker` Playwright
  normally uses a temporary profile. Keep the Codex MCP config on Chrome's
  native `--chrome-arg=--window-size=1600,1000`, not MCP's `--viewport`, because
  the latter calls a CDP method unavailable in the local Chrome 129 build. If
  the persistent MCP profile still leaves Next/Turbopack chunks pending after
  the config is fixed, clear browser cache plus localhost CacheStorage and
  ServiceWorker data while preserving Cookies, Local Storage, and IndexedDB.
  Killing the broken MCP process closes the current Codex transport; restart or
  refresh Codex so the server is relaunched from the corrected config.
- If Chrome MCP is unavailable, use the Playwright dependency already installed
  under `services/crawler-worker`. Run Node commands from that directory so
  `import('playwright')` resolves; this reuses the package and does not require
  starting or restarting the crawler worker service:

  ```powershell
  cd .\services\crawler-worker
  node -e "import('playwright').then(async ({ chromium }) => { const browser = await chromium.launch({ headless: true }); const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } }); await page.goto('http://localhost:3000', { waitUntil: 'domcontentloaded', timeout: 15000 }); console.log(await page.title()); await browser.close(); })"
  ```

  This launches a fresh Playwright browser context and does not inherit the
  normal Chrome profile or auth cookies. Expect protected workspace routes to
  redirect to `/login` unless a storage state is provided or the script logs in
  inside that Playwright context. For authenticated browser acceptance, prefer
  Chrome MCP/current Chrome cookies when available; crawler-worker Playwright is
  still useful for unauthenticated route checks, public rendering probes, and
  code-driven page diagnostics.
- `multica-direct-api` is intentionally **stopped**. Do not start, restart, or
  replace it: it would compete for `8080` and return the API to the broken
  Podman/WSL OSS path.
- `multica-direct-postgres` remains the live data service.
  `tmp/native-crawler-worker-network/start-native-crawler-worker.ps1 -Port 19516`
  starts the crawler worker as a native Windows process. This is required so
  its Playwright browser uses the same `127.0.0.1:7897` proxy path as the
  interactive Windows Chrome session; keep `multica-crawler-worker` stopped
  while the native worker owns `19516`.
  `multica-direct-postgres-host-bridge` maps only
  `127.0.0.1:15432` to the existing Postgres container so the Windows API uses
  the same database, accounts, material records, and encrypted state.
- Start or restart the native API through
  `tmp/native-api-oss-network/start-native-api.ps1 -Port 8080`. The script
  reads the stopped API container's existing runtime configuration without
  printing secrets, rewrites its database host to the loopback bridge, and
  keeps OSS reads on the Windows network.
- Before accepting an API update, build the current `server/cmd/server` source
  as a Windows binary, replace `tmp/native-api-oss-network/server.exe`, then
  restart the native process through the script and poll `/healthz`. Keep the
  Postgres container, crawler, bridge, and existing volumes intact.
- This checkout does not require a host Go SDK. Build the native API with the
  verified `docker.io/library/golang:1.26.1` Podman image and the existing
  workspace caches:

  ```powershell
  $workspace = (Resolve-Path .).Path
  podman run --rm -e GOOS=windows -e GOARCH=amd64 -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false `
    -v "${workspace}:/workspace" `
    -v "${workspace}/tmp/go-mod-cache:/go/pkg/mod" `
    -v "${workspace}/tmp/go-build-cache:/root/.cache/go-build" `
    -w /workspace/server docker.io/library/golang:1.26.1 `
    sh -lc '/usr/local/go/bin/go build -ldflags "-s -w" -o /workspace/tmp/native-api-oss-network/server.next.exe ./cmd/server'
  ```

  Only after that command succeeds, stop the confirmed native `server.exe`,
  retain a timestamped copy of the old binary, rename `server.next.exe` to
  `server.exe`, and start it with `start-native-api.ps1 -Port 8080`.
- The native OSS SDK completed a real `GetObject` for an archived material in
  this topology. Treat a future container-side OSS timeout as a deployment
  regression, not missing OSS data or a reason to fall back to local files.

Docker Compose on Windows normally uses Docker Desktop's WSL2/virtualized
Linux networking, so it is not a substitute for this host-network API path.
Do not use `make selfhost*`, `podman compose up`, or a new Compose stack for
this local data set.

### Legacy Direct-Podman API Procedure

When `podman ps` shows `multica-direct-api`, `multica-direct-postgres`, and
`multica-crawler-worker`, this checkout is using the existing local acceptance
stack, not the packaged Docker Compose deployment. It owns the live local
Postgres volume, credential state, and the normal acceptance ports:

```powershell
podman ps --format '{{.Names}}|{{.Status}}|{{.Ports}}'
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/healthz
```

- Do **not** run `make selfhost*`, `podman compose up`, or create a new
  Compose stack against this environment: that creates a different database
  and can conflict with `localhost:3000`, `8080`, and `19516`.
- Do **not** remove or recreate any `multica-direct-*` container or its volume
  for a routine code deployment. The direct API is backed by
  `multica-direct-postgres`; the crawler is a separate
  `multica-crawler-worker` container.
- `localhost:3000` is the Next.js process from this checkout. The current
  hybrid stack above is the local default; use this legacy procedure only when
  an explicit switch back to a container API has been requested.

For a tested backend-only update, build a Linux `server` binary, preserve a
copy of the current `/app/server`, copy the new binary to a temporary path in
the running API container, set mode `755`, atomically replace `/app/server`,
then restart and poll `/healthz`. For example:

```powershell
podman cp .\tmp\server-linux multica-direct-api:/tmp/multica-update-server
podman exec multica-direct-api sh -lc "set -eu; chmod 755 /tmp/multica-update-server; cp /tmp/multica-update-server /app/server.next; chmod 755 /app/server.next; mv /app/server.next /app/server"
podman restart multica-direct-api
Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/healthz
```

If the update fails health checks, restore the saved `/app/server` with the
same copy-and-restart procedure. For a migration, entrypoint, or image-base
change, first build and validate a replacement direct image while reusing the
existing `multica-direct` network and Postgres container; never switch to the
self-hosted Compose stack as a shortcut.

### Direct Agent Runtime And Collector

The direct-Podman API stack and the `direct-image2` host daemon are separate.
The daemon executes AppGrowing collection, native analysis fanout, and image
work; restart it through `scripts/start-direct-image-daemon.ps1` after a CLI
runtime update or a daemon configuration change.

- Before accepting a collector update, confirm the daemon's profile CLI
  supports `multica task fanout --help`. A stale CLI can finish the browser
  crawl but cannot enqueue the reference-analysis tasks.
- When changing a built-in creative Skill under
  `scripts/creative-platform-skills/`, update its persisted workspace Skill
  content, config version, and `references/` files before creating a new task.
  Source files alone do not change the Skill snapshot already used by the
  running workspace.
- `delegate_preanalysis.py` resolves its executable as explicit `--cli`, then
  `MULTICA_CLI`, then Windows `multica.com`, then `multica`. Preserve that
  order. It validates native fanout before reading the Crawl Run; an error
  `multica task fanout unavailable; upgrade CLI` means the materials remain
  imported but automatic analysis was not queued.
- Do not use direct HTTP or database writes to repair a missing fanout. After
  the CLI/daemon is corrected, re-run the collector's native, idempotent
  preanalysis delegation for that exact Crawl Run.

See CLAUDE.md for the complete command reference.

### Platform Change Log

Bug fixes, internal refactors, and implementation-only changes do not require a
changelog entry. Add a dated entry here for a major feature or an important
user-visible behavior change. User-facing entries must also update the
platform-owned changelog under `packages/views/locales/*/changelog.json`; keep
the version and release id in `packages/views/changelog/releases.ts` as the
single source of truth. The first platform release is `0.3.23`; when there is
no previously seen release, the release announcement opens by default.

- **2026-08-18 / 0.3.42** — 精准调整改为基于同尺寸无品牌 generated 底图交给素材_改图 direct_image_edit 智能体；未改尺寸复用上一 revision 的底图和过程证据，调整前后成图在对比区直接并列，直接改图结果单独登记过程资产。

- **2026-08-18 / 0.3.43** — 精准调整改为只修改贴片前的无品牌底图：有标注时上传同尺寸编号红框引导图作为第二模型输入，模型回图无论采用与否先登记过程资产；采用的底图由平台确定性重新贴片并直接登记 delivered，不创建 QC task，失败 revision 可从同一 source revision 重试。

- **2026-08-18 / 0.3.44** — 移除独立贴片 Agent，改为绑定到素材_出图和素材_改图的贴片 Skill；模型原图、规范化底图、后端 Prime 合成成图按尺寸登记过程证据，标准出图合成后进入 QC，直接改图合成后直接交付；视觉自动返工仅限真实 Prime 遮挡或官方 Prime 文字不可读且最多一次。已有 ready 工作区按现有 bootstrap 路径刷新 Skill 快照、Agent 绑定并归档旧贴片 Agent，不覆盖用户自定义资源。
- **2026-08-19 / 0.3.45** — 创意工厂支持按工作区启用，并串联素材采集、分析、预适配、素材与文案确认、生图和交付；子 issue 按工作流阶段、变体、版本和 issue 编号稳定排序；需求支持按标签筛选，多选标签按 OR 语义匹配；平台统一更新日志入口并支持中文、英文、日文和韩文，同时优化任务恢复、状态同步和过程结果展示，减少流程中断、重复执行和状态误判。

- **2026-08-19 / 0.3.46** — 交付图片按 MM_P_AK_MY_YYYYMMDD_类型_活动_机型_制作方_尺寸缩写、视频按 MM_V_AK_MY_YYYYMMDD_类型_活动_机型_制作方_尺寸缩写_时长命名，尺寸缩写固定为 11、169、191、916、45；精准改图只使用运行时 task ID，asset-put 的 metadata/evidence 由四份证据交给 CLI 生成，已有回图的协议错误复用同一结果修复，Prime 回填完成后必须回读订单确认所有尺寸和过程图后才能 complete；隔离底图对位 demo 支持 84%–108% 自由缩放与满版覆盖切换，透明边缘不做拉伸补纹理。

- **2026-08-18 / 0.3.41** — 移除 Prime 流程对二维码的强制验证：发布、确定性合成和 QC 不再要求读取、解码或校验二维码；若上传模板或最终成图含二维码，则记录可解码内容作为非阻断证据，完整透明贴片按上传文件原样叠加；无二维码市场可直接上传并发布，文件、尺寸、布局和合成完整性校验保留。创意工厂初始化、默认资源快照和现有工作区 bootstrap 同步到 Prime 合同 v6。

- **2026-08-18 / 0.3.40** — 预适配数值改为以我方冻结还款方案为准：实际渲染行数取原图可渲染行数与我方兼容已审核方案数的较小值，原图多出的数值区块作为默认移除项保留追踪；我方没有某一期限方案时不借用其他期限金额。预适配 Skill、分析 Agent、创意工厂初始化和现有工作区 bootstrap 同步到同一版本契约；数值 layout 的渲染说明必须逐字列出冻结展示值，服务二进制内置 IANA 时区数据，确保 Windows 部署可创建计划触发器。

- **2026-08-18 / 0.3.39** — 修复还款计划未匹配时确认页退化为普通空文案的问题：保留原始数字区块，恢复已审核还款方案选择；用户选择后只冻结我方方案和数值版式，不自动复制或猜测竞品金额。

- **2026-08-18 / 0.3.38** — 拆分出图产物状态与原素材参考分析状态：已有出图订单的已归档图片仍可重新分析，已生成素材仍可再次选入新的出图确认；已拒绝和无可配置文案素材不重复排队，重新分析不会自动复制已有订单。

- **2026-08-18 / 0.3.37** — 素材卡片、预览和确认页统一保留分析中的“重新分析”入口，并复用幂等排队动作；普通文案没有兼容的已审核片段时，服务端将 `missing` 兜底为带依据、无文案库来源的 `recommended` 候选，金融数值、利率、法律文字、二维码和品牌事实仍必须使用审核来源或冻结计算。

- **2026-08-18 / 0.3.35** — 文案库没有兼容已审核片段时，普通文案由模型或平台恢复路径生成 `recommended` 候选，`source_keys` 保持为空并写入推荐依据；金融数值、利率、法律文字、二维码和品牌事实仍必须有审核来源或冻结计算。确认页显示“待确认推荐”，用户采用或改写后才允许提交订单；创意工厂初始化 Skill 版本升至 23，分析 Agent 同步新契约。

- **2026-08-18 / 0.3.36** — 已归档的图片素材始终保留“重新分析”入口，包括“等待分析完成”和“可用”状态；用户可手动重新排队当前工作区的参考分析，用于恢复未真正执行的 Agent 或验证新的分析逻辑，已生成和已拒绝素材不重复排队。

- **2026-08-17 / 0.3.34** — 预适配改为按可渲染数值行做部分映射：已匹配行继续使用冻结方案，容量外或无法匹配的区块保留原始身份并交给确认页选择或留空移除；模型提示词补齐后端字段、容量和冻结值校验，金融语义字段没有真实命中审核文案时不会套用通用标题兜底，失败修正最多一次。素材库的自动继续入口已收敛为一个，避免同一分析重复排队。本版本已部署，并通过平台入口重跑 `test-creative5` 的两张素材。

- **2026-08-17 / 0.3.33** — 修正创意工厂订单边界：采集和预适配自动运行，但仅用户选图并确认文案后创建订单；daemon 恢复只修复预适配，不再自动选中素材或补建后台订单。

- **2026-08-17 / 0.3.29** — 创意工作台第四项改为本周交付，保留异常、状态、采用和时间明细供后续统计；残留的无活动任务队列不再冒充进行中。流程反馈按完整交付包计算产线采用率，平台自有更新日志改为显示可读的多语言内容。

- **2026-08-17 / 0.3.30** — 修复创意工厂首次开启后资源已写入但平台列表不可见的问题：初始化会创建或复用工作区级“创意工厂自动化”，已有安装可在再次开启时补齐自动化元数据，前端同步刷新智能体、Skills、小队和自动化缓存；不覆盖用户已编辑的工作区资源。

- **2026-08-17 / 0.3.31** — 修正创意工厂初始化的自动化职责：采集计划现在创建长 Prompt 的“印尼竞品素材周度采集”，绑定当前工作区的“素材_采集” Agent，并保留 2 张图片、图片过滤、分析 fanout 和凭证恢复契约；仅迁移之前误建的默认短描述小队记录，用户编辑过的自动化不覆盖。

- **2026-08-17 / 0.3.32** — 对齐当前 Ad Creative 的创意工厂 Agent、Skill 和提示词模板；初始化种子资源直接可用于预适配，同时保留 Prime 配置待完善状态。无可编辑文字的静态素材不再进入失败重试，分析到预适配的交接错误进入可恢复日志，采集 Agent 的分析绑定按工作区动态注入，用户编辑资源不覆盖。

- **2026-08-17 / 0.3.32** — 修复旧版市场资源包发布时缺少 `prime_template_set` 的兼容问题：已有完整六个 Prime 标准附件会在发布校验阶段补齐标准模板契约；附件缺失、尺寸或二维码不合规仍会阻止发布。

- **2026-08-17 / runtime** — daemon 注册 runtime 时读取工作区 `settings.runtime_providers` 白名单；未配置的工作区保持全量 provider，`test_creative4` 明确只注册 Codex，避免删除其他 runtime 后被 daemon 自动恢复。

- **2026-08-17 / creative-resources** — 创意工厂首次启用时，从随服务发布的 Ad Creative 默认资源快照复制完整文案库、市场规则、Prime 附件与工作区专属资源记录；不依赖任何已有工作区，已有资源及用户编辑不覆盖。

- **2026-08-17 / 0.3.28** — Fixed stale Creative Production blockers while a
  same-Variant continuation is queued or running, so incomplete parent tasks
  remain visibly in progress instead of showing as generation failures. The
  production Skill and workspace producer now register Prime context, model
  originals, and normalized process evidence as each size is produced, and
  require evidence and canonical assets before task completion.

- **2026-08-16 / 0.3.27** — Fixed numeric pre-adaptation failures caused by
  harmless currency whitespace differences and added deterministic repair from
  frozen repayment plans after the bounded model retry. Recent manual-required
  adaptations are recovered automatically after worker or API restarts; manual
  confirmation remains only when the source regions or frozen bindings are not
  safe to infer.

- **2026-08-16 / 0.3.23** — Added workspace-scoped Creative Factory capability
  controls with API, navigation, and direct-page enforcement, connecting the
  workflow from material collection through image generation and delivery.
  Codex agents can select GPT-5.6 Sol, GPT-5.6 Terra, or GPT-5.6 Luna with the
  thinking levels available for each model. The platform also added the
  internal update log, persisted requirement label filters with OR semantics,
  and stable workflow child-issue ordering by stage, variant, revision, and
  issue identifier.
- **2026-08-16 / 0.3.24** — Expanded the pre-adaptation Skill with the exact
  frozen-resource JSON contract and made failed direct tasks retry with their
  original context before falling back to manual confirmation.
- **2026-08-16 / 0.3.25** — Added workspace-owned Creative Factory initialization
  and automatic handoff from a successful pre-adaptation into order creation,
  planning, production, and QC. Collection requests are clamped to two images,
  and the handoff uses the workspace installation's Agent, Skill, prompt,
  binding, resource, and squad snapshots without hard-coded IDs or overwrites.
- **2026-08-16 / 0.3.26** — Made the automatic handoff recover completed
  pre-adaptation tasks after worker restarts, bounded production recovery to
  reuse completed sizes and generate only missing sizes, and aligned the
  persisted production Skill validator with the current semantic prompt
  contract. The collector now resolves the task-runtime CLI by default instead
  of guessing a stale absolute path. The tested path now completes collection,
  analysis, order, planning, production, and QC without a user click. The
  release resource contract now verifies the current version exists in every
  supported locale.
- **2026-08-24 / creative-production** — Removed copy validation as a hard
  generated-asset write blocker, made `asset-put` require only model result,
  prompt contract, and normalization evidence, added size-specific CANVAS LOCK
  prompt guidance for all production sizes, raised default aspect tolerance to
  10%, and documented recovery that reuses existing generated evidence before
  re-running image generation.
