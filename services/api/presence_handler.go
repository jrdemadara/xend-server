package api

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/presence"
	"xend.chat/m/pkg/httputil"
)

type PresenceHandler struct {
	presence *presence.Service
	repo     *auth.Repository
}

func NewPresenceHandler(presenceSvc *presence.Service, repo *auth.Repository) *PresenceHandler {
	return &PresenceHandler{presence: presenceSvc, repo: repo}
}

func (h *PresenceHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.presence.Heartbeat(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *PresenceHandler) Offline(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.presence.MarkOffline(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	slog.Info("presence offline", "user_id", claims.UserID, "device_id", claims.DeviceID, "source", "api")
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *PresenceHandler) GetUserPresence(w http.ResponseWriter, r *http.Request) {
	_, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	userID := strings.TrimSpace(r.PathValue("user_id"))
	if userID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}

	isOnline, err := h.presence.IsUserOnline(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	lastSeen, err := h.presence.LastSeen(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := map[string]any{
		"user_id":    userID,
		"is_online":  isOnline,
		"updated_at": time.Now().UTC().Unix(),
		"last_seen":  nil,
	}
	if lastSeen != nil {
		response["last_seen"] = lastSeen.Unix()
	}
	httputil.JSON(w, http.StatusOK, response)
}
