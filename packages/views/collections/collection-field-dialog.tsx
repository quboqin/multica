"use client";

import { useEffect, useRef, useState } from "react";
import { GripVertical, Plus, X } from "lucide-react";
import type { CollectionField } from "@multica/core/collections";
import { ISSUE_PROPERTY_TYPES } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { ColorPicker, COLOR_PICKER_PRESETS } from "../common/color-picker";
import { useT } from "../i18n";
import { PropertyTypeLabel } from "../settings/components/properties-tab";
import {
  FieldTypeIcon,
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

/**
 * Creates a field or edits one: name, a safe type change and the option list.
 * Removing an option clears it from every record, so the dialog says so
 * before saving.
 */
export function CollectionFieldDialog({
  open,
  onOpenChange,
  field,
  commands,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  field?: CollectionField | null;
  commands: CollectionCommands;
}) {
  const { t } = useT("issues");
  const [name, setName] = useState("");
  const [type, setType] = useState("text");
  const [options, setOptions] = useState<OptionDraft[]>([]);
  const [error, setError] = useState<string | null>(null);
  const listEnd = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    setError(null);
    setName(field?.name ?? "");
    setType(field?.type ?? "text");
    setOptions(
      field
        ? field.config.options.map((option, index) => ({
            id: option.id,
            name: option.name,
            color: option.color ?? presetColor(index),
          }))
        : [],
    );
  }, [open, field]);

  const typeChoices = field
    ? [field.type, ...safeFieldConversions(field.type)]
    : [...ISSUE_PROPERTY_TYPES];
  const showOptions = fieldHasOptions(type);
  const valid = options.filter((option) => option.name.trim());
  const removed = field
    ? field.config.options.filter(
        (option) => !options.some((draft) => draft.id === option.id),
      )
    : [];
  const pending = commands.createField.isPending || commands.updateField.isPending;
  const canSubmit =
    name.trim().length > 0 && (!showOptions || valid.length > 0) && !pending;

  const addOption = () => {
    setOptions((current) => [
      ...current,
      { name: "", color: presetColor(current.length) },
    ]);
    requestAnimationFrame(() => {
      const inputs = listEnd.current?.querySelectorAll<HTMLInputElement>("input");
      inputs?.[inputs.length - 1]?.focus();
    });
  };
  const submit = async () => {
    if (!canSubmit) return;
    setError(null);
    const config = showOptions
      ? {
          options: valid.map((option) => ({
            ...(option.id ? { id: option.id } : {}),
            name: option.name.trim(),
            color: option.color,
          })),
        }
      : undefined;
    try {
      if (field) {
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
        });
      }
      onOpenChange(false);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : String(cause));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85dvh] flex-col sm:max-w-md">
        <DialogHeader>
          <DialogTitle>
            {field
              ? t(($) => $.cortex_table.edit_field)
              : t(($) => $.cortex_table.new_field)}
          </DialogTitle>
          <DialogDescription>
            {t(($) => $.cortex_table.field_quota_hint)}
          </DialogDescription>
        </DialogHeader>
        <form
          className="-mx-1 min-h-0 flex-1 space-y-4 overflow-y-auto px-1 py-1"
          onSubmit={(event) => {
            event.preventDefault();
            void submit();
          }}
        >
          <div className="grid grid-cols-[minmax(0,1fr)_10rem] gap-3">
            <div className="space-y-2">
              <Label htmlFor="collection-field-name">
                {t(($) => $.cortex_table.field_name)}
              </Label>
              <Input
                id="collection-field-name"
                autoFocus
                maxLength={32}
                value={name}
                onChange={(event) => setName(event.target.value)}
              />
            </div>
            <div className="space-y-2">
              <Label>{t(($) => $.cortex_table.field_type)}</Label>
              <Select
                items={typeChoices.map((value) => ({
                  value,
                  label: <PropertyTypeLabel type={value} />,
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
                <SelectTrigger aria-label={t(($) => $.cortex_table.field_type)}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {typeChoices.map((value) => (
                    <SelectItem key={value} value={value}>
                      <span className="flex items-center gap-2">
                        <FieldTypeIcon type={value} />
                        <PropertyTypeLabel type={value} />
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          {field && typeChoices.length < 2 && (
            <p className="text-caption text-muted-foreground">
              {t(($) => $.cortex_table.type_locked)}
            </p>
          )}
          {showOptions && (
            <div className="space-y-2" ref={listEnd}>
              <Label>{t(($) => $.cortex_table.options)}</Label>
              {options.map((option, index) => (
                <div key={option.id ?? `new-${index}`} className="flex items-center gap-2">
                  <GripVertical className="size-4 shrink-0 text-muted-foreground/50" />
                  <ColorPicker
                    value={option.color}
                    onChange={(color) =>
                      setOptions((current) =>
                        current.map((item, i) => (i === index ? { ...item, color } : item)),
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
                    aria-label={t(($) => $.cortex_table.option_name, { index: index + 1 })}
                    className="h-8"
                    maxLength={32}
                    value={option.name}
                    onChange={(event) =>
                      setOptions((current) =>
                        current.map((item, i) =>
                          i === index ? { ...item, name: event.target.value } : item,
                        ),
                      )
                    }
                    onKeyDown={(event) => {
                      if (event.key === "Enter" && index === options.length - 1 && option.name.trim()) {
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
          <button type="submit" hidden />
        </form>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t(($) => $.cortex_table.cancel)}
          </Button>
          <Button onClick={() => void submit()} disabled={!canSubmit} aria-busy={pending}>
            {field
              ? t(($) => $.cortex_table.save)
              : t(($) => $.cortex_table.create_field)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
