package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"xend.chat/m/internal/auth"
	"xend.chat/m/internal/notify"
	"xend.chat/m/internal/queue"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type RelationshipHandler struct {
	repo          *auth.Repository
	emailEnqueuer *queue.VerificationEmailEnqueuer
	hub           *realtime.Hub
	pushNotifier  notify.PushNotifier
}

func NewRelationshipHandler(repo *auth.Repository, emailEnqueuer *queue.VerificationEmailEnqueuer, hub *realtime.Hub, pushNotifier notify.PushNotifier) *RelationshipHandler {
	return &RelationshipHandler{repo: repo, emailEnqueuer: emailEnqueuer, hub: hub, pushNotifier: pushNotifier}
}

type createInviteRequest struct {
	Identifier string  `json:"identifier"`
	Note       *string `json:"note"`
}

type relationshipSpaceResponse struct {
	RelationshipSpaceID string  `json:"relationship_space_id"`
	Name                *string `json:"name,omitempty"`
	CreatedByUserID     string  `json:"created_by_user_id"`
	CurrentLevel        int16   `json:"current_level"`
	CurrentLevelName    string  `json:"current_level_name"`
	ArchivedAt          *int64  `json:"archived_at,omitempty"`
	CreatedAt           int64   `json:"created_at"`
	UpdatedAt           int64   `json:"updated_at"`
}

type relationshipLevelResponse struct {
	Level       int16   `json:"level"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type relationshipLevelProgressResponse struct {
	RelationshipSpaceID string `json:"relationship_space_id"`
	Level               int16  `json:"level"`
	RequiredPoints      int32  `json:"required_points"`
	CurrentPoints       int32  `json:"current_points"`
	UnlockedAt          *int64 `json:"unlocked_at,omitempty"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

type inviteOutboxResponse struct {
	InviteID          string  `json:"invite_id"`
	InviteeIdentifier string  `json:"invitee_identifier"`
	Status            string  `json:"status"`
	Note              *string `json:"note,omitempty"`
	CreatedAt         int64   `json:"created_at"`
}

func (h *RelationshipHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
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

	inviteID, inviteeUserID, inviteeEmail, err := h.repo.CreateRelationshipInviteByIdentifier(r.Context(), claims.UserID, req.Identifier, req.Note)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidInput) {
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

	// Best-effort email notification for invite recipient.
	if h.emailEnqueuer != nil {
		profile, pErr := h.repo.GetUserProfileByID(r.Context(), claims.UserID)
		if pErr == nil {
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
	} else if h.pushNotifier != nil {
		profile, pErr := h.repo.GetUserProfileByID(r.Context(), claims.UserID)
		if pErr == nil {
			tokens, tErr := h.repo.ListActivePushTokensByUser(r.Context(), inviteeUserID)
			if tErr == nil && len(tokens) > 0 {
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
			} else if tErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID, "error", tErr)
			} else {
				slog.Info("push skipped no active tokens", "event", "relationship_invite_received", "invite_id", inviteID, "target_user_id", inviteeUserID)
			}
		}
	}
	httputil.JSON(w, http.StatusCreated, map[string]string{"invite_id": inviteID})
}

