import { expect, it } from "vitest";
import { creativeAssetPrimeComposition, creativeOrderPrimeConfig, creativePrimeConfig } from "./prime-composition";

it("separates historical config defaults from missing execution evidence", () => {
  expect(creativePrimeConfig({}).mode).toBe("deterministic");
  expect(creativePrimeConfig(undefined).mode).toBe("unknown");
  expect(creativePrimeConfig({ prime_composition_mode: "future" }).mode).toBe("unknown");
  expect(creativeOrderPrimeConfig({ input_snapshot: { market_pack: { config: { prime_composition_mode: "model_integrated", prime_model_template_family: "light" } } } })).toMatchObject({ mode: "model_integrated", templateFamilyId: "light" });
  expect(creativeAssetPrimeComposition({ stage: "delivered", status: "completed", metadata: {}, evidence: {} }).mode).toBe("unknown");
});
it("reads actual composition and template identity from both backend evidence formats", () => {
  expect(creativeAssetPrimeComposition({ stage: "primed", status: "completed", metadata: { composition: "backend_full_transparent_template", template: { family_id: "dark", source_role: "square_dark" } }, evidence: {} })).toMatchObject({ mode: "deterministic", templateFamilyId: "dark", templateRole: "square_dark" });
  const asset = { stage: "delivered", status: "completed", metadata: {}, evidence: { composition_mode: "model_integrated", template_family_id: "light", template_attachment_id: "template-id" } };
  expect(creativeAssetPrimeComposition(asset)).toMatchObject({ mode: "model_integrated", templateFamilyId: "light", templateAttachmentId: "template-id" });
  expect(creativeAssetPrimeComposition({ ...asset, stage: "generated" }).mode).toBe("unknown");
  expect(creativeAssetPrimeComposition({ ...asset, status: "running" }).mode).toBe("unknown");
});
