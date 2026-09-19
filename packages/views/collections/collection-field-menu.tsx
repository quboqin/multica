"use client";

import {
  Archive,
  ArrowDown,
  ArrowUp,
  EyeOff,
  Filter,
  Group,
  ListChecks,
  Pencil,
} from "lucide-react";
import type { CollectionField } from "@multica/core/collections";
import {
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@multica/ui/components/ui/dropdown-menu";
import { useT } from "../i18n";
import { PropertyTypeLabel } from "../settings/components/properties-tab";
import {
  FieldTypeIcon,
  ISSUE_PROPERTY_QUOTA,
  MAX_COLLECTION_FIELDS,
  fieldHasOptions,
} from "./collection-fields";

/** The per-field quota line that closes every field menu (FR-022). */
export function FieldQuota({ count }: { count: number }) {
  const { t } = useT("issues");
  return (
    <div className="space-y-1.5 px-2 pb-1.5 pt-2">
      <p className="text-caption text-muted-foreground">
        {t(($) => $.cortex_table.field_quota, {
          count,
          max: MAX_COLLECTION_FIELDS,
          issueQuota: ISSUE_PROPERTY_QUOTA,
        })}
      </p>
      <div className="h-1 overflow-hidden rounded-full bg-muted">
        <div
          className="h-full rounded-full bg-primary"
          style={{ width: `${Math.min(100, (count / MAX_COLLECTION_FIELDS) * 100)}%` }}
        />
      </div>
    </div>
  );
}

/**
 * Menu behind a column header: field settings for managers, then the view
 * actions (sort, group, filter, hide) everyone can use. Archiving keeps the
 * stored values; it only removes the definition from the table.
 */
export function CollectionFieldMenu({
  field,
  fieldCount,
  canManage,
  onEdit,
  onSort,
  onGroup,
  onFilter,
  onHide,
  onArchive,
}: {
  field: CollectionField;
  fieldCount: number;
  canManage: boolean;
  onEdit: () => void;
  onSort: (direction: "asc" | "desc") => void;
  onGroup?: () => void;
  onFilter: () => void;
  onHide: () => void;
  onArchive: () => void;
}) {
  const { t } = useT("issues");
  return (
    <>
      <DropdownMenuGroup>
        <DropdownMenuLabel className="flex items-center gap-2 text-foreground">
          <FieldTypeIcon type={field.type} />
          <span className="truncate">{field.name}</span>
        </DropdownMenuLabel>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      {canManage && (
        <>
          <DropdownMenuItem onClick={onEdit}>
            <Pencil />
            <span className="flex-1">{t(($) => $.cortex_table.field_type)}</span>
            <span className="text-caption text-muted-foreground">
              <PropertyTypeLabel type={field.type} />
            </span>
          </DropdownMenuItem>
          {fieldHasOptions(field.type) && (
            <DropdownMenuItem onClick={onEdit}>
              <ListChecks />
              <span className="flex-1">{t(($) => $.cortex_table.edit_options)}</span>
              <span className="text-caption text-muted-foreground">
                {t(($) => $.cortex_table.option_count, {
                  count: field.config.options.length,
                })}
              </span>
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
        </>
      )}
      <DropdownMenuItem onClick={() => onSort("asc")}>
        <ArrowUp />
        {t(($) => $.cortex_table.sort_ascending)}
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => onSort("desc")}>
        <ArrowDown />
        {t(($) => $.cortex_table.sort_descending)}
      </DropdownMenuItem>
      {onGroup && (
        <DropdownMenuItem onClick={onGroup}>
          <Group />
          {t(($) => $.cortex_table.group_by_field)}
        </DropdownMenuItem>
      )}
      <DropdownMenuItem onClick={onFilter}>
        <Filter />
        {t(($) => $.cortex_table.filter_by_field)}
      </DropdownMenuItem>
      <DropdownMenuSeparator />
      <DropdownMenuItem onClick={onHide}>
        <EyeOff />
        {t(($) => $.cortex_table.hide_in_view)}
      </DropdownMenuItem>
      {canManage && (
        <DropdownMenuItem variant="destructive" onClick={onArchive}>
          <Archive />
          <span className="flex-1">{t(($) => $.cortex_table.archive_field)}</span>
          <span className="text-caption opacity-80">
            {t(($) => $.cortex_table.archive_keeps_data)}
          </span>
        </DropdownMenuItem>
      )}
      <DropdownMenuSeparator />
      <FieldQuota count={fieldCount} />
    </>
  );
}