func (h *RelationshipHandler) Inbox(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
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
		items = []auth.RelationshipInvite{}
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *RelationshipHandler) Outbox(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	items, err := h.repo.ListInviteOutbox(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]inviteOutboxResponse, 0, len(items))
	for _, it := range items {
		resp = append(resp, inviteOutboxResponse{
			InviteID:          it.InviteID,
			InviteeIdentifier: it.InviteeIdentifier,
			Status:            it.Status,
			Note:              it.Note,
			CreatedAt:         it.CreatedAt.Unix(),
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *RelationshipHandler) Accept(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	inviteID := r.PathValue("invite_id")
	if strings.TrimSpace(inviteID) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invite_id is required")
		return
	}
	spaceID, conversationID, inviterUserID, err := h.repo.AcceptRelationshipInvite(r.Context(), inviteID, claims.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrInviteNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "invite not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if h.hub != nil {
		h.hub.SendToUser(claims.UserID, wsutil.NewEvent("relationship_invite_accepted", map[string]string{
			"invite_id":             inviteID,
			"relationship_space_id": spaceID,
			"conversation_id":       conversationID,
		}))
		h.hub.SendToUser(inviterUserID, wsutil.NewEvent("relationship_invite_accepted", map[string]string{
			"invite_id":             inviteID,
			"relationship_space_id": spaceID,
			"conversation_id":       conversationID,
		}))
	}
	if h.pushNotifier != nil {
		profile, pErr := h.repo.GetUserProfileByID(r.Context(), claims.UserID)
		if pErr == nil {
			tokens, tErr := h.repo.ListActivePushTokensByUser(r.Context(), inviterUserID)
			if tErr == nil && len(tokens) > 0 {
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
			} else if tErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_accepted", "invite_id", inviteID, "target_user_id", inviterUserID, "error", tErr)
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

func (h *RelationshipHandler) Decline(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}
	inviteID := r.PathValue("invite_id")
	if strings.TrimSpace(inviteID) == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invite_id is required")
		return
	}
	inviterUserID, err := h.repo.DeclineRelationshipInvite(r.Context(), inviteID, claims.UserID)
	if err != nil {
		if errors.Is(err, auth.ErrInviteNotFound) {
			httputil.Error(w, http.StatusNotFound, "not_found", "invite not found")
			return
		}
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}
	if h.hub != nil {
		h.hub.SendToUser(claims.UserID, wsutil.NewEvent("relationship_invite_declined", map[string]string{
			"invite_id": inviteID,
		}))
		h.hub.SendToUser(inviterUserID, wsutil.NewEvent("relationship_invite_declined", map[string]string{
			"invite_id": inviteID,
		}))
	}
	if h.pushNotifier != nil {
		profile, pErr := h.repo.GetUserProfileByID(r.Context(), claims.UserID)
		if pErr == nil {
			tokens, tErr := h.repo.ListActivePushTokensByUser(r.Context(), inviterUserID)
			if tErr == nil && len(tokens) > 0 {
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
			} else if tErr != nil {
				slog.Error("push token lookup failed", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID, "error", tErr)
			} else {
				slog.Info("push skipped no active tokens", "event", "relationship_invite_declined", "invite_id", inviteID, "target_user_id", inviterUserID)
			}
		}
	}
	httputil.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RelationshipHandler) ListSpaces(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListRelationshipSpacesByUser(r.Context(), claims.UserID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]relationshipSpaceResponse, 0, len(items))
	for _, it := range items {
		resp = append(resp, relationshipSpaceResponse{
			RelationshipSpaceID: it.RelationshipSpaceID,
			Name:                it.Name,
			CreatedByUserID:     it.CreatedByUserID,
			CurrentLevel:        it.CurrentLevel,
			CurrentLevelName:    it.CurrentLevelName,
			ArchivedAt:          timeToUnixPtr(it.ArchivedAt),
			CreatedAt:           it.CreatedAt.Unix(),
			UpdatedAt:           it.UpdatedAt.Unix(),
		})
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *RelationshipHandler) ListLevels(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListRelationshipLevels(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]relationshipLevelResponse, 0, len(items))
	for _, it := range items {
		resp = append(resp, relationshipLevelResponse{
			Level:       it.Level,
			Name:        it.Name,
			Description: it.Description,
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *RelationshipHandler) ListLevelProgress(w http.ResponseWriter, r *http.Request) {
	claims, ok := claimsFromContext(r.Context())
	if !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	spaceID := strings.TrimSpace(r.PathValue("space_id"))
	if spaceID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "space_id is required")
		return
	}

	items, err := h.repo.ListRelationshipLevelProgressBySpace(r.Context(), claims.UserID, spaceID)
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]relationshipLevelProgressResponse, 0, len(items))
	for _, it := range items {
		resp = append(resp, relationshipLevelProgressResponse{
			RelationshipSpaceID: it.RelationshipSpaceID,
			Level:               it.Level,
			RequiredPoints:      it.RequiredPoints,
			CurrentPoints:       it.CurrentPoints,
			UnlockedAt:          timeToUnixPtr(it.UnlockedAt),
			CreatedAt:           it.CreatedAt.Unix(),
			UpdatedAt:           it.UpdatedAt.Unix(),
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func timeToUnixPtr(v *time.Time) *int64 {
	if v == nil {
		return nil
	}
	ts := v.Unix()
	return &ts
}
