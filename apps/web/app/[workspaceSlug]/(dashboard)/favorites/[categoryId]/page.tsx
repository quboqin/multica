"use client";

import { use } from "react";
import { FavoritesPage } from "@multica/views/favorites";

export default function FavoriteCategoryRoute({
  params,
}: {
  params: Promise<{ categoryId: string }>;
}) {
  const { categoryId } = use(params);
  return <FavoritesPage categoryId={categoryId} />;
}
