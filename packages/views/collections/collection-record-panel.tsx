"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRight,
  Link2,
  MessageSquareOff,
  MoreHorizontal,
  Trash2,
  X,
} from "lucide-react";
import { toast } from "sonner";
import {
  collectionRecordOptions,
  recordBacklinksOptions,
  type CollectionField,
  type CollectionRecord,
} from "@multica/core/collections";
import { useCreateIssue } from "@multica/core/issues/mutations";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { useT, useTimeAgo } from "../i18n";
import { AppLink, useNavigation } from "../navigation";
import { CollectionFieldEditor } from "./collection-cell";
import {
  FieldTypeIcon,
  RELATION_TYPE,
  isRelation,
  recordLinks,
  relatesToTasks,
} from "./collection-fields";
import { RelationEditor, RelationList } from "./collection-relation";
import type { CollectionCommands } from "./use-collection-commands";

/**
 * Side panel for one record (S6). A record is plain data: no status, no
 * comments and no agent assignment, so the panel explains where discussion
 * belongs — and offers the way there: converting the record to a task that
 * stays linked to it.
 */
export function CollectionRecordPanel({
  wsId,
  collectionId,
  projectId,
  recordId,
  fields,
  canManage,
  commands,
  onClose,
}: {
  wsId: string;
  collectionId: string;
  /** The table's project; a task made from a record starts there too. */
  projectId: string | null;
  recordId: string;
  fields: CollectionField[];
  /** Managers get the task relation field created for them when it is missing. */
  canManage: boolean;
  commands: CollectionCommands;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const timeAgo = useTimeAgo();
  const { data: record, isLoading } = useQuery(
    collectionRecordOptions(wsId, collectionId, recordId),
  );
  const [title, setTitle] = useState(record?.title ?? "");
  const [editing, setEditing] = useState<string | null>(null);
  const [converting, setConverting] = useState(false);
  useEffect(() => {
    setTitle(record?.title ?? "");
  }, [record?.title]);
  const commitTitle = () => {
    if (record && title.trim() !== record.title)
      void commands.setTitle(record, title.trim());
  };
  const valueFields = fields.filter((field) => !isRelation(field));
  const relationFields = fields.filter(isRelation);

  return (
    <aside
      aria-label={t(($) => $.cortex_table.record_details)}
      className="flex w-[420px] min-w-0 shrink-0 flex-col border-l bg-background"
    >
      <header className="flex items-center gap-2 border-b px-4 py-2.5">
        <span className="rounded-sm bg-muted px-1.5 py-px font-mono text-caption text-muted-foreground">
          {t(($) => $.cortex_table.record_badge)}
        </span>
        <input
          aria-label={t(($) => $.cortex_table.record_title)}
          className="min-w-0 flex-1 bg-transparent text-body font-semibold outline-none"
          value={title}
          disabled={!record}
          onChange={(event) => setTitle(event.target.value)}
          onBlur={commitTitle}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.currentTarget.blur();
            if (event.key === "Escape") {
              setTitle(record?.title ?? "");
              event.currentTarget.blur();
            }
          }}
        />
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={t(($) => $.cortex_table.record_actions)}
              />
            }
          >
            <MoreHorizontal />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-44">
            <DropdownMenuItem
              variant="destructive"
              disabled={!record || commands.deleteRecord.isPending}
              onClick={() =>
                record &&
                commands.deleteRecord.mutate(record.id, { onSuccess: onClose })
              }
            >
              <Trash2 />
              {t(($) => $.cortex_table.move_to_trash)}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t(($) => $.cortex_table.close)}
          onClick={onClose}
        >
          <X />
        </Button>
      </header>
      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4">
        {isLoading && (
          <p role="status" className="text-caption text-muted-foreground">
            {t(($) => $.cortex.loading)}
          </p>
        )}
        {!isLoading && !record && (
          <p role="alert" className="text-caption text-muted-foreground">
            {t(($) => $.cortex_table.record_missing)}
          </p>
        )}
        {record && (
          <>
            {valueFields.length > 0 && (
              <dl className="grid grid-cols-[8rem_minmax(0,1fr)] items-center gap-x-3 gap-y-1">
                {valueFields.map((field) => (
                  <div key={field.id} className="contents">
                    <dt className="flex min-w-0 items-center gap-2 py-1.5 text-label text-muted-foreground">
                      <FieldTypeIcon type={field.type} />
                      <span className="truncate">{field.name}</span>
                    </dt>
                    <dd className="min-w-0 rounded-sm">
                      <CollectionFieldEditor
                        record={record}
                        field={field}
                        open={editing === field.id}
                        onOpenChange={(open) => setEditing(open ? field.id : null)}
                        onChange={(value) => void commands.setField(record, field.id, value)}
                        className="rounded-sm"
                      />
                    </dd>
                  </div>
                ))}
              </dl>
            )}
            {/* A relation gets room for titles here, which a cell does not have. */}
            {relationFields.map((field) => (
              <RelationSection
                key={field.id}
                record={record}
                field={field}
                open={editing === field.id}
                onOpenChange={(open) => setEditing(open ? field.id : null)}
                commands={commands}
              />
            ))}
            <RecordBacklinks wsId={wsId} collectionId={collectionId} recordId={record.id} />
            <p className="text-caption text-muted-foreground">
              {t(($) => $.cortex_table.created_ago, {
                time: timeAgo(record.created_at),
              })}
            </p>
            <section className="rounded-md border border-l-4 border-l-destructive/70 bg-muted/30 p-3">
              <h3 className="flex items-center gap-2 text-label font-medium">
                <MessageSquareOff className="size-4 text-muted-foreground" />
                {t(($) => $.cortex_table.no_comments_title)}
              </h3>
              <p className="mt-1.5 text-caption leading-5 text-muted-foreground">
                {t(($) => $.cortex_table.no_comments_body)}
              </p>
              <Button
                variant="outline"
                size="sm"
                className="mt-2.5"
                onClick={() => setConverting(true)}
              >
                {t(($) => $.cortex_table.convert_to_task)}
                <ArrowRight />
              </Button>
            </section>
            <ConvertToTaskDialog
              open={converting}
              onOpenChange={setConverting}
              record={record}
              fields={fields}
              projectId={projectId}
              canManage={canManage}
              commands={commands}
            />
          </>
        )}
      </div>
    </aside>
  );
}

