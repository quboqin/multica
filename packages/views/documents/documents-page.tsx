"use client";

import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, FileText, Plus, Star } from "lucide-react";
import { useAuthStore } from "@multica/core/auth";
import { memberListOptions } from "@multica/core/workspace/queries";
import { api } from "@multica/core/api";
import {
  documentKeys,
  documentPath,
  documentTreeOptions,
  useDocumentPreferences,
} from "@multica/core/documents";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { projectListOptions } from "@multica/core/projects/queries";
import { issueKeys } from "@multica/core/issues/queries";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { IssueDetail } from "../issues/components/issue-detail";
import { AppLink, useNavigation } from "../navigation";
import { useT } from "../i18n";

const emptyIds: string[] = [];
export function DocumentsPage({ documentId }: { documentId?: string }) {
  const ws = useCurrentWorkspace();
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const { t } = useT("issues");
  const queryClient = useQueryClient();
  const wsId = ws?.id ?? "";
  const user = useAuthStore((state) => state.user);
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const role = members.find((member) => member.user_id === user?.id)?.role;
  const {
    data: documents = [],
    isLoading,
    error,
  } = useQuery(documentTreeOptions(wsId));
  const { data: projects = [] } = useQuery(projectListOptions(wsId));
  const [projectId, setProjectId] = useState("");
  const [filter, setFilter] = useState("all");
  const [search, setSearch] = useState("");
  const [title, setTitle] = useState("");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const favorites = useDocumentPreferences(
    (state) => state.favorites[wsId] ?? emptyIds,
  );
  const recent = useDocumentPreferences(
    (state) => state.recent[wsId] ?? emptyIds,
  );
  const visit = useDocumentPreferences((state) => state.visit);
  const toggleFavorite = useDocumentPreferences(
    (state) => state.toggleFavorite,
  );
  useEffect(() => {
    if (documentId && wsId) visit(wsId, documentId);
  }, [documentId, wsId, visit]);
  const hasDraft = useDocumentPreferences(
    (state) => !!state.drafts[JSON.stringify([wsId, documentId])],
  );
  const current = documents.find((doc) => doc.id === documentId);
  const command = useMutation({
    mutationFn: async (action: () => Promise<Issue>) => action(),
    onSuccess: (doc) => {
      queryClient.setQueryData<Issue>(issueKeys.detail(wsId, doc.id), (old) =>
        !old || (old.revision ?? 0) <= (doc.revision ?? 0) ? doc : old,
      );
      void queryClient.invalidateQueries({ queryKey: documentKeys.all(wsId) });
    },
  });
  const create = async (parent?: Issue) => {
    if (!title.trim()) return;
    const doc = await command.mutateAsync(() =>
      api.createIssue({
        kind: "doc",
        title: title.trim(),
        parent_issue_id: parent?.id,
        project_id: parent?.project_id ?? (projectId || undefined),
      }),
    );
    setTitle("");
    navigation.push(paths.documentDetail(doc.id));
  };
  const visible = (
    filter === "recent"
      ? [...documents].sort(
          (a, b) => recent.indexOf(a.id) - recent.indexOf(b.id),
        )
      : documents
  ).filter(
    (doc) =>
      (!projectId || doc.project_id === projectId) &&
      (!search ||
        (doc.title + " " + (doc.description ?? ""))
          .toLowerCase()
          .includes(search.toLowerCase())) &&
      (filter !== "favorites" || favorites.includes(doc.id)) &&
      (filter !== "recent" || recent.includes(doc.id)),
  );
  const move = (
    id: string,
    parent: string | null,
    position: number,
    beforeId?: string,
  ) => command.mutate(() => api.moveDocument(id, parent, position, beforeId));
  const row = (doc: Issue, depth: number): React.ReactNode => (
    <div key={doc.id}>
      <div
        className="h-2 rounded hover:bg-primary/30"
        role="separator"
        aria-label={`${t(($) => $.cortex.move_before)} ${doc.title}`}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          event.preventDefault();
          event.stopPropagation();
          const id = event.dataTransfer.getData(
            "application/x-multica-document",
          );
          const siblings = documents.filter(
            (item) =>
              item.parent_issue_id === doc.parent_issue_id &&
              item.project_id === doc.project_id &&
              item.id !== id,
          );
          const index = siblings.findIndex((item) => item.id === doc.id);
          const previous = siblings[index - 1];
          if (id && id !== doc.id)
            move(
              id,
              doc.parent_issue_id,
              previous
                ? (previous.position + doc.position) / 2
                : doc.position - 1,
              doc.id,
            );
        }}
      />
      <div
        className="flex min-w-0 items-center gap-1 rounded hover:bg-accent"
        style={{ paddingInlineStart: depth * 14 }}
        draggable
        onDragStart={(event) => {
          event.dataTransfer.setData("application/x-multica-document", doc.id);
          event.dataTransfer.effectAllowed = "move";
        }}
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          event.preventDefault();
          event.stopPropagation();
          const id = event.dataTransfer.getData(
            "application/x-multica-document",
          );
          if (id && id !== doc.id) move(id, doc.id, 0);
        }}
      >
        <button
          aria-label={t(($) => $.cortex.expand)}
          aria-expanded={!collapsed.has(doc.id)}
          className="p-1"
          onClick={() =>
            setCollapsed((old) => {
              const next = new Set(old);
              if (next.has(doc.id)) next.delete(doc.id);
              else next.add(doc.id);
              return next;
            })
          }
        >
          <ChevronRight
            className={`size-3 ${collapsed.has(doc.id) ? "" : "rotate-90"}`}
          />
        </button>
        <AppLink
          className={`min-w-0 flex-1 truncate py-1 text-body-sm ${doc.id === documentId ? "font-semibold text-primary" : ""}`}
          href={paths.documentDetail(doc.id)}
        >
          {doc.title}
        </AppLink>
        <button
          className="p-1"
          aria-label={t(($) => $.cortex.favorite)}
          aria-pressed={favorites.includes(doc.id)}
          onClick={() => toggleFavorite(wsId, doc.id)}
        >
          <Star
            className={`size-3 ${favorites.includes(doc.id) ? "fill-current" : ""}`}
          />
        </button>
        <button
          className="px-1"
          aria-label={t(($) => $.cortex.move_before)}
          onClick={() => {
            const siblings = documents.filter(
              (item) =>
                item.parent_issue_id === doc.parent_issue_id &&
                item.project_id === doc.project_id,
            );
            const index = siblings.findIndex((item) => item.id === doc.id);
            const previous = siblings[index - 1];
            const before = siblings[index - 2];
            if (previous)
              move(
                doc.id,
                doc.parent_issue_id,
                before
                  ? (before.position + previous.position) / 2
                  : previous.position - 1,
                previous.id,
              );
          }}
        >
          ↑
        </button>
      </div>
      {!collapsed.has(doc.id) &&
        filter === "all" &&
        !search &&
        visible
          .filter((child) => child.parent_issue_id === doc.id)
          .map((child) => row(child, depth + 1))}
    </div>
  );
  return (
    <div className="flex min-h-0 flex-1">
      <aside
        className="w-64 shrink-0 space-y-3 overflow-auto border-r p-3"
        aria-label={t(($) => $.cortex.documents)}
      >
        <h1 className="flex items-center gap-2 text-body font-medium">
          <FileText className="size-4" />
          {t(($) => $.cortex.documents)}
        </h1>
        <select
          aria-label={t(($) => $.cortex.scope)}
          className="w-full rounded border bg-background p-1"
          value={projectId}
          onChange={(event) => setProjectId(event.target.value)}
        >
          <option value="">{t(($) => $.cortex.workspace)}</option>
          {projects.map((project) => (
            <option key={project.id} value={project.id}>
              {project.title}
            </option>
          ))}
        </select>
        <Input
          aria-label={t(($) => $.cortex.search)}
          placeholder={t(($) => $.cortex.search)}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <div className="flex gap-1">
          {(["all", "favorites", "recent"] as const).map((value) => (
            <Button
              key={value}
              size="sm"
              variant={filter === value ? "secondary" : "ghost"}
              onClick={() => setFilter(value)}
            >
              {t(($) => $.cortex[value])}
            </Button>
          ))}
        </div>
        <form
          className="flex gap-1"
          onSubmit={(event) => {
            event.preventDefault();
            void create();
          }}
        >
          <Input
            aria-label={t(($) => $.cortex.title)}
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder={t(($) => $.cortex.title)}
          />
          <Button
            type="submit"
            size="icon-sm"
            disabled={!title.trim() || command.isPending}
            aria-label={t(($) => $.cortex.create)}
          >
            <Plus />
          </Button>
        </form>
        {current && (
          <Button
            size="sm"
            variant="outline"
            disabled={!title.trim() || command.isPending}
            onClick={() => void create(current)}
          >
            {t(($) => $.cortex.add_child)}
          </Button>
        )}
        <div
          className="min-h-12 rounded p-1"
          onDragOver={(event) => event.preventDefault()}
          onDrop={(event) => {
            event.preventDefault();
            const id = event.dataTransfer.getData(
              "application/x-multica-document",
            );
            if (id) move(id, null, 0);
          }}
        >
          <p className="mb-2 text-caption text-muted-foreground">
            {t(($) => $.cortex.root)}
          </p>
          {(filter === "all" && !search
            ? visible.filter(
                (doc) =>
                  !doc.parent_issue_id ||
                  !visible.some((parent) => parent.id === doc.parent_issue_id),
              )
            : filter === "recent"
              ? [...visible].sort(
                  (a, b) => recent.indexOf(a.id) - recent.indexOf(b.id),
                )
              : visible
          ).map((doc) => row(doc, 0))}
        </div>
        {(error || command.error) && (
          <p role="alert" className="text-caption text-destructive">
            {(error || command.error)?.message}
          </p>
        )}
        {isLoading && <p role="status">{t(($) => $.cortex.loading)}</p>}
      </aside>
      <main className="flex min-w-0 flex-1 flex-col">
        {current ? (
          <>
            <div className="flex flex-wrap items-center gap-2 border-b p-2">
              <nav
                className="flex min-w-0 flex-1 flex-wrap gap-1"
                aria-label={t(($) => $.cortex.breadcrumb)}
              >
                {documentPath(documents, current.id).map((doc) => (
                  <AppLink
                    key={doc.id}
                    href={paths.documentDetail(doc.id)}
                    className="max-w-48 truncate text-caption"
                  >
                    / {doc.title}
                  </AppLink>
                ))}
              </nav>
              <span className="text-caption">{current.status}</span>
              <Button
                size="sm"
                variant="outline"
                disabled={
                  command.isPending ||
                  hasDraft ||
                  current.status === "reviewing"
                }
                onClick={() =>
                  command.mutate(() =>
                    api.transitionDocument(
                      current.id,
                      "review",
                      current.document_revision ?? 1,
                    ),
                  )
                }
              >
                {t(($) => $.cortex.review)}
              </Button>
              {(role === "owner" || role === "admin") && (
                <Button
                  size="sm"
                  disabled={
                    command.isPending ||
                    hasDraft ||
                    current.status !== "reviewing"
                  }
                  onClick={() =>
                    command.mutate(() =>
                      api.transitionDocument(
                        current.id,
                        "publish",
                        current.document_revision ?? 1,
                      ),
                    )
                  }
                >
                  {t(($) => $.cortex.publish)}
                </Button>
              )}
            </div>
            <IssueDetail
              key={current.id}
              issueId={current.id}
              defaultSidebarOpen={false}
              onDelete={() => navigation.push(paths.documents())}
            />
          </>
        ) : (
          <div className="m-auto text-muted-foreground">
            {t(($) => $.cortex.select_document)}
          </div>
        )}
      </main>
    </div>
  );
}
