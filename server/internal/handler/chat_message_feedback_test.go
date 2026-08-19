package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func listHandlerTestChatMessages(t *testing.T, sessionID string) []ChatMessageResponse {
	t.Helper()

	req := newRequest(http.MethodGet, "/api/chat/sessions/"+sessionID+"/messages", nil)
	req = withURLParam(req, "sessionId", sessionID)
	req = withChatTestWorkspaceCtx(t, req)
	w := httptest.NewRecorder()
	testHandler.ListChatMessages(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list chat messages: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var messages []ChatMessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &messages); err != nil {
		t.Fatalf("decode chat messages: %v", err)
	}
	return messages
}

func findHandlerTestChatMessage(t *testing.T, messages []ChatMessageResponse, messageID string) ChatMessageResponse {
	t.Helper()
	for _, message := range messages {
		if message.ID == messageID {
			return message
		}
	}
	t.Fatalf("chat message %s not found", messageID)
	return ChatMessageResponse{}
}

func createHandlerTestChatMessage(t *testing.T, sessionID, role string) string {
	t.Helper()

	var messageID string
	if err := testPool.QueryRow(context.Background(), `
		INSERT INTO chat_message (chat_session_id, role, content)
		VALUES ($1, $2, 'feedback test message')
		RETURNING id
	`, sessionID, role).Scan(&messageID); err != nil {
		t.Fatalf("create chat message: %v", err)
	}
	return messageID
}

func handlerTestFeedbackSentiment(value string) *string {
	return &value
}

func submitHandlerTestChatMessageFeedback(
	t *testing.T,
	sessionID string,
	messageID string,
	body UpsertChatMessageFeedbackRequest,
) *httptest.ResponseRecorder {
	t.Helper()

	req := newRequest(
		http.MethodPut,
		"/api/chat/sessions/"+sessionID+"/messages/"+messageID+"/feedback",
		body,
	)
	req = withURLParams(req, "sessionId", sessionID, "messageId", messageID)
	req = withChatTestWorkspaceCtx(t, req)
	w := httptest.NewRecorder()
	testHandler.UpsertChatMessageFeedback(w, req)
	return w
}

func TestUpsertChatMessageFeedback_AllowsPositiveFeedbackWithoutComment(t *testing.T) {
	agentID := createHandlerTestAgent(t, "PositiveFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "assistant")

	w := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("positive"),
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	var sentiment, comment string
	if err := testPool.QueryRow(context.Background(), `
		SELECT sentiment, comment
		FROM chat_message_feedback
		WHERE workspace_id = $1 AND user_id = $2 AND chat_message_id = $3
	`, testWorkspaceID, testUserID, messageID).Scan(&sentiment, &comment); err != nil {
		t.Fatalf("load feedback: %v", err)
	}
	if sentiment != "positive" || comment != "" {
		t.Fatalf("feedback = (%q, %q), want (positive, empty)", sentiment, comment)
	}
}

func TestUpsertChatMessageFeedback_RequiresNegativeComment(t *testing.T) {
	agentID := createHandlerTestAgent(t, "NegativeFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "assistant")

	w := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("negative"),
		Comment:   "   ",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpsertChatMessageFeedback_UpdatesExistingFeedback(t *testing.T) {
	agentID := createHandlerTestAgent(t, "UpdatedFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "assistant")

	first := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("positive"),
		Comment:   "Useful",
	})
	if first.Code != http.StatusNoContent {
		t.Fatalf("first submit: expected 204, got %d: %s", first.Code, first.Body.String())
	}
	second := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("negative"),
		Comment:   "Missing the requested details",
	})
	if second.Code != http.StatusNoContent {
		t.Fatalf("second submit: expected 204, got %d: %s", second.Code, second.Body.String())
	}

	var count int
	var sentiment, comment string
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*), max(sentiment), max(comment)
		FROM chat_message_feedback
		WHERE workspace_id = $1 AND user_id = $2 AND chat_message_id = $3
	`, testWorkspaceID, testUserID, messageID).Scan(&count, &sentiment, &comment); err != nil {
		t.Fatalf("load feedback: %v", err)
	}
	if count != 1 || sentiment != "negative" || comment != "Missing the requested details" {
		t.Fatalf("feedback = (%d, %q, %q), want one updated negative row", count, sentiment, comment)
	}
}

func TestUpsertChatMessageFeedback_RejectsUserMessage(t *testing.T) {
	agentID := createHandlerTestAgent(t, "UserMessageFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "user")

	w := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("positive"),
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatMessageFeedback_ListAndDelete(t *testing.T) {
	agentID := createHandlerTestAgent(t, "ListedFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "assistant")

	submitted := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Sentiment: handlerTestFeedbackSentiment("positive"),
		Comment:   "Useful",
	})
	if submitted.Code != http.StatusNoContent {
		t.Fatalf("submit: expected 204, got %d: %s", submitted.Code, submitted.Body.String())
	}

	listed := findHandlerTestChatMessage(t, listHandlerTestChatMessages(t, sessionID), messageID)
	if listed.Feedback == nil || listed.Feedback.Sentiment == nil || *listed.Feedback.Sentiment != "positive" || listed.Feedback.Comment != "Useful" {
		t.Fatalf("listed feedback = %#v, want positive Useful", listed.Feedback)
	}

	cleared := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{})
	if cleared.Code != http.StatusNoContent {
		t.Fatalf("clear: expected 204, got %d: %s", cleared.Code, cleared.Body.String())
	}

	listed = findHandlerTestChatMessage(t, listHandlerTestChatMessages(t, sessionID), messageID)
	if listed.Feedback != nil {
		t.Fatalf("listed feedback after clear = %#v, want nil", listed.Feedback)
	}
}

func TestUpsertChatMessageFeedback_AllowsCommentWithoutSentiment(t *testing.T) {
	agentID := createHandlerTestAgent(t, "CommentOnlyFeedbackAgent", []byte("[]"))
	sessionID := createHandlerTestChatSession(t, agentID)
	messageID := createHandlerTestChatMessage(t, sessionID, "assistant")

	w := submitHandlerTestChatMessageFeedback(t, sessionID, messageID, UpsertChatMessageFeedbackRequest{
		Comment: "Keep this note separate",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	listed := findHandlerTestChatMessage(t, listHandlerTestChatMessages(t, sessionID), messageID)
	if listed.Feedback == nil || listed.Feedback.Sentiment != nil || listed.Feedback.Comment != "Keep this note separate" {
		t.Fatalf("listed feedback = %#v, want comment-only feedback", listed.Feedback)
	}
}
