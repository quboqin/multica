// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api";
import { IssueSchema } from "../api/schemas";
import { createDocumentDataSource, documentQuery } from "./data-source";

vi.mock("../api", () => ({ api: { listIssueTableRows: vi.fn() } }));
beforeEach(() => vi.clearAllMocks());

describe("document data source", () => {
  it("uses a workspace-specific identity and queries documents before pagination", async () => {
    vi.mocked(api.listIssueTableRows).mockResolvedValue({
      rows: [],
      total: 0,
      next_cursor: null,
      query_fingerprint: "docs",
      group_key: null,
      parent_id: null,
      branch_total: 0,
    });
    const source = createDocumentDataSource("ws", "slug");
    expect(source.identity).toEqual({
      workspaceId: "ws",
      namespace: "issues",
      sourceId: "docs",
    });
    expect(source.capabilities.hierarchy).toBe(false);
    expect(source.capabilities.maxPageSize).toBe(100);
    const signal = new AbortController().signal;
    await source.read(documentQuery, { limit: 50, cursor: "next" }, signal);
    expect(api.listIssueTableRows).toHaveBeenCalledWith(
      expect.objectContaining({
        query: expect.objectContaining({ kind: "doc" }),
        page: { limit: 50, cursor: "next" },
      }),
      { workspaceSlug: "slug", signal },
    );
  });

  it("rejects a task or another workspace in document responses", async () => {
    vi.mocked(api.listIssueTableRows).mockResolvedValue({
      rows: [{ issue: { id: "x", kind: "task", workspace_id: "ws" } }],
      total: 1,
    } as never);
    await expect(
      createDocumentDataSource("ws", "slug").read(documentQuery, {}),
    ).rejects.toThrow("Invalid document");
  });

  it("defaults absent kind to task and retains future kinds", () => {
    const kind = IssueSchema.shape.kind;
    expect(kind.parse(undefined)).toBe("task");
    expect(kind.parse("doc")).toBe("doc");
    expect(kind.parse("future")).toBe("future");
    expect(kind.safeParse(42).success).toBe(false);
  });
});
