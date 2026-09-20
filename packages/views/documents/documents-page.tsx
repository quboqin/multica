"use client";

import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUp,
  FilePlus,
  FileText,
  History,
  MoreHorizontal,
  Plus,
  Share2,
  Star,
  Trash2,
} from "lucide-react";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import {
  documentKeys,
  documentPath,
  documentTreeOptions,
  useDocumentPreferences,
} from "@multica/core/documents";
import {
  issueDetailOptions,
  issueTimelineOptions,
} from "@multica/core/issues/queries";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { useActorName } from "@multica/core/workspace/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { cn } from "@multica/ui/lib/utils";
import { ActorAvatar } from "../common/actor-avatar";
import { DocumentNavigator } from "../cortex";
import {
  useCreateDocument,
  useDocumentCommand,
} from "../cortex/use-document-commands";
import { useIssueActions } from "../issues/actions/use-issue-actions";
import { IssueDetail } from "../issues/components/issue-detail";
import { AppLink, useNavigation } from "../navigation";
import { useT, useTimeAgo } from "../i18n";
import { DocumentRail, type DocumentRailTab } from "./document-rail";
import {
  documentLifecycle,
  incomingReferences,
  type DocumentLifecycle,
} from "./document-structure";

const emptyIds: string[] = [];

export function DocumentsPage({ documentId }: { documentId?: string }) {
  return (
    <div className="flex min-h-0 min-w-0 flex-1">
      <DocumentNavigator activeDocumentId={documentId} />
      {documentId ? (
        <DocumentView key={documentId} documentId={documentId} />
      ) : (
        <DocumentsEmptyState />
      )}
    </div>
  );
}

function DocumentsEmptyState() {
  const { t } = useT("issues");
  const ws = useCurrentWorkspace();
  const creator = useCreateDocument(ws?.id ?? "");
  return (
    <main className="hidden min-w-0 flex-1 flex-col items-center justify-center gap-3 p-6 text-center md:flex">
      <FileText className="size-8 text-muted-foreground" aria-hidden />
      <h1 className="text-title-sm font-medium">
        {t(($) => $.cortex_docs.empty_title)}
      </h1>
      <Button
        disabled={creator.isPending || !ws}
        aria-busy={creator.isPending}
        onClick={() => void creator.create({}).catch(() => undefined)}
      >
        <Plus />
        {t(($) => $.cortex_docs.new_document)}
      </Button>
      {creator.error && (
        <p role="alert" className="text-caption text-destructive">
          {creator.error.message}
        </p>
      )}
    </main>
  );
}

const LIFECYCLE_DOT: Record<DocumentLifecycle, string> = {
  draft: "bg-muted-foreground/60",
  reviewing: "bg-info",
  published: "bg-success",
};
const LIFECYCLE_CHIP: Record<DocumentLifecycle, string> = {
  draft: "bg-muted text-muted-foreground",
  reviewing: "bg-info/10 text-info",
  published: "bg-success/10 text-success",
};

