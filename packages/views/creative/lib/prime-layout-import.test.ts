import { describe, expect, it } from "vitest";
import { createDefaultPrimeComposition } from "../components/prime-composition-editor";
import type { CreativeMarketPackComponentExtraction } from "@multica/core/types";
import {
  applyPrimeLayoutImport,
  detectedPrimeImportCandidates,
  inferPrimeImportSize,
  primeImportCandidates,
  scalePrimeImportRect,
  updatePrimeImportCandidateRect,
} from "./prime-layout-import";

describe("Prime layout import", () => {
  it("infers a supported source size from its aspect ratio", () => {
    expect(inferPrimeImportSize(2160, 2160)).toBe("1080x1080");
    expect(inferPrimeImportSize(1600, 2000)).toBe("800x1000");
    expect(inferPrimeImportSize(1200, 628)).toBe("1200x628");
    expect(inferPrimeImportSize(1080, 1920)).toBeNull();
  });

  it("finds reusable image candidates from the current layout", () => {
    const composition = createDefaultPrimeComposition();
    const candidates = primeImportCandidates(composition, "1080x1080");
    expect(candidates.map((candidate) => candidate.componentId)).toEqual([
      "logo",
      "qr",
      "store_badges",
      "afpi",
      "pindai_legal",
    ]);
    expect(candidates.find((candidate) => candidate.componentId === "logo")?.selected).toBe(true);
    expect(candidates.find((candidate) => candidate.componentId === "qr")?.selected).toBe(false);
  });

  it("enables selected components and carries their normalized positions to every size", () => {
    const composition = createDefaultPrimeComposition();
    const candidates = primeImportCandidates(composition, "1080x1080").map((candidate) => ({
      ...candidate,
      selected: candidate.componentId === "logo" || candidate.componentId === "qr",
      rect: candidate.componentId === "logo" ? [100, 50, 400, 150] as [number, number, number, number] : candidate.rect,
    }));
    const imported = applyPrimeLayoutImport(composition, "1080x1080", candidates);

    expect(imported.qr_mode).toBe("static");
    expect(imported.components.find((component) => component.id === "logo")?.enabled).toBe(true);
    expect(imported.components.find((component) => component.id === "store_badges")?.enabled).toBe(false);
    expect(imported.layouts["800x1000"].components.logo?.destination_rect).toEqual([74, 46, 296, 139]);
  });

  it("scales and clamps source rectangles safely", () => {
    expect(scalePrimeImportRect([100, 100, 300, 200], "1080x1080", 2160, 2160)).toEqual([200, 200, 600, 400]);
    const candidate = primeImportCandidates(createDefaultPrimeComposition(), "1080x1080")[0]!;
    expect(updatePrimeImportCandidateRect(candidate, { x: 1070, width: 200 }, "1080x1080").rect).toEqual([880, 32, 1080, 151]);
  });

  it("maps a variable number of detections to known and custom Prime components", () => {
    const extraction: CreativeMarketPackComponentExtraction = {
      id: "extract-1", resource_id: "market-1", source_attachment_id: "attachment-1",
      source_url: "/source.png", source_filename: "source.png", source_width: 2160, source_height: 2160,
      status: "completed", error_message: "", task_id: "task-1", agent_id: "agent-1", created_at: "", updated_at: "",
      result: {
        summary: "two components",
        candidates: [
          { id: "detected-logo", label: "Logo", kind: "image", suggested_component_id: "logo", suggested_role: "", content: "", rect: [60, 56, 628, 200], confidence: 0.98, evidence: [] },
          { id: "detected-seal", label: "New seal", kind: "image", suggested_component_id: "", suggested_role: "", content: "", rect: [1900, 1900, 2100, 2100], confidence: 0.7, evidence: [] },
        ],
      },
    };
    const candidates = detectedPrimeImportCandidates(extraction, createDefaultPrimeComposition(), "1080x1080");
    expect(candidates).toHaveLength(2);
    expect(candidates[0]).toMatchObject({ componentId: "logo", role: "prime_logo", rect: [30, 28, 314, 100] });
    expect(candidates[1]).toMatchObject({ componentId: "custom_1", role: "prime_custom_1" });

    const imported = applyPrimeLayoutImport(createDefaultPrimeComposition(), "1080x1080", candidates);
    expect(imported.components.find((component) => component.id === "custom_1")).toMatchObject({ enabled: true, source_role: "prime_custom_1" });
    expect(imported.layouts["800x1000"].components.custom_1).toBeDefined();
  });
});
