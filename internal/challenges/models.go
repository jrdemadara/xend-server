package challenges

import (
	"errors"
	"time"
)

var (
	ErrRelationshipSpaceNotFound = errors.New("relationship space not found")
	ErrTemplateNotFound          = errors.New("challenge template not found")
	ErrPartnerNotFound           = errors.New("challenge partner not found")
	ErrChallengeNotFound         = errors.New("challenge not found")
	ErrChallengeUnavailable      = errors.New("challenge is not available")
	ErrChallengeNotAllowed       = errors.New("challenge action is not allowed")
	ErrTextResponseRequired      = errors.New("challenge text response is required")
	ErrImageRequired             = errors.New("challenge image is required")
	ErrUnsupportedSubmissionType = errors.New("challenge submission type is not supported")
)

type SubmissionType string

const (
	SubmissionTypeNone  SubmissionType = "none"
	SubmissionTypeText  SubmissionType = "text"
	SubmissionTypeImage SubmissionType = "image"
)

type Status string

const (
	StatusSent      Status = "sent"
	StatusAccepted  Status = "accepted"
	StatusCompleted Status = "completed"
	StatusDeclined  Status = "declined"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)

type Template struct {
	ID             string
	Slug           string
	Title          string
	Description    string
	Category       string
	IconKey        string
	SubmissionType SubmissionType
	MinLevel       int16
	MaxLevel       *int16
	DefaultPoints  int
	ExpiryHours    *int
	DisplayOrder   int16
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Challenge struct {
	ChallengeID         string
	RelationshipSpaceID string
	TemplateID          string
	Title               string
	Description         string
	Category            string
	IconKey             string
	SubmissionType      SubmissionType
	SenderUserID        string
	SenderDisplayName   string
	ReceiverUserID      string
	ReceiverDisplayName string
	AssignedLevel       int16
	RewardPoints        int
	Note                *string
	Status              Status
	ExpiresAt           *time.Time
	AcceptedAt          *time.Time
	CompletedAt         *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	SubmittedByMe       bool
	CanAccept           bool
	CanDecline          bool
	CanComplete         bool
}

type Overview struct {
	RelationshipSpaceID string
	Incoming            []Challenge
	Sent                []Challenge
	History             []Challenge
}

type Submission struct {
	TextResponse *string
	ImagePath    *string
}
