"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { issueRecordLinksOptions } from "@multica/core/collections";
import { useDeleteIssue } from "@multica/core/issues/mutations";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useBackOrReplace } from "../navigation";
import { useT } from "../i18n";

export function DeleteIssueConfirmModal({
  onClose,
  data,
}: {
  onClose: () => void;
  data: Record<string, unknown> | null;
}) {
  const { t } = useT("modals");
  const issueId = (data?.issueId as string) || "";
  // Set only by callers that are rendering the issue we are about to delete
  // (the detail page). List surfaces leave it undefined and simply stay put.
  const fallbackPath = (data?.onDeletedFallbackPath as string | undefined) || undefined;
  const [deleting, setDeleting] = useState(false);
  const deleteIssue = useDeleteIssue();
  // Table records that link to this task keep their link, which then reads as
  // deleted. Worth knowing before confirming, and only worth a line when true.
  const wsId = useCurrentWorkspace()?.id ?? "";
  const { data: recordLinks = [] } = useQuery(issueRecordLinksOptions(wsId, issueId));
  const backOrReplace = useBackOrReplace();

  const handleDelete = async () => {
    if (!issueId) return;
    setDeleting(true);
    try {
      await deleteIssue.mutateAsync(issueId);
      toast.success(t(($) => $.delete_issue.toast_deleted));
      onClose();
      // Back to whichever list the user opened this issue from; `fallbackPath`
      // only kicks in when there is no in-app history to step back into.
      if (fallbackPath) backOrReplace(fallbackPath);
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.delete_issue.toast_delete_failed),
      );
      setDeleting(false);
    }
  };

  return (
    <AlertDialog open onOpenChange={(v) => { if (!v && !deleting) onClose(); }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{t(($) => $.delete_issue.title)}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(($) => $.delete_issue.description)}
            {recordLinks.length > 0 && (
              <span className="mt-1 block">
                {t(($) => $.delete_issue.linked_records, { count: recordLinks.length })}
              </span>
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={deleting}>{t(($) => $.delete_issue.cancel)}</AlertDialogCancel>
          <AlertDialogAction
            onClick={handleDelete}
            disabled={deleting}
            className="bg-destructive text-white hover:bg-destructive/90"
          >
            {deleting ? t(($) => $.delete_issue.deleting) : t(($) => $.delete_issue.confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
