"use client";

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Check, Download, Eye, FileText, ImageIcon, LoaderCircle, Pencil, Plus, Replace, ScanSearch, Trash2, Upload } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, creativeMarketPackComponentExtractionOptions, creativeResourceFilesOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { CreativeResource, CreativeResourceFile } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";
import type { PrimeComposition } from "./prime-composition-editor";
import { PrimeLayoutImportDialog, extractPrimeComponentFiles, type PrimeLayoutImportSelection } from "./prime-layout-import-dialog";
import { applyPrimeLayoutImport } from "../lib/prime-layout-import";

const PURPOSES = [
  { role: "prime_logo", label: "品牌 Logo", description: "显示在成图品牌区域" },
  { role: "prime_store_badges", label: "应用商店标识", description: "Google Play、App Store 等下载标识" },
  { role: "prime_qr", label: "静态二维码", description: "选择使用二维码图片时显示" },
  { role: "prime_afpi", label: "AFPI 标识", description: "按市场要求选择是否启用" },
  { role: "prime_pindai_legal", label: "Pindai Legal", description: "按市场要求选择是否启用" },
  { role: "prime_custom", label: "其他图片组件", description: "用于新增的品牌或合规图片" },
  { role: "app_ui_reference", label: "App UI 参考图", description: "用于创意生成时参考产品界面" },
  { role: "brand_asset", label: "品牌素材", description: "其他可供创意使用的品牌图片" },
  { role: "brand_guideline", label: "品牌与版式说明", description: "品牌规范和视觉使用说明" },
  { role: "compliance_reference", label: "市场合规材料", description: "该市场适用的合规参考文件" },
] as const;

const PRIME_COMPONENT_ROLES = ["prime_logo", "prime_store_badges", "prime_qr", "prime_afpi", "prime_pindai_legal"] as const;
export const PRIME_RESOURCE_UPLOAD_EVENT = "creative-prime-resource-upload";

const LEGACY_PRIME_SHEET_ROLES = new Set(["prime_square", "prime_landscape", "prime_portrait"]);

type AssetDraft = {
  role: string;
  label: string;
  description: string;
  dimensions: string;
  market: string;
  locale: string;
  tags: string;
};

const EMPTY_DRAFT: AssetDraft = {
  role: "",
  label: "",
  description: "",
  dimensions: "",
  market: "Indonesia",
  locale: "id-ID",
  tags: "",
};

