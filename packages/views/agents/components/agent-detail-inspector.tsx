"use client";

import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Camera, Loader2, Pencil, Plus, Tag } from "lucide-react";
import { toast } from "sonner";
import { useQuery } from "@tanstack/react-query";
import type {
  Agent,
  AgentRuntime,
  Label as ResourceLabel,
  MemberWithUser,
} from "@multica/core/types";
import {
  AGENT_DESCRIPTION_MAX_LENGTH,
  type AgentPresenceDetail,
} from "@multica/core/agents";
import { api } from "@multica/core/api";
import {
  labelListOptions,
  useAttachAgentLabel,
  useCreateLabel,
  useDetachAgentLabel,
} from "@multica/core/labels";
import { useWorkspaceId } from "@multica/core/hooks";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { isImeComposing } from "@multica/core/utils";
import { useTimeAgo } from "../../i18n";
import { Button } from "@multica/ui/components/ui/button";
import { ActorAvatar } from "../../common/actor-avatar";
import { Input } from "@multica/ui/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { PropRow } from "../../common/prop-row";
import { availabilityConfig } from "../presence";
import { CharCounter } from "./char-counter";
import { useT } from "../../i18n";
import { ConcurrencyPicker } from "./inspector/concurrency-picker";
import { ModelPicker } from "./inspector/model-picker";
import { RuntimePicker } from "./inspector/runtime-picker";
import { SkillAttach } from "./inspector/skill-attach";
import { ThinkingPropRow } from "./inspector/thinking-prop-row";
import { VisibilityPicker } from "./inspector/visibility-picker";
import { LarkAgentBindButton } from "../../settings/components/lark-tab";
import { LabelChip } from "../../labels/label-chip";

const INLINE_LABEL_COLORS = [
  "#ef4444",
  "#f97316",
  "#eab308",
  "#22c55e",
  "#14b8a6",
  "#3b82f6",
  "#6366f1",
  "#a855f7",
  "#ec4899",
  "#64748b",
] as const;

function pickInlineLabelColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  }
  return (
    INLINE_LABEL_COLORS[hash % INLINE_LABEL_COLORS.length] ??
    INLINE_LABEL_COLORS[0]!
  );
}

interface InspectorProps {
  agent: Agent;
  runtime: AgentRuntime | null;
  owner: MemberWithUser | null;
  presence: AgentPresenceDetail | null | undefined;
  // Below: needed for inline edit. The inspector now owns the editing surface
  // (no Settings tab anymore), so the parent has to pass through everything
  // a write needs.
  runtimes: AgentRuntime[];
  members: MemberWithUser[];
  currentUserId: string | null;
  /**
   * Computed by the parent via `useAgentPermissions(agent).canEdit.allowed`.
   * When false the inspector renders all editable surfaces as static
   * read-only displays — pickers become text/badges, name/description lose
   * their pencil affordance, the avatar is no longer clickable, and the
   * "Attach skill" trigger is hidden. Mirrors the backend gate at
   * `server/internal/handler/agent.go:519-535`.
   */
  canEdit: boolean;
  onUpdate: (id: string, data: Record<string, unknown>) => Promise<void>;
  /**
   * Focus the overview pane's Integrations tab. The inspector's Lark status
   * row is read-only and deep-links here; Manage / Disconnect live in the
   * tab so the destructive action exists in exactly one place.
   */
  onShowIntegrations: () => void;
}

/**
 * Left 320px column of the agent detail page. Holds the agent's identity card
 * (avatar / name / description / status), inline-editable properties, and
 * skills.
 *
 * **All editing happens here** — there is no separate Settings tab. The
 * trade-off is that the inspector carries some weight (4 inline pickers plus
 * 3 popovers for name/description/avatar), but it eliminates the "see vs
 * edit" mode split that the previous Settings tab created. Users no longer
 * have to switch tabs and hunt for the field they were already looking at.
 */
