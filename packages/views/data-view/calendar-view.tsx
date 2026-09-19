"use client";
import { useState, type ReactNode } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { calendarDate, calendarDays } from "@multica/core/data-source";
import type { DataSourceCapabilities } from "@multica/core/data-source";
import { Button } from "@multica/ui/components/ui/button";
import { cn } from "@multica/ui/lib/utils";

export interface CalendarLabels {
  month: string;
  week: string;
  previous: string;
  next: string;
  date: string;
  unscheduled: string;
  today?: string;
}

const DRAG_TYPE = "application/x-data-view-row";

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
  toolbar,
  color,
  locale,
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
  /** Source controls shown before the period switch, e.g. the date field. */
  toolbar?: ReactNode;
  /** Optional per-row accent, e.g. the color of the row's status option. */
  color?: (row: Row) => string | undefined;
  /** BCP 47 locale for weekday and month names. */
  locale?: string;
}) {
  const [over, setOver] = useState<string | null>(null);
  if (!capabilities.layouts.includes("calendar")) return null;
  const days = calendarDays(anchor, period);
  const editable = capabilities.editing === "adapter" && !!onMoveDate;
  const today = calendarDate(new Date());
  const month = anchor.slice(0, 7);
  const weekdayFormat = new Intl.DateTimeFormat(locale, { weekday: "short" });
  const weekdays = days
    .slice(0, 7)
    .map((day) => weekdayFormat.format(new Date(`${day}T12:00:00`)));
  const heading = new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
  }).format(new Date(`${anchor}T12:00:00`));

  const pill = (row: Row) => (
    <button
      key={rowId(row)}
      type="button"
      title={title(row)}
      draggable={editable}
      data-row-id={rowId(row)}
      onDragStart={(event) => {
        event.dataTransfer.setData(DRAG_TYPE, rowId(row));
        event.dataTransfer.effectAllowed = "move";
      }}
      onClick={() => onOpen?.(row)}
      style={color?.(row) ? { backgroundColor: color(row), color: "#fff" } : undefined}
      className={cn(
        "block w-full truncate rounded-sm bg-primary px-1.5 py-0.5 text-left text-caption font-medium text-primary-foreground hover:opacity-90",
        editable && "cursor-grab active:cursor-grabbing",
      )}
    >
      {title(row) || "—"}
    </button>
  );
  const shift = (direction: number) => {
    const next = new Date(`${anchor}T12:00:00`);
    if (period === "month") next.setMonth(next.getMonth() + direction, 1);
    else next.setDate(next.getDate() + direction * 7);
    onAnchorChange(calendarDate(next));
  };
  const dropTo = (day: string | null) => (event: React.DragEvent) => {
    event.preventDefault();
    setOver(null);
    const id = event.dataTransfer.getData(DRAG_TYPE);
    const row = rows.find((item) => rowId(item) === id);
    if (row && editable && (date(row)?.slice(0, 10) ?? null) !== day)
      onMoveDate?.(row, day);
  };
  const unscheduled = rows.filter((row) => !date(row));
  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3 p-4">
      <div className="flex flex-wrap items-center gap-2">
        {toolbar}
        <div className="inline-flex rounded-md border p-0.5">
          {(["month", "week"] as const).map((value) => (
            <Button
              key={value}
              size="xs"
              variant={period === value ? "secondary" : "ghost"}
              aria-pressed={period === value}
              onClick={() => onPeriodChange(value)}
            >
              {labels[value]}
            </Button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-1">
          <span className="mr-2 text-label font-medium tabular-nums">
            {heading}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={labels.previous}
            onClick={() => shift(-1)}
          >
            <ChevronLeft />
          </Button>
          <Button
            variant="outline"
            size="xs"
            onClick={() => onAnchorChange(today)}
          >
            {labels.today ?? today}
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={labels.next}
            onClick={() => shift(1)}
          >
            <ChevronRight />
          </Button>
        </div>
      </div>
      <div className="min-h-0 overflow-auto rounded-md border">
        <div className="grid min-w-[640px] grid-cols-7">
          {weekdays.map((weekday) => (
            <div
              key={weekday}
              className="border-b bg-muted/40 py-1.5 text-center text-caption text-muted-foreground"
            >
              {weekday}
            </div>
          ))}
          {days.map((day, index) => {
            const outside = period === "month" && day.slice(0, 7) !== month;
            const items = rows.filter((row) => date(row)?.slice(0, 10) === day);
            return (
              <section
                key={day}
                aria-label={day}
                className={cn(
                  "space-y-1 border-b border-r p-1.5",
                  period === "month" ? "min-h-24" : "min-h-64",
                  (index + 1) % 7 === 0 && "border-r-0",
                  outside && "bg-muted/40",
                  over === day && "bg-primary/5 outline-2 -outline-offset-2 outline-dashed outline-primary/60",
                )}
                onDragOver={(event) => {
                  if (!editable) return;
                  event.preventDefault();
                  setOver(day);
                }}
                onDragLeave={() => setOver((current) => (current === day ? null : current))}
                onDrop={dropTo(day)}
              >
                <h3
                  className={cn(
                    "text-caption tabular-nums text-muted-foreground",
                    outside && "text-muted-foreground/60",
                    day === today &&
                      "inline-flex size-5 items-center justify-center rounded-full bg-primary font-medium text-primary-foreground",
                  )}
                >
                  {Number(day.slice(8))}
                </h3>
                {items.map(pill)}
              </section>
            );
          })}
        </div>
      </div>
      {unscheduled.length > 0 && (
        <section
          aria-label={labels.unscheduled}
          className="rounded-md border border-dashed p-2"
          onDragOver={(event) => {
            if (editable) event.preventDefault();
          }}
          onDrop={dropTo(null)}
        >
          <h3 className="mb-2 text-caption text-muted-foreground">
            {labels.unscheduled} · {unscheduled.length}
          </h3>
          <div className="grid grid-cols-[repeat(auto-fill,minmax(160px,1fr))] gap-1.5">
            {unscheduled.map(pill)}
          </div>
        </section>
      )}
    </div>
  );
}
