package api

import (
	"errors"
	"net/http"
	"strings"

	"xend.chat/m/internal/dailyritual"
	"xend.chat/m/pkg/httputil"
)

type DailyRitualHandler struct {
	repo *dailyritual.Repository
}

func NewDailyRitualHandler(repo *dailyritual.Repository) *DailyRitualHandler {
	return &DailyRitualHandler{repo: repo}
}

type updateSpaceDailyRitualsRequest struct {
	TemplateIDs []string `json:"template_ids"`
}

type dailyRitualTemplateResponse struct {
	TemplateID    string  `json:"template_id"`
	Slug          string  `json:"slug"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	Category      string  `json:"category"`
	IconKey       string  `json:"icon_key"`
	SuggestedTime *string `json:"suggested_time,omitempty"`
	DefaultPoints int     `json:"default_points"`
	DisplayOrder  int16   `json:"display_order"`
}

type spaceDailyRitualSelectionResponse struct {
	SelectionID         string  `json:"selection_id"`
	RelationshipSpaceID string  `json:"relationship_space_id"`
	TemplateID          string  `json:"template_id"`
	SelectedByUserID    string  `json:"selected_by_user_id"`
	SortOrder           int16   `json:"sort_order"`
	SelectedAt          int64   `json:"selected_at"`
	Slug                string  `json:"slug"`
	Title               string  `json:"title"`
	Description         string  `json:"description"`
	Category            string  `json:"category"`
	IconKey             string  `json:"icon_key"`
	SuggestedTime       *string `json:"suggested_time,omitempty"`
	DefaultPoints       int     `json:"default_points"`
}

func (h *DailyRitualHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := claimsFromContext(r.Context()); !ok {
		httputil.Error(w, http.StatusUnauthorized, "invalid_token", "access token is invalid")
		return
	}

	items, err := h.repo.ListTemplates(r.Context())
	if err != nil {
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	resp := make([]dailyRitualTemplateResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, dailyRitualTemplateResponse{
			TemplateID:    item.ID,
			Slug:          item.Slug,
			Title:         item.Title,
			Description:   item.Description,
			Category:      item.Category,
			IconKey:       item.IconKey,
			SuggestedTime: item.SuggestedTime,
			DefaultPoints: item.DefaultPoints,
			DisplayOrder:  item.DisplayOrder,
		})
	}
	httputil.JSON(w, http.StatusOK, map[string]any{"items": resp})
}

func (h *DailyRitualHandler) GetSpaceSelections(w http.ResponseWriter, r *http.Request) {
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

	items, err := h.repo.ListSpaceSelections(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"max_active_rituals": dailyritual.MaxActiveSelections,
		"items":              toSpaceDailyRitualSelectionsResponse(items),
	})
}

func (h *DailyRitualHandler) UpdateSpaceSelections(w http.ResponseWriter, r *http.Request) {
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

	var req updateSpaceDailyRitualsRequest
	if err := httputil.DecodeJSON(r, &req); err != nil {
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	items, err := h.repo.ReplaceSpaceSelections(r.Context(), claims.UserID, spaceID, req.TemplateIDs)
	if err != nil {
		h.writeError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, map[string]any{
		"max_active_rituals": dailyritual.MaxActiveSelections,
		"items":              toSpaceDailyRitualSelectionsResponse(items),
	})
}

func (h *DailyRitualHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dailyritual.ErrRelationshipSpaceNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
	case errors.Is(err, dailyritual.ErrInvalidTemplateID):
		httputil.Error(w, http.StatusBadRequest, "invalid_request", "one or more ritual template ids are invalid")
	case errors.Is(err, dailyritual.ErrTemplateNotFound):
		httputil.Error(w, http.StatusUnprocessableEntity, "invalid_request", "one or more ritual templates are unavailable")
	case errors.Is(err, dailyritual.ErrSelectionLimitExceeded):
		httputil.Error(w, http.StatusUnprocessableEntity, "selection_limit_exceeded", "you can select up to 3 daily rituals")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func toSpaceDailyRitualSelectionsResponse(items []dailyritual.SpaceSelection) []spaceDailyRitualSelectionResponse {
	resp := make([]spaceDailyRitualSelectionResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, spaceDailyRitualSelectionResponse{
			SelectionID:         item.SelectionID,
			RelationshipSpaceID: item.RelationshipSpaceID,
			TemplateID:          item.TemplateID,
			SelectedByUserID:    item.SelectedByUserID,
			SortOrder:           item.SortOrder,
			SelectedAt:          item.SelectedAt.Unix(),
			Slug:                item.TemplateSlug,
			Title:               item.TemplateTitle,
			Description:         item.TemplateDescription,
			Category:            item.TemplateCategory,
			IconKey:             item.TemplateIconKey,
			SuggestedTime:       item.TemplateSuggestedTime,
			DefaultPoints:       item.TemplateDefaultPoints,
		})
	}
	return resp
}
