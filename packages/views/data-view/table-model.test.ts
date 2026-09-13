// @vitest-environment node

import { describe, expect, it } from "vitest";

import {
  getDataViewSelectionRange,
  refreshFrozenDataViewRows,
} from "./table-model";

type RecordRow = { id: string; title: string };
type DisplayRow =
  | { kind: "group"; key: string }
  | { kind: "record"; key: string; record: RecordRow; depth: number };

describe("data view table model", () => {
  it("selects an inclusive range without knowing the row domain", () => {
    expect(
      getDataViewSelectionRange(["a", "b", "c"], "c", "a"),
    ).toEqual(["a", "b", "c"]);
  });

  it("refreshes record values without changing the structural snapshot", () => {
    const stale = { id: "record-1", title: "Old" };
    const live = { id: "record-1", title: "New" };
    const group: DisplayRow = { kind: "group", key: "group-1" };
    const snapshot: DisplayRow[] = [
      group,
      { kind: "record", key: stale.id, record: stale, depth: 2 },
    ];

    const refreshed = refreshFrozenDataViewRows(
      snapshot,
      new Map([[live.id, live]]),
      {
        rowId: (row) => (row.kind === "record" ? row.record.id : null),
        replaceRow: (row, record) =>
          row.kind === "record" ? { ...row, record } : row,
      },
    );

    expect(refreshed).toEqual([
      group,
      { kind: "record", key: "record-1", record: live, depth: 2 },
    ]);
    expect(refreshed[0]).toBe(group);
  });
});
