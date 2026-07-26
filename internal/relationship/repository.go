package relationship

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"xend.chat/m/internal/auth"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateInviteByIdentifier(ctx context.Context, inviterUserID, inviteeIdentifier string, note *string) (string, string, string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inviteeUserID string
	var inviteeEmail string
	err = tx.QueryRow(ctx, `
		SELECT id, email::text
		FROM users
		WHERE lower(identifier) = lower($1) AND deleted_at IS NULL
	`, inviteeIdentifier).Scan(&inviteeUserID, &inviteeEmail)
	if err != nil {
		return "", "", "", err
	}
	if inviteeUserID == inviterUserID {
		return "", "", "", ErrInvalidInput
	}

	var existingInviteID string
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM relationship_invites
		WHERE inviter_user_id = $1
		  AND invitee_user_id = $2
		  AND status = 'pending'
		LIMIT 1
	`, inviterUserID, inviteeUserID).Scan(&existingInviteID)
	if err == nil {
		if err = tx.Commit(ctx); err != nil {
			return "", "", "", err
		}
		return existingInviteID, inviteeUserID, inviteeEmail, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", err
	}

	inviteID := uuid.NewString()
	if _, err = tx.Exec(ctx, `
		INSERT INTO relationship_invites (id, inviter_user_id, invitee_user_id, status, note)
		VALUES ($1, $2, $3, 'pending', $4)
	`, inviteID, inviterUserID, inviteeUserID, note); err != nil {
		return "", "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", "", err
	}
	return inviteID, inviteeUserID, inviteeEmail, nil
}

func (r *Repository) ListInviteInbox(ctx context.Context, inviteeUserID string) ([]Invite, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ri.id,
		       ri.relationship_space_id,
		       ri.inviter_user_id,
		       u.display_name,
		       u.identifier,
		       u.avatar_url,
		       ri.note,
		       ri.status,
		       ri.created_at
		FROM relationship_invites ri
		JOIN users u ON u.id = ri.inviter_user_id
		WHERE ri.invitee_user_id = $1
		  AND ri.status = 'pending'
		ORDER BY ri.created_at DESC
	`, inviteeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Invite, 0)
	for rows.Next() {
		var item Invite
		if err := rows.Scan(
			&item.InviteID,
			&item.RelationshipSpaceID,
			&item.InviterUserID,
			&item.InviterDisplayName,
			&item.InviterIdentifier,
			&item.InviterAvatarURL,
			&item.Note,
			&item.Status,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListInviteOutbox(ctx context.Context, inviterUserID string) ([]InviteOutbox, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ri.id, u.identifier, ri.status, ri.note, ri.created_at
		FROM relationship_invites ri
		JOIN users u ON u.id = ri.invitee_user_id
		WHERE ri.inviter_user_id = $1
		ORDER BY ri.created_at DESC
	`, inviterUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]InviteOutbox, 0)
	for rows.Next() {
		var item InviteOutbox
		if err := rows.Scan(&item.InviteID, &item.InviteeIdentifier, &item.Status, &item.Note, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) AcceptInvite(ctx context.Context, inviteID, inviteeUserID string) (string, string, string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return "", "", "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var inviterUserID string
	var existingSpaceID *string
	err = tx.QueryRow(ctx, `
		SELECT relationship_space_id, inviter_user_id
		FROM relationship_invites
		WHERE id = $1
		  AND invitee_user_id = $2
		  AND status = 'pending'
		  AND (expires_at IS NULL OR expires_at > now())
		FOR UPDATE
	`, inviteID, inviteeUserID).Scan(&existingSpaceID, &inviterUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", "", ErrInviteNotFound
		}
		return "", "", "", err
	}

	spaceID := uuid.NewString()
	if existingSpaceID != nil && *existingSpaceID != "" {
		spaceID = *existingSpaceID
	} else {
		if _, err = tx.Exec(ctx, `
			INSERT INTO relationship_spaces (id, created_by_user_id)
			VALUES ($1, $2)
		`, spaceID, inviterUserID); err != nil {
			return "", "", "", err
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO relationship_space_members (relationship_space_id, user_id, joined_by_user_id, role, membership_status)
			VALUES ($1, $2, $2, 'owner', 'active')
			ON CONFLICT (relationship_space_id, user_id)
			DO UPDATE SET membership_status = 'active', left_at = NULL, joined_at = now()
		`, spaceID, inviterUserID); err != nil {
			return "", "", "", err
		}

		if _, err = tx.Exec(ctx, `
			INSERT INTO user_space_preferences (user_id, default_relationship_space_id)
			VALUES ($1, $2)
			ON CONFLICT (user_id) DO NOTHING
		`, inviterUserID, spaceID); err != nil {
			return "", "", "", err
		}
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO relationship_space_members (relationship_space_id, user_id, joined_by_user_id, role, membership_status)
		VALUES ($1, $2, $3, 'member', 'active')
		ON CONFLICT (relationship_space_id, user_id)
		DO UPDATE SET membership_status = 'active', left_at = NULL, joined_at = now()
	`, spaceID, inviteeUserID, inviterUserID); err != nil {
		return "", "", "", err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO user_space_preferences (user_id, default_relationship_space_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO NOTHING
	`, inviteeUserID, spaceID); err != nil {
		return "", "", "", err
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO relationship_space_level_progress (
			relationship_space_id, level, required_points, current_points, unlocked_at
		)
		SELECT $1, rl.level, rl.required_points, 0, now()
		FROM relationship_levels rl
		WHERE rl.level = 1
		ON CONFLICT (relationship_space_id, level) DO NOTHING
	`, spaceID); err != nil {
		return "", "", "", err
	}

	if _, err = tx.Exec(ctx, `
		UPDATE relationship_invites
		SET status = 'accepted', responded_at = now(), relationship_space_id = $2
		WHERE id = $1
	`, inviteID, spaceID); err != nil {
		return "", "", "", err
	}

	var conversationID string
	err = tx.QueryRow(ctx, `
		INSERT INTO conversations (relationship_space_id, created_by_user_id)
		VALUES ($1, $2)
		ON CONFLICT (relationship_space_id)
		DO UPDATE SET updated_at = now()
		RETURNING id
	`, spaceID, inviterUserID).Scan(&conversationID)
	if err != nil {
		return "", "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", "", err
	}
	return spaceID, conversationID, inviterUserID, nil
}

func (r *Repository) DeclineInvite(ctx context.Context, inviteID, inviteeUserID string) (string, error) {
	var inviterUserID string
	err := r.db.QueryRow(ctx, `
		UPDATE relationship_invites
		SET status = 'declined', responded_at = now()
		WHERE id = $1
		  AND invitee_user_id = $2
		  AND status = 'pending'
		RETURNING inviter_user_id
	`, inviteID, inviteeUserID).Scan(&inviterUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrInviteNotFound
		}
		return "", err
	}
	return inviterUserID, nil
}

func (r *Repository) ListSpacesByUser(ctx context.Context, userID string) ([]SpaceSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rs.id,
		       COALESCE(c.id::text, ''),
		       rs.name,
		       rs.created_by_user_id,
		       rs.current_level,
		       COALESCE(rl.name, 'Tease') AS current_level_name,
		       rs.cover_photo_path,
		       rs.couple_photo_path,
		       rs.relationship_start_date,
		       rs.celebrate_monthsary,
		       rs.celebrate_anniversary,
		       COALESCE(usp.default_relationship_space_id = rs.id, false) AS is_default,
		       rsma.access_hint,
		       COALESCE(rsma.access_passphrase_hash IS NOT NULL AND rsma.access_passphrase_hash <> '', false) AS access_configured,
		       rs.archived_at,
		       rs.created_at,
		       rs.updated_at
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm ON rsm.relationship_space_id = rs.id
		LEFT JOIN conversations c ON c.relationship_space_id = rs.id
		LEFT JOIN user_space_preferences usp ON usp.user_id = rsm.user_id
		LEFT JOIN relationship_space_member_access rsma
		  ON rsma.relationship_space_id = rs.id AND rsma.user_id = rsm.user_id
		LEFT JOIN relationship_levels rl ON rl.level = rs.current_level
		WHERE rsm.user_id = $1
		  AND rsm.membership_status = 'active'
		ORDER BY rs.updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SpaceSummary, 0)
	for rows.Next() {
		var item SpaceSummary
		if err := rows.Scan(
			&item.RelationshipSpaceID,
			&item.ConversationID,
			&item.Name,
			&item.CreatedByUserID,
			&item.CurrentLevel,
			&item.CurrentLevelName,
			&item.CoverPhotoPath,
			&item.CouplePhotoPath,
			&item.RelationshipStartDate,
			&item.CelebrateMonthsary,
			&item.CelebrateAnniversary,
			&item.IsDefault,
			&item.AccessHint,
			&item.AccessConfigured,
			&item.ArchivedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetSpaceByIDForUser(ctx context.Context, userID, spaceID string) (SpaceSummary, error) {
	var item SpaceSummary
	err := r.db.QueryRow(ctx, `
		SELECT rs.id,
		       COALESCE(c.id::text, ''),
		       rs.name,
		       rs.created_by_user_id,
		       rs.current_level,
		       COALESCE(rl.name, 'Tease') AS current_level_name,
		       rs.cover_photo_path,
		       rs.couple_photo_path,
		       rs.relationship_start_date,
		       rs.celebrate_monthsary,
		       rs.celebrate_anniversary,
		       COALESCE(usp.default_relationship_space_id = rs.id, false) AS is_default,
		       rsma.access_hint,
		       COALESCE(rsma.access_passphrase_hash IS NOT NULL AND rsma.access_passphrase_hash <> '', false) AS access_configured,
		       rs.archived_at,
		       rs.created_at,
		       rs.updated_at
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm ON rsm.relationship_space_id = rs.id
		LEFT JOIN conversations c ON c.relationship_space_id = rs.id
		LEFT JOIN user_space_preferences usp ON usp.user_id = rsm.user_id
		LEFT JOIN relationship_space_member_access rsma
		  ON rsma.relationship_space_id = rs.id AND rsma.user_id = rsm.user_id
		LEFT JOIN relationship_levels rl ON rl.level = rs.current_level
		WHERE rs.id = $2
		  AND rsm.user_id = $1
		  AND rsm.membership_status = 'active'
	`, userID, spaceID).Scan(
		&item.RelationshipSpaceID,
		&item.ConversationID,
		&item.Name,
		&item.CreatedByUserID,
		&item.CurrentLevel,
		&item.CurrentLevelName,
		&item.CoverPhotoPath,
		&item.CouplePhotoPath,
		&item.RelationshipStartDate,
		&item.CelebrateMonthsary,
		&item.CelebrateAnniversary,
		&item.IsDefault,
		&item.AccessHint,
		&item.AccessConfigured,
		&item.ArchivedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SpaceSummary{}, ErrSpaceNotFound
		}
		return SpaceSummary{}, err
	}
	return item, nil
}

func (r *Repository) UpdateSpaceSettings(
	ctx context.Context,
	userID string,
	spaceID string,
	name *string,
	relationshipStartDate *time.Time,
	celebrateMonthsary *bool,
	celebrateAnniversary *bool,
) (SpaceSummary, []string, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE relationship_spaces rs
		SET name = $3,
		    relationship_start_date = COALESCE($4::date, relationship_start_date),
		    celebrate_monthsary = COALESCE($5::boolean, celebrate_monthsary),
		    celebrate_anniversary = COALESCE($6::boolean, celebrate_anniversary)
		WHERE rs.id = $2
		  AND EXISTS (
			SELECT 1
			FROM relationship_space_members rsm
			WHERE rsm.relationship_space_id = rs.id
			  AND rsm.user_id = $1
			  AND rsm.membership_status = 'active'
		  )
	`, userID, spaceID, name, relationshipStartDate, celebrateMonthsary, celebrateAnniversary)
	if err != nil {
		return SpaceSummary{}, nil, err
	}
	if tag.RowsAffected() == 0 {
		return SpaceSummary{}, nil, ErrSpaceNotFound
	}

	memberIDs, err := r.listActiveSpaceMemberIDs(ctx, spaceID)
	if err != nil {
		return SpaceSummary{}, nil, err
	}
	item, err := r.GetSpaceByIDForUser(ctx, userID, spaceID)
	if err != nil {
		return SpaceSummary{}, nil, err
	}
	return item, memberIDs, nil
}

