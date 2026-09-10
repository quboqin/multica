import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient, setApiInstance } from "../api";
import { creativeOrderOptions } from "./queries";

afterEach(() => vi.unstubAllGlobals());

it("aborts the actual order HTTP request when the detail observer leaves", () => {
  let signal: AbortSignal | null | undefined;
  let aborted = false;
  vi.stubGlobal("fetch", vi.fn((_url: string, init?: RequestInit) => {
    signal = init?.signal;
    return new Promise<Response>((_resolve, reject) => {
      signal?.addEventListener("abort", () => {
        aborted = true;
        reject(new DOMException("Aborted", "AbortError"));
      }, { once: true });
    });
  }));
  setApiInstance(new ApiClient("https://example.test"));
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: Infinity } } });
  try {
    const observer = new QueryObserver(client, creativeOrderOptions("ws", "order"));
    const unsubscribe = observer.subscribe(() => {});
    expect(signal).toBeInstanceOf(AbortSignal);
    unsubscribe();
    expect(aborted).toBe(true);
    expect(signal?.aborted).toBe(true);
  } finally {
    client.clear();
  }
});

it("passes cancellation through every creative read API", async () => {
  const client = new ApiClient("https://example.test");
  const controller = new AbortController();
  const fetch = vi.fn(async (_url: string, _init?: RequestInit) => new Response("{}", { status: 200 }));
  vi.stubGlobal("fetch", fetch);
  const signal = controller.signal;
  await Promise.all([
    client.listCreativeMaterialLibrary(undefined, signal), client.listCreativeResources(undefined, signal),
    client.listCreativeResourceFiles("resource", signal), client.getCreativeMaterials("issue", signal),
    client.listCreativeFeedback(undefined, undefined, signal), client.getCreativeFeedbackMetrics(signal),
    client.getCreativeFeedbackDashboard(signal), client.listCreativeOrders(signal),
    client.getCreativeOrder("order", signal), client.listCreativeSourceAnalyses(undefined, signal),
  ]);
  expect(fetch).toHaveBeenCalledTimes(10);
  for (const call of fetch.mock.calls) expect(call[1]?.signal).toBe(signal);
});
