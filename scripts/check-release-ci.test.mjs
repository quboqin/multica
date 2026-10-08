import assert from "node:assert/strict";
import test from "node:test";
import { requireSuccessfulRun } from "./check-release-ci.mjs";

const run = {
  head_sha: "abc", head_branch: "qqb_main", event: "push", run_number: 1,
  repository: { full_name: "quboqin/multica" }, path: ".github/workflows/ci.yml",
  status: "completed", conclusion: "success", html_url: "https://github.com/run/1",
};
test("release requires the exact commit, source branch and workflow", () => {
  assert.equal(requireSuccessfulRun([run], "abc", "ci.yml"), run.html_url);
  for (const patch of [
    { head_sha: "other" }, { head_branch: "main" }, { event: "pull_request" },
    { repository: { full_name: "other/multica" } }, { path: ".github/workflows/other.yml" },
    { status: "in_progress" }, { conclusion: "cancelled" }, { conclusion: "skipped" },
  ]) assert.throws(() => requireSuccessfulRun([{ ...run, ...patch }], "abc", "ci.yml"));
  assert.throws(() => requireSuccessfulRun([], "abc", "ci.yml"));
});
test("a previous success cannot hide a newer failed or pending run", () => {
  for (const patch of [{ conclusion: "failure" }, { status: "queued", conclusion: null }]) {
    assert.throws(() => requireSuccessfulRun([run, { ...run, run_number: 2, ...patch }], "abc", "ci.yml"));
  }
});
