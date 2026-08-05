"use client";

import { useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Eye, FileImage, ImageIcon, Layers3, Maximize2, Move, Plus, QrCode, Trash2, Upload, ZoomIn, ZoomOut } from "lucide-react";
import { creativeResourceFilesOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import type { CreativeResource, CreativeResourceFile } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { NativeSelect, NativeSelectOption } from "@multica/ui/components/ui/native-select";
import { Switch } from "@multica/ui/components/ui/switch";
import { Tabs, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { PRIME_RESOURCE_UPLOAD_EVENT, purposeLabel } from "./market-resource-files";

export const PRIME_SIZES = ["1080x1080", "1200x628", "800x1000"] as const;
export type PrimeSize = (typeof PRIME_SIZES)[number];
export type PrimeRect = [number, number, number, number];
type PrimeComponentKind = "image" | "text" | "qr";
type PrimeQRMode = "none" | "static" | "dynamic";
type PrimePosition = { destination_rect: PrimeRect };

export type PrimeCompositionComponent = {
  id: string;
  label: string;
  kind: PrimeComponentKind;
  enabled: boolean;
  source_role: string;
  content: string;
  backdrop_rule: "none" | "quiet" | "light";
};

export type PrimeComposition = {
  schema_version: 2;
  qr_mode: PrimeQRMode;
  components: PrimeCompositionComponent[];
  layouts: Record<PrimeSize, {
    components: Record<string, PrimePosition>;
  }>;
};

const SIZE_META: Record<PrimeSize, { width: number; height: number; label: string }> = {
  "1080x1080": { width: 1080, height: 1080, label: "方形" },
  "1200x628": { width: 1200, height: 628, label: "横版" },
  "800x1000": { width: 800, height: 1000, label: "竖版" },
};

const COMPONENT_PRESETS: Omit<PrimeCompositionComponent, "enabled">[] = [
  { id: "logo", label: "品牌 Logo", kind: "image", source_role: "prime_logo", content: "", backdrop_rule: "none" },
  { id: "terms", label: "条款文字", kind: "text", source_role: "", content: "Syarat dan ketentuan berlaku", backdrop_rule: "quiet" },
  { id: "qr", label: "二维码", kind: "qr", source_role: "prime_qr", content: "", backdrop_rule: "light" },
  { id: "store_badges", label: "应用商店标识", kind: "image", source_role: "prime_store_badges", content: "", backdrop_rule: "quiet" },
  { id: "regulatory", label: "监管说明", kind: "text", source_role: "", content: "", backdrop_rule: "quiet" },
  { id: "afpi", label: "AFPI 标识", kind: "image", source_role: "prime_afpi", content: "", backdrop_rule: "none" },
  { id: "pindai_legal", label: "Pindai Legal", kind: "image", source_role: "prime_pindai_legal", content: "", backdrop_rule: "none" },
];

const DEFAULT_NORMALIZED_RECTS: Record<string, PrimeRect> = {
  logo: [0.03, 0.03, 0.35, 0.14],
  terms: [0.64, 0.03, 0.91, 0.14],
  qr: [0.91, 0.03, 0.985, 0.14],
  store_badges: [0.03, 0.89, 0.34, 0.985],
  regulatory: [0.42, 0.89, 0.76, 0.985],
  afpi: [0.76, 0.89, 0.87, 0.985],
  pindai_legal: [0.87, 0.89, 0.985, 0.985],
};

const LEGACY_SOURCE_ROLES = new Set(["prime_square", "prime_landscape", "prime_portrait"]);

export function primeComponentSourceRoles(files: CreativeResourceFile[]): string[] {
  return [...new Set(files
    .filter((file) => file.content_type.startsWith("image/") && !LEGACY_SOURCE_ROLES.has(file.role))
    .map((file) => file.role))];
}

export function createDefaultPrimeComposition(): PrimeComposition {
  const components = COMPONENT_PRESETS.map((component) => ({
    ...component,
    enabled: component.id === "logo" || component.id === "terms" || component.id === "store_badges",
  }));
  return { schema_version: 2, qr_mode: "none", components, layouts: createLayouts(components) };
}

export function readPrimeComposition(value: unknown): PrimeComposition {
  const fallback = createDefaultPrimeComposition();
  if (!value || typeof value !== "object") return fallback;
  const record = value as Record<string, unknown>;
  const qrMode = record.qr_mode === "static" || record.qr_mode === "dynamic" ? record.qr_mode : "none";
  const rawComponents = Array.isArray(record.components) ? record.components : [];
  const rawById = new Map(rawComponents.flatMap((item): [string, Record<string, unknown>][] => {
    if (!item || typeof item !== "object") return [];
    const component = item as Record<string, unknown>;
    return typeof component.id === "string" ? [[component.id, component]] : [];
  }));
  const presetIds = new Set(COMPONENT_PRESETS.map((component) => component.id));
  const components = COMPONENT_PRESETS.map((preset) => readComponent(rawById.get(preset.id), preset))
    .concat(rawComponents.flatMap((item): PrimeCompositionComponent[] => {
      if (!item || typeof item !== "object") return [];
      const component = item as Record<string, unknown>;
      if (typeof component.id !== "string" || presetIds.has(component.id) || !component.id.startsWith("custom_")) return [];
      return [readComponent(component, {
        id: component.id,
        label: typeof component.label === "string" ? component.label : "自定义组件",
        kind: component.kind === "text" ? "text" : "image",
        source_role: `prime_${component.id}`,
        content: "",
        backdrop_rule: "none",
      })];
    }));
  const layouts = createLayouts(components, record.layouts);
  return { schema_version: 2, qr_mode: qrMode, components, layouts };
}

export function updatePrimeCompositionRects(
  composition: PrimeComposition,
  size: PrimeSize,
  id: string,
  patch: Partial<PrimePosition>,
): PrimeComposition {
  const layout = composition.layouts[size];
  const position = layout.components[id];
  if (!position) return composition;
  const meta = SIZE_META[size];
  return {
    ...composition,
    layouts: {
      ...composition.layouts,
      [size]: {
        components: {
          ...layout.components,
          [id]: {
            destination_rect: patch.destination_rect
              ? clampRect(patch.destination_rect, meta.width, meta.height)
              : position.destination_rect,
          },
        },
      },
    },
  };
}

export function removePrimeCompositionComponent(composition: PrimeComposition, id: string): PrimeComposition {
  if (!id.startsWith("custom_") || !composition.components.some((component) => component.id === id)) return composition;
  return {
    ...composition,
    components: composition.components.filter((component) => component.id !== id),
    layouts: Object.fromEntries(PRIME_SIZES.map((size) => [size, {
      components: Object.fromEntries(Object.entries(composition.layouts[size].components).filter(([componentId]) => componentId !== id)),
    }])) as PrimeComposition["layouts"],
  };
}

export function PrimeCompositionEditor({ resource, value, onChange, qrPayload = "", onQRPayloadChange }: {
  resource: CreativeResource;
  value: unknown;
  onChange: (value: PrimeComposition) => void;
  qrPayload?: string;
  onQRPayloadChange?: (value: string) => void;
}) {
  const wsId = useWorkspaceId();
  const files = useQuery(creativeResourceFilesOptions(wsId, resource.id));
  const composition = useMemo(() => readPrimeComposition(value), [value]);
  const resourceFiles = files.data?.files ?? [];
  const [size, setSize] = useState<PrimeSize>("1080x1080");
  const [activeId, setActiveId] = useState(composition.components[0]?.id ?? "logo");
  const [expanded, setExpanded] = useState(false);
  const [previewAll, setPreviewAll] = useState(false);
  const [canvasZoom, setCanvasZoom] = useState(1);
  const active = composition.components.find((component) => component.id === activeId) ?? composition.components[0];
  const layout = composition.layouts[size];
  const missingCount = composition.components.filter((component) => component.enabled && componentMissing(component, composition.qr_mode, resourceFiles)).length;

  const updateComponent = (id: string, patch: Partial<PrimeCompositionComponent>) => {
    onChange({ ...composition, components: composition.components.map((component) => component.id === id ? { ...component, ...patch } : component) });
  };
  const setQRMode = (qrMode: PrimeQRMode) => {
    onChange({
      ...composition,
      qr_mode: qrMode,
      components: composition.components.map((component) => component.id === "qr" ? { ...component, enabled: qrMode !== "none" } : component),
    });
  };
  const updateRects = (id: string, patch: Partial<PrimePosition>) => onChange(updatePrimeCompositionRects(composition, size, id, patch));
  const requestResource = (role: string) => {
    window.dispatchEvent(new CustomEvent(PRIME_RESOURCE_UPLOAD_EVENT, { detail: { role } }));
  };
  const addCustom = () => {
    const sequence = composition.components.reduce((largest, component) => {
      const match = /^custom_(\d+)$/.exec(component.id);
      return match ? Math.max(largest, Number(match[1])) : largest;
    }, 0) + 1;
    const component: PrimeCompositionComponent = {
      id: `custom_${sequence}`,
      label: `自定义组件 ${sequence}`,
      kind: "image",
      enabled: true,
      source_role: `prime_custom_${sequence}`,
      content: "",
      backdrop_rule: "none",
    };
    onChange({ ...composition, components: [...composition.components, component], layouts: addComponentToLayouts(composition.layouts, component.id) });
    setActiveId(component.id);
  };
  const removeCustom = (id: string) => {
    const next = removePrimeCompositionComponent(composition, id);
    onChange(next);
    setActiveId(next.components[0]?.id ?? "");
  };
  const editor = (large: boolean) => <div className={cn("grid overflow-hidden border lg:grid-cols-[240px_minmax(0,1fr)_280px]", large ? "h-full min-h-0" : "min-h-[560px]") }>
    <div className={cn("border-b bg-muted/10 lg:border-b-0 lg:border-r", large && "min-h-0 overflow-y-auto") }>
      <div className="border-b px-3 py-3"><Label className="text-xs text-muted-foreground">二维码方式</Label><div className="mt-2 grid gap-1" role="group" aria-label="二维码方式"><Button size="sm" variant={composition.qr_mode === "none" ? "secondary" : "ghost"} className="justify-start" onClick={() => setQRMode("none")}>不使用二维码</Button><Button size="sm" variant={composition.qr_mode === "static" ? "secondary" : "ghost"} className="justify-start" onClick={() => setQRMode("static")}>使用二维码图片</Button><Button size="sm" variant={composition.qr_mode === "dynamic" ? "secondary" : "ghost"} className="justify-start" onClick={() => setQRMode("dynamic")}>根据地址生成</Button></div>{composition.qr_mode === "dynamic" && <div className="mt-3 space-y-1.5"><Label htmlFor={`prime-qr-payload-${large ? "full" : "inline"}`} className="text-[11px] text-muted-foreground">二维码目标地址</Label><Input id={`prime-qr-payload-${large ? "full" : "inline"}`} type="url" value={qrPayload} placeholder="https://..." onChange={(event) => onQRPayloadChange?.(event.target.value)} /></div>}</div>
      <div className="divide-y">{composition.components.map((component) => {
        const missing = component.enabled && componentMissing(component, composition.qr_mode, resourceFiles);
        const isCustom = component.id.startsWith("custom_");
        return <div key={component.id} className={cn("flex items-center gap-2 px-3 py-3 hover:bg-muted/40", component.id === active?.id && "bg-background shadow-[inset_2px_0_0_hsl(var(--primary))]")}>
          <Switch checked={component.enabled} disabled={component.id === "qr" && composition.qr_mode === "none"} onCheckedChange={(enabled) => updateComponent(component.id, { enabled })} aria-label={`${component.label}${component.enabled ? "已启用" : "已停用"}`} />
          <button type="button" onClick={() => setActiveId(component.id)} className="min-w-0 flex-1 text-left">
            <span className="block truncate text-sm font-medium">{component.label}</span>
            <span className={cn("mt-0.5 flex items-center gap-1 text-[11px]", missing ? "text-amber-700 dark:text-amber-400" : "text-muted-foreground")}>{!component.enabled ? "不显示" : missing ? component.kind === "text" ? "待填写内容" : "待上传图片" : <><Check className="h-3 w-3" />{component.kind === "qr" && composition.qr_mode === "dynamic" ? "将自动生成" : "已就绪"}</>}</span>
          </button>
          {isCustom && <Button size="icon-sm" variant="ghost" className="shrink-0 text-muted-foreground hover:text-destructive" title={`删除${component.label}`} aria-label={`删除${component.label}`} onClick={() => removeCustom(component.id)}><Trash2 className="h-4 w-4" /></Button>}
        </div>;
      })}</div>
    </div>
    <div className="flex min-h-0 min-w-0 flex-col bg-muted/15">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b bg-background px-3 py-2"><Tabs value={size} onValueChange={(next) => setSize(next as PrimeSize)}><TabsList className="h-8">{PRIME_SIZES.map((target) => <TabsTrigger key={target} value={target}>{SIZE_META[target].label}</TabsTrigger>)}</TabsList></Tabs><span className="flex items-center gap-1 text-xs text-muted-foreground"><Move className="h-3.5 w-3.5" />拖动组件调整位置</span></div>
      <div className={cn("flex flex-1 items-start justify-center overflow-auto", large ? "min-h-0 p-4" : "min-h-[460px] items-center p-5") }><PrimeCanvas composition={composition} size={size} activeId={active?.id ?? ""} files={resourceFiles} zoom={large ? canvasZoom : 1} expanded={large} onSelect={setActiveId} onRectsChange={updateRects} /></div>
      <div className="border-t bg-background px-3 py-2 text-xs text-muted-foreground">三个尺寸共用同一套组件资源；这里只调整当前尺寸的位置和大小。</div>
    </div>
    <div className={cn("border-t bg-background p-4 lg:border-l lg:border-t-0", large && "min-h-0 overflow-y-auto") }>{active ? <PrimeComponentInspector component={active} size={size} position={layout.components[active.id]} qrMode={composition.qr_mode} files={resourceFiles} onComponentChange={(patch) => updateComponent(active.id, patch)} onRectChange={(rect) => updateRects(active.id, { destination_rect: rect })} onRequestResource={requestResource} onRemove={() => removeCustom(active.id)} /> : <p className="text-sm text-muted-foreground">选择一个组件开始配置。</p>}</div>
  </div>;

  return <section className="space-y-4">
    <div className="flex flex-wrap items-end justify-between gap-3"><div><div className="flex items-center gap-2"><Layers3 className="h-4 w-4" /><h3 className="text-sm font-semibold">成图品牌布局</h3>{missingCount > 0 && <Badge variant="secondary">{missingCount} 项待补充</Badge>}</div><p className="mt-1 text-xs text-muted-foreground">同一份组件自动应用到三个尺寸，分别调整位置和大小。</p></div><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => setPreviewAll(true)}><Eye className="h-4 w-4" />预览三尺寸</Button><Button size="sm" variant="outline" onClick={() => setExpanded(true)}><Maximize2 className="h-4 w-4" />精细调整</Button><Button size="sm" variant="outline" onClick={addCustom}><Plus className="h-4 w-4" />添加组件</Button></div></div>
    {!expanded && editor(false)}
    {expanded && <Dialog open onOpenChange={setExpanded}><DialogContent className="grid h-[96vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[98vw]"><DialogHeader className="flex-row items-center justify-between border-b px-4 py-3 pr-14"><DialogTitle>Prime 精细编辑</DialogTitle><div className="flex items-center gap-1"><Button size="icon-sm" variant="outline" title="缩小画布" aria-label="缩小画布" disabled={canvasZoom <= 0.75} onClick={() => setCanvasZoom((current) => Math.max(0.75, Number((current - 0.25).toFixed(2))))}><ZoomOut className="h-4 w-4" /></Button><span className="w-12 text-center text-xs text-muted-foreground">{Math.round(canvasZoom * 100)}%</span><Button size="icon-sm" variant="outline" title="放大画布" aria-label="放大画布" disabled={canvasZoom >= 2} onClick={() => setCanvasZoom((current) => Math.min(2, Number((current + 0.25).toFixed(2))))}><ZoomIn className="h-4 w-4" /></Button></div></DialogHeader><div className="min-h-0 p-3">{editor(true)}</div></DialogContent></Dialog>}
    {previewAll && <Dialog open onOpenChange={setPreviewAll}><DialogContent className="max-h-[94vh] overflow-y-auto sm:max-w-[min(94vw,1180px)]"><DialogHeader><DialogTitle>三尺寸预览</DialogTitle></DialogHeader><div className="grid items-start gap-4 lg:grid-cols-3">{PRIME_SIZES.map((target) => <div key={target} className="min-w-0"><div className="mb-2 flex items-center justify-between text-xs"><span className="font-medium">{SIZE_META[target].label}</span><span className="text-muted-foreground">{target}</span></div><PrimeOverview composition={composition} size={target} files={resourceFiles} /></div>)}</div></DialogContent></Dialog>}
  </section>;
}

function PrimeOverview({ composition, size, files }: { composition: PrimeComposition; size: PrimeSize; files: CreativeResourceFile[] }) {
  const meta = SIZE_META[size];
  const layout = composition.layouts[size];
  return <div className="relative w-full overflow-hidden border bg-muted/20" style={{ aspectRatio: `${meta.width} / ${meta.height}` }}>
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center"><span className="border bg-background/90 px-2 py-1 text-[10px] text-muted-foreground">创意底图区域</span></div>
    {composition.components.filter((component) => component.enabled).map((component) => {
      const position = layout.components[component.id];
      if (!position) return null;
      return <PrimeLayer key={component.id} component={component} destination={position.destination_rect} canvas={meta} sourceFile={findComponentFile(files, component.source_role)} qrMode={composition.qr_mode} />;
    })}
  </div>;
}

function PrimeCanvas({ composition, size, activeId, files, zoom, expanded, onSelect, onRectsChange }: {
  composition: PrimeComposition;
  size: PrimeSize;
  activeId: string;
  files: CreativeResourceFile[];
  zoom: number;
  expanded: boolean;
  onSelect: (id: string) => void;
  onRectsChange: (id: string, patch: Partial<PrimePosition>) => void;
}) {
  const canvasRef = useRef<HTMLDivElement>(null);
  const meta = SIZE_META[size];
  const layout = composition.layouts[size];
  const drag = useRef<{ id: string; mode: "move" | "resize"; startX: number; startY: number; rect: PrimeRect } | null>(null);
  const coordinate = (event: React.PointerEvent) => {
    const bounds = canvasRef.current?.getBoundingClientRect();
    if (!bounds) return { x: 0, y: 0 };
    return { x: Math.round((event.clientX - bounds.left) / bounds.width * meta.width), y: Math.round((event.clientY - bounds.top) / bounds.height * meta.height) };
  };
  const startDrag = (event: React.PointerEvent, id: string, mode: "move" | "resize" = "move") => {
    event.preventDefault();
    event.stopPropagation();
    onSelect(id);
    const point = coordinate(event);
    const existing = layout.components[id]?.destination_rect ?? [point.x, point.y, point.x + 1, point.y + 1];
    drag.current = { id, mode, startX: point.x, startY: point.y, rect: existing };
    event.currentTarget.setPointerCapture(event.pointerId);
  };
  const moveDrag = (event: React.PointerEvent) => {
    if (!drag.current) return;
    const point = coordinate(event);
    const current = drag.current;
    const [x1, y1, x2, y2] = current.rect;
    const dx = point.x - current.startX;
    const dy = point.y - current.startY;
    onRectsChange(current.id, {
      destination_rect: current.mode === "resize"
        ? [x1, y1, Math.max(x1 + 1, x2 + dx), Math.max(y1 + 1, y2 + dy)]
        : [x1 + dx, y1 + dy, x2 + dx, y2 + dy],
    });
  };
  const endDrag = (event: React.PointerEvent) => {
    drag.current = null;
    if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
  };
  return <div ref={canvasRef} data-testid="prime-composition-canvas" onPointerMove={moveDrag} onPointerUp={endDrag} className="relative shrink-0 touch-none overflow-hidden border bg-background shadow-sm" style={{ aspectRatio: `${meta.width} / ${meta.height}`, width: `${zoom * 100}%`, maxWidth: expanded ? `${1200 * zoom}px` : "760px" }}>
    <div className="pointer-events-none absolute inset-0 flex items-center justify-center bg-muted/30"><span className="border px-3 py-2 text-[11px] text-muted-foreground">创意底图区域</span></div>
    {composition.components.filter((component) => component.enabled).map((component) => {
      const position = layout.components[component.id];
      if (!position) return null;
      return <PrimeLayer key={component.id} component={component} destination={position.destination_rect} canvas={meta} sourceFile={findComponentFile(files, component.source_role)} qrMode={composition.qr_mode} />;
    })}
    {composition.components.filter((component) => component.enabled).map((component) => {
      const position = layout.components[component.id];
      if (!position) return null;
      const selected = component.id === activeId;
      return <div key={component.id} role="button" tabIndex={0} aria-label={`选择组件 ${component.label}`} onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") { event.preventDefault(); onSelect(component.id); return; }
        const offset = event.shiftKey ? 10 : 1;
        const [x1, y1, x2, y2] = position.destination_rect;
        const delta: [number, number] | null = event.key === "ArrowLeft" ? [-offset, 0] : event.key === "ArrowRight" ? [offset, 0] : event.key === "ArrowUp" ? [0, -offset] : event.key === "ArrowDown" ? [0, offset] : null;
        if (!delta) return;
        event.preventDefault();
        onSelect(component.id);
        onRectsChange(component.id, { destination_rect: [x1 + delta[0], y1 + delta[1], x2 + delta[0], y2 + delta[1]] });
      }} onPointerDown={(event) => startDrag(event, component.id)} className={cn("absolute cursor-move border border-emerald-600 bg-emerald-500/5 text-[10px] font-medium text-emerald-950 outline-none focus-visible:ring-2 focus-visible:ring-ring dark:text-emerald-100", selected && "z-20 border-2 border-primary bg-primary/10 text-foreground") } style={rectStyle(position.destination_rect, meta.width, meta.height)}><span className="absolute left-0 top-0 max-w-full truncate bg-background/90 px-1 py-0.5">{component.label}</span>{selected && <span aria-hidden="true" onPointerDown={(event) => startDrag(event, component.id, "resize")} className="absolute -bottom-1.5 -right-1.5 size-3 cursor-nwse-resize border-2 border-background bg-primary shadow-sm" />}</div>;
    })}
  </div>;
}

