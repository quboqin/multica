import type { QueryClient, QueryKey } from "@tanstack/react-query";

/** A server event invalidates the read already in flight as well as cached data. */
export async function refreshDataSource(
  client: QueryClient,
  queryKey: QueryKey,
): Promise<void> {
  await client.cancelQueries({ queryKey });
  await client.invalidateQueries({ queryKey });
}
