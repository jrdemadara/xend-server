package dailyritual

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const ritualDateLayout = "2006-01-02"

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
	Timezone            string
	ServerNow           time.Time
}

func (r *Repository) ListTemplates(ctx context.Context) ([]Template, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id,
		       slug,
		       title,
		       description,
		       category,
		       icon_key,
		       suggested_time,
		       default_points,
		       submission_type,
		       target_type,
		       completion_rule,
		       min_level,
		       max_level,
		       display_order,
		       is_active,
		       created_at,
		       updated_at
		FROM daily_ritual_templates
		WHERE is_active = TRUE
		ORDER BY min_level ASC, display_order ASC, title ASC
	`)
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
			&item.SuggestedTime,
			&item.DefaultPoints,
			&item.SubmissionType,
			&item.TargetType,
			&item.CompletionRule,
			&item.MinLevel,
			&item.MaxLevel,
			&item.DisplayOrder,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) GetOverview(ctx context.Context, userID, spaceID string) (Overview, error) {
	space, err := r.loadSpaceContext(ctx, r.db, userID, spaceID)
	if err != nil {
		return Overview{}, err
	}

	ritualDate, err := ritualDateForSpace(space.ServerNow, space.Timezone)
	if err != nil {
		return Overview{}, err
	}

	todayRitual, err := r.getOrCreateTodayAssignment(ctx, space, ritualDate, userID)
	if err != nil {
		return Overview{}, err
	}

	history, err := r.listHistory(ctx, r.db, userID, space.RelationshipSpaceID, ritualDate, 8)
	if err != nil {
		return Overview{}, err
	}

	return Overview{
		RelationshipSpaceID: space.RelationshipSpaceID,
		RitualDate:          ritualDate.Format(ritualDateLayout),
		TodayRitual:         todayRitual,
		History:             history,
	}, nil
}

func (r *Repository) Submit(ctx context.Context, userID, spaceID, assignmentID string, submission Submission) (Overview, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Overview{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	space, err := r.loadSpaceContext(ctx, tx, userID, spaceID)
	if err != nil {
		return Overview{}, err
	}

	ritualDate, err := ritualDateForSpace(space.ServerNow, space.Timezone)
	if err != nil {
		return Overview{}, err
	}

	assignment, err := r.loadAssignmentByID(ctx, tx, userID, space.RelationshipSpaceID, assignmentID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Overview{}, ErrAssignmentNotFound
		}
		return Overview{}, err
	}

	if assignment.RitualDate != ritualDate.Format(ritualDateLayout) {
		return Overview{}, ErrAssignmentUnavailable
	}
	if assignment.Status == AssignmentStatusExpired {
		return Overview{}, ErrAssignmentUnavailable
	}
	if assignment.SubmittedByMe {
		if err := tx.Commit(ctx); err != nil {
			return Overview{}, err
		}
		return r.GetOverview(ctx, userID, spaceID)
	}
	if !assignment.CanSubmit {
		return Overview{}, ErrSubmissionNotAllowed
	}

	trimmedText := strings.TrimSpace(valueOrEmpty(submission.TextResponse))
	switch assignment.SubmissionType {
	case SubmissionTypeNone:
	case SubmissionTypeText:
		if trimmedText == "" {
			return Overview{}, ErrTextResponseRequired
		}
	case SubmissionTypeImage:
		if submission.ImagePath == nil || strings.TrimSpace(*submission.ImagePath) == "" {
			return Overview{}, ErrImageRequired
		}
	default:
		return Overview{}, ErrUnsupportedSubmissionType
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO relationship_space_daily_ritual_completions (
			assignment_id,
			user_id,
			text_response,
			image_path
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (assignment_id, user_id) DO NOTHING
	`, assignment.AssignmentID, userID, nullableText(trimmedText), submission.ImagePath); err != nil {
		return Overview{}, err
	}

	updatedAssignment, err := r.loadAssignmentByID(ctx, tx, userID, space.RelationshipSpaceID, assignmentID)
	if err != nil {
		return Overview{}, err
	}

	if updatedAssignment.SubmittedCount >= updatedAssignment.RequiredCount && updatedAssignment.Status != AssignmentStatusCompleted {
		awardedPoints, awardErr := r.completeAssignmentAndLockReward(ctx, tx, updatedAssignment.AssignmentID)
		if awardErr != nil {
			return Overview{}, awardErr
		}
		if awardedPoints > 0 {
			if err := r.applyBondPoints(ctx, tx, space.RelationshipSpaceID, awardedPoints); err != nil {
				return Overview{}, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Overview{}, err
	}
	return r.GetOverview(ctx, userID, spaceID)
}

func (r *Repository) getOrCreateTodayAssignment(ctx context.Context, space spaceContext, ritualDate time.Time, userID string) (*AssignedRitual, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := r.loadAssignmentForDate(ctx, tx, userID, space.RelationshipSpaceID, ritualDate)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return current, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	template, err := r.pickTemplateForSpace(ctx, tx, space.RelationshipSpaceID, space.CurrentLevel)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, err
	}

	memberIDs, err := r.listActiveMemberIDs(ctx, tx, space.RelationshipSpaceID)
	if err != nil {
		return nil, err
	}
	targetUserID := resolveTargetUserID(template.TargetType, memberIDs, ritualDate)

	insertErr := tx.QueryRow(ctx, `
		INSERT INTO relationship_space_daily_ritual_assignments (
			relationship_space_id,
			template_id,
			ritual_date,
			timezone_name,
			assigned_level,
			target_user_id,
			reward_points,
			status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 'assigned')
		ON CONFLICT (relationship_space_id, ritual_date) DO NOTHING
		RETURNING id
	`, space.RelationshipSpaceID, template.ID, ritualDate, space.Timezone, space.CurrentLevel, targetUserID, template.DefaultPoints).Scan(new(string))
	if insertErr != nil && !errors.Is(insertErr, pgx.ErrNoRows) {
		return nil, insertErr
	}

	current, err = r.loadAssignmentForDate(ctx, tx, userID, space.RelationshipSpaceID, ritualDate)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return current, nil
}

