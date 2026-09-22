"use client";

import { useRef, useState } from "react";
import { GripVertical, ListTodo, Plus, Table2, X } from "lucide-react";
import type {
  CollectionField,
  CollectionFieldInput,
} from "@multica/core/collections";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Popover, PopoverContent } from "@multica/ui/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { ColorPicker, COLOR_PICKER_PRESETS } from "../common/color-picker";
import { useT } from "../i18n";
import {
  COLLECTION_FIELD_TYPES,
  FieldTypeIcon,
  FieldTypeLabel,
  MAX_COLLECTION_FIELDS,
  RELATION_TYPE,
  fieldHasOptions,
  safeFieldConversions,
} from "./collection-fields";
import type { CollectionCommands } from "./use-collection-commands";

interface OptionDraft {
  id?: string;
  name: string;
  color: string;
}

const presetColor = (index: number) =>
  COLOR_PICKER_PRESETS[index % COLOR_PICKER_PRESETS.length] ?? "#6b7280";

/** What a relation field can point at, besides workspace tasks. */
export interface RelationTable {
  id: string;
  name: string;
}
/** The "link to" choice that means workspace tasks rather than a table. */
const TASKS = "issue";

// Formula function names are language-independent expression syntax.
const FORMULA_FUNCTIONS = [
  "IF", "IFERROR", "AND", "OR", "NOT", "SUM", "AVG", "MIN", "MAX", "ROUND",
  "ABS", "CEIL", "FLOOR", "CONCAT", "LEN", "LOWER", "UPPER", "TRIM",
  "ISBLANK", "COALESCE", "VALUE",
];

/** What the panel edits. */
export type FieldPanelTarget =
  /** A new field; `type` preselects what the caller is missing. */
  | { kind: "new"; type?: string }
  | { kind: "field"; field: CollectionField }
  /**
   * The title column. It is not a catalog field: only its label changes, and
   * an empty `name` means the localized `defaultName` is showing.
   */
  | { kind: "title"; name: string; defaultName: string };

/**
 * Creates a field or edits one, in a panel that hangs under the control it
 * was opened from: the "+" cell for a new field, the column for an existing
 * one.
 */
export function CollectionFieldPanel({
  open,
  onOpenChange,
  anchor,
  target,
  fieldCount,
  fields = [],
  tables = [],
  commands,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  anchor: Element | null;
  target: FieldPanelTarget;
  fieldCount: number;
  fields?: CollectionField[];
  /** Tables a relation field may point at, this one included. */
  tables?: RelationTable[];
  commands: CollectionCommands;
}) {
  const { t } = useT("issues");
  return (
    <Popover open={open} onOpenChange={onOpenChange}>
      <PopoverContent
        anchor={anchor}
        align="start"
        sideOffset={2}
        aria-label={
          target.kind === "new"
            ? t(($) => $.cortex_table.new_field)
            : t(($) => $.cortex_table.edit_field)
        }
        className="max-h-(--available-height) w-80 gap-0 overflow-y-auto p-3"
      >
        {/* Mounted per opening, so the draft always starts from `target`. */}
        <FieldForm
          target={target}
          fieldCount={fieldCount}
          fields={fields}
          tables={tables}
          commands={commands}
          onClose={() => onOpenChange(false)}
        />
      </PopoverContent>
    </Popover>
  );
}

