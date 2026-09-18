import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { issueKeys } from "../issues/queries";
import {
  assertClientWorkspaceAccessAllowed,
  captureClientSessionGeneration,
  captureClientWorkspaceAccessGeneration,
  isClientSessionGenerationCurrent,
  isClientWorkspaceAccessGenerationCurrent,
} from "../platform";
import { documentKeys } from "./data-source";

export function useSaveDocument(workspaceId: string, workspaceSlug: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id?: string;
      revision?: number;
      title: string;
      description: string;
    }) => {
      assertClientWorkspaceAccessAllowed(queryClient, workspaceId);
      const session = captureClientSessionGeneration(queryClient);
      const access = captureClientWorkspaceAccessGeneration(
        queryClient,
        workspaceId,
      );
      const issue = input.id
        ? await api.updateDocument(
            input.id,
            {
              title: input.title,
              description: input.description,
              expected_revision: input.revision!,
            },
            workspaceSlug,
          )
        : await api.createDocument(
            input.title,
            input.description,
            workspaceSlug,
          );
      if (
        issue.workspace_id !== workspaceId ||
        !isClientSessionGenerationCurrent(queryClient, session) ||
        !isClientWorkspaceAccessGenerationCurrent(
          queryClient,
          workspaceId,
          access,
        )
      ) {
        throw new Error("Document access expired");
      }
      return issue;
    },
    onSuccess: (issue) => {
      queryClient.setQueryData(issueKeys.detail(workspaceId, issue.id), issue);
      return queryClient.invalidateQueries({
        queryKey: documentKeys.rows(workspaceId),
      });
    },
  });
}