function PrimeLayer({ component, destination, canvas, sourceFile, qrMode }: { component: PrimeCompositionComponent; destination: PrimeRect; canvas: { width: number; height: number }; sourceFile?: CreativeResourceFile; qrMode: PrimeQRMode }) {
  const style = rectStyle(destination, canvas.width, canvas.height);
  if (component.kind === "text") return <div className="pointer-events-none absolute flex items-center whitespace-pre-line px-1 text-[clamp(5px,1vw,13px)] font-medium leading-tight text-foreground" style={style}>{component.content || "待填写文字"}</div>;
  if (component.kind === "qr" && qrMode === "dynamic") return <div className="pointer-events-none absolute flex items-center justify-center bg-background" style={style}><QrCode className="h-2/3 w-2/3" /></div>;
  const source = sourceFile ? resolvePublicFileUrl(sourceFile.url) : "";
  return <div className="pointer-events-none absolute flex items-center justify-center overflow-hidden" style={style}>{source ? <img src={source} alt="" className="h-full w-full object-contain" /> : <span className="flex h-full w-full items-center justify-center border border-dashed bg-background/80"><ImageIcon className="h-4 w-4 text-muted-foreground" /></span>}</div>;
}

function PrimeComponentInspector({ component, size, position, qrMode, files, onComponentChange, onRectChange, onRequestResource, onRemove }: { component: PrimeCompositionComponent; size: PrimeSize; position?: PrimePosition; qrMode: PrimeQRMode; files: CreativeResourceFile[]; onComponentChange: (value: Partial<PrimeCompositionComponent>) => void; onRectChange: (rect: PrimeRect) => void; onRequestResource: (role: string) => void; onRemove: () => void }) {
  if (!position) return null;
  const rect = position.destination_rect;
  const meta = SIZE_META[size];
  const sourceRoles = primeComponentSourceRoles(files);
  const isCustom = component.id.startsWith("custom_");
  const needsSource = component.kind === "image" || (component.kind === "qr" && qrMode === "static");
  const sourceFile = findComponentFile(files, component.source_role);
  const componentKindLabel = component.kind === "text" ? "文字" : component.kind === "qr" ? "二维码" : "图片";
  const setBounds = (field: "x" | "y" | "width" | "height", value: string) => {
    const parsed = Number(value) || 0;
    const [x1, y1, x2, y2] = rect;
    if (field === "x") onRectChange([parsed, y1, parsed + rectWidth(rect), y2]);
    if (field === "y") onRectChange([x1, parsed, x2, parsed + rectHeight(rect)]);
    if (field === "width") onRectChange([x1, y1, x1 + Math.max(1, parsed), y2]);
    if (field === "height") onRectChange([x1, y1, x2, y1 + Math.max(1, parsed)]);
  };
  return <div className="space-y-5">
    <div className="flex items-start justify-between gap-3"><div className="min-w-0"><p className="truncate text-sm font-semibold">{component.label}</p><p className="mt-0.5 text-xs text-muted-foreground">{componentKindLabel}组件</p></div><Badge variant={component.enabled ? "secondary" : "outline"}>{component.enabled ? "显示" : "隐藏"}</Badge></div>
    {isCustom && <div><Label className="text-xs text-muted-foreground">组件名称</Label><Input className="mt-1.5" value={component.label} onChange={(event) => onComponentChange({ label: event.target.value })} /></div>}
    {component.kind === "text" && <div><Label className="text-xs text-muted-foreground">显示内容</Label><Textarea className="mt-1.5" rows={5} value={component.content} placeholder="输入条款或监管说明" onChange={(event) => onComponentChange({ content: event.target.value })} /></div>}
    {needsSource && <div><Label className="text-xs text-muted-foreground">组件图片</Label>{isCustom && sourceRoles.length > 0 && <NativeSelect className="mt-1.5" value={component.source_role} onChange={(event) => onComponentChange({ source_role: event.target.value })}><NativeSelectOption value="">选择已上传图片</NativeSelectOption>{sourceRoles.map((role) => { const file = findComponentFile(files, role); return <NativeSelectOption key={role} value={role}>{file?.label || file?.filename || purposeLabel(role)}</NativeSelectOption>; })}</NativeSelect>}{sourceFile ? <div className="mt-2 flex items-center gap-2 border bg-muted/20 p-2"><FileImage className="h-4 w-4 shrink-0" /><span className="min-w-0 flex-1 truncate text-xs">{sourceFile.label || sourceFile.filename}</span><Button size="sm" variant="ghost" onClick={() => onRequestResource(component.source_role)}>替换</Button></div> : <div className="mt-2 border border-dashed p-3"><p className="text-xs text-amber-700 dark:text-amber-400">启用前需要上传{component.label}图片。</p><Button size="sm" variant="outline" className="mt-2" onClick={() => onRequestResource(component.source_role)}><Upload className="h-4 w-4" />上传图片</Button></div>}</div>}
    <div><div className="flex items-center justify-between"><Label className="text-xs text-muted-foreground">{SIZE_META[size].label}位置与大小</Label><span className="text-[11px] text-muted-foreground">{meta.width} × {meta.height}</span></div><div className="mt-2 grid grid-cols-2 gap-2"><CoordinateInput label="左" value={rect[0]} max={meta.width} onChange={(value) => setBounds("x", value)} /><CoordinateInput label="上" value={rect[1]} max={meta.height} onChange={(value) => setBounds("y", value)} /><CoordinateInput label="宽" value={rectWidth(rect)} max={meta.width} onChange={(value) => setBounds("width", value)} /><CoordinateInput label="高" value={rectHeight(rect)} max={meta.height} onChange={(value) => setBounds("height", value)} /></div><p className="mt-2 text-[11px] text-muted-foreground">可在画布上拖动；方向键微调 1 px，Shift + 方向键微调 10 px。</p></div>
    <div className="border-t pt-4 text-xs text-muted-foreground">图片替换后会同步用于三个尺寸，位置仍按各尺寸独立保存。</div>
    {isCustom && <Button variant="outline" className="w-full text-destructive hover:text-destructive" onClick={onRemove}><Trash2 className="h-4 w-4" />删除此组件</Button>}
  </div>;
}

