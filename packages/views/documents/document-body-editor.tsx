"use client";

import { useEffect, useRef, useState, type ComponentProps } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import {
  documentKeys,
  useDocumentPreferences,
  type DocumentDraft,
} from "@multica/core/documents";
import { issueKeys } from "@multica/core/issues/queries";
import type { Issue } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { ContentEditor } from "../editor";
import { useT } from "../i18n";

export function DocumentBodyEditor({
  issue,
  attachmentIds,
  ...editorProps
}: {
  issue: Issue;
  attachmentIds: (markdown: string) => string[];
} & Omit<
  ComponentProps<typeof ContentEditor>,
  "value" | "defaultValue" | "onUpdate" | "ref"
>) {
  const { t } = useT("issues");
  const queryClient = useQueryClient();
  const key = JSON.stringify([issue.workspace_id, issue.id]);
  const draft = useDocumentPreferences((state) => state.drafts[key]);
  const setDraft = useDocumentPreferences((state) => state.setDraft);
  const version = useRef(draft?.version ?? issue.document_revision ?? 1);
  const saving = useRef(false);
  const queued = useRef<DocumentDraft | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [remote, setRemote] = useState<Issue | null>(null);
  const [pending, setPending] = useState(false);
  useEffect(() => {
    if (!draft && !saving.current)
      version.current = issue.document_revision ?? 1;
  }, [draft, issue.document_revision]);
  const accept = (updated: Issue) => {
    queryClient.setQueryData<Issue>(
      issueKeys.detail(issue.workspace_id, issue.id),
      (current) =>
        !current || (current.revision ?? 0) <= (updated.revision ?? 0)
          ? updated
          : current,
    );
    void queryClient.invalidateQueries({
      queryKey: documentKeys.all(issue.workspace_id),
    });
  };
  const save = async (next: DocumentDraft) => {
    setDraft(key, next);
    if (saving.current) {
      queued.current = next;
      return;
    }
    saving.current = true;
    setPending(true);
    try {
      const updated = await api.updateIssue(issue.id, {
        description: next.body,
        description_base: next.base,
        expected_document_revision: next.version,
        attachment_ids: next.attachmentIds,
      });
      version.current = updated.document_revision ?? next.version;
      accept(updated);
      setError(null);
      setRemote(null);
      const after = queued.current;
      queued.current = null;
      saving.current = false;
      if (after) {
        await save({ ...after, version: version.current, base: next.body });
      } else {
        const latest = useDocumentPreferences.getState().drafts[key];
        if (latest?.body === next.body) setDraft(key, null);
        else if (latest?.version === next.version)
          setDraft(key, {
            ...latest,
            version: version.current,
            base: updated.description ?? next.body,
          });
        setPending(false);
      }
    } catch (cause) {
      saving.current = false;
      setPending(false);
      queued.current = null;
      setError(cause instanceof Error ? cause.message : String(cause));
      try {
        setRemote(await api.getIssue(issue.id));
      } catch {
        /* The draft also survives an offline failure. */
      }
    }
  };
  return (
    <div>
      <ContentEditor
        {...editorProps}
        enableSlashCommands
        slashCommandMode="command"
        value={draft?.body ?? issue.description ?? ""}
        onDraftChange={(body, base) => {
          const previous = useDocumentPreferences.getState().drafts[key];
          setDraft(key, {
            body,
            base: previous?.base ?? base,
            version: previous?.version ?? version.current,
            attachmentIds: attachmentIds(body),
          });
        }}
        onUpdate={(body, base) => {
          const next = {
            body,
            base,
            version:
              useDocumentPreferences.getState().drafts[key]?.version ??
              version.current,
            attachmentIds: attachmentIds(body),
          };
          setDraft(key, next);
          if (!error) void save(next);
        }}
      />
      <div className="text-caption text-muted-foreground" role="status">
        {pending
          ? t(($) => $.cortex.saving)
          : draft
            ? t(($) => $.cortex.unsaved)
            : t(($) => $.cortex.saved)}
      </div>
      {draft && !error && !pending && (
        <Button size="sm" variant="outline" onClick={() => void save(draft)}>
          {t(($) => $.cortex.retry_save)}
        </Button>
      )}
      {error && (
        <section
          className="mt-3 space-y-2 rounded-md border border-destructive p-3"
          aria-label={t(($) => $.cortex.conflict)}
        >
          <p role="alert">{error}</p>
          {remote && (
            <details open>
              <summary>
                {t(($) => $.cortex.current_version)} ·{" "}
                {remote.document_revision}
              </summary>
              <pre className="max-h-60 overflow-auto whitespace-pre-wrap">
                {remote.description}
              </pre>
            </details>
          )}
          <label className="block">
            {t(($) => $.cortex.your_draft)}
            <textarea
              className="min-h-40 w-full rounded border bg-background p-2"
              value={draft?.body ?? ""}
              onChange={(event) => {
                if (draft)
                  setDraft(key, { ...draft, body: event.target.value });
              }}
            />
          </label>
          <Button
            disabled={pending || !draft}
            onClick={() => {
              if (draft)
                void save({
                  ...draft,
                  version: remote?.document_revision ?? draft.version,
                  base: remote?.description ?? draft.base,
                });
            }}
          >
            {t(($) => $.cortex.merge_retry)}
          </Button>
        </section>
      )}
    </div>
  );
}
