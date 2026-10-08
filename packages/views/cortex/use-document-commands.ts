"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@multica/core/api";
import { documentKeys } from "@multica/core/documents";
import { issueKeys } from "@multica/core/issues/queries";
import { useWorkspacePaths } from "@multica/core/paths";
import type { Issue } from "@multica/core/types";
import { useNavigation } from "../navigation";
import { useT } from "../i18n";

/**
 * Runs a document command (create / move / transition) and folds the returned
 * document into the detail cache without regressing to an older revision.
 */
export function useDocumentCommand(wsId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (action: () => Promise<Issue>) => action(),
    onSuccess: (doc) => {
      queryClient.setQueryData<Issue>(issueKeys.detail(wsId, doc.id), (old) =>
        !old || (old.revision ?? 0) <= (doc.revision ?? 0) ? doc : old,
      );
      void queryClient.invalidateQueries({ queryKey: documentKeys.all(wsId) });
    },
  });
}

/** Creates an untitled page (optionally under `parent`) and opens it. */
export function useCreateDocument(wsId: string) {
  const command = useDocumentCommand(wsId);
  const navigation = useNavigation();
  const paths = useWorkspacePaths();
  const { t } = useT("issues");
  const create = async (options: {
    parent?: Pick<Issue, "id" | "project_id">;
    projectId?: string;
  }) => {
    const doc = await command.mutateAsync(() =>
      api.createIssue({
        kind: "doc",
        title: t(($) => $.cortex_docs.untitled),
        parent_issue_id: options.parent?.id,
        project_id:
          options.parent?.project_id ?? (options.projectId || undefined),
      }),
    );
    navigation.push(paths.documentDetail(doc.id));
    return doc;
  };
  return { create, isPending: command.isPending, error: command.error };
}
