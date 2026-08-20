"use client";

import { useEffect, useState } from "react";
import type { ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Archive, CheckCircle2, ChevronDown, Plus, Save, Trash2 } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeCopyContentGroupForFragment, creativeCopyContentGroupLabel, creativeKeys, parseCreativeCopyLibraryConfig } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type { CreativeCopyContentGroup, CreativeCopyFragment, CreativeCopyLibraryConfig, CreativeRepaymentPlanEntry, CreativeResource, CreativeType } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { copyLibraryDraftError } from "../lib/copy-template";

export { copyLibraryDraftError } from "../lib/copy-template";

const COPY_GROUPS: Array<{
  value: CreativeCopyContentGroup;
  description: string;
  role: CreativeCopyFragment["role"];
  types: CreativeType[];
  usage: CreativeCopyFragment["usage"];
}> = [
  { value: "standard_headline", description: "填原图最醒目的开场句或主标题。", role: "headline", types: ["num", "repayment_plan"], usage: "core" },
  { value: "core_benefit", description: "额度、期限、低息等主要信息。每条都是完整可投放文案。", role: "benefit", types: ["num", "repayment_plan"], usage: "core" },
  { value: "other_benefit", description: "只放任何文字空位都成立的审核卖点，如无需抵押、无初始费用；不放品牌、产品条件或金额期限。", role: "supporting", types: ["num", "repayment_plan"], usage: "fallback" },
  { value: "call_to_action", description: "只用于原图明确的下载、申请、领取等行动区。", role: "cta", types: ["num", "repayment_plan"], usage: "fallback" },
  { value: "repayment_headline", description: "原图出现金额卡或还款表时，填在表格上方。", role: "headline", types: ["num", "repayment_plan"], usage: "core" },
];

export function ComposableCopyLibraryEditor({ resources, onCreate, onArchive }: {
  resources: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const [draft, setDraft] = useState<CreativeCopyLibraryConfig>(() => parseCreativeCopyLibraryConfig(active?.config ?? {}));
  useEffect(() => { setDraft(parseCreativeCopyLibraryConfig(active?.config ?? {})); }, [active?.id, active?.version]);

  const save = useMutation({
    mutationFn: () => {
      if (!active) throw new Error("请选择文案库");
      return api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft as unknown as Record<string, unknown> });
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success("文案库草稿已保存"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "文案库保存失败"),
  });
  const publish = useMutation({
    mutationFn: async () => {
      if (!active) throw new Error("请选择文案库");
      const validationError = copyLibraryDraftError(draft);
      if (validationError) throw new Error(validationError);
      const saved = await api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft as unknown as Record<string, unknown> });
      return api.publishCreativeResource(saved.id);
    },
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success("新版本已发布，只用于后续素材"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "文案库发布失败"),
  });

  if (!active) return <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><p>还没有文案库</p><Button size="sm" onClick={onCreate}><Plus className="h-4 w-4" />创建文案库</Button></div>;

  return <div className="grid h-full min-h-0 w-full min-w-0 grid-cols-1 overflow-hidden border md:grid-cols-[240px_minmax(0,1fr)]">
    <aside className="min-h-0 min-w-0 overflow-y-auto border-r bg-muted/10 p-2">
      <div className="flex items-center justify-between px-2 py-2"><span className="text-xs font-semibold">文案库</span><Button size="icon-sm" variant="ghost" title="创建文案库" aria-label="创建文案库" onClick={onCreate}><Plus className="h-4 w-4" /></Button></div>
      {resources.map((resource) => <button key={resource.id} type="button" onClick={() => setActiveId(resource.id)} className={`w-full border-l-2 px-3 py-3 text-left ${resource.id === active.id ? "border-foreground bg-background" : "border-transparent text-muted-foreground hover:bg-muted"}`}><span className="block truncate text-sm font-medium">{resource.name}</span><span className="mt-1 block text-[11px]">{resource.status === "published" ? "已发布" : "草稿"}</span></button>)}
    </aside>
    <main className="min-w-0 overflow-y-auto bg-background">
      <header className="sticky top-0 z-10 flex min-w-0 flex-wrap items-center justify-between gap-3 border-b bg-background px-5 py-4">
        <div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h2 className="min-w-0 truncate text-sm font-semibold">{active.name}</h2><Badge variant="outline">{active.status === "published" ? "已发布" : "草稿"}</Badge>{active.published_version > 0 && active.status !== "published" && <Badge variant="secondary">线上已发布</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">模型只会使用这里已审核的文案和还款计划，不会拼变量或计算金额。</p></div>
        <div className="flex flex-wrap items-center gap-2"><Button size="sm" variant="outline" disabled={save.isPending || publish.isPending} onClick={() => save.mutate()}><Save className="h-4 w-4" />保存草稿</Button><Button size="sm" disabled={save.isPending || publish.isPending} onClick={() => publish.mutate()}><CheckCircle2 className="h-4 w-4" />发布版本</Button><Button size="icon-sm" variant="ghost" title="归档文案库" aria-label="归档文案库" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button></div>
      </header>
      <Tabs defaultValue="copy" className="px-5 py-4">
        <TabsList><TabsTrigger value="copy">投放文案 {draft.fragments.length}</TabsTrigger><TabsTrigger value="repayment">还款计划 {draft.repayment_plan.entries.length}</TabsTrigger></TabsList>
        <TabsContent value="copy"><CopyGroupEditor value={draft.fragments} onChange={(fragments) => setDraft((current) => ({ ...current, fragments }))} /></TabsContent>
        <TabsContent value="repayment"><RepaymentPlanEditor value={draft.repayment_plan} onChange={(repayment_plan) => setDraft((current) => ({ ...current, repayment_plan }))} /></TabsContent>
      </Tabs>
    </main>
  </div>;
}

