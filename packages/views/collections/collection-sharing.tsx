"use client";

import { useQuery } from "@tanstack/react-query";
import { Pin, PinOff } from "lucide-react";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import type { CollectionAccess } from "@multica/core/collections";
import { pinListOptions, useCreatePin, useDeletePin } from "@multica/core/pins";
import { Button } from "@multica/ui/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { ResourceSharing } from "../documents/document-sharing";
import { useT } from "../i18n";

export function CollectionSharing({
  id,
  wsId,
  access,
}: {
  id: string;
  wsId: string;
  access: CollectionAccess | null;
}) {
  const { t } = useT("issues");
  const userId = useAuthStore((s) => s.user?.id ?? "");
  const { data: pins = [] } = useQuery({
    ...pinListOptions(wsId, userId),
    enabled: !!userId,
  });
  const create = useCreatePin();
  const remove = useDeletePin();
  const pinned = pins.some(
    (p) => p.item_type === "collection" && p.item_id === id,
  );
  const pending = create.isPending || remove.isPending;
  const label = t(($) =>
    pinned ? $.detail.unpin_tooltip : $.detail.pin_tooltip,
  );
  const onError = (error: Error) => toast.error(error.message);
  return (
    <>
      {access?.can_manage && (
        <ResourceSharing
          kind="collection"
          id={id}
          wsId={wsId}
          access={access}
        />
      )}
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant={pinned ? "secondary" : "ghost"}
              size="icon-sm"
              aria-label={label}
              aria-pressed={pinned}
              aria-busy={pending}
              disabled={pending}
              onClick={() =>
                pinned
                  ? remove.mutate(
                      { itemType: "collection", itemId: id },
                      { onError },
                    )
                  : create.mutate(
                      { item_type: "collection", item_id: id },
                      { onError },
                    )
              }
            >
              {pinned ? <PinOff /> : <Pin />}
            </Button>
          }
        />
        <TooltipContent>{label}</TooltipContent>
      </Tooltip>
    </>
  );
}