func (r *Repository) UpdateSpaceMediaPath(ctx context.Context, userID, spaceID, kind, imagePath string) (SpaceSummary, *string, []string, error) {
	var query string
	switch kind {
	case "cover-photo":
		query = `
			WITH target AS (
				SELECT rs.cover_photo_path AS old_path
				FROM relationship_spaces rs
				WHERE rs.id = $2
				  AND EXISTS (
					SELECT 1
					FROM relationship_space_members rsm
					WHERE rsm.relationship_space_id = rs.id
					  AND rsm.user_id = $1
					  AND rsm.membership_status = 'active'
				  )
				FOR UPDATE
			)
			UPDATE relationship_spaces rs
			SET cover_photo_path = $3
			FROM target
			WHERE rs.id = $2
			RETURNING target.old_path
		`
	case "couple-photo":
		query = `
			WITH target AS (
				SELECT rs.couple_photo_path AS old_path
				FROM relationship_spaces rs
				WHERE rs.id = $2
				  AND EXISTS (
					SELECT 1
					FROM relationship_space_members rsm
					WHERE rsm.relationship_space_id = rs.id
					  AND rsm.user_id = $1
					  AND rsm.membership_status = 'active'
				  )
				FOR UPDATE
			)
			UPDATE relationship_spaces rs
			SET couple_photo_path = $3
			FROM target
			WHERE rs.id = $2
			RETURNING target.old_path
		`
	default:
		return SpaceSummary{}, nil, nil, ErrInvalidInput
	}

	var oldPath *string
	if err := r.db.QueryRow(ctx, query, userID, spaceID, imagePath).Scan(&oldPath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return SpaceSummary{}, nil, nil, ErrSpaceNotFound
		}
		return SpaceSummary{}, nil, nil, err
	}

	memberIDs, err := r.listActiveSpaceMemberIDs(ctx, spaceID)
	if err != nil {
		return SpaceSummary{}, nil, nil, err
	}
	item, err := r.GetSpaceByIDForUser(ctx, userID, spaceID)
	if err != nil {
		return SpaceSummary{}, nil, nil, err
	}
	return item, oldPath, memberIDs, nil
}

