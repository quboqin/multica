import { fireEvent, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { renderWithI18n } from "../test/i18n";
import { DocumentBreadcrumb } from "./document-breadcrumb";
vi.mock("@multica/core/paths", () => ({ useWorkspacePaths: () => ({ documents: () => "/acme/documents", documentDetail: (id: string) => `/acme/documents/${id}` }) }));
vi.mock("../navigation", () => ({ AppLink: ({ children, ...props }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => <a {...props}>{children}</a> }));
it("keeps a deep path in a menu with every ancestor still reachable", async () => {
  const trail = Array.from({ length: 12 }, (_, i) => ({ id: String(i), title: `Document ${i}` }) as Issue);
  renderWithI18n(<DocumentBreadcrumb trail={trail} />);
  expect(screen.getAllByRole("link")).toHaveLength(2);
  expect(screen.getByRole("link", { name: "Document 11" })).toHaveAttribute("aria-current", "page");
  fireEvent.click(screen.getByRole("button", { name: "Show parent documents" }));
  expect(await screen.findAllByRole("menuitem")).toHaveLength(11);
  expect(screen.getByRole("menuitem", { name: "Document 0" })).toHaveAttribute("href", "/acme/documents/0");
});
