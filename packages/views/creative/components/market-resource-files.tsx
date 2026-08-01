"use client";

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, Eye, FileText, ImageIcon, Pencil, Plus, Trash2, Upload } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, creativeResourceFilesOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { CreativeResource, CreativeResourceFile } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { toast } from "sonner";

const PURPOSES = [
  { role: "prime_square", label: "Prime 方图模板" },
  { role: "prime_landscape", label: "Prime 横图模板" },
  { role: "prime_portrait", label: "Prime 竖图模板" },
  { role: "app_ui_reference", label: "App UI 参考图" },
  { role: "brand_asset", label: "品牌素材" },
  { role: "brand_guideline", label: "品牌与版式说明" },
  { role: "compliance_reference", label: "市场合规材料" },
] as const;

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

export function MarketResourceFiles({ resource }: { resource: CreativeResource }) {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const { upload, uploading } = useFileUpload(api);
  const files = useQuery(creativeResourceFilesOptions(wsId, resource.id));
  const [editing, setEditing] = useState<CreativeResourceFile | "new" | null>(null);
  const [preview, setPreview] = useState<CreativeResourceFile | null>(null);
  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: creativeKeys.resourceFiles(wsId, resource.id) });
    queryClient.invalidateQueries({ queryKey: creativeKeys.resources(wsId) });
  };
  const remove = useMutation({
    mutationFn: (fileId: string) => api.removeCreativeResourceFile(resource.id, fileId),
    onSuccess: () => { refresh(); toast.success("资源文件已从新版本移除"); },
    onError: () => toast.error("无法移除资源文件"),
  });

  return <section>
    <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
      <div><h3 className="text-sm font-semibold">资源文件</h3><p className="mt-1 text-xs text-muted-foreground">文件会随已发布版本进入 issue 快照。用途支持自定义，同一用途可以添加多个文件。</p></div>
      <Button size="sm" variant="outline" onClick={() => setEditing("new")}><Plus className="h-4 w-4" />添加文件</Button>
    </div>
    <div className="border-y">
      {(files.data?.files ?? []).map((file) => <ResourceFileRow key={file.id} file={file} onPreview={() => setPreview(file)} onEdit={() => setEditing(file)} onRemove={() => remove.mutate(file.id)} />)}
      {!files.isLoading && (files.data?.files.length ?? 0) === 0 && <div className="flex min-h-32 items-center justify-center text-sm text-muted-foreground">还没有资源文件</div>}
    </div>
    <ResourceFileDialog resource={resource} file={editing} uploading={uploading} upload={upload} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); refresh(); }} />
    <ResourcePreviewDialog file={preview} onClose={() => setPreview(null)} />
  </section>;
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
      <div className="flex min-w-0 flex-wrap items-center gap-2"><p className="truncate text-sm font-medium">{file.label || file.filename}</p><Badge variant="outline" className="font-mono text-[10px]">{file.role}</Badge>{dimensions && <Badge variant="secondary" className="text-[10px]">{dimensions}</Badge>}</div>
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
      if (!draft.role.trim()) throw new Error("请填写资源用途");
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
  return <Dialog open={file !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-w-2xl"><DialogHeader><DialogTitle>{file === "new" ? "添加资源文件" : "编辑资源文件"}</DialogTitle><DialogDescription>资源用途用于智能体查找文件，可以使用建议值，也可以输入新的用途。</DialogDescription></DialogHeader>
    <div className="grid gap-4 sm:grid-cols-2">
      {file === "new" && <div className="sm:col-span-2"><input ref={inputRef} type="file" className="hidden" onChange={(event) => setSelectedFile(event.target.files?.[0] ?? null)} /><Button type="button" variant="outline" onClick={() => inputRef.current?.click()}><Upload className="h-4 w-4" />{selectedFile?.name || "选择文件"}</Button></div>}
      <AssetField label="资源用途"><Input list="creative-resource-purposes" value={draft.role} placeholder="例如 app_ui_reference" onChange={(event) => set("role", event.target.value)} /><datalist id="creative-resource-purposes">{PURPOSES.map((purpose) => <option key={purpose.role} value={purpose.role}>{purpose.label}</option>)}</datalist></AssetField>
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
