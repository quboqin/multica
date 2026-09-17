// @vitest-environment node

import { describe, expect, it } from "vitest";
import {
  CollectionDetailSchema,
  CollectionRecordPageSchema,
} from "./schemas";

describe("collection wire schemas", () => {
  it("maps snake-case detail and preserves falsy record values", () => {
    const detail = CollectionDetailSchema.parse({
      collection: {
        id: "collection-1",
        workspace_id: "ws-1",
        name: "Orders",
        revision: 1,
        archived_at: null,
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-01T00:00:00Z",
      },
      fields: [],
      capabilities: {
        layouts: ["table"],
        grouping: false,
        hierarchy: false,
        writable: true,
        max_page_size: 200,
      },
    });
    expect(detail.collection.workspaceId).toBe("ws-1");
    expect(detail.capabilities.maxPageSize).toBe(200);

    const page = CollectionRecordPageSchema.parse({
      records: [
        {
          id: "record-1",
          workspace_id: "ws-1",
          collection_id: "collection-1",
          title: "Falsy",
          fields: { text: "", amount: 0, checked: false },
          position: 0,
          revision: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-01-01T00:00:00Z",
        },
      ],
      total: 1,
      next_cursor: null,
    });
    expect(page.records[0]?.fields).toEqual({
      text: "",
      amount: 0,
      checked: false,
    });
  });

  it("rejects malformed and oversized capability payloads", () => {
    expect(() =>
      CollectionRecordPageSchema.parse({ records: "wrong" }),
    ).toThrow();
    expect(() =>
      CollectionDetailSchema.parse({
        collection: {},
        fields: [],
        capabilities: {
          layouts: ["table"],
          grouping: false,
          hierarchy: false,
          writable: true,
          max_page_size: 201,
        },
      }),
    ).toThrow();
  });
});
