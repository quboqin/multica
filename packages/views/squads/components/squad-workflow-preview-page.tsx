"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { useCurrentWorkspace, useWorkspacePaths } from "@multica/core/paths";
import {
  agentListOptions,
  squadMemberStatusOptions,
  workspaceKeys,
} from "@multica/core/workspace/queries";
import type {
  Squad,
  SquadMember,
  SquadMemberStatus,
  SquadMemberStatusValue,
} from "@multica/core/types";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  BookOpen,
  CheckCircle2,
  CircleHelp,
  ClipboardCheck,
  Code2,
  Crown,
  Lightbulb,
  Network,
  Rocket,
  Search,
  ShieldCheck,
  TestTube2,
  Users,
  type LucideIcon,
} from "lucide-react";
import { Skeleton } from "@multica/ui/components/ui/skeleton";
import { ActorAvatar } from "../../common/actor-avatar";
import { BreadcrumbHeader } from "../../layout/breadcrumb-header";
import { useT } from "../../i18n";
import { AppLink, useNavigation } from "../../navigation";
import {
  inferSquadWorkflow,
  type InferredWorkflowStage,
  type WorkflowMatchSource,
  type WorkflowStageId,
} from "./squad-workflow-model";

type LaneDefinition = {
  id: "discover" | "build" | "deliver";
  stageIds: WorkflowStageId[];
  accentClass: string;
  railClass: string;
};

const LANE_DEFINITIONS: LaneDefinition[] = [
  {
    id: "discover",
    stageIds: ["requirements", "knowledge", "research", "design"],
    accentClass: "text-cyan-700 dark:text-cyan-300",
    railClass: "bg-cyan-500",
  },
  {
    id: "build",
    stageIds: ["implementation", "review", "test"],
    accentClass: "text-blue-700 dark:text-blue-300",
    railClass: "bg-blue-500",
  },
  {
    id: "deliver",
    stageIds: ["delivery", "support"],
    accentClass: "text-emerald-700 dark:text-emerald-300",
    railClass: "bg-emerald-500",
  },
];

const STAGE_ICON: Record<WorkflowStageId, LucideIcon> = {
  requirements: ClipboardCheck,
  knowledge: BookOpen,
  research: Search,
  design: Lightbulb,
  implementation: Code2,
  review: ShieldCheck,
  test: TestTube2,
  delivery: Rocket,
  support: CircleHelp,
};

const STAGE_BORDER: Record<WorkflowStageId, string> = {
  requirements: "border-t-amber-400",
  knowledge: "border-t-violet-400",
  research: "border-t-cyan-500",
  design: "border-t-rose-400",
  implementation: "border-t-blue-500",
  review: "border-t-orange-400",
  test: "border-t-red-400",
  delivery: "border-t-emerald-500",
  support: "border-t-zinc-400",
};

const STATUS_DOT: Record<SquadMemberStatusValue, string> = {
  working: "bg-success",
  idle: "bg-muted-foreground/40",
  offline: "bg-muted-foreground/40",
  unstable: "bg-warning",
  archived: "bg-muted-foreground/40",
};

