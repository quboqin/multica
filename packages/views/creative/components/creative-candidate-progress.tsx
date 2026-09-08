import { useMutation, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type { CreativeCandidateProgress, CreativeOrderItem } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

function progressLabel(p: CreativeCandidateProgress, t: ReturnType<typeof useT<"creative">>["t"]) {
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

export function CreativeCandidateProgressPanel({ orderId, item }: { orderId: string; item: CreativeOrderItem }) {
  const { t } = useT("creative");
  const workspaceId = useWorkspaceId();
  const queryClient = useQueryClient();
  const recovery = useMutation({
    mutationFn: () => api.recoverCreativeOrderCandidates(orderId, item.id),
    onSettled: () => queryClient.invalidateQueries({ queryKey: creativeKeys.orders(workspaceId) }),
  });
  const p = item.candidate_progress;
  if (!p) return null;
  const recoverPlan = p.state === "planning_incomplete" && Boolean(p.plan_task_id) && ["completed", "failed"].includes(p.plan_status);
  const recoverSelection = p.state === "selection_ready" || p.state === "selection_incomplete";
  return <section className="flex flex-wrap items-center gap-3 border-y py-3" aria-label={t(($) => $.candidateProgress.title)}>
    <div className="min-w-0 flex-1"><p role="status" className="text-sm font-medium">{progressLabel(p, t)}</p><p className="mt-1 text-xs text-muted-foreground">{t(($) => $.candidateProgress.counts, { planned: p.planned, expected: p.expected, generated: p.generated, primed: p.primed, target: p.target })}</p>
      {recovery.error && <p role="alert" className="mt-2 text-sm text-destructive">{recovery.error.message}</p>}
    </div>
    {(recoverPlan || recoverSelection) && <Button size="sm" variant="outline" disabled={recovery.isPending} onClick={() => recovery.mutate()}><RefreshCw className="h-4 w-4" />{recovery.isPending ? t(($) => $.candidateProgress.recovering) : recoverPlan ? t(($) => $.candidateProgress.recoverPlan) : t(($) => $.candidateProgress.recoverSelection)}</Button>}
  </section>;
}
