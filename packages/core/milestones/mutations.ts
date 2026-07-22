import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import type { CreateMilestoneRequest, ListMilestonesResponse, Milestone, UpdateMilestoneRequest } from "../types";
import { milestoneKeys } from "./queries";

export function useCreateMilestone() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: (data: CreateMilestoneRequest) => api.createMilestone(data),
    onSuccess: (newMilestone) => {
      qc.setQueryData<ListMilestonesResponse>(milestoneKeys.list(wsId), (old) =>
        old && !old.milestones.some((m) => m.id === newMilestone.id)
          ? { ...old, milestones: [...old.milestones, newMilestone], total: old.total + 1 }
          : old,
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: milestoneKeys.list(wsId) });
    },
  });
}

export function useUpdateMilestone() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string } & UpdateMilestoneRequest) =>
      api.updateMilestone(id, data),
    onMutate: async ({ id, ...data }) => {
      await qc.cancelQueries({ queryKey: milestoneKeys.list(wsId) });
      const prevList = qc.getQueryData<ListMilestonesResponse>(milestoneKeys.list(wsId));
      const prevDetail = qc.getQueryData<Milestone>(milestoneKeys.detail(wsId, id));
      qc.setQueryData<ListMilestonesResponse>(milestoneKeys.list(wsId), (old) =>
        old ? { ...old, milestones: old.milestones.map((m) => (m.id === id ? { ...m, ...data } : m)) } : old,
      );
      qc.setQueryData<Milestone>(milestoneKeys.detail(wsId, id), (old) =>
        old ? { ...old, ...data } : old,
      );
      return { prevList, prevDetail, id };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prevList) qc.setQueryData(milestoneKeys.list(wsId), ctx.prevList);
      if (ctx?.prevDetail) qc.setQueryData(milestoneKeys.detail(wsId, ctx.id), ctx.prevDetail);
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: milestoneKeys.detail(wsId, vars.id) });
      qc.invalidateQueries({ queryKey: milestoneKeys.list(wsId) });
    },
  });
}