export function SquadWorkflowPreviewPage() {
  const { t } = useT("squads");
  const language = useAuthStore((state) => state.user?.language ?? "en");
  const isChinese = language.startsWith("zh");
  const workspace = useCurrentWorkspace();
  const wsId = useWorkspaceId();
  const p = useWorkspacePaths();
  const { pathname } = useNavigation();
  const segments = pathname.split("/").filter(Boolean);
  const squadId = segments[segments.indexOf("squads") + 1] ?? "";

  const { data: squad } = useQuery<Squad>({
    queryKey: [...workspaceKeys.squads(wsId), squadId],
    queryFn: () => api.getSquad(squadId),
    enabled: !!workspace?.id && !!squadId,
  });
  const { data: members = [] } = useQuery<SquadMember[]>({
    queryKey: [...workspaceKeys.squads(wsId), squadId, "members"],
    queryFn: () => api.listSquadMembers(squadId),
    enabled: !!workspace?.id && !!squadId,
  });
  const { data: memberStatusResp } = useQuery({
    ...squadMemberStatusOptions(wsId, squadId),
    enabled: !!workspace?.id && !!squadId,
  });
  const { data: agents = [] } = useQuery(agentListOptions(wsId));

  const memberStatusById = useMemo(() => {
    const statusMap = new Map<string, SquadMemberStatus>();
    for (const status of memberStatusResp?.members ?? []) statusMap.set(status.member_id, status);
    return statusMap;
  }, [memberStatusResp]);

  if (!squad) return <WorkflowPreviewSkeleton />;

  const leaderAgent = agents.find((agent) => agent.id === squad.leader_id);
  const agentMembers = members.filter(
    (member) => member.member_type === "agent" && member.member_id !== squad.leader_id,
  );
  const workflow = inferSquadWorkflow(agentMembers, agents);
  const stageById = new Map(workflow.map((stage) => [stage.id, stage]));
  const lanes = LANE_DEFINITIONS.map((lane) => ({
    ...lane,
    stages: lane.stageIds.flatMap((stageId) => {
      const stage = stageById.get(stageId);
      return stage ? [stage] : [];
    }),
  })).filter((lane) => lane.stages.length > 0);
  const activeAgents = members.filter(
    (member) => memberStatusById.get(member.member_id)?.status === "working",
  ).length;

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-muted/15">
      <BreadcrumbHeader
        segments={[
          { href: p.squads(), label: t(($) => $.page.title) },
          { href: p.squadDetail(squad.id), label: squad.name },
        ]}
        leaf={
          <>
            <Network className="size-4 text-muted-foreground" />
            <span className="truncate text-sm font-medium">
              {isChinese ? "流程编排方案 B" : "Workflow concept B"}
            </span>
          </>
        }
      />

      <main className="min-h-0 flex-1 overflow-y-auto px-4 py-6 md:px-8 lg:px-10">
        <div className="mx-auto w-full max-w-[1580px]">
          <header className="flex flex-col gap-5 border-b pb-6 lg:flex-row lg:items-end lg:justify-between">
            <div className="min-w-0">
              <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                <span className="h-px w-7 bg-foreground/40" />
                {isChinese ? "小队协作地图" : "Squad collaboration map"}
              </div>
              <h1 className="truncate text-2xl font-semibold text-foreground md:text-3xl">
                {squad.name}
              </h1>
              <p className="mt-2 max-w-3xl text-sm leading-6 text-muted-foreground">
                {squad.description ||
                  (isChinese
                    ? "从任务进入到最终交付，按 Agent 职责展示协作阶段与交接关系。"
                    : "Agent responsibilities and handoffs from task intake to delivery.")}
              </p>
            </div>

            <div className="flex flex-wrap items-center gap-x-6 gap-y-3 text-sm">
              <WorkflowMetric
                icon={Network}
                value={workflow.length}
                label={isChinese ? "流程阶段" : "Stages"}
              />
              <WorkflowMetric
                icon={Users}
                value={agentMembers.length}
                label={isChinese ? "执行 Agent" : "Execution agents"}
              />
              <WorkflowMetric
                icon={CheckCircle2}
                value={activeAgents}
                label={isChinese ? "工作中" : "Working"}
              />
            </div>
          </header>

          <section className="py-7">
            <div className="mx-auto flex max-w-xl flex-col items-center">
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                {isChinese ? "任务入口" : "Task intake"}
              </div>
              <AppLink
                href={leaderAgent ? p.agentDetail(leaderAgent.id) : p.squadDetail(squad.id)}
                className="group flex w-full min-w-0 items-center gap-4 rounded-md bg-foreground px-5 py-4 text-background shadow-sm transition-transform hover:-translate-y-0.5"
              >
                <ActorAvatar
                  actorType="agent"
                  actorId={squad.leader_id}
                  size={40}
                  showStatusDot
                  enableHoverCard={!!leaderAgent}
                  hoverCardVariant="live"
                />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <span className="truncate text-sm font-semibold">
                      {leaderAgent?.name ?? (isChinese ? "小队负责人" : "Squad leader")}
                    </span>
                    <Crown className="size-4 shrink-0 text-amber-400" />
                  </div>
                  <p className="mt-1 truncate text-xs text-background/65">
                    {leaderAgent?.description ||
                      (isChinese ? "接收用户目标，拆解任务并协调后续 Agent" : "Receives goals and coordinates downstream agents")}
                  </p>
                </div>
                <ArrowRight className="size-4 shrink-0 text-background/60 transition-transform group-hover:translate-x-1" />
              </AppLink>
              <div className="flex h-14 flex-col items-center text-muted-foreground">
                <div className="h-9 w-0.5 bg-foreground/25" />
                <ArrowDown className="size-5 -translate-y-0.5" />
              </div>
            </div>

            <div className="space-y-0">
              {lanes.map((lane, laneIndex) => {
                const nextLane = lanes[laneIndex + 1];
                return (
                  <div key={lane.id}>
                    <WorkflowLane
                      lane={lane}
                      stageOffset={lanes
                        .slice(0, laneIndex)
                        .reduce((total, previousLane) => total + previousLane.stages.length, 0)}
                      memberStatusById={memberStatusById}
                      isChinese={isChinese}
                      workspacePaths={p}
                      t={t}
                    />
                    {nextLane && (
                      <LaneHandoff
                        from={lane.stages[lane.stages.length - 1]!}
                        to={nextLane.stages[0]!}
                        isChinese={isChinese}
                        t={t}
                      />
                    )}
                  </div>
                );
              })}
            </div>
          </section>

          <footer className="flex flex-col gap-3 border-t py-5 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
            <span>
              {isChinese
                ? "编排由 Agent 名称、角色、描述和指令自动推断，不改变实际调度逻辑。"
                : "The workflow is inferred from agent metadata and does not change runtime routing."}
            </span>
            <AppLink
              href={p.squadDetail(squad.id)}
              className="inline-flex items-center gap-1.5 font-medium text-foreground hover:underline"
            >
              <ArrowLeft className="size-3.5" />
              {isChinese ? "返回原小队视图" : "Back to squad view"}
            </AppLink>
          </footer>
        </div>
      </main>
    </div>
  );
}

