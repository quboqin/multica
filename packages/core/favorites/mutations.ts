import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useAuthStore } from "../auth";
import type {
  Attachment,
  AttachmentFavorite,
  Favorite,
  FavoriteCategory,
  FavoriteItem,
  FavoriteItemType,
  FavoriteType,
} from "../types";
import {
  favoriteCategoryListOptions,
  favoriteKeys,
  favoriteListOptions,
} from "./queries";

interface ToggleAttachmentFavoriteVariables {
  attachment: Attachment;
  favorite: boolean;
}

interface ToggleFavoriteItemVariables {
  itemType: FavoriteItemType;
  itemId: string;
  favorite: boolean;
}

export interface AttachmentFavoriteToggleResult {
  favorite: boolean;
  entry?: AttachmentFavorite;
}

function optimisticDefaultCategory(
  wsId: string,
  userId: string,
): FavoriteCategory {
  return {
    id: "",
    workspaceId: wsId,
    userId,
    name: "Default",
    isDefault: true,
    favoriteCount: 0,
    createdAt: "",
    updatedAt: "",
  };
}

export function useAttachmentFavorite(
  wsId: string,
  attachment: Attachment | undefined,
) {
  const userId = useAuthStore.getState?.().user?.id ?? "";
  const queryClient = useQueryClient();
  const queryKey = favoriteKeys.list(wsId, userId);
  const categoriesKey = favoriteKeys.categories(wsId);
  const favoritesQuery = useQuery(favoriteListOptions(wsId, userId));
  const categoriesQuery = useQuery(favoriteCategoryListOptions(wsId));
  const favoriteEntry =
    attachment === undefined
      ? undefined
      : (favoritesQuery.data ?? []).find(
          (favorite): favorite is AttachmentFavorite =>
            favorite.itemType === "attachment" &&
            favorite.itemId === attachment.id,
        );

  const mutation = useMutation({
    mutationFn: async ({
      attachment: target,
      favorite,
    }: ToggleAttachmentFavoriteVariables) => {
      if (favorite) {
        const created = await api.putFavorite("attachment", target.id);
        return created.itemType === "attachment" ? created : undefined;
      }
      await api.deleteFavorite("attachment", target.id);
      return undefined;
    },
    onMutate: async ({ attachment: target, favorite }) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<Favorite[]>(queryKey);
      const defaultCategory =
        (categoriesQuery.data ?? []).find((category) => category.isDefault) ??
        optimisticDefaultCategory(wsId, userId);

      queryClient.setQueryData<Favorite[]>(queryKey, (current = []) => {
        if (favorite) {
          return current.some(
            (item) =>
              item.itemType === "attachment" && item.itemId === target.id,
          )
            ? current
            : [
                {
                  id: "",
                  workspaceId: wsId,
                  userId,
                  itemType: "attachment",
                  itemId: target.id,
                  attachment: target,
                  category: defaultCategory,
                  createdAt: new Date().toISOString(),
                },
                ...current,
              ];
        }
        return current.filter(
          (item) =>
            item.itemType !== "attachment" || item.itemId !== target.id,
        );
      });
      return { previous };
    },
    onSuccess: (result, variables) => {
      if (!variables.favorite || result === undefined) return;
      queryClient.setQueryData<Favorite[]>(queryKey, (current = []) => [
        result,
        ...current.filter(
          (item) =>
            item.itemType !== "attachment" || item.itemId !== result.itemId,
        ),
      ]);
    },
    onError: (_error, _variables, context) => {
      if (context?.previous !== undefined) {
        queryClient.setQueryData(queryKey, context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({ queryKey: categoriesKey });
    },
  });

  return {
    canFavorite:
      attachment !== undefined && wsId.length > 0 && userId.length > 0,
    favoriteEntry,
    isFavorite: favoriteEntry !== undefined,
    isPending: mutation.isPending,
    toggle: async (): Promise<AttachmentFavoriteToggleResult | undefined> => {
      if (attachment === undefined || mutation.isPending) return undefined;
      const favorite = favoriteEntry === undefined;
      const entry = await mutation.mutateAsync({ attachment, favorite });
      return favorite ? { favorite: true, entry } : { favorite: false };
    },
  };
}

export function useFavoriteItem(
  wsId: string,
  itemType: FavoriteItemType,
  itemId: string,
) {
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const queryClient = useQueryClient();
  const queryKey = favoriteKeys.list(wsId, userId);
  const favoritesQuery = useQuery(favoriteListOptions(wsId, userId));
  const favoriteEntry = (favoritesQuery.data ?? []).find(
    (item): item is FavoriteItem =>
      item.itemType === itemType && item.itemId === itemId,
  );

  const mutation = useMutation({
    mutationFn: async ({
      itemType: type,
      itemId: id,
      favorite,
    }: ToggleFavoriteItemVariables) => {
      if (favorite) {
        const created = await api.putFavorite(type, id);
        return created.itemType === "attachment" ? undefined : created;
      }
      await api.deleteFavorite(type, id);
      return undefined;
    },
    onMutate: async ({ itemType: type, itemId: id, favorite }) => {
      await queryClient.cancelQueries({ queryKey });
      const previous = queryClient.getQueryData<Favorite[]>(queryKey);
      if (!favorite) {
        queryClient.setQueryData<Favorite[]>(queryKey, (current = []) =>
          current.filter(
            (item) => item.itemType !== type || item.itemId !== id,
          ),
        );
      }
      return { previous };
    },
    onSuccess: (created, variables) => {
      if (!variables.favorite || !created) return;
      queryClient.setQueryData<Favorite[]>(queryKey, (current = []) => [
        created,
        ...current.filter(
          (item) =>
            item.itemType !== created.itemType || item.itemId !== created.itemId,
        ),
      ]);
    },
    onError: (_error, _variables, context) => {
      if (context?.previous !== undefined) {
        queryClient.setQueryData(queryKey, context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey });
      queryClient.invalidateQueries({
        queryKey: favoriteKeys.categories(wsId),
      });
    },
  });

  return {
    favoriteEntry,
    isFavorite: favoriteEntry !== undefined,
    isPending: mutation.isPending,
    toggle: () =>
      mutation.mutateAsync({
        itemType,
        itemId,
        favorite: favoriteEntry === undefined,
      }),
  };
}

export function useCreateFavoriteCategory(wsId: string) {
  const queryClient = useQueryClient();
  const queryKey = favoriteKeys.categories(wsId);

  return useMutation({
    mutationFn: (name: string) => api.createFavoriteCategory(name),
    onSuccess: (created) => {
      queryClient.setQueryData<FavoriteCategory[]>(
        queryKey,
        (current = []) => [
          ...current.filter((category) => category.id !== created.id),
          created,
        ],
      );
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey }),
  });
}

