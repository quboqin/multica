import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type { CreativeCandidateProgress, CreativeOrderItem } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

export function creativeCandidateProgressLabel(p: CreativeCandidateProgress, t: ReturnType<typeof useT<"creative">>["t"]) {
  switch (p.state) {
    case "planning_incomplete": return t(($) => $.candidateProgress.planning);
    case "planning_invalid": return t(($) => $.candidateProgress.invalid);
    case "generating": return t(($) => $.candidateProgress.generating);
    case "priming": return t(($) => $.candidateProgress.priming);
    case "settling": return t(($) => $.candidateProgress.settling);
    case "primary_incomplete": return t(($) => $.candidateProgress.incomplete);
    case "selection_ready": return t(($) => $.candidateProgress.ready);
    case "selection_queued": return t(($) => $.candidateProgress.queued);
    case "selecting": return t(($) => $.candidateProgress.selecting);
    case "selection_incomplete": return t(($) => $.candidateProgress.selectionFailed);
    case "cancelled": return t(($) => $.candidateProgress.cancelled);
    default: return t(($) => $.candidateProgress.pending);
  }
}

export function CreativeCandidateRecoveryAction({ orderId, item, disabled = false }: { orderId: string; item: CreativeOrderItem; disabled?: boolean }) {
  const { t } = useT("creative");
  const workspaceId = useWorkspaceId();
  const queryClient = useQueryClient();
  const recovery = useMutation({
    mutationFn: () => api.recoverCreativeOrderCandidates(orderId, item.id),
    onSettled: () => Promise.all([
      queryClient.invalidateQueries({ queryKey: creativeKeys.order(workspaceId, orderId), exact: true }),
      queryClient.invalidateQueries({ queryKey: creativeKeys.orders(workspaceId), exact: true }),
    ]),
  });
  const p = item.candidate_progress;
  if (!p) return null;
  const recoverPlan = p.state === "planning_incomplete" && Boolean(p.plan_task_id) && ["completed", "failed"].includes(p.plan_status);
  const recoverSelection = p.state === "selection_ready" || p.state === "selection_incomplete";
  if (!recoverPlan && !recoverSelection) return null;
  return <>
    <Button size="sm" variant="outline" disabled={disabled || recovery.isPending} onClick={() => recovery.mutate()}><RefreshCw className="h-4 w-4" />{recovery.isPending ? t(($) => $.candidateProgress.recovering) : recoverPlan ? t(($) => $.candidateProgress.recoverPlan) : t(($) => $.candidateProgress.recoverSelection)}</Button>
    {recovery.error && <p role="alert" className="text-sm text-destructive">{recovery.error.message}</p>}
  </>;
}