function CoordinateInput({ label, value, max, onChange }: { label: string; value: number; max: number; onChange: (value: string) => void }) {
  return <label className="grid grid-cols-[24px_minmax(0,1fr)] items-center gap-1 text-[11px] text-muted-foreground"><span>{label}</span><Input type="number" min={0} max={max} value={value} onChange={(event) => onChange(event.target.value)} className="h-8 text-xs" /></label>;
}

function readComponent(raw: Record<string, unknown> | undefined, preset: Omit<PrimeCompositionComponent, "enabled">): PrimeCompositionComponent {
  const kind: PrimeComponentKind = raw?.kind === "text" || raw?.kind === "qr" ? raw.kind : preset.kind;
  const backdrop = raw?.backdrop_rule === "quiet" || raw?.backdrop_rule === "light" ? raw.backdrop_rule : preset.backdrop_rule;
  const rawRole = typeof raw?.source_role === "string" ? raw.source_role.trim() : "";
  return {
    id: preset.id,
    label: typeof raw?.label === "string" ? raw.label : preset.label,
    kind,
    enabled: raw ? raw.enabled === true : preset.id === "logo" || preset.id === "terms" || preset.id === "store_badges",
    source_role: rawRole && !LEGACY_SOURCE_ROLES.has(rawRole) ? rawRole : preset.source_role,
    content: typeof raw?.content === "string" ? raw.content : preset.content,
    backdrop_rule: backdrop,
  };
}

