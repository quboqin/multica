"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, LoaderCircle } from "lucide-react";
import { creativeOrderOptions, useCreativeGalleryMutation } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useT } from "../../i18n";
import { creativeVariantQCDetailsForRevision } from "./creative-order-delivery";

export function CreativeGalleryConfirmationDialog({ orderId, variantId, revision, onClose, onConfirmed }: {
  orderId: string; variantId: string; revision: number; onClose: () => void; onConfirmed: () => void;
}) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const order = useQuery(creativeOrderOptions(wsId, orderId));
  const mutation = useCreativeGalleryMutation(wsId);
  const item = order.data?.items.find((entry) => entry.variants.some((variant) => variant.id === variantId));
  const variant = item?.variants.find((entry) => entry.id === variantId);
  const details = variant ? creativeVariantQCDetailsForRevision(variant, revision) : [];
  const risk = details.some((entry) => entry.status === "failed");
  const [acknowledged, setAcknowledged] = useState(false);
  const [reason, setReason] = useState("");
  const confirm = async () => {
    if (!item || !variant || !order.data) return;
    try {
      await mutation.mutateAsync({ orderId, itemId: item.id, issueId: order.data.issue_id, variantId, variantKey: variant.variant_key, revision, qcRiskAcknowledged: risk && acknowledged, qcRiskReason: risk ? reason.trim() : "" });
      toast.success(t(($) => $.gallery.added));
      onConfirmed();
    } catch { /* The mutation error remains visible with the user's input. */ }
  };
  return <Dialog open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose(); }}>
    <DialogContent className="max-h-[90dvh] overflow-y-auto">
      <DialogHeader><DialogTitle>{t(($) => $.gallery.confirmDelivery)}</DialogTitle><DialogDescription>{variant?.variant_key || variantId.slice(0, 8)} · r{revision}</DialogDescription></DialogHeader>
      {order.isLoading ? <LoaderCircle className="h-5 w-5 animate-spin" /> : risk ? <div className="space-y-3">
        <p className="text-sm text-amber-700 dark:text-amber-300">{t(($) => $.gallery.riskNotice)}</p>
        {details.flatMap((entry) => [...entry.blockingFailures, ...entry.qualityWarnings]).map((message, index) => <p key={`${index}:${message}`} className="text-sm">{message}</p>)}
        <label className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1 h-4 w-4 shrink-0" checked={acknowledged} onChange={(event) => setAcknowledged(event.target.checked)} disabled={mutation.isPending} /><span>{t(($) => $.gallery.riskAcknowledge)}</span></label>
        <label className="grid gap-2 text-sm">{t(($) => $.gallery.riskReason)}<Textarea value={reason} maxLength={1000} onChange={(event) => setReason(event.target.value)} disabled={mutation.isPending} /></label>
      </div> : <p className="text-sm">{t(($) => $.gallery.confirmNotice)}</p>}
      {(order.error || mutation.error) && <p role="alert" className="break-words text-sm text-destructive">{(order.error || mutation.error)?.message}</p>}
      <DialogFooter><Button variant="outline" disabled={mutation.isPending} onClick={onClose}>{t(($) => $.copyOrder.cancel)}</Button><Button disabled={!variant || order.isFetching || mutation.isPending || (risk && (!acknowledged || !reason.trim()))} onClick={() => void confirm()}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}{t(($) => $.gallery.confirmDelivery)}</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}