export function MarketResourceFiles({ resource, value, composition, onChange }: {
  resource: CreativeResource;
  value: Record<string, unknown>;
  composition: PrimeComposition;
  onChange: (value: Record<string, unknown>) => void;
}) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const { upload, uploading } = useFileUpload(api);
  const files = useQuery(creativeResourceFilesOptions(wsId, resource.id));
  const extraction = useQuery(creativeMarketPackComponentExtractionOptions(wsId, resource.id));
  const [editing, setEditing] = useState<CreativeResourceFile | "new" | null>(null);
  const [preview, setPreview] = useState<CreativeResourceFile | null>(null);
  const [quickRole, setQuickRole] = useState<string | null>(null);
  const [layoutImportOpen, setLayoutImportOpen] = useState(false);
  const quickInputRef = useRef<HTMLInputElement>(null);
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: creativeKeys.resourceFiles(wsId, resource.id) });
    queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
  };
  const remove = useMutation({
    mutationFn: (fileId: string) => api.removeCreativeResourceFile(resource.id, fileId),
    onSuccess: () => { refresh(); toast.success("资源文件已从新版本移除"); },
    onError: () => toast.error("无法移除资源文件"),
  });
  const currentFiles = files.data?.files ?? [];
  const currentPrimeFile = (role: string) => [...currentFiles].reverse().find((file) => file.role === role);
  const startExtraction = useMutation({
    mutationFn: async ({ file, width, height }: { file: File; width: number; height: number }) => {
      const uploaded = await upload(file);
      if (!uploaded) throw new Error("完整成图上传失败");
      return api.createCreativeMarketPackComponentExtraction(resource.id, { attachment_id: uploaded.id, width, height });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.marketPackExtraction(wsId, resource.id) });
      toast.success("图片已上传，视觉智能体正在后台识别");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法启动品牌组件识别"),
  });
  const importLayout = useMutation({
    mutationFn: async (selection: PrimeLayoutImportSelection) => {
      const extracted = await extractPrimeComponentFiles(selection);
      const added = await Promise.all(extracted.map(async ({ candidate, file }) => {
        const uploaded = await upload(file);
        if (!uploaded) throw new Error(`无法上传${candidate.label}`);
        return api.addCreativeResourceFile(resource.id, {
          attachment_id: uploaded.id,
          role: candidate.role,
          label: `${candidate.label} · 从 ${selection.source.name} 提取`,
          metadata: {
            market: textMetadata(value, "market"),
            locale: textMetadata(value, "locale"),
            source_type: "extracted_from_finished_image",
            source_filename: selection.source.name,
            source_attachment_id: selection.source instanceof File ? "" : selection.source.attachmentId,
            source_extraction_id: selection.extractionId,
            source_layout_size: selection.size,
            source_rect: candidate.rect,
            reused_across_sizes: true,
          },
        });
      }));
      await Promise.all(added.map(async (file) => {
        const previous = currentPrimeFile(file.role);
        if (previous && previous.id !== file.id) await api.removeCreativeResourceFile(resource.id, previous.id);
      }));
      const nextComposition = applyPrimeLayoutImport(composition, selection.size, selection.candidates);
      const nextValue = { ...value, prime_composition: nextComposition };
      await api.updateCreativeResource(resource.id, { name: resource.name, description: resource.description, config: nextValue });
      await api.applyCreativeMarketPackComponentExtraction(resource.id, selection.extractionId);
      return nextValue;
    },
    onSuccess: (nextValue) => {
      onChange(nextValue);
      setLayoutImportOpen(false);
      refresh();
      queryClient.invalidateQueries({ queryKey: creativeKeys.marketPackExtraction(wsId, resource.id) });
      toast.success("品牌组件已提取，三个尺寸布局已更新");
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法从成图提取品牌组件"),
  });
  const quickUpload = useMutation({
    mutationFn: async ({ role, selected, previous }: { role: string; selected: File; previous?: CreativeResourceFile }) => {
      const uploaded = await upload(selected);
      if (!uploaded) throw new Error("文件上传失败");
      const next = await api.addCreativeResourceFile(resource.id, {
        attachment_id: uploaded.id,
        role,
        label: selected.name,
        metadata: {
          market: textMetadata(resource.config, "market"),
          locale: textMetadata(resource.config, "locale"),
        },
      });
      if (previous) await api.removeCreativeResourceFile(resource.id, previous.id);
      return next;
    },
    onSuccess: () => { refresh(); toast.success("品牌组件已更新，三个尺寸将自动复用"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法更新品牌组件"),
    onSettled: () => {
      setQuickRole(null);
      if (quickInputRef.current) quickInputRef.current.value = "";
    },
  });
  const requestQuickUpload = (role: string) => {
    setQuickRole(role);
    quickInputRef.current?.click();
  };
  useEffect(() => {
    const openSlot = (event: Event) => {
      const role = (event as CustomEvent<{ role?: string }>).detail?.role;
      if (!role) return;
      document.getElementById("market-prime-resources")?.scrollIntoView({ behavior: "smooth", block: "center" });
      setQuickRole(role);
      quickInputRef.current?.click();
    };
    window.addEventListener(PRIME_RESOURCE_UPLOAD_EVENT, openSlot);
    return () => window.removeEventListener(PRIME_RESOURCE_UPLOAD_EVENT, openSlot);
  }, []);
  const auxiliaryFiles = currentFiles.filter((file) => !PRIME_COMPONENT_ROLES.includes(file.role as (typeof PRIME_COMPONENT_ROLES)[number]) && !LEGACY_PRIME_SHEET_ROLES.has(file.role));
  const readyCount = PRIME_COMPONENT_ROLES.filter((role) => currentPrimeFile(role)).length;

  return <section className="space-y-8">
    <input ref={quickInputRef} type="file" accept="image/*" className="hidden" onChange={(event) => {
      const selected = event.target.files?.[0];
      if (!selected || !quickRole) return;
      quickUpload.mutate({ role: quickRole, selected, previous: currentPrimeFile(quickRole) });
    }} />
    <section id="market-prime-resources">
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div><h3 className="text-sm font-semibold">品牌组件</h3><p className="mt-1 text-xs text-muted-foreground">每项上传一次，方形、横版和竖版自动复用。</p></div>
        <div className="flex items-center gap-3"><span className="text-xs text-muted-foreground">{readyCount}/{PRIME_COMPONENT_ROLES.length} 项已有资源</span><Button size="sm" variant="outline" onClick={() => setLayoutImportOpen(true)}><ScanSearch className="h-4 w-4" />从成图提取</Button></div>
      </div>
      {extraction.data && extraction.data.status !== "applied" && <button type="button" onClick={() => setLayoutImportOpen(true)} className={cn("mb-3 flex w-full items-center gap-3 border px-3 py-2.5 text-left", extraction.data.status === "failed" ? "border-destructive/30 bg-destructive/5" : "bg-muted/20")}>
        {extraction.data.status === "pending" || extraction.data.status === "running" ? <LoaderCircle className="h-4 w-4 shrink-0 animate-spin" /> : extraction.data.status === "failed" ? <AlertTriangle className="h-4 w-4 shrink-0 text-destructive" /> : <Check className="h-4 w-4 shrink-0 text-emerald-700" />}
        <span className="min-w-0 flex-1"><span className="block text-sm font-medium">{extraction.data.status === "pending" || extraction.data.status === "running" ? "品牌组件识别中" : extraction.data.status === "failed" ? "品牌组件识别失败" : `识别完成 · ${extraction.data.result.candidates.length} 个候选`}</span><span className="mt-0.5 block truncate text-xs text-muted-foreground">{extraction.data.status === "completed" ? "打开确认候选，确认前不会修改市场包" : extraction.data.error_message || extraction.data.source_filename}</span></span>
        <span className="text-xs font-medium">查看</span>
      </button>}
      <div className="border-y divide-y">
        {PRIME_COMPONENT_ROLES.map((role) => {
          const purpose = PURPOSES.find((item) => item.role === role)!;
          const file = currentPrimeFile(role);
          return <PrimeResourceSlot key={role} label={purpose.label} description={purpose.description} file={file} busy={quickUpload.isPending && quickRole === role} onUpload={() => requestQuickUpload(role)} onPreview={() => file && setPreview(file)} onRemove={() => file && remove.mutate(file.id)} />;
        })}
      </div>
    </section>
    <section>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div><h3 className="text-sm font-semibold">辅助资料</h3><p className="mt-1 text-xs text-muted-foreground">补充产品界面、品牌说明和市场合规文件。</p></div>
        <Button size="sm" variant="outline" onClick={() => setEditing("new")}><Plus className="h-4 w-4" />添加资料</Button>
      </div>
      <div className="border-y">
        {auxiliaryFiles.map((file) => <ResourceFileRow key={file.id} file={file} onPreview={() => setPreview(file)} onEdit={() => setEditing(file)} onRemove={() => remove.mutate(file.id)} />)}
        {!files.isLoading && auxiliaryFiles.length === 0 && <div className="flex min-h-24 items-center justify-center text-sm text-muted-foreground">暂无辅助资料</div>}
      </div>
    </section>
    <ResourceFileDialog resource={resource} file={editing} uploading={uploading} upload={upload} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refresh(); }} />
    <ResourcePreviewDialog file={preview} onClose={() => setPreview(null)} />
    <PrimeLayoutImportDialog open={layoutImportOpen} composition={composition} extraction={extraction.data ?? null} starting={startExtraction.isPending} busy={importLayout.isPending} onClose={() => setLayoutImportOpen(false)} onStart={async (file, width, height) => { await startExtraction.mutateAsync({ file, width, height }); }} onImport={(selection) => importLayout.mutate(selection)} />
  </section>;
}

