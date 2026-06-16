/**
 * Thin wrapper around persistent storage for the auth token.
 * Keyed identically to web/desktop ("multica_token") so logic stays aligned
 * with packages/core/auth/store.ts even though storage backends differ.
 */
import {
  deletePersistentItem,
  getPersistentItem,
  setPersistentItem,
} from "@/lib/persistent-storage";

const TOKEN_KEY = "multica_token";

export async function getToken(): Promise<string | null> {
  return getPersistentItem(TOKEN_KEY);
}

export async function setToken(token: string): Promise<void> {
  await setPersistentItem(TOKEN_KEY, token);
}

export async function clearToken(): Promise<void> {
  await deletePersistentItem(TOKEN_KEY);
}
