"use client";

import { useCallback, useEffect, useRef } from "react";
import {
  dataSourceIdentityString,
  type DataSourceIdentity,
} from "@multica/core/data-source";
import { getDataViewSelectionRange } from "./table-model";

/** Shared range-selection lifecycle with source-switch isolation. */
export function useDataViewSelection(options: {
  sourceIdentity: DataSourceIdentity;
  rowIds: string[];
  selectedIds: ReadonlySet<string>;
  select(ids: string[]): void;
  deselect(ids: string[]): void;
  toggle(id: string): void;
  clear(): void;
}) {
  const {
    sourceIdentity,
    rowIds,
    selectedIds,
    select,
    deselect,
    toggle,
    clear,
  } = options;
  const identityKey = dataSourceIdentityString(sourceIdentity);
  const previousIdentityRef = useRef(identityKey);
  const anchorRef = useRef<string | null>(null);

  useEffect(() => {
    if (previousIdentityRef.current === identityKey) return;
    previousIdentityRef.current = identityKey;
    anchorRef.current = null;
    clear();
  }, [clear, identityKey]);
  useEffect(() => {
    if (selectedIds.size === 0) anchorRef.current = null;
  }, [selectedIds]);

  return useCallback(
    (rowId: string, shiftKey: boolean) => {
      const range = shiftKey
        ? getDataViewSelectionRange(rowIds, anchorRef.current, rowId)
        : null;
      if (range) {
        if (selectedIds.has(rowId)) deselect(range);
        else select(range);
        return;
      }
      toggle(rowId);
      anchorRef.current = rowId;
    },
    [deselect, rowIds, select, selectedIds, toggle],
  );
}