function PrimeResourceSlot({ label, description, file, busy, onUpload, onPreview, onRemove }: { label: string; description: string; file?: CreativeResourceFile; busy: boolean; onUpload: () => void; onPreview: () => void; onRemove: () => void }) {
  const source = file ? resolvePublicFileUrl(file.url) : "";
  return <div className="grid gap-3 px-3 py-3 sm:grid-cols-[48px_minmax(0,1fr)_auto] sm:items-center">
    <button type="button" disabled={!file} onClick={onPreview} className="flex size-12 items-center justify-center overflow-hidden border bg-muted/20 disabled:cursor-default" aria-label={file ? `预览${label}` : `${label}尚未上传`}>
      {file && source ? <img src={source} alt="" className="h-full w-full object-contain" /> : <ImageIcon className="h-4 w-4 text-muted-foreground" />}
    </button>
    <div className="min-w-0"><div className="flex items-center gap-2"><p className="text-sm font-medium">{label}</p>{file ? <span className="inline-flex items-center gap-1 text-xs text-emerald-700 dark:text-emerald-400"><Check className="h-3.5 w-3.5" />已就绪</span> : <span className="text-xs text-amber-700 dark:text-amber-400">待上传</span>}</div><p className="mt-0.5 truncate text-xs text-muted-foreground">{file ? file.label || file.filename : description}</p></div>
    <div className="flex items-center justify-end gap-1">{file && <Button size="icon-sm" variant="ghost" title={`预览${label}`} onClick={onPreview}><Eye className="h-4 w-4" /></Button>}<Button size="sm" variant={file ? "ghost" : "outline"} disabled={busy} onClick={onUpload}>{file ? <Replace className="h-4 w-4" /> : <Upload className="h-4 w-4" />}{busy ? "上传中" : file ? "替换" : "上传"}</Button>{file && <Button size="icon-sm" variant="ghost" title={`移除${label}`} onClick={onRemove}><Trash2 className="h-4 w-4" /></Button>}</div>
  </div>;
}

