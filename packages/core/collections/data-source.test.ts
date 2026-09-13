// @vitest-environment node

import { describe, expect, it, vi } from "vitest";
import type { CollectionDetail, CollectionRecord } from "../types";
import { createCollectionRecordDataSource } from "./data-source";

const record: CollectionRecord = {
  id: "record-1",
  workspaceId: "ws-1",
  collectionId: "collection-1",
  title: "Original",
  fields: { "field-1": "note" },
  position: 0,
  revision: 1,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
};

const detail: CollectionDetail = {
  collection: {
    id: "collection-1",
    workspaceId: "ws-1",
    name: "Orders",
    revision: 1,
    archivedAt: null,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
  fields: [
    {
      id: "field-1",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Note",
      type: "text",
      position: 0,
      revision: 1,
    },
  ],
  capabilities: {
    layouts: ["table"],
    grouping: false,
    hierarchy: false,
    writable: true,
    maxPageSize: 200,
  },
};

describe("createCollectionRecordDataSource", () => {
  it("isolates identity, maps reads, and awaits accepted title updates", async () => {
    const read = vi.fn(async () => ({
      records: [record],
      total: 1,
      nextCursor: "next",
    }));
    const execute = vi.fn(async () => ({
      ...record,
      title: "Changed",
      revision: 2,
    }));
    const source = createCollectionRecordDataSource({ detail, read, execute });

    expect(source.identity).toEqual({
      workspaceId: "ws-1",
      namespace: "collection",
      sourceId: "collection-1",
    });
    await expect(
      source.read({ version: 1, sort: "created_at_asc" }, { limit: 25 }),
    ).resolves.toMatchObject({ rows: [record], total: 1, nextCursor: "next" });
    await expect(
      source.execute({
        row: record,
        fieldId: "title",
        change: { op: "set", value: "Changed" },
      }),
    ).resolves.toMatchObject({
      status: "accepted",
      row: { title: "Changed", revision: 2 },
    });
    expect(execute).toHaveBeenCalledOnce();
  });

  it("keeps T2a custom fields and missing-executor sources read-only", async () => {
    const execute = vi.fn();
    const source = createCollectionRecordDataSource({
      detail,
      read: async () => ({ records: [], total: 0, nextCursor: null }),
      execute,
    });
    const customField = source.fields.find((field) => field.id === "field:field-1");
    expect(customField?.canSet(record)).toBe(false);
    await expect(
      source.execute({
        row: record,
        fieldId: "field:field-1",
        change: { op: "set", value: "changed" },
      }),
    ).resolves.toMatchObject({ status: "failed" });
    expect(execute).not.toHaveBeenCalled();

    const readonly = createCollectionRecordDataSource({
      detail,
      read: async () => ({ records: [], total: 0, nextCursor: null }),
    });
    expect(readonly.capabilities.writable).toBe(false);
    await expect(
      readonly.execute({
        row: record,
        fieldId: "title",
        change: { op: "set", value: "changed" },
      }),
    ).resolves.toMatchObject({ status: "failed" });
  });

  it("turns executor rejection into a final failed result", async () => {
    const source = createCollectionRecordDataSource({
      detail,
      read: async () => ({ records: [], total: 0, nextCursor: null }),
      execute: async () => {
        throw new Error("revision conflict");
      },
    });
    await expect(
      source.execute({
        row: record,
        fieldId: "title",
        change: { op: "set", value: "changed" },
      }),
    ).resolves.toMatchObject({
      status: "failed",
      error: expect.objectContaining({ message: "revision conflict" }),
    });
  });
});
