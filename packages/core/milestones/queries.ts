import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const milestoneKeys = {
  all: (wsId: string) => ["milestones", wsId] as const,
  list: (wsId: string) => [...milestoneKeys.all(wsId), "list"] as const,
  detail: (wsId: string, id: string) =>
    [...milestoneKeys.all(wsId), "detail", id] as const,
};

export function milestoneListOptions(wsId: string) {
  return queryOptions({
    queryKey: milestoneKeys.list(wsId),
    queryFn: () => api.listMilestones(),
    select: (data) => data.milestones,
  });
}

export function milestoneDetailOptions(wsId: string, id: string) {
  return queryOptions({
    queryKey: milestoneKeys.detail(wsId, id),
    queryFn: () => api.getMilestone(id),
  });
}
