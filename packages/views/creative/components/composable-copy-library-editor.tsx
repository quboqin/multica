"use client";

import { useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Archive, CheckCircle2, Download, Plus, Save, Trash2, Upload, Variable } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, parseCreativeCopyLibraryConfig } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type {
  CreativeCopyCalculationRule,
  CreativeCopyFragment,
  CreativeCopyFragmentRole,
  CreativeCopyLibraryConfig,
  CreativeProductFact,
  CreativeResource,
  CreativeType,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { downloadCopyLibraryWorkbook, readCopyLibraryWorkbook, type CopyLibraryImportPreview } from "../lib/copy-library-xlsx";
import { appendCopyFactToken, copyLibraryDraftError, resolveCopyTemplateForBusiness } from "../lib/copy-template";

export { copyLibraryDraftError } from "../lib/copy-template";

const ROLES: Array<{ value: CreativeCopyFragmentRole; label: string }> = [
  { value: "headline", label: "主标题" }, { value: "subheadline", label: "副标题" },
  { value: "benefit", label: "利益点" }, { value: "supporting", label: "补充信息" },
  { value: "cta", label: "行动按钮" }, { value: "legal", label: "合规文字" },
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
  const fileRef = useRef<HTMLInputElement>(null);
  const [importPreview, setImportPreview] = useState<CopyLibraryImportPreview | null>(null);
  useEffect(() => {
    setDraft(parseCreativeCopyLibraryConfig(active?.config ?? {}));
    setImportPreview(null);
  }, [active?.id, active?.version]);

  const importWorkbook = async (file?: File) => {
    if (!file) return;
    try {
      setImportPreview(await readCopyLibraryWorkbook(file));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法读取文案库工作簿");
    }
  };

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
    onSuccess: () => { void queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success("新文案版本已发布，只影响后续订单"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "文案库发布失败"),
  });

  if (!active) return <div className="flex h-full flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><p>还没有文案库</p><Button size="sm" onClick={onCreate}><Plus className="h-4 w-4" />创建文案库</Button></div>;

  return <div className="grid h-full min-h-0 grid-cols-1 overflow-hidden border md:grid-cols-[240px_minmax(0,1fr)]">
    <aside className="min-h-0 overflow-y-auto border-r bg-muted/10 p-2">
      <div className="flex items-center justify-between px-2 py-2"><span className="text-xs font-semibold">文案库</span><Button size="icon-sm" variant="ghost" title="创建文案库" aria-label="创建文案库" onClick={onCreate}><Plus className="h-4 w-4" /></Button></div>
      {resources.map((resource) => <button key={resource.id} type="button" onClick={() => setActiveId(resource.id)} className={`w-full border-l-2 px-3 py-3 text-left ${resource.id === active.id ? "border-foreground bg-background" : "border-transparent text-muted-foreground hover:bg-muted"}`}><span className="block truncate text-sm font-medium">{resource.name}</span><span className="mt-1 block text-[11px]">v{resource.version} · {resource.status === "published" ? "已发布" : "草稿"}</span></button>)}
    </aside>
    <main className="min-w-0 overflow-y-auto bg-background">
      <header className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 border-b bg-background px-5 py-4">
        <div><div className="flex items-center gap-2"><h2 className="text-sm font-semibold">{active.name}</h2><Badge variant="outline">草稿 v{active.version}</Badge>{active.published_version > 0 && <Badge variant="secondary">线上 v{active.published_version}</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">维护文案原子、已审核产品事实和通用兜底内容；系统会按素材语义动态组合</p></div>
        <div className="flex flex-wrap items-center gap-2"><input ref={fileRef} type="file" accept=".xlsx" className="hidden" onChange={(event) => { void importWorkbook(event.target.files?.[0]); event.currentTarget.value = ""; }} /><Button size="sm" variant="outline" onClick={() => fileRef.current?.click()}><Upload className="h-4 w-4" />导入 Excel</Button><Button size="sm" variant="outline" onClick={() => downloadCopyLibraryWorkbook(draft, active.name)}><Download className="h-4 w-4" />导出模板</Button><Button size="sm" variant="outline" disabled={save.isPending || publish.isPending} onClick={() => save.mutate()}><Save className="h-4 w-4" />保存草稿</Button><Button size="sm" disabled={save.isPending || publish.isPending} onClick={() => publish.mutate()}><CheckCircle2 className="h-4 w-4" />发布版本</Button><Button size="icon-sm" variant="ghost" title="归档文案库" aria-label="归档文案库" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button></div>
      </header>
      <SourceStatus config={draft} />
      {importPreview && <CopyLibraryImportReview preview={importPreview} onDiscard={() => setImportPreview(null)} onApply={() => { setDraft(importPreview.config); setImportPreview(null); toast.success("工作簿内容已应用到草稿，请核对后保存或发布"); }} />}
      <Tabs defaultValue="fragments" className="px-5 py-4">
        <TabsList><TabsTrigger value="fragments">文案原子 {draft.fragments.length}</TabsTrigger><TabsTrigger value="facts">产品事实 {draft.product_facts.length}</TabsTrigger><TabsTrigger value="rules">计算规则 {draft.calculation_rules.length}</TabsTrigger></TabsList>
        <TabsContent value="fragments"><FragmentEditor value={draft.fragments} facts={draft.product_facts} onChange={(fragments) => setDraft((current) => ({ ...current, fragments }))} /></TabsContent>
        <TabsContent value="facts"><FactEditor value={draft.product_facts} onChange={(product_facts) => setDraft((current) => ({ ...current, product_facts }))} /></TabsContent>
        <TabsContent value="rules"><RuleEditor value={draft.calculation_rules} onChange={(calculation_rules) => setDraft((current) => ({ ...current, calculation_rules }))} /></TabsContent>
      </Tabs>
    </main>
  </div>;
}

function CopyLibraryImportReview({ preview, onDiscard, onApply }: { preview: CopyLibraryImportPreview; onDiscard: () => void; onApply: () => void }) {
  const hasErrors = preview.errors.length > 0;
  return <section aria-label="Excel 导入预览" className="border-b bg-muted/10 px-5 py-4">
    <div className="flex flex-wrap items-start justify-between gap-3"><div><div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-semibold">导入预览</h3><Badge variant={hasErrors ? "destructive" : "secondary"}>{hasErrors ? `${preview.errors.length} 个待修正项` : "可应用到草稿"}</Badge></div><p className="mt-1 text-xs text-muted-foreground">{preview.sourceName} · 产品事实 {preview.counts.productFacts} · 文案原子 {preview.counts.fragments} · 规则 {preview.counts.calculationRules}</p></div><div className="flex items-center gap-2"><Button size="sm" variant="ghost" onClick={onDiscard}>取消</Button><Button size="sm" disabled={hasErrors} onClick={onApply}>应用到草稿</Button></div></div>
    {hasErrors && <ul className="mt-3 space-y-1 border-l-2 border-destructive pl-3 text-xs text-destructive">{preview.errors.map((error) => <li key={error}>{error}</li>)}</ul>}
    <ul className="mt-3 space-y-1 text-xs text-muted-foreground">{preview.warnings.map((warning) => <li key={warning}>{warning}</li>)}</ul>
  </section>;
}

function SourceStatus({ config }: { config: CreativeCopyLibraryConfig }) {
  const synced = config.source.sync_status === "synced";
  return <div className="grid gap-3 border-b bg-muted/10 px-5 py-3 text-xs sm:grid-cols-[auto_1fr]"><span className="font-medium">内容来源</span><div><div className="flex flex-wrap items-center gap-2"><span>{config.source.name || "未设置"}</span><Badge variant={synced ? "secondary" : "outline"}>{synced ? "已同步" : "待同步"}</Badge></div>{config.source.note && <p className="mt-1 text-muted-foreground">{config.source.note}</p>}</div></div>;
}

function FragmentEditor({ value, facts, onChange }: { value: CreativeCopyFragment[]; facts: CreativeProductFact[]; onChange: (value: CreativeCopyFragment[]) => void }) {
  const add = () => onChange([...value, { id: crypto.randomUUID(), key: `fragment-${value.length + 1}`, name: "新文案原子", creative_types: ["num"], role: "benefit", semantic_group: "", text: "", tags: [], usage: "core", status: "draft" }]);
  return <EditorSection title="文案原子" description="系统根据当前市场的语义标签动态组合三套完整文案；只有匹配不到时才使用通用兜底。" onAdd={add}>
    {value.map((item, index) => <div key={item.id} className="grid gap-3 border-b py-4 last:border-b-0 lg:grid-cols-[150px_120px_150px_150px_minmax(150px,1fr)_minmax(260px,1.4fr)_120px_36px]">
      <Labeled label="名称"><Input value={item.name} onChange={(event) => onChange(replaceAt(value, index, { ...item, name: event.target.value }))} /></Labeled>
      <Labeled label="职责"><NativeSelect value={item.role} onChange={(event) => onChange(replaceAt(value, index, { ...item, role: event.target.value as CreativeCopyFragmentRole }))}>{ROLES.map((role) => <NativeSelectOption key={role.value} value={role.value}>{role.label}</NativeSelectOption>)}</NativeSelect></Labeled>
      <Labeled label="适用类型"><TypeToggle value={item.creative_types} onChange={(creative_types) => onChange(replaceAt(value, index, { ...item, creative_types }))} /></Labeled>
      <Labeled label="用途"><FragmentUsageSelect value={item.usage} onChange={(usage) => onChange(replaceAt(value, index, { ...item, usage }))} /></Labeled>
      <Labeled label="信息类别"><Input value={item.semantic_group ?? ""} onChange={(event) => onChange(replaceAt(value, index, { ...item, semantic_group: event.target.value }))} placeholder="例如：额度、期限、利率、月供示例" /></Labeled>
      <Labeled label="语义标签"><Input value={item.tags.join("、")} onChange={(event) => onChange(replaceAt(value, index, { ...item, tags: splitList(event.target.value) }))} placeholder="额度、快速、生活场景" /></Labeled>
      <CopyTemplateField value={item.text} facts={facts} onChange={(text) => onChange(replaceAt(value, index, { ...item, text }))} />
      <Labeled label="状态"><StatusSelect value={item.status} onChange={(status) => onChange(replaceAt(value, index, { ...item, status }))} /></Labeled>
      <DeleteButton onClick={() => onChange(value.filter((entry) => entry.id !== item.id))} label="删除片段" />
    </div>)}
  </EditorSection>;
}

function CopyTemplateField({ value, facts, onChange }: { value: string; facts: CreativeProductFact[]; onChange: (value: string) => void }) {
  const preview = resolveCopyTemplateForBusiness(value, facts);
  const availableFacts = facts.filter((fact) => fact.status === "approved" && fact.key.trim() && fact.copy_text.trim());
  return <div className="grid content-start gap-2 text-xs">
    <span className="font-medium text-muted-foreground">投放预览</span>
    <div className="min-h-16 border bg-muted/10 px-3 py-2">
      {preview.text ? <p className="whitespace-pre-wrap text-sm font-medium text-foreground">{preview.text}</p> : <p className="text-muted-foreground">还没有文案内容</p>}
      {preview.usedFacts.length > 0 && <div className="mt-2 flex flex-wrap gap-1.5">{preview.usedFacts.map((fact) => <Badge key={fact.key} variant="outline">{fact.label || fact.key}：{fact.copy_text}</Badge>)}</div>}
      {preview.missingFactKeys.length > 0 && <p className="mt-2 text-destructive">缺少产品事实：{preview.missingFactKeys.join("、")}</p>}
    </div>
    <details open={!value.trim()} className="group border px-3 py-2">
      <summary className="cursor-pointer select-none font-medium text-foreground">编辑文案结构</summary>
      <div className="mt-3 space-y-2">
        <Textarea aria-label="投放文案结构" rows={3} value={value} onChange={(event) => onChange(event.target.value)} placeholder="输入当前市场的投放文案，再按需插入产品事实" />
        <div className="flex flex-wrap items-center gap-2">
          <Variable className="h-4 w-4 text-muted-foreground" />
          <NativeSelect aria-label="插入产品事实" value="" disabled={availableFacts.length === 0} onChange={(event) => {
            const key = event.target.value;
            if (key) onChange(appendCopyFactToken(value, key));
          }}>
            <NativeSelectOption value="">插入产品事实</NativeSelectOption>
            {availableFacts.map((fact) => <NativeSelectOption key={fact.id} value={fact.key}>{fact.label || fact.key} · {fact.copy_text}</NativeSelectOption>)}
          </NativeSelect>
          <span className="text-muted-foreground">动态数字由事实库统一更新。</span>
        </div>
      </div>
    </details>
  </div>;
}

function FactEditor({ value, onChange }: { value: CreativeProductFact[]; onChange: (value: CreativeProductFact[]) => void }) {
  const add = () => onChange([...value, { id: crypto.randomUUID(), key: `fact_${value.length + 1}`, label: "新产品事实", value: "", copy_text: "", source: "业务审核", status: "draft" }]);
  return <EditorSection title="产品事实" description="金融数字只从当前市场的事实库进入文案；数值、展示文本和来源必须可审计。" onAdd={add}>{value.map((fact, index) => <div key={fact.id} className="grid gap-3 border-b py-4 last:border-b-0 lg:grid-cols-[150px_150px_150px_minmax(180px,1fr)_minmax(180px,1fr)_120px_36px]"><Labeled label="事实键"><Input value={fact.key} onChange={(event) => onChange(replaceAt(value, index, { ...fact, key: event.target.value }))} /></Labeled><Labeled label="业务名称"><Input value={fact.label} onChange={(event) => onChange(replaceAt(value, index, { ...fact, label: event.target.value }))} /></Labeled><Labeled label="原始值"><Input value={fact.value} onChange={(event) => onChange(replaceAt(value, index, { ...fact, value: event.target.value }))} /></Labeled><Labeled label="投放展示"><Input aria-invalid={fact.status === "approved" && !fact.copy_text.trim()} value={fact.copy_text} onChange={(event) => onChange(replaceAt(value, index, { ...fact, copy_text: event.target.value }))} /></Labeled><Labeled label="来源 / 依据"><Input aria-invalid={fact.status === "approved" && !fact.source.trim()} value={fact.source} onChange={(event) => onChange(replaceAt(value, index, { ...fact, source: event.target.value }))} placeholder="文件、Sheet 或业务审批" /></Labeled><Labeled label="状态"><StatusSelect value={fact.status} onChange={(status) => onChange(replaceAt(value, index, { ...fact, status }))} /></Labeled><DeleteButton onClick={() => onChange(value.filter((entry) => entry.id !== fact.id))} label="删除事实" /></div>)}</EditorSection>;
}

function RuleEditor({ value, onChange }: { value: CreativeCopyCalculationRule[]; onChange: (value: CreativeCopyCalculationRule[]) => void }) {
  const add = () => onChange([...value, { id: crypto.randomUUID(), key: `rule_${value.length + 1}`, name: "新计算规则", expression: "", input_fact_keys: [], output_fact_key: "", source: "", status: "draft" }]);
  return <EditorSection title="计算规则" description="记录可审计公式及输入输出。未审核规则不会参与推荐或写入订单。" onAdd={add}>{value.map((rule, index) => <div key={rule.id} className="grid gap-3 border-b py-4 last:border-b-0 lg:grid-cols-2"><Labeled label="规则名称"><Input value={rule.name} onChange={(event) => onChange(replaceAt(value, index, { ...rule, name: event.target.value }))} /></Labeled><Labeled label="公式来源"><Input value={rule.source} onChange={(event) => onChange(replaceAt(value, index, { ...rule, source: event.target.value }))} /></Labeled><Labeled label="公式"><Textarea rows={2} value={rule.expression} onChange={(event) => onChange(replaceAt(value, index, { ...rule, expression: event.target.value }))} /></Labeled><div className="grid grid-cols-[1fr_1fr_130px_36px] gap-3"><Labeled label="输入事实键"><Input value={rule.input_fact_keys.join("、")} onChange={(event) => onChange(replaceAt(value, index, { ...rule, input_fact_keys: splitList(event.target.value) }))} /></Labeled><Labeled label="输出事实键"><Input value={rule.output_fact_key} onChange={(event) => onChange(replaceAt(value, index, { ...rule, output_fact_key: event.target.value }))} /></Labeled><Labeled label="状态"><StatusSelect value={rule.status} onChange={(status) => onChange(replaceAt(value, index, { ...rule, status }))} /></Labeled><DeleteButton onClick={() => onChange(value.filter((entry) => entry.id !== rule.id))} label="删除规则" /></div></div>)}</EditorSection>;
}

function EditorSection({ title, description, onAdd, children }: { title: string; description: string; onAdd: () => void; children: ReactNode }) {
  return <section className="mt-4"><div className="flex flex-wrap items-start justify-between gap-3 border-b pb-3"><div><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1 text-xs text-muted-foreground">{description}</p></div><Button size="sm" variant="outline" onClick={onAdd}><Plus className="h-4 w-4" />添加</Button></div>{children}</section>;
}

function Labeled({ label, children }: { label: string; children: ReactNode }) { return <label className="grid content-start gap-1.5 text-xs font-medium text-muted-foreground"><span>{label}</span>{children}</label>; }
function DeleteButton({ onClick, label }: { onClick: () => void; label: string }) { return <Button size="icon-sm" variant="ghost" className="self-end" title={label} aria-label={label} onClick={onClick}><Trash2 className="h-4 w-4" /></Button>; }
function FragmentUsageSelect({ value, onChange }: { value: CreativeCopyFragment["usage"]; onChange: (value: CreativeCopyFragment["usage"]) => void }) { return <NativeSelect value={value} onChange={(event) => onChange(event.target.value as CreativeCopyFragment["usage"])}><NativeSelectOption value="core">核心匹配</NativeSelectOption><NativeSelectOption value="fallback">通用兜底</NativeSelectOption><NativeSelectOption value="required">必须附带</NativeSelectOption></NativeSelect>; }
function StatusSelect({ value, onChange }: { value: "draft" | "approved" | "disabled"; onChange: (value: "draft" | "approved" | "disabled") => void }) { return <NativeSelect value={value} onChange={(event) => onChange(event.target.value as typeof value)}><NativeSelectOption value="draft">草稿</NativeSelectOption><NativeSelectOption value="approved">已审核</NativeSelectOption><NativeSelectOption value="disabled">停用</NativeSelectOption></NativeSelect>; }
function TypeToggle({ value, onChange }: { value: CreativeType[]; onChange: (value: CreativeType[]) => void }) { return <div className="flex h-9 items-center gap-3 border px-3">{(["num", "repayment_plan"] as CreativeType[]).map((type) => <label key={type} className="flex items-center gap-1.5 text-xs"><input type="checkbox" checked={value.includes(type)} onChange={(event) => onChange(event.target.checked ? [...new Set([...value, type])] : value.filter((item) => item !== type))} />{type === "num" ? "NUM" : "还款计划"}</label>)}</div>; }
function replaceAt<T>(value: T[], index: number, next: T): T[] { return value.map((item, current) => current === index ? next : item); }
function splitList(value: string): string[] { return [...new Set(value.split(/[、,，\n]/).map((item) => item.trim()).filter(Boolean))]; }
