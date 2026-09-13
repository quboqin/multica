import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import {
  assertWorkspaceRequestContext,
  captureClientSessionGeneration,
  isClientSessionGenerationCurrent,
  type WorkspaceRequestContext,
} from "../platform";
import type {
  CollectionRecord,
  CreateCollectionInput,
  CreateCollectionRecordInput,
  UpdateCollectionRecordInput,
  Workspace,
} from "../types";
import { workspaceKeys } from "../workspace/queries";
import { collectionKeys } from "./queries";

type CollectionMutationContext = { sessionGeneration: number };

function canCoordinateCollectionCache(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceContext: WorkspaceRequestContext,
  context: CollectionMutationContext | undefined,
): boolean {
  if (
    !context ||
    !isClientSessionGenerationCurrent(queryClient, context.sessionGeneration)
  ) {
    return false;
  }
  const workspaces = queryClient.getQueryData<Workspace[]>(workspaceKeys.list());
  return (
    workspaces?.some(
      (workspace) =>
        workspace.id === workspaceContext.workspaceId &&
        workspace.slug === workspaceContext.workspaceSlug,
    ) ?? false
  );
}

function captureMutationContext(
  queryClient: ReturnType<typeof useQueryClient>,
): CollectionMutationContext {
  return { sessionGeneration: captureClientSessionGeneration(queryClient) };
}

export function useCreateCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    onMutate: () => captureMutationContext(queryClient),
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
    onSuccess: (_result, { workspaceContext }, context) => {
      if (!canCoordinateCollectionCache(queryClient, workspaceContext, context)) {
        return;
      }
      void queryClient.invalidateQueries({
        queryKey: collectionKeys.all(workspaceContext.workspaceId),
      });
    },
  });
}
export function useCreateCollectionRecord() {
  const queryClient = useQueryClient();
  return useMutation({
    onMutate: () => captureMutationContext(queryClient),
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
    onSuccess: ({ record }, { collectionId, workspaceContext }, context) => {
      if (!canCoordinateCollectionCache(queryClient, workspaceContext, context)) {
        return;
      }
      queryClient.setQueryData(
        collectionKeys.record(
          workspaceContext.workspaceId,
          collectionId,
          record.id,
        ),
        (current: CollectionRecord | undefined) =>
          !current || record.revision >= current.revision ? record : current,
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
    onMutate: () => captureMutationContext(queryClient),
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
    onSuccess: (record, { collectionId, workspaceContext }, context) => {
      if (!canCoordinateCollectionCache(queryClient, workspaceContext, context)) {
        return;
      }
      const detailKey = collectionKeys.record(
        workspaceContext.workspaceId,
        collectionId,
        record.id,
      );
      queryClient.setQueryData<CollectionRecord>(detailKey, (current) =>
        !current || record.revision >= current.revision ? record : current,
      );
    },
    onSettled: (
      _record,
      _error,
      { collectionId, workspaceContext },
      context,
    ) => {
      if (!canCoordinateCollectionCache(queryClient, workspaceContext, context)) {
        return;
      }
      void queryClient.invalidateQueries({
        queryKey: collectionKeys.rows(
          workspaceContext.workspaceId,
          collectionId,
        ),
      });
    },
  });
}
