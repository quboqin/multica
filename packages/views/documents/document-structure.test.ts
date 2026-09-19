// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { Issue } from "@multica/core/types";
import {
  documentLifecycle,
  incomingReferences,
  parseOutline,
  parseReferences,
} from "./document-structure";

describe("parseOutline", () => {
  it("lists h1-h3 headings with inline markdown stripped and skips code", () => {
    const markdown = [
      "# Title",
      "Intro with # not a heading",
      "## Current **backlog**",
      "```md",
      "## inside code",
      "```",
      "### [record_link](https://example.com)",
      "#### too deep",
      "## Current **backlog**",
      "$$",
      "# math",
      "$$",
    ].join("\n");
    expect(parseOutline(markdown)).toEqual([
      { level: 1, text: "Title", occurrence: 0 },
      { level: 2, text: "Current backlog", occurrence: 0 },
      { level: 3, text: "record_link", occurrence: 0 },
      { level: 2, text: "Current backlog", occurrence: 1 },
    ]);
  });
});

describe("parseReferences", () => {
  it("collects issue mentions and saved-view embeds, counting repeats", () => {
    const view = "10000000-0000-4000-8000-000000000001";
    const markdown = [
      "See [MAGI-2](mention://issue/abc-1) and [@Spec](mention://issue/doc-2).",
      `:::multica-view ${view}`,
      "Again [MAGI-2](mention://issue/abc-1), [Ann](mention://member/u1)",
      ":::multica-view pending",
    ].join("\n");
    expect(parseReferences(markdown)).toEqual([
      { kind: "view", id: view, label: "", count: 1 },
      { kind: "issue", id: "abc-1", label: "MAGI-2", count: 2 },
      { kind: "issue", id: "doc-2", label: "Spec", count: 1 },
    ]);
  });
});

describe("incomingReferences", () => {
  it("returns other documents that mention the id", () => {
    const docs = [
      { id: "a", description: "[x](mention://issue/target) [y](mention://issue/target)" },
      { id: "b", description: "no links" },
      { id: "target", description: "[self](mention://issue/target)" },
    ] as Issue[];
    expect(
      incomingReferences(docs, "target").map(({ doc, count }) => [doc.id, count]),
    ).toEqual([["a", 2]]);
  });
});

describe("documentLifecycle", () => {
  it("maps document status keys and falls back to the lifecycle category", () => {
    expect(documentLifecycle({ status: "draft" })).toBe("draft");
    expect(documentLifecycle({ status: "reviewing" })).toBe("reviewing");
    expect(documentLifecycle({ status: "published" })).toBe("published");
    expect(
      documentLifecycle({ status: "custom", status_category: "started" }),
    ).toBe("reviewing");
    expect(
      documentLifecycle({ status: "custom", status_category: "done" }),
    ).toBe("published");
  });
});
