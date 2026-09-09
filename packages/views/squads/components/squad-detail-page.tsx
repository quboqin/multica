"use client";

import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import { useWorkspaceId } from "@multica/core/hooks";
import { resolvePublicFileUrl } from "@multica/core/workspace/avatar-url";
import { useFileUpload } from "@multica/core/hooks/use-file-upload";
import { isImeComposing } from "@multica/core/utils";
import { useTimeAgo } from "../../i18n";
import { agentListOptions, memberListOptions, squadMemberStatusOptions, workspaceKeys } from "@multica/core/workspace/queries";
import { runtimeListOptions } from "@multica/core/runtimes";
import { CreateAgentDialog } from "../../agents/components/create-agent-dialog";
import { useNavigation } from "../../navigation";
import { AppLink } from "../../navigation";
import { BreadcrumbHeader } from "../../layout/breadcrumb-header";
import { PageHeader } from "../../layout/page-header";
import {
  Users,
  Plus,
  Trash2,
  ArrowUpRight,
  ArrowDown,
  ArrowRight,
  Crown,
  Camera,
  Loader2,
  Pencil,
  FileText,
  Save,
  Network,
  ClipboardCheck,
  BookOpen,
  Search,
  Lightbulb,
  Code2,
  ShieldCheck,
  TestTube2,
  Rocket,
  CircleHelp,
  GripVertical,
  Sparkles,
  ChevronUp,
  ChevronDown as ChevronDownIcon,
  LayoutList,
  Workflow as WorkflowIcon,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Textarea } from "@multica/ui/components/ui/textarea";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@multica/ui/components/ui/dialog";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { ActorAvatar as ActorAvatarBase } from "@multica/ui/components/common/actor-avatar";
import { ActorAvatar } from "../../common/actor-avatar";
import { ContentEditor } from "../../editor/content-editor";
import {
  PickerItem,
  PickerSection,
  PickerEmpty,
} from "../../issues/components/pickers/property-picker";
import { ChevronDown, UserPlus } from "lucide-react";
import { toast } from "sonner";
import type { Squad, SquadMember, SquadMemberStatus, SquadMemberStatusValue, SquadWorkflowAssignmentsResponse, SquadWorkflowCanvasLayout, SquadWorkflowStage, Agent, CreateAgentRequest, MemberWithUser } from "@multica/core/types";
import { useT } from "../../i18n";
import { matchesPinyin } from "../../editor/extensions/pinyin-match";
import { SquadWorkflowCanvas } from "./squad-workflow-canvas";

