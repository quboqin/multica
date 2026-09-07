import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { defaultStorage } from "../platform/storage";

type ConfigChange = { value: unknown } | { remove: true };
export type CreativeResourceChanges = Record<string, ConfigChange>;
export const EMPTY_CREATIVE_RESOURCE_CHANGES: CreativeResourceChanges = {};
export const creativeResourceDraftKey = (workspaceId: string, resourceId: string) => `${workspaceId}:${resourceId}`;

export function applyCreativeResourceChanges(config: Record<string, unknown>, changes: CreativeResourceChanges): Record<string, unknown> {
  const result = { ...config };
  for (const [key, change] of Object.entries(changes)) {
    if ("remove" in change) delete result[key];
    else result[key] = change.value;
  }
  return result;
}

export function hasCreativeResourceChanges(config: Record<string, unknown>, changes: CreativeResourceChanges): boolean {
  return Object.entries(changes).some(([key, change]) => "remove" in change
    ? key in config : JSON.stringify(config[key]) !== JSON.stringify(change.value));
}

interface ResourceDraftState {
  changes: Record<string, CreativeResourceChanges>;
  update: (key: string, before: Record<string, unknown>, after: Record<string, unknown>) => void;
  clear: (key: string, submitted?: CreativeResourceChanges) => void;
}

export const useCreativeResourceDraftStore = create<ResourceDraftState>()(persist((set) => ({
  changes: {},
  update: (key, before, after) => set((state) => {
    const changes = { ...state.changes[key] };
    for (const field of new Set([...Object.keys(before), ...Object.keys(after)])) {
      if (Object.is(before[field], after[field])) continue;
      changes[field] = field in after ? { value: after[field] } : { remove: true };
    }
    return { changes: { ...state.changes, [key]: changes } };
  }),
  clear: (key, submitted) => set((state) => {
    const changes = { ...state.changes };
    const remaining = Object.fromEntries(Object.entries(changes[key] ?? {}).filter(([field, change]) => submitted && change !== submitted[field]));
    if (Object.keys(remaining).length) changes[key] = remaining;
    else delete changes[key];
    return { changes };
  }),
}), { name: "multica_creative_resource_changes", storage: createJSONStorage(() => defaultStorage), partialize: (state) => ({ changes: state.changes }) }));
