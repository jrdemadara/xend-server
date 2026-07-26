package relationship

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/device"
	"xend.chat/m/internal/notify"
	"xend.chat/m/internal/queue"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/internal/user"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type Handler struct {
	repo          *Repository
	users         *user.Repository
	devices       *device.Repository
	emailEnqueuer *queue.VerificationEmailEnqueuer
	hub           *realtime.Hub
	pushNotifier  notify.PushNotifier
	mediaStore    *MediaStore
}

func NewHandler(repo *Repository, users *user.Repository, devices *device.Repository, emailEnqueuer *queue.VerificationEmailEnqueuer, hub *realtime.Hub, pushNotifier notify.PushNotifier, mediaStore *MediaStore) *Handler {
	return &Handler{
		repo:          repo,
		users:         users,
		devices:       devices,
		emailEnqueuer: emailEnqueuer,
		hub:           hub,
		pushNotifier:  pushNotifier,
		mediaStore:    mediaStore,
	}
}

type createInviteRequest struct {
	Identifier string  `json:"identifier"`
	Note       *string `json:"note"`
}

type spaceResponse struct {
	RelationshipSpaceID   string  `json:"relationship_space_id"`
	ConversationID        string  `json:"conversation_id"`
	Name                  *string `json:"name,omitempty"`
	CoverPhotoURL         *string `json:"cover_photo_url,omitempty"`
	CoverPhotoVersion     *string `json:"cover_photo_version,omitempty"`
	CouplePhotoURL        *string `json:"couple_photo_url,omitempty"`
	CouplePhotoVersion    *string `json:"couple_photo_version,omitempty"`
	RelationshipStartDate string  `json:"relationship_start_date"`
	CelebrateMonthsary    bool    `json:"celebrate_monthsary"`
	CelebrateAnniversary  bool    `json:"celebrate_anniversary"`
	CreatedByUserID       string  `json:"created_by_user_id"`
	CurrentLevel          int16   `json:"current_level"`
	CurrentLevelName      string  `json:"current_level_name"`
	IsDefault             bool    `json:"is_default"`
	AccessHint            *string `json:"access_hint,omitempty"`
	AccessConfigured      bool    `json:"access_configured"`
	ArchivedAt            *int64  `json:"archived_at,omitempty"`
	CreatedAt             int64   `json:"created_at"`
	UpdatedAt             int64   `json:"updated_at"`
}

type levelResponse struct {
	Level       int16   `json:"level"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type levelProgressResponse struct {
	RelationshipSpaceID string `json:"relationship_space_id"`
	Level               int16  `json:"level"`
	RequiredPoints      int32  `json:"required_points"`
	CurrentPoints       int32  `json:"current_points"`
	UnlockedAt          *int64 `json:"unlocked_at,omitempty"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

type memberResponse struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Identifier  string `json:"identifier"`
}

type moodRequest struct {
	MoodKey string `json:"mood_key"`
	Emoji   string `json:"emoji"`
	Label   string `json:"label"`
}

type updateSpaceSettingsRequest struct {
	Name                  *string `json:"name"`
	RelationshipStartDate *string `json:"relationship_start_date"`
	CelebrateMonthsary    *bool   `json:"celebrate_monthsary"`
	CelebrateAnniversary  *bool   `json:"celebrate_anniversary"`
}

type moodResponse struct {
	RelationshipSpaceID string `json:"relationship_space_id"`
	UserID              string `json:"user_id"`
	DisplayName         string `json:"display_name"`
	MoodKey             string `json:"mood_key,omitempty"`
	Emoji               string `json:"emoji,omitempty"`
	Label               string `json:"label,omitempty"`
	UpdatedAt           *int64 `json:"updated_at,omitempty"`
	IsMe                bool   `json:"is_me"`
}

type configureSpaceAccessRequest struct {
	Passphrase string  `json:"passphrase"`
	Hint       *string `json:"hint"`
}

type unlockSpaceRequest struct {
	Passphrase string `json:"passphrase"`
}

