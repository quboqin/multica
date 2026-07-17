import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const credentialKeys = {
  all: (wsId: string) => ["credential", wsId] as const,
  connectors: (wsId: string) => [...credentialKeys.all(wsId), "connectors"] as const,
  profiles: (wsId: string) => [...credentialKeys.all(wsId), "profiles"] as const,
};

export const credentialConnectorsOptions = (wsId: string) =>
  queryOptions({
    queryKey: credentialKeys.connectors(wsId),
    queryFn: () => api.listCredentialConnectors(),
    enabled: !!wsId,
  });

export const credentialProfilesOptions = (wsId: string) =>
  queryOptions({
    queryKey: credentialKeys.profiles(wsId),
    queryFn: () => api.listCredentialProfiles(),
    enabled: !!wsId,
  });
