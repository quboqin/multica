"use client";

import { useState, type DragEvent, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight,
  FileText,
  ListFilter,
  Plus,
  Search,
  Star,
  Table2,
} from "lucide-react";
import { api } from "@multica/core/api";
import {
  collectionKeys,
  collectionListOptions,
} from "@multica/core/collections";
import {
  defaultNavigatorPreferences,
  documentTreeOptions,
  useDocumentPreferences,
  type DocumentTreeFilter,
} from "@multica/core/documents";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { cn } from "@multica/ui/lib/utils";
import { AppLink, useNavigation } from "../navigation";
import { useT } from "../i18n";
import { useCreateDocument, useDocumentCommand } from "./use-document-commands";

const DRAG_TYPE = "application/x-multica-document";
const emptyIds: string[] = [];
const FILTERS: DocumentTreeFilter[] = ["all", "favorites", "recent"];

export interface CortexNavigatorProps {
  activeDocumentId?: string;
  activeCollectionId?: string;
}

/**
 * Secondary navigator shared by the Cortex documents and tables pages: the
 * document tree on top, the workspace's tables below.
 */
export function CortexNavigator({
  activeDocumentId,
  activeCollectionId,
}: CortexNavigatorProps) {
  const { t } = useT("issues");
  const ws = useCurrentWorkspace();
  const wsId = ws?.id ?? "";
  return (
    <aside
      className="flex w-60 shrink-0 flex-col overflow-y-auto border-r bg-sidebar px-2 py-3 text-sidebar-foreground"
      aria-label={t(($) => $.cortex_docs.navigator)}
    >
      <DocumentTree wsId={wsId} activeDocumentId={activeDocumentId} />
      <CollectionList wsId={wsId} activeCollectionId={activeCollectionId} />
    </aside>
  );
}

function SectionHeader({
  id,
  title,
  children,
}: {
  id: string;
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex h-7 items-center gap-0.5 px-2">
      <h2
        id={id}
        className="flex-1 truncate text-caption font-medium text-muted-foreground"
      >
        {title}
      </h2>
      {children}
    </div>
  );
}