func (r *Repository) GetSpaceMediaPath(ctx context.Context, userID, spaceID, kind string) (string, error) {
	var query string
	switch kind {
	case "cover-photo":
		query = `
			SELECT rs.cover_photo_path
			FROM relationship_spaces rs
			JOIN relationship_space_members rsm ON rsm.relationship_space_id = rs.id
			WHERE rs.id = $2
			  AND rsm.user_id = $1
			  AND rsm.membership_status = 'active'
		`
	case "couple-photo":
		query = `
			SELECT rs.couple_photo_path
			FROM relationship_spaces rs
			JOIN relationship_space_members rsm ON rsm.relationship_space_id = rs.id
			WHERE rs.id = $2
			  AND rsm.user_id = $1
			  AND rsm.membership_status = 'active'
		`
	default:
		return "", ErrInvalidInput
	}

	var imagePath *string
	if err := r.db.QueryRow(ctx, query, userID, spaceID).Scan(&imagePath); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrSpaceNotFound
		}
		return "", err
	}
	if imagePath == nil || *imagePath == "" {
		return "", ErrSpaceImageNotFound
	}
	return *imagePath, nil
}

func (r *Repository) SetDefaultSpace(ctx context.Context, userID, spaceID string) error {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO user_space_preferences (user_id, default_relationship_space_id)
		SELECT $1, $2
		WHERE EXISTS (
			SELECT 1
			FROM relationship_space_members
			WHERE relationship_space_id = $2
			  AND user_id = $1
			  AND membership_status = 'active'
		)
		ON CONFLICT (user_id)
		DO UPDATE SET default_relationship_space_id = EXCLUDED.default_relationship_space_id
	`, userID, spaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSpaceNotFound
	}
	return nil
}

func (r *Repository) listActiveSpaceMemberIDs(ctx context.Context, spaceID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 2)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}
	return items, rows.Err()
}

func (r *Repository) UpsertSpaceAccess(ctx context.Context, userID, spaceID, passphrase string, hint *string) error {
	passphraseHash, err := auth.HashPassword(passphrase)
	if err != nil {
		return err
	}

	tag, err := r.db.Exec(ctx, `
		INSERT INTO relationship_space_member_access (
			relationship_space_id, user_id, access_passphrase_hash, access_hint
		)
		SELECT $2, $1, $3, $4
		WHERE EXISTS (
			SELECT 1
			FROM relationship_space_members
			WHERE relationship_space_id = $2
			  AND user_id = $1
			  AND membership_status = 'active'
		)
		ON CONFLICT (relationship_space_id, user_id)
		DO UPDATE SET
			access_passphrase_hash = EXCLUDED.access_passphrase_hash,
			access_hint = EXCLUDED.access_hint
	`, userID, spaceID, passphraseHash, hint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSpaceNotFound
	}
	return nil
}

func (r *Repository) UnlockSpace(ctx context.Context, userID, passphrase string) (SpaceSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rs.id,
		       COALESCE(c.id::text, ''),
		       rs.name,
		       rs.created_by_user_id,
		       rs.current_level,
		       COALESCE(rl.name, 'Tease') AS current_level_name,
		       rs.cover_photo_path,
		       rs.couple_photo_path,
		       rs.relationship_start_date,
		       rs.celebrate_monthsary,
		       rs.celebrate_anniversary,
		       COALESCE(usp.default_relationship_space_id = rs.id, false) AS is_default,
		       rsma.access_hint,
		       COALESCE(rsma.access_passphrase_hash IS NOT NULL AND rsma.access_passphrase_hash <> '', false) AS access_configured,
		       rsma.access_passphrase_hash,
		       rs.archived_at,
		       rs.created_at,
		       rs.updated_at
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm ON rsm.relationship_space_id = rs.id
		JOIN relationship_space_member_access rsma
		  ON rsma.relationship_space_id = rs.id AND rsma.user_id = rsm.user_id
		LEFT JOIN user_space_preferences usp ON usp.user_id = rsm.user_id
		LEFT JOIN conversations c ON c.relationship_space_id = rs.id
		LEFT JOIN relationship_levels rl ON rl.level = rs.current_level
		WHERE rsm.user_id = $1
		  AND rsm.membership_status = 'active'
		  AND COALESCE(usp.default_relationship_space_id = rs.id, false) = false
		  AND rsma.access_passphrase_hash IS NOT NULL
		ORDER BY rs.created_at ASC
	`, userID)
	if err != nil {
		return SpaceSummary{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var item SpaceSummary
		var passphraseHash string
		if err := rows.Scan(
			&item.RelationshipSpaceID,
			&item.ConversationID,
			&item.Name,
			&item.CreatedByUserID,
			&item.CurrentLevel,
			&item.CurrentLevelName,
			&item.CoverPhotoPath,
			&item.CouplePhotoPath,
			&item.RelationshipStartDate,
			&item.CelebrateMonthsary,
			&item.CelebrateAnniversary,
			&item.IsDefault,
			&item.AccessHint,
			&item.AccessConfigured,
			&passphraseHash,
			&item.ArchivedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return SpaceSummary{}, err
		}
		if auth.VerifyPassword(passphraseHash, passphrase) == nil {
			return item, nil
		}
	}
	if err := rows.Err(); err != nil {
		return SpaceSummary{}, err
	}
	return SpaceSummary{}, ErrSpaceAccessNotFound
}

