"use client";

import { Clock3, FileText, Image as ImageIcon, Layers3, Sparkles } from "lucide-react";
import type { CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { attachmentDownloadPath } from "@multica/core/types";
import { creativePrimeConfig, creativeAssetPrimeComposition, type CreativePrimeComposition } from "@multica/core/creative";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { Badge } from "@multica/ui/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { useT } from "../../i18n";
import { formatCreativeDateTime } from "../lib/creative-time";
import { creativePrimeFamilyLabel, creativePrimeModeLabel } from "./creative-prime-mode";

type GenerationFact = { label: string; value: string };
type GenerationCopyLine = { field: keyof GenerationInfoLabels["copyFields"]; label: string; value: string };

type GenerationInfoLabels = {
  layoutContract: string;
  layoutContractVersion: (version: string) => string;
  protectedRegions: (count: number) => string;
  backdropRule: (rule: string) => string;
  marketBound: string;
  copyFields: Record<"headline" | "subheadline" | "benefit" | "supporting" | "cta" | "legal", string>;
  sizeLabel: (size: string) => string;
  stageLabel: (stage: string) => string;
};

const DEFAULT_GENERATION_INFO_LABELS: GenerationInfoLabels = {
  layoutContract: "Prime layout contract",
  layoutContractVersion: (version) => `Prime layout contract v${version}`,
  protectedRegions: (count) => `${count} protected regions`,
  backdropRule: (rule) => `Backdrop rule ${rule}`,
  marketBound: "Market resource pack is bound",
  copyFields: { headline: "Headline", subheadline: "Subheadline", benefit: "Benefit", supporting: "Supporting information", cta: "Call to action", legal: "Legal copy" },
  sizeLabel: creativeSizeLabel,
  stageLabel,
};

export type CreativeGenerationInfo = {
  assetId: string;
  variantLabel: string;
  sizeLabel: string;
  revision: number;
  direction: string;
  copyLines: GenerationCopyLine[];
  repaymentPlans: GenerationFact[];
  model: string;
  provider: string;
  marketRule: string;
  marketResource: string;
  createdAt: string;
  prompt: string;
  promptSha256: string;
  requestId: string;
  attempts: string;
  lineage: CreativeOrderAsset[];
  primeConfigured: CreativePrimeComposition;
  primeActual: CreativePrimeComposition;
};

export function creativeGenerationInfo(
  item: CreativeOrderItem,
  variant: CreativeOrderVariant,
  asset: CreativeOrderAsset,
  labels: GenerationInfoLabels = DEFAULT_GENERATION_INFO_LABELS,
): CreativeGenerationInfo {
  const lineage = creativeAssetLineage(variant, asset);
  const generated = lineage.find((candidate) => candidate.stage === "generated");
  const primed = lineage.find((candidate) => candidate.stage === "primed");
  const snapshot = record(item.copy_snapshot);
  const brief = record(variant.brief);
  const revisionBrief = record(variant.revisions?.find((entry) => entry.revision === asset.revision)?.brief ?? (variant.revision === asset.revision ? variant.brief : undefined));
  const primeContract = record(revisionBrief.prime_composition);
  const primeConfigured = creativePrimeConfig({ prime_composition_mode: primeContract.mode ?? "unknown", prime_model_template_family: primeContract.template_family_id });
  const primeActual = creativeAssetPrimeComposition([...lineage].reverse().find((entry) => creativeAssetPrimeComposition(entry).mode !== "unknown"));
  const generatedMetadata = record(generated?.metadata);
  const generatedEvidence = record(generated?.evidence);
  const promptSources = [generatedMetadata, generatedEvidence, record(asset.metadata), record(asset.evidence), record(primed?.metadata), record(primed?.evidence)];
  const evidenceSources = [generatedEvidence, record(asset.evidence), record(primed?.evidence), generatedMetadata, record(asset.metadata), record(primed?.metadata)];
  const marketSources = [record(primed?.metadata), record(primed?.evidence), record(asset.metadata), record(asset.evidence), brief];
  const layoutContract = record(brief.prime_layout_contract);
  const hardRegions = array(layoutContract.hard_regions);
  const backdropRule = stringValue(layoutContract.backdrop_rule);
  const layoutVersion = firstScalar([layoutContract, ...marketSources], ["version", "layout_contract_version", "package_contract_version"]);
  const marketName = firstString(marketSources, ["market_pack_name", "resource_pack_name", "market", "brand"])
    || firstString([brief], ["market_pack_id", "resource_pack_id"]);
  const marketVersion = firstScalar(marketSources, ["market_pack_version", "resource_pack_version", "market_version"]);
  const marketResource = marketName && marketVersion ? `${marketName} · v${marketVersion}` : marketName;
  const explicitRule = firstString([brief, ...marketSources], ["market_rule", "market_rules", "compliance_rule", "compliance_rules"]);
  const layoutSummary = [
    layoutVersion ? labels.layoutContractVersion(layoutVersion) : hardRegions.length > 0 || backdropRule ? labels.layoutContract : "",
    hardRegions.length > 0 ? labels.protectedRegions(hardRegions.length) : "",
    backdropRule ? labels.backdropRule(backdropRule) : "",
  ].filter(Boolean).join(" · ");

  return {
    assetId: asset.id,
    variantLabel: variant.variant_key || variant.id.slice(0, 8),
    sizeLabel: labels.sizeLabel(asset.size_key),
    revision: asset.revision,
    direction: item.direction.trim() || firstString([brief, record(brief.copy_adaptation)], ["creative_direction", "visual_direction", "direction", "concept", "summary", "theme"]),
    copyLines: ([
      { field: "headline", label: labels.copyFields.headline, value: stringValue(snapshot.headline) },
      { field: "subheadline", label: labels.copyFields.subheadline, value: stringValue(snapshot.subheadline) },
      { field: "benefit", label: labels.copyFields.benefit, value: stringValue(snapshot.benefit) },
      { field: "supporting", label: labels.copyFields.supporting, value: stringValue(snapshot.supporting) },
      { field: "cta", label: labels.copyFields.cta, value: stringValue(snapshot.cta) },
      { field: "legal", label: labels.copyFields.legal, value: stringValue(snapshot.legal_text) },
    ] satisfies GenerationCopyLine[]).filter((entry) => Boolean(entry.value)),
    repaymentPlans: parseRepaymentPlanSelections(snapshot.repayment_plan_selections ?? record(snapshot.pre_adaptation).repayment_plan_selections),
    model: firstString(promptSources, ["model", "model_name", "generation_model"]),
    provider: firstString(promptSources, ["provider", "image_provider", "generation_provider"]),
    marketRule: explicitRule || layoutSummary || (marketName ? labels.marketBound : ""),
    marketResource,
    createdAt: asset.updated_at || asset.created_at || generated?.updated_at || generated?.created_at || "",
    prompt: firstString(promptSources, ["prompt", "model_prompt", "final_prompt", "provider_prompt"]),
    promptSha256: firstString(evidenceSources, ["prompt_sha256", "model_prompt_sha256", "final_prompt_sha256"]),
    requestId: firstString(evidenceSources, ["request_id", "model_request_id", "generation_request_id"]),
    attempts: firstScalar(evidenceSources, ["attempts", "attempt", "generation_attempts"]),
    lineage,
    primeConfigured,
    primeActual,
  };
}

export function creativeAssetLineage(variant: CreativeOrderVariant, selected: CreativeOrderAsset): CreativeOrderAsset[] {
  const eligible = variant.assets.filter((asset) =>
    asset.variant_id === selected.variant_id
    && asset.revision === selected.revision
    && asset.size_key === selected.size_key,
  );
  const byId = new Map(eligible.map((asset) => [asset.id, asset]));
  const seen = new Set<string>();
  const lineage: CreativeOrderAsset[] = [];
  let current: CreativeOrderAsset | undefined = byId.get(selected.id) ?? selected;

  while (current && !seen.has(current.id) && lineage.length < 6) {
    seen.add(current.id);
    lineage.push(current);
    const derived: CreativeOrderAsset | undefined = current.derived_from_asset_id ? byId.get(current.derived_from_asset_id) : undefined;
    if (derived) {
      current = derived;
      continue;
    }
    const previousStage: string = current.stage === "delivered" ? "primed" : current.stage === "primed" ? "generated" : "";
    current = previousStage ? latestAsset(eligible.filter((asset) => asset.stage === previousStage && !seen.has(asset.id))) : undefined;
  }

  return lineage.reverse();
}

export function CreativeGenerationInfoDialog({
  item,
  variant,
  asset,
  imageUrl,
  open,
  onOpenChange,
}: {
  item?: CreativeOrderItem;
  variant?: CreativeOrderVariant;
  asset?: CreativeOrderAsset;
  imageUrl: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useT("creative");
  const labels = generationInfoLabels(t);
  const info = item && variant && asset ? creativeGenerationInfo(item, variant, asset, labels) : undefined;
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="grid max-h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(94vw,1120px)]">
      <DialogHeader className="border-b px-5 py-4 pr-14">
        <div className="flex flex-wrap items-center gap-2">
          <DialogTitle className="text-base">{t(($) => $.generationInfo.title)}</DialogTitle>
          {info && <><Badge variant="outline">{info.variantLabel}</Badge><Badge variant="outline">{info.sizeLabel}</Badge><Badge variant="secondary">r{info.revision}</Badge></>}
        </div>
        <DialogDescription>{t(($) => $.generationInfo.description)}</DialogDescription>
      </DialogHeader>
      {info ? <div className="grid min-h-0 overflow-y-auto overscroll-contain lg:grid-cols-[minmax(260px,0.72fr)_minmax(0,1.28fr)]">
        <figure className="min-w-0 border-b bg-muted/20 p-4 lg:sticky lg:top-0 lg:h-full lg:border-b-0 lg:border-r">
          <div className="flex min-h-72 items-center justify-center overflow-hidden border bg-background">
            {imageUrl ? <img src={imageUrl} alt={t(($) => $.generationInfo.imageAlt, { variant: info.variantLabel, size: info.sizeLabel })} width={1200} height={1200} loading="lazy" className="max-h-[72vh] w-full object-contain" /> : <span className="flex min-h-72 flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon aria-hidden="true" className="h-5 w-5" />{t(($) => $.generationInfo.previewUnavailable)}</span>}
          </div>
          <figcaption className="mt-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>{info.variantLabel} · {info.sizeLabel}</span>
            <span translate="no">{compactId(info.assetId)}</span>
          </figcaption>
        </figure>
        <div className="min-w-0 divide-y">
          <InfoSection icon={<Sparkles aria-hidden="true" className="h-4 w-4" />} title={t(($) => $.generationInfo.creativeDirection)}>
            <p className="whitespace-pre-wrap break-words text-sm leading-6">{info.direction || t(($) => $.generationInfo.noDirection)}</p>
          </InfoSection>
          <InfoSection icon={<FileText aria-hidden="true" className="h-4 w-4" />} title={t(($) => $.generationInfo.creativeCopy)}>
            {info.copyLines.length > 0 ? <dl className="space-y-2">
              {info.copyLines.map((line) => <div key={line.label} className="grid gap-1 sm:grid-cols-[88px_minmax(0,1fr)]"><dt className="text-xs text-muted-foreground">{line.label}</dt><dd className="whitespace-pre-wrap break-words text-sm">{line.value}</dd></div>)}
            </dl> : <p className="text-sm text-muted-foreground">{t(($) => $.generationInfo.noCopy)}</p>}
            {info.repaymentPlans.length > 0 && <div className="mt-4 border-t pt-3"><p className="mb-2 text-xs font-medium text-muted-foreground">{t(($) => $.generationInfo.repaymentPlans)}</p><div className="flex flex-wrap gap-2">{info.repaymentPlans.map((plan, index) => <Badge key={`repayment-plan-${index}-${plan.label}-${plan.value}`} variant="outline" className="max-w-full whitespace-normal text-left"><span className="text-muted-foreground">{plan.label}</span><span className="mx-1">·</span><span className="break-words">{plan.value}</span></Badge>)}</div></div>}
          </InfoSection>
          <InfoSection icon={<Layers3 aria-hidden="true" className="h-4 w-4" />} title={t(($) => $.generationInfo.generationAndMarket)}>
            <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
              <InfoValue label={t(($) => $.generationInfo.generationModel)} value={[info.provider, info.model].filter(Boolean).join(" · ")} fallback={t(($) => $.generationInfo.notRecorded)} />
              <InfoValue label={t(($) => $.primeMode.orderMode)} value={creativePrimeModeLabel(t, info.primeConfigured.mode)} fallback={t(($) => $.primeMode.unknown)} />
              <InfoValue label={t(($) => $.primeMode.actualMode)} value={creativePrimeModeLabel(t, info.primeActual.mode)} fallback={t(($) => $.primeMode.unknown)} />
              <InfoValue label={t(($) => $.primeMode.actualTemplate)} value={[creativePrimeFamilyLabel(t, info.primeActual.templateFamilyId), info.primeActual.templateRole].filter(Boolean).join(" · ")} fallback={t(($) => $.primeMode.unknown)} />
              <InfoValue label={t(($) => $.generationInfo.marketResource)} value={info.marketResource} fallback={t(($) => $.generationInfo.notRecorded)} />
              <InfoValue label={t(($) => $.generationInfo.marketRule)} value={info.marketRule} fallback={t(($) => $.generationInfo.notRecorded)} wide />
            </dl>
            {info.primeActual.templateAttachmentId && <a className="mt-3 inline-block text-xs underline underline-offset-4" href={resolvePublicFileUrl(attachmentDownloadPath(info.primeActual.templateAttachmentId)) ?? undefined} target="_blank" rel="noreferrer">{t(($) => $.primeMode.viewTemplate)}</a>}
            {info.primeConfigured.mode !== "unknown" && info.primeActual.mode !== "unknown" && info.primeActual.mode !== info.primeConfigured.mode && <p role="status" className="mt-3 text-sm text-destructive">{t(($) => $.primeMode.mismatch)}</p>}
          </InfoSection>
          <InfoSection icon={<Clock3 aria-hidden="true" className="h-4 w-4" />} title={t(($) => $.generationInfo.versionAndTime)}>
            <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
              <InfoValue label={t(($) => $.generationInfo.assetLineage)} value={info.lineage.map((entry) => labels.stageLabel(entry.stage)).join(" → ")} fallback={t(($) => $.generationInfo.notRecorded)} />
              <InfoValue label={t(($) => $.generationInfo.lastUpdated)} value={info.createdAt ? `${formatCreativeDateTime(info.createdAt)} (${t(($) => $.generationInfo.beijingTime)})` : ""} fallback={t(($) => $.generationInfo.notRecorded)} />
            </dl>
          </InfoSection>
          <InfoSection icon={<FileText aria-hidden="true" className="h-4 w-4" />} title={t(($) => $.generationInfo.fullPrompt)}>
            <pre className="max-h-80 overflow-auto overscroll-contain whitespace-pre-wrap break-words border bg-muted/20 p-3 font-sans text-xs leading-5">{info.prompt || t(($) => $.generationInfo.noPrompt)}</pre>
          </InfoSection>
          <details className="group px-5 py-4">
            <summary className="cursor-pointer text-sm font-medium marker:text-muted-foreground">{t(($) => $.generationInfo.advanced)}</summary>
            <div className="mt-4 space-y-4">
              <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
                <InfoValue label={t(($) => $.generationInfo.modelRequest)} value={info.requestId ? compactId(info.requestId, 20) : ""} fallback={t(($) => $.generationInfo.notRecorded)} />
                <InfoValue label={t(($) => $.generationInfo.attempts)} value={info.attempts} fallback={t(($) => $.generationInfo.notRecorded)} />
                <InfoValue label={t(($) => $.generationInfo.promptHash)} value={info.promptSha256 ? compactId(info.promptSha256, 20) : ""} fallback={t(($) => $.generationInfo.notRecorded)} />
              </dl>
              <div><p className="text-xs font-medium text-muted-foreground">{t(($) => $.generationInfo.trace)}</p><ol className="mt-2 space-y-2">{info.lineage.map((entry) => <li key={entry.id} className="flex flex-wrap items-center gap-2 text-xs"><Badge variant="outline">{labels.stageLabel(entry.stage)}</Badge><span>{entry.size_key}</span><span className="text-muted-foreground" translate="no">{compactId(entry.id, 18)}</span></li>)}</ol></div>
            </div>
          </details>
        </div>
      </div> : <div className="flex min-h-72 items-center justify-center text-sm text-muted-foreground">{t(($) => $.generationInfo.noInfo)}</div>}
    </DialogContent>
  </Dialog>;
}

function InfoSection({ icon, title, children }: { icon: React.ReactNode; title: string; children: React.ReactNode }) {
  return <section className="px-5 py-4"><div className="mb-3 flex items-center gap-2"><span className="text-muted-foreground">{icon}</span><h3 className="text-sm font-semibold">{title}</h3></div>{children}</section>;
}

function InfoValue({ label, value, fallback, wide = false }: { label: string; value: string; fallback: string; wide?: boolean }) {
  return <div className={wide ? "sm:col-span-2" : undefined}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 whitespace-pre-wrap break-words text-sm">{value || fallback}</dd></div>;
}

function parseRepaymentPlanSelections(value: unknown): GenerationFact[] {
  return array(value).flatMap((entry) => {
    const selection = record(entry);
    const values = record(selection.values);
    const principal = stringValue(values.principal);
    const tenor = stringValue(values.tenor);
    const monthlyInstallment = stringValue(values.monthly_installment);
    return principal && tenor && monthlyInstallment
      ? [{ label: `${principal} / ${tenor}`, value: monthlyInstallment }]
      : [];
  });
}

function latestAsset(assets: CreativeOrderAsset[]): CreativeOrderAsset | undefined {
  return [...assets].sort((left, right) => timestamp(right.updated_at || right.created_at) - timestamp(left.updated_at || left.created_at) || right.id.localeCompare(left.id))[0];
}

function firstString(records: Record<string, unknown>[], keys: string[]): string {
  for (const current of records) {
    for (const key of keys) {
      const value = current[key];
      if (typeof value === "string" && value.trim()) return value.trim();
      if (Array.isArray(value)) {
        const joined = value.filter((entry): entry is string => typeof entry === "string" && Boolean(entry.trim())).map((entry) => entry.trim()).join("；");
        if (joined) return joined;
      }
    }
  }
  return "";
}

function firstScalar(records: Record<string, unknown>[], keys: string[]): string {
  for (const current of records) {
    for (const key of keys) {
      const value = current[key];
      if (typeof value === "string" && value.trim()) return value.trim().replace(/^v/i, "");
      if (typeof value === "number" && Number.isFinite(value)) return String(value);
    }
  }
  return "";
}

function record(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function array(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function timestamp(value: string | undefined): number {
  const parsed = Date.parse(value ?? "");
  return Number.isNaN(parsed) ? 0 : parsed;
}

function compactId(value: string, length = 12): string {
  if (!value) return "Not recorded";
  return value.length > length ? `${value.slice(0, length)}…` : value;
}

function creativeSizeLabel(size: string): string {
  if (size === "1080x1080") return "Square - 1080x1080";
  if (size === "1200x628") return "Landscape - 1200x628";
  if (size === "800x1000") return "Portrait - 800x1000";
  return size || "Unknown size";
}

function stageLabel(stage: string): string {
  if (stage === "generated") return "Model base image";
  if (stage === "primed") return "Prime creative";
  if (stage === "delivered") return "Delivered creative";
  return stage || "Unknown stage";
}

function generationInfoLabels(t: ReturnType<typeof useT<"creative">>["t"]): GenerationInfoLabels {
  return {
    layoutContract: t(($) => $.generationInfo.layoutContract),
    layoutContractVersion: (version) => t(($) => $.generationInfo.layoutContractVersion, { version }),
    protectedRegions: (count) => t(($) => $.generationInfo.protectedRegions, { count }),
    backdropRule: (rule) => t(($) => $.generationInfo.backdropRule, { rule }),
    marketBound: t(($) => $.generationInfo.marketBound),
    copyFields: {
      headline: t(($) => $.generationInfo.copyFields.headline), subheadline: t(($) => $.generationInfo.copyFields.subheadline), benefit: t(($) => $.generationInfo.copyFields.benefit), supporting: t(($) => $.generationInfo.copyFields.supporting), cta: t(($) => $.generationInfo.copyFields.cta), legal: t(($) => $.generationInfo.copyFields.legal),
    },
    sizeLabel: (size) => {
      if (size === "1080x1080") return t(($) => $.generationInfo.sizes.square);
      if (size === "1200x628") return t(($) => $.generationInfo.sizes.landscape);
      if (size === "800x1000") return t(($) => $.generationInfo.sizes.portrait);
      return size || t(($) => $.generationInfo.sizes.unknown);
    },
    stageLabel: (stage) => {
      if (stage === "generated") return t(($) => $.generationInfo.stages.generated);
      if (stage === "primed") return t(($) => $.generationInfo.stages.primed);
      if (stage === "delivered") return t(($) => $.generationInfo.stages.delivered);
      return stage || t(($) => $.generationInfo.stages.unknown);
    },
  };
}
