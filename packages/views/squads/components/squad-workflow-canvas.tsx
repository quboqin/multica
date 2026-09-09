"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent as ReactPointerEvent,
} from "react";
import {
  DndContext,
  PointerSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragMoveEvent,
} from "@dnd-kit/core";
import { Crown, GripVertical, Maximize2, Minimize2, Minus, Plus, RotateCcw, Scan, type LucideIcon } from "lucide-react";
import type {
  Agent,
  SquadMember,
  SquadMemberStatus,
  SquadMemberStatusValue,
  SquadWorkflowCanvasLayout,
  SquadWorkflowCanvasPoint,
} from "@multica/core/types";
import { useWorkspacePaths } from "@multica/core/paths";
import { Button } from "@multica/ui/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@multica/ui/components/ui/tooltip";
import { ActorAvatar } from "../../common/actor-avatar";
import { useT } from "../../i18n";
import { AppLink } from "../../navigation";

const CANVAS_MIN_WIDTH = 1280;
const CANVAS_HEIGHT = 760;
const CANVAS_VERTICAL_PADDING = 400;
const LEADER_WIDTH = 224;
const LEADER_HEIGHT = 86;
const STAGE_WIDTH = 360;
const STAGE_HEADER_HEIGHT = 76;
const STAGE_MIN_HEIGHT = 280;
const STAGE_GAP = 48;
const AGENT_WIDTH = 260;
const AGENT_HEIGHT = 72;
const AGENT_GAP = 12;
const AGENT_INSET = 20;

const STATUS_DOT_CLASS: Record<SquadMemberStatusValue, string> = {
  working: "bg-success",
  idle: "bg-muted-foreground/40",
  offline: "bg-muted-foreground/25",
  unstable: "bg-warning",
  archived: "bg-muted-foreground/40",
};

export type SquadWorkflowCanvasPlacement = {
  member: SquadMember;
  agent: Agent | undefined;
  reason: string;
};

export type SquadWorkflowCanvasStage = {
  id: string;
  title: string;
  description: string;
  icon: LucideIcon;
  placements: SquadWorkflowCanvasPlacement[];
};

type SquadWorkflowCanvasProps = {
  leader: {
    member: SquadMember;
    title: string;
    subtitle: string;
    status: SquadMemberStatus | undefined;
  };
  stages: SquadWorkflowCanvasStage[];
  memberStatusById: Map<string, SquadMemberStatus>;
  canManage: boolean;
  layout: SquadWorkflowCanvasLayout;
  onLayoutChange: (layout: SquadWorkflowCanvasLayout) => Promise<void> | void;
  onMoveAgent: (agentId: string, stageId: string) => Promise<boolean>;
};

function clamp(value: number, minimum: number, maximum: number) {
  return Math.min(maximum, Math.max(minimum, value));
}

function stageHeight(stage: SquadWorkflowCanvasStage) {
  return Math.max(
    STAGE_MIN_HEIGHT,
    STAGE_HEADER_HEIGHT + AGENT_INSET * 2 + stage.placements.length * AGENT_HEIGHT +
      Math.max(0, stage.placements.length - 1) * AGENT_GAP,
  );
}

function statusLabel(status: SquadMemberStatusValue | null, t: any) {
  if (status === "working") return t(($: any) => $.members_tab.status_working);
  if (status === "idle") return t(($: any) => $.members_tab.status_idle);
  if (status === "offline") return t(($: any) => $.members_tab.status_offline);
  if (status === "unstable") return t(($: any) => $.members_tab.status_unstable);
  if (status === "archived") return t(($: any) => $.members_tab.status_archived);
  return t(($: any) => $.visual_tab.status_unknown);
}

function CanvasStatus({ status }: { status: SquadMemberStatus | undefined }) {
  const { t } = useT("squads");
  const value = status?.status ?? null;
  const dotClass = value ? STATUS_DOT_CLASS[value] : "bg-muted-foreground/30";
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 text-[11px] text-muted-foreground">
      <span className={`size-1.5 rounded-full ${dotClass}`} />
      {statusLabel(value, t)}
    </span>
  );
}