export function AgentDetailInspector({
  agent,
  runtime,
  owner,
  presence,
  runtimes,
  members,
  currentUserId,
  canEdit,
  onUpdate,
  onShowIntegrations,
}: InspectorProps) {
  const { t } = useT("agents");
  const timeAgo = useTimeAgo();
  const update = (data: Record<string, unknown>) => onUpdate(agent.id, data);
  const isOnline = runtime?.status === "online";

  return (
    <aside className="flex w-full flex-col rounded-lg border bg-background md:h-full md:min-h-0 md:overflow-y-auto">
      {/* Identity */}
      <div className="flex flex-col gap-3 border-b px-5 pb-5 pt-5">
        <AvatarEditor agent={agent} canEdit={canEdit} onUpdate={update} />
        <NameAndDescription
          agent={agent}
          canEdit={canEdit}
          onUpdate={update}
        />
        <PresenceBadge presence={presence} />
      </div>

      {/* Properties — editable when canEdit. When the current user lacks
          permission, each picker self-renders a static read-only display so
          the value is visible but not interactive. */}
      <Section label={t(($) => $.inspector.section_properties)}>
        <PropRow label={t(($) => $.inspector.prop_runtime)} interactive={false}>
          <RuntimePicker
            value={agent.runtime_id}
            runtimes={runtimes}
            members={members}
            currentUserId={currentUserId}
            canEdit={canEdit}
            onChange={(id) => update({ runtime_id: id })}
          />
        </PropRow>
        <PropRow label={t(($) => $.inspector.prop_model)} interactive={false}>
          <ModelPicker
            runtimeId={agent.runtime_id}
            runtimeOnline={!!isOnline}
            value={agent.model ?? ""}
            canEdit={canEdit}
            onChange={(m) => update({ model: m })}
          />
        </PropRow>
        <ThinkingPropRow
          runtimeId={agent.runtime_id}
          runtimeOnline={!!isOnline}
          model={agent.model ?? ""}
          value={agent.thinking_level ?? ""}
          canEdit={canEdit}
          onChange={(v) => update({ thinking_level: v })}
        />
        <PropRow label={t(($) => $.inspector.prop_visibility)} interactive={false}>
          <VisibilityPicker
            value={agent.visibility}
            canEdit={canEdit}
            onChange={(v) => update({ visibility: v })}
          />
        </PropRow>
        <PropRow label={t(($) => $.inspector.prop_concurrency)} interactive={false}>
          <ConcurrencyPicker
            value={agent.max_concurrent_tasks}
            canEdit={canEdit}
            onChange={(n) => update({ max_concurrent_tasks: n })}
          />
        </PropRow>
      </Section>

      {/* Details — read-only (no hover, no chip styling — these aren't clickable) */}
      <Section label={t(($) => $.inspector.section_details)}>
        {owner && (
          <PropRow label={t(($) => $.inspector.prop_owner)} interactive={false}>
            <span className="flex min-w-0 items-center gap-1.5">
              <ActorAvatar
                actorType="member"
                actorId={owner.user_id}
                size={14}
              />
              <span className="truncate">{owner.name}</span>
            </span>
          </PropRow>
        )}
        <PropRow label={t(($) => $.inspector.prop_created)} interactive={false}>
          <span className="text-muted-foreground">
            {timeAgo(agent.created_at)}
          </span>
        </PropRow>
        <PropRow label={t(($) => $.inspector.prop_updated)} interactive={false}>
          <span className="text-muted-foreground">
            {timeAgo(agent.updated_at)}
          </span>
        </PropRow>
      </Section>

      <AgentTagsSection agent={agent} canEdit={canEdit} />

      {/* Skills */}
      <div className="flex flex-col border-b px-5 py-4">
        <div className="mb-2 flex items-center gap-2">
          <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
            {t(($) => $.inspector.section_skills)}
          </span>
          <span className="font-mono text-[10px] tabular-nums text-muted-foreground/70">
            {agent.skills.length}
          </span>
        </div>
        <div className="flex flex-wrap gap-1">
          {agent.skills.map((s) => (
            <span
              key={s.id}
              className="rounded-md bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground"
            >
              {s.name}
            </span>
          ))}
          <SkillAttach agent={agent} canEdit={canEdit} />
        </div>
      </div>

      {/* Integrations — surfaces external-channel bind entry points
          (Lark Bot today; Slack / Discord in the future). The bind
          button self-hides when the server-side device-flow install
          capability gate is closed, so this section may render empty
          on deployments without a configured Lark app — that's
          intentional and matches the "don't surface a flow that will
          fail" guarantee. We only mount it for editors: viewers
          shouldn't see a CTA they can't action. */}
      {canEdit && (
        <div className="flex flex-col px-5 py-4">
          <div className="mb-2 flex items-center gap-2">
            <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
              {t(($) => $.inspector.section_integrations)}
            </span>
          </div>
          <div className="flex flex-wrap gap-2">
            <LarkAgentBindButton
              agentId={agent.id}
              agentName={agent.name}
              onShowConnectedDetails={onShowIntegrations}
            />
          </div>
        </div>
      )}
    </aside>
  );
}

