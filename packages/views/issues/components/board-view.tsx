"use client";

import { useIssueStatuses } from "@multica/core/issue-statuses/hooks";
import { cn } from "@multica/ui/lib/utils";

import { useQuery } from "@tanstack/react-query";
import { useState, useCallback, useMemo, useEffect, useRef, memo } from "react";
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  type DragStartEvent,
  type DragEndEvent,
  type DragOverEvent,
} from "@dnd-kit/core";
import { arrayMove } from "@dnd-kit/sortable";
import { toast } from "sonner";
import type {
  Issue,
  IssueStatus,
  Project,
} from "@multica/core/types";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { useWorkspaceId } from "@multica/core/hooks";
import { BoardColumn, BOARD_CARD_WIDTH, type BoardColumnGroup } from "./board-column";
import { useIssueViewGroups } from "./use-issue-view-groups";
import { BoardWorkflowContext, type BoardWorkflowContextValue } from "./board-workflow-context";
import {
  issueStatusWorkflow,
  resolveProjectWorkflow,
  stepHandsOff,
  useIssueWorkflows,
  workflowAllowsStatus,
  workflowStep,
} from "@multica/core/issue-workflows";
import { WorkflowHandoffConfirmDialog } from "../../workflows/handoff-confirm-dialog";
import { stepHandlerActor, useStepHandlerLabel } from "../../workflows/step-handler";
import { projectListOptions } from "@multica/core/projects/queries";
import { BoardCardContent } from "./board-card";
import { InfiniteScrollSentinel } from "./infinite-scroll-sentinel";
import { ListLoadMoreFooter } from "./list-load-more-footer";
import type { ChildProgress } from "./list-row";
import type { IssueCreateDefaults } from "../surface/types";
import type {
  IssueStatusPageState,
  IssueStatusPagination,
} from "../surface/use-issue-status-branches";
import type {
  IssueGroupBranches,
  IssueGroupPageState,
} from "../surface/use-issue-group-branches";
import { useDragSettle } from "./use-drag-settle";
import { useIssuePeekActions } from "../surface/peek-context";
import { useBoardDragPan } from "./use-board-drag-pan";
import { useRestoredScrollRef } from "../../platform";
import { useT } from "../../i18n";
import {
  type DragMoveUpdates,
  makeKanbanCollision,
  buildColumns,
  computePosition,
  findColumn,
  getMoveAnchors,
  insertIdByPosition,
  issueMatchesGroup,
  getMoveUpdates,
} from "../utils/drag-utils";
import { useStatusLabel } from "../utils/status-label";

function isStatusGroup(
  group: BoardColumnGroup,
): group is BoardColumnGroup & { status: IssueStatus } {
  return group.status !== undefined;
}

const EMPTY_PROGRESS_MAP = new Map<string, ChildProgress>();
const EMPTY_IDS: string[] = [];

