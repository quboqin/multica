"use client";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link2, Plus, Save, Search } from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import {
  collectionDetailOptions,
  collectionRecordsOptions,
  type CollectionField,
  type CollectionQuery,
  type CollectionRecord,
} from "@multica/core/collections";
import {
  calendarDate,
  calendarDays,
  dataSourceIdentityKey,
  defaultDataViewPreferences,
  parseDataViewPreferences,
  useDataViewPreferences,
  type DataSourceField,
  type DataViewFilter,
  type DataViewPreferences,
} from "@multica/core/data-source";
import { useWorkspaceId } from "@multica/core/hooks";
import { issueViewKeys } from "@multica/core/issue-views/queries";
import { useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import { memberListOptions } from "@multica/core/workspace/queries";
import type { IssueView } from "@multica/core/api/schemas";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { Switch } from "@multica/ui/components/ui/switch";
import { copyText } from "@multica/ui/lib/clipboard";
import { cn } from "@multica/ui/lib/utils";
import { CollectionNavigator } from "../cortex";
import { DataViewCalendar, DataViewGallery } from "../data-view";
import { AppLink, useNavigation } from "../navigation";
import { useLocale, useT } from "../i18n";
import { CollectionBoard } from "./collection-board";
import {
  CollectionFieldPanel,
  type FieldPanelTarget,
} from "./collection-field-panel";
import { CollectionValue } from "./collection-cell";
import { fieldText, recordValue } from "./collection-fields";
import { FieldQuota } from "./collection-field-menu";
import { CollectionRecordPanel } from "./collection-record-panel";
import { CollectionTable, type CollectionTableActions } from "./collection-table";
import { CollectionTrash } from "./collection-trash";
import {
  DateFieldChip,
  DisplayPopover,
  FilterPopover,
  GroupPopover,
  LayoutSwitch,
  SortPopover,
  filtersToProperties,
} from "./collection-view-bar";
import { useCollectionCommands } from "./use-collection-commands";

const visualCapabilities = {
  layouts: ["calendar", "gallery"],
  editing: "adapter",
  sideEffects: "none",
  sorting: "adapter",
} as const;

/** Reads a saved view, including filters stored by earlier builds in its query. */
export function savedViewPreferences(view: IssueView): DataViewPreferences {
  const prefs = parseDataViewPreferences(view.display);
  if (prefs.filters.length === 0 && view.query.properties && typeof view.query.properties === "object") {
    const legacy: DataViewFilter[] = [];
    for (const [field, values] of Object.entries(view.query.properties)) {
      const first: unknown = Array.isArray(values) ? values[0] : undefined;
      if (first === undefined) continue;
      if (first && typeof first === "object" && "op" in first)
        legacy.push({
          field,
          op: String((first as { op: unknown }).op),
          value: (first as { value?: unknown }).value ?? "",
        });
      else legacy.push({ field, op: "exact", value: Array.isArray(first) ? first[0] : first });
    }
    prefs.filters = legacy;
  }
  if (!prefs.sortBy && typeof view.query.sort_by === "string") {
    prefs.sortBy = view.query.sort_by;
    prefs.sortDir = view.query.sort_dir === "desc" ? "desc" : "asc";
  }
  return prefs;
}

export function CollectionDetailPage({
  id,
  savedView,
}: {
  id: string;
  /** Renders one saved view without page chrome (document embeds). */
  savedView?: IssueView;
}) {
  const embedded = !!savedView;
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const { t } = useT("issues");
  const locale = useLocale();
  const { data, error } = useQuery(collectionDetailOptions(wsId, id));
  const userId = useAuthStore((state) => state.user?.id);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: projects = [] } = useQuery({
    ...projectListOptions(wsId),
    enabled: !embedded && !!wsId,
  });
  const role = members.find((member) => member.user_id === userId)?.role;
  const canManage =
    !!userId &&
    (data?.collection.created_by === userId || role === "owner" || role === "admin");
  const fieldsRef = useRef<CollectionField[]>([]);
  fieldsRef.current = data?.fields ?? [];
  const describeValue = useCallback((fieldId: string, value: unknown) => {
    const field = fieldsRef.current.find((item) => item.id === fieldId);
    return (field ? fieldText(field, value) : "") || "—";
  }, []);
  const commands = useCollectionCommands(wsId, id, describeValue);

  const viewScope = useMemo(
    () => ({
      scope_type: (data?.collection.project_id ? "project" : "workspace") as
        | "project"
        | "workspace",
      scope_id: data?.collection.project_id ?? null,
      collection_id: id,
    }),
    [data?.collection.project_id, id],
  );
  const views = useQuery({
    queryKey: issueViewKeys.list(wsId, viewScope),
    queryFn: () => api.listIssueViews(viewScope),
    enabled: !!data && !embedded,
  });
  const [activeViewId, setActiveViewId] = useState<string | null>(null);
  const activeView =
    savedView ?? views.data?.find((view) => view.id === activeViewId) ?? null;
  const baseline = useMemo(
    () => (activeView ? savedViewPreferences(activeView) : defaultDataViewPreferences),
    [activeView],
  );
  const source = dataSourceIdentityKey({
    workspaceId: wsId,
    namespace: "collections",
    sourceId: activeView ? `${activeView.id}:${activeView.revision}` : id,
  });
  const stored = useDataViewPreferences((state) => state.bySource[source]);
  const prefs = useMemo<DataViewPreferences>(
    () => ({ ...baseline, ...stored }),
    [baseline, stored],
  );
  const updatePrefs = useDataViewPreferences((state) => state.update);
  const update = useCallback(
    (patch: Partial<DataViewPreferences>) =>
      updatePrefs(source, { ...prefs, ...patch }),
    [updatePrefs, source, prefs],
  );
  const dirty =
    !!activeView && !!stored && JSON.stringify(prefs) !== JSON.stringify(baseline);

  const [search, setSearch] = useState("");
  useEffect(() => {
    setSearch(typeof activeView?.query.search === "string" ? activeView.query.search : "");
  }, [activeView]);
  const [anchor, setAnchor] = useState(() => calendarDate(new Date()));
  const [filterOpen, setFilterOpen] = useState(false);
  // What the field panel edits and the control it hangs under. Both outlive
  // `fieldPanelOpen` so the panel does not change shape while it closes.
  const [fieldPanel, setFieldPanel] = useState<{
    target: FieldPanelTarget;
    anchor: Element | null;
  }>({ target: { kind: "new" }, anchor: null });
  const [fieldPanelOpen, setFieldPanelOpen] = useState(false);
  const openFieldPanel = useCallback(
    (target: FieldPanelTarget, anchor: Element | null) => {
      setFieldPanel({ target, anchor });
      setFieldPanelOpen(true);
    },
    [],
  );
  const [quotaOpen, setQuotaOpen] = useState(false);
  const quotaTrigger = useRef<HTMLButtonElement>(null);
  const [selectedRecord, setSelectedRecord] = useState<string | null>(null);

  const allFields = useMemo(
    () => [...(data?.fields ?? [])].sort((a, b) => a.position - b.position),
    [data?.fields],
  );
  const defaultTitleName = t(($) => $.cortex_table.name_column);
  const storedTitleName = data?.collection.title_name ?? "";
  const titleName = storedTitleName || defaultTitleName;
  const tableFields = allFields.filter((field) => !prefs.hiddenFields.includes(field.id));
  const groupField = allFields.find(
    (field) => field.id === prefs.groupBy && field.type === "select",
  );
  const boardGroup = groupField ?? allFields.find((field) => field.type === "select");
  const dateField =
    allFields.find((field) => field.id === prefs.dateField && field.type === "date") ??
    allFields.find((field) => field.type === "date");
  // Cards show a few fields until the view picks its own.
  const displayedFields = prefs.displayedFields.length
    ? prefs.displayedFields
    : allFields
        .filter((field) => field.id !== boardGroup?.id && field.type !== "url")
        .slice(0, 3)
        .map((field) => field.id);
  const cardPrefs = { ...prefs, displayedFields };
  const cardFields = allFields.filter(
    (field) => displayedFields.includes(field.id) && field.id !== boardGroup?.id,
  );
  const accentField = allFields.find((field) => field.type === "select");

  const query: CollectionQuery = {
    ...(search ? { search } : {}),
    ...(filtersToProperties(prefs.filters, allFields)
      ? { properties: filtersToProperties(prefs.filters, allFields) }
      : {}),
    ...(prefs.sortBy ? { sort_by: prefs.sortBy, sort_dir: prefs.sortDir } : {}),
  };
  const visualQuery: CollectionQuery = { ...query };
  if (prefs.layout === "calendar" && dateField) {
    const days = calendarDays(anchor, prefs.period);
    visualQuery.date_field = dateField.id;
    visualQuery.date_start = days[0];
    visualQuery.date_end = days[days.length - 1];
  }
  const visual = useInfiniteQuery({
    ...collectionRecordsOptions(wsId, id, visualQuery),
    enabled:
      !!wsId && (prefs.layout === "gallery" || (prefs.layout === "calendar" && !!dateField)),
  });
  const summary = useInfiniteQuery({
    ...collectionRecordsOptions(wsId, id, query),
    enabled: !!wsId && prefs.layout === "table",
  });
  const visualRows = visual.data?.pages.flatMap((page) => page.records) ?? [];

  const saveView = useMutation({
    mutationFn: async ({ name, shared }: { name: string; shared: boolean }) =>
      api.createIssueView({
        name,
        scope_type: viewScope.scope_type,
        scope_id: viewScope.scope_id ?? undefined,
        collection_id: id,
        visibility: shared ? "workspace" : "private",
        definition_version: 1,
        query: { ...query },
        display: { ...prefs },
      }),
    onSuccess: async (view) => {
      await qc.invalidateQueries({ queryKey: issueViewKeys.all(wsId) });
      if (view) setActiveViewId(view.id);
    },
    onError: (cause) => toast.error(cause.message),
  });
  const updateView = useMutation({
    mutationFn: (view: IssueView) =>
      api.updateIssueView(view.id, {
        query: { ...query },
        display: { ...prefs },
        expected_revision: view.revision,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: issueViewKeys.all(wsId) }),
    onError: (cause) => toast.error(cause.message),
  });

  // The title column is `record.title`, not a catalog field: only its label
  // is editable, from its header menu or from Display.
  const editTitle = useCallback(
    (anchor: Element | null) =>
      openFieldPanel(
        { kind: "title", name: storedTitleName, defaultName: defaultTitleName },
        anchor,
      ),
    [openFieldPanel, storedTitleName, defaultTitleName],
  );
  const reorder = useCallback(
    (activeId: string, overId: string) => {
      const from = allFields.findIndex((field) => field.id === activeId);
      const to = allFields.findIndex((field) => field.id === overId);
      if (from < 0 || to < 0) return;
      const order = allFields.map((field) => field.id);
      order.splice(from, 1);
      order.splice(to, 0, activeId);
      const byId = new Map(allFields.map((field) => [field.id, field]));
      const previous = byId.get(order[to - 1] ?? "");
      const next = byId.get(order[to + 1] ?? "");
      const position =
        previous && next
          ? (previous.position + next.position) / 2
          : previous
            ? previous.position + 1
            : next
              ? next.position - 1
              : 0;
      commands.updateField.mutate(
        { fieldId: activeId, patch: { position } },
        { onError: (cause) => toast.error(cause.message) },
      );
    },
    [allFields, commands.updateField],
  );
  const tableActions = useMemo<CollectionTableActions>(
    () => ({
      canManage,
      fieldCount: allFields.length,
      sortBy: prefs.sortBy,
      sortDir: prefs.sortDir,
      onSort: (sortBy, sortDir) => update({ sortBy, sortDir }),
      onGroup: (groupBy) => update({ groupBy }),
      onFilter: (fieldId) => {
        const field = allFields.find((item) => item.id === fieldId);
        if (field && !prefs.filters.some((filter) => filter.field === fieldId))
          update({
            filters: [
              ...prefs.filters,
              { field: fieldId, op: field.type === "text" || field.type === "url" ? "contains" : "exact", value: "" },
            ],
          });
        setFilterOpen(true);
      },
      onHide: (fieldId) =>
        update({ hiddenFields: [...new Set([...prefs.hiddenFields, fieldId])] }),
      onEditField: (field, anchor) =>
        openFieldPanel({ kind: "field", field }, anchor),
      onEditTitle: editTitle,
      onArchiveField: (field) =>
        commands.updateField.mutate(
          { fieldId: field.id, patch: { archived: true } },
          {
            onSuccess: () => toast.success(t(($) => $.cortex_table.field_archived, { name: field.name })),
            onError: (cause) => toast.error(cause.message),
          },
        ),
      onAddField: (anchor) => openFieldPanel({ kind: "new" }, anchor),
      onReorderField: reorder,
      onOpenRecord: setSelectedRecord,
    }),
    [
      canManage,
      allFields,
      prefs,
      update,
      commands.updateField,
      reorder,
      openFieldPanel,
      editTitle,
      t,
    ],
  );

  const openRow = (row: CollectionRecord) => setSelectedRecord(row.id);
  const galleryFields: DataSourceField<CollectionRecord>[] = allFields
    .filter((field) => displayedFields.includes(field.id))
    .map((field) => ({
      id: field.id,
      label: field.name,
      kind: field.type as DataSourceField<CollectionRecord>["kind"],
      value: (row) => fieldText(field, row.fields[field.id]),
    }));
  const project = projects.find((item) => item.id === data?.collection.project_id);
  const total = summary.data?.pages[0]?.total;
  const loadedCount = summary.data?.pages.reduce((sum, page) => sum + page.records.length, 0) ?? 0;

  // The layouts without column headers name the field type they are missing,
  // so their empty state opens the panel with that type already chosen.
  const newFieldButton = (type: string) => (
    <Button
      variant="outline"
      size="sm"
      onClick={(event) =>
        openFieldPanel({ kind: "new", type }, event.currentTarget)
      }
    >
      <Plus />
      {t(($) => $.cortex_table.new_field)}
    </Button>
  );
  const content = (() => {
    if (!data) return null;
    if (prefs.layout === "board") {
      if (!boardGroup)
        return (
          <EmptyHint
            action={canManage ? newFieldButton("select") : undefined}
          >
            {t(($) => $.cortex_table.board_needs_select)}
          </EmptyHint>
        );
      return (
        <CollectionBoard
          collectionId={id}
          groupField={boardGroup}
          cardFields={cardFields}
          query={query}
          commands={commands}
          onOpenRecord={setSelectedRecord}
        />
      );
    }
    if (prefs.layout === "calendar") {
      if (!dateField)
        return (
          <EmptyHint action={canManage ? newFieldButton("date") : undefined}>
            {t(($) => $.cortex_table.calendar_needs_date)}
          </EmptyHint>
        );
      return (
        <DataViewCalendar
          rows={visualRows}
          rowId={(row) => row.id}
          title={(row) => row.title}
          date={(row) => {
            const value = row.fields[dateField.id];
            return typeof value === "string" ? value : null;
          }}
          anchor={anchor}
          period={prefs.period}
          onAnchorChange={setAnchor}
          onPeriodChange={(period) => update({ period })}
          onMoveDate={(row, value) => void commands.setField(row, dateField.id, value)}
          onOpen={openRow}
          locale={locale}
          capabilities={visualCapabilities}
          color={(row) =>
            accentField?.config.options.find(
              (option) => option.id === row.fields[accentField.id],
            )?.color
          }
          toolbar={
            <DateFieldChip
              fields={allFields}
              value={dateField.id}
              onChange={(fieldId) => update({ dateField: fieldId })}
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
      );
    }
    if (prefs.layout === "gallery")
      return (
        <DataViewGallery
          rows={visualRows}
          rowId={(row) => row.id}
          title={(row) => row.title}
          fields={galleryFields}
          renderField={(item, row) => {
            const field = allFields.find((candidate) => candidate.id === item.id);
            return field ? (
              <CollectionValue field={field} value={recordValue(row, field)} compact />
            ) : null;
          }}
          capabilities={visualCapabilities}
          onOpen={openRow}
          cover={(row) => {
            const value = row.fields[prefs.coverField];
            return typeof value === "string" ? value : null;
          }}
        />
      );
    if (groupField)
      return (
        <div className="min-h-0 flex-1 overflow-auto">
          {[...groupField.config.options.map((option) => option.id), "__none__"].map((key) => {
            const option = groupField.config.options.find((item) => item.id === key);
            return (
              <section key={key} aria-label={option?.name ?? t(($) => $.cortex_table.no_value)}>
                <h2 className="sticky left-0 flex items-center gap-2 px-4 pb-1 pt-4 text-label font-medium">
                  <span
                    className={cn("size-2.5 rounded-full", !option && "border border-muted-foreground")}
                    style={option?.color ? { backgroundColor: option.color } : undefined}
                  />
                  {option?.name ?? t(($) => $.cortex_table.no_value)}
                  <span className="text-caption font-normal text-muted-foreground tabular-nums">
                    {summary.data?.pages[0]?.groups.find((group) => group.key === key)?.count ?? 0}
                  </span>
                </h2>
                <CollectionTable
                  collectionId={id}
                  titleName={titleName}
                  fields={tableFields}
                  query={{ ...query, group_by: groupField.id, group_key: key }}
                  commands={commands}
                  actions={tableActions}
                  selectedRecordId={selectedRecord}
                  newRecordFields={option ? { [groupField.id]: option.id } : undefined}
                />
              </section>
            );
          })}
        </div>
      );
    return (
      <CollectionTable
        collectionId={id}
        titleName={titleName}
        fields={tableFields}
        query={query}
        commands={commands}
        actions={tableActions}
        selectedRecordId={selectedRecord}
      />
    );
  })();

  const viewBar = (
    <div className="flex flex-wrap items-center gap-1 border-b px-4 py-1.5">
      {!embedded && (
        <div role="tablist" aria-label={t(($) => $.cortex_table.views)} className="flex items-center gap-0.5">
          <ViewTab active={!activeView} onClick={() => setActiveViewId(null)}>
            {t(($) => $.cortex_table.all_records)}
          </ViewTab>
          {views.data?.map((view) => (
            <ViewTab key={view.id} active={activeView?.id === view.id} onClick={() => setActiveViewId(view.id)}>
              {view.name}
            </ViewTab>
          ))}
          <SaveViewPopover
            pending={saveView.isPending}
            onSave={(name, shared) => saveView.mutateAsync({ name, shared })}
          />
        </div>
      )}
      {!embedded && <span className="mx-1.5 h-4 w-px bg-border" aria-hidden />}
      <LayoutSwitch layout={prefs.layout} onChange={(layout) => update({ layout })} />
      <FilterPopover
        fields={allFields}
        filters={prefs.filters}
        onChange={(filters) => update({ filters })}
        open={filterOpen}
        onOpenChange={setFilterOpen}
      />
      {(prefs.layout === "table" || prefs.layout === "board") && (
        <GroupPopover
          fields={allFields}
          groupBy={prefs.layout === "board" ? (boardGroup?.id ?? "") : prefs.groupBy}
          onChange={(groupBy) => update({ groupBy })}
        />
      )}
      <SortPopover
        titleName={titleName}
        fields={allFields}
        sortBy={prefs.sortBy}
        sortDir={prefs.sortDir}
        onChange={(sortBy, sortDir) => update({ sortBy, sortDir })}
      />
      {/* A calendar has nothing to show or hide; there the popover only manages fields. */}
      {(prefs.layout !== "calendar" || canManage) && (
        <DisplayPopover
          layout={prefs.layout}
          titleName={titleName}
          fields={allFields}
          prefs={cardPrefs}
          onChange={update}
          manage={
            canManage
              ? {
                  onEdit: (field, anchor) =>
                    openFieldPanel({ kind: "field", field }, anchor),
                  onEditTitle: editTitle,
                  onAdd: (anchor) => openFieldPanel({ kind: "new" }, anchor),
                }
              : undefined
          }
        />
      )}
      <div className="ml-auto flex items-center gap-1.5">
        {dirty && activeView && (
          <>
            <Button variant="ghost" size="xs" onClick={() => updatePrefs(source, baseline)}>
              {t(($) => $.cortex_table.reset_view)}
            </Button>
            <Button
              variant="outline"
              size="xs"
              disabled={updateView.isPending}
              onClick={() =>
                updateView.mutate(activeView, {
                  onSuccess: () => updatePrefs(source, baseline),
                })
              }
            >
              <Save />
              {t(($) => $.cortex_table.update_view)}
            </Button>
          </>
        )}
      </div>
    </div>
  );

  const main = (
    <main
      className={cn(
        "flex min-h-0 min-w-0 flex-1 flex-col",
        embedded && "not-prose text-body",
      )}
    >
      {!embedded && (
        <header className="flex items-center gap-2 border-b px-4 py-2">
          <nav aria-label={t(($) => $.cortex.breadcrumb)} className="flex min-w-0 flex-1 items-center gap-1.5 text-label">
            <AppLink href={paths.collections()} className="shrink-0 text-muted-foreground hover:text-foreground">
              {t(($) => $.cortex_table.tables)}
            </AppLink>
            <span className="text-muted-foreground">/</span>
            <CollectionName
              name={data?.collection.name ?? ""}
              editable={canManage}
              onRename={(name) => commands.renameCollection.mutate(name)}
            />
            {project && (
              <AppLink
                href={paths.projectDetail(project.id)}
                className="ml-1 truncate rounded-sm bg-muted px-1.5 py-px text-caption text-muted-foreground hover:text-foreground"
              >
                {t(($) => $.cortex_table.project_chip, { name: project.title })}
              </AppLink>
            )}
          </nav>
          <label className="relative mr-1 hidden items-center lg:flex">
            <Search className="pointer-events-none absolute left-2 size-3.5 text-muted-foreground" />
            <input
              aria-label={t(($) => $.cortex.search_records)}
              placeholder={t(($) => $.cortex.search_records)}
              className="h-7 w-44 rounded-md border bg-transparent pl-7 pr-2 text-label outline-none focus:ring-2 focus:ring-ring/40"
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </label>
          <Button
            variant="outline"
            size="sm"
            onClick={async () => {
              const url = navigation.getShareableUrl(paths.collectionDetail(id));
              if (await copyText(url)) toast.success(t(($) => $.cortex_table.link_copied));
              else toast.error(t(($) => $.cortex_table.link_copy_failed));
            }}
          >
            <Link2 />
            {t(($) => $.cortex_table.copy_reference)}
          </Button>
          <Button
            size="sm"
            disabled={commands.createRecord.isPending || !data}
            onClick={() =>
              void commands.createRecord
                .mutateAsync({ title: "" })
                .then((record) => setSelectedRecord(record.id))
            }
          >
            <Plus />
            {t(($) => $.cortex_table.new_row)}
          </Button>
        </header>
      )}
      {viewBar}
      {(error || visual.error) && (
        <p role="alert" className="px-4 py-2 text-caption text-destructive">
          {(error || visual.error)?.message}
        </p>
      )}
      <div className="flex min-h-0 flex-1">
        <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-auto">
          {content}
          {(prefs.layout === "calendar" || prefs.layout === "gallery") && visual.hasNextPage && (
            <Button
              className="m-4 self-start"
              variant="outline"
              size="sm"
              disabled={visual.isFetching}
              onClick={() => void visual.fetchNextPage()}
            >
              {t(($) => $.cortex.load_more)}
            </Button>
          )}
        </div>
        {selectedRecord && (
          <CollectionRecordPanel
            key={selectedRecord}
            wsId={wsId}
            collectionId={id}
            recordId={selectedRecord}
            fields={allFields}
            commands={commands}
            onClose={() => setSelectedRecord(null)}
          />
        )}
      </div>
      {!embedded && (
        <footer className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t py-1.5 pl-4 pr-16 text-caption text-muted-foreground">
          {prefs.layout === "table" && total !== undefined && (
            <span className="tabular-nums">
              {t(($) => $.cortex_table.row_count, { count: total, loaded: loadedCount })}
            </span>
          )}
          <Popover open={quotaOpen} onOpenChange={setQuotaOpen}>
            <PopoverTrigger
              render={
                <button
                  ref={quotaTrigger}
                  type="button"
                  className="rounded-sm px-1 hover:bg-accent hover:text-foreground"
                />
              }
            >
              {t(($) => $.cortex_table.fields_used, { count: allFields.length, max: 50 })}
            </PopoverTrigger>
            <PopoverContent side="top" align="start" className="w-72 p-1">
              <FieldQuota count={allFields.length} />
              {canManage && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full justify-start"
                  onClick={() => {
                    setQuotaOpen(false);
                    openFieldPanel({ kind: "new" }, quotaTrigger.current);
                  }}
                >
                  <Plus />
                  {t(($) => $.cortex_table.new_field)}
                </Button>
              )}
            </PopoverContent>
          </Popover>
          <span className="ml-auto" />
          <CollectionTrash wsId={wsId} collectionId={id} commands={commands} />
        </footer>
      )}
      <CollectionFieldPanel
        open={fieldPanelOpen}
        onOpenChange={setFieldPanelOpen}
        anchor={fieldPanel.anchor}
        target={fieldPanel.target}
        fieldCount={allFields.length}
        commands={commands}
      />
    </main>
  );

  if (embedded) return main;
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <CollectionNavigator activeCollectionId={id} />
      {main}
    </div>
  );
}

