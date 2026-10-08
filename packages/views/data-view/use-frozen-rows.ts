"use client";

import { useMemo, useRef } from "react";
import {
  dataSourceIdentityKey,
  type DataSourceIdentity,
} from "@multica/core/data-source";

/** Keep the editor's anchor in place while refreshing values from Query data. */
export function useFrozenRows<Row>(
  source: DataSourceIdentity,
  rows: Row[],
  editingKey: string | null,
  refresh: (snapshot: Row[]) => Row[],
): Row[] {
  const identity = dataSourceIdentityKey(source);
  const state = useRef<{ identity: string; snapshot: Row[] | null }>({
    identity,
    snapshot: null,
  });
  // A workspace/source transition must never retain another source's rows.
  if (state.current.identity !== identity)
    state.current = { identity, snapshot: null };
  if (editingKey === null) state.current.snapshot = null;
  else if (state.current.snapshot === null) state.current.snapshot = rows;
  const snapshot = state.current.snapshot;
  return useMemo(
    () => (snapshot && snapshot !== rows ? refresh(snapshot) : rows),
    [snapshot, rows, refresh],
  );
}
