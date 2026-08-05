import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type {
  Attachment,
  Favorite,
  FavoriteCategory,
  FavoriteItem,
} from "@multica/core/types";
import { FavoritesPage } from "./favorites-page";

const { queryState } = vi.hoisted(() => ({
  queryState: {
    categories: [] as FavoriteCategory[],
    favorites: [] as Favorite[],
    content: { text: "", originalContentType: "text/markdown" },
  },
}));

vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: ({ queryKey }: { queryKey: readonly unknown[] }) => ({
    data:
      queryKey[0] === "attachment-content"
        ? queryState.content
        : queryKey[2] === "categories"
        ? queryState.categories
        : queryState.favorites,
    isLoading: false,
    isError: false,
    refetch: vi.fn(),
  }),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));
vi.mock("@multica/core/paths", () => ({
  useWorkspacePaths: () => ({
    favorites: () => "/acme/favorites",
    favoriteCategory: (id: string) => `/acme/favorites/${id}`,
    issueDetail: (id: string) => `/acme/issues/${id}`,
  }),
}));
vi.mock("../../editor", () => ({
  Attachment: ({
    attachment,
    trailingAction,
  }: {
    attachment: { kind: "record"; attachment: Attachment };
    trailingAction?: React.ReactNode;
  }) => (
    <div>
      {attachment.attachment.filename}
      {trailingAction}
    </div>
  ),
}));
vi.mock("../../i18n", () => ({
  useT: () => ({
    t: (selector: (resources: Record<string, unknown>) => string) =>
      selector({
        favorites: {
          page: {
            title: "My Favorites",
            new_category: "New category",
            empty_categories_title: "No categories",
            empty_categories_description: "Create one",
            category_empty_title: "No documents",
            category_empty_description: "Move documents here",
            category_not_found: "Not found",
            back_to_categories: "Back",
            documents: "documents",
            load_failed: "Load failed",
            retry: "Retry",
            open_source_conversation: "Open source conversation",
            filter_label: "Document type",
            filter_all: "All",
            filter_issue: "Tasks",
            filter_project: "Projects",
            filter_markdown: "MD",
            filter_html: "HTML",
            filter_empty_title: "No documents of this type",
          },
          category: { default_name: "Default" },
          item: { unavailable: "Unavailable favorite" },
        },
      }),
  }),
}));
vi.mock("../../layout/page-header", () => ({
  PageHeader: ({ children }: { children: React.ReactNode }) => <header>{children}</header>,
}));
vi.mock("../../navigation", () => ({
  AppLink: ({ children, href, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement> & { href: string }) => (
    <a href={href} {...props}>{children}</a>
  ),
  useNavigation: () => ({ push: vi.fn() }),
}));
vi.mock("./favorite-category-dialog", () => ({
  CreateFavoriteCategoryDialog: () => null,
}));
vi.mock("./favorite-category-actions", () => ({
  FavoriteCategoryActions: ({ category }: { category: FavoriteCategory }) => (
    <button type="button">Manage {category.name}</button>
  ),
}));
vi.mock("./favorite-item-row", () => ({
  FavoriteItemRow: ({ favorite }: { favorite: FavoriteItem }) => (
    <div>{favorite.itemType}:{favorite.itemId}</div>
  ),
}));

const category: FavoriteCategory = {
  id: "category-1",
  workspaceId: "ws-1",
  userId: "user-1",
  name: "Default",
  isDefault: true,
  favoriteCount: 1,
  createdAt: "2026-07-21T00:00:00Z",
  updatedAt: "2026-07-21T00:00:00Z",
};

const attachment: Attachment = {
  id: "attachment-1",
  workspace_id: "ws-1",
  issue_id: "issue-1",
  comment_id: "comment-1",
  chat_session_id: null,
  chat_message_id: null,
  uploader_type: "member",
  uploader_id: "user-1",
  filename: "summary.md",
  url: "/uploads/summary.md",
  download_url: "/api/attachments/attachment-1/download",
  markdown_url: "/api/attachments/attachment-1/download",
  content_type: "text/markdown",
  size_bytes: 128,
  created_at: "2026-07-21T00:00:00Z",
};

const htmlAttachment: Attachment = {
  ...attachment,
  id: "attachment-2",
  issue_id: "issue-2",
  comment_id: "comment-2",
  filename: "report.html",
  url: "/uploads/report.html",
  download_url: "/api/attachments/attachment-2/download",
  markdown_url: "/api/attachments/attachment-2/download",
  content_type: "text/html",
};

describe("FavoritesPage", () => {
  beforeEach(() => {
    queryState.categories = [category];
    queryState.favorites = [
      {
        id: "favorite-attachment-1",
        workspaceId: "ws-1",
        userId: "user-1",
        itemType: "attachment",
        itemId: attachment.id,
        attachment,
        category,
        createdAt: "2026-07-21T00:00:00Z",
      },
    ];
    queryState.content = {
      text: "First line\n\nSecond line",
      originalContentType: "text/markdown",
    };
  });

  it("shows categories at the root without rendering documents", () => {
    render(<FavoritesPage />);

    expect(screen.getByText("Default")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Default/ })).toHaveAttribute(
      "href",
      "/acme/favorites/category-1",
    );
    expect(screen.queryByText("summary.md")).not.toBeInTheDocument();
  });

  it("shows only documents in the selected category", () => {
    render(<FavoritesPage categoryId="category-1" />);

    expect(screen.getByText("summary.md")).toBeInTheDocument();
    const summary = screen.getByText("First line Second line");
    expect(summary).toBeInTheDocument();
    expect(summary.parentElement).toContainElement(screen.getByText("summary.md"));
    expect(summary.parentElement).toHaveClass("overflow-hidden", "border");
    expect(
      screen.getByRole("link", { name: "Open source conversation" }),
    ).toHaveAttribute(
      "href",
      "/acme/issues/issue-1?comment=comment-1",
    );
  });

  it("filters favorite documents by MD and HTML type", () => {
    queryState.favorites = [
      {
        id: "favorite-attachment-1",
        workspaceId: "ws-1",
        userId: "user-1",
        itemType: "attachment",
        itemId: attachment.id,
        attachment,
        category,
        createdAt: "2026-07-21T00:00:00Z",
      },
      {
        id: "favorite-attachment-2",
        workspaceId: "ws-1",
        userId: "user-1",
        itemType: "attachment",
        itemId: htmlAttachment.id,
        attachment: htmlAttachment,
        category,
        createdAt: "2026-07-21T00:00:00Z",
      },
    ];
    render(<FavoritesPage categoryId="category-1" />);

    for (const name of ["All", "Tasks", "Projects", "MD", "HTML"]) {
      expect(screen.getByRole("tab", { name })).toHaveClass("cursor-pointer");
    }

    expect(screen.getByText("summary.md")).toBeInTheDocument();
    expect(screen.getByText("report.html")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "MD" }));
    expect(screen.getByText("summary.md")).toBeInTheDocument();
    expect(screen.queryByText("report.html")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "HTML" }));
    expect(screen.queryByText("summary.md")).not.toBeInTheDocument();
    expect(screen.getByText("report.html")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "All" }));
    expect(screen.getByText("summary.md")).toBeInTheDocument();
    expect(screen.getByText("report.html")).toBeInTheDocument();
  });

  it("filters task and project favorites in the selected category", () => {
    queryState.favorites = [
      ...queryState.favorites,
      {
        id: "favorite-issue",
        workspaceId: "ws-1",
        userId: "user-1",
        itemType: "issue",
        itemId: "issue-1",
        category,
        createdAt: "2026-07-21T00:00:02Z",
      },
      {
        id: "favorite-project",
        workspaceId: "ws-1",
        userId: "user-1",
        itemType: "project",
        itemId: "project-1",
        category,
        createdAt: "2026-07-21T00:00:01Z",
      },
    ];
    render(<FavoritesPage categoryId="category-1" />);

    fireEvent.click(screen.getByRole("tab", { name: "Tasks" }));
    expect(screen.getByText("issue:issue-1")).toBeInTheDocument();
    expect(screen.queryByText("project:project-1")).not.toBeInTheDocument();
    expect(screen.queryByText("summary.md")).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: "Projects" }));
    expect(screen.queryByText("issue:issue-1")).not.toBeInTheDocument();
    expect(screen.getByText("project:project-1")).toBeInTheDocument();
  });

  it("limits the summary to 160 Unicode characters", () => {
    queryState.content = {
      text: `${"文".repeat(161)} trailing`,
      originalContentType: "text/markdown",
    };

    render(<FavoritesPage categoryId="category-1" />);

    expect(screen.getByText(`${"文".repeat(160)}...`)).toBeInTheDocument();
  });

  it("shows a renamed default category by its new name", () => {
    queryState.categories = [{ ...category, name: "Research" }];

    render(<FavoritesPage />);

    expect(screen.getByText("Research")).toBeInTheDocument();
    expect(screen.queryByText("Default")).not.toBeInTheDocument();
  });
});
