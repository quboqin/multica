"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, ImageUp, LoaderCircle, Move, Plus, ScanSearch, Trash2 } from "lucide-react";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { CreativeMarketPackComponentExtraction } from "@multica/core/types";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import type { PrimeComposition, PrimeRect, PrimeSize } from "./prime-composition-editor";
import {
  detectedPrimeImportCandidates,
  inferPrimeImportSize,
  PRIME_IMPORT_SIZE_META,
  scalePrimeImportRect,
  type PrimeImportCandidate,
  updatePrimeImportCandidateRect,
} from "../lib/prime-layout-import";

export type PrimeLayoutImportSource = File | {
  name: string;
  url: string;
  attachmentId: string;
};

export type PrimeLayoutImportSelection = {
  extractionId: string;
  source: PrimeLayoutImportSource;
  size: PrimeSize;
  candidates: PrimeImportCandidate[];
};

type SourceImage = {
  name: string;
  url: string;
  width: number;
  height: number;
};

export function PrimeLayoutImportDialog({
  open,
  composition,
  extraction,
  starting,
  busy,
  onClose,
  onStart,
  onImport,
}: {
  open: boolean;
  composition: PrimeComposition;
  extraction: CreativeMarketPackComponentExtraction | null;
  starting: boolean;
  busy: boolean;
  onClose: () => void;
  onStart: (file: File, width: number, height: number) => Promise<void>;
  onImport: (selection: PrimeLayoutImportSelection) => void;
}) {
  const inputRef = useRef<HTMLInputElement>(null);
  const canvasRef = useRef<HTMLDivElement>(null);
  const drag = useRef<{ id: string; mode: "move" | "resize"; startX: number; startY: number; rect: PrimeRect } | null>(null);
  const [pendingSource, setPendingSource] = useState<SourceImage | null>(null);
  const [fileError, setFileError] = useState("");
  const [size, setSize] = useState<PrimeSize>("1080x1080");
  const [candidates, setCandidates] = useState<PrimeImportCandidate[]>([]);
  const [activeId, setActiveId] = useState("");
  const source = useMemo<SourceImage | null>(() => pendingSource ?? (extraction ? {
    name: extraction.source_filename,
    url: resolvePublicFileUrl(extraction.source_url) ?? extraction.source_url,
    width: extraction.source_width,
    height: extraction.source_height,
  } : null), [extraction, pendingSource]);
  const active = candidates.find((candidate) => candidate.componentId === activeId) ?? candidates[0];
  const selectedCount = candidates.filter((candidate) => candidate.selected).length;
  const selectedHasMissingContent = candidates.some((candidate) => candidate.selected && (
    (candidate.kind === "text" && candidate.content.trim() === "") ||
    (candidate.kind !== "text" && candidate.role.trim() === "")
  ));
  const meta = PRIME_IMPORT_SIZE_META[size];
  const recognizing = starting || extraction?.status === "pending" || extraction?.status === "running";
  const ready = extraction?.status === "completed";

  useEffect(() => () => {
    if (pendingSource?.url) URL.revokeObjectURL(pendingSource.url);
  }, [pendingSource?.url]);

  useEffect(() => {
    if (!extraction || (extraction.status !== "completed" && extraction.status !== "applied")) return;
    const inferred = inferPrimeImportSize(extraction.source_width, extraction.source_height) ?? "1080x1080";
    const next = detectedPrimeImportCandidates(extraction, composition, inferred);
    setSize(inferred);
    setCandidates(next);
    setActiveId(next[0]?.componentId ?? "");
    setPendingSource(null);
  }, [extraction?.id]);

  const selectFile = async (file: File) => {
    setFileError("");
    let width = 0;
    let height = 0;
    try {
      const bitmap = await createImageBitmap(file);
      width = bitmap.width;
      height = bitmap.height;
      bitmap.close();
      const inferred = inferPrimeImportSize(width, height);
      if (!inferred) {
        setFileError("图片比例不属于方形、横版或竖版，请上传完整的标准成图。");
        return;
      }
      setSize(inferred);
      setCandidates([]);
      setActiveId("");
      setPendingSource({ name: file.name, url: URL.createObjectURL(file), width, height });
    } catch {
      setPendingSource(null);
      setFileError("无法读取这张图片，请换一张 PNG、JPG 或 WebP。");
      return;
    }
    try {
      await onStart(file, width, height);
    } catch {
      setPendingSource(null);
      setFileError("图片已读取，但上传或启动识别失败，请重试。");
    }
  };
  const resetCandidates = (nextSize: PrimeSize) => {
    if (!extraction || (extraction.status !== "completed" && extraction.status !== "applied")) return;
    const next = detectedPrimeImportCandidates(extraction, composition, nextSize);
    setCandidates(next);
    setActiveId(next[0]?.componentId ?? "");
  };
  const setCandidate = (componentId: string, update: (candidate: PrimeImportCandidate) => PrimeImportCandidate) => {
    setCandidates((current) => current.map((candidate) => candidate.componentId === componentId ? update(candidate) : candidate));
  };
  const setCandidateRect = (componentId: string, rect: PrimeRect) => {
    setCandidate(componentId, (candidate) => ({ ...candidate, rect }));
  };
  const addCandidate = () => {
    const used = new Set([...composition.components.map((component) => component.id), ...candidates.map((candidate) => candidate.componentId)]);
    let sequence = 1;
    while (used.has(`custom_${sequence}`)) sequence += 1;
    const componentId = `custom_${sequence}`;
    const candidate: PrimeImportCandidate = {
      componentId,
      label: `新组件 ${sequence}`,
      role: `prime_${componentId}`,
      kind: "image",
      content: "",
      rect: [Math.round(meta.width * 0.1), Math.round(meta.height * 0.1), Math.round(meta.width * 0.3), Math.round(meta.height * 0.2)],
      selected: true,
      confidence: 1,
      evidence: ["用户手动补充"],
    };
    setCandidates((current) => [...current, candidate]);
    setActiveId(componentId);
  };
  const removeCandidate = (componentId: string) => {
    setCandidates((current) => {
      const next = current.filter((candidate) => candidate.componentId !== componentId);
      setActiveId(next[0]?.componentId ?? "");
      return next;
    });
  };
  const coordinate = (event: React.PointerEvent) => {
    const bounds = canvasRef.current?.getBoundingClientRect();
    if (!bounds) return { x: 0, y: 0 };
    return {
      x: Math.round((event.clientX - bounds.left) / bounds.width * meta.width),
      y: Math.round((event.clientY - bounds.top) / bounds.height * meta.height),
    };
  };
  const startDrag = (event: React.PointerEvent, candidate: PrimeImportCandidate, mode: "move" | "resize") => {
    event.preventDefault();
    event.stopPropagation();
    setActiveId(candidate.componentId);
    const point = coordinate(event);
    drag.current = { id: candidate.componentId, mode, startX: point.x, startY: point.y, rect: candidate.rect };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const moveDrag = (event: React.PointerEvent) => {
    if (!drag.current) return;
    const point = coordinate(event);
    const current = drag.current;
    const [x1, y1, x2, y2] = current.rect;
    const dx = point.x - current.startX;
    const dy = point.y - current.startY;
    const candidate = candidates.find((entry) => entry.componentId === current.id);
    if (!candidate) return;
    const updated = current.mode === "resize"
      ? updatePrimeImportCandidateRect(candidate, { width: Math.max(1, x2 - x1 + dx), height: Math.max(1, y2 - y1 + dy) }, size)
      : updatePrimeImportCandidateRect(candidate, { x: x1 + dx, y: y1 + dy }, size);
    setCandidateRect(current.id, updated.rect);
  };
  const endDrag = (event: React.PointerEvent) => {
    drag.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };

  return <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
    <DialogContent className="grid h-[94vh] grid-rows-[auto_minmax(0,1fr)_auto] gap-0 overflow-hidden p-0 sm:max-w-[min(96vw,1380px)]">
      <DialogHeader className="border-b px-5 py-4 pr-14">
        <DialogTitle>从完整成图识别品牌组件</DialogTitle>
        <DialogDescription>图片在后台由视觉智能体识别。确认候选后才会提取组件并更新市场包。</DialogDescription>
      </DialogHeader>
      <div className="grid min-h-0 lg:grid-cols-[minmax(0,1fr)_380px]">
        <div className="flex min-h-0 flex-col border-b bg-muted/15 lg:border-b-0 lg:border-r">
          <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-background px-4 py-3">
            <div className="flex flex-wrap items-center gap-2">
              <input ref={inputRef} type="file" accept="image/png,image/jpeg,image/webp" className="hidden" onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) void selectFile(file);
                event.target.value = "";
              }} />
              <Button size="sm" variant="outline" disabled={starting} onClick={() => inputRef.current?.click()}><ImageUp className="h-4 w-4" />{source ? "上传另一张" : "上传完整成图"}</Button>
              {ready && <NativeSelect value={size} className="h-8 w-36" onChange={(event) => {
                const next = event.target.value as PrimeSize;
                setSize(next);
                resetCandidates(next);
              }}>{(Object.keys(PRIME_IMPORT_SIZE_META) as PrimeSize[]).map((key) => <NativeSelectOption key={key} value={key}>{PRIME_IMPORT_SIZE_META[key].label} · {key}</NativeSelectOption>)}</NativeSelect>}
            </div>
            {source && <span className="text-xs text-muted-foreground">{source.name} · {source.width} × {source.height}</span>}
          </div>
          <div className="flex min-h-0 flex-1 items-center justify-center overflow-auto p-4">
            {!source ? <button type="button" onClick={() => inputRef.current?.click()} className="flex min-h-80 w-full max-w-2xl flex-col items-center justify-center border border-dashed bg-background text-center hover:bg-muted/20"><ScanSearch className="h-8 w-8 text-muted-foreground" /><span className="mt-4 text-sm font-medium">上传一张带完整品牌信息的成图</span><span className="mt-1 text-xs text-muted-foreground">支持 PNG、JPG 和 WebP</span></button> : <div ref={canvasRef} onPointerMove={moveDrag} onPointerUp={endDrag} className="relative max-h-full max-w-full shrink-0 touch-none overflow-hidden border bg-background shadow-sm" style={{ aspectRatio: `${source.width} / ${source.height}`, width: source.width >= source.height ? "min(100%, 980px)" : "min(70%, 720px)" }}>
              <img src={source.url} alt="品牌布局来源成图" className="block h-full w-full object-fill" />
              {candidates.map((candidate, index) => {
                const isActive = candidate.componentId === active?.componentId;
                return <div key={candidate.componentId} role="button" tabIndex={0} aria-label={`调整${candidate.label}识别区域`} onClick={() => setActiveId(candidate.componentId)} onPointerDown={(event) => startDrag(event, candidate, "move")} className={cn("absolute cursor-move border text-[10px] font-medium outline-none", candidate.selected ? "border-emerald-600 bg-emerald-500/10 text-emerald-950 dark:text-emerald-100" : "border-dashed border-muted-foreground/60 bg-background/10 text-foreground", isActive && "z-20 border-2 border-primary bg-primary/10")} style={primeRectStyle(candidate.rect, meta.width, meta.height)}><span className="absolute -top-px left-0 flex max-w-full -translate-y-full items-center gap-1 bg-background/95 px-1.5 py-0.5 shadow-sm"><span className="inline-flex size-4 shrink-0 items-center justify-center bg-foreground text-[9px] text-background">{index + 1}</span><span className="truncate">{candidate.label}</span></span>{isActive && <span aria-hidden="true" onPointerDown={(event) => startDrag(event, candidate, "resize")} className="absolute -bottom-1.5 -right-1.5 size-3 cursor-nwse-resize border-2 border-background bg-primary shadow-sm" />}</div>;
              })}
              {recognizing && <div className="absolute inset-0 flex items-center justify-center bg-background/75 backdrop-blur-[1px]"><div className="border bg-background px-5 py-4 text-center shadow-sm"><LoaderCircle className="mx-auto h-5 w-5 animate-spin" /><p className="mt-2 text-sm font-medium">视觉智能体识别中</p><p className="mt-1 text-xs text-muted-foreground">可以关闭窗口，后台会继续运行</p></div></div>}
            </div>}
          </div>
          <div className="flex items-center gap-2 border-t bg-background px-4 py-2 text-xs text-muted-foreground"><Move className="h-3.5 w-3.5" />识别完成后可拖动候选框，右下角调整大小。</div>
        </div>
        <div className="min-h-0 overflow-y-auto bg-background">
          <div className="flex items-start justify-between gap-3 border-b px-4 py-3"><div><p className="text-sm font-semibold">组件候选</p><p className="mt-1 text-xs text-muted-foreground">候选数量由图片决定，确认前可增删。</p></div>{ready && <Button size="sm" variant="outline" onClick={addCandidate}><Plus className="h-4 w-4" />补充组件</Button>}</div>
          {fileError && <div className="m-4 flex gap-2 border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive"><AlertTriangle className="h-4 w-4 shrink-0" />{fileError}</div>}
          {extraction?.status === "failed" && <div className="m-4 flex gap-2 border border-destructive/30 bg-destructive/5 p-3 text-xs text-destructive"><AlertTriangle className="h-4 w-4 shrink-0" /><span><span className="block font-medium">识别失败</span><span className="mt-1 block">{extraction.error_message || "请重新上传后再试。"}</span></span></div>}
          {recognizing && <div className="space-y-3 p-4"><div className="h-16 animate-pulse bg-muted" /><div className="h-16 animate-pulse bg-muted" /><div className="h-16 animate-pulse bg-muted" /></div>}
          {!recognizing && ready && candidates.length === 0 && <div className="p-8 text-center text-sm text-muted-foreground">没有发现可复用组件。可以手动补充候选。</div>}
          {!recognizing && candidates.length > 0 && <div className="divide-y">{candidates.map((candidate, index) => <div key={candidate.componentId} className={cn("grid grid-cols-[22px_64px_minmax(0,1fr)_32px] items-center gap-3 px-4 py-3 hover:bg-muted/30", candidate.componentId === active?.componentId && "bg-muted/40")}>
            <Checkbox checked={candidate.selected} onCheckedChange={(checked) => setCandidate(candidate.componentId, (current) => ({ ...current, selected: checked === true }))} aria-label={`${candidate.selected ? "取消" : "选择"}${candidate.label}`} />
            <button type="button" onClick={() => setActiveId(candidate.componentId)}><CropPreview source={source} rect={candidate.rect} canvas={meta} /></button>
            <button type="button" onClick={() => setActiveId(candidate.componentId)} className="min-w-0 text-left"><span className="flex items-center gap-2 text-sm font-medium"><span className="inline-flex size-4 shrink-0 items-center justify-center bg-foreground text-[9px] text-background">{index + 1}</span><span className="truncate">{candidate.label}</span></span><span className="mt-1 block truncate text-[10px] text-muted-foreground">{candidate.kind === "text" ? "可编辑文字" : candidate.role || "新图片组件"} · 置信度 {Math.round(candidate.confidence * 100)}%</span></button>
            <Button size="icon-sm" variant="ghost" title={`删除${candidate.label}`} aria-label={`删除${candidate.label}`} onClick={() => removeCandidate(candidate.componentId)}><Trash2 className="h-4 w-4" /></Button>
          </div>)}</div>}
          {active && <div className="space-y-4 border-t px-4 py-4">
            <div><Label className="text-xs text-muted-foreground">组件名称</Label><Input className="mt-1.5" value={active.label} onChange={(event) => setCandidate(active.componentId, (candidate) => ({ ...candidate, label: event.target.value }))} /></div>
            {active.componentId.startsWith("custom_") && <div><Label className="text-xs text-muted-foreground">组件类型</Label><NativeSelect className="mt-1.5" value={active.kind} onChange={(event) => setCandidate(active.componentId, (candidate) => {
              const kind = event.target.value as "image" | "text";
              return { ...candidate, kind, role: kind === "text" ? "" : candidate.role || `prime_${candidate.componentId}` };
            })}><NativeSelectOption value="image">图片组件</NativeSelectOption><NativeSelectOption value="text">文字组件</NativeSelectOption></NativeSelect></div>}
            {active.kind === "text" && <div><Label className="text-xs text-muted-foreground">显示文字</Label><Textarea className="mt-1.5" rows={4} value={active.content} onChange={(event) => setCandidate(active.componentId, (candidate) => ({ ...candidate, content: event.target.value }))} /></div>}
            <div><div className="flex items-center justify-between"><Label className="text-xs text-muted-foreground">识别边界</Label><span className="text-[10px] text-muted-foreground">{meta.width} × {meta.height}</span></div><div className="mt-2 grid grid-cols-2 gap-2"><RectInput label="左" value={active.rect[0]} max={meta.width} onChange={(value) => setCandidate(active.componentId, (candidate) => updatePrimeImportCandidateRect(candidate, { x: value }, size))} /><RectInput label="上" value={active.rect[1]} max={meta.height} onChange={(value) => setCandidate(active.componentId, (candidate) => updatePrimeImportCandidateRect(candidate, { y: value }, size))} /><RectInput label="宽" value={active.rect[2] - active.rect[0]} max={meta.width} onChange={(value) => setCandidate(active.componentId, (candidate) => updatePrimeImportCandidateRect(candidate, { width: value }, size))} /><RectInput label="高" value={active.rect[3] - active.rect[1]} max={meta.height} onChange={(value) => setCandidate(active.componentId, (candidate) => updatePrimeImportCandidateRect(candidate, { height: value }, size))} /></div></div>
            {active.evidence.length > 0 && <p className="text-xs text-muted-foreground">{active.evidence.join("；")}</p>}
          </div>}
        </div>
      </div>
      <DialogFooter className="border-t bg-background px-5 py-3 sm:justify-between">
        <p className={cn("mr-auto text-xs text-muted-foreground", selectedHasMissingContent && "text-amber-700 dark:text-amber-400")}>{selectedHasMissingContent ? "选中的组件信息还不完整。" : "确认后才会生成组件文件，并按比例写入三个尺寸。"}</p>
        <Button variant="outline" onClick={onClose}>关闭</Button>
        <Button disabled={!ready || selectedCount === 0 || selectedHasMissingContent || busy} onClick={() => extraction && onImport({ extractionId: extraction.id, source: { name: extraction.source_filename, url: extraction.source_url, attachmentId: extraction.source_attachment_id }, size, candidates })}>{busy ? "正在保存" : `确认并提取 ${selectedCount} 个`}</Button>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}