func (r *Repository) ListLevels(ctx context.Context) ([]Level, error) {
	rows, err := r.db.Query(ctx, `
		SELECT level, name, description
		FROM relationship_levels
		ORDER BY level ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Level, 0)
	for rows.Next() {
		var item Level
		if err := rows.Scan(&item.Level, &item.Name, &item.Description); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListLevelProgressBySpace(ctx context.Context, userID, spaceID string) ([]LevelProgress, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rlp.relationship_space_id,
		       rlp.level,
		       rlp.required_points,
		       rlp.current_points,
		       rlp.unlocked_at,
		       rlp.created_at,
		       rlp.updated_at
		FROM relationship_space_level_progress rlp
		JOIN relationship_space_members rsm ON rsm.relationship_space_id = rlp.relationship_space_id
		WHERE rlp.relationship_space_id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		ORDER BY rlp.level ASC
	`, spaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]LevelProgress, 0)
	for rows.Next() {
		var item LevelProgress
		if err := rows.Scan(
			&item.RelationshipSpaceID,
			&item.Level,
			&item.RequiredPoints,
			&item.CurrentPoints,
			&item.UnlockedAt,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListSpaceMembers(ctx context.Context, userID, spaceID string) ([]SpaceMemberSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.id, u.display_name, u.identifier
		FROM relationship_space_members me
		JOIN relationship_space_members member
		  ON member.relationship_space_id = me.relationship_space_id
		 AND member.membership_status = 'active'
		JOIN users u ON u.id = member.user_id
		WHERE me.relationship_space_id = $1
		  AND me.user_id = $2
		  AND me.membership_status = 'active'
		  AND u.deleted_at IS NULL
		ORDER BY member.joined_at ASC
	`, spaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SpaceMemberSummary, 0)
	for rows.Next() {
		var item SpaceMemberSummary
		if err := rows.Scan(&item.UserID, &item.DisplayName, &item.Identifier); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListCurrentSpaceMoods(ctx context.Context, userID, spaceID string) ([]SpaceMood, error) {
	rows, err := r.db.Query(ctx, `
		WITH active_members AS (
			SELECT member.relationship_space_id,
			       member.user_id,
			       u.display_name
			FROM relationship_space_members me
			JOIN relationship_space_members member
			  ON member.relationship_space_id = me.relationship_space_id
			 AND member.membership_status = 'active'
			JOIN users u ON u.id = member.user_id
			WHERE me.relationship_space_id = $1
			  AND me.user_id = $2
			  AND me.membership_status = 'active'
			  AND u.deleted_at IS NULL
		),
		latest_moods AS (
			SELECT DISTINCT ON (relationship_space_id, user_id)
			       relationship_space_id,
			       user_id,
			       mood_key,
			       emoji,
			       label,
			       created_at
			FROM relationship_moods
			WHERE relationship_space_id = $1
			ORDER BY relationship_space_id, user_id, created_at DESC
		)
		SELECT am.relationship_space_id,
		       am.user_id,
		       am.display_name,
		       lm.mood_key,
		       lm.emoji,
		       lm.label,
		       lm.created_at,
		       am.user_id = $2 AS is_me
		FROM active_members am
		LEFT JOIN latest_moods lm
		  ON lm.relationship_space_id = am.relationship_space_id
		 AND lm.user_id = am.user_id
		ORDER BY is_me DESC, am.display_name ASC
	`, spaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SpaceMood, 0)
	for rows.Next() {
		var item SpaceMood
		if err := rows.Scan(
			&item.RelationshipSpaceID,
			&item.UserID,
			&item.DisplayName,
			&item.MoodKey,
			&item.Emoji,
			&item.Label,
			&item.UpdatedAt,
			&item.IsMe,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) CreateSpaceMood(ctx context.Context, userID, spaceID, moodKey, emoji, label string) ([]SpaceMood, []string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		INSERT INTO relationship_moods (relationship_space_id, user_id, mood_key, emoji, label)
		SELECT $2, $1, $3, $4, $5
		WHERE EXISTS (
			SELECT 1
			FROM relationship_space_members
			WHERE relationship_space_id = $2
			  AND user_id = $1
			  AND membership_status = 'active'
		)
	`, userID, spaceID, moodKey, emoji, label)
	if err != nil {
		return nil, nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, nil, ErrSpaceNotFound
	}

	rows, err := tx.Query(ctx, `
		SELECT user_id
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
	`, spaceID)
	if err != nil {
		return nil, nil, err
	}
	memberIDs := make([]string, 0, 2)
	for rows.Next() {
		var memberID string
		if err := rows.Scan(&memberID); err != nil {
			rows.Close()
			return nil, nil, err
		}
		memberIDs = append(memberIDs, memberID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	moods, err := r.ListCurrentSpaceMoods(ctx, userID, spaceID)
	if err != nil {
		return nil, nil, err
	}
	return moods, memberIDs, nil
}

func (r *Repository) ListRelatedUserIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT other.user_id
		FROM relationship_space_members me
		JOIN relationship_space_members other
		  ON other.relationship_space_id = me.relationship_space_id
		WHERE me.user_id = $1
		  AND me.membership_status = 'active'
		  AND other.membership_status = 'active'
		  AND other.user_id <> $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0, 4)
	for rows.Next() {
		var relatedUserID string
		if err := rows.Scan(&relatedUserID); err != nil {
			return nil, err
		}
		items = append(items, relatedUserID)
	}
	return items, rows.Err()
}
