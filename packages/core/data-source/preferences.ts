import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { defaultStorage } from "../platform/storage";

export type DataViewLayout = "table" | "board" | "calendar" | "gallery";
export interface DataViewFilter {
  field: string;
  op: string;
  value: unknown;
}
export interface DataViewPreferences {
  layout: DataViewLayout;
  dateField: string;
  period: "month" | "week";
  coverField: string;
  /** Fields shown on board and gallery cards. */
  displayedFields: string[];
  /** Table columns hidden in this view. */
  hiddenFields: string[];
  groupBy: string;
  sortBy: string;
  sortDir: "asc" | "desc";
  filters: DataViewFilter[];
}
export const defaultDataViewPreferences: DataViewPreferences = {
  layout: "table",
  dateField: "",
  period: "month",
  coverField: "",
  displayedFields: [],
  hiddenFields: [],
  groupBy: "",
  sortBy: "",
  sortDir: "asc",
  filters: [],
};
export const useDataViewPreferences = create<{
  bySource: Record<string, DataViewPreferences>;
  update: (source: string, patch: Partial<DataViewPreferences>) => void;
}>()(
  persist(
    (set) => ({
      bySource: {},
      update: (source, patch) =>
        set((state) => ({
          bySource: {
            ...state.bySource,
            [source]: {
              ...(state.bySource[source] ?? defaultDataViewPreferences),
              ...patch,
            },
          },
        })),
    }),
    {
      name: "data-view-preferences",
      storage: createJSONStorage(() => defaultStorage),
    },
  ),
);

const strings = (value: unknown): string[] =>
  Array.isArray(value)
    ? value.filter((v): v is string => typeof v === "string")
    : [];

export function parseDataViewPreferences(
  display: Record<string, unknown>,
): DataViewPreferences {
  return {
    layout:
      display.layout === "calendar" ||
      display.layout === "gallery" ||
      display.layout === "board"
        ? display.layout
        : "table",
    period: display.period === "week" ? "week" : "month",
    dateField: typeof display.dateField === "string" ? display.dateField : "",
    coverField:
      typeof display.coverField === "string" ? display.coverField : "",
    displayedFields: strings(display.displayedFields),
    hiddenFields: strings(display.hiddenFields),
    groupBy: typeof display.groupBy === "string" ? display.groupBy : "",
    sortBy: typeof display.sortBy === "string" ? display.sortBy : "",
    sortDir: display.sortDir === "desc" ? "desc" : "asc",
    filters: Array.isArray(display.filters)
      ? display.filters.flatMap((item: unknown) => {
          if (!item || typeof item !== "object") return [];
          const { field, op, value } = item as Record<string, unknown>;
          return typeof field === "string" && typeof op === "string"
            ? [{ field, op, value }]
            : [];
        })
      : [],
  };
}