function ResourceFileRow({ file, onPreview, onEdit, onRemove }: { file: CreativeResourceFile; onPreview: () => void; onEdit: () => void; onRemove: () => void }) {
  const source = resolvePublicFileUrl(file.url);
  const image = file.content_type.startsWith("image/");
  const description = textMetadata(file.metadata, "description");
  const dimensions = textMetadata(file.metadata, "dimensions");
  return <div className="grid gap-3 border-b px-3 py-3 last:border-b-0 sm:grid-cols-[72px_minmax(0,1fr)_auto] sm:items-center">
    <button type="button" onClick={onPreview} className="flex aspect-square w-[72px] items-center justify-center overflow-hidden border bg-muted/30" title="预览资源文件">
      {image && source ? <img src={source} alt={file.label || file.filename} className="h-full w-full object-contain" /> : <FileText className="h-5 w-5 text-muted-foreground" />}
    </button>
    <div className="min-w-0">
      <div className="flex min-w-0 flex-wrap items-center gap-2"><p className="truncate text-sm font-medium">{file.label || file.filename}</p><Badge variant="outline" className="text-[10px]">{purposeLabel(file.role)}</Badge>{dimensions && <Badge variant="secondary" className="text-[10px]">{dimensions}</Badge>}</div>
      <p className="mt-1 truncate text-xs text-muted-foreground">{description || file.filename}</p>
    </div>
    <div className="flex items-center justify-end gap-1">
      <Button size="icon-sm" variant="ghost" title="预览文件" onClick={onPreview}><Eye className="h-4 w-4" /></Button>
      {source && <a href={source} download={file.filename} title="下载文件" className={buttonVariants({ size: "icon-sm", variant: "ghost" })}><Download className="h-4 w-4" /></a>}
      <Button size="icon-sm" variant="ghost" title="编辑文件信息" onClick={onEdit}><Pencil className="h-4 w-4" /></Button>
      <Button size="icon-sm" variant="ghost" title="移除文件" onClick={onRemove}><Trash2 className="h-4 w-4" /></Button>
    </div>
  </div>;
}

