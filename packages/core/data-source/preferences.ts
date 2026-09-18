import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { defaultStorage } from "../platform/storage";

export interface DataViewPreferences {
  layout: "table" | "calendar" | "gallery";
  dateField: string;
  period: "month" | "week";
  coverField: string;
  displayedFields: string[];
  groupBy: string;
}
export const defaultDataViewPreferences: DataViewPreferences = {
  layout: "table",
  dateField: "",
  period: "month",
  coverField: "",
  displayedFields: [],
  groupBy: "",
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

export function parseDataViewPreferences(
  display: Record<string, unknown>,
): DataViewPreferences {
  return {
    layout:
      display.layout === "calendar" || display.layout === "gallery"
        ? display.layout
        : "table",
    period: display.period === "week" ? "week" : "month",
    dateField: typeof display.dateField === "string" ? display.dateField : "",
    coverField:
      typeof display.coverField === "string" ? display.coverField : "",
    displayedFields: Array.isArray(display.displayedFields)
      ? display.displayedFields.filter(
          (v): v is string => typeof v === "string",
        )
      : [],
    groupBy: typeof display.groupBy === "string" ? display.groupBy : "",
  };
}