function WorkflowLane({
  lane,
  stageOffset,
  memberStatusById,
  isChinese,
  workspacePaths,
  t,
}: {
  lane: LaneDefinition & { stages: InferredWorkflowStage[] };
  stageOffset: number;
  memberStatusById: Map<string, SquadMemberStatus>;
  isChinese: boolean;
  workspacePaths: ReturnType<typeof useWorkspacePaths>;
  t: any;
}) {
  const copy = laneCopy(lane.id, isChinese);

  return (
    <section className="grid min-w-0 border-y bg-background/70 lg:grid-cols-[160px_minmax(0,1fr)]">
      <div className="relative flex flex-col justify-between overflow-hidden border-b p-5 lg:border-b-0 lg:border-r lg:p-6">
        <div className={`absolute inset-y-0 left-0 w-1 ${lane.railClass}`} />
        <div>
          <div className={`text-[11px] font-semibold uppercase tracking-wide ${lane.accentClass}`}>
            {copy.eyebrow}
          </div>
          <h2 className="mt-2 text-lg font-semibold text-foreground">{copy.title}</h2>
          <p className="mt-2 text-xs leading-5 text-muted-foreground">{copy.description}</p>
        </div>
        <div className="mt-4 text-xs tabular-nums text-muted-foreground">
          {lane.stages.length} {isChinese ? "个阶段" : "stages"}
        </div>
      </div>

      <div className="min-w-0 overflow-x-auto p-4 lg:p-5">
        <div className="flex min-w-max items-stretch">
          {lane.stages.map((stage, index) => (
            <div key={stage.id} className="flex items-center">
              <WorkflowStageCard
                stage={stage}
                sequence={stageOffset + index + 1}
                memberStatusById={memberStatusById}
                isChinese={isChinese}
                workspacePaths={workspacePaths}
                t={t}
              />
              {index < lane.stages.length - 1 && (
                <div className="flex w-8 shrink-0 items-center px-1 text-muted-foreground">
                  <div className="h-0.5 flex-1 bg-foreground/25" />
                  <ArrowRight className="size-5 -translate-x-0.5" />
                </div>
              )}
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function WorkflowStageCard({
  stage,
  sequence,
  memberStatusById,
  isChinese,
  workspacePaths,
  t,
}: {
  stage: InferredWorkflowStage;
  sequence: number;
  memberStatusById: Map<string, SquadMemberStatus>;
  isChinese: boolean;
  workspacePaths: ReturnType<typeof useWorkspacePaths>;
  t: any;
}) {
  const Icon = STAGE_ICON[stage.id];

  return (
    <article className={`flex w-[196px] min-w-[196px] flex-col border border-t-[3px] bg-background ${STAGE_BORDER[stage.id]}`}>
      <div className="border-b px-4 py-3.5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2.5">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-foreground">
              <Icon className="size-4" />
            </div>
            <div className="min-w-0">
              <div className="text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
                {isChinese ? `阶段 ${String(sequence).padStart(2, "0")}` : `Stage ${String(sequence).padStart(2, "0")}`}
              </div>
              <h3 className="truncate text-sm font-semibold text-foreground">{stageTitle(stage.id, t)}</h3>
            </div>
          </div>
          <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{stage.placements.length}</span>
        </div>
        <p className="mt-3 h-10 overflow-hidden text-xs leading-5 text-muted-foreground">
          {stageDescription(stage.id, t)}
        </p>
      </div>

      <div className="flex-1 px-4 py-1">
        {stage.placements.map((placement, index) => {
          const status = memberStatusById.get(placement.member.member_id);
          return (
            <AppLink
              key={placement.member.id}
              href={workspacePaths.agentDetail(placement.member.member_id)}
              className={`group flex min-w-0 gap-2.5 py-3 ${index > 0 ? "border-t" : ""}`}
            >
              <ActorAvatar
                actorType="agent"
                actorId={placement.member.member_id}
                size={28}
                showStatusDot
                enableHoverCard
                hoverCardVariant="live"
              />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  <span className="truncate text-xs font-semibold text-foreground group-hover:underline">
                    {placement.agent?.name ?? placement.member.member_id.slice(0, 8)}
                  </span>
                  <StatusDot status={status} isChinese={isChinese} />
                </div>
                <p className="mt-1 line-clamp-2 text-[11px] leading-4 text-muted-foreground">
                  {placement.agent?.description || placement.member.role || (isChinese ? "暂无职责描述" : "No responsibility description")}
                </p>
                <p className="mt-1.5 truncate text-[10px] text-muted-foreground/75">
                  {formatMatch(placement.match?.source ?? null, placement.match?.keyword ?? null, isChinese)}
                </p>
              </div>
            </AppLink>
          );
        })}
      </div>
    </article>
  );
}

function LaneHandoff({
  from,
  to,
  isChinese,
  t,
}: {
  from: InferredWorkflowStage;
  to: InferredWorkflowStage;
  isChinese: boolean;
  t: any;
}) {
  return (
    <div className="flex h-20 items-center justify-center">
      <div className="flex items-center gap-3 rounded-full border bg-background px-4 py-2 text-xs shadow-sm">
        <span className="font-medium text-foreground">{stageTitle(from.id, t)}</span>
        <div className="flex items-center text-muted-foreground">
          <div className="h-px w-5 bg-border" />
          <ArrowDown className="size-4" />
          <div className="h-px w-5 bg-border" />
        </div>
        <span className="font-medium text-foreground">{stageTitle(to.id, t)}</span>
        <span className="text-muted-foreground">{isChinese ? "阶段交接" : "handoff"}</span>
      </div>
    </div>
  );
}

function WorkflowMetric({ icon: Icon, value, label }: { icon: LucideIcon; value: number; label: string }) {
  return (
    <div className="flex items-center gap-2.5">
      <Icon className="size-4 text-muted-foreground" />
      <div>
        <div className="text-base font-semibold tabular-nums text-foreground">{value}</div>
        <div className="text-[11px] text-muted-foreground">{label}</div>
      </div>
    </div>
  );
}

function StatusDot({ status, isChinese }: { status: SquadMemberStatus | undefined; isChinese: boolean }) {
  const value = status?.status ?? null;
  const dotClass = value ? STATUS_DOT[value] : "bg-muted-foreground/30";
  const label = value
    ? isChinese
      ? ({ working: "工作中", idle: "空闲", offline: "离线", unstable: "不稳定", archived: "已归档" } as const)[value]
      : value
    : isChinese
      ? "未知"
      : "unknown";
  return (
    <span className="inline-flex shrink-0 items-center gap-1 text-[10px] text-muted-foreground">
      <span className={`size-1.5 rounded-full ${dotClass}`} />
      {label}
    </span>
  );
}

function laneCopy(id: LaneDefinition["id"], isChinese: boolean) {
  const zh = {
    discover: { eyebrow: "01 / DISCOVER", title: "理解与设计", description: "先确认做什么、依据什么，再形成可执行方案。" },
    build: { eyebrow: "02 / BUILD", title: "构建与验证", description: "完成实现，通过评审和测试逐步收敛质量风险。" },
    deliver: { eyebrow: "03 / DELIVER", title: "发布与协作", description: "整理交付物并完成提测、发布和专项协作。" },
  } as const;
  const en = {
    discover: { eyebrow: "01 / DISCOVER", title: "Understand and design", description: "Clarify the goal and context before defining an actionable solution." },
    build: { eyebrow: "02 / BUILD", title: "Build and validate", description: "Implement, review, test, and progressively reduce delivery risk." },
    deliver: { eyebrow: "03 / DELIVER", title: "Release and collaborate", description: "Prepare deliverables and complete test handoff, release, and specialist work." },
  } as const;
  return (isChinese ? zh : en)[id];
}

function stageTitle(stageId: WorkflowStageId, t: any) {
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

function stageDescription(stageId: WorkflowStageId, t: any) {
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

function formatMatch(source: WorkflowMatchSource | null, keyword: string | null, isChinese: boolean) {
  if (!source || !keyword) return isChinese ? "职责未分类" : "Unclassified responsibility";
  const sourceLabel = isChinese
    ? ({ name: "名称", role: "角色", description: "描述", instructions: "指令" } as const)[source]
    : source;
  return isChinese ? `${sourceLabel}命中“${keyword}”` : `${sourceLabel} matched "${keyword}"`;
}

function WorkflowPreviewSkeleton() {
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b px-6 py-3"><Skeleton className="h-5 w-64" /></div>
      <div className="space-y-8 p-8">
        <div className="space-y-3"><Skeleton className="h-8 w-72" /><Skeleton className="h-4 w-[520px] max-w-full" /></div>
        <Skeleton className="mx-auto h-20 w-[520px] max-w-full" />
        <Skeleton className="h-64 w-full" />
        <Skeleton className="h-64 w-full" />
      </div>
    </div>
  );
}
