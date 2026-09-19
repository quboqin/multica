"use client";

import type { ReactNode } from "react";
import {
  ArrowDownUp,
  CalendarDays,
  Check,
  Eye,
  Filter,
  Group,
  Kanban,
  LayoutGrid,
  Plus,
  Table2,
  X,
} from "lucide-react";
import type { CollectionField } from "@multica/core/collections";
import type {
  DataViewFilter,
  DataViewLayout,
  DataViewPreferences,
} from "@multica/core/data-source";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import { FieldTypeIcon, OptionChip } from "./collection-fields";

const LAYOUTS: { layout: DataViewLayout; icon: typeof Table2 }[] = [
  { layout: "table", icon: Table2 },
  { layout: "board", icon: Kanban },
  { layout: "calendar", icon: CalendarDays },
  { layout: "gallery", icon: LayoutGrid },
];

/** Operators the record query understands, by field type. */
export function filterOperators(type: string): string[] {
  switch (type) {
    case "text":
    case "url":
      return ["contains", "exact"];
    case "number":
      return ["exact", "gt", "gte", "lt", "lte"];
    case "date":
      return ["exact", "before", "after"];
    default:
      return ["exact"];
  }
}

/** Converts view filters into the record query's `properties` shape. */
export function filtersToProperties(
  filters: DataViewFilter[],
  fields: CollectionField[],
): Record<string, unknown[]> | undefined {
  const out: Record<string, unknown[]> = {};
  for (const filter of filters) {
    const field = fields.find((item) => item.id === filter.field);
    if (!field || filter.value === "" || filter.value == null) continue;
    let value: unknown = filter.value;
    if (filter.op === "exact") {
      if (field.type === "number") value = Number(filter.value);
      else if (field.type === "checkbox") value = filter.value === true || filter.value === "true";
      else if (field.type === "multi_select") value = [filter.value];
    } else {
      value = { op: filter.op, value: String(filter.value) };
    }
    if (field.type === "number" && filter.op === "exact" && Number.isNaN(value)) continue;
    // One condition per field: alternatives on the same field are OR-ed by
    // the server, which would turn a range into a union.
    out[field.id] = [value];
  }
  return Object.keys(out).length ? out : undefined;
}

function BarButton({
  icon,
  label,
  value,
  active,
  ...props
}: {
  icon: ReactNode;
  label: string;
  value?: ReactNode;
  active?: boolean;
} & React.ComponentProps<"button">) {
  return (
    <button
      type="button"
      {...props}
      className={cn(
        "inline-flex h-7 items-center gap-1.5 rounded-md border border-transparent px-2 text-label text-muted-foreground transition-colors hover:bg-accent hover:text-foreground data-[popup-open]:bg-accent",
        active && "border-border bg-background text-foreground shadow-xs",
      )}
    >
      {icon}
      <span>{label}</span>
      {value !== undefined && value !== "" && (
        <span className="font-medium text-foreground">{value}</span>
      )}
    </button>
  );
}

export function LayoutSwitch({
  layout,
  onChange,
}: {
  layout: DataViewLayout;
  onChange: (layout: DataViewLayout) => void;
}) {
  const { t } = useT("issues");
  const names: Record<DataViewLayout, string> = {
    table: t(($) => $.cortex_table.layout_table),
    board: t(($) => $.cortex_table.layout_board),
    calendar: t(($) => $.cortex.calendar),
    gallery: t(($) => $.cortex.gallery),
  };
  return (
    <div
      role="radiogroup"
      aria-label={t(($) => $.cortex_table.layout)}
      className="inline-flex items-center rounded-md border bg-muted/40 p-0.5"
    >
      {LAYOUTS.map(({ layout: value, icon: Icon }) => (
        <Tooltip key={value}>
          <TooltipTrigger
            render={
              <button
                type="button"
                role="radio"
                aria-checked={layout === value}
                aria-label={names[value]}
                onClick={() => onChange(value)}
                className={cn(
                  "inline-flex size-6 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground",
                  layout === value && "bg-background text-foreground shadow-xs",
                )}
              />
            }
          >
            <Icon className="size-3.5" />
          </TooltipTrigger>
          <TooltipContent>{names[value]}</TooltipContent>
        </Tooltip>
      ))}
    </div>
  );
}

