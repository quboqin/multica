// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient } from "../api/client";
afterEach(() => vi.unstubAllGlobals());
const response = (value: unknown) =>
  new Response(JSON.stringify(value), {
    headers: { "Content-Type": "application/json" },
  });
it("fails closed on malformed collection permissions and pin responses", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockImplementation(() =>
        Promise.resolve(response({ can_edit: "true", can_manage: "true" })),
      ),
  );
  const client = new ApiClient("https://example.test");
  await expect(
    client.getCollectionAccess("table", { workspaceId: "ws" }),
  ).rejects.toThrow("Invalid collection sharing");
  await expect(
    client.updateCollectionAccess(
      "table",
      {
        scope: "workspace",
        scope_role: "edit",
        project_id: null,
        collaborators: [],
        expected_revision: 1,
      },
      "ws",
    ),
  ).rejects.toThrow("Invalid collection sharing");
  expect(await client.listPins()).toEqual([]);
  await expect(
    client.createPin({ item_type: "collection", item_id: "table" }),
  ).rejects.toThrow("Invalid pin response");
});
it("defaults absent rights to false and includes the sharing revision and workspace", async () => {
  const fetch = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(
        response({ owner_id: "owner", scope: "private", revision: 2 }),
      ),
    );
  vi.stubGlobal("fetch", fetch);
  const client = new ApiClient("https://example.test");
  expect(
    await client.getCollectionAccess("table", { workspaceId: "ws" }),
  ).toMatchObject({
    can_edit: false,
    can_manage: false,
    scope_role: "view",
    collaborators: [],
  });
  await client.updateCollectionAccess(
    "table",
    {
      scope: "workspace",
      scope_role: "edit",
      project_id: null,
      collaborators: [],
      expected_revision: 2,
    },
    "ws",
  );
  expect(JSON.parse(fetch.mock.calls[1]![1].body)).toMatchObject({
    scope_role: "edit",
    expected_revision: 2,
  });
  expect(
    new Headers(fetch.mock.calls[1]![1].headers).get("X-Workspace-ID"),
  ).toBe("ws");
});
it("opts in to collection pins and hides unknown pin kinds", async () => {
  const pin = {
    id: "p",
    workspace_id: "ws",
    user_id: "u",
    item_type: "collection",
    item_id: "c",
    position: 1,
    created_at: "now",
  };
  const fetch = vi
    .fn()
    .mockImplementation(() =>
      Promise.resolve(response([pin, { ...pin, item_type: "future" }])),
    );
  vi.stubGlobal("fetch", fetch);
  expect(await new ApiClient("https://example.test").listPins()).toEqual([pin]);
  expect(fetch.mock.calls[0]![0]).toContain("include=view,collection");
});
