"use client";

import { useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { AlertTriangle, Download, ExternalLink, ImageIcon, LoaderCircle, Plus, RefreshCw, Search, Upload } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeKeys, creativeMaterialLibraryOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { CreativeMaterialCandidate } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button, buttonVariants } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";

type ImportDraft = {
  mode: "file" | "url";
  sourceUrl: string;
  title: string;
  competitor: string;
  tags: string;
  note: string;
};

const EMPTY_IMPORT: ImportDraft = {
  mode: "file",
  sourceUrl: "",
  title: "",
  competitor: "",
  tags: "",
  note: "",
};

export function CreativeMaterialLibrary() {
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const materials = useQuery({
    ...creativeMaterialLibraryOptions(wsId),
    refetchInterval: (query) => query.state.data?.candidates.some((item) => item.archive_status === "pending" || item.archive_status === "running") ? 3000 : false,
  });
  const [query, setQuery] = useState("");
  const [competitor, setCompetitor] = useState("");
  const [preview, setPreview] = useState<CreativeMaterialCandidate | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const candidates = useMemo(() => materials.data?.candidates ?? [], [materials.data?.candidates]);
  const failed = candidates.filter((item) => !item.archived_url && item.archive_status === "failed");
  const retryArchives = useMutation({
    mutationFn: (candidateIds: string[]) => api.retryCreativeMaterialArchives(candidateIds),
    onSuccess: (result) => {
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
      toast.success(`已重新安排 ${result.scheduled_count} 条素材归档`);
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法重新归档素材"),
  });
  const competitors = useMemo(
    () => Array.from(new Set(candidates.map((item) => item.competitor).filter(Boolean))).sort(),
    [candidates],
  );
  const visible = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase();
    return candidates.filter((item) => {
      if (competitor && item.competitor !== competitor) return false;
      if (!needle) return true;
      return [item.title, item.competitor, item.connector_id, ...item.tags]
        .join(" ")
        .toLocaleLowerCase()
        .includes(needle);
    });
  }, [candidates, competitor, query]);

  return <div className="mx-auto max-w-[1440px]">
    <div className="mb-4 flex flex-wrap items-end justify-between gap-4">
      <div>
        <h2 className="text-base font-semibold">工作区素材库</h2>
        <p className="mt-1 text-sm text-muted-foreground">AppGrowing 采集和人工导入的素材统一归档，候选池从这里引用稳定文件。</p>
      </div>
      <div className="flex items-center gap-2">
        <Badge variant="outline">{visible.length} / {candidates.length} 条</Badge>
        {failed.length > 0 && <Button size="sm" variant="outline" disabled={retryArchives.isPending} onClick={() => retryArchives.mutate(failed.map((item) => item.id))}><RefreshCw className={retryArchives.isPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重试归档 ({failed.length})</Button>}
        <Button size="sm" onClick={() => setImportOpen(true)}><Plus className="h-4 w-4" />导入素材</Button>
      </div>
    </div>
    <div className="mb-4 flex flex-wrap gap-2 border-y bg-muted/10 py-3">
      <div className="relative min-w-56 flex-1 md:max-w-md"><Search className="pointer-events-none absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" /><Input className="pl-8" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索标题、竞品或标签" /></div>
      <select className="h-9 min-w-40 border bg-background px-3 text-sm" value={competitor} onChange={(event) => setCompetitor(event.target.value)} aria-label="按竞品筛选">
        <option value="">全部竞品</option>
        {competitors.map((name) => <option key={name} value={name}>{name}</option>)}
      </select>
    </div>
    {materials.isLoading ? <div className="py-16 text-center text-sm text-muted-foreground">正在加载素材库...</div> : visible.length === 0 ? (
      <div className="flex min-h-64 flex-col items-center justify-center border border-dashed text-sm text-muted-foreground"><ImageIcon className="mb-3 h-6 w-6" /><p>没有符合条件的素材</p><Button className="mt-4" size="sm" variant="outline" onClick={() => setImportOpen(true)}>导入第一条素材</Button></div>
    ) : (
      <div className="grid grid-cols-2 gap-px overflow-hidden border bg-border sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5">
        {visible.map((candidate) => <MaterialTile key={candidate.id} candidate={candidate} onOpen={() => setPreview(candidate)} />)}
      </div>
    )}
    <MaterialPreview candidate={candidates.find((item) => item.id === preview?.id) ?? preview} onClose={() => setPreview(null)} onRetry={(candidateId) => retryArchives.mutate([candidateId])} retrying={retryArchives.isPending} />
    <MaterialImportDialog open={importOpen} onClose={() => setImportOpen(false)} onImported={() => {
      setImportOpen(false);
      queryClient.invalidateQueries({ queryKey: creativeKeys.materials(wsId) });
    }} />
  </div>;
}

