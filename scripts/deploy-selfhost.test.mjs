import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, rmSync, existsSync, readdirSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

const script = fileURLToPath(new URL("./deploy-selfhost.sh", import.meta.url));
const sha = "a".repeat(40);
const integrationSettings = {
  GITHUB_APP_SLUG: "multica-test",
  GITHUB_WEBHOOK_SECRET: "webhook-$literal-'quoted'",
  GITHUB_APP_CLIENT_ID: "Iv1.test",
  GITHUB_APP_CLIENT_SECRET: "client-test-secret",
  GITHUB_APP_ID: "12345",
  GITHUB_APP_PRIVATE_KEY: "-----BEGIN RSA PRIVATE KEY-----\nTEST-ONLY\n-----END RSA PRIVATE KEY-----\n",
  MULTICA_TELEGRAM_SECRET_KEY: Buffer.alloc(32, 1).toString("base64"),
  MULTICA_WECOM_SECRET_KEY: Buffer.alloc(32, 2).toString("base64"),
};

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
if [[ '${command}' == scp ]]; then cp "$RUNNER_TEMP/deploy-ssh/integrations.json" "$RUNNER_TEMP/transferred.json"; fi
exit 0
`, { mode: 0o755 });
  }
  writeFileSync(join(bin, "git"), `#!/bin/bash\necho ${sha}\n`, { mode: 0o755 });
  const run = (port, env = {}) => spawnSync("bash", ["-c", body], {
    encoding: "utf8",
    cwd: fileURLToPath(new URL("../", import.meta.url)),
    env: {
      ...process.env, PATH: `${bin}:${process.env.PATH}`, TEST_SSH_LOG: log,
      RUNNER_TEMP: root, RELEASE_TAG: "v1.2.3", DEPLOY_HOST: "deploy.example.test",
      DEPLOY_PORT: port, DEPLOY_USER: "deploy", DEPLOY_PATH: "/srv/multica",
      DEPLOY_SSH_KEY: "test-only-key", DEPLOY_KNOWN_HOSTS: "test-only-host-key",
      ...integrationSettings, ...env,
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
    assert.equal(existsSync(join(root, "deploy-ssh/integrations.json")), false, "temporary settings are removed");
    assert.deepEqual(JSON.parse(readFileSync(join(root, "transferred.json"), "utf8")), integrationSettings);
    for (const value of Object.values(integrationSettings)) {
      assert.ok(!readFileSync(log, "utf8").includes(value), "settings never appear in SSH arguments");
      assert.ok(!(result.stdout + result.stderr).includes(value), "settings never appear in logs");
    }
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

test("missing production integration settings fail before connecting and clean up temporary files", (t) => {
  const { root, log, run } = setupTransfer(t);
  for (const key of Object.keys(integrationSettings)) {
    const result = run("22", { [key]: "" });
    assert.notEqual(result.status, 0, key);
    assert.equal(existsSync(log), false);
    assert.equal(existsSync(join(root, "deploy-ssh/integrations.json")), false);
  }
});

test("invalid channel encryption keys fail before SSH without logging their values", (t) => {
  const { root, log, run } = setupTransfer(t);
  for (const key of ["MULTICA_TELEGRAM_SECRET_KEY", "MULTICA_WECOM_SECRET_KEY"]) {
    for (const value of ["not-base64!", Buffer.alloc(31).toString("base64"), Buffer.alloc(33).toString("base64"), ` ${integrationSettings[key]}`]) {
      const result = run("22", { [key]: value });
      assert.notEqual(result.status, 0, `${key} must decode to 32 bytes`);
      assert.equal(existsSync(log), false);
      assert.equal(existsSync(join(root, "deploy-ssh/integrations.json")), false);
      assert.ok(!(result.stdout + result.stderr).includes(value));
    }
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
  *'psql'*'-c SELECT'*) echo '2|3|4' ;;
  *'psql'*)
    [[ "$FAIL_DB_QUERY" != true ]] || exit 1
    cat >/dev/null
    if [[ "$DB_BAD_SCHEMA" == true ]]; then echo '{"compatible":false,"collections":2,"records":3,"fields":4}'
    elif [[ "$DB_COUNTS_REDUCED" == true ]]; then echo '{"compatible":true,"collections":2,"records":2,"fields":4}'
    else echo '{"compatible":true,"collections":2,"records":3,"fields":4}'; fi ;;
  *'node -e'*'result.compatible'*)
    while [[ "$1" != node ]]; do shift; done
    exec "$@" ;;
  *'node -e'*) [[ "$FAIL_HEALTH" != true ]] || exit 1 ;;
esac
`, { mode: 0o755 });
  writeFileSync(join(bin, "sleep"), "#!/bin/bash\nexit 0\n", { mode: 0o755 });
  const log = join(root, "log");
  const payload = join(root, "integrations.json");
  writeFileSync(payload, JSON.stringify(integrationSettings));
  const run = (env = {}, tag = "v1.2.3", settings = false) => spawnSync("bash", [script, tag, sha, deploy, ...(settings ? [payload] : [])], {
    encoding: "utf8", env: { ...process.env, PATH: `${bin}:${process.env.PATH}`, TEST_LOG: log, FAIL_BACKUP: "false", FAIL_HEALTH: "false", ...env },
  });
  return { deploy, log, run, payload };
}

test("successful upgrade persists integration settings and keeps the previous environment in its backup", (t) => {
  const { deploy, log, run } = setup(t);
  const previous = "JWT_SECRET=preserve-me\nGITHUB_APP_PRIVATE_KEY=\"old\nkey\"\nGITHUB_APP_ID=old\nUNRELATED=\"keep\nmultiline\"\n";
  writeFileSync(join(deploy, ".env"), previous);
  const result = run({}, "v1.2.3", true);
  assert.equal(result.status, 0, result.stderr);
  const updated = readFileSync(join(deploy, ".env"), "utf8");
  assert.ok(updated.startsWith("JWT_SECRET=preserve-me\n"));
  assert.equal(JSON.parse(updated.match(/^GITHUB_APP_PRIVATE_KEY=(.*)$/m)[1]), integrationSettings.GITHUB_APP_PRIVATE_KEY);
  for (const key of ["MULTICA_TELEGRAM_SECRET_KEY", "MULTICA_WECOM_SECRET_KEY"]) {
    assert.equal(JSON.parse(updated.match(new RegExp(`^${key}=(.*)$`, "m"))[1]), integrationSettings[key]);
  }
  assert.ok(updated.includes('UNRELATED="keep\nmultiline"\n'));
  assert.doesNotMatch(updated, /old/);
  assert.match(readFileSync(log, "utf8"), /--env-file .*\.env\.integrations\./);
  const backup = readdirSync(join(deploy, "backups"))[0];
  assert.equal(readFileSync(join(deploy, "backups", backup, "env"), "utf8"), previous);
  assert.equal(readdirSync(deploy).some((name) => name.startsWith(".env.integrations.")), false);
  assert.equal(run({}, "v1.2.3", true).status, 0);
  assert.equal(readFileSync(join(deploy, ".env"), "utf8"), updated, "repeated deployment replaces credentials without accumulating entries");
});

test("failed upgrade preserves previous integration settings and removes the staged environment", (t) => {
  const { deploy, run } = setup(t);
  const previous = readFileSync(join(deploy, ".env"), "utf8");
  assert.notEqual(run({ FAIL_HEALTH: "true" }, "v1.2.3", true).status, 0);
  assert.equal(readFileSync(join(deploy, ".env"), "utf8"), previous);
  assert.equal(readdirSync(deploy).some((name) => name.startsWith(".env.integrations.")), false);
});

test("malformed integration settings stop deployment before Docker operations without leaking input", (t) => {
  const { deploy, log, run, payload } = setup(t);
  writeFileSync(payload, "{secret-invalid-json");
  const previous = readFileSync(join(deploy, ".env"), "utf8");
  const result = run({}, "v1.2.3", true);
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(result.stderr, /secret-invalid-json/);
  assert.equal(existsSync(log), false);
  assert.equal(readFileSync(join(deploy, ".env"), "utf8"), previous);
  assert.equal(existsSync(join(deploy, ".deploy-lock")), false);
});
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

for (const failure of ["FAIL_DB_QUERY", "DB_BAD_SCHEMA", "DB_COUNTS_REDUCED"]) {
  test(`${failure} prevents recording a successful deployment without rolling back data`, (t) => {
    const { deploy, run } = setup(t);
    const result = run({ [failure]: "true" });
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, /database compatibility checks/);
    assert.match(result.stderr, /No automatic database rollback/);
    assert.match(readFileSync(join(deploy, ".env"), "utf8"), /MULTICA_IMAGE_TAG=v1.0.0/);
    assert.equal(existsSync(join(deploy, ".current-release")), false);
    assert.equal(existsSync(join(deploy, ".deploy-lock")), false);
  });
}
