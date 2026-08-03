import {
  useMutation,
  useQueryClient,
  type InfiniteData,
  type QueryClient,
} from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { chatKeys } from "./queries";
import { createLogger } from "../logger";
import type {
  ChatMessage,
  ChatMessageFeedback,
  ChatMessageFeedbackSentiment,
  ChatMessagesPage,
  ChatSession,
} from "../types";

const logger = createLogger("chat.mut");

export type { ChatMessageFeedbackSentiment } from "../types";

export interface UpsertChatMessageFeedbackInput {
  sessionId: string;
  messageId: string;
  sentiment: ChatMessageFeedbackSentiment | null;
  comment: string;
}

function patchMessageFeedback(
  messages: ChatMessage[] | undefined,
  messageId: string,
  feedback: ChatMessageFeedback | undefined,
): ChatMessage[] | undefined {
  return messages?.map((message) =>
    message.id === messageId ? { ...message, feedback } : message,
  );
}

function setMessageFeedback(
  qc: QueryClient,
  sessionId: string,
  messageId: string,
  feedback: ChatMessageFeedback | undefined,
) {
  qc.setQueryData<ChatMessage[] | undefined>(
    chatKeys.messages(sessionId),
    (messages) => patchMessageFeedback(messages, messageId, feedback),
  );
  qc.setQueryData<InfiniteData<ChatMessagesPage> | undefined>(
    chatKeys.messagesPage(sessionId),
    (data) => data && ({
      ...data,
      pages: data.pages.map((page) => ({
        ...page,
        messages: patchMessageFeedback(page.messages, messageId, feedback) ?? [],
      })),
    }),
  );
}

export function useUpsertChatMessageFeedback() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ sessionId, messageId, sentiment, comment }: UpsertChatMessageFeedbackInput) =>
      api.upsertChatMessageFeedback(sessionId, messageId, { sentiment, comment }),
    onMutate: async (input) => {
      await Promise.all([
        qc.cancelQueries({ queryKey: chatKeys.messages(input.sessionId) }),
        qc.cancelQueries({ queryKey: chatKeys.messagesPage(input.sessionId) }),
      ]);
      const previousMessages = qc.getQueryData<ChatMessage[]>(chatKeys.messages(input.sessionId));
      const previousPages = qc.getQueryData<InfiniteData<ChatMessagesPage>>(
        chatKeys.messagesPage(input.sessionId),
      );
      const feedback = input.sentiment === null && input.comment === ""
        ? undefined
        : { sentiment: input.sentiment, comment: input.comment };
      setMessageFeedback(qc, input.sessionId, input.messageId, feedback);
      return { previousMessages, previousPages };
    },
    onError: (_error, input, context) => {
      qc.setQueryData(chatKeys.messages(input.sessionId), context?.previousMessages);
      qc.setQueryData(chatKeys.messagesPage(input.sessionId), context?.previousPages);
    },
    onSettled: (_data, _error, input) => {
      qc.invalidateQueries({ queryKey: chatKeys.messages(input.sessionId) });
      qc.invalidateQueries({ queryKey: chatKeys.messagesPage(input.sessionId) });
    },
  });
}

export function useCreateChatSession() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (data: { agent_id: string; title?: string }) => {
      logger.info("createChatSession.start", { agent_id: data.agent_id, titleLength: data.title?.length ?? 0 });
      return api.createChatSession(data);
    },
    onSuccess: (session) => {
      logger.info("createChatSession.success", { sessionId: session.id, agentId: session.agent_id });
    },
    onError: (err) => {
      logger.error("createChatSession.error", err);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
    },
  });
}

/**
 * Clears the session's unread state server-side. Optimistically flips
 * has_unread to false in the cached list so the FAB badge drops
 * immediately. The server broadcasts chat:session_read so other devices
 * also sync.
 */
export function useMarkChatSessionRead() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (sessionId: string) => {
      logger.info("markChatSessionRead.start", { sessionId });
      return api.markChatSessionRead(sessionId);
    },
    onMutate: async (sessionId) => {
      await qc.cancelQueries({ queryKey: chatKeys.sessions(wsId) });

      const prevSessions = qc.getQueryData<ChatSession[]>(chatKeys.sessions(wsId));

      const clear = (old?: ChatSession[]) =>
        old?.map((s) => (s.id === sessionId ? { ...s, has_unread: false } : s));
      qc.setQueryData<ChatSession[]>(chatKeys.sessions(wsId), clear);

      return { prevSessions };
    },
    onError: (err, sessionId, ctx) => {
      logger.error("markChatSessionRead.error.rollback", { sessionId, err });
      if (ctx?.prevSessions) qc.setQueryData(chatKeys.sessions(wsId), ctx.prevSessions);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
    },
  });
}

/**
 * Renames a chat session. Optimistically swaps the title in the cached
 * list so the dropdown reflects the new label immediately; rolls back on
 * error. The matching `chat:session_updated` WS event keeps other
 * tabs/devices in sync — see use-realtime-sync.ts.
 */
export function useUpdateChatSession() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (data: { sessionId: string; title: string }) => {
      logger.info("updateChatSession.start", {
        sessionId: data.sessionId,
        titleLength: data.title.length,
      });
      return api.updateChatSession(data.sessionId, { title: data.title });
    },
    onMutate: async ({ sessionId, title }) => {
      await qc.cancelQueries({ queryKey: chatKeys.sessions(wsId) });

      const prevSessions = qc.getQueryData<ChatSession[]>(chatKeys.sessions(wsId));

      const patch = (old?: ChatSession[]) =>
        old?.map((s) => (s.id === sessionId ? { ...s, title } : s));
      qc.setQueryData<ChatSession[]>(chatKeys.sessions(wsId), patch);

      return { prevSessions };
    },
    onError: (err, vars, ctx) => {
      logger.error("updateChatSession.error.rollback", { sessionId: vars.sessionId, err });
      if (ctx?.prevSessions) qc.setQueryData(chatKeys.sessions(wsId), ctx.prevSessions);
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
    },
  });
}

/**
 * Deletes a chat session. Optimistically removes the row from the
 * sessions list so the dropdown updates instantly; rolls back on error.
 * The matching `chat:session_deleted` WS event keeps other tabs/devices
 * in sync — see use-realtime-sync.ts.
 */
export function useDeleteChatSession() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();

  return useMutation({
    mutationFn: (sessionId: string) => {
      logger.info("deleteChatSession.start", { sessionId });
      return api.deleteChatSession(sessionId);
    },
    onMutate: async (sessionId) => {
      await qc.cancelQueries({ queryKey: chatKeys.sessions(wsId) });

      const prevSessions = qc.getQueryData<ChatSession[]>(chatKeys.sessions(wsId));

      const drop = (old?: ChatSession[]) => old?.filter((s) => s.id !== sessionId);
      qc.setQueryData<ChatSession[]>(chatKeys.sessions(wsId), drop);

      logger.debug("deleteChatSession.optimistic", { sessionId });
      return { prevSessions };
    },
    onError: (err, sessionId, ctx) => {
      logger.error("deleteChatSession.error.rollback", { sessionId, err });
      if (ctx?.prevSessions) qc.setQueryData(chatKeys.sessions(wsId), ctx.prevSessions);
    },
    onSettled: (_data, _err, sessionId) => {
      logger.debug("deleteChatSession.settled", { sessionId });
      qc.invalidateQueries({ queryKey: chatKeys.sessions(wsId) });
    },
  });
}
