import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import {
  previewSessionKeys,
  previewSessionListOptions,
} from "./queries";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("preview session queries", () => {
  it("isolates issue lists by workspace and issue", () => {
    expect(previewSessionKeys.issue("ws-a", "issue-1")).toEqual([
      "preview-sessions",
      "ws-a",
      "issue",
      "issue-1",
    ]);
    expect(previewSessionKeys.issue("ws-a", "issue-1")).not.toEqual(
      previewSessionKeys.issue("ws-b", "issue-1"),
    );
  });

  it("fetches the issue endpoint and selects the session list", async () => {
    const listPreviewSessions = vi.fn().mockResolvedValue({
      previewSessions: [{ id: "preview-1" }],
      total: 1,
    });
    setApiInstance({ listPreviewSessions } as unknown as ApiClient);
    const options = previewSessionListOptions("ws-a", "issue-1");

    const response = await options.queryFn?.({
      queryKey: options.queryKey,
      signal: new AbortController().signal,
      meta: undefined,
      client: undefined as never,
    });

    expect(listPreviewSessions).toHaveBeenCalledWith("issue-1");
    expect(options.select?.(response!)).toEqual([{ id: "preview-1" }]);
  });
});