export async function extractPrimeComponentFiles(selection: PrimeLayoutImportSelection): Promise<Array<{ candidate: PrimeImportCandidate; file: File }>> {
  const sourceBlob = selection.source instanceof File
    ? selection.source
    : await fetch(resolvePublicFileUrl(selection.source.url) ?? selection.source.url, { credentials: "include" }).then((response) => {
      if (!response.ok) throw new Error("无法读取识别来源图片");
      return response.blob();
    });
  const bitmap = await createImageBitmap(sourceBlob);
  try {
    return await Promise.all(selection.candidates.filter((candidate) => candidate.selected && candidate.kind !== "text").map(async (candidate) => {
      const [left, top, right, bottom] = scalePrimeImportRect(candidate.rect, selection.size, bitmap.width, bitmap.height);
      const canvas = document.createElement("canvas");
      canvas.width = Math.max(1, right - left);
      canvas.height = Math.max(1, bottom - top);
      const context = canvas.getContext("2d", { willReadFrequently: candidate.kind !== "qr" });
      if (!context) throw new Error("当前浏览器无法处理图片");
      context.drawImage(bitmap, left, top, canvas.width, canvas.height, 0, 0, canvas.width, canvas.height);
      if (candidate.kind !== "qr") removeConnectedLightBackground(context, canvas.width, canvas.height);
      const blob = await canvasBlob(canvas);
      const sourceName = selection.source.name.replace(/\.[^.]+$/, "").replace(/[^a-zA-Z0-9_-]+/g, "-") || "brand-layout";
      return { candidate, file: new File([blob], `${sourceName}-${candidate.role}.png`, { type: "image/png" }) };
    }));
  } finally {
    bitmap.close();
  }
}

