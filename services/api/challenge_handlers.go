package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"xend.chat/m/internal/challenges"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type ChallengeHandler struct {
	repo            *challenges.Repository
	submissionStore *challenges.SubmissionStore
	hub             *realtime.Hub
}

func NewChallengeHandler(repo *challenges.Repository, submissionStore *challenges.SubmissionStore, hub *realtime.Hub) *ChallengeHandler {
	return &ChallengeHandler{
		repo:            repo,
		submissionStore: submissionStore,
		hub:             hub,
	}
}

type createChallengeRequest struct {
	TemplateID string  `json:"template_id"`
	Note       *string `json:"note"`
}

type completeChallengeRequest struct {
	TextResponse *string `json:"text_response"`
}

type challengeTemplateResponse struct {
	TemplateID     string `json:"template_id"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	Category       string `json:"category"`
	IconKey        string `json:"icon_key"`
	SubmissionType string `json:"submission_type"`
	MinLevel       int16  `json:"min_level"`
	MaxLevel       *int16 `json:"max_level,omitempty"`
	DefaultPoints  int    `json:"default_points"`
	ExpiryHours    *int   `json:"expiry_hours,omitempty"`
	DisplayOrder   int16  `json:"display_order"`
}

type challengeOverviewResponse struct {
	RelationshipSpaceID string                  `json:"relationship_space_id"`
	Incoming            []challengeItemResponse `json:"incoming"`
	Sent                []challengeItemResponse `json:"sent"`
	History             []challengeItemResponse `json:"history"`
}

type challengeItemResponse struct {
	ChallengeID         string  `json:"challenge_id"`
	RelationshipSpaceID string  `json:"relationship_space_id"`
	TemplateID          string  `json:"template_id"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	Category            string  `json:"category"`
	IconKey             string  `json:"icon_key"`
	SubmissionType      string  `json:"submission_type"`
	SenderUserID        string  `json:"sender_user_id"`
	SenderDisplayName   string  `json:"sender_display_name"`
	ReceiverUserID      string  `json:"receiver_user_id"`
	ReceiverDisplayName string  `json:"receiver_display_name"`
	AssignedLevel       int16   `json:"assigned_level"`
	RewardPoints        int     `json:"reward_points"`
	Note                *string `json:"note,omitempty"`
	Status              string  `json:"status"`
	ExpiresAt           *int64  `json:"expires_at,omitempty"`
	AcceptedAt          *int64  `json:"accepted_at,omitempty"`
	CompletedAt         *int64  `json:"completed_at,omitempty"`
	CreatedAt           int64   `json:"created_at"`
	UpdatedAt           int64   `json:"updated_at"`
	SubmittedByMe       bool    `json:"submitted_by_me"`
	CanAccept           bool    `json:"can_accept"`
	CanDecline          bool    `json:"can_decline"`
	CanComplete         bool    `json:"can_complete"`
}

func (h *ChallengeHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
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

	items, err := h.repo.ListTemplates(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	resp := make([]challengeTemplateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, challengeTemplateResponse{
			TemplateID:     item.ID,
			Slug:           item.Slug,
			Title:          item.Title,
			Description:    item.Description,
			Category:       item.Category,
			IconKey:        item.IconKey,
			SubmissionType: string(item.SubmissionType),
			MinLevel:       item.MinLevel,
			MaxLevel:       item.MaxLevel,
			DefaultPoints:  item.DefaultPoints,
			ExpiryHours:    item.ExpiryHours,
			DisplayOrder:   item.DisplayOrder,
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *ChallengeHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
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

	overview, err := h.repo.GetOverview(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, toChallengeOverviewResponse(overview))
}

func (h *ChallengeHandler) Create(w http.ResponseWriter, r *http.Request) {
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

	var req createChallengeRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	if req.TemplateID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "template_id is required")
		return
	}

	overview, partnerUserID, err := h.repo.CreateChallenge(r.Context(), claims.UserID, spaceID, req.TemplateID, req.Note)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.sendEvent(partnerUserID, "challenge_received", map[string]any{
		"relationship_space_id": spaceID,
	})
	httputil.JSON(w, http.StatusCreated, toChallengeOverviewResponse(overview))
}

func (h *ChallengeHandler) Accept(w http.ResponseWriter, r *http.Request) {
	h.handleTransition(w, r, func(userID, spaceID, challengeID string) (challenges.Overview, string, error) {
		return h.repo.AcceptChallenge(r.Context(), userID, spaceID, challengeID)
	}, "challenge_accepted")
}

func (h *ChallengeHandler) Decline(w http.ResponseWriter, r *http.Request) {
	h.handleTransition(w, r, func(userID, spaceID, challengeID string) (challenges.Overview, string, error) {
		return h.repo.DeclineChallenge(r.Context(), userID, spaceID, challengeID)
	}, "challenge_declined")
}

func (h *ChallengeHandler) Complete(w http.ResponseWriter, r *http.Request) {
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
	challengeID := strings.TrimSpace(r.PathValue("challenge_id"))
	if challengeID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "challenge_id is required")
		return
	}

	submission, storedPath, err := h.parseCompletionRequest(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	overview, senderUserID, err := h.repo.CompleteChallenge(r.Context(), claims.UserID, spaceID, challengeID, submission)
	if err != nil {
		if storedPath != "" && h.submissionStore != nil {
			_ = h.submissionStore.Delete(storedPath)
		}
		h.writeError(w, err)
		return
	}

	h.sendEvent(senderUserID, "challenge_completed", map[string]any{
		"relationship_space_id": spaceID,
		"challenge_id":          challengeID,
	})
	httputil.JSON(w, http.StatusOK, toChallengeOverviewResponse(overview))
}

