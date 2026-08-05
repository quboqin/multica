import { describe, expect, it, vi } from "vitest";

vi.mock("@multica/core/workspace/avatar-url", () => ({
  resolvePublicFileUrl: (url: string | undefined) => url ? `resolved:${url}` : null,
}));

import { creativeAttachmentBrowserURL } from "./creative-attachment-url";

describe("creativeAttachmentBrowserURL", () => {
  it("uses the authenticated same-origin endpoint instead of a private object URL", () => {
    expect(creativeAttachmentBrowserURL({
      id: "11111111-2222-4333-8444-555555555555",
      url: "https://cdn.example.test/output.png",
      download_url: "/api/attachments/11111111-2222-4333-8444-555555555555/download",
      markdown_url: "/api/attachments/11111111-2222-4333-8444-555555555555/download",
    })).toBe("/api/attachments/11111111-2222-4333-8444-555555555555/download");
  });

  it("prefers the same-origin endpoint even when the response also has a signed URL", () => {
    expect(creativeAttachmentBrowserURL({
      id: "11111111-2222-4333-8444-555555555555",
      url: "https://private.example.test/output.png",
      download_url: "https://cdn.example.test/output.png?Signature=fresh",
      markdown_url: "",
    })).toBe("/api/attachments/11111111-2222-4333-8444-555555555555/download");
  });

  it("keeps a signed URL as a legacy fallback when attachment identity is unavailable", () => {
    expect(creativeAttachmentBrowserURL({
      id: "legacy-attachment",
      url: "https://private.example.test/output.png",
      download_url: "https://cdn.example.test/output.png?Signature=fresh",
      markdown_url: "",
    })).toBe("resolved:https://cdn.example.test/output.png?Signature=fresh");
  });

  it("returns an empty URL before attachment metadata is available", () => {
    expect(creativeAttachmentBrowserURL(undefined)).toBe("");
  });
});
