"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Check, Download, Expand, Info, Minus, MousePointer2, PencilRuler, Plus, SquareDashedMousePointer, Trash2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";

export type CreativeAnnotationDraft = {
  kind: "point" | "rect";
  x: number;
  y: number;
  width: number;
  height: number;
  issueType: string;
  scope: "size" | "variant" | "order";
  comment: string;
};

type PendingCreativeAnnotation = CreativeAnnotationDraft & { localId: string };

export type CreativeComparisonAsset = { id: string; label: string; finalUrl: string; baseUrl?: string; thumbnailUrl?: string; size?: string; variant?: string };

const ANNOTATION_SCOPE_LABELS: Record<CreativeAnnotationDraft["scope"], string> = {
  size: "当前尺寸",
  variant: "当前变体三尺寸",
  order: "全部变体",
};

export function normalizeCreativeAnnotation(
  start: { x: number; y: number },
  end: { x: number; y: number },
  bounds: { width: number; height: number },
  kind: "point" | "rect",
): Pick<CreativeAnnotationDraft, "kind" | "x" | "y" | "width" | "height"> {
  const normalise = (value: number, total: number) => Number(Math.max(0, Math.min(1, value / Math.max(total, 1))).toFixed(6));
  const x1 = normalise(start.x, bounds.width);
  const y1 = normalise(start.y, bounds.height);
  const x2 = normalise(end.x, bounds.width);
  const y2 = normalise(end.y, bounds.height);
  return kind === "point"
    ? { kind, x: x1, y: y1, width: 0, height: 0 }
    : { kind, x: Math.min(x1, x2), y: Math.min(y1, y2), width: Number(Math.abs(x2 - x1).toFixed(6)), height: Number(Math.abs(y2 - y1).toFixed(6)) };
}

export function containedImageBounds(container: { width: number; height: number }, image: { width: number; height: number }) {
  const scale = Math.min(container.width / Math.max(image.width, 1), container.height / Math.max(image.height, 1));
  const width = image.width * scale;
  const height = image.height * scale;
  return { left: (container.width - width) / 2, top: (container.height - height) / 2, width, height };
}

export function transformedContainedImageBounds(
  container: { width: number; height: number },
  image: { width: number; height: number },
  zoom: number,
  pan: { x: number; y: number },
) {
  const bounds = containedImageBounds(container, image);
  const centerX = container.width / 2;
  const centerY = container.height / 2;
  return {
    left: centerX + (bounds.left - centerX) * zoom + pan.x,
    top: centerY + (bounds.top - centerY) * zoom + pan.y,
    width: bounds.width * zoom,
    height: bounds.height * zoom,
  };
}

export function normalizeCreativeAnnotationInImage(
  start: { x: number; y: number },
  end: { x: number; y: number },
  container: { width: number; height: number },
  image: { width: number; height: number },
  kind: "point" | "rect",
) {
  const bounds = containedImageBounds(container, image);
  return normalizeCreativeAnnotation(
    { x: start.x - bounds.left, y: start.y - bounds.top },
    { x: end.x - bounds.left, y: end.y - bounds.top },
    { width: bounds.width, height: bounds.height },
    kind,
  );
}

