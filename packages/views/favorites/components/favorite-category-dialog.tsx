"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Folder, Plus } from "lucide-react";
import { toast } from "sonner";
import {
  favoriteCategoryListOptions,
  useCreateFavoriteCategory,
  useMoveFavorite,
} from "@multica/core/favorites";
import { useWorkspaceId } from "@multica/core/hooks";
import type {
  FavoriteCategory,
  FavoriteType,
} from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { favoriteCategoryDisplayName } from "./favorite-category-name";

interface NewCategoryFormProps {
  onCreated?: (category: FavoriteCategory) => Promise<void> | void;
  onCancel?: () => void;
  autofocus?: boolean;
}

function NewCategoryForm({ onCreated, onCancel, autofocus }: NewCategoryFormProps) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const createCategory = useCreateFavoriteCategory(wsId);
  const [name, setName] = useState("");

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || createCategory.isPending) return;

    try {
      const created = await createCategory.mutateAsync(trimmedName);
      setName("");
      await onCreated?.(created);
    } catch {
      toast.error(t(($) => $.favorites.create_dialog.failed));
    }
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3">
      <label className="block space-y-1.5">
        <span className="text-xs font-medium text-muted-foreground">
          {t(($) => $.favorites.create_dialog.name_label)}
        </span>
        <Input
          value={name}
          onChange={(event) => setName(event.target.value)}
          placeholder={t(($) => $.favorites.create_dialog.name_placeholder)}
          autoFocus={autofocus}
          maxLength={80}
        />
      </label>
      <div className="flex justify-end gap-2">
        {onCancel && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={createCategory.isPending}
            onClick={onCancel}
          >
            {t(($) => $.favorites.create_dialog.cancel)}
          </Button>
        )}
        <Button
          type="submit"
          size="sm"
          disabled={!name.trim() || createCategory.isPending}
        >
          {createCategory.isPending
            ? t(($) => $.favorites.create_dialog.creating)
            : t(($) => $.favorites.create_dialog.create)}
        </Button>
      </div>
    </form>
  );
}

export function CreateFavoriteCategoryDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useT("layout");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(($) => $.favorites.create_dialog.title)}</DialogTitle>
          <DialogDescription>
            {t(($) => $.favorites.create_dialog.description)}
          </DialogDescription>
        </DialogHeader>
        <NewCategoryForm
          autofocus
          onCancel={() => onOpenChange(false)}
          onCreated={() => {
            toast.success(t(($) => $.favorites.create_dialog.created));
            onOpenChange(false);
          }}
        />
      </DialogContent>
    </Dialog>
  );
}

export function FavoriteCategoryDialog({
  open,
  onOpenChange,
  itemType,
  itemId,
  itemLabel,
  currentCategoryId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  itemType: FavoriteType;
  itemId: string;
  itemLabel: string;
  currentCategoryId: string;
}) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const { data: categories = [] } = useQuery(
    favoriteCategoryListOptions(wsId),
  );
  const moveFavorite = useMoveFavorite(wsId);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    if (!open) setCreating(false);
  }, [open]);

  const categoryName = (category: FavoriteCategory) =>
    favoriteCategoryDisplayName(
      category,
      t(($) => $.favorites.category.default_name),
    );

  const moveTo = async (category: FavoriteCategory) => {
    if (category.id === currentCategoryId) {
      onOpenChange(false);
      return;
    }

    try {
      await moveFavorite.mutateAsync({ itemType, itemId, category });
      toast.success(
        t(($) => $.favorites.move_dialog.moved, {
          category: categoryName(category),
        }),
      );
      onOpenChange(false);
    } catch {
      toast.error(t(($) => $.favorites.move_dialog.failed));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(($) => $.favorites.move_dialog.title)}</DialogTitle>
          <DialogDescription>
            {t(($) => $.favorites.move_dialog.description, {
              filename: itemLabel,
            })}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-64 space-y-1 overflow-y-auto">
          {categories.map((category) => (
            <button
              key={category.id}
              type="button"
              className={cn(
                "flex h-9 w-full items-center gap-2 rounded-md px-2 text-sm hover:bg-accent",
                category.id === currentCategoryId && "bg-accent",
              )}
              disabled={moveFavorite.isPending}
              onClick={() => void moveTo(category)}
            >
              <Folder className="size-4 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate text-left">
                {categoryName(category)}
              </span>
              <span className="text-xs tabular-nums text-muted-foreground">
                {category.favoriteCount}
              </span>
              {category.id === currentCategoryId && <Check className="size-4" />}
            </button>
          ))}
        </div>

        {creating ? (
          <NewCategoryForm
            autofocus
            onCancel={() => setCreating(false)}
            onCreated={moveTo}
          />
        ) : (
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              className="mr-auto"
              onClick={() => setCreating(true)}
            >
              <Plus className="size-4" />
              {t(($) => $.favorites.move_dialog.new_category)}
            </Button>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              {t(($) => $.favorites.create_dialog.cancel)}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
