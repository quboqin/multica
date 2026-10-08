import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { useReleaseEditingCellOnUnmount } from "./use-release-editing-cell";

// A cell unmounting is exactly what a virtual-window change does; probing the
// hook directly keeps the assertion deterministic (jsdom has no layout for a
// real virtualizer to react to).
describe("useReleaseEditingCellOnUnmount", () => {
  function Probe({
    cellKey,
    editingCellKey,
    setEditingCellKey,
  }: {
    cellKey: string | null;
    editingCellKey: string | null;
    setEditingCellKey: (key: string | null) => void;
  }) {
    useReleaseEditingCellOnUnmount(cellKey, editingCellKey, setEditingCellKey);
    return null;
  }

  afterEach(cleanup);

  it("clears the key when the cell that owns the open editor unmounts", () => {
    const setEditingCellKey = vi.fn();
    const { unmount } = render(
      <Probe
        cellKey="issue-a:status"
        editingCellKey="issue-a:status"
        setEditingCellKey={setEditingCellKey}
      />,
    );

    unmount();

    expect(setEditingCellKey).toHaveBeenCalledWith(null);
  });

  it("leaves the key untouched when a different cell unmounts", () => {
    const setEditingCellKey = vi.fn();
    const { unmount } = render(
      <Probe
        cellKey="issue-b:status"
        editingCellKey="issue-a:status"
        setEditingCellKey={setEditingCellKey}
      />,
    );

    unmount();

    expect(setEditingCellKey).not.toHaveBeenCalled();
  });

  it("does not fire on mount while the cell is not yet the active editor", () => {
    const setEditingCellKey = vi.fn();
    // Mount not-owning, then the editor opens on THIS cell (rerender, no
    // remount), then it unmounts — the responder reads the latest key.
    const { rerender, unmount } = render(
      <Probe
        cellKey="issue-a:status"
        editingCellKey={null}
        setEditingCellKey={setEditingCellKey}
      />,
    );
    expect(setEditingCellKey).not.toHaveBeenCalled();

    rerender(
      <Probe
        cellKey="issue-a:status"
        editingCellKey="issue-a:status"
        setEditingCellKey={setEditingCellKey}
      />,
    );
    expect(setEditingCellKey).not.toHaveBeenCalled();

    unmount();
    expect(setEditingCellKey).toHaveBeenCalledTimes(1);
    expect(setEditingCellKey).toHaveBeenCalledWith(null);
  });
});
