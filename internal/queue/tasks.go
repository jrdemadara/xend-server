package queue

const (
	TaskTypeSendVerificationEmail       = "email:send_verification"
	TaskTypeRotateIdentifiers           = "user:rotate_identifiers"
	TaskTypeSendRelationshipInviteEmail = "email:send_relationship_invite"
)

type SendVerificationEmailPayload struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type SendRelationshipInviteEmailPayload struct {
	Email              string `json:"email"`
	InviterDisplayName string `json:"inviter_display_name"`
	InviterIdentifier  string `json:"inviter_identifier"`
	Note               string `json:"note,omitempty"`
}
