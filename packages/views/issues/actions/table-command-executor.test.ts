// @vitest-environment node

import { describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import type { IssueSurfaceActions } from "../surface/actions-context";
import type { RunConfirmData } from "./run-confirm-gate";
import { createIssueTableCommandExecutor } from "./table-command-executor";

function makeIssue(overrides: Partial<Issue> = {}): Issue {
  return {
    id: "issue-1",
    workspace_id: "ws-1",
    number: 1,
    identifier: "MUL-1",
    title: "First issue",
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
    ...overrides,
  };
}

function actions(
  updateIssueAsync: IssueSurfaceActions["updateIssueAsync"] = vi.fn(
    async () => makeIssue(),
  ),
): IssueSurfaceActions {
  return { updateIssueAsync } as IssueSurfaceActions;
}

const statusCatalog = { entryOf: () => undefined };

describe("createIssueTableCommandExecutor", () => {
  it("returns no executor when the surface cannot write", () => {
    expect(
      createIssueTableCommandExecutor({
        actions: null,
        statusCatalog,
        openRunConfirm: vi.fn(),
      }),
    ).toBeUndefined();
  });

  it("waits for a direct mutation's success before accepting", async () => {
    let finishWrite: ((issue: Issue) => void) | undefined;
    const updateIssueAsync = vi.fn(() => {
      return new Promise<Issue>((resolve) => {
        finishWrite = resolve;
      });
    });
    const execute = createIssueTableCommandExecutor({
      actions: actions(updateIssueAsync),
      statusCatalog,
      openRunConfirm: vi.fn(),
    })!;
    let settled = false;

    const result = execute({
      issue: makeIssue(),
      updates: { title: "Renamed" },
    }).then((value) => {
      settled = true;
      return value;
    });
    await Promise.resolve();

    expect(settled).toBe(false);
    expect(updateIssueAsync).toHaveBeenCalledTimes(1);
    finishWrite?.(makeIssue({ title: "Renamed" }));
    await expect(result).resolves.toEqual({ status: "accepted" });
  });

  it("returns failed when a direct mutation reports an error", async () => {
    const error = new Error("write failed");
    const execute = createIssueTableCommandExecutor({
      actions: actions(async () => {
        throw error;
      }),
      statusCatalog,
      openRunConfirm: vi.fn(),
    })!;

    const result = await execute({
      issue: makeIssue(),
      updates: { title: "Renamed" },
    });

    expect(result).toEqual({ status: "failed", error });
  });

  it("does not accept or write a gated command before confirmation", async () => {
    let modalData: RunConfirmData | undefined;
    const updateIssueAsync = vi.fn(async () => makeIssue());
    const execute = createIssueTableCommandExecutor({
      actions: actions(updateIssueAsync),
      statusCatalog,
      openRunConfirm: (data) => {
        modalData = data;
      },
    })!;
    let settled = false;

    const result = execute({
      issue: makeIssue(),
      updates: { assignee_type: "agent", assignee_id: "agent-1" },
    }).then((value) => {
      settled = true;
      return value;
    });
    await Promise.resolve();

    expect(updateIssueAsync).not.toHaveBeenCalled();
    expect(settled).toBe(false);
    modalData?.onAccepted?.();
    await expect(result).resolves.toEqual({ status: "accepted" });
  });

  it("returns cancelled when the confirmation is dismissed", async () => {
    let modalData: RunConfirmData | undefined;
    const execute = createIssueTableCommandExecutor({
      actions: actions(vi.fn()),
      statusCatalog,
      openRunConfirm: (data) => {
        modalData = data;
      },
    })!;

    const result = execute({
      issue: makeIssue(),
      updates: { assignee_type: "agent", assignee_id: "agent-1" },
    });
    modalData?.onCancelled?.();

    await expect(result).resolves.toEqual({ status: "cancelled" });
  });

  it("ignores cancellation after submission and waits for the mutation", async () => {
    let modalData: RunConfirmData | undefined;
    const execute = createIssueTableCommandExecutor({
      actions: actions(),
      statusCatalog,
      openRunConfirm: (data) => {
        modalData = data;
      },
    })!;
    let settled = false;

    const result = execute({
      issue: makeIssue(),
      updates: { assignee_type: "agent", assignee_id: "agent-1" },
    }).then((value) => {
      settled = true;
      return value;
    });
    expect(modalData?.canCancel?.()).toBe(true);
    modalData?.onSubmitting?.();
    expect(modalData?.canCancel?.()).toBe(false);
    modalData?.onCancelled?.();
    await Promise.resolve();

    expect(settled).toBe(false);
    modalData?.onAccepted?.();
    await expect(result).resolves.toEqual({ status: "accepted" });
  });

  it("returns failed when the confirmed mutation fails", async () => {
    let modalData: RunConfirmData | undefined;
    const error = new Error("confirmed write failed");
    const execute = createIssueTableCommandExecutor({
      actions: actions(vi.fn()),
      statusCatalog,
      openRunConfirm: (data) => {
        modalData = data;
      },
    })!;

    const result = execute({
      issue: makeIssue(),
      updates: { assignee_type: "agent", assignee_id: "agent-1" },
    });
    modalData?.onFailed?.(error);

    await expect(result).resolves.toEqual({ status: "failed", error });
  });
});
