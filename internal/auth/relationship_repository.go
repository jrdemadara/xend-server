package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type RelationshipInvite struct {
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

type RelationshipSpaceSummary struct {
	RelationshipSpaceID string
	Name                *string
	CreatedByUserID     string
	CurrentLevel        int16
	CurrentLevelName    string
	ArchivedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type RelationshipInviteOutbox struct {
	InviteID          string
	InviteeIdentifier string
	Status            string
	Note              *string
	CreatedAt         time.Time
}

type RelationshipLevel struct {
	Level       int16
	Name        string
	Description *string
}

type RelationshipLevelProgress struct {
	RelationshipSpaceID string
	Level               int16
	RequiredPoints      int32
	CurrentPoints       int32
	UnlockedAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

var (
	ErrInviteNotFound = errors.New("invite not found")
)

func (r *Repository) CreateRelationshipInviteByIdentifier(ctx context.Context, inviterUserID, inviteeIdentifier string, note *string) (string, string, string, error) {
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
		INSERT INTO relationship_invites (
			id, inviter_user_id, invitee_user_id, status, note
		)
		VALUES ($1, $2, $3, 'pending', $4)
	`, inviteID, inviterUserID, inviteeUserID, note); err != nil {
		return "", "", "", err
	}

	if err = tx.Commit(ctx); err != nil {
		return "", "", "", err
	}
	return inviteID, inviteeUserID, inviteeEmail, nil
}

func (r *Repository) ListInviteInbox(ctx context.Context, inviteeUserID string) ([]RelationshipInvite, error) {
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

	var invites []RelationshipInvite
	for rows.Next() {
		var item RelationshipInvite
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
		invites = append(invites, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return invites, nil
}

func (r *Repository) ListInviteOutbox(ctx context.Context, inviterUserID string) ([]RelationshipInviteOutbox, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ri.id,
		       u.identifier,
		       ri.status,
		       ri.note,
		       ri.created_at
		FROM relationship_invites ri
		JOIN users u ON u.id = ri.invitee_user_id
		WHERE ri.inviter_user_id = $1
		ORDER BY ri.created_at DESC
	`, inviterUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelationshipInviteOutbox, 0)
	for rows.Next() {
		var it RelationshipInviteOutbox
		if err := rows.Scan(
			&it.InviteID,
			&it.InviteeIdentifier,
			&it.Status,
			&it.Note,
			&it.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) AcceptRelationshipInvite(ctx context.Context, inviteID, inviteeUserID string) (string, string, string, error) {
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
			INSERT INTO relationship_space_members (
				relationship_space_id, user_id, joined_by_user_id, role, membership_status
			)
			VALUES ($1, $2, $2, 'owner', 'active')
			ON CONFLICT (relationship_space_id, user_id)
			DO UPDATE SET membership_status = 'active', left_at = NULL, joined_at = now()
		`, spaceID, inviterUserID); err != nil {
			return "", "", "", err
		}
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO relationship_space_members (
			relationship_space_id, user_id, joined_by_user_id, role, membership_status
		)
		VALUES ($1, $2, $3, 'member', 'active')
		ON CONFLICT (relationship_space_id, user_id)
		DO UPDATE SET
			membership_status = 'active',
			left_at = NULL,
			joined_at = now()
	`, spaceID, inviteeUserID, inviterUserID); err != nil {
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

func (r *Repository) DeclineRelationshipInvite(ctx context.Context, inviteID, inviteeUserID string) (string, error) {
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

func (r *Repository) ListRelationshipSpacesByUser(ctx context.Context, userID string) ([]RelationshipSpaceSummary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rs.id,
		       rs.name,
		       rs.created_by_user_id,
		       rs.current_level,
		       COALESCE(rl.name, 'Tease') AS current_level_name,
		       rs.archived_at,
		       rs.created_at,
		       rs.updated_at
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rs.id
		LEFT JOIN relationship_levels rl
		  ON rl.level = rs.current_level
		WHERE rsm.user_id = $1
		  AND rsm.membership_status = 'active'
		ORDER BY rs.updated_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelationshipSpaceSummary, 0)
	for rows.Next() {
		var it RelationshipSpaceSummary
		if err := rows.Scan(
			&it.RelationshipSpaceID,
			&it.Name,
			&it.CreatedByUserID,
			&it.CurrentLevel,
			&it.CurrentLevelName,
			&it.ArchivedAt,
			&it.CreatedAt,
			&it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListRelationshipLevels(ctx context.Context) ([]RelationshipLevel, error) {
	rows, err := r.db.Query(ctx, `
		SELECT level, name, description
		FROM relationship_levels
		ORDER BY level ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelationshipLevel, 0)
	for rows.Next() {
		var it RelationshipLevel
		if err := rows.Scan(&it.Level, &it.Name, &it.Description); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ListRelationshipLevelProgressBySpace(ctx context.Context, userID, spaceID string) ([]RelationshipLevelProgress, error) {
	rows, err := r.db.Query(ctx, `
		SELECT rlp.relationship_space_id,
		       rlp.level,
		       rlp.required_points,
		       rlp.current_points,
		       rlp.unlocked_at,
		       rlp.created_at,
		       rlp.updated_at
		FROM relationship_space_level_progress rlp
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rlp.relationship_space_id
		WHERE rlp.relationship_space_id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		ORDER BY rlp.level ASC
	`, spaceID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]RelationshipLevelProgress, 0)
	for rows.Next() {
		var it RelationshipLevelProgress
		if err := rows.Scan(
			&it.RelationshipSpaceID,
			&it.Level,
			&it.RequiredPoints,
			&it.CurrentPoints,
			&it.UnlockedAt,
			&it.CreatedAt,
			&it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
