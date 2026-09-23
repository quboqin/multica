"use client";

import { useEffect, useState, type ReactElement, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Unlink } from "lucide-react";
import { api } from "@multica/core/api";
import {
  collectionListOptions,
  type CollectionField,
  type CollectionRecord,
  type RecordLink,
} from "@multica/core/collections";
import { useWorkspaceId } from "@multica/core/hooks";
import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import type { IssueStatusCatalog } from "@multica/core/issue-statuses/queries";
import { useWorkspacePaths } from "@multica/core/paths";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../i18n";
import {
  PickerEmpty,
  PickerItem,
  PickerSection,
  PropertyPicker,
} from "../issues/components/pickers";
import { StatusIcon } from "../issues/components/status-icon";
import { AppLink } from "../navigation";
import { linkLabel, recordLinks, relatesToTasks } from "./collection-fields";

const CHIP =
  "inline-flex max-w-40 shrink-0 items-center gap-1 rounded-sm px-1.5 py-px text-caption font-medium leading-5";

function LinkStatusIcon({
  link,
  statuses,
}: {
  link: RecordLink;
  statuses: IssueStatusCatalog;
}) {
  return (
    <StatusIcon
      status={link.status}
      category={statuses.categoryOf(link.status)}
      color={statuses.colorOf(link.status)}
      icon={statuses.iconOf(link.status)}
      className="h-3.5 w-3.5 shrink-0"
    />
  );
}

/**
 * The links of one relation cell. A task reads as a blue chip with its status,
 * a record as a neutral one: following a task link leaves the table for work
 * that has an owner and a state, and the cell says so before the click. A link
 * whose target was deleted stays, struck through, rather than vanishing.
 */
export function RelationChips({
  links,
  compact = false,
}: {
  links: RecordLink[];
  compact?: boolean;
}) {
  const { t } = useT("issues");
  const statuses = useIssueStatuses(useWorkspaceId());
  if (!links.length)
    return compact ? null : <span className="text-muted-foreground/60">—</span>;
  return (
    // A table cell keeps its row height: links past its width are clipped, and
    // the record panel lists them all. Cards have the room to wrap.
    <span
      className={cn(
        "flex min-w-0 items-center gap-1",
        compact ? "flex-wrap" : "overflow-hidden",
      )}
    >
      {links.map((link) => {
        if (link.missing)
          return (
            <span
              key={link.id}
              className={cn(CHIP, "bg-muted text-muted-foreground line-through opacity-70")}
            >
              {t(($) => $.cortex_table.link_deleted)}
            </span>
          );
        if (link.to_type === "issue")
          return (
            <span
              key={link.id}
              title={link.title}
              className={cn(CHIP, "bg-info/10 text-info")}
            >
              <LinkStatusIcon link={link} statuses={statuses} />
              <span className="truncate">{link.identifier}</span>
            </span>
          );
        return (
          <span key={link.id} className={cn(CHIP, "bg-muted text-foreground")}>
            <span className="truncate">
              {link.title || t(($) => $.cortex_table.untitled)}
            </span>
          </span>
        );
      })}
    </span>
  );
}

interface Candidate {
  id: string;
  title: string;
  identifier?: string;
  status?: string;
}

/** Waits for typing to pause before a search goes out. */
function useDebounced(value: string, delay = 250) {
  const [settled, setSettled] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setSettled(value), delay);
    return () => clearTimeout(timer);
  }, [value, delay]);
  return settled;
}

/**
 * Picks what a relation cell links to. Before anything is typed, the current
 * links sit on top and unlink on click, above what the target offers. Typing
 * turns the whole list into search results over the field's target — workspace
 * tasks, or the records of one table — with a check on the ones already linked,
 * so Enter always acts on a match and never on a link that merely sat first.
 * It stays open between picks, like multi-select.
 */
