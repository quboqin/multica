import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import {
  assertClientWorkspaceAccessAllowed,
  assertWorkspaceRequestContext,
  captureClientSessionGeneration,
  captureClientWorkspaceAccessGeneration,
  isClientSessionGenerationCurrent,
  isClientWorkspaceAccessGenerationCurrent,
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

type CollectionMutationContext = {
  sessionGeneration: number;
  workspaceAccessGeneration: number;
};

function canCoordinateCollectionCache(
  queryClient: ReturnType<typeof useQueryClient>,
  workspaceContext: WorkspaceRequestContext,
  context: CollectionMutationContext | undefined,
): boolean {
  if (
    !context ||
    !isClientSessionGenerationCurrent(queryClient, context.sessionGeneration) ||
    !isClientWorkspaceAccessGenerationCurrent(
      queryClient,
      workspaceContext.workspaceId,
      context.workspaceAccessGeneration,
    )
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
  workspaceId: string,
): CollectionMutationContext {
  return {
    sessionGeneration: captureClientSessionGeneration(queryClient),
    workspaceAccessGeneration: captureClientWorkspaceAccessGeneration(
      queryClient,
      workspaceId,
    ),
  };
}

export function useCreateCollection() {
  const queryClient = useQueryClient();
  return useMutation({
    onMutate: ({ workspaceContext }) =>
      captureMutationContext(queryClient, workspaceContext.workspaceId),
    mutationFn: ({
      input,
      workspaceContext,
    }: {
      input: CreateCollectionInput;
      workspaceContext: WorkspaceRequestContext;
    }) => {
      assertClientWorkspaceAccessAllowed(
        queryClient,
        workspaceContext.workspaceId,
      );
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
    onMutate: ({ workspaceContext }) =>
      captureMutationContext(queryClient, workspaceContext.workspaceId),
    mutationFn: ({
      collectionId,
      input,
      workspaceContext,
    }: {
      collectionId: string;
      input: CreateCollectionRecordInput;
      workspaceContext: WorkspaceRequestContext;
    }) => {
      assertClientWorkspaceAccessAllowed(
        queryClient,
        workspaceContext.workspaceId,
      );
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
    onMutate: ({ workspaceContext }) =>
      captureMutationContext(queryClient, workspaceContext.workspaceId),
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
      assertClientWorkspaceAccessAllowed(
        queryClient,
        workspaceContext.workspaceId,
      );
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
      { collectionId, recordId, workspaceContext },
      context,
    ) => {
      if (!canCoordinateCollectionCache(queryClient, workspaceContext, context)) {
        return;
      }
      // Keep the paginated source and the active editor's authoritative record
      // in one completion boundary. In particular, a tail row may no longer be
      // present in the refreshed first page after a CAS conflict; awaiting the
      // focused record refetch lets the user's explicit retry adopt the latest
      // revision instead of looping on its frozen snapshot.
      return Promise.all([
        queryClient.invalidateQueries({
          queryKey: collectionKeys.rows(
            workspaceContext.workspaceId,
            collectionId,
          ),
        }),
        queryClient.invalidateQueries({
          queryKey: collectionKeys.record(
            workspaceContext.workspaceId,
            collectionId,
            recordId,
          ),
        }),
      ]);
    },
  });
}
