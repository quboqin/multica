// @vitest-environment node
import { expect, it } from "vitest";
import { calendarDays } from "./calendar";
import { parseDataViewPreferences } from "./preferences";
it("uses complete month/week grids across year and leap-day boundaries", () => {
  const month = calendarDays("2024-02-29", "month");
  expect(month).toHaveLength(42);
  expect(month).toContain("2024-02-29");
  expect(new Set(month).size).toBe(42);
  const week = calendarDays("2026-01-01", "week");
  expect(week).toHaveLength(7);
  expect(week).toContain("2025-12-31");
  expect(week).toContain("2026-01-01");
});
it("restores supported view options and sanitizes malformed saved display fields", () => {
  expect(
    parseDataViewPreferences({
      layout: "gallery",
      period: "week",
      dateField: "date",
      displayedFields: ["a", 42],
    }),
  ).toMatchObject({
    layout: "gallery",
    period: "week",
    dateField: "date",
    displayedFields: ["a"],
  });
  expect(
    parseDataViewPreferences({ layout: "future", displayedFields: null }),
  ).toMatchObject({ layout: "table", displayedFields: [] });
  expect(
    parseDataViewPreferences({
      layout: "board",
      hiddenFields: ["a", 1],
      sortBy: "f",
      sortDir: "desc",
      filters: [{ field: "f", op: "exact", value: "x" }, { field: 3 }, null],
    }),
  ).toMatchObject({
    layout: "board",
    hiddenFields: ["a"],
    sortBy: "f",
    sortDir: "desc",
    filters: [{ field: "f", op: "exact", value: "x" }],
  });
});