export function SquadDetailPage() {
  const { t } = useT("squads");
  const workspace = useCurrentWorkspace();
  const wsId = useWorkspaceId();
  const p = useWorkspacePaths();
  const { pathname, push } = useNavigation();
  const queryClient = useQueryClient();
  const squadId = pathname.split("/").pop() ?? "";

  const { data: squad, refetch: refetchSquad } = useQuery<Squad>({
    queryKey: [...workspaceKeys.squads(wsId), squadId],
    queryFn: () => api.getSquad(squadId),
    enabled: !!workspace?.id && !!squadId,
  });

  const { data: members = [], refetch: refetchMembers } = useQuery<SquadMember[]>({
    queryKey: [...workspaceKeys.squads(wsId), squadId, "members"],
    queryFn: () => api.listSquadMembers(squadId),
    enabled: !!workspace?.id && !!squadId,
  });

  // Per-squad working/idle/offline + active-issue snapshot. WS task / agent /
  // daemon events invalidate this via use-realtime-sync; the staleTime is a
  // tab-focus safety net. Indexed by member_id so SquadMembersTab can look up
  // its row in O(1).
  const { data: memberStatusResp } = useQuery({
    ...squadMemberStatusOptions(wsId, squadId),
    enabled: !!workspace?.id && !!squadId,
  });
  const memberStatusById = useMemo(() => {
    const map = new Map<string, SquadMemberStatus>();
    for (const s of memberStatusResp?.members ?? []) map.set(s.member_id, s);
    return map;
  }, [memberStatusResp]);

  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: wsMembers = [] } = useQuery(memberListOptions(wsId));

  // Runtimes are only fetched when the Create Agent dialog might open;
  // gating on isWorkspaceAdmin below means non-admins never trigger the
  // request. The runtime list mirrors the agents page so the picker
  // (and the "only my runtimes" filter) behaves identically here.
  const currentUser = useAuthStore((s) => s.user);
  const myRole = useMemo(() => {
    if (!currentUser) return null;
    return wsMembers.find((m) => m.user_id === currentUser.id)?.role ?? null;
  }, [wsMembers, currentUser]);
  const isWorkspaceAdmin = myRole === "owner" || myRole === "admin";

  const { data: runtimes = [], isLoading: runtimesLoading } = useQuery({
    ...runtimeListOptions(wsId),
    enabled: !!wsId && isWorkspaceAdmin,
  });

  const [showAddMember, setShowAddMember] = useState(false);
  const [showCreateAgent, setShowCreateAgent] = useState(false);
  const [confirmArchive, setConfirmArchive] = useState(false);

  const updateSquadMut = useMutation({
    mutationFn: (data: { name?: string; description?: string; instructions?: string; avatar_url?: string; leader_id?: string }) => api.updateSquad(squadId, data),
    onSuccess: () => {
      refetchSquad();
      refetchMembers();
      queryClient.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
    },
  });

  const addMemberMut = useMutation({
    mutationFn: (input: { type: "agent" | "member"; id: string; role?: string }) =>
      api.addSquadMember(squadId, {
        member_type: input.type,
        member_id: input.id,
        role: input.role?.trim() || undefined,
      }),
    onSuccess: () => { refetchMembers(); toast.success("Member added"); },
    onError: (err) =>
      toast.error(err instanceof Error && err.message ? err.message : "Failed to add member"),
  });

  const removeMemberMut = useMutation({
    mutationFn: (m: SquadMember) => api.removeSquadMember(squadId, { member_type: m.member_type, member_id: m.member_id }),
    onSuccess: () => { refetchMembers(); toast.success("Member removed"); },
    onError: (err) =>
      toast.error(err instanceof Error && err.message ? err.message : "Failed to remove member"),
  });

  const updateRoleMut = useMutation({
    mutationFn: (input: { member: SquadMember; role: string }) =>
      api.updateSquadMemberRole(squadId, {
        member_type: input.member.member_type,
        member_id: input.member.member_id,
        role: input.role,
      }),
    onSuccess: () => { refetchMembers(); toast.success("Role updated"); },
    onError: (err) =>
      toast.error(err instanceof Error && err.message ? err.message : "Failed to update role"),
  });

  const setLeaderMut = useMutation({
    mutationFn: (agentId: string) => api.updateSquad(squadId, { leader_id: agentId }),
    onSuccess: () => {
      refetchSquad();
      refetchMembers();
      queryClient.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) });
      toast.success("Leader updated");
    },
    onError: (err) =>
      toast.error(err instanceof Error && err.message ? err.message : "Failed to update leader"),
  });

  const deleteMut = useMutation({
    mutationFn: () => api.deleteSquad(squadId),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: workspaceKeys.squads(wsId) }); push(p.squads()); toast.success("Squad archived"); },
    onError: (err) =>
      toast.error(err instanceof Error && err.message ? err.message : "Failed to archive squad"),
  });

  // CreateAgentDialog's onCreate contract: hit POST /api/agents and
  // return the created agent so the dialog can run its skill follow-up.
  // We deliberately do NOT navigate to the agent detail page (that's
  // the agents-page behaviour) — the user clicked Create Agent from
  // inside this squad, so the dialog will stay open just long enough
  // to also call addSquadMember (handled by the dialog when squadId
  // is set), then close the user back to Members where they can
  // verify the new agent appeared. Cache-update keeps the agents list
  // fresh for any pickers that read from it.
  const handleCreateAgent = async (data: CreateAgentRequest): Promise<Agent> => {
    const agent = await api.createAgent(data);
    queryClient.setQueryData<Agent[]>(workspaceKeys.agents(wsId), (current = []) => {
      const exists = current.some((a) => a.id === agent.id);
      return exists ? current.map((a) => (a.id === agent.id ? agent : a)) : [...current, agent];
    });
    queryClient.invalidateQueries({ queryKey: workspaceKeys.agents(wsId) });
    return agent;
  };

  const getEntityName = (type: string, id: string) => {
    if (type === "agent") return agents.find((a: Agent) => a.id === id)?.name ?? id.slice(0, 8);
    return wsMembers.find((m) => m.user_id === id)?.name ?? id.slice(0, 8);
  };

  if (!squad) {
    return <SquadDetailSkeleton />;
  }

  const availableAgents = agents.filter((a: Agent) => !a.archived_at && !members.some((m) => m.member_type === "agent" && m.member_id === a.id));
  const availableMembers = wsMembers.filter((m) => !members.some((sm) => sm.member_type === "member" && sm.member_id === m.user_id));
  const isLeader = (m: SquadMember) => m.member_type === "agent" && squad.leader_id === m.member_id;
  const isArchived = (m: SquadMember) =>
    m.member_type === "agent" && !!agents.find((a: Agent) => a.id === m.member_id)?.archived_at;

  const initials = squad.name
    .split(" ")
    .map((w) => w[0])
    .join("")
    .toUpperCase()
    .slice(0, 2);

  return (
    <div className="flex flex-1 min-h-0 flex-col">
      <BreadcrumbHeader
        segments={[{ href: p.squads(), label: t(($) => $.page.title) }]}
        leaf={
          <>
            <SquadHeaderAvatar squad={squad} initials={initials} />
            <h1 className="truncate text-sm font-medium text-foreground">{squad.name}</h1>
          </>
        }
        actions={
          <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" onClick={() => setConfirmArchive(true)}>
            <Trash2 className="size-3.5 mr-1" />
            {t(($) => $.inspector.archive_button)}
          </Button>
        }
      />

      {/* Two-column grid mirrors agent-detail-page: left inspector (identity +
          properties + leader), right pane with tabs (Members | Instructions).
          Mobile collapses to stacked single column. */}
      <div className="flex flex-1 min-h-0 flex-col gap-3 overflow-y-auto p-3 md:grid md:grid-cols-[280px_minmax(0,1fr)] md:gap-4 md:overflow-hidden md:p-6 lg:grid-cols-[320px_minmax(0,1fr)]">
        <SquadDetailInspector
          squad={squad}
          memberCount={members.length}
          leaderName={getEntityName("agent", squad.leader_id)}
          creatorName={getEntityName("member", squad.creator_id)}
          uploadingAvatar={updateSquadMut.isPending}
          onUploadAvatar={(url) => updateSquadMut.mutateAsync({ avatar_url: url })}
          onRename={async (next) => { await updateSquadMut.mutateAsync({ name: next.trim() }); }}
          onUpdateDescription={async (next) => { await updateSquadMut.mutateAsync({ description: next }); }}
        />

        <SquadOverviewPane
          squad={squad}
          members={members}
          agents={agents}
          memberStatusById={memberStatusById}
          isLeader={isLeader}
          isArchived={isArchived}
          getEntityName={getEntityName}
          onAddMemberClick={() => setShowAddMember(true)}
          onCreateAgentClick={isWorkspaceAdmin ? () => setShowCreateAgent(true) : undefined}
          onSetLeader={(id) => setLeaderMut.mutate(id)}
          onRemoveMember={(m) => removeMemberMut.mutate(m)}
          onUpdateRole={async (m, role) => { await updateRoleMut.mutateAsync({ member: m, role }); }}
          onSaveInstructions={async (next) => { await updateSquadMut.mutateAsync({ instructions: next }); toast.success("Instructions saved"); }}
          setLeaderPending={setLeaderMut.isPending}
          canManage={isWorkspaceAdmin}
        />
      </div>

      {showAddMember && (
        <AddMemberDialog
          availableMembers={availableMembers}
          availableAgents={availableAgents}
          onClose={() => setShowAddMember(false)}
          onSubmit={async (input) => { await addMemberMut.mutateAsync(input); }}
        />
      )}

      {/* Squad-scoped create flow: same dialog as the Agents page but
          with squadId set, so the dialog runs api.addSquadMember after
          api.createAgent and skips the agent-detail navigation. Only
          mounted for workspace owner/admin since AddSquadMember is
          owner/admin-gated server-side; for everyone else the trigger
          never renders. */}
      {showCreateAgent && isWorkspaceAdmin && (
        <CreateAgentDialog
          runtimes={runtimes}
          runtimesLoading={runtimesLoading}
          members={wsMembers}
          currentUserId={currentUser?.id ?? null}
          squadId={squadId}
          onClose={() => setShowCreateAgent(false)}
          onCreate={handleCreateAgent}
        />
      )}

      {confirmArchive && (
        <AlertDialog
          open
          onOpenChange={(v) => { if (!v && !deleteMut.isPending) setConfirmArchive(false); }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t(($) => $.archive_dialog.title)}</AlertDialogTitle>
              <AlertDialogDescription>
                {t(($) => $.archive_dialog.description, { name: squad.name })}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel disabled={deleteMut.isPending}>
                {t(($) => $.archive_dialog.cancel)}
              </AlertDialogCancel>
              <AlertDialogAction
                onClick={() => deleteMut.mutate()}
                disabled={deleteMut.isPending}
                className="bg-destructive text-white hover:bg-destructive/90"
              >
                {deleteMut.isPending
                  ? t(($) => $.archive_dialog.archiving)
                  : t(($) => $.archive_dialog.confirm)}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </div>
  );
}

// Initial-load skeleton — mirrors the two-column layout of the loaded page
// (left inspector + right tabs panel) so the swap to real content doesn't
// shift layout. Column widths match the md:/lg: breakpoints used below.
function SquadDetailSkeleton() {
  return (
    <div className="flex flex-1 min-h-0 flex-col">
      <PageHeader className="px-5">
        <Skeleton className="h-5 w-48" />
      </PageHeader>
      <div className="flex flex-1 min-h-0 flex-col gap-3 overflow-y-auto p-3 md:grid md:grid-cols-[280px_minmax(0,1fr)] md:gap-4 md:overflow-hidden md:p-6 lg:grid-cols-[320px_minmax(0,1fr)]">
        <div className="flex flex-col gap-4 rounded-lg border p-5">
          <Skeleton className="h-16 w-16 rounded-lg" />
          <Skeleton className="h-5 w-40" />
          <Skeleton className="h-3 w-full" />
          <div className="space-y-2">
            <Skeleton className="h-3 w-3/4" />
            <Skeleton className="h-3 w-2/3" />
            <Skeleton className="h-3 w-1/2" />
          </div>
        </div>
        <div className="flex flex-col gap-4 rounded-lg border p-6">
          <div className="flex items-center gap-4">
            <Skeleton className="h-4 w-20" />
            <Skeleton className="h-4 w-24" />
          </div>
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-5/6" />
          <Skeleton className="h-4 w-4/6" />
        </div>
      </div>
    </div>
  );
}

// Compact 16px avatar shown next to the name in the page header. Falls back
// to the Users icon when no custom avatar is set so the squad still has a
// recognisable glyph in the breadcrumb strip.
function SquadHeaderAvatar({ squad, initials }: { squad: Squad; initials: string }) {
  if (!squad.avatar_url) {
    return <Users className="h-4 w-4 text-muted-foreground" />;
  }
  return (
    <ActorAvatarBase
      name={squad.name}
      initials={initials}
      avatarUrl={resolvePublicFileUrl(squad.avatar_url)}
      size={16}
      className="rounded"
    />
  );
}

// Large click-to-upload avatar editor. Mirrors AvatarEditor in
// agent-detail-inspector.tsx — square (rounded-md) treatment is reserved
// for non-human actors (agent, squad), circles for humans.
function SquadAvatarEditor({
  squad,
  initials,
  uploading,
  onUpload,
}: {
  squad: Squad;
  initials: string;
  uploading: boolean;
  onUpload: (url: string) => Promise<unknown>;
}) {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { upload, uploading: fileUploading } = useFileUpload(api);
  const busy = uploading || fileUploading;

  const handleFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    e.target.value = "";
    try {
      const result = await upload(file);
      if (!result) return;
      await onUpload(result.link);
      toast.success("Avatar updated");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Failed to upload avatar");
    }
  };

  return (
    <>
      <button
        type="button"
        className="group relative h-16 w-16 shrink-0 overflow-hidden rounded-lg bg-muted focus:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        onClick={() => fileInputRef.current?.click()}
        disabled={busy}
        aria-label="Change squad avatar"
      >
        {squad.avatar_url ? (
          <ActorAvatarBase
            name={squad.name}
            initials={initials}
            avatarUrl={resolvePublicFileUrl(squad.avatar_url)}
            size={64}
            className="rounded-none"
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-muted-foreground">
            <Users className="h-7 w-7" />
          </div>
        )}
        <div className="absolute inset-0 flex items-center justify-center bg-black/40 opacity-0 transition-opacity group-hover:opacity-100">
          {busy ? (
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

// Inline name editor — reveals a Pencil affordance on hover, opens a small
// popover with a single-line input. Mirrors the NameAndDescription editor
// in the agent inspector.
function SquadNameEditor({
  value,
  onSave,
}: {
  value: string;
  onSave: (next: string) => Promise<void>;
}) {
  return (
    <InlineEditPopover
      value={value}
      onSave={onSave}
      title="Rename squad"
      placeholder="Squad name"
      validate={(v) => (v.trim().length > 0 ? null : "Name is required")}
    >
      {(triggerProps) => (
        <button
          type="button"
          {...triggerProps}
          className="group -mx-1 inline-flex items-center gap-1.5 self-start rounded px-1 text-left text-lg font-semibold leading-tight transition-colors hover:bg-accent/50"
        >
          <span>{value}</span>
          <Pencil className="h-3.5 w-3.5 shrink-0 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground" />
        </button>
      )}
    </InlineEditPopover>
  );
}

function InlineEditPopover({
  value,
  onSave,
  title,
  placeholder,
  validate,
  children,
}: {
  value: string;
  onSave: (next: string) => Promise<void>;
  title: string;
  placeholder?: string;
  validate?: (v: string) => string | null;
  children: (triggerProps: { onClick: (e: React.MouseEvent) => void }) => ReactNode;
}) {
  const { t } = useT("squads");
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
      toast.success("Saved");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Failed to save");
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
          {error && <p className="text-xs text-destructive">{error}</p>}
          <div className="flex items-center justify-end gap-2">
            <Button variant="ghost" size="sm" onClick={() => setOpen(false)} disabled={saving}>
              {t(($) => $.name_editor.cancel)}
            </Button>
            <Button size="sm" onClick={() => void commit()} disabled={saving || draft === value}>
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : "Save"}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}

// Two-step add-member dialog (mirrors CreateAgentDialog's compact layout):
// 1) pick a target — Members + Agents in one searchable popover, each row
//    with an avatar so visual recognition matches the issue assignee picker;
// 2) optionally describe the role they'll play in this squad. Description
//    lives here (not on the picker) because role is per-squad context that
//    only makes sense at the moment of joining.
function AddMemberDialog({
  availableMembers,
  availableAgents,
  onClose,
  onSubmit,
}: {
  availableMembers: MemberWithUser[];
  availableAgents: Agent[];
  onClose: () => void;
  onSubmit: (input: { type: "agent" | "member"; id: string; role?: string }) => Promise<void>;
}) {
  const { t } = useT("squads");
  const [target, setTarget] = useState<{ type: "agent" | "member"; id: string; name: string } | null>(null);
  const [role, setRole] = useState("");
  const [pickerOpen, setPickerOpen] = useState(false);
  const [pickerFilter, setPickerFilter] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const query = pickerFilter.trim().toLowerCase();
  const filteredMembers = availableMembers.filter((m) => m.name.toLowerCase().includes(query) || matchesPinyin(m.name, query));
  const filteredAgents = availableAgents.filter((a) => a.name.toLowerCase().includes(query) || matchesPinyin(a.name, query));

  const canSubmit = !!target && !submitting;

  const handleSubmit = async () => {
    if (!target) return;
    setSubmitting(true);
    try {
      await onSubmit({ type: target.type, id: target.id, role });
      onClose();
    } catch {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose(); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t(($) => $.add_member_dialog.title)}</DialogTitle>
          <DialogDescription>{t(($) => $.add_member_dialog.description)}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4 min-w-0">
          <div>
            <Label className="text-xs text-muted-foreground">{t(($) => $.add_member_dialog.label_member)}</Label>
            <Popover open={pickerOpen} onOpenChange={(v) => { setPickerOpen(v); if (!v) setPickerFilter(""); }}>
              <PopoverTrigger className="flex w-full min-w-0 items-center gap-3 rounded-lg border border-border bg-background px-3 py-2.5 mt-1 text-left text-sm transition-colors hover:bg-muted">
                {target ? (
                  <ActorAvatar actorType={target.type} actorId={target.id} size={20} />
                ) : (
                  <UserPlus className="h-4 w-4 shrink-0 text-muted-foreground" />
                )}
                <div className="min-w-0 flex-1">
                  <div className="truncate font-medium">
                    {target?.name ?? "Select a member or agent"}
                  </div>
                  {target && (
                    <div className="truncate text-xs text-muted-foreground capitalize">{target.type}</div>
                  )}
                </div>
                <ChevronDown className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${pickerOpen ? "rotate-180" : ""}`} />
              </PopoverTrigger>
              <PopoverContent align="start" className="w-[var(--anchor-width)] p-0">
                <div className="px-2 py-1.5 border-b">
                  <input
                    autoFocus
                    type="text"
                    value={pickerFilter}
                    onChange={(e) => setPickerFilter(e.target.value)}
                    placeholder="Search members or agents..."
                    className="w-full bg-transparent text-sm placeholder:text-muted-foreground outline-none"
                  />
                </div>
                <div className="p-1 max-h-72 overflow-y-auto">
                  {filteredMembers.length > 0 && (
                    <PickerSection label="Members">
                      {filteredMembers.map((m) => (
                        <PickerItem
                          key={m.user_id}
                          selected={target?.type === "member" && target.id === m.user_id}
                          onClick={() => {
                            setTarget({ type: "member", id: m.user_id, name: m.name });
                            setPickerOpen(false);
                            setPickerFilter("");
                          }}
                        >
                          <ActorAvatar actorType="member" actorId={m.user_id} size={18} />
                          <span>{m.name}</span>
                        </PickerItem>
                      ))}
                    </PickerSection>
                  )}
                  {filteredAgents.length > 0 && (
                    <PickerSection label="Agents">
                      {filteredAgents.map((a) => (
                        <PickerItem
                          key={a.id}
                          selected={target?.type === "agent" && target.id === a.id}
                          onClick={() => {
                            setTarget({ type: "agent", id: a.id, name: a.name });
                            setPickerOpen(false);
                            setPickerFilter("");
                          }}
                        >
                          <ActorAvatar actorType="agent" actorId={a.id} size={18} showStatusDot />
                          <span>{a.name}</span>
                        </PickerItem>
                      ))}
                    </PickerSection>
                  )}
                  {filteredMembers.length === 0 && filteredAgents.length === 0 && <PickerEmpty />}
                </div>
              </PopoverContent>
            </Popover>
          </div>

          <div>
            <Label className="text-xs text-muted-foreground">
              {t(($) => $.add_member_dialog.label_role)}{" "}
              <span className="text-muted-foreground/60">{t(($) => $.add_member_dialog.label_optional)}</span>
            </Label>
            <Input
              type="text"
              value={role}
              onChange={(e) => setRole(e.target.value)}
              placeholder="e.g. Reviewer, Frontend Lead"
              className="mt-1"
              onKeyDown={(e) => {
                if (isImeComposing(e)) return;
                if (e.key === "Enter" && canSubmit) void handleSubmit();
              }}
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>{t(($) => $.add_member_dialog.cancel)}</Button>
          <Button onClick={() => void handleSubmit()} disabled={!canSubmit}>
            {submitting ? <Loader2 className="size-3.5 animate-spin" /> : "Add"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// Inline click-to-edit role line. Renders the current role as muted text;
// click (or click the placeholder when empty) to swap in an input that
// commits on blur / Enter and cancels on Escape. Avoids opening a modal
// for what is usually a one-word change.
function RoleEditor({ value, onSave }: { value: string; onSave: (next: string) => Promise<void> }) {
  const { t } = useT("squads");
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(value);
  const [saving, setSaving] = useState(false);

  useEffect(() => { if (!editing) setDraft(value); }, [value, editing]);

  const commit = async () => {
    const next = draft.trim();
    if (next === value.trim()) { setEditing(false); return; }
    setSaving(true);
    try {
      await onSave(next);
      setEditing(false);
    } catch {
      // toast handled by mutation
    } finally {
      setSaving(false);
    }
  };

  if (editing) {
    return (
      <Input
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(e) => {
          if (isImeComposing(e)) return;
          if (e.key === "Enter") void commit();
          else if (e.key === "Escape") { setDraft(value); setEditing(false); }
        }}
        disabled={saving}
        placeholder="Role (e.g. Reviewer)"
        className="h-6 mt-0.5 text-xs px-1.5"
      />
    );
  }

  return (
    <button
      type="button"
      onClick={() => setEditing(true)}
      className="text-xs text-muted-foreground mt-0.5 text-left hover:text-foreground transition-colors"
    >
      {value || <span className="italic opacity-60">{t(($) => $.add_member_dialog.placeholder_role_inline)}</span>}
    </button>
  );
}

// ---------------------------------------------------------------------------
// SquadDetailInspector — left 320px column, mirrors AgentDetailInspector.
// Holds identity (avatar / name / description) + leader / member count /
// timestamps. All inline-editable.
// ---------------------------------------------------------------------------
function SquadDetailInspector({
  squad,
  memberCount,
  leaderName,
  creatorName,
  uploadingAvatar,
  onUploadAvatar,
  onRename,
  onUpdateDescription,
}: {
  squad: Squad;
  memberCount: number;
  leaderName: string;
  creatorName: string;
  uploadingAvatar: boolean;
  onUploadAvatar: (url: string) => Promise<unknown>;
  onRename: (next: string) => Promise<void>;
  onUpdateDescription: (next: string) => Promise<void>;
}) {
  const { t } = useT("squads");
  const timeAgo = useTimeAgo();
  const initials = squad.name
    .split(" ")
    .map((w) => w[0])
    .join("")
    .toUpperCase()
    .slice(0, 2);

  return (
    <aside className="flex w-full flex-col rounded-lg border bg-background md:h-full md:min-h-0 md:overflow-y-auto">
      {/* Identity */}
      <div className="flex flex-col gap-3 border-b px-5 pb-5 pt-5">
        <SquadAvatarEditor
          squad={squad}
          initials={initials}
          uploading={uploadingAvatar}
          onUpload={onUploadAvatar}
        />
        <div className="flex flex-col gap-1">
          <SquadNameEditor value={squad.name} onSave={onRename} />
          <SquadDescriptionEditor
            value={squad.description ?? ""}
            onSave={onUpdateDescription}
          />
        </div>
      </div>

      {/* Details — read-only */}
      <div className="border-b px-5 py-4">
        <div className="mb-1 -mx-2 px-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
          {t(($) => $.inspector.details_section)}
        </div>
        <div className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5">
          <InspectorRow label="Leader">
            <span className="flex min-w-0 items-center gap-1.5">
              <ActorAvatar actorType="agent" actorId={squad.leader_id} size={14} />
              <span className="truncate">{leaderName}</span>
            </span>
          </InspectorRow>
          <InspectorRow label="Members">
            <span className="text-muted-foreground tabular-nums">{memberCount}</span>
          </InspectorRow>
          <InspectorRow label="Created by">
            <span className="flex min-w-0 items-center gap-1.5">
              <ActorAvatar actorType="member" actorId={squad.creator_id} size={14} />
              <span className="truncate">{creatorName}</span>
            </span>
          </InspectorRow>
          <InspectorRow label="Created">
            <span className="text-muted-foreground">{timeAgo(squad.created_at)}</span>
          </InspectorRow>
          <InspectorRow label="Updated">
            <span className="text-muted-foreground">{timeAgo(squad.updated_at)}</span>
          </InspectorRow>
        </div>
      </div>
    </aside>
  );
}

function InspectorRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <div className="px-2 py-1 text-xs text-muted-foreground">{label}</div>
      <div className="min-w-0 px-2 py-1 text-xs">{children}</div>
    </>
  );
}

// Click-to-edit description editor for the inspector. Mirrors
// agent-detail-inspector's DescriptionEditor: opens a modal with a textarea
// (enough room for multi-paragraph descriptions); the inline trigger shows
// the current value (or a placeholder) with a hover-revealed Pencil.
function SquadDescriptionEditor({
  value,
  onSave,
}: {
  value: string;
  onSave: (next: string) => Promise<void>;
}) {
  const { t } = useT("squads");
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
          <span className="italic text-muted-foreground/50">{t(($) => $.description_dialog.placeholder_empty)}</span>
        )}
        <Pencil className="mt-0.5 h-3 w-3 shrink-0 text-muted-foreground/0 transition-colors group-hover:text-muted-foreground" />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg">
          {open && (
            <SquadDescriptionEditorBody
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

function SquadDescriptionEditorBody({
  initialValue,
  onSave,
  onClose,
}: {
  initialValue: string;
  onSave: (next: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useT("squads");
  const [draft, setDraft] = useState(initialValue);
  const [saving, setSaving] = useState(false);
  const dirty = draft !== initialValue;

  const commit = async () => {
    if (!dirty) { onClose(); return; }
    setSaving(true);
    try {
      await onSave(draft);
      onClose();
    } catch {
      // toast handled by parent's mutation
    } finally {
      setSaving(false);
    }
  };

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t(($) => $.description_dialog.title)}</DialogTitle>
      </DialogHeader>
      <textarea
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        placeholder="What is this squad responsible for?"
        rows={6}
        onKeyDown={(e) => {
          if (e.key === "Escape") { onClose(); return; }
          if (isImeComposing(e)) return;
          if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
            e.preventDefault();
            void commit();
          }
        }}
        className="w-full resize-none rounded-md border bg-transparent px-3 py-2 text-sm outline-none focus-visible:border-input"
      />
      <DialogFooter>
        <Button variant="ghost" size="sm" onClick={onClose} disabled={saving}>{t(($) => $.description_dialog.cancel)}</Button>
        <Button size="sm" onClick={() => void commit()} disabled={saving || !dirty}>
          {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : "Save"}
        </Button>
      </DialogFooter>
    </>
  );
}

// ---------------------------------------------------------------------------
// SquadOverviewPane — right column with two tabs (Members | Instructions).
// Mirrors AgentOverviewPane: dirty-guard via AlertDialog when switching tabs
// with unsaved Instructions.
// ---------------------------------------------------------------------------
type SquadDetailTab = "visual" | "members" | "instructions";

const squadDetailTabs: { id: SquadDetailTab; icon: typeof FileText }[] = [
  { id: "visual", icon: Network },
  { id: "members", icon: Users },
  { id: "instructions", icon: FileText },
];

function squadDetailTabLabel(tab: SquadDetailTab, t: any) {
  if (tab === "visual") return t(($: any) => $.tabs.visual);
  if (tab === "members") return t(($: any) => $.tabs.members);
  return t(($: any) => $.tabs.instructions);
}

function SquadOverviewPane({
  squad,
  members,
  agents,
  memberStatusById,
  isLeader,
  isArchived,
  getEntityName,
  onAddMemberClick,
  onCreateAgentClick,
  onSetLeader,
  onRemoveMember,
  onUpdateRole,
  onSaveInstructions,
  setLeaderPending,
  canManage,
}: {
  squad: Squad;
  members: SquadMember[];
  agents: Agent[];
  memberStatusById: Map<string, SquadMemberStatus>;
  isLeader: (m: SquadMember) => boolean;
  isArchived: (m: SquadMember) => boolean;
  getEntityName: (type: string, id: string) => string;
  onAddMemberClick: () => void;
  // Optional — only passed when the current user can manage the squad
  // (workspace owner/admin). Hidden otherwise so plain members don't
  // see a button they can't action.
  onCreateAgentClick?: () => void;
  onSetLeader: (agentId: string) => void;
  onRemoveMember: (m: SquadMember) => void;
  onUpdateRole: (m: SquadMember, role: string) => Promise<void>;
  onSaveInstructions: (next: string) => Promise<void>;
  setLeaderPending: boolean;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const [activeTab, setActiveTab] = useState<SquadDetailTab>("visual");
  const [activeDirty, setActiveDirty] = useState(false);
  const [pendingTab, setPendingTab] = useState<SquadDetailTab | null>(null);

  const requestTabChange = (next: SquadDetailTab) => {
    if (next === activeTab) return;
    if (activeDirty) { setPendingTab(next); return; }
    setActiveTab(next);
  };

  const commitTabChange = () => {
    if (pendingTab) {
      setActiveTab(pendingTab);
      setActiveDirty(false);
      setPendingTab(null);
    }
  };

  return (
    <div className="flex min-h-[60vh] flex-col overflow-hidden rounded-lg border bg-background md:h-full md:min-h-0">
      <div className="flex shrink-0 items-center gap-0 overflow-x-auto border-b px-2 md:px-4">
        {squadDetailTabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => requestTabChange(tab.id)}
            className={`flex shrink-0 items-center gap-1.5 whitespace-nowrap border-b-2 px-3 py-2.5 text-xs font-medium transition-colors ${
              activeTab === tab.id
                ? "border-foreground text-foreground"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            <tab.icon className="h-3.5 w-3.5" />
            {squadDetailTabLabel(tab.id, t)}
          </button>
        ))}
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto">
        {activeTab === "visual" && (
          <div className="flex h-full flex-col p-4 md:p-6">
            <SquadVisualTab
              squad={squad}
              members={members}
              agents={agents}
              memberStatusById={memberStatusById}
              isLeader={isLeader}
              getEntityName={getEntityName}
              canManage={canManage}
            />
          </div>
        )}
        {activeTab === "members" && (
          <div className="flex h-full flex-col p-4 md:p-6">
            <SquadMembersTab
              members={members}
              memberStatusById={memberStatusById}
              isLeader={isLeader}
              isArchived={isArchived}
              getEntityName={getEntityName}
              onAddMemberClick={onAddMemberClick}
              onCreateAgentClick={onCreateAgentClick}
              onSetLeader={onSetLeader}
              onRemoveMember={onRemoveMember}
              onUpdateRole={onUpdateRole}
              setLeaderPending={setLeaderPending}
            />
          </div>
        )}
        {activeTab === "instructions" && (
          <div className="flex h-full flex-col p-4 md:p-6">
            <SquadInstructionsTab
              squad={squad}
              onSave={onSaveInstructions}
              onDirtyChange={setActiveDirty}
            />
          </div>
        )}
      </div>

      {pendingTab !== null && (
        <AlertDialog open onOpenChange={(v) => { if (!v) setPendingTab(null); }}>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>{t(($) => $.discard_changes_dialog.title)}</AlertDialogTitle>
              <AlertDialogDescription>
                {t(($) => $.discard_changes_dialog.description)}
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>{t(($) => $.discard_changes_dialog.keep_editing)}</AlertDialogCancel>
              <AlertDialogAction variant="destructive" onClick={commitTabChange}>
                {t(($) => $.discard_changes_dialog.discard_button)}
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      )}
    </div>
  );
}

// Visual config for the five squad member status buckets. Mirrors
// availabilityConfig + workloadConfig in packages/views/agents/presence.ts —
// same semantic tokens so a status dot here matches the agent page's dot.
// Unknown / null statuses (human members, server-side enum drift) render as
// a neutral muted pill; this is the "downgrade, don't crash" defense from
// CLAUDE.md > API Response Compatibility.
const SQUAD_STATUS_DOT_CLASS: Record<SquadMemberStatusValue, string> = {
  working: "bg-success",
  idle: "bg-muted-foreground/40",
  offline: "bg-muted-foreground/40",
  unstable: "bg-warning",
  archived: "bg-muted-foreground/40",
};

// Members tab body — re-uses the existing list/role editing patterns.
function squadStatusLabel(statusValue: SquadMemberStatusValue | null, t: any) {
  if (statusValue === "working") return t(($: any) => $.members_tab.status_working);
  if (statusValue === "idle") return t(($: any) => $.members_tab.status_idle);
  if (statusValue === "offline") return t(($: any) => $.members_tab.status_offline);
  if (statusValue === "unstable") return t(($: any) => $.members_tab.status_unstable);
  if (statusValue === "archived") return t(($: any) => $.members_tab.status_archived);
  return null;
}

function SquadVisualTab({
  squad,
  members,
  agents,
  memberStatusById,
  isLeader,
  getEntityName,
  canManage,
}: {
  squad: Squad;
  members: SquadMember[];
  agents: Agent[];
  memberStatusById: Map<string, SquadMemberStatus>;
  isLeader: (m: SquadMember) => boolean;
  getEntityName: (type: string, id: string) => string;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const p = useWorkspacePaths();
  const wsId = useWorkspaceId();
  const queryClient = useQueryClient();
  const [viewMode, setViewMode] = useState<"list" | "canvas">("list");
  const [stageEditor, setStageEditor] = useState<WorkflowStageDefinition | "new" | null>(null);
  const [stageToDelete, setStageToDelete] = useState<WorkflowStageDefinition | null>(null);
  const [generateDialogOpen, setGenerateDialogOpen] = useState(false);
  const [isGenerating, setIsGenerating] = useState(false);
  const assignmentQueryKey = [...workspaceKeys.squads(wsId), squad.id, "workflow-assignments"];
  const { data: workflowAssignmentData = {
    stages: [],
    assignments: {},
    assignment_sources: {},
    is_default_order: true,
    generation_source: "legacy_default",
    generated_profile: null,
    canvas_layout: { stages: {}, agents: {} },
  } } = useQuery<SquadWorkflowAssignmentsResponse>({
    queryKey: assignmentQueryKey,
    queryFn: () => api.getSquadWorkflowAssignments(squad.id),
    enabled: !!wsId && !!squad.id,
  });
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
  );
  const leaderMember =
    members.find((m) => m.member_type === "agent" && m.member_id === squad.leader_id) ??
    ({
      id: `leader:${squad.leader_id}`,
      squad_id: squad.id,
      member_type: "agent",
      member_id: squad.leader_id,
      role: "Leader",
      created_at: squad.created_at,
    } satisfies SquadMember);
  const workerMembers = members.filter((m) => !isLeader(m));
  const agentMembers = workerMembers.filter((m) => m.member_type === "agent");
  const humanMembers = workerMembers.filter((m) => m.member_type === "member");
  const persistedStages = workflowAssignmentData.stages.length > 0
    ? workflowAssignmentData.stages
    : WORKFLOW_STAGE_DEFINITIONS.map((stage, position) => ({
        id: stage.id,
        name: null,
        description: null,
        position,
        keywords: stage.keywords,
      } satisfies SquadWorkflowStage));
  const stageDefinitions = [...persistedStages]
    .sort((left, right) => left.position - right.position)
    .map((persistedStage) => {
      const builtIn = WORKFLOW_STAGE_DEFINITIONS.find((stage) => stage.id === persistedStage.id);
      return {
        id: persistedStage.id,
        title: persistedStage.name || workflowStageTitle(persistedStage.id, t),
        description: persistedStage.description ?? workflowStageDescription(persistedStage.id, t),
        icon: builtIn?.icon ?? workflowStageIcon(persistedStage.id),
        keywords: persistedStage.keywords.length > 0 ? persistedStage.keywords : (builtIn?.keywords ?? []),
      } satisfies WorkflowStageDefinition;
    });
  const inferredWorkflow = inferSquadWorkflow(agentMembers, agents, t, stageDefinitions);
  const inferredPlacements = inferredWorkflow.flatMap((stage) =>
    stage.placements.map((placement) => ({ ...placement, inferredStageId: stage.id })),
  );
  const workflow = stageDefinitions.map((stage) => ({
    ...stage,
    placements: inferredPlacements
      .filter((placement) =>
        (workflowAssignmentData.assignments[placement.member.member_id] ?? placement.inferredStageId) === stage.id,
      )
      .map((placement) => ({
        ...placement,
        reason: workflowAssignmentData.assignment_sources[placement.member.member_id] === "manual"
          ? t(($: any) => $.visual_tab.manual_reason)
          : workflowAssignmentData.assignment_sources[placement.member.member_id] === "generated"
            ? t(($: any) => $.visual_tab.generated_reason)
            : placement.reason,
      })),
  }));
  const canvasLayout = workflowAssignmentData.canvas_layout ?? { stages: {}, agents: {} };

  const moveAgentToStage = async (agentID: string, stageID: string) => {
    if (!canManage) return false;
    const currentStage = workflow.find((stage) =>
      stage.placements.some((placement) => placement.member.member_id === agentID),
    )?.id;
    if (!stageDefinitions.some((stage) => stage.id === stageID) || currentStage === stageID) return true;

    const previous = workflowAssignmentData;
    const next: SquadWorkflowAssignmentsResponse = {
      ...previous,
      assignments: { ...previous.assignments, [agentID]: stageID },
      assignment_sources: { ...previous.assignment_sources, [agentID]: "manual" },
    };
    queryClient.setQueryData(assignmentQueryKey, next);
    try {
      await api.setSquadWorkflowAssignment(squad.id, agentID, stageID);
      toast.success(t(($: any) => $.visual_tab.move_success));
      return true;
    } catch (error) {
      queryClient.setQueryData(assignmentQueryKey, previous);
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.move_failed));
      return false;
    }
  };

  const handleDragEnd = async ({ active, over }: DragEndEvent) => {
    if (!over) return;
    await moveAgentToStage(
      String(active.id).replace("agent:", ""),
      String(over.id).replace("stage:", ""),
    );
  };

  const saveCanvasLayout = async (layout: SquadWorkflowCanvasLayout) => {
    const previousLayout = queryClient.getQueryData<SquadWorkflowAssignmentsResponse>(assignmentQueryKey)?.canvas_layout
      ?? { stages: {}, agents: {} };
    queryClient.setQueryData<SquadWorkflowAssignmentsResponse>(assignmentQueryKey, (current) =>
      current ? { ...current, canvas_layout: layout } : current,
    );
    try {
      await api.setSquadWorkflowCanvasLayout(squad.id, layout);
    } catch (error) {
      queryClient.setQueryData<SquadWorkflowAssignmentsResponse>(assignmentQueryKey, (current) =>
        current ? { ...current, canvas_layout: previousLayout } : current,
      );
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.move_failed));
    }
  };

  const generateWorkflow = async () => {
    if (isGenerating) return;
    setIsGenerating(true);
    try {
      await api.generateSquadWorkflow(squad.id);
      await queryClient.invalidateQueries({ queryKey: assignmentQueryKey });
      setGenerateDialogOpen(false);
      toast.success(t(($: any) => $.visual_tab.generate_success));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.generate_failed));
    } finally {
      setIsGenerating(false);
    }
  };

  const refreshWorkflow = () => queryClient.invalidateQueries({ queryKey: assignmentQueryKey });

  const moveStage = async (stageID: string, direction: -1 | 1) => {
    const currentIndex = stageDefinitions.findIndex((stage) => stage.id === stageID);
    const targetIndex = currentIndex + direction;
    if (currentIndex < 0 || targetIndex < 0 || targetIndex >= stageDefinitions.length) return;
    const nextStageIDs = stageDefinitions.map((stage) => stage.id);
    [nextStageIDs[currentIndex], nextStageIDs[targetIndex]] = [nextStageIDs[targetIndex]!, nextStageIDs[currentIndex]!];
    const previous = workflowAssignmentData;
    queryClient.setQueryData<SquadWorkflowAssignmentsResponse>(assignmentQueryKey, {
      ...previous,
      is_default_order: false,
      stages: nextStageIDs.map((id, position) => ({
        ...persistedStages.find((stage) => stage.id === id)!,
        position,
      })),
    });
    try {
      await api.reorderSquadWorkflowStages(squad.id, nextStageIDs);
      await queryClient.invalidateQueries({ queryKey: assignmentQueryKey });
      toast.success(t(($: any) => $.visual_tab.stage_order_success));
    } catch (error) {
      queryClient.setQueryData(assignmentQueryKey, previous);
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.stage_order_failed));
    }
  };

  const deleteStage = async () => {
    if (!stageToDelete) return;
    try {
      await api.deleteSquadWorkflowStage(squad.id, stageToDelete.id);
      setStageToDelete(null);
      await refreshWorkflow();
      toast.success(t(($: any) => $.visual_tab.stage_delete_success));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.stage_delete_failed));
    }
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex min-h-[420px] flex-1 flex-col gap-4 bg-muted/20 p-3 md:p-4">
        {viewMode === "list" && <div className="flex flex-col items-center">
          <div className="mb-1.5 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            {t(($: any) => $.visual_tab.entry)}
          </div>
          <VisualMemberNode
            member={leaderMember}
            title={getEntityName("agent", squad.leader_id)}
            subtitle={leaderMember.role || "Leader"}
            status={memberStatusById.get(squad.leader_id)}
            isLeader
            href={p.agentDetail(squad.leader_id)}
          />
          {workflow.length > 0 && (
            <div className="flex h-10 flex-col items-center justify-center text-foreground/60">
              <div className="h-4 border-l-2 border-foreground/20" />
              <ArrowDown className="size-5" strokeWidth={2.25} />
            </div>
          )}
        </div>}

        <div className="min-w-0 flex-1">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-2 border-b pb-2.5">
            <div>
              <h3 className="text-sm font-semibold text-foreground">{t(($: any) => $.visual_tab.workflow_title)}</h3>
              <p className="mt-1 text-xs text-muted-foreground">
                {t(($: any) => $.visual_tab.workflow_description)}
              </p>
            </div>
            <div className="flex flex-wrap items-center justify-end gap-2">
              <div className="inline-flex h-8 items-center rounded-md border bg-background p-0.5" role="group" aria-label={t(($: any) => $.visual_tab.view_mode)}>
                <Button
                  data-workflow-view="list"
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-pressed={viewMode === "list"}
                  className={`h-7 gap-1.5 px-2.5 ${viewMode === "list" ? "bg-muted text-foreground shadow-sm hover:bg-muted" : "text-muted-foreground"}`}
                  onClick={() => setViewMode("list")}
                >
                  <LayoutList className="size-3.5" />
                  {t(($: any) => $.visual_tab.list_view)}
                </Button>
                <Button
                  data-workflow-view="canvas"
                  type="button"
                  size="sm"
                  variant="ghost"
                  aria-pressed={viewMode === "canvas"}
                  className={`h-7 gap-1.5 px-2.5 ${viewMode === "canvas" ? "bg-muted text-foreground shadow-sm hover:bg-muted" : "text-muted-foreground"}`}
                  onClick={() => setViewMode("canvas")}
                >
                  <WorkflowIcon className="size-3.5" />
                  {t(($: any) => $.visual_tab.canvas_view)}
                </Button>
              </div>
              {canManage && (
                <>
                  <Button data-workflow-generate size="sm" variant="outline" onClick={() => setGenerateDialogOpen(true)}>
                    <Sparkles className="mr-1.5 size-3.5" />
                    {t(($: any) => $.visual_tab.generate)}
                  </Button>
                  <Button data-workflow-add-stage size="sm" variant="outline" onClick={() => setStageEditor("new")}>
                    <Plus className="mr-1.5 size-3.5" />
                    {t(($: any) => $.visual_tab.add_stage)}
                  </Button>
                </>
              )}
            </div>
          </div>

          {viewMode === "canvas" ? (
            <SquadWorkflowCanvas
              leader={{
                member: leaderMember,
                title: getEntityName("agent", squad.leader_id),
                subtitle: leaderMember.role || "Leader",
                status: memberStatusById.get(squad.leader_id),
              }}
              stages={workflow}
              memberStatusById={memberStatusById}
              canManage={canManage}
              layout={canvasLayout}
              onLayoutChange={saveCanvasLayout}
              onMoveAgent={moveAgentToStage}
            />
          ) : (
            <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={(event) => void handleDragEnd(event)}>
              <div className="space-y-0">
                {workflow.map((stage, index) => (
                  <WorkflowStageRow
                    key={stage.id}
                    stage={stage}
                    index={index}
                    nextStage={workflow[index + 1]}
                    memberStatusById={memberStatusById}
                    canManage={canManage}
                    canDelete={workflow.length > 1}
                    canMoveUp={index > 0}
                    canMoveDown={index < workflow.length - 1}
                    onEdit={() => setStageEditor(stage)}
                    onDelete={() => setStageToDelete(stage)}
                    onMoveUp={() => void moveStage(stage.id, -1)}
                    onMoveDown={() => void moveStage(stage.id, 1)}
                  />
                ))}
              </div>
            </DndContext>
          )}
        </div>

        {humanMembers.length > 0 && (
          <div className="border-t pt-5">
            <VisualGroup title={t(($: any) => $.visual_tab.human_members)} count={humanMembers.length}>
              <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                {humanMembers.map((member) => (
                  <VisualMemberNode
                    key={member.id}
                    member={member}
                    title={getEntityName(member.member_type, member.member_id)}
                    subtitle={member.role || "Member"}
                    status={memberStatusById.get(member.member_id)}
                  />
                ))}
              </div>
            </VisualGroup>
          </div>
        )}
      </div>

      {stageEditor && (
        <WorkflowStageEditorDialog
          stage={stageEditor === "new" ? null : stageEditor}
          onClose={() => setStageEditor(null)}
          onSaved={async () => {
            setStageEditor(null);
            await refreshWorkflow();
          }}
          squadId={squad.id}
        />
      )}

      <AlertDialog open={!!stageToDelete} onOpenChange={(open) => { if (!open) setStageToDelete(null); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($: any) => $.visual_tab.delete_stage_title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($: any) => $.visual_tab.delete_stage_description, { name: stageToDelete?.title ?? "" })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t(($: any) => $.visual_tab.cancel)}</AlertDialogCancel>
            <AlertDialogAction data-workflow-stage-delete-confirm variant="destructive" onClick={() => void deleteStage()}>
              {t(($: any) => $.visual_tab.delete_stage_confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={generateDialogOpen} onOpenChange={(open) => { if (!isGenerating) setGenerateDialogOpen(open); }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($: any) => $.visual_tab.generate_title)}</AlertDialogTitle>
            <AlertDialogDescription>{t(($: any) => $.visual_tab.generate_description)}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isGenerating}>{t(($: any) => $.visual_tab.cancel)}</AlertDialogCancel>
            <AlertDialogAction data-workflow-generate-confirm onClick={() => void generateWorkflow()} disabled={isGenerating}>
              {isGenerating && <Loader2 className="mr-1.5 size-3.5 animate-spin" />}
              {t(($: any) => $.visual_tab.generate_confirm)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function WorkflowStageEditorDialog({
  squadId,
  stage,
  onClose,
  onSaved,
}: {
  squadId: string;
  stage: WorkflowStageDefinition | null;
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const { t } = useT("squads");
  const [name, setName] = useState(stage?.title ?? "");
  const [description, setDescription] = useState(stage?.description ?? "");
  const [isSaving, setIsSaving] = useState(false);
  const canSave = name.trim().length > 0 && name.trim().length <= 80 && description.trim().length <= 500;

  const save = async () => {
    if (!canSave || isSaving) return;
    setIsSaving(true);
    try {
      const data = { name: name.trim(), description: description.trim() };
      if (stage) {
        await api.updateSquadWorkflowStage(squadId, stage.id, data);
      } else {
        await api.createSquadWorkflowStage(squadId, data);
      }
      await onSaved();
      toast.success(t(($: any) => stage ? $.visual_tab.stage_update_success : $.visual_tab.stage_create_success));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($: any) => $.visual_tab.stage_save_failed));
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => { if (!open && !isSaving) onClose(); }}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t(($: any) => stage ? $.visual_tab.edit_stage_title : $.visual_tab.add_stage_title)}
          </DialogTitle>
          <DialogDescription>{t(($: any) => $.visual_tab.stage_dialog_description)}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="workflow-stage-name">{t(($: any) => $.visual_tab.stage_name)}</Label>
            <Input
              id="workflow-stage-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              maxLength={80}
              autoFocus
              placeholder={t(($: any) => $.visual_tab.stage_name_placeholder)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="workflow-stage-description">{t(($: any) => $.visual_tab.stage_description)}</Label>
            <Textarea
              id="workflow-stage-description"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
              maxLength={500}
              className="min-h-24 resize-none"
              placeholder={t(($: any) => $.visual_tab.stage_description_placeholder)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={isSaving}>
            {t(($: any) => $.visual_tab.cancel)}
          </Button>
          <Button data-workflow-stage-save onClick={() => void save()} disabled={!canSave || isSaving}>
            {isSaving && <Loader2 className="mr-1.5 size-3.5 animate-spin" />}
            {t(($: any) => $.visual_tab.save_stage)}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type WorkflowStageDefinition = {
  id: string;
  title: string;
  description: string;
  icon: LucideIcon;
  keywords: string[];
};

type WorkflowPlacement = {
  member: SquadMember;
  agent: Agent | undefined;
  reason: string;
};

type InferredWorkflowStage = WorkflowStageDefinition & {
  placements: WorkflowPlacement[];
};

function workflowStageIcon(stageId: string): LucideIcon {
  const semanticID = stageId.toLocaleLowerCase();
  if (/(requirement|goal|intake|discover|lead|strategy)/.test(semanticID)) return ClipboardCheck;
  if (/(knowledge|context)/.test(semanticID)) return BookOpen;
  if (/(research|evidence|data)/.test(semanticID)) return Search;
  if (/(design|planning|analysis|model|proposal)/.test(semanticID)) return Lightbulb;
  if (/(implementation|feature|policy|creative|acquisition|execution|resolution|domain|growth)/.test(semanticID)) return Code2;
  if (/(review|validation|compliance|quality)/.test(semanticID)) return ShieldCheck;
  if (/test/.test(semanticID)) return TestTube2;
  if (/(delivery|monitoring|success|closing)/.test(semanticID)) return Rocket;
  return CircleHelp;
}

const WORKFLOW_STAGE_DEFINITIONS: WorkflowStageDefinition[] = [
  {
    id: "requirements",
    title: "需求理解",
    description: "澄清目标、范围、约束与验收标准",
    icon: ClipboardCheck,
    keywords: ["需求分析", "需求理解", "需求", "prd", "requirement", "产品分析", "业务分析", "验收标准"],
  },
  {
    id: "knowledge",
    title: "规范与知识",
    description: "补充规范、领域知识和已有上下文",
    icon: BookOpen,
    keywords: ["规范知识", "知识库", "知识", "规范", "context", "上下文", "文档检索", "knowledge"],
  },
  {
    id: "research",
    title: "系统调研",
    description: "查看现状、代码和依赖，定位关键影响面",
    icon: Search,
    keywords: ["系统调研", "代码调研", "技术调研", "调研", "排查", "诊断", "research", "investigation", "analysis"],
  },
  {
    id: "design",
    title: "方案设计",
    description: "形成技术方案、架构决策与任务拆分",
    icon: Lightbulb,
    keywords: ["方案设计", "技术设计", "架构设计", "设计", "方案", "architecture", "design", "任务拆分"],
  },
  {
    id: "implementation",
    title: "代码实现",
    description: "按照方案完成编码、配置和必要验证",
    icon: Code2,
    keywords: ["代码实现", "编码实现", "开发实现", "写代码", "开发", "编码", "implementation", "coding", "developer"],
  },
  {
    id: "review",
    title: "代码评审",
    description: "检查正确性、完整性、风险和可维护性",
    icon: ShieldCheck,
    keywords: ["代码评审", "代码审核", "质量评审", "review", "审查", "评审", "code quality"],
  },
  {
    id: "test",
    title: "测试与修复",
    description: "验证功能、处理缺陷并完成回归",
    icon: TestTube2,
    keywords: ["测试修复", "缺陷修复", "测试", "修复", "回归", "qa", "test", "bugfix", "bug fix"],
  },
  {
    id: "delivery",
    title: "提测与交付",
    description: "整理交付物、提测、发布并同步结果",
    icon: Rocket,
    keywords: ["测试提测", "交付动作", "发布上线", "提测", "交付", "发布", "上线", "submit", "delivery", "release", "deploy"],
  },
  {
    id: "support",
    title: "协作支持",
    description: "承接暂未归入标准研发阶段的专项职责",
    icon: CircleHelp,
    keywords: [],
  },
];

function workflowStageTitle(stageId: string, t: any) {
  if (stageId === "requirements") return t(($: any) => $.visual_tab.stages.requirements.title);
  if (stageId === "knowledge") return t(($: any) => $.visual_tab.stages.knowledge.title);
  if (stageId === "research") return t(($: any) => $.visual_tab.stages.research.title);
  if (stageId === "design") return t(($: any) => $.visual_tab.stages.design.title);
  if (stageId === "implementation") return t(($: any) => $.visual_tab.stages.implementation.title);
  if (stageId === "review") return t(($: any) => $.visual_tab.stages.review.title);
  if (stageId === "test") return t(($: any) => $.visual_tab.stages.test.title);
  if (stageId === "delivery") return t(($: any) => $.visual_tab.stages.delivery.title);
  return t(($: any) => $.visual_tab.stages.support.title);
}

function workflowStageDescription(stageId: string, t: any) {
  if (stageId === "requirements") return t(($: any) => $.visual_tab.stages.requirements.description);
  if (stageId === "knowledge") return t(($: any) => $.visual_tab.stages.knowledge.description);
  if (stageId === "research") return t(($: any) => $.visual_tab.stages.research.description);
  if (stageId === "design") return t(($: any) => $.visual_tab.stages.design.description);
  if (stageId === "implementation") return t(($: any) => $.visual_tab.stages.implementation.description);
  if (stageId === "review") return t(($: any) => $.visual_tab.stages.review.description);
  if (stageId === "test") return t(($: any) => $.visual_tab.stages.test.description);
  if (stageId === "delivery") return t(($: any) => $.visual_tab.stages.delivery.description);
  return t(($: any) => $.visual_tab.stages.support.description);
}

function inferSquadWorkflow(
  members: SquadMember[],
  agents: Agent[],
  t: any,
  stageDefinitions: WorkflowStageDefinition[],
): InferredWorkflowStage[] {
  const placementsByStage = new Map<string, WorkflowPlacement[]>();
  const fallbackStage = stageDefinitions[stageDefinitions.length - 1];
  if (!fallbackStage) return [];

  for (const member of members) {
    const agent = agents.find((candidate) => candidate.id === member.member_id);
    const sources = [
      { label: t(($: any) => $.visual_tab.source_name), value: agent?.name ?? "", weight: 8 },
      { label: t(($: any) => $.visual_tab.source_role), value: member.role ?? "", weight: 6 },
      { label: t(($: any) => $.visual_tab.source_description), value: agent?.description ?? "", weight: 3 },
      { label: t(($: any) => $.visual_tab.source_instructions), value: agent?.instructions?.slice(0, 4000) ?? "", weight: 1 },
    ];
    let bestStage: WorkflowStageDefinition = fallbackStage;
    let bestScore = 0;
    let bestReason = t(($: any) => $.visual_tab.fallback_reason);

    for (const stage of stageDefinitions) {
      for (const source of sources) {
        const normalizedValue = source.value.toLocaleLowerCase();
        for (const keyword of stage.keywords) {
          if (!normalizedValue.includes(keyword.toLocaleLowerCase())) continue;
          const score = source.weight * 100 + keyword.length;
          if (score > bestScore) {
            bestScore = score;
            bestStage = stage;
            bestReason = t(($: any) => $.visual_tab.match_reason, {
              source: source.label,
              keyword,
            });
          }
        }
      }
    }

    const placements = placementsByStage.get(bestStage.id) ?? [];
    placements.push({ member, agent, reason: bestReason });
    placementsByStage.set(bestStage.id, placements);
  }

  return stageDefinitions.flatMap((stage) => {
    const placements = placementsByStage.get(stage.id);
    return placements?.length ? [{ ...stage, placements }] : [];
  });
}

function WorkflowStageRow({
  stage,
  index,
  nextStage,
  memberStatusById,
  canManage,
  canDelete,
  canMoveUp,
  canMoveDown,
  onEdit,
  onDelete,
  onMoveUp,
  onMoveDown,
}: {
  stage: InferredWorkflowStage;
  index: number;
  nextStage: InferredWorkflowStage | undefined;
  memberStatusById: Map<string, SquadMemberStatus>;
  canManage: boolean;
  canDelete: boolean;
  canMoveUp: boolean;
  canMoveDown: boolean;
  onEdit: () => void;
  onDelete: () => void;
  onMoveUp: () => void;
  onMoveDown: () => void;
}) {
  const { t } = useT("squads");
  const StageIcon = stage.icon;
  const { setNodeRef, isOver } = useDroppable({ id: `stage:${stage.id}` });

  return (
    <div>
      <section
        ref={setNodeRef}
        data-workflow-stage={stage.id}
        className={`grid min-w-0 grid-cols-[40px_minmax(0,1fr)] gap-2.5 rounded-md border p-2.5 transition-colors md:p-3 ${
          isOver ? "border-primary bg-primary/5 ring-2 ring-primary/20" : "border-border bg-background/80"
        }`}
      >
        <div className="flex justify-center">
          <div className="mt-0.5 flex size-8 items-center justify-center rounded-md border border-border bg-muted/60 shadow-sm">
            <span className="font-mono text-xs font-semibold tabular-nums text-foreground/80">
              {String(index + 1).padStart(2, "0")}
            </span>
          </div>
        </div>

        <div className="min-w-0">
          <div className="mb-2 flex min-w-0 items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-2">
              <StageIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <h4 className="text-sm font-semibold text-foreground">{stage.title}</h4>
                  <span className="text-[11px] tabular-nums text-muted-foreground">
                    {t(($) => $.visual_tab.stage_agent_count, { count: stage.placements.length })}
                  </span>
                </div>
                <p className="mt-0.5 text-xs leading-5 text-muted-foreground">{stage.description}</p>
              </div>
            </div>
            {canManage && (
              <div className="flex shrink-0 items-center gap-0.5">
                <span className={`mr-2 hidden text-[11px] lg:inline ${isOver ? "font-medium text-primary" : "text-muted-foreground"}`}>
                  {isOver ? t(($: any) => $.visual_tab.drop_here) : t(($: any) => $.visual_tab.drag_hint)}
                </span>
                <Button data-workflow-stage-action="up" type="button" size="icon-sm" variant="ghost" disabled={!canMoveUp} onClick={onMoveUp} title={t(($: any) => $.visual_tab.move_stage_up)}>
                  <ChevronUp className="size-3.5" />
                </Button>
                <Button data-workflow-stage-action="down" type="button" size="icon-sm" variant="ghost" disabled={!canMoveDown} onClick={onMoveDown} title={t(($: any) => $.visual_tab.move_stage_down)}>
                  <ChevronDownIcon className="size-3.5" />
                </Button>
                <Button data-workflow-stage-action="edit" type="button" size="icon-sm" variant="ghost" onClick={onEdit} title={t(($: any) => $.visual_tab.edit_stage)}>
                  <Pencil className="size-3.5" />
                </Button>
                <Button data-workflow-stage-action="delete" type="button" size="icon-sm" variant="ghost" disabled={!canDelete} onClick={onDelete} title={t(($: any) => $.visual_tab.delete_stage)}>
                  <Trash2 className="size-3.5" />
                </Button>
              </div>
            )}
          </div>

          <div className="grid min-w-0 gap-2 xl:grid-cols-2">
            {stage.placements.map(({ member, agent, reason }) => (
              <DraggableWorkflowAgent
                key={member.id}
                member={member}
                agent={agent}
                reason={reason}
                status={memberStatusById.get(member.member_id)}
                canManage={canManage}
              />
            ))}
            {stage.placements.length === 0 && (
              <div className={`col-span-full flex h-12 items-center justify-center rounded-md border border-dashed text-xs ${
                isOver ? "border-primary text-primary" : "text-muted-foreground"
              }`}>
                {t(($: any) => $.visual_tab.empty_stage)}
              </div>
            )}
          </div>
        </div>
      </section>

      {nextStage && (
        <div
          className="relative h-11 overflow-hidden text-muted-foreground"
          data-workflow-handoff={`${stage.id}:${nextStage.id}`}
          aria-label={t(($: any) => $.visual_tab.handoff, {
            from: stage.title,
            to: nextStage.title,
          })}
        >
          <div className="absolute inset-y-0 left-[29px] border-l-2 border-foreground/25 md:left-[31px]" />
          <div className="absolute left-[18px] top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded-md border border-border bg-background shadow-sm md:left-[20px]">
            <ArrowDown className="size-4 text-foreground/70" strokeWidth={2.25} />
          </div>
          <div className="absolute inset-y-0 left-[58px] right-2 flex min-w-0 items-center md:left-[62px]">
            <span className="flex w-full min-w-0 items-center gap-2 text-xs">
              <Network className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="truncate font-medium text-foreground/75">{stage.title}</span>
              <ArrowRight className="size-4 shrink-0 text-foreground/70" strokeWidth={2.5} />
              <span className="truncate text-muted-foreground">{nextStage.title}</span>
            </span>
          </div>
        </div>
      )}
    </div>
  );
}

function DraggableWorkflowAgent({
  member,
  agent,
  reason,
  status,
  canManage,
}: {
  member: SquadMember;
  agent: Agent | undefined;
  reason: string;
  status: SquadMemberStatus | undefined;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const p = useWorkspacePaths();
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `agent:${member.member_id}`,
    disabled: !canManage,
  });

  return (
    <div
      ref={setNodeRef}
      data-workflow-agent={member.member_id}
      style={{ transform: CSS.Translate.toString(transform) }}
      className={`group relative z-10 flex min-w-0 items-start gap-2.5 rounded-md border bg-background p-2.5 shadow-sm transition-[border-color,box-shadow,opacity] ${
        isDragging ? "z-50 opacity-80 shadow-xl ring-2 ring-primary/30" : "hover:border-foreground/25"
      }`}
    >
      {canManage && (
        <button
          type="button"
          className="-ml-1 flex h-7 w-5 shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
          title={t(($: any) => $.visual_tab.drag_handle)}
          aria-label={t(($: any) => $.visual_tab.drag_handle)}
          {...listeners}
          {...attributes}
        >
          <GripVertical className="size-4" />
        </button>
      )}
      <ActorAvatar
        actorType="agent"
        actorId={member.member_id}
        size={28}
        showStatusDot
        enableHoverCard
        hoverCardVariant="live"
      />
      <AppLink href={p.agentDetail(member.member_id)} className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-sm font-medium text-foreground hover:underline">
            {agent?.name ?? member.member_id.slice(0, 8)}
          </span>
          <WorkflowStatus status={status} />
        </div>
        <p className="mt-0.5 line-clamp-2 text-xs leading-4 text-muted-foreground">
          {agent?.description || member.role || t(($: any) => $.visual_tab.no_description)}
        </p>
        <p className="mt-1 truncate text-[11px] text-muted-foreground/80">
          {t(($: any) => $.visual_tab.placement_reason, { reason })}
        </p>
      </AppLink>
      <ArrowUpRight className="mt-1 size-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
    </div>
  );
}

function WorkflowStatus({ status }: { status: SquadMemberStatus | undefined }) {
  const { t } = useT("squads");
  const value = status?.status ?? null;
  const label = squadStatusLabel(value, t);
  const dotClass =
    value && value in SQUAD_STATUS_DOT_CLASS
      ? SQUAD_STATUS_DOT_CLASS[value as keyof typeof SQUAD_STATUS_DOT_CLASS]
      : "bg-muted-foreground/30";

  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-[11px] text-muted-foreground">
      <span className={`size-1.5 rounded-full ${dotClass}`} />
      {label ?? t(($: any) => $.visual_tab.status_unknown)}
    </span>
  );
}

function VisualGroup({
  title,
  count,
  children,
}: {
  title: string;
  count: number;
  children: ReactNode;
}) {
  return (
    <section className="flex min-h-0 flex-col gap-3">
      <div className="flex items-center justify-between">
        <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{title}</h3>
        <span className="rounded-full bg-background px-2 py-0.5 text-xs text-muted-foreground">{count}</span>
      </div>
      <div className="min-h-0 flex-1">{children}</div>
    </section>
  );
}

function VisualMemberNode({
  member,
  title,
  subtitle,
  status,
  isLeader = false,
  href,
}: {
  member: SquadMember;
  title: string;
  subtitle: string;
  status: SquadMemberStatus | undefined;
  isLeader?: boolean;
  href?: string;
}) {
  const { t } = useT("squads");
  const statusValue = status?.status ?? null;
  const statusLabel = squadStatusLabel(statusValue, t);
  const dotClass =
    statusValue && statusValue in SQUAD_STATUS_DOT_CLASS
      ? SQUAD_STATUS_DOT_CLASS[statusValue as keyof typeof SQUAD_STATUS_DOT_CLASS]
      : "bg-muted-foreground/30";
  const activeIssue = status?.active_issues[0];
  const content = (
    <div
      className={`group flex min-h-[96px] min-w-0 items-start gap-3 rounded-lg border bg-background p-3 shadow-sm transition-colors hover:border-foreground/20 ${
        isLeader ? "border-amber-300/70 bg-amber-50/60 dark:bg-amber-950/10" : ""
      }`}
    >
      <ActorAvatar
        actorType={member.member_type}
        actorId={member.member_id}
        size={36}
        showStatusDot
        enableHoverCard={member.member_type === "agent"}
        hoverCardVariant="live"
      />
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 items-center gap-1.5">
          <span className="truncate text-sm font-semibold text-foreground">{title}</span>
          {isLeader && <Crown className="size-3.5 shrink-0 text-amber-600" />}
        </div>
        <div className="mt-0.5 truncate text-xs text-muted-foreground">{subtitle}</div>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
            <span className={`h-1.5 w-1.5 rounded-full ${dotClass}`} />
            {statusLabel ?? (member.member_type === "agent" ? "Unknown" : "Member")}
          </span>
          <span className="text-xs text-muted-foreground capitalize">{member.member_type}</span>
        </div>
        {activeIssue && (
          <div className="mt-2 min-w-0 truncate text-xs text-muted-foreground">
            <span className="font-mono text-[10px] uppercase">{activeIssue.identifier}</span>
            <span className="mx-1">/</span>
            {activeIssue.title}
          </div>
        )}
      </div>
      {href && <ArrowUpRight className="mt-1 size-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />}
    </div>
  );

  if (!href) return content;
  return (
    <AppLink href={href} className="block min-w-0">
      {content}
    </AppLink>
  );
}

function SquadMembersTab({
  members,
  memberStatusById,
  isLeader,
  isArchived,
  getEntityName,
  onAddMemberClick,
  onCreateAgentClick,
  onSetLeader,
  onRemoveMember,
  onUpdateRole,
  setLeaderPending,
}: {
  members: SquadMember[];
  memberStatusById: Map<string, SquadMemberStatus>;
  isLeader: (m: SquadMember) => boolean;
  isArchived: (m: SquadMember) => boolean;
  getEntityName: (type: string, id: string) => string;
  onAddMemberClick: () => void;
  // Hidden for non-admins — see SquadOverviewPane.
  onCreateAgentClick?: () => void;
  onSetLeader: (agentId: string) => void;
  onRemoveMember: (m: SquadMember) => void;
  onUpdateRole: (m: SquadMember, role: string) => Promise<void>;
  setLeaderPending: boolean;
}) {
  const { t } = useT("squads");
  const timeAgo = useTimeAgo();
  const p = useWorkspacePaths();
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-medium">{t(($) => $.members_tab.section_title)}</h3>
          <p className="text-xs text-muted-foreground mt-0.5">
            {t(($) => $.members_tab.section_count, { count: members.length })}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {onCreateAgentClick && (
            <Button size="sm" variant="outline" onClick={onCreateAgentClick}>
              <Plus className="size-3.5 mr-1.5" />
              {t(($) => $.members_tab.create_agent_button)}
            </Button>
          )}
          <Button size="sm" variant="outline" onClick={onAddMemberClick}>
            <Plus className="size-3.5 mr-1.5" />
            {t(($) => $.members_tab.add_member_button)}
          </Button>
        </div>
      </div>

      <div className="space-y-2">
        {members.map((m) => {
          const status = memberStatusById.get(m.member_id);
          const statusValue = status?.status ?? null;
          const dotClass =
            statusValue && statusValue in SQUAD_STATUS_DOT_CLASS
              ? SQUAD_STATUS_DOT_CLASS[statusValue as keyof typeof SQUAD_STATUS_DOT_CLASS]
              : null;
          const statusLabel = squadStatusLabel(statusValue, t);
          const activeIssues = status?.active_issues ?? [];
          const primaryIssue = activeIssues[0];
          const extraIssueCount = Math.max(0, activeIssues.length - 1);
          // Show last_active only when the agent isn't currently working —
          // a "working" pill already implies the agent is live, and a
          // "last active 2s ago" line next to it is just noise.
          const showLastActive =
            m.member_type === "agent" && statusValue && statusValue !== "working" && status?.last_active_at;
          return (
            <div key={m.id} className="group flex items-start gap-3 rounded-lg border p-3">
              <ActorAvatar
                actorType={m.member_type}
                actorId={m.member_id}
                size={32}
                showStatusDot
                enableHoverCard={m.member_type === "agent"}
                hoverCardVariant="live"
              />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className="text-sm font-medium">{getEntityName(m.member_type, m.member_id)}</span>
                  <span className="text-xs text-muted-foreground capitalize">{m.member_type}</span>
                  {isLeader(m) && (
                    <span className="inline-flex items-center gap-0.5 text-xs bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-400 px-1.5 py-0.5 rounded">
                      <Crown className="size-3" />
                      {t(($) => $.members_tab.leader_chip)}
                    </span>
                  )}
                  {m.member_type === "agent" && statusLabel && (
                    <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
                      <span className={`h-1.5 w-1.5 rounded-full ${dotClass ?? "bg-muted-foreground/40"}`} />
                      {statusLabel}
                    </span>
                  )}
                </div>
                <RoleEditor
                  value={m.role ?? ""}
                  onSave={async (next) => { await onUpdateRole(m, next); }}
                />
                {primaryIssue && (
                  <div className="mt-1 flex items-center gap-1 text-xs text-muted-foreground min-w-0">
                    <AppLink
                      href={p.issueDetail(primaryIssue.issue_id)}
                      className="inline-flex items-center gap-1 min-w-0 hover:text-foreground transition-colors"
                    >
                      <span className="font-mono text-[10px] uppercase shrink-0">{primaryIssue.identifier}</span>
                      <span className="truncate">{primaryIssue.title}</span>
                      {primaryIssue.issue_status === "blocked" && (
                        <span className="shrink-0 inline-flex items-center text-[10px] uppercase tracking-wide text-warning">
                          {t(($) => $.members_tab.issue_status_blocked)}
                        </span>
                      )}
                    </AppLink>
                    {extraIssueCount > 0 && (
                      <span className="shrink-0">
                        · {t(($) => $.members_tab.active_issue_more, { count: extraIssueCount })}
                      </span>
                    )}
                  </div>
                )}
                {showLastActive && (
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    {t(($) => $.members_tab.last_active_label, {
                      time: timeAgo(status!.last_active_at!),
                    })}
                  </div>
                )}
              </div>
            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity">
              {m.member_type === "agent" && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <AppLink
                        href={p.agentDetail(m.member_id)}
                        className="inline-flex items-center justify-center h-8 w-8 rounded-md text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
                        aria-label={t(($) => $.members_tab.view_agent_tooltip)}
                      >
                        <ArrowUpRight className="size-3.5" />
                      </AppLink>
                    }
                  />
                  <TooltipContent>
                    {t(($) => $.members_tab.view_agent_tooltip)}
                  </TooltipContent>
                </Tooltip>
              )}
              {m.member_type === "agent" && !isLeader(m) && !isArchived(m) && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-muted-foreground hover:text-amber-600 h-8 w-8 p-0"
                        onClick={() => onSetLeader(m.member_id)}
                        disabled={setLeaderPending}
                        aria-label={t(($) => $.members_tab.make_leader_tooltip)}
                      >
                        <Crown className="size-3.5" />
                      </Button>
                    }
                  />
                  <TooltipContent>
                    {t(($) => $.members_tab.make_leader_tooltip)}
                  </TooltipContent>
                </Tooltip>
              )}
              {!isLeader(m) && (
                <Tooltip>
                  <TooltipTrigger
                    render={
                      <Button
                        size="sm"
                        variant="ghost"
                        className="text-muted-foreground hover:text-destructive h-8 w-8 p-0"
                        onClick={() => onRemoveMember(m)}
                        aria-label={t(($) => $.members_tab.remove_member_tooltip)}
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    }
                  />
                  <TooltipContent>
                    {t(($) => $.members_tab.remove_member_tooltip)}
                  </TooltipContent>
                </Tooltip>
              )}
            </div>
          </div>
          );
        })}
      </div>
    </div>
  );
}

