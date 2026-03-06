package auth

import "time"

type RegisterRequest struct {
	DisplayName       string `json:"display_name"`
	Email             string `json:"email"`
	Password          string `json:"password"`
	DeviceName        string `json:"device_name"`
	Platform          string `json:"platform"`
	RegistrationID    int    `json:"registration_id"`
	IdentityKeyPublic string `json:"identity_key_public"`
}

type RegisterResponse struct {
	UserID               string `json:"user_id"`
	Email                string `json:"email"`
	RequiresVerification bool   `json:"requires_verification"`
}

type LoginRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	DeviceName string `json:"device_name"`
}

type GoogleAuthRequest struct {
	IDToken           string `json:"id_token"`
	DisplayName       string `json:"display_name"`
	DeviceName        string `json:"device_name"`
	Platform          string `json:"platform"`
	RegistrationID    int    `json:"registration_id"`
	IdentityKeyPublic string `json:"identity_key_public"`
}

type VerifyEmailRequest struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type ResendVerificationRequest struct {
	Email string `json:"email"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	UserID       string `json:"user_id"`
	DeviceID     string `json:"device_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type TokenRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

type UserProfileResponse struct {
	UserID      string  `json:"user_id"`
	DeviceID    string  `json:"device_id"`
	DisplayName string  `json:"display_name"`
	Email       string  `json:"email"`
	AvatarURL   *string `json:"avatar_url"`
	Identifier  string  `json:"identifier"`
}

type Session struct {
	ID               string
	UserID           string
	DeviceID         string
	RefreshTokenHash string
	ExpiresAt        time.Time
}
