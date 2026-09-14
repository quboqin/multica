import type {
  DataSourceBranchRef,
  DataSourceIdentity,
  DataSourcePageRequest,
} from "./types";

export function dataSourceIdentityKey(identity: DataSourceIdentity) {
  return [
    "data-source",
    identity.workspaceId,
    identity.namespace,
    identity.sourceId,
  ] as const;
}

/**
 * Collision-free scalar form for React keys, DOM diagnostics and local state.
 * JSON tuple encoding preserves component boundaries (including empty strings)
 * instead of relying on a delimiter that opaque source ids may contain.
 */
export function dataSourceIdentityString(identity: DataSourceIdentity) {
  return JSON.stringify(dataSourceIdentityKey(identity));
}

export function dataSourceRowQueryKey<Query>(options: {
  identity: DataSourceIdentity;
  query: Query;
  groupBy: { fieldId: string } | null;
  branch: DataSourceBranchRef;
  hierarchy: boolean;
  page: DataSourcePageRequest;
}) {
  return [
    ...dataSourceIdentityKey(options.identity),
    "rows",
    options.query,
    options.groupBy,
    options.branch,
    options.hierarchy,
    options.page.limit,
    options.page.cursor ?? null,
  ] as const;
}

export function dataSourceGroupQueryKey<Query>(options: {
  identity: DataSourceIdentity;
  query: Query;
  groupBy: { fieldId: string };
  page: DataSourcePageRequest;
}) {
  return [
    ...dataSourceIdentityKey(options.identity),
    "groups",
    options.query,
    options.groupBy,
    options.page.limit,
    options.page.cursor ?? null,
  ] as const;
}

export function dataSourceViewStateKey(
  identity: DataSourceIdentity,
  viewId: string,
) {
  return [...dataSourceIdentityKey(identity), "view", viewId] as const;
}
