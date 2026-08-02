import { describe, expect, it } from "vitest";

import {
  candidateIdFromResultComment,
  creativeDeliveryInfo,
  groupCreativeDeliveries,
  isCreativeDeliveryFilename,
  recommendCopyEntries,
} from "./creative-material-pool";
import type { CreativeCopyEntry, CreativeMaterialCandidate } from "@multica/core/types";

describe("creative delivery filenames", () => {
  it("accepts the market pack naming rule and groups all sizes by its stable prefix", () => {
    expect(creativeDeliveryInfo("JULY_AdaKami_Indonesia_easycash-0-bunga_800x1000_v1.png")).toEqual({
      branch: "JULY_AdaKami_Indonesia_easycash-0-bunga",
      size: "800x1000",
      revision: 1,
    });
    expect(creativeDeliveryInfo("JULY_AdaKami_Indonesia_easycash-0-bunga_1080x1080_v12.png")).toEqual({
      branch: "JULY_AdaKami_Indonesia_easycash-0-bunga",
      size: "1080x1080",
      revision: 12,
    });
    const accepted = [
      "January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_800x1000_v1.png",
      "January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1080x1080_v2.png",
      "January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2_1200x628_v4.png",
    ].map((filename) => creativeDeliveryInfo(filename));
    expect(new Set(accepted.map((info) => info?.branch))).toEqual(
      new Set(["January_AdaKami_Indonesia_e12968bb-af24-4a6f-aa17-05d6bec59ab2"]),
    );
  });

  it("keeps compatibility with the original issue-native delivery names", () => {
    expect(creativeDeliveryInfo("candidate-1__prime-1200x628.png")).toEqual({
      branch: "candidate-1",
      size: "1200x628",
      revision: null,
    });
    expect(creativeDeliveryInfo("1080x1080.png")).toEqual({
      branch: "current",
      size: "1080x1080",
      revision: null,
    });
  });

  it("rejects source and evidence images that are not final deliveries", () => {
    expect(isCreativeDeliveryFilename("adc47_base_800x1000.png")).toBe(false);
    expect(isCreativeDeliveryFilename("prime_template.png")).toBe(false);
    expect(isCreativeDeliveryFilename("creative_800x1000_v1.jpg")).toBe(false);
  });

  it("shows only the highest revision for each creative size", () => {
    const filenames = [
      "AdaKami_ID_06c7762d_1080x1080_v1.png",
      "AdaKami_ID_06c7762d_1200x628_v1.png",
      "AdaKami_ID_06c7762d_800x1000_v1.png",
      "AdaKami_ID_06c7762d_1080x1080_v2.png",
      "AdaKami_ID_06c7762d_1200x628_v2.png",
      "AdaKami_ID_06c7762d_800x1000_v2.png",
    ];
    const groups = groupCreativeDeliveries(
      filenames.map((filename, index) => ({ id: String(index), filename })),
    );
    expect(groups).toHaveLength(1);
    expect(groups[0]?.assets.map((asset) => asset.filename)).toEqual([
      "AdaKami_ID_06c7762d_1080x1080_v2.png",
      "AdaKami_ID_06c7762d_1200x628_v2.png",
      "AdaKami_ID_06c7762d_800x1000_v2.png",
    ]);
  });

  it("keeps three variants of one candidate as three creative groups", () => {
    const filenames = [1, 2, 3].flatMap((variant) =>
      ["1080x1080", "1200x628", "800x1000"].map(
        (size) => `August_AdaKami_Indonesia_candidate-42_V0${variant}_${size}_v1.png`,
      ),
    );
    const groups = groupCreativeDeliveries(
      filenames.map((filename, index) => ({ id: String(index), filename })),
    );

    expect(groups).toHaveLength(3);
    expect(groups.map((group) => group.variant)).toEqual([1, 2, 3]);
    expect(groups.every((group) => group.assets.length === 3)).toBe(true);
    expect(groups.map((group) => group.setKey)).toEqual([
      "August_AdaKami_Indonesia_candidate-42",
      "August_AdaKami_Indonesia_candidate-42",
      "August_AdaKami_Indonesia_candidate-42",
    ]);
  });

  it("links a published result comment back to its source candidate", () => {
    expect(candidateIdFromResultComment("结果发布\n候选 ID：`7c618bb5-6efe-4b55-8eaa-4be430b6e536`\n三尺寸通过")).toBe(
      "7c618bb5-6efe-4b55-8eaa-4be430b6e536",
    );
    expect(candidateIdFromResultComment("已发布 9 张图\n候选素材：`19c9a621-51c2-4dad-9529-c4544865066e`；文案快照 v15")).toBe(
      "19c9a621-51c2-4dad-9529-c4544865066e",
    );
  });

  it("ranks approved copy by the confirmed primary benefit instead of crawl metadata", () => {
    const candidate = {
      title: "Bunga lebih rendah mulai 0,01%",
      tags: ["rate down"],
      media_names: [],
    } as unknown as CreativeMaterialCandidate;
    const base = {
      status: "approved",
      tags: [],
      subheadline: "",
      benefit: "",
      cta: "",
      copy_role: "PRIME DESIGN",
      metadata: {},
      updated_at: "2026-08-01T00:00:00Z",
    } as unknown as CreativeCopyEntry;
    const ranked = recommendCopyEntries(candidate, [
      { ...base, id: "rate", external_key: "rate", headline: "Bunga mulai dari 0,01%" },
      { ...base, id: "fee", external_key: "fee", headline: "Potongan biaya 25%" },
    ], {
      theme: "世界杯 / 足球赛事",
      theme_elements: ["足球", "球场"],
      primary_benefit: "费用减免",
      secondary_benefits: [],
      benefit_value: "Potongan biaya 25%",
      source_semantics: "以足球赛事氛围表达费用减免活动",
      information_mechanism: "赛事主视觉加降费信息",
      visual_anchors: ["足球", "主标题"],
      palette_anchors: ["绿色主色"],
      must_preserve: ["赛事语义", "费用减免"],
      allowed_variations: ["版式布局"],
      evidence: ["画面主标题出现 biaya 与 25%"],
      detected_text: ["Potongan biaya 25%"],
      visual_type: "主题活动海报",
      analysis_summary: "足球赛事氛围承载降费主张",
      status: "confirmed",
      source: "mixed",
      confidence: 0.96,
      analysis_issue_id: "",
    });
    expect(ranked[0]?.entry.id).toBe("fee");
    expect(ranked[0]?.reasons).toContain("主利益点：费用减免");
  });
});
