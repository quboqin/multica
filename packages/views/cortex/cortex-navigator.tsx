"use client";

import {
  useState,
  type DragEvent,
  type ReactElement,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ChevronRight,
  FileText,
  ListFilter,
  MoreHorizontal,
  Plus,
  Search,
  Star,
  Table2,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import {
  collectionKeys,
  collectionListOptions,
  type Collection,
} from "@multica/core/collections";
import {
  defaultNavigatorPreferences,
  documentKeys,
  documentTreeOptions,
  useDocumentPreferences,
  type DocumentTreeFilter,
} from "@multica/core/documents";
import { useDeleteIssue } from "@multica/core/issues/mutations";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import type { Issue } from "@multica/core/types";
import { memberListOptions } from "@multica/core/workspace/queries";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
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
import { CollapsedNavTrigger } from "../layout/page-header";
import { AppLink, useNavigation } from "../navigation";
import { useT } from "../i18n";
import { useCreateDocument, useDocumentCommand } from "./use-document-commands";
import { CollectionImportDialog } from "../collections/collection-import-dialog";

const DRAG_TYPE = "application/x-multica-document";
const emptyIds: string[] = [];
const FILTERS: DocumentTreeFilter[] = ["all", "favorites", "recent"];

const DOCUMENTS_HEADING_ID = "cortex-nav-docs";
const TABLES_HEADING_ID = "cortex-nav-tables";

// Row actions take no room until their row is hovered or holds focus, so a
// title keeps the width of this narrow column. An open menu keeps its own
// trigger in place, and touch screens, which cannot hover, always show them.
const ROW_ACTION =
  "hidden text-muted-foreground group-hover/row:inline-flex group-focus-within/row:inline-flex data-popup-open:inline-flex [@media(hover:none)]:inline-flex";

/**
 * The column a Cortex page opens beside the app sidebar. Documents and tables
 * each get their own: the app sidebar keeps a single entry for either and
 * never expands in place.
 *
 * Below `md` there is room for one column, so the index route shows the
 * navigator as the page and an opened item takes its place.
 */
function NavigatorShell({
  labelledBy,
  hasSelection,
  children,
}: {
  labelledBy: string;
  hasSelection: boolean;
  children: ReactNode;
}) {
  return (
    <aside
      aria-labelledby={labelledBy}
      className={cn(
        "shrink-0 flex-col overflow-y-auto border-r bg-sidebar px-2 py-3 text-sidebar-foreground md:flex md:w-60",
        hasSelection ? "hidden" : "flex w-full",
      )}
    >
      {children}
    </aside>
  );
}

/**
 * Top row of the column. These pages build their own chrome instead of a
 * `PageHeader`, so the way back to a collapsed app sidebar lives here.
 */
function SearchField({
  value,
  onChange,
}: {
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useT("issues");
  return (
    <div className="mb-2 flex items-center gap-1 px-1">
      <CollapsedNavTrigger />
      <div className="relative min-w-0 flex-1">
        <Search className="pointer-events-none absolute top-1/2 left-2 size-3.5 -translate-y-1/2 text-muted-foreground" />
        <Input
          aria-label={t(($) => $.cortex_docs.search)}
          placeholder={t(($) => $.cortex_docs.search)}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          className="h-7 bg-background pl-7 text-label"
        />
      </div>
    </div>
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

/** The "more" menu at the end of a row. Deleting asks for confirmation first. */
function RowMenu({
  label,
  deleteLabel,
  onDelete,
}: {
  label: string;
  deleteLabel: string;
  onDelete: () => void;
}) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={label}
            className={cn(ROW_ACTION, "data-popup-open:bg-sidebar-accent")}
          >
            <MoreHorizontal />
          </Button>
        }
      />
      <DropdownMenuContent align="start" className="w-44">
        <DropdownMenuItem variant="destructive" onClick={onDelete}>
          <Trash2 />
          {deleteLabel}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

/** Stays open until the server confirms, and keeps the failure in view. */
function DeleteConfirm({
  open,
  name,
  description,
  pending,
  onConfirm,
  onClose,
}: {
  open: boolean;
  name: string;
  description: ReactNode;
  pending: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  return (
    <AlertDialog
      open={open}
      onOpenChange={(next) => {
        if (!next && !pending) onClose();
      }}
    >
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {t(($) => $.cortex_docs.delete_title, { name })}
          </AlertDialogTitle>
          <AlertDialogDescription>{description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>
            {t(($) => $.cortex_docs.cancel)}
          </AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={pending}
            aria-busy={pending}
            onClick={onConfirm}
          >
            {pending
              ? t(($) => $.cortex_docs.deleting)
              : t(($) => $.cortex_docs.delete_confirm)}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export interface DocumentNavigatorProps {
  activeDocumentId?: string;
}

/** Second column of the documents pages: the workspace's document tree. */
export function DocumentNavigator({ activeDocumentId }: DocumentNavigatorProps) {
  const { t } = useT("issues");
  const wsId = useCurrentWorkspace()?.id ?? "";
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
  // What the confirmation is about. It outlives `confirmOpen` so the dialog
  // keeps its wording while it closes.
  const [deleting, setDeleting] = useState<DocumentDeletion | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
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

  const titleOf = (doc: Issue) => doc.title || t(($) => $.cortex_docs.untitled);
  const askDelete = (doc: Issue) => {
    setDeleting({
      doc,
      children: documents.filter((item) => item.parent_issue_id === doc.id)
        .length,
    });
    setConfirmOpen(true);
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
            {titleOf(doc)}
          </AppLink>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t(($) =>
              favorite ? $.cortex_docs.unfavorite : $.cortex_docs.favorite,
            )}
            aria-pressed={favorite}
            // A favorite keeps its star in view as the row's marker.
            className={favorite ? "text-warning" : ROW_ACTION}
            onClick={() => toggleFavorite(wsId, doc.id)}
          >
            <Star className={cn(favorite && "fill-current")} />
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            aria-label={t(($) => $.cortex_docs.add_child)}
            className={ROW_ACTION}
            disabled={creator.isPending}
            onClick={() => {
              if (collapsed.has(doc.id)) toggleCollapsed(wsId, doc.id);
              void creator.create({ parent: doc }).catch(() => undefined);
            }}
          >
            <Plus />
          </Button>
          <RowMenu
            label={t(($) => $.cortex_docs.row_actions, { name: titleOf(doc) })}
            deleteLabel={t(($) => $.cortex_docs.delete)}
            onDelete={() => askDelete(doc)}
          />
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
    <NavigatorShell
      labelledBy={DOCUMENTS_HEADING_ID}
      hasSelection={!!activeDocumentId}
    >
      <SearchField value={search} onChange={setSearch} />
      <SectionHeader
        id={DOCUMENTS_HEADING_ID}
        title={t(($) => $.cortex_docs.documents)}
      >
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
        aria-labelledby={DOCUMENTS_HEADING_ID}
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
      {/* The delete mutation needs a workspace, so it mounts only with one. */}
      {wsId && deleting && (
        <DocumentDeleteConfirm
          wsId={wsId}
          open={confirmOpen}
          deletion={deleting}
          isActive={deleting.doc.id === activeDocumentId}
          onClose={() => setConfirmOpen(false)}
        />
      )}
    </NavigatorShell>
  );
}

interface DocumentDeletion {
  doc: Issue;
  /** Child pages at the time of asking; the server moves them to the top level. */
  children: number;
}

function DocumentDeleteConfirm({
  wsId,
  open,
  deletion,
  isActive,
  onClose,
}: {
  wsId: string;
  open: boolean;
  deletion: DocumentDeletion;
  /** The document is the one on screen, so deleting it leaves the page. */
  isActive: boolean;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const deleteIssue = useDeleteIssue();
  // Covers the tree refresh too, so the row is gone when the dialog closes.
  const [pending, setPending] = useState(false);
  const confirm = async () => {
    setPending(true);
    try {
      await deleteIssue.mutateAsync(deletion.doc.id);
      // The server has confirmed: leave the deleted page before anything on it
      // asks for the document again.
      if (isActive) navigation.push(paths.documents());
      await queryClient.invalidateQueries({ queryKey: documentKeys.all(wsId) });
      toast.success(t(($) => $.cortex_docs.document_deleted));
      onClose();
    } catch (cause) {
      toast.error(cause instanceof Error ? cause.message : String(cause));
    } finally {
      setPending(false);
    }
  };
  return (
    <DeleteConfirm
      open={open}
      name={deletion.doc.title || t(($) => $.cortex_docs.untitled)}
      description={
        <>
          {t(($) => $.cortex_docs.delete_document_description)}
          {deletion.children > 0 && (
            // Its own line: sentence spacing differs between languages.
            <span className="mt-1 block">
              {t(($) => $.cortex_docs.delete_document_children, {
                count: deletion.children,
              })}
            </span>
          )}
        </>
      }
      pending={pending}
      onConfirm={() => void confirm()}
      onClose={onClose}
    />
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

/**
 * Names a new table, then opens it once the server confirms. `trigger` is the
 * button that opens the form.
 */
export function NewTablePopover({
  trigger,
  align = "start",
  projectId: initialProjectId = "",
}: {
  trigger: ReactElement;
  projectId?: string;
  align?: "start" | "center" | "end";
}) {
  const { t } = useT("issues");
  const wsId = useCurrentWorkspace()?.id ?? "";
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [projectId, setProjectId] = useState(initialProjectId);
  const [icon, setIcon] = useState("");
  const [description, setDescription] = useState("");
  const {data: projects = []} = useQuery(projectListOptions(wsId));
  const create = useMutation({
    mutationFn: (value: string) => api.createCollection(value, projectId || undefined, {icon, description}),
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
    <Popover
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) create.reset();
      }}
    >
      <PopoverTrigger render={trigger} />
      <PopoverContent align={align} className="w-64">
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
          <label className="block text-caption">{t($=>$.cortex_bulk.project)}<select className="w-full rounded border bg-background p-1" value={projectId} onChange={e=>setProjectId(e.target.value)}><option value="">{t($=>$.cortex_bulk.workspace)}</option>{projects.map(p=><option key={p.id} value={p.id}>{p.title}</option>)}</select></label>
          <label className="block text-caption">{t($=>$.cortex_bulk.icon)}<Input maxLength={32} value={icon} onChange={e=>setIcon(e.target.value)}/></label>
          <label className="block text-caption">{t($=>$.cortex_bulk.description)}<Input maxLength={4000} value={description} onChange={e=>setDescription(e.target.value)}/></label>
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
  );
}

export interface CollectionNavigatorProps {
  activeCollectionId?: string;
}

/** Second column of the collections pages: the workspace's tables. */
export function CollectionNavigator({
  activeCollectionId,
}: CollectionNavigatorProps) {
  const { t } = useT("issues");
  const wsId = useCurrentWorkspace()?.id ?? "";
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const [archived, setArchived] = useState(false);
  const restore = useMutation({mutationFn: (id:string)=>api.restoreCollection(id),onSuccess:()=>queryClient.invalidateQueries({queryKey:collectionKeys.all(wsId)})});
  const { data: collections = [], isLoading, error } = useQuery(
    collectionListOptions(wsId, archived),
  );
  // Same rule the server applies: the creator, or a workspace owner or admin.
  const userId = useAuthStore((state) => state.user?.id);
  const { data: members = [] } = useQuery({
    ...memberListOptions(wsId),
    enabled: !!wsId,
  });
  const role = members.find((member) => member.user_id === userId)?.role;
  const canDelete = (collection: Collection) =>
    !!userId &&
    (collection.created_by === userId || role === "owner" || role === "admin");
  // Outlives `confirmOpen` so the dialog keeps its wording while it closes.
  const [deleting, setDeleting] = useState<Collection | null>(null);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const remove = useMutation({
    mutationFn: (collection: Collection) =>
      api.updateCollection(collection.id, { archived: true }),
    onSuccess: async (_, collection) => {
      // The server has confirmed: leave the deleted table's page first, and
      // refresh the list alone. The table's own queries would only get 404s.
      if (collection.id === activeCollectionId)
        navigation.push(paths.collections());
      await queryClient.invalidateQueries({
        queryKey: collectionListOptions(wsId).queryKey,
      });
      toast.success(t(($) => $.cortex_docs.table_deleted));
      setConfirmOpen(false);
    },
    onError: (cause) => toast.error(cause.message),
  });
  const [search, setSearch] = useState("");
  const query = search.trim().toLowerCase();
  const rows = query
    ? collections.filter((collection) =>
        collection.name.toLowerCase().includes(query),
      )
    : collections;
  return (
    <NavigatorShell
      labelledBy={TABLES_HEADING_ID}
      hasSelection={!!activeCollectionId}
    >
      <SearchField value={search} onChange={setSearch} />
      <SectionHeader
        id={TABLES_HEADING_ID}
        title={t(($) => $.cortex_docs.tables)}
      >
        <NewTablePopover
          trigger={
            <Button
              variant="ghost"
              size="icon-xs"
              className="text-muted-foreground"
              aria-label={t(($) => $.cortex_docs.new_table)}
              disabled={!wsId}
            >
              <Plus />
            </Button>
          }
        />
      </SectionHeader>
      <div className="px-2 py-2"><CollectionImportDialog /></div>
      <label className="flex items-center gap-2 px-2 py-2 text-caption"><input type="checkbox" checked={archived} onChange={e=>setArchived(e.target.checked)}/>{t($=>$.cortex_bulk.archived_tables)}</label>
      {restore.error && <p role="alert">{restore.error.message}</p>}
      <ul aria-labelledby={TABLES_HEADING_ID}>
        {rows.map((collection) => {
          const active = collection.id === activeCollectionId;
          const deletable = canDelete(collection);
          if (archived) return <li key={collection.id} className="flex items-center gap-2 px-2 py-1 text-label"><span className="min-w-0 flex-1 truncate">{collection.icon} {collection.name}</span>{deletable && <Button size="xs" variant="outline" disabled={restore.isPending} onClick={()=>restore.mutate(collection.id)}>{t($=>$.cortex_bulk.restore)}</Button>}</li>;
          return (
            <li
              key={collection.id}
              className={cn(
                "group/row flex h-7 min-w-0 items-center rounded-md text-label transition-colors",
                deletable && "pr-1",
                active
                  ? "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
                  : "hover:bg-sidebar-accent/60",
              )}
            >
              <AppLink
                href={paths.collectionDetail(collection.id)}
                aria-current={active ? "page" : undefined}
                className="flex min-w-0 flex-1 items-center gap-2 self-stretch px-2"
              >
                {collection.icon ? <span aria-hidden>{collection.icon}</span> : <Table2 className="size-3.5 shrink-0 text-muted-foreground" />}
                <span className="min-w-0 flex-1 truncate">{collection.name}</span>
                {collection.record_count !== undefined && (
                  <span
                    className={cn(
                      "shrink-0 text-caption tabular-nums text-muted-foreground",
                      // The count makes way for the row menu.
                      deletable &&
                        "group-hover/row:hidden group-focus-within/row:hidden group-has-data-popup-open/row:hidden [@media(hover:none)]:hidden",
                    )}
                  >
                    {collection.record_count}
                  </span>
                )}
              </AppLink>
              {deletable && (
                <RowMenu
                  label={t(($) => $.cortex_docs.row_actions, {
                    name: collection.name,
                  })}
                  deleteLabel={t(($) => $.cortex_docs.delete_table)}
                  onDelete={() => {
                    setDeleting(collection);
                    setConfirmOpen(true);
                  }}
                />
              )}
            </li>
          );
        })}
      </ul>
      {!isLoading && !error && rows.length === 0 && (
        <p className="px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.no_tables)}
        </p>
      )}
      {isLoading && (
        <p role="status" className="px-2 py-1 text-caption text-muted-foreground">
          {t(($) => $.cortex_docs.loading)}
        </p>
      )}
      {error && (
        <p role="alert" className="px-2 py-1 text-caption text-destructive">
          {error.message}
        </p>
      )}
      <DeleteConfirm
        open={confirmOpen}
        name={deleting?.name ?? ""}
        description={t(($) => $.cortex_docs.delete_table_description)}
        pending={remove.isPending}
        onConfirm={() => deleting && remove.mutate(deleting)}
        onClose={() => setConfirmOpen(false)}
      />
    </NavigatorShell>
  );
}
