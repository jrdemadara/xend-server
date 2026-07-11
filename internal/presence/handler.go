package presence

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.service.Heartbeat(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) Offline(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.service.MarkOffline(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	slog.Info("presence offline", "user_id", claims.UserID, "device_id", claims.DeviceID, "source", "api")
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) GetUserPresence(w http.ResponseWriter, r *http.Request) {
	_, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	userID := strings.TrimSpace(r.PathValue("user_id"))
	if userID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}

	isOnline, err := h.service.IsUserOnline(r.Context(), userID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	lastSeen, err := h.service.LastSeen(r.Context(), userID)
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
