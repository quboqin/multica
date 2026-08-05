import { describe, expect, it } from "vitest";
import { createDefaultPrimeComposition, primeComponentSourceRoles, readPrimeComposition, removePrimeCompositionComponent, updatePrimeCompositionRects } from "./prime-composition-editor";
import { purposeLabel } from "./market-resource-files";

describe("Prime composition configuration", () => {
  it("starts with the three requested Malaysia components and no QR", () => {
    const composition = createDefaultPrimeComposition();
    expect(composition.qr_mode).toBe("none");
    expect(composition.components.filter((component) => component.enabled).map((component) => component.id)).toEqual([
      "logo",
      "terms",
      "store_badges",
    ]);
    expect(composition.schema_version).toBe(2);
    expect(composition.components.find((component) => component.id === "logo")?.source_role).toBe("prime_logo");
    expect(composition.components.find((component) => component.id === "store_badges")?.source_role).toBe("prime_store_badges");
  });

  it("falls back safely when an older market pack has no component contract", () => {
    const composition = readPrimeComposition(undefined);
    expect(composition.schema_version).toBe(2);
    expect(composition.layouts["1080x1080"].components.logo!.destination_rect).toHaveLength(4);
  });

  it("preserves custom components and repairs malformed rectangles", () => {
    const composition = readPrimeComposition({
      schema_version: 1,
      qr_mode: "dynamic",
      components: [{ id: "custom_terms", label: "补充条款", kind: "text", enabled: true, backdrop_rule: "quiet" }],
      layouts: {
        "1080x1080": {
          components: { custom_terms: { source_rect: [10, 20, 200, 80], destination_rect: null } },
        },
      },
    });
    expect(composition.qr_mode).toBe("dynamic");
    expect(composition.schema_version).toBe(2);
    expect(composition.components.find((component) => component.id === "custom_terms")).toMatchObject({ id: "custom_terms", enabled: true, kind: "text" });
    expect(composition.layouts["1080x1080"].components.custom_terms!.destination_rect).toEqual([108, 108, 324, 216]);
  });

  it("updates an output position without introducing a source crop", () => {
    const composition = createDefaultPrimeComposition();
    const selected = [120, 80, 420, 220] as const;
    const updated = updatePrimeCompositionRects(composition, "1080x1080", "logo", {
      destination_rect: [...selected],
    });

    expect(updated.layouts["1080x1080"].components.logo).toEqual({
      destination_rect: selected,
    });
  });

  it("migrates legacy full-sheet roles to reusable component roles", () => {
    const composition = readPrimeComposition({
      schema_version: 1,
      qr_mode: "none",
      components: [{ id: "logo", label: "Logo", kind: "image", enabled: true, source_role: "prime_square" }],
      layouts: { "1080x1080": { components: { logo: { destination_rect: [30, 28, 312, 99] } } } },
    });
    expect(composition.components.find((component) => component.id === "logo")?.source_role).toBe("prime_logo");
    expect(composition.layouts["1080x1080"].components.logo?.destination_rect).toEqual([30, 28, 312, 99]);
  });

  it("does not offer legacy full-sheet files as component resources", () => {
    expect(primeComponentSourceRoles([
      { role: "prime_square", content_type: "image/png" },
      { role: "prime_logo", content_type: "image/png" },
      { role: "prime_store_badges", content_type: "image/svg+xml" },
      { role: "brand_guideline", content_type: "application/pdf" },
    ] as never[])).toEqual(["prime_logo", "prime_store_badges"]);
  });

  it("shows business names instead of internal resource roles", () => {
    expect(purposeLabel("prime_qr")).toBe("静态二维码");
    expect(purposeLabel("compliance_reference")).toBe("市场合规材料");
    expect(purposeLabel("unknown_role")).toBe("其他资料");
  });

  it("removes only custom components and their three-size positions", () => {
    const composition = readPrimeComposition({
      components: [{ id: "custom_stamp", label: "活动角标", kind: "image", enabled: true, source_role: "prime_custom_stamp" }],
    });
    const removed = removePrimeCompositionComponent(composition, "custom_stamp");
    expect(removed.components.some((component) => component.id === "custom_stamp")).toBe(false);
    expect(Object.values(removed.layouts).every((layout) => !layout.components.custom_stamp)).toBe(true);
    expect(removePrimeCompositionComponent(removed, "logo")).toBe(removed);
  });
});
