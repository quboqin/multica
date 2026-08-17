export interface FavoriteCategory {
  id: string;
  workspaceId: string;
  userId: string;
  name: string;
  isDefault: boolean;
  favoriteCount: number;
  createdAt: string;
  updatedAt: string;
}
