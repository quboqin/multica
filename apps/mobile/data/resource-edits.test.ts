// @vitest-environment node
import { describe, expect, it } from "vitest";
import { buildDocumentUpdate, editableFieldValue } from "./resource-edits";

describe("resource edit payloads", () => {
  it("sends both adopted text baselines and the document revision", () => {
    expect(buildDocumentUpdate({ title: "Before", body: "Old", revision: 4 }, " After ", "New")).toEqual({
      title: "After", title_base: "Before", description: "New", description_base: "Old", expected_document_revision: 4,
    });
  });

  it("converts basic collection values without sending invalid numbers", () => {
    expect(editableFieldValue("number", "12.5")).toBe(12.5);
    expect(editableFieldValue("number", "oops")).toBeUndefined();
    expect(editableFieldValue("number", "")).toBeNull();
    expect(editableFieldValue("text", "hello")).toBe("hello");
    expect(editableFieldValue("formula", "x")).toBeUndefined();
  });
});
