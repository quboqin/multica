"use client";

import { useEffect, useRef } from "react";

/**
 * Release the hoisted editing key when the cell that owns it unmounts.
 *
 * Row virtualization (see data-table.tsx) unmounts a cell as its row scrolls
 * out of the rendered window. Base UI does NOT call onOpenChange(false) on
 * unmount, so without this the open picker's key — and the frozen row
 * structure keyed off it — would persist after the anchor row leaves the
 * viewport: the table would stay frozen, and scrolling the row back would
 * silently reopen the picker and discard any in-progress rename draft
 * (MUL-5108 review R1#3). Clearing the key iff this unmounting cell still owns
 * it thaws the structure and closes the editor.
 *
 * Live values are read through refs so the empty-dep cleanup always sees the
 * current key/setter. At initial mount a cell is never yet the active editor
 * (the editor is opened by a later interaction, which does not remount the
 * cell), so this never fires spuriously — including under StrictMode's
 * mount → unmount → mount probe, whose first cleanup sees `editingCellKey`
 * still unequal to this cell's key.
 */
export function useReleaseEditingCellOnUnmount(
  cellKey: string | null,
  editingCellKey: string | null,
  setEditingCellKey: (key: string | null) => void,
) {
  const editingCellKeyRef = useRef(editingCellKey);
  editingCellKeyRef.current = editingCellKey;
  const setEditingCellKeyRef = useRef(setEditingCellKey);
  setEditingCellKeyRef.current = setEditingCellKey;
  useEffect(() => {
    return () => {
      if (cellKey !== null && editingCellKeyRef.current === cellKey) {
        setEditingCellKeyRef.current(null);
      }
    };
  }, [cellKey]);
}
