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
): {
  settle: (result: IssueTableActionResult) => void;
  isSettled: () => boolean;
} {
  let settled = false;
  return {
    settle: (result) => {
      if (settled) return;
      settled = true;
      resolve(result);
    },
    isSettled: () => settled,
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
        const completion = once(resolve);
        let phase: "awaiting-confirmation" | "submitting" | "settled" =
          "awaiting-confirmation";
        const settle = (result: IssueTableActionResult) => {
          phase = "settled";
          completion.settle(result);
        };
        openRunConfirm({
          ...intent,
          canCancel: () => phase === "awaiting-confirmation",
          onSubmitting: () => {
            if (!completion.isSettled()) phase = "submitting";
          },
          onAccepted: () => settle({ status: "accepted" }),
          onCancelled: () => {
            if (phase === "awaiting-confirmation") {
              settle({ status: "cancelled" });
            }
          },
          onFailed: (error) => settle(failed(error)),
        });
      });
    }

    return actions
      .updateIssueAsync(command.issue.id, command.updates)
      .then(() => ({ status: "accepted" as const }))
      .catch(failed);
  };
}