function AgentTagsSection({
  agent,
  canEdit,
}: {
  agent: Agent;
  canEdit: boolean;
}) {
  const { t } = useT("agents");
  const labels = agent.labels ?? [];

  if (labels.length === 0 && !canEdit) return null;

  return (
    <div className="flex flex-col border-b px-5 py-4">
      <div className="mb-2 flex items-center gap-2">
        <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          {t(($) => $.inspector.section_tags)}
        </span>
        <span className="font-mono text-[10px] tabular-nums text-muted-foreground/70">
          {labels.length}
        </span>
        {canEdit && <AgentTagEditor agent={agent} />}
      </div>
      {labels.length > 0 ? (
        <div className="flex flex-wrap gap-1">
          {labels.map((label) => (
            <LabelChip key={label.id} label={label} />
          ))}
        </div>
      ) : (
        <span className="text-xs text-muted-foreground">
          {t(($) => $.create_dialog.tags_section.empty)}
        </span>
      )}
    </div>
  );
}

function AgentTagEditor({ agent }: { agent: Agent }) {
  const { t } = useT("agents");
  const wsId = useWorkspaceId();
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState("");
  const creatingRef = useRef(false);

  const { data: allLabels = [], isLoading } = useQuery(
    labelListOptions(wsId, "agent"),
  );
  const attach = useAttachAgentLabel(agent.id);
  const detach = useDetachAgentLabel(agent.id);
  const create = useCreateLabel("agent");

  const currentIds = useMemo(
    () => new Set((agent.labels ?? []).map((label) => label.id)),
    [agent.labels],
  );
  const [draftIds, setDraftIds] = useState<Set<string>>(() => new Set());
  const query = filter.trim();
  const queryLower = query.toLowerCase();
  const filteredLabels = allLabels.filter((label) =>
    label.name.toLowerCase().includes(queryLower),
  );
  const exactMatch = allLabels.some(
    (label) => label.name.toLowerCase() === queryLower,
  );
  const isSaving = attach.isPending || detach.isPending;
  const isBusy = isSaving || create.isPending;
  const canCreate = query.length > 0 && !exactMatch && !isBusy;
  const hasChanges =
    draftIds.size !== currentIds.size ||
    [...draftIds].some((labelId) => !currentIds.has(labelId));

  useEffect(() => {
    if (!open) setDraftIds(new Set(currentIds));
  }, [currentIds, open]);

  const toggle = (labelId: string) => {
    setDraftIds((prev) => {
      const next = new Set(prev);
      if (next.has(labelId)) next.delete(labelId);
      else next.add(labelId);
      return next;
    });
  };

  const createAndSelect = () => {
    if (!canCreate || creatingRef.current) return;
    creatingRef.current = true;
    const name = query;
    create.mutate(
      { name, color: pickInlineLabelColor(name) },
      {
        onSuccess: (label) => {
          setDraftIds((prev) => new Set(prev).add(label.id));
          setFilter("");
        },
        onError: (err: unknown) => {
          toast.error(
            err instanceof Error
              ? err.message
              : t(($) => $.create_dialog.tags_section.create_failed_toast),
          );
        },
        onSettled: () => {
          creatingRef.current = false;
        },
      },
    );
  };

  const saveChanges = async () => {
    if (!hasChanges) {
      setOpen(false);
      setFilter("");
      return;
    }
    const toAttach = [...draftIds].filter((labelId) => !currentIds.has(labelId));
    const toDetach = [...currentIds].filter((labelId) => !draftIds.has(labelId));
    try {
      for (const labelId of toDetach) {
        await detach.mutateAsync(labelId);
      }
      for (const labelId of toAttach) {
        await attach.mutateAsync(labelId);
      }
      setOpen(false);
      setFilter("");
    } catch (err) {
      toast.error(
        err instanceof Error
          ? err.message
          : t(($) => $.inspector.save_tags_failed_toast),
      );
    }
  };

  return (
    <Popover
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) setDraftIds(new Set(currentIds));
        setOpen(nextOpen);
        if (!nextOpen) setFilter("");
      }}
    >
      <PopoverTrigger
        render={
          <button
            type="button"
            className="ml-auto inline-flex h-6 w-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
            aria-label={t(($) => $.inspector.edit_tags_aria)}
          >
            <Pencil className="h-3.5 w-3.5" />
          </button>
        }
      />
      <PopoverContent align="start" className="w-80 p-3">
        <div className="space-y-2">
          <div className="relative">
            <Tag className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={filter}
              onChange={(event) => setFilter(event.target.value)}
              onKeyDown={(event) => {
                if (isImeComposing(event)) return;
                if (event.key === "Enter") {
                  event.preventDefault();
                  createAndSelect();
                }
              }}
              placeholder={t(
                ($) => $.create_dialog.tags_section.search_placeholder,
              )}
              className="h-8 pl-8 text-sm"
              maxLength={32}
            />
          </div>
          <div className="max-h-52 overflow-y-auto rounded-md border bg-background p-1">
            {isLoading && (
              <div className="px-2 py-1.5 text-sm text-muted-foreground">
                {t(($) => $.create_dialog.tags_section.loading)}
              </div>
            )}
            {!isLoading && filteredLabels.length === 0 && !canCreate && (
              <div className="px-2 py-1.5 text-sm text-muted-foreground">
                {t(($) => $.create_dialog.tags_section.empty)}
              </div>
            )}
            {filteredLabels.map((label: ResourceLabel) => {
              const selected = draftIds.has(label.id);
              return (
                <button
                  key={label.id}
                  type="button"
                  onClick={() => toggle(label.id)}
                  disabled={isBusy}
                  className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors disabled:opacity-60 ${
                    selected ? "bg-accent" : "hover:bg-accent/70"
                  }`}
                >
                  <span
                    className="h-3 w-3 shrink-0 rounded-full"
                    style={{ backgroundColor: label.color }}
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1 truncate">{label.name}</span>
                  {selected && (
                    <span className="text-xs text-muted-foreground">
                      {t(($) => $.create_dialog.tags_section.selected)}
                    </span>
                  )}
                </button>
              );
            })}
            {canCreate && (
              <button
                type="button"
                onClick={createAndSelect}
                className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent/70"
              >
                <Plus className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">
                  {t(($) => $.create_dialog.tags_section.create_action)}{" "}
                  <span className="font-medium">&ldquo;{query}&rdquo;</span>
                </span>
                <span
                  className="h-3 w-3 shrink-0 rounded-full"
                  style={{ backgroundColor: pickInlineLabelColor(query) }}
                  aria-hidden
                />
              </button>
            )}
          </div>
          <div className="flex items-center justify-end gap-2 pt-1">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={() => setOpen(false)}
              disabled={isBusy}
            >
              {t(($) => $.inspector.cancel)}
            </Button>
            <Button
              type="button"
              size="sm"
              onClick={saveChanges}
              disabled={!hasChanges || isBusy}
            >
              {isSaving && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
              {t(($) => $.inspector.save)}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// Layout helpers
// ---------------------------------------------------------------------------

function Section({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="border-b px-5 py-4">
      <div className="mb-1 -mx-2 px-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </div>
      <div className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5">
        {children}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Identity — avatar / name / description editors
// ---------------------------------------------------------------------------

function AvatarEditor({
  agent,
  canEdit,
  onUpdate,
}: {
  agent: Agent;
  canEdit: boolean;
  onUpdate: (data: Record<string, unknown>) => Promise<void>;
}) {
  const { t } = useT("agents");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { upload, uploading } = useFileUpload(api);

  if (!canEdit) {
    return (
      <div className="h-14 w-14 shrink-0 overflow-hidden rounded-lg">
        <ActorAvatar
          actorType="agent"
          actorId={agent.id}
          size={56}
          className="rounded-none"
        />
      </div>
    );
  }

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    try {
      const result = await upload(file);
      if (!result) return;
      await onUpdate({ avatar_url: result.link });
      toast.success(t(($) => $.inspector.avatar_updated_toast));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.inspector.avatar_upload_failed_toast));
    }
  };

  return (
    <>
      <button
        type="button"
        // rounded-lg matches the standard agent avatar treatment used in
        // list rows. Avoid rounded-full — circles are reserved for humans.
        className="group relative h-14 w-14 shrink-0 overflow-hidden rounded-lg focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={() => fileInputRef.current?.click()}
        disabled={uploading}
        aria-label={t(($) => $.inspector.change_avatar_aria)}
      >
        <ActorAvatar
          actorType="agent"
          actorId={agent.id}
          size={56}
          className="rounded-none"
        />
        <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
          {uploading ? (
            <Loader2 className="h-4 w-4 animate-spin text-white" />
          ) : (
            <Camera className="h-4 w-4 text-white" />
          )}
        </div>
      </button>
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={handleFile}
      />
    </>
  );
}

function NameAndDescription({
  agent,
  canEdit,
  onUpdate,
}: {
  agent: Agent;
  canEdit: boolean;
  onUpdate: (data: Record<string, unknown>) => Promise<void>;
}) {
  const { t } = useT("agents");
  if (!canEdit) {
    return (
      <div className="flex flex-col gap-1">
        <span className="text-base font-semibold leading-tight">
          {agent.name}
        </span>
        {agent.description ? (
          <span className="text-xs leading-relaxed text-muted-foreground">
            {agent.description}
          </span>
        ) : (
          <span className="text-xs italic leading-relaxed text-muted-foreground/50">
            {t(($) => $.inspector.no_description_placeholder)}
          </span>
        )}
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-1">
      <InlineEditPopover
        value={agent.name}
        onSave={(v) => onUpdate({ name: v.trim() })}
        kind="input"
        title={t(($) => $.inspector.rename_title)}
        placeholder={t(($) => $.inspector.rename_placeholder)}
        validate={(v) => (v.trim().length > 0 ? null : t(($) => $.inspector.rename_required))}
      >
        {(triggerProps) => (
          <button
            type="button"
            {...triggerProps}
            className="group -mx-1 inline-flex items-center gap-1.5 self-start rounded px-1 text-left text-base font-semibold leading-tight transition-colors hover:bg-accent/50"
          >
            <span>{agent.name}</span>
            <Pencil className="h-3 w-3 shrink-0 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground" />
          </button>
        )}
      </InlineEditPopover>

      <DescriptionEditor
        value={agent.description ?? ""}
        onSave={(v) => onUpdate({ description: v })}
      />
    </div>
  );
}

// Description editor — modal because the description benefits from a roomy
// composition surface (the inline popover was 288 px wide × 3 rows, too
// cramped to read or edit anything substantial). Name stays in the inline
// popover above: a single line is the right shape for it.
//
// The editor body is split into a child component that mounts only while
// the dialog is open. That way the draft state is initialised from `value`
// at mount time and never reset by an external update mid-edit — closing
// the dialog unmounts the body, reopening starts fresh with the latest
// value. This is the React-recommended replacement for the
// `useEffect(reset, [value])` anti-pattern (see "You Might Not Need an
// Effect" — Resetting state with a key / mount).
function DescriptionEditor({
  value,
  onSave,
}: {
  value: string;
  onSave: (next: string) => Promise<void>;
}) {
  const { t } = useT("agents");
  const [open, setOpen] = useState(false);

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="group -mx-1 inline-flex items-start gap-1.5 self-start rounded px-1 text-left text-xs leading-relaxed transition-colors hover:bg-accent/50"
      >
        {value ? (
          <span className="text-muted-foreground">{value}</span>
        ) : (
          <span className="italic text-muted-foreground/50">{t(($) => $.inspector.no_description_placeholder)}</span>
        )}
        <Pencil className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground" />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          {open && (
            <DescriptionEditorBody
              initialValue={value}
              onSave={onSave}
              onClose={() => setOpen(false)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

function DescriptionEditorBody({
  initialValue,
  onSave,
  onClose,
}: {
  initialValue: string;
  onSave: (next: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useT("agents");
  const [draft, setDraft] = useState(initialValue);
  const [saving, setSaving] = useState(false);

  const length = [...draft].length;
  const overLimit = length > AGENT_DESCRIPTION_MAX_LENGTH;
  const dirty = draft !== initialValue;

  const commit = async () => {
    if (overLimit || !dirty) return;
    setSaving(true);
    try {
      await onSave(draft);
      onClose();
    } catch {
      // toast handled by parent's onUpdate
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t(($) => $.inspector.edit_description_title)}</DialogTitle>
      </DialogHeader>
      <div className="flex flex-col gap-2">
        <textarea
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          placeholder={t(($) => $.inspector.description_placeholder)}
          rows={6}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              onClose();
              return;
            }
            if (isImeComposing(e)) return;
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault();
              void commit();
            }
          }}
          className="w-full resize-none rounded-md border bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-input"
        />
        <CharCounter length={length} max={AGENT_DESCRIPTION_MAX_LENGTH} />
      </div>
      <DialogFooter>
        <Button
          variant="ghost"
          size="sm"
          onClick={onClose}
          disabled={saving}
        >
          {t(($) => $.inspector.cancel)}
        </Button>
        <Button
          size="sm"
          onClick={() => void commit()}
          disabled={saving || overLimit || !dirty}
        >
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : t(($) => $.inspector.save)}
        </Button>
      </DialogFooter>
    </>
  );
}


// Generic single-field popover editor used for name / description. Keeps the
// trigger styling fully in the caller's hands by using a render prop.
function InlineEditPopover({
  value,
  onSave,
  kind,
  title,
  placeholder,
  validate,
  children,
}: {
  value: string;
  onSave: (next: string) => Promise<void>;
  kind: "input" | "textarea";
  title: string;
  placeholder?: string;
  validate?: (v: string) => string | null;
  children: (triggerProps: {
    onClick: (e: React.MouseEvent) => void;
  }) => ReactNode;
}) {
  const { t } = useT("agents");
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset draft when popover opens or upstream value changes between sessions.
  useEffect(() => {
    if (open) {
      setDraft(value);
      setError(null);
    }
  }, [open, value]);

  const commit = async () => {
    const err = validate?.(draft) ?? null;
    if (err) {
      setError(err);
      return;
    }
    if (draft === value) {
      setOpen(false);
      return;
    }
    setSaving(true);
    try {
      await onSave(draft);
      setOpen(false);
    } catch {
      // toast handled by parent's onUpdate
    } finally {
      setSaving(false);
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={children({ onClick: () => setOpen(true) }) as React.ReactElement}
      />
      <PopoverContent align="start" className="w-72 p-3">
        <div className="space-y-2">
          <p className="text-xs font-medium">{title}</p>
          {kind === "input" ? (
            <Input
              autoFocus
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                if (error) setError(null);
              }}
              placeholder={placeholder}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setOpen(false);
                  return;
                }
                if (isImeComposing(e)) return;
                if (e.key === "Enter") {
                  e.preventDefault();
                  void commit();
                }
              }}
              className="h-8"
            />
          ) : (
            <textarea
              autoFocus
              value={draft}
              onChange={(e) => {
                setDraft(e.target.value);
                if (error) setError(null);
              }}
              placeholder={placeholder}
              onKeyDown={(e) => {
                if (e.key === "Escape") {
                  setOpen(false);
                  return;
                }
                if (isImeComposing(e)) return;
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault();
                  void commit();
                }
              }}
              rows={3}
              className="w-full resize-none rounded-md border bg-transparent px-2 py-1.5 text-xs outline-none focus-visible:border-input"
            />
          )}
          {error && <p className="text-xs text-destructive">{error}</p>}
          <div className="flex items-center justify-end gap-2">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setOpen(false)}
              disabled={saving}
            >
              {t(($) => $.inspector.cancel)}
            </Button>
            <Button
              size="sm"
              onClick={() => void commit()}
              disabled={saving || draft === value}
            >
              {saving ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                t(($) => $.inspector.save)
              )}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

// ---------------------------------------------------------------------------
// Presence badge — unchanged from the previous version
// ---------------------------------------------------------------------------

function PresenceBadge({
  presence,
}: {
  presence: AgentPresenceDetail | null | undefined;
}) {
  const { t } = useT("agents");
  // Archived is carried by the unified presence (deriveAgentPresenceDetail
  // sets availability="archived" before any runtime/task scan), so the
  // normal path below renders the gray "Archived" badge with no special
  // case here — same single source of truth as every other status surface.
  if (!presence) {
    return (
      <span className="inline-flex h-5 w-20 animate-pulse rounded-md bg-muted" />
    );
  }
  const av = availabilityConfig[presence.availability];
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span
        className={`inline-flex items-center gap-1.5 rounded-md border px-1.5 py-0.5 text-xs ${av.textClass}`}
      >
        <span className={`h-1.5 w-1.5 rounded-full ${av.dotClass}`} />
        {t(($) => $.availability[presence.availability])}
      </span>
    </div>
  );
}
