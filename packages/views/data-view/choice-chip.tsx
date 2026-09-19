"use client";

import type { ReactNode } from "react";
import { Check } from "lucide-react";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { cn } from "@multica/ui/lib/utils";

export interface ChoiceOption {
  id: string;
  label: string;
}

const chipClass =
  "inline-flex h-7 items-center gap-1.5 rounded-md border bg-background px-2 text-label text-muted-foreground shadow-xs hover:bg-accent data-[popup-open]:bg-accent";

/** A labelled view-bar chip that picks one value, e.g. a calendar date field. */
export function DataViewChoiceChip({
  icon,
  label,
  options,
  value,
  onChange,
  emptyLabel,
}: {
  icon?: ReactNode;
  label: string;
  options: readonly ChoiceOption[];
  value: string;
  onChange: (value: string) => void;
  /** Offers an explicit "none" row when set. */
  emptyLabel?: string;
}) {
  const current = options.find((option) => option.id === value);
  return (
    <Popover>
      <PopoverTrigger render={<button type="button" className={chipClass} />}>
        {icon}
        {label}
        <span className="font-medium text-foreground">
          {current?.label ?? emptyLabel ?? "—"}
        </span>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-56 p-1">
        {emptyLabel !== undefined && (
          <ChoiceRow selected={!current} onClick={() => onChange("")}>
            {emptyLabel}
          </ChoiceRow>
        )}
        {options.map((option) => (
          <ChoiceRow
            key={option.id}
            selected={option.id === value}
            onClick={() => onChange(option.id)}
          >
            {option.label}
          </ChoiceRow>
        ))}
      </PopoverContent>
    </Popover>
  );
}

/** A view-bar chip that toggles which fields a card shows. */
export function DataViewFieldToggles({
  icon,
  label,
  options,
  selected,
  onChange,
}: {
  icon?: ReactNode;
  label: string;
  options: readonly ChoiceOption[];
  selected: readonly string[];
  onChange: (selected: string[]) => void;
}) {
  return (
    <Popover>
      <PopoverTrigger render={<button type="button" className={chipClass} />}>
        {icon}
        {label}
        <span className="font-medium text-foreground tabular-nums">
          {selected.filter((id) => options.some((option) => option.id === id)).length}
        </span>
      </PopoverTrigger>
      <PopoverContent align="start" className="max-h-80 w-60 overflow-y-auto p-1">
        {options.map((option) => {
          const on = selected.includes(option.id);
          return (
            <ChoiceRow
              key={option.id}
              selected={on}
              checkbox
              onClick={() =>
                onChange(
                  on
                    ? selected.filter((id) => id !== option.id)
                    : [...selected, option.id],
                )
              }
            >
              {option.label}
            </ChoiceRow>
          );
        })}
      </PopoverContent>
    </Popover>
  );
}

function ChoiceRow({
  selected,
  onClick,
  checkbox,
  children,
}: {
  selected: boolean;
  onClick: () => void;
  checkbox?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      role={checkbox ? "menuitemcheckbox" : "menuitemradio"}
      aria-checked={selected}
      onClick={onClick}
      className="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-label hover:bg-accent"
    >
      <span className="min-w-0 flex-1 truncate">{children}</span>
      <Check className={cn("size-3.5 shrink-0", !selected && "invisible")} />
    </button>
  );
}