function DocumentView({ documentId }: { documentId: string }) {
  const { t } = useT("issues");
  const ws = useCurrentWorkspace();
  const wsId = ws?.id ?? "";
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const queryClient = useQueryClient();
  const timeAgo = useTimeAgo();
  const { getActorName } = useActorName();
  const user = useAuthStore((state) => state.user);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const role = members.find((member) => member.user_id === user?.id)?.role;
  const { data: documents = [] } = useQuery(documentTreeOptions(wsId));
  const { data: issue } = useQuery(issueDetailOptions(wsId, documentId));
  const { data: timeline = [] } = useQuery(issueTimelineOptions(documentId));
  const actions = useIssueActions(issue ?? null);
  const command = useDocumentCommand(wsId);
  const creator = useCreateDocument(wsId);
  const [tab, setTab] = useState<DocumentRailTab>("outline");
  const visit = useDocumentPreferences((state) => state.visit);
  const favorites = useDocumentPreferences(
    (state) => state.favorites[wsId] ?? emptyIds,
  );
  const toggleFavorite = useDocumentPreferences(
    (state) => state.toggleFavorite,
  );
  const draft = useDocumentPreferences(
    (state) => state.drafts[JSON.stringify([wsId, documentId])],
  );
  useEffect(() => {
    if (wsId) visit(wsId, documentId);
  }, [documentId, wsId, visit]);

  const trail = documentPath(documents, documentId);
  const children = documents.filter(
    (doc) => doc.parent_issue_id === documentId,
  );
  const incoming = useMemo(
    () => incomingReferences(documents, documentId),
    [documents, documentId],
  );
  const incomingCount = incoming.reduce((sum, entry) => sum + entry.count, 0);
  const commentCount = timeline.filter(
    (entry) => entry.type === "comment",
  ).length;
  const favorite = favorites.includes(documentId);
  const lifecycle = issue ? documentLifecycle(issue) : "draft";
  const canPublish = role === "owner" || role === "admin";
  const hasDraft = !!draft;
  const body = draft?.body ?? issue?.description ?? "";

  const transition = (action: "review" | "publish") => {
    if (!issue) return;
    command.mutate(() =>
      api.transitionDocument(issue.id, action, issue.document_revision ?? 1),
    );
  };
  const moveUp = () => {
    if (!issue) return;
    const siblings = documents.filter(
      (item) =>
        item.parent_issue_id === issue.parent_issue_id &&
        item.project_id === issue.project_id,
    );
    const index = siblings.findIndex((item) => item.id === issue.id);
    const previous = siblings[index - 1];
    const before = siblings[index - 2];
    if (!previous) return;
    command.mutate(() =>
      api.moveDocument(
        issue.id,
        issue.parent_issue_id,
        before
          ? (before.position + previous.position) / 2
          : previous.position - 1,
        previous.id,
      ),
    );
  };
  const scrollToDiscussion = () =>
    document
      .querySelector("[data-document-discussion]")
      ?.scrollIntoView({ behavior: "smooth", block: "start" });

  const lifecycleLabel = t(($) => $.cortex_docs[`status_${lifecycle}`]);
  const primaryAction =
    lifecycle === "draft" ? (
      <Tooltip>
        <TooltipTrigger
          render={
            <span className="inline-flex">
              <Button
                size="sm"
                disabled={command.isPending || hasDraft || !issue}
                onClick={() => transition("review")}
              >
                {t(($) => $.cortex_docs.submit_review)}
              </Button>
            </span>
          }
        />
        {hasDraft && (
          <TooltipContent side="bottom">
            {t(($) => $.cortex_docs.unsaved_blocks_transition)}
          </TooltipContent>
        )}
      </Tooltip>
    ) : lifecycle === "reviewing" && canPublish ? (
      <Button
        size="sm"
        disabled={command.isPending || hasDraft || !issue}
        onClick={() => transition("publish")}
      >
        {t(($) => $.cortex_docs.publish)}
      </Button>
    ) : null;

  return (
    <>
      <main className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
          <nav
            aria-label={t(($) => $.cortex_docs.breadcrumb)}
            className="flex min-w-0 flex-1 items-center gap-1.5 text-label text-muted-foreground"
          >
            <AppLink
              href={paths.documents()}
              className="shrink-0 hover:text-foreground"
            >
              {t(($) => $.cortex_docs.documents)}
            </AppLink>
            {trail.map((doc, index) => {
              const last = index === trail.length - 1;
              return (
                <span
                  key={doc.id}
                  className={cn(
                    "flex items-center gap-1.5",
                    last ? "min-w-0" : "shrink-0",
                  )}
                >
                  <span aria-hidden className="text-faint-foreground">
                    /
                  </span>
                  <AppLink
                    href={paths.documentDetail(doc.id)}
                    aria-current={last ? "page" : undefined}
                    className={cn(
                      "truncate hover:text-foreground",
                      !last && "max-w-32",
                      last && "font-medium text-foreground",
                    )}
                  >
                    {doc.title || t(($) => $.cortex_docs.untitled)}
                  </AppLink>
                </span>
              );
            })}
          </nav>
          {issue && (
            <span
              className={cn(
                "shrink-0 rounded-md px-2 py-0.5 text-caption font-medium",
                LIFECYCLE_CHIP[lifecycle],
              )}
            >
              {lifecycleLabel}
            </span>
          )}
          {primaryAction}
          <Button
            variant="ghost"
            size="sm"
            className="text-muted-foreground"
            onClick={() => setTab("versions")}
          >
            <History />
            <span className="max-2xl:sr-only">
              {t(($) => $.cortex_docs.versions)}
            </span>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="text-muted-foreground"
            onClick={() => void actions.copyLink()}
          >
            <Share2 />
            <span className="max-2xl:sr-only">
              {t(($) => $.cortex_docs.share)}
            </span>
          </Button>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="text-muted-foreground"
                  aria-label={t(($) => $.cortex_docs.more)}
                >
                  <MoreHorizontal />
                </Button>
              }
            />
            <DropdownMenuContent align="end" className="w-48">
              <DropdownMenuItem onClick={() => toggleFavorite(wsId, documentId)}>
                <Star className={cn(favorite && "fill-current")} />
                {t(($) =>
                  favorite ? $.cortex_docs.unfavorite : $.cortex_docs.favorite,
                )}
              </DropdownMenuItem>
              <DropdownMenuItem
                disabled={!issue || creator.isPending}
                onClick={() => {
                  if (issue)
                    void creator.create({ parent: issue }).catch(() => undefined);
                }}
              >
                <FilePlus />
                {t(($) => $.cortex_docs.add_child)}
              </DropdownMenuItem>
              <DropdownMenuItem disabled={!issue} onClick={moveUp}>
                <ArrowUp />
                {t(($) => $.cortex_docs.move_up)}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                disabled={!issue}
                onClick={() =>
                  actions.openDeleteConfirm({
                    onDeletedFallbackPath: paths.documents(),
                  })
                }
              >
                <Trash2 />
                {t(($) => $.cortex_docs.delete)}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </header>
        {(command.error || creator.error) && (
          <p role="alert" className="border-b px-4 py-2 text-caption text-destructive">
            {(command.error ?? creator.error)?.message}
          </p>
        )}
        <IssueDetail
          issueId={documentId}
          variant="document"
          defaultSidebarOpen={false}
          onDelete={() => {
            void queryClient.invalidateQueries({
              queryKey: documentKeys.all(wsId),
            });
            navigation.push(paths.documents());
          }}
          documentSlots={
            issue
              ? {
                  beforeTitle: (
                    <div className="mb-3 flex items-center gap-2 text-caption text-muted-foreground">
                      <span className="inline-flex items-center gap-1.5">
                        <span
                          aria-hidden
                          className={cn(
                            "size-2 rounded-full",
                            LIFECYCLE_DOT[lifecycle],
                          )}
                        />
                        {lifecycleLabel}
                      </span>
                      <span aria-hidden>·</span>
                      <span className="rounded bg-brand/10 px-1.5 py-0.5 font-mono text-micro text-brand">
                        {t(($) => $.cortex_docs.kind_doc)}
                      </span>
                    </div>
                  ),
                  afterTitle: (
                    <AuthorLine
                      issue={issue}
                      authorName={getActorName(
                        issue.creator_type,
                        issue.creator_id,
                      )}
                      edited={timeAgo(issue.updated_at)}
                      childCount={children.length}
                      referenceCount={incomingCount}
                      onReferences={() => setTab("backlinks")}
                    />
                  ),
                  afterBody:
                    children.length > 0 ? (
                      <ChildPages pages={children} />
                    ) : undefined,
                }
              : undefined
          }
        />
      </main>
      {issue && (
        <DocumentRail
          issue={issue}
          body={body}
          documents={documents}
          incoming={incoming}
          lifecycleLabel={lifecycleLabel}
          commentCount={commentCount}
          tab={tab}
          onTabChange={setTab}
          onJumpToComments={scrollToDiscussion}
        />
      )}
    </>
  );
}

