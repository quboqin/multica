"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Check, Download, Expand, Info, Minus, MousePointer2, PencilRuler, Plus, SquareDashedMousePointer, Trash2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

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
  annotationScopeLabels,
  acceptance,
  showDecisionActions = true,
  allowDownload = true,
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
  annotationScopeLabels?: Partial<Record<CreativeAnnotationDraft["scope"], string>>;
  acceptance?: { enabled: boolean; status: string };
  showDecisionActions?: boolean;
  allowDownload?: boolean;
  comparisonMode?: "source" | "adjustment";
}) {
  const { t } = useT("creative");
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
  const displayedSource = pair === "source-final" ? source : { label: t(($) => $.comparison.baseImage), url: selectedResult.baseUrl || "" };
  const displayedResult = pair === "source-final"
    ? { ...selectedResult, label: isAdjustmentComparison ? t(($) => $.comparison.afterAdjustment, { label: selectedResult.label }) : selectedResult.label, url: selectedResult.finalUrl }
    : { ...selectedResult, label: t(($) => $.comparison.primeImage), url: selectedResult.finalUrl };
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
  const image = (asset: { url: string; label: string }, captureSize = false, extra?: string) => asset.url ? <img draggable={false} src={asset.url} alt={asset.label} width={1600} height={1200} loading="lazy" onLoad={captureSize ? (event) => setResultImageSize({ width: event.currentTarget.naturalWidth || 1, height: event.currentTarget.naturalHeight || 1 }) : undefined} className={cn("pointer-events-none h-full w-full select-none object-contain", extra)} style={{ transform, transformOrigin: "center center" }} /> : <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{t(($) => $.comparison.baseUnavailable)}</div>;
  const annotationOverlay = <div ref={annotationSurfaceRef} className="pointer-events-none absolute inset-0 z-20">
    <div data-testid="creative-annotation-surface" className={cn("absolute inset-0", annotationTool && "pointer-events-auto cursor-crosshair")} onPointerDown={onAnnotationPointerDown} onPointerMove={onAnnotationPointerMove} onPointerUp={onAnnotationPointerUp} onPointerCancel={() => { setStart(null); setDrawingEnd(null); }} />
    {drawingAnnotation && <div data-testid="creative-drawing-annotation" className={cn("absolute border-2 border-rose-500 bg-rose-500/10", drawingAnnotation.kind === "point" && "rounded-full bg-rose-500/25")} style={annotationBoxStyle(drawingAnnotation)} />}
    {annotations.map((annotation, index) => <button key={annotation.localId} type="button" aria-label={t(($) => $.comparison.selectAnnotation, { count: index + 1 })} onPointerDown={(event) => event.stopPropagation()} onClick={() => setActiveAnnotationId(annotation.localId)} className={cn("pointer-events-auto absolute border-2 border-rose-500 bg-rose-500/5", annotation.kind === "point" && "rounded-full bg-rose-500/20", annotation.localId === activeAnnotationId && "ring-2 ring-white ring-offset-1 ring-offset-rose-500")} style={annotationBoxStyle(annotation)}><span className="absolute -left-3 -top-3 flex h-6 min-w-6 items-center justify-center rounded-full bg-rose-600 px-1 text-[11px] font-semibold text-white shadow-sm">{index + 1}</span>{annotation.comment && <span className="absolute left-0 top-full mt-1 max-w-56 truncate border bg-background/95 px-2 py-1 text-left text-[11px] text-foreground shadow-sm">{annotation.comment}</span>}</button>)}
    {activeAnnotation && !annotationTool && <div className="pointer-events-auto absolute z-30 w-80 border bg-background p-2 shadow-lg" style={calloutStyle} onPointerDown={(event) => event.stopPropagation()}><div className="mb-2 flex items-center justify-between gap-2"><span className="text-xs font-semibold">{t(($) => $.comparison.annotation, { count: annotations.findIndex((annotation) => annotation.localId === activeAnnotation.localId) + 1 })}</span><Button size="icon-sm" variant="ghost" title={t(($) => $.comparison.deleteAnnotation)} aria-label={t(($) => $.comparison.deleteAnnotation)} onClick={() => removeAnnotation(activeAnnotation.localId)}><Trash2 className="h-4 w-4" /></Button></div><div className="mb-2 grid gap-2"><NativeSelect size="sm" aria-label={t(($) => $.comparison.issueType)} value={activeAnnotation.issueType} onChange={(event) => updateAnnotation(activeAnnotation.localId, { issueType: event.target.value })}><NativeSelectOption value="copy_error">{t(($) => $.comparison.copyError)}</NativeSelectOption><NativeSelectOption value="theme_drift">{t(($) => $.comparison.themeDrift)}</NativeSelectOption><NativeSelectOption value="brand_prime">{t(($) => $.comparison.brandPrime)}</NativeSelectOption><NativeSelectOption value="artifact">{t(($) => $.comparison.artifact)}</NativeSelectOption><NativeSelectOption value="other">{t(($) => $.comparison.other)}</NativeSelectOption></NativeSelect></div><Textarea autoFocus aria-label={t(($) => $.comparison.annotationInstruction)} value={activeAnnotation.comment} onChange={(event) => updateAnnotation(activeAnnotation.localId, { comment: event.target.value })} rows={2} placeholder={t(($) => $.comparison.annotationInstruction)} /><div className="mt-2 flex justify-end"><Button size="sm" variant="outline" disabled={!activeAnnotation.comment.trim()} onClick={() => setActiveAnnotationId("")}>{t(($) => $.comparison.completeAnnotation)}</Button></div></div>}
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
      <span className="shrink-0 text-sm font-semibold">{t(($) => $.comparison.highDefinition)}</span>
      <div className="inline-flex border"><button type="button" onClick={() => setPair("source-final")} className={cn("h-7 whitespace-nowrap px-2.5 text-[11px]", pair === "source-final" && "bg-foreground text-background")}>{isAdjustmentComparison ? t(($) => $.comparison.beforeAfter) : t(($) => $.comparison.sourceFinal)}</button><button type="button" onClick={() => setPair("base-prime")} className={cn("h-7 whitespace-nowrap border-l px-2.5 text-[11px]", pair === "base-prime" && "bg-foreground text-background")}>{t(($) => $.comparison.basePrime)}</button></div>
      <div className="inline-flex border" role="group" aria-label={t(($) => $.comparison.creativeVariant)}>{variants.map((variant) => <button key={variant} type="button" aria-pressed={variant === selectedResult.variant} onClick={() => chooseVariant(variant)} className={cn("h-7 min-w-12 px-2.5 text-[11px]", variant === selectedResult.variant && "bg-foreground text-background")}>{variant}</button>)}</div>
      <div className="inline-flex border" role="group" aria-label={t(($) => $.comparison.creativeSize)}>{sizes.map((assetSize) => <button key={assetSize} type="button" aria-pressed={assetSize === selectedResult.size} onClick={() => chooseSize(assetSize)} className={cn("h-7 min-w-12 px-2.5 text-[11px]", assetSize === selectedResult.size && "bg-foreground text-background")}>{assetSize === "1080x1080" ? t(($) => $.comparison.square) : assetSize === "1200x628" ? t(($) => $.comparison.landscape) : assetSize === "800x1000" ? t(($) => $.comparison.portrait) : assetSize}</button>)}</div>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <Button size="icon-sm" variant="ghost" title={t(($) => $.comparison.fit)} aria-label={t(($) => $.comparison.fit)} disabled={zoom === 1 && pan.x === 0 && pan.y === 0} onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><Expand className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title={t(($) => $.comparison.zoomOut)} aria-label={t(($) => $.comparison.zoomOut)} onClick={() => scale(-0.2)}><Minus className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title={t(($) => $.comparison.zoomIn)} aria-label={t(($) => $.comparison.zoomIn)} onClick={() => scale(0.2)}><Plus className="h-4 w-4" /></Button>
        <span className="w-10 text-right text-[11px] text-muted-foreground">{Math.round(zoom * 100)}%</span>
      </div>
    </div>
    <div className="min-h-0 bg-muted/20 p-2">
      <div ref={stageRef} data-testid="creative-comparison-stage" className={cn("relative h-full min-h-[480px] overflow-hidden border bg-background", annotationTool ? "cursor-crosshair" : "cursor-grab active:cursor-grabbing")} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onPointerCancel={() => setDrag(null)}>
        <div className="grid h-full grid-cols-1 divide-y md:grid-cols-[minmax(160px,0.3fr)_minmax(0,1fr)] md:divide-x md:divide-y-0"><ComparisonPane title={isAdjustmentComparison ? t(($) => $.comparison.beforeTitle) : t(($) => $.comparison.sourceTitle)} label={displayedSource.label} muted testId="creative-comparison-source-pane">{displayedSource.url ? <img draggable={false} src={displayedSource.url} alt={displayedSource.label} width={800} height={800} loading="lazy" className="pointer-events-none h-full w-full select-none object-contain" /> : <div className="flex h-full items-center justify-center text-xs text-muted-foreground">{t(($) => $.comparison.baseUnavailable)}</div>}</ComparisonPane><ComparisonPane title={isAdjustmentComparison ? t(($) => $.comparison.afterTitle) : t(($) => $.comparison.resultTitle)} label={displayedResult.label} overlay={annotationOverlay} containerRef={resultPaneRef} testId="creative-comparison-result-pane">{image(displayedResult, true)}</ComparisonPane></div>
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">
      {canAnnotate && <><Button size="icon-sm" variant={annotationTool === "point" ? "default" : "outline"} aria-pressed={annotationTool === "point"} title={t(($) => $.comparison.pointAnnotation)} aria-label={t(($) => $.comparison.pointAnnotation)} onClick={() => beginAnnotation("point")}><MousePointer2 className="h-4 w-4" /></Button><Button size="icon-sm" variant={annotationTool === "rect" ? "default" : "outline"} aria-pressed={annotationTool === "rect"} title={t(($) => $.comparison.boxAnnotation)} aria-label={t(($) => $.comparison.boxAnnotation)} onClick={() => beginAnnotation("rect")}><SquareDashedMousePointer className="h-4 w-4" /></Button></>}
      {canAnnotate && annotationScopes.length > 1 && <div className="inline-flex border" role="group" aria-label={t(($) => $.comparison.adjustmentScope)}>{annotationScopes.map((scope) => <button key={scope} type="button" aria-pressed={annotationScope === scope} onClick={() => chooseAnnotationScope(scope)} className={cn("h-8 whitespace-nowrap border-l px-2.5 text-xs first:border-l-0", annotationScope === scope && "bg-foreground text-background")}>{annotationScopeLabels?.[scope] ?? annotationScopeLabel(t, scope)}</button>)}</div>}
      {annotationTool && <span className="text-xs font-medium text-amber-700" role="status">{annotationTool === "point" ? t(($) => $.comparison.markPoint) : t(($) => $.comparison.markArea)}</span>}
      {annotations.length > 0 && <span className="text-xs font-medium text-rose-700">{t(($) => $.comparison.annotated, { count: annotations.length })}</span>}
      {onAdjust && <Button size="sm" variant="outline" onClick={onAdjust}><PencilRuler className="h-4 w-4" />{t(($) => $.comparison.adjust)}</Button>}
      {onViewInfo && <Button size="sm" variant="outline" onClick={onViewInfo}><Info className="h-4 w-4" />{t(($) => $.comparison.generationInfo)}</Button>}
      {showDecisionActions && <Button size="sm" variant="outline" onClick={() => onDecision?.("abandoned")}>{t(($) => $.comparison.abandon)}</Button>}
      {showDecisionActions && <Button size="sm" disabled={acceptance ? !acceptance.enabled : false} aria-describedby={acceptance ? acceptanceStatusId : undefined} onClick={() => onDecision?.("accepted")}><Check className="h-4 w-4" />{t(($) => $.comparison.accept)}</Button>}
      {allowDownload && <Button size="icon-sm" variant="outline" title={t(($) => $.comparison.downloadCurrent)} aria-label={t(($) => $.comparison.downloadCurrent)} onClick={() => { onDecision?.("downloaded"); window.open(selectedResult.finalUrl, "_blank", "noopener,noreferrer"); }}><Download className="h-4 w-4" /></Button>}
      {acceptance && <span id={acceptanceStatusId} role="status" className={cn("text-xs", acceptance.enabled ? "text-emerald-700" : "text-muted-foreground")}>{acceptance.status}</span>}
      <span className="ml-auto text-xs text-muted-foreground">{t(($) => $.comparison.variantsSizes, { variants: variants.length, sizes: sizes.length })}</span>
      {annotations.length > 0 && <Button size="sm" disabled={annotationBusy || annotations.some((annotation) => !annotation.comment.trim())} onClick={() => { if (!onAnnotations) return; setAnnotationBusy(true); const drafts = annotations.map(({ localId: _, ...annotation }) => ({ ...annotation, scope: annotationScope })); void onAnnotations(drafts).then((saved) => { if (saved) { setAnnotations([]); setActiveAnnotationId(""); } }).finally(() => setAnnotationBusy(false)); }}>{annotationBusy ? t(($) => $.comparison.submitting) : t(($) => $.comparison.submitAdjustments, { count: annotations.length })}</Button>}
    </div>
  </div>;
}

function annotationScopeLabel(t: ReturnType<typeof useT<"creative">>["t"], scope: CreativeAnnotationDraft["scope"]): string {
  if (scope === "size") return t(($) => $.comparison.scopeSize);
  if (scope === "variant") return t(($) => $.comparison.scopeVariant);
  return t(($) => $.comparison.scopeOrder);
}

function ComparisonPane({ label, title, children, overlay, muted = false, containerRef, testId }: { label: string; title?: string; children: React.ReactNode; overlay?: React.ReactNode; muted?: boolean; containerRef?: React.Ref<HTMLElement>; testId?: string }) {
  return <section ref={containerRef} title={title} data-testid={testId} className={cn("relative h-full min-h-0 overflow-hidden", muted ? "bg-muted/40" : "bg-background")}><span className="absolute left-3 top-3 z-10 max-w-[calc(100%-1.5rem)] truncate border bg-background/90 px-2 py-1 text-[11px] font-medium text-foreground shadow-sm">{label}</span>{children}{overlay}</section>;
}
