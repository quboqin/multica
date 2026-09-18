export function getDataViewSelectionRange(
  rowIds: string[],
  anchorId: string | null,
  targetId: string,
): string[] | null {
  if (!anchorId) return null;
  const anchorIndex = rowIds.indexOf(anchorId);
  const targetIndex = rowIds.indexOf(targetId);
  if (anchorIndex === -1 || targetIndex === -1) return null;

  const start = Math.min(anchorIndex, targetIndex);
  const end = Math.max(anchorIndex, targetIndex);
  return rowIds.slice(start, end + 1);
}

type FrozenRowAdapter<DisplayRow, Row> = {
  rowId(displayRow: DisplayRow): string | null;
  replaceRow(displayRow: DisplayRow, row: Row): DisplayRow;
};

/**
 * Keeps a data view's structural snapshot stable while replacing entity values
 * with their newest server snapshots. Domain adapters describe how a rendered
 * row contains its entity; the shared model never branches on an object kind.
 */
export function refreshFrozenDataViewRows<DisplayRow, Row>(
  snapshot: DisplayRow[],
  liveRows: ReadonlyMap<string, Row>,
  adapter: FrozenRowAdapter<DisplayRow, Row>,
): DisplayRow[] {
  return snapshot.map((displayRow) => {
    const id = adapter.rowId(displayRow);
    if (!id) return displayRow;
    const live = liveRows.get(id);
    return live === undefined ? displayRow : adapter.replaceRow(displayRow, live);
  });
}
