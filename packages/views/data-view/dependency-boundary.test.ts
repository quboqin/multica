// @vitest-environment node

import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const DATA_VIEW_DIR = fileURLToPath(new URL(".", import.meta.url));

describe("shared data-view dependency boundary", () => {
  it("does not import task DTOs, queries, stores or run gates", () => {
    const productionFiles = readdirSync(DATA_VIEW_DIR).filter(
      (name) =>
        /\.tsx?$/.test(name) &&
        !name.endsWith(".test.ts") &&
        !name.endsWith(".test.tsx"),
    );

    for (const name of productionFiles) {
      const source = readFileSync(`${DATA_VIEW_DIR}/${name}`, "utf8");
      expect(source, name).not.toMatch(
        /@multica\/core\/(?:issues|documents|collections)|\.\.\/(?:issues|documents|collections)/,
      );
      expect(source, name).not.toMatch(
        /Issue(?:Table|Query|Store)|run-confirm/,
      );
    }
  });
});
