// @vitest-environment node
import { describe, expect, it } from "vitest";
import { CollectionFieldSchema, CollectionRecordSchema } from "./index";

describe("formula response compatibility", () => {
  const field = { id: "f", name: "Total", type: "formula" };
  const row = {
    id: "r",
    workspace_id: "w",
    collection_id: "c",
    title: "Row",
    revision: 1,
    created_at: "",
  };
  it("keeps scalar results, stable bindings and error codes", () => {
    expect(
      CollectionFieldSchema.parse({
        ...field,
        config: {
          formula: { expression: "{Price} * 2", bindings: { Price: "p" } },
        },
      }).config.formula?.bindings,
    ).toEqual({ Price: "p" });
    expect(
      CollectionRecordSchema.parse({
        ...row,
        fields: { f: 12 },
        formula_errors: { broken: "#REF!" },
      }),
    ).toMatchObject({ fields: { f: 12 }, formula_errors: { broken: "#REF!" } });
  });
  it("tolerates old and malformed optional projections", () => {
    expect(CollectionFieldSchema.parse(field).config.formula).toBeUndefined();
    expect(
      CollectionFieldSchema.parse({
        ...field,
        config: { formula: { expression: 42 } },
      }).config.formula,
    ).toBeUndefined();
    expect(
      CollectionFieldSchema.parse({
        ...field,
        config: { formula: { expression: "1", bindings: "bad" } },
      }).config.formula,
    ).toEqual({ expression: "1", bindings: undefined });
    expect(
      CollectionRecordSchema.parse({ ...row, formula_errors: { f: 42 } })
        .formula_errors,
    ).toBeUndefined();
    expect(CollectionRecordSchema.parse(row).formula_errors).toBeUndefined();
  });
});
