"use client";
import { calendarDays } from "@multica/core/data-source";
import { Button } from "@multica/ui/components/ui/button";
import type { DataSourceCapabilities } from "@multica/core/data-source";

export interface CalendarLabels {
  month: string;
  week: string;
  previous: string;
  next: string;
  date: string;
  unscheduled: string;
}
export function DataViewCalendar<Row>({
  rows,
  rowId,
  title,
  date,
  anchor,
  period,
  onAnchorChange,
  onPeriodChange,
  onMoveDate,
  onOpen,
  capabilities,
  labels,
}: {
  rows: readonly Row[];
  rowId: (row: Row) => string;
  title: (row: Row) => string;
  date: (row: Row) => string | null;
  anchor: string;
  period: "month" | "week";
  onAnchorChange: (date: string) => void;
  onPeriodChange: (period: "month" | "week") => void;
  onMoveDate?: (row: Row, date: string | null) => void;
  onOpen?: (row: Row) => void;
  capabilities: DataSourceCapabilities;
  labels: CalendarLabels;
}) {
  if (!capabilities.layouts.includes("calendar")) return null;
  const days = calendarDays(anchor, period);
  const editable = capabilities.editing === "adapter" && !!onMoveDate;
  const card = (row: Row) => (
    <div
      key={rowId(row)}
      className="space-y-1 rounded border bg-background p-1.5"
      draggable={editable}
      onDragStart={(event) =>
        event.dataTransfer.setData("application/x-data-view-row", rowId(row))
      }
    >
      <button
        className="w-full break-words text-left text-caption"
        onClick={() => onOpen?.(row)}
      >
        {title(row)}
      </button>
      {editable && (
        <input
          aria-label={`${labels.date}: ${title(row)}`}
          className="w-full bg-transparent text-caption"
          type="date"
          value={date(row)?.slice(0, 10) ?? ""}
          onChange={(event) => onMoveDate?.(row, event.target.value || null)}
        />
      )}
    </div>
  );
  const shift = (direction: number) => {
    const next = new Date(`${anchor}T12:00:00`);
    if (period === "month") next.setMonth(next.getMonth() + direction, 1);
    else next.setDate(next.getDate() + direction * 7);
    onAnchorChange(
      `${next.getFullYear()}-${String(next.getMonth() + 1).padStart(2, "0")}-${String(next.getDate()).padStart(2, "0")}`,
    );
  };
  return (
    <div className="space-y-3 p-3">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          aria-label={labels.previous}
          onClick={() => shift(-1)}
        >
          ←
        </Button>
        <input
          aria-label={labels.date}
          type="date"
          value={anchor}
          onChange={(event) => {
            if (event.target.value) onAnchorChange(event.target.value);
          }}
        />
        <Button
          variant="outline"
          size="sm"
          aria-label={labels.next}
          onClick={() => shift(1)}
        >
          →
        </Button>
        {(["month", "week"] as const).map((value) => (
          <Button
            key={value}
            size="sm"
            variant={period === value ? "secondary" : "ghost"}
            onClick={() => onPeriodChange(value)}
          >
            {labels[value]}
          </Button>
        ))}
      </div>
      <div className="grid min-w-[560px] grid-cols-7 border-l border-t">
        {days.map((day) => (
          <section
            key={day}
            aria-label={day}
            className="min-h-28 space-y-1 border-b border-r p-1"
            onDragOver={(event) => {
              if (editable) event.preventDefault();
            }}
            onDrop={(event) => {
              event.preventDefault();
              const id = event.dataTransfer.getData(
                "application/x-data-view-row",
              );
              const row = rows.find((row) => rowId(row) === id);
              if (row && editable) onMoveDate?.(row, day);
            }}
          >
            <h3 className="text-caption text-muted-foreground">
              {day.slice(5)}
            </h3>
            {rows.filter((row) => date(row)?.slice(0, 10) === day).map(card)}
          </section>
        ))}
      </div>
      {rows.some((row) => !date(row)) && (
        <section>
          <h3 className="mb-2 text-caption text-muted-foreground">
            {labels.unscheduled}
          </h3>
          <div className="grid grid-cols-3 gap-2">
            {rows.filter((row) => !date(row)).map(card)}
          </div>
        </section>
      )}
    </div>
  );
}
