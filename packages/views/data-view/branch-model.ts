export type DataViewBranch = {
  key: string;
  groupKey: string | null;
  parentRowId: string | null;
  ancestorRowIds: string[];
  cursors: Array<string | null>;
};

export type DataViewBranchState = {
  identity: string;
  structureIdentity: string;
  branches: Map<string, DataViewBranch>;
};

export function dataViewBranchKey(
  groupKey: string | null,
  parentRowId: string | null,
) {
  return JSON.stringify([groupKey, parentRowId]);
}

export function sameDataViewPath(left: string[], right: string[]) {
  return (
    left.length === right.length &&
    left.every((value, index) => value === right[index])
  );
}

/**
 * Rebase a paged branch graph when the source/query changes. Query-only
 * changes retain structural branches for previous-data painting but discard
 * every tail cursor. Source/group/hierarchy changes discard the graph so data
 * and late requests from one identity cannot appear under another.
 */
export function rebaseDataViewBranchState(
  previous: DataViewBranchState,
  identity: string,
  structureIdentity: string,
  usesGrouping: boolean,
): DataViewBranchState {
  if (previous.identity === identity) return previous;

  const branches =
    previous.structureIdentity === structureIdentity
      ? new Map(
          [...previous.branches].map(([key, branch]) => [
            key,
            { ...branch, cursors: [null] },
          ]),
        )
      : new Map<string, DataViewBranch>();

  if (!usesGrouping) {
    const key = dataViewBranchKey(null, null);
    if (!branches.has(key)) {
      branches.set(key, {
        key,
        groupKey: null,
        parentRowId: null,
        ancestorRowIds: [],
        cursors: [null],
      });
    }
  }

  return { identity, structureIdentity, branches };
}
