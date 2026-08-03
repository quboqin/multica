"use client";

import { useEffect, useState } from "react";
import { MoreHorizontal, Pencil, Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
  useDeleteFavoriteCategory,
  useUpdateFavoriteCategory,
} from "@multica/core/favorites";
import { useWorkspaceId } from "@multica/core/hooks";
import type { FavoriteCategory } from "@multica/core/types";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";
import { favoriteCategoryDisplayName } from "./favorite-category-name";

export function FavoriteCategoryActions({
  category,
  className,
  onDeleted,
}: {
  category: FavoriteCategory;
  className?: string;
  onDeleted?: (categoryId: string) => void;
}) {
  if (category.isDefault) return null;

  return (
    <MutableFavoriteCategoryActions
      category={category}
      className={className}
      onDeleted={onDeleted}
    />
  );
}

function MutableFavoriteCategoryActions({
  category,
  className,
  onDeleted,
}: {
  category: FavoriteCategory;
  className?: string;
  onDeleted?: (categoryId: string) => void;
}) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const updateCategory = useUpdateFavoriteCategory(wsId);
  const deleteCategory = useDeleteFavoriteCategory(wsId);
  const defaultName = t(($) => $.favorites.category.default_name);
  const displayName = favoriteCategoryDisplayName(category, defaultName);
  const [renameOpen, setRenameOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [name, setName] = useState(displayName);

  useEffect(() => {
    if (renameOpen) setName(displayName);
  }, [displayName, renameOpen]);

  const handleRename = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmedName = name.trim();
    if (!trimmedName || trimmedName === displayName || updateCategory.isPending) {
      return;
    }

    try {
      await updateCategory.mutateAsync({ category, name: trimmedName });
      toast.success(t(($) => $.favorites.rename_dialog.renamed));
      setRenameOpen(false);
    } catch {
      toast.error(t(($) => $.favorites.rename_dialog.failed));
    }
  };

  const handleDelete = async () => {
    if (deleteCategory.isPending) return;

    try {
      await deleteCategory.mutateAsync(category);
      toast.success(t(($) => $.favorites.delete_dialog.deleted));
      setDeleteOpen(false);
      onDeleted?.(category.id);
    } catch {
      toast.error(t(($) => $.favorites.delete_dialog.failed));
    }
  };

  return (
    <>
      <div className={cn("shrink-0", className)} onClick={(event) => event.stopPropagation()}>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <button
                type="button"
                aria-label={t(($) => $.favorites.category.actions_tooltip)}
                title={t(($) => $.favorites.category.actions_tooltip)}
                className="flex size-7 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-accent-foreground data-popup-open:bg-accent data-popup-open:text-accent-foreground"
              >
                <MoreHorizontal className="size-4" />
              </button>
            }
          />
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => setRenameOpen(true)}>
              <Pencil className="size-4" />
              {t(($) => $.favorites.rename_dialog.action)}
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onClick={() => setDeleteOpen(true)}>
              <Trash2 className="size-4" />
              {t(($) => $.favorites.delete_dialog.action)}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <Dialog open={renameOpen} onOpenChange={setRenameOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{t(($) => $.favorites.rename_dialog.title)}</DialogTitle>
            <DialogDescription>
              {t(($) => $.favorites.rename_dialog.description)}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleRename} className="space-y-4">
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">
                {t(($) => $.favorites.rename_dialog.name_label)}
              </span>
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                autoFocus
                maxLength={80}
              />
            </label>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={updateCategory.isPending}
                onClick={() => setRenameOpen(false)}
              >
                {t(($) => $.favorites.rename_dialog.cancel)}
              </Button>
              <Button
                type="submit"
                disabled={
                  !name.trim() ||
                  name.trim() === displayName ||
                  updateCategory.isPending
                }
              >
                {updateCategory.isPending
                  ? t(($) => $.favorites.rename_dialog.saving)
                  : t(($) => $.favorites.rename_dialog.save)}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.favorites.delete_dialog.title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.favorites.delete_dialog.description, {
                name: displayName,
                count: category.favoriteCount,
              })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteCategory.isPending}>
              {t(($) => $.favorites.delete_dialog.cancel)}
            </AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={deleteCategory.isPending}
              onClick={() => void handleDelete()}
            >
              {deleteCategory.isPending
                ? t(($) => $.favorites.delete_dialog.deleting)
                : t(($) => $.favorites.delete_dialog.delete)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
}
