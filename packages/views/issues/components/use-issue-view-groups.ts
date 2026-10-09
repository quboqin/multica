"use client";

import { useMemo, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  isFilterablePropertyType,
  type Issue,
  type IssueStatus,
  type IssueAssigneeType,
  type IssueProperty,
  type Project,
} from "@multica/core/types";
import { useWorkspaceId } from "@multica/core/hooks";
import { useActorName } from "@multica/core/workspace/hooks";
import {
  propertyGroupLabel,
  propertyListOptions,
  useSetIssueProperty,
  useUnsetIssueProperty,
} from "@multica/core/properties";
import { useViewStore } from "@multica/core/issues/stores/view-store-context";
import { propertyIdFromViewKey, type IssueGrouping } from "@multica/core/issues/stores/view-store";
import { useT } from "../../i18n";
import type { BoardColumnGroup } from "./board-column";
import type { IssueGroupBranches, IssueGroupPageState } from "../surface/use-issue-group-branches";
import { statusGroupId, propertyGroupId, projectGroupId, assigneeGroupId } from "../utils/drag-utils";

interface ProjectColumnLabels {
  noProject: string;
  /** A project id the projects query cannot resolve — deleted, or not visible
   *  to this member. Shares the Table's wording so one board column and one
   *  table group row never describe the same project differently. */
  unavailableProject: string;
}

interface BuildGroupsContext extends ProjectColumnLabels {
  getActorName: (type: string, id: string) => string;
  groupingProperty: IssueProperty | null;
  projectMap: Map<string, Project> | undefined;
  noAssigneeLabel: string;
  noValueLabel: string;
}

/**
 * One project column. Shared by the client fallback (columns derived from
 * loaded cards) and the server path (columns derived from group descriptors)
 * so the two can never describe the same project differently.
 */
function projectColumn(
  id: string,
  projectId: string | null,
  projectMap: Map<string, Project> | undefined,
  labels: ProjectColumnLabels,
  totalCount?: number,
): BoardColumnGroup {
  const project = projectId ? projectMap?.get(projectId) ?? null : null;
  return {
    id,
    title: projectId
      ? project?.title ?? labels.unavailableProject
      : labels.noProject,
    projectId,
    project,
    totalCount,
    createData: { project_id: projectId },
  };
}

/**
 * Keep the "No project" column present as a drop target — clearing a card's
 * project by dragging has to stay possible even in a workspace where every
 * card currently has one. A board with no columns at all is left alone: that
 * is the surface's empty state, not a board missing one column.
 */
function withNoProjectColumn(
  columns: BoardColumnGroup[],
  projectMap: Map<string, Project> | undefined,
  labels: ProjectColumnLabels,
): BoardColumnGroup[] {
  if (columns.length === 0) return columns;
  if (columns.some((column) => column.projectId === null)) return columns;
  // No-project sorts first server-side, so it is always in the first page of
  // descriptors when it exists — an absent one cannot arrive with a later page.
  return [
    projectColumn(projectGroupId(null), null, projectMap, labels, 0),
    ...columns,
  ];
}

function buildGroups(
  issues: Issue[],
  visibleStatuses: IssueStatus[],
  grouping: IssueGrouping,
  {
    getActorName,
    groupingProperty,
    projectMap,
    noAssigneeLabel,
    noValueLabel,
    ...projectLabels
  }: BuildGroupsContext,
): BoardColumnGroup[] {
  if (grouping === "status") {
    return visibleStatuses.map((status) => ({
      id: statusGroupId(status),
      title: status,
      status,
      createData: { status: status },
    }));
  }

  // Select-property board: one column per option (definition order) plus a
  // trailing "No value" column. Empty columns stay visible — they are drop
  // targets for assigning the value.
  if (groupingProperty) {
    const columns: BoardColumnGroup[] = (groupingProperty.config.options ?? []).map(
      (option) => ({
        id: propertyGroupId(groupingProperty.id, option.id),
        title: option.name,
        propertyId: groupingProperty.id,
        propertyOptionId: option.id,
        createData: { properties: { [groupingProperty.id]: option.id } },
        propertyOptionColor: option.color,
      }),
    );
    columns.push({
      id: propertyGroupId(groupingProperty.id, null),
      title: noValueLabel,
      propertyId: groupingProperty.id,
      propertyOptionId: null,
    });
    return columns;
  }

  // Project board: one column per project the loaded cards reference, plus the
  // "No project" column. Ordering mirrors the server's group order (no-project
  // first, then project title) so the client fallback and the paged server
  // columns cannot disagree.
  if (grouping === "project") {
    const columns = new Map<string, BoardColumnGroup>();
    for (const issue of issues) {
      const projectId = issue.project_id ?? null;
      const id = projectGroupId(projectId);
      if (columns.has(id)) continue;
      columns.set(id, projectColumn(id, projectId, projectMap, projectLabels));
    }
    const ordered = Array.from(columns.values()).toSorted((a, b) => {
      if (a.projectId === null) return b.projectId === null ? 0 : -1;
      if (b.projectId === null) return 1;
      return a.title.localeCompare(b.title);
    });
    return withNoProjectColumn(ordered, projectMap, projectLabels);
  }

  const groups = new Map<string, BoardColumnGroup>();
  for (const issue of issues) {
    const id = assigneeGroupId(issue.assignee_type, issue.assignee_id);
    if (groups.has(id)) continue;

    if (issue.assignee_type && issue.assignee_id) {
      groups.set(id, {
        id,
        title: getActorName(issue.assignee_type, issue.assignee_id),
        assigneeType: issue.assignee_type,
        assigneeId: issue.assignee_id,
        createData: {
          assignee_type: issue.assignee_type,
          assignee_id: issue.assignee_id,
        },
      });
      continue;
    }

    groups.set(id, {
      id,
      title: noAssigneeLabel,
      assigneeType: null,
      assigneeId: null,
      createData: {
        assignee_type: null,
        assignee_id: null,
      },
    });
  }

  const order: Record<string, number> = {
    member: 0,
    agent: 1,
    squad: 2,
    none: 3,
  };

  return Array.from(groups.values()).toSorted((a, b) => {
    const aOrder = order[a.assigneeType ?? "none"] ?? 99;
    const bOrder = order[b.assigneeType ?? "none"] ?? 99;
    if (aOrder !== bOrder) return aOrder - bOrder;
    return a.title.localeCompare(b.title);
  });
}

