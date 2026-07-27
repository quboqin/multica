"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Eraser, Loader2, Lock, PlugZap, Save } from "lucide-react";
import type { Agent, IntegrationTokens, WorkspaceMCPConnection } from "@multica/core/types";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  DEFAULT_INTEGRATION_TOKEN_KEYS,
  integrationTokenPlaceholder,
} from "@multica/core/integration-tokens";
import { workspaceMCPConnectionsOptions } from "@multica/core/workspace-mcp";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useT } from "../../../i18n";

const EMPTY_INTEGRATION_TOKENS: IntegrationTokens = {};
const WORKSPACE_MCP_REFS_KEY = "workspaceMcpRefs";
const WORKSPACE_MCP_REFS_SNAKE_KEY = "workspace_mcp_refs";

// `null` and the empty string are the two ways the user can mean "no
// config" — the server stores either as a NULL column and the daemon
// falls back to the runtime CLI default at launch. We normalise to
// the empty string in the editor so the dirty check has one canonical
// form to compare against.
function configToText(value: unknown): string {
  if (value === null || value === undefined) return "";
  return JSON.stringify(value, null, 2);
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

function workspaceMCPRefs(value: unknown): Array<Record<string, unknown>> {
  if (!isRecord(value)) return [];
  const refs = value[WORKSPACE_MCP_REFS_KEY] ?? value[WORKSPACE_MCP_REFS_SNAKE_KEY];
  if (!Array.isArray(refs)) return [];
  return refs.filter(isRecord);
}

function workspaceMCPConnectionId(ref: Record<string, unknown>): string {
  const value = ref.connectionId ?? ref.connection_id;
  return typeof value === "string" ? value : "";
}

function normalizeServerName(value: string): string {
  const out = value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/[-_]+$/g, "")
    .replace(/^[-_]+/g, "")
    .slice(0, 64)
    .replace(/[-_]+$/g, "");
  return out || "workspace-mcp";
}

function defaultWorkspaceMCPServerName(connection: WorkspaceMCPConnection): string {
  return normalizeServerName(`workspace-${connection.name || connection.capability || "mcp"}`);
}

function setWorkspaceMCPRef(
  value: unknown | null,
  connection: WorkspaceMCPConnection | null,
): unknown | null {
  const next: Record<string, unknown> = isRecord(value) ? { ...value } : {};
  delete next[WORKSPACE_MCP_REFS_KEY];
  delete next[WORKSPACE_MCP_REFS_SNAKE_KEY];
  if (connection) {
    next[WORKSPACE_MCP_REFS_KEY] = [{
      connectionId: connection.id,
      serverName: defaultWorkspaceMCPServerName(connection),
    }];
  }
  return Object.keys(next).length > 0 ? next : null;
}