function createLayouts(components: PrimeCompositionComponent[], rawValue?: unknown): PrimeComposition["layouts"] {
  const rawLayouts = rawValue && typeof rawValue === "object" ? rawValue as Record<string, unknown> : {};
  return Object.fromEntries(PRIME_SIZES.map((size) => {
    const meta = SIZE_META[size];
    const rawLayout = rawLayouts[size] && typeof rawLayouts[size] === "object" ? rawLayouts[size] as Record<string, unknown> : {};
    const rawPositions = rawLayout.components && typeof rawLayout.components === "object" ? rawLayout.components as Record<string, unknown> : {};
    const positions = Object.fromEntries(components.map((component) => {
      const normalized = DEFAULT_NORMALIZED_RECTS[component.id] ?? [0.1, 0.1, 0.3, 0.2];
      const fallback = normalizedRect(normalized, meta.width, meta.height);
      const raw = rawPositions[component.id] && typeof rawPositions[component.id] === "object" ? rawPositions[component.id] as Record<string, unknown> : {};
      return [component.id, { destination_rect: clampRect(readRect(raw.destination_rect, fallback), meta.width, meta.height) }];
    }));
    return [size, { components: positions }];
  })) as PrimeComposition["layouts"];
}

function addComponentToLayouts(layouts: PrimeComposition["layouts"], id: string): PrimeComposition["layouts"] {
  return Object.fromEntries(PRIME_SIZES.map((size) => {
    const meta = SIZE_META[size];
    return [size, { components: { ...layouts[size].components, [id]: { destination_rect: normalizedRect([0.1, 0.1, 0.3, 0.2], meta.width, meta.height) } } }];
  })) as PrimeComposition["layouts"];
}

