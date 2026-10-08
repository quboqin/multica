// Calendar arithmetic uses local calendar dates, never UTC conversion, so
// dragging around a daylight-saving transition does not shift the chosen day.
export function calendarDate(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}
export function calendarDays(
  anchor: string,
  period: "month" | "week",
): string[] {
  const [year = 1970, month = 1, day = 1] = anchor.split("-").map(Number);
  const start = new Date(year, month - 1, period === "month" ? 1 : day, 12);
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7));
  return Array.from({ length: period === "month" ? 42 : 7 }, (_, index) => {
    const date = new Date(start);
    date.setDate(date.getDate() + index);
    return calendarDate(date);
  });
}
