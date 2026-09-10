import { QueryClient, QueryObserver, type QueryKey } from "@tanstack/react-query";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { creativeKeys } from "./queries";
import { createCreativeRefreshQueue } from "./realtime-refresh";

let client: QueryClient;
let refresh: ReturnType<typeof createCreativeRefreshQueue>;
let cleanups: (() => void)[];

beforeEach(() => {
  vi.useFakeTimers();
  client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity, gcTime: Infinity } } });
  refresh = createCreativeRefreshQueue(client);
  cleanups = [];
});
afterEach(() => {
  refresh.dispose();
  cleanups.forEach((cleanup) => cleanup());
  client.clear();
  vi.useRealTimers();
});

function observe(key: QueryKey, queryFn: () => Promise<number>, seed = true) {
  if (seed) client.setQueryData(key, 0);
  const observer = new QueryObserver(client, { queryKey: key, queryFn });
  cleanups.push(observer.subscribe(() => {}));
}

function deferred() {
  let resolve!: (value: number) => void;
  const promise = new Promise<number>((done) => { resolve = done; });
  return { promise, resolve };
}

it("coalesces a burst while preserving the final order value", async () => {
  const key = creativeKeys.order("ws", "order");
  let version = 0;
  const fetch = vi.fn(async () => version);
  observe(key, fetch);
  for (let i = 0; i < 25; i++) { version++; refresh.invalidate("ws", { scope: "order", order_id: "order" }); }
  await vi.advanceTimersByTimeAsync(250);
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(client.getQueryData(key)).toBe(25);
});

it("waits for an in-flight request and follows it with one fresh read", async () => {
  const key = creativeKeys.order("ws", "order");
  const first = deferred();
  const fetch = vi.fn().mockImplementationOnce(() => first.promise).mockResolvedValue(25);
  observe(key, fetch, false);
  for (let i = 0; i < 25; i++) refresh.invalidate("ws");
  await vi.advanceTimersByTimeAsync(1000);
  expect(fetch).toHaveBeenCalledTimes(1);
  first.resolve(1);
  await vi.advanceTimersByTimeAsync(250);
  expect(fetch).toHaveBeenCalledTimes(2);
  expect(client.getQueryData(key)).toBe(25);
});

it("does not let a slow dashboard block order progress", async () => {
  const dashboard = deferred();
  observe(creativeKeys.feedbackDashboard("ws"), () => dashboard.promise);
  const key = creativeKeys.order("ws", "order");
  const fetch = vi.fn().mockResolvedValueOnce(1).mockResolvedValueOnce(2);
  observe(key, fetch);
  refresh.invalidate("ws");
  await vi.advanceTimersByTimeAsync(250);
  refresh.invalidate("ws", { scope: "order", order_id: "order" });
  await vi.advanceTimersByTimeAsync(250);
  expect(client.getQueryData(key)).toBe(2);
  expect(fetch).toHaveBeenCalledTimes(2);
  dashboard.resolve(2);
});

it("refreshes only the changed order and invalidates inactive data without fetching it", async () => {
  const changed = vi.fn(async () => 1);
  const sibling = vi.fn(async () => 1);
  const material = vi.fn(async () => 1);
  observe(creativeKeys.order("ws", "changed"), changed);
  observe(creativeKeys.order("ws", "sibling"), sibling);
  observe(creativeKeys.materials("ws"), material);
  client.setQueryData(creativeKeys.feedbackDashboard("ws"), 0);
  refresh.invalidate("ws", { scope: "order", order_id: "changed" });
  await vi.advanceTimersByTimeAsync(250);
  expect(changed).toHaveBeenCalledTimes(1);
  expect(sibling).not.toHaveBeenCalled();
  expect(material).not.toHaveBeenCalled();
  expect(client.getQueryState(creativeKeys.feedbackDashboard("ws"))?.isInvalidated).toBe(true);
});

it("keeps inactive data stale when an older request completes after a notification", async () => {
  const key = creativeKeys.order("ws", "order");
  const first = deferred();
  observe(key, () => first.promise, false);
  cleanups[0]!();
  refresh.invalidate("ws");
  first.resolve(1);
  await vi.advanceTimersByTimeAsync(250);
  expect(client.getQueryState(key)?.isInvalidated).toBe(true);
});

it("continues updating during sustained notifications instead of postponing forever", async () => {
  const fetch = vi.fn(async () => 1);
  observe(creativeKeys.order("ws", "order"), fetch);
  for (let i = 0; i < 20; i++) { refresh.invalidate("ws"); await vi.advanceTimersByTimeAsync(50); }
  expect(fetch.mock.calls.length).toBeGreaterThanOrEqual(3);
  expect(fetch.mock.calls.length).toBeLessThanOrEqual(4);
});

it("disposes pending refreshes when the workspace subscription is removed", async () => {
  const fetch = vi.fn(async () => 1);
  observe(creativeKeys.order("ws", "order"), fetch);
  refresh.invalidate("ws");
  refresh.dispose();
  await vi.advanceTimersByTimeAsync(1000);
  expect(fetch).not.toHaveBeenCalled();
});
