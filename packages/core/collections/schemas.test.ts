// @vitest-environment node
import { describe, expect, it } from "vitest";
import {
  CollectionFieldSchema,
  CollectionRecordSchema,
  CollectionSchema,
  CollectionTrashSchema,
  RecordBacklinksSchema,
} from ".";

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

  it("reads a relation field's target and drops a malformed one", () => {
    const base = { id: "f1", name: "Tasks", type: "relation" };
    expect(
      CollectionFieldSchema.parse({
        ...base,
        config: { relation: { to_type: "issue" } },
      }).config,
    ).toEqual({ options: [], relation: { to_type: "issue", collection_id: "" } });
    expect(
      CollectionFieldSchema.parse({
        ...base,
        config: { relation: { to_type: "record", collection_id: "c2" } },
      }).config.relation,
    ).toEqual({ to_type: "record", collection_id: "c2" });
    // A target this build has never heard of still parses; the UI decides.
    expect(
      CollectionFieldSchema.parse({
        ...base,
        config: { relation: { to_type: "document" } },
      }).config.relation?.to_type,
    ).toBe("document");
    expect(
      CollectionFieldSchema.parse({ ...base, config: { relation: "tasks" } })
        .config.relation,
    ).toBeUndefined();
    expect(CollectionFieldSchema.parse(base).config.relation).toBeUndefined();
  });

  it("reads relation cells, and none from a server that predates them", () => {
    const base = {
      id: "r1",
      workspace_id: "w1",
      collection_id: "c1",
      title: "Row",
      fields: {},
      revision: 1,
      created_at: "2026-09-20T00:00:00Z",
    };
    expect(CollectionRecordSchema.parse(base).links).toEqual({});
    expect(CollectionRecordSchema.parse({ ...base, links: null }).links).toEqual({});
    const parsed = CollectionRecordSchema.parse({
      ...base,
      links: {
        f1: [
          { id: "l1", to_type: "issue", to_id: "i1", title: "Ship it", identifier: "MUL-7", status: "todo", missing: false },
          { id: "l2", to_type: "issue", to_id: "i2", missing: true },
        ],
        // One unreadable cell must not take the other cells with it.
        f2: [{ to_type: "record" }],
      },
    });
    expect(parsed.links.f1).toEqual([
      { id: "l1", to_type: "issue", to_id: "i1", title: "Ship it", identifier: "MUL-7", status: "todo", collection_id: "", missing: false },
      { id: "l2", to_type: "issue", to_id: "i2", title: "", identifier: "", status: "", collection_id: "", missing: true },
    ]);
    expect(parsed.links.f2).toEqual([]);
  });

  it("degrades malformed backlinks to an empty list", () => {
    expect(RecordBacklinksSchema.parse({ links: "nope" }).links).toEqual([]);
    expect(RecordBacklinksSchema.parse({ links: [{ id: "l1" }] }).links).toEqual([]);
    expect(
      RecordBacklinksSchema.parse({
        links: [{ id: "l1", collection_id: "c1", record_id: "r1", record_title: null }],
      }).links,
    ).toEqual([
      { id: "l1", collection_id: "c1", collection_name: "", record_id: "r1", record_title: "", field_id: "", field_name: "" },
    ]);
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
