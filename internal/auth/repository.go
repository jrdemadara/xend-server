package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type userRecord struct {
	ID              string
	Email           string
	PasswordHash    string
	EmailVerifiedAt *time.Time
}

type userBasic struct {
	ID              string
	EmailVerifiedAt *time.Time
}

type deviceRecord struct {
	ID string
}

type userProfileRecord struct {
	ID          string
	DisplayName string
	Email       string
	AvatarURL   *string
	Identifier  string
}

func (r *Repository) CreateUserWithDevice(ctx context.Context, req RegisterRequest, passwordHash, identifier string) (userID, deviceID string, err error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err = tx.QueryRow(ctx, `
		INSERT INTO users (
			display_name,
			email,
			identifier,
			auto_rotate_identifier,
			identifier_rotation_days,
			identifier_rotates_at
		)
		VALUES ($1, $2, $3, TRUE, 1, now() + interval '1 minute')
		RETURNING id
	`, req.DisplayName, req.Email, identifier).Scan(&userID); err != nil {
		return "", "", err
	}

	if _, err = tx.Exec(ctx, `INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, $2)`, userID, passwordHash); err != nil {
		return "", "", err
	}

	if err = tx.QueryRow(ctx, `
		INSERT INTO devices (user_id, device_name, platform, registration_id, identity_key_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, req.DeviceName, req.Platform, req.RegistrationID, req.IdentityKeyPublic).Scan(&deviceID); err != nil {
		return "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", err
	}
	return userID, deviceID, nil
}

func (r *Repository) MarkEmailVerifiedByEmail(ctx context.Context, email string) (bool, error) {
	res, err := r.db.Exec(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, now())
		WHERE email = $1 AND deleted_at IS NULL
	`, email)
	if err != nil {
		return false, err
	}
	return res.RowsAffected() > 0, nil
}

func (r *Repository) GetUserForLogin(ctx context.Context, email string) (userRecord, error) {
	var u userRecord
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.email, uc.password_hash, u.email_verified_at
		FROM users u
		JOIN user_credentials uc ON uc.user_id = u.id
		WHERE u.email = $1 AND u.deleted_at IS NULL
	`, email).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.EmailVerifiedAt)
	if err != nil {
		return userRecord{}, err
	}
	return u, nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (userBasic, error) {
	var u userBasic
	err := r.db.QueryRow(ctx, `SELECT id, email_verified_at FROM users WHERE email = $1 AND deleted_at IS NULL`, email).Scan(&u.ID, &u.EmailVerifiedAt)
	if err != nil {
		return userBasic{}, err
	}
	return u, nil
}

func (r *Repository) GetUserByOAuth(ctx context.Context, provider, providerUserID string) (userBasic, error) {
	var u userBasic
	err := r.db.QueryRow(ctx, `
		SELECT u.id, u.email_verified_at
		FROM user_oauth_accounts oa
		JOIN users u ON u.id = oa.user_id
		WHERE oa.provider = $1 AND oa.provider_user_id = $2
		LIMIT 1
	`, provider, providerUserID).Scan(&u.ID, &u.EmailVerifiedAt)
	if err != nil {
		return userBasic{}, err
	}
	return u, nil
}

func (r *Repository) CreateUserForOAuth(ctx context.Context, displayName, email, identifier string) (string, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO users (
			display_name,
			email,
			identifier,
			email_verified_at,
			auto_rotate_identifier,
			identifier_rotation_days,
			identifier_rotates_at
		)
		VALUES ($1, $2, $3, now(), TRUE, 1, now() + interval '1 minute')
		RETURNING id
	`, displayName, email, identifier).Scan(&userID)
	return userID, err
}

func (r *Repository) LinkOAuthAccount(ctx context.Context, userID, provider, providerUserID, email string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_oauth_accounts (user_id, provider, provider_user_id, email)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, provider_user_id)
		DO UPDATE SET user_id = EXCLUDED.user_id, email = EXCLUDED.email, updated_at = now()
	`, userID, provider, providerUserID, email)
	return err
}

func (r *Repository) EnsureActiveDevice(ctx context.Context, userID, deviceName, platform string, registrationID int, identityKeyPublic string) (deviceRecord, error) {
	d, err := r.GetActiveDeviceByName(ctx, userID, deviceName)
	if err == nil {
		return d, nil
	}
	if err != pgx.ErrNoRows {
		return deviceRecord{}, err
	}

	var created deviceRecord
	err = r.db.QueryRow(ctx, `
		INSERT INTO devices (user_id, device_name, platform, registration_id, identity_key_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, deviceName, platform, registrationID, identityKeyPublic).Scan(&created.ID)
	if err != nil {
		return deviceRecord{}, err
	}
	return created, nil
}

