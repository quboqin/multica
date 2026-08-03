import type { FavoriteCategory } from "@multica/core/types";

export function favoriteCategoryDisplayName(
  category: FavoriteCategory,
  defaultName: string,
): string {
  return category.isDefault && category.name === "Default"
    ? defaultName
    : category.name;
}
