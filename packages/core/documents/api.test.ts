// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import { ApiClient } from "../api/client";

afterEach(() => vi.unstubAllGlobals());

it("does not accept malformed document write responses", async () => {
  const client = new ApiClient("https://api.example.test");
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify({ id: "doc", kind: "doc" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  await expect(
    client.createDocument("Title", "Body", "workspace"),
  ).rejects.toThrow();
  await expect(
    client.updateDocument(
      "doc",
      { title: "Title", description: "Body", expected_revision: 1 },
      "workspace",
    ),
  ).rejects.toThrow();
});
