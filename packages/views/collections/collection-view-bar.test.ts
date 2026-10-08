import { describe, expect, it } from "vitest";
import type { CollectionField } from "@multica/core/collections";
import { filterOperators, filtersToProperties } from "./collection-view-bar";

const field = (id: string, type: string): CollectionField => ({
  id,
  name: id,
  type,
  position: 0,
  config: { options: [] },
});
const fields = [
  field("n", "number"),
  field("t", "text"),
  field("m", "multi_select"),
  field("c", "checkbox"),
];

describe("collection filters", () => {
  it("offers operators the record query supports for each type", () => {
    expect(filterOperators("number")).toEqual(["exact", "gt", "gte", "lt", "lte"]);
    expect(filterOperators("text")).toContain("contains");
    expect(filterOperators("select")).toEqual(["exact"]);
  });

  it("encodes values in the shape the server matches", () => {
    expect(
      filtersToProperties(
        [
          { field: "n", op: "exact", value: "4" },
          { field: "t", op: "contains", value: "ab" },
          { field: "m", op: "exact", value: "opt" },
          { field: "c", op: "exact", value: false },
        ],
        fields,
      ),
    ).toEqual({
      n: [4],
      t: [{ op: "contains", value: "ab" }],
      m: [["opt"]],
      c: [false],
    });
  });

  it("drops incomplete conditions and fields that no longer exist", () => {
    expect(
      filtersToProperties(
        [
          { field: "n", op: "gt", value: "" },
          { field: "gone", op: "exact", value: "x" },
          { field: "n", op: "exact", value: "not a number" },
        ],
        fields,
      ),
    ).toBeUndefined();
  });
});
