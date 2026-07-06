package dailycheckin

import "errors"

const DailyBondPoints = 5

var (
	ErrRelationshipSpaceNotFound = errors.New("relationship space not found")
	ErrInvalidTimezone           = errors.New("invalid timezone")
)

type MilestoneAward struct {
	MilestoneID   string
	CompletedDays int
	BonusPoints   int
	Title         *string
	Description   *string
}

type TodayStatus struct {
	RelationshipSpaceID          string
	Timezone                     string
	CheckInDate                  string
	MyCheckedIn                  bool
	PartnerCheckedIn             bool
	AllMembersCheckedIn          bool
	ActiveMemberCount            int
	SubmittedMemberCount         int
	CompletedDaysCount           int
	CurrentStreak                int
	DailyRewardAwarded           bool
	DailyRewardPoints            int
	MilestoneAward               *MilestoneAward
	TotalCheckInBondPointsEarned int
}
