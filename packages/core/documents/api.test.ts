// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient } from "../api/client";
afterEach(() => vi.unstubAllGlobals());
const response = (value: unknown) =>
  new Response(JSON.stringify(value), {
    headers: { "Content-Type": "application/json" },
  });
it("fails closed on malformed sharing and history responses", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockImplementation(() =>
      Promise.resolve(
        response({
          can_edit: "true",
          can_manage: "true",
          versions: "invalid",
        }),
      ),
    ),
  );
  const client = new ApiClient("https://example.test");
  await expect(
    client.getDocumentAccess("doc", { workspaceId: "ws" }),
  ).rejects.toThrow("Invalid document sharing");
  await expect(client.listDocumentVersions("doc", "ws")).rejects.toThrow(
    "Invalid document versions",
  );
  await expect(client.getDocumentVersion("doc", 1, "ws")).rejects.toThrow(
    "Invalid document version",
  );
});
it("defaults omitted permissions to false and sends the sharing revision", async () => {
  const fetch = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(
        response({ owner_id: "owner", scope: "private", revision: 2 }),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const client = new ApiClient("https://example.test");
  const access = await client.getDocumentAccess("doc", { workspaceId: "ws" });
  expect(access).toMatchObject({
    can_edit: false,
    can_manage: false,
    scope_role: "view",
    collaborators: [],
  });
  await client.updateDocumentAccess(
    "doc",
    {
      scope: "workspace",
      scope_role: "edit",
      project_id: null,
      collaborators: [],
      expected_revision: 2,
    },
    "ws",
  );
  expect(JSON.parse(String(fetch.mock.calls[1]![1].body))).toMatchObject({
    scope_role: "edit",
    expected_revision: 2,
  });
  expect(
    new Headers(fetch.mock.calls[1]![1].headers).get("X-Workspace-ID"),
  ).toBe("ws");
});
