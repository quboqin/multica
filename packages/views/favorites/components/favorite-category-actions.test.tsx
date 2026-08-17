import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { FavoriteCategory } from "@multica/core/types";
import { FavoriteCategoryActions } from "./favorite-category-actions";

const defaultCategory: FavoriteCategory = {
  id: "category-default",
  workspaceId: "ws-1",
  userId: "user-1",
  name: "Default",
  isDefault: true,
  favoriteCount: 1,
  createdAt: "2026-07-21T00:00:00Z",
  updatedAt: "2026-07-21T00:00:00Z",
};

describe("FavoriteCategoryActions", () => {
  it("does not expose rename or delete controls for the default category", () => {
    const { container } = render(
      <FavoriteCategoryActions category={defaultCategory} />,
    );

    expect(container).toBeEmptyDOMElement();
  });
});
