"use client";

import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Archive, CheckCircle2, ChevronDown, Plus, Save, Trash2 } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeCopyContentGroupForFragment, creativeKeys, parseCreativeCopyLibraryConfig } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type { CreativeCopyContentGroup, CreativeCopyFragment, CreativeCopyLibraryConfig, CreativeRepaymentPlanEntry, CreativeResource, CreativeType } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useT } from "../../i18n";
import { copyLibraryDraftError } from "../lib/copy-template";
import { CopyLibraryOrderDialog } from "./copy-library-order-dialog";

export { copyLibraryDraftError } from "../lib/copy-template";

type CreativeT = ReturnType<typeof useT<"creative">>["t"];

const COPY_GROUPS: Array<{
  value: CreativeCopyContentGroup;
  role: CreativeCopyFragment["role"];
  types: CreativeType[];
  usage: CreativeCopyFragment["usage"];
}> = [
  { value: "standard_headline", role: "headline", types: ["num", "repayment_plan"], usage: "core" },
  { value: "core_benefit", role: "benefit", types: ["num", "repayment_plan"], usage: "core" },
  { value: "other_benefit", role: "supporting", types: ["num", "repayment_plan"], usage: "fallback" },
  { value: "call_to_action", role: "cta", types: ["num", "repayment_plan"], usage: "fallback" },
  { value: "repayment_headline", role: "headline", types: ["num", "repayment_plan"], usage: "core" },
];

