import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const previewSessionKeys = {
  all: (wsId: string) => ["preview-sessions", wsId] as const,
  issue: (wsId: string, issueId: string) =>
    [...previewSessionKeys.all(wsId), "issue", issueId] as const,
  detail: (wsId: string, sessionId: string) =>
    [...previewSessionKeys.all(wsId), "detail", sessionId] as const,
};

export function previewSessionListOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: previewSessionKeys.issue(wsId, issueId),
    queryFn: () => api.listPreviewSessions(issueId),
    select: (data) => data.previewSessions,
  });
}

export function previewSessionDetailOptions(wsId: string, sessionId: string) {
  return queryOptions({
    queryKey: previewSessionKeys.detail(wsId, sessionId),
    queryFn: () => api.getPreviewSession(sessionId),
  });
}
