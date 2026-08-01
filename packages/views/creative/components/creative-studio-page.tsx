"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Archive,
  BookOpenText,
  Check,
	CheckCircle2,
  FileSpreadsheet,
  FolderOpen,
  Globe2,
  Plus,
  QrCode,
  Save,
  Search,
  Settings2,
  Upload,
} from "lucide-react";
import { api } from "@multica/core/api";
import {
  creativeCopyEntriesOptions,
  creativeKeys,
  creativeResourcesOptions,
} from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import type {
  CreativeCopyEntry,
  CreativeCopyEntryInput,
  CreativeResource,
  CreativeResourceKind,
} from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import { PageHeader } from "../../layout/page-header";
import { readCopySpreadsheet, type SpreadsheetData } from "../lib/xlsx-copy-import";
import { CreativeMaterialLibrary } from "./creative-material-library";
import { MarketResourceFiles } from "./market-resource-files";

type CopyField = keyof Pick<
  CreativeCopyEntryInput,
  "external_key" | "headline" | "subheadline" | "benefit" | "cta" | "legal_text" |
  "copy_role" | "market" | "locale" | "tags" | "status"
>;

const COPY_FIELDS: { key: CopyField; label: string; required?: boolean }[] = [
  { key: "external_key", label: "文案编号" },
  { key: "headline", label: "主标题", required: true },
  { key: "subheadline", label: "副标题" },
  { key: "benefit", label: "卖点" },
  { key: "cta", label: "CTA" },
  { key: "legal_text", label: "条款" },
  { key: "copy_role", label: "文案职责" },
  { key: "market", label: "市场" },
  { key: "locale", label: "语言" },
  { key: "tags", label: "标签" },
  { key: "status", label: "状态" },
];

const PRIME_ROLE_LABELS: Record<string, string> = {
  prime_square: "Prime 方图模板",
  prime_landscape: "Prime 横图模板",
  prime_portrait: "Prime 竖图模板",
};

const EMPTY_COPY: CreativeCopyEntryInput = {
  external_key: "",
  headline: "",
  subheadline: "",
  benefit: "",
  cta: "",
  legal_text: "",
  copy_role: "",
  market: "",
  locale: "",
  tags: [],
  status: "draft",
  metadata: {},
};

export function CreativeStudioPage() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState("materials");
  const [createKind, setCreateKind] = useState<CreativeResourceKind | null>(null);
  const resources = useQuery(creativeResourcesOptions(wsId));
  const allResources = resources.data?.resources ?? [];

  const refreshResources = () => queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
  const archiveResource = useMutation({
    mutationFn: (id: string) => api.archiveCreativeResource(id),
    onSuccess: () => { refreshResources(); toast.success("资源已归档"); },
    onError: () => toast.error("无法归档资源"),
  });

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between px-5">
        <div className="flex min-w-0 items-center gap-2">
          <Settings2 className="h-4 w-4 text-emerald-700" />
          <h1 className="text-sm font-medium">创意工厂</h1>
          <span className="hidden text-xs text-muted-foreground md:inline">维护素材、文案和市场资源</span>
        </div>
        <Badge variant="outline">{allResources.length} 个可配置资源</Badge>
      </PageHeader>

      <Tabs value={tab} onValueChange={setTab} className="flex min-h-0 flex-1 flex-col">
        <div className="border-b px-5 py-2">
          <TabsList className="h-8">
            <TabsTrigger value="materials"><FolderOpen className="h-3.5 w-3.5" />素材库</TabsTrigger>
            <TabsTrigger value="copy"><BookOpenText className="h-3.5 w-3.5" />文案库</TabsTrigger>
            <TabsTrigger value="market"><Globe2 className="h-3.5 w-3.5" />市场资源包</TabsTrigger>
          </TabsList>
        </div>

        <TabsContent value="materials" className="min-h-0 flex-1 overflow-y-auto p-5">
          <CreativeMaterialLibrary />
        </TabsContent>
        <TabsContent value="copy" className="min-h-0 flex-1 overflow-hidden p-5">
          <CopyLibraries
            resources={allResources.filter((resource) => resource.kind === "copy_library")}
            onCreate={() => setCreateKind("copy_library")}
            onArchive={(id) => archiveResource.mutate(id)}
          />
        </TabsContent>
        <TabsContent value="market" className="min-h-0 flex-1 overflow-hidden p-5">
          <ResourceEditor
            resources={allResources.filter((resource) => resource.kind === "market_pack")}
            copyLibraries={allResources.filter((resource) => resource.kind === "copy_library")}
            onCreate={() => setCreateKind("market_pack")}
            onArchive={(id) => archiveResource.mutate(id)}
          />
        </TabsContent>
      </Tabs>

      <CreateResourceDialog kind={createKind} onClose={() => setCreateKind(null)} onCreated={() => {
        setCreateKind(null);
        refreshResources();
      }} />
    </div>
  );
}

