import type { QueryClient, QueryFilters, QueryKey } from "@tanstack/react-query";
import { creativeKeys } from "./queries";

const refreshWindowMs = 250;

type PendingRefresh = {
  key: QueryKey;
  dirty: boolean;
  running: boolean;
  timer?: ReturnType<typeof setTimeout>;
};

export function createCreativeRefreshQueue(client: QueryClient) {
  const pending = new Map<string, PendingRefresh>();
  let disposed = false;

  const schedule = (hash: string, entry: PendingRefresh) => {
    if (disposed || entry.timer !== undefined || entry.running) return;
    entry.timer = setTimeout(() => {
      entry.timer = undefined;
      if (disposed || pending.get(hash) !== entry) return;
      const query = client.getQueryCache().get(hash);
      if (!query || !query.isActive() || query.isDisabled()) {
        pending.delete(hash);
        query?.invalidate();
        return;
      }
      if (query.state.fetchStatus !== "idle") return;
      entry.dirty = false;
      entry.running = true;
      void client.refetchQueries(
        { queryKey: entry.key, exact: true, type: "active" },
        { cancelRefetch: false },
      ).finally(() => {
        entry.running = false;
        if (disposed || pending.get(hash) !== entry) return;
        if (entry.dirty) schedule(hash, entry);
        else pending.delete(hash);
      });
    }, refreshWindowMs);
  };

  // A notification received during a request must survive that request's
  // success, which otherwise clears React Query's invalidated flag.
  const unsubscribe = client.getQueryCache().subscribe((event) => {
    const entry = pending.get(event.query.queryHash);
    if (!entry) return;
    if (event.type === "removed") {
      if (entry.timer !== undefined) clearTimeout(entry.timer);
      pending.delete(event.query.queryHash);
    } else if (event.type === "updated" && event.query.state.fetchStatus === "idle" && entry.dirty) {
      schedule(event.query.queryHash, entry);
    }
  });

  return {
    invalidate(workspaceId: string, payload?: unknown) {
      if (disposed) return;
      const filters: QueryFilters[] = payload && typeof payload === "object"
        && "scope" in payload && payload.scope === "order"
        && "order_id" in payload && typeof payload.order_id === "string" && payload.order_id
        ? [
          { queryKey: creativeKeys.orders(workspaceId), exact: true },
          { queryKey: creativeKeys.order(workspaceId, payload.order_id), exact: true },
          { queryKey: creativeKeys.feedbackDashboard(workspaceId), exact: true },
          { queryKey: creativeKeys.feedbackMetrics(workspaceId), exact: true },
        ]
        : [{ queryKey: creativeKeys.all(workspaceId) }];

      for (const filter of filters) {
        for (const query of client.getQueryCache().findAll(filter)) {
          query.invalidate();
          if ((!query.isActive() || query.isDisabled()) && query.state.fetchStatus === "idle") continue;
          let entry = pending.get(query.queryHash);
          if (!entry) {
            entry = { key: query.queryKey, dirty: true, running: false };
            pending.set(query.queryHash, entry);
          }
          entry.dirty = true;
          if (query.state.fetchStatus === "idle") schedule(query.queryHash, entry);
        }
      }
    },
    dispose() {
      disposed = true;
      unsubscribe();
      for (const entry of pending.values()) {
        if (entry.timer !== undefined) clearTimeout(entry.timer);
      }
      pending.clear();
    },
  };
}
