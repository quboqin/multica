"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  CalendarDays,
  CircleDot,
  GanttChart,
  Kanban,
  LayoutGrid,
  List,
  Search,
  Table2,
  type LucideIcon,
} from "lucide-react";
import {
  collectionDetailOptions,
  collectionListOptions,
  type Collection,
} from "@multica/core/collections";
import type { IssueView } from "@multica/core/api/schemas";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  issueViewDetailOptions,
  issueViewListOptions,
} from "@multica/core/issue-views/queries";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";

/** The Markdown line a saved-view embed serializes to (see saved-view-embed). */
export function savedViewEmbedMarkdown(viewId: string): string {
  return `:::multica-view ${viewId || "pending"}`;
}

const TASKS_SOURCE = "tasks";

export type ViewLayout =
  | "table"
  | "board"
  | "list"
  | "calendar"
  | "gallery"
  | "gantt";

const LAYOUT_ICON: Record<ViewLayout, LucideIcon> = {
  table: Table2,
  board: Kanban,
  list: List,
  calendar: CalendarDays,
  gallery: LayoutGrid,
  gantt: GanttChart,
};

export interface ViewSummary {
  layout: ViewLayout;
  /** Grouping key (issue views) or field id (collection views); "" = none. */
  grouping: string;
  filterCount: number;
  visibility: "workspace" | "private";
}

function isActiveFilter(value: unknown): boolean {
  if (Array.isArray(value)) return value.length > 0;
  if (value && typeof value === "object")
    return Object.values(value).some(isActiveFilter);
  if (typeof value === "string") return value.trim() !== "";
  return value === true;
}

/** Reads a saved view's layout, grouping, filter count and visibility. */
export function summarizeView(view: IssueView): ViewSummary {
  const display = view.display;
  const collection = !!view.collection_id;
  const rawLayout = collection ? display.layout : display.viewMode;
  const layout: ViewLayout =
    typeof rawLayout === "string" && rawLayout in LAYOUT_ICON
      ? (rawLayout as ViewLayout)
      : collection
        ? "table"
        : "list";
  const rawGrouping = collection ? display.groupBy : display.grouping;
  const grouping =
    typeof rawGrouping === "string" && rawGrouping !== "none" ? rawGrouping : "";
  const filterCount = Object.entries(view.query).filter(
    ([key, value]) => key !== "sort" && isActiveFilter(value),
  ).length;
  return {
    layout,
    grouping,
    filterCount,
    visibility: view.visibility === "workspace" ? "workspace" : "private",
  };
}

interface Source {
  id: string;
  name: string;
  count?: number;
  collection?: Collection;
}