type inviteOutboxResponse struct {
	InviteID          string  `json:"invite_id"`
	InviteeIdentifier string  `json:"invitee_identifier"`
	Status            string  `json:"status"`
	Note              *string `json:"note,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

func (h *Handler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	var req createInviteRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.Identifier = strings.TrimSpace(req.Identifier)
	if req.Identifier == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "identifier is required")
		return
	}

	inviteID, inviteeUserID, inviteeEmail, err := h.repo.CreateInviteByIdentifier(r.Context(), claims.UserID, req.Identifier, req.Note)
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid invite target")
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			httputil.Error(w, http.StatusNotFound, "not_found", "user identifier not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if h.emailEnqueuer != nil && h.users != nil {
		profile, profileErr := h.users.GetProfileByID(r.Context(), claims.UserID)
		if profileErr == nil {
			note := ""
			if req.Note != nil {
				note = strings.TrimSpace(*req.Note)
			}
			_ = h.emailEnqueuer.EnqueueRelationshipInviteEmail(r.Context(), inviteeEmail, profile.DisplayName, profile.Identifier, note)
		}
	}

	if h.hub != nil {
		h.hub.SendToUser(claims.UserID, wsutil.NewEvent("relationship_invite_sent", map[string]string{
			"invite_id": inviteID,
		}))
	}
	if h.hub != nil && h.hub.HasActiveUser(inviteeUserID) {
		h.hub.SendToUser(inviteeUserID, wsutil.NewEvent("relationship_invite_received", map[string]string{
			"invite_id":       inviteID,
			"inviter_user_id": claims.UserID,
		}))
	} else if h.pushNotifier != nil && h.users != nil && h.devices != nil {
		profile, profileErr := h.users.GetProfileByID(r.Context(), claims.UserID)
		if profileErr == nil {
			tokens, tokenErr := h.devices.ListActivePushTokensByUser(r.Context(), inviteeUserID)
			if tokenErr == nil && len(tokens) > 0 {
				pushErr := h.pushNotifier.SendToTokens(r.Context(), tokens, notify.PushMessage{
					Title: fmt.Sprintf("%s invited you", profile.DisplayName),
					Body:  "Open Xend to respond to the relationship invite.",
					Data: map[string]string{
						"type":       "relationship_invite_received",
						"invite_id":  inviteID,
						"inviter_id": claims.UserID,
					},
				})
				if pushErr != nil {
					slog.Error("push send failed", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID, "token_count", len(tokens), "error", pushErr)
				} else {
					slog.Info("push sent", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID, "token_count", len(tokens))
				}
			} else if tokenErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID, "error", tokenErr)
			} else {
				slog.Info("push skipped no active tokens", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID)
			}
		}
	}

	httputil.JSON(w, http.StatusCreated, map[string]string{"invite_id": inviteID})
}

func (h *Handler) Inbox(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListInviteInbox(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if items == nil {
		items = []Invite{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) Outbox(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListInviteOutbox(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := make([]inviteOutboxResponse, 0, len(items))
	for _, item := range items {
		response = append(response, inviteOutboxResponse{
			InviteID:          item.InviteID,
			InviteeIdentifier: item.InviteeIdentifier,
			Status:            item.Status,
			Note:              item.Note,
			CreatedAt:         item.CreatedAt.Unix(),
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": response})
}

func (h *Handler) Accept(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	inviteID := strings.TrimSpace(r.PathValue("invite_id"))
	if inviteID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invite_id is required")
		return
	}

	spaceID, conversationID, inviterUserID, err := h.repo.AcceptInvite(r.Context(), inviteID, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "invite not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if h.hub != nil {
		event := wsutil.NewEvent("relationship_invite_accepted", map[string]string{
			"invite_id":             inviteID,
			"relationship_space_id": spaceID,
			"conversation_id":       conversationID,
		})
		h.hub.SendToUser(claims.UserID, event)
		h.hub.SendToUser(inviterUserID, event)
	}

	if h.pushNotifier != nil && h.users != nil && h.devices != nil {
		profile, profileErr := h.users.GetProfileByID(r.Context(), claims.UserID)
		if profileErr == nil {
			tokens, tokenErr := h.devices.ListActivePushTokensByUser(r.Context(), inviterUserID)
			if tokenErr == nil && len(tokens) > 0 {
				pushErr := h.pushNotifier.SendToTokens(r.Context(), tokens, notify.PushMessage{
					Title: fmt.Sprintf("%s accepted your invite", profile.DisplayName),
					Body:  "Open Xend to enter your relationship space.",
					Data: map[string]string{
						"type":                  "relationship_invite_accepted",
						"invite_id":             inviteID,
						"relationship_space_id": spaceID,
						"conversation_id":       conversationID,
						"invitee_user_id":       claims.UserID,
					},
				})
				if pushErr != nil {
					slog.Error("push send failed", "event", "relationship_invite_accepted", "invite_id", inviteID, "target_user_id", inviterUserID, "token_count", len(tokens), "error", pushErr)
				} else {
					slog.Info("push sent", "event", "relationship_invite_accepted", "invite_id", inviteID, "target_user_id", inviterUserID, "token_count", len(tokens))
				}
			} else if tokenErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_accepted", "invite_id", inviteID, "target_user_id", inviterUserID, "error", tokenErr)
			} else {
				slog.Info("push skipped no active tokens", "event", "relationship_invite_accepted", "invite_id", inviteID, "target_user_id", inviterUserID)
			}
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]string{
		"relationship_space_id": spaceID,
		"conversation_id":       conversationID,
	})
}

func (h *Handler) Decline(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	inviteID := strings.TrimSpace(r.PathValue("invite_id"))
	if inviteID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invite_id is required")
		return
	}

	inviterUserID, err := h.repo.DeclineInvite(r.Context(), inviteID, claims.UserID)
	if err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "invite not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if h.hub != nil {
		event := wsutil.NewEvent("relationship_invite_declined", map[string]string{
			"invite_id": inviteID,
		})
		h.hub.SendToUser(claims.UserID, event)
		h.hub.SendToUser(inviterUserID, event)
	}

	if h.pushNotifier != nil && h.users != nil && h.devices != nil {
		profile, profileErr := h.users.GetProfileByID(r.Context(), claims.UserID)
		if profileErr == nil {
			tokens, tokenErr := h.devices.ListActivePushTokensByUser(r.Context(), inviterUserID)
			if tokenErr == nil && len(tokens) > 0 {
				pushErr := h.pushNotifier.SendToTokens(r.Context(), tokens, notify.PushMessage{
					Title: fmt.Sprintf("%s declined your invite", profile.DisplayName),
					Body:  "Open Xend to manage your invites.",
					Data: map[string]string{
						"type":            "relationship_invite_declined",
						"invite_id":       inviteID,
						"invitee_user_id": claims.UserID,
					},
				})
				if pushErr != nil {
					slog.Error("push send failed", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID, "token_count", len(tokens), "error", pushErr)
				} else {
					slog.Info("push sent", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID, "token_count", len(tokens))
				}
			} else if tokenErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID, "error", tokenErr)
			} else {
				slog.Info("push skipped no active tokens", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID)
			}
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListSpacesByUser(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := make([]spaceResponse, 0, len(items))
	for _, item := range items {
		response = append(response, toSpaceResponse(item))
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": response})
}

func (h *Handler) ListLevels(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListLevels(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := make([]levelResponse, 0, len(items))
	for _, item := range items {
		response = append(response, levelResponse{
			Level:       item.Level,
			Name:        item.Name,
			Description: item.Description,
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": response})
}

func (h *Handler) ListLevelProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	items, err := h.repo.ListLevelProgressBySpace(r.Context(), claims.UserID, spaceID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := make([]levelProgressResponse, 0, len(items))
	for _, item := range items {
		response = append(response, levelProgressResponse{
			RelationshipSpaceID: item.RelationshipSpaceID,
			Level:               item.Level,
			RequiredPoints:      item.RequiredPoints,
			CurrentPoints:       item.CurrentPoints,
			UnlockedAt:          timeToUnixPtr(item.UnlockedAt),
			CreatedAt:           item.CreatedAt.Unix(),
			UpdatedAt:           item.UpdatedAt.Unix(),
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": response})
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	items, err := h.repo.ListSpaceMembers(r.Context(), claims.UserID, spaceID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	response := make([]memberResponse, 0, len(items))
	for _, item := range items {
		response = append(response, memberResponse{
			UserID:      item.UserID,
			DisplayName: item.DisplayName,
			Identifier:  item.Identifier,
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": response})
}

func (h *Handler) UpdateSpaceSettings(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	var req updateSpaceSettingsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	var name *string
	if req.Name != nil {
		cleaned := strings.TrimSpace(*req.Name)
		if len([]rune(cleaned)) > 120 {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "name must be 120 characters or fewer")
			return
		}
		if cleaned != "" {
			name = &cleaned
		}
	}

	var relationshipStartDate *time.Time
	if req.RelationshipStartDate != nil {
		cleaned := strings.TrimSpace(*req.RelationshipStartDate)
		parsed, err := time.Parse("2006-01-02", cleaned)
		if err != nil {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "relationship_start_date must use YYYY-MM-DD")
			return
		}
		if parsed.Before(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "relationship_start_date is too old")
			return
		}
		if parsed.After(time.Now().UTC().Truncate(24 * time.Hour)) {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "relationship_start_date cannot be in the future")
			return
		}
		relationshipStartDate = &parsed
	}

	item, memberIDs, err := h.repo.UpdateSpaceSettings(
		r.Context(),
		claims.UserID,
		spaceID,
		name,
		relationshipStartDate,
		req.CelebrateMonthsary,
		req.CelebrateAnniversary,
	)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.sendSpaceUpdatedEvent(memberIDs, item)
	httputil.JSON(w, http.StatusOK, toSpaceResponse(item))
}

func (h *Handler) UploadCoverPhoto(w http.ResponseWriter, r *http.Request) {
	h.uploadSpaceImage(w, r, "cover-photo")
}

func (h *Handler) UploadCouplePhoto(w http.ResponseWriter, r *http.Request) {
	h.uploadSpaceImage(w, r, "couple-photo")
}

func (h *Handler) GetSpaceMedia(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}
	kind := strings.TrimSpace(r.PathValue("kind"))
	if kind != "cover-photo" && kind != "couple-photo" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "unsupported media kind")
		return
	}
	if h.mediaStore == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "image_unavailable", "relationship space images are unavailable")
		return
	}

	imagePath, err := h.repo.GetSpaceMediaPath(r.Context(), claims.UserID, spaceID, kind)
	if err != nil {
		h.writeError(w, err)
		return
	}

	data, contentType, err := h.mediaStore.ReadImage(r.Context(), imagePath)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) || errors.Is(err, ErrImageRequired) || errors.Is(err, os.ErrNotExist) {
			h.writeError(w, ErrSpaceImageNotFound)
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) ListCurrentMoods(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	items, err := h.repo.ListCurrentSpaceMoods(r.Context(), claims.UserID, spaceID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if len(items) == 0 {
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
		return
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"items": toMoodResponses(items)})
}

func (h *Handler) SetMood(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	var req moodRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.MoodKey = strings.TrimSpace(req.MoodKey)
	req.Emoji = strings.TrimSpace(req.Emoji)
	req.Label = strings.TrimSpace(req.Label)
	if req.MoodKey == "" || req.Emoji == "" || req.Label == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "mood_key, emoji, and label are required")
		return
	}
	if len([]rune(req.MoodKey)) > 64 || len([]rune(req.Emoji)) > 16 || len([]rune(req.Label)) > 64 {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "mood values are too long")
		return
	}

	items, memberIDs, err := h.repo.CreateSpaceMood(r.Context(), claims.UserID, spaceID, req.MoodKey, req.Emoji, req.Label)
	if err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if h.hub != nil {
		event := wsutil.NewEvent("relationship_mood_updated", map[string]string{
			"relationship_space_id": spaceID,
			"user_id":               claims.UserID,
			"mood_key":              req.MoodKey,
			"emoji":                 req.Emoji,
			"label":                 req.Label,
		})
		for _, memberID := range memberIDs {
			h.hub.SendToUser(memberID, event)
		}
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"items": toMoodResponses(items)})
}

func (h *Handler) SetDefaultSpace(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	if err := h.repo.SetDefaultSpace(r.Context(), claims.UserID, spaceID); err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) ConfigureSpaceAccess(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	var req configureSpaceAccessRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.Passphrase = strings.TrimSpace(req.Passphrase)
	if len(req.Passphrase) < 4 {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "passphrase must be at least 4 characters")
		return
	}

	if err := h.repo.UpsertSpaceAccess(r.Context(), claims.UserID, spaceID, req.Passphrase, req.Hint); err != nil {
		if errors.Is(err, ErrSpaceNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) UnlockSpace(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	var req unlockSpaceRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.Passphrase = strings.TrimSpace(req.Passphrase)
	if req.Passphrase == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "passphrase is required")
		return
	}

	item, err := h.repo.UnlockSpace(r.Context(), claims.UserID, req.Passphrase)
	if err != nil {
		if errors.Is(err, ErrSpaceAccessNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "hidden space not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	httputil.JSON(w, http.StatusOK, toSpaceResponse(item))
}

func (h *Handler) uploadSpaceImage(w http.ResponseWriter, r *http.Request, kind string) {
	claims, ok := auth.AccessClaimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}
	if h.mediaStore == nil {
		httputil.Error(w, http.StatusServiceUnavailable, "image_unavailable", "relationship space image uploads are unavailable")
		return
	}
	if err := r.ParseMultipartForm(9 << 20); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid multipart body")
		return
	}

	file, err := openUploadedImageFile(r, "image")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			httputil.Error(w, http.StatusBadRequest, "invalid_request", "image is required")
			return
		}
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid image upload")
		return
	}
	defer file.Close()

	storedPath, err := h.mediaStore.SaveSpaceImage(r.Context(), spaceID, kind, file)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	item, oldPath, memberIDs, err := h.repo.UpdateSpaceMediaPath(r.Context(), claims.UserID, spaceID, kind, storedPath)
	if err != nil {
		_ = h.mediaStore.Delete(r.Context(), storedPath)
		h.writeError(w, err)
		return
	}
	if oldPath != nil && strings.TrimSpace(*oldPath) != "" && *oldPath != storedPath {
		_ = h.mediaStore.Delete(r.Context(), *oldPath)
	}

	h.sendSpaceUpdatedEvent(memberIDs, item)
	httputil.JSON(w, http.StatusOK, toSpaceResponse(item))
}

func openUploadedImageFile(r *http.Request, fieldName string) (multipart.File, error) {
	file, _, err := r.FormFile(fieldName)
	if err == nil || !errors.Is(err, http.ErrMissingFile) {
		return file, err
	}

	form := r.MultipartForm
	if form == nil {
		return nil, http.ErrMissingFile
	}

	for _, fallbackKey := range []string{"file", "photo", "attachment"} {
		headers := form.File[fallbackKey]
		if len(headers) == 0 {
			continue
		}
		return headers[0].Open()
	}

	var onlyHeader *multipart.FileHeader
	for _, headers := range form.File {
		for _, header := range headers {
			if onlyHeader != nil {
				return nil, http.ErrMissingFile
			}
			onlyHeader = header
		}
	}
	if onlyHeader == nil {
		return nil, http.ErrMissingFile
	}
	return onlyHeader.Open()
}

func (h *Handler) sendSpaceUpdatedEvent(memberIDs []string, item SpaceSummary) {
	if h.hub == nil {
		return
	}
	event := wsutil.NewEvent("relationship_space_updated", map[string]any{
		"relationship_space_id":   item.RelationshipSpaceID,
		"name":                    stringValue(item.Name),
		"cover_photo_url":         spaceMediaURL(item.RelationshipSpaceID, "cover-photo", item.CoverPhotoPath),
		"cover_photo_version":     spaceMediaVersion(item.CoverPhotoPath),
		"couple_photo_url":        spaceMediaURL(item.RelationshipSpaceID, "couple-photo", item.CouplePhotoPath),
		"couple_photo_version":    spaceMediaVersion(item.CouplePhotoPath),
		"relationship_start_date": formatDate(item.RelationshipStartDate),
		"celebrate_monthsary":     item.CelebrateMonthsary,
		"celebrate_anniversary":   item.CelebrateAnniversary,
	})
	for _, memberID := range memberIDs {
		h.hub.SendToUser(memberID, event)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid relationship space request")
	case errors.Is(err, ErrSpaceNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
	case errors.Is(err, ErrSpaceImageNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space image not found")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func toSpaceResponse(item SpaceSummary) spaceResponse {
	return spaceResponse{
		RelationshipSpaceID:   item.RelationshipSpaceID,
		ConversationID:        item.ConversationID,
		Name:                  item.Name,
		CoverPhotoURL:         spaceMediaURL(item.RelationshipSpaceID, "cover-photo", item.CoverPhotoPath),
		CoverPhotoVersion:     spaceMediaVersion(item.CoverPhotoPath),
		CouplePhotoURL:        spaceMediaURL(item.RelationshipSpaceID, "couple-photo", item.CouplePhotoPath),
		CouplePhotoVersion:    spaceMediaVersion(item.CouplePhotoPath),
		RelationshipStartDate: formatDate(item.RelationshipStartDate),
		CelebrateMonthsary:    item.CelebrateMonthsary,
		CelebrateAnniversary:  item.CelebrateAnniversary,
		CreatedByUserID:       item.CreatedByUserID,
		CurrentLevel:          item.CurrentLevel,
		CurrentLevelName:      item.CurrentLevelName,
		IsDefault:             item.IsDefault,
		AccessHint:            item.AccessHint,
		AccessConfigured:      item.AccessConfigured,
		ArchivedAt:            timeToUnixPtr(item.ArchivedAt),
		CreatedAt:             item.CreatedAt.Unix(),
		UpdatedAt:             item.UpdatedAt.Unix(),
	}
}

func formatDate(value time.Time) string {
	return value.UTC().Format("2006-01-02")
}

func spaceMediaURL(spaceID, kind string, imagePath *string) *string {
	if imagePath == nil || strings.TrimSpace(*imagePath) == "" {
		return nil
	}
	url := fmt.Sprintf("/v1/relationship-spaces/%s/media/%s", spaceID, kind)
	return &url
}

func spaceMediaVersion(imagePath *string) *string {
	if imagePath == nil || strings.TrimSpace(*imagePath) == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(*imagePath)))
	version := hex.EncodeToString(sum[:8])
	return &version
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func toMoodResponses(items []SpaceMood) []moodResponse {
	response := make([]moodResponse, 0, len(items))
	for _, item := range items {
		resp := moodResponse{
			RelationshipSpaceID: item.RelationshipSpaceID,
			UserID:              item.UserID,
			DisplayName:         item.DisplayName,
			UpdatedAt:           timeToUnixPtr(item.UpdatedAt),
			IsMe:                item.IsMe,
		}
		if item.MoodKey != nil {
			resp.MoodKey = *item.MoodKey
		}
		if item.Emoji != nil {
			resp.Emoji = *item.Emoji
		}
		if item.Label != nil {
			resp.Label = *item.Label
		}
		response = append(response, resp)
	}
	return response
}

func timeToUnixPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	ts := value.Unix()
	return &ts
}