function CopyGroupEditor({ value, onChange }: { value: CreativeCopyFragment[]; onChange: (value: CreativeCopyFragment[]) => void }) {
  return <div className="space-y-3 pt-4">{COPY_GROUPS.map((group, index) => {
    const entries = value.filter((item) => creativeCopyContentGroupForFragment(item) === group.value);
    const add = () => onChange([...value, { id: crypto.randomUUID(), key: `copy-${group.value}-${entries.length + 1}`, name: `新${creativeCopyContentGroupLabel(group.value)}`, content_group: group.value, creative_types: group.types, role: group.role, semantic_group: "", text: "", tags: [], usage: group.usage, status: "draft" }]);
    return <details key={group.value} open={index === 0} className="group border"><summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-3 marker:content-none"><div className="min-w-0"><h3 id={`copy-group-${group.value}`} className="text-sm font-semibold">{creativeCopyContentGroupLabel(group.value)} <span className="font-normal text-muted-foreground">{entries.length}</span></h3><p className="mt-1 truncate text-xs text-muted-foreground">{group.description}</p></div><ChevronDown className="h-4 w-4 shrink-0 transition-transform group-open:rotate-180" /></summary><div className="border-t px-4"><div className="flex justify-end py-3"><Button size="sm" variant="outline" onClick={add}><Plus className="h-4 w-4" />添加</Button></div>{entries.length === 0 ? <p className="border-t py-4 text-sm text-muted-foreground">暂未维护文案。</p> : <div className="divide-y border-t">{entries.map((entry) => <CopyEntryEditor key={entry.id} entry={entry} onChange={(next) => onChange(value.map((item) => item.id === entry.id ? next : item))} onDelete={() => onChange(value.filter((item) => item.id !== entry.id))} />)}</div>}</div></details>;
  })}</div>;
}

function CopyEntryEditor({ entry, onChange, onDelete }: { entry: CreativeCopyFragment; onChange: (entry: CreativeCopyFragment) => void; onDelete: () => void }) {
  return <div className="grid min-w-0 gap-3 py-4 lg:grid-cols-[minmax(0,1fr)_minmax(180px,240px)_130px_36px]"><Labeled label="投放文案"><Textarea rows={2} value={entry.text} onChange={(event) => onChange({ ...entry, text: event.target.value })} placeholder="填写当前市场已审核的投放文案" /></Labeled><Labeled label="中文释义"><Input value={entry.name} onChange={(event) => onChange({ ...entry, name: event.target.value })} placeholder="准确填写左侧投放文案的中文含义" /></Labeled><Labeled label="审核状态"><StatusSelect value={entry.status} onChange={(status) => onChange({ ...entry, status })} /></Labeled><div className="flex items-end justify-end"><DeleteButton onClick={onDelete} label="删除文案" /></div></div>;
}

