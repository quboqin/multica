// @vitest-environment node
import { QueryClient, QueryObserver } from "@tanstack/react-query";
import { expect, it, vi } from "vitest";
import { refreshDataSource } from "./query";
it("discards a pre-event HTTP response after reconnect/event refresh and isolates workspace keys", async () => {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  const reads: Array<(value: { revision: number }) => void> = [];
  const key = ["records", "workspace-a", "source"];
  const observer = new QueryObserver(qc, {
    queryKey: key,
    queryFn: () =>
      new Promise<{ revision: number }>((resolve) => reads.push(resolve)),
  });
  const off = observer.subscribe(() => {});
  await vi.waitFor(() => expect(reads).toHaveLength(1));
  qc.setQueryData(["records", "workspace-b", "source"], { revision: 10 });
  const refresh = refreshDataSource(qc, ["records", "workspace-a"]);
  await vi.waitFor(() => expect(reads).toHaveLength(2));
  reads[1]!({ revision: 2 });
  await refresh;
  reads[0]!({ revision: 1 });
  await Promise.resolve();
  expect(qc.getQueryData(key)).toEqual({ revision: 2 });
  expect(qc.getQueryData(["records", "workspace-b", "source"])).toEqual({
    revision: 10,
  });
  off();
  qc.clear();
});
