import { useQuery } from "@tanstack/react-query";
import type { Squad } from "../types";
import { squadListOptions, workspaceCapabilitiesOptions } from "../workspace/queries";

export function resolveCreativeSquad<T extends Pick<Squad, "id" | "name"> & Partial<Pick<Squad, "archived_at">>>(
  squads: readonly T[],
  factorySquadId?: string,
): T | undefined {
  const active = squads.filter((squad) => !squad.archived_at);
  if (factorySquadId) return active.find((squad) => squad.id === factorySquadId);
  const materialSquads = active.filter((squad) => squad.name.includes("素材"));
  if (materialSquads.length === 1) return materialSquads[0];
  return active.length === 1 ? active[0] : undefined;
}

export function useCreativeSquad(wsId: string) {
  const squads = useQuery(squadListOptions(wsId));
  const capabilities = useQuery(workspaceCapabilitiesOptions(wsId));
  const isLoading = squads.isLoading || capabilities.isLoading;
  const availableSquads = (squads.data ?? []).filter((squad) => !squad.archived_at);
  return {
    squads: availableSquads,
    selectedSquad: isLoading || capabilities.isError
      ? undefined
      : resolveCreativeSquad(availableSquads, capabilities.data?.creative_factory_squad_id),
    isLoading,
  };
}
