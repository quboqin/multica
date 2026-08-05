import type {
  PrimeComposition,
  PrimeCompositionComponent,
  PrimeRect,
  PrimeSize,
} from "../components/prime-composition-editor";
import type { CreativeMarketPackComponentExtraction } from "@multica/core/types";

export const PRIME_IMPORT_SIZE_META: Record<PrimeSize, { width: number; height: number; label: string }> = {
  "1080x1080": { width: 1080, height: 1080, label: "方形" },
  "1200x628": { width: 1200, height: 628, label: "横版" },
  "800x1000": { width: 800, height: 1000, label: "竖版" },
};

export type PrimeImportCandidate = {
  componentId: string;
  label: string;
  role: string;
  kind: "image" | "qr" | "text";
  content: string;
  rect: PrimeRect;
  selected: boolean;
  confidence: number;
  evidence: string[];
};

export function inferPrimeImportSize(width: number, height: number): PrimeSize | null {
  if (width <= 0 || height <= 0) return null;
  const sourceRatio = width / height;
  const closest = (Object.entries(PRIME_IMPORT_SIZE_META) as [PrimeSize, (typeof PRIME_IMPORT_SIZE_META)[PrimeSize]][])
    .map(([size, meta]) => ({ size, difference: Math.abs(sourceRatio - meta.width / meta.height) }))
    .sort((left, right) => left.difference - right.difference)[0];
  return closest && closest.difference <= 0.08 ? closest.size : null;
}

export function primeImportCandidates(composition: PrimeComposition, size: PrimeSize): PrimeImportCandidate[] {
  const positions = composition.layouts[size]?.components ?? {};
  return composition.components.flatMap((component): PrimeImportCandidate[] => {
    if ((component.kind !== "image" && component.kind !== "qr") || !component.source_role) return [];
    const position = positions[component.id];
    if (!position) return [];
    return [{
      componentId: component.id,
      label: component.label,
      role: component.source_role,
      kind: component.kind,
      content: component.content,
      rect: [...position.destination_rect],
      selected: component.enabled,
      confidence: 1,
      evidence: [],
    }];
  });
}

export function detectedPrimeImportCandidates(
  extraction: CreativeMarketPackComponentExtraction,
  composition: PrimeComposition,
  size: PrimeSize,
): PrimeImportCandidate[] {
  const standardRoles: Record<string, string> = {
    logo: "prime_logo",
    qr: "prime_qr",
    store_badges: "prime_store_badges",
    afpi: "prime_afpi",
    pindai_legal: "prime_pindai_legal",
  };
  const standardKinds: Record<string, PrimeImportCandidate["kind"]> = {
    logo: "image",
    terms: "text",
    qr: "qr",
    store_badges: "image",
    regulatory: "text",
    afpi: "image",
    pindai_legal: "image",
  };
  const meta = PRIME_IMPORT_SIZE_META[size];
  const usedIds = new Set<string>();
  let customSequence = composition.components.reduce((largest, component) => {
    const match = /^custom_(\d+)$/.exec(component.id);
    return match ? Math.max(largest, Number(match[1])) : largest;
  }, 0);
  const existingIds = new Set(composition.components.map((component) => component.id));
  const nextCustom = () => {
    do { customSequence += 1; } while (existingIds.has(`custom_${customSequence}`) || usedIds.has(`custom_${customSequence}`));
    return `custom_${customSequence}`;
  };
  return extraction.result.candidates.map((candidate) => {
    const suggested = candidate.suggested_component_id;
    const componentId = suggested && existingIds.has(suggested) && !usedIds.has(suggested) ? suggested : nextCustom();
    usedIds.add(componentId);
    const kind = standardKinds[componentId] ?? (candidate.kind === "qr" ? "image" : candidate.kind);
    const role = kind === "text"
      ? ""
      : componentId.startsWith("custom_")
        ? `prime_${componentId}`
        : standardRoles[componentId] || candidate.suggested_role;
    return {
      componentId,
      label: candidate.label,
      role,
      kind,
      content: candidate.content,
      rect: scaleRect(candidate.rect, extraction.source_width, extraction.source_height, meta.width, meta.height),
      selected: candidate.kind !== "qr" || componentId === "qr",
      confidence: candidate.confidence,
      evidence: candidate.evidence,
    };
  });
}

