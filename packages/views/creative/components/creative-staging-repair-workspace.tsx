"use client";

import { useEffect, useMemo, useState } from "react";
import { AlertTriangle, PencilRuler } from "lucide-react";
import type { CreativeDeliverySize, CreativeOrderAsset, CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { useT } from "../../i18n";
import { creativeAttachmentBrowserURL } from "../lib/creative-attachment-url";
import { CreativeComparisonWorkspace, type CreativeAnnotationDraft } from "./creative-comparison-workspace";
import type { DeliveryAttachment } from "./creative-order-delivery";

const DELIVERY_SIZES: CreativeDeliverySize[] = ["1080x1080", "1200x628", "800x1000"];
const DELIVERY_SIZE_SET = new Set<string>(DELIVERY_SIZES);

export type CreativeStagingRepairEntry = {
  item: CreativeOrderItem;
  variant: CreativeOrderVariant;
  asset: CreativeOrderAsset;
  baseAsset?: CreativeOrderAsset;
  activeAsset?: CreativeOrderAsset;
  expectedSizes: CreativeDeliverySize[];
};

export function creativeVariantRevisionExpectedSizes(
  variant: Pick<CreativeOrderVariant, "revisions">,
  revision: number,
): CreativeDeliverySize[] {
  const contract = (variant.revisions ?? []).find((candidate) => candidate.revision === revision);
  if (!contract) return [];
  return [...new Set(contract.expected_sizes.filter((size): size is CreativeDeliverySize => DELIVERY_SIZE_SET.has(size)))];
}

export function creativeStagingRepairEntries(items: CreativeOrderItem[]): CreativeStagingRepairEntry[] {
  return items.flatMap((item) => item.variants.flatMap((variant) => {
    const activeRevision = variant.active_revision ?? 0;
    const stagingRevision = variant.staging_revision ?? 0;
    if (activeRevision < 1 || stagingRevision < 1 || activeRevision === stagingRevision) return [];
    const revision = (variant.revisions ?? []).find((candidate) => candidate.revision === stagingRevision);
    const needsRepair = ["action_required", "failed"].includes(revision?.status ?? "")
      || ["action_required", "failed"].includes(variant.status);
    if (!needsRepair) return [];
    const expectedSizes = creativeVariantRevisionExpectedSizes(variant, stagingRevision);
    return expectedSizes.flatMap((size) => {
      const asset = bestRevisionAsset(variant.assets, stagingRevision, size, new Set(["delivered", "primed"]));
      if (!asset) return [];
      return [{
        item,
        variant,
        asset,
        baseAsset: bestRevisionAsset(variant.assets, stagingRevision, size, new Set(["generated"])),
        activeAsset: bestRevisionAsset(variant.assets, activeRevision, size, new Set(["delivered", "primed", "generated"])),
        expectedSizes,
      }];
    });
  }));
}

export function CreativeStagingRepairWorkspace({
  items,
  attachments,
  disabled = false,
  onAnnotations,
  onViewInfo,
  onDiscard,
}: {
  items: CreativeOrderItem[];
  attachments: Map<string, DeliveryAttachment>;
  disabled?: boolean;
  onAnnotations: (asset: CreativeOrderAsset, annotations: CreativeAnnotationDraft[]) => Promise<boolean>;
  onViewInfo?: (asset: CreativeOrderAsset) => void;
  onDiscard?: (variant: CreativeOrderVariant) => Promise<void>;
}) {
  const { t } = useT("creative");
  const entries = useMemo(() => creativeStagingRepairEntries(items), [items]);
  const [selectedAssetId, setSelectedAssetId] = useState("");
  const [discardingVariantId, setDiscardingVariantId] = useState("");
  const selected = entries.find((entry) => entry.asset.id === selectedAssetId);
  const selectedVariantEntries = selected
    ? entries.filter((entry) => entry.variant.id === selected.variant.id)
    : [];
  useEffect(() => {
    if (selectedAssetId && !entries.some((entry) => entry.asset.id === selectedAssetId)) setSelectedAssetId("");
  }, [entries, selectedAssetId]);
  if (entries.length === 0) return null;

  const attachmentURL = (asset?: CreativeOrderAsset) => creativeAttachmentBrowserURL(asset ? attachments.get(asset.attachment_id) : undefined);
  return <section className="border-y bg-amber-50/50 px-4 py-3 dark:bg-amber-950/20" data-testid="creative-staging-repair-workspace">
    <div className="flex flex-wrap items-center gap-2">
      <AlertTriangle className="h-4 w-4 text-amber-700 dark:text-amber-300" />
      <h3 className="text-sm font-semibold">{t(($) => $.stagingRepair.title)}</h3>
      <Badge variant="outline">{t(($) => $.stagingRepair.sizes, { count: entries.length })}</Badge>
      <p className="text-xs text-muted-foreground">{t(($) => $.stagingRepair.activeRevision)}</p>
    </div>
    <div className="mt-3 flex flex-wrap gap-2">
      {entries.map((entry, index) => <div key={entry.asset.id} className="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={!attachmentURL(entry.asset)}
          onClick={() => setSelectedAssetId(entry.asset.id)}
        >
          <PencilRuler className="h-4 w-4" />
          {t(($) => $.stagingRepair.viewAndAnnotate, { variant: entry.variant.variant_key || entry.variant.id.slice(0, 8), revision: entry.asset.revision, size: deliverySizeLabel(t, entry.asset.size_key) })}
        </Button>
        {onDiscard && !disabled && entries.findIndex((candidate) => candidate.variant.id === entry.variant.id) === index && <Button
          size="sm"
          variant="ghost"
          disabled={discardingVariantId === entry.variant.id}
          onClick={() => {
            setDiscardingVariantId(entry.variant.id);
            void onDiscard(entry.variant).finally(() => setDiscardingVariantId(""));
          }}
        >
          {discardingVariantId === entry.variant.id ? t(($) => $.stagingRepair.discarding) : t(($) => $.stagingRepair.discardDraft, { revision: entry.variant.staging_revision })}
        </Button>}
      </div>)}
    </div>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => { if (!open) setSelectedAssetId(""); }}>
      <DialogContent className="flex h-[min(92vh,920px)] max-w-6xl flex-col overflow-hidden p-0">
        <DialogHeader className="shrink-0 border-b px-5 py-4 pr-14">
          <DialogTitle>{t(($) => $.stagingRepair.dialogTitle)}</DialogTitle>
          <DialogDescription>
            {selected ? t(($) => $.stagingRepair.dialogDescription, { variant: selected.variant.variant_key || t(($) => $.stagingRepair.currentVariant), stagingRevision: selected.asset.revision, size: deliverySizeLabel(t, selected.asset.size_key), activeRevision: selected.variant.active_revision }) : ""}
          </DialogDescription>
        </DialogHeader>
        {selected && <div className="min-h-0 flex-1">
          <CreativeComparisonWorkspace
            source={{ label: t(($) => $.stagingRepair.liveRevision, { revision: selected.variant.active_revision }), url: attachmentURL(selected.activeAsset) }}
            result={{
              id: selected.asset.id,
              label: t(($) => $.stagingRepair.productionRevision, { revision: selected.asset.revision, size: deliverySizeLabel(t, selected.asset.size_key) }),
              finalUrl: attachmentURL(selected.asset),
              baseUrl: attachmentURL(selected.baseAsset),
              size: selected.asset.size_key,
              variant: selected.variant.variant_key,
            }}
            assets={selectedVariantEntries.map((entry) => ({
              id: entry.asset.id,
              label: t(($) => $.stagingRepair.productionRevision, { revision: entry.asset.revision, size: deliverySizeLabel(t, entry.asset.size_key) }),
              finalUrl: attachmentURL(entry.asset),
              baseUrl: attachmentURL(entry.baseAsset),
              thumbnailUrl: attachmentURL(entry.asset),
              size: entry.asset.size_key,
              variant: entry.variant.variant_key,
            }))}
            onAssetChange={setSelectedAssetId}
            onViewInfo={onViewInfo ? () => onViewInfo(selected.asset) : undefined}
            onAnnotations={disabled ? undefined : (annotations) => onAnnotations(selected.activeAsset ?? selected.asset, annotations)}
            annotationScopes={selected.expectedSizes.length > 1 ? ["size", "variant"] : ["size"]}
            annotationScopeLabels={{ variant: t(($) => $.stagingRepair.allProductionSizes, { count: selected.expectedSizes.length }) }}
            showDecisionActions={false}
            allowDownload={false}
            comparisonMode="adjustment"
          />
        </div>}
      </DialogContent>
    </Dialog>
  </section>;
}

function bestRevisionAsset(
  assets: CreativeOrderAsset[],
  revision: number,
  size: CreativeDeliverySize,
  stages: Set<string>,
): CreativeOrderAsset | undefined {
  return assets
    .filter((asset) => asset.revision === revision && asset.size_key === size && stages.has(asset.stage) && asset.status === "completed" && Boolean(asset.attachment_id))
    .sort(compareAssets)[0];
}

function compareAssets(left: CreativeOrderAsset, right: CreativeOrderAsset): number {
  const rank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 3 : asset.stage === "primed" ? 2 : asset.stage === "generated" ? 1 : 0;
  return rank(right) - rank(left) || right.updated_at.localeCompare(left.updated_at) || right.id.localeCompare(left.id);
}

function deliverySizeLabel(t: ReturnType<typeof useT>["t"], size: string): string {
  if (size === "1080x1080") return t(($) => $.stagingRepair.square);
  if (size === "1200x628") return t(($) => $.stagingRepair.landscape);
  if (size === "800x1000") return t(($) => $.stagingRepair.portrait);
  return size;
}
