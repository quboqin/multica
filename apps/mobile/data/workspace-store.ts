/**
 * Mobile workspace store — Zustand. Holds the active workspace (id + slug)
 * and persists the slug so cold starts restore the last
 * selection without re-prompting.
 *
 * The route is the source of truth for which workspace is active
 * (`/[workspace]/...` URL segment, set by the layout that reads
 * useLocalSearchParams). This store is a fast cache that ApiClient.fetch
 * reads synchronously to inject the X-Workspace-Slug header — touching
 * the router or React context on every fetch would be ugly. Routes
 * sync into the store on mount via setCurrentWorkspace.
 *
 * Logic mirrors packages/core/platform/workspace-storage.ts:
 *   - One slug + one id at a time (no multi-ws state)
 *   - Cleared on logout
 */
import { create } from "zustand";
import {
  deletePersistentItem,
  getPersistentItem,
  setPersistentItem,
} from "@/lib/persistent-storage";

const SLUG_KEY = "multica_current_workspace_slug";

interface WorkspaceState {
  currentWorkspaceId: string | null;
  currentWorkspaceSlug: string | null;
  /** Set the active workspace and persist the slug (id is in-memory only —
   *  it's resolved from the workspaces list query, not stored). */
  setCurrentWorkspace: (id: string, slug: string) => Promise<void>;
  /** Restore the slug from SecureStore on cold start. id stays null until
   *  the workspaces list query resolves. */
  restoreSlug: () => Promise<string | null>;
  clear: () => Promise<void>;
}

export const useWorkspaceStore = create<WorkspaceState>((set) => ({
  currentWorkspaceId: null,
  currentWorkspaceSlug: null,

  setCurrentWorkspace: async (id, slug) => {
    set({ currentWorkspaceId: id, currentWorkspaceSlug: slug });
    await setPersistentItem(SLUG_KEY, slug);
  },

  restoreSlug: async () => {
    const slug = await getPersistentItem(SLUG_KEY);
    if (slug) set({ currentWorkspaceSlug: slug });
    return slug;
  },

  clear: async () => {
    set({ currentWorkspaceId: null, currentWorkspaceSlug: null });
    await deletePersistentItem(SLUG_KEY);
  },
}));

/** Sync helper for ApiClient.fetch — reads the current slug without React. */
export function getCurrentSlug(): string | null {
  return useWorkspaceStore.getState().currentWorkspaceSlug;
}
