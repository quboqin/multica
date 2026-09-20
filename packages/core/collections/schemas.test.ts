// @vitest-environment node
import { describe, expect, it } from "vitest";
import { CollectionSchema, CollectionTrashSchema } from ".";

describe("collection schemas", () => {
  it("keeps the record count optional for older servers", () => {
    const base = { id: "c1", workspace_id: "w1", name: "Backlog" };
    expect(CollectionSchema.parse(base).record_count).toBeUndefined();
    expect(CollectionSchema.parse({ ...base, record_count: 12 }).record_count).toBe(12);
    expect(CollectionSchema.parse({ ...base, record_count: "x" }).record_count).toBeUndefined();
  });

  it("reads a missing or malformed title column label as the default", () => {
    const base = { id: "c1", workspace_id: "w1", name: "Backlog" };
    expect(CollectionSchema.parse(base).title_name).toBe("");
    expect(CollectionSchema.parse({ ...base, title_name: null }).title_name).toBe("");
    expect(CollectionSchema.parse({ ...base, title_name: "Customer" }).title_name).toBe("Customer");
  });

  it("tolerates malformed trash metadata", () => {
    const parsed = CollectionTrashSchema.parse({
      records: [
        {
          id: "r1",
          workspace_id: "w1",
          collection_id: "c1",
          title: "Deleted",
          fields: null,
          revision: 2,
          created_at: "2026-09-19T00:00:00Z",
        },
      ],
      total: "unknown",
    });
    expect(parsed.total).toBe(0);
    expect(parsed.retention_days).toBe(30);
    expect(parsed.records[0]?.deleted_at).toBe("");
    expect(parsed.records[0]?.fields).toEqual({});
  });
});
