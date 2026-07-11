package realtime

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/message"
	"xend.chat/m/internal/presence"
	internalrealtime "xend.chat/m/internal/realtime"
	"xend.chat/m/internal/relationship"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type ConversationRecipientLookup interface {
	ListConversationRecipientUserIDs(ctx context.Context, conversationID, excludeUserID string) ([]string, error)
}

type RelatedUserLookup interface {
	ListRelatedUserIDs(ctx context.Context, userID string) ([]string, error)
}

type Handler struct {
	tokens                 *auth.TokenManager
	hub                    *internalrealtime.Hub
	presence               *presence.Service
	conversationRecipients ConversationRecipientLookup
	relatedUsers           RelatedUserLookup
	upgrader               websocket.Upgrader
}

func NewHandler(tokens *auth.TokenManager, hub *internalrealtime.Hub, presenceSvc *presence.Service, messages *message.Repository, relationships *relationship.Repository) *Handler {
	return &Handler{
		tokens:                 tokens,
		hub:                    hub,
		presence:               presenceSvc,
		conversationRecipients: messages,
		relatedUsers:           relationships,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	token := tokenFromRequest(r)
	if token == "" {
		http.Error(w, "missing access token", http.StatusUnauthorized)
		return
	}

	claims, err := h.tokens.ParseAccessToken(token)
	if err != nil {
		http.Error(w, "invalid access token", http.StatusUnauthorized)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	h.hub.Add(claims.UserID, claims.DeviceID, conn)
	stats := h.hub.Stats()
	slog.Info("ws connected", "user_id", claims.UserID, "device_id", claims.DeviceID, "users", stats.Users, "connections", stats.Connections, "source", "ws")
	if h.presence != nil {
		_ = h.presence.MarkOnline(r.Context(), claims.UserID, claims.DeviceID)
	}
	h.broadcastPresence(r, claims.UserID, true)
	defer func() {
		h.hub.Remove(claims.UserID, claims.DeviceID)
		stats := h.hub.Stats()
		slog.Info("ws disconnected", "user_id", claims.UserID, "device_id", claims.DeviceID, "users", stats.Users, "connections", stats.Connections, "source", "ws")
		if h.presence != nil {
			_ = h.presence.MarkOffline(r.Context(), claims.UserID, claims.DeviceID)
		}
		h.broadcastPresence(r, claims.UserID, false)
	}()

	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error {
		if h.presence != nil {
			_ = h.presence.Heartbeat(r.Context(), claims.UserID, claims.DeviceID)
		}
		return conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	})

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			h.handleClientEvent(r, claims.UserID, data)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
				return
			}
			if h.presence != nil {
				_ = h.presence.Heartbeat(r.Context(), claims.UserID, claims.DeviceID)
			}
		}
	}
}

type clientEvent struct {
	Type    string            `json:"type"`
	Payload map[string]string `json:"payload"`
}

func (h *Handler) handleClientEvent(r *http.Request, senderUserID string, data []byte) {
	if h.conversationRecipients == nil {
		return
	}
	var event clientEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return
	}
	if event.Type != "typing" {
		return
	}
	conversationID := strings.TrimSpace(event.Payload["conversation_id"])
	if conversationID == "" {
		return
	}
	recipientUserIDs, err := h.conversationRecipients.ListConversationRecipientUserIDs(r.Context(), conversationID, senderUserID)
	if err != nil {
		return
	}
	payload := map[string]string{
		"conversation_id": conversationID,
		"sender_user_id":  senderUserID,
		"is_typing":       strings.TrimSpace(event.Payload["is_typing"]),
	}
	for _, recipientUserID := range recipientUserIDs {
		h.hub.SendToUser(recipientUserID, wsutil.NewEvent("typing", payload))
	}
}

func (h *Handler) broadcastPresence(r *http.Request, userID string, isOnline bool) {
	if h.relatedUsers == nil || h.hub == nil {
		return
	}
	relatedUserIDs, err := h.relatedUsers.ListRelatedUserIDs(r.Context(), userID)
	if err != nil {
		return
	}
	payload := map[string]any{
		"user_id":    userID,
		"is_online":  isOnline,
		"updated_at": time.Now().UTC().Unix(),
	}
	for _, relatedUserID := range relatedUserIDs {
		h.hub.SendToUser(relatedUserID, wsutil.NewEvent("presence_updated", payload))
	}
}

func (h *Handler) Connections(w http.ResponseWriter, r *http.Request) {
	stats := h.hub.Stats()
	httputil.JSON(w, http.StatusOK, stats)
}

func tokenFromRequest(r *http.Request) string {
	rawAuth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(rawAuth), "bearer ") {
		return strings.TrimSpace(rawAuth[7:])
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}
