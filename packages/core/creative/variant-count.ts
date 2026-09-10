export const DEFAULT_CREATIVE_VARIANT_COUNT = 3;
export const MAX_CREATIVE_VARIANT_COUNT = 10;

export function creativeOrderTargetVariantCount(snapshot?: Record<string, unknown>): number {
  const value = snapshot?.target_variant_count;
  return typeof value === "number" && Number.isInteger(value) && value >= 1 && value <= MAX_CREATIVE_VARIANT_COUNT
    ? value : DEFAULT_CREATIVE_VARIANT_COUNT;
}