function CopyLibraries({ resources, onCreate, onArchive }: {
  resources: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const [spreadsheet, setSpreadsheet] = useState<{ data: SpreadsheetData; filename: string } | null>(null);
  const [mapping, setMapping] = useState<Record<CopyField, string>>(emptyCopyMapping());
  const [editing, setEditing] = useState<CreativeCopyEntry | "new" | null>(null);
  const [query, setQuery] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const entries = useQuery(creativeCopyEntriesOptions(wsId, active?.id ?? ""));
  const visibleEntries = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    if (!needle) return entries.data?.entries ?? [];
    return (entries.data?.entries ?? []).filter((entry) => [
      entry.external_key,
      entry.headline,
      entry.subheadline,
      entry.benefit,
      entry.cta,
      entry.legal_text,
      entry.copy_role,
      entry.market,
      entry.locale,
      ...entry.tags,
    ].join(" ").toLocaleLowerCase().includes(needle));
  }, [entries.data?.entries, query]);

  const importRows = useMutation({
    mutationFn: async () => {
      if (!active || !spreadsheet) throw new Error("请选择文案库和文件");
      const parsed = spreadsheet.data.rows.map((row, index) => spreadsheetRowToCopy(row, spreadsheet.data.headers, mapping, index));
      return api.importCreativeCopyEntries(active.id, {
        mode: "upsert",
        source_filename: spreadsheet.filename,
        mapping,
        entries: parsed,
      });
    },
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.copyEntries(wsId, active?.id ?? "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      setSpreadsheet(null);
      toast.success(`导入完成：新增 ${result.created}，更新 ${result.updated}，跳过 ${result.skipped}`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法导入文案"),
  });

  const saveEntry = useMutation({
    mutationFn: async (input: CreativeCopyEntryInput) => {
      if (!active) throw new Error("请选择文案库");
      if (editing === "new") {
        await api.importCreativeCopyEntries(active.id, {
          mode: "upsert", source_filename: "在线编辑", mapping: {}, entries: [input],
        });
        return;
      }
      if (editing) await api.updateCreativeCopyEntry(editing.id, input);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.copyEntries(wsId, active?.id ?? "") });
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      setEditing(null);
      toast.success("文案已保存为草稿版本");
    },
    onError: () => toast.error("无法保存文案"),
  });

  const handleFile = async (file?: File) => {
    if (!file) return;
    try {
      const data = await readCopySpreadsheet(file);
      setMapping(inferCopyMapping(data.headers));
      setSpreadsheet({ data, filename: file.name });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "无法读取表格");
    }
  };

  return (
    <div className="grid h-full min-h-0 grid-cols-1 overflow-hidden border md:grid-cols-[260px_minmax(0,1fr)]">
      <ResourceList title="文案库" resources={resources} activeId={active?.id ?? ""} onSelect={setActiveId} onCreate={onCreate} />
      <div className="min-w-0 overflow-y-auto bg-background">
        {!active ? <EmptyResource title="还没有文案库" action="创建文案库" onAction={onCreate} /> : (
          <>
            <div className="sticky top-0 z-10 border-b bg-background px-4 py-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <ResourceTitle resource={active} />
                <div className="flex items-center gap-2">
                <input ref={fileRef} type="file" accept=".xlsx,.csv" className="hidden" onChange={(event) => handleFile(event.target.files?.[0])} />
                <Button size="sm" variant="outline" onClick={() => fileRef.current?.click()}><Upload className="h-4 w-4" />导入 Excel</Button>
                <Button size="sm" variant="outline" onClick={() => setEditing("new")}><Plus className="h-4 w-4" />添加文案</Button>
                <PublishButton resource={active} />
                <Button size="icon-sm" variant="ghost" title="归档文案库" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button>
                </div>
              </div>
              <div className="mt-3 flex items-center gap-3">
                <div className="relative max-w-xl flex-1"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索编号、标题、卖点、职责或标签" /></div>
                <span className="shrink-0 text-xs text-muted-foreground">{visibleEntries.length} / {entries.data?.entries.length ?? 0} 条</span>
              </div>
            </div>
            <div className="min-w-[920px]">
              <div className="grid grid-cols-[120px_minmax(180px,1fr)_minmax(160px,1fr)_140px_100px_72px_56px] gap-3 border-b bg-muted/30 px-4 py-2 text-xs font-medium text-muted-foreground">
                <span>编号</span><span>主标题</span><span>副标题 / 卖点</span><span>职责</span><span>市场</span><span>状态</span><span />
              </div>
              {visibleEntries.map((entry) => (
                <div key={entry.id} className="grid min-h-14 grid-cols-[120px_minmax(180px,1fr)_minmax(160px,1fr)_140px_100px_72px_56px] items-center gap-3 border-b px-4 py-2 text-sm hover:bg-muted/20">
                  <span className="truncate font-mono text-xs text-muted-foreground">{entry.external_key}</span>
                  <span className="truncate font-medium">{entry.headline || "未填写"}</span>
                  <span className="truncate text-muted-foreground">{entry.subheadline || entry.benefit || "-"}</span>
                  <span className="truncate">{entry.copy_role || "通用"}</span>
                  <span className="truncate">{entry.market || "-"}</span>
                  <Badge variant="outline" className="w-fit text-[10px]">{copyStatusLabel(entry.status)}</Badge>
                  <Button size="icon-sm" variant="ghost" title="编辑文案" onClick={() => setEditing(entry)}><Settings2 className="h-4 w-4" /></Button>
                </div>
              ))}
              {!entries.isLoading && (entries.data?.entries.length ?? 0) === 0 && <div className="py-20 text-center text-sm text-muted-foreground">导入 Excel 或在线添加第一条文案。</div>}
              {!entries.isLoading && (entries.data?.entries.length ?? 0) > 0 && visibleEntries.length === 0 && <div className="py-20 text-center text-sm text-muted-foreground">没有匹配的文案。</div>}
            </div>
          </>
        )}
      </div>
      <SpreadsheetImportDialog
        value={spreadsheet}
        mapping={mapping}
        onMapping={setMapping}
        onClose={() => setSpreadsheet(null)}
        onImport={() => importRows.mutate()}
        busy={importRows.isPending}
      />
      <CopyEntryDialog
        entry={editing}
        onClose={() => setEditing(null)}
        onSave={(input) => saveEntry.mutate(input)}
        busy={saveEntry.isPending}
      />
    </div>
  );
}