export function InsertViewDialog({
  open,
  onOpenChange,
  initialViewId,
  onInsert,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialViewId?: string;
  onInsert: (viewId: string) => void;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const { data: collections = [] } = useQuery({
    ...collectionListOptions(wsId),
    enabled: open && !!wsId,
  });
  const { data: initialView } = useQuery({
    ...issueViewDetailOptions(wsId, initialViewId ?? ""),
    enabled: open && !!wsId && !!initialViewId,
  });
  const sources = useMemo<Source[]>(
    () => [
      ...collections.map((collection) => ({
        id: collection.id,
        name: collection.name,
        count: collection.record_count,
        collection,
      })),
      { id: TASKS_SOURCE, name: t(($) => $.cortex_docs.tasks_source) },
    ],
    [collections, t],
  );
  const [search, setSearch] = useState("");
  const [sourceId, setSourceId] = useState<string | null>(null);
  const [viewId, setViewId] = useState(initialViewId ?? "");
  useEffect(() => {
    if (!open) return;
    setViewId(initialViewId ?? "");
    setSearch("");
  }, [open, initialViewId]);
  useEffect(() => {
    if (initialView && open)
      setSourceId(initialView.collection_id ?? TASKS_SOURCE);
  }, [initialView, open]);
  const source =
    sources.find((item) => item.id === sourceId) ?? sources[0] ?? null;
  const collection = source?.collection;
  const { data: views = [], isLoading } = useQuery({
    ...issueViewListOptions(
      wsId,
      collection
        ? {
            scope_type: collection.project_id ? "project" : "workspace",
            scope_id: collection.project_id,
            collection_id: collection.id,
          }
        : { scope_type: "workspace" },
    ),
    enabled: open && !!wsId && !!source,
  });
  const { data: detail } = useQuery({
    ...collectionDetailOptions(wsId, collection?.id ?? ""),
    enabled: open && !!collection,
  });
  const sourceViews = views.filter((view) =>
    collection ? view.collection_id === collection.id : !view.collection_id,
  );
  const query = search.trim().toLowerCase();
  const visibleSources = sources.filter(
    (item) => !query || item.name.toLowerCase().includes(query),
  );
  const selected = sourceViews.find((view) => view.id === viewId);
  const firstViewId = sourceViews[0]?.id;
  // Keep one view picked so Insert is ready as soon as a source is chosen.
  useEffect(() => {
    if (open && !viewId && firstViewId) setViewId(firstViewId);
  }, [open, viewId, firstViewId]);

  const groupingLabel = (grouping: string) => {
    if (!grouping) return t(($) => $.cortex_docs.summary_no_group);
    if (collection)
      return (
        detail?.fields.find((field) => field.id === grouping)?.name ?? grouping
      );
    const known = ["status", "assignee", "priority", "project", "label"];
    return known.includes(grouping)
      ? t(($) => $.cortex_docs[`group_${grouping as "status"}`])
      : t(($) => $.cortex_docs.group_property);
  };
  const summaryLine = (view: IssueView) => {
    const summary = summarizeView(view);
    return [
      t(($) => $.cortex_docs[`layout_${summary.layout}`]),
      t(($) => $.cortex_docs.summary_group, {
        value: groupingLabel(summary.grouping),
      }),
      summary.filterCount
        ? t(($) => $.cortex_docs.summary_filters, {
            count: summary.filterCount,
          })
        : t(($) => $.cortex_docs.summary_no_filter),
      t(($) => $.cortex_docs[`visibility_${summary.visibility}`]),
    ].join(" · ");
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-3xl">
        <DialogHeader className="border-b px-4 py-3">
          <DialogTitle className="flex items-center gap-2">
            <Table2 className="size-4 text-muted-foreground" />
            {t(($) => $.cortex_docs.insert_title)}
          </DialogTitle>
        </DialogHeader>
        <div className="grid min-h-0 grid-cols-1 sm:grid-cols-[220px_1fr]">
          <div className="border-b bg-muted/30 p-2 sm:border-r sm:border-b-0">
            <div className="relative mb-2">
              <Search className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                aria-label={t(($) => $.cortex_docs.search_sources)}
                placeholder={t(($) => $.cortex_docs.search_sources)}
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                className="h-8 bg-background pl-8 text-label"
              />
            </div>
            <ul
              aria-label={t(($) => $.cortex_docs.sources)}
              className="max-h-40 space-y-0.5 overflow-y-auto sm:max-h-80"
            >
              {visibleSources.map((item) => {
                const active = item.id === source?.id;
                const Icon = item.collection ? Table2 : CircleDot;
                return (
                  <li key={item.id}>
                    <button
                      type="button"
                      aria-pressed={active}
                      className={cn(
                        "flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-label transition-colors",
                        active
                          ? "bg-surface-selected font-medium text-surface-selected-foreground"
                          : "hover:bg-accent/60",
                      )}
                      onClick={() => {
                        setSourceId(item.id);
                        setViewId("");
                      }}
                    >
                      <Icon className="size-3.5 shrink-0 text-muted-foreground" />
                      <span className="min-w-0 flex-1 truncate">
                        {item.name}
                      </span>
                      {item.count !== undefined && (
                        <span className="text-caption tabular-nums text-muted-foreground">
                          {item.count}
                        </span>
                      )}
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
          <div className="max-h-[60dvh] space-y-4 overflow-y-auto p-4">
            <fieldset>
              <legend className="mb-2 text-caption font-medium text-muted-foreground">
                {t(($) => $.cortex_docs.saved_views_of, {
                  name: source?.name ?? "",
                })}
              </legend>
              {isLoading ? (
                <p role="status" className="text-caption text-muted-foreground">
                  {t(($) => $.cortex_docs.loading)}
                </p>
              ) : sourceViews.length === 0 ? (
                <p className="rounded-lg border border-dashed p-3 text-caption text-muted-foreground">
                  {t(($) => $.cortex_docs.no_saved_views)}
                </p>
              ) : (
                <div className="space-y-2">
                  {sourceViews.map((view) => {
                    const Icon = LAYOUT_ICON[summarizeView(view).layout];
                    const checked = view.id === viewId;
                    return (
                      <label
                        key={view.id}
                        className={cn(
                          "flex cursor-pointer gap-3 rounded-lg border px-3 py-2.5 transition-colors",
                          checked
                            ? "border-brand bg-brand/5"
                            : "hover:bg-accent/40",
                        )}
                      >
                        <input
                          type="radio"
                          name="insert-view"
                          value={view.id}
                          checked={checked}
                          onChange={() => setViewId(view.id)}
                          className="mt-0.5 size-4 shrink-0 accent-[var(--color-brand)]"
                        />
                        <span className="min-w-0">
                          <span className="flex items-center gap-1.5 text-body font-medium">
                            <Icon className="size-3.5 shrink-0" />
                            <span className="truncate">{view.name}</span>
                          </span>
                          <span className="mt-0.5 block text-caption text-muted-foreground">
                            {summaryLine(view)}
                          </span>
                        </span>
                      </label>
                    );
                  })}
                </div>
              )}
            </fieldset>
            <fieldset className="border-t pt-4">
              <legend className="sr-only">
                {t(($) => $.cortex_docs.embed_mode)}
              </legend>
              <p aria-hidden className="mb-2 text-caption font-medium text-muted-foreground">
                {t(($) => $.cortex_docs.embed_mode)}
              </p>
              <div className="space-y-2">
                <label className="flex cursor-pointer gap-3 rounded-lg border border-brand bg-brand/5 px-3 py-2.5">
                  <input
                    type="radio"
                    name="embed-mode"
                    defaultChecked
                    className="mt-0.5 size-4 shrink-0 accent-[var(--color-brand)]"
                  />
                  <span>
                    <span className="flex items-center gap-2 text-body font-medium">
                      {t(($) => $.cortex_docs.mode_live)}
                      <span className="rounded bg-success/10 px-1.5 py-0.5 text-micro font-medium text-success">
                        {t(($) => $.cortex_docs.recommended)}
                      </span>
                    </span>
                    <span className="mt-0.5 block text-caption text-muted-foreground">
                      {t(($) => $.cortex_docs.mode_live_hint)}
                    </span>
                  </span>
                </label>
                <label className="flex cursor-not-allowed gap-3 rounded-lg border px-3 py-2.5 opacity-60">
                  <input
                    type="radio"
                    name="embed-mode"
                    disabled
                    className="mt-0.5 size-4 shrink-0"
                  />
                  <span>
                    <span className="block text-body font-medium">
                      {t(($) => $.cortex_docs.mode_snapshot)}
                    </span>
                    <span className="mt-0.5 block text-caption text-muted-foreground">
                      {t(($) => $.cortex_docs.mode_snapshot_hint)}
                    </span>
                  </span>
                </label>
              </div>
            </fieldset>
            <div>
              <p className="mb-1 text-caption text-muted-foreground">
                {t(($) => $.cortex_docs.markdown_preview)}
              </p>
              <code
                data-testid="insert-view-preview"
                className="block truncate rounded-md border bg-muted/40 px-3 py-2 font-mono text-caption text-muted-foreground"
              >
                {savedViewEmbedMarkdown(selected?.id ?? "")}
              </code>
            </div>
          </div>
        </div>
        <DialogFooter className="m-0 border-t px-4 py-3">
          <DialogClose render={<Button variant="outline" />}>
            {t(($) => $.cortex_docs.cancel)}
          </DialogClose>
          <Button
            disabled={!selected}
            onClick={() => {
              if (!selected) return;
              onInsert(selected.id);
              onOpenChange(false);
            }}
          >
            {t(($) => $.cortex_docs.insert)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
