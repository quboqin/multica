"use client";

import { createContext, useContext, type ReactNode } from "react";
import type { Issue, UpdateIssueRequest } from "@multica/core/types";
import type { WorkspaceRequestContext } from "@multica/core/platform";
import type { IssueCreateDefaults } from "./types";

export type IssueSurfaceMutationOptions = {
  errorMessage?: string;
  onSuccess?: (issue: Issue) => void;
  onError?: (err: unknown) => void;
  onSettled?: () => void;
};

export interface IssueSurfaceActions {
  isPending: boolean;
  createIssue: (defaults?: IssueCreateDefaults) => void;
  updateIssue: (
    issueId: string,
    updates: Partial<UpdateIssueRequest>,
    options?: IssueSurfaceMutationOptions,
  ) => void;
  /**
   * Per-request completion channel for callers that must observe the final
   * server result. Unlike mutate callbacks, concurrent calls keep independent
   * promises and continue settling after the surface unmounts.
   */
  updateIssueAsync: (
    issueId: string,
    updates: Partial<UpdateIssueRequest>,
    workspaceContext?: WorkspaceRequestContext,
  ) => Promise<Issue>;
  moveIssue: (
    issueId: string,
    updates: Partial<UpdateIssueRequest>,
    options?: IssueSurfaceMutationOptions,
  ) => void;
  batchUpdate: (
    issueIds: string[],
    updates: Partial<UpdateIssueRequest>,
  ) => Promise<void>;
  batchDelete: (issueIds: string[]) => Promise<void>;
}

const IssueSurfaceActionsContext = createContext<IssueSurfaceActions | null>(
  null,
);

export function IssueSurfaceActionsProvider({
  actions,
  children,
}: {
  actions: IssueSurfaceActions;
  children: ReactNode;
}) {
  return (
    <IssueSurfaceActionsContext.Provider value={actions}>
      {children}
    </IssueSurfaceActionsContext.Provider>
  );
}

export function useIssueSurfaceActionsOptional() {
  return useContext(IssueSurfaceActionsContext);
}
