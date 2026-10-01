// @vitest-environment node
import { describe, expect, it } from "vitest";
import { CollectionDetailSchema, CollectionPageSchema, ResourceAccessSchema } from "./resource-schemas";

describe("mobile resource responses", () => {
  it("keeps permission booleans conservative when a newer backend omits them", () => {
    const access = ResourceAccessSchema.parse({
      owner_id: "owner", scope: "workspace", project_id: null, revision: 2,
    });
    expect(access.can_edit).toBe(false);
    expect(access.can_manage).toBe(false);
  });

  it("parses collection detail and paginated records while tolerating extra server fields", () => {
    const collection = { id: "c", workspace_id: "w", name: "Notes" };
    const detail = CollectionDetailSchema.parse({
      collection, access: null,
      fields: [{ id: "f", name: "Status", type: "select", position: 0, config: { options: [] } }],
      capabilities: { layouts: ["table"] },
    });
    expect(detail.collection.name).toBe("Notes");
    expect(detail.fields[0].type).toBe("select");
    const page = CollectionPageSchema.parse({
      records: [{ id: "r", collection_id: "c", title: "One", fields: {}, revision: 1 }],
      total: 1, next_cursor: null, groups: [],
    });
    expect(page.records).toHaveLength(1);
  });
});
