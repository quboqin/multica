import type { DataSourceActionResult } from "@multica/core/data-source";
import type { IssueTableCommand } from "@multica/core/issues/table-data-source";
import type { IssueStatusCatalog } from "@multica/core/issue-statuses";
import type { IssueTableRow } from "@multica/core/types";
import type { IssueSurfaceActions } from "../surface/actions-context";
import {
  runConfirmIntent,
  type RunConfirmData,
} from "./run-confirm-gate";

type IssueTableActionResult = DataSourceActionResult<IssueTableRow>;

type CreateIssueTableCommandExecutorOptions = {
  actions: IssueSurfaceActions | null;
  statusCatalog: Pick<IssueStatusCatalog, "entryOf">;
  openRunConfirm: (data: RunConfirmData) => void;
};

function failed(error: unknown): IssueTableActionResult {
  return {
    status: "failed",
    error: error instanceof Error ? error : new Error(String(error)),
  };
}

function once(
  resolve: (result: IssueTableActionResult) => void,
): (result: IssueTableActionResult) => void {
  let settled = false;
  return (result) => {
    if (settled) return;
    settled = true;
    resolve(result);
  };
}

/**
 * Bridges task-specific confirmation and mutation callbacks to the shared
 * data-source result contract. A missing surface provider means there is no
 * mutation completion channel, so the caller must expose a read-only source.
 */
export function createIssueTableCommandExecutor({
  actions,
  statusCatalog,
  openRunConfirm,
}: CreateIssueTableCommandExecutorOptions):
  | ((command: IssueTableCommand) => Promise<IssueTableActionResult>)
  | undefined {
  if (!actions) return undefined;

  return (command) => {
    const intent = runConfirmIntent(command.issue, command.updates, statusCatalog);
    if (intent) {
      return new Promise((resolve) => {
        const settle = once(resolve);
        openRunConfirm({
          ...intent,
          onAccepted: () => settle({ status: "accepted" }),
          onCancelled: () => settle({ status: "cancelled" }),
          onFailed: (error) => settle(failed(error)),
        });
      });
    }

    return new Promise((resolve) => {
      const settle = once(resolve);
      try {
        actions.updateIssue(command.issue.id, command.updates, {
          onSuccess: () => settle({ status: "accepted" }),
          onError: (error) => settle(failed(error)),
        });
      } catch (error) {
        settle(failed(error));
      }
    });
  };
}
