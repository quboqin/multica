"use client";

import {
  Archive,
  ArrowDown,
  ArrowUp,
  EyeOff,
  Filter,
  Group,
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
import {
  FieldTypeIcon,
  FieldTypeLabel,
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
 * Menu behind the title column. The column is `record.title` rather than a
 * catalog field, so it can be renamed and sorted but not hidden, grouped,
 * filtered or archived.
 */
export function CollectionTitleMenu({
  name,
  canManage,
  onEdit,
  onSort,
}: {
  name: string;
  canManage: boolean;
  onEdit: () => void;
  onSort: (direction: "asc" | "desc") => void;
}) {
  const { t } = useT("issues");
  return (
    <>
      <DropdownMenuGroup>
        <DropdownMenuLabel className="flex items-center gap-2 text-foreground">
          <FieldTypeIcon type="text" />
          <span className="truncate">{name}</span>
        </DropdownMenuLabel>
      </DropdownMenuGroup>
      <DropdownMenuSeparator />
      {canManage && (
        <>
          <DropdownMenuItem onClick={onEdit}>
            <Pencil />
            <span className="flex-1">{t(($) => $.cortex_table.edit_field)}</span>
            <span className="text-caption text-muted-foreground">
              <FieldTypeLabel type="text" />
            </span>
          </DropdownMenuItem>
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
    </>
  );
}

/**
 * Menu behind a column header: "Edit field" for managers (it opens the field
 * panel under this column), then the view actions (sort, group, filter, hide)
 * everyone can use. A field the record query cannot read — a relation — leaves
 * sort and filter out. Archiving keeps the stored values; it only removes the
 * definition from the table.
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
  onSort?: (direction: "asc" | "desc") => void;
  onGroup?: () => void;
  onFilter?: () => void;
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
            <span className="flex-1">{t(($) => $.cortex_table.edit_field)}</span>
            <span className="text-caption text-muted-foreground">
              <FieldTypeLabel type={field.type} />
              {fieldHasOptions(field.type) &&
                ` · ${t(($) => $.cortex_table.option_count, {
                  count: field.config.options.length,
                })}`}
            </span>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
        </>
      )}
      {onSort && (
        <>
          <DropdownMenuItem onClick={() => onSort("asc")}>
            <ArrowUp />
            {t(($) => $.cortex_table.sort_ascending)}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => onSort("desc")}>
            <ArrowDown />
            {t(($) => $.cortex_table.sort_descending)}
          </DropdownMenuItem>
        </>
      )}
      {onGroup && (
        <DropdownMenuItem onClick={onGroup}>
          <Group />
          {t(($) => $.cortex_table.group_by_field)}
        </DropdownMenuItem>
      )}
      {onFilter && (
        <DropdownMenuItem onClick={onFilter}>
          <Filter />
          {t(($) => $.cortex_table.filter_by_field)}
        </DropdownMenuItem>
      )}
      {(onSort || onGroup || onFilter) && <DropdownMenuSeparator />}
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
