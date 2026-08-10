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