function ResourceFileDialog({ resource, file, uploading, upload, onClose, onSaved }: {
  resource: CreativeResource;
  file: CreativeResourceFile | "new" | null;
  uploading: boolean;
  upload: (file: File) => Promise<{ id: string } | null>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const key = file === "new" ? "new" : file?.id ?? "closed";
  const source = file && file !== "new" ? draftFromFile(file) : EMPTY_DRAFT;
  const [loadedKey, setLoadedKey] = useState(key);
  const [draft, setDraft] = useState<AssetDraft>(source);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  useEffect(() => {
    if (key === loadedKey) return;
    setLoadedKey(key);
    setDraft(source);
    setSelectedFile(null);
  }, [key, loadedKey, source]);
  const save = useMutation({
    mutationFn: async () => {
      const metadata = {
        description: draft.description.trim(), dimensions: draft.dimensions.trim(),
        market: draft.market.trim(), locale: draft.locale.trim(), tags: splitTags(draft.tags),
      };
      if (!file) throw new Error("资源文件尚未加载");
      if (!draft.role.trim()) throw new Error("请选择用途");
      if (LEGACY_PRIME_SHEET_ROLES.has(draft.role.trim())) throw new Error("Prime 已使用独立组件，不再支持上传方形、横版或竖版整图模板");
      if (file === "new") {
        if (!selectedFile) throw new Error("请选择文件");
        const uploaded = await upload(selectedFile);
        if (!uploaded) throw new Error("文件上传失败");
        return api.addCreativeResourceFile(resource.id, { attachment_id: uploaded.id, role: draft.role.trim(), label: draft.label.trim() || selectedFile.name, metadata });
      }
      return api.updateCreativeResourceFile(resource.id, file.id, { role: draft.role.trim(), label: draft.label.trim() || file.filename, metadata });
    },
    onSuccess: () => { onSaved(); toast.success(file === "new" ? "资源文件已添加" : "资源文件信息已更新"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法保存资源文件"),
  });
  const set = (field: keyof AssetDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }));
  return <Dialog open={file !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-2xl"><DialogHeader><DialogTitle>{file === "new" ? "添加辅助资料" : "编辑资料"}</DialogTitle><DialogDescription>选择这份资料在创意生产中的用途。</DialogDescription></DialogHeader>
    <div className="grid gap-4 sm:grid-cols-2">
      {file === "new" && <div className="sm:col-span-2"><input ref={inputRef} type="file" className="hidden" onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)} /><Button type="button" variant="outline" onClick={() => inputRef.current?.click()}><Upload className="h-4 w-4" />{selectedFile?.name || "选择文件"}</Button></div>}
      <AssetField label="用途"><NativeSelect value={draft.role} onChange={(event) => set("role", event.target.value)}><NativeSelectOption value="">选择用途</NativeSelectOption>{draft.role && !PURPOSES.some((purpose) => purpose.role === draft.role) && <NativeSelectOption value={draft.role}>其他用途</NativeSelectOption>}{PURPOSES.filter((purpose) => !PRIME_COMPONENT_ROLES.includes(purpose.role as (typeof PRIME_COMPONENT_ROLES)[number])).map((purpose) => <NativeSelectOption key={purpose.role} value={purpose.role}>{purpose.label}</NativeSelectOption>)}</NativeSelect></AssetField>
      <AssetField label="显示名称"><Input value={draft.label} placeholder="例如 AdaKami 首页" onChange={(event) => set("label", event.target.value)} /></AssetField>
      <AssetField label="尺寸"><Input value={draft.dimensions} placeholder="例如 1080x2160" onChange={(event) => set("dimensions", event.target.value)} /></AssetField>
      <AssetField label="市场"><Input value={draft.market} onChange={(event) => set("market", event.target.value)} /></AssetField>
      <AssetField label="语言"><Input value={draft.locale} onChange={(event) => set("locale", event.target.value)} /></AssetField>
      <AssetField label="标签"><Input value={draft.tags} placeholder="首页, 新客, 借款额度" onChange={(event) => set("tags", event.target.value)} /></AssetField>
      <AssetField label="使用说明" wide><Textarea rows={4} value={draft.description} placeholder="说明智能体在什么情况下使用这份资源" onChange={(event) => set("description", event.target.value)} /></AssetField>
    </div>
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={uploading || save.isPending || !draft.role.trim() || (file === "new" && !selectedFile)} onClick={() => save.mutate()}>保存</Button></DialogFooter>
  </DialogContent></Dialog>;
}