export function CreativeComparisonWorkspace({
  source,
  result,
  assets,
  onAssetChange,
  onAdjust,
  onViewInfo,
  onDecision,
  onAnnotations,
  annotationScopes = ["size", "variant"],
  acceptance,
  showDecisionActions = true,
  comparisonMode = "source",
}: {
  source: { label: string; url: string };
  result: CreativeComparisonAsset;
  assets: CreativeComparisonAsset[];
  onAssetChange: (id: string) => void;
  onAdjust?: () => void;
  onViewInfo?: () => void;
  onDecision?: (decision: "accepted" | "abandoned" | "downloaded") => void;
  onAnnotations?: (annotations: CreativeAnnotationDraft[]) => Promise<boolean>;
  annotationScopes?: CreativeAnnotationDraft["scope"][];
  acceptance?: { enabled: boolean; status: string };
  showDecisionActions?: boolean;
  comparisonMode?: "source" | "adjustment";
}) {
  const [selectedAssetId, setSelectedAssetId] = useState(result.id);
  const [pair, setPair] = useState<"source-final" | "base-prime">("source-final");
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [annotationTool, setAnnotationTool] = useState<"point" | "rect" | null>(null);
  const [annotationScope, setAnnotationScope] = useState<CreativeAnnotationDraft["scope"]>(annotationScopes[0] ?? "size");
  const [annotations, setAnnotations] = useState<PendingCreativeAnnotation[]>([]);
  const [activeAnnotationId, setActiveAnnotationId] = useState("");
  const [annotationBusy, setAnnotationBusy] = useState(false);
  const [start, setStart] = useState<{ x: number; y: number } | null>(null);
  const [drawingEnd, setDrawingEnd] = useState<{ x: number; y: number } | null>(null);
  const [drag, setDrag] = useState<{ x: number; y: number; panX: number; panY: number } | null>(null);
  const [resultImageSize, setResultImageSize] = useState({ width: 1, height: 1 });
  const [annotationViewport, setAnnotationViewport] = useState({ width: 1, height: 1 });
  const stageRef = useRef<HTMLDivElement>(null);
  const resultPaneRef = useRef<HTMLElement>(null);
  const annotationSurfaceRef = useRef<HTMLDivElement>(null);
  const annotationSequence = useRef(0);
  const acceptanceStatusId = useId();
  const selectedResult = assets.find((asset) => asset.id === selectedAssetId) ?? result;
  const isAdjustmentComparison = comparisonMode === "adjustment";
  const displayedSource = pair === "source-final" ? source : { label: "底图", url: selectedResult.baseUrl || "" };
  const displayedResult = pair === "source-final"
    ? { ...selectedResult, label: isAdjustmentComparison ? `调整后 · ${selectedResult.label}` : selectedResult.label, url: selectedResult.finalUrl }
    : { ...selectedResult, label: "Prime 成图", url: selectedResult.finalUrl };
  const transform = `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`;
  const canAnnotate = Boolean(onAnnotations);
  const defaultAnnotationScope = annotationScopes[0] ?? "size";
  const annotationScopeKey = annotationScopes.join("|");
  const annotationImageBounds = transformedContainedImageBounds(annotationViewport, resultImageSize, zoom, pan);
  const activeAnnotation = annotations.find((annotation) => annotation.localId === activeAnnotationId);
  const drawingAnnotation = annotationTool && start && drawingEnd
    ? { ...normalizeCreativeAnnotationInImage(start, drawingEnd, annotationViewport, resultImageSize, annotationTool), localId: "drawing", issueType: "other", scope: annotationScope, comment: "" }
    : null;
  const updateAnnotation = (id: string, patch: Partial<CreativeAnnotationDraft>) => setAnnotations((current) => current.map((annotation) => annotation.localId === id ? { ...annotation, ...patch } : annotation));
  const chooseAnnotationScope = (scope: CreativeAnnotationDraft["scope"]) => {
    setAnnotationScope(scope);
    setAnnotations((current) => current.map((annotation) => ({ ...annotation, scope })));
  };
  const removeAnnotation = (id: string) => {
    setAnnotations((current) => current.filter((annotation) => annotation.localId !== id));
    setActiveAnnotationId((current) => current === id ? "" : current);
  };
  const annotationBoxStyle = (annotation: PendingCreativeAnnotation): React.CSSProperties => {
    const pointOffset = annotation.kind === "point" ? 8 : 0;
    return {
      left: annotationImageBounds.left + annotation.x * annotationImageBounds.width - pointOffset,
      top: annotationImageBounds.top + annotation.y * annotationImageBounds.height - pointOffset,
      width: annotation.kind === "point" ? 16 : Math.max(annotation.width * annotationImageBounds.width, 2),
      height: annotation.kind === "point" ? 16 : Math.max(annotation.height * annotationImageBounds.height, 2),
    };
  };
  const activeBox = activeAnnotation ? annotationBoxStyle(activeAnnotation) : undefined;
  const calloutStyle: React.CSSProperties | undefined = activeBox ? {
    left: Math.max(8, Math.min(Number(activeBox.left), annotationViewport.width - 336)),
    top: Number(activeBox.top) + Number(activeBox.height) + 176 < annotationViewport.height
      ? Number(activeBox.top) + Number(activeBox.height) + 8
      : Math.max(8, Number(activeBox.top) - 172),
  } : undefined;
  useEffect(() => {
    setSelectedAssetId(result.id);
  }, [result.id]);
  useEffect(() => {
    if (!annotationScopes.includes(annotationScope)) {
      chooseAnnotationScope(defaultAnnotationScope);
    }
  }, [annotationScope, annotationScopeKey, defaultAnnotationScope]);
  useEffect(() => {
    const element = annotationSurfaceRef.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    const update = () => setAnnotationViewport({ width: element.clientWidth || 1, height: element.clientHeight || 1 });
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [pair, selectedResult.id]);
  useEffect(() => {
    setAnnotations([]);
    setActiveAnnotationId("");
    setAnnotationTool(null);
    setStart(null);
    setDrawingEnd(null);
  }, [selectedResult.id]);
  useEffect(() => {
    const element = resultPaneRef.current;
    if (!element) return;
    const handleWheel = (event: WheelEvent) => {
      event.preventDefault();
      setZoom((value) => Math.max(0.25, Math.min(4, Number((value + (event.deltaY < 0 ? 0.15 : -0.15)).toFixed(2)))));
    };
    element.addEventListener("wheel", handleWheel, { passive: false });
    return () => element.removeEventListener("wheel", handleWheel);
  }, [selectedResult.id]);
  const onPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect();
    const point = { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
    setDrag({ ...point, panX: pan.x, panY: pan.y });
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const onPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!drag) return;
    setPan({ x: drag.panX + event.clientX - stageRef.current!.getBoundingClientRect().left - drag.x, y: drag.panY + event.clientY - stageRef.current!.getBoundingClientRect().top - drag.y });
  };
  const onPointerUp = () => {
    setDrag(null);
  };
  const onAnnotationPointerDown = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!annotationTool) return;
    event.stopPropagation();
    const bounds = event.currentTarget.getBoundingClientRect();
    const point = { x: event.clientX - bounds.left, y: event.clientY - bounds.top };
    setStart(point);
    setDrawingEnd(point);
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const onAnnotationPointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!annotationTool || !start) return;
    event.stopPropagation();
    const bounds = event.currentTarget.getBoundingClientRect();
    setDrawingEnd({ x: event.clientX - bounds.left, y: event.clientY - bounds.top });
  };
  const onAnnotationPointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!annotationTool || !start) return;
    event.stopPropagation();
    const bounds = event.currentTarget.getBoundingClientRect();
    const next = normalizeCreativeAnnotationInImage(start, { x: event.clientX - bounds.left, y: event.clientY - bounds.top }, bounds, resultImageSize, annotationTool);
    annotationSequence.current += 1;
    const localId = `annotation-${annotationSequence.current}`;
    setAnnotations((current) => [...current, { ...next, localId, issueType: "other", scope: annotationScope, comment: "" }]);
    setActiveAnnotationId(localId);
    setAnnotationTool(null);
    setStart(null);
    setDrawingEnd(null);
  };
  const scale = (delta: number) => setZoom((value) => Math.max(0.25, Math.min(4, Number((value + delta).toFixed(2)))));
  const beginAnnotation = (tool: "point" | "rect") => { setZoom(1); setPan({ x: 0, y: 0 }); setAnnotationTool(tool); };
  const image = (asset: { url: string; label: string }, captureSize = false, extra?: string) => asset.url ? <img draggable={false} src={asset.url} alt={asset.label} width={1600} height={1200} loading="lazy" onLoad={captureSize ? (event) => setResultImageSize({ width: event.currentTarget.naturalWidth || 1, height: event.currentTarget.naturalHeight || 1 }) : undefined} className={cn("pointer-events-none h-full w-full select-none object-contain", extra)} style={{ transform, transformOrigin: "center center" }} /> : <div className="flex h-full items-center justify-center text-xs text-muted-foreground">底图尚未登记</div>;
  const annotationOverlay = <div ref={annotationSurfaceRef} className="pointer-events-none absolute inset-0 z-20">
    <div data-testid="creative-annotation-surface" className={cn("absolute inset-0", annotationTool && "pointer-events-auto cursor-crosshair")} onPointerDown={onAnnotationPointerDown} onPointerMove={onAnnotationPointerMove} onPointerUp={onAnnotationPointerUp} onPointerCancel={() => { setStart(null); setDrawingEnd(null); }} />
    {drawingAnnotation && <div data-testid="creative-drawing-annotation" className={cn("absolute border-2 border-rose-500 bg-rose-500/10", drawingAnnotation.kind === "point" && "rounded-full bg-rose-500/25")} style={annotationBoxStyle(drawingAnnotation)} />}
    {annotations.map((annotation, index) => <button key={annotation.localId} type="button" aria-label={`选择标注 ${index + 1}`} onPointerDown={(event) => event.stopPropagation()} onClick={() => setActiveAnnotationId(annotation.localId)} className={cn("pointer-events-auto absolute border-2 border-rose-500 bg-rose-500/5", annotation.kind === "point" && "rounded-full bg-rose-500/20", annotation.localId === activeAnnotationId && "ring-2 ring-white ring-offset-1 ring-offset-rose-500")} style={annotationBoxStyle(annotation)}><span className="absolute -left-3 -top-3 flex h-6 min-w-6 items-center justify-center rounded-full bg-rose-600 px-1 text-[11px] font-semibold text-white shadow-sm">{index + 1}</span>{annotation.comment && <span className="absolute left-0 top-full mt-1 max-w-56 truncate border bg-background/95 px-2 py-1 text-left text-[11px] text-foreground shadow-sm">{annotation.comment}</span>}</button>)}
    {activeAnnotation && !annotationTool && <div className="pointer-events-auto absolute z-30 w-80 border bg-background p-2 shadow-lg" style={calloutStyle} onPointerDown={(event) => event.stopPropagation()}><div className="mb-2 flex items-center justify-between gap-2"><span className="text-xs font-semibold">标注 {annotations.findIndex((annotation) => annotation.localId === activeAnnotation.localId) + 1}</span><Button size="icon-sm" variant="ghost" title="删除标注" aria-label="删除标注" onClick={() => removeAnnotation(activeAnnotation.localId)}><Trash2 className="h-4 w-4" /></Button></div><div className="mb-2 grid gap-2"><NativeSelect size="sm" aria-label="标注问题类型" value={activeAnnotation.issueType} onChange={(event) => updateAnnotation(activeAnnotation.localId, { issueType: event.target.value })}><NativeSelectOption value="copy_error">文案错误</NativeSelectOption><NativeSelectOption value="theme_drift">主题偏离</NativeSelectOption><NativeSelectOption value="brand_prime">品牌或 Prime</NativeSelectOption><NativeSelectOption value="artifact">破图</NativeSelectOption><NativeSelectOption value="other">其他</NativeSelectOption></NativeSelect></div><Textarea autoFocus aria-label={`标注 ${annotations.findIndex((annotation) => annotation.localId === activeAnnotation.localId) + 1} 调整说明`} value={activeAnnotation.comment} onChange={(event) => updateAnnotation(activeAnnotation.localId, { comment: event.target.value })} rows={2} placeholder="写下这个区域需要怎么调整" /><div className="mt-2 flex justify-end"><Button size="sm" variant="outline" disabled={!activeAnnotation.comment.trim()} onClick={() => setActiveAnnotationId("")}>完成标注</Button></div></div>}
  </div>;
  const variants = useMemo(() => [...new Set(assets.map((asset) => asset.variant).filter((value): value is string => Boolean(value)))], [assets]);
  const sizes = useMemo(() => [...new Set(assets.map((asset) => asset.size).filter((value): value is string => Boolean(value)))], [assets]);
  const chooseVariant = (variant: string) => {
    const next = assets.find((asset) => asset.variant === variant && asset.size === selectedResult.size) ?? assets.find((asset) => asset.variant === variant);
    if (!next) return;
    setSelectedAssetId(next.id);
    onAssetChange(next.id);
  };
  const chooseSize = (size: string) => {
    const next = assets.find((asset) => asset.variant === selectedResult.variant && asset.size === size) ?? assets.find((asset) => asset.size === size);
    if (!next) return;
    setSelectedAssetId(next.id);
    onAssetChange(next.id);
  };
  return <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)_auto] bg-background" data-testid="creative-comparison-workspace">
    <div className="flex min-w-0 flex-nowrap items-center gap-2 overflow-x-auto border-b px-3 py-2 pr-14" data-testid="creative-preview-toolbar">
      <span className="shrink-0 text-sm font-semibold">高清对比</span>
      <div className="inline-flex border"><button type="button" onClick={() => setPair("source-final")} className={cn("h-7 whitespace-nowrap px-2.5 text-[11px]", pair === "source-final" && "bg-foreground text-background")}>{isAdjustmentComparison ? "调整前 / 调整后" : "原图 / 成图"}</button><button type="button" onClick={() => setPair("base-prime")} className={cn("h-7 whitespace-nowrap border-l px-2.5 text-[11px]", pair === "base-prime" && "bg-foreground text-background")}>底图 / Prime</button></div>
      <div className="inline-flex border" role="group" aria-label="创意变体">{variants.map((variant) => <button key={variant} type="button" aria-pressed={variant === selectedResult.variant} onClick={() => chooseVariant(variant)} className={cn("h-7 min-w-12 px-2.5 text-[11px]", variant === selectedResult.variant && "bg-foreground text-background")}>{variant}</button>)}</div>
      <div className="inline-flex border" role="group" aria-label="成图尺寸">{sizes.map((assetSize) => <button key={assetSize} type="button" aria-pressed={assetSize === selectedResult.size} onClick={() => chooseSize(assetSize)} className={cn("h-7 min-w-12 px-2.5 text-[11px]", assetSize === selectedResult.size && "bg-foreground text-background")}>{assetSize === "1080x1080" ? "方形" : assetSize === "1200x628" ? "横版" : assetSize === "800x1000" ? "竖版" : assetSize}</button>)}</div>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <Button size="icon-sm" variant="ghost" title="适应窗口" aria-label="适应窗口" disabled={zoom === 1 && pan.x === 0 && pan.y === 0} onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><Expand className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title="缩小" aria-label="缩小" onClick={() => scale(-0.2)}><Minus className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title="放大" aria-label="放大" onClick={() => scale(0.2)}><Plus className="h-4 w-4" /></Button>
        <span className="w-10 text-right text-[11px] text-muted-foreground">{Math.round(zoom * 100)}%</span>
      </div>
    </div>
    <div className="min-h-0 bg-muted/20 p-2">
      <div ref={stageRef} data-testid="creative-comparison-stage" className={cn("relative h-full min-h-[480px] overflow-hidden border bg-background", annotationTool ? "cursor-crosshair" : "cursor-grab active:cursor-grabbing")} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={() => setDrag(null)}>
        <div className="grid h-full grid-cols-1 divide-y md:grid-cols-[minmax(160px,0.3fr)_minmax(0,1fr)] md:divide-x md:divide-y-0"><ComparisonPane title={isAdjustmentComparison ? "查看调整前成图" : "查看竞品原图"} label={displayedSource.label} muted testId="creative-comparison-source-pane">{displayedSource.url ? <img draggable={false} src={displayedSource.url} alt={displayedSource.label} width={800} height={800} loading="lazy" className="pointer-events-none h-full w-full select-none object-contain" /> : <div className="flex h-full items-center justify-center text-xs text-muted-foreground">底图尚未登记</div>}</ComparisonPane><ComparisonPane title={isAdjustmentComparison ? "查看调整后成图" : "查看修图结果大图"} label={displayedResult.label} overlay={annotationOverlay} containerRef={resultPaneRef} testId="creative-comparison-result-pane">{image(displayedResult, true)}</ComparisonPane></div>
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">
      {canAnnotate && <><Button size="icon-sm" variant={annotationTool === "point" ? "default" : "outline"} aria-pressed={annotationTool === "point"} title="点标注" aria-label="点标注" onClick={() => beginAnnotation("point")}><MousePointer2 className="h-4 w-4" /></Button><Button size="icon-sm" variant={annotationTool === "rect" ? "default" : "outline"} aria-pressed={annotationTool === "rect"} title="框选标注" aria-label="框选标注" onClick={() => beginAnnotation("rect")}><SquareDashedMousePointer className="h-4 w-4" /></Button></>}
      {canAnnotate && annotationScopes.length > 1 && <div className="inline-flex border" role="group" aria-label="调整范围">{annotationScopes.map((scope) => <button key={scope} type="button" aria-pressed={annotationScope === scope} onClick={() => chooseAnnotationScope(scope)} className={cn("h-8 whitespace-nowrap border-l px-2.5 text-xs first:border-l-0", annotationScope === scope && "bg-foreground text-background")}>{ANNOTATION_SCOPE_LABELS[scope]}</button>)}</div>}
      {annotationTool && <span className="text-xs font-medium text-amber-700" role="status">{annotationTool === "point" ? "在右侧成图上标记位置" : "在右侧成图上拖动圈选区域"}</span>}
      {annotations.length > 0 && <span className="text-xs font-medium text-rose-700">已标注 {annotations.length} 处</span>}
      {onAdjust && <Button size="sm" variant="outline" onClick={onAdjust}><PencilRuler className="h-4 w-4" />调整</Button>}
      {onViewInfo && <Button size="sm" variant="outline" onClick={onViewInfo}><Info className="h-4 w-4" />生成信息</Button>}
      {showDecisionActions && <Button size="sm" variant="outline" onClick={() => onDecision?.("abandoned")}>放弃</Button>}
      {showDecisionActions && <Button size="sm" disabled={acceptance ? !acceptance.enabled : false} aria-describedby={acceptance ? acceptanceStatusId : undefined} onClick={() => onDecision?.("accepted")}><Check className="h-4 w-4" />接受</Button>}
      <Button size="icon-sm" variant="outline" title="下载当前成图" aria-label="下载当前成图" onClick={() => { onDecision?.("downloaded"); window.open(selectedResult.finalUrl, "_blank", "noopener,noreferrer"); }}><Download className="h-4 w-4" /></Button>
      {acceptance && <span id={acceptanceStatusId} role="status" className={cn("text-xs", acceptance.enabled ? "text-emerald-700" : "text-muted-foreground")}>{acceptance.status}</span>}
      <span className="ml-auto text-xs text-muted-foreground">{variants.length} 个变体 · {sizes.length} 个尺寸</span>
      {annotations.length > 0 && <Button size="sm" disabled={annotationBusy || annotations.some((annotation) => !annotation.comment.trim())} onClick={() => { if (!onAnnotations) return; setAnnotationBusy(true); const drafts = annotations.map(({ localId: _, ...annotation }) => ({ ...annotation, scope: annotationScope })); void onAnnotations(drafts).then((saved) => { if (saved) { setAnnotations([]); setActiveAnnotationId(""); } }).finally(() => setAnnotationBusy(false)); }}>{annotationBusy ? "正在提交" : `提交 ${annotations.length} 处调整`}</Button>}
    </div>
  </div>;
}

function ComparisonPane({ label, title, children, overlay, muted = false, containerRef, testId }: { label: string; title?: string; children: React.ReactNode; overlay?: React.ReactNode; muted?: boolean; containerRef?: React.Ref<HTMLElement>; testId?: string }) {
  return <section ref={containerRef} title={title} data-testid={testId} className={cn("relative h-full min-h-0 overflow-hidden", muted ? "bg-muted/40" : "bg-background")}><span className="absolute left-3 top-3 z-10 max-w-[calc(100%-1.5rem)] truncate border bg-background/90 px-2 py-1 text-[11px] font-medium text-foreground shadow-sm">{label}</span>{children}{overlay}</section>;
}
