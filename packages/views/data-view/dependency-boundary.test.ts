// @vitest-environment node
import { readdirSync, readFileSync } from "node:fs";
import { resolve } from "node:path";
import ts from "typescript";
import { expect, it } from "vitest";

it("keeps the shared engine independent of business sources and platform routers", () => {
  const directories = [
    resolve(import.meta.dirname),
    resolve(import.meta.dirname, "../../core/data-source"),
  ];
  const allowed =
    /^(react$|zustand(?:\/middleware)?$|\.\.\/platform\/storage$|@dnd-kit\/|@tanstack\/react-(?:table|query)$|@multica\/ui\/|@multica\/core\/data-source$|lucide-react$|\.\/)/;
  for (const directory of directories) {
    for (const file of readdirSync(directory).filter(
      (name) => /\.tsx?$/.test(name) && !name.includes(".test."),
    )) {
      const path = resolve(directory, file);
      const source = ts.createSourceFile(
        path,
        readFileSync(path, "utf8"),
        ts.ScriptTarget.Latest,
      );
      const visit = (node: ts.Node) => {
        if (
          (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) &&
          node.moduleSpecifier &&
          ts.isStringLiteral(node.moduleSpecifier)
        ) {
          expect(
            node.moduleSpecifier.text,
            `${file} imports a business/platform dependency`,
          ).toMatch(allowed);
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
    }
  }
});
