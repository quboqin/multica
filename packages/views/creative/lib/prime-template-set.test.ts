import { describe, expect, it } from "vitest";
import { createDefaultPrimeTemplateSet, isPrimeTemplateRole, withDefaultPrimeTemplateSet } from "./prime-template-set";

describe("Prime template set", () => {
  it("defines full template slots for every production size", () => {
    const templateSet = createDefaultPrimeTemplateSet();
    expect(templateSet.families.map((family) => family.id)).toEqual(["light_background", "dark_background"]);
    expect(templateSet.families[0]?.templates["1200x628"].filename).toBe("191-01.png");
    expect(templateSet.families[1]?.templates["800x1000"].source_role).toBe("prime_dark_portrait");
  });

  it("uses a fixed complete-template contract instead of component configuration", () => {
    expect(isPrimeTemplateRole("prime_dark_square")).toBe(true);
    expect(isPrimeTemplateRole("prime_light_logo")).toBe(false);
  });

  it("restores the default template set for an older market pack draft", () => {
    const config = { brand: "AdaKami" };
    expect(withDefaultPrimeTemplateSet(config)).toMatchObject({ brand: "AdaKami", prime_template_set: createDefaultPrimeTemplateSet() });
    const configured = { ...config, prime_template_set: { schema_version: 2 } };
    expect(withDefaultPrimeTemplateSet(configured)).toBe(configured);
  });
});
