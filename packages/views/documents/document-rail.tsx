"use client";

import { useMemo, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { AtSign, FileText, LayoutGrid } from "lucide-react";
import { useWorkspaceId } from "@multica/core/hooks";
import { issueViewDetailOptions } from "@multica/core/issue-views/queries";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@multica/ui/components/ui/tabs";
import { AppLink } from "../navigation";
import { useT, useTimeAgo } from "../i18n";
import {
  parseOutline,
  parseReferences,
  scrollToHeading,
  type DocumentReference,
} from "./document-structure";

export type DocumentRailTab = "outline" | "backlinks" | "versions" | "comments";

export function DocumentRail({
  issue,
  body,
  documents,
  incoming,
  lifecycleLabel,
  commentCount,
  tab,
  onTabChange,
  onJumpToComments,
}: {
  issue: Issue;
  /** The body as the author currently sees it (local draft or saved). */
  body: string;
  documents: readonly Issue[];
  incoming: { doc: Issue; count: number }[];
  lifecycleLabel: string;
  commentCount: number;
  tab: DocumentRailTab;
  onTabChange: (tab: DocumentRailTab) => void;
  onJumpToComments: () => void;
}) {
  const { t } = useT("issues");
  const incomingCount = incoming.reduce((sum, entry) => sum + entry.count, 0);
  return (
    <aside
      aria-label={t(($) => $.cortex_docs.rail)}
      className="hidden w-[280px] shrink-0 flex-col border-l xl:flex"
    >
      <Tabs
        value={tab}
        onValueChange={(value) => onTabChange(value as DocumentRailTab)}
        className="min-h-0 flex-1 gap-0"
      >
        <div className="flex h-12 shrink-0 items-center border-b px-3">
          <TabsList variant="line" className="h-8 w-full justify-start gap-0.5">
            <RailTrigger value="outline">
              {t(($) => $.cortex_docs.tab_outline)}
            </RailTrigger>
            <RailTrigger value="backlinks" count={incomingCount}>
              {t(($) => $.cortex_docs.tab_backlinks)}
            </RailTrigger>
            <RailTrigger value="versions">
              {t(($) => $.cortex_docs.tab_versions)}
            </RailTrigger>
            <RailTrigger value="comments" count={commentCount}>
              {t(($) => $.cortex_docs.tab_comments)}
            </RailTrigger>
          </TabsList>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-3">
          <TabsContent value="outline">
            <Outline body={body} />
          </TabsContent>
          <TabsContent value="backlinks" className="space-y-4">
            <Backlinks body={body} documents={documents} incoming={incoming} />
          </TabsContent>
          <TabsContent value="versions">
            <Versions issue={issue} lifecycleLabel={lifecycleLabel} />
          </TabsContent>
          <TabsContent value="comments" className="space-y-3">
            <p className="text-body">
              {t(($) => $.cortex_docs.comment_count, { count: commentCount })}
            </p>
            <Button variant="outline" size="sm" onClick={onJumpToComments}>
              {t(($) => $.cortex_docs.jump_to_comments)}
            </Button>
          </TabsContent>
        </div>
      </Tabs>
    </aside>
  );
}

function RailTrigger({
  value,
  count,
  children,
}: {
  value: DocumentRailTab;
  count?: number;
  children: string;
}) {
  return (
    <TabsTrigger value={value} className="flex-none px-1.5 text-caption">
      {children}
      {count ? (
        <span className="text-caption tabular-nums text-brand">{count}</span>
      ) : null}
    </TabsTrigger>
  );
}

function Outline({ body }: { body: string }) {
  const { t } = useT("issues");
  const headings = useMemo(() => parseOutline(body), [body]);
  if (!headings.length)
    return (
      <p className="text-caption text-muted-foreground">
        {t(($) => $.cortex_docs.outline_empty)}
      </p>
    );
  return (
    <nav aria-label={t(($) => $.cortex_docs.tab_outline)}>
      <ul className="border-l">
        {headings.map((heading, index) => (
          <li key={`${index}:${heading.text}`}>
            <button
              type="button"
              className="-ml-px block w-full truncate border-l-2 border-transparent py-1 pr-1 text-left text-label text-muted-foreground transition-colors hover:border-foreground/40 hover:text-foreground"
              style={{ paddingInlineStart: 8 + (heading.level - 1) * 12 }}
              onClick={() => {
                const root = document.querySelector("[data-document-body]");
                if (root) scrollToHeading(root, heading);
              }}
            >
              {heading.text}
            </button>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function RailSection({
  title,
  children,
}: {
  title: string;
  children: ReactNode;
}) {
  return (
    <section>
      <h3 className="mb-2 text-caption font-medium text-muted-foreground">
        {title}
      </h3>
      {children}
    </section>
  );
}

const cardClass =
  "block rounded-lg border bg-card px-3 py-2 transition-colors hover:bg-accent/50";

function Backlinks({
  body,
  documents,
  incoming,
}: {
  body: string;
  documents: readonly Issue[];
  incoming: { doc: Issue; count: number }[];
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const outgoing = useMemo(() => parseReferences(body), [body]);
  const none = (
    <p className="text-caption text-muted-foreground">
      {t(($) => $.cortex_docs.no_references)}
    </p>
  );
  return (
    <>
      <RailSection title={t(($) => $.cortex_docs.outgoing)}>
        {outgoing.length ? (
          <ul className="space-y-2">
            {outgoing.map((ref) => (
              <li key={`${ref.kind}:${ref.id}`}>
                {ref.kind === "view" ? (
                  <ViewReferenceCard reference={ref} />
                ) : (
                  <IssueReferenceCard reference={ref} documents={documents} />
                )}
              </li>
            ))}
          </ul>
        ) : (
          none
        )}
      </RailSection>
      <RailSection title={t(($) => $.cortex_docs.incoming)}>
        {incoming.length ? (
          <ul className="space-y-2">
            {incoming.map(({ doc, count }) => (
              <li key={doc.id}>
                <AppLink href={paths.documentDetail(doc.id)} className={cardClass}>
                  <span className="flex items-center gap-1.5 text-label font-medium">
                    <FileText className="size-3.5 shrink-0 text-brand" />
                    <span className="truncate">
                      {doc.title || t(($) => $.cortex_docs.untitled)}
                    </span>
                  </span>
                  <span className="mt-0.5 block text-caption text-muted-foreground">
                    {t(($) => $.cortex_docs.inline_mentions, { count })}
                  </span>
                </AppLink>
              </li>
            ))}
          </ul>
        ) : (
          none
        )}
      </RailSection>
    </>
  );
}

function IssueReferenceCard({
  reference,
  documents,
}: {
  reference: DocumentReference;
  documents: readonly Issue[];
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const doc = documents.find((item) => item.id === reference.id);
  return (
    <AppLink
      href={doc ? paths.documentDetail(doc.id) : paths.issueDetail(reference.id)}
      className={cardClass}
    >
      <span className="flex items-center gap-1.5 text-label font-medium">
        {doc ? (
          <FileText className="size-3.5 shrink-0 text-brand" />
        ) : (
          <AtSign className="size-3.5 shrink-0 text-muted-foreground" />
        )}
        <span className="truncate">{doc?.title || reference.label}</span>
      </span>
      <span className="mt-0.5 block text-caption text-muted-foreground">
        {t(($) => $.cortex_docs.inline_mention)}
      </span>
    </AppLink>
  );
}

function ViewReferenceCard({ reference }: { reference: DocumentReference }) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const paths = useWorkspacePaths();
  const { data: view, isError } = useQuery(
    issueViewDetailOptions(wsId, reference.id),
  );
  const href = view?.collection_id
    ? paths.collectionDetail(view.collection_id)
    : paths.issues();
  return (
    <AppLink href={href} className={cardClass}>
      <span className="flex items-center gap-1.5 text-label font-medium">
        <LayoutGrid className="size-3.5 shrink-0 text-brand" />
        <span className="truncate">
          {view?.name ||
            (isError
              ? t(($) => $.cortex_docs.view_unavailable)
              : t(($) => $.cortex_docs.saved_view))}
        </span>
      </span>
      <span className="mt-0.5 block text-caption text-muted-foreground">
        {t(($) => $.cortex_docs.live_view)}
      </span>
    </AppLink>
  );
}

function Versions({
  issue,
  lifecycleLabel,
}: {
  issue: Issue;
  lifecycleLabel: string;
}) {
  const { t } = useT("issues");
  const timeAgo = useTimeAgo();
  const rows: [string, string][] = [
    [
      t(($) => $.cortex_docs.version_current),
      `r${issue.document_revision ?? 1}`,
    ],
    [t(($) => $.cortex_docs.version_status), lifecycleLabel],
    [t(($) => $.cortex_docs.version_edited), timeAgo(issue.updated_at)],
    [
      t(($) => $.cortex_docs.version_created),
      new Date(issue.created_at).toLocaleDateString(),
    ],
  ];
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-label">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="truncate tabular-nums">{value}</dd>
          </div>
        ))}
      </dl>
      <p className="text-caption text-muted-foreground">
        {t(($) => $.cortex_docs.version_history_pending)}
      </p>
    </div>
  );
}
