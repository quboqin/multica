"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FolderInput, FolderKanban, ListTodo } from "lucide-react";
import { issueDetailOptions } from "@multica/core/issues/queries";
import { projectDetailOptions } from "@multica/core/projects/queries";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { FavoriteItem } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";
import { FavoriteCategoryDialog } from "./favorite-category-dialog";
import { FavoriteItemAction } from "./favorite-item-action";

export function FavoriteItemRow({ favorite }: { favorite: FavoriteItem }) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const [categoryDialogOpen, setCategoryDialogOpen] = useState(false);
  const isIssue = favorite.itemType === "issue";
  const issueQuery = useQuery({
    ...issueDetailOptions(wsId, favorite.itemId),
    enabled: isIssue,
  });
  const projectQuery = useQuery({
    ...projectDetailOptions(wsId, favorite.itemId),
    enabled: !isIssue,
  });
  const loading = isIssue ? issueQuery.isLoading : projectQuery.isLoading;
  const issue = isIssue ? issueQuery.data : undefined;
  const project = !isIssue ? projectQuery.data : undefined;
  const label = issue
    ? `${issue.identifier} ${issue.title}`
    : project?.title ?? t(($) => $.favorites.item.unavailable);
  const href = isIssue
    ? paths.issueDetail(favorite.itemId)
    : paths.projectDetail(favorite.itemId);

  return (
    <div className="flex h-11 items-center gap-1 rounded-md border bg-background px-2">
      <AppLink
        href={href}
        className="flex min-w-0 flex-1 items-center gap-2 self-stretch px-1 text-sm"
      >
        {isIssue ? (
          <ListTodo className="size-4 shrink-0 text-muted-foreground" />
        ) : (
          <FolderKanban className="size-4 shrink-0 text-muted-foreground" />
        )}
        {loading ? (
          <Skeleton className="h-4 w-48" />
        ) : (
          <span className="truncate">{label}</span>
        )}
      </AppLink>
      <FavoriteItemAction
        itemType={favorite.itemType}
        itemId={favorite.itemId}
        itemLabel={label}
      />
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant="ghost"
              size="icon-sm"
              className="text-muted-foreground"
              onClick={() => setCategoryDialogOpen(true)}
            >
              <FolderInput />
            </Button>
          }
        />
        <TooltipContent side="bottom">
          {t(($) => $.favorites.item.change_category)}
        </TooltipContent>
      </Tooltip>
      <FavoriteCategoryDialog
        open={categoryDialogOpen}
        onOpenChange={setCategoryDialogOpen}
        itemType={favorite.itemType}
        itemId={favorite.itemId}
        itemLabel={label}
        currentCategoryId={favorite.category.id}
      />
    </div>
  );
}
