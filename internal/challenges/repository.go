package challenges

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type dbtx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type spaceContext struct {
	RelationshipSpaceID string
	CurrentLevel        int16
}

func (r *Repository) ListTemplates(ctx context.Context, userID, spaceID string) ([]Template, error) {
	space, err := r.loadSpaceContext(ctx, r.db, userID, spaceID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id,
		       slug,
		       title,
		       description,
		       category,
		       icon_key,
		       submission_type,
		       min_level,
		       max_level,
		       default_points,
		       expiry_hours,
		       display_order,
		       is_active,
		       created_at,
		       updated_at
		FROM challenge_templates
		WHERE is_active = TRUE
		  AND min_level <= $1
		  AND (max_level IS NULL OR max_level >= $1)
		ORDER BY display_order ASC, title ASC
	`, space.CurrentLevel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Template, 0)
	for rows.Next() {
		var item Template
		if err := rows.Scan(
			&item.ID,
			&item.Slug,
			&item.Title,
			&item.Description,
			&item.Category,
			&item.IconKey,
			&item.SubmissionType,
			&item.MinLevel,
			&item.MaxLevel,
			&item.DefaultPoints,
			&item.ExpiryHours,
			&item.DisplayOrder,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) GetOverview(ctx context.Context, userID, spaceID string) (Overview, error) {
	if _, err := r.loadSpaceContext(ctx, r.db, userID, spaceID); err != nil {
		return Overview{}, err
	}

	if err := r.expireChallengesForSpace(ctx, r.db, spaceID); err != nil {
		return Overview{}, err
	}

	incoming, err := r.listChallengesByRole(ctx, r.db, userID, spaceID, "incoming")
	if err != nil {
		return Overview{}, err
	}
	sent, err := r.listChallengesByRole(ctx, r.db, userID, spaceID, "sent")
	if err != nil {
		return Overview{}, err
	}
	history, err := r.listChallengesByRole(ctx, r.db, userID, spaceID, "history")
	if err != nil {
		return Overview{}, err
	}

	return Overview{
		RelationshipSpaceID: spaceID,
		Incoming:            incoming,
		Sent:                sent,
		History:             history,
	}, nil
}

func (r *Repository) CreateChallenge(ctx context.Context, senderUserID, spaceID, templateID string, note *string) (Overview, string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Overview{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	space, err := r.loadSpaceContext(ctx, tx, senderUserID, spaceID)
	if err != nil {
		return Overview{}, "", err
	}

	template, err := r.loadEligibleTemplate(ctx, tx, templateID, space.CurrentLevel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Overview{}, "", ErrTemplateNotFound
		}
		return Overview{}, "", err
	}

	partnerUserID, err := r.loadPartnerUserID(ctx, tx, spaceID, senderUserID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Overview{}, "", ErrPartnerNotFound
		}
		return Overview{}, "", err
	}

	var challengeID string
	err = tx.QueryRow(ctx, `
		INSERT INTO relationship_space_challenges (
			relationship_space_id,
			template_id,
			sender_user_id,
			receiver_user_id,
			assigned_level,
			note,
			reward_points,
			expires_at,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'sent')
		RETURNING id
	`, spaceID, template.ID, senderUserID, partnerUserID, space.CurrentLevel, normalizeOptionalText(note), template.DefaultPoints, expiresAtValue(template.ExpiryHours)).Scan(&challengeID)
	if err != nil {
		return Overview{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return Overview{}, "", err
	}

	overview, err := r.GetOverview(ctx, senderUserID, spaceID)
	return overview, partnerUserID, err
}

func (r *Repository) AcceptChallenge(ctx context.Context, userID, spaceID, challengeID string) (Overview, string, error) {
	return r.transitionChallenge(ctx, userID, spaceID, challengeID, func(ctx context.Context, tx dbtx, item Challenge) (string, error) {
		if item.ReceiverUserID != userID || item.Status != StatusSent {
			return "", ErrChallengeNotAllowed
		}
		var senderUserID string
		err := tx.QueryRow(ctx, `
			UPDATE relationship_space_challenges
			SET status = 'accepted',
			    accepted_at = now()
			WHERE id = $1
			RETURNING sender_user_id
		`, item.ChallengeID).Scan(&senderUserID)
		return senderUserID, err
	})
}

func (r *Repository) DeclineChallenge(ctx context.Context, userID, spaceID, challengeID string) (Overview, string, error) {
	return r.transitionChallenge(ctx, userID, spaceID, challengeID, func(ctx context.Context, tx dbtx, item Challenge) (string, error) {
		if item.ReceiverUserID != userID || (item.Status != StatusSent && item.Status != StatusAccepted) {
			return "", ErrChallengeNotAllowed
		}
		var senderUserID string
		err := tx.QueryRow(ctx, `
			UPDATE relationship_space_challenges
			SET status = 'declined'
			WHERE id = $1
			RETURNING sender_user_id
		`, item.ChallengeID).Scan(&senderUserID)
		return senderUserID, err
	})
}

func (r *Repository) CompleteChallenge(ctx context.Context, userID, spaceID, challengeID string, submission Submission) (Overview, string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Overview{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := r.loadSpaceContext(ctx, tx, userID, spaceID); err != nil {
		return Overview{}, "", err
	}
	if err := r.expireChallengesForChallenge(ctx, tx, challengeID); err != nil {
		return Overview{}, "", err
	}

	item, err := r.loadChallengeByID(ctx, tx, userID, spaceID, challengeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Overview{}, "", ErrChallengeNotFound
		}
		return Overview{}, "", err
	}
	if item.Status == StatusExpired || item.Status == StatusDeclined || item.Status == StatusCancelled || item.Status == StatusCompleted {
		return Overview{}, "", ErrChallengeUnavailable
	}
	if item.ReceiverUserID != userID || !item.CanComplete {
		return Overview{}, "", ErrChallengeNotAllowed
	}

	trimmedText := strings.TrimSpace(valueOrEmpty(submission.TextResponse))
	switch item.SubmissionType {
	case SubmissionTypeNone:
	case SubmissionTypeText:
		if trimmedText == "" {
			return Overview{}, "", ErrTextResponseRequired
		}
	case SubmissionTypeImage:
		if submission.ImagePath == nil || strings.TrimSpace(*submission.ImagePath) == "" {
			return Overview{}, "", ErrImageRequired
		}
	default:
		return Overview{}, "", ErrUnsupportedSubmissionType
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO relationship_space_challenge_submissions (
			challenge_id,
			submitted_by_user_id,
			text_response,
			image_path
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (challenge_id, submitted_by_user_id) DO NOTHING
	`, challengeID, userID, nullableText(trimmedText), submission.ImagePath); err != nil {
		return Overview{}, "", err
	}

	awardedPoints, senderUserID, err := r.completeChallengeAndLockReward(ctx, tx, challengeID)
	if err != nil {
		return Overview{}, "", err
	}
	if awardedPoints > 0 {
		if err := r.applyBondPoints(ctx, tx, spaceID, awardedPoints); err != nil {
			return Overview{}, "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Overview{}, "", err
	}
	overview, err := r.GetOverview(ctx, userID, spaceID)
	return overview, senderUserID, err
}

func (r *Repository) transitionChallenge(ctx context.Context, userID, spaceID, challengeID string, action func(context.Context, dbtx, Challenge) (string, error)) (Overview, string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Overview{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := r.loadSpaceContext(ctx, tx, userID, spaceID); err != nil {
		return Overview{}, "", err
	}
	if err := r.expireChallengesForChallenge(ctx, tx, challengeID); err != nil {
		return Overview{}, "", err
	}

	item, err := r.loadChallengeByID(ctx, tx, userID, spaceID, challengeID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Overview{}, "", ErrChallengeNotFound
		}
		return Overview{}, "", err
	}
	if item.Status == StatusExpired || item.Status == StatusDeclined || item.Status == StatusCancelled || item.Status == StatusCompleted {
		return Overview{}, "", ErrChallengeUnavailable
	}

	otherUserID, err := action(ctx, tx, item)
	if err != nil {
		return Overview{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return Overview{}, "", err
	}
	overview, err := r.GetOverview(ctx, userID, spaceID)
	return overview, otherUserID, err
}

func (r *Repository) loadSpaceContext(ctx context.Context, q dbtx, userID, spaceID string) (spaceContext, error) {
	var item spaceContext
	err := q.QueryRow(ctx, `
		SELECT rs.id, rs.current_level
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rs.id
		WHERE rs.id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		  AND rs.archived_at IS NULL
		LIMIT 1
	`, spaceID, userID).Scan(&item.RelationshipSpaceID, &item.CurrentLevel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spaceContext{}, ErrRelationshipSpaceNotFound
		}
		return spaceContext{}, err
	}
	return item, nil
}

func (r *Repository) loadEligibleTemplate(ctx context.Context, q dbtx, templateID string, level int16) (Template, error) {
	var item Template
	err := q.QueryRow(ctx, `
		SELECT id,
		       slug,
		       title,
		       description,
		       category,
		       icon_key,
		       submission_type,
		       min_level,
		       max_level,
		       default_points,
		       expiry_hours,
		       display_order,
		       is_active,
		       created_at,
		       updated_at
		FROM challenge_templates
		WHERE id = $1
		  AND is_active = TRUE
		  AND min_level <= $2
		  AND (max_level IS NULL OR max_level >= $2)
		LIMIT 1
	`, templateID, level).Scan(
		&item.ID,
		&item.Slug,
		&item.Title,
		&item.Description,
		&item.Category,
		&item.IconKey,
		&item.SubmissionType,
		&item.MinLevel,
		&item.MaxLevel,
		&item.DefaultPoints,
		&item.ExpiryHours,
		&item.DisplayOrder,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) loadPartnerUserID(ctx context.Context, q dbtx, spaceID, userID string) (string, error) {
	var partnerUserID string
	err := q.QueryRow(ctx, `
		SELECT user_id
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
		  AND user_id <> $2
		ORDER BY joined_at ASC
		LIMIT 1
	`, spaceID, userID).Scan(&partnerUserID)
	return partnerUserID, err
}

func (r *Repository) expireChallengesForSpace(ctx context.Context, q dbtx, spaceID string) error {
	_, err := q.Exec(ctx, `
		UPDATE relationship_space_challenges
		SET status = 'expired'
		WHERE relationship_space_id = $1
		  AND status IN ('sent', 'accepted')
		  AND expires_at IS NOT NULL
		  AND expires_at <= now()
	`, spaceID)
	return err
}

func (r *Repository) expireChallengesForChallenge(ctx context.Context, q dbtx, challengeID string) error {
	_, err := q.Exec(ctx, `
		UPDATE relationship_space_challenges
		SET status = 'expired'
		WHERE id = $1
		  AND status IN ('sent', 'accepted')
		  AND expires_at IS NOT NULL
		  AND expires_at <= now()
	`, challengeID)
	return err
}

func (r *Repository) listChallengesByRole(ctx context.Context, q dbtx, userID, spaceID, role string) ([]Challenge, error) {
	var (
		whereClause string
		limit       int
	)
	switch role {
	case "incoming":
		whereClause = "c.receiver_user_id = $2 AND c.status IN ('sent', 'accepted')"
		limit = 20
	case "sent":
		whereClause = "c.sender_user_id = $2 AND c.status IN ('sent', 'accepted')"
		limit = 20
	case "history":
		whereClause = "(c.sender_user_id = $2 OR c.receiver_user_id = $2) AND c.status IN ('completed', 'declined', 'expired', 'cancelled')"
		limit = 40
	default:
		return []Challenge{}, nil
	}

	rows, err := q.Query(ctx, `
		SELECT c.id,
		       c.relationship_space_id,
		       c.template_id,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.submission_type,
		       c.sender_user_id,
		       su.display_name,
		       c.receiver_user_id,
		       ru.display_name,
		       c.assigned_level,
		       c.reward_points,
		       c.note,
		       c.status,
		       c.expires_at,
		       c.accepted_at,
		       c.completed_at,
		       c.created_at,
		       c.updated_at,
		       EXISTS (
		           SELECT 1
		           FROM relationship_space_challenge_submissions s
		           WHERE s.challenge_id = c.id
		             AND s.submitted_by_user_id = $2
		       ) AS submitted_by_me
		FROM relationship_space_challenges c
		JOIN challenge_templates t
		  ON t.id = c.template_id
		JOIN users su
		  ON su.id = c.sender_user_id
		JOIN users ru
		  ON ru.id = c.receiver_user_id
		WHERE c.relationship_space_id = $1
		  AND `+whereClause+`
		ORDER BY c.created_at DESC
		LIMIT $3
	`, spaceID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Challenge, 0)
	for rows.Next() {
		item, err := scanChallenge(rows, userID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) loadChallengeByID(ctx context.Context, q dbtx, userID, spaceID, challengeID string) (Challenge, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id,
		       c.relationship_space_id,
		       c.template_id,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.submission_type,
		       c.sender_user_id,
		       su.display_name,
		       c.receiver_user_id,
		       ru.display_name,
		       c.assigned_level,
		       c.reward_points,
		       c.note,
		       c.status,
		       c.expires_at,
		       c.accepted_at,
		       c.completed_at,
		       c.created_at,
		       c.updated_at,
		       EXISTS (
		           SELECT 1
		           FROM relationship_space_challenge_submissions s
		           WHERE s.challenge_id = c.id
		             AND s.submitted_by_user_id = $3
		       ) AS submitted_by_me
		FROM relationship_space_challenges c
		JOIN challenge_templates t
		  ON t.id = c.template_id
		JOIN users su
		  ON su.id = c.sender_user_id
		JOIN users ru
		  ON ru.id = c.receiver_user_id
		WHERE c.relationship_space_id = $1
		  AND c.id = $2
		  AND (c.sender_user_id = $3 OR c.receiver_user_id = $3)
		LIMIT 1
	`, spaceID, challengeID, userID)
	if err != nil {
		return Challenge{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Challenge{}, err
		}
		return Challenge{}, pgx.ErrNoRows
	}
	return scanChallenge(rows, userID)
}

func scanChallenge(scanner interface{ Scan(dest ...any) error }, userID string) (Challenge, error) {
	var (
		item      Challenge
		status    string
		note      *string
		expiresAt *time.Time
	)
	err := scanner.Scan(
		&item.ChallengeID,
		&item.RelationshipSpaceID,
		&item.TemplateID,
		&item.Title,
		&item.Description,
		&item.Category,
		&item.IconKey,
		&item.SubmissionType,
		&item.SenderUserID,
		&item.SenderDisplayName,
		&item.ReceiverUserID,
		&item.ReceiverDisplayName,
		&item.AssignedLevel,
		&item.RewardPoints,
		&note,
		&status,
		&expiresAt,
		&item.AcceptedAt,
		&item.CompletedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
		&item.SubmittedByMe,
	)
	if err != nil {
		return Challenge{}, err
	}
	item.Note = normalizeOptionalText(note)
	item.Status = Status(status)
	item.ExpiresAt = expiresAt
	item.CanAccept = item.ReceiverUserID == userID && item.Status == StatusSent
	item.CanDecline = item.ReceiverUserID == userID && (item.Status == StatusSent || item.Status == StatusAccepted)
	item.CanComplete = item.ReceiverUserID == userID &&
		(item.Status == StatusSent || item.Status == StatusAccepted) &&
		!item.SubmittedByMe
	return item, nil
}

func (r *Repository) completeChallengeAndLockReward(ctx context.Context, q dbtx, challengeID string) (int, string, error) {
	var (
		rewardPoints int
		senderUserID string
	)
	err := q.QueryRow(ctx, `
		UPDATE relationship_space_challenges
		SET status = 'completed',
		    accepted_at = COALESCE(accepted_at, now()),
		    completed_at = COALESCE(completed_at, now()),
		    reward_awarded_at = now()
		WHERE id = $1
		  AND reward_awarded_at IS NULL
		RETURNING reward_points, sender_user_id
	`, challengeID).Scan(&rewardPoints, &senderUserID)
	if err == nil {
		return rewardPoints, senderUserID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, "", err
	}

	err = q.QueryRow(ctx, `
		UPDATE relationship_space_challenges
		SET status = 'completed',
		    accepted_at = COALESCE(accepted_at, now()),
		    completed_at = COALESCE(completed_at, now())
		WHERE id = $1
		RETURNING sender_user_id
	`, challengeID).Scan(&senderUserID)
	if err != nil {
		return 0, "", err
	}
	return 0, senderUserID, nil
}

func expiresAtValue(expiryHours *int) *time.Time {
	if expiryHours == nil || *expiryHours <= 0 {
		return nil
	}
	value := time.Now().UTC().Add(time.Duration(*expiryHours) * time.Hour)
	return &value
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (r *Repository) applyBondPoints(ctx context.Context, q dbtx, spaceID string, points int) error {
	if points <= 0 {
		return nil
	}

	var currentLevel int
	err := q.QueryRow(ctx, `
		SELECT current_level
		FROM relationship_spaces
		WHERE id = $1
		FOR UPDATE
	`, spaceID).Scan(&currentLevel)
	if err != nil {
		return err
	}

	maxLevel, err := r.maxLevel(ctx, q)
	if err != nil {
		return err
	}

	remaining := points
	for remaining > 0 {
		requiredPoints, err := r.ensureLevelProgressRow(ctx, q, spaceID, currentLevel)
		if err != nil {
			return err
		}

		currentPoints, err := r.lockCurrentLevelPoints(ctx, q, spaceID, currentLevel)
		if err != nil {
			return err
		}

		available := requiredPoints - currentPoints
		if available <= 0 {
			if currentLevel >= maxLevel {
				return nil
			}
			currentLevel++
			if _, err := q.Exec(ctx, `
				UPDATE relationship_spaces
				SET current_level = $2
				WHERE id = $1
			`, spaceID, currentLevel); err != nil {
				return err
			}
			continue
		}

		applied := remaining
		if applied > available {
			applied = available
		}

		newPoints := currentPoints + applied
		if _, err := q.Exec(ctx, `
			UPDATE relationship_space_level_progress
			SET current_points = $3
			WHERE relationship_space_id = $1
			  AND level = $2
		`, spaceID, currentLevel, newPoints); err != nil {
			return err
		}

		remaining -= applied
		if newPoints < requiredPoints || currentLevel >= maxLevel {
			return nil
		}

		currentLevel++
		if _, err := q.Exec(ctx, `
			UPDATE relationship_spaces
			SET current_level = $2
			WHERE id = $1
		`, spaceID, currentLevel); err != nil {
			return err
		}
	}

	return nil
}

func (r *Repository) ensureLevelProgressRow(ctx context.Context, q dbtx, spaceID string, level int) (int, error) {
	var requiredPoints int
	err := q.QueryRow(ctx, `
		SELECT required_points
		FROM relationship_levels
		WHERE level = $1
	`, level).Scan(&requiredPoints)
	if err != nil {
		return 0, err
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO relationship_space_level_progress (
			relationship_space_id,
			level,
			required_points,
			current_points,
			unlocked_at
		)
		VALUES ($1, $2, $3, 0, now())
		ON CONFLICT (relationship_space_id, level) DO NOTHING
	`, spaceID, level, requiredPoints); err != nil {
		return 0, err
	}
	return requiredPoints, nil
}

func (r *Repository) lockCurrentLevelPoints(ctx context.Context, q dbtx, spaceID string, level int) (int, error) {
	var currentPoints int
	err := q.QueryRow(ctx, `
		SELECT current_points
		FROM relationship_space_level_progress
		WHERE relationship_space_id = $1
		  AND level = $2
		FOR UPDATE
	`, spaceID, level).Scan(&currentPoints)
	return currentPoints, err
}

func (r *Repository) maxLevel(ctx context.Context, q dbtx) (int, error) {
	var highestLevel int
	err := q.QueryRow(ctx, `
		SELECT MAX(level)
		FROM relationship_levels
	`).Scan(&highestLevel)
	return highestLevel, err
}