function FieldForm({
  target,
  fieldCount,
  fields,
  tables,
  commands,
  onClose,
}: {
  target: FieldPanelTarget;
  fieldCount: number;
  fields: CollectionField[];
  tables: RelationTable[];
  commands: CollectionCommands;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const field = target.kind === "field" ? target.field : null;
  const isTitle = target.kind === "title";
  const initialType =
    field?.type ?? (target.kind === "new" ? (target.type ?? "text") : "text");
  const [name, setName] = useState(
    field?.name ??
      (target.kind === "title" ? target.name || target.defaultName : ""),
  );
  const [type, setType] = useState(initialType);
  const [options, setOptions] = useState<OptionDraft[]>(() =>
    field
      ? field.config.options.map((option, index) => ({
          id: option.id,
          name: option.name,
          color: option.color ?? presetColor(index),
        }))
      : fieldHasOptions(initialType)
        ? [{ name: "", color: presetColor(0) }]
        : [],
  );
  // What a relation points at. It is chosen once: the server refuses to move
  // an existing relation, whose links would then aim at the wrong kind of thing.
  const [linkTo, setLinkTo] = useState(() => {
    const relation = field?.config.relation;
    if (!relation || relation.to_type === TASKS) return TASKS;
    return relation.collection_id;
  });
  const [expression, setExpression] = useState(
    field?.config.formula?.expression ?? "",
  );
  const formulaInput = useRef<HTMLTextAreaElement>(null);
  const [error, setError] = useState<string | null>(null);
  const optionList = useRef<HTMLDivElement>(null);

  // An existing field only converts where the server keeps its values, and
  // the title column is always text.
  const typeChoices = isTitle
    ? ["text"]
    : field
      ? [field.type, ...safeFieldConversions(field.type)]
      : [...COLLECTION_FIELD_TYPES];
  const showOptions = fieldHasOptions(type);
  const showLinkTo = type === RELATION_TYPE;
  const linkChoices = [
    { value: TASKS, name: t(($) => $.cortex_table.relation_tasks) },
    ...tables.map((table) => ({ value: table.id, name: table.name })),
    // The table an existing relation points at may since have been deleted.
    ...(linkTo !== TASKS && !tables.some((table) => table.id === linkTo)
      ? [
          {
            value: linkTo,
            name: t(($) => $.cortex_table.relation_table_deleted),
          },
        ]
      : []),
  ].map((choice) => ({
    value: choice.value,
    // Issues are not one more table: the glyph says so before the name does.
    label: (
      <>
        {choice.value === TASKS ? (
          <ListTodo
            aria-hidden
            className="size-3.5 shrink-0 text-muted-foreground"
          />
        ) : (
          <Table2
            aria-hidden
            className="size-3.5 shrink-0 text-muted-foreground"
          />
        )}
        <span className="truncate">{choice.name}</span>
      </>
    ),
  }));
  const valid = options.filter((option) => option.name.trim());
  const removed = field
    ? field.config.options.filter(
        (option) => !options.some((draft) => draft.id === option.id),
      )
    : [];
  const pending =
    commands.createField.isPending ||
    commands.updateField.isPending ||
    commands.renameTitleColumn.isPending;
  const full = target.kind === "new" && fieldCount >= MAX_COLLECTION_FIELDS;
  const canSubmit =
    // An empty title label is a valid choice: it restores the default.
    (isTitle || name.trim().length > 0) &&
    (!showOptions || valid.length > 0) &&
    (type !== "formula" || !!expression.trim()) &&
    !pending &&
    !full;

  const addOption = () => {
    setOptions((current) => [
      ...current,
      { name: "", color: presetColor(current.length) },
    ]);
    requestAnimationFrame(() => {
      const inputs =
        optionList.current?.querySelectorAll<HTMLInputElement>("input");
      inputs?.[inputs.length - 1]?.focus();
    });
  };
  const submit = async () => {
    if (!canSubmit) return;
    setError(null);
    const config =
      type === "formula"
        ? {
            formula: {
              expression: expression.trim(),
              ...(field?.config.formula?.bindings
                ? { bindings: field.config.formula.bindings }
                : {}),
            },
          }
        : showOptions
          ? {
              options: valid.map((option) => ({
                ...(option.id ? { id: option.id } : {}),
                name: option.name.trim(),
                color: option.color,
              })),
            }
          : undefined;
    const relation: CollectionFieldInput["config"] = showLinkTo
      ? {
          relation:
            linkTo === TASKS
              ? { to_type: "issue" }
              : { to_type: "record", collection_id: linkTo },
        }
      : undefined;
    try {
      if (target.kind === "title") {
        // The default label is not stored, so it keeps following the language.
        const label = name.trim() === target.defaultName ? "" : name.trim();
        if (label !== target.name)
          await commands.renameTitleColumn.mutateAsync(label);
      } else if (field) {
        await commands.updateField.mutateAsync({
          fieldId: field.id,
          patch: {
            ...(name.trim() !== field.name ? { name: name.trim() } : {}),
            ...(type !== field.type ? { type } : {}),
            ...(config ? { config } : {}),
          },
        });
      } else {
        await commands.createField.mutateAsync({
          name: name.trim(),
          type,
          ...(config ? { config } : {}),
          ...(relation ? { config: relation } : {}),
        });
      }
      onClose();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  return (
    <form
      className="space-y-3"
      aria-busy={pending}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor="collection-field-name">
          {t(($) => $.cortex_table.field_name)}
        </Label>
        <Input
          id="collection-field-name"
          autoFocus
          maxLength={32}
          placeholder={
            target.kind === "title"
              ? target.defaultName
              : t(($) => $.cortex_table.field_name_placeholder)
          }
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t(($) => $.cortex_table.field_type)}</Label>
        <Select
          items={typeChoices.map((value) => ({
            value,
            label: (
              <>
                <FieldTypeIcon type={value} />
                <FieldTypeLabel type={value} />
              </>
            ),
          }))}
          value={type}
          disabled={typeChoices.length < 2}
          onValueChange={(value) => {
            if (!value) return;
            setType(value);
            if (fieldHasOptions(value) && options.length === 0)
              setOptions([{ name: "", color: presetColor(0) }]);
          }}
        >
          {/* The list flies out beside the panel, so the chevron points at it. */}
          <SelectTrigger
            aria-label={t(($) => $.cortex_table.field_type)}
            className="w-full [&>svg:last-child]:-rotate-90"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent
            side="right"
            align="start"
            sideOffset={8}
            alignItemWithTrigger={false}
            className="w-44"
          >
            {typeChoices.map((value) => (
              <SelectItem key={value} value={value}>
                <FieldTypeIcon type={value} />
                <FieldTypeLabel type={value} />
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {target.kind !== "new" && (
          <p className="text-caption text-muted-foreground">
            {isTitle
              ? t(($) => $.cortex_table.title_column_locked)
              : typeChoices.length < 2
                ? t(($) => $.cortex_table.type_locked)
                : t(($) => $.cortex_table.type_limited)}
          </p>
        )}
      </div>
      {type === "formula" && (
        <div className="space-y-2">
          <Label htmlFor="collection-formula">
            {t(($) => $.cortex_formula.expression)}
          </Label>
          <textarea
            ref={formulaInput}
            id="collection-formula"
            className="min-h-24 w-full resize-y rounded-md border bg-transparent px-3 py-2 font-mono text-label outline-none focus-visible:ring-2 focus-visible:ring-ring"
            value={expression}
            maxLength={4096}
            placeholder={t(($) => $.cortex_formula.placeholder)}
            onChange={(event) => setExpression(event.target.value)}
            aria-describedby="collection-formula-help"
          />
          <p
            id="collection-formula-help"
            className="text-caption text-muted-foreground"
          >
            {t(($) => $.cortex_formula.help)}
          </p>
          <div
            role="group"
            className="flex max-h-28 flex-wrap gap-1 overflow-y-auto"
            aria-label={t(($) => $.cortex_formula.insert_field)}
          >
            {[
              { id: "title", name: "title", type: "text" },
              ...fields.filter(
                (f) =>
                  f.id !== field?.id &&
                  [
                    "text",
                    "number",
                    "checkbox",
                    "date",
                    "url",
                    "select",
                    "formula",
                  ].includes(f.type),
              ),
            ].map((f) => (
              <Button
                key={f.id}
                type="button"
                variant="outline"
                size="xs"
                onClick={() => {
                  const reference =
                    "{" +
                    (f.name === "title" ? f.id : f.name)
                      .replaceAll("\\", "\\\\")
                      .replaceAll("}", "\\}") +
                    "}";
                  const start =
                    formulaInput.current?.selectionStart ?? expression.length;
                  const end = formulaInput.current?.selectionEnd ?? start;
                  setExpression(
                    expression.slice(0, start) +
                      reference +
                      expression.slice(end),
                  );
                  requestAnimationFrame(() => {
                    formulaInput.current?.focus();
                    formulaInput.current?.setSelectionRange(
                      start + reference.length,
                      start + reference.length,
                    );
                  });
                }}
              >
                {f.name}
              </Button>
            ))}
          </div>
          <details className="text-caption text-muted-foreground">
            <summary className="cursor-pointer">
              {t(($) => $.cortex_formula.functions)}
            </summary>
            <p className="mt-1 break-words">
              {FORMULA_FUNCTIONS.join(" · ")}
            </p>
            <pre className="mt-1 whitespace-pre-wrap">
              {'IF({title} == "", 0, 1)\nROUND(10 / 3, 2)'}
            </pre>
          </details>
        </div>
      )}
      {showLinkTo && (
        <div className="space-y-1.5">
          <Label>{t(($) => $.cortex_table.relation_link_to)}</Label>
          <Select
            items={linkChoices}
            value={linkTo}
            disabled={!!field}
            onValueChange={(value) => value && setLinkTo(value)}
          >
            <SelectTrigger
              aria-label={t(($) => $.cortex_table.relation_link_to)}
              className="w-full"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false} className="max-h-72">
              {linkChoices.map((choice) => (
                <SelectItem key={choice.value} value={choice.value}>
                  {choice.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_table.relation_target_fixed)}
          </p>
        </div>
      )}
      {showOptions && (
        <div className="space-y-1.5" ref={optionList}>
          <Label>{t(($) => $.cortex_table.options)}</Label>
          {options.map((option, index) => (
            <div
              key={option.id ?? `new-${index}`}
              className="flex items-center gap-2"
            >
              <GripVertical className="size-4 shrink-0 text-muted-foreground/50" />
              <ColorPicker
                value={option.color}
                onChange={(color) =>
                  setOptions((current) =>
                    current.map((item, i) =>
                      i === index ? { ...item, color } : item,
                    ),
                  )
                }
                trigger={
                  <button
                    type="button"
                    aria-label={t(($) => $.cortex_table.option_color, {
                      name: option.name || index + 1,
                    })}
                    className="size-5 shrink-0 cursor-pointer rounded-full border border-surface-border"
                    style={{ backgroundColor: option.color }}
                  />
                }
              />
              <Input
                aria-label={t(($) => $.cortex_table.option_name, {
                  index: index + 1,
                })}
                className="h-8"
                maxLength={32}
                value={option.name}
                onChange={(event) =>
                  setOptions((current) =>
                    current.map((item, i) =>
                      i === index
                        ? { ...item, name: event.target.value }
                        : item,
                    ),
                  )
                }
                onKeyDown={(event) => {
                  if (
                    event.key === "Enter" &&
                    index === options.length - 1 &&
                    option.name.trim()
                  ) {
                    event.preventDefault();
                    addOption();
                  }
                }}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t(($) => $.cortex_table.remove_option, {
                  name: option.name || index + 1,
                })}
                disabled={options.length <= 1}
                onClick={() =>
                  setOptions((current) => current.filter((_, i) => i !== index))
                }
              >
                <X />
              </Button>
            </div>
          ))}
          <Button type="button" variant="ghost" size="sm" onClick={addOption}>
            <Plus />
            {t(($) => $.cortex_table.add_option)}
          </Button>
          {removed.length > 0 && (
            <p className="text-caption text-warning">
              {t(($) => $.cortex_table.options_removed_warning, {
                names: removed.map((option) => option.name).join(", "),
              })}
            </p>
          )}
        </div>
      )}
      {error && (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      )}
      <div className="flex items-center gap-2 border-t pt-3">
        {/* The title column is outside the field quota. */}
        {!isTitle && (
          <span
            className={
              full
                ? "text-caption text-warning"
                : "text-caption text-muted-foreground"
            }
          >
            {t(($) => $.cortex_table.fields_used, {
              count: fieldCount,
              max: MAX_COLLECTION_FIELDS,
            })}
          </span>
        )}
        <span className="flex-1" />
        <Button type="button" variant="outline" size="sm" onClick={onClose}>
          {t(($) => $.cortex_table.cancel)}
        </Button>
        <Button type="submit" size="sm" disabled={!canSubmit}>
          {target.kind === "new"
            ? t(($) => $.cortex_table.create_field)
            : t(($) => $.cortex_table.save)}
        </Button>
      </div>
    </form>
  );
}
