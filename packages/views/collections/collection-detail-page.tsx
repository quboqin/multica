"use client";
import { useCallback, useMemo, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ColumnDef, ColumnSizingState } from "@tanstack/react-table";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import {
  collectionDetailOptions,
  collectionKeys,
  collectionRecordsOptions,
  collectionRecordOptions,
  type CollectionField,
  type CollectionRecord,
  type CollectionQuery,
} from "@multica/core/collections";
import {
  calendarDate,
  calendarDays,
  dataSourceIdentityKey,
  defaultDataViewPreferences,
  parseDataViewPreferences,
  useDataViewPreferences,
  type DataSourceField,
  type DataSourceFieldKind,
} from "@multica/core/data-source";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import type { IssueView } from "@multica/core/api/schemas";
import { ISSUE_PROPERTY_TYPES } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  DataFieldEditor,
  DataViewCalendar,
  DataViewGallery,
  DataViewTable,
  useFrozenRows,
} from "../data-view";
import { useT } from "../i18n";

const capabilities = {
  layouts: ["table", "calendar", "gallery"],
  editing: "adapter",
  sideEffects: "none",
  sorting: "none",
} as const;
export function CollectionDetailPage({
  id,
  savedView,
}: {
  id: string;
  savedView?: IssueView;
}) {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { t } = useT("issues");
  const { t: tSettings } = useT("settings");
  const { data, error } = useQuery(collectionDetailOptions(wsId, id));
  const userId = useAuthStore((state) => state.user?.id);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const role = members.find((member) => member.user_id === userId)?.role;
  const canManageFields =
    !!userId &&
    (data?.collection.created_by === userId ||
      role === "owner" ||
      role === "admin");
  const savedPreferences = useMemo(
    () =>
      savedView
        ? parseDataViewPreferences(savedView.display)
        : defaultDataViewPreferences,
    [savedView],
  );
  const identity = useMemo(
    () => ({
      workspaceId: wsId,
      namespace: "collections",
      sourceId: savedView?.id ?? id,
    }),
    [wsId, id, savedView?.id],
  );
  const source = dataSourceIdentityKey(identity);
  const prefs = useDataViewPreferences(
    (state) => state.bySource[source] ?? savedPreferences,
  );
  const update = useDataViewPreferences((state) => state.update);
  const [search, setSearch] = useState(
    typeof savedView?.query.search === "string" ? savedView.query.search : "",
  );
  const [savedProperties, setSavedProperties] = useState<
    CollectionQuery["properties"]
  >(
    savedView?.query.properties &&
      typeof savedView.query.properties === "object"
      ? Object.fromEntries(
          Object.entries(savedView.query.properties).filter(([, value]) =>
            Array.isArray(value),
          ),
        )
      : undefined,
  );
  const [title, setTitle] = useState("");
  const [fieldName, setFieldName] = useState("");
  const [fieldType, setFieldType] = useState("text");
  const [options, setOptions] = useState("");
  const [anchor, setAnchor] = useState(() => calendarDate(new Date()));
  const [filterField, setFilterField] = useState("");
  const [filterOp, setFilterOp] = useState("exact");
  const [filterValue, setFilterValue] = useState("");
  const fields = data?.fields ?? [];
  const dateField =
    fields.find(
      (field) => field.id === prefs.dateField && field.type === "date",
    ) ?? fields.find((field) => field.type === "date");
  const query: CollectionQuery = {
    search,
    properties: savedProperties,
    group_by: prefs.layout === "table" ? prefs.groupBy || undefined : undefined,
  };
  if (filterField && filterValue) {
    const field = fields.find((field) => field.id === filterField);
    query.properties = {
      [filterField]: [
        filterOp === "exact"
          ? field?.type === "number"
            ? Number(filterValue)
            : field?.type === "checkbox"
              ? filterValue === "true"
              : filterValue
          : { op: filterOp, value: filterValue },
      ],
    };
  }
  if (prefs.layout === "calendar" && dateField) {
    const days = calendarDays(anchor, prefs.period);
    query.date_field = dateField.id;
    query.date_start = days[0];
    query.date_end = days[days.length - 1];
  }
  const overview = useInfiniteQuery(collectionRecordsOptions(wsId, id, query));
  const command = useMutation({
    mutationFn: (fn: () => Promise<unknown>) => fn(),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) }),
  });
  const displayed = (row: CollectionRecord, field: CollectionField) => {
    const value = row.fields[field.id];
    if (field.type === "select")
      return (
        field.config.options.find((option) => option.id === value)?.name ??
        value
      );
    return value;
  };
  const galleryFields: DataSourceField<CollectionRecord>[] = fields
    .filter((field) => prefs.displayedFields.includes(field.id))
    .map((field) => ({
      id: field.id,
      label: field.name,
      kind: field.type as DataSourceFieldKind,
      value: (row) => displayed(row, field),
    }));
  const rows = overview.data?.pages.flatMap((page) => page.records) ?? [];
  const views = useQuery({
    queryKey: ["issue-views", wsId, "collection", id],
    queryFn: () =>
      api.listIssueViews({
        scope_type: data?.collection.project_id ? "project" : "workspace",
        scope_id: data?.collection.project_id,
        collection_id: id,
      }),
    enabled: !!data,
  });
  const [viewName, setViewName] = useState("");
  return (
    <main className="flex min-h-0 flex-1 flex-col">
      <header className="space-y-3 border-b p-3">
        <h1 className="text-body font-medium">
          {data?.collection.name ?? t(($) => $.cortex.loading)}
        </h1>
        <div className="flex flex-wrap items-center gap-2">
          {(["table", "calendar", "gallery"] as const).map((layout) => (
            <Button
              key={layout}
              variant={prefs.layout === layout ? "secondary" : "outline"}
              size="sm"
              onClick={() => update(source, { layout })}
            >
              {layout === "table"
                ? t(($) => $.view.table)
                : t(($) => $.cortex[layout])}
            </Button>
          ))}
          <Input
            className="max-w-60"
            aria-label={t(($) => $.cortex.search_records)}
            placeholder={t(($) => $.cortex.search_records)}
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
          <select
            aria-label={t(($) => $.cortex.group_by)}
            value={prefs.groupBy}
            onChange={(event) =>
              update(source, { groupBy: event.target.value })
            }
            className="rounded border bg-background p-1"
          >
            <option value="">{t(($) => $.cortex.no_group)}</option>
            {fields
              .filter((field) => field.type === "select")
              .map((field) => (
                <option value={field.id} key={field.id}>
                  {field.name}
                </option>
              ))}
          </select>
          <select
            aria-label={t(($) => $.cortex.field_name)}
            value={filterField}
            onChange={(event) => {
              setSavedProperties(undefined);
              setFilterField(event.target.value);
            }}
            className="rounded border bg-background p-1"
          >
            <option value="">—</option>
            {fields.map((field) => (
              <option key={field.id} value={field.id}>
                {field.name}
              </option>
            ))}
          </select>
          {filterField && (
            <>
              <select
                aria-label={t(($) => $.cortex.filter_operator)}
                className="rounded border bg-background p-1"
                value={filterOp}
                onChange={(event) => setFilterOp(event.target.value)}
              >
                {[
                  "exact",
                  "contains",
                  "gt",
                  "gte",
                  "lt",
                  "lte",
                  "before",
                  "after",
                ].map((op) => (
                  <option key={op}>{op}</option>
                ))}
              </select>
              <Input
                className="max-w-40"
                aria-label={t(($) => $.cortex.filter_value)}
                value={filterValue}
                onChange={(event) => setFilterValue(event.target.value)}
              />
            </>
          )}
        </div>
        <div className="flex flex-wrap gap-2">
          <Input
            className="max-w-60"
            aria-label={t(($) => $.cortex.record_title)}
            placeholder={t(($) => $.cortex.record_title)}
            value={title}
            onChange={(event) => setTitle(event.target.value)}
          />
          <Button
            size="sm"
            disabled={command.isPending || !title.trim()}
            onClick={() =>
              command.mutate(async () => {
                await api.createCollectionRecord(id, title);
                setTitle("");
              })
            }
          >
            {t(($) => $.cortex.add_record)}
          </Button>
          {canManageFields && (
            <>
              <Input
                className="max-w-40"
                aria-label={t(($) => $.cortex.field_name)}
                placeholder={t(($) => $.cortex.field_name)}
                value={fieldName}
                onChange={(event) => setFieldName(event.target.value)}
              />
              <select
                className="rounded border bg-background p-1"
                aria-label={t(($) => $.cortex.field_type)}
                value={fieldType}
                onChange={(event) => setFieldType(event.target.value)}
              >
                {ISSUE_PROPERTY_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {tSettings(($) => $.properties.types[type])}
                  </option>
                ))}
              </select>
              {fieldType.includes("select") && (
                <Input
                  className="max-w-60"
                  aria-label={t(($) => $.cortex.options)}
                  placeholder={t(($) => $.cortex.options)}
                  value={options}
                  onChange={(event) => setOptions(event.target.value)}
                />
              )}
              <Button
                size="sm"
                disabled={!fieldName.trim() || command.isPending}
                onClick={() =>
                  command.mutate(async () => {
                    await api.createCollectionField(id, {
                      name: fieldName,
                      type: fieldType,
                      config: fieldType.includes("select")
                        ? {
                            options: options
                              .split(/[,，]/)
                              .filter(Boolean)
                              .map((name) => ({
                                name: name.trim(),
                                color: "#6b7280",
                              })),
                          }
                        : undefined,
                    });
                    setFieldName("");
                  })
                }
              >
                {t(($) => $.cortex.create_field)}
              </Button>
            </>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="max-w-40"
            aria-label={t(($) => $.cortex.view_name)}
            placeholder={t(($) => $.cortex.view_name)}
            value={viewName}
            onChange={(event) => setViewName(event.target.value)}
          />
          <Button
            size="sm"
            disabled={!viewName.trim() || command.isPending}
            onClick={() =>
              command.mutate(async () => {
                await api.createIssueView({
                  name: viewName,
                  scope_type: data?.collection.project_id
                    ? "project"
                    : "workspace",
                  scope_id: data?.collection.project_id,
                  collection_id: id,
                  visibility: "private",
                  definition_version: 1,
                  query: { ...query },
                  display: { ...prefs },
                });
                setViewName("");
                await views.refetch();
              })
            }
          >
            {t(($) => $.cortex.save_view)}
          </Button>
          {views.data?.map((view) => (
            <Button
              size="sm"
              variant="ghost"
              key={view.id}
              onClick={() => {
                const display = view.display;
                if (
                  display.layout === "table" ||
                  display.layout === "calendar" ||
                  display.layout === "gallery"
                )
                  update(source, {
                    layout: display.layout,
                    period: display.period === "week" ? "week" : "month",
                    dateField:
                      typeof display.dateField === "string"
                        ? display.dateField
                        : "",
                    coverField:
                      typeof display.coverField === "string"
                        ? display.coverField
                        : "",
                    displayedFields: Array.isArray(display.displayedFields)
                      ? display.displayedFields.filter(
                          (v): v is string => typeof v === "string",
                        )
                      : [],
                    groupBy:
                      typeof display.groupBy === "string"
                        ? display.groupBy
                        : "",
                  });
                setSavedProperties(undefined);
                const filters = view.query.properties;
                const entry =
                  filters && typeof filters === "object"
                    ? Object.entries(filters)[0]
                    : undefined;
                const value: unknown =
                  entry && Array.isArray(entry[1]) ? entry[1][0] : undefined;
                setFilterField(entry?.[0] ?? "");
                setFilterOp(
                  value && typeof value === "object" && "op" in value
                    ? String(value.op)
                    : "exact",
                );
                setFilterValue(
                  value && typeof value === "object" && "value" in value
                    ? String(value.value)
                    : value == null
                      ? ""
                      : String(value),
                );
                setSearch(
                  typeof view.query.search === "string"
                    ? view.query.search
                    : "",
                );
              }}
            >
              {view.name}
            </Button>
          ))}
        </div>
        {prefs.layout === "calendar" && (
          <label>
            {t(($) => $.cortex.date_field)}{" "}
            <select
              className="rounded border bg-background p-1"
              value={dateField?.id ?? ""}
              onChange={(event) =>
                update(source, { dateField: event.target.value })
              }
            >
              <option value="">—</option>
              {fields
                .filter((field) => field.type === "date")
                .map((field) => (
                  <option key={field.id} value={field.id}>
                    {field.name}
                  </option>
                ))}
            </select>
          </label>
        )}
        {prefs.layout === "gallery" && (
          <div className="flex flex-wrap gap-3">
            <label>
              {t(($) => $.cortex.cover)}{" "}
              <select
                className="rounded border bg-background p-1"
                value={prefs.coverField}
                onChange={(event) =>
                  update(source, { coverField: event.target.value })
                }
              >
                <option value="">—</option>
                {fields
                  .filter((field) => field.type === "url")
                  .map((field) => (
                    <option key={field.id} value={field.id}>
                      {field.name}
                    </option>
                  ))}
              </select>
            </label>
            {fields.map((field) => (
              <label key={field.id}>
                <input
                  type="checkbox"
                  checked={prefs.displayedFields.includes(field.id)}
                  onChange={(event) =>
                    update(source, {
                      displayedFields: event.target.checked
                        ? [...prefs.displayedFields, field.id]
                        : prefs.displayedFields.filter((id) => id !== field.id),
                    })
                  }
                />{" "}
                {field.name}
              </label>
            ))}
          </div>
        )}
        {(error || overview.error || command.error) && (
          <p role="alert">
            {(error || overview.error || command.error)?.message}
          </p>
        )}
      </header>
      <div className="min-h-0 flex-1 overflow-auto">
        {prefs.layout === "table" ? (
          prefs.groupBy ? (
            [
              ...(fields
                .find((field) => field.id === prefs.groupBy)
                ?.config.options.map((option) => option.id) ?? []),
              "__none__",
            ]
              .map((key) => ({
                key,
                count:
                  overview.data?.pages[0]?.groups.find(
                    (group) => group.key === key,
                  )?.count ?? 0,
              }))
              .map((group) => (
                <section key={group.key}>
                  <h2 className="px-3 py-2 text-body-sm font-medium">
                    {fields
                      .find((field) => field.id === prefs.groupBy)
                      ?.config.options.find((option) => option.id === group.key)
                      ?.name ?? "—"}{" "}
                    · {group.count}
                  </h2>
                  <RecordTable
                    id={id}
                    fields={fields}
                    query={{ ...query, group_key: group.key }}
                  />
                </section>
              ))
          ) : (
            <RecordTable id={id} fields={fields} query={query} />
          )
        ) : prefs.layout === "gallery" ? (
          <DataViewGallery
            rows={rows}
            rowId={(row) => row.id}
            title={(row) => row.title}
            fields={galleryFields}
            capabilities={capabilities}
            cover={(row) => {
              const value = row.fields[prefs.coverField];
              return typeof value === "string" ? value : null;
            }}
          />
        ) : dateField ? (
          <DataViewCalendar
            rows={rows}
            rowId={(row) => row.id}
            title={(row) => row.title}
            date={(row) => {
              const value = row.fields[dateField.id];
              return typeof value === "string" ? value : null;
            }}
            anchor={anchor}
            period={prefs.period}
            onAnchorChange={setAnchor}
            onPeriodChange={(period) => update(source, { period })}
            onMoveDate={(row, value) =>
              command.mutate(() =>
                api.setCollectionRecordField(
                  id,
                  row.id,
                  dateField.id,
                  value,
                  row.fields[dateField.id],
                ),
              )
            }
            capabilities={capabilities}
            labels={{
              month: t(($) => $.cortex.month),
              week: t(($) => $.cortex.week),
              previous: t(($) => $.cortex.previous),
              next: t(($) => $.cortex.next),
              date: t(($) => $.cortex.date_field),
              unscheduled: t(($) => $.cortex.unscheduled),
            }}
          />
        ) : (
          <p className="p-4">
            {t(($) => $.cortex.date_field)}: {t(($) => $.cortex.create_field)}
          </p>
        )}
        {prefs.layout !== "table" && overview.hasNextPage && (
          <Button
            className="m-3"
            variant="outline"
            disabled={overview.isFetching}
            onClick={() => void overview.fetchNextPage()}
          >
            {t(($) => $.cortex.load_more)}
          </Button>
        )}
      </div>
    </main>
  );
}

