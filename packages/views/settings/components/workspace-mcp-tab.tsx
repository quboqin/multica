"use client";

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, CircleAlert, Pencil, PlugZap, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentMember } from "@multica/core/permissions";
import type {
  SaveWorkspaceMCPConnectionRequest,
  WorkspaceMCPConnection,
} from "@multica/core/types";
import {
  workspaceMCPConnectionsOptions,
  workspaceMCPKeys,
} from "@multica/core/workspace-mcp";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { cn } from "@multica/ui/lib/utils";
import { useT } from "../../i18n";

interface ConnectionDraft {
  name: string;
  serverURL: string;
  createTool: string;
  getTool: string;
  secretHeaders: string;
  clearSecretHeaders: boolean;
  active: boolean;
  isDefault: boolean;
}

const emptyDraft: ConnectionDraft = {
  name: "",
  serverURL: "",
  createTool: "create_creative_job",
  getTool: "get_creative_job",
  secretHeaders: "",
  clearSecretHeaders: false,
  active: true,
  isDefault: true,
};

export function WorkspaceMCPTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const { role, isLoading: memberLoading } = useCurrentMember(wsId);
  const canManage = role === "owner" || role === "admin";
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editing, setEditing] = useState<WorkspaceMCPConnection | null>(null);
  const [draft, setDraft] = useState<ConnectionDraft>(emptyDraft);

  const connectionsQuery = useQuery({
    ...workspaceMCPConnectionsOptions(wsId),
    enabled: !!wsId && canManage,
  });
  const connections = connectionsQuery.data?.connections ?? [];

  const saveConnection = useMutation({
    mutationFn: ({ connectionId, data }: {
      connectionId?: string;
      data: SaveWorkspaceMCPConnectionRequest;
    }) => connectionId
      ? api.updateWorkspaceMCPConnection(connectionId, data)
      : api.createWorkspaceMCPConnection(data),
    onSuccess: async () => {
      setDialogOpen(false);
      toast.success(t(($) => $.workspace_mcp.toast_saved));
      await qc.invalidateQueries({ queryKey: workspaceMCPKeys.connections(wsId) });
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : t(($) => $.workspace_mcp.toast_save_failed));
    },
  });

  const verifyConnection = useMutation({
    mutationFn: (connectionId: string) => api.verifyWorkspaceMCPConnection(connectionId),
    onSuccess: async (result) => {
      toast.success(t(($) => $.workspace_mcp.toast_verified, { count: result.tools.length }));
      await qc.invalidateQueries({ queryKey: workspaceMCPKeys.connections(wsId) });
    },
    onError: async (error) => {
      toast.error(error instanceof Error ? error.message : t(($) => $.workspace_mcp.toast_verify_failed));
      await qc.invalidateQueries({ queryKey: workspaceMCPKeys.connections(wsId) });
    },
  });

  function openCreate() {
    setEditing(null);
    setDraft(emptyDraft);
    setDialogOpen(true);
  }

  function openEdit(connection: WorkspaceMCPConnection) {
    setEditing(connection);
    setDraft({
      name: connection.name,
      serverURL: connection.server_url,
      createTool: connection.tool_create,
      getTool: connection.tool_get,
      secretHeaders: "",
      clearSecretHeaders: false,
      active: connection.status === "active",
      isDefault: connection.is_default,
    });
    setDialogOpen(true);
  }

  function submit() {
    let secretHeaders: Record<string, string> | undefined;
    if (draft.secretHeaders.trim()) {
      try {
        const parsed = JSON.parse(draft.secretHeaders) as unknown;
        if (!isStringRecord(parsed)) throw new Error("invalid secret headers");
        secretHeaders = parsed;
      } catch {
        toast.error(t(($) => $.workspace_mcp.secret_headers_invalid));
        return;
      }
    }
    saveConnection.mutate({
      connectionId: editing?.id,
      data: {
        name: draft.name.trim(),
        server_url: draft.serverURL.trim(),
        transport: "streamable_http",
        tool_create: draft.createTool.trim(),
        tool_get: draft.getTool.trim(),
        status: draft.active ? "active" : "disabled",
        is_default: draft.active && draft.isDefault,
        ...(secretHeaders ? { secret_headers: secretHeaders } : {}),
        ...(draft.clearSecretHeaders ? { clear_secret_headers: true } : {}),
      },
    });
  }

  return (
    <>
      <Card>
        <CardContent className="space-y-4">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div className="flex min-w-0 flex-1 items-start gap-3">
              <div className="rounded-md border bg-muted/50 p-2 text-muted-foreground">
                <PlugZap className="h-4 w-4" />
              </div>
              <div className="space-y-1">
                <h3 className="text-sm font-semibold">{t(($) => $.workspace_mcp.title)}</h3>
                <p className="max-w-3xl text-sm text-muted-foreground">
                  {t(($) => $.workspace_mcp.description)}
                </p>
              </div>
            </div>
            {canManage && (
              <Button size="sm" onClick={openCreate}>
                <Plus />
                {t(($) => $.workspace_mcp.add)}
              </Button>
            )}
          </div>

          {!memberLoading && !canManage && (
            <p className="border-t pt-4 text-sm text-muted-foreground">
              {t(($) => $.workspace_mcp.manage_hint)}
            </p>
          )}
          {canManage && connectionsQuery.isLoading && (
            <p className="border-t pt-4 text-sm text-muted-foreground">
              {t(($) => $.workspace_mcp.loading)}
            </p>
          )}
          {canManage && !connectionsQuery.isLoading && connections.length === 0 && (
            <p className="border-t pt-4 text-sm text-muted-foreground">
              {t(($) => $.workspace_mcp.empty)}
            </p>
          )}

          {canManage && connections.length > 0 && (
            <div className="divide-y border-t">
              {connections.map((connection) => (
                <div
                  key={connection.id}
                  className="flex flex-col gap-3 py-4 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="min-w-0 space-y-1.5">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-medium">{connection.name}</span>
                      <Badge variant={connection.status === "active" ? "secondary" : "outline"}>
                        {connection.status === "active"
                          ? t(($) => $.workspace_mcp.active)
                          : t(($) => $.workspace_mcp.disabled)}
                      </Badge>
                      {connection.is_default && (
                        <Badge variant="outline">{t(($) => $.workspace_mcp.default_badge)}</Badge>
                      )}
                      {connection.last_verified_at && !connection.last_error && (
                        <span className="inline-flex items-center gap-1 text-xs text-emerald-700 dark:text-emerald-400">
                          <CheckCircle2 className="h-3.5 w-3.5" />
                          {t(($) => $.workspace_mcp.verified)}
                        </span>
                      )}
                      {connection.last_error && (
                        <span className="inline-flex items-center gap-1 text-xs text-destructive">
                          <CircleAlert className="h-3.5 w-3.5" />
                          {t(($) => $.workspace_mcp.action_needed)}
                        </span>
                      )}
                    </div>
                    <p className="truncate text-xs text-muted-foreground" title={connection.server_url}>
                      {connection.server_url}
                    </p>
                    {connection.has_secret_headers && (
                      <p className="text-xs text-muted-foreground">
                        {t(($) => $.workspace_mcp.secret_headers_saved, {
                          names: connection.secret_header_names.join(", "),
                        })}
                      </p>
                    )}
                    {connection.last_error && (
                      <p className="line-clamp-2 text-xs text-destructive" title={connection.last_error}>
                        {connection.last_error}
                      </p>
                    )}
                  </div>
                  <div className="flex shrink-0 items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => verifyConnection.mutate(connection.id)}
                      disabled={verifyConnection.isPending || connection.status !== "active"}
                    >
                      <RefreshCw className={cn(
                        verifyConnection.isPending
                          && verifyConnection.variables === connection.id
                          && "animate-spin",
                      )} />
                      {t(($) => $.workspace_mcp.verify)}
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label={t(($) => $.workspace_mcp.edit)}
                      title={t(($) => $.workspace_mcp.edit)}
                      onClick={() => openEdit(connection)}
                    >
                      <Pencil />
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-xl">
          <DialogHeader>
            <DialogTitle>
              {editing
                ? t(($) => $.workspace_mcp.edit_title)
                : t(($) => $.workspace_mcp.create_title)}
            </DialogTitle>
            <DialogDescription>{t(($) => $.workspace_mcp.form_description)}</DialogDescription>
          </DialogHeader>

          <div className="grid gap-4 py-1">
            <div className="grid gap-2">
              <Label htmlFor="workspace-mcp-name">{t(($) => $.workspace_mcp.name)}</Label>
              <Input
                id="workspace-mcp-name"
                value={draft.name}
                onChange={(event) => setDraft((value) => ({ ...value, name: event.target.value }))}
                placeholder={t(($) => $.workspace_mcp.name_placeholder)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workspace-mcp-url">{t(($) => $.workspace_mcp.server_url)}</Label>
              <Input
                id="workspace-mcp-url"
                value={draft.serverURL}
                onChange={(event) => setDraft((value) => ({ ...value, serverURL: event.target.value }))}
                placeholder="https://creative.example.com/mcp"
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="grid gap-2">
                <Label htmlFor="workspace-mcp-create-tool">{t(($) => $.workspace_mcp.create_tool)}</Label>
                <Input
                  id="workspace-mcp-create-tool"
                  value={draft.createTool}
                  onChange={(event) => setDraft((value) => ({ ...value, createTool: event.target.value }))}
                />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="workspace-mcp-get-tool">{t(($) => $.workspace_mcp.get_tool)}</Label>
                <Input
                  id="workspace-mcp-get-tool"
                  value={draft.getTool}
                  onChange={(event) => setDraft((value) => ({ ...value, getTool: event.target.value }))}
                />
              </div>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="workspace-mcp-secret-headers">{t(($) => $.workspace_mcp.secret_headers)}</Label>
              <Textarea
                id="workspace-mcp-secret-headers"
                value={draft.secretHeaders}
                onChange={(event) => setDraft((value) => ({
                  ...value,
                  secretHeaders: event.target.value,
                  clearSecretHeaders: false,
                }))}
                placeholder={'{"Authorization":"Bearer ..."}'}
                rows={3}
              />
              <p className="text-xs text-muted-foreground">
                {editing?.has_secret_headers
                  ? t(($) => $.workspace_mcp.secret_headers_keep)
                  : t(($) => $.workspace_mcp.secret_headers_hint)}
              </p>
              {editing?.has_secret_headers && (
                <label className="flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
                  <span>{t(($) => $.workspace_mcp.clear_secret_headers)}</span>
                  <Switch
                    size="sm"
                    checked={draft.clearSecretHeaders}
                    onCheckedChange={(checked) => setDraft((value) => ({
                      ...value,
                      clearSecretHeaders: checked,
                      secretHeaders: checked ? "" : value.secretHeaders,
                    }))}
                  />
                </label>
              )}
            </div>
            <div className="grid gap-2 sm:grid-cols-2">
              <label className="flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
                <span>{t(($) => $.workspace_mcp.active)}</span>
                <Switch
                  checked={draft.active}
                  onCheckedChange={(checked) => setDraft((value) => ({
                    ...value,
                    active: checked,
                    isDefault: checked ? value.isDefault : false,
                  }))}
                />
              </label>
              <label className="flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm">
                <span>{t(($) => $.workspace_mcp.use_as_default)}</span>
                <Switch
                  checked={draft.isDefault}
                  disabled={!draft.active}
                  onCheckedChange={(checked) => setDraft((value) => ({ ...value, isDefault: checked }))}
                />
              </label>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setDialogOpen(false)}>
              {t(($) => $.workspace_mcp.cancel)}
            </Button>
            <Button
              onClick={submit}
              disabled={saveConnection.isPending || !draft.name.trim() || !draft.serverURL.trim()}
            >
              {saveConnection.isPending
                ? t(($) => $.workspace_mcp.saving)
                : t(($) => $.workspace_mcp.save)}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function isStringRecord(value: unknown): value is Record<string, string> {
  return !!value
    && typeof value === "object"
    && !Array.isArray(value)
    && Object.values(value).every((item) => typeof item === "string");
}