function ResourcePreviewDialog({ file, onClose }: { file: CreativeResourceFile | null; onClose: () => void }) {
  const source = file ? resolvePublicFileUrl(file.url) : "";
  const image = file?.content_type.startsWith("image/") === true;
  const pdf = file?.content_type === "application/pdf";
  return <Dialog open={file !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-h-[94vh] gap-0 overflow-hidden p-0 sm:max-w-[min(94vw,1100px)]"><DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{file?.label || file?.filename}</DialogTitle><DialogDescription className="truncate">{file?.filename}</DialogDescription></DialogHeader>
    <div className={cn("flex h-[min(78vh,820px)] items-center justify-center bg-muted/20 p-3", image && "bg-black")}>{source && image ? <img src={source} alt={file?.label || file?.filename} className="h-full w-full object-contain" /> : source && pdf ? <iframe src={source} title={file?.label || file?.filename} className="h-full w-full border-0 bg-background" /> : <div className="text-center"><ImageIcon className="mx-auto h-8 w-8 text-muted-foreground" /><p className="mt-3 text-sm text-muted-foreground">该文件不支持页面内预览</p>{source && <a href={source} download={file?.filename} className={cn(buttonVariants({ variant: "outline" }), "mt-4")}><Download className="h-4 w-4" />下载文件</a>}</div>}</div>
  </DialogContent></Dialog>;
}

function AssetField({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) {
  return <div className={cn("space-y-1.5", wide && "sm:col-span-2")}><Label className="text-xs text-muted-foreground">{label}</Label>{children}</div>;
}

function draftFromFile(file: CreativeResourceFile): AssetDraft {
  return {
    role: file.role, label: file.label, description: textMetadata(file.metadata, "description"),
    dimensions: textMetadata(file.metadata, "dimensions"), market: textMetadata(file.metadata, "market") || "Indonesia",
    locale: textMetadata(file.metadata, "locale") || "id-ID", tags: arrayMetadata(file.metadata, "tags").join(", "),
  };
}

function textMetadata(metadata: Record<string, unknown>, key: string): string { return typeof metadata[key] === "string" ? metadata[key] : ""; }
function arrayMetadata(metadata: Record<string, unknown>, key: string): string[] { return Array.isArray(metadata[key]) ? metadata[key].filter((value): value is string => typeof value === "string") : []; }
function splitTags(value: string): string[] { return value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean); }

export function purposeLabel(role: string): string {
  return PURPOSES.find((purpose) => purpose.role === role)?.label ?? "其他资料";
}
