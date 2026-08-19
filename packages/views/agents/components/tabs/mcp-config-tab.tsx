"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { Eraser, Loader2, Lock, Save } from "lucide-react";
import type { Agent } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { toast } from "sonner";
import { useT } from "../../../i18n";

function configToText(value: unknown): string {
  return value == null ? "" : JSON.stringify(value, null, 2);
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
  const original = useMemo(() => configToText(agent.mcp_config), [agent.mcp_config]);
  const [text, setText] = useState(original);
  const [saving, setSaving] = useState(false);
  const previousOriginal = useRef(original);

  useEffect(() => {
    setText((current) => current === previousOriginal.current ? original : current);
    previousOriginal.current = original;
  }, [original]);

  const parsed = useMemo(() => {
    const trimmed = text.trim();
    if (trimmed === "") return { value: null as unknown | null, error: "" };
    try {
      const value: unknown = JSON.parse(trimmed);
      if (value == null || typeof value !== "object" || Array.isArray(value)) {
        return { value: null, error: "mcp_config_not_object" };
      }
      return { value, error: "" };
    } catch (error) {
      return { value: null, error: error instanceof Error ? error.message : "invalid JSON" };
    }
  }, [text]);

  const dirty = text !== original;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  if (agent.mcp_config_redacted === true) {
    return <div className="space-y-3"><p className="flex items-center gap-2 text-sm font-medium"><Lock className="h-3.5 w-3.5 text-muted-foreground" />{t(($) => $.tab_body.mcp_config.redacted_title)}</p><p className="text-xs text-muted-foreground">{t(($) => $.tab_body.mcp_config.redacted_hint)}</p></div>;
  }

  const save = async () => {
    if (parsed.error !== "") return;
    setSaving(true);
    try {
      await onSave({ mcp_config: parsed.value });
      setText(configToText(parsed.value));
      toast.success(t(($) => $.tab_body.mcp_config.saved_toast));
    } catch (error) {
      toast.error(error instanceof Error && error.message ? error.message : t(($) => $.tab_body.mcp_config.save_failed_toast));
    } finally {
      setSaving(false);
    }
  };

  const invalid = parsed.error !== "";
  const invalidMessage = parsed.error === "mcp_config_not_object"
    ? t(($) => $.tab_body.mcp_config.invalid_not_object)
    : t(($) => $.tab_body.mcp_config.invalid_json, { error: parsed.error });

  return <div className="flex h-full flex-col space-y-3"><div className="flex items-start justify-between gap-3"><p className="text-xs text-muted-foreground">{t(($) => $.tab_body.mcp_config.intro)}</p>{text.trim() !== "" && <Button type="button" variant="outline" size="sm" onClick={() => setText("")}><Eraser className="h-3 w-3" />{t(($) => $.tab_body.mcp_config.clear_action)}</Button>}</div><Textarea value={text} onChange={(event) => setText(event.target.value)} className="min-h-72 flex-1 font-mono text-xs" spellCheck={false} aria-invalid={invalid || undefined} />{invalid && <p className="text-xs text-destructive">{invalidMessage}</p>}<div className="flex items-center justify-end gap-3">{dirty && <span className="text-xs text-muted-foreground">{t(($) => $.tab_body.common.unsaved_changes)}</span>}<Button type="button" size="sm" onClick={() => void save()} disabled={!dirty || invalid || saving}>{saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}{t(($) => $.tab_body.common.save)}</Button></div></div>;
}
