package dailyritual

import (
	"errors"
	"time"
)

const MaxActiveSelections = 3

var (
	ErrRelationshipSpaceNotFound = errors.New("relationship space not found")
	ErrInvalidTemplateID         = errors.New("invalid template id")
	ErrTemplateNotFound          = errors.New("template not found")
	ErrSelectionLimitExceeded    = errors.New("selection limit exceeded")
)

type Template struct {
	ID            string
	Slug          string
	Title         string
	Description   string
	Category      string
	IconKey       string
	SuggestedTime *string
	DefaultPoints int
	DisplayOrder  int16
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SpaceSelection struct {
	SelectionID           string
	RelationshipSpaceID   string
	TemplateID            string
	SelectedByUserID      string
	SortOrder             int16
	SelectedAt            time.Time
	TemplateSlug          string
	TemplateTitle         string
	TemplateDescription   string
	TemplateCategory      string
	TemplateIconKey       string
	TemplateSuggestedTime *string
	TemplateDefaultPoints int
}
