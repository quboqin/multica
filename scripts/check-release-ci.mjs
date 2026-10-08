#!/usr/bin/env node
import { execFileSync } from "node:child_process";
import { pathToFileURL } from "node:url";

export function requireSuccessfulRun(runs, sha, workflow) {
  const candidates = runs.filter((run) =>
    run.head_sha === sha && run.head_branch === "qqb_main" &&
    run.event === "push" && run.repository?.full_name === "quboqin/multica" &&
    run.path === `.github/workflows/${workflow}`);
  candidates.sort((a, b) => b.run_number - a.run_number);
  const run = candidates[0];
  if (!run || run.status !== "completed" || run.conclusion !== "success") {
    throw new Error(`${workflow}: the latest qqb_main push run for ${sha} must succeed before tagging`);
  }
  return run.html_url;
}

export async function checkReleaseCI() {
  if (process.env.GITHUB_REPOSITORY !== "quboqin/multica") throw new Error("Unexpected repository");
  const sha = execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
  for (const workflow of ["ci.yml", "mobile-verify.yml"]) {
    const query = new URLSearchParams({ branch: "qqb_main", event: "push", head_sha: sha, per_page: "100" });
    const response = await fetch(`https://api.github.com/repos/quboqin/multica/actions/workflows/${workflow}/runs?${query}`, {
      headers: { Authorization: `Bearer ${process.env.GH_TOKEN}`, Accept: "application/vnd.github+json" },
      signal: AbortSignal.timeout(30_000),
    });
    if (!response.ok) throw new Error(`Cannot inspect ${workflow}: HTTP ${response.status}`);
    const data = await response.json();
    if (!Array.isArray(data.workflow_runs)) throw new Error("Invalid workflow response");
    console.log(requireSuccessfulRun(data.workflow_runs, sha, workflow));
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  checkReleaseCI().catch((error) => { console.error(error.message); process.exitCode = 1; });
}
