import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

export const workspaceMCPKeys = {
  all: (wsId: string) => ["workspace-mcp", wsId] as const,
  connections: (wsId: string) => [...workspaceMCPKeys.all(wsId), "connections"] as const,
};

export const workspaceMCPConnectionsOptions = (wsId: string) =>
  queryOptions({
    queryKey: workspaceMCPKeys.connections(wsId),
    queryFn: () => api.listWorkspaceMCPConnections(),
    enabled: !!wsId,
  });
