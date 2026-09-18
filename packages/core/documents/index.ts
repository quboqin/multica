import { queryOptions } from "@tanstack/react-query";
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { api } from "../api";
import { defaultStorage } from "../platform/storage";
import type { Issue } from "../types";

export const documentKeys = {
  all: (wsId: string) => ["documents", wsId] as const,
  tree: (wsId: string, projectId?: string) =>
    ["documents", wsId, projectId ?? null] as const,
};
export function documentTreeOptions(wsId: string, projectId?: string) {
  return queryOptions({
    queryKey: documentKeys.tree(wsId, projectId),
    queryFn: ({ signal }) =>
      api.listDocuments(projectId, { workspaceId: wsId, signal }),
    enabled: !!wsId,
    refetchInterval: 15000,
  });
}

export function documentPath(documents: readonly Issue[], id: string): Issue[] {
  const byId = new Map(documents.map((doc) => [doc.id, doc]));
  const path: Issue[] = [];
  const visited = new Set<string>();
  for (
    let node = byId.get(id);
    node && !visited.has(node.id);
    node = node.parent_issue_id ? byId.get(node.parent_issue_id) : undefined
  ) {
    visited.add(node.id);
    path.unshift(node);
  }
  return path;
}

export interface DocumentDraft {
  body: string;
  base: string;
  version: number;
  attachmentIds: string[];
}
interface DocumentPreferences {
  drafts: Record<string, DocumentDraft>;
  favorites: Record<string, string[]>;
  recent: Record<string, string[]>;
  setDraft: (key: string, draft: DocumentDraft | null) => void;
  toggleFavorite: (wsId: string, id: string) => void;
  visit: (wsId: string, id: string) => void;
}
export const useDocumentPreferences = create<DocumentPreferences>()(
  persist(
    (set) => ({
      drafts: {},
      favorites: {},
      recent: {},
      setDraft: (key, draft) =>
        set((state) => {
          const drafts = { ...state.drafts };
          if (draft) drafts[key] = draft;
          else delete drafts[key];
          return { drafts };
        }),
      toggleFavorite: (wsId, id) =>
        set((state) => {
          const previous = state.favorites[wsId] ?? [];
          return {
            favorites: {
              ...state.favorites,
              [wsId]: previous.includes(id)
                ? previous.filter((value) => value !== id)
                : [...previous, id],
            },
          };
        }),
      visit: (wsId, id) =>
        set((state) => ({
          recent: {
            ...state.recent,
            [wsId]: [
              id,
              ...(state.recent[wsId] ?? []).filter((value) => value !== id),
            ].slice(0, 30),
          },
        })),
    }),
    {
      name: "cortex-document-preferences",
      storage: createJSONStorage(() => defaultStorage),
    },
  ),
);
