import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import {
  assertWorkspaceRequestContext,
  type WorkspaceRequestContext,
} from "../platform";
import type {
  CollectionRecord,
  CreateCollectionInput,
  CreateCollectionRecordInput,
  UpdateCollectionRecordInput,
} from "../types";
import { collectionKeys } from "./queries";

export function useCreateCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      input,
      workspaceContext,
    }: {
      input: CreateCollectionInput;
      workspaceContext: WorkspaceRequestContext;
    }) => {
      assertWorkspaceRequestContext(workspaceContext);
      return api.createCollection(input, workspaceContext.workspaceSlug);
    },
    onSuccess: (_result, { workspaceContext }) => {
      void queryClient.invalidateQueries({
        queryKey: collectionKeys.all(workspaceContext.workspaceId),
      });
    },
  });
}
export function useCreateCollectionRecord() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      collectionId,
      input,
      workspaceContext,
    }: {
      collectionId: string;
      input: CreateCollectionRecordInput;
      workspaceContext: WorkspaceRequestContext;
    }) => {
      assertWorkspaceRequestContext(workspaceContext);
      return api.createCollectionRecord(
        collectionId,
        input,
        workspaceContext.workspaceSlug,
      );
    },
    onSuccess: ({ record }, { collectionId, workspaceContext }) => {
      queryClient.setQueryData(
        collectionKeys.record(
          workspaceContext.workspaceId,
          collectionId,
          record.id,
        ),
        record,
      );
      void queryClient.invalidateQueries({
        queryKey: collectionKeys.rows(
          workspaceContext.workspaceId,
          collectionId,
        ),
      });
    },
  });
}

export function useUpdateCollectionRecord() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({
      collectionId,
      recordId,
      input,
      workspaceContext,
    }: {
      collectionId: string;
      recordId: string;
      input: UpdateCollectionRecordInput;
      workspaceContext: WorkspaceRequestContext;
    }) => {
      assertWorkspaceRequestContext(workspaceContext);
      return api.updateCollectionRecord(
        collectionId,
        recordId,
        input,
        workspaceContext.workspaceSlug,
      );
    },
    onSuccess: (record, { collectionId, workspaceContext }) => {
      const detailKey = collectionKeys.record(
        workspaceContext.workspaceId,
        collectionId,
        record.id,
      );
      queryClient.setQueryData<CollectionRecord>(detailKey, (current) =>
        !current || record.revision >= current.revision ? record : current,
      );
    },
    onSettled: (_record, _error, { collectionId, workspaceContext }) => {
      void queryClient.invalidateQueries({
        queryKey: collectionKeys.rows(
          workspaceContext.workspaceId,
          collectionId,
        ),
      });
    },
  });
}
