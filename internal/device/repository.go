package device

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
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

func (r *Repository) RegisterDevice(ctx context.Context, userID string, req RegisterRequest) (string, error) {
	var deviceID string
	err := r.db.QueryRow(ctx, `
		INSERT INTO devices (user_id, device_name, platform, registration_id, identity_key_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, req.DeviceName, req.Platform, req.RegistrationID, req.IdentityKeyPublic).Scan(&deviceID)
	return deviceID, err
}

func (r *Repository) GetActiveDeviceByName(ctx context.Context, userID, deviceName string) (string, error) {
	var deviceID string
	err := r.db.QueryRow(ctx, `
		SELECT id FROM devices
		WHERE user_id = $1 AND device_name = $2 AND is_active = TRUE AND revoked_at IS NULL
		LIMIT 1
	`, userID, deviceName).Scan(&deviceID)
	if err != nil {
		return "", err
	}
	return deviceID, nil
}

func (r *Repository) EnsureActiveDevice(ctx context.Context, userID, deviceName, platform string, registrationID int, identityKeyPublic string) (string, error) {
	deviceID, err := r.GetActiveDeviceByName(ctx, userID, deviceName)
	if err == nil {
		return deviceID, nil
	}
	if err != pgx.ErrNoRows {
		return "", err
	}

	err = r.db.QueryRow(ctx, `
		INSERT INTO devices (user_id, device_name, platform, registration_id, identity_key_public)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, deviceName, platform, registrationID, identityKeyPublic).Scan(&deviceID)
	if err != nil {
		return "", err
	}
	return deviceID, nil
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

func (r *Repository) UpsertKyberPrekey(ctx context.Context, deviceID string, req KyberPrekeyRequest) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err = tx.Exec(ctx, `
		UPDATE kyber_prekeys
		SET is_active = FALSE, revoked_at = now()
		WHERE device_id = $1 AND is_active = TRUE AND revoked_at IS NULL
	`, deviceID); err != nil {
		return err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO kyber_prekeys (device_id, key_id, public_key, signature, is_active)
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
	for _, prekey := range prekeys {
		tag, err := r.db.Exec(ctx, `
			INSERT INTO one_time_prekeys (device_id, key_id, public_key)
			VALUES ($1, $2, $3)
			ON CONFLICT (device_id, key_id) DO NOTHING
		`, deviceID, prekey.KeyID, prekey.PublicKey)
		if err != nil {
			return inserted, err
		}
		inserted += tag.RowsAffected()
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
	       sp.key_id, sp.public_key, sp.signature,
	       kp.key_id, kp.public_key, kp.signature
		FROM devices d
		JOIN signed_prekeys sp ON sp.device_id = d.id AND sp.is_active = TRUE AND sp.revoked_at IS NULL
		JOIN kyber_prekeys kp ON kp.device_id = d.id AND kp.is_active = TRUE AND kp.revoked_at IS NULL
		WHERE d.user_id = $1 AND d.is_active = TRUE AND d.revoked_at IS NULL
	`, targetUserID)
	if err != nil {
		return PrekeyBundleResponse{}, err
	}
	defer rows.Close()

	response := PrekeyBundleResponse{UserID: targetUserID, Devices: []DevicePrekeyBundle{}}
	for rows.Next() {
		var device DevicePrekeyBundle
		if err = rows.Scan(
			&device.DeviceID,
			&device.RegistrationID,
			&device.IdentityKeyPublic,
			&device.SignedPrekey.KeyID,
			&device.SignedPrekey.PublicKey,
			&device.SignedPrekey.Signature,
			&device.KyberPrekey.KeyID,
			&device.KyberPrekey.PublicKey,
			&device.KyberPrekey.Signature,
		); err != nil {
			return PrekeyBundleResponse{}, err
		}
		response.Devices = append(response.Devices, device)
	}
	if err = rows.Err(); err != nil {
		return PrekeyBundleResponse{}, err
	}
	rows.Close()

	for i := range response.Devices {
		var keyID int
		var publicKey string
		var prekeyID string
		err = tx.QueryRow(ctx, `
			WITH picked AS (
				SELECT id, key_id, public_key
				FROM one_time_prekeys
				WHERE device_id = $1 AND consumed_at IS NULL
				ORDER BY created_at ASC
				LIMIT 1
				FOR UPDATE SKIP LOCKED
			)
			UPDATE one_time_prekeys otp
			SET consumed_at = now()
			FROM picked p
			WHERE otp.id = p.id
			RETURNING p.id, p.key_id, p.public_key
		`, response.Devices[i].DeviceID).Scan(&prekeyID, &keyID, &publicKey)
		if err == nil {
			response.Devices[i].OneTimePrekey = &OneTimePrekey{KeyID: keyID, PublicKey: publicKey}
		} else if err != pgx.ErrNoRows {
			return PrekeyBundleResponse{}, err
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return PrekeyBundleResponse{}, err
	}
	return response, nil
}