export function McpConfigTab({
  agent,
  onSave,
  onDirtyChange,
}: {
  agent: Agent;
  onSave: (updates: { mcp_config: unknown | null }) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const integrationTokens = useAuthStore(
    (s) => s.user?.integration_tokens ?? EMPTY_INTEGRATION_TOKENS,
  );

  const redacted = agent.mcp_config_redacted === true;
  const original = useMemo(() => configToText(agent.mcp_config), [agent.mcp_config]);
  const [text, setText] = useState(original);
  const [saving, setSaving] = useState(false);
  const workspaceConnectionsQuery = useQuery({
    ...workspaceMCPConnectionsOptions(wsId),
    enabled: !!wsId && !redacted,
  });
  const activeWorkspaceConnections = useMemo(
    () => (workspaceConnectionsQuery.data?.connections ?? [])
      .filter((connection) => connection.status === "active"),
    [workspaceConnectionsQuery.data?.connections],
  );
  const profileCredentialPlaceholders = useMemo(() => {
    const keys = new Set(DEFAULT_INTEGRATION_TOKEN_KEYS);
    for (const [key, value] of Object.entries(integrationTokens)) {
      if (key.trim() !== "" && (value ?? "").trim() !== "") {
        keys.add(key);
      }
    }
    return Array.from(keys)
      .map((key) => ({ key, placeholder: integrationTokenPlaceholder(key) }))
      .filter((item) => item.placeholder !== "")
      .sort((a, b) => a.key.localeCompare(b.key));
  }, [integrationTokens]);

  // Sync local draft when the agent prop changes (e.g. after a successful
  // save invalidates the cache and a fresh agent arrives). We only sync
  // when the user has no in-flight edits — comparing the current draft
  // against the *previous* original (not the new one) is what tells us
  // "they haven't touched this since the last sync". Comparing against
  // the new original would skip the sync whenever the server-side value
  // changes underneath an untouched draft, leaving the editor showing a
  // stale value that a later Save would write back, clobbering another
  // admin's edit.
  const previousOriginalRef = useRef(original);
  useEffect(() => {
    setText((current) =>
      current === previousOriginalRef.current ? original : current,
    );
    previousOriginalRef.current = original;
  }, [original]);

  const trimmed = text.trim();
  const parseResult = useMemo<
    | { ok: true; value: unknown | null }
    | { ok: false; error: string }
  >(() => {
    if (trimmed === "") return { ok: true, value: null };
    try {
      const value = JSON.parse(trimmed);
      // The MCP CLI accepts an object (`{"mcpServers": …}`); a top-level
      // array or primitive is almost certainly a user mistake, so reject
      // here rather than surprise them with a server-side error later.
      if (value === null || typeof value !== "object" || Array.isArray(value)) {
        return {
          ok: false,
          error: "mcp_config_not_object",
        };
      }
      return { ok: true, value };
    } catch (err) {
      return {
        ok: false,
        error: err instanceof Error ? err.message : "invalid JSON",
      };
    }
  }, [trimmed]);

  const dirty = text !== original;
  const selectedWorkspaceConnectionId = parseResult.ok
    ? workspaceMCPConnectionId(workspaceMCPRefs(parseResult.value)[0] ?? {})
    : "";
  const selectedWorkspaceConnection = activeWorkspaceConnections.find(
    (connection) => connection.id === selectedWorkspaceConnectionId,
  ) ?? null;

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  if (redacted) {
    return (
      <div className="space-y-3">
        <p className="flex items-center gap-2 text-sm font-medium">
          <Lock className="h-3.5 w-3.5 text-muted-foreground" />
          {t(($) => $.tab_body.mcp_config.redacted_title)}
        </p>
        <p className="text-xs text-muted-foreground">
          {t(($) => $.tab_body.mcp_config.redacted_hint)}
        </p>
      </div>
    );
  }

  const handleSave = async () => {
    if (!parseResult.ok) return;
    setSaving(true);
    try {
      await onSave({ mcp_config: parseResult.value });
      // Normalise the editor to the pretty-printed canonical form so the
      // dirty check stops firing after a successful save (the user's
      // raw input may differ from what configToText would emit).
      setText(configToText(parseResult.value));
      toast.success(t(($) => $.tab_body.mcp_config.saved_toast));
    } catch (err) {
      toast.error(
        err instanceof Error && err.message
          ? err.message
          : t(($) => $.tab_body.mcp_config.save_failed_toast),
      );
    } finally {
      setSaving(false);
    }
  };

  const handleClear = () => {
    setText("");
  };

  const handleWorkspaceConnectionChange = (connectionId: string) => {
    if (!parseResult.ok) return;
    const connection = activeWorkspaceConnections.find((item) => item.id === connectionId) ?? null;
    setText(configToText(setWorkspaceMCPRef(parseResult.value, connection)));
  };

  const showInvalid = trimmed !== "" && !parseResult.ok;
  const invalidMessage = !parseResult.ok && parseResult.error === "mcp_config_not_object"
    ? t(($) => $.tab_body.mcp_config.invalid_not_object)
    : !parseResult.ok
      ? t(($) => $.tab_body.mcp_config.invalid_json, { error: parseResult.error })
      : "";

  return (
    <div className="flex h-full flex-col space-y-3">
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-2 text-xs text-muted-foreground">
          <p>{t(($) => $.tab_body.mcp_config.intro)}</p>
          <p>{t(($) => $.tab_body.mcp_config.credential_hint)}</p>
          {profileCredentialPlaceholders.length > 0 ? (
            <div className="flex flex-wrap gap-1.5">
              {profileCredentialPlaceholders.map(({ key, placeholder }) => (
                <code
                  key={key}
                  className="rounded bg-muted px-1.5 py-0.5 font-mono text-[10px] text-foreground"
                >
                  {placeholder}
                </code>
              ))}
            </div>
          ) : (
            <p>{t(($) => $.tab_body.mcp_config.no_profile_credentials)}</p>
          )}
        </div>
        {trimmed !== "" && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleClear}
            className="shrink-0"
          >
            <Eraser className="h-3 w-3" />
            {t(($) => $.tab_body.mcp_config.clear_action)}
          </Button>
        )}
      </div>

      <div className="rounded-md border bg-muted/20 p-3">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0 space-y-1">
            <p className="flex items-center gap-2 text-xs font-medium text-foreground">
              <PlugZap className="h-3.5 w-3.5 text-muted-foreground" />
              {t(($) => $.tab_body.mcp_config.workspace_refs_title)}
            </p>
            <p className="text-xs text-muted-foreground">
              {t(($) => $.tab_body.mcp_config.workspace_refs_hint)}
            </p>
          </div>
          <label className="flex min-w-[220px] flex-col gap-1 text-xs text-muted-foreground">
            <span>{t(($) => $.tab_body.mcp_config.workspace_select_label)}</span>
            <select
              value={selectedWorkspaceConnectionId}
              onChange={(event) => handleWorkspaceConnectionChange(event.target.value)}
              disabled={!parseResult.ok || workspaceConnectionsQuery.isLoading}
              className="h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground outline-none focus:border-ring focus:ring-2 focus:ring-ring/30 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <option value="">
                {workspaceConnectionsQuery.isLoading
                  ? t(($) => $.tab_body.mcp_config.workspace_loading)
                  : t(($) => $.tab_body.mcp_config.workspace_none)}
              </option>
              {activeWorkspaceConnections.map((connection) => (
                <option key={connection.id} value={connection.id}>
                  {connection.name}
                </option>
              ))}
            </select>
          </label>
        </div>
        {!workspaceConnectionsQuery.isLoading && activeWorkspaceConnections.length === 0 && (
          <p className="mt-2 text-xs text-muted-foreground">
            {t(($) => $.tab_body.mcp_config.workspace_empty)}
          </p>
        )}
        {!parseResult.ok && (
          <p className="mt-2 text-xs text-destructive">
            {t(($) => $.tab_body.mcp_config.workspace_invalid_json_hint)}
          </p>
        )}
        {selectedWorkspaceConnection && (
          <p className="mt-2 truncate text-xs text-muted-foreground">
            {t(($) => $.tab_body.mcp_config.workspace_selected_summary, {
              serverName: defaultWorkspaceMCPServerName(selectedWorkspaceConnection),
              capability: selectedWorkspaceConnection.capability,
            })}
          </p>
        )}
      </div>

      <Textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder={t(($) => $.tab_body.mcp_config.placeholder)}
        aria-invalid={showInvalid || undefined}
        aria-label={t(($) => $.tab_body.mcp_config.editor_aria)}
        spellCheck={false}
        className="min-h-[240px] flex-1 font-mono text-xs"
      />

      {showInvalid && (
        <p className="text-xs text-destructive">{invalidMessage}</p>
      )}

      <div className="flex items-center justify-end gap-3">
        {dirty && (
          <span className="text-xs text-muted-foreground">
            {t(($) => $.tab_body.common.unsaved_changes)}
          </span>
        )}
        <Button
          onClick={handleSave}
          disabled={!dirty || !parseResult.ok || saving}
          size="sm"
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t(($) => $.tab_body.common.save)}
        </Button>
      </div>
    </div>
  );
}