function ResourceEditor({ resources, copyLibraries, onCreate, onArchive }: {
  resources: CreativeResource[];
  copyLibraries: CreativeResource[];
  onCreate: () => void;
  onArchive: (id: string) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [activeId, setActiveId] = useState(resources[0]?.id ?? "");
  const active = resources.find((resource) => resource.id === activeId) ?? resources[0];
  const [draft, setDraft] = useState<Record<string, unknown>>(active?.config ?? {});
  const draftKey = active ? `${active.id}:${active.version}` : "";
  useEffect(() => {
    setDraft(active?.config ?? {});
  }, [draftKey, active]);
  const save = useMutation({
    mutationFn: () => {
      if (!active) throw new Error("resource missing");
      return api.updateCreativeResource(active.id, { name: active.name, description: active.description, config: draft });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
      toast.success("配置已保存为新草稿版本");
    },
    onError: () => toast.error("无法保存配置"),
  });
  return (
    <div className="grid h-full min-h-0 grid-cols-1 overflow-hidden border md:grid-cols-[260px_minmax(0,1fr)]">
      <ResourceList title="市场资源包" resources={resources} activeId={active?.id ?? ""} onSelect={setActiveId} onCreate={onCreate} />
      <div className="min-w-0 overflow-y-auto bg-background">
        {!active ? <EmptyResource title="还没有市场资源包" action="创建资源包" onAction={onCreate} /> : (
          <>
            <div className="sticky top-0 z-10 flex flex-wrap items-center justify-between gap-3 border-b bg-background px-5 py-3">
              <ResourceTitle resource={active} />
              <div className="flex items-center gap-2">
                <Button size="sm" variant="outline" onClick={() => save.mutate()} disabled={save.isPending}><Save className="h-4 w-4" />保存草稿</Button>
                <PublishButton resource={active} />
                <Button size="icon-sm" variant="ghost" title="归档资源" onClick={() => onArchive(active.id)}><Archive className="h-4 w-4" /></Button>
              </div>
            </div>
            <MarketPackForm resource={active} value={draft} onChange={setDraft} copyLibraries={copyLibraries} />
          </>
        )}
      </div>
    </div>
  );
}

function MarketPackForm({ resource, value, onChange, copyLibraries }: {
  resource: CreativeResource;
  value: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
  copyLibraries: CreativeResource[];
}) {
  const set = (key: string, next: unknown) => onChange({ ...value, [key]: next });
  const qrValidation = readQRValidation(resource.config.qr_validation);
  return (
    <div className="mx-auto max-w-5xl space-y-8 px-5 py-6">
      <FormSection title="市场与语言" description="运行时根据这里选择文案、合规和交付规则。">
        <Field label="品牌"><Input value={stringValue(value.brand)} onChange={(event) => set("brand", event.target.value)} /></Field>
        <Field label="市场"><Input value={stringValue(value.market)} placeholder="Indonesia" onChange={(event) => set("market", event.target.value)} /></Field>
        <Field label="语言"><Input value={stringValue(value.locale)} placeholder="id-ID" onChange={(event) => set("locale", event.target.value)} /></Field>
        <Field label="币种"><Input value={stringValue(value.currency)} placeholder="IDR" onChange={(event) => set("currency", event.target.value)} /></Field>
        <Field label="绑定文案库" wide>
          <NativeSelect value={stringValue(value.copy_library_id)} onChange={(event) => set("copy_library_id", event.target.value)}>
            <NativeSelectOption value="">选择已发布文案库</NativeSelectOption>
            {copyLibraries.map((library) => <NativeSelectOption key={library.id} value={library.id}>{library.name} · v{library.published_version || library.version}</NativeSelectOption>)}
          </NativeSelect>
        </Field>
      </FormSection>
      <FormSection title="创意理解" description="候选图按这里的市场词表识别主题与利益点；用户仍可逐图输入词表外的新值。">
        <Field label="利益点词表" wide><Textarea rows={4} value={stringListMultilineValue(value.benefit_taxonomy)} placeholder={"额度\n低利率\n费用减免\n快速放款"} onChange={(event) => set("benefit_taxonomy", splitList(event.target.value))} /></Field>
        <Field label="主题预设" wide><Textarea rows={4} value={stringListMultilineValue(value.theme_presets)} placeholder={"世界杯 / 足球赛事\n斋月\n开斋节\n发薪日"} onChange={(event) => set("theme_presets", splitList(event.target.value))} /></Field>
      </FormSection>
      <MarketResourceFiles resource={resource} />
      <FormSection title="二维码与规则" description="二维码必须经过业务批准，并与三个 Prime 模板的机器解码结果一致后才能发布。">
        <Field label="成品二维码目标地址" wide><Input value={stringValue(value.qr_payload)} placeholder="https://www.example.com/terms" onChange={(event) => set("qr_payload", event.target.value)} /></Field>
        <Field label="Prime 模板标准地址" wide><Input value={stringValue(value.qr_canonical_payload)} placeholder="从已批准 Prime 模板机器解码得到" onChange={(event) => set("qr_canonical_payload", event.target.value)} /></Field>
        <Field label="允许域名"><Input value={stringListValue(value.qr_allowed_domains)} placeholder="www.example.com" onChange={(event) => set("qr_allowed_domains", splitList(event.target.value))} /></Field>
        <Field label="业务批准状态"><NativeSelect value={stringValue(value.qr_approval_status) || "pending"} onChange={(event) => set("qr_approval_status", event.target.value)}><NativeSelectOption value="pending">待批准</NativeSelectOption><NativeSelectOption value="approved">已批准</NativeSelectOption></NativeSelect></Field>
        <Field label="批准说明" wide><Input value={stringValue(value.qr_approval_note)} placeholder="记录地址来源或批准依据" onChange={(event) => set("qr_approval_note", event.target.value)} /></Field>
        <Field label="合规规则" wide><Textarea rows={5} value={stringValue(value.compliance_rules)} onChange={(event) => set("compliance_rules", event.target.value)} /></Field>
        <Field label="文件命名" wide><Input value={stringValue(value.naming_rule)} placeholder="Month_P_Brand_Country_Date_Type_Designer_Size.png" onChange={(event) => set("naming_rule", event.target.value)} /></Field>
      </FormSection>
      <section>
        <div className="mb-3 flex items-center gap-2"><QrCode className="h-4 w-4" /><h3 className="text-sm font-semibold">发布校验</h3></div>
        {qrValidation ? <div className="border-y">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-emerald-50/50 px-4 py-3 dark:bg-emerald-950/15">
            <div className="flex min-w-0 items-center gap-2"><CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600" /><div className="min-w-0"><p className="text-sm font-medium">三个 Prime 模板校验通过</p><p className="truncate font-mono text-xs text-muted-foreground">{qrValidation.approved_payload}</p></div></div>
            <span className="text-xs text-muted-foreground">{formatValidationTime(qrValidation.validated_at)}</span>
          </div>
          <div className="divide-y">{qrValidation.templates.map((template) => <div key={template.role} className="grid gap-1 px-4 py-3 text-xs sm:grid-cols-[150px_minmax(0,1fr)_auto] sm:items-center"><span className="font-medium">{resourceSlotLabel(template.role)}</span><span className="truncate font-mono text-muted-foreground">{template.filename}</span><span className="text-emerald-700 dark:text-emerald-400">解码一致</span></div>)}</div>
        </div> : <div className="flex gap-3 border-y bg-amber-50/50 px-4 py-3 dark:bg-amber-950/15"><AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" /><div><p className="text-sm font-medium">当前草稿尚未通过二维码校验</p><p className="mt-1 text-xs text-muted-foreground">保存配置并点击发布。服务端会读取三个 Prime 模板，真实解码后再决定是否允许发布。</p></div></div>}
      </section>
    </div>
  );
}

function ResourceList({ title, resources, activeId, onSelect, onCreate }: {
  title: string;
  resources: CreativeResource[];
  activeId: string;
  onSelect: (id: string) => void;
  onCreate: () => void;
}) {
  return (
    <aside className="min-h-0 overflow-y-auto border-r bg-muted/15">
      <div className="sticky top-0 flex items-center justify-between border-b bg-background px-3 py-2.5">
        <span className="text-sm font-semibold">{title}</span>
        <Button size="icon-sm" variant="ghost" title={`创建${title}`} onClick={onCreate}><Plus className="h-4 w-4" /></Button>
      </div>
      {resources.map((resource) => (
        <button key={resource.id} type="button" onClick={() => onSelect(resource.id)} className={cn("block w-full border-b px-3 py-3 text-left hover:bg-muted/40", resource.id === activeId && "bg-background shadow-[inset_2px_0_0_hsl(var(--primary))]") }>
          <div className="flex items-center justify-between gap-2"><span className="truncate text-sm font-medium">{resource.name}</span><span className="shrink-0 font-mono text-[10px] text-muted-foreground">v{resource.version}</span></div>
          <p className="mt-1 truncate text-xs text-muted-foreground">{resource.description || (resource.status === "published" ? "已发布" : "草稿")}</p>
        </button>
      ))}
    </aside>
  );
}

function ResourceTitle({ resource }: { resource: CreativeResource }) {
  return <div className="min-w-0"><div className="flex items-center gap-2"><h2 className="truncate text-sm font-semibold">{resource.name}</h2><Badge variant="outline">{resource.status === "published" ? `已发布 v${resource.published_version}` : `草稿 v${resource.version}`}</Badge></div><p className="mt-0.5 truncate text-xs text-muted-foreground">{resource.description}</p></div>;
}

function PublishButton({ resource }: { resource: CreativeResource }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const publish = useMutation({
    mutationFn: () => api.publishCreativeResource(resource.id),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) }); toast.success("版本已发布"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法发布版本"),
  });
  return <Button size="sm" onClick={() => publish.mutate()} disabled={publish.isPending || (resource.status === "published" && resource.published_version === resource.version)}><Check className="h-4 w-4" />发布</Button>;
}

