// @vitest-environment node
import { describe, expect, it } from "vitest";
import { collectionHasHierarchy, flattenCollectionTree } from "./hierarchy";
import type { CollectionRecord } from ".";

const row = (id: string): CollectionRecord => ({ id, workspace_id: "w", collection_id: "c", title: id, fields: {}, links: {}, revision: 1, created_at: "" });

describe("collection hierarchy", () => {
  it("expands nested branches in sibling order and deduplicates malformed edges", () => {
    const children = new Map([["a", [row("b"), row("c")]], ["b", [row("d"), row("a")]]]);
    const tree = flattenCollectionTree([row("a"), row("z")], new Set(["a", "b"]), children);
    expect(tree.records.map((record) => record.id)).toEqual(["a", "b", "d", "c", "z"]);
    expect([...tree.depths.values()]).toEqual([0, 1, 2, 1, 0]);
    expect(flattenCollectionTree([row("a")], new Set(), children).records.map((record) => record.id)).toEqual(["a"]);
  });
  it("keeps sorting hierarchical and filtered or grouped matches flat", () => {
    expect(collectionHasHierarchy({ sort_by: "title", sort_dir: "asc" })).toBe(true);
    expect(collectionHasHierarchy({ properties: {} })).toBe(true);
    for (const query of [{ search: "child" }, { group_by: "stage" }, { properties: { field: ["x"] } }, { record_id: "child" }, { date_field: "due" }])
      expect(collectionHasHierarchy(query)).toBe(false);
  });
});