export function ComposableCopyLibraryEditor({ resources, onCreate, onArchive, onOrderCreated }: {
  resources: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
  onOrderCreated?: (orderId: string) => void;
}) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const [orderOpen, setOrderOpen] = useState(false);
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const [draft, setDraft] = useState<CreativeCopyLibraryConfig>(() => parseCreativeCopyLibraryConfig(active?.config ?? {}));
  useEffect(() => { setDraft(parseCreativeCopyLibraryConfig(active?.config ?? {})); }, [active?.id, active?.version]);
  const publishError = copyLibraryDraftError(draft);
  const hasUnsavedChanges = Boolean(active && JSON.stringify(draft) !== JSON.stringify(parseCreativeCopyLibraryConfig(active.config ?? {})));
  const hasUnpublishedDraft = Boolean(active && active.version !== active.published_version);
  const hasPendingRelease = hasUnsavedChanges || hasUnpublishedDraft;
  const releaseVersion = active ? active.version + (hasUnsavedChanges ? 1 : 0) : 0;

  const save = useMutation({
    mutationFn: () => {
      if (!active) throw new Error(t(($) => $.copyLibrary.noLibrary));
      return api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft as unknown as Record<string, unknown> });
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success(t(($) => $.copyLibrary.saved)); },
    onError: (error) => toast.error(error instanceof Error ? error.message : t(($) => $.copyLibrary.saveFailed)),
  });
  const publish = useMutation({
    mutationFn: async () => {
      if (!active) throw new Error(t(($) => $.copyLibrary.noLibrary));
      const validationError = copyLibraryDraftError(draft);
      if (validationError) throw new Error(validationError);
      const saved = await api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft as unknown as Record<string, unknown> });
      return api.publishCreativeResource(saved.id);
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success(t(($) => $.copyLibrary.published)); },
    onError: (error) => toast.error(error instanceof Error ? error.message : t(($) => $.copyLibrary.publishFailed)),
  });

  if (!active) return <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><p>{t(($) => $.copyLibrary.noLibrary)}</p><Button size="sm" onClick={onCreate}><Plus className="h-4 w-4" />{t(($) => $.copyLibrary.create)}</Button></div>;

  return <div className="grid h-full min-h-0 w-full min-w-0 grid-cols-1 overflow-hidden border md:grid-cols-[240px_minmax(0,1fr)]">
    <aside className="min-h-0 min-w-0 overflow-y-auto border-r bg-muted/10 p-2">
      <div className="flex items-center justify-between px-2 py-2"><span className="text-xs font-semibold">{t(($) => $.copyLibrary.title)}</span><Button size="icon-sm" variant="ghost" title={t(($) => $.copyLibrary.create)} aria-label={t(($) => $.copyLibrary.create)} onClick={onCreate}><Plus className="h-4 w-4" /></Button></div>
      {resources.map((resource) => <button key={resource.id} type="button" onClick={() => setActiveId(resource.id)} className={`w-full border-l-2 px-3 py-3 text-left ${resource.id === active.id ? "border-foreground bg-background" : "border-transparent text-muted-foreground hover:bg-muted"}`}><span className="block truncate text-sm font-medium">{resource.name}</span><span className="mt-1 block text-[11px]">{copyLibraryReleaseLabel(resource, t)}</span></button>)}
    </aside>
    <main className="min-w-0 overflow-y-auto bg-background">
      <header className="sticky top-0 z-10 flex min-w-0 flex-wrap items-center justify-between gap-3 border-b bg-background px-5 py-4">
        <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h2 className="min-w-0 truncate text-sm font-semibold">{active.name}</h2><Badge variant="secondary">{t(($) => $.copyLibrary.editDraft, { version: active.version })}</Badge>{hasUnsavedChanges && <Badge variant="outline">{t(($) => $.copyLibrary.unsavedEdits)}</Badge>}{active.published_version > 0 ? <Badge variant="outline">{t(($) => $.copyLibrary.activeProduction, { version: active.published_version })}</Badge> : <Badge variant="outline">{t(($) => $.copyLibrary.notPublished)}</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">{hasUnsavedChanges ? t(($) => $.copyLibrary.currentUnsaved, { version: releaseVersion }) : hasUnpublishedDraft ? t(($) => $.copyLibrary.unpublishedDraft, { draftVersion: active.version, productionVersion: active.published_version }) : t(($) => $.copyLibrary.productionUsing, { version: active.published_version })}</p></div>
        <div className="flex flex-wrap items-center gap-2"><Button size="sm" variant="outline" disabled={save.isPending || publish.isPending || !hasUnsavedChanges} onClick={() => save.mutate()}><Save className="h-4 w-4" />{t(($) => $.copyLibrary.saveDraft)}</Button><Button size="sm" disabled={save.isPending || publish.isPending || !hasPendingRelease || Boolean(publishError)} onClick={() => publish.mutate()}><CheckCircle2 className="h-4 w-4" />{hasPendingRelease ? t(($) => $.copyLibrary.publishToProduction, { version: releaseVersion }) : t(($) => $.copyLibrary.productionActive, { version: active.published_version })}</Button><Button size="icon-sm" variant="ghost" title={t(($) => $.copyLibrary.archive)} aria-label={t(($) => $.copyLibrary.archive)} onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button></div>
      </header>
      <div className={`border-b px-5 py-2 text-xs ${publishError ? "border-amber-300 bg-amber-50 text-amber-900" : "bg-muted/20 text-muted-foreground"}`} role={publishError ? "alert" : "status"}>{publishError ? t(($) => $.copyLibrary.validation, { error: publishError }) : t(($) => $.copyLibrary.validationPassed)}</div>
      {onOrderCreated && active.published_version > 0 && <div className="flex justify-end border-b px-5 py-3"><Button size="sm" onClick={() => setOrderOpen(true)}><Plus className="h-4 w-4" />{t(($) => $.copyOrder.title)}</Button></div>}
      {orderOpen && onOrderCreated && <CopyLibraryOrderDialog key={`${active.id}:${active.published_version}`} library={active} onClose={() => setOrderOpen(false)} onCreated={(orderId) => { setOrderOpen(false); onOrderCreated(orderId); }} />}
      <Tabs defaultValue="copy" className="px-5 py-4">
        <TabsList><TabsTrigger value="copy">{t(($) => $.copyLibrary.advertisingCopy, { count: draft.fragments.length })}</TabsTrigger><TabsTrigger value="repayment">{t(($) => $.copyLibrary.repaymentPlans, { count: draft.repayment_plan.entries.length })}</TabsTrigger></TabsList>
        <TabsContent value="copy"><CopyGroupEditor t={t} value={draft.fragments} onChange={(fragments) => setDraft((current) => ({ ...current, fragments }))} /></TabsContent>
        <TabsContent value="repayment"><RepaymentPlanEditor t={t} value={draft.repayment_plan} onChange={(repayment_plan) => setDraft((current) => ({ ...current, repayment_plan }))} /></TabsContent>
      </Tabs>
    </main>
  </div>;
}

