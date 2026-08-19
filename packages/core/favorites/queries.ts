import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const favoriteKeys = {
  all: (wsId: string) => ["favorites", wsId] as const,
  list: (wsId: string, userId: string) =>
    [...favoriteKeys.all(wsId), "list", userId] as const,
  categories: (wsId: string) =>
    [...favoriteKeys.all(wsId), "categories"] as const,
};

export function favoriteListOptions(wsId: string, userId: string) {
  return queryOptions({
    queryKey: favoriteKeys.list(wsId, userId),
    queryFn: () => api.listFavorites(),
    enabled: wsId.length > 0 && userId.length > 0,
  });
}

export function favoriteCategoryListOptions(wsId: string) {
  return queryOptions({
    queryKey: favoriteKeys.categories(wsId),
    queryFn: () => api.listFavoriteCategories(),
    enabled: wsId.length > 0,
  });
}
