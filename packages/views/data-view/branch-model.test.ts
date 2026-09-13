import { describe, expect, it } from "vitest";
import {
  dataViewBranchKey,
  rebaseDataViewBranchState,
  type DataViewBranchState,
} from "./branch-model";

function state(): DataViewBranchState {
  return {
    identity: "source-a:query-a",
    structureIdentity: "source-a:group-a",
    branches: new Map([
      [
        dataViewBranchKey("g-1", null),
        {
          key: dataViewBranchKey("g-1", null),
          groupKey: "g-1",
          parentRowId: null,
          ancestorRowIds: [],
          cursors: [null, "tail-a"],
        },
      ],
    ]),
  };
}

describe("rebaseDataViewBranchState", () => {
  it("keeps null, empty, root and delimiter-bearing opaque ids distinct", () => {
    const keys = [
      dataViewBranchKey(null, null),
      dataViewBranchKey(null, "root"),
      dataViewBranchKey("", null),
      dataViewBranchKey("a::b", "c"),
      dataViewBranchKey("a", "b::c"),
    ];
    expect(new Set(keys)).toHaveProperty("size", keys.length);
  });

  it("keeps branches but evicts tail cursors for a query-only change", () => {
    const next = rebaseDataViewBranchState(
      state(),
      "source-a:query-b",
      "source-a:group-a",
      true,
    );

    expect(next.branches.get(dataViewBranchKey("g-1", null))?.cursors).toEqual([
      null,
    ]);
  });

  it("does not carry branches across a source or structure identity", () => {
    const next = rebaseDataViewBranchState(
      state(),
      "source-b:query-a",
      "source-b:group-a",
      false,
    );

    expect([...next.branches.keys()]).toEqual([
      dataViewBranchKey(null, null),
    ]);
  });
});
