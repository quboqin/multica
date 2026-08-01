#!/usr/bin/env node

import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";

const repoRoot = fileURLToPath(new URL("..", import.meta.url));
const webRoot = fileURLToPath(new URL("../apps/web", import.meta.url));
const nextBin = fileURLToPath(
  new URL("../apps/web/node_modules/next/dist/bin/next", import.meta.url),
);
const warmupScript = fileURLToPath(
  new URL("./warm-web-routes.mjs", import.meta.url),
);
const port = process.env.FRONTEND_PORT || "3000";
const baseURL = process.env.MULTICA_WEB_WARMUP_URL || `http://localhost:${port}`;

console.log(`[web dev] starting Next.js on ${baseURL}`);
const next = spawn(
  process.execPath,
  [nextBin, "dev", "--turbopack", "--port", port],
  {
    cwd: webRoot,
    env: process.env,
    stdio: "inherit",
  },
);

let warmup;
if (process.env.MULTICA_WEB_WARMUP?.toLowerCase() === "true") {
  warmup = spawn(
    process.execPath,
    [warmupScript, "--base-url", baseURL],
    {
      cwd: repoRoot,
      env: process.env,
      stdio: "inherit",
    },
  );
  warmup.on("exit", (code) => {
    if (code && code !== 0) {
      console.warn(`[web dev] route warmup exited with code ${code}; Next.js remains available`);
    }
  });
}

let shuttingDown = false;
function shutdown(signal) {
  if (shuttingDown) return;
  shuttingDown = true;
  warmup?.kill(signal);
  next.kill(signal);
}

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => shutdown(signal));
}

next.on("error", (error) => {
  console.error(`[web dev] failed to start Next.js: ${error.message}`);
  warmup?.kill();
  process.exitCode = 1;
});

next.on("exit", (code, signal) => {
  warmup?.kill();
  if (signal) {
    process.kill(process.pid, signal);
    return;
  }
  process.exit(code ?? 1);
});
