import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const projectKeys = {
  all: (wsId: string) => ["projects", wsId] as const,
  list: (wsId: string, params?: ProjectListParams) =>
    [...projectKeys.all(wsId), "list", params ?? {}] as const,
  detail: (wsId: string, id: string) =>
    [...projectKeys.all(wsId), "detail", id] as const,
};

export interface ProjectListParams {
  status?: string;
  priority?: string;
  milestone_id?: string | null;
}

export function projectListOptions(wsId: string, params?: ProjectListParams) {
  return queryOptions({
    queryKey: projectKeys.list(wsId, params),
    queryFn: () => api.listProjects(params),
    select: (data) => data.projects,
  });
}

export function projectDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: projectKeys.detail(wsId, id),
    queryFn: () => api.getProject(id),
  });
}
