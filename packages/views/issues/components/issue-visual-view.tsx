"use client";
import { useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  calendarDate,
  calendarDays,
  dataSourceIdentityKey,
  defaultDataViewPreferences,
  useDataViewPreferences,
  type DataSourceField,
} from "@multica/core/data-source";
import { issueKeys } from "@multica/core/issues/queries";
import { propertyListOptions } from "@multica/core/properties";
import { useWorkspaceId } from "@multica/core/hooks";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Issue, IssueTableQuerySpec } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { CalendarDays, Eye, Image } from "lucide-react";
import {
  DataViewCalendar,
  DataViewChoiceChip,
  DataViewFieldToggles,
  DataViewGallery,
} from "../../data-view";
import { useNavigation } from "../../navigation";
import { useLocale, useT } from "../../i18n";

const capabilities = {
  layouts: ["calendar", "gallery"],
  editing: "adapter",
  sideEffects: "adapter",
  sorting: "adapter",
} as const;
export function IssueVisualView({
  mode,
  query,
}: {
  mode: "calendar" | "gallery";
  query: IssueTableQuerySpec;
}) {
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const nav = useNavigation();
  const { t } = useT("issues");
  const locale = useLocale();
  const qc = useQueryClient();
  const source = dataSourceIdentityKey({
    workspaceId: wsId,
    namespace: "issues",
    sourceId: JSON.stringify(query.scope),
  });
  const prefs = useDataViewPreferences(
    (state) => state.bySource[source] ?? defaultDataViewPreferences,
  );
  const update = useDataViewPreferences((state) => state.update);
  const [anchor, setAnchor] = useState(() => calendarDate(new Date()));
  const days = calendarDays(anchor, prefs.period);
  const { data: properties = [] } = useQuery(propertyListOptions(wsId));
  const fields: DataSourceField<Issue>[] = [
    {
      id: "due_date",
      label: t(($) => $.detail.prop_due_date),
      kind: "date",
      value: (row) => row.due_date,
    },
    {
      id: "start_date",
      label: t(($) => $.detail.prop_start_date),
      kind: "date",
      value: (row) => row.start_date,
    },
    ...properties.map((property) => ({
      id: `property:${property.id}`,
      label: property.name,
      kind: property.type as DataSourceField<Issue>["kind"],
      value: (row: Issue) => row.properties?.[property.id],
    })),
  ];
  const dateField =
    fields.find(
      (field) => field.id === prefs.dateField && field.kind === "date",
    ) ?? fields[0]!;
  const calendarQuery: IssueTableQuerySpec =
    mode === "calendar"
      ? {
          ...query,
          filters: {
            ...query.filters,
            calendar: {
              field: dateField.id as
                | "due_date"
                | "start_date"
                | `property:${string}`,
              start: days[0]!,
              end: days[days.length - 1]!,
            },
          },
        }
      : query;
  const pages = useInfiniteQuery({
    queryKey: [...issueKeys.tableAll(wsId), "visual", mode, calendarQuery],
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      api.listIssueTableRows(
        {
          query: calendarQuery,
          group: { kind: "none" },
          group_key: null,
          hierarchy: { enabled: false },
          parent_id: null,
          page: { limit: 100, cursor: pageParam },
        },
        { workspaceId: wsId, signal },
      ),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  });
  const rows =
    pages.data?.pages.flatMap((page) => page.rows.map((row) => row.issue)) ??
    [];
  const mutation = useMutation({
    mutationFn: async ({
      row,
      value,
    }: {
      row: Issue;
      value: string | null;
    }) => {
      if (dateField.id.startsWith("property:")) {
        const id = dateField.id.slice(9);
        return value === null
          ? api.unsetIssueProperty(row.id, id)
          : api.setIssueProperty(row.id, id, value);
      }
      return api.updateIssue(row.id, { [dateField.id]: value });
    },
    onSettled: () => qc.invalidateQueries({ queryKey: issueKeys.all(wsId) }),
  });
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      {(pages.error || mutation.error) && (
        <p role="alert">{(pages.error || mutation.error)?.message}</p>
      )}
      {mode === "calendar" ? (
        <DataViewCalendar
          rows={rows}
          rowId={(row) => row.id}
          title={(row) => row.title}
          date={(row) => {
            const value = dateField.value(row);
            return typeof value === "string" ? value : null;
          }}
          anchor={anchor}
          period={prefs.period}
          onAnchorChange={setAnchor}
          onPeriodChange={(period) => update(source, { period })}
          onMoveDate={(row, value) => mutation.mutate({ row, value })}
          onOpen={(row) => nav.push(paths.issueDetail(row.identifier))}
          locale={locale}
          capabilities={capabilities}
          toolbar={
            <DataViewChoiceChip
              icon={<CalendarDays className="size-3.5" />}
              label={t(($) => $.cortex.date_field)}
              options={fields
                .filter((field) => field.kind === "date")
                .map((field) => ({ id: field.id, label: field.label }))}
              value={dateField.id}
              onChange={(value) => update(source, { dateField: value })}
            />
          }
          labels={{
            month: t(($) => $.cortex.month),
            week: t(($) => $.cortex.week),
            previous: t(($) => $.cortex.previous),
            next: t(($) => $.cortex.next),
            date: t(($) => $.cortex.date_field),
            unscheduled: t(($) => $.cortex.unscheduled),
            today: t(($) => $.cortex_table.today),
          }}
        />
      ) : (
        <DataViewGallery
          rows={rows}
          rowId={(row) => row.id}
          title={(row) => row.title}
          fields={fields.filter((field) =>
            prefs.displayedFields.includes(field.id),
          )}
          cover={(row) => {
            const value = fields
              .find((field) => field.id === prefs.coverField)
              ?.value(row);
            return typeof value === "string" ? value : null;
          }}
          onOpen={(row) => nav.push(paths.issueDetail(row.identifier))}
          capabilities={capabilities}
          toolbar={
            <>
              <DataViewChoiceChip
                icon={<Image className="size-3.5" />}
                label={t(($) => $.cortex.cover)}
                options={fields
                  .filter((field) => field.kind === "url")
                  .map((field) => ({ id: field.id, label: field.label }))}
                value={prefs.coverField}
                emptyLabel={t(($) => $.cortex_table.no_cover)}
                onChange={(value) => update(source, { coverField: value })}
              />
              <DataViewFieldToggles
                icon={<Eye className="size-3.5" />}
                label={t(($) => $.cortex_table.card_fields)}
                options={fields.map((field) => ({ id: field.id, label: field.label }))}
                selected={prefs.displayedFields}
                onChange={(displayedFields) => update(source, { displayedFields })}
              />
            </>
          }
        />
      )}
      <div className="px-4 pb-3 text-caption text-muted-foreground" role="status">
        {rows.length} / {pages.data?.pages[0]?.total ?? "…"}{" "}
        {pages.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            disabled={pages.isFetching}
            onClick={() => void pages.fetchNextPage()}
          >
            {t(($) => $.cortex.load_more)}
          </Button>
        )}
      </div>
    </div>
  );
}
