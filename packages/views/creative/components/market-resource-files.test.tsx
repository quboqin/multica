import { describe, expect, it, vi } from "vitest";
import type { CreativeResourceFile } from "@multica/core/types";
import { imagePreviewBackground, resourceFileBrowserURL } from "./market-resource-files";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "https://fat-cybertron.adakamicorp.id" } }));

function file(overrides: Partial<CreativeResourceFile> = {}): CreativeResourceFile {
  return {
    id: "file-1",
    resource_id: "market-pack-1",
    attachment_id: "attachment-1",
    role: "prime_light_square",
    label: "11-01.png",
    filename: "11-01.png",
    url: "http://storage.internal/workspaces/demo/11-01.png",
    content_type: "image/png",
    size_bytes: 1024,
    metadata: { family: "light_background" },
    created_version: 1,
    created_at: "2026-08-20T00:00:00Z",
    ...overrides,
  };
}

describe("market resource files", () => {
  it("renders bootstrapped Prime images through the stable attachment download route", () => {
    expect(resourceFileBrowserURL(file())).toBe(
      "https://fat-cybertron.adakamicorp.id/api/attachments/attachment-1/download",
    );
  });

  it("uses a white preview background for light Prime templates", () => {
    expect(imagePreviewBackground(file({ role: "prime_light_landscape", metadata: { family: "light_background" } }))).toBe("bg-white");
  });

  it("uses a black preview background only for dark Prime templates", () => {
    expect(imagePreviewBackground(file({ role: "prime_dark_landscape", metadata: { family: "dark_background" } }))).toBe("bg-black");
  });
});
