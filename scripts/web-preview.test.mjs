import assert from "node:assert/strict";
import { mkdtemp, mkdir, readFile, readlink, writeFile, access, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { prepareWebPreview } from "./web-preview.mjs";

test("preview isolates generated files and secrets, refreshes sources and retains its build cache", async () => {
  const temp = await mkdtemp(join(tmpdir(), "web-preview-"));
  try {
    const root = join(temp, "checkout with spaces");
    const target = join(temp, "preview");
    const source = join(root, "apps/web");
    const app = join(target, "apps/web");
    for (const dir of ["app", ".next/dev", ".source", "node_modules"]) {
      await mkdir(join(source, dir), { recursive: true });
    }
    for (const file of [".env.local", ".next/dev/marker", ".source/index.ts", "next-env.d.ts", "tsconfig.tsbuildinfo"]) {
      await writeFile(join(source, file), "development-only");
    }
    await writeFile(join(source, "app/page.tsx"), "first");
    await prepareWebPreview(root, target);
    assert.equal(await readFile(join(app, "app/page.tsx"), "utf8"), "first");
    for (const file of [".env.local", ".next", ".source", "next-env.d.ts", "tsconfig.tsbuildinfo"]) {
      await assert.rejects(access(join(app, file)), { code: "ENOENT" });
    }
    assert.equal(await readlink(join(app, "node_modules")), join(source, "node_modules"));
    assert.equal(await readlink(join(target, "packages")), join(root, "packages"));
    await mkdir(join(app, ".next/cache"), { recursive: true });
    await writeFile(join(app, ".next/cache/marker"), "keep");
    await writeFile(join(app, "next-env.d.ts"), "production types");
    await rm(join(source, "app/page.tsx"));
    await writeFile(join(source, "app/new.tsx"), "second");
    await prepareWebPreview(root, target);
    await assert.rejects(access(join(app, "app/page.tsx")), { code: "ENOENT" });
    assert.equal(await readFile(join(app, "app/new.tsx"), "utf8"), "second");
    assert.equal(await readFile(join(app, ".next/cache/marker"), "utf8"), "keep");
    assert.equal(await readFile(join(source, "next-env.d.ts"), "utf8"), "development-only");
    assert.equal(await readFile(join(source, ".source/index.ts"), "utf8"), "development-only");
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});
