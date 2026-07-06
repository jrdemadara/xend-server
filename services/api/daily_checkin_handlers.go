package api

import (
	"errors"
	"net/http"
	"strings"

	"xend.chat/m/internal/dailycheckin"
	"xend.chat/m/internal/realtime"
	"xend.chat/m/pkg/httputil"
	"xend.chat/m/pkg/wsutil"
)

type DailyCheckInHandler struct {
	repo *dailycheckin.Repository
	hub  *realtime.Hub
}

func NewDailyCheckInHandler(repo *dailycheckin.Repository, hub *realtime.Hub) *DailyCheckInHandler {
	return &DailyCheckInHandler{repo: repo, hub: hub}
}

type dailyCheckInStatusResponse struct {
	RelationshipSpaceID          string                              `json:"relationship_space_id"`
	Timezone                     string                              `json:"timezone"`
	CheckInDate                  string                              `json:"checkin_date"`
	MyCheckedIn                  bool                                `json:"my_checked_in"`
	PartnerCheckedIn             bool                                `json:"partner_checked_in"`
	AllMembersCheckedIn          bool                                `json:"all_members_checked_in"`
	ActiveMemberCount            int                                 `json:"active_member_count"`
	SubmittedMemberCount         int                                 `json:"submitted_member_count"`
	CompletedDaysCount           int                                 `json:"completed_days_count"`
	CurrentStreak                int                                 `json:"current_streak"`
	DailyRewardAwarded           bool                                `json:"daily_reward_awarded"`
	DailyRewardPoints            int                                 `json:"daily_reward_points"`
	MilestoneAward               *dailyCheckInMilestoneAwardResponse `json:"milestone_award,omitempty"`
	TotalCheckInBondPointsEarned int                                 `json:"total_checkin_bond_points_earned"`
}

type dailyCheckInMilestoneAwardResponse struct {
	MilestoneID   string  `json:"milestone_id"`
	CompletedDays int     `json:"completed_days"`
	BonusPoints   int     `json:"bonus_points"`
	Title         *string `json:"title,omitempty"`
	Description   *string `json:"description,omitempty"`
}

func (h *DailyCheckInHandler) GetToday(w http.ResponseWriter, r *http.Request) {
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

	status, err := h.repo.GetTodayStatus(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, toDailyCheckInStatusResponse(status))
}

func (h *DailyCheckInHandler) Submit(w http.ResponseWriter, r *http.Request) {
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

	status, memberIDs, err := h.repo.SubmitTodayCheckIn(r.Context(), claims.UserID, spaceID)
	if err != nil {
		h.writeError(w, err)
		return
	}

	if h.hub != nil {
		payload := map[string]any{
			"relationship_space_id":            status.RelationshipSpaceID,
			"checkin_date":                     status.CheckInDate,
			"all_members_checked_in":           status.AllMembersCheckedIn,
			"active_member_count":              status.ActiveMemberCount,
			"submitted_member_count":           status.SubmittedMemberCount,
			"completed_days_count":             status.CompletedDaysCount,
			"current_streak":                   status.CurrentStreak,
			"daily_reward_awarded":             status.DailyRewardAwarded,
			"daily_reward_points":              status.DailyRewardPoints,
			"total_checkin_bond_points_earned": status.TotalCheckInBondPointsEarned,
		}
		if status.MilestoneAward != nil {
			payload["milestone_award"] = map[string]any{
				"milestone_id":   status.MilestoneAward.MilestoneID,
				"completed_days": status.MilestoneAward.CompletedDays,
				"bonus_points":   status.MilestoneAward.BonusPoints,
			}
		}
		event := wsutil.NewEvent("daily_checkin_updated", payload)
		for _, memberID := range memberIDs {
			h.hub.SendToUser(memberID, event)
		}
	}

	httputil.JSON(w, http.StatusOK, toDailyCheckInStatusResponse(status))
}

func (h *DailyCheckInHandler) writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dailycheckin.ErrRelationshipSpaceNotFound):
		httputil.Error(w, http.StatusNotFound, "not_found", "relationship space not found")
	case errors.Is(err, dailycheckin.ErrInvalidTimezone):
		httputil.Error(w, http.StatusUnprocessableEntity, "invalid_timezone", "relationship space timezone is invalid")
	default:
		httputil.Error(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func toDailyCheckInStatusResponse(status dailycheckin.TodayStatus) dailyCheckInStatusResponse {
	resp := dailyCheckInStatusResponse{
		RelationshipSpaceID:          status.RelationshipSpaceID,
		Timezone:                     status.Timezone,
		CheckInDate:                  status.CheckInDate,
		MyCheckedIn:                  status.MyCheckedIn,
		PartnerCheckedIn:             status.PartnerCheckedIn,
		AllMembersCheckedIn:          status.AllMembersCheckedIn,
		ActiveMemberCount:            status.ActiveMemberCount,
		SubmittedMemberCount:         status.SubmittedMemberCount,
		CompletedDaysCount:           status.CompletedDaysCount,
		CurrentStreak:                status.CurrentStreak,
		DailyRewardAwarded:           status.DailyRewardAwarded,
		DailyRewardPoints:            status.DailyRewardPoints,
		TotalCheckInBondPointsEarned: status.TotalCheckInBondPointsEarned,
	}
	if status.MilestoneAward != nil {
		resp.MilestoneAward = &dailyCheckInMilestoneAwardResponse{
			MilestoneID:   status.MilestoneAward.MilestoneID,
			CompletedDays: status.MilestoneAward.CompletedDays,
			BonusPoints:   status.MilestoneAward.BonusPoints,
			Title:         status.MilestoneAward.Title,
			Description:   status.MilestoneAward.Description,
		}
	}
	return resp
}