interface UpdateFavoriteCategoryVariables {
  category: FavoriteCategory;
  name: string;
}

export function useUpdateFavoriteCategory(wsId: string) {
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const queryClient = useQueryClient();
  const categoriesKey = favoriteKeys.categories(wsId);
  const favoritesKey = favoriteKeys.list(wsId, userId);

  return useMutation({
    mutationFn: ({ category, name }: UpdateFavoriteCategoryVariables) =>
      api.updateFavoriteCategory(category.id, name),
    onMutate: async ({ category, name }) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: categoriesKey }),
        queryClient.cancelQueries({ queryKey: favoritesKey }),
      ]);
      const previousCategories =
        queryClient.getQueryData<FavoriteCategory[]>(categoriesKey);
      const previousFavorites =
        queryClient.getQueryData<Favorite[]>(favoritesKey);
      const updatedAt = new Date().toISOString();

      queryClient.setQueryData<FavoriteCategory[]>(
        categoriesKey,
        (current = []) =>
          current.map((item) =>
            item.id === category.id ? { ...item, name, updatedAt } : item,
          ),
      );
      queryClient.setQueryData<Favorite[]>(favoritesKey, (current = []) =>
        current.map((favorite) =>
          favorite.category.id === category.id
            ? {
                ...favorite,
                category: { ...favorite.category, name, updatedAt },
              }
            : favorite,
        ),
      );

      return { previousCategories, previousFavorites };
    },
    onSuccess: (updated) => {
      queryClient.setQueryData<FavoriteCategory[]>(
        categoriesKey,
        (current = []) =>
          current.map((category) =>
            category.id === updated.id ? updated : category,
          ),
      );
      queryClient.setQueryData<Favorite[]>(favoritesKey, (current = []) =>
        current.map((favorite) =>
          favorite.category.id === updated.id
            ? { ...favorite, category: updated }
            : favorite,
        ),
      );
    },
    onError: (_error, _variables, context) => {
      if (context?.previousCategories !== undefined) {
        queryClient.setQueryData(categoriesKey, context.previousCategories);
      }
      if (context?.previousFavorites !== undefined) {
        queryClient.setQueryData(favoritesKey, context.previousFavorites);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: categoriesKey });
      queryClient.invalidateQueries({ queryKey: favoritesKey });
    },
  });
}

