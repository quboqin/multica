/** @vitest-environment jsdom */

import { useState } from "react";
import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider, useQuery } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api, setApiInstance } from "@multica/core/api";
import type { ApiClient } from "@multica/core/api/client";
import { issueKeys } from "@multica/core/issues/queries";
import { useSetIssueProperty } from "@multica/core/properties";
import type { Issue, IssueProperty } from "@multica/core/types";
import { renderWithI18n } from "../../../test/i18n";
import { CustomPropertyValueInput } from "./custom-property-picker";

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const property: IssueProperty = {
  id: "notes",
  workspace_id: "ws-1",
  name: "Notes",
  type: "text",
  config: {},
  position: 1,
  archived: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function makeIssue(value: string): Issue {
  return {
    id: "issue-1",
    workspace_id: "ws-1",
    number: 1,
    identifier: "MUL-1",
    title: "Issue",
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
    properties: { notes: value },
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

function Harness() {
  const [open, setOpen] = useState(true);
  const issue = useQuery({
    queryKey: issueKeys.detail("ws-1", "issue-1"),
    queryFn: () => api.getIssue("issue-1"),
  }).data;
  const mutation = useSetIssueProperty();
  if (!issue) return null;
  return (
    <CustomPropertyValueInput
      property={property}
      value={issue.properties.notes}
      open={open}
      onOpenChange={setOpen}
      onChange={async (value) => {
        try {
          await mutation.mutateAsync({
            issueId: issue.id,
            propertyId: property.id,
            value: value ?? "",
          });
          return true;
        } catch {
          return false;
        }
      }}
    />
  );
}

afterEach(() => vi.restoreAllMocks());

describe("CustomPropertyValueInput mutation lifecycle", () => {
  it("preserves a draft through optimistic update, rollback and authoritative refetch", async () => {
    const user = userEvent.setup({ delay: null });
    let serverIssue = makeIssue("Original");
    let attempt = 0;
    const getIssue = vi.fn(async () => structuredClone(serverIssue));
    const setIssueProperty = vi.fn(
      async (_issueId: string, _propertyId: string, value: string) => {
        attempt += 1;
        if (attempt === 1) throw new Error("first write failed");
        serverIssue = makeIssue(value);
        return { properties: serverIssue.properties };
      },
    );
    setApiInstance({ getIssue, setIssueProperty } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: Infinity } },
    });
    queryClient.setQueryData(issueKeys.detail("ws-1", "issue-1"), serverIssue);
    renderWithI18n(
      <QueryClientProvider client={queryClient}>
        <Harness />
      </QueryClientProvider>,
    );

    const input = screen.getByRole("textbox");
    await user.clear(input);
    await user.type(input, "Draft survives");
    fireEvent.submit(input.closest("form")!);

    await waitFor(() => expect(setIssueProperty).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(getIssue).toHaveBeenCalled());
    expect(screen.getByRole("textbox")).toHaveValue("Draft survives");
    expect(serverIssue.properties.notes).toBe("Original");

    fireEvent.submit(screen.getByRole("textbox").closest("form")!);
    await waitFor(() => expect(setIssueProperty).toHaveBeenCalledTimes(2));
    await waitFor(() =>
      expect(screen.queryByRole("textbox")).not.toBeInTheDocument(),
    );
    expect(serverIssue.properties.notes).toBe("Draft survives");
    queryClient.clear();
  });
});
