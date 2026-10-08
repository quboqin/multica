"use client";

import { useCallback, useRef, useState } from "react";
import type {
  CommentAgentGrant,
  CommentTriggerPreviewAgent,
  DocumentGrantPermission,
} from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { useT } from "../../i18n";

// A run of a mentioned agent authenticates as its runtime's owner, who may be
// outside this document's audience. The trigger preview says so per agent
// (`document_access`); before the comment is sent, the person posting decides
// whether that one run may read — and optionally reply on and edit — this
// document. The grant is recorded with the comment and lives only as long as
// the run; it never shares the document with the runtime owner as a person.

/** Agents in a trigger preview whose run cannot yet do all the poster could grant. */
export function agentsNeedingGrant(agents: CommentTriggerPreviewAgent[]): CommentTriggerPreviewAgent[] {
  return agents.filter((agent) => !!agent.document_access?.max_grant);
}

interface PendingPrompt {
  agents: CommentTriggerPreviewAgent[];
  resolve: (grants: CommentAgentGrant[] | null) => void;
}

/**
 * Asks, at send time, what the runs a comment triggers may do on this
 * document. `ask` resolves with the grants to send (possibly none), or null
 * when the person cancelled the send altogether.
 */
export function useAgentAccessGrantPrompt() {
  const [pending, setPending] = useState<PendingPrompt | null>(null);
  const pendingRef = useRef<PendingPrompt | null>(null);

  const ask = useCallback((agents: CommentTriggerPreviewAgent[]): Promise<CommentAgentGrant[] | null> => {
    const needing = agentsNeedingGrant(agents);
    if (needing.length === 0) return Promise.resolve([]);
    return new Promise((resolve) => {
      const prompt: PendingPrompt = { agents: needing, resolve };
      pendingRef.current = prompt;
      setPending(prompt);
    });
  }, []);

  const settle = useCallback((grants: CommentAgentGrant[] | null) => {
    const prompt = pendingRef.current;
    pendingRef.current = null;
    setPending(null);
    prompt?.resolve(grants);
  }, []);

  const dialog = pending ? <AgentAccessGrantDialog agents={pending.agents} onSettle={settle} /> : null;
  return { ask, dialog };
}

function permissionLabel(permission: string, t: ReturnType<typeof useT<"issues">>["t"]): string {
  switch (permission) {
    case "view":
      return t(($) => $.cortex_docs.can_read);
    case "edit":
    case "owner":
      return t(($) => $.cortex_docs.can_edit);
    default:
      return t(($) => $.comment.agent_grant.no_access);
  }
}

export function AgentAccessGrantDialog({
  agents,
  onSettle,
}: {
  agents: CommentTriggerPreviewAgent[];
  onSettle: (grants: CommentAgentGrant[] | null) => void;
}) {
  const { t } = useT("issues");
  // One choice per agent: what to grant, or nothing (send anyway).
  const [choices, setChoices] = useState<Record<string, DocumentGrantPermission | "none">>(() =>
    Object.fromEntries(agents.map((agent) => [agent.id, agent.document_access?.max_grant || "view"])),
  );
  const choose = (agentId: string, value: DocumentGrantPermission | "none") =>
    setChoices((prev) => ({ ...prev, [agentId]: value }));
  const confirm = () =>
    onSettle(
      agents.flatMap((agent) => {
        const choice = choices[agent.id];
        return choice && choice !== "none" ? [{ agent_id: agent.id, permission: choice }] : [];
      }),
    );

  return (
    <Dialog open onOpenChange={(open) => { if (!open) onSettle(null); }}>
      <DialogContent className="sm:max-w-md" data-testid="agent-access-grant-dialog">
        <DialogHeader>
          <DialogTitle>{t(($) => $.comment.agent_grant.title)}</DialogTitle>
          <DialogDescription>{t(($) => $.comment.agent_grant.description)}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          {agents.map((agent) => {
            const access = agent.document_access;
            const current = access?.runtime_owner_permission ?? "";
            const maxGrant = access?.max_grant || "view";
            const choice = choices[agent.id] ?? maxGrant;
            const options: Array<{ value: DocumentGrantPermission | "none"; label: string }> = [];
            if (maxGrant === "edit") {
              options.push({ value: "edit", label: t(($) => $.comment.agent_grant.grant_edit) });
            }
            if (current !== "view") {
              options.push({ value: "view", label: t(($) => $.comment.agent_grant.grant_view) });
            }
            options.push({ value: "none", label: t(($) => $.comment.agent_grant.grant_none) });
            return (
              <fieldset key={agent.id} className="flex flex-col gap-2 rounded-md border border-border p-3">
                <legend className="px-1 text-sm font-medium">{agent.name}</legend>
                <p className="text-caption text-muted-foreground">
                  {t(($) => $.comment.agent_grant.currently, { permission: permissionLabel(current, t) })}
                </p>
                {options.map((option) => (
                  <label key={option.value} className="flex cursor-pointer items-center gap-2 text-sm">
                    <input
                      type="radio"
                      name={`agent-grant-${agent.id}`}
                      value={option.value}
                      checked={choice === option.value}
                      onChange={() => choose(agent.id, option.value)}
                    />
                    <span>{option.label}</span>
                  </label>
                ))}
              </fieldset>
            );
          })}
          <p className="text-caption text-muted-foreground">{t(($) => $.comment.agent_grant.scope_note)}</p>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onSettle(null)}>
            {t(($) => $.comment.agent_grant.cancel)}
          </Button>
          <Button onClick={confirm}>{t(($) => $.comment.agent_grant.confirm)}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
