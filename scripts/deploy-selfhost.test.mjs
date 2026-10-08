import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync, existsSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

const script = fileURLToPath(new URL("./deploy-selfhost.sh", import.meta.url));
const sha = "a".repeat(40);

function setupTransfer(t) {
  const workflow = readFileSync(new URL("../.github/workflows/deploy.yml", import.meta.url), "utf8");
  const step = workflow.split("      - name: Transfer tagged deployment files and upgrade\n")[1];
  assert.ok(step, "deployment transfer step exists");
  const body = step.split("        run: |\n")[1].replace(/^          /gm, "");
  const root = mkdtempSync(join(tmpdir(), "multica-transfer-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const bin = join(root, "bin"); mkdirSync(bin);
  const log = join(root, "calls");
  for (const command of ["ssh", "scp"]) {
    writeFileSync(join(bin, command), `#!/bin/bash
printf '%s\\n' '${command}' "$@" 'END' >> "$TEST_SSH_LOG"
if [[ "$*" == *'mktemp -d'* ]]; then echo /tmp/multica-release.TEST1234; fi
exit 0
`, { mode: 0o755 });
  }
  writeFileSync(join(bin, "git"), `#!/bin/bash\necho ${sha}\n`, { mode: 0o755 });
  const run = (port) => spawnSync("bash", ["-c", body], {
    encoding: "utf8",
    env: {
      ...process.env, PATH: `${bin}:${process.env.PATH}`, TEST_SSH_LOG: log,
      RUNNER_TEMP: root, RELEASE_TAG: "v1.2.3", DEPLOY_HOST: "deploy.example.test",
      DEPLOY_PORT: port, DEPLOY_USER: "deploy", DEPLOY_PATH: "/srv/multica",
      DEPLOY_SSH_KEY: "test-only-key", DEPLOY_KNOWN_HOSTS: "test-only-host-key",
    },
  });
  return { root, log, run };
}

for (const port of ["22", "23022"]) {
  test(`deployment transfer uses port ${port} and strict host verification for every SSH/SCP call`, (t) => {
    const { root, log, run } = setupTransfer(t);
    const result = run(port);
    assert.equal(result.status, 0, result.stderr);
    const calls = readFileSync(log, "utf8").trim().split("\nEND\n");
    assert.equal(calls.length, 4, "connect, copy, upgrade and cleanup");
    assert.deepEqual(calls.map((call) => call.split("\n")[0]), ["ssh", "scp", "ssh", "ssh"]);
    for (const call of calls) {
      const args = call.split("\n");
      assert.ok(args.includes(`Port=${port}`));
      assert.ok(args.includes("StrictHostKeyChecking=yes"));
      assert.ok(args.includes(`UserKnownHostsFile=${root}/deploy-ssh/known_hosts`));
    }
    assert.equal(existsSync(join(root, "deploy-ssh/key")), false, "temporary key is removed");
  });
}

test("invalid deployment ports fail before any SSH connection or key write", (t) => {
  const { root, log, run } = setupTransfer(t);
  for (const port of ["", "0", "-1", "65536", "999999999999", "22; touch bad", "023022"]) {
    assert.notEqual(run(port).status, 0, `reject ${JSON.stringify(port)}`);
    assert.equal(existsSync(log), false);
    assert.equal(existsSync(join(root, "deploy-ssh/key")), false);
  }
});

function setup(t) {
  const root = mkdtempSync(join(tmpdir(), "multica-deploy-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  const bin = join(root, "bin"); mkdirSync(bin);
  const deploy = join(root, "production"); mkdirSync(deploy);
  writeFileSync(join(deploy, ".env"), "JWT_SECRET=preserve-me\nMULTICA_IMAGE_TAG=v1.0.0\n");
  writeFileSync(join(bin, "docker"), `#!/bin/bash
echo "$*" >> "$TEST_LOG"
case "$*" in
  *'ps -q postgres'*) echo pg ;;
  *'ps -q backend'*) echo api ;;
  *'pg_dump'*) [[ "$FAIL_BACKUP" != true ]] || exit 1; echo dump ;;
  *'--entrypoint tar'*) echo archive ;;
  *'node -e'*) [[ "$FAIL_HEALTH" != true ]] || exit 1 ;;
esac
`, { mode: 0o755 });
  writeFileSync(join(bin, "sleep"), "#!/bin/bash\nexit 0\n", { mode: 0o755 });
  const log = join(root, "log");
  const run = (env = {}, tag = "v1.2.3") => spawnSync("bash", [script, tag, sha, deploy], {
    encoding: "utf8", env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, TEST_LOG: log, FAIL_BACKUP: "false", FAIL_HEALTH: "false", ...env },
  });
  return { deploy, log, run };
}
test("upgrade backs up quiesced data before starting and records a pinned version", (t) => {
  const { deploy, log, run } = setup(t);
  const result = run(); assert.equal(result.status, 0, result.stderr);
  const calls = readFileSync(log, "utf8");
  assert.ok(calls.indexOf("stop frontend backend") < calls.indexOf("pg_dump"));
  assert.ok(calls.indexOf("pg_dump") < calls.indexOf("up -d backend frontend"));
  assert.match(calls, /\.commit !== process\.argv\[1\]/);
  assert.match(readFileSync(join(deploy, ".env"), "utf8"), /JWT_SECRET=preserve-me\n/);
  assert.match(readFileSync(join(deploy, ".env"), "utf8"), /MULTICA_IMAGE_TAG=v1.2.3/);
  assert.equal(readFileSync(join(deploy, ".current-release"), "utf8"), `v1.2.3 ${sha}\n`);
  assert.equal(existsSync(join(deploy, ".deploy-lock")), false);
});
test("failed backup prevents startup, preserves old config and releases the lock", (t) => {
  const { deploy, log, run } = setup(t);
  assert.notEqual(run({ FAIL_BACKUP: "true" }).status, 0);
  assert.doesNotMatch(readFileSync(log, "utf8"), /up -d backend frontend/);
  assert.match(readFileSync(join(deploy, ".env"), "utf8"), /MULTICA_IMAGE_TAG=v1.0.0/);
  assert.equal(existsSync(join(deploy, ".deploy-lock")), false);
});
test("invalid tags and concurrent deployments cannot mutate the deployment", (t) => {
  const { deploy, log, run } = setup(t);
  assert.notEqual(run({}, "v1.2.3; touch bad").status, 0);
  assert.equal(existsSync(log), false);
  mkdirSync(join(deploy, ".deploy-lock"));
  assert.notEqual(run().status, 0);
  assert.equal(existsSync(log), false);
});

test("unhealthy new containers do not record a successful release or overwrite the version pin", (t) => {
  const { deploy, log, run } = setup(t);
  const result = run({ FAIL_HEALTH: "true" });
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /No automatic database rollback/);
  assert.match(readFileSync(log, "utf8"), /up -d backend frontend/);
  assert.match(readFileSync(join(deploy, ".env"), "utf8"), /MULTICA_IMAGE_TAG=v1.0.0/);
  assert.equal(existsSync(join(deploy, ".current-release")), false);
  assert.equal(existsSync(join(deploy, ".deploy-lock")), false);
});
