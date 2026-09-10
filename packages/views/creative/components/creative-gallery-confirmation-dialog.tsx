"use client";

import { useQuery } from "@tanstack/react-query";
import { Check, LoaderCircle } from "lucide-react";
import { creativeOrderOptions, useCreativeGalleryMutation } from "@multica/core/creative";
import { useWorkspaceId } from "@multica/core/hooks";
import { Button } from "@multica/ui/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@multica/ui/components/ui/dialog";
import { toast } from "sonner";
import { useT } from "../../i18n";

export function CreativeGalleryConfirmationDialog({ orderId, variantId, revision, onClose, onConfirmed }: {
  orderId: string; variantId: string; revision: number; onClose: () => void; onConfirmed: () => void;
}) {
  const { t } = useT("creative");
  const wsId = useWorkspaceId();
  const order = useQuery(creativeOrderOptions(wsId, orderId));
  const mutation = useCreativeGalleryMutation(wsId);
  const item = order.data?.items.find((entry) => entry.variants.some((variant) => variant.id === variantId));
  const variant = item?.variants.find((entry) => entry.id === variantId);
  const confirm = async () => {
    if (!item || !variant || !order.data) return;
    try {
      await mutation.mutateAsync({ orderId, itemId: item.id, issueId: order.data.issue_id, variantId, variantKey: variant.variant_key, revision, qcRiskAcknowledged: false, qcRiskReason: "" });
      toast.success(t(($) => $.gallery.added));
      onConfirmed();
    } catch { /* The mutation error remains visible in the dialog. */ }
  };
  return <Dialog open onOpenChange={(open) => { if (!open && !mutation.isPending) onClose(); }}>
    <DialogContent className="max-h-[90dvh] overflow-y-auto">
      <DialogHeader><DialogTitle>{t(($) => $.gallery.confirmDelivery)}</DialogTitle><DialogDescription>{variant?.variant_key || variantId.slice(0, 8)} · r{revision}</DialogDescription></DialogHeader>
      {order.isLoading ? <LoaderCircle className="h-5 w-5 animate-spin" /> : <p className="text-sm">{t(($) => $.gallery.confirmNotice)}</p>}
      {(order.error || mutation.error) && <p role="alert" className="break-words text-sm text-destructive">{(order.error || mutation.error)?.message}</p>}
      <DialogFooter><Button variant="outline" disabled={mutation.isPending} onClick={onClose}>{t(($) => $.copyOrder.cancel)}</Button><Button disabled={!variant || order.isFetching || mutation.isPending} onClick={() => void confirm()}>{mutation.isPending ? <LoaderCircle className="h-4 w-4 animate-spin" /> : <Check className="h-4 w-4" />}{t(($) => $.gallery.confirmDelivery)}</Button></DialogFooter>
    </DialogContent>
  </Dialog>;
}
