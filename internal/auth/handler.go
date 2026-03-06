package auth

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"xend.chat/m/pkg/httputil"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := h.svc.Register(r.Context(), req, clientIP(r))
	if err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusCreated, resp)
}

func (h *Handler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var req VerifyEmailRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := h.svc.VerifyEmail(r.Context(), req); err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"verified": true})
}

func (h *Handler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	var req ResendVerificationRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := h.svc.ResendEmailVerification(r.Context(), req, clientIP(r)); err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := h.svc.Login(r.Context(), req, clientIP(r))
	if err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, resp)
}

func (h *Handler) GoogleSignIn(w http.ResponseWriter, r *http.Request) {
	var req GoogleAuthRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := h.svc.GoogleSignIn(r.Context(), req)
	if err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, resp)
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		h.handleError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, resp)
}

func (h *Handler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httputil.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrEmailExists):
		httputil.Error(w, http.StatusConflict, "email_exists", "email already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httputil.Error(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
	case errors.Is(err, ErrDeviceNotFound):
		httputil.Error(w, http.StatusBadRequest, "device_not_found", "device_name was not found for this account")
	case errors.Is(err, ErrEmailNotVerified):
		httputil.Error(w, http.StatusUnauthorized, "email_not_verified", "email is not verified")
	case errors.Is(err, ErrRateLimited):
		httputil.Error(w, http.StatusTooManyRequests, "rate_limited", "too many verification requests, try again later")
	case errors.Is(err, ErrInvalidToken):
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "token is invalid")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func clientIP(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return ip
}
