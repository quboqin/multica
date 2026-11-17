import type { CollectionQuery, CollectionRecord } from ".";

/** Filters and groups show all matching records, including collapsed children. */
export function collectionHasHierarchy(query: CollectionQuery): boolean {
  return !query.search && !query.group_by && !query.record_id && !query.date_field &&
    (!query.properties || Object.keys(query.properties).length === 0);
}

/** Flatten only expanded branches, retaining sibling order from the server. */
export function flattenCollectionTree(
  roots: CollectionRecord[],
  expanded: ReadonlySet<string>,
  children: ReadonlyMap<string, CollectionRecord[]>,
) {
  const records: CollectionRecord[] = [];
  const depths = new Map<string, number>();
  const stack = roots.map((record) => ({ record, depth: 0 })).reverse();
  while (stack.length) {
    const { record, depth } = stack.pop()!;
    // Also protects the renderer from duplicated pages or malformed cycles.
    if (depths.has(record.id)) continue;
    depths.set(record.id, depth);
    records.push(record);
    if (expanded.has(record.id)) {
      const branch = children.get(record.id) ?? [];
      for (let index = branch.length - 1; index >= 0; index--)
        stack.push({ record: branch[index]!, depth: depth + 1 });
    }
  }
  return { records, depths };
}
