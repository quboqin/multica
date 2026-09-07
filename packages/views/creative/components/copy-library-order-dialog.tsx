"use client";

import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, LoaderCircle, Sparkles } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, creativeResourceFilesOptions, creativeResourcesOptions, parseCreativeCopyLibraryConfig } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { squadListOptions } from "@multica/core/workspace/queries";
import type { CreativeCopyFragmentRole, CreativeResource, CreativeType } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Badge } from "@multica/ui/components/ui/badge";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { toast } from "sonner";
import { useT } from "../../i18n";
import { creativeSubmissionKey, EMPTY_VISUAL_DIRECTION, VisualDirectionEditor, visualDirectionSummary } from "./creative-material-library";

const COPY_SLOTS: CreativeCopyFragmentRole[] = ["headline", "subheadline", "benefit", "supporting", "cta", "legal"];

export function CopyLibraryOrderDialog({ library, onClose, onCreated }: { library: CreativeResource; onClose: () => void; onCreated: (orderId: string) => void }) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const resources = useQuery(creativeResourcesOptions(wsId));
  const squads = useQuery(squadListOptions(wsId));
  const markets = (resources.data?.resources ?? []).filter((resource) => resource.kind === "market_pack" && resource.status !== "archived" && resource.published_version > 0 && resource.published_config?.copy_library_id === library.id);
  const [marketId, setMarketId] = useState("");
  const market = markets.find((resource) => resource.id === marketId) ?? (markets.length === 1 ? markets[0] : undefined);
  const files = useQuery(creativeResourceFilesOptions(wsId, market?.id ?? ""));
  const [squadId, setSquadId] = useState("");
  const squad = squads.data?.find((entry) => entry.id === squadId) ?? (squads.data?.length === 1 ? squads.data[0] : undefined);
  const config = useMemo(() => parseCreativeCopyLibraryConfig(library.published_config ?? {}), [library.published_config]);
  const [creativeType, setCreativeType] = useState<CreativeType>("num");
  const [slots, setSlots] = useState<Partial<Record<CreativeCopyFragmentRole, string[]>>>({});
  const [plans, setPlans] = useState<string[]>([]);
  const [direction, setDirection] = useState(EMPTY_VISUAL_DIRECTION);
  const [recipeId, setRecipeId] = useState("");
  const available = config.fragments.filter((fragment) => fragment.status === "approved" && fragment.creative_types.includes(creativeType) && fragment.text.trim());
  const selectedIds = new Set(Object.values(slots).flat());
  const visualOnly = selectedIds.size === 0 && plans.length === 0;
  const recovery = useRef({ key: "", issueId: "", orderId: "" });
  const labels: Record<CreativeCopyFragmentRole, string> = {
    headline: t(($) => $.copyOrder.headline), subheadline: t(($) => $.copyOrder.subheadline), benefit: t(($) => $.copyOrder.benefit),
    supporting: t(($) => $.copyOrder.supporting), cta: t(($) => $.copyOrder.cta), legal: t(($) => $.copyOrder.legal),
  };
  const create = useMutation({
    mutationFn: async () => {
      if (!market || !squad || !files.data) throw new Error(t(($) => $.copyOrder.missingConfiguration));
      const selection = { library_version: library.published_version, creative_type: creativeType, slots, repayment_plan_keys: plans, visual_only: visualOnly, visual_direction: direction };
      const key = await creativeSubmissionKey({ source_kind: "copy_library", library: library.id, market: market.id, marketVersion: market.published_version, squad: squad.id, selection });
      if (recovery.current.key !== key) recovery.current = { key, issueId: "", orderId: "" };
      if (!recovery.current.issueId) {
        const issue = await api.createIssue({ title: `${t(($) => $.copyOrder.title)} · ${library.name}`, status: "todo", allow_duplicate: true, metadata: { workflow: "creative_order", creative_submission_key: key } });
        if (!issue.id) throw new Error(t(($) => $.copyOrder.createFailed));
        recovery.current.issueId = issue.id;
      }
      if (!recovery.current.orderId) {
        const order = await api.createCreativeOrder({
          issue_id: recovery.current.issueId, submission_key: key, status: "queued", trigger_evidence_kind: "copy_library", trigger_evidence_ref_id: library.id,
          input_snapshot: { pipeline_version: "candidate_v1", market_pack: { id: market.id, version: market.published_version, config: market.published_config ?? {}, files: files.data.files }, squad_snapshot: { squad_id: squad.id } },
          items: [{ source_kind: "copy_library", copy_library_id: library.id, candidate_id: "", source_analysis_id: "", copy_snapshot: selection, direction: visualDirectionSummary(direction) }],
        });
        if (!order.id) throw new Error(t(($) => $.copyOrder.createFailed));
        recovery.current.orderId = order.id;
      }
      await api.setIssueMetadataKey(recovery.current.issueId, "creative_order_id", recovery.current.orderId);
      await api.updateIssue(recovery.current.issueId, { assignee_type: "squad", assignee_id: squad.id });
      return recovery.current.orderId;
    },
    onSuccess: (orderId) => { void queryClient.invalidateQueries({ queryKey: creativeKeys.orders(wsId) }); toast.success(t(($) => $.copyOrder.created)); onCreated(orderId); },
  });
  const chooseRecipe = (id: string) => {
    setRecipeId(id);
    const recipe = config.recipes.find((entry) => entry.id === id);
    if (!recipe) { setSlots({}); return; }
    setCreativeType(recipe.creative_type);
    setSlots(Object.fromEntries(COPY_SLOTS.map((role) => [role, (recipe.fragment_ids[role] ?? []).filter((fragmentId) => config.fragments.some((fragment) => fragment.id === fragmentId && fragment.status === "approved" && fragment.creative_types.includes(recipe.creative_type)))])));
  };
  return <Dialog open onOpenChange={(open) => { if (!open && !create.isPending) onClose(); }}>
    <DialogContent className="flex max-h-[94dvh] w-[96vw] !max-w-5xl flex-col gap-0 overflow-hidden p-0" aria-describedby={undefined}>
      <DialogHeader className="border-b px-5 py-4"><DialogTitle>{t(($) => $.copyOrder.title)}</DialogTitle><div className="flex flex-wrap gap-2 pt-1"><Badge variant="outline">{library.name} · v{library.published_version}</Badge>{visualOnly && <Badge variant="secondary">{t(($) => $.copyOrder.visualOnly)}</Badge>}</div></DialogHeader>
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="copy-order-scroll"><fieldset disabled={create.isPending} className="min-w-0 p-5">
        <div className="grid gap-4 sm:grid-cols-3">
          <label className="grid gap-2 text-sm">{t(($) => $.copyOrder.market)}<NativeSelect value={market?.id ?? ""} onChange={(event) => setMarketId(event.target.value)}><NativeSelectOption value="">{t(($) => $.copyOrder.selectMarket)}</NativeSelectOption>{markets.map((entry) => <NativeSelectOption key={entry.id} value={entry.id}>{entry.name}</NativeSelectOption>)}</NativeSelect></label>
          <label className="grid gap-2 text-sm">{t(($) => $.copyOrder.squad)}<NativeSelect value={squad?.id ?? ""} onChange={(event) => setSquadId(event.target.value)}><NativeSelectOption value="">{t(($) => $.copyOrder.selectSquad)}</NativeSelectOption>{squads.data?.map((entry) => <NativeSelectOption key={entry.id} value={entry.id}>{entry.name}</NativeSelectOption>)}</NativeSelect></label>
          <label className="grid gap-2 text-sm">{t(($) => $.copyOrder.type)}<NativeSelect value={creativeType} onChange={(event) => { setCreativeType(event.target.value as CreativeType); setSlots({}); setRecipeId(""); }}><NativeSelectOption value="num">Num</NativeSelectOption><NativeSelectOption value="repayment_plan">Repayment Plan</NativeSelectOption></NativeSelect></label>
        </div>
        {config.recipes.some((recipe) => recipe.status === "approved") && <label className="mt-4 grid gap-2 text-sm">{t(($) => $.copyOrder.recipe)}<NativeSelect value={recipeId} onChange={(event) => chooseRecipe(event.target.value)}><NativeSelectOption value="">{t(($) => $.copyOrder.custom)}</NativeSelectOption>{config.recipes.filter((recipe) => recipe.status === "approved").map((recipe) => <NativeSelectOption key={recipe.id} value={recipe.id}>{recipe.name}</NativeSelectOption>)}</NativeSelect></label>}
        <div className="mt-5 grid min-w-0 gap-6 md:grid-cols-2">
          <div className="min-w-0 divide-y border-y">{COPY_SLOTS.map((role, index) => <details key={role} open={index === 0 || (slots[role]?.length ?? 0) > 0} className="group py-3">
            <summary className="flex cursor-pointer list-none items-center justify-between gap-2 text-sm font-medium">{labels[role]}<span className="flex items-center gap-2 text-xs text-muted-foreground">{slots[role]?.length || t(($) => $.copyOrder.optional)}<ChevronDown className="h-4 w-4 group-open:rotate-180" /></span></summary>
            <div className="mt-3 max-h-52 space-y-2 overflow-y-auto">{available.length === 0 ? <p className="text-xs text-muted-foreground">{t(($) => $.copyOrder.noCopy)}</p> : available.map((fragment) => {
              const checked = slots[role]?.includes(fragment.id) ?? false;
              return <label key={fragment.id} className="flex cursor-pointer items-start gap-2 py-1 text-sm"><input type="checkbox" className="mt-1 h-4 w-4 shrink-0" checked={checked} disabled={!checked && selectedIds.has(fragment.id)} onChange={(event) => { setRecipeId(""); setSlots((current) => ({ ...current, [role]: event.target.checked ? [...(current[role] ?? []), fragment.id] : (current[role] ?? []).filter((id) => id !== fragment.id) })); }} /><span className="min-w-0 break-words">{fragment.text}{fragment.name && <span className="mt-0.5 block text-xs text-muted-foreground">{fragment.name}</span>}</span></label>;
            })}</div>
          </details>)}</div>
          <div className="min-w-0 space-y-5"><section><h3 className="text-sm font-medium">{t(($) => $.copyOrder.selectedCopy)}</h3>{visualOnly ? <p className="mt-3 text-sm text-muted-foreground">{t(($) => $.copyOrder.visualOnly)}</p> : COPY_SLOTS.filter((role) => slots[role]?.length).map((role) => <div key={role} className="mt-3 border-l-2 pl-3"><p className="text-xs text-muted-foreground">{labels[role]}</p>{slots[role]?.map((id) => <p key={id} className="mt-1 whitespace-pre-wrap break-words text-sm">{available.find((fragment) => fragment.id === id)?.text}</p>)}</div>)}</section>
            <section className="border-t pt-4"><h3 className="text-sm font-medium">{t(($) => $.copyOrder.repayment)} <span className="text-xs font-normal text-muted-foreground">{t(($) => $.copyOrder.optional)}</span></h3><div className="mt-3 max-h-52 space-y-3 overflow-y-auto">{config.repayment_plan.entries.filter((entry) => entry.status === "approved").map((entry) => <label key={entry.key} className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1 h-4 w-4 shrink-0" checked={plans.includes(entry.key)} onChange={(event) => setPlans((current) => event.target.checked ? [...current, entry.key] : current.filter((key) => key !== entry.key))} /><span>{config.repayment_plan.labels.principal}: {entry.principal.toLocaleString()} · {config.repayment_plan.labels.tenor}: {entry.tenor_months}<span className="block text-xs text-muted-foreground">{config.repayment_plan.labels.monthly_installment}: {entry.monthly_installment.toLocaleString()} · {config.repayment_plan.labels.total_repayment}: {entry.total_repayment.toLocaleString()}</span></span></label>)}</div></section>
          </div>
        </div>
        <div className="mt-5"><VisualDirectionEditor value={direction} onChange={setDirection} /></div>
      </fieldset></div>
      <div className="flex flex-wrap items-center justify-between gap-3 border-t px-5 py-4">
        <p role={create.error ? "alert" : "status"} className="min-w-0 flex-1 break-words text-sm text-destructive">{create.error ? create.error.message : !market || !squad ? t(($) => $.copyOrder.missingConfiguration) : ""}</p>
        <Button variant="outline" disabled={create.isPending} onClick={onClose}>{t(($) => $.copyOrder.cancel)}</Button>
        <Button disabled={create.isPending || !market || !squad || !files.data || files.isError} onClick={() => create.mutate()}>{create.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Sparkles className="h-4 w-4" />}{create.isPending ? t(($) => $.copyOrder.submitting) : visualOnly ? t(($) => $.copyOrder.startVisualOnly) : t(($) => $.copyOrder.submit)}</Button>
      </div>
    </DialogContent>
  </Dialog>;
}
