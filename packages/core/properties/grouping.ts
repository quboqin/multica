/** Multi-valued fields group by their complete, order-independent combination. */
export function groupValueKey(value: unknown): string {
  if (
    value == null ||
    value === "" ||
    (Array.isArray(value) && value.length === 0)
  )
    return "null";
  return JSON.stringify(Array.isArray(value) ? [...value].sort() : value);
}

export function groupValuesEqual(left: unknown, right: unknown): boolean {
  return groupValueKey(left) === groupValueKey(right);
}

export function propertyGroupLabel(
  value: unknown,
  options: readonly { id: string; name: string }[],
  actorName: (type: "member", id: string) => string,
  type?: string,
): string {
  const label = (item: string | number | boolean) => {
    if (
      (type === "actor" || type === "multi_actor") &&
      typeof item === "string" &&
      item.startsWith("member:")
    )
      return actorName("member", item.slice(7));
    return options.find((option) => option.id === item)?.name ?? String(item);
  };
  if (value == null) return "";
  if (Array.isArray(value)) {
    const items = value.filter(
      (item): item is string => typeof item === "string",
    );
    if (type === "multi_select") {
      const order = new Map(options.map((option, index) => [option.id, index]));
      items.sort(
        (a, b) =>
          (order.get(a) ?? options.length) - (order.get(b) ?? options.length),
      );
    }
    return items.map(label).join(", ");
  }
  return typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
    ? label(value)
    : "";
}
