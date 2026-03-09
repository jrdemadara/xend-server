package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/notify"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type MessageHandler struct {
	repo         *auth.Repository
	hub          *realtime.Hub
	pushNotifier notify.PushNotifier
}

func NewMessageHandler(repo *auth.Repository, hub *realtime.Hub, pushNotifier notify.PushNotifier) *MessageHandler {
	return &MessageHandler{repo: repo, hub: hub, pushNotifier: pushNotifier}
}

type sendMessageRequest struct {
	ClientMessageID string `json:"client_message_id"`
	MessageType     string `json:"message_type"`
	Ciphertext      string `json:"ciphertext"`
	SenderTimestamp *int64 `json:"sender_timestamp"`
}

type messageResponse struct {
	MessageID       string `json:"message_id"`
	ConversationID  string `json:"conversation_id"`
	SenderUserID    string `json:"sender_user_id"`
	SenderDeviceID  string `json:"sender_device_id"`
	ClientMessageID string `json:"client_message_id"`
	MessageType     string `json:"message_type"`
	Ciphertext      string `json:"ciphertext"`
	SenderTimestamp *int64 `json:"sender_timestamp,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	ReceiptUserID   string `json:"receipt_user_id,omitempty"`
	ReceiptStatus   string `json:"receipt_status,omitempty"`
	DeliveredAt     *int64 `json:"delivered_at,omitempty"`
	ReadAt          *int64 `json:"read_at,omitempty"`
}

func (h *MessageHandler) Send(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	if conversationID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "conversation_id is required")
		return
	}

	var req sendMessageRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.ClientMessageID = strings.TrimSpace(req.ClientMessageID)
	req.MessageType = strings.TrimSpace(req.MessageType)
	req.Ciphertext = strings.TrimSpace(req.Ciphertext)
	if req.ClientMessageID == "" || req.MessageType == "" || req.Ciphertext == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "client_message_id, message_type, and ciphertext are required")
		return
	}

	var senderTimestamp *time.Time
	if req.SenderTimestamp != nil && *req.SenderTimestamp > 0 {
		t := time.Unix(*req.SenderTimestamp, 0).UTC()
		senderTimestamp = &t
	}

	item, err := h.repo.CreateConversationMessage(
		r.Context(),
		claims.UserID,
		claims.DeviceID,
		conversationID,
		req.ClientMessageID,
		req.MessageType,
		req.Ciphertext,
		senderTimestamp,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.Error(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	h.broadcastMessageCreated(r, item)
	httputil.JSON(w, http.StatusCreated, toMessageResponse(item))
}

func (h *MessageHandler) ListByConversation(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	if conversationID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "conversation_id is required")
		return
	}

	limit := parseLimit(r.URL.Query().Get("limit"), 100, 200)
	before := parseUnixTimePtr(r.URL.Query().Get("before"))

	items, err := h.repo.ListConversationMessages(r.Context(), claims.UserID, conversationID, limit, before)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]messageResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toMessageResponse(item))
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *MessageHandler) Sync(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	updatedSenderIDs, err := h.repo.MarkMessagesDeliveredForDevice(r.Context(), claims.UserID, claims.DeviceID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.broadcastReceiptUpdated(updatedSenderIDs)

	limit := parseLimit(r.URL.Query().Get("limit"), 200, 500)
	since := parseUnixTimePtr(r.URL.Query().Get("since"))

	items, err := h.repo.ListMessagesForUserSince(r.Context(), claims.UserID, since, limit)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]messageResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, toMessageResponse(item))
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *MessageHandler) MarkConversationRead(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	conversationID := strings.TrimSpace(r.PathValue("conversation_id"))
	if conversationID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "conversation_id is required")
		return
	}

	updatedSenderIDs, err := h.repo.MarkConversationMessagesRead(r.Context(), claims.UserID, claims.DeviceID, conversationID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	h.broadcastReceiptUpdated(updatedSenderIDs)
	httputil.JSON(w, http.StatusOK, map[string]any{"updated": len(updatedSenderIDs) > 0})
}

func (h *MessageHandler) broadcastMessageCreated(r *http.Request, item auth.MessageRecord) {
	if h.hub == nil && h.pushNotifier == nil {
		return
	}

	targetUserIDs, err := h.repo.ListConversationRecipientUserIDs(r.Context(), item.ConversationID, item.SenderUserID)
	if err != nil {
		slog.Error("message recipient lookup failed", "conversation_id", item.ConversationID, "message_id", item.MessageID, "error", err)
		return
	}

	eventPayload := map[string]string{
		"message_id":       item.MessageID,
		"conversation_id":  item.ConversationID,
		"sender_user_id":   item.SenderUserID,
		"sender_device_id": item.SenderDeviceID,
		"message_type":     item.MessageType,
	}

	var senderName string
	if profile, err := h.repo.GetUserProfileByID(r.Context(), item.SenderUserID); err == nil {
		senderName = profile.DisplayName
	}

	for _, userID := range targetUserIDs {
		if h.hub != nil {
			h.hub.SendToUser(userID, wsutil.NewEvent("message_created", eventPayload))
		}
		if h.pushNotifier == nil || h.hub != nil && h.hub.HasActiveUser(userID) {
			continue
		}

		tokens, err := h.repo.ListActivePushTokensByUser(r.Context(), userID)
		if err != nil {
			slog.Error("push token lookup failed", "event", "message_created", "message_id", item.MessageID, "target_user_id", userID, "error", err)
			continue
		}
		if len(tokens) == 0 {
			slog.Info("push skipped no active tokens", "event", "message_created", "message_id", item.MessageID, "target_user_id", userID)
			continue
		}

		title := "New message"
		if senderName != "" {
			title = fmt.Sprintf("%s sent a message", senderName)
		}
		if err := h.pushNotifier.SendToTokens(r.Context(), tokens, notify.PushMessage{
			Title: title,
			Body:  "Open Xend to read it.",
			Data: map[string]string{
				"type":            "message_created",
				"message_id":      item.MessageID,
				"conversation_id": item.ConversationID,
				"sender_user_id":  item.SenderUserID,
			},
		}); err != nil {
			slog.Error("push send failed", "event", "message_created", "message_id", item.MessageID, "target_user_id", userID, "token_count", len(tokens), "error", err)
		} else {
			slog.Info("push sent", "event", "message_created", "message_id", item.MessageID, "target_user_id", userID, "token_count", len(tokens))
		}
	}
}

func toMessageResponse(item auth.MessageRecord) messageResponse {
	var senderTimestamp *int64
	var deliveredAt *int64
	var readAt *int64
	var receiptUserID string
	var receiptStatus string
	if item.SenderTimestamp != nil {
		value := item.SenderTimestamp.Unix()
		senderTimestamp = &value
	}
	if item.DeliveredAt != nil {
		value := item.DeliveredAt.Unix()
		deliveredAt = &value
	}
	if item.ReadAt != nil {
		value := item.ReadAt.Unix()
		readAt = &value
	}
	if item.ReceiptUserID != nil {
		receiptUserID = *item.ReceiptUserID
	}
	if item.ReceiptStatus != nil {
		receiptStatus = *item.ReceiptStatus
	}
	return messageResponse{
		MessageID:       item.MessageID,
		ConversationID:  item.ConversationID,
		SenderUserID:    item.SenderUserID,
		SenderDeviceID:  item.SenderDeviceID,
		ClientMessageID: item.ClientMessageID,
		MessageType:     item.MessageType,
		Ciphertext:      item.Ciphertext,
		SenderTimestamp: senderTimestamp,
		CreatedAt:       item.CreatedAt.Unix(),
		ReceiptUserID:   receiptUserID,
		ReceiptStatus:   receiptStatus,
		DeliveredAt:     deliveredAt,
		ReadAt:          readAt,
	}
}

func (h *MessageHandler) broadcastReceiptUpdated(targetUserIDs []string) {
	if h.hub == nil || len(targetUserIDs) == 0 {
		return
	}
	for _, userID := range targetUserIDs {
		h.hub.SendToUser(userID, wsutil.NewEvent("message_receipt_updated", map[string]string{
			"type": "message_receipt_updated",
		}))
	}
}

func parseLimit(raw string, fallback int, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func parseUnixTimePtr(raw string) *time.Time {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return nil
	}
	t := time.Unix(value, 0).UTC()
	return &t
}
