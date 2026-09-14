// @vitest-environment node

import { describe, expect, it, vi } from "vitest";
import type { CollectionDetail, CollectionRecord } from "../types";
import {
  createCollectionRecordDataSource,
  type CollectionRecordExecutor,
} from "./data-source";

const record: CollectionRecord = {
  id: "record-1",
  workspaceId: "ws-1",
  collectionId: "collection-1",
  title: "Original",
  fields: { "field-1": "note", "field-2": 3, "field-3": false },
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
    {
      id: "field-2",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Amount",
      type: "number",
      position: 1,
      revision: 1,
    },
    {
      id: "field-3",
      workspaceId: "ws-1",
      collectionId: "collection-1",
      name: "Done",
      type: "checkbox",
      position: 2,
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

  it("maps custom field values and delegates typed set and clear commands", async () => {
    const execute = vi.fn(async (input: Parameters<CollectionRecordExecutor>[0]) => ({
      ...input.record,
      revision: input.record.revision + 1,
    }));
    const source = createCollectionRecordDataSource({
      detail,
      read: async () => ({ records: [], total: 0, nextCursor: null }),
      execute,
    });
    expect(source.fields.map((field) => [field.id, field.kind, field.value(record)])).toEqual([
      ["title", "text", "Original"],
      ["field:field-1", "text", "note"],
      ["field:field-2", "number", 3],
      ["field:field-3", "checkbox", false],
    ]);
    for (const command of [
      { fieldId: "field:field-1", change: { op: "set" as const, value: "" } },
      { fieldId: "field:field-2", change: { op: "set" as const, value: 0 } },
      { fieldId: "field:field-3", change: { op: "set" as const, value: false } },
      { fieldId: "field:field-1", change: { op: "clear" as const } },
    ]) {
      await expect(source.execute({ row: record, ...command })).resolves.toMatchObject({
        status: "accepted",
      });
    }
    expect(execute).toHaveBeenCalledTimes(4);
    expect(source.fields[1]?.canSet(record)).toBe(true);
    expect(source.fields[1]?.canClear(record)).toBe(true);
  });

  it("rejects invalid custom values and keeps missing-executor sources read-only", async () => {
    const execute = vi.fn();
    const source = createCollectionRecordDataSource({
      detail,
      read: async () => ({ records: [], total: 0, nextCursor: null }),
      execute,
    });
    for (const command of [
      { fieldId: "field:field-1", change: { op: "set" as const, value: 42 } },
      { fieldId: "field:field-2", change: { op: "set" as const, value: Number.NaN } },
      { fieldId: "field:field-3", change: { op: "set" as const, value: "false" } },
      { fieldId: "title", change: { op: "clear" as const } },
      { fieldId: "field:unknown", change: { op: "clear" as const } },
    ]) {
      await expect(source.execute({ row: record, ...command })).resolves.toMatchObject({
        status: "failed",
      });
    }
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