function EmptyHint({
  children,
  action,
}: {
  children: React.ReactNode;
  action?: React.ReactNode;
}) {
  return (
    <div className="m-auto flex max-w-sm flex-col items-center gap-3 p-8 text-center text-label text-muted-foreground">
      <p>{children}</p>
      {action}
    </div>
  );
}

function ViewTab({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      role="tab"
      aria-selected={active}
      onClick={onClick}
      className={cn(
        "h-7 max-w-40 truncate rounded-md px-2.5 text-label text-muted-foreground hover:bg-accent hover:text-foreground",
        active && "bg-accent font-medium text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function SaveViewPopover({
  pending,
  onSave,
}: {
  pending: boolean;
  onSave: (name: string, shared: boolean) => Promise<unknown>;
}) {
  const { t } = useT("issues");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [shared, setShared] = useState(false);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button variant="ghost" size="icon-xs" aria-label={t(($) => $.cortex.save_view)} />
        }
      >
        <Plus />
      </PopoverTrigger>
      <PopoverContent align="start" className="w-64 space-y-3 p-3">
        <form
          className="space-y-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (!name.trim()) return;
            void onSave(name.trim(), shared).then(() => {
              setName("");
              setOpen(false);
            });
          }}
        >
          <Input
            autoFocus
            aria-label={t(($) => $.cortex.view_name)}
            placeholder={t(($) => $.cortex.view_name)}
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
          <label className="flex items-center justify-between gap-2 text-label">
            {t(($) => $.cortex_table.share_view)}
            <Switch checked={shared} onCheckedChange={setShared} />
          </label>
          <Button type="submit" size="sm" className="w-full" disabled={!name.trim() || pending}>
            {t(($) => $.cortex.save_view)}
          </Button>
        </form>
      </PopoverContent>
    </Popover>
  );
}

