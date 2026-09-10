import type { CreativeOrder, CreativeOrderAsset } from "../types";

export type CreativePrimeMode = "deterministic" | "model_integrated" | "unknown";
export type CreativePrimeComposition = {
  mode: CreativePrimeMode;
  templateFamilyId: string;
  templateRole: string;
  templateAttachmentId: string;
};

function record(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}
function text(value: unknown): string { return typeof value === "string" ? value : ""; }
function mode(value: unknown): CreativePrimeMode { return value === "deterministic" || value === "model_integrated" ? value : "unknown"; }

export function creativePrimeConfig(config: unknown): CreativePrimeComposition {
  const value = record(config);
  const configuredMode = value.prime_composition_mode;
  return {
    mode: config && typeof config === "object" && !Array.isArray(config) && (configuredMode === undefined || configuredMode === "") ? "deterministic" : mode(configuredMode),
    templateFamilyId: text(value.prime_model_template_family), templateRole: "", templateAttachmentId: "",
  };
}

export function creativeOrderPrimeConfig(order: Pick<CreativeOrder, "input_snapshot"> | undefined): CreativePrimeComposition {
  return creativePrimeConfig(record(record(order?.input_snapshot).market_pack).config);
}

export function creativeAssetPrimeComposition(asset?: Pick<CreativeOrderAsset, "stage" | "status" | "metadata" | "evidence">): CreativePrimeComposition {
  const evidence = record(asset?.evidence);
  const metadata = record(asset?.metadata);
  const template = record(metadata.template);
  const selection = record(evidence.template_selection);
  const completed = asset?.status === "completed" && (asset.stage === "primed" || asset.stage === "delivered");
  const actualMode = evidence.composition_mode !== undefined ? mode(evidence.composition_mode)
    : metadata.composition === "model_integrated_frozen_template" ? "model_integrated"
      : metadata.composition === "backend_full_transparent_template" ? "deterministic" : "unknown";
  return {
    mode: completed ? actualMode : "unknown",
    templateFamilyId: text(evidence.template_family_id) || text(template.family_id) || text(selection.selected_family_id),
    templateRole: text(evidence.template_source_role) || text(template.source_role) || text(selection.selected_source_role),
    templateAttachmentId: text(evidence.template_attachment_id) || text(template.attachment_id),
  };
}
