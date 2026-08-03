import type { Attachment } from "./attachment";
import type { FavoriteCategory } from "./favorite-category";

export type FavoriteType = "attachment" | "issue" | "project";
export type FavoriteItemType = Exclude<FavoriteType, "attachment">;

interface FavoriteBase {
  id: string;
  workspaceId: string;
  userId: string;
  itemId: string;
  category: FavoriteCategory;
  createdAt: string;
}

export interface AttachmentFavorite extends FavoriteBase {
  itemType: "attachment";
  attachment: Attachment;
}

export interface FavoriteItem extends FavoriteBase {
  itemType: FavoriteItemType;
  attachment?: never;
}

export type Favorite = AttachmentFavorite | FavoriteItem;