func (r *Repository) GetActiveDeviceByName(ctx context.Context, userID, deviceName string) (deviceRecord, error) {
	var d deviceRecord
	err := r.db.QueryRow(ctx, `
		SELECT id FROM devices
		WHERE user_id = $1 AND device_name = $2 AND is_active = TRUE AND revoked_at IS NULL
		LIMIT 1
	`, userID, deviceName).Scan(&d.ID)
	if err != nil {
		return deviceRecord{}, err
	}
	return d, nil
}

func (r *Repository) InsertRefreshSession(ctx context.Context, s Session) error {
	_, err := r.db.Exec(ctx, `INSERT INTO refresh_sessions (id, user_id, device_id, refresh_token_hash, expires_at) VALUES ($1, $2, $3, $4, $5)`, s.ID, s.UserID, s.DeviceID, s.RefreshTokenHash, s.ExpiresAt)
	return err
}

func (r *Repository) RotateRefreshSession(ctx context.Context, sessionID, newSessionID, tokenHash string, expiresAt time.Time) (Session, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Session{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existing Session
	err = tx.QueryRow(ctx, `
		SELECT id, user_id, device_id FROM refresh_sessions
		WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()
		FOR UPDATE
	`, sessionID).Scan(&existing.ID, &existing.UserID, &existing.DeviceID)
	if err != nil {
		return Session{}, err
	}

	if newSessionID == "" {
		newSessionID = uuid.NewString()
	}
	newSession := Session{ID: newSessionID, UserID: existing.UserID, DeviceID: existing.DeviceID, RefreshTokenHash: tokenHash, ExpiresAt: expiresAt}

	if _, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = now(), replaced_by_session_id = $2 WHERE id = $1`, sessionID, newSession.ID); err != nil {
		return Session{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO refresh_sessions (id, user_id, device_id, refresh_token_hash, expires_at) VALUES ($1, $2, $3, $4, $5)`, newSession.ID, newSession.UserID, newSession.DeviceID, newSession.RefreshTokenHash, newSession.ExpiresAt); err != nil {
		return Session{}, err
	}

	if err = tx.Commit(ctx); err != nil {
		return Session{}, err
	}
	return newSession, nil
}

func (r *Repository) GetUserProfileByID(ctx context.Context, userID string) (userProfileRecord, error) {
	var u userProfileRecord
	err := r.db.QueryRow(ctx, `
		SELECT id, display_name, email::text, avatar_url, identifier
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&u.ID, &u.DisplayName, &u.Email, &u.AvatarURL, &u.Identifier)
	if err != nil {
		return userProfileRecord{}, err
	}
	return u, nil
}

func (r *Repository) RevokeRefreshSessionsByUserDevice(ctx context.Context, userID, deviceID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1
		  AND device_id = $2
		  AND revoked_at IS NULL
	`, userID, deviceID)
	return err
}

func (r *Repository) RevokeAllRefreshSessionsByUser(ctx context.Context, userID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE refresh_sessions
		SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, userID)
	return err
}

