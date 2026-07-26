package relationship

import (
	"errors"
	"time"
)

type Invite struct {
	InviteID            string
	RelationshipSpaceID *string
	InviterUserID       string
	InviterDisplayName  string
	InviterIdentifier   string
	InviterAvatarURL    *string
	Note                *string
	Status              string
	CreatedAt           time.Time
}

type SpaceSummary struct {
	RelationshipSpaceID string
	ConversationID      string
	Name                *string
	CreatedByUserID     string
	CurrentLevel        int16
	CurrentLevelName    string
	CoverPhotoPath      *string
	CouplePhotoPath     *string
	IsDefault           bool
	AccessHint          *string
	AccessConfigured    bool
	ArchivedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type InviteOutbox struct {
	InviteID          string
	InviteeIdentifier string
	Status            string
	Note              *string
	CreatedAt         time.Time
}

type Level struct {
	Level       int16
	Name        string
	Description *string
}

type LevelProgress struct {
	RelationshipSpaceID string
	Level               int16
	RequiredPoints      int32
	CurrentPoints       int32
	UnlockedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type SpaceMemberSummary struct {
	UserID      string
	DisplayName string
	Identifier  string
}

type SpaceMood struct {
	RelationshipSpaceID string
	UserID              string
	DisplayName         string
	MoodKey             *string
	Emoji               *string
	Label               *string
	UpdatedAt           *time.Time
	IsMe                bool
}

var (
	ErrInvalidInput        = errors.New("invalid input")
	ErrInviteNotFound      = errors.New("invite not found")
	ErrSpaceNotFound       = errors.New("relationship space not found")
	ErrSpaceAccessNotFound = errors.New("relationship space access not found")
	ErrImageRequired       = errors.New("image is required")
	ErrSpaceImageNotFound  = errors.New("relationship space image not found")
)