function BoardViewImpl({
  issues,
  visibleStatuses,
  onMoveIssue,
  childProgressMap = EMPTY_PROGRESS_MAP,
  projectMap,
  projectId,
  onCreateIssue,
  statusPagination,
  groupBranches,
}: {
  issues: Issue[];
  visibleStatuses: IssueStatus[];
  onMoveIssue: (issueId: string, updates: DragMoveUpdates, onSettled?: () => void) => void;
  childProgressMap?: Map<string, ChildProgress>;
  projectMap?: Map<string, Project>;
  /** When set, the per-column "+" pre-fills the project on the create form. */
  projectId?: string;
  onCreateIssue?: (defaults: IssueCreateDefaults) => void;
  statusPagination?: IssueStatusPagination;
  groupBranches?: IssueGroupBranches;
}) {
  const { t } = useT("issues");
  const sortBy = useViewStore((s) => s.sortBy);
  const boardWsId = useWorkspaceId();
  const catalog = useIssueStatuses(boardWsId);
  const labelOf = useStatusLabel(boardWsId);
  const { groups, groupedIssues, grouping, groupingOptionIds, groupPagination, applyPropertyGroupValue, sortLabel } =
    useIssueViewGroups({ issues, visibleStatuses, projectMap, groupBranches });
  const groupIds = useMemo(
    () => new Set(groups.map((group) => group.id)),
    [groups],
  );
  const groupMap = useMemo(
    () => new Map(groups.map((group) => [group.id, group])),
    [groups],
  );
  const collisionDetection = useMemo(
    () => makeKanbanCollision(groupIds),
    [groupIds],
  );

  // --- Drag state ---
  const [activeIssue, setActiveIssue] = useState<Issue | null>(null);
  const [pendingHandoff, setPendingHandoff] = useState<{
    issue: Issue;
    status: IssueStatus;
    updates: ReturnType<typeof getMoveUpdates>;
    anchors: ReturnType<typeof getMoveAnchors>;
  } | null>(null);

  // Project workflows (MUL-7420): a card can only move to a status its
  // project's workflow lists, and a project board names each step's handler.
  const { workflows } = useIssueWorkflows(boardWsId);
  const { data: workspaceProjects } = useQuery(projectListOptions(boardWsId));
  const workflowOfIssue = useCallback(
    (issue: Issue | null | undefined) => {
      if (!issue?.project_id) return null;
      return resolveProjectWorkflow(
        workflows,
        workspaceProjects?.find((p) => p.id === issue.project_id),
      );
    },
    [workflows, workspaceProjects],
  );
  // Which statuses a card may move to: its project's workflow, or Default
  // outside a project, where nothing hands off. (MUL-8250)
  const statusWorkflowOfIssue = useCallback(
    (issue: Issue | null | undefined) => (issue ? issueStatusWorkflow(workflows, workspaceProjects, issue) : null),
    [workflows, workspaceProjects],
  );
  const scopeProject = projectId ? workspaceProjects?.find((p) => p.id === projectId) ?? null : null;
  const activeWorkflow = workflowOfIssue(activeIssue);
  const activeStatusWorkflow = statusWorkflowOfIssue(activeIssue);
  const activeProject = activeIssue?.project_id
    ? workspaceProjects?.find((p) => p.id === activeIssue.project_id) ?? null
    : null;
  const activeHandlerLabel = useStepHandlerLabel(activeProject);
  const boardWorkflow = useMemo<BoardWorkflowContextValue>(
    () => ({
      scopeWorkflow: resolveProjectWorkflow(workflows, scopeProject),
      scopeProject,
      dropBlockedReason: (status) =>
        activeStatusWorkflow && activeIssue?.status !== status && !workflowAllowsStatus(activeStatusWorkflow, status)
          ? t(($) => $.workflows.not_in_workflow, { name: activeStatusWorkflow.name })
          : null,
      dropHint: (status) => {
        if (!activeWorkflow || activeIssue?.status === status) return null;
        const step = workflowStep(activeWorkflow, status);
        const name = activeHandlerLabel(step);
        if (!step || !name) return null;
        const actor = stepHandlerActor(step, activeProject);
        return { name, actor, run: actor ? actor.type !== "member" : step.handler.type === "creator" };
      },
    }),
    [activeHandlerLabel, activeIssue?.status, activeProject, activeStatusWorkflow, activeWorkflow, scopeProject, t, workflows],
  );
  // Shared drag/settle primitive: owns the local column mirror, the
  // dragging/settling locks, the post-move animation-frame throttle, and the
  // settle callback. Shared with list-view (and swimlane) so the surfaces
  // can't drift apart. Local columns follow TQ between drags via the resync
  // effect below; during a drag/settle they are frozen by the locks.
  const {
    columns,
    setColumns,
    columnsRef,
    isDraggingRef,
    isSettlingRef,
    recentlyMovedRef,
    settleVersion,
    beginSettle,
  } = useDragSettle(() => buildColumns(groupedIssues, groups, grouping, groupingOptionIds));

  useEffect(() => {
    if (!isDraggingRef.current && !isSettlingRef.current) {
      setColumns(buildColumns(groupedIssues, groups, grouping, groupingOptionIds));
    }
  }, [groupedIssues, groups, grouping, groupingOptionIds, settleVersion, setColumns, isDraggingRef, isSettlingRef]);

  // --- Issue map ---
  // Frozen during drag so BoardColumn/DraggableBoardCard props stay
  // referentially stable even if a TQ refetch lands mid-drag.
  const issueMap = useMemo(() => {
    const map = new Map<string, Issue>();
    for (const issue of groupedIssues) map.set(issue.id, issue);
    return map;
  }, [groupedIssues]);

  const issueMapRef = useRef(issueMap);
  if (!isDraggingRef.current && !isSettlingRef.current) {
    issueMapRef.current = issueMap;
  }

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 5 },
    })
  );

  // Side peek steps through a column with J / K, in the order shown here.
  const peek = useIssuePeekActions();
  useEffect(() => {
    peek?.publishColumns(groups.map((group) => columns[group.id] ?? EMPTY_IDS));
  }, [peek, groups, columns]);
  useEffect(() => () => peek?.publishColumns(null), [peek]);

  // #6700: drag empty board background with the left button to pan horizontally
  // (Trello/Linear). Card drags start on `[data-board-card]` and are ignored.
  const pan = useBoardDragPan<HTMLDivElement>();
  // Tab-session scroll restoration (MUL-4741) for the horizontal column
  // strip; each column's own vertical offset is restored by BoardColumn.
  const restoreScrollRef = useRestoredScrollRef("board");
  const attachScroller = useCallback(
    (el: HTMLDivElement | null) => {
      pan.ref.current = el;
      restoreScrollRef(el);
    },
    [pan.ref, restoreScrollRef],
  );

  const handleDragStart = useCallback(
    (event: DragStartEvent) => {
      isDraggingRef.current = true;
      const issue = issueMapRef.current.get(event.active.id as string) ?? null;
      setActiveIssue(issue);
    },
    [isDraggingRef],
  );

  const handleDragOver = useCallback(
    (event: DragOverEvent) => {
      const { active, over } = event;
      if (!over || recentlyMovedRef.current) return;

      const activeId = active.id as string;
      const overId = over.id as string;

      setColumns((prev) => {
        const activeCol = findColumn(prev, activeId, groupIds);
        const overCol = findColumn(prev, overId, groupIds);
        if (!activeCol || !overCol || activeCol === overCol) return prev;
        const targetStatus = groups.find((group) => group.id === overCol)?.status;
        if (targetStatus && catalog.entryOf(targetStatus)?.archived_at) return prev;
        const dragged = issueMapRef.current.get(activeId);
        if (
          targetStatus &&
          dragged?.status !== targetStatus &&
          !workflowAllowsStatus(statusWorkflowOfIssue(dragged), targetStatus)
        ) {
          return prev;
        }

        if (sortBy !== "position") return prev;

        recentlyMovedRef.current = true;
        const oldIds = prev[activeCol]!.filter((id) => id !== activeId);
        const newIds = [...prev[overCol]!];
        const overIndex = newIds.indexOf(overId);
        const insertIndex = overIndex >= 0 ? overIndex : newIds.length;
        newIds.splice(insertIndex, 0, activeId);
        return { ...prev, [activeCol]: oldIds, [overCol]: newIds };
      });
    },
    [groupIds, groups, catalog, sortBy, recentlyMovedRef, setColumns, statusWorkflowOfIssue],
  );

  const handleDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      isDraggingRef.current = false;
      setActiveIssue(null);

      const resetColumns = () =>
        setColumns(buildColumns(groupedIssues, groups, grouping, groupingOptionIds));

      if (!over) {
        resetColumns();
        return;
      }

      const activeId = active.id as string;
      const overId = over.id as string;

      const cols = columnsRef.current;
      const activeCol = findColumn(cols, activeId, groupIds);
      const overCol = findColumn(cols, overId, groupIds);
      if (!activeCol || !overCol) {
        resetColumns();
        return;
      }

      // Same-column reorder (manual sort only)
      let finalColumns = cols;
      if (activeCol === overCol && sortBy === "position") {
        const ids = cols[activeCol]!;
        const oldIndex = ids.indexOf(activeId);
        const newIndex = ids.indexOf(overId);
        if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
          const reordered = arrayMove(ids, oldIndex, newIndex);
          finalColumns = { ...cols, [activeCol]: reordered };
          setColumns(finalColumns);
        }
      }

      const finalCol = sortBy === "position"
        ? findColumn(finalColumns, activeId, groupIds)
        : overCol;
      if (!finalCol) {
        resetColumns();
        return;
      }
      const finalGroup = groupMap.get(finalCol);
      if (!finalGroup) {
        resetColumns();
        return;
      }

      const map = issueMapRef.current;
      if (finalGroup.status && map.get(activeId)?.status !== finalGroup.status && catalog.entryOf(finalGroup.status)?.archived_at) {
        resetColumns();
        return;
      }
      const movingWorkflow = workflowOfIssue(map.get(activeId));
      const movingStatusWorkflow = statusWorkflowOfIssue(map.get(activeId));
      if (
        finalGroup.status &&
        movingStatusWorkflow &&
        map.get(activeId)?.status !== finalGroup.status &&
        !workflowAllowsStatus(movingStatusWorkflow, finalGroup.status)
      ) {
        resetColumns();
        toast.info(
          t(($) => $.workflows.drop_blocked, {
            status: labelOf(finalGroup.status),
            name: movingStatusWorkflow.name,
          }),
        );
        return;
      }

      // Entering a step that hands the issue off is confirmed first: who it
      // goes to, the previous agent's run, and the brief. The card returns to
      // its column until then. (MUL-7420)
      const handoffTarget =
        finalGroup.status &&
        movingWorkflow &&
        map.get(activeId)?.status !== finalGroup.status &&
        stepHandsOff(workflowStep(movingWorkflow, finalGroup.status))
          ? finalGroup.status
          : null;

      if (sortBy !== "position") {
        // Cross-column: only update group (status/assignee), keep original position.
        const currentIssue = map.get(activeId);
        if (!currentIssue || issueMatchesGroup(currentIssue, finalGroup)) {
          resetColumns();
          if (activeId !== overId) {
            toast.info(t(($) => $.board.manual_reorder_hint), {
              id: "issue-manual-reorder-hint",
            });
          }
          return;
        }
        if (handoffTarget) {
          resetColumns();
          setPendingHandoff({
            issue: currentIssue,
            status: handoffTarget,
            updates: getMoveUpdates(finalGroup, currentIssue.position, currentIssue),
            anchors: getMoveAnchors(
              insertIdByPosition((cols[overCol] ?? []).filter((id) => id !== activeId), activeId, currentIssue.position, map),
              activeId,
            ),
          });
          return;
        }
        // Optimistically move the card into the target column *now*. Without
        // this, the sortBy != "position" path never touches local columns on
        // drop, so onDragOver having been a no-op leaves the card in its origin
        // column for the whole request — it only jumps across when the mutation
        // settles. That is the "snaps back to origin, then moves" glitch.
        // Placement mirrors the cache (insertByPosition) so the settle rebuild
        // from TanStack Query is a visual no-op.
        const targetIds = insertIdByPosition(
          (cols[overCol] ?? []).filter((id) => id !== activeId),
          activeId,
          currentIssue.position,
          map,
        );
        setColumns((prev) => {
          const fromIds = (prev[activeCol] ?? []).filter((cid) => cid !== activeId);
          return { ...prev, [activeCol]: fromIds, [overCol]: targetIds };
        });
        onMoveIssue(
          activeId,
          {
            ...getMoveUpdates(finalGroup, currentIssue.position, currentIssue),
            ...getMoveAnchors(targetIds, activeId),
          },
          beginSettle(),
        );
        applyPropertyGroupValue(finalGroup, activeId);
        return;
      }

      const finalIds = finalColumns[finalCol]!;
      const newPosition = computePosition(finalIds, activeId, map);
      const currentIssue = map.get(activeId);

      if (
        currentIssue &&
        issueMatchesGroup(currentIssue, finalGroup) &&
        currentIssue.position === newPosition
      ) {
        return;
      }

      if (handoffTarget && currentIssue) {
        resetColumns();
        setPendingHandoff({
          issue: currentIssue,
          status: handoffTarget,
          updates: getMoveUpdates(finalGroup, newPosition, currentIssue),
          anchors: getMoveAnchors(finalIds, activeId),
        });
        return;
      }

      // beginSettle() holds the lock and returns the onSettled callback that
      // releases it and resyncs local columns from the cache: a no-op on
      // success (onSuccess already patched the moved card in place), the revert
      // on error (onError restored the snapshot). Without it a failed move would
      // strand the card at the drop target, since onSettled no longer refetches.
      onMoveIssue(
        activeId,
        {
          ...getMoveUpdates(finalGroup, newPosition, currentIssue),
          ...getMoveAnchors(finalIds, activeId),
        },
        beginSettle(),
      );
      applyPropertyGroupValue(finalGroup, activeId);
    },
    [groupedIssues, groups, grouping, groupingOptionIds, onMoveIssue, groupIds, groupMap, sortBy, beginSettle, columnsRef, isDraggingRef, setColumns, applyPropertyGroupValue, catalog, labelOf, t, workflowOfIssue, statusWorkflowOfIssue],
  );

  // An aborted drag (pointercancel, window resize, tab hide, Escape) fires
  // onDragCancel instead of onDragEnd. Releasing the drag lock here keeps the
  // column mirror resyncing with the cache afterwards — see the same handler in
  // list-view for the touch path that makes this routine (MUL-6240).
  const handleDragCancel = useCallback(() => {
    isDraggingRef.current = false;
    setActiveIssue(null);
    setColumns(buildColumns(groupedIssues, groups, grouping, groupingOptionIds));
  }, [groupedIssues, groups, grouping, groupingOptionIds, setColumns, isDraggingRef]);

  return (
    <BoardWorkflowContext.Provider value={boardWorkflow}>
    <DndContext
      sensors={sensors}
      collisionDetection={collisionDetection}
      onDragStart={handleDragStart}
      onDragOver={handleDragOver}
      onDragEnd={handleDragEnd}
      onDragCancel={handleDragCancel}
    >
      <div
        ref={attachScroller}
        onPointerDown={pan.onPointerDown}
        onPointerMove={pan.onPointerMove}
        onPointerUp={pan.onPointerUp}
        onPointerCancel={pan.onPointerCancel}
        onLostPointerCapture={pan.onLostPointerCapture}
        data-board-scroller=""
        data-tab-scroll-root="board"
        className={cn(
          "flex flex-1 min-h-0 gap-4 overflow-x-auto p-2",
          // While a side peek is open, a trailing spacer as wide as the panel
          // lets every column scroll clear of it (IssuePeekHost).
          "group-data-[peek-open]/peek:after:w-(--issue-peek-width) group-data-[peek-open]/peek:after:shrink-0 group-data-[peek-open]/peek:after:content-['']",
        )}
      >
        {groups.length === 0 ? (
          groupBranches?.isError ? (
            <button
              type="button"
              className="flex min-w-full flex-1 items-center justify-center text-body text-destructive hover:underline"
              onClick={groupBranches.retryGroups}
            >
              {t(($) => $.table.load_more_failed_retry)}
            </button>
          ) : (
            <div className="flex min-w-full flex-1 items-center justify-center text-body text-muted-foreground">
              {t(($) => $.board.empty_grouping)}
            </div>
          )
        ) : (
          groups.map((group) =>
            isStatusGroup(group) ? (
              <ServerPaginatedBoardColumn
                key={group.id}
                group={group}
                issueIds={columns[group.id] ?? EMPTY_IDS}
                issueMap={issueMapRef.current}
                childProgressMap={childProgressMap}
                projectMap={projectMap}
                page={statusPagination?.[group.status]}
                projectId={projectId}
                onCreateIssue={onCreateIssue}
                sortLabel={sortLabel}
              />
            ) : (
              groupPagination?.[group.id] ? (
                <ServerPaginatedBoardColumn
                  key={group.id}
                  group={group}
                  issueIds={columns[group.id] ?? EMPTY_IDS}
                  issueMap={issueMapRef.current}
                  childProgressMap={childProgressMap}
                  projectMap={projectMap}
                  page={groupPagination[group.id]!}
                  projectId={projectId}
                  onCreateIssue={onCreateIssue}
                  sortLabel={sortLabel}
                />
              ) : (
                <BoardColumn
                  key={group.id}
                  group={group}
                  issueIds={columns[group.id] ?? EMPTY_IDS}
                  issueMap={issueMapRef.current}
                  childProgressMap={childProgressMap}
                  projectMap={projectMap}
                  projectId={projectId}
                  onCreateIssue={onCreateIssue}
                  totalCount={group.totalCount}
                  sortLabel={sortLabel}
                />
              )
            ),
          )
        )}
        {groupBranches?.hasMoreGroups && (
          <div className="flex w-8 shrink-0 items-center justify-center">
            <InfiniteScrollSentinel
              onVisible={groupBranches.loadMoreGroups}
              loading={groupBranches.isLoadingMoreGroups}
            />
          </div>
        )}
      </div>

      <DragOverlay dropAnimation={null}>
        {activeIssue ? (
          <div style={{ width: BOARD_CARD_WIDTH }} className="rotate-1 cursor-grabbing opacity-90 shadow-lg shadow-black/10">
            <BoardCardContent
              issue={activeIssue}
              childProgress={childProgressMap.get(activeIssue.id)}
              project={
                activeIssue.project_id
                  ? projectMap?.get(activeIssue.project_id)
                  : undefined
              }
            />
          </div>
        ) : null}
      </DragOverlay>
    </DndContext>
    <WorkflowHandoffConfirmDialog
      issue={pendingHandoff?.issue ?? null}
      toStatus={pendingHandoff?.status ?? ""}
      onCancel={() => setPendingHandoff(null)}
      onConfirm={({ stopPreviousRuns }) => {
        const pending = pendingHandoff;
        setPendingHandoff(null);
        if (!pending) return;
        onMoveIssue(
          pending.issue.id,
          {
            ...pending.updates,
            ...pending.anchors,
            ...(stopPreviousRuns ? { stop_previous_assignee_runs: true } : {}),
          },
          beginSettle(),
        );
      }}
    />
    </BoardWorkflowContext.Provider>
  );
}


