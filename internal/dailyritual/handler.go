package dailyritual

import (
	"errors"
	"mime/multipart"
	"net/http"
	"strings"

	"xend.chat/m/internal/auth"
	"xend.chat/m/pkg/httputil"
)

type Handler struct {
	repo            *Repository
	submissionStore *SubmissionStore
}

func NewHandler(repo *Repository, submissionStore *SubmissionStore) *Handler {
	return &Handler{
		repo:            repo,
		submissionStore: submissionStore,
	}
}

type submitRequest struct {
	TextResponse *string `json:"text_response"`
}

type templateResponse struct {
	TemplateID     string  `json:"template_id"`
	Slug           string  `json:"slug"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Category       string  `json:"category"`
	IconKey        string  `json:"icon_key"`
	SuggestedTime  *string `json:"suggested_time,omitempty"`
	DefaultPoints  int     `json:"default_points"`
	SubmissionType string  `json:"submission_type"`
	TargetType     string  `json:"target_type"`
	CompletionRule string  `json:"completion_rule"`
	MinLevel       int16   `json:"min_level"`
	MaxLevel       *int16  `json:"max_level,omitempty"`
	DisplayOrder   int16   `json:"display_order"`
}

type overviewResponse struct {
	RelationshipSpaceID string             `json:"relationship_space_id"`
	RitualDate          string             `json:"ritual_date"`
	TodayRitual         *assignedResponse  `json:"today_ritual,omitempty"`
	History             []assignedResponse `json:"history"`
}

type assignedResponse struct {
	AssignmentID   string  `json:"assignment_id"`
	RitualDate     string  `json:"ritual_date"`
	Title          string  `json:"title"`
	Description    string  `json:"description"`
	Category       string  `json:"category"`
	IconKey        string  `json:"icon_key"`
	SuggestedTime  *string `json:"suggested_time,omitempty"`
	RewardPoints   int     `json:"reward_points"`
	SubmissionType string  `json:"submission_type"`
	TargetType     string  `json:"target_type"`
	CompletionRule string  `json:"completion_rule"`
	Status         string  `json:"status"`
	Completed      bool    `json:"completed"`
	TargetUserID   *string `json:"target_user_id,omitempty"`
	SubmittedByMe  bool    `json:"submitted_by_me"`
	SubmittedCount int     `json:"submitted_count"`
	RequiredCount  int     `json:"required_count"`
	CanSubmit      bool    `json:"can_submit"`
}

func (h *Handler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.AccessClaimsFromContext(r.Context()); !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListTemplates(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]templateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, templateResponse{
			TemplateID:     item.ID,
			Slug:           item.Slug,
			Title:          item.Title,
			Description:    item.Description,
			Category:       item.Category,
			IconKey:        item.IconKey,
			SuggestedTime:  item.SuggestedTime,
			DefaultPoints:  item.DefaultPoints,
			SubmissionType: string(item.SubmissionType),
			TargetType:     string(item.TargetType),
			CompletionRule: string(item.CompletionRule),
			MinLevel:       item.MinLevel,
			MaxLevel:       item.MaxLevel,
			DisplayOrder:   item.DisplayOrder,
		})
	}

	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
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

	overview, err := h.repo.GetOverview(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, overviewResponse{
		RelationshipSpaceID: overview.RelationshipSpaceID,
		RitualDate:          overview.RitualDate,
		TodayRitual:         toAssignedResponse(overview.TodayRitual),
		History:             toAssignedListResponse(overview.History),
	})
}

func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
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

	assignmentID := strings.TrimSpace(r.PathValue("assignment_id"))
	if assignmentID == "" {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "assignment_id is required")
		return
	}

	submission, storedPath, err := h.parseSubmissionRequest(r)
	if err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	overview, err := h.repo.Submit(r.Context(), claims.UserID, spaceID, assignmentID, submission)
	if err != nil {
		if storedPath != "" && h.submissionStore != nil {
			_ = h.submissionStore.Delete(r.Context(), storedPath)
		}
		h.writeError(w, err)
		return
	}

	httputil.JSON(w, http.StatusOK, overviewResponse{
		RelationshipSpaceID: overview.RelationshipSpaceID,
		RitualDate:          overview.RitualDate,
		TodayRitual:         toAssignedResponse(overview.TodayRitual),
		History:             toAssignedListResponse(overview.History),
	})
}

func (h *Handler) parseSubmissionRequest(r *http.Request) (Submission, string, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return h.parseMultipartSubmissionRequest(r)
	}

	var req submitRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		return Submission{}, "", errors.New("invalid JSON body")
	}
	return Submission{
		TextResponse: req.TextResponse,
	}, "", nil
}

func (h *Handler) parseMultipartSubmissionRequest(r *http.Request) (Submission, string, error) {
	if err := r.ParseMultipartForm(9 << 20); err != nil {
		return Submission{}, "", errors.New("invalid multipart body")
	}

	var storedPath string
	var imagePath *string
	file, err := openUploadedImageFile(r, "image")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		return Submission{}, "", errors.New("invalid image upload")
	}
	if err == nil {
		defer file.Close()
		if h.submissionStore == nil {
			return Submission{}, "", errors.New("image uploads are unavailable")
		}
		path, saveErr := h.submissionStore.SaveImage(r.Context(), file)
		if saveErr != nil {
			return Submission{}, "", saveErr
		}
		storedPath = path
		imagePath = &storedPath
	}

	textResponse := normalizeFormText(r.FormValue("text_response"))
	return Submission{
		TextResponse: textResponse,
		ImagePath:    imagePath,
	}, storedPath, nil
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

func normalizeFormText(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRelationshipSpaceNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
	case errors.Is(err, ErrAssignmentNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "daily ritual assignment not found")
	case errors.Is(err, ErrAssignmentUnavailable):
		httputil.Error(w, http.StatusConflict, "ritual_unavailable", "daily ritual is no longer available")
	case errors.Is(err, ErrSubmissionNotAllowed):
		httputil.Error(w, http.StatusForbidden, "submission_not_allowed", "daily ritual submission is not allowed")
	case errors.Is(err, ErrTextResponseRequired):
		httputil.Error(w, http.StatusBadRequest, "text_response_required", "text response is required")
	case errors.Is(err, ErrImageRequired):
		httputil.Error(w, http.StatusBadRequest, "image_required", "image is required")
	case errors.Is(err, ErrUnsupportedSubmissionType):
		httputil.Error(w, http.StatusUnprocessableEntity, "unsupported_submission_type", "daily ritual submission type is not supported yet")
	case errors.Is(err, ErrInvalidTimezone):
		httputil.Error(w, http.StatusUnprocessableEntity, "invalid_timezone", "relationship space timezone is invalid")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func toAssignedListResponse(items []AssignedRitual) []assignedResponse {
	resp := make([]assignedResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, assignedResponse{
			AssignmentID:   item.AssignmentID,
			RitualDate:     item.RitualDate,
			Title:          item.Title,
			Description:    item.Description,
			Category:       item.Category,
			IconKey:        item.IconKey,
			SuggestedTime:  item.SuggestedTime,
			RewardPoints:   item.RewardPoints,
			SubmissionType: string(item.SubmissionType),
			TargetType:     string(item.TargetType),
			CompletionRule: string(item.CompletionRule),
			Status:         string(item.Status),
			Completed:      item.Completed,
			TargetUserID:   item.TargetUserID,
			SubmittedByMe:  item.SubmittedByMe,
			SubmittedCount: item.SubmittedCount,
			RequiredCount:  item.RequiredCount,
			CanSubmit:      item.CanSubmit,
		})
	}
	return resp
}

func toAssignedResponse(item *AssignedRitual) *assignedResponse {
	if item == nil {
		return nil
	}
	return &assignedResponse{
		AssignmentID:   item.AssignmentID,
		RitualDate:     item.RitualDate,
		Title:          item.Title,
		Description:    item.Description,
		Category:       item.Category,
		IconKey:        item.IconKey,
		SuggestedTime:  item.SuggestedTime,
		RewardPoints:   item.RewardPoints,
		SubmissionType: string(item.SubmissionType),
		TargetType:     string(item.TargetType),
		CompletionRule: string(item.CompletionRule),
		Status:         string(item.Status),
		Completed:      item.Completed,
		TargetUserID:   item.TargetUserID,
		SubmittedByMe:  item.SubmittedByMe,
		SubmittedCount: item.SubmittedCount,
		RequiredCount:  item.RequiredCount,
		CanSubmit:      item.CanSubmit,
	}
}
