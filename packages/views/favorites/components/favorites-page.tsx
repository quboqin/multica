"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, Folder, FolderPlus, Star } from "lucide-react";
import {
  favoriteCategoryListOptions,
  favoriteListOptions,
} from "@multica/core/favorites";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import type { FavoriteCategory } from "@multica/core/types";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@multica/ui/components/ui/tabs";
import { useT } from "../../i18n";
import { getPreviewKind } from "../../editor/utils/preview";
import { PageHeader } from "../../layout/page-header";
import { AppLink, useNavigation } from "../../navigation";
import { FavoriteCategoryActions } from "./favorite-category-actions";
import { CreateFavoriteCategoryDialog } from "./favorite-category-dialog";
import { favoriteCategoryDisplayName } from "./favorite-category-name";
import { FavoriteDocumentRow } from "./favorite-document-row";
import { FavoriteItemRow } from "./favorite-item-row";

const EMPTY_CATEGORIES: FavoriteCategory[] = [];
type FavoriteTypeFilter = "all" | "issue" | "project" | "markdown" | "html";

export function FavoritesPage({ categoryId }: { categoryId?: string }) {
  const { t } = useT("layout");
  const wsId = useWorkspaceId();
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const p = useWorkspacePaths();
  const navigation = useNavigation();
  const [createOpen, setCreateOpen] = useState(false);
  const [typeFilter, setTypeFilter] = useState<FavoriteTypeFilter>("all");
  const categoriesQuery = useQuery(favoriteCategoryListOptions(wsId));
  const favoritesQuery = useQuery(favoriteListOptions(wsId, userId));
  const categories = categoriesQuery.data ?? EMPTY_CATEGORIES;
  const category = categoryId
    ? categories.find((item) => item.id === categoryId)
    : undefined;
  const favorites = categoryId
    ? (favoritesQuery.data ?? []).filter(
        (favorite) => favorite.category.id === categoryId,
      )
    : [];
  const filteredEntries = favorites.filter((favorite) => {
      if (typeFilter === "all") return true;
      if (favorite.itemType !== "attachment") {
        return favorite.itemType === typeFilter;
      }
      return (
        getPreviewKind(
          favorite.attachment.content_type,
          favorite.attachment.filename,
        ) === typeFilter
      );
    });
  const favoriteCount = favorites.length;
  const loading =
    categoriesQuery.isLoading ||
    (categoryId && favoritesQuery.isLoading);
  const failed =
    categoriesQuery.isError ||
    (categoryId && favoritesQuery.isError);

  const categoryName = (item: FavoriteCategory) =>
    favoriteCategoryDisplayName(
      item,
      t(($) => $.favorites.category.default_name),
    );

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader className="justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          {categoryId ? (
            <Button
              variant="ghost"
              size="icon-sm"
              title={t(($) => $.favorites.page.back_to_categories)}
              render={<AppLink href={p.favorites()} />}
            >
              <ArrowLeft className="size-4" />
            </Button>
          ) : (
            <Star className="size-4 text-muted-foreground" />
          )}
          <h1 className="truncate text-sm font-medium">
            {category ? categoryName(category) : t(($) => $.favorites.page.title)}
          </h1>
        </div>
        {category ? (
          <FavoriteCategoryActions
            category={category}
            onDeleted={() => navigation.push(p.favorites())}
          />
        ) : !categoryId ? (
          <Button size="sm" variant="outline" onClick={() => setCreateOpen(true)}>
            <FolderPlus className="size-4" />
            {t(($) => $.favorites.page.new_category)}
          </Button>
        ) : null}
      </PageHeader>

      {category && !loading && !failed ? (
        <div className="shrink-0 border-b">
          <div className="mx-auto w-full max-w-3xl px-4 py-2">
            <Tabs
              value={typeFilter}
              onValueChange={(value) => {
                if (
                  value === "all" ||
                  value === "issue" ||
                  value === "project" ||
                  value === "markdown" ||
                  value === "html"
                ) {
                  setTypeFilter(value);
                }
              }}
            >
              <TabsList aria-label={t(($) => $.favorites.page.filter_label)}>
                <TabsTrigger value="all">
                  {t(($) => $.favorites.page.filter_all)}
                </TabsTrigger>
                <TabsTrigger value="issue">
                  {t(($) => $.favorites.page.filter_issue)}
                </TabsTrigger>
                <TabsTrigger value="project">
                  {t(($) => $.favorites.page.filter_project)}
                </TabsTrigger>
                <TabsTrigger value="markdown">
                  {t(($) => $.favorites.page.filter_markdown)}
                </TabsTrigger>
                <TabsTrigger value="html">
                  {t(($) => $.favorites.page.filter_html)}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>
        </div>
      ) : null}

      {loading ? (
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-2 p-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-11 w-full rounded-md" />
          ))}
        </div>
      ) : failed ? (
        <EmptyState
          icon={<Star className="size-10 text-muted-foreground/40" />}
          title={t(($) => $.favorites.page.load_failed)}
          action={
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                void categoriesQuery.refetch();
                if (categoryId) {
                  void favoritesQuery.refetch();
                }
              }}
            >
              {t(($) => $.favorites.page.retry)}
            </Button>
          }
        />
      ) : categoryId && !category ? (
        <EmptyState
          icon={<Folder className="size-10 text-muted-foreground/40" />}
          title={t(($) => $.favorites.page.category_not_found)}
          action={
            <Button variant="outline" size="sm" render={<AppLink href={p.favorites()} />}>
              {t(($) => $.favorites.page.back_to_categories)}
            </Button>
          }
        />
      ) : categoryId && favoriteCount === 0 ? (
        <EmptyState
          icon={<Folder className="size-10 text-muted-foreground/40" />}
          title={t(($) => $.favorites.page.category_empty_title)}
          description={t(($) => $.favorites.page.category_empty_description)}
        />
      ) : categoryId && filteredEntries.length === 0 ? (
        <EmptyState
          icon={<Folder className="size-10 text-muted-foreground/40" />}
          title={t(($) => $.favorites.page.filter_empty_title)}
        />
      ) : categoryId ? (
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto flex w-full max-w-3xl flex-col gap-2 p-4">
            {filteredEntries.map((favorite) =>
              favorite.itemType === "attachment" ? (
                <FavoriteDocumentRow
                  key={`attachment-${favorite.itemId}`}
                  attachment={favorite.attachment}
                />
              ) : (
                <FavoriteItemRow
                  key={`${favorite.itemType}-${favorite.itemId}`}
                  favorite={favorite}
                />
              ),
            )}
          </div>
        </div>
      ) : categories.length === 0 ? (
        <EmptyState
          icon={<Folder className="size-10 text-muted-foreground/40" />}
          title={t(($) => $.favorites.page.empty_categories_title)}
          description={t(($) => $.favorites.page.empty_categories_description)}
          action={
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <FolderPlus className="size-4" />
              {t(($) => $.favorites.page.new_category)}
            </Button>
          }
        />
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto w-full max-w-3xl p-4">
            <div className="overflow-hidden rounded-md border">
              {categories.map((item) => (
                <div
                  key={item.id}
                  className="flex h-12 items-center border-b pr-2 text-sm last:border-b-0 hover:bg-muted/60"
                >
                  <AppLink
                    href={p.favoriteCategory(item.id)}
                    className="flex min-w-0 flex-1 items-center gap-3 self-stretch px-3"
                  >
                    <Folder className="size-4 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate">{categoryName(item)}</span>
                    <span className="text-xs tabular-nums text-muted-foreground">
                      {t(($) => $.favorites.page.documents, {
                        count: item.favoriteCount,
                      })}
                    </span>
                  </AppLink>
                  <FavoriteCategoryActions category={item} />
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      <CreateFavoriteCategoryDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon: React.ReactNode;
  title: string;
  description?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-2 px-6 text-center">
      {icon}
      <p className="text-sm text-muted-foreground">{title}</p>
      {description && <p className="text-xs text-muted-foreground">{description}</p>}
      {action && <div className="mt-1">{action}</div>}
    </div>
  );
}
