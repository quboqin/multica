import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import type {
  CreatePreviewSessionRequest,
  PreviewSession,
  PreviewSessionListResponse,
} from "../types";
import { previewSessionKeys } from "./queries";

export function useCreatePreviewSession(wsId: string, issueId: string) {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreatePreviewSessionRequest) =>
      api.createPreviewSession(issueId, data),
    onSuccess: (session) => {
      queryClient.setQueryData<PreviewSessionListResponse>(
        previewSessionKeys.issue(wsId, issueId),
        (old) => {
          if (!old || old.previewSessions.some((item) => item.id === session.id)) {
            return old;
          }
          return {
            previewSessions: [session, ...old.previewSessions],
            total: old.total + 1,
          };
        },
      );
      queryClient.setQueryData(
        previewSessionKeys.detail(wsId, session.id),
        session,
      );
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: previewSessionKeys.issue(wsId, issueId),
      });
    },
  });
}

export function useStopPreviewSession(wsId: string, issueId: string) {
  const queryClient = useQueryClient();
  const listKey = previewSessionKeys.issue(wsId, issueId);

  return useMutation({
    mutationFn: (sessionId: string) => api.stopPreviewSession(sessionId),
    onMutate: async (sessionId) => {
      await queryClient.cancelQueries({ queryKey: listKey });
      const previous = queryClient.getQueryData<PreviewSessionListResponse>(listKey);
      const previousDetail = queryClient.getQueryData<PreviewSession>(
        previewSessionKeys.detail(wsId, sessionId),
      );
      const patchStatus = (session: PreviewSession): PreviewSession =>
        session.id === sessionId ? { ...session, status: "stopping" } : session;

      queryClient.setQueryData<PreviewSessionListResponse>(listKey, (old) =>
        old
          ? { ...old, previewSessions: old.previewSessions.map(patchStatus) }
          : old,
      );
      queryClient.setQueryData<PreviewSession>(
        previewSessionKeys.detail(wsId, sessionId),
        (old) => (old ? patchStatus(old) : old),
      );
      return { previous, previousDetail, sessionId };
    },
    onError: (_error, _sessionId, context) => {
      if (context?.previous) {
        queryClient.setQueryData(listKey, context.previous);
      }
      if (context?.previousDetail) {
        queryClient.setQueryData(
          previewSessionKeys.detail(wsId, context.sessionId),
          context.previousDetail,
        );
      }
    },
    onSuccess: (session) => {
      queryClient.setQueryData<PreviewSessionListResponse>(listKey, (old) =>
        old
          ? {
              ...old,
              previewSessions: old.previewSessions.map((item) =>
                item.id === session.id ? session : item,
              ),
            }
          : old,
      );
      queryClient.setQueryData(
        previewSessionKeys.detail(wsId, session.id),
        session,
      );
    },
    onSettled: (_data, _error, sessionId) => {
      queryClient.invalidateQueries({ queryKey: listKey });
      queryClient.invalidateQueries({
        queryKey: previewSessionKeys.detail(wsId, sessionId),
      });
    },
  });
}
