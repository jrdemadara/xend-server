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
		VALUES ($1, $2, $3, TRUE, 30, now() + interval '30 days')
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
		VALUES ($1, $2, $3, now(), TRUE, 30, now() + interval '30 days')
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
