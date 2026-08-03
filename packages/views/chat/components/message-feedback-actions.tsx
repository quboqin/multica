"use client";

import { useState } from "react";
import { toast } from "sonner";
import { MessageSquareText, ThumbsDown, ThumbsUp } from "lucide-react";
import {
  useUpsertChatMessageFeedback,
  type ChatMessageFeedbackSentiment,
} from "@multica/core/chat/mutations";
import type { ChatMessageFeedback } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@multica/ui/components/ui/dialog";
import { Textarea } from "@multica/ui/components/ui/textarea";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { useT } from "../../i18n";

const MAX_COMMENT_LENGTH = 2000;
type FeedbackDialogMode = "negative" | "comment";

export function MessageFeedbackActions({
  sessionId,
  messageId,
  feedback,
}: {
  sessionId: string;
  messageId: string;
  feedback?: ChatMessageFeedback | null;
}) {
  const { t } = useT("chat");
  const upsertFeedback = useUpsertChatMessageFeedback();
  const [dialogMode, setDialogMode] = useState<FeedbackDialogMode | null>(null);
  const [comment, setComment] = useState("");

  const closeDialog = () => {
    setDialogMode(null);
    setComment("");
  };

  const openDialog = (mode: FeedbackDialogMode) => {
    setComment(feedback?.comment ?? "");
    setDialogMode(mode);
  };

  const saveFeedback = async (
    sentiment: ChatMessageFeedbackSentiment | null,
    nextComment: string,
    removed = false,
  ) => {
    try {
      await upsertFeedback.mutateAsync({
        sessionId,
        messageId,
        sentiment,
        comment: nextComment,
      });
      toast.success(t(($) => removed
        ? $.message_list.feedback_dialog.removed_toast
        : $.message_list.feedback_dialog.success_toast));
      return true;
    } catch {
      toast.error(t(($) => removed
        ? $.message_list.feedback_dialog.remove_error_toast
        : $.message_list.feedback_dialog.error_toast));
      return false;
    }
  };

  const handlePositiveClick = async () => {
    const removed = feedback?.sentiment === "positive";
    await saveFeedback(removed ? null : "positive", feedback?.comment ?? "", removed);
  };

  const handleNegativeClick = async () => {
    if (feedback?.sentiment === "negative") {
      await saveFeedback(null, feedback.comment, true);
      return;
    }
    openDialog("negative");
  };

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!dialogMode) return;

    const trimmedComment = comment.trim();
    const nextSentiment = dialogMode === "negative"
      ? "negative"
      : feedback?.sentiment ?? null;
    if (nextSentiment === "negative" && !trimmedComment) return;

    if (await saveFeedback(nextSentiment, trimmedComment)) {
      closeDialog();
    }
  };

  const commentRequired = dialogMode === "negative" || feedback?.sentiment === "negative";
  const existingComment = feedback?.comment.trim() ?? "";
  const commentUnchanged = dialogMode === "comment" && comment.trim() === existingComment;
  const isPending = upsertFeedback.isPending;
  const submitDisabled =
    isPending || commentUnchanged || (commentRequired && !comment.trim());
  const removeLabel = t(($) => $.message_list.feedback_dialog.remove_action);
  const hasComment = existingComment.length > 0;

  return (
    <>
      <FeedbackButton
        label={feedback?.sentiment === "positive"
          ? removeLabel
          : t(($) => $.message_list.positive_feedback_action)}
        active={feedback?.sentiment === "positive"}
        disabled={isPending}
        onClick={() => void handlePositiveClick()}
      >
        <ThumbsUp />
      </FeedbackButton>
      <FeedbackButton
        label={feedback?.sentiment === "negative"
          ? removeLabel
          : t(($) => $.message_list.negative_feedback_action)}
        active={feedback?.sentiment === "negative"}
        disabled={isPending}
        onClick={() => void handleNegativeClick()}
      >
        <ThumbsDown />
      </FeedbackButton>
      <FeedbackButton
        label={t(($) => hasComment
          ? $.message_list.feedback_dialog.edit_comment_action
          : $.message_list.feedback_dialog.comment_action)}
        active={hasComment}
        disabled={isPending}
        onClick={() => openDialog("comment")}
      >
        <MessageSquareText />
      </FeedbackButton>

      <Dialog
        open={dialogMode !== null}
        onOpenChange={(open) => {
          if (!open) closeDialog();
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>
              {dialogMode === "negative"
                ? t(($) => $.message_list.feedback_dialog.negative_title)
                : t(($) => hasComment
                  ? $.message_list.feedback_dialog.edit_comment_title
                  : $.message_list.feedback_dialog.comment_title)}
            </DialogTitle>
            <DialogDescription>
              {dialogMode === "negative"
                ? t(($) => $.message_list.feedback_dialog.negative_description)
                : t(($) => $.message_list.feedback_dialog.comment_description)}
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleSubmit} className="space-y-4">
            <label className="block space-y-1.5">
              <span className="text-xs font-medium text-muted-foreground">
                {commentRequired
                  ? t(($) => $.message_list.feedback_dialog.comment_required)
                  : t(($) => $.message_list.feedback_dialog.comment_optional)}
              </span>
              <Textarea
                value={comment}
                onChange={(event) => setComment(event.target.value)}
                placeholder={t(($) => $.message_list.feedback_dialog.comment_placeholder)}
                maxLength={MAX_COMMENT_LENGTH}
                rows={4}
                autoFocus
                aria-required={commentRequired}
              />
            </label>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                disabled={isPending}
                onClick={closeDialog}
              >
                {t(($) => $.message_list.feedback_dialog.cancel)}
              </Button>
              <Button type="submit" disabled={submitDisabled}>
                {upsertFeedback.isPending
                  ? t(($) => $.message_list.feedback_dialog.submitting)
                  : t(($) => $.message_list.feedback_dialog.submit)}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

function FeedbackButton({
  label,
  active,
  disabled,
  onClick,
  children,
}: {
  label: string;
  active: boolean;
  disabled: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon-xs"
            className={active
              ? "bg-muted text-foreground hover:bg-muted [&_svg]:fill-current"
              : "text-muted-foreground/70 hover:text-foreground"}
            disabled={disabled}
            onClick={onClick}
            aria-label={label}
            aria-pressed={active}
          />
        }
      >
        {children}
      </TooltipTrigger>
      <TooltipContent side="top">{label}</TooltipContent>
    </Tooltip>
  );
}
