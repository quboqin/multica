// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  groupValueKey,
  groupValuesEqual,
  propertyGroupLabel,
} from "./grouping";
import {
  IssueTableGroupsResponseSchema,
  EMPTY_ISSUE_TABLE_GROUPS_RESPONSE,
} from "../api/schemas";
import { parseWithFallback } from "../api/schema";
import { CollectionPageSchema } from "../collections";

describe("field grouping", () => {
  it("normalizes multi-values without changing their stored order", () => {
    const value = ["b", "a"];
    expect(groupValuesEqual(value, ["a", "b"])).toBe(true);
    expect(value).toEqual(["b", "a"]);
    expect(groupValuesEqual(false, null)).toBe(false);
    expect(groupValuesEqual(0, null)).toBe(false);
    expect(groupValuesEqual([], null)).toBe(true);
    expect(groupValuesEqual("", undefined)).toBe(true);
    expect(groupValueKey("null")).not.toBe(groupValueKey(null));
  });
  it("resolves option and member combinations", () => {
    expect(
      propertyGroupLabel(
        ["b", "a"],
        [
          { id: "a", name: "Alpha" },
          { id: "b", name: "Beta" },
        ],
        () => "",
        "multi_select",
      ),
    ).toBe("Alpha, Beta");
    expect(
      propertyGroupLabel(
        ["member:1", "member:2"],
        [],
        (_, id) => `Person ${id}`,
        "multi_actor",
      ),
    ).toBe("Person 1, Person 2");
  });
  it.each([0, false, "North", ["a", "b"]])(
    "parses typed group values: %j",
    (value) => {
      const parsed = IssueTableGroupsResponseSchema.parse({
        query_fingerprint: "q",
        total: 1,
        groups: [
          {
            key: "k",
            count: 1,
            value: {
              kind: "property",
              property_id: "p",
              value,
              value_state: "value",
            },
          },
        ],
      });
      expect(parsed.groups[0]?.value).toMatchObject({ value });
    },
  );
  it("rejects malformed issue group values at the API boundary", () => {
    const parsed = parseWithFallback(
      {
        query_fingerprint: "q",
        total: 1,
        groups: [
          {
            key: "k",
            count: 1,
            value: {
              kind: "property",
              property_id: "p",
              value: [{}],
              value_state: "value",
            },
          },
        ],
      },
      IssueTableGroupsResponseSchema,
      EMPTY_ISSUE_TABLE_GROUPS_RESPONSE,
      { endpoint: "/api/issues/table/groups" },
    );
    expect(parsed).toEqual(EMPTY_ISSUE_TABLE_GROUPS_RESPONSE);
  });
  it("accepts existing collection group responses without display payloads", () => {
    expect(
      CollectionPageSchema.parse({
        records: [],
        total: 0,
        groups: [{ key: "a", count: 0 }],
        next_cursor: null,
      }).groups,
    ).toEqual([{ key: "a", count: 0 }]);
  });
});
