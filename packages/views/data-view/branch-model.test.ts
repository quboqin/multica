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
  it("keeps branches but evicts tail cursors for a query-only change", () => {
    const next = rebaseDataViewBranchState(
      state(),
      "source-a:query-b",
      "source-a:group-a",
      true,
    );

    expect(next.branches.get("g-1::root")?.cursors).toEqual([null]);
  });

  it("does not carry branches across a source or structure identity", () => {
    const next = rebaseDataViewBranchState(
      state(),
      "source-b:query-a",
      "source-b:group-a",
      false,
    );

    expect([...next.branches.keys()]).toEqual(["ungrouped::root"]);
  });
});
