import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setApiInstance } from "../api";
import type { ApiClient } from "../api/client";
import { revokeClientWorkspaceAccess } from "../platform";
import {
  collectionDetailOptions,
  collectionListOptions,
  collectionRecordOptions,
} from "./queries";

describe("collection query authorization fence", () => {
  afterEach(() => vi.restoreAllMocks());

  it("rejects new list, detail, and record requests after realtime revocation", async () => {
    const listCollections = vi.fn();
    const getCollection = vi.fn();
    const getCollectionRecord = vi.fn();
    setApiInstance({
      listCollections,
      getCollection,
      getCollectionRecord,
    } as unknown as ApiClient);
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    revokeClientWorkspaceAccess(queryClient, "ws-1");

    await expect(
      queryClient.fetchQuery(collectionListOptions("ws-1", "alpha")),
    ).rejects.toThrow("Workspace access was revoked");
    await expect(
      queryClient.fetchQuery(
        collectionDetailOptions("ws-1", "alpha", "collection-1"),
      ),
    ).rejects.toThrow("Workspace access was revoked");
    await expect(
      queryClient.fetchQuery(
        collectionRecordOptions(
          "ws-1",
          "alpha",
          "collection-1",
          "record-1",
        ),
      ),
    ).rejects.toThrow("Workspace access was revoked");
    expect(listCollections).not.toHaveBeenCalled();
    expect(getCollection).not.toHaveBeenCalled();
    expect(getCollectionRecord).not.toHaveBeenCalled();
  });
});
