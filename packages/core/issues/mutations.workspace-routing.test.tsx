/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";

import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { useWorkspaceId } from "../hooks";
import { WorkspaceSlugProvider } from "../paths";
import { setCurrentWorkspace } from "../platform";
import type { Issue, Workspace } from "../types";
import { workspaceKeys } from "../workspace/queries";
import { useUpdateIssue } from "./mutations";
import { issueKeys } from "./queries";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

function makeWorkspace(id: string, slug: string): Workspace {
  return {
    id,
    slug,
    name: slug,
    description: null,
    context: null,
    settings: {},
    repos: [],
    issue_prefix: slug.toUpperCase(),
    avatar_url: null,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function makeIssue(workspaceId: string, title: string, revision: number): Issue {
  return {
    id: "issue-1",
    workspace_id: workspaceId,
    number: 1,
    identifier: "MUL-1",
    title,
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
    properties: {},
    revision,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

describe("useUpdateIssue workspace routing", () => {
  afterEach(() => {
    setCurrentWorkspace(null, null);
    vi.restoreAllMocks();
  });

  it.each(["success", "failure"] as const)(
    "keeps an already-submitted %s lifecycle on the workspace selected by the real slug provider",
    async (outcome) => {
      const route = { slug: "alpha" };
      const workspaceA = makeWorkspace("ws-a", "alpha");
      const workspaceB = makeWorkspace("ws-b", "beta");
      const issueA = makeIssue(workspaceA.id, "Original A", 1);
      const issueB = makeIssue(workspaceB.id, "Unrelated B", 20);
      const keyA = issueKeys.detail(workspaceA.id, issueA.id);
      const keyB = issueKeys.detail(workspaceB.id, issueB.id);
      const request = deferred<Issue>();
      const updateIssue = vi.fn(() => request.promise);
      setApiInstance({ updateIssue } as unknown as ApiClient);

      const queryClient = new QueryClient({
        defaultOptions: {
          queries: { retry: false, staleTime: Infinity },
          mutations: { retry: false },
        },
      });
      queryClient.setQueryData(workspaceKeys.list(), [workspaceA, workspaceB]);
      queryClient.setQueryData(keyA, issueA);
      queryClient.setQueryData(keyB, issueB);
      const wrapper = ({ children }: { children: ReactNode }) => (
        <QueryClientProvider client={queryClient}>
          <WorkspaceSlugProvider slug={route.slug}>
            {children}
          </WorkspaceSlugProvider>
        </QueryClientProvider>
      );
      setCurrentWorkspace(workspaceA.slug, workspaceA.id);
      const hook = renderHook(
        () => ({ workspaceId: useWorkspaceId(), update: useUpdateIssue() }),
        { wrapper },
      );
      expect(hook.result.current.workspaceId).toBe(workspaceA.id);

      let completion!: Promise<Issue>;
      act(() => {
        completion = hook.result.current.update.mutateAsync({
          id: issueA.id,
          title: "Optimistic A",
          workspaceContext: {
            workspaceId: workspaceA.id,
            workspaceSlug: workspaceA.slug,
          },
        });
      });
      const settled = completion.catch((error: unknown) => error);
      await waitFor(() =>
        expect(updateIssue).toHaveBeenCalledWith(
          issueA.id,
          { title: "Optimistic A" },
          workspaceA.slug,
        ),
      );

      route.slug = workspaceB.slug;
      setCurrentWorkspace(workspaceB.slug, workspaceB.id);
      hook.rerender();
      expect(hook.result.current.workspaceId).toBe(workspaceB.id);
      await act(async () => {
        if (outcome === "success") {
          request.resolve({
            ...issueA,
            title: "Committed A",
            revision: 2,
          });
        } else {
          request.reject(new Error("write failed"));
        }
        await settled;
      });

      expect(queryClient.getQueryData<Issue>(keyA)?.title).toBe(
        outcome === "success" ? "Committed A" : "Original A",
      );
      expect(queryClient.getQueryData(keyB)).toEqual(issueB);
      expect(queryClient.getQueryState(keyB)?.isInvalidated).toBe(false);
      hook.unmount();
      queryClient.clear();
    },
  );
});
