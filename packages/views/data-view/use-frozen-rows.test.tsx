import { afterEach, describe, expect, it } from "vitest";
import { cleanup, renderHook } from "@testing-library/react";
import { useFrozenRows } from "./use-frozen-rows";

afterEach(cleanup);

describe("editor row snapshots", () => {
  it("holds order while values refresh, then applies the live order on close", () => {
    const source = { workspaceId: "ws", namespace: "records", sourceId: "one" };
    let live = [
      { id: "a", value: 1 },
      { id: "b", value: 2 },
    ];
    const refresh = (snapshot: typeof live) =>
      snapshot.map((row) => live.find((item) => item.id === row.id) ?? row);
    const { result, rerender } = renderHook(
      ({ editing }) => useFrozenRows(source, live, editing, refresh),
      {
        initialProps: { editing: "a:value" as string | null },
      },
    );
    live = [
      { id: "b", value: 2 },
      { id: "a", value: 3 },
    ];
    rerender({ editing: "a:value" });
    expect(result.current).toEqual([
      { id: "a", value: 3 },
      { id: "b", value: 2 },
    ]);
    rerender({ editing: null });
    expect(result.current).toBe(live);
  });

  it.each(["workspaceId", "namespace", "sourceId"] as const)(
    "discards the snapshot when %s changes",
    (key) => {
      const oldRow = { id: "same-id", title: "Private old row" };
      const newRow = { id: "same-id", title: "New source row" };
      const identity = {
        workspaceId: "ws",
        namespace: "documents",
        sourceId: "one",
      };
      const { result, rerender } = renderHook(
        ({ source, rows }) =>
          useFrozenRows(source, rows, "same-id:title", (snapshot) => snapshot),
        {
          initialProps: { source: identity, rows: [oldRow] },
        },
      );
      rerender({ source: { ...identity, [key]: "different" }, rows: [newRow] });
      expect(result.current).toEqual([newRow]);
    },
  );
});
