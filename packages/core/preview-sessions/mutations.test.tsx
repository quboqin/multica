/**
 * @vitest-environment jsdom
 */
import type { ReactNode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import type { PreviewSession, PreviewSessionListResponse } from "../types";
import { useStopPreviewSession } from "./mutations";
import { previewSessionKeys } from "./queries";

function makeSession(
  overrides: Partial<PreviewSession> = {},
): PreviewSession {
  return {
    id: "preview-1",
    workspaceId: "ws-1",
    issueId: "issue-1",
    taskId: null,
    platform: "web",
    provider: "external_web",
    title: "Preview",
    previewUrl: "https://preview.example.test",
    status: "running",
    creatorType: "member",
    creatorId: "user-1",
    errorMessage: null,
    expiresAt: null,
    lastActiveAt: null,
    leaseExpiresAt: null,
    startedAt: "2026-07-13T12:00:00Z",
    stoppedAt: null,
    createdAt: "2026-07-13T12:00:00Z",
    updatedAt: "2026-07-13T12:00:00Z",
    ...overrides,
  };
}

function wrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        {children}
      </QueryClientProvider>
    );
  };
}

afterEach(() => {
  setApiInstance(undefined as unknown as ApiClient);
  vi.restoreAllMocks();
});

describe("useStopPreviewSession", () => {
  it("optimistically marks the list and replaces it with the server result", async () => {
    let resolveStop!: (session: PreviewSession) => void;
    const stopPreviewSession = vi.fn(
      () => new Promise<PreviewSession>((resolve) => {
        resolveStop = resolve;
      }),
    );
    setApiInstance({ stopPreviewSession } as unknown as ApiClient);

    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
        mutations: { retry: false },
      },
    });
    const listKey = previewSessionKeys.issue("ws-1", "issue-1");
    queryClient.setQueryData<PreviewSessionListResponse>(listKey, {
      previewSessions: [makeSession()],
      total: 1,
    });
    const { result } = renderHook(
      () => useStopPreviewSession("ws-1", "issue-1"),
      { wrapper: wrapper(queryClient) },
    );

    act(() => result.current.mutate("preview-1"));
    await waitFor(() =>
      expect(
        queryClient.getQueryData<PreviewSessionListResponse>(listKey)
          ?.previewSessions[0]?.status,
      ).toBe("stopping"),
    );

    act(() =>
      resolveStop(
        makeSession({
          status: "stopped",
          stoppedAt: "2026-07-13T12:01:00Z",
          updatedAt: "2026-07-13T12:01:00Z",
        }),
      ),
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));

    expect(stopPreviewSession).toHaveBeenCalledWith("preview-1");
    expect(
      queryClient.getQueryData<PreviewSessionListResponse>(listKey)
        ?.previewSessions[0]?.status,
    ).toBe("stopped");
    queryClient.clear();
  });
});