function RelationSection({
  record,
  field,
  open,
  onOpenChange,
  commands,
}: {
  record: CollectionRecord;
  field: CollectionField;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  commands: CollectionCommands;
}) {
  const { t } = useT("issues");
  return (
    <section aria-label={field.name} className="space-y-2">
      <h3 className="flex min-w-0 items-center gap-2 text-label text-muted-foreground">
        <FieldTypeIcon type={field.type} />
        <span className="truncate">{field.name}</span>
      </h3>
      <RelationList
        links={recordLinks(record, field)}
        onUnlink={(link) => void commands.unlinkRecord(record.id, field.id, link.id)}
      />
      <RelationEditor
        record={record}
        field={field}
        open={open}
        onOpenChange={onOpenChange}
        onLink={(toId) => commands.linkRecord(record.id, field.id, toId)}
        onUnlink={(linkId) => commands.unlinkRecord(record.id, field.id, linkId)}
        trigger={
          <>
            <Link2 />
            {relatesToTasks(field)
              ? t(($) => $.cortex_table.link_task)
              : t(($) => $.cortex_table.link_record)}
          </>
        }
        triggerRender={<Button variant="outline" size="sm" className="w-full" />}
      />
    </section>
  );
}

/** Records elsewhere whose relation fields point at this one. */
function RecordBacklinks({
  wsId,
  collectionId,
  recordId,
}: {
  wsId: string;
  collectionId: string;
  recordId: string;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const { data: links = [] } = useQuery(
    recordBacklinksOptions(wsId, collectionId, recordId),
  );
  if (!links.length) return null;
  return (
    <section className="space-y-2">
      <h3 className="text-label text-muted-foreground">
        {t(($) => $.cortex_table.referenced_by)}
      </h3>
      <ul className="space-y-1">
        {links.map((link) => (
          <li key={link.id}>
            <AppLink
              href={`${paths.collectionDetail(link.collection_id)}?record=${link.record_id}`}
              className="flex min-w-0 items-center gap-1.5 rounded-sm px-1.5 py-1 text-label hover:bg-accent/50"
            >
              <span className="shrink-0 text-muted-foreground">{link.collection_name}</span>
              <span className="shrink-0 text-muted-foreground/60">›</span>
              <span className="truncate">
                {link.record_title || t(($) => $.cortex_table.untitled)}
              </span>
            </AppLink>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * The exit from a record to work that can be discussed and assigned: a new
 * task, linked from the record through its first relation to tasks. The record
 * stays. A table without such a relation gets one when the person converting
 * may manage its fields; otherwise the dialog says who can add it.
 */
function ConvertToTaskDialog({
  open,
  onOpenChange,
  ...form
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  record: CollectionRecord;
  fields: CollectionField[];
  projectId: string | null;
  canManage: boolean;
  commands: CollectionCommands;
}) {
  const [pending, setPending] = useState(false);
  return (
    <Dialog open={open} onOpenChange={(next) => (next || !pending) && onOpenChange(next)}>
      <DialogContent className="sm:max-w-md">
        {/* Mounted per opening, so the draft always starts from the record. */}
        <ConvertToTaskForm
          {...form}
          pending={pending}
          onPendingChange={setPending}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

function ConvertToTaskForm({
  record,
  fields,
  projectId,
  canManage,
  commands,
  pending,
  onPendingChange,
  onClose,
}: {
  record: CollectionRecord;
  fields: CollectionField[];
  projectId: string | null;
  canManage: boolean;
  commands: CollectionCommands;
  pending: boolean;
  onPendingChange: (pending: boolean) => void;
  onClose: () => void;
}) {
  const { t } = useT("issues");
  const paths = useWorkspacePaths();
  const navigation = useNavigation();
  const createIssue = useCreateIssue();
  const [title, setTitle] = useState(record.title);
  const [error, setError] = useState<string | null>(null);

  const taskField = fields.find(relatesToTasks);
  const newFieldName = t(($) => $.cortex_table.linked_tasks_field);
  const blocked = !taskField && !canManage;
  const canSubmit = title.trim() !== "" && !pending && !blocked;

  const submit = async () => {
    if (!canSubmit) return;
    onPendingChange(true);
    setError(null);
    try {
      // The relation comes first: a task that was created and then could not
      // be linked is the failure nobody would notice.
      const field =
        taskField ??
        (await commands.createField.mutateAsync({
          name: newFieldName,
          type: RELATION_TYPE,
          config: { relation: { to_type: "issue" } },
        }));
      const issue = await createIssue.mutateAsync({
        title: title.trim(),
        ...(projectId ? { project_id: projectId } : {}),
      });
      const linked = await commands.linkRecord(record.id, field.id, issue.id);
      const openTask = {
        label: t(($) => $.cortex_table.open),
        onClick: () => navigation.push(paths.issueDetail(issue.id)),
      };
      if (linked)
        toast.success(
          t(($) => $.cortex_table.converted_to_task, { identifier: issue.identifier }),
          { action: openTask },
        );
      else
        toast.warning(
          t(($) => $.cortex_table.converted_unlinked, { identifier: issue.identifier }),
          { action: openTask },
        );
      onPendingChange(false);
      onClose();
    } catch (cause) {
      onPendingChange(false);
      setError(cause instanceof Error && cause.message ? cause.message : String(cause));
    }
  };

  return (
    <form
      className="contents"
      aria-busy={pending}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <DialogHeader>
        <DialogTitle>{t(($) => $.cortex_table.convert_to_task)}</DialogTitle>
        <DialogDescription>
          {t(($) => $.cortex_table.convert_description)}
        </DialogDescription>
      </DialogHeader>
      <div className="space-y-1.5">
        <Label htmlFor="convert-task-title">
          {t(($) => $.cortex_table.task_title)}
        </Label>
        <Input
          id="convert-task-title"
          autoFocus
          value={title}
          onChange={(event) => setTitle(event.target.value)}
        />
      </div>
      {!taskField && (
        <p className="text-caption text-muted-foreground">
          {blocked
            ? t(($) => $.cortex_table.convert_needs_field)
            : t(($) => $.cortex_table.convert_adds_field, { name: newFieldName })}
        </p>
      )}
      {error && (
        <p role="alert" className="text-caption text-destructive">
          {error}
        </p>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" disabled={pending} onClick={onClose}>
          {t(($) => $.cortex_table.cancel)}
        </Button>
        <Button type="submit" disabled={!canSubmit}>
          {t(($) => $.cortex_table.create_task)}
        </Button>
      </DialogFooter>
    </form>
  );
}
