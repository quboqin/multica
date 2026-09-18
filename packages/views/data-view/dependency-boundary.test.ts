// @vitest-environment node

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";

const DATA_VIEW_DIR = fileURLToPath(new URL(".", import.meta.url));
const PACKAGES_DIR = resolve(DATA_VIEW_DIR, "../..");
const domains = /^(task|tasks|issue|issues|doc|docs|document|documents|record|records|collection|collections)$/;

function inspect(source: string, filename: string) {
  const tree = ts.createSourceFile(filename, source, ts.ScriptTarget.Latest, true);
  const imports: string[] = [];
  const branches: string[] = [];
  function visit(node: ts.Node) {
    if (ts.isIdentifier(node) && ts.isPropertyAssignment(node.parent) &&
        node.parent.name === node && domains.test(node.text)) {
      branches.push(node.text);
    }
    // Include type imports, re-exports, import types, require and lazy imports.
    if (ts.isStringLiteralLike(node)) {
      const parent = node.parent;
      if (ts.isImportDeclaration(parent) || ts.isExportDeclaration(parent) ||
          (ts.isLiteralTypeNode(parent) && ts.isImportTypeNode(parent.parent)) ||
          (ts.isCallExpression(parent) &&
            (parent.expression.kind === ts.SyntaxKind.ImportKeyword ||
              parent.expression.getText(tree) === "require"))) {
        imports.push(node.text);
      } else if (domains.test(node.text)) {
        // Forbid domain tags also in dispatch maps and includes checks; only
        // field kinds and structural row kinds belong in the shared engine.
        branches.push(node.getText(tree));
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(tree);
  return { imports, branches };
}

function resolveLocal(specifier: string, importer: string): string | null {
  let base: string;
  if (specifier.startsWith(".")) base = resolve(dirname(importer), specifier);
  else if (specifier.startsWith("@multica/")) {
    const [name, ...subpath] = specifier.slice("@multica/".length).split("/");
    const packageRoot = resolve(PACKAGES_DIR, name!);
    const manifest = JSON.parse(readFileSync(resolve(packageRoot, "package.json"), "utf8"));
    const key = subpath.length ? `./${subpath.join("/")}` : ".";
    let entry: string | undefined = manifest.exports[key];
    if (!entry) {
      for (const [pattern, target] of Object.entries(manifest.exports)) {
        if (pattern.endsWith("*") && key.startsWith(pattern.slice(0, -1))) {
          entry = String(target).replace("*", key.slice(pattern.length - 1));
          break;
        }
      }
    }
    if (!entry) throw new Error(`Unresolved package import: ${specifier}`);
    base = resolve(packageRoot, entry);
  } else return null;
  const file = [base, `${base}.ts`, `${base}.tsx`, `${base}/index.ts`, `${base}/index.tsx`]
    .find((path) => /\.tsx?$/.test(path) && existsSync(path));
  if (!file) throw new Error(`Unresolved local import: ${specifier} from ${importer}`);
  return file;
}

function productionFiles(directory: string): string[] {
  return readdirSync(directory, { withFileTypes: true }).flatMap((entry) => {
    const path = resolve(directory, entry.name);
    return entry.isDirectory() ? productionFiles(path)
      : /\.tsx?$/.test(path) && !/\.test\.tsx?$/.test(path) ? [path] : [];
  });
}

describe("shared data-view dependency boundary", () => {
  it("has no direct or transitive dependency on any of the three data sources", () => {
    const visited = new Set<string>();
    function check(file: string, chain: string[]) {
      if (visited.has(file)) return;
      visited.add(file);
      expect(file, chain.join(" -> ")).not.toMatch(/\/(?:core|views)\/(?:issues|documents|collections)\//);
      const result = inspect(readFileSync(file, "utf8"), file);
      for (const specifier of result.imports) {
        const dependency = resolveLocal(specifier, file);
        if (dependency) check(dependency, [...chain, specifier]);
      }
    }
    for (const file of productionFiles(DATA_VIEW_DIR)) check(file, [file]);
  });

  it("contains no object-type tags in engine code or headless contracts", () => {
    for (const file of [...productionFiles(DATA_VIEW_DIR),
      ...productionFiles(resolve(PACKAGES_DIR, "core/data-source"))]) {
      expect(inspect(readFileSync(file, "utf8"), file).branches, file).toEqual([]);
    }
  });

  it.each(["task", "doc", "record"])("detects %s comparisons, switches and dispatch maps", (kind) => {
    for (const source of [
      `if (row.kind === "${kind}") render();`,
      `row.kind !== '${kind}' ? a : b`,
      `switch (kind) { case '${kind}': break; }`,
      `['${kind}'].includes(kind)`,
      `const renderers = { '${kind}': render };`,
      `const renderers = { ${kind}: render };`,
    ]) expect(inspect(source, "fixture.ts").branches).not.toHaveLength(0);
  });

  it.each(["issues", "documents", "collections"])("detects %s imports including erased and lazy forms", (domain) => {
    const module = `@multica/core/${domain}`;
    for (const source of [
      `import type { Source } from '${module}';`,
      `export * from '${module}';`,
      `type Source = import('${module}').Source;`,
      `const source = import('${module}');`,
      `const source = require('${module}');`,
    ]) expect(inspect(source, "fixture.ts").imports).toEqual([module]);
  });
});
