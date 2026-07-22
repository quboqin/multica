import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const creativeKeys = {
  all: (wsId: string) => ["creative", wsId] as const,
  issue: (wsId: string, issueId: string) =>
    [...creativeKeys.all(wsId), "issue", issueId] as const,
};

export const creativeMaterialsOptions = (wsId: string, issueId: string) =>
  queryOptions({
    queryKey: creativeKeys.issue(wsId, issueId),
    queryFn: () => api.getCreativeMaterials(issueId),
    enabled: !!wsId && !!issueId,
  });
