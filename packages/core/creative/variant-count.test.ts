import { describe, expect, it } from "vitest";
import { creativeOrderTargetVariantCount } from "./variant-count";

describe("creative order target", () => {
  it.each([1, 3, 10])("reads a frozen target of %i", (target) => {
    expect(creativeOrderTargetVariantCount({ target_variant_count: target })).toBe(target);
  });
  it.each([undefined, null, 0, 11, 1.5, "10"])("uses the historical default for invalid values %s", (target) => {
    expect(creativeOrderTargetVariantCount({ target_variant_count: target })).toBe(3);
  });
});