function copyLibraryReleaseLabel(resource: CreativeResource, t: CreativeT): string {
  if (resource.published_version <= 0) return t(($) => $.copyLibrary.draftUnpublished, { version: resource.version });
  if (resource.version !== resource.published_version) return t(($) => $.copyLibrary.draftProduction, { draftVersion: resource.version, productionVersion: resource.published_version });
  return t(($) => $.copyLibrary.productionActiveVersion, { version: resource.published_version });
}

function CopyGroupEditor({ t, value, onChange }: { t: CreativeT; value: CreativeCopyFragment[]; onChange: (value: CreativeCopyFragment[]) => void }) {
  return <div className="space-y-3 pt-4">{COPY_GROUPS.map((group, index) => {
    const entries = value.filter((item) => creativeCopyContentGroupForFragment(item) === group.value);
    const label = copyGroupLabel(t, group.value);
    const add = () => onChange([...value, { id: crypto.randomUUID(), key: `copy-${group.value}-${entries.length + 1}`, name: t(($) => $.copyLibrary.newEntry, { group: label }), content_group: group.value, creative_types: group.types, role: group.role, semantic_group: "", text: "", tags: [], usage: group.usage, status: "draft" }]);
    return <details key={group.value} open={index === 0} className="group border"><summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 marker:content-none"><div className="min-w-0"><h3 id={`copy-group-${group.value}`} className="text-sm font-semibold">{label} <span className="font-normal text-muted-foreground">{entries.length}</span></h3><p className="mt-1 truncate text-xs text-muted-foreground">{copyGroupDescription(t, group.value)}</p></div><ChevronDown className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180" /></summary><div className="border-t px-4"><div className="flex justify-end py-3"><Button size="sm" variant="outline" onClick={add}><Plus className="h-4 w-4" />{t(($) => $.copyLibrary.add)}</Button></div>{entries.length === 0 ? <p className="border-t py-4 text-sm text-muted-foreground">{t(($) => $.copyLibrary.noCopy)}</p> : <div className="divide-y border-t">{entries.map((entry) => <CopyEntryEditor t={t} key={entry.id} entry={entry} onChange={(next) => onChange(value.map((item) => item.id === entry.id ? next : item))} onDelete={() => onChange(value.filter((item) => item.id !== entry.id))} />)}</div>}</div></details>;
  })}</div>;
}

function CopyEntryEditor({ t, entry, onChange, onDelete }: { t: CreativeT; entry: CreativeCopyFragment; onChange: (entry: CreativeCopyFragment) => void; onDelete: () => void }) {
  return <div className="grid min-w-0 gap-3 py-4 lg:grid-cols-[minmax(0,1fr)_minmax(180px,240px)_130px_36px]"><Labeled label={t(($) => $.copyLibrary.copy)}><Textarea rows={2} value={entry.text} onChange={(event) => onChange({ ...entry, text: event.target.value })} placeholder={t(($) => $.copyLibrary.copyPlaceholder)} /></Labeled><Labeled label={t(($) => $.copyLibrary.chineseMeaning)}><Input value={entry.name} onChange={(event) => onChange({ ...entry, name: event.target.value })} placeholder={t(($) => $.copyLibrary.meaningPlaceholder)} /></Labeled><Labeled label={t(($) => $.copyLibrary.reviewStatus)}><StatusSelect t={t} value={entry.status} onChange={(status) => onChange({ ...entry, status })} /></Labeled><div className="flex items-end justify-end"><DeleteButton onClick={onDelete} label={t(($) => $.copyLibrary.deleteCopy)} /></div></div>;
}

