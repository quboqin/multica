"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowUpRight, Layers3 } from "lucide-react";
import { api } from "@multica/core/api";
import { creativeMaterialLibraryOptions, creativeOrderOptions } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { CreativeOrderAsset } from "@multica/core/types";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { Badge } from "@multica/ui/components/ui/badge";
import { creativeAttachmentBrowserURL } from "../../creative/lib/creative-attachment-url";
import { AppLink } from "../../navigation";

export function CreativeOrderSummary({ orderId }: { orderId: string }) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const order = useQuery(creativeOrderOptions(wsId, orderId));
  const library = useQuery(creativeMaterialLibraryOptions(wsId));
  const data = order.data;
  const variants = data?.items.flatMap((item) => item.variants) ?? [];
  const assets = variants.flatMap((variant) => variant.assets);
  const deliveryAssets = useMemo(() => data?.items.map((item) => ({
    candidate: library.data?.candidates.find((candidate) => candidate.id === item.candidate_id),
    asset: currentDeliveryAsset(item.variants.flatMap((variant) => variant.assets)),
  })).filter((entry): entry is { candidate: NonNullable<typeof entry.candidate>; asset: CreativeOrderAsset } => Boolean(entry.candidate && entry.asset)) ?? [], [data?.items, library.data?.candidates]);
  const attachmentIds = useMemo(() => [...new Set(deliveryAssets.map((entry) => entry.asset.attachment_id).filter(Boolean))], [deliveryAssets]);
  const attachments = useQuery({
    queryKey: ["creative", wsId, "order-summary-attachments", orderId, attachmentIds],
    queryFn: () => Promise.all(attachmentIds.map((id) => api.getAttachment(id))),
    enabled: attachmentIds.length > 0,
  });
  const attachmentById = new Map((attachments.data ?? []).map((attachment) => [attachment.id, attachment]));
  return <section className="my-5 border bg-background" data-testid="creative-order-summary"><div className="flex flex-wrap items-center justify-between gap-3 border-b px-4 py-3"><div className="flex items-center gap-2"><Layers3 className="h-4 w-4 text-emerald-700" /><span className="text-sm font-semibold">创意订单</span><Badge variant="outline">{data?.derived_status || data?.status || "加载中"}</Badge></div><AppLink href={paths.creativeOrder(orderId)} className="inline-flex items-center gap-1 text-xs font-medium underline-offset-4 hover:underline">查看订单交付<ArrowUpRight className="h-3.5 w-3.5" /></AppLink></div><div className="grid gap-3 px-4 py-3 text-xs sm:grid-cols-3"><div><p className="text-muted-foreground">素材条目</p><p className="mt-1 font-semibold">{data?.items.length ?? 0}</p></div><div><p className="text-muted-foreground">创意变体</p><p className="mt-1 font-semibold">{variants.length}</p></div><div><p className="text-muted-foreground">已登记资产</p><p className="mt-1 font-semibold">{assets.length}</p></div></div>{deliveryAssets.length > 0 && <div className="border-t px-4 py-4"><div className="mb-3 flex items-center justify-between"><p className="text-sm font-medium">原图与最新成图</p><span className="text-xs text-muted-foreground">点击订单查看高清对比、标注和验收</span></div><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{deliveryAssets.map(({ candidate, asset }) => { const final = attachmentById.get(asset.attachment_id); const sourceUrl = resolvePublicFileUrl(candidate.archived_url || candidate.preview_url); const finalUrl = creativeAttachmentBrowserURL(final); return <div key={asset.id} className="grid overflow-hidden border bg-muted/10 grid-cols-2"><div className="min-w-0 border-r bg-muted/30"><p className="px-2 py-1 text-[11px] text-muted-foreground">原图 · {candidate.id.slice(0, 8)}</p>{sourceUrl ? <img src={sourceUrl} alt={`原图 ${candidate.title || candidate.id}`} width={720} height={540} loading="lazy" className="aspect-[4/3] h-auto w-full object-contain" /> : <div className="aspect-[4/3]" />}</div><div className="min-w-0 bg-muted/30"><p className="px-2 py-1 text-[11px] text-muted-foreground">成图 · {asset.size_key}</p>{finalUrl ? <img src={finalUrl} alt={`成图 ${asset.size_key}`} width={720} height={540} loading="lazy" className="aspect-[4/3] h-auto w-full object-contain" /> : <div className="aspect-[4/3]" />}</div></div>; })}</div></div>}{variants.some((variant) => variant.qc_status === "warning" || variant.qc_status === "failed") && <p className="border-t px-4 py-2 text-xs text-amber-700">部分 QC 需要处理，请在创意工厂查看对应变体和标注。</p>}</section>;
}

function currentDeliveryAsset(assets: CreativeOrderAsset[]): CreativeOrderAsset | undefined {
  const rank = (asset: CreativeOrderAsset) => asset.stage === "delivered" ? 3 : asset.stage === "primed" ? 2 : asset.stage === "generated" ? 1 : 0;
  return [...assets].filter((asset) => asset.status === "completed" && asset.attachment_id).sort((left, right) => right.revision - left.revision || rank(right) - rank(left) || right.updated_at.localeCompare(left.updated_at))[0];
}