function FieldSelect({
  fields,
  value,
  onChange,
  label,
  allowNone,
  noneLabel,
}: {
  fields: CollectionField[];
  value: string;
  onChange: (value: string) => void;
  label: string;
  allowNone?: boolean;
  noneLabel?: string;
}) {
  const items = [
    ...(allowNone ? [{ value: "", label: noneLabel ?? "—" }] : []),
    ...fields.map((field) => ({ value: field.id, label: field.name })),
  ];
  return (
    <Select items={items} value={value} onValueChange={(next) => onChange(next ?? "")}>
      <SelectTrigger size="sm" aria-label={label} className="min-w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {allowNone && <SelectItem value="">{noneLabel ?? "—"}</SelectItem>}
        {fields.map((field) => (
          <SelectItem key={field.id} value={field.id}>
            <span className="flex items-center gap-2">
              <FieldTypeIcon type={field.type} />
              {field.name}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function FilterValue({
  field,
  filter,
  onChange,
}: {
  field: CollectionField;
  filter: DataViewFilter;
  onChange: (value: unknown) => void;
}) {
  const { t } = useT("issues");
  const label = t(($) => $.cortex.filter_value);
  if (field.type === "select" || field.type === "multi_select")
    return (
      <Select
        items={field.config.options.map((option) => ({ value: option.id, label: option.name }))}
        value={typeof filter.value === "string" ? filter.value : ""}
        onValueChange={(next) => onChange(next ?? "")}
      >
        <SelectTrigger size="sm" aria-label={label} className="min-w-32">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {field.config.options.map((option) => (
            <SelectItem key={option.id} value={option.id}>
              <OptionChip name={option.name} color={option.color} />
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  if (field.type === "checkbox")
    return (
      <Select
        items={[
          { value: "true", label: t(($) => $.pickers.custom_property.true_label) },
          { value: "false", label: t(($) => $.pickers.custom_property.false_label) },
        ]}
        value={filter.value === true || filter.value === "true" ? "true" : "false"}
        onValueChange={(next) => onChange(next === "true")}
      >
        <SelectTrigger size="sm" aria-label={label} className="min-w-24">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="true">{t(($) => $.pickers.custom_property.true_label)}</SelectItem>
          <SelectItem value="false">{t(($) => $.pickers.custom_property.false_label)}</SelectItem>
        </SelectContent>
      </Select>
    );
  return (
    <Input
      aria-label={label}
      className="h-7 w-36"
      type={field.type === "date" ? "date" : field.type === "number" ? "number" : "text"}
      value={filter.value == null ? "" : String(filter.value)}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

export function FilterPopover({
  fields,
  filters,
  onChange,
  open,
  onOpenChange,
}: {
  fields: CollectionField[];
  filters: DataViewFilter[];
  onChange: (filters: DataViewFilter[]) => void;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useT("issues");
  const filterable = fields.filter(
    (field) => field.type !== "actor" && field.type !== "multi_actor",
  );
  const unused = filterable.filter(
    (field) => !filters.some((filter) => filter.field === field.id),
  );
  const active = filters.filter((filter) => filter.value !== "" && filter.value != null).length;
  const opLabel = (op: string) =>
    t(($) => $.cortex_table.operators[op as "exact"]) || op;
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverTrigger
        render={
          <BarButton
            icon={<Filter className="size-3.5" />}
            label={t(($) => $.cortex_table.filter)}
            value={active ? String(active) : undefined}
            active={active > 0}
          />
        }
      />
      <PopoverContent align="start" className="w-[28rem] space-y-2 p-3">
        {filters.length === 0 && (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_table.no_filters)}
          </p>
        )}
        {filters.map((filter, index) => {
          const field = fields.find((item) => item.id === filter.field);
          if (!field) return null;
          const replace = (patch: Partial<DataViewFilter>) =>
            onChange(filters.map((item, i) => (i === index ? { ...item, ...patch } : item)));
          const operators = filterOperators(field.type);
          return (
            <div key={filter.field} className="flex items-center gap-1.5">
              <FieldSelect
                label={t(($) => $.cortex.field_name)}
                fields={[field, ...unused]}
                value={field.id}
                onChange={(next) => {
                  const nextField = fields.find((item) => item.id === next);
                  if (nextField)
                    replace({ field: next, op: filterOperators(nextField.type)[0]!, value: "" });
                }}
              />
              <Select
                items={operators.map((op) => ({ value: op, label: opLabel(op) }))}
                value={filter.op}
                onValueChange={(next) => next && replace({ op: next })}
              >
                <SelectTrigger size="sm" aria-label={t(($) => $.cortex.filter_operator)}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {operators.map((op) => (
                    <SelectItem key={op} value={op}>
                      {opLabel(op)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <FilterValue field={field} filter={filter} onChange={(value) => replace({ value })} />
              <Button
                variant="ghost"
                size="icon-xs"
                aria-label={t(($) => $.cortex_table.remove_filter, { name: field.name })}
                onClick={() => onChange(filters.filter((_, i) => i !== index))}
              >
                <X />
              </Button>
            </div>
          );
        })}
        <div className="flex items-center justify-between pt-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={unused.length === 0}
            onClick={() => {
              const field = unused[0];
              if (field)
                onChange([...filters, { field: field.id, op: filterOperators(field.type)[0]!, value: "" }]);
            }}
          >
            <Plus />
            {t(($) => $.cortex_table.add_filter)}
          </Button>
          {filters.length > 0 && (
            <Button variant="ghost" size="sm" onClick={() => onChange([])}>
              {t(($) => $.cortex_table.clear_filters)}
            </Button>
          )}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function GroupPopover({
  fields,
  groupBy,
  onChange,
}: {
  fields: CollectionField[];
  groupBy: string;
  onChange: (fieldId: string) => void;
}) {
  const { t } = useT("issues");
  const groupable = fields.filter((field) => field.type === "select");
  const current = groupable.find((field) => field.id === groupBy);
  return (
    <Popover>
      <PopoverTrigger
        render={
          <BarButton
            icon={<Group className="size-3.5" />}
            label={t(($) => $.cortex_table.group)}
            value={current?.name}
            active={!!current}
          />
        }
      />
      <PopoverContent align="start" className="w-56 p-1">
        <OptionRow selected={!current} onClick={() => onChange("")}>
          {t(($) => $.cortex.no_group)}
        </OptionRow>
        {groupable.map((field) => (
          <OptionRow key={field.id} selected={field.id === groupBy} onClick={() => onChange(field.id)}>
            <FieldTypeIcon type={field.type} />
            {field.name}
          </OptionRow>
        ))}
        {groupable.length === 0 && (
          <p className="px-2 py-1.5 text-caption text-muted-foreground">
            {t(($) => $.cortex_table.group_needs_select)}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}

export function SortPopover({
  fields,
  sortBy,
  sortDir,
  onChange,
}: {
  fields: CollectionField[];
  sortBy: string;
  sortDir: "asc" | "desc";
  onChange: (sortBy: string, sortDir: "asc" | "desc") => void;
}) {
  const { t } = useT("issues");
  const sortable = fields.filter(
    (field) => field.type !== "multi_actor" && field.type !== "actor",
  );
  const name =
    sortBy === "title"
      ? t(($) => $.cortex_table.name_column)
      : sortable.find((field) => field.id === sortBy)?.name;
  return (
    <Popover>
      <PopoverTrigger
        render={
          <BarButton
            icon={<ArrowDownUp className="size-3.5" />}
            label={t(($) => $.cortex_table.sort)}
            value={name}
            active={!!name}
          />
        }
      />
      <PopoverContent align="start" className="w-72 space-y-2 p-3">
        <div className="flex items-center gap-1.5">
          <Select
            items={[
              { value: "", label: t(($) => $.cortex_table.sort_created) },
              { value: "title", label: t(($) => $.cortex_table.name_column) },
              ...sortable.map((field) => ({ value: field.id, label: field.name })),
            ]}
            value={name ? sortBy : ""}
            onValueChange={(next) => onChange(next ?? "", sortDir)}
          >
            <SelectTrigger size="sm" aria-label={t(($) => $.cortex_table.sort)} className="flex-1">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">{t(($) => $.cortex_table.sort_created)}</SelectItem>
              <SelectItem value="title">{t(($) => $.cortex_table.name_column)}</SelectItem>
              {sortable.map((field) => (
                <SelectItem key={field.id} value={field.id}>
                  <span className="flex items-center gap-2">
                    <FieldTypeIcon type={field.type} />
                    {field.name}
                  </span>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select
            items={[
              { value: "asc", label: t(($) => $.cortex_table.sort_ascending) },
              { value: "desc", label: t(($) => $.cortex_table.sort_descending) },
            ]}
            value={sortDir}
            disabled={!name}
            onValueChange={(next) => onChange(sortBy, next === "desc" ? "desc" : "asc")}
          >
            <SelectTrigger size="sm" aria-label={t(($) => $.cortex_table.sort_direction)}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="asc">{t(($) => $.cortex_table.sort_ascending)}</SelectItem>
              <SelectItem value="desc">{t(($) => $.cortex_table.sort_descending)}</SelectItem>
            </SelectContent>
          </Select>
        </div>
      </PopoverContent>
    </Popover>
  );
}

/**
 * "Display" settings for the current layout: which fields are visible, and
 * the layout-specific choices (calendar date field, gallery cover).
 */
export function DisplayPopover({
  layout,
  fields,
  prefs,
  onChange,
}: {
  layout: DataViewLayout;
  fields: CollectionField[];
  prefs: DataViewPreferences;
  onChange: (patch: Partial<DataViewPreferences>) => void;
}) {
  const { t } = useT("issues");
  const table = layout === "table";
  const shown = table
    ? fields.filter((field) => !prefs.hiddenFields.includes(field.id))
    : fields.filter((field) => prefs.displayedFields.includes(field.id));
  const toggle = (id: string) => {
    if (table)
      onChange({
        hiddenFields: prefs.hiddenFields.includes(id)
          ? prefs.hiddenFields.filter((item) => item !== id)
          : [...prefs.hiddenFields, id],
      });
    else
      onChange({
        displayedFields: prefs.displayedFields.includes(id)
          ? prefs.displayedFields.filter((item) => item !== id)
          : [...prefs.displayedFields, id],
      });
  };
  return (
    <Popover>
      <PopoverTrigger
        render={
          <BarButton
            icon={<Eye className="size-3.5" />}
            label={t(($) => $.cortex_table.display)}
            value={`${shown.length}/${fields.length}`}
          />
        }
      />
      <PopoverContent align="start" className="w-64 space-y-2 p-2">
        {layout === "gallery" && (
          <div className="space-y-1 px-1 pb-1">
            <p className="text-caption text-muted-foreground">{t(($) => $.cortex.cover)}</p>
            <FieldSelect
              label={t(($) => $.cortex.cover)}
              fields={fields.filter((field) => field.type === "url")}
              value={prefs.coverField}
              allowNone
              noneLabel={t(($) => $.cortex_table.no_cover)}
              onChange={(coverField) => onChange({ coverField })}
            />
          </div>
        )}
        <p className="px-1 text-caption text-muted-foreground">
          {table ? t(($) => $.cortex_table.visible_columns) : t(($) => $.cortex_table.card_fields)}
        </p>
        <div className="max-h-72 overflow-y-auto">
          {fields.map((field) => {
            const visible = shown.includes(field);
            return (
              <OptionRow key={field.id} selected={visible} onClick={() => toggle(field.id)} checkbox>
                <FieldTypeIcon type={field.type} />
                {field.name}
              </OptionRow>
            );
          })}
        </div>
      </PopoverContent>
    </Popover>
  );
}

export function DateFieldChip({
  fields,
  value,
  onChange,
}: {
  fields: CollectionField[];
  value: string;
  onChange: (fieldId: string) => void;
}) {
  const { t } = useT("issues");
  const dates = fields.filter((field) => field.type === "date");
  const current = dates.find((field) => field.id === value);
  return (
    <Popover>
      <PopoverTrigger
        render={
          <BarButton
            icon={<CalendarDays className="size-3.5" />}
            label={t(($) => $.cortex.date_field)}
            value={current?.name ?? "—"}
            active
          />
        }
      />
      <PopoverContent align="start" className="w-56 p-1">
        {dates.map((field) => (
          <OptionRow key={field.id} selected={field.id === value} onClick={() => onChange(field.id)}>
            {field.name}
          </OptionRow>
        ))}
        {dates.length === 0 && (
          <p className="px-2 py-1.5 text-caption text-muted-foreground">
            {t(($) => $.cortex_table.calendar_needs_date)}
          </p>
        )}
      </PopoverContent>
    </Popover>
  );
}

function OptionRow({
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
      <span className="flex min-w-0 flex-1 items-center gap-2 truncate">{children}</span>
      <Check className={cn("size-3.5 shrink-0", !selected && "invisible")} />
    </button>
  );
}
