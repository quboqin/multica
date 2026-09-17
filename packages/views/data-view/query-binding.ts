import type { QueryKey } from "@tanstack/react-query";
import type {
  DataSourceBranchRef,
  DataSourceGroupPage,
  DataSourceIdentity,
  DataSourcePageRequest,
} from "@multica/core/data-source";

export type DataViewGroupBy = { fieldId: string } | null;

export type DataViewRowPage<Row> = {
  rows: Row[];
  total: number;
  /** Total rows in this branch, independent from the full query total. */
  branchTotal: number;
  nextCursor: string | null;
};

export type DataViewRowPageRequest<Query> = {
  query: Query;
  groupBy: DataViewGroupBy;
  branch: DataSourceBranchRef;
  hierarchy: boolean;
  page: DataSourcePageRequest;
};

export type DataViewRowBranchRequest<Query> = Omit<
  DataViewRowPageRequest<Query>,
  "page"
>;

export type DataViewGroupPageRequest<Query> = {
  query: Query;
  groupBy: { fieldId: string };
  page: DataSourcePageRequest;
};

export type DataViewGroupPagesRequest<Query> = Omit<
  DataViewGroupPageRequest<Query>,
  "page"
>;

/**
 * Bridges a shared data view to a domain query/cache. Raw page types remain in
 * React Query so domain mutation and websocket updaters keep their established
 * cache shape; only the renderer receives generic projections.
 */
export type DataViewQueryBinding<
  Row,
  Query,
  RawRowPage,
  RawGroupPage,
> = {
  identity: DataSourceIdentity;
  rowPageKey(request: DataViewRowPageRequest<Query>): QueryKey;
  rowBranchKey(request: DataViewRowBranchRequest<Query>): QueryKey;
  readRowPage(
    request: DataViewRowPageRequest<Query>,
    signal?: AbortSignal,
  ): Promise<RawRowPage>;
  mapRowPage(page: RawRowPage): DataViewRowPage<Row>;
  groupPagesKey(request: DataViewGroupPagesRequest<Query>): QueryKey;
  readGroupPage(
    request: DataViewGroupPageRequest<Query>,
    signal?: AbortSignal,
  ): Promise<RawGroupPage>;
  mapGroupPage(page: RawGroupPage): DataSourceGroupPage;
};
