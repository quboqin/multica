"use client";

import { useState } from "react";
import { Star } from "lucide-react";
import { toast } from "sonner";
import { useFavoriteItem } from "@multica/core/favorites";
import { useWorkspaceId } from "@multica/core/hooks";
import type { FavoriteItem, FavoriteItemType } from "@multica/core/types";
import { CENTERED_TOASTER_ID } from "@multica/ui/components/ui/sonner";
import { Button } from "@multica/ui/components/ui/button";
import { DropdownMenuItem } from "@multica/ui/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { FavoriteCategoryDialog } from "./favorite-category-dialog";
import { favoriteCategoryDisplayName } from "./favorite-category-name";

interface FavoriteItemActionProps {
  itemType: FavoriteItemType;
  itemId: string;
  itemLabel: string;
  presentation?: "icon" | "menu";
  onRequestCategoryChange?: (favorite: FavoriteItem) => void;
}

export function FavoriteItemAction({
  itemType,
  itemId,
  itemLabel,
  presentation = "icon",
  onRequestCategoryChange,
}: FavoriteItemActionProps) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const favorite = useFavoriteItem(wsId, itemType, itemId);
  const [categoryDialogOpen, setCategoryDialogOpen] = useState(false);

  const toggle = () => {
    const removing = favorite.isFavorite;
    void favorite
      .toggle()
      .then((created) => {
        if (removing) {
          toast.success(t(($) => $.favorites.item.removed));
          return;
        }
        if (!created) return;
        const category = favoriteCategoryDisplayName(
          created.category,
          t(($) => $.favorites.category.default_name),
        );
        toast.success(
          t(($) => $.favorites.item.saved_to, { category }),
          {
            toasterId: CENTERED_TOASTER_ID,
            action: {
              label: t(($) => $.favorites.item.change_category),
              onClick: () => {
                if (onRequestCategoryChange) {
                  onRequestCategoryChange(created);
                } else {
                  setCategoryDialogOpen(true);
                }
              },
            },
          },
        );
      })
      .catch(() => {
        toast.error(
          t(($) =>
            removing
              ? $.favorites.item.remove_failed
              : $.favorites.item.save_failed,
          ),
        );
      });
  };

  const label = favorite.isFavorite
    ? t(($) => $.favorites.item.remove)
    : t(($) => $.favorites.item.add);

  return (
    <>
      {presentation === "menu" ? (
        <DropdownMenuItem
          className="cursor-pointer"
          disabled={favorite.isPending}
          onClick={toggle}
        >
          <Star
            className="size-3.5"
            fill={favorite.isFavorite ? "currentColor" : "none"}
          />
          {label}
        </DropdownMenuItem>
      ) : (
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                className={cn(
                  "cursor-pointer text-muted-foreground",
                  favorite.isFavorite && "text-foreground",
                )}
                aria-pressed={favorite.isFavorite}
                disabled={favorite.isPending}
                onClick={toggle}
              >
                <Star fill={favorite.isFavorite ? "currentColor" : "none"} />
              </Button>
            }
          />
          <TooltipContent side="bottom">{label}</TooltipContent>
        </Tooltip>
      )}

      {!onRequestCategoryChange && favorite.favoriteEntry && (
        <FavoriteCategoryDialog
          open={categoryDialogOpen}
          onOpenChange={setCategoryDialogOpen}
          itemType={itemType}
          itemId={itemId}
          itemLabel={itemLabel}
          currentCategoryId={favorite.favoriteEntry.category.id}
        />
      )}
    </>
  );
}
