import { describe, expect, it, vi } from "vitest";

vi.mock("@multica/core/workspace/avatar-url", () => ({
  resolvePublicFileUrl: (url: string | undefined) => url ? `resolved:${url}` : null,
}));

import { creativeAttachmentBrowserURL } from "./creative-attachment-url";

describe("creativeAttachmentBrowserURL", () => {
  it("uses the browser-loadable storage URL when download requires API auth", () => {
    expect(creativeAttachmentBrowserURL({
      url: "https://cdn.example.test/output.png",
      download_url: "/api/attachments/11111111-2222-4333-8444-555555555555/download",
      markdown_url: "/api/attachments/11111111-2222-4333-8444-555555555555/download",
    })).toBe("resolved:https://cdn.example.test/output.png");
  });

  it("prefers a signed download URL that a native image request can load", () => {
    expect(creativeAttachmentBrowserURL({
      url: "https://private.example.test/output.png",
      download_url: "https://cdn.example.test/output.png?Signature=fresh",
      markdown_url: "",
    })).toBe("resolved:https://cdn.example.test/output.png?Signature=fresh");
  });

  it("returns an empty URL before attachment metadata is available", () => {
    expect(creativeAttachmentBrowserURL(undefined)).toBe("");
  });
});
