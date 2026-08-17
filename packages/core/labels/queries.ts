import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";
import type { LabelResourceType } from "../types";

export const labelKeys = {
  all: (wsId: string) => ["labels", wsId] as const,
  list: (wsId: string, resourceType: LabelResourceType = "issue") =>
    [...labelKeys.all(wsId), "list", resourceType] as const,
  detail: (wsId: string, id: string) =>
    [...labelKeys.all(wsId), "detail", id] as const,
  byIssue: (wsId: string, issueId: string) =>
    [...labelKeys.all(wsId), "issue", issueId] as const,
  byProject: (wsId: string, projectId: string) =>
    [...labelKeys.all(wsId), "project", projectId] as const,
  byAgent: (wsId: string, agentId: string) =>
    [...labelKeys.all(wsId), "agent", agentId] as const,
};

export function labelListOptions(wsId: string, resourceType: LabelResourceType = "issue") {
  return queryOptions({
    queryKey: labelKeys.list(wsId, resourceType),
    queryFn: () => api.listLabels({ resource_type: resourceType }),
    select: (data) => data.labels,
  });
}

export function issueLabelsOptions(wsId: string, issueId: string) {
  return queryOptions({
    queryKey: labelKeys.byIssue(wsId, issueId),
    queryFn: () => api.listLabelsForIssue(issueId),
    select: (data) => data.labels,
    enabled: Boolean(issueId),
  });
}

export function projectLabelsOptions(wsId: string, projectId: string) {
  return queryOptions({
    queryKey: labelKeys.byProject(wsId, projectId),
    queryFn: () => api.listLabelsForProject(projectId),
    select: (data) => data.labels,
    enabled: Boolean(projectId),
  });
}

export function agentLabelsOptions(wsId: string, agentId: string) {
  return queryOptions({
    queryKey: labelKeys.byAgent(wsId, agentId),
    queryFn: () => api.listLabelsForAgent(agentId),
    select: (data) => data.labels,
    enabled: Boolean(agentId),
  });
}