function CropPreview({ source, rect, canvas }: { source: SourceImage | null; rect: PrimeRect; canvas: { width: number; height: number } }) {
  const width = Math.max(1, rect[2] - rect[0]);
  const height = Math.max(1, rect[3] - rect[1]);
  return <span className="relative flex h-12 w-16 items-center justify-center overflow-hidden border bg-muted/20">{source ? <img src={source.url} alt="" className="pointer-events-none absolute max-w-none" style={{ width: `${canvas.width / width * 100}%`, height: `${canvas.height / height * 100}%`, left: `${-rect[0] / width * 100}%`, top: `${-rect[1] / height * 100}%` }} /> : <ScanSearch className="h-4 w-4 text-muted-foreground" />}</span>;
}

function RectInput({ label, value, max, onChange }: { label: string; value: number; max: number; onChange: (value: number) => void }) {
  return <label className="grid grid-cols-[24px_minmax(0,1fr)] items-center gap-1 text-[11px] text-muted-foreground"><span>{label}</span><Input type="number" min={0} max={max} value={value} className="h-8 text-xs" onChange={(event) => onChange(Number(event.target.value) || 0)} /></label>;
}

function primeRectStyle(rect: PrimeRect, width: number, height: number): React.CSSProperties {
  return { left: `${rect[0] / width * 100}%`, top: `${rect[1] / height * 100}%`, width: `${(rect[2] - rect[0]) / width * 100}%`, height: `${(rect[3] - rect[1]) / height * 100}%` };
}