export function useDeleteFavoriteCategory(wsId: string) {
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const queryClient = useQueryClient();
  const categoriesKey = favoriteKeys.categories(wsId);
  const favoritesKey = favoriteKeys.list(wsId, userId);

  return useMutation({
    mutationFn: (category: FavoriteCategory) =>
      api.deleteFavoriteCategory(category.id),
    onMutate: async (category) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: categoriesKey }),
        queryClient.cancelQueries({ queryKey: favoritesKey }),
      ]);
      const previousCategories =
        queryClient.getQueryData<FavoriteCategory[]>(categoriesKey);
      const previousFavorites =
        queryClient.getQueryData<Favorite[]>(favoritesKey);

      queryClient.setQueryData<FavoriteCategory[]>(
        categoriesKey,
        (current = []) => current.filter((item) => item.id !== category.id),
      );
      queryClient.setQueryData<Favorite[]>(favoritesKey, (current = []) =>
        current.filter((favorite) => favorite.category.id !== category.id),
      );

      return { previousCategories, previousFavorites };
    },
    onError: (_error, _variables, context) => {
      if (context?.previousCategories !== undefined) {
        queryClient.setQueryData(categoriesKey, context.previousCategories);
      }
      if (context?.previousFavorites !== undefined) {
        queryClient.setQueryData(favoritesKey, context.previousFavorites);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: categoriesKey });
      queryClient.invalidateQueries({ queryKey: favoritesKey });
    },
  });
}

interface MoveFavoriteVariables {
  itemType: FavoriteType;
  itemId: string;
  category: FavoriteCategory;
}

export function useMoveFavorite(wsId: string) {
  const userId = useAuthStore((state) => state.user?.id ?? "");
  const queryClient = useQueryClient();
  const favoritesKey = favoriteKeys.list(wsId, userId);
  const categoriesKey = favoriteKeys.categories(wsId);

  return useMutation({
    mutationFn: ({ itemType, itemId, category }: MoveFavoriteVariables) =>
      api.moveFavorite(itemType, itemId, category.id),
    onMutate: async ({ itemType, itemId, category }) => {
      await Promise.all([
        queryClient.cancelQueries({ queryKey: favoritesKey }),
        queryClient.cancelQueries({ queryKey: categoriesKey }),
      ]);
      const previousFavorites =
        queryClient.getQueryData<Favorite[]>(favoritesKey);
      const previousCategories =
        queryClient.getQueryData<FavoriteCategory[]>(categoriesKey);
      const previousCategoryId = previousFavorites?.find(
        (favorite) =>
          favorite.itemType === itemType && favorite.itemId === itemId,
      )?.category.id;

      queryClient.setQueryData<Favorite[]>(favoritesKey, (current = []) =>
        current.map((favorite) =>
          favorite.itemType === itemType && favorite.itemId === itemId
            ? { ...favorite, category }
            : favorite,
        ),
      );
      queryClient.setQueryData<FavoriteCategory[]>(
        categoriesKey,
        (current = []) =>
          current.map((item) => {
            if (item.id === previousCategoryId && item.id !== category.id) {
              return {
                ...item,
                favoriteCount: Math.max(0, item.favoriteCount - 1),
              };
            }
            if (item.id === category.id && item.id !== previousCategoryId) {
              return { ...item, favoriteCount: item.favoriteCount + 1 };
            }
            return item;
          }),
      );
      return { previousFavorites, previousCategories };
    },
    onSuccess: (updated) => {
      queryClient.setQueryData<Favorite[]>(favoritesKey, (current = []) =>
        current.map((favorite) =>
          favorite.itemType === updated.itemType &&
          favorite.itemId === updated.itemId
            ? updated
            : favorite,
        ),
      );
    },
    onError: (_error, _variables, context) => {
      if (context?.previousFavorites !== undefined) {
        queryClient.setQueryData(favoritesKey, context.previousFavorites);
      }
      if (context?.previousCategories !== undefined) {
        queryClient.setQueryData(categoriesKey, context.previousCategories);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: favoritesKey });
      queryClient.invalidateQueries({ queryKey: categoriesKey });
    },
  });
}