function RepaymentPlanEditor({ value, onChange }: { value: CreativeCopyLibraryConfig["repayment_plan"]; onChange: (value: CreativeCopyLibraryConfig["repayment_plan"]) => void }) {
  const entries = [...value.entries].sort((left, right) => left.principal - right.principal || left.tenor_months - right.tenor_months);
  const add = () => onChange({ ...value, entries: [...value.entries, { id: crypto.randomUUID(), key: `plan-${value.entries.length + 1}`, principal: 0, tenor_months: 0, monthly_installment: 0, total_interest: 0, total_repayment: 0, source: "业务审核计划表", status: "draft" }] });
  const update = (entry: CreativeRepaymentPlanEntry) => onChange({ ...value, entries: value.entries.map((item) => item.id === entry.id ? entry : item) });
  return <section className="mt-4"><div className="flex flex-wrap items-start justify-between gap-3 border-b pb-3"><div><h3 className="text-sm font-semibold">还款计划表</h3><p className="mt-1 text-xs text-muted-foreground">AI 只能从已审核行选择借款金额和期限，并将该行的月供结果填入原图对应位置。</p></div><Button size="sm" variant="outline" onClick={add}><Plus className="h-4 w-4" />添加计划</Button></div><div className="grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-5"><Labeled label="借款金额"><Input value={value.labels.principal} onChange={(event) => onChange({ ...value, labels: { ...value.labels, principal: event.target.value } })} /></Labeled><Labeled label="期限"><Input value={value.labels.tenor} onChange={(event) => onChange({ ...value, labels: { ...value.labels, tenor: event.target.value } })} /></Labeled><Labeled label="每月还款"><Input value={value.labels.monthly_installment} onChange={(event) => onChange({ ...value, labels: { ...value.labels, monthly_installment: event.target.value } })} /></Labeled><Labeled label="总利息"><Input value={value.labels.total_interest} onChange={(event) => onChange({ ...value, labels: { ...value.labels, total_interest: event.target.value } })} /></Labeled><Labeled label="总还款"><Input value={value.labels.total_repayment} onChange={(event) => onChange({ ...value, labels: { ...value.labels, total_repayment: event.target.value } })} /></Labeled></div><div className="divide-y border-t">{entries.map((entry) => <RepaymentPlanRow key={entry.id} entry={entry} onChange={update} onDelete={() => onChange({ ...value, entries: value.entries.filter((item) => item.id !== entry.id) })} />)}</div></section>;
}

function RepaymentPlanRow({ entry, onChange, onDelete }: { entry: CreativeRepaymentPlanEntry; onChange: (entry: CreativeRepaymentPlanEntry) => void; onDelete: () => void }) {
  const updateNumber = (field: "principal" | "tenor_months" | "monthly_installment" | "total_interest" | "total_repayment", raw: string) => {
    const next = Number(raw);
    const updated = { ...entry, [field]: Number.isFinite(next) ? Math.round(next) : 0 };
    if (field === "principal" || field === "tenor_months") updated.key = `plan-${updated.principal}-${updated.tenor_months}`;
    onChange(updated);
  };
  return <div className="grid gap-3 py-4 sm:grid-cols-2 xl:grid-cols-[minmax(120px,1fr)_120px_minmax(130px,1fr)_minmax(130px,1fr)_minmax(130px,1fr)_minmax(180px,1fr)_130px_36px]"><Labeled label="借款金额 (Rp)"><Input inputMode="numeric" value={entry.principal || ""} onChange={(event) => updateNumber("principal", event.target.value)} /></Labeled><Labeled label="期限（月）"><Input inputMode="numeric" value={entry.tenor_months || ""} onChange={(event) => updateNumber("tenor_months", event.target.value)} /></Labeled><Labeled label="每月还款 (Rp)"><Input inputMode="numeric" value={entry.monthly_installment || ""} onChange={(event) => updateNumber("monthly_installment", event.target.value)} /></Labeled><Labeled label="总利息 (Rp)"><Input inputMode="numeric" value={entry.total_interest || ""} onChange={(event) => updateNumber("total_interest", event.target.value)} /></Labeled><Labeled label="总还款 (Rp)"><Input inputMode="numeric" value={entry.total_repayment || ""} onChange={(event) => updateNumber("total_repayment", event.target.value)} /></Labeled><Labeled label="来源 / 依据"><Input value={entry.source} onChange={(event) => onChange({ ...entry, source: event.target.value })} /></Labeled><Labeled label="状态"><StatusSelect value={entry.status} onChange={(status) => onChange({ ...entry, status })} /></Labeled><DeleteButton onClick={onDelete} label="删除计划" /></div>;
}

function Labeled({ label, children }: { label: string; children: ReactNode }) { return <label className="grid content-start gap-1.5 text-xs font-medium text-muted-foreground"><span>{label}</span>{children}</label>; }
function DeleteButton({ onClick, label }: { onClick: () => void; label: string }) { return <Button size="icon-sm" variant="ghost" className="self-end" title={label} aria-label={label} onClick={onClick}><Trash2 className="h-4 w-4" /></Button>; }
function StatusSelect({ value, onChange }: { value: "draft" | "approved" | "disabled"; onChange: (value: "draft" | "approved" | "disabled") => void }) { return <NativeSelect value={value} onChange={(event) => onChange(event.target.value as typeof value)}><NativeSelectOption value="draft">草稿</NativeSelectOption><NativeSelectOption value="approved">已审核</NativeSelectOption><NativeSelectOption value="disabled">停用</NativeSelectOption></NativeSelect>; }