const ServerPaginatedBoardColumn = memo(function ServerPaginatedBoardColumn({
  group,
  issueIds,
  issueMap,
  childProgressMap,
  projectMap,
  page,
  projectId,
  onCreateIssue,
  sortLabel,
}: {
  group: BoardColumnGroup;
  issueIds: string[];
  issueMap: Map<string, Issue>;
  childProgressMap?: Map<string, ChildProgress>;
  projectMap?: Map<string, Project>;
  page?: IssueStatusPageState | IssueGroupPageState;
  projectId?: string;
  onCreateIssue?: (defaults: IssueCreateDefaults) => void;
  sortLabel?: string | null;
}) {
  const footer = page ? (
    <ListLoadMoreFooter
      hasMore={page.hasMore}
      isLoading={page.isLoading || page.isFetching}
      total={page.total}
      onLoadMore={page.loadMore}
      isError={page.isError}
      onRetry={page.retry}
    />
  ) : undefined;
  return (
    <BoardColumn
      group={group}
      issueIds={issueIds}
      issueMap={issueMap}
      childProgressMap={childProgressMap}
      projectMap={projectMap}
      totalCount={page?.total ?? group.totalCount}
      projectId={projectId}
      onCreateIssue={onCreateIssue}
      sortLabel={sortLabel}
      footer={footer}
    />
  );
});


/**
 * Memoized: the surface controller re-renders on loading-flag flips (e.g. a
 * query enabling when the view changes) — without memo every such flip
 * re-rendered this entire view tree (hundreds of ms). All props are
 * referentially stable useMemo/useCallback outputs from the controller.
 */
export const BoardView = memo(BoardViewImpl);
