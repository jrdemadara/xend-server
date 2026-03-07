package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type DeviceHandler struct {
	repo *auth.Repository
}

func NewDeviceHandler(repo *auth.Repository) *DeviceHandler {
	return &DeviceHandler{repo: repo}
}

func (h *DeviceHandler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	var req auth.DeviceRegisterRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.DeviceName = strings.TrimSpace(req.DeviceName)
	req.Platform = strings.TrimSpace(strings.ToLower(req.Platform))
	if req.DeviceName == "" || req.Platform == "" || req.RegistrationID <= 0 || strings.TrimSpace(req.IdentityKeyPublic) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "missing required fields")
		return
	}

	deviceID, err := h.repo.RegisterDevice(r.Context(), claims.UserID, req)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]string{"device_id": deviceID})
}

func (h *DeviceHandler) UpsertSignedPrekey(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "device_id is required")
		return
	}
	owns, err := h.repo.DeviceBelongsToUser(r.Context(), deviceID, claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !owns {
		slog.Warn("push token rejected device ownership", "user_id", claims.UserID, "device_id", deviceID)
		httputil.Error(w, http.StatusForbidden, "forbidden", "device does not belong to user")
		return
	}

	var req auth.SignedPrekeyRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.KeyID <= 0 || strings.TrimSpace(req.PublicKey) == "" || strings.TrimSpace(req.Signature) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "missing required fields")
		return
	}

	if err := h.repo.UpsertSignedPrekey(r.Context(), deviceID, req); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DeviceHandler) UploadOneTimePrekeys(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	deviceID := r.PathValue("device_id")
	owns, err := h.repo.DeviceBelongsToUser(r.Context(), deviceID, claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !owns {
		httputil.Error(w, http.StatusForbidden, "forbidden", "device does not belong to user")
		return
	}

	var req auth.OneTimePrekeyBatchRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if len(req.Prekeys) == 0 {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "prekeys are required")
		return
	}

	inserted, err := h.repo.InsertOneTimePrekeys(r.Context(), deviceID, req.Prekeys)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusCreated, map[string]int64{"inserted_count": inserted})
}

func (h *DeviceHandler) UpsertKyberPrekey(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	deviceID := r.PathValue("device_id")
	if deviceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "device_id is required")
		return
	}
	owns, err := h.repo.DeviceBelongsToUser(r.Context(), deviceID, claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !owns {
		httputil.Error(w, http.StatusForbidden, "forbidden", "device does not belong to user")
		return
	}

	var req auth.KyberPrekeyRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.KeyID <= 0 || strings.TrimSpace(req.PublicKey) == "" || strings.TrimSpace(req.Signature) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "missing required fields")
		return
	}

	if err := h.repo.UpsertKyberPrekey(r.Context(), deviceID, req); err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DeviceHandler) UpsertPushToken(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	deviceID := r.PathValue("device_id")
	owns, err := h.repo.DeviceBelongsToUser(r.Context(), deviceID, claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if !owns {
		httputil.Error(w, http.StatusForbidden, "forbidden", "device does not belong to user")
		return
	}

	var req auth.PushTokenRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.Provider = strings.TrimSpace(strings.ToLower(req.Provider))
	if req.Provider == "" || strings.TrimSpace(req.Token) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "missing required fields")
		return
	}

	if err := h.repo.UpsertPushToken(r.Context(), deviceID, req.Provider, req.Token); err != nil {
		slog.Error("push token upsert failed", "user_id", claims.UserID, "device_id", deviceID, "provider", req.Provider, "error", err)
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	slog.Info("push token upserted", "user_id", claims.UserID, "device_id", deviceID, "provider", req.Provider, "token_len", len(req.Token))
	httputil.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *DeviceHandler) GetPrekeys(w http.ResponseWriter, r *http.Request) {
	_, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	targetUserID := r.PathValue("user_id")
	if targetUserID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "user_id is required")
		return
	}

	bundle, err := h.repo.GetPrekeyBundle(r.Context(), targetUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.Error(w, http.StatusNotFound, "not_found", "user prekeys not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, bundle)
}
