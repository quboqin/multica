"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import { Check, Download, Expand, Minus, MousePointer2, PencilRuler, Plus, RotateCcw, SquareDashedMousePointer } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";

export type CreativeComparisonMode = "side-by-side" | "slider" | "overlay";
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
  onDecision,
  onAnnotation,
  acceptance,
}: {
  source: { label: string; url: string };
  result: CreativeComparisonAsset;
  assets: CreativeComparisonAsset[];
  onAssetChange: (id: string) => void;
  onAdjust: () => void;
  onDecision?: (decision: "accepted" | "abandoned" | "downloaded") => void;
  onAnnotation?: (annotation: CreativeAnnotationDraft) => void;
  acceptance?: { enabled: boolean; status: string };
}) {
  const [mode, setMode] = useState<CreativeComparisonMode>("side-by-side");
  const [pair, setPair] = useState<"source-final" | "base-prime">("source-final");
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [slider, setSlider] = useState(50);
  const [annotationTool, setAnnotationTool] = useState<"point" | "rect" | null>(null);
  const [annotation, setAnnotation] = useState<CreativeAnnotationDraft | null>(null);
  const [start, setStart] = useState<{ x: number; y: number } | null>(null);
  const [drag, setDrag] = useState<{ x: number; y: number; panX: number; panY: number } | null>(null);
  const [resultImageSize, setResultImageSize] = useState({ width: 1, height: 1 });
  const [annotationViewport, setAnnotationViewport] = useState({ width: 1, height: 1 });
  const stageRef = useRef<HTMLDivElement>(null);
  const annotationSurfaceRef = useRef<HTMLDivElement>(null);
  const acceptanceStatusId = useId();
  const displayedSource = pair === "source-final" ? source : { label: "底图", url: result.baseUrl || "" };
  const displayedResult = pair === "source-final" ? { ...result, url: result.finalUrl } : { ...result, label: "Prime 成图", url: result.finalUrl };
  const transform = `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`;
  const canAnnotate = Boolean(onAnnotation);
  const annotationImageBounds = containedImageBounds(annotationViewport, resultImageSize);
  const annotationStyle = annotation ? {
    left: annotationImageBounds.left + annotation.x * annotationImageBounds.width,
    top: annotationImageBounds.top + annotation.y * annotationImageBounds.height,
    width: Math.max(annotation.width * annotationImageBounds.width, annotation.kind === "point" ? 12 : 0),
    height: Math.max(annotation.height * annotationImageBounds.height, annotation.kind === "point" ? 12 : 0),
  } : undefined;
  useEffect(() => {
    const element = annotationSurfaceRef.current;
    if (!element || typeof ResizeObserver === "undefined") return;
    const update = () => setAnnotationViewport({ width: element.clientWidth || 1, height: element.clientHeight || 1 });
    update();
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [mode, pair, result.id]);
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
    setStart({ x: event.clientX - bounds.left, y: event.clientY - bounds.top });
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const onAnnotationPointerUp = (event: React.PointerEvent<HTMLDivElement>) => {
    if (!annotationTool || !start) return;
    event.stopPropagation();
    const bounds = event.currentTarget.getBoundingClientRect();
    const next = normalizeCreativeAnnotationInImage(start, { x: event.clientX - bounds.left, y: event.clientY - bounds.top }, bounds, resultImageSize, annotationTool);
    setAnnotation({ ...next, issueType: "other", scope: "size", comment: "" });
    setAnnotationTool(null);
    setStart(null);
  };
  const scale = (delta: number) => setZoom((value) => Math.max(0.25, Math.min(4, Number((value + delta).toFixed(2)))));
  const beginAnnotation = (tool: "point" | "rect") => { setMode("side-by-side"); setZoom(1); setPan({ x: 0, y: 0 }); setAnnotationTool(tool); };
  const image = (asset: { url: string; label: string }, captureSize = false, extra?: string) => asset.url ? <img draggable={false} src={asset.url} alt={asset.label} width={1600} height={1200} loading="lazy" onLoad={captureSize ? (event) => setResultImageSize({ width: event.currentTarget.naturalWidth || 1, height: event.currentTarget.naturalHeight || 1 }) : undefined} className={cn("pointer-events-none h-full w-full select-none object-contain", extra)} style={{ transform, transformOrigin: "center center" }} /> : <div className="flex h-full items-center justify-center text-xs text-white/60">底图尚未登记</div>;
  const annotationOverlay = <div ref={annotationSurfaceRef} className={cn("absolute inset-0 z-20", annotationTool ? "cursor-crosshair pointer-events-auto" : "pointer-events-none")} onPointerDown={onAnnotationPointerDown} onPointerUp={onAnnotationPointerUp}>{annotation && <div className={cn("pointer-events-none absolute border-2 border-amber-400 bg-amber-300/20", annotation.kind === "point" && "rounded-full")} style={annotationStyle} />}</div>;
  const variants = useMemo(() => [...new Set(assets.map((asset) => asset.variant).filter(Boolean))], [assets]);
  return <div className="grid h-full min-h-0 grid-rows-[auto_auto_auto_minmax(0,1fr)_auto] bg-background" data-testid="creative-comparison-workspace">
    <div className="flex min-w-0 items-center gap-2 border-b px-4 py-2 pr-14" data-testid="creative-preview-toolbar">
      <span className="shrink-0 text-sm font-semibold">高清对比</span>
      <div className="flex min-w-0 overflow-x-auto border" aria-label="对比模式">
        {(["side-by-side", "slider", "overlay"] as const).map((value) => <button key={value} type="button" onClick={() => setMode(value)} className={cn("h-8 shrink-0 px-3 text-xs", mode === value && "bg-foreground text-background")}>
          {value === "side-by-side" ? "双栏" : value === "slider" ? "滑杆" : "叠加"}
        </button>)}
      </div>
      <div className="ml-auto flex shrink-0 items-center gap-1">
        <Button size="icon-sm" variant="ghost" title="适应窗口" aria-label="适应窗口" onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><Expand className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title="100% 缩放" aria-label="100% 缩放" onClick={() => setZoom(1)}><span className="text-[10px] font-semibold">100</span></Button>
        <Button size="icon-sm" variant="ghost" title="缩小" aria-label="缩小" onClick={() => scale(-0.2)}><Minus className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title="放大" aria-label="放大" onClick={() => scale(0.2)}><Plus className="h-4 w-4" /></Button>
        <Button size="icon-sm" variant="ghost" title="重置画布" aria-label="重置画布" onClick={() => { setZoom(1); setPan({ x: 0, y: 0 }); }}><RotateCcw className="h-4 w-4" /></Button>
      </div>
    </div>
    <div className="flex overflow-x-auto border-b px-4 py-2"><div className="inline-flex border"><button type="button" onClick={() => setPair("source-final")} className={cn("h-8 whitespace-nowrap px-3 text-xs", pair === "source-final" && "bg-foreground text-background")}>原图与正式成图</button><button type="button" onClick={() => setPair("base-prime")} className={cn("h-8 whitespace-nowrap border-l px-3 text-xs", pair === "base-prime" && "bg-foreground text-background")}>底图与 Prime</button></div></div>
    <div className="flex gap-2 overflow-x-auto border-b bg-muted/20 p-2" data-testid="creative-variant-strip">{assets.map((asset) => <button key={asset.id} type="button" onClick={() => onAssetChange(asset.id)} className={cn("grid h-14 w-28 shrink-0 grid-cols-[42px_1fr] items-center gap-1 border bg-background p-1 text-left", asset.id === result.id && "border-emerald-600 ring-1 ring-emerald-600/20")}><img src={asset.thumbnailUrl || asset.finalUrl} alt="" width={40} height={40} loading="lazy" className="h-10 w-10 object-contain" /><span className="min-w-0 text-[10px]"><span className="block font-semibold">{asset.variant || "结果"}</span><span className="block truncate">{asset.size || asset.label}</span></span></button>)}</div>
    <div className="min-h-0 bg-black p-2">
      <div ref={stageRef} className={cn("relative h-full min-h-[360px] overflow-hidden bg-neutral-950", annotationTool ? "cursor-crosshair" : "cursor-grab active:cursor-grabbing")} onPointerDown={onPointerDown} onPointerMove={onPointerMove} onPointerUp={onPointerUp} onWheel={(event) => { event.preventDefault(); scale(event.deltaY < 0 ? 0.15 : -0.15); }}>
        {mode === "side-by-side" && <div className="grid h-full grid-cols-1 divide-y divide-white/20 md:grid-cols-2 md:divide-x md:divide-y-0"><ComparisonPane title="查看竞品原图大图" label={displayedSource.label}>{image(displayedSource)}</ComparisonPane><ComparisonPane title="查看修图结果大图" label={displayedResult.label} overlay={annotationOverlay}>{image(displayedResult, true)}</ComparisonPane></div>}
        {mode === "slider" && <div className="relative h-full"><ComparisonPane label={displayedSource.label}>{image(displayedSource)}</ComparisonPane><div className="absolute inset-y-0 left-0 overflow-hidden border-r-2 border-white" style={{ width: `${slider}%` }}><div className="h-full" style={{ width: `${10000 / slider}%` }}><ComparisonPane label={displayedResult.label}>{image(displayedResult, true)}</ComparisonPane></div></div><input aria-label="对比滑杆" type="range" min="1" max="100" value={slider} onChange={(event) => setSlider(Number(event.target.value))} className="absolute inset-x-4 bottom-4 z-10" /></div>}
        {mode === "overlay" && <div className="relative h-full"><ComparisonPane label={displayedSource.label}>{image(displayedSource)}</ComparisonPane><div className="absolute inset-0 opacity-50"><ComparisonPane label={displayedResult.label}>{image(displayedResult, true)}</ComparisonPane></div></div>}
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">
      {canAnnotate && <><Button size="icon-sm" variant={annotationTool === "point" ? "default" : "outline"} title="添加点标注" aria-label="添加点标注" onClick={() => beginAnnotation("point")}><MousePointer2 className="h-4 w-4" /></Button><Button size="icon-sm" variant={annotationTool === "rect" ? "default" : "outline"} title="添加矩形标注" aria-label="添加矩形标注" onClick={() => beginAnnotation("rect")}><SquareDashedMousePointer className="h-4 w-4" /></Button></>}
      <Button size="sm" variant="outline" onClick={onAdjust}><PencilRuler className="h-4 w-4" />调整</Button>
      <Button size="sm" variant="outline" onClick={() => onDecision?.("abandoned")}>放弃</Button>
      <Button size="sm" disabled={acceptance ? !acceptance.enabled : false} aria-describedby={acceptance ? acceptanceStatusId : undefined} onClick={() => onDecision?.("accepted")}><Check className="h-4 w-4" />接受</Button>
      <Button size="icon-sm" variant="outline" title="下载当前成图" aria-label="下载当前成图" onClick={() => { onDecision?.("downloaded"); window.open(result.finalUrl, "_blank", "noopener,noreferrer"); }}><Download className="h-4 w-4" /></Button>
      {acceptance && <span id={acceptanceStatusId} role="status" className={cn("text-xs", acceptance.enabled ? "text-emerald-700" : "text-muted-foreground")}>{acceptance.status}</span>}
      <span className="ml-auto text-xs text-muted-foreground">{Math.round(zoom * 100)}% · {variants.length} 个变体</span>
      {annotation && <div className="grid w-full gap-2 border-t pt-2 sm:grid-cols-[150px_150px_minmax(0,1fr)_auto]"><NativeSelect size="sm" value={annotation.issueType} onChange={(event) => setAnnotation({ ...annotation, issueType: event.target.value })}><NativeSelectOption value="copy_error">文案错误</NativeSelectOption><NativeSelectOption value="theme_drift">主题偏离</NativeSelectOption><NativeSelectOption value="brand_prime">品牌或 Prime</NativeSelectOption><NativeSelectOption value="artifact">破图</NativeSelectOption><NativeSelectOption value="other">其他</NativeSelectOption></NativeSelect><NativeSelect size="sm" value={annotation.scope} onChange={(event) => setAnnotation({ ...annotation, scope: event.target.value as CreativeAnnotationDraft["scope"] })}><NativeSelectOption value="size">当前尺寸</NativeSelectOption><NativeSelectOption value="variant">当前变体三尺寸</NativeSelectOption><NativeSelectOption value="order">全部变体</NativeSelectOption></NativeSelect><Textarea value={annotation.comment} onChange={(event) => setAnnotation({ ...annotation, comment: event.target.value })} rows={1} placeholder="说明问题和修改要求..." /><Button size="sm" onClick={() => { onAnnotation?.(annotation); setAnnotation(null); }}>保存标注</Button></div>}
    </div>
  </div>;
}

function ComparisonPane({ label, title, children, overlay }: { label: string; title?: string; children: React.ReactNode; overlay?: React.ReactNode }) {
  return <section title={title} className="relative h-full min-h-0 overflow-hidden"><span className="absolute left-3 top-3 z-10 bg-black/70 px-2 py-1 text-[11px] font-medium text-white">{label}</span>{children}{overlay}</section>;
}
