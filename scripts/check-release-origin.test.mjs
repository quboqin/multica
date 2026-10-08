import assert from "node:assert/strict";
import { execFileSync, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import test from "node:test";

test("release rejects foreign branches, mismatched tags, prereleases and other repositories", (t) => {
  const root = mkdtempSync(join(tmpdir(), "multica-release-origin-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const remote = join(root, "remote.git");
  const checkout = join(root, "checkout");
  execFileSync("git", ["init", "--bare", remote], { stdio: "pipe" });
  execFileSync("git", ["clone", remote, checkout], { stdio: "pipe" });
  const git = (...args) => execFileSync("git", args, { cwd: checkout, stdio: "pipe" });
  git("config", "user.name", "Release Test"); git("config", "user.email", "test@example.com");
  git("switch", "-c", "qqb_main");
  writeFileSync(join(checkout, "file"), "one"); git("add", "."); git("commit", "-m", "one");
  git("tag", "-a", "v1.2.3", "-m", "release"); git("push", "origin", "qqb_main");
  const script = fileURLToPath(new URL("./check-release-origin.sh", import.meta.url));
  const check = (tag = "v1.2.3", repo = "quboqin/multica") => spawnSync("bash", [script, tag], {
    cwd: checkout, encoding: "utf8", env: { ...process.env, GITHUB_REPOSITORY: repo },
  });
  assert.equal(check().status, 0);
  assert.notEqual(check("v1.2.3-rc.1").status, 0);
  assert.notEqual(check("v01.2.3").status, 0);
  assert.notEqual(check("v1.2.3", "multica-ai/multica").status, 0);
  git("switch", "-c", "feature");
  writeFileSync(join(checkout, "file"), "two"); git("commit", "-am", "two"); git("tag", "v1.2.4");
  assert.notEqual(check().status, 0, "HEAD must match tag");
  assert.notEqual(check("v1.2.4").status, 0, "unmerged feature cannot release");
});