function componentMissing(component: PrimeCompositionComponent, qrMode: PrimeQRMode, files: CreativeResourceFile[]): boolean {
  if (component.kind === "text") return !component.content.trim();
  if (component.kind === "qr" && qrMode === "dynamic") return false;
  if (component.kind === "qr" && qrMode === "none") return false;
  return !component.source_role || !findComponentFile(files, component.source_role);
}

function findComponentFile(files: CreativeResourceFile[], role: string): CreativeResourceFile | undefined {
  return [...files].reverse().find((file) => file.role === role && file.content_type.startsWith("image/"));
}

function normalizedRect(rect: PrimeRect, width: number, height: number): PrimeRect {
  return [Math.round(rect[0] * width), Math.round(rect[1] * height), Math.round(rect[2] * width), Math.round(rect[3] * height)];
}

function readRect(value: unknown, fallback: PrimeRect): PrimeRect {
  if (!Array.isArray(value) || value.length !== 4 || value.some((entry) => typeof entry !== "number" || !Number.isFinite(entry))) return fallback;
  return [value[0], value[1], value[2], value[3]];
}

function clampRect(rect: PrimeRect, width: number, height: number): PrimeRect {
  const rectWidthValue = Math.max(1, Math.min(width, rectWidth(rect)));
  const rectHeightValue = Math.max(1, Math.min(height, rectHeight(rect)));
  const x1 = Math.max(0, Math.min(width - rectWidthValue, rect[0]));
  const y1 = Math.max(0, Math.min(height - rectHeightValue, rect[1]));
  return [Math.round(x1), Math.round(y1), Math.round(x1 + rectWidthValue), Math.round(y1 + rectHeightValue)];
}

function rectStyle(rect: PrimeRect, width: number, height: number): React.CSSProperties {
  return { left: `${rect[0] / width * 100}%`, top: `${rect[1] / height * 100}%`, width: `${rectWidth(rect) / width * 100}%`, height: `${rectHeight(rect) / height * 100}%` };
}

function rectWidth(rect: PrimeRect): number { return Math.max(1, rect[2] - rect[0]); }
function rectHeight(rect: PrimeRect): number { return Math.max(1, rect[3] - rect[1]); }