function RecordTable({
  id,
  fields,
  query,
}: {
  id: string;
  fields: CollectionField[];
  query: CollectionQuery;
}) {
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { t } = useT("issues");
  const pages = useInfiniteQuery(collectionRecordsOptions(wsId, id, query));
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const [editing, setEditing] = useState<string | null>(null);
  const focused = useQuery(
    collectionRecordOptions(wsId, id, editing?.split(":")[0] ?? null),
  );
  const [sizing, setSizing] = useState<ColumnSizingState>({ title: 260 });
  const write = useCallback(
    async (action: () => Promise<unknown>) => {
      try {
        return await action();
      } finally {
        await qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
      }
    },
    [qc, wsId],
  );
  const columns = useMemo<ColumnDef<CollectionRecord>[]>(
    () => [
      {
        id: "title",
        header: t(($) => $.cortex.record_title),
        accessorKey: "title",
        cell: ({ row }) => (
          <DataFieldEditor
            labels={{
              current: t(($) => $.cortex.current_value),
              retry: t(($) => $.cortex.retry_field),
              discard: t(($) => $.cortex.discard_field),
            }}
            label={`${t(($) => $.cortex.record_title)}: ${row.original.title}`}
            onEditingChange={(active) =>
              setEditing(active ? `${row.original.id}:title` : null)
            }
            kind="text"
            value={row.original.title}
            save={(value, base) =>
              write(() =>
                api.updateCollectionRecord(
                  id,
                  row.original.id,
                  String(value ?? ""),
                  String(base ?? ""),
                ),
              )
            }
          />
        ),
      },
      ...fields.map((field) => ({
        id: field.id,
        header: field.name,
        accessorFn: (row: CollectionRecord) => row.fields[field.id],
        cell: ({ row }: { row: { original: CollectionRecord } }) => (
          <DataFieldEditor
            labels={{
              current: t(($) => $.cortex.current_value),
              retry: t(($) => $.cortex.retry_field),
              discard: t(($) => $.cortex.discard_field),
            }}
            label={`${field.name}: ${row.original.title}`}
            onEditingChange={(active) =>
              setEditing(active ? `${row.original.id}:${field.id}` : null)
            }
            kind={
              ISSUE_PROPERTY_TYPES.includes(field.type as never)
                ? (field.type as DataSourceFieldKind)
                : "readonly"
            }
            value={row.original.fields[field.id]}
            options={
              field.type.includes("actor")
                ? members.map((member) => ({
                    id: `member:${member.user_id}`,
                    label: member.name,
                  }))
                : field.config.options.map((option) => ({
                    id: option.id,
                    label: option.name,
                  }))
            }
            save={(value, base) =>
              write(() =>
                api.setCollectionRecordField(
                  id,
                  row.original.id,
                  field.id,
                  value,
                  base,
                ),
              )
            }
          />
        ),
      })),
    ],
    [fields, id, members, t, write],
  );
  const loaded = pages.data?.pages.flatMap((page) => page.records) ?? [];
  const rows = useFrozenRows(
    {
      workspaceId: wsId,
      namespace: "collection",
      sourceId: `${id}:${query.group_key ?? ""}`,
    },
    loaded,
    editing,
    (snapshot) => {
      const byId = new Map(loaded.map((row) => [row.id, row]));
      if (focused.data) {
        const visible = byId.get(focused.data.id);
        if (!visible || visible.revision <= focused.data.revision)
          byId.set(focused.data.id, focused.data);
      }
      return snapshot.map((row) => byId.get(row.id) ?? row);
    },
  );
  return (
    <div className="min-h-48">
      <DataViewTable
        sourceIdentity={{
          workspaceId: wsId,
          namespace: "collection",
          sourceId: `${id}:${query.group_key ?? ""}`,
        }}
        rows={rows}
        rowId={(row) => row.id}
        columns={columns}
        visibleColumnIds={["title", ...fields.map((field) => field.id)]}
        columnSizing={sizing}
        onColumnSizingChange={setSizing}
        onReorderColumn={() => undefined}
        capabilities={capabilities}
      />
      {pages.error && <p role="alert">{pages.error.message}</p>}
      {pages.hasNextPage && (
        <Button
          className="m-3"
          variant="outline"
          disabled={pages.isFetching}
          onClick={() => void pages.fetchNextPage()}
        >
          {t(($) => $.cortex.load_more)}
        </Button>
      )}
    </div>
  );
}

export function EmbeddedCollectionView({
  id,
  view,
}: {
  id: string;
  view: IssueView;
}) {
  return (
    <div className="flex h-[420px] min-h-0 overflow-auto">
      <CollectionDetailPage
        key={`${view.id}:${view.revision}`}
        id={id}
        savedView={view}
      />
    </div>
  );
}