function DocumentTree({
  wsId,
  activeDocumentId,
}: {
  wsId: string;
  activeDocumentId?: string;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const { data: documents = [], isLoading, error } = useQuery(
    documentTreeOptions(wsId),
  );
  const { data: projects = [] } = useQuery(projectListOptions(wsId));
  const prefs = useDocumentPreferences(
    (state) => state.navigator[wsId] ?? defaultNavigatorPreferences,
  );
  const setNavigator = useDocumentPreferences((state) => state.setNavigator);
  const toggleCollapsed = useDocumentPreferences(
    (state) => state.toggleCollapsed,
  );
  const favorites = useDocumentPreferences(
    (state) => state.favorites[wsId] ?? emptyIds,
  );
  const recent = useDocumentPreferences(
    (state) => state.recent[wsId] ?? emptyIds,
  );
  const toggleFavorite = useDocumentPreferences(
    (state) => state.toggleFavorite,
  );
  const command = useDocumentCommand(wsId);
  const creator = useCreateDocument(wsId);
  const [search, setSearch] = useState("");
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<string | null>(null);
  const { filter, projectId } = prefs;
  const collapsed = new Set(prefs.collapsed);
  const project = projects.find((item) => item.id === projectId);

  const scoped = documents.filter(
    (doc) => !projectId || doc.project_id === projectId,
  );
  const query = search.trim().toLowerCase();
  const matches = scoped.filter(
    (doc) =>
      (!query ||
        `${doc.title} ${doc.description ?? ""}`.toLowerCase().includes(query)) &&
      (filter !== "favorites" || favorites.includes(doc.id)) &&
      (filter !== "recent" || recent.includes(doc.id)),
  );
  const flat = filter !== "all" || !!query;
  const childrenOf = (id: string | null) =>
    scoped.filter((doc) =>
      id
        ? doc.parent_issue_id === id
        : !doc.parent_issue_id ||
          !scoped.some((parent) => parent.id === doc.parent_issue_id),
    );
  const rows = flat
    ? filter === "recent"
      ? [...matches].sort(
          (a, b) => recent.indexOf(a.id) - recent.indexOf(b.id),
        )
      : matches
    : childrenOf(null);

  const move = (
    id: string,
    parent: string | null,
    position: number,
    beforeId?: string,
  ) => command.mutate(() => api.moveDocument(id, parent, position, beforeId));
  const readDrag = (event: DragEvent) => {
    event.preventDefault();
    event.stopPropagation();
    setDropTarget(null);
    setDragId(null);
    return event.dataTransfer.getData(DRAG_TYPE);
  };
  const dropBefore = (doc: Issue, id: string) => {
    if (!id || id === doc.id) return;
    const siblings = documents.filter(
      (item) =>
        item.parent_issue_id === doc.parent_issue_id &&
        item.project_id === doc.project_id &&
        item.id !== id,
    );
    const index = siblings.findIndex((item) => item.id === doc.id);
    const previous = siblings[index - 1];
    move(
      id,
      doc.parent_issue_id,
      previous ? (previous.position + doc.position) / 2 : doc.position - 1,
      doc.id,
    );
  };

  const row = (doc: Issue, depth: number): ReactNode => {
    const kids = flat ? [] : childrenOf(doc.id);
    const open = !collapsed.has(doc.id);
    const active = doc.id === activeDocumentId;
    const favorite = favorites.includes(doc.id);
    return (
      <li
        key={doc.id}
        role="treeitem"
        className="relative"
        aria-expanded={kids.length ? open : undefined}
        aria-selected={active}
      >
        {dragId && dragId !== doc.id && (
          <div
            role="separator"
            aria-label={t(($) => $.cortex_docs.move_before, {
              title: doc.title,
            })}
            className={cn(
              "absolute inset-x-2 -top-0.5 z-10 h-1 rounded-full",
              dropTarget === `before:${doc.id}` && "bg-brand",
            )}
            onDragOver={(event) => {
              event.preventDefault();
              setDropTarget(`before:${doc.id}`);
            }}
            onDragLeave={() => setDropTarget(null)}
            onDrop={(event) => dropBefore(doc, readDrag(event))}
          />
        )}
        <div
          className={cn(
            "group/row flex h-7 min-w-0 items-center gap-1 rounded-md pr-1 text-label transition-colors",
            active
              ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
              : "hover:bg-sidebar-accent/60",
            dropTarget === `into:${doc.id}` && "ring-1 ring-brand",
          )}
          style={{ paddingInlineStart: 4 + depth * 14 }}
          draggable
          onDragStart={(event) => {
            event.dataTransfer.setData(DRAG_TYPE, doc.id);
            event.dataTransfer.effectAllowed = "move";
            setDragId(doc.id);
          }}
          onDragEnd={() => {
            setDragId(null);
            setDropTarget(null);
          }}
          onDragOver={(event) => {
            event.preventDefault();
            if (dragId && dragId !== doc.id) setDropTarget(`into:${doc.id}`);
          }}
          onDrop={(event) => {
            const id = readDrag(event);
            if (id && id !== doc.id) move(id, doc.id, 0);
          }}
        >
          {kids.length ? (
            <button
              type="button"
              aria-label={t(($) =>
                open ? $.cortex_docs.collapse : $.cortex_docs.expand,
              )}
              className="flex size-4 shrink-0 items-center justify-center rounded text-muted-foreground hover:text-foreground"
              onClick={() => toggleCollapsed(wsId, doc.id)}
            >
              <ChevronRight
                className={cn("size-3 transition-transform", open && "rotate-90")}
              />
            </button>
          ) : (
            <span aria-hidden className="size-4 shrink-0" />
          )}
          <FileText className="size-3.5 shrink-0 text-muted-foreground" />
          <AppLink
            href={paths.documentDetail(doc.id)}
            className="min-w-0 flex-1 truncate py-1"
            aria-current={active ? "page" : undefined}
          >
            {doc.title || t(($) => $.cortex_docs.untitled)}
          </AppLink>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t(($) =>
              favorite ? $.cortex_docs.unfavorite : $.cortex_docs.favorite,
            )}
            aria-pressed={favorite}
            className={cn(
              "text-muted-foreground opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100",
              favorite && "text-warning opacity-100",
            )}
            onClick={() => toggleFavorite(wsId, doc.id)}
          >
            <Star className={cn(favorite && "fill-current")} />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t(($) => $.cortex_docs.add_child)}
            className="text-muted-foreground opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100"
            disabled={creator.isPending}
            onClick={() => {
              if (collapsed.has(doc.id)) toggleCollapsed(wsId, doc.id);
              void creator.create({ parent: doc }).catch(() => undefined);
            }}
          >
            <Plus />
          </Button>
        </div>
        {kids.length > 0 && open && (
          <ul role="group">{kids.map((child) => row(child, depth + 1))}</ul>
        )}
      </li>
    );
  };

  const filtered = filter !== "all" || !!projectId;
  const failure = error ?? command.error ?? creator.error;
  return (
    <section aria-labelledby="cortex-nav-docs">
      <div className="relative mb-2 px-1">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={t(($) => $.cortex_docs.search)}
          placeholder={t(($) => $.cortex_docs.search)}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          className="h-7 bg-background pl-7 text-label"
        />
      </div>
      <SectionHeader id="cortex-nav-docs" title={t(($) => $.cortex_docs.documents)}>
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant={filtered ? "brandSubtle" : "ghost"}
                size="icon-xs"
                aria-label={t(($) => $.cortex_docs.filter)}
                className={cn(!filtered && "text-muted-foreground")}
              >
                <ListFilter />
              </Button>
            }
          />
          <DropdownMenuContent align="end" className="w-48">
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t(($) => $.cortex_docs.show)}</DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={filter}
                onValueChange={(value) =>
                  setNavigator(wsId, { filter: value as DocumentTreeFilter })
                }
              >
                {FILTERS.map((value) => (
                  <DropdownMenuRadioItem key={value} value={value}>
                    {t(($) => $.cortex_docs[`filter_${value}`])}
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t(($) => $.cortex_docs.scope)}</DropdownMenuLabel>
              <DropdownMenuRadioGroup
                value={projectId}
                onValueChange={(value) =>
                  setNavigator(wsId, { projectId: String(value) })
                }
              >
                <DropdownMenuRadioItem value="">
                  {t(($) => $.cortex_docs.workspace)}
                </DropdownMenuRadioItem>
                {projects.map((item) => (
                  <DropdownMenuRadioItem key={item.id} value={item.id}>
                    <span className="truncate">{item.title}</span>
                  </DropdownMenuRadioItem>
                ))}
              </DropdownMenuRadioGroup>
            </DropdownMenuGroup>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon-xs"
          className="text-muted-foreground"
          aria-label={t(($) => $.cortex_docs.new_document)}
          disabled={creator.isPending || !wsId}
          onClick={() =>
            void creator.create({ projectId }).catch(() => undefined)
          }
        >
          <Plus />
        </Button>
      </SectionHeader>
      {filtered && (
        <div className="mb-1 flex flex-wrap gap-1 px-2">
          {filter !== "all" && (
            <FilterChip
              label={t(($) => $.cortex_docs[`filter_${filter}`])}
              clearLabel={t(($) => $.cortex_docs.clear_filter)}
              onClear={() => setNavigator(wsId, { filter: "all" })}
            />
          )}
          {project && (
            <FilterChip
              label={project.title}
              clearLabel={t(($) => $.cortex_docs.clear_filter)}
              onClear={() => setNavigator(wsId, { projectId: "" })}
            />
          )}
        </div>
      )}
      <ul
        role="tree"
        aria-labelledby="cortex-nav-docs"
        className="min-h-8 pb-1"
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          const id = readDrag(event);
          if (id) move(id, null, 0);
        }}
      >
        {rows.map((doc) => row(doc, 0))}
      </ul>
      {dragId && (
        <p className="mx-2 rounded-md border border-dashed px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.drop_root)}
        </p>
      )}
      {!isLoading && rows.length === 0 && (
        <p className="px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.no_documents)}
        </p>
      )}
      {isLoading && (
        <p role="status" className="px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.loading)}
        </p>
      )}
      {failure && (
        <p role="alert" className="px-2 py-1 text-caption text-destructive">
          {failure.message}
        </p>
      )}
    </section>
  );
}