func (h *ChallengeHandler) handleTransition(
	w http.ResponseWriter,
	r *http.Request,
	action func(userID, spaceID, challengeID string) (challenges.Overview, string, error),
	eventType string,
) {
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
	challengeID := strings.TrimSpace(r.PathValue("challenge_id"))
	if challengeID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "challenge_id is required")
		return
	}

	overview, otherUserID, err := action(claims.UserID, spaceID, challengeID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.sendEvent(otherUserID, eventType, map[string]any{
		"relationship_space_id": spaceID,
		"challenge_id":          challengeID,
	})
	httputil.JSON(w, http.StatusOK, toChallengeOverviewResponse(overview))
}

func (h *ChallengeHandler) parseCompletionRequest(r *http.Request) (challenges.Submission, string, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return h.parseMultipartCompletionRequest(r)
	}

	var req completeChallengeRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		return challenges.Submission{}, "", errors.New("invalid JSON body")
	}
	return challenges.Submission{TextResponse: req.TextResponse}, "", nil
}

func (h *ChallengeHandler) parseMultipartCompletionRequest(r *http.Request) (challenges.Submission, string, error) {
	if err := r.ParseMultipartForm(9 << 20); err != nil {
		return challenges.Submission{}, "", errors.New("invalid multipart body")
	}

	var storedPath string
	var imagePath *string
	file, _, err := r.FormFile("image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		return challenges.Submission{}, "", errors.New("invalid image upload")
	}
	if err == nil {
		defer file.Close()
		if h.submissionStore == nil {
			return challenges.Submission{}, "", errors.New("image uploads are unavailable")
		}
		path, saveErr := h.submissionStore.SaveImage(file)
		if saveErr != nil {
			return challenges.Submission{}, "", saveErr
		}
		storedPath = path
		imagePath = &storedPath
	}

	textResponse := normalizeChallengeOptionalText(r.FormValue("text_response"))
	return challenges.Submission{
		TextResponse: textResponse,
		ImagePath:    imagePath,
	}, storedPath, nil
}

func normalizeChallengeOptionalText(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (h *ChallengeHandler) sendEvent(userID, eventType string, payload map[string]any) {
	if h.hub == nil || strings.TrimSpace(userID) == "" {
		return
	}
	h.hub.SendToUser(userID, wsutil.NewEvent(eventType, payload))
}

func (h *ChallengeHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, challenges.ErrRelationshipSpaceNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
	case errors.Is(err, challenges.ErrTemplateNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "challenge template not found")
	case errors.Is(err, challenges.ErrPartnerNotFound):
		httputil.Error(w, http.StatusConflict, "partner_not_found", "challenge partner is not available")
	case errors.Is(err, challenges.ErrChallengeNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "challenge not found")
	case errors.Is(err, challenges.ErrChallengeUnavailable):
		httputil.Error(w, http.StatusConflict, "challenge_unavailable", "challenge is no longer available")
	case errors.Is(err, challenges.ErrChallengeNotAllowed):
		httputil.Error(w, http.StatusForbidden, "challenge_not_allowed", "challenge action is not allowed")
	case errors.Is(err, challenges.ErrTextResponseRequired):
		httputil.Error(w, http.StatusBadRequest, "text_response_required", "text response is required")
	case errors.Is(err, challenges.ErrImageRequired):
		httputil.Error(w, http.StatusBadRequest, "image_required", "image is required")
	case errors.Is(err, challenges.ErrUnsupportedSubmissionType):
		httputil.Error(w, http.StatusUnprocessableEntity, "unsupported_submission_type", "challenge submission type is not supported")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func toChallengeOverviewResponse(overview challenges.Overview) challengeOverviewResponse {
	return challengeOverviewResponse{
		RelationshipSpaceID: overview.RelationshipSpaceID,
		Incoming:            toChallengeItemResponses(overview.Incoming),
		Sent:                toChallengeItemResponses(overview.Sent),
		History:             toChallengeItemResponses(overview.History),
	}
}

func toChallengeItemResponses(items []challenges.Challenge) []challengeItemResponse {
	resp := make([]challengeItemResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, challengeItemResponse{
			ChallengeID:         item.ChallengeID,
			RelationshipSpaceID: item.RelationshipSpaceID,
			TemplateID:          item.TemplateID,
			Title:               item.Title,
			Description:         item.Description,
			Category:            item.Category,
			IconKey:             item.IconKey,
			SubmissionType:      string(item.SubmissionType),
			SenderUserID:        item.SenderUserID,
			SenderDisplayName:   item.SenderDisplayName,
			ReceiverUserID:      item.ReceiverUserID,
			ReceiverDisplayName: item.ReceiverDisplayName,
			AssignedLevel:       item.AssignedLevel,
			RewardPoints:        item.RewardPoints,
			Note:                item.Note,
			Status:              string(item.Status),
			ExpiresAt:           unixOrNil(item.ExpiresAt),
			AcceptedAt:          unixOrNil(item.AcceptedAt),
			CompletedAt:         unixOrNil(item.CompletedAt),
			CreatedAt:           item.CreatedAt.Unix(),
			UpdatedAt:           item.UpdatedAt.Unix(),
			SubmittedByMe:       item.SubmittedByMe,
			CanAccept:           item.CanAccept,
			CanDecline:          item.CanDecline,
			CanComplete:         item.CanComplete,
		})
	}
	return resp
}

func unixOrNil(value *time.Time) *int64 {
	if value == nil {
		return nil
	}
	unix := value.Unix()
	return &unix
}
