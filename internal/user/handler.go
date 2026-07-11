package user

import (
	"net/http"

	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type Handler struct {
	repo *Repository
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	profile, err := h.repo.GetProfileByID(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	httputil.JSON(w, http.StatusOK, ProfileResponse{
		UserID:      profile.ID,
		DeviceID:    claims.DeviceID,
		DisplayName: profile.DisplayName,
		Email:       profile.Email,
		AvatarURL:   profile.AvatarURL,
		Identifier:  profile.Identifier,
	})
}