function RepaymentPlanEditor({ t, value, onChange }: { t: CreativeT; value: CreativeCopyLibraryConfig["repayment_plan"]; onChange: (value: CreativeCopyLibraryConfig["repayment_plan"]) => void }) {
  const entries = [...value.entries].sort((left, right) => left.principal - right.principal || left.tenor_months - right.tenor_months);
  const add = () => onChange({ ...value, entries: [...value.entries, { id: crypto.randomUUID(), key: `plan-${value.entries.length + 1}`, principal: 0, tenor_months: 0, monthly_installment: 0, total_interest: 0, total_repayment: 0, source: t(($) => $.copyLibrary.repaymentTable), status: "draft" }] });
  const update = (entry: CreativeRepaymentPlanEntry) => onChange({ ...value, entries: value.entries.map((item) => item.id === entry.id ? entry : item) });
  return <section className="mt-4"><div className="flex flex-wrap items-start justify-between gap-3 border-b pb-3"><div><h3 className="text-sm font-semibold">{t(($) => $.copyLibrary.repaymentTable)}</h3><p className="mt-1 text-xs text-muted-foreground">{t(($) => $.copyLibrary.repaymentDescription)}</p></div><Button size="sm" variant="outline" onClick={add}><Plus className="h-4 w-4" />{t(($) => $.copyLibrary.addPlan)}</Button></div><div className="grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-5"><Labeled label={t(($) => $.copyLibrary.principal)}><Input value={value.labels.principal} onChange={(event) => onChange({ ...value, labels: { ...value.labels, principal: event.target.value } })} /></Labeled><Labeled label={t(($) => $.copyLibrary.tenor)}><Input value={value.labels.tenor} onChange={(event) => onChange({ ...value, labels: { ...value.labels, tenor: event.target.value } })} /></Labeled><Labeled label={t(($) => $.copyLibrary.monthlyInstallment)}><Input value={value.labels.monthly_installment} onChange={(event) => onChange({ ...value, labels: { ...value.labels, monthly_installment: event.target.value } })} /></Labeled><Labeled label={t(($) => $.copyLibrary.totalInterest)}><Input value={value.labels.total_interest} onChange={(event) => onChange({ ...value, labels: { ...value.labels, total_interest: event.target.value } })} /></Labeled><Labeled label={t(($) => $.copyLibrary.totalRepayment)}><Input value={value.labels.total_repayment} onChange={(event) => onChange({ ...value, labels: { ...value.labels, total_repayment: event.target.value } })} /></Labeled></div><div className="divide-y border-t">{entries.map((entry) => <RepaymentPlanRow t={t} key={entry.id} entry={entry} onChange={update} onDelete={() => onChange({ ...value, entries: value.entries.filter((item) => item.id !== entry.id) })} />)}</div></section>;
}