func (r *Repository) DeviceBelongsToUser(ctx context.Context, deviceID, userID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM devices
			WHERE id = $1 AND user_id = $2 AND is_active = TRUE AND revoked_at IS NULL
		)
	`, deviceID, userID).Scan(&exists)
	return exists, err
}

func (r *Repository) RegisterDevice(ctx context.Context, userID string, req DeviceRegisterRequest) (string, error) {
	var deviceID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO devices (user_id, device_name, platform, registration_id, identity_key_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, req.DeviceName, req.Platform, req.RegistrationID, req.IdentityKeyPublic).Scan(&deviceID)
	return deviceID, err
}

func (r *Repository) UpsertSignedPrekey(ctx context.Context, deviceID string, req SignedPrekeyRequest) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		UPDATE signed_prekeys
		SET is_active = FALSE, revoked_at = now()
		WHERE device_id = $1 AND is_active = TRUE AND revoked_at IS NULL
	`, deviceID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO signed_prekeys (device_id, key_id, public_key, signature, is_active)
		VALUES ($1, $2, $3, $4, TRUE)
		ON CONFLICT (device_id, key_id)
		DO UPDATE SET public_key = EXCLUDED.public_key, signature = EXCLUDED.signature, is_active = TRUE, revoked_at = NULL
	`, deviceID, req.KeyID, req.PublicKey, req.Signature); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) InsertOneTimePrekeys(ctx context.Context, deviceID string, prekeys []OneTimePrekey) (int64, error) {
	var inserted int64
	for _, k := range prekeys {
		ct, err := r.db.Exec(ctx, `
			INSERT INTO one_time_prekeys (device_id, key_id, public_key)
			VALUES ($1, $2, $3)
			ON CONFLICT (device_id, key_id) DO NOTHING
		`, deviceID, k.KeyID, k.PublicKey)
		if err != nil {
			return inserted, err
		}
		inserted += ct.RowsAffected()
	}
	return inserted, nil
}

func (r *Repository) UpsertPushToken(ctx context.Context, deviceID, provider, token string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO push_tokens (device_id, provider, token, last_validated_at, revoked_at)
		VALUES ($1, $2, $3, now(), NULL)
		ON CONFLICT (device_id, token)
		DO UPDATE SET provider = EXCLUDED.provider, last_validated_at = now(), revoked_at = NULL
	`, deviceID, provider, token)
	return err
}

func (r *Repository) ListActivePushTokensByUser(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT pt.token
		FROM push_tokens pt
		JOIN devices d ON d.id = pt.device_id
		WHERE d.user_id = $1
		  AND d.is_active = TRUE
		  AND d.revoked_at IS NULL
		  AND pt.revoked_at IS NULL
		  AND pt.token <> ''
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]string, 0, 4)
	for rows.Next() {
		var token string
		if err := rows.Scan(&token); err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tokens, nil
}

func (r *Repository) GetPrekeyBundle(ctx context.Context, targetUserID string) (PrekeyBundleResponse, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return PrekeyBundleResponse{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT d.id, d.registration_id, d.identity_key_public,
	       sp.key_id, sp.public_key, sp.signature
		FROM devices d
		JOIN signed_prekeys sp ON sp.device_id = d.id AND sp.is_active = TRUE AND sp.revoked_at IS NULL
		WHERE d.user_id = $1 AND d.is_active = TRUE AND d.revoked_at IS NULL
	`, targetUserID)
	if err != nil {
		return PrekeyBundleResponse{}, err
	}
	defer rows.Close()

	resp := PrekeyBundleResponse{UserID: targetUserID, Devices: []DevicePrekeyBundle{}}
	for rows.Next() {
		var d DevicePrekeyBundle
		if err = rows.Scan(&d.DeviceID, &d.RegistrationID, &d.IdentityKeyPublic, &d.SignedPrekey.KeyID, &d.SignedPrekey.PublicKey, &d.SignedPrekey.Signature); err != nil {
			return PrekeyBundleResponse{}, err
		}
		resp.Devices = append(resp.Devices, d)
	}
	if err = rows.Err(); err != nil {
		return PrekeyBundleResponse{}, err
	}
	rows.Close()

	for i := range resp.Devices {
		var otpID string
		var otKeyID int
		var otPub string
		err = tx.QueryRow(ctx, `
			WITH picked AS (
				SELECT id, key_id, public_key
				FROM one_time_prekeys
				WHERE device_id = $1 AND consumed_at IS NULL
				ORDER BY created_at ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE one_time_prekeys o
			SET consumed_at = now()
			FROM picked p
			WHERE o.id = p.id
			RETURNING p.id, p.key_id, p.public_key
		`, resp.Devices[i].DeviceID).Scan(&otpID, &otKeyID, &otPub)
		if err == nil {
			resp.Devices[i].OneTimePrekey = &OneTimePrekey{KeyID: otKeyID, PublicKey: otPub}
		} else if err != pgx.ErrNoRows {
			return PrekeyBundleResponse{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return PrekeyBundleResponse{}, err
	}
	return resp, nil
}
