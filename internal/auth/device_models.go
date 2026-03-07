package auth

type DeviceRegisterRequest struct {
	DeviceName        string `json:"device_name"`
	Platform          string `json:"platform"`
	RegistrationID    int    `json:"registration_id"`
	IdentityKeyPublic string `json:"identity_key_public"`
}

type SignedPrekeyRequest struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type KyberPrekeyRequest struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type OneTimePrekey struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
}

type OneTimePrekeyBatchRequest struct {
	Prekeys []OneTimePrekey `json:"prekeys"`
}

type PushTokenRequest struct {
	Provider string `json:"provider"`
	Token    string `json:"token"`
}

type DevicePrekeyBundle struct {
	DeviceID          string         `json:"device_id"`
	RegistrationID    int            `json:"registration_id"`
	IdentityKeyPublic string         `json:"identity_key_public"`
	SignedPrekey      SignedPrekey   `json:"signed_prekey"`
	KyberPrekey       KyberPrekey    `json:"kyber_prekey"`
	OneTimePrekey     *OneTimePrekey `json:"one_time_prekey,omitempty"`
}

type SignedPrekey struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type KyberPrekey struct {
	KeyID     int    `json:"key_id"`
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"`
}

type PrekeyBundleResponse struct {
	UserID  string               `json:"user_id"`
	Devices []DevicePrekeyBundle `json:"devices"`
}
