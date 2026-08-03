import { beforeEach, describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import type { ChatMessageFeedback } from "@multica/core/types";
import enChat from "../../locales/en/chat.json";

const feedbackMutation = vi.hoisted(() => ({
  mutateAsync: vi.fn(),
  isPending: false,
}));

vi.mock("@multica/core/chat/mutations", () => ({
  useUpsertChatMessageFeedback: () => feedbackMutation,
}));

import { MessageFeedbackActions } from "./message-feedback-actions";

const TEST_RESOURCES = { en: { chat: enChat } };

function renderActions(feedback?: ChatMessageFeedback) {
  render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <MessageFeedbackActions
        sessionId="session-1"
        messageId="message-1"
        feedback={feedback}
      />
    </I18nProvider>,
  );
}

beforeEach(() => {
  feedbackMutation.mutateAsync.mockReset();
  feedbackMutation.mutateAsync.mockResolvedValue(undefined);
  feedbackMutation.isPending = false;
});

describe("MessageFeedbackActions", () => {
  it("allows positive feedback without a comment", async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole("button", { name: "Good response" }));

    await waitFor(() => {
      expect(feedbackMutation.mutateAsync).toHaveBeenCalledWith({
        sessionId: "session-1",
        messageId: "message-1",
        sentiment: "positive",
        comment: "",
      });
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("requires a comment for negative feedback", async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole("button", { name: "Poor response" }));
    const submit = screen.getByRole("button", { name: "Submit feedback" });
    expect(submit).toBeDisabled();

    await user.type(screen.getByPlaceholderText("Enter your comment..."), "Too vague");
    expect(submit).toBeEnabled();
    await user.click(submit);

    await waitFor(() => {
      expect(feedbackMutation.mutateAsync).toHaveBeenCalledWith({
        sessionId: "session-1",
        messageId: "message-1",
        sentiment: "negative",
        comment: "Too vague",
      });
    });
  });

  it("shows existing feedback and removes it on a repeated click", async () => {
    const user = userEvent.setup();
    renderActions({ sentiment: "positive", comment: "Useful" });

    const button = screen.getByRole("button", { name: "Remove feedback" });
    expect(button).toHaveAttribute("aria-pressed", "true");

    await user.click(button);

    await waitFor(() => {
      expect(feedbackMutation.mutateAsync).toHaveBeenCalledWith({
        sessionId: "session-1",
        messageId: "message-1",
        sentiment: null,
        comment: "Useful",
      });
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("submits a comment without a rating", async () => {
    const user = userEvent.setup();
    renderActions();

    await user.click(screen.getByRole("button", { name: "Add comment" }));
    await user.type(screen.getByPlaceholderText("Enter your comment..."), "Useful note");
    await user.click(screen.getByRole("button", { name: "Submit feedback" }));

    await waitFor(() => {
      expect(feedbackMutation.mutateAsync).toHaveBeenCalledWith({
        sessionId: "session-1",
        messageId: "message-1",
        sentiment: null,
        comment: "Useful note",
      });
    });
  });

  it("edits a comment without changing the rating", async () => {
    const user = userEvent.setup();
    renderActions({ sentiment: "positive", comment: "Old note" });

    await user.click(screen.getByRole("button", { name: "Edit comment" }));
    const textarea = screen.getByPlaceholderText("Enter your comment...");
    await user.clear(textarea);
    await user.type(textarea, "Updated note");
    await user.click(screen.getByRole("button", { name: "Submit feedback" }));

    await waitFor(() => {
      expect(feedbackMutation.mutateAsync).toHaveBeenCalledWith({
        sessionId: "session-1",
        messageId: "message-1",
        sentiment: "positive",
        comment: "Updated note",
      });
    });
  });
});
