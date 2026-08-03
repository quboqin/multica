package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	chatMessageFeedbackMaxCommentLen = 2000
	chatMessageFeedbackBodyLimit     = 16 * 1024
)

type UpsertChatMessageFeedbackRequest struct {
	Sentiment *string `json:"sentiment"`
	Comment   string  `json:"comment"`
}

type ChatMessageFeedbackResponse struct {
	Sentiment *string `json:"sentiment"`
	Comment   string  `json:"comment"`
}

func (h *Handler) loadAssistantChatMessageForFeedback(
	w http.ResponseWriter,
	r *http.Request,
	userID string,
	workspaceID string,
) (db.ChatSession, db.ChatMessage, bool) {
	session, ok := h.gateChatSessionForUser(
		w,
		r,
		userID,
		workspaceID,
		chi.URLParam(r, "sessionId"),
	)
	if !ok {
		return db.ChatSession{}, db.ChatMessage{}, false
	}

	messageID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "messageId"), "chat message id")
	if !ok {
		return db.ChatSession{}, db.ChatMessage{}, false
	}
	message, err := h.Queries.GetChatMessage(r.Context(), messageID)
	if err != nil || message.ChatSessionID != session.ID {
		writeError(w, http.StatusNotFound, "chat message not found")
		return db.ChatSession{}, db.ChatMessage{}, false
	}
	if message.Role != "assistant" {
		writeError(w, http.StatusBadRequest, "only assistant messages can receive feedback")
		return db.ChatSession{}, db.ChatMessage{}, false
	}

	return session, message, true
}

func (h *Handler) listChatMessageFeedback(
	ctx context.Context,
	workspaceID string,
	userID string,
	messageIDs []pgtype.UUID,
) (map[string]*ChatMessageFeedbackResponse, error) {
	if len(messageIDs) == 0 {
		return nil, nil
	}

	rows, err := h.Queries.ListChatMessageFeedbackByMessageIDs(ctx, db.ListChatMessageFeedbackByMessageIDsParams{
		MessageIds:  messageIDs,
		WorkspaceID: parseUUID(workspaceID),
		UserID:      parseUUID(userID),
	})
	if err != nil {
		return nil, err
	}

	feedback := make(map[string]*ChatMessageFeedbackResponse, len(rows))
	for _, row := range rows {
		feedback[uuidToString(row.ChatMessageID)] = &ChatMessageFeedbackResponse{
			Sentiment: textToPtr(row.Sentiment),
			Comment:   row.Comment,
		}
	}
	return feedback, nil
}

func (h *Handler) UpsertChatMessageFeedback(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	workspaceID := ctxWorkspaceID(r.Context())
	session, message, ok := h.loadAssistantChatMessageForFeedback(w, r, userID, workspaceID)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, chatMessageFeedbackBodyLimit)
	var req UpsertChatMessageFeedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var sentiment string
	if req.Sentiment != nil {
		sentiment = strings.TrimSpace(*req.Sentiment)
	}
	if sentiment != "" && sentiment != "positive" && sentiment != "negative" {
		writeError(w, http.StatusBadRequest, "sentiment must be positive or negative")
		return
	}
	comment := strings.TrimSpace(req.Comment)
	if sentiment == "negative" && comment == "" {
		writeError(w, http.StatusBadRequest, "comment is required for negative feedback")
		return
	}
	if utf8.RuneCountInString(comment) > chatMessageFeedbackMaxCommentLen {
		writeError(w, http.StatusBadRequest, "comment too long")
		return
	}
	if sentiment == "" && comment == "" {
		if err := h.Queries.DeleteChatMessageFeedback(r.Context(), db.DeleteChatMessageFeedbackParams{
			WorkspaceID:   session.WorkspaceID,
			UserID:        parseUUID(userID),
			ChatMessageID: message.ID,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete chat message feedback")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err := h.Queries.UpsertChatMessageFeedback(r.Context(), db.UpsertChatMessageFeedbackParams{
		WorkspaceID:   session.WorkspaceID,
		UserID:        parseUUID(userID),
		ChatMessageID: message.ID,
		Sentiment:     pgtype.Text{String: sentiment, Valid: sentiment != ""},
		Comment:       comment,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save chat message feedback")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
