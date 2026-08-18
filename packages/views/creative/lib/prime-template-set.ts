export type PrimeTemplateSize = "1080x1080" | "1200x628" | "800x1000";

export type PrimeTemplateSlot = {
  source_role: string;
  filename: string;
};

export type PrimeTemplateFamily = {
  id: "light_background" | "dark_background";
  label: string;
  description: string;
  templates: Record<PrimeTemplateSize, PrimeTemplateSlot>;
};

export type PrimeTemplateSet = {
  schema_version: 2;
  selection_mode: "automatic_family_contrast";
  families: PrimeTemplateFamily[];
};

export const PRIME_TEMPLATE_SIZES: Array<{ size: PrimeTemplateSize; label: string }> = [
  { size: "1080x1080", label: "方形" },
  { size: "1200x628", label: "横版" },
  { size: "800x1000", label: "竖版" },
];

export function createDefaultPrimeTemplateSet(): PrimeTemplateSet {
  return {
    schema_version: 2,
    selection_mode: "automatic_family_contrast",
    families: [
      {
        id: "light_background",
        label: "明亮底图方案",
        description: "绿色 AdaKami 标识，适合浅色或明亮的画面。",
        templates: {
          "1080x1080": { filename: "11-01.png", source_role: "prime_light_square" },
          "1200x628": { filename: "191-01.png", source_role: "prime_light_landscape" },
          "800x1000": { filename: "45-01.png", source_role: "prime_light_portrait" },
        },
      },
      {
        id: "dark_background",
        label: "深色底图方案",
        description: "白色 AdaKami 标识，适合深色或低明度的画面。",
        templates: {
          "1080x1080": { filename: "11-02.png", source_role: "prime_dark_square" },
          "1200x628": { filename: "191-03.png", source_role: "prime_dark_landscape" },
          "800x1000": { filename: "45-03.png", source_role: "prime_dark_portrait" },
        },
      },
    ],
  };
}

export function withDefaultPrimeTemplateSet(config: Record<string, unknown>): Record<string, unknown> {
  if (config.prime_template_set !== undefined && config.prime_template_set !== null) return config;
  return { ...config, prime_template_set: createDefaultPrimeTemplateSet() };
}

export function primeTemplateSlots(templateSet: PrimeTemplateSet): Array<PrimeTemplateSlot & { family: PrimeTemplateFamily; size: PrimeTemplateSize }> {
  return templateSet.families.flatMap((family) => PRIME_TEMPLATE_SIZES.map(({ size }) => ({ family, size, ...family.templates[size] })));
}

export function isPrimeTemplateRole(role: string): boolean {
  return primeTemplateSlots(createDefaultPrimeTemplateSet()).some((slot) => slot.source_role === role);
}
