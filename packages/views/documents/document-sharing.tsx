"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { collectionKeys } from "@multica/core/collections";
import { documentKeys, type DocumentAccess } from "@multica/core/documents";
import { projectListOptions } from "@multica/core/projects/queries";
import { memberListOptions } from "@multica/core/workspace/queries";
import { issueKeys } from "@multica/core/issues/queries";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogTrigger,
} from "@multica/ui/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import { useT } from "../i18n";

type Role = "view" | "edit";
function RolePicker({
  value,
  onChange,
  label,
}: {
  value: Role;
  onChange: (value: Role) => void;
  label: string;
}) {
  const { t } = useT("issues");
  return (
    <Select
      items={{
        view: t(($) => $.cortex_docs.can_read),
        edit: t(($) => $.cortex_docs.can_edit),
      }}
      value={value}
      onValueChange={(value) => {
        if (value === "view" || value === "edit") onChange(value);
      }}
    >
      <SelectTrigger aria-label={label} className="w-32">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="view">{t(($) => $.cortex_docs.can_read)}</SelectItem>
        <SelectItem value="edit">{t(($) => $.cortex_docs.can_edit)}</SelectItem>
      </SelectContent>
    </Select>
  );
}
export function ResourceSharing({
  kind,
  id,
  wsId,
  access,
  disabled,
}: {
  id: string;
  wsId: string;
  access: DocumentAccess;
  disabled?: boolean;
  kind: "document" | "collection";
}) {
  const { t } = useT("issues");
  const [open, setOpen] = useState(false);
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button size="sm" disabled={disabled} />}>
        {t(($) =>
          kind === "document" ? $.cortex_docs.publish : $.cortex_docs.share,
        )}
      </DialogTrigger>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {t(($) =>
              kind === "document"
                ? $.cortex_docs.share_document
                : $.cortex_table.share_table,
            )}
          </DialogTitle>
        </DialogHeader>
        {open && (
          <SharingForm
            kind={kind}
            id={id}
            wsId={wsId}
            access={access}
            onSaved={() => setOpen(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
function SharingForm({
  kind,
  id,
  wsId,
  access,
  onSaved,
}: {
  id: string;
  wsId: string;
  access: DocumentAccess;
  onSaved: () => void;
  kind: "document" | "collection";
}) {
  const { t } = useT("issues");
  const qc = useQueryClient();
  const [sharingRevision] = useState(access.revision);
  const [scope, setScope] = useState(access.scope);
  const [role, setRole] = useState(access.scope_role);
  const [projectId, setProjectId] = useState(access.project_id ?? "");
  const [people, setPeople] = useState(access.collaborators);
  const [search, setSearch] = useState("");
  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const { data: projects = [] } = useQuery(projectListOptions(wsId));
  const save = useMutation({
    mutationFn: () =>
      (kind === "document"
        ? api.updateDocumentAccess.bind(api)
        : api.updateCollectionAccess.bind(api))(
        id,
        {
          scope,
          scope_role: role,
          project_id: scope === "project" ? projectId : null,
          collaborators: people,
          expected_revision: sharingRevision,
        },
        wsId,
      ),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({
          queryKey:
            kind === "document"
              ? documentKeys.all(wsId)
              : collectionKeys.all(wsId),
        }),
        ...(kind === "document"
          ? [qc.invalidateQueries({ queryKey: issueKeys.detail(wsId, id) })]
          : []),
      ]);
      onSaved();
    },
  });
  const candidates = members.filter(
    (m) =>
      m.user_id !== access.owner_id &&
      !people.some((p) => p.user_id === m.user_id) &&
      (m.name + " " + m.email)
        .toLocaleLowerCase()
        .includes(search.toLocaleLowerCase()),
  );
  return (
    <div className="space-y-5">
      <section className="space-y-2">
        <h3 className="text-label font-medium">
          {t(($) => $.cortex_docs.collaborators)}
        </h3>
        <div className="flex items-center justify-between gap-2 text-body">
          <span className="truncate">
            {members.find((m) => m.user_id === access.owner_id)?.name ??
              access.owner_id}
          </span>
          <span className="text-caption text-muted-foreground">
            {t(($) => $.cortex_docs.document_owner)}
          </span>
        </div>
        {people.map((person) => (
          <div key={person.user_id} className="flex items-center gap-2">
            <span className="min-w-0 flex-1 truncate text-body">
              {members.find((m) => m.user_id === person.user_id)?.name ??
                person.user_id}
            </span>
            <RolePicker
              value={person.role}
              label={t(($) => $.cortex_docs.permission)}
              onChange={(role) =>
                setPeople(
                  people.map((p) =>
                    p.user_id === person.user_id ? { ...p, role } : p,
                  ),
                )
              }
            />
            <Button
              variant="ghost"
              size="sm"
              onClick={() =>
                setPeople(people.filter((p) => p.user_id !== person.user_id))
              }
            >
              {t(($) => $.cortex_docs.remove_collaborator)}
            </Button>
          </div>
        ))}
        <Input
          aria-label={t(($) => $.cortex_docs.find_collaborator)}
          placeholder={t(($) => $.cortex_docs.find_collaborator)}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="max-h-40 overflow-y-auto rounded-md border">
          {candidates.length === 0 ? (
            <p className="p-2 text-caption text-muted-foreground">
              {t(($) => $.cortex_docs.no_members)}
            </p>
          ) : (
            candidates.map((m) => (
              <button
                type="button"
                key={m.user_id}
                onClick={() => {
                  setPeople([...people, { user_id: m.user_id, role: "view" }]);
                  setSearch("");
                }}
                className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-body hover:bg-accent focus-visible:bg-accent"
              >
                <span className="truncate">{m.name}</span>
                <span className="truncate text-caption text-muted-foreground">
                  {m.email}
                </span>
              </button>
            ))
          )}
        </div>
      </section>
      <section className="space-y-3">
        <h3 className="text-label font-medium">
          {t(($) => $.cortex_docs.shared_scope)}
        </h3>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            items={{
              private: t(($) => $.cortex_docs.only_collaborators),
              project: t(($) => $.cortex_docs.project_share),
              workspace: t(($) => $.cortex_docs.workspace_share),
            }}
            value={scope}
            onValueChange={(value) => {
              if (
                value === "private" ||
                value === "project" ||
                value === "workspace"
              )
                setScope(value);
            }}
          >
            <SelectTrigger
              className="min-w-48 flex-1"
              aria-label={t(($) => $.cortex_docs.shared_scope)}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="private">
                {t(($) => $.cortex_docs.only_collaborators)}
              </SelectItem>
              <SelectItem value="project">
                {t(($) => $.cortex_docs.project_share)}
              </SelectItem>
              <SelectItem value="workspace">
                {t(($) => $.cortex_docs.workspace_share)}
              </SelectItem>
            </SelectContent>
          </Select>
          {scope !== "private" && (
            <RolePicker
              value={role}
              onChange={setRole}
              label={t(($) => $.cortex_docs.permission)}
            />
          )}
        </div>
        {scope === "project" && (
          <>
            <Select
              items={projects.map((p) => ({ value: p.id, label: p.title }))}
              value={projectId}
              onValueChange={(value) => setProjectId(value ?? "")}
            >
              <SelectTrigger
                aria-label={t(($) => $.cortex_docs.select_project)}
                className="w-full"
              >
                <SelectValue
                  placeholder={t(($) => $.cortex_docs.select_project)}
                />
              </SelectTrigger>
              <SelectContent>
                {projects.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.title}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-caption text-muted-foreground">
              {t(($) => $.cortex_docs.project_audience)}
            </p>
          </>
        )}
        {scope === "private" && people.length === 0 && (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_docs.owner_only)}
          </p>
        )}
        {scope !== "private" && (
          <p className="text-caption text-muted-foreground">
            {t(($) => $.cortex_docs.highest_permission)}
          </p>
        )}
      </section>
      {save.error && (
        <p role="alert" className="text-caption text-destructive">
          {save.error.message}
        </p>
      )}
      <DialogFooter>
        <Button variant="outline" disabled={save.isPending} onClick={onSaved}>
          {t(($) => $.cortex_docs.cancel_sharing)}
        </Button>
        <Button
          disabled={save.isPending || (scope === "project" && !projectId)}
          onClick={() => save.mutate()}
        >
          {t(($) =>
            save.isPending ? $.cortex.saving : $.cortex_docs.save_sharing,
          )}
        </Button>
      </DialogFooter>
    </div>
  );
}

export function DocumentSharing(
  props: Omit<Parameters<typeof ResourceSharing>[0], "kind">,
) {
  return <ResourceSharing {...props} kind="document" />;
}
