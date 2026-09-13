/**
 * @vitest-environment jsdom
 */
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import type { Issue, IssuePropertiesResponse } from "../types";
import { issueKeys } from "../issues/queries";
import { setCurrentWorkspace } from "../platform";
import { useSetIssueProperty } from "./mutations";

const workspaceState = vi.hoisted(() => ({ id: "ws-1" }));

vi.mock("../hooks", () => ({ useWorkspaceId: () => workspaceState.id }));

const issue: Issue = {
  id: "issue-1",
  workspace_id: "ws-1",
  number: 1,
  identifier: "MUL-1",
  title: "Estimate",
  description: null,
  status: "todo",
  priority: "none",
  assignee_type: null,
  assignee_id: null,
  creator_type: "member",
  creator_id: "member-1",
  parent_issue_id: null,
  project_id: null,
  position: 1,
  stage: null,
  start_date: null,
  due_date: null,
  labels: [],
  metadata: {},
  properties: { estimate: 1 },
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function wrapper(qc: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("useSetIssueProperty", () => {
  afterEach(() => {
    workspaceState.id = "ws-1";
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
  });

  it.each([
    { change: "workspace switch", switchWorkspace: true },
    { change: "capability revoke", switchWorkspace: false },
  ])("rejects a queued write after $change", async ({ switchWorkspace }) => {
    setCurrentWorkspace("alpha", "ws-1");
    let writable = true;
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    qc.setQueryData(issueKeys.detail("ws-1", issue.id), issue);
    qc.setQueryData(issueKeys.detail("ws-2", issue.id), {
      ...issue,
      workspace_id: "ws-2",
      properties: { estimate: 99 },
    });
    const firstResponse = deferred<IssuePropertiesResponse>();
    const setIssueProperty = vi
      .fn()
      .mockImplementationOnce(() => firstResponse.promise)
      .mockResolvedValue({ properties: { estimate: 3 } });
    setApiInstance({ setIssueProperty } as unknown as ApiClient);
    const { result } = renderHook(() => useSetIssueProperty(), {
      wrapper: wrapper(qc),
    });
    const workspaceContext = {
      workspaceId: "ws-1",
      workspaceSlug: "alpha",
      isActive: () => writable,
    };

    const first = result.current.mutateAsync({
      issueId: issue.id,
      propertyId: "estimate",
      value: 2,
      workspaceContext,
    });
    await waitFor(() => expect(setIssueProperty).toHaveBeenCalledTimes(1));
    const second = result.current
      .mutateAsync({
        issueId: issue.id,
        propertyId: "estimate",
        value: 3,
        workspaceContext,
      })
      .catch((error: unknown) => error);

    if (switchWorkspace) setCurrentWorkspace("beta", "ws-2");
    else writable = false;
    firstResponse.resolve({ properties: { estimate: 2 } });
    await first;
    expect(await second).toMatchObject({
      message: "Workspace or write capability changed before submission",
    });
    expect(setIssueProperty).toHaveBeenCalledTimes(1);
    expect(setIssueProperty).toHaveBeenCalledWith(
      issue.id,
      "estimate",
      2,
      "alpha",
    );
    expect(
      qc.getQueryData<Issue>(issueKeys.detail("ws-2", issue.id))?.properties
        .estimate,
    ).toBe(99);
    qc.clear();
  });

  it("keeps a submitted write's cache callbacks in the captured workspace", async () => {
    setCurrentWorkspace("alpha", "ws-1");
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const unrelated = {
      ...issue,
      workspace_id: "ws-2",
      properties: { estimate: 99 },
    };
    qc.setQueryData(issueKeys.detail("ws-1", issue.id), issue);
    qc.setQueryData(issueKeys.detail("ws-2", issue.id), unrelated);
    const response = deferred<IssuePropertiesResponse>();
    const setIssueProperty = vi.fn(() => response.promise);
    setApiInstance({ setIssueProperty } as unknown as ApiClient);
    const hook = renderHook(() => useSetIssueProperty(), {
      wrapper: wrapper(qc),
    });
    const completion = hook.result.current.mutateAsync({
      issueId: issue.id,
      propertyId: "estimate",
      value: 2,
      workspaceContext: {
        workspaceId: "ws-1",
        workspaceSlug: "alpha",
      },
    });
    await waitFor(() => expect(setIssueProperty).toHaveBeenCalledTimes(1));

    workspaceState.id = "ws-2";
    setCurrentWorkspace("beta", "ws-2");
    hook.rerender();
    response.resolve({ properties: { estimate: 2 } });
    await completion;

    expect(
      qc.getQueryData<Issue>(issueKeys.detail("ws-1", issue.id))?.properties
        .estimate,
    ).toBe(2);
    expect(qc.getQueryData(issueKeys.detail("ws-2", issue.id))).toEqual(
      unrelated,
    );
    hook.unmount();
    qc.clear();
  });

  it("keeps an optimistic children-cache patch when an older fetch resolves late", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const parentId = "parent-1";
    const childKey = issueKeys.children("ws-1", parentId);
    const child = {
      ...issue,
      parent_issue_id: parentId,
      properties: { estimate: 1, environment: "staging" },
    };
    qc.setQueryData<Issue[]>(childKey, [child]);

    const staleChildren = deferred<Issue[]>();
    const staleFetch = qc
      .fetchQuery({
        queryKey: childKey,
        queryFn: () => staleChildren.promise,
      })
      .catch(() => undefined);
    await waitFor(() =>
      expect(qc.getQueryState(childKey)?.fetchStatus).toBe("fetching"),
    );

    const write = deferred<IssuePropertiesResponse>();
    setApiInstance({
      setIssueProperty: vi.fn(() => write.promise),
    } as unknown as ApiClient);

    const { result } = renderHook(() => useSetIssueProperty(), {
      wrapper: wrapper(qc),
    });
    act(() => {
      result.current.mutate({
        issueId: issue.id,
        propertyId: "estimate",
        value: 2,
      });
    });

    await waitFor(() =>
      expect(qc.getQueryData<Issue[]>(childKey)?.[0]?.properties).toEqual({
        estimate: 2,
        environment: "staging",
      }),
    );

    // Resolve the response captured before the mutation. cancelQueries must
    // keep it from restoring estimate=1 while the write is still pending.
    staleChildren.resolve([child]);
    await staleFetch;
    expect(qc.getQueryData<Issue[]>(childKey)?.[0]?.properties).toEqual({
      estimate: 2,
      environment: "staging",
    });

    await act(async () => {
      write.resolve({
        properties: { estimate: 2, environment: "staging" },
      });
    });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    qc.clear();
  });

  it("does not refetch a property window before the mutation commits", async () => {
    const qc = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const flatKey = issueKeys.flat(
      "ws-1",
      "workspace:all",
      {},
      { sort_by: "property:estimate", properties: { estimate: ["2"] } },
    );
    qc.setQueryData(flatKey, {
      pages: [{ issues: [issue], total: 1 }],
      pageParams: [0],
    });

    let resolveWrite!: (value: IssuePropertiesResponse) => void;
    const setIssueProperty = vi.fn(
      () =>
        new Promise<IssuePropertiesResponse>((resolve) => {
          resolveWrite = resolve;
        }),
    );
    setApiInstance({ setIssueProperty } as unknown as ApiClient);

    const { result } = renderHook(() => useSetIssueProperty(), {
      wrapper: wrapper(qc),
    });
    act(() => {
      result.current.mutate({
        issueId: issue.id,
        propertyId: "estimate",
        value: 2,
      });
    });

    await waitFor(() =>
      expect(
        qc.getQueryData<{ pages: { issues: Issue[] }[] }>(flatKey)?.pages[0]
          ?.issues[0]?.properties.estimate,
      ).toBe(2),
    );
    expect(qc.getQueryState(flatKey)?.isInvalidated).toBe(false);

    await act(async () => {
      resolveWrite({ properties: { estimate: 2 } });
    });
    await waitFor(() =>
      expect(qc.getQueryState(flatKey)?.isInvalidated).toBe(true),
    );
    qc.clear();
  });
});