function FilterChip({
  label,
  clearLabel,
  onClear,
}: {
  label: string;
  clearLabel: string;
  onClear: () => void;
}) {
  return (
    <span className="inline-flex max-w-full items-center gap-1 rounded-full bg-sidebar-accent py-0.5 pr-1 pl-2 text-caption">
      <span className="truncate">{label}</span>
      <button
        type="button"
        aria-label={`${clearLabel} ${label}`}
        className="rounded-full px-1 text-muted-foreground hover:text-foreground"
        onClick={onClear}
      >
        ×
      </button>
    </span>
  );
}

function CollectionList({
  wsId,
  activeCollectionId,
}: {
  wsId: string;
  activeCollectionId?: string;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const { data: collections = [], isLoading } = useQuery(
    collectionListOptions(wsId),
  );
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const create = useMutation({
    mutationFn: (value: string) => api.createCollection(value),
    onSuccess: async (collection) => {
      await queryClient.invalidateQueries({
        queryKey: collectionKeys.all(wsId),
      });
      setOpen(false);
      setName("");
      navigation.push(paths.collectionDetail(collection.id));
    },
  });
  return (
    <section aria-labelledby="cortex-nav-tables" className="mt-4">
      <SectionHeader id="cortex-nav-tables" title={t(($) => $.cortex_docs.tables)}>
        <Popover
          open={open}
          onOpenChange={(next) => {
            setOpen(next);
            if (!next) create.reset();
          }}
        >
          <PopoverTrigger
            render={
              <Button
                variant="ghost"
                size="icon-xs"
                className="text-muted-foreground"
                aria-label={t(($) => $.cortex_docs.new_table)}
              >
                <Plus />
              </Button>
            }
          />
          <PopoverContent align="start" className="w-64">
            <form
              className="space-y-2"
              aria-busy={create.isPending}
              onSubmit={(event) => {
                event.preventDefault();
                if (name.trim()) create.mutate(name.trim());
              }}
            >
              <label className="block space-y-1 text-caption font-medium">
                <span>{t(($) => $.cortex_docs.table_name)}</span>
                <Input
                  autoFocus
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  className="h-8"
                />
              </label>
              {create.error && (
                <p role="alert" className="text-caption text-destructive">
                  {create.error.message}
                </p>
              )}
              <div className="flex justify-end">
                <Button
                  type="submit"
                  size="sm"
                  disabled={!name.trim() || create.isPending}
                >
                  {t(($) => $.cortex_docs.create_table)}
                </Button>
              </div>
            </form>
          </PopoverContent>
        </Popover>
      </SectionHeader>
      <ul>
        {collections.map((collection) => {
          const active = collection.id === activeCollectionId;
          return (
            <li key={collection.id}>
              <AppLink
                href={paths.collectionDetail(collection.id)}
                aria-current={active ? "page" : undefined}
                className={cn(
                  "flex h-7 min-w-0 items-center gap-2 rounded-md px-2 text-label transition-colors",
                  active
                    ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                    : "hover:bg-sidebar-accent/60",
                )}
              >
                <Table2 className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{collection.name}</span>
                {collection.record_count !== undefined && (
                  <span className="shrink-0 text-caption tabular-nums text-muted-foreground">
                    {collection.record_count}
                  </span>
                )}
              </AppLink>
            </li>
          );
        })}
      </ul>
      {!isLoading && collections.length === 0 && (
        <p className="px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.no_tables)}
        </p>
      )}
    </section>
  );
}