function CollectionName({
  name,
  editable,
  onRename,
}: {
  name: string;
  editable: boolean;
  onRename: (name: string) => void;
}) {
  const { t } = useT("issues");
  const [draft, setDraft] = useState<string | null>(null);
  if (draft !== null)
    return (
      <input
        autoFocus
        aria-label={t(($) => $.cortex_table.rename_table)}
        className="min-w-0 rounded-xs bg-background px-1 font-medium outline-none ring-2 ring-ring/40"
        value={draft}
        maxLength={80}
        onChange={(event) => setDraft(event.target.value)}
        onBlur={() => {
          if (draft.trim() && draft.trim() !== name) onRename(draft.trim());
          setDraft(null);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter") event.currentTarget.blur();
          if (event.key === "Escape") setDraft(null);
        }}
      />
    );
  return (
    <h1 className="min-w-0 truncate font-medium">
      {editable ? (
        <button
          type="button"
          className="rounded-xs px-1 hover:bg-accent"
          aria-label={t(($) => $.cortex_table.rename_table)}
          onClick={() => setDraft(name)}
        >
          {name}
        </button>
      ) : (
        name
      )}
    </h1>
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
    <div className="flex h-[420px] min-h-0 overflow-hidden">
      <CollectionDetailPage key={`${view.id}:${view.revision}`} id={id} savedView={view} />
    </div>
  );
}
