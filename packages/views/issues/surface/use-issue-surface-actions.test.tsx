/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, renderHook } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { WorkspaceSlugProvider } from "@multica/core/paths";
import { setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { workspaceKeys } from "@multica/core/workspace/queries";
import type { Issue, Workspace } from "@multica/core/types";
import type { ReactNode } from "react";
import { RESOURCES } from "../../test/i18n";
import { createIssueTableCommandExecutor } from "../actions/table-command-executor";
import { useIssueSurfaceActions } from "./use-issue-surface-actions";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function makeIssue(id: string, title = id): Issue {
  return {
    id,
    workspace_id: "ws-1",
    number: id === "issue-a" ? 1 : 2,
    identifier: id === "issue-a" ? "MUL-1" : "MUL-2",
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
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

const workspace: Workspace = {
  id: "ws-1",
  name: "Acme",
  slug: "acme",
  description: null,
  context: null,
  settings: {},
  repos: [],
  issue_prefix: "MUL",
  avatar_url: null,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function setup(updateIssue: ApiClient["updateIssue"]) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  });
  queryClient.setQueryData<Workspace[]>(workspaceKeys.list(), [workspace]);
  setApiInstance({ updateIssue } as ApiClient);

  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>
        <WorkspaceSlugProvider slug="acme">
          <I18nProvider locale="en" resources={RESOURCES}>
            {children}
          </I18nProvider>
        </WorkspaceSlugProvider>
      </QueryClientProvider>
    );
  }

  const hook = renderHook(
    () => useIssueSurfaceActions({ createDefaults: {} }),
    { wrapper: Wrapper },
  );
  const execute = createIssueTableCommandExecutor({
    actions: hook.result.current.actions,
    statusCatalog: { entryOf: () => undefined },
    openRunConfirm: vi.fn(),
  })!;
  return { ...hook, execute, queryClient };
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("issue surface per-request table writes", () => {
  it("settles consecutive writes independently in reverse completion order", async () => {
    const writes = {
      "issue-a": deferred<Issue>(),
      "issue-b": deferred<Issue>(),
    };
    const updateIssue = vi.fn((id: string) => writes[id as keyof typeof writes].promise);
    const { execute, queryClient } = setup(updateIssue as ApiClient["updateIssue"]);
    let resultA!: ReturnType<typeof execute>;
    let resultB!: ReturnType<typeof execute>;
    let aSettled = false;

    act(() => {
      resultA = execute({
        issue: makeIssue("issue-a"),
        updates: { title: "A updated" },
      }).then((result) => {
        aSettled = true;
        return result;
      });
      resultB = execute({
        issue: makeIssue("issue-b"),
        updates: { title: "B updated" },
      });
    });

    await act(async () => {
      writes["issue-b"].resolve(makeIssue("issue-b", "B updated"));
      await expect(resultB).resolves.toEqual({ status: "accepted" });
    });
    expect(aSettled).toBe(false);

    const error = new Error("A failed");
    await act(async () => {
      writes["issue-a"].reject(error);
      await expect(resultA).resolves.toEqual({ status: "failed", error });
    });
    expect(updateIssue).toHaveBeenCalledTimes(2);
    queryClient.clear();
  });

  it("settles every in-flight write after the surface unmounts", async () => {
    const writes = {
      "issue-a": deferred<Issue>(),
      "issue-b": deferred<Issue>(),
    };
    const updateIssue = vi.fn((id: string) => writes[id as keyof typeof writes].promise);
    const { execute, unmount, queryClient } = setup(
      updateIssue as ApiClient["updateIssue"],
    );
    let resultA!: ReturnType<typeof execute>;
    let resultB!: ReturnType<typeof execute>;

    act(() => {
      resultA = execute({
        issue: makeIssue("issue-a"),
        updates: { title: "A updated" },
      });
      resultB = execute({
        issue: makeIssue("issue-b"),
        updates: { title: "B updated" },
      });
    });
    unmount();

    const error = new Error("B failed");
    writes["issue-b"].reject(error);
    writes["issue-a"].resolve(makeIssue("issue-a", "A updated"));

    await expect(Promise.all([resultA, resultB])).resolves.toEqual([
      { status: "accepted" },
      { status: "failed", error },
    ]);
    queryClient.clear();
  });
});
