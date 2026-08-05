import { describe, expect, it } from "vitest";
import type { CreativeOrderItem } from "@multica/core/types";
import { creativeOrderSummarySelections } from "./creative-order-summary";

describe("creativeOrderSummarySelections", () => {
  it("returns only persisted adopted variants and never falls back to a latest asset", () => {
    const items = [
      {
        id: "item-pending",
        adopted_variant_id: "",
        variants: [
          { id: "pending-v01", variant_key: "V01", assets: [{ id: "latest-unadopted", updated_at: "2026-08-05T12:00:00Z" }] },
        ],
      },
      {
        id: "item-adopted",
        adopted_variant_id: "adopted-v02",
        variants: [
          { id: "adopted-v01", variant_key: "V01", assets: [{ id: "newer-but-unadopted", updated_at: "2026-08-05T13:00:00Z" }] },
          { id: "adopted-v02", variant_key: "V02", assets: [{ id: "persisted-choice", updated_at: "2026-08-05T11:00:00Z" }] },
        ],
      },
    ] as unknown as CreativeOrderItem[];

    expect(creativeOrderSummarySelections(items).map(({ item, variant }) => [item.id, variant.id])).toEqual([
      ["item-adopted", "adopted-v02"],
    ]);
  });

  it("uses the unique completed variant only for a direct edit order", () => {
    const directItem = {
      id: "direct-item",
      adopted_variant_id: "",
      variants: [{ id: "direct-result", status: "completed", variant_key: "DIRECT" }],
    } as unknown as CreativeOrderItem;

    expect(creativeOrderSummarySelections([directItem])).toEqual([]);
    expect(creativeOrderSummarySelections([directItem], true).map(({ variant }) => variant.id)).toEqual(["direct-result"]);
  });
});
