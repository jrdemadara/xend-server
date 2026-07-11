package relationship

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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
}

func NewHandler(repo *Repository, users *user.Repository, devices *device.Repository, emailEnqueuer *queue.VerificationEmailEnqueuer, hub *realtime.Hub, pushNotifier notify.PushNotifier) *Handler {
	return &Handler{
		repo:          repo,
		users:         users,
		devices:       devices,
		emailEnqueuer: emailEnqueuer,
		hub:           hub,
		pushNotifier:  pushNotifier,
	}
}

type createInviteRequest struct {
	Identifier string  `json:"identifier"`
	Note       *string `json:"note"`
}

type spaceResponse struct {
	RelationshipSpaceID string  `json:"relationship_space_id"`
	ConversationID      string  `json:"conversation_id"`
	Name                *string `json:"name,omitempty"`
	CreatedByUserID     string  `json:"created_by_user_id"`
	CurrentLevel        int16   `json:"current_level"`
	CurrentLevelName    string  `json:"current_level_name"`
	IsDefault           bool    `json:"is_default"`
	AccessHint          *string `json:"access_hint,omitempty"`
	AccessConfigured    bool    `json:"access_configured"`
	ArchivedAt          *int64  `json:"archived_at,omitempty"`
	CreatedAt           int64   `json:"created_at"`
	UpdatedAt           int64   `json:"updated_at"`
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

func toSpaceResponse(item SpaceSummary) spaceResponse {
	return spaceResponse{
		RelationshipSpaceID: item.RelationshipSpaceID,
		ConversationID:      item.ConversationID,
		Name:                item.Name,
		CreatedByUserID:     item.CreatedByUserID,
		CurrentLevel:        item.CurrentLevel,
		CurrentLevelName:    item.CurrentLevelName,
		IsDefault:           item.IsDefault,
		AccessHint:          item.AccessHint,
		AccessConfigured:    item.AccessConfigured,
		ArchivedAt:          timeToUnixPtr(item.ArchivedAt),
		CreatedAt:           item.CreatedAt.Unix(),
		UpdatedAt:           item.UpdatedAt.Unix(),
	}
}

func timeToUnixPtr(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	ts := value.Unix()
	return &ts
}