func (r *Repository) loadSpaceContext(ctx context.Context, q dbtx, userID, spaceID string) (spaceContext, error) {
	var item spaceContext
	err := q.QueryRow(ctx, `
		SELECT rs.id, rs.current_level, rs.daily_checkin_timezone, now()
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rs.id
		WHERE rs.id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		  AND rs.archived_at IS NULL
		LIMIT 1
	`, spaceID, userID).Scan(&item.RelationshipSpaceID, &item.CurrentLevel, &item.Timezone, &item.ServerNow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spaceContext{}, ErrRelationshipSpaceNotFound
		}
		return spaceContext{}, err
	}
	return item, nil
}

func ritualDateForSpace(now time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, ErrInvalidTimezone
	}
	localNow := now.In(location)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location), nil
}

func (r *Repository) pickTemplateForSpace(ctx context.Context, q dbtx, spaceID string, level int16) (Template, error) {
	var item Template
	err := q.QueryRow(ctx, `
		SELECT t.id,
		       t.slug,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.suggested_time,
		       t.default_points,
		       t.submission_type,
		       t.target_type,
		       t.completion_rule,
		       t.min_level,
		       t.max_level,
		       t.display_order,
		       t.is_active,
		       t.created_at,
		       t.updated_at
		FROM daily_ritual_templates t
		LEFT JOIN relationship_space_daily_ritual_assignments a
		  ON a.relationship_space_id = $1
		 AND a.template_id = t.id
		WHERE t.is_active = TRUE
		  AND t.min_level <= $2
		  AND (t.max_level IS NULL OR t.max_level >= $2)
		GROUP BY t.id
		ORDER BY COALESCE(MAX(a.ritual_date), DATE '1970-01-01') ASC,
		         t.display_order ASC,
		         t.title ASC
		LIMIT 1
	`, spaceID, level).Scan(
		&item.ID,
		&item.Slug,
		&item.Title,
		&item.Description,
		&item.Category,
		&item.IconKey,
		&item.SuggestedTime,
		&item.DefaultPoints,
		&item.SubmissionType,
		&item.TargetType,
		&item.CompletionRule,
		&item.MinLevel,
		&item.MaxLevel,
		&item.DisplayOrder,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

func (r *Repository) loadAssignmentForDate(ctx context.Context, q dbtx, userID, spaceID string, ritualDate time.Time) (*AssignedRitual, error) {
	return loadAssignedRitual(ctx, q, userID, `
		SELECT a.id,
		       a.ritual_date,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.suggested_time,
		       a.reward_points,
		       t.submission_type,
		       t.target_type,
		       t.completion_rule,
		       a.status,
		       a.target_user_id,
		       EXISTS (
		           SELECT 1
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		             AND c.user_id = $3
		       ) AS submitted_by_me,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		       ) AS submitted_count,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_members m
		           WHERE m.relationship_space_id = a.relationship_space_id
		             AND m.membership_status = 'active'
		       ) AS active_member_count
		FROM relationship_space_daily_ritual_assignments a
		JOIN daily_ritual_templates t
		  ON t.id = a.template_id
		WHERE a.relationship_space_id = $1
		  AND a.ritual_date = $2
		LIMIT 1
	`, spaceID, ritualDate, userID)
}

func (r *Repository) loadAssignmentByID(ctx context.Context, q dbtx, userID, spaceID, assignmentID string) (*AssignedRitual, error) {
	return loadAssignedRitual(ctx, q, userID, `
		SELECT a.id,
		       a.ritual_date,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.suggested_time,
		       a.reward_points,
		       t.submission_type,
		       t.target_type,
		       t.completion_rule,
		       a.status,
		       a.target_user_id,
		       EXISTS (
		           SELECT 1
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		             AND c.user_id = $3
		       ) AS submitted_by_me,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		       ) AS submitted_count,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_members m
		           WHERE m.relationship_space_id = a.relationship_space_id
		             AND m.membership_status = 'active'
		       ) AS active_member_count
		FROM relationship_space_daily_ritual_assignments a
		JOIN daily_ritual_templates t
		  ON t.id = a.template_id
		WHERE a.relationship_space_id = $1
		  AND a.id = $2
		LIMIT 1
	`, spaceID, assignmentID, userID)
}

func (r *Repository) listHistory(ctx context.Context, q dbtx, userID, spaceID string, ritualDate time.Time, limit int) ([]AssignedRitual, error) {
	rows, err := q.Query(ctx, `
		SELECT a.id,
		       a.ritual_date,
		       t.title,
		       t.description,
		       t.category,
		       t.icon_key,
		       t.suggested_time,
		       a.reward_points,
		       t.submission_type,
		       t.target_type,
		       t.completion_rule,
		       a.status,
		       a.target_user_id,
		       EXISTS (
		           SELECT 1
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		             AND c.user_id = $4
		       ) AS submitted_by_me,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_daily_ritual_completions c
		           WHERE c.assignment_id = a.id
		       ) AS submitted_count,
		       (
		           SELECT COUNT(*)
		           FROM relationship_space_members m
		           WHERE m.relationship_space_id = a.relationship_space_id
		             AND m.membership_status = 'active'
		       ) AS active_member_count
		FROM relationship_space_daily_ritual_assignments a
		JOIN daily_ritual_templates t
		  ON t.id = a.template_id
		WHERE a.relationship_space_id = $1
		  AND a.ritual_date < $2
		ORDER BY a.ritual_date DESC
		LIMIT $3
	`, spaceID, ritualDate, limit, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]AssignedRitual, 0)
	for rows.Next() {
		item, err := scanAssignedRitual(rows, userID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func loadAssignedRitual(ctx context.Context, q dbtx, userID, sql string, args ...any) (*AssignedRitual, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, pgx.ErrNoRows
	}

	item, err := scanAssignedRitual(rows, userID)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanAssignedRitual(scanner interface {
	Scan(dest ...any) error
}, userID string) (AssignedRitual, error) {
	var (
		item              AssignedRitual
		ritualDate        time.Time
		status            string
		activeMemberCount int
	)
	err := scanner.Scan(
		&item.AssignmentID,
		&ritualDate,
		&item.Title,
		&item.Description,
		&item.Category,
		&item.IconKey,
		&item.SuggestedTime,
		&item.RewardPoints,
		&item.SubmissionType,
		&item.TargetType,
		&item.CompletionRule,
		&status,
		&item.TargetUserID,
		&item.SubmittedByMe,
		&item.SubmittedCount,
		&activeMemberCount,
	)
	if err != nil {
		return AssignedRitual{}, err
	}
	item.RitualDate = ritualDate.Format(ritualDateLayout)
	item.Status = AssignmentStatus(status)
	item.Completed = item.Status == AssignmentStatusCompleted
	item.RequiredCount = 1
	if item.CompletionRule == CompletionRuleBothPartners && activeMemberCount > 0 {
		item.RequiredCount = activeMemberCount
	}
	item.CanSubmit = item.Status == AssignmentStatusAssigned &&
		!item.SubmittedByMe &&
		(item.TargetUserID == nil || *item.TargetUserID == userID)
	return item, nil
}

func (r *Repository) listActiveMemberIDs(ctx context.Context, q dbtx, spaceID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT user_id
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
		ORDER BY user_id ASC
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		items = append(items, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func resolveTargetUserID(targetType TargetType, memberIDs []string, ritualDate time.Time) *string {
	if targetType != TargetTypeOnePartner || len(memberIDs) == 0 {
		return nil
	}

	seed, err := strconv.ParseInt(ritualDate.Format("20060102"), 10, 64)
	if err != nil {
		target := memberIDs[0]
		return &target
	}

	target := memberIDs[int(seed%int64(len(memberIDs)))]
	return &target
}

func (r *Repository) completeAssignmentAndLockReward(ctx context.Context, q dbtx, assignmentID string) (int, error) {
	var rewardPoints int
	err := q.QueryRow(ctx, `
		UPDATE relationship_space_daily_ritual_assignments
		SET status = 'completed',
		    completed_at = COALESCE(completed_at, now()),
		    reward_awarded_at = now()
		WHERE id = $1
		  AND reward_awarded_at IS NULL
		RETURNING reward_points
	`, assignmentID).Scan(&rewardPoints)
	if err == nil {
		return rewardPoints, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	_, err = q.Exec(ctx, `
		UPDATE relationship_space_daily_ritual_assignments
		SET status = 'completed',
		    completed_at = COALESCE(completed_at, now())
		WHERE id = $1
	`, assignmentID)
	if err != nil {
		return 0, err
	}
	return 0, nil
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
