"use client";

import type { CollectionField } from "@multica/core/collections";
import { RecordLinkSchema } from "@multica/core/collections";
import { propertyGroupLabel } from "@multica/core/properties";
import { useActorName } from "@multica/core/workspace/hooks";
import { useT } from "../i18n";

export function collectionGroupValue(
  field: CollectionField,
  key: string,
): unknown {
  if (key === "__none__") return null;
  if (field.type === "select") return key;
  if (!key.startsWith("value:")) return null;
  try {
    return JSON.parse(key.slice(6));
  } catch {
    return null;
  }
}

export function collectionGroups(
  field: CollectionField,
  groups: { key: string; count: number; value?: unknown }[],
) {
  const keys =
    field.type === "select"
      ? [...field.config.options.map((option) => option.id), "__none__"]
      : [...groups.map((group) => group.key), "__none__"];
  return [...new Set([...keys, ...groups.map((group) => group.key)])].map(
    (key) => ({
      key,
      count: groups.find((group) => group.key === key)?.count ?? 0,
      value: groups.find((group) => group.key === key)?.value,
    }),
  );
}

export function useCollectionGroupLabel(
  field: CollectionField,
  group: { key: string; value?: unknown },
): string {
  const { t } = useT("issues");
  const { getActorName } = useActorName();
  const value = collectionGroupValue(field, group.key);
  if (value == null) return t(($) => $.cortex_table.no_value);
  if (field.type === "relation") {
    const parsed = RecordLinkSchema.array().safeParse(group.value);
    return parsed.success
      ? parsed.data
          .map((link) =>
            link.missing
              ? t(($) => $.table.value_unavailable)
              : link.identifier || link.title,
          )
          .join(", ")
      : t(($) => $.table.value_unavailable);
  }
  if (typeof value === "boolean")
    return value
      ? t(($) => $.pickers.custom_property.true_label)
      : t(($) => $.pickers.custom_property.false_label);
  return propertyGroupLabel(
    value,
    field.config.options,
    getActorName,
    field.type,
  );
}

export function CollectionGroupLabel({
  field,
  group,
}: {
  field: CollectionField;
  group: { key: string; value?: unknown };
}) {
  return <>{useCollectionGroupLabel(field, group)}</>;
}