export function RelationEditor({
  record,
  field,
  open,
  onOpenChange,
  onLink,
  onUnlink,
  trigger,
  triggerRender,
}: {
  record: CollectionRecord;
  field: CollectionField;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onLink: (toId: string) => Promise<unknown>;
  onUnlink: (linkId: string) => Promise<unknown>;
  trigger: ReactNode;
  triggerRender: ReactElement;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const statuses = useIssueStatuses(wsId);
  const links = recordLinks(record, field);
  const toTasks = relatesToTasks(field);
  const targetId = field.config.relation?.collection_id ?? "";
  const { data: tables = [] } = useQuery({
    ...collectionListOptions(wsId),
    enabled: open && !toTasks,
  });
  const targetName = toTasks
    ? t(($) => $.cortex_table.relation_tasks)
    : (tables.find((table) => table.id === targetId)?.name ??
      t(($) => $.cortex_table.relation_records));

  const [query, setQuery] = useState("");
  const search = useDebounced(query.trim());
  const results = useQuery({
    // Outside the collections key on purpose: every record event refreshes
    // that whole tree, and a search the user is reading should hold still.
    queryKey: ["relation-search", wsId, toTasks ? "issue" : targetId, search],
    queryFn: async ({ signal }): Promise<Candidate[]> => {
      if (toTasks) {
        const found = await api.searchIssues({
          q: search,
          kind: "task",
          limit: 20,
          include_closed: true,
          signal,
        });
        return found.issues.map((issue) => ({
          id: issue.id,
          title: issue.title,
          identifier: issue.identifier,
          status: issue.status,
        }));
      }
      const page = await api.listCollectionRecords(
        targetId,
        search ? { search } : {},
        null,
        signal,
        wsId,
      );
      return page.records.map((row) => ({ id: row.id, title: row.title }));
    },
    // Task search needs something to search for; a table lists its newest rows.
    enabled: open && !!wsId && (toTasks ? search !== "" : targetId !== ""),
    staleTime: 10_000,
  });

  // A pick shows as taken at once and settles when the server answers.
  const [pending, setPending] = useState<ReadonlySet<string>>(new Set());
  const run = (key: string, action: () => Promise<unknown>) => {
    if (pending.has(key)) return;
    setPending((current) => new Set(current).add(key));
    void action().finally(() =>
      setPending((current) => {
        const next = new Set(current);
        next.delete(key);
        return next;
      }),
    );
  };

  const searching = query.trim() !== "";
  const linkOf = new Map(links.map((link) => [link.to_id, link]));
  const candidates = (results.data ?? []).filter(
    (item) =>
      // A row of the same table cannot point at itself.
      item.id !== record.id &&
      // Unsearched, what is linked is already listed above.
      (searching || !linkOf.has(item.id)),
  );
  const waiting = results.isFetching || (searching && query.trim() !== search);

  return (
    <PropertyPicker
      open={open}
      onOpenChange={onOpenChange}
      align="start"
      width="w-80"
      searchable
      searchPlaceholder={t(($) => $.cortex_table.relation_search, { name: targetName })}
      onSearchChange={setQuery}
      trigger={trigger}
      triggerRender={triggerRender}
    >
      {!searching && links.length > 0 && (
        <PickerSection label={t(($) => $.cortex_table.relation_linked)}>
          {links.map((link) => (
            <PickerItem
              key={link.id}
              selected
              disabled={pending.has(link.id)}
              onClick={() => run(link.id, () => onUnlink(link.id))}
            >
              {link.missing ? (
                <span className="truncate text-muted-foreground line-through">
                  {t(($) => $.cortex_table.link_deleted)}
                </span>
              ) : (
                <CandidateLabel
                  item={{ ...link, id: link.to_id }}
                  statuses={statuses}
                />
              )}
            </PickerItem>
          ))}
        </PickerSection>
      )}
      <PickerSection label={targetName}>
        {candidates.map((item) => {
          const link = linkOf.get(item.id);
          const busy = pending.has(item.id) || (!!link && pending.has(link.id));
          return (
            <PickerItem
              key={item.id}
              selected={link ? !busy : busy}
              disabled={busy}
              onClick={() =>
                link
                  ? run(link.id, () => onUnlink(link.id))
                  : run(item.id, () => onLink(item.id))
              }
            >
              <CandidateLabel item={item} statuses={statuses} />
            </PickerItem>
          );
        })}
        {candidates.length === 0 &&
          (toTasks && !searching ? (
            <p className="px-2 py-3 text-center text-body text-muted-foreground">
              {t(($) => $.cortex_table.relation_type_to_search)}
            </p>
          ) : waiting ? (
            <p role="status" className="px-2 py-3 text-center text-body text-muted-foreground">
              {t(($) => $.cortex.loading)}
            </p>
          ) : results.isError ? (
            <p role="alert" className="px-2 py-3 text-center text-body text-destructive">
              {results.error instanceof Error ? results.error.message : String(results.error)}
            </p>
          ) : (
            <PickerEmpty />
          ))}
      </PickerSection>
    </PropertyPicker>
  );
}

function CandidateLabel({
  item,
  statuses,
}: {
  item: Candidate;
  statuses: IssueStatusCatalog;
}) {
  const { t } = useT("issues");
  return (
    <>
      {item.identifier && item.status !== undefined && (
        <>
          <StatusIcon
            status={item.status}
            category={statuses.categoryOf(item.status)}
            color={statuses.colorOf(item.status)}
            icon={statuses.iconOf(item.status)}
            className="h-3.5 w-3.5 shrink-0"
          />
          <span className="shrink-0 text-muted-foreground">{item.identifier}</span>
        </>
      )}
      <span className="truncate">
        {item.title || t(($) => $.cortex_table.untitled)}
      </span>
    </>
  );
}

/**
 * The links of one relation field as rows that open their target, for the
 * record panel where there is room for a title. Each row can be unlinked; a
 * dead link cannot be opened but can still be removed.
 */
export function RelationList({
  links,
  onUnlink,
}: {
  links: RecordLink[];
  onUnlink?: (link: RecordLink) => void;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const statuses = useIssueStatuses(useWorkspaceId());
  if (!links.length) return null;
  return (
    <ul className="space-y-1.5">
      {links.map((link) => {
        const name = link.missing
          ? t(($) => $.cortex_table.link_deleted)
          : linkLabel(link) || t(($) => $.cortex_table.untitled);
        const body = link.missing ? (
          <span className="truncate text-muted-foreground line-through">{name}</span>
        ) : (
          <>
            {link.to_type === "issue" && (
              <>
                <LinkStatusIcon link={link} statuses={statuses} />
                <span className="shrink-0 font-medium text-muted-foreground">
                  {link.identifier}
                </span>
              </>
            )}
            <span className="truncate group-hover/link:underline">
              {link.title || t(($) => $.cortex_table.untitled)}
            </span>
          </>
        );
        return (
          <li
            key={link.id}
            className="group/row flex items-center gap-1 rounded-md border bg-muted/30 pl-2.5 pr-1"
          >
            {link.missing ? (
              <span className="flex min-w-0 flex-1 items-center gap-2 py-1.5 text-label">
                {body}
              </span>
            ) : (
              <AppLink
                href={
                  link.to_type === "issue"
                    ? paths.issueDetail(link.to_id)
                    : `${paths.collectionDetail(link.collection_id)}?record=${link.to_id}`
                }
                className="group/link flex min-w-0 flex-1 items-center gap-2 py-1.5 text-label"
              >
                {body}
              </AppLink>
            )}
            {onUnlink && <button
              type="button"
              aria-label={t(($) => $.cortex_table.unlink, { name })}
              title={t(($) => $.cortex_table.unlink, { name })}
              onClick={() => onUnlink(link)}
              className="shrink-0 rounded-xs p-1 text-muted-foreground opacity-0 transition-opacity hover:bg-accent hover:text-foreground focus-visible:opacity-100 group-hover/row:opacity-100 [@media(hover:none)]:opacity-100"
            >
              <Unlink className="size-3.5" />
            </button>}
          </li>
        );
      })}
    </ul>
  );
}
