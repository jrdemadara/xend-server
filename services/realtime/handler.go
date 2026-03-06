package realtime

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/presence"
	internalrealtime "xend.chat/m/internal/realtime"
	"xend.chat/m/pkg/httputil"
)

type Handler struct {
	tokens   *auth.TokenManager
	hub      *internalrealtime.Hub
	presence *presence.Service
	upgrader websocket.Upgrader
}

func NewHandler(tokens *auth.TokenManager, hub *internalrealtime.Hub, presenceSvc *presence.Service) *Handler {
	return &Handler{
		tokens:   tokens,
		hub:      hub,
		presence: presenceSvc,
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
	defer func() {
		h.hub.Remove(claims.UserID, claims.DeviceID)
		stats := h.hub.Stats()
		slog.Info("ws disconnected", "user_id", claims.UserID, "device_id", claims.DeviceID, "users", stats.Users, "connections", stats.Connections, "source", "ws")
		if h.presence != nil {
			_ = h.presence.MarkOffline(r.Context(), claims.UserID, claims.DeviceID)
		}
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
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
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