function canvasBlob(canvas: HTMLCanvasElement): Promise<Blob> {
  return new Promise((resolve, reject) => canvas.toBlob((blob) => blob ? resolve(blob) : reject(new Error("无法生成组件图片")), "image/png"));
}

function removeConnectedLightBackground(context: CanvasRenderingContext2D, width: number, height: number): void {
  const image = context.getImageData(0, 0, width, height);
  const pixels = image.data;
  const visited = new Uint8Array(width * height);
  const queue = new Int32Array(width * height);
  let head = 0;
  let tail = 0;
  const enqueue = (index: number) => {
    if (visited[index] === 1 || !isLightNeutralPixel(pixels, index)) return;
    visited[index] = 1;
    queue[tail++] = index;
  };
  for (let x = 0; x < width; x += 1) { enqueue(x); enqueue((height - 1) * width + x); }
  for (let y = 0; y < height; y += 1) { enqueue(y * width); enqueue(y * width + width - 1); }
  while (head < tail) {
    const index = queue[head++]!;
    pixels[index * 4 + 3] = 0;
    const x = index % width;
    const y = Math.floor(index / width);
    if (x > 0) enqueue(index - 1);
    if (x + 1 < width) enqueue(index + 1);
    if (y > 0) enqueue(index - width);
    if (y + 1 < height) enqueue(index + width);
  }
  context.putImageData(image, 0, 0);
}

function isLightNeutralPixel(pixels: Uint8ClampedArray, index: number): boolean {
  const offset = index * 4;
  const red = pixels[offset] ?? 0;
  const green = pixels[offset + 1] ?? 0;
  const blue = pixels[offset + 2] ?? 0;
  return Math.min(red, green, blue) >= 225 && Math.max(red, green, blue) - Math.min(red, green, blue) <= 22;
}