function RepaymentPlanRow({ t, entry, onChange, onDelete }: { t: CreativeT; entry: CreativeRepaymentPlanEntry; onChange: (entry: CreativeRepaymentPlanEntry) => void; onDelete: () => void }) {
  const updateNumber = (field: "principal" | "tenor_months" | "monthly_installment" | "total_interest" | "total_repayment", raw: string) => {
    const next = Number(raw);
    const updated = { ...entry, [field]: Number.isFinite(next) ? Math.round(next) : 0 };
    if (field === "principal" || field === "tenor_months") updated.key = `plan-${updated.principal}-${updated.tenor_months}`;
    onChange(updated);
  };
  return <div className="grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-[minmax(120px,1fr)_120px_minmax(130px,1fr)_minmax(130px,1fr)_minmax(130px,1fr)_minmax(180px,1fr)_130px_36px]"><Labeled label={t(($) => $.copyLibrary.principalRp)}><Input inputMode="numeric" value={entry.principal || ""} onChange={(event) => updateNumber("principal", event.target.value)} /></Labeled><Labeled label={t(($) => $.copyLibrary.tenorMonths)}><Input inputMode="numeric" value={entry.tenor_months || ""} onChange={(event) => updateNumber("tenor_months", event.target.value)} /></Labeled><Labeled label={t(($) => $.copyLibrary.monthlyInstallmentRp)}><Input inputMode="numeric" value={entry.monthly_installment || ""} onChange={(event) => updateNumber("monthly_installment", event.target.value)} /></Labeled><Labeled label={t(($) => $.copyLibrary.totalInterestRp)}><Input inputMode="numeric" value={entry.total_interest || ""} onChange={(event) => updateNumber("total_interest", event.target.value)} /></Labeled><Labeled label={t(($) => $.copyLibrary.totalRepaymentRp)}><Input inputMode="numeric" value={entry.total_repayment || ""} onChange={(event) => updateNumber("total_repayment", event.target.value)} /></Labeled><Labeled label={t(($) => $.copyLibrary.source)}><Input value={entry.source} onChange={(event) => onChange({ ...entry, source: event.target.value })} /></Labeled><Labeled label={t(($) => $.copyLibrary.status)}><StatusSelect t={t} value={entry.status} onChange={(status) => onChange({ ...entry, status })} /></Labeled><DeleteButton onClick={onDelete} label={t(($) => $.copyLibrary.deletePlan)} /></div>;
}

function Labeled({ label, children }: { label: string; children: ReactNode }) { return <label className="grid content-start gap-1.5 text-xs font-medium text-muted-foreground"><span>{label}</span>{children}</label>; }
function DeleteButton({ onClick, label }: { onClick: () => void; label: string }) { return <Button size="icon-sm" variant="ghost" className="self-end" title={label} aria-label={label} onClick={onClick}><Trash2 className="h-4 w-4" /></Button>; }
function StatusSelect({ t, value, onChange }: { t: CreativeT; value: "draft" | "approved" | "disabled"; onChange: (value: "draft" | "approved" | "disabled") => void }) { return <NativeSelect value={value} onChange={(event) => onChange(event.target.value as typeof value)}><NativeSelectOption value="draft">{t(($) => $.copyLibrary.draft)}</NativeSelectOption><NativeSelectOption value="approved">{t(($) => $.copyLibrary.approved)}</NativeSelectOption><NativeSelectOption value="disabled">{t(($) => $.copyLibrary.disabled)}</NativeSelectOption></NativeSelect>; }

function copyGroupLabel(t: CreativeT, group: CreativeCopyContentGroup): string {
  if (group === "standard_headline") return t(($) => $.copyLibrary.groups.standardHeadline.label);
  if (group === "core_benefit") return t(($) => $.copyLibrary.groups.coreBenefit.label);
  if (group === "other_benefit") return t(($) => $.copyLibrary.groups.otherBenefit.label);
  if (group === "call_to_action") return t(($) => $.copyLibrary.groups.callToAction.label);
  return t(($) => $.copyLibrary.groups.repaymentHeadline.label);
}

function copyGroupDescription(t: CreativeT, group: CreativeCopyContentGroup): string {
  if (group === "standard_headline") return t(($) => $.copyLibrary.groups.standardHeadline.description);
  if (group === "core_benefit") return t(($) => $.copyLibrary.groups.coreBenefit.description);
  if (group === "other_benefit") return t(($) => $.copyLibrary.groups.otherBenefit.description);
  if (group === "call_to_action") return t(($) => $.copyLibrary.groups.callToAction.description);
  return t(($) => $.copyLibrary.groups.repaymentHeadline.description);
}
