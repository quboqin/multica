import { useQueryClient } from "@tanstack/react-query";
import { useWSSubscriptions } from "@/lib/use-ws-subscriptions";
import { collectionKeys, documentKeys } from "@/data/queries/resources";
import { issueKeys } from "@/data/queries/issue-keys";

export function useResourcesRealtime() {
  const qc = useQueryClient();
  useWSSubscriptions((ws, wsId) => {
    const refreshDocuments = () => qc.invalidateQueries({ queryKey: documentKeys.all(wsId) });
    const refreshCollections = () => qc.invalidateQueries({ queryKey: collectionKeys.all(wsId) });
    return [
      ws.on("issue:created", ({ issue }) => { if (issue.kind === "doc") void refreshDocuments(); }),
      ws.on("issue:updated", ({ issue }) => { if (issue.kind === "doc") void refreshDocuments(); }),
      ws.on("issue:deleted", () => { void refreshDocuments(); }),
      ws.on("document:access_changed", ({ document_id }) => {
        void refreshDocuments();
        void qc.invalidateQueries({ queryKey: documentKeys.access(wsId, document_id) });
        void qc.invalidateQueries({ queryKey: issueKeys.detail(wsId, document_id) });
      }),
      ws.on("collection:updated", refreshCollections),
      ws.on("collection:access_changed", refreshCollections),
      ws.on("record:updated", (payload) => {
        if (!payload || typeof payload !== "object" || !("collection_id" in payload) || typeof payload.collection_id !== "string") return;
        void qc.invalidateQueries({ queryKey: collectionKeys.records(wsId, payload.collection_id) });
      }),
      ws.onReconnect(() => { void refreshDocuments(); void refreshCollections(); }),
    ];
  }, [qc]);
}