function MaterialTile({ candidate, onOpen }: { candidate: CreativeMaterialCandidate; onOpen: () => void }) {
  const source = candidateSource(candidate);
  return <button type="button" onClick={onOpen} className="min-w-0 bg-background text-left transition-colors hover:bg-muted/30">
    <div className="relative aspect-[4/3] bg-muted/40">
      {source ? <img src={source} alt={candidate.title || candidate.competitor} className="h-full w-full object-contain" /> : <div className="flex h-full flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon className="h-5 w-5" />{candidate.archive_status === "failed" ? "归档失败" : "正在归档"}</div>}
      {candidate.archive_status === "failed" && <Badge variant="destructive" className="absolute left-2 top-2"><AlertTriangle className="h-3 w-3" />归档失败</Badge>}
      {(candidate.archive_status === "pending" || candidate.archive_status === "running") && <Badge variant="secondary" className="absolute left-2 top-2 bg-background/90"><LoaderCircle className="h-3 w-3 animate-spin" />归档中</Badge>}
    </div>
    <div className="space-y-1 border-t px-3 py-2.5">
      <p className="truncate text-sm font-medium">{candidate.title || "未命名素材"}</p>
      <div className="flex items-center justify-between gap-2 text-xs text-muted-foreground"><span className="truncate">{candidate.competitor || sourceLabel(candidate.connector_id)}</span><span className="shrink-0">{candidate.area_names[0] ?? ""}</span></div>
    </div>
  </button>;
}

function MaterialPreview({ candidate, onClose, onRetry, retrying }: { candidate: CreativeMaterialCandidate | null; onClose: () => void; onRetry: (candidateId: string) => void; retrying: boolean }) {
  const source = candidate ? candidateSource(candidate) : "";
  const original = candidate ? resolvePublicFileUrl(candidate.original_url) : "";
  return <Dialog open={candidate !== null} onOpenChange={(open) => !open && onClose()}><DialogContent className="max-h-[94vh] overflow-hidden p-0 sm:max-w-[min(94vw,1100px)]">
    <DialogHeader className="border-b px-4 py-3 pr-12"><DialogTitle className="truncate text-sm">{candidate?.title || "素材详情"}</DialogTitle><DialogDescription>{candidate?.competitor || sourceLabel(candidate?.connector_id ?? "")}</DialogDescription></DialogHeader>
    <div className="grid max-h-[82vh] overflow-y-auto md:grid-cols-[minmax(0,1fr)_300px]">
      <div className="flex min-h-80 items-center justify-center bg-black p-3">{source ? <img src={source} alt={candidate?.title || "素材"} className="max-h-[72vh] w-full object-contain" /> : <ImageIcon className="h-8 w-8 text-white/50" />}</div>
      <div className="space-y-5 p-4 text-sm">
        <MetadataRow label="来源" value={sourceLabel(candidate?.connector_id ?? "")} />
        <MetadataRow label="市场" value={candidate?.area_names.join("、") || "未填写"} />
        <MetadataRow label="语言" value={candidate?.language_names.join("、") || "未填写"} />
        <MetadataRow label="设备" value={candidate?.platform_names.join("、") || "未填写"} />
        <MetadataRow label="标签" value={candidate?.tags.join("、") || "未填写"} />
        <MetadataRow label="备注" value={candidate?.note || "未填写"} />
        {candidate?.archive_status === "failed" && <div className="border border-destructive/30 bg-destructive/5 p-3"><p className="flex items-center gap-2 text-sm font-medium text-destructive"><AlertTriangle className="h-4 w-4" />稳定归档失败</p><p className="mt-2 break-words text-xs text-muted-foreground">{archiveErrorLabel(candidate.archive_error)}</p><Button className="mt-3" size="sm" variant="outline" disabled={retrying} onClick={() => onRetry(candidate.id)}><RefreshCw className={retrying ? "h-4 w-4 animate-spin" : "h-4 w-4"} />重新归档</Button></div>}
        <div className="flex flex-wrap gap-2 border-t pt-4">
          {source && <a href={source} download className={buttonVariants({ size: "sm", variant: "outline" })}><Download className="h-4 w-4" />下载归档文件</a>}
          {original && <a href={original} target="_blank" rel="noreferrer" className={buttonVariants({ size: "sm", variant: "outline" })}><ExternalLink className="h-4 w-4" />查看来源</a>}
        </div>
      </div>
    </div>
  </DialogContent></Dialog>;
}

function MaterialImportDialog({ open, onClose, onImported }: { open: boolean; onClose: () => void; onImported: () => void }) {
  const { upload, uploading } = useFileUpload(api);
  const fileRef = useRef<HTMLInputElement>(null);
  const [draft, setDraft] = useState<ImportDraft>(EMPTY_IMPORT);
  const [file, setFile] = useState<File | null>(null);
  const save = useMutation({
    mutationFn: async () => {
      let attachmentId: string | undefined;
      if (draft.mode === "file") {
        if (!file) throw new Error("请选择要导入的图片或视频");
        const uploaded = await upload(file);
        if (!uploaded) throw new Error("文件上传失败");
        attachmentId = uploaded.id;
      } else if (!draft.sourceUrl.trim()) {
        throw new Error("请输入素材 URL");
      }
      return api.importCreativeMaterialLibrary({
        attachment_id: attachmentId,
        source_url: draft.mode === "url" ? draft.sourceUrl.trim() : undefined,
        title: draft.title.trim() || file?.name,
        competitor: draft.competitor.trim(),
        asset_type: file?.type.startsWith("video/") ? "video" : "image",
        area_names: ["Indonesia"],
        language_names: ["Indonesian"],
        tags: splitTags(draft.tags),
        note: draft.note.trim(),
      });
    },
    onSuccess: () => { setDraft(EMPTY_IMPORT); setFile(null); onImported(); toast.success("素材已导入工作区素材库"); },
    onError: (error) => toast.error(error instanceof Error ? error.message : "无法导入素材"),
  });
  const set = (field: keyof ImportDraft, value: string) => setDraft((current) => ({ ...current, [field]: value }));
  return <Dialog open={open} onOpenChange={(next) => !next && onClose()}><DialogContent className="max-w-xl"><DialogHeader><DialogTitle>导入素材</DialogTitle><DialogDescription>导入后成为工作区素材，后续可以被创意 Issue 候选池引用。</DialogDescription></DialogHeader>
    <div className="flex w-fit border p-0.5"><Button size="sm" variant={draft.mode === "file" ? "secondary" : "ghost"} onClick={() => set("mode", "file")}>本地文件</Button><Button size="sm" variant={draft.mode === "url" ? "secondary" : "ghost"} onClick={() => set("mode", "url")}>素材 URL</Button></div>
    <div className="grid gap-4 sm:grid-cols-2">
      {draft.mode === "file" ? <div className="sm:col-span-2"><input ref={fileRef} type="file" accept="image/*,video/*" className="hidden" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /><Button variant="outline" onClick={() => fileRef.current?.click()}><Upload className="h-4 w-4" />{file?.name || "选择图片或视频"}</Button></div> : <Field label="素材 URL" wide><Input value={draft.sourceUrl} onChange={(event) => set("sourceUrl", event.target.value)} placeholder="https://..." /></Field>}
      <Field label="标题"><Input value={draft.title} onChange={(event) => set("title", event.target.value)} placeholder="素材名称" /></Field>
      <Field label="竞品 / 来源"><Input value={draft.competitor} onChange={(event) => set("competitor", event.target.value)} placeholder="例如 Easycash" /></Field>
      <Field label="标签" wide><Input value={draft.tags} onChange={(event) => set("tags", event.target.value)} placeholder="跑量, 首页, 利率" /></Field>
      <Field label="备注" wide><Textarea rows={4} value={draft.note} onChange={(event) => set("note", event.target.value)} placeholder="记录用途、选材原因或后续注意事项" /></Field>
    </div>
    <DialogFooter><Button variant="outline" onClick={onClose}>取消</Button><Button disabled={uploading || save.isPending || (draft.mode === "file" ? !file : !draft.sourceUrl.trim())} onClick={() => save.mutate()}>导入</Button></DialogFooter>
  </DialogContent></Dialog>;
}

function Field({ label, children, wide = false }: { label: string; children: React.ReactNode; wide?: boolean }) {
  return <div className={wide ? "space-y-1.5 sm:col-span-2" : "space-y-1.5"}><Label className="text-xs text-muted-foreground">{label}</Label>{children}</div>;
}

function MetadataRow({ label, value }: { label: string; value: string }) {
  return <div><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 break-words">{value}</p></div>;
}

function candidateSource(candidate: CreativeMaterialCandidate): string {
  return resolvePublicFileUrl(candidate.archived_url || candidate.poster_url || candidate.preview_url) ?? "";
}

function sourceLabel(connector: string): string {
  if (connector === "manual_upload") return "人工上传";
  if (connector === "manual_url") return "URL 导入";
  return connector || "未知来源";
}

function archiveErrorLabel(error: string): string {
  const normalized = error.toLocaleLowerCase();
  if (normalized.includes("lookup") || normalized.includes("no such host")) return "素材源站暂时无法解析。可直接重试；若源链接已过期，请重新执行采集以刷新链接。";
  if (normalized.includes("http 401") || normalized.includes("http 403")) return "素材源链接授权已过期，请重新执行采集以刷新链接。";
  if (normalized.includes("http 404")) return "素材源文件已失效，请重新采集或人工导入。";
  return error || "归档没有完成，可先重试；仍失败时重新采集以刷新素材源链接。";
}

function splitTags(value: string): string[] {
  return value.split(/[,，]/).map((tag) => tag.trim()).filter(Boolean);
}
