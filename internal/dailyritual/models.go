package dailyritual

import (
	"errors"
	"time"
)

var (
	ErrRelationshipSpaceNotFound = errors.New("relationship space not found")
	ErrInvalidTimezone           = errors.New("invalid timezone")
	ErrAssignmentNotFound        = errors.New("daily ritual assignment not found")
	ErrAssignmentUnavailable     = errors.New("daily ritual assignment is not available")
	ErrSubmissionNotAllowed      = errors.New("daily ritual submission is not allowed")
	ErrTextResponseRequired      = errors.New("daily ritual text response is required")
	ErrImageRequired             = errors.New("daily ritual image is required")
	ErrUnsupportedSubmissionType = errors.New("daily ritual submission type is not supported")
)

type SubmissionType string

const (
	SubmissionTypeNone  SubmissionType = "none"
	SubmissionTypeText  SubmissionType = "text"
	SubmissionTypeImage SubmissionType = "image"
)

type TargetType string

const (
	TargetTypeBoth       TargetType = "both"
	TargetTypeOnePartner TargetType = "one_partner"
)

type CompletionRule string

const (
	CompletionRuleSingleActor  CompletionRule = "single_actor"
	CompletionRuleBothPartners CompletionRule = "both_partners"
)

type AssignmentStatus string

const (
	AssignmentStatusAssigned  AssignmentStatus = "assigned"
	AssignmentStatusCompleted AssignmentStatus = "completed"
	AssignmentStatusExpired   AssignmentStatus = "expired"
)

type Template struct {
	ID             string
	Slug           string
	Title          string
	Description    string
	Category       string
	IconKey        string
	SuggestedTime  *string
	DefaultPoints  int
	SubmissionType SubmissionType
	TargetType     TargetType
	CompletionRule CompletionRule
	MinLevel       int16
	MaxLevel       *int16
	DisplayOrder   int16
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AssignedRitual struct {
	AssignmentID   string
	RitualDate     string
	Title          string
	Description    string
	Category       string
	IconKey        string
	SuggestedTime  *string
	RewardPoints   int
	SubmissionType SubmissionType
	TargetType     TargetType
	CompletionRule CompletionRule
	Status         AssignmentStatus
	Completed      bool
	TargetUserID   *string
	SubmittedByMe  bool
	SubmittedCount int
	RequiredCount  int
	CanSubmit      bool
}

type Overview struct {
	RelationshipSpaceID string
	RitualDate          string
	TodayRitual         *AssignedRitual
	History             []AssignedRitual
}

type Submission struct {
	TextResponse *string
	ImagePath    *string
}