/** Shared grouping, labels, pagination and property writes for Board and List. */
export function useIssueViewGroups({ issues, visibleStatuses, projectMap, groupBranches }: {
  issues: Issue[];
  visibleStatuses: IssueStatus[];
  projectMap?: Map<string, Project>;
  groupBranches?: IssueGroupBranches;
}) {
  const { t } = useT("issues");
  const storeGrouping = useViewStore((s) => s.grouping);
  const sortBy = useViewStore((s) => s.sortBy);
  const boardWsId = useWorkspaceId();
  const { data: workspaceProperties = [] } = useQuery(propertyListOptions(boardWsId));
  const groupingPropertyId = propertyIdFromViewKey(storeGrouping);
  const groupingProperty = groupingPropertyId
    ? workspaceProperties.find((p) => p.id === groupingPropertyId && isFilterablePropertyType(p.type)) ?? null
    : null;
  // A persisted `property:<id>` grouping whose definition is gone (archived,
  // deleted, other workspace) falls back to status columns.
  const grouping: IssueGrouping =
    groupingPropertyId && !groupingProperty ? "status" : storeGrouping;
  const groupingOptionIds = useMemo(
    () =>
      groupingProperty?.type === "select"
        ? new Set((groupingProperty.config.options ?? []).map((option) => option.id))
        : undefined,
    [groupingProperty],
  );
  const setIssuePropertyMutation = useSetIssueProperty();
  const unsetIssuePropertyMutation = useUnsetIssueProperty();
  const applyPropertyGroupValue = useCallback(
    (group: BoardColumnGroup, issueId: string) => {
      if (group.propertyId === undefined) return;
      // Surface failures like status/assignee drags do (use-issue-surface-
      // actions): the mutation rolls the card back, but without a toast the
      // snap-back reads as a UI glitch instead of a rejected write.
      const onError = (err: unknown) => {
        toast.error(
          err instanceof Error && err.message
            ? err.message
            : t(($) => $.page.move_failed),
        );
      };
      if (group.propertyOptionId === null) {
        unsetIssuePropertyMutation.mutate(
          { issueId, propertyId: group.propertyId },
          { onError },
        );
      } else if (group.propertyOptionId !== undefined) {
        setIssuePropertyMutation.mutate(
          {
            issueId,
            propertyId: group.propertyId,
            value: group.propertyOptionId,
          },
          { onError },
        );
      }
    },
    [setIssuePropertyMutation, t, unsetIssuePropertyMutation],
  );
  const sortFieldKey = sortBy === "created_at" ? "created" : sortBy;
  const sortPropertyId = propertyIdFromViewKey(sortBy);
  const sortLabel = sortBy !== "position"
    ? t(($) => $.board.ordered_by, {
        field: sortPropertyId
          ? workspaceProperties.find((p) => p.id === sortPropertyId)?.name ?? ""
          : t(($) => $.display[`sort_${sortFieldKey}` as keyof typeof $.display]),
      })
    : null;
  const { getActorName } = useActorName();
  const groupedIssues = useMemo(
    () => (groupBranches?.enabled ? groupBranches.issues : issues),
    [groupBranches, issues],
  );
  const hydratedAssigneeGroups = useMemo<BoardColumnGroup[] | undefined>(() => {
    if (grouping === "assignee" && groupBranches?.enabled) {
      return groupBranches.descriptors.flatMap((descriptor): BoardColumnGroup[] => {
        if (descriptor.value.kind !== "assignee") return [];
        const actorRef = descriptor.value.actor;
        const actor: { type: IssueAssigneeType; id: string } | null =
          actorRef &&
          (actorRef.type === "member" ||
            actorRef.type === "agent" ||
            actorRef.type === "squad")
            ? { type: actorRef.type, id: actorRef.id }
            : null;
        return [{
          id: descriptor.key,
          title: actor
            ? getActorName(actor.type, actor.id)
            : t(($) => $.filters.no_assignee),
          assigneeType: actor?.type ?? null,
          assigneeId: actor?.id ?? null,
          totalCount: descriptor.count,
          createData: {
            assignee_type: actor?.type ?? null,
            assignee_id: actor?.id ?? null,
          },
        }];
      });
    }
    return undefined;
  }, [getActorName, groupBranches, grouping, t]);
  const projectColumnLabels = useMemo<ProjectColumnLabels>(
    () => ({
      noProject: t(($) => $.swimlane.no_project),
      unavailableProject: t(($) => $.table.value_unavailable),
    }),
    [t],
  );
  const hydratedProjectGroups = useMemo<BoardColumnGroup[] | undefined>(() => {
    if (grouping !== "project" || !groupBranches?.enabled) return undefined;
    const columns = groupBranches.descriptors.flatMap(
      (descriptor): BoardColumnGroup[] =>
        descriptor.value.kind === "project"
          ? [
              projectColumn(
                // The descriptor key, not our own: it is what `groupPagination`
                // is keyed by. `projectGroupId` reproduces it exactly, which is
                // what lets cards bucket into these columns at all.
                descriptor.key,
                descriptor.value.project_id ?? null,
                projectMap,
                projectColumnLabels,
                descriptor.count,
              ),
            ]
          : [],
    );
    return withNoProjectColumn(columns, projectMap, projectColumnLabels);
  }, [groupBranches, grouping, projectColumnLabels, projectMap]);
  const groupPagination = useMemo(() => {
    if (!groupBranches?.enabled) return undefined;
    const grouped = new Map<string, IssueGroupPageState[]>();
    for (const descriptor of groupBranches.descriptors) {
      const page = groupBranches.pagination[descriptor.key];
      if (!page) continue;
      let id = descriptor.key;
      if (descriptor.value.kind === "property") {
        const value = descriptor.value;
        id =
          value.value_state === "value"
            ? propertyGroupId(value.property_id, value.value ?? null)
            : propertyGroupId(value.property_id, null);
      }
      const pages = grouped.get(id) ?? [];
      pages.push(page);
      grouped.set(id, pages);
    }
    return Object.fromEntries(
      Array.from(grouped, ([id, pages]) => [
        id,
        {
          total: pages.reduce((sum, page) => sum + page.total, 0),
          loaded: pages.reduce((sum, page) => sum + page.loaded, 0),
          hasMore: pages.some((page) => page.hasMore),
          isLoading: pages.some((page) => page.isLoading),
          isFetching: pages.some((page) => page.isFetching),
          isError: pages.some((page) => page.isError),
          loadMore: () => {
            for (const page of pages) {
              if (page.hasMore) page.loadMore();
            }
          },
          retry: () => {
            for (const page of pages) {
              if (page.isError) page.retry();
            }
          },
        },
      ]),
    ) as Record<string, IssueGroupPageState>;
  }, [groupBranches]);
  const hydratedPropertyGroups = useMemo<BoardColumnGroup[] | undefined>(() => {
    if (!groupingProperty || groupingProperty.type === "select" || !groupBranches?.enabled) return undefined;
    return groupBranches.descriptors.flatMap((descriptor): BoardColumnGroup[] => {
      const value = descriptor.value;
      if (value.kind !== "property") return [];
      const fieldValue = value.value_state === "value" ? value.value ?? null : null;
      return [{
        id: propertyGroupId(value.property_id, fieldValue),
        title: fieldValue === null ? t(($) => $.table.no_value)
          : typeof fieldValue === "boolean" ? (fieldValue ? t(($) => $.pickers.custom_property.true_label) : t(($) => $.pickers.custom_property.false_label))
          : propertyGroupLabel(fieldValue, groupingProperty.config.options ?? [], getActorName, groupingProperty.type),
        propertyId: value.property_id,
        propertyOptionId: fieldValue,
        createData: fieldValue === null ? undefined : { properties: { [value.property_id]: fieldValue } },
        totalCount: descriptor.count,
      }];
    });
  }, [groupingProperty, groupBranches, getActorName, t]);
  const groups = useMemo(
    () => {
      const built =
        hydratedPropertyGroups ??
        hydratedAssigneeGroups ??
        hydratedProjectGroups ??
        buildGroups(issues, visibleStatuses, grouping, {
          getActorName,
          groupingProperty,
          projectMap,
          noAssigneeLabel: t(($) => $.filters.no_assignee),
          noValueLabel: t(($) => $.board.no_value),
          ...projectColumnLabels,
        });
      return built.map((group) => ({
        ...group,
        totalCount: groupPagination?.[group.id]?.total ?? group.totalCount,
      }));
    },
    [hydratedPropertyGroups, hydratedAssigneeGroups, hydratedProjectGroups, issues, visibleStatuses, grouping, getActorName, groupingProperty, projectMap, projectColumnLabels, groupPagination, t],
  );
  return { groups, groupedIssues, grouping, groupingOptionIds, groupPagination, applyPropertyGroupValue, sortLabel };
}
