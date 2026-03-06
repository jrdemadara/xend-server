package api

import (
	"net/http"

	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type ProtectedAuthHandler struct {
	svc *auth.Service
}

func NewProtectedAuthHandler(svc *auth.Service) *ProtectedAuthHandler {
	return &ProtectedAuthHandler{svc: svc}
}

func (h *ProtectedAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.svc.Logout(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ProtectedAuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	if err := h.svc.LogoutAll(r.Context(), claims.UserID); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}
