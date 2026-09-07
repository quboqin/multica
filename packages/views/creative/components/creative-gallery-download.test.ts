import { Blob as NodeBlob } from "node:buffer";
import { afterEach, describe, expect, it, vi } from "vitest";
import { unzipSync } from "fflate";
import type { CreativeOrderItem, CreativeOrderVariant } from "@multica/core/types";
import { creativeVariantArchiveEntries, downloadCreativeVariantArchive, downloadCreativeVariantArchives, type DeliveryAttachment } from "./creative-order-delivery";
import { galleryItemComplete } from "./creative-material-library";

vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "" } }));
const sizes = ["1080x1080", "1200x628", "800x1000"];
const png = Uint8Array.from(Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jD1sAAAAASUVORK5CYII=", "base64"));

function fixture(activeRevision = 0) {
  const variant = { id: "v1", variant_key: "C01", revision: 2, active_revision: activeRevision, candidate_state: "selected", revisions: [{ revision: 1, expected_sizes: sizes }], assets: [1, 2].flatMap((revision) => sizes.map((size) => ({
    id: `r${revision}-${size}`, variant_id: "v1", revision, size_key: size, stage: "delivered", status: "completed", attachment_id: `r${revision}-${size}`, created_at: "2026-09-07T02:00:00Z",
  }))) } as CreativeOrderVariant;
  const item = { id: "item", candidate_id: "candidate", copy_snapshot: {}, variants: [variant] } as CreativeOrderItem;
  const attachments = new Map<string, DeliveryAttachment>(variant.assets.map((asset) => [asset.attachment_id, { id: asset.attachment_id, url: `https://images.test/${asset.id}.png`, filename: `${asset.id}.png`, content_type: "image/png", download_url: "", markdown_url: "" }]));
  return { orderId: "order", variant, item, attachments };
}

function browserDownloads() {
  let archive: NodeBlob | undefined;
  // Keep encoded text in the jsdom Uint8Array realm used by fflate.
  vi.stubGlobal("TextEncoder", class extends TextEncoder { encode(value?: string) { return Uint8Array.from(super.encode(value)); } });
  const fetchMock = vi.fn(async (_url: RequestInfo | URL, _init?: RequestInit) => new Response(new Uint8Array(png).buffer, { headers: { "content-type": "image/png" } }));
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("Blob", NodeBlob);
  vi.stubGlobal("URL", class extends URL { static createObjectURL(blob: unknown) { if (!(blob instanceof NodeBlob)) throw new Error("Expected archive Blob"); archive = blob; return "blob:archive"; } static revokeObjectURL() {} });
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  vi.useFakeTimers();
  return { fetchMock, archive: () => archive, unzip: async () => unzipSync(new Uint8Array(await archive!.arrayBuffer())) };
}

afterEach(() => { vi.runOnlyPendingTimers(); vi.useRealTimers(); vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("gallery archives", () => {
  it("packages all ten copy-library sets as thirty uniquely numbered images", async () => {
    const browser = browserDownloads();
    const packages = Array.from({ length: 10 }, (_, index) => {
      const input = fixture(2);
      const key = `C${String(index + 1).padStart(2, "0")}`;
      const variant = { ...input.variant, id: key, variant_key: key };
      const item = { ...input.item, source_kind: "copy_library", candidate_id: "", variants: [variant] } as CreativeOrderItem;
      return { ...input, variant, item, folderName: key, selection: { revision: 1, expectedSizes: sizes } };
    });
    await downloadCreativeVariantArchives({ packages });
    const zip = await browser.unzip();
    const images = Object.keys(zip).filter((name) => name.endsWith(".png"));
    expect(images).toHaveLength(30);
    expect(Object.keys(zip)).toHaveLength(31);
    expect(new Set(images).size).toBe(30);
    expect(images.map((name) => Number(name.match(/_(\d+)\.png$/)![1]))).toEqual(Array.from({ length: 30 }, (_, index) => index + 1));
    for (const [index, image] of images.entries()) {
      expect(image.startsWith(`C${String(Math.floor(index / 3) + 1).padStart(2, "0")}/`)).toBe(true);
      expect(zip[image]).toEqual(png);
    }
    expect(new TextDecoder().decode(zip["manifest.csv"])).toContain("C10");
    expect(browser.fetchMock).toHaveBeenCalledTimes(30);
    expect(browser.fetchMock.mock.calls.every(([url]) => String(url).includes("/r1-"))).toBe(true);
  });

  it("rejects zero-image packages before downloading or creating a ZIP", async () => {
    const browser = browserDownloads();
    const input = fixture();
    await expect(downloadCreativeVariantArchive(input)).rejects.toThrow("尚未齐备");
    await expect(downloadCreativeVariantArchives({ packages: [input] })).rejects.toThrow("尚未齐备");
    expect(galleryItemComplete({ ...input, order: undefined } as never, input.attachments)).toBe(false);
    expect(browser.fetchMock).not.toHaveBeenCalled();
    expect(browser.archive()).toBeUndefined();
  });

  it("downloads the saved gallery version even without an active revision", async () => {
    const browser = browserDownloads();
    const input = fixture();
    const selection = { revision: 1, expectedSizes: sizes, assetIds: sizes.map((size) => `r1-${size}`) };
    await downloadCreativeVariantArchive({ ...input, selection, addedAt: "2026-09-07T03:00:00Z" });
    const zip = await browser.unzip();
    const images = Object.keys(zip).filter((name) => name.endsWith(".png"));
    expect(images).toEqual(["20260907_P_AK_MY_Num_Regular_ALL_AI_11_1.png", "20260907_P_AK_MY_Num_Regular_ALL_AI_191_2.png", "20260907_P_AK_MY_Num_Regular_ALL_AI_45_3.png"]);
    for (const image of images) expect(zip[image]).toEqual(png);
    expect(browser.fetchMock).toHaveBeenCalledTimes(3);
    expect(browser.fetchMock.mock.calls.every(([url]) => String(url).includes("/r1-"))).toBe(true);
  });

  it("keeps batch downloads on the gallery version after the order activates another revision", async () => {
    const browser = browserDownloads();
    const input = fixture(2);
    const selection = { revision: 1, expectedSizes: sizes };
    await downloadCreativeVariantArchives({ packages: [{ ...input, selection, folderName: "selected-version" }] });
    const zip = await browser.unzip();
    expect(Object.keys(zip).filter((name) => name.startsWith("selected-version/") && name.endsWith(".png"))).toHaveLength(3);
    expect(browser.fetchMock.mock.calls.every(([url]) => String(url).includes("/r1-"))).toBe(true);
    expect(creativeVariantArchiveEntries(input.variant, input.attachments, undefined, input.item, 1, { ...selection, assetIds: ["missing"] })).toEqual([]);
  });

  it("rejects partial saved packages instead of substituting another revision", async () => {
    const browser = browserDownloads();
    const input = fixture(2);
    input.attachments.delete("r1-800x1000");
    await expect(downloadCreativeVariantArchive({ ...input, selection: { revision: 1, expectedSizes: sizes } })).rejects.toThrow("尚未齐备");
    expect(browser.fetchMock).not.toHaveBeenCalled();
    expect(browser.archive()).toBeUndefined();
  });
});
