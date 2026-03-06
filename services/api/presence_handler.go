package api

import (
	"log/slog"
	"net/http"

	"xend.chat/m/internal/presence"
	"xend.chat/m/pkg/httputil"
)

type PresenceHandler struct {
	presence *presence.Service
}

func NewPresenceHandler(presenceSvc *presence.Service) *PresenceHandler {
	return &PresenceHandler{presence: presenceSvc}
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