function CanvasAgentNode({
  placement,
  status,
  position,
  zoom,
  canManage,
}: {
  placement: SquadWorkflowCanvasPlacement;
  status: SquadMemberStatus | undefined;
  position: SquadWorkflowCanvasPoint;
  zoom: number;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const p = useWorkspacePaths();
  const { member, agent } = placement;
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `agent:${member.member_id}`,
    disabled: !canManage,
  });
  const dragTransform = transform
    ? `translate3d(${transform.x / zoom}px, ${transform.y / zoom}px, 0)`
    : undefined;

  return (
    <div
      ref={setNodeRef}
      data-canvas-node
      data-canvas-agent={member.member_id}
      className={`group absolute flex min-w-0 items-center gap-2 rounded-md border bg-background px-2.5 shadow-sm transition-[border-color,box-shadow,opacity] ${
        isDragging
          ? "z-50 opacity-90 shadow-xl ring-2 ring-primary/30"
          : "z-10 hover:border-foreground/30 hover:shadow-md"
      }`}
      style={{
        left: position.x,
        top: position.y,
        width: AGENT_WIDTH,
        height: AGENT_HEIGHT,
        transform: dragTransform,
      }}
      title={placement.reason}
    >
      {canManage && (
        <button
          type="button"
          className="-ml-1 flex h-8 w-5 shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
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
        size={30}
        showStatusDot
        enableHoverCard
        hoverCardVariant="live"
      />
      <AppLink href={p.agentDetail(member.member_id)} className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center justify-between gap-2">
          <span className="truncate text-sm font-semibold text-foreground">
            {agent?.name ?? member.member_id.slice(0, 8)}
          </span>
          <CanvasStatus status={status} />
        </span>
        <span className="mt-1 block truncate text-xs text-muted-foreground">
          {agent?.description || member.role || t(($: any) => $.visual_tab.no_description)}
        </span>
      </AppLink>
    </div>
  );
}

function CanvasLeaderNode({
  leader,
  position,
  zoom,
  canManage,
}: {
  leader: SquadWorkflowCanvasProps["leader"];
  position: SquadWorkflowCanvasPoint;
  zoom: number;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const p = useWorkspacePaths();
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `leader:${leader.member.member_id}`,
    disabled: !canManage,
  });
  const dragTransform = transform
    ? `translate3d(${transform.x / zoom}px, ${transform.y / zoom}px, 0)`
    : undefined;

  return (
    <div
      ref={setNodeRef}
      data-canvas-node
      data-canvas-leader
      className={`absolute flex items-center gap-2 rounded-md border border-amber-300/70 bg-background px-2.5 shadow-sm transition-[box-shadow,opacity] dark:border-amber-700/60 ${
        isDragging ? "z-50 opacity-90 shadow-xl ring-2 ring-amber-400/30" : "z-10 hover:shadow-md"
      }`}
      style={{
        left: position.x,
        top: position.y,
        width: LEADER_WIDTH,
        height: LEADER_HEIGHT,
        transform: dragTransform,
      }}
    >
      {canManage && (
        <button
          type="button"
          className="-ml-1 flex h-8 w-5 shrink-0 cursor-grab touch-none items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
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
        actorId={leader.member.member_id}
        size={32}
        showStatusDot
        enableHoverCard
        hoverCardVariant="live"
      />
      <AppLink href={p.agentDetail(leader.member.member_id)} className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="truncate text-sm font-semibold text-foreground">{leader.title}</span>
          <Crown className="size-3.5 shrink-0 text-amber-600" />
        </span>
        <span className="mt-1 block truncate text-xs text-muted-foreground">{leader.subtitle}</span>
        <span className="mt-1.5 block"><CanvasStatus status={leader.status} /></span>
      </AppLink>
    </div>
  );
}

function CanvasStageGroup({
  stage,
  index,
  position,
  agentPositions,
  memberStatusById,
  zoom,
  canManage,
}: {
  stage: SquadWorkflowCanvasStage;
  index: number;
  position: SquadWorkflowCanvasPoint;
  agentPositions: Record<string, SquadWorkflowCanvasPoint>;
  memberStatusById: Map<string, SquadMemberStatus>;
  zoom: number;
  canManage: boolean;
}) {
  const { t } = useT("squads");
  const StageIcon = stage.icon;
  const draggable = useDraggable({ id: `stage:${stage.id}`, disabled: !canManage });
  const droppable = useDroppable({ id: `stage-drop:${stage.id}`, disabled: !canManage });
  const setDraggableNodeRef = draggable.setNodeRef;
  const setDroppableNodeRef = droppable.setNodeRef;
  const setNodeRef = useCallback((node: HTMLDivElement | null) => {
    setDraggableNodeRef(node);
    setDroppableNodeRef(node);
  }, [setDraggableNodeRef, setDroppableNodeRef]);
  const dragTransform = draggable.transform
    ? `translate3d(${draggable.transform.x / zoom}px, ${draggable.transform.y / zoom}px, 0)`
    : undefined;
  const height = stageHeight(stage);

  return (
    <section
      ref={setNodeRef}
      data-canvas-node
      data-canvas-stage={stage.id}
      className={`absolute overflow-visible rounded-md border bg-background/60 transition-[border-color,box-shadow,background-color,opacity] ${
        draggable.isDragging
          ? "z-40 opacity-90 shadow-xl ring-2 ring-primary/25"
          : droppable.isOver
            ? "border-primary bg-primary/[0.04] shadow-md ring-2 ring-primary/20"
            : "border-border/90 shadow-sm"
      }`}
      style={{
        left: position.x,
        top: position.y,
        width: STAGE_WIDTH,
        height,
        transform: dragTransform,
      }}
    >
      <header
        className={`flex h-[76px] items-center gap-2.5 rounded-t-md border-b bg-muted/45 px-3 ${canManage ? "cursor-grab touch-none active:cursor-grabbing" : ""}`}
        title={canManage ? t(($: any) => $.visual_tab.drag_handle) : undefined}
        {...draggable.listeners}
        {...draggable.attributes}
      >
        {canManage && (
          <span className="-ml-1 flex h-9 w-5 shrink-0 items-center justify-center text-muted-foreground">
            <GripVertical className="size-4" />
          </span>
        )}
        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-primary shadow-sm">
          <StageIcon className="size-4" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex min-w-0 items-center gap-2">
            <span className="font-mono text-[10px] font-semibold tabular-nums text-muted-foreground">
              {String(index + 1).padStart(2, "0")}
            </span>
            <span className="truncate text-sm font-semibold text-foreground">{stage.title}</span>
          </span>
          <span className="mt-1 block truncate text-xs text-muted-foreground">{stage.description}</span>
          <span className={`mt-0.5 block text-[11px] ${droppable.isOver ? "font-medium text-primary" : "text-muted-foreground"}`}>
            {droppable.isOver
              ? t(($: any) => $.visual_tab.drop_here)
              : t(($) => $.visual_tab.stage_agent_count, { count: stage.placements.length })}
          </span>
        </span>
      </header>

      <div className="absolute inset-x-0 bottom-0 top-[76px] rounded-b-md bg-muted/[0.12]">
        {stage.placements.map((placement) => (
          <CanvasAgentNode
            key={placement.member.id}
            placement={placement}
            status={memberStatusById.get(placement.member.member_id)}
            position={agentPositions[placement.member.member_id]!}
            zoom={zoom}
            canManage={canManage}
          />
        ))}
      </div>
    </section>
  );
}

export function SquadWorkflowCanvas({
  leader,
  stages,
  memberStatusById,
  canManage,
  layout,
  onLayoutChange,
  onMoveAgent,
}: SquadWorkflowCanvasProps) {
  const { t } = useT("squads");
  const containerRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const panState = useRef<{ pointerId: number; x: number; y: number; left: number; top: number } | null>(null);
  const [zoom, setZoom] = useState(1);
  const [isPanning, setIsPanning] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [viewportHeight, setViewportHeight] = useState(0);
  const [dragPreview, setDragPreview] = useState<{
    id: string;
    delta: SquadWorkflowCanvasPoint;
  } | null>(null);
  const initializedViewportRef = useRef(false);
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }));
  const canvasWidth = Math.max(
    CANVAS_MIN_WIDTH,
    340 + stages.length * STAGE_WIDTH + Math.max(0, stages.length - 1) * STAGE_GAP + 64,
  );
  const maximumStageHeight = Math.max(STAGE_MIN_HEIGHT, ...stages.map(stageHeight));
  const visibleCanvasHeight = viewportHeight > 0 ? viewportHeight / zoom : CANVAS_HEIGHT;
  const canvasHeight = Math.max(
    maximumStageHeight + CANVAS_VERTICAL_PADDING * 2,
    visibleCanvasHeight + CANVAS_VERTICAL_PADDING * 2,
  );
  const defaultStageY = CANVAS_VERTICAL_PADDING + Math.max(48, (visibleCanvasHeight - maximumStageHeight) / 2);
  const hasSavedLayout = !!layout.leader
    || Object.keys(layout.stages ?? {}).length > 0
    || Object.keys(layout.agents ?? {}).length > 0;

  const effectiveLayout = useMemo<SquadWorkflowCanvasLayout>(() => {
    const stagePositions: Record<string, SquadWorkflowCanvasPoint> = {};
    const agentPositions: Record<string, SquadWorkflowCanvasPoint> = {};
    stages.forEach((stage, stageIndex) => {
      stagePositions[stage.id] = layout.stages?.[stage.id] ?? {
        x: 340 + stageIndex * (STAGE_WIDTH + STAGE_GAP),
        y: defaultStageY,
      };
      stage.placements.forEach((placement, placementIndex) => {
        agentPositions[placement.member.member_id] = layout.agents?.[placement.member.member_id] ?? {
          x: AGENT_INSET,
          y: AGENT_INSET + placementIndex * (AGENT_HEIGHT + AGENT_GAP),
        };
      });
    });
    return {
      leader: layout.leader ?? {
        x: 48,
        y: defaultStageY + (STAGE_HEADER_HEIGHT - LEADER_HEIGHT) / 2,
      },
      stages: stagePositions,
      agents: agentPositions,
    };
  }, [defaultStageY, layout, stages]);

  const connectionLayout = useMemo<SquadWorkflowCanvasLayout>(() => {
    if (!dragPreview) return effectiveLayout;
    if (dragPreview.id.startsWith("leader:")) {
      const current = effectiveLayout.leader!;
      return {
        ...effectiveLayout,
        leader: {
          x: current.x + dragPreview.delta.x,
          y: current.y + dragPreview.delta.y,
        },
      };
    }
    if (dragPreview.id.startsWith("stage:")) {
      const stageID = dragPreview.id.slice("stage:".length);
      const current = effectiveLayout.stages[stageID];
      if (!current) return effectiveLayout;
      return {
        ...effectiveLayout,
        stages: {
          ...effectiveLayout.stages,
          [stageID]: {
            x: current.x + dragPreview.delta.x,
            y: current.y + dragPreview.delta.y,
          },
        },
      };
    }
    return effectiveLayout;
  }, [dragPreview, effectiveLayout]);

  const fitCanvas = useCallback(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const nextZoom = Math.min(
      1,
      Math.max(
        0.55,
        Math.min(
          (viewport.clientWidth - 40) / canvasWidth,
          (viewport.clientHeight - 40) / (maximumStageHeight + 96),
        ),
      ),
    );
    setZoom(Number(nextZoom.toFixed(2)));
    requestAnimationFrame(() => viewport.scrollTo({
      left: 0,
      top: CANVAS_VERTICAL_PADDING * nextZoom,
      behavior: "smooth",
    }));
  }, [canvasWidth, maximumStageHeight]);

  const changeZoom = (delta: number) => {
    setZoom((current) => Math.min(1.35, Math.max(0.45, Number((current + delta).toFixed(2)))));
  };

  useEffect(() => {
    const handleFullscreenChange = () => {
      setIsFullscreen(document.fullscreenElement === containerRef.current);
    };
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  useEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport) return;
    const observer = new ResizeObserver(([entry]) => {
      if (!entry) return;
      setViewportHeight(entry.contentRect.height);
      if (!initializedViewportRef.current) {
        initializedViewportRef.current = true;
        requestAnimationFrame(() => {
          viewport.scrollTop = CANVAS_VERTICAL_PADDING;
        });
      }
    });
    observer.observe(viewport);
    return () => observer.disconnect();
  }, []);

  const toggleFullscreen = async () => {
    if (document.fullscreenElement === containerRef.current) {
      await document.exitFullscreen();
      return;
    }
    setZoom(1);
    await containerRef.current?.requestFullscreen();
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        const viewport = viewportRef.current;
        if (viewport) viewport.scrollTop = CANVAS_VERTICAL_PADDING;
      });
    });
  };

  const handleDragEnd = async ({ active, over, delta }: DragEndEvent) => {
    const activeID = String(active.id);
    const nextLayout: SquadWorkflowCanvasLayout = {
      leader: effectiveLayout.leader,
      stages: { ...effectiveLayout.stages },
      agents: { ...effectiveLayout.agents },
    };
    const deltaX = delta.x / zoom;
    const deltaY = delta.y / zoom;

    if (activeID.startsWith("leader:")) {
      const current = effectiveLayout.leader!;
      nextLayout.leader = {
        x: clamp(current.x + deltaX, 24, canvasWidth - LEADER_WIDTH - 24),
        y: clamp(current.y + deltaY, 24, canvasHeight - LEADER_HEIGHT - 24),
      };
      await onLayoutChange(nextLayout);
      return;
    }

    if (activeID.startsWith("stage:")) {
      const stageID = activeID.slice("stage:".length);
      const current = effectiveLayout.stages[stageID];
      const stage = stages.find((candidate) => candidate.id === stageID);
      if (!current || !stage) return;
      nextLayout.stages[stageID] = {
        x: clamp(current.x + deltaX, 24, canvasWidth - STAGE_WIDTH - 24),
        y: clamp(current.y + deltaY, 24, canvasHeight - stageHeight(stage) - 24),
      };
      await onLayoutChange(nextLayout);
      return;
    }

    if (!activeID.startsWith("agent:")) return;
    const agentID = activeID.slice("agent:".length);
    const sourceStage = stages.find((stage) =>
      stage.placements.some((placement) => placement.member.member_id === agentID),
    );
    const currentAgentPosition = effectiveLayout.agents[agentID];
    if (!sourceStage || !currentAgentPosition) return;
    const sourceStagePosition = effectiveLayout.stages[sourceStage.id]!;
    const targetStageID = over && String(over.id).startsWith("stage-drop:")
      ? String(over.id).slice("stage-drop:".length)
      : sourceStage.id;
    const targetStage = stages.find((stage) => stage.id === targetStageID) ?? sourceStage;
    const targetStagePosition = effectiveLayout.stages[targetStage.id]!;
    const globalX = sourceStagePosition.x + currentAgentPosition.x + deltaX;
    const globalY = sourceStagePosition.y + STAGE_HEADER_HEIGHT + currentAgentPosition.y + deltaY;
    const targetHeight = stageHeight(targetStage);
    nextLayout.agents[agentID] = {
      x: clamp(globalX - targetStagePosition.x, 12, STAGE_WIDTH - AGENT_WIDTH - 12),
      y: clamp(
        globalY - targetStagePosition.y - STAGE_HEADER_HEIGHT,
        12,
        targetHeight - STAGE_HEADER_HEIGHT - AGENT_HEIGHT - 12,
      ),
    };
    if (targetStage.id !== sourceStage.id) {
      const moved = await onMoveAgent(agentID, targetStage.id);
      if (!moved) return;
    }
    await onLayoutChange(nextLayout);
  };

  const handleDragMove = ({ active, delta }: DragMoveEvent) => {
    setDragPreview({
      id: String(active.id),
      delta: { x: delta.x / zoom, y: delta.y / zoom },
    });
  };

  const handlePointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || (event.target as HTMLElement).closest("[data-canvas-node], [data-canvas-control]")) return;
    const viewport = viewportRef.current;
    if (!viewport) return;
    panState.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      left: viewport.scrollLeft,
      top: viewport.scrollTop,
    };
    viewport.setPointerCapture(event.pointerId);
    setIsPanning(true);
  };

  const handlePointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
    const viewport = viewportRef.current;
    const drag = panState.current;
    if (!viewport || !drag || drag.pointerId !== event.pointerId) return;
    viewport.scrollLeft = drag.left - (event.clientX - drag.x);
    viewport.scrollTop = drag.top - (event.clientY - drag.y);
  };

  const stopPanning = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (panState.current?.pointerId !== event.pointerId) return;
    panState.current = null;
    setIsPanning(false);
    if (viewportRef.current?.hasPointerCapture(event.pointerId)) {
      viewportRef.current.releasePointerCapture(event.pointerId);
    }
  };

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={pointerWithin}
      onDragStart={({ active }) => setDragPreview({ id: String(active.id), delta: { x: 0, y: 0 } })}
      onDragMove={handleDragMove}
      onDragCancel={() => setDragPreview(null)}
      onDragEnd={(event) => {
        setDragPreview(null);
        void handleDragEnd(event);
      }}
    >
      <div
        ref={containerRef}
        className={`relative overflow-hidden bg-background shadow-sm ${
          isFullscreen
            ? "h-screen min-h-0 w-screen rounded-none border-0"
            : "min-h-[480px] rounded-md border md:h-[calc(100vh-260px)] md:min-h-[540px]"
        }`}
      >
        <div
          ref={viewportRef}
          data-workflow-canvas
          className={`h-full min-h-[480px] touch-none overflow-auto bg-muted/20 text-muted-foreground select-none md:min-h-[540px] ${isPanning ? "cursor-grabbing" : "cursor-grab"}`}
          style={{ backgroundImage: "radial-gradient(circle, var(--border) 1px, transparent 1px)", backgroundSize: "20px 20px" }}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={stopPanning}
          onPointerCancel={stopPanning}
        >
          <div className="relative" style={{ width: canvasWidth * zoom, height: canvasHeight * zoom }}>
            <div
              className="absolute left-0 top-0 origin-top-left"
              style={{ width: canvasWidth, height: canvasHeight, transform: `scale(${zoom})` }}
            >
              <svg className="pointer-events-none absolute inset-0 size-full overflow-visible" aria-hidden="true">
                <defs>
                  <marker id="workflow-group-arrow" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="6" markerHeight="6" orient="auto-start-reverse">
                    <path d="M 0 0 L 10 5 L 0 10 z" fill="currentColor" />
                  </marker>
                </defs>
                {stages[0] && (() => {
                  const first = connectionLayout.stages[stages[0].id]!;
                  const leaderPosition = connectionLayout.leader!;
                  const startX = leaderPosition.x + LEADER_WIDTH;
                  const startY = leaderPosition.y + LEADER_HEIGHT / 2;
                  const endX = first.x - 8;
                  const endY = first.y + STAGE_HEADER_HEIGHT / 2;
                  const middleX = (startX + endX) / 2;
                  return (
                    <path
                      data-canvas-edge="leader"
                      d={`M ${startX} ${startY} C ${middleX} ${startY}, ${middleX} ${endY}, ${endX} ${endY}`}
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      markerEnd="url(#workflow-group-arrow)"
                    />
                  );
                })()}
                {stages.slice(0, -1).map((stage, index) => {
                  const nextStage = stages[index + 1]!;
                  const current = connectionLayout.stages[stage.id]!;
                  const next = connectionLayout.stages[nextStage.id]!;
                  const startX = current.x + STAGE_WIDTH;
                  const startY = current.y + STAGE_HEADER_HEIGHT / 2;
                  const endX = next.x - 8;
                  const endY = next.y + STAGE_HEADER_HEIGHT / 2;
                  const middleX = (startX + endX) / 2;
                  return (
                    <path
                      key={`stage-edge:${stage.id}`}
                      data-canvas-edge={`stage:${stage.id}`}
                      d={`M ${startX} ${startY} C ${middleX} ${startY}, ${middleX} ${endY}, ${endX} ${endY}`}
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      markerEnd="url(#workflow-group-arrow)"
                    />
                  );
                })}
              </svg>

              <CanvasLeaderNode
                leader={leader}
                position={effectiveLayout.leader!}
                zoom={zoom}
                canManage={canManage}
              />

              {stages.map((stage, index) => (
                <CanvasStageGroup
                  key={stage.id}
                  stage={stage}
                  index={index}
                  position={effectiveLayout.stages[stage.id]!}
                  agentPositions={effectiveLayout.agents}
                  memberStatusById={memberStatusById}
                  zoom={zoom}
                  canManage={canManage}
                />
              ))}
            </div>
          </div>
        </div>

        <div data-canvas-control className="absolute bottom-3 right-3 flex items-center gap-1 rounded-md border bg-background/95 p-1 shadow-sm">
          {canManage && (
            <>
              <Tooltip>
                <TooltipTrigger
                  render={<Button type="button" size="icon-sm" variant="ghost" disabled={!hasSavedLayout} onClick={() => void onLayoutChange({ stages: {}, agents: {} })} aria-label={t(($: any) => $.visual_tab.canvas_reset_layout)} />}
                >
                  <RotateCcw className="size-3.5" />
                </TooltipTrigger>
                <TooltipContent>{t(($: any) => $.visual_tab.canvas_reset_layout)}</TooltipContent>
              </Tooltip>
              <span className="mx-0.5 h-4 border-l" />
            </>
          )}
          <Tooltip>
            <TooltipTrigger
              render={<Button type="button" size="icon-sm" variant="ghost" onClick={() => changeZoom(-0.1)} disabled={zoom <= 0.45} aria-label={t(($: any) => $.visual_tab.canvas_zoom_out)} />}
            >
              <Minus className="size-3.5" />
            </TooltipTrigger>
            <TooltipContent>{t(($: any) => $.visual_tab.canvas_zoom_out)}</TooltipContent>
          </Tooltip>
          <span className="w-11 text-center text-[11px] tabular-nums text-muted-foreground">{Math.round(zoom * 100)}%</span>
          <Tooltip>
            <TooltipTrigger
              render={<Button type="button" size="icon-sm" variant="ghost" onClick={() => changeZoom(0.1)} disabled={zoom >= 1.35} aria-label={t(($: any) => $.visual_tab.canvas_zoom_in)} />}
            >
              <Plus className="size-3.5" />
            </TooltipTrigger>
            <TooltipContent>{t(($: any) => $.visual_tab.canvas_zoom_in)}</TooltipContent>
          </Tooltip>
          <span className="mx-0.5 h-4 border-l" />
          <Tooltip>
            <TooltipTrigger
              render={<Button type="button" size="icon-sm" variant="ghost" onClick={fitCanvas} aria-label={t(($: any) => $.visual_tab.canvas_fit)} />}
            >
              <Scan className="size-3.5" />
            </TooltipTrigger>
            <TooltipContent>{t(($: any) => $.visual_tab.canvas_fit)}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger
              render={<Button type="button" size="icon-sm" variant="ghost" onClick={() => void toggleFullscreen()} aria-label={t(($: any) => isFullscreen ? $.visual_tab.canvas_exit_fullscreen : $.visual_tab.canvas_fullscreen)} />}
            >
              {isFullscreen ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
            </TooltipTrigger>
            <TooltipContent>
              {t(($: any) => isFullscreen ? $.visual_tab.canvas_exit_fullscreen : $.visual_tab.canvas_fullscreen)}
            </TooltipContent>
          </Tooltip>
        </div>
      </div>
    </DndContext>
  );
}