// Instructions tab body — mirrors agent's InstructionsTab. ContentEditor +
// Save button. The squad leader's prompt picks these up at task claim time
// (server/internal/handler/daemon.go).
function SquadInstructionsTab({
  squad,
  onSave,
  onDirtyChange,
}: {
  squad: Squad;
  onSave: (instructions: string) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useT("squads");
  const [value, setValue] = useState(squad.instructions ?? "");
  const [saving, setSaving] = useState(false);
  const isDirty = value !== (squad.instructions ?? "");

  useEffect(() => {
    setValue(squad.instructions ?? "");
  }, [squad.id, squad.instructions]);

  useEffect(() => {
    onDirtyChange?.(isDirty);
  }, [isDirty, onDirtyChange]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(value);
    } catch {
      // toast handled by parent
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex h-full flex-col gap-4">
      <p className="text-xs text-muted-foreground">
        {t(($) => $.instructions_tab.description)}
      </p>

      <div className="flex-1 min-h-0 overflow-y-auto rounded-md border bg-background px-4 py-3 transition-colors focus-within:border-input">
        <ContentEditor
          key={squad.id}
          defaultValue={value}
          onUpdate={setValue}
          placeholder="e.g. Always start by writing a failing test. Prefer small, atomic commits."
          debounceMs={150}
          disableMentions
          className="min-h-full"
        />
      </div>

      <div className="flex items-center justify-end gap-3">
        {isDirty && (
          <span className="text-xs text-muted-foreground">{t(($) => $.instructions_tab.unsaved_changes)}</span>
        )}
        <Button size="sm" onClick={handleSave} disabled={!isDirty || saving}>
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {t(($) => $.instructions_tab.save_button)}
        </Button>
      </div>
    </div>
  );
}
