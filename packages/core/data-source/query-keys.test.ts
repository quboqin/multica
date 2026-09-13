import { describe, expect, it } from "vitest";
import {
  dataSourceGroupQueryKey,
  dataSourceRowQueryKey,
  dataSourceViewStateKey,
} from "./query-keys";

const branch = { groupKey: "same-group", parentRowId: null };
const page = { limit: 2, cursor: "same-cursor" };

describe("data-source identity keys", () => {
  it("isolates equal queries by source and workspace", () => {
    const options = {
      query: { search: "same", sort: ["amount", "desc"] },
      groupBy: { fieldId: "bucket" },
      branch,
      hierarchy: false,
      page,
    } as const;
    const sourceA = dataSourceRowQueryKey({
      ...options,
      identity: { workspaceId: "ws-a", namespace: "sample", sourceId: "a" },
    });
    const sourceB = dataSourceRowQueryKey({
      ...options,
      identity: { workspaceId: "ws-a", namespace: "sample", sourceId: "b" },
    });
    const workspaceB = dataSourceRowQueryKey({
      ...options,
      identity: { workspaceId: "ws-b", namespace: "sample", sourceId: "a" },
    });

    expect(sourceA).not.toEqual(sourceB);
    expect(sourceA).not.toEqual(workspaceB);
    expect(sourceA.at(-2)).toBe(2);
    expect(sourceA.at(-1)).toBe("same-cursor");
  });

  it("separates row, group and view-state namespaces", () => {
    const identity = {
      workspaceId: "ws-a",
      namespace: "sample",
      sourceId: "a",
    };
    const rows = dataSourceRowQueryKey({
      identity,
      query: {},
      groupBy: { fieldId: "bucket" },
      branch,
      hierarchy: false,
      page,
    });
    const groups = dataSourceGroupQueryKey({
      identity,
      query: {},
      groupBy: { fieldId: "bucket" },
      page,
    });
    const view = dataSourceViewStateKey(identity, "same-view");
    const otherView = dataSourceViewStateKey(identity, "other-view");
    const otherSourceView = dataSourceViewStateKey(
      { ...identity, sourceId: "b" },
      "same-view",
    );

    expect(rows).not.toEqual(groups);
    expect(groups).not.toEqual(view);
    expect(view).not.toEqual(otherView);
    expect(view).not.toEqual(otherSourceView);
  });
});
