package user

type Profile struct {
	ID          string
	DisplayName string
	Email       string
	AvatarURL   *string
	Identifier  string
}

type ProfileResponse struct {
	UserID      string  `json:"user_id"`
	DeviceID    string  `json:"device_id"`
	DisplayName string  `json:"display_name"`
	Email       string  `json:"email"`
	AvatarURL   *string `json:"avatar_url"`
	Identifier  string  `json:"identifier"`
}
