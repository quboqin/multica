"use client";
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { issueViewDetailOptions } from "@multica/core/issue-views/queries";
import { baselineFromQuery } from "@multica/core/issue-views/baseline";
import {
  createIssueViewStore,
  mergeViewStatePersisted,
} from "@multica/core/issues/stores/view-store";
import { IssueSurfaceWithStore } from "../issues/surface/issue-surface";
import { EmbeddedCollectionView } from "../collections/collection-detail-page";
import { useT } from "../i18n";

export default function EmbeddedSavedView({ viewId }: { viewId: string }) {
  const user = useAuthStore((state) => state.user);
  const wsId = useWorkspaceId();
  const { t } = useT("issues");
  const { data: view, error } = useQuery(issueViewDetailOptions(wsId, viewId));
  const store = useMemo(() => {
    const result = createIssueViewStore(`embed:${wsId}:${viewId}`);
    if (view)
      result.setState(mergeViewStatePersisted(view.display, result.getState()));
    return result;
  }, [wsId, viewId, view]);
  if (error || !view)
    return (
      <p className="p-3" role={error ? "alert" : "status"}>
        {error ? error.message : t(($) => $.cortex.loading)}
      </p>
    );
  if (view.collection_id)
    return <EmbeddedCollectionView id={view.collection_id} view={view} />;
  const scope =
    view.scope_type === "project" && view.scope_id
      ? { type: "project" as const, projectId: view.scope_id }
      : view.scope_type === "my"
        ? {
            type: "my" as const,
            relation: "all" as const,
            userId: user?.id ?? "",
          }
        : { type: "workspace" as const, actorKind: "all" as const };
  return (
    <div className="flex h-[420px] min-h-0 flex-col">
      <IssueSurfaceWithStore
        store={store}
        baseline={baselineFromQuery(view.query)}
        scope={scope}
        modes={["table", "board", "list", "calendar", "gallery"]}
      />
    </div>
  );
}
