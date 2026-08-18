"use client";

import { Clock3, FileText, Image as ImageIcon, Layers3, Sparkles } from "lucide-react";
import type { CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { formatCreativeDateTime, creativeTimeZoneLabel } from "../lib/creative-time";

type GenerationFact = { label: string; value: string };
type GenerationCopyLine = { label: string; value: string };

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
  marketVersion: string;
  createdAt: string;
  prompt: string;
  promptSha256: string;
  requestId: string;
  attempts: string;
  lineage: CreativeOrderAsset[];
};

export function creativeGenerationInfo(
  item: CreativeOrderItem,
  variant: CreativeOrderVariant,
  asset: CreativeOrderAsset,
): CreativeGenerationInfo {
  const lineage = creativeAssetLineage(variant, asset);
  const generated = lineage.find((candidate) => candidate.stage === "generated");
  const primed = lineage.find((candidate) => candidate.stage === "primed");
  const snapshot = record(item.copy_snapshot);
  const brief = record(variant.brief);
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
  const marketVersion = firstScalar(marketSources, ["market_pack_version", "resource_pack_version", "published_version"]);
  const explicitRule = firstString([brief, ...marketSources], ["market_rule", "market_rules", "compliance_rule", "compliance_rules"]);
  const layoutSummary = [
    layoutVersion ? `Prime 布局合同 v${layoutVersion}` : hardRegions.length > 0 || backdropRule ? "Prime 布局合同" : "",
    hardRegions.length > 0 ? `${hardRegions.length} 个保护区` : "",
    backdropRule ? `背景规则 ${backdropRule}` : "",
  ].filter(Boolean).join(" · ");

  return {
    assetId: asset.id,
    variantLabel: variant.variant_key || variant.id.slice(0, 8),
    sizeLabel: creativeSizeLabel(asset.size_key),
    revision: asset.revision,
    direction: item.direction.trim() || firstString([brief, record(brief.copy_adaptation)], ["creative_direction", "visual_direction", "direction", "concept", "summary", "theme"]),
    copyLines: [
      { label: "主标题", value: stringValue(snapshot.headline) },
      { label: "副标题", value: stringValue(snapshot.subheadline) },
      { label: "利益点", value: stringValue(snapshot.benefit) },
      { label: "补充信息", value: stringValue(snapshot.supporting) },
      { label: "行动文案", value: stringValue(snapshot.cta) },
      { label: "合规文案", value: stringValue(snapshot.legal_text) },
    ].filter((entry) => Boolean(entry.value)),
    repaymentPlans: parseRepaymentPlanSelections(record(snapshot.pre_adaptation).repayment_plan_selections),
    model: firstString(promptSources, ["model", "model_name", "generation_model"]),
    provider: firstString(promptSources, ["provider", "image_provider", "generation_provider"]),
    marketRule: explicitRule || layoutSummary || (marketName ? "已绑定市场资源包" : ""),
    marketVersion: [marketName, marketVersion ? `v${marketVersion}` : ""].filter(Boolean).join(" · "),
    createdAt: asset.updated_at || asset.created_at || generated?.updated_at || generated?.created_at || "",
    prompt: firstString(promptSources, ["prompt", "model_prompt", "final_prompt", "provider_prompt"]),
    promptSha256: firstString(evidenceSources, ["prompt_sha256", "model_prompt_sha256", "final_prompt_sha256"]),
    requestId: firstString(evidenceSources, ["request_id", "model_request_id", "generation_request_id"]),
    attempts: firstScalar(evidenceSources, ["attempts", "attempt", "generation_attempts"]),
    lineage,
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
  const info = item && variant && asset ? creativeGenerationInfo(item, variant, asset) : undefined;
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="grid max-h-[94vh] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-[min(94vw,1120px)]">
      <DialogHeader className="border-b px-5 py-4 pr-14">
        <div className="flex flex-wrap items-center gap-2">
          <DialogTitle className="text-base">成图详情</DialogTitle>
          {info && <><Badge variant="outline">{info.variantLabel}</Badge><Badge variant="outline">{info.sizeLabel}</Badge><Badge variant="secondary">r{info.revision}</Badge></>}
        </div>
        <DialogDescription>查看这张成图采用的创意方向、冻结文案、市场规则与生成记录。</DialogDescription>
      </DialogHeader>
      {info ? <div className="grid min-h-0 overflow-y-auto overscroll-contain lg:grid-cols-[minmax(260px,0.72fr)_minmax(0,1.28fr)]">
        <figure className="min-w-0 border-b bg-muted/20 p-4 lg:sticky lg:top-0 lg:h-full lg:border-b-0 lg:border-r">
          <div className="flex min-h-72 items-center justify-center overflow-hidden border bg-background">
            {imageUrl ? <img src={imageUrl} alt={`${info.variantLabel} ${info.sizeLabel} 成图`} width={1200} height={1200} loading="lazy" className="max-h-[72vh] w-full object-contain" /> : <span className="flex min-h-72 flex-col items-center justify-center gap-2 text-xs text-muted-foreground"><ImageIcon aria-hidden="true" className="h-5 w-5" />成图预览不可用</span>}
          </div>
          <figcaption className="mt-3 flex items-center justify-between gap-3 text-xs text-muted-foreground">
            <span>{info.variantLabel} · {info.sizeLabel}</span>
            <span translate="no">{compactId(info.assetId)}</span>
          </figcaption>
        </figure>
        <div className="min-w-0 divide-y">
          <InfoSection icon={<Sparkles aria-hidden="true" className="h-4 w-4" />} title="创意方向">
            <p className="whitespace-pre-wrap break-words text-sm leading-6">{info.direction || "未记录创意方向"}</p>
          </InfoSection>
          <InfoSection icon={<FileText aria-hidden="true" className="h-4 w-4" />} title="成图文案">
            {info.copyLines.length > 0 ? <dl className="space-y-2">
              {info.copyLines.map((line) => <div key={line.label} className="grid gap-1 sm:grid-cols-[88px_minmax(0,1fr)]"><dt className="text-xs text-muted-foreground">{line.label}</dt><dd className="whitespace-pre-wrap break-words text-sm">{line.value}</dd></div>)}
            </dl> : <p className="text-sm text-muted-foreground">未记录冻结文案</p>}
            {info.repaymentPlans.length > 0 && <div className="mt-4 border-t pt-3"><p className="mb-2 text-xs font-medium text-muted-foreground">已选还款计划</p><div className="flex flex-wrap gap-2">{info.repaymentPlans.map((plan, index) => <Badge key={`repayment-plan-${index}-${plan.label}-${plan.value}`} variant="outline" className="max-w-full whitespace-normal text-left"><span className="text-muted-foreground">{plan.label}</span><span className="mx-1">·</span><span className="break-words">{plan.value}</span></Badge>)}</div></div>}
          </InfoSection>
          <InfoSection icon={<Layers3 aria-hidden="true" className="h-4 w-4" />} title="生成与市场规则">
            <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
              <InfoValue label="生成模型" value={[info.provider, info.model].filter(Boolean).join(" · ")} />
              <InfoValue label="市场资源版本" value={info.marketVersion} />
              <InfoValue label="市场规则" value={info.marketRule} wide />
            </dl>
          </InfoSection>
          <InfoSection icon={<Clock3 aria-hidden="true" className="h-4 w-4" />} title="版本与时间">
            <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
              <InfoValue label="资产链" value={info.lineage.map((entry) => stageLabel(entry.stage)).join(" → ")} />
              <InfoValue label="最后更新" value={info.createdAt ? `${formatCreativeDateTime(info.createdAt)}（${creativeTimeZoneLabel()}）` : ""} />
            </dl>
          </InfoSection>
          <InfoSection icon={<FileText aria-hidden="true" className="h-4 w-4" />} title="完整模型提示词">
            <pre className="max-h-80 overflow-auto overscroll-contain whitespace-pre-wrap break-words border bg-muted/20 p-3 font-sans text-xs leading-5">{info.prompt || "未记录完整模型提示词"}</pre>
          </InfoSection>
          <details className="group px-5 py-4">
            <summary className="cursor-pointer text-sm font-medium marker:text-muted-foreground">高级信息</summary>
            <div className="mt-4 space-y-4">
              <dl className="grid gap-x-5 gap-y-3 sm:grid-cols-2">
                <InfoValue label="模型请求" value={info.requestId ? compactId(info.requestId, 20) : ""} />
                <InfoValue label="调用次数" value={info.attempts} />
                <InfoValue label="提示词 SHA-256" value={info.promptSha256 ? compactId(info.promptSha256, 20) : ""} />
              </dl>
              <div><p className="text-xs font-medium text-muted-foreground">资产溯源</p><ol className="mt-2 space-y-2">{info.lineage.map((entry) => <li key={entry.id} className="flex flex-wrap items-center gap-2 text-xs"><Badge variant="outline">{stageLabel(entry.stage)}</Badge><span>{entry.size_key}</span><span className="text-muted-foreground" translate="no">{compactId(entry.id, 18)}</span></li>)}</ol></div>
            </div>
          </details>
        </div>
      </div> : <div className="flex min-h-72 items-center justify-center text-sm text-muted-foreground">没有可展示的生成信息</div>}
    </DialogContent>
  </Dialog>;
}

function InfoSection({ icon, title, children }: { icon: React.ReactNode; title: string; children: React.ReactNode }) {
  return <section className="px-5 py-4"><div className="mb-3 flex items-center gap-2"><span className="text-muted-foreground">{icon}</span><h3 className="text-sm font-semibold">{title}</h3></div>{children}</section>;
}

function InfoValue({ label, value, wide = false }: { label: string; value: string; wide?: boolean }) {
  return <div className={wide ? "sm:col-span-2" : undefined}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1 whitespace-pre-wrap break-words text-sm">{value || "未记录"}</dd></div>;
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
  if (!value) return "未记录";
  return value.length > length ? `${value.slice(0, length)}…` : value;
}

function creativeSizeLabel(size: string): string {
  if (size === "1080x1080") return "方形 · 1080x1080";
  if (size === "1200x628") return "横版 · 1200x628";
  if (size === "800x1000") return "竖版 · 800x1000";
  return size || "未知尺寸";
}

function stageLabel(stage: string): string {
  if (stage === "generated") return "模型底图";
  if (stage === "primed") return "Prime 成图";
  if (stage === "delivered") return "正式交付";
  return stage || "未知阶段";
}