function CreateResourceDialog({ kind, onClose, onCreated }: { kind: CreativeResourceKind | null; onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const create = useMutation({
    mutationFn: () => api.createCreativeResource({ kind: kind!, name, description, config: initialResourceConfig(kind!) }),
    onSuccess: () => { setName(""); setDescription(""); onCreated(); toast.success("资源已创建"); },
    onError: () => toast.error("无法创建资源"),
  });
  return (
    <Dialog open={kind !== null} onOpenChange={(open) => !open && onClose()}>
      <DialogContent><DialogHeader><DialogTitle>创建{kindLabel(kind)}</DialogTitle><DialogDescription>先创建草稿，配置完成后再发布给 Issue 使用。</DialogDescription></DialogHeader>
        <div className="space-y-4"><Field label="名称" wide><Input value={name} onChange={(event) => setName(event.target.value)} /></Field><Field label="描述" wide><Textarea rows={3} value={description} onChange={(event) => setDescription(event.target.value)} /></Field></div>
        <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={!name.trim() || create.isPending} onClick={() => create.mutate()}>创建</Button></DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SpreadsheetImportDialog({ value, mapping, onMapping, onClose, onImport, busy }: {
  value: { data: SpreadsheetData; filename: string } | null;
  mapping: Record<CopyField, string>;
  onMapping: (value: Record<CopyField, string>) => void;
  onClose: () => void;
  onImport: () => void;
  busy: boolean;
}) {
  return <Dialog open={value !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-4xl"><DialogHeader><DialogTitle>映射 Excel 文案字段</DialogTitle><DialogDescription>{value ? `${value.filename} · ${value.data.sheetName} · ${value.data.rows.length} 行` : ""}</DialogDescription></DialogHeader>
    {value && <div className="grid max-h-[55vh] grid-cols-2 gap-4 overflow-y-auto pr-1 md:grid-cols-3">{COPY_FIELDS.map((field) => <Field key={field.key} label={`${field.label}${field.required ? " *" : ""}`} wide><NativeSelect value={mapping[field.key]} onChange={(event) => onMapping({ ...mapping, [field.key]: event.target.value })}><NativeSelectOption value="">不导入</NativeSelectOption>{value.data.headers.map((header) => <NativeSelectOption key={header} value={header}>{header}</NativeSelectOption>)}</NativeSelect></Field>)}</div>}
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button onClick={onImport} disabled={busy || !mapping.headline}>{busy ? "正在导入" : "确认导入"}</Button></DialogFooter></DialogContent></Dialog>;
}

function CopyEntryDialog({ entry, onClose, onSave, busy }: { entry: CreativeCopyEntry | "new" | null; onClose: () => void; onSave: (value: CreativeCopyEntryInput) => void; busy: boolean }) {
  const source = entry && entry !== "new" ? copyEntryToInput(entry) : EMPTY_COPY;
  const key = entry && entry !== "new" ? `${entry.id}:${entry.version}` : String(entry);
  const [loadedKey, setLoadedKey] = useState(key);
  const [draft, setDraft] = useState(source);
  if (key !== loadedKey) { setLoadedKey(key); setDraft(source); }
  const set = <K extends keyof CreativeCopyEntryInput>(field: K, value: CreativeCopyEntryInput[K]) => setDraft({ ...draft, [field]: value });
  return <Dialog open={entry !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-3xl"><DialogHeader><DialogTitle>{entry === "new" ? "添加文案" : "编辑文案"}</DialogTitle><DialogDescription>保存会创建可追溯的新版本，不覆盖已运行 Issue 的快照。</DialogDescription></DialogHeader>
    <div className="grid max-h-[60vh] grid-cols-1 gap-4 overflow-y-auto pr-1 sm:grid-cols-2">
      <Field label="文案编号"><Input value={draft.external_key} onChange={(event) => set("external_key", event.target.value)} /></Field><Field label="状态"><NativeSelect value={draft.status} onChange={(event) => set("status", event.target.value as CreativeCopyEntryInput["status"])}><NativeSelectOption value="draft">草稿</NativeSelectOption><NativeSelectOption value="approved">已审核</NativeSelectOption><NativeSelectOption value="disabled">停用</NativeSelectOption></NativeSelect></Field>
      <Field label="主标题" wide><Input value={draft.headline} onChange={(event) => set("headline", event.target.value)} /></Field><Field label="副标题" wide><Input value={draft.subheadline} onChange={(event) => set("subheadline", event.target.value)} /></Field><Field label="卖点" wide><Textarea rows={3} value={draft.benefit} onChange={(event) => set("benefit", event.target.value)} /></Field><Field label="CTA"><Input value={draft.cta} onChange={(event) => set("cta", event.target.value)} /></Field><Field label="文案职责"><Input value={draft.copy_role} onChange={(event) => set("copy_role", event.target.value)} /></Field><Field label="市场"><Input value={draft.market} onChange={(event) => set("market", event.target.value)} /></Field><Field label="语言"><Input value={draft.locale} onChange={(event) => set("locale", event.target.value)} /></Field><Field label="条款" wide><Textarea rows={3} value={draft.legal_text} onChange={(event) => set("legal_text", event.target.value)} /></Field><Field label="标签" wide><Input value={draft.tags.join(", ")} onChange={(event) => set("tags", event.target.value.split(",").map((item) => item.trim()).filter(Boolean))} /></Field>
    </div><DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={busy || !draft.headline.trim()} onClick={() => onSave(draft)}>保存</Button></DialogFooter></DialogContent></Dialog>;
}

function FormSection({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <section><div className="mb-3"><h3 className="text-sm font-semibold">{title}</h3><p className="mt-1 text-xs text-muted-foreground">{description}</p></div><div className="grid grid-cols-1 gap-4 border-t pt-4 sm:grid-cols-2">{children}</div></section>;
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) {
  return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label className="text-xs text-muted-foreground">{label}</Label>{children}</div>;
}

function EmptyResource({ title, action, onAction }: { title: string; action: string; onAction: () => void }) {
  return <div className="flex min-h-80 flex-col items-center justify-center gap-3 text-sm text-muted-foreground"><FileSpreadsheet className="h-6 w-6" /><span>{title}</span><Button size="sm" onClick={onAction}>{action}</Button></div>;
}

function initialResourceConfig(kind: CreativeResourceKind): Record<string, unknown> {
  if (kind === "copy_library") return { market: "", locale: "", source_filename: "", import_mapping: {} };
  return { brand: "", market: "", locale: "", currency: "", copy_library_id: "", benefit_taxonomy: [], theme_presets: [], qr_payload: "", qr_canonical_payload: "", qr_allowed_domains: [], qr_approval_status: "pending", qr_approval_note: "", compliance_rules: "", naming_rule: "" };
}

function kindLabel(kind: CreativeResourceKind | null): string {
  if (kind === "copy_library") return "文案库";
  return "市场资源包";
}

function stringValue(value: unknown): string { return typeof value === "string" ? value : ""; }
function stringListValue(value: unknown): string { return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").join(", ") : ""; }
function stringListMultilineValue(value: unknown): string { return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string").join("\n") : ""; }
function splitList(value: string): string[] { return [...new Set(value.split(/[\n,，;；]+/).map((item) => item.trim()).filter(Boolean))]; }
function copyStatusLabel(status: string): string { return status === "approved" ? "已审核" : status === "disabled" ? "停用" : "草稿"; }

function copyEntryToInput(entry: CreativeCopyEntry): CreativeCopyEntryInput {
  return { external_key: entry.external_key, headline: entry.headline, subheadline: entry.subheadline, benefit: entry.benefit, cta: entry.cta, legal_text: entry.legal_text, copy_role: entry.copy_role, market: entry.market, locale: entry.locale, tags: entry.tags, status: entry.status, metadata: entry.metadata };
}

function emptyCopyMapping(): Record<CopyField, string> {
  return { external_key: "", headline: "", subheadline: "", benefit: "", cta: "", legal_text: "", copy_role: "", market: "", locale: "", tags: "", status: "" };
}

type QRValidation = {
  approved_payload: string;
  validated_at: string;
  templates: { role: string; filename: string; decoded_payload: string }[];
};

function readQRValidation(value: unknown): QRValidation | null {
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  if (record.status !== "passed" || typeof record.approved_payload !== "string" || !Array.isArray(record.templates)) return null;
  const templates = record.templates.flatMap((item) => {
    if (!item || typeof item !== "object") return [];
    const template = item as Record<string, unknown>;
    if (typeof template.role !== "string" || typeof template.filename !== "string" || typeof template.decoded_payload !== "string") return [];
    return [{ role: template.role, filename: template.filename, decoded_payload: template.decoded_payload }];
  });
  if (templates.length !== 3) return null;
  return { approved_payload: record.approved_payload, validated_at: stringValue(record.validated_at), templates };
}

function formatValidationTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "已由服务端验证" : `验证于 ${date.toLocaleString("zh-CN")}`;
}

function resourceSlotLabel(role: string): string {
  return PRIME_ROLE_LABELS[role] ?? role;
}

function inferCopyMapping(headers: string[]): Record<CopyField, string> {
  const synonyms: Record<CopyField, string[]> = {
    external_key: ["id", "code", "key", "编号", "文案编号"], headline: ["headline", "title", "judul", "主标题", "标题"],
    subheadline: ["subheadline", "subtitle", "副标题"], benefit: ["benefit", "selling point", "manfaat", "卖点", "利益点"],
    cta: ["cta", "button", "action", "按钮"], legal_text: ["legal", "terms", "syarat", "条款", "合规"],
    copy_role: ["role", "type", "职责", "文案类型"], market: ["market", "country", "市场", "国家"],
    locale: ["locale", "language", "语言"], tags: ["tags", "tag", "标签"], status: ["status", "状态"],
  };
  const mapping = emptyCopyMapping();
  for (const field of COPY_FIELDS) {
    const match = headers.find((header) => synonyms[field.key].some((synonym) => header.trim().toLowerCase() === synonym.toLowerCase()));
    mapping[field.key] = match ?? "";
  }
  if (!mapping.headline && headers[0]) mapping.headline = headers[0];
  return mapping;
}

function spreadsheetRowToCopy(row: string[], headers: string[], mapping: Record<CopyField, string>, index: number): CreativeCopyEntryInput {
  const read = (field: CopyField) => { const column = headers.indexOf(mapping[field]); return column >= 0 ? row[column] ?? "" : ""; };
  const status = read("status").toLowerCase();
  return {
    external_key: read("external_key") || `row-${index + 2}`,
    headline: read("headline"), subheadline: read("subheadline"), benefit: read("benefit"), cta: read("cta"), legal_text: read("legal_text"), copy_role: read("copy_role"), market: read("market"), locale: read("locale"),
    tags: read("tags").split(/[,;|]/).map((item) => item.trim()).filter(Boolean),
    status: /approved|active|已审核|确认/.test(status) ? "approved" : /disabled|停用/.test(status) ? "disabled" : "draft",
    metadata: { spreadsheet_row: index + 2 },
  };
}
