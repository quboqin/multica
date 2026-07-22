#!/usr/bin/env node

import { parseArgs } from "node:util";

const { values } = parseArgs({
  options: {
    "base-url": { type: "string" },
    "workspace-slug": { type: "string" },
    concurrency: { type: "string" },
  },
  strict: true,
});

if (process.env.MULTICA_WEB_WARMUP?.toLowerCase() === "false") {
  console.log("[web warmup] disabled by MULTICA_WEB_WARMUP=false");
  process.exit(0);
}

const port = process.env.FRONTEND_PORT || "3000";
const baseURL = normalizeBaseURL(
  values["base-url"]
    || process.env.MULTICA_WEB_WARMUP_URL
    || process.env.FRONTEND_ORIGIN
    || `http://localhost:${port}`,
);
const workspaceSlug = encodeURIComponent(
  values["workspace-slug"]
    || process.env.MULTICA_WEB_WARMUP_WORKSPACE_SLUG
    || "warmup-workspace",
);
const concurrency = boundedInteger(
  values.concurrency || process.env.MULTICA_WEB_WARMUP_CONCURRENCY,
  2,
  1,
  6,
);
const requestTimeout = boundedInteger(
  process.env.MULTICA_WEB_WARMUP_TIMEOUT_MS,
  300_000,
  10_000,
  900_000,
);

const authRoutes = [
  "/login",
  "/invitations",
  "/invite/warmup-id",
  "/onboarding",
  "/workspaces/new",
  "/auth/callback",
  "/lark/bind",
  "/lark/start",
];

const landingRoutes = [
  "/",
  "/about",
  "/changelog",
  "/contact-sales",
  "/download",
  "/homepage",
  "/usecases",
  "/usecases/warmup",
];
const includeLanding = process.env.MULTICA_WEB_WARMUP_INCLUDE_LANDING?.toLowerCase() === "true";

const workspaceRoutes = [
  "/issues",
  "/issues/warmup-id",
  "/my-issues",
  "/inbox",
  "/projects",
  "/projects/warmup-id",
  "/agents",
  "/agents/warmup-id",
  "/autopilots",
  "/autopilots/warmup-id",
  "/runtimes",
  "/runtimes/warmup-id",
  "/skills",
  "/skills/warmup-id",
  "/settings?tab=integrations",
  "/kpi",
  "/plans",
  "/plans/charts",
  "/plans/kpi",
  "/squads",
  "/squads/warmup-id",
  "/members/warmup-id",
  "/usage",
  "/billing",
  "/attachments/warmup-id/preview",
].map((route) => `/${workspaceSlug}${route}`);

const configuredRoutes = (process.env.MULTICA_WEB_WARMUP_ROUTES || "")
  .split(",")
  .map((route) => route.trim())
  .filter(Boolean)
  .map((route) => route.startsWith("/") ? route : `/${route}`);
const routes = [...new Set(configuredRoutes.length > 0
  ? configuredRoutes
  : [...workspaceRoutes, ...authRoutes, ...(includeLanding ? landingRoutes : [])])];

console.log(`[web warmup] waiting for ${baseURL} (landing=${includeLanding ? "included" : "skipped"})`);
await waitForServer(baseURL, requestTimeout);
console.log(`[web warmup] compiling ${routes.length} routes with concurrency ${concurrency}`);

const startedAt = Date.now();
const failures = [];
let cursor = 0;

async function worker() {
  while (cursor < routes.length) {
    const index = cursor;
    cursor += 1;
    const route = routes[index];
    const routeStartedAt = Date.now();
    try {
      const response = await fetch(new URL(route, baseURL), {
        headers: { "x-multica-warmup": "1" },
        redirect: "manual",
        signal: AbortSignal.timeout(requestTimeout),
      });
      const elapsed = formatDuration(Date.now() - routeStartedAt);
      console.log(`[web warmup] ${response.status} ${route} (${elapsed})`);
      if (response.status >= 500) failures.push(`${route}: HTTP ${response.status}`);
      await response.arrayBuffer();
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      failures.push(`${route}: ${message}`);
      console.error(`[web warmup] failed ${route}: ${message}`);
    }
  }
}

await Promise.all(Array.from({ length: concurrency }, () => worker()));

const elapsed = formatDuration(Date.now() - startedAt);
if (failures.length > 0) {
  console.error(`[web warmup] completed with ${failures.length} failure(s) in ${elapsed}`);
  for (const failure of failures) console.error(`[web warmup]   ${failure}`);
  process.exitCode = 1;
} else {
  console.log(`[web warmup] ready: ${routes.length} routes compiled in ${elapsed}`);
}

async function waitForServer(url, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  const healthURL = new URL("/favicon.ico", url);
  while (Date.now() < deadline) {
    try {
      const response = await fetch(healthURL, {
        redirect: "manual",
        signal: AbortSignal.timeout(3_000),
      });
      await response.body?.cancel();
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 1_000));
    }
  }
  throw new Error(`frontend did not become ready within ${formatDuration(timeoutMs)}`);
}

function normalizeBaseURL(value) {
  const parsed = new URL(value);
  parsed.pathname = "/";
  parsed.search = "";
  parsed.hash = "";
  return parsed.toString();
}

function boundedInteger(value, fallback, min, max) {
  const parsed = Number.parseInt(value || "", 10);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(max, Math.max(min, parsed));
}

function formatDuration(milliseconds) {
  if (milliseconds < 1_000) return `${milliseconds}ms`;
  return `${(milliseconds / 1_000).toFixed(1)}s`;
}