export function applyPrimeLayoutImport(
  composition: PrimeComposition,
  sourceSize: PrimeSize,
  candidates: PrimeImportCandidate[],
): PrimeComposition {
  const decisions = new Map(candidates.map((candidate) => [candidate.componentId, candidate]));
  const selected = candidates.filter((candidate) => candidate.selected);
  const qrSelected = selected.some((candidate) => candidate.kind === "qr");
  const components = composition.components.map((component) => {
    const decision = decisions.get(component.id);
    if (!decision) return component;
    return { ...component, enabled: decision.selected, content: decision.kind === "text" ? decision.content : component.content };
  });
  const existingComponentIds = new Set(components.map((component) => component.id));
  for (const candidate of selected) {
    if (existingComponentIds.has(candidate.componentId)) continue;
    components.push({
      id: candidate.componentId,
      label: candidate.label,
      kind: candidate.kind,
      enabled: true,
      source_role: candidate.role,
      content: candidate.content,
      backdrop_rule: candidate.kind === "text" ? "quiet" : candidate.kind === "qr" ? "light" : "none",
    });
    existingComponentIds.add(candidate.componentId);
  }
  const layouts = Object.fromEntries((Object.keys(PRIME_IMPORT_SIZE_META) as PrimeSize[]).map((targetSize) => {
    const targetMeta = PRIME_IMPORT_SIZE_META[targetSize];
    const sourceMeta = PRIME_IMPORT_SIZE_META[sourceSize];
    const current = composition.layouts[targetSize];
    const imported = Object.fromEntries(selected.map((candidate) => [
      candidate.componentId,
      { destination_rect: scaleRect(candidate.rect, sourceMeta.width, sourceMeta.height, targetMeta.width, targetMeta.height) },
    ]));
    return [targetSize, { components: { ...current.components, ...imported } }];
  })) as PrimeComposition["layouts"];

  return {
    ...composition,
    qr_mode: qrSelected ? "static" : "none",
    components,
    layouts,
  };
}

export function scalePrimeImportRect(
  rect: PrimeRect,
  sourceSize: PrimeSize,
  imageWidth: number,
  imageHeight: number,
): PrimeRect {
  const meta = PRIME_IMPORT_SIZE_META[sourceSize];
  return scaleRect(rect, meta.width, meta.height, imageWidth, imageHeight);
}

export function updatePrimeImportCandidateRect(
  candidate: PrimeImportCandidate,
  patch: Partial<{ x: number; y: number; width: number; height: number }>,
  size: PrimeSize,
): PrimeImportCandidate {
  const meta = PRIME_IMPORT_SIZE_META[size];
  const [x1, y1, x2, y2] = candidate.rect;
  const width = Math.max(1, patch.width ?? x2 - x1);
  const height = Math.max(1, patch.height ?? y2 - y1);
  const left = patch.x ?? x1;
  const top = patch.y ?? y1;
  return { ...candidate, rect: clampRect([left, top, left + width, top + height], meta.width, meta.height) };
}

export function primeImportComponentById(
  components: PrimeCompositionComponent[],
  componentId: string,
): PrimeCompositionComponent | undefined {
  return components.find((component) => component.id === componentId);
}

function scaleRect(rect: PrimeRect, sourceWidth: number, sourceHeight: number, targetWidth: number, targetHeight: number): PrimeRect {
  return clampRect([
    Math.round(rect[0] * targetWidth / sourceWidth),
    Math.round(rect[1] * targetHeight / sourceHeight),
    Math.round(rect[2] * targetWidth / sourceWidth),
    Math.round(rect[3] * targetHeight / sourceHeight),
  ], targetWidth, targetHeight);
}

function clampRect(rect: PrimeRect, width: number, height: number): PrimeRect {
  const rectWidth = Math.max(1, Math.min(width, rect[2] - rect[0]));
  const rectHeight = Math.max(1, Math.min(height, rect[3] - rect[1]));
  const x = Math.max(0, Math.min(width - rectWidth, rect[0]));
  const y = Math.max(0, Math.min(height - rectHeight, rect[1]));
  return [Math.round(x), Math.round(y), Math.round(x + rectWidth), Math.round(y + rectHeight)];
}