function AuthorLine({
  issue,
  authorName,
  edited,
  childCount,
  referenceCount,
  onReferences,
}: {
  issue: Issue;
  authorName: string;
  edited: string;
  childCount: number;
  referenceCount: number;
  onReferences: () => void;
}) {
  const { t } = useT("issues");
  return (
    <div className="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-caption text-muted-foreground">
      <span className="inline-flex items-center gap-1.5">
        <ActorAvatar
          actorType={issue.creator_type}
          actorId={issue.creator_id}
          size="sm"
        />
        <span className="text-foreground">{authorName}</span>
      </span>
      <span aria-hidden>·</span>
      <span>{t(($) => $.cortex_docs.edited_ago, { time: edited })}</span>
      {childCount > 0 && (
        <>
          <span aria-hidden>·</span>
          <span>{t(($) => $.cortex_docs.child_count, { count: childCount })}</span>
        </>
      )}
      <span aria-hidden>·</span>
      <button
        type="button"
        className="text-brand hover:underline"
        onClick={onReferences}
      >
        {t(($) => $.cortex_docs.reference_count, { count: referenceCount })}
      </button>
    </div>
  );
}

function ChildPages({ pages }: { pages: Issue[] }) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  return (
    <section className="mt-10" aria-labelledby="document-child-pages">
      <h2
        id="document-child-pages"
        className="mb-2 text-caption font-medium text-muted-foreground"
      >
        {t(($) => $.cortex_docs.child_pages)}
      </h2>
      <ul className="divide-y rounded-lg border">
        {pages.map((page) => (
          <li key={page.id}>
            <AppLink
              href={paths.documentDetail(page.id)}
              className="flex items-center gap-2 px-3 py-2 text-body transition-colors hover:bg-accent/50"
            >
              <FileText className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">
                {page.title || t(($) => $.cortex_docs.untitled)}
              </span>
              <span
                aria-hidden
                className={cn(
                  "size-2 rounded-full",
                  LIFECYCLE_DOT[documentLifecycle(page)],
                )}
              />
            </AppLink>
          </li>
        ))}
      </ul>
    </section>
  );
}
