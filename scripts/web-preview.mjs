// Build Next.js in a separate app directory so its generated TypeScript and
// MDX files cannot invalidate an active development server.
import { cp, mkdir, readdir, rm, symlink } from "node:fs/promises";
import { basename, join, relative, resolve } from "node:path";
import { pathToFileURL } from "node:url";

export async function prepareWebPreview(root, destination) {
  const source = join(root, "apps/web");
  const app = join(destination, "apps/web");
  await mkdir(join(destination, "apps"), { recursive: true });
  // The caller stops the preview before replacing its sources. Keep the
  // webpack build cache, but remove old source files including deleted routes.
  for (const entry of await readdir(app).catch((error) => {
    if (error.code === "ENOENT") return [];
    throw error;
  })) {
    if (entry !== ".next") await rm(join(app, entry), { recursive: true, force: true });
  }
  await cp(source, app, {
    recursive: true,
    filter: (file) => {
      if (file === source) return true;
      const name = basename(file);
      return !["node_modules", ".next", ".source", "next-env.d.ts", "coverage", "test-results"].includes(name)
        && !name.startsWith(".env") && !name.endsWith(".tsbuildinfo")
        && !relative(source, file).split("/").includes(".git");
    },
  });
  // Workspace packages are compiled from the checkout. Only Next/MDX's app
  // directory needs writable isolation; installed dependencies are reused.
  for (const [target, link] of [
    [join(root, "apps/web/node_modules"), join(app, "node_modules")],
    [join(root, "node_modules"), join(destination, "node_modules")],
    [join(root, "packages"), join(destination, "packages")],
    [join(root, "pnpm-lock.yaml"), join(destination, "pnpm-lock.yaml")],
  ]) {
    await rm(link, { force: true });
    await symlink(target, link);
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  const [root, destination] = process.argv.slice(2);
  if (!root || !destination) throw new Error("Usage: web-preview.mjs <checkout> <preview-directory>");
  await prepareWebPreview(resolve(root), resolve(destination));
}
