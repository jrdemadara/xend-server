package dailycheckin

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const dateLayout = "2006-01-02"

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
	Timezone            string
	ServerNow           time.Time
}

type statsRow struct {
	CompletedDaysCount       int
	CurrentStreak            int
	LastCompletedCheckinDate *time.Time
}

type milestoneRow struct {
	ID            string
	CompletedDays int
	BonusPoints   int
	Title         *string
	Description   *string
}

func (r *Repository) GetTodayStatus(ctx context.Context, userID, spaceID string) (TodayStatus, error) {
	space, err := r.loadSpaceContext(ctx, r.db, userID, spaceID)
	if err != nil {
		return TodayStatus{}, err
	}
	checkinDate, err := checkinDateForSpace(space.ServerNow, space.Timezone)
	if err != nil {
		return TodayStatus{}, err
	}
	return r.loadTodayStatus(ctx, r.db, userID, space.RelationshipSpaceID, space.Timezone, checkinDate)
}

func (r *Repository) SubmitTodayCheckIn(ctx context.Context, userID, spaceID string) (TodayStatus, []string, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return TodayStatus{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	space, err := r.loadSpaceContext(ctx, tx, userID, spaceID)
	if err != nil {
		return TodayStatus{}, nil, err
	}
	checkinDate, err := checkinDateForSpace(space.ServerNow, space.Timezone)
	if err != nil {
		return TodayStatus{}, nil, err
	}

	if err := r.ensureStatsRow(ctx, tx, space.RelationshipSpaceID); err != nil {
		return TodayStatus{}, nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO relationship_space_daily_checkins (
			relationship_space_id, user_id, checkin_date, timezone_name
		)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (relationship_space_id, user_id, checkin_date) DO NOTHING
	`, space.RelationshipSpaceID, userID, checkinDate, space.Timezone); err != nil {
		return TodayStatus{}, nil, err
	}

	memberCount, err := r.countActiveMembers(ctx, tx, space.RelationshipSpaceID)
	if err != nil {
		return TodayStatus{}, nil, err
	}
	submittedCount, err := r.countSubmittedMembers(ctx, tx, space.RelationshipSpaceID, checkinDate)
	if err != nil {
		return TodayStatus{}, nil, err
	}

	if memberCount >= 2 && submittedCount == memberCount {
		awarded, err := r.insertDailyReward(ctx, tx, space.RelationshipSpaceID, checkinDate)
		if err != nil {
			return TodayStatus{}, nil, err
		}
		if awarded {
			stats, err := r.lockStatsRow(ctx, tx, space.RelationshipSpaceID)
			if err != nil {
				return TodayStatus{}, nil, err
			}
			completedDaysCount, currentStreak := nextStats(stats, checkinDate)
			if _, err := tx.Exec(ctx, `
				UPDATE relationship_space_daily_checkin_stats
				SET completed_days_count = $2,
				    current_streak = $3,
				    last_completed_checkin_date = $4
				WHERE relationship_space_id = $1
			`, space.RelationshipSpaceID, completedDaysCount, currentStreak, checkinDate); err != nil {
				return TodayStatus{}, nil, err
			}

			pointsToAward := DailyBondPoints
			milestone, err := r.findMilestoneForCompletedDays(ctx, tx, completedDaysCount)
			if err != nil {
				return TodayStatus{}, nil, err
			}
			if milestone != nil {
				tag, err := tx.Exec(ctx, `
					INSERT INTO relationship_space_daily_checkin_rewards (
						relationship_space_id, checkin_date, reward_type, milestone_id, points
					)
					VALUES ($1, $2, 'milestone', $3, $4)
					ON CONFLICT DO NOTHING
				`, space.RelationshipSpaceID, checkinDate, milestone.ID, milestone.BonusPoints)
				if err != nil {
					return TodayStatus{}, nil, err
				}
				if tag.RowsAffected() > 0 {
					pointsToAward += milestone.BonusPoints
				}
			}

			if err := r.applyBondPoints(ctx, tx, space.RelationshipSpaceID, pointsToAward); err != nil {
				return TodayStatus{}, nil, err
			}
		}
	}

	status, err := r.loadTodayStatus(ctx, tx, userID, space.RelationshipSpaceID, space.Timezone, checkinDate)
	if err != nil {
		return TodayStatus{}, nil, err
	}
	memberIDs, err := r.listActiveMemberIDs(ctx, tx, space.RelationshipSpaceID)
	if err != nil {
		return TodayStatus{}, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return TodayStatus{}, nil, err
	}
	return status, memberIDs, nil
}

func (r *Repository) loadSpaceContext(ctx context.Context, q dbtx, userID, spaceID string) (spaceContext, error) {
	var item spaceContext
	err := q.QueryRow(ctx, `
		SELECT rs.id, rs.daily_checkin_timezone, now()
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rs.id
		WHERE rs.id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
	`, spaceID, userID).Scan(&item.RelationshipSpaceID, &item.Timezone, &item.ServerNow)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spaceContext{}, ErrRelationshipSpaceNotFound
		}
		return spaceContext{}, err
	}
	return item, nil
}

func checkinDateForSpace(now time.Time, timezone string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, ErrInvalidTimezone
	}
	localNow := now.In(location)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location), nil
}

func (r *Repository) ensureStatsRow(ctx context.Context, q dbtx, spaceID string) error {
	_, err := q.Exec(ctx, `
		INSERT INTO relationship_space_daily_checkin_stats (relationship_space_id)
		VALUES ($1)
		ON CONFLICT (relationship_space_id) DO NOTHING
	`, spaceID)
	return err
}

func (r *Repository) lockStatsRow(ctx context.Context, q dbtx, spaceID string) (statsRow, error) {
	var row statsRow
	err := q.QueryRow(ctx, `
		SELECT completed_days_count, current_streak, last_completed_checkin_date
		FROM relationship_space_daily_checkin_stats
		WHERE relationship_space_id = $1
		FOR UPDATE
	`, spaceID).Scan(&row.CompletedDaysCount, &row.CurrentStreak, &row.LastCompletedCheckinDate)
	return row, err
}

func nextStats(current statsRow, checkinDate time.Time) (int, int) {
	completedDaysCount := current.CompletedDaysCount + 1
	currentStreak := 1
	if current.LastCompletedCheckinDate != nil {
		previousDate := current.LastCompletedCheckinDate.Format(dateLayout)
		targetPrevious := checkinDate.AddDate(0, 0, -1).Format(dateLayout)
		if previousDate == targetPrevious {
			currentStreak = current.CurrentStreak + 1
		}
	}
	return completedDaysCount, currentStreak
}

func (r *Repository) countActiveMembers(ctx context.Context, q dbtx, spaceID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
	`, spaceID).Scan(&count)
	return count, err
}

func (r *Repository) countSubmittedMembers(ctx context.Context, q dbtx, spaceID string, checkinDate time.Time) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM relationship_space_daily_checkins
		WHERE relationship_space_id = $1
		  AND checkin_date = $2
	`, spaceID, checkinDate).Scan(&count)
	return count, err
}

func (r *Repository) insertDailyReward(ctx context.Context, q dbtx, spaceID string, checkinDate time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `
		INSERT INTO relationship_space_daily_checkin_rewards (
			relationship_space_id, checkin_date, reward_type, points
		)
		VALUES ($1, $2, 'daily', $3)
		ON CONFLICT DO NOTHING
	`, spaceID, checkinDate, DailyBondPoints)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (r *Repository) findMilestoneForCompletedDays(ctx context.Context, q dbtx, completedDays int) (*milestoneRow, error) {
	var item milestoneRow
	err := q.QueryRow(ctx, `
		SELECT id, completed_days, bonus_points, title, description
		FROM daily_checkin_milestones
		WHERE completed_days = $1
		  AND is_active = TRUE
		LIMIT 1
	`, completedDays).Scan(&item.ID, &item.CompletedDays, &item.BonusPoints, &item.Title, &item.Description)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
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
	var maxLevel int
	err := q.QueryRow(ctx, `
		SELECT MAX(level)
		FROM relationship_levels
	`).Scan(&maxLevel)
	return maxLevel, err
}

func (r *Repository) listActiveMemberIDs(ctx context.Context, q dbtx, spaceID string) ([]string, error) {
	rows, err := q.Query(ctx, `
		SELECT user_id
		FROM relationship_space_members
		WHERE relationship_space_id = $1
		  AND membership_status = 'active'
		ORDER BY joined_at ASC
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
	return items, rows.Err()
}

func (r *Repository) loadTodayStatus(ctx context.Context, q dbtx, userID, spaceID, timezone string, checkinDate time.Time) (TodayStatus, error) {
	if err := r.ensureStatsRow(ctx, q, spaceID); err != nil {
		return TodayStatus{}, err
	}

	var activeMemberCount int
	var submittedMemberCount int
	var myCheckedIn bool
	var partnerCheckedIn bool
	err := q.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*)
			 FROM relationship_space_members
			 WHERE relationship_space_id = $1
			   AND membership_status = 'active') AS active_member_count,
			(SELECT COUNT(*)
			 FROM relationship_space_daily_checkins
			 WHERE relationship_space_id = $1
			   AND checkin_date = $2) AS submitted_member_count,
			EXISTS (
				SELECT 1
				FROM relationship_space_daily_checkins
				WHERE relationship_space_id = $1
				  AND user_id = $3
				  AND checkin_date = $2
			) AS my_checked_in,
			EXISTS (
				SELECT 1
				FROM relationship_space_daily_checkins
				WHERE relationship_space_id = $1
				  AND user_id <> $3
				  AND checkin_date = $2
			) AS partner_checked_in
	`, spaceID, checkinDate, userID).Scan(&activeMemberCount, &submittedMemberCount, &myCheckedIn, &partnerCheckedIn)
	if err != nil {
		return TodayStatus{}, err
	}

	var stats statsRow
	if stats, err = r.getStatsRow(ctx, q, spaceID); err != nil {
		return TodayStatus{}, err
	}

	var dailyRewardPoints int
	err = q.QueryRow(ctx, `
		SELECT COALESCE(points, 0)
		FROM relationship_space_daily_checkin_rewards
		WHERE relationship_space_id = $1
		  AND checkin_date = $2
		  AND reward_type = 'daily'
		LIMIT 1
	`, spaceID, checkinDate).Scan(&dailyRewardPoints)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return TodayStatus{}, err
	}
	dailyRewardAwarded := dailyRewardPoints > 0

	var milestoneAward *MilestoneAward
	var milestone milestoneRow
	err = q.QueryRow(ctx, `
		SELECT m.id, m.completed_days, r.points, m.title, m.description
		FROM relationship_space_daily_checkin_rewards r
		JOIN daily_checkin_milestones m
		  ON m.id = r.milestone_id
		WHERE r.relationship_space_id = $1
		  AND r.checkin_date = $2
		  AND r.reward_type = 'milestone'
		LIMIT 1
	`, spaceID, checkinDate).Scan(&milestone.ID, &milestone.CompletedDays, &milestone.BonusPoints, &milestone.Title, &milestone.Description)
	if err == nil {
		milestoneAward = &MilestoneAward{
			MilestoneID:   milestone.ID,
			CompletedDays: milestone.CompletedDays,
			BonusPoints:   milestone.BonusPoints,
			Title:         milestone.Title,
			Description:   milestone.Description,
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return TodayStatus{}, err
	}

	var totalCheckInBondPointsEarned int
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(SUM(points), 0)
		FROM relationship_space_daily_checkin_rewards
		WHERE relationship_space_id = $1
	`, spaceID).Scan(&totalCheckInBondPointsEarned); err != nil {
		return TodayStatus{}, err
	}

	return TodayStatus{
		RelationshipSpaceID:          spaceID,
		Timezone:                     timezone,
		CheckInDate:                  checkinDate.Format(dateLayout),
		MyCheckedIn:                  myCheckedIn,
		PartnerCheckedIn:             partnerCheckedIn,
		AllMembersCheckedIn:          activeMemberCount >= 2 && submittedMemberCount == activeMemberCount,
		ActiveMemberCount:            activeMemberCount,
		SubmittedMemberCount:         submittedMemberCount,
		CompletedDaysCount:           stats.CompletedDaysCount,
		CurrentStreak:                stats.CurrentStreak,
		DailyRewardAwarded:           dailyRewardAwarded,
		DailyRewardPoints:            dailyRewardPoints,
		MilestoneAward:               milestoneAward,
		TotalCheckInBondPointsEarned: totalCheckInBondPointsEarned,
	}, nil
}

func (r *Repository) getStatsRow(ctx context.Context, q dbtx, spaceID string) (statsRow, error) {
	var row statsRow
	err := q.QueryRow(ctx, `
		SELECT completed_days_count, current_streak, last_completed_checkin_date
		FROM relationship_space_daily_checkin_stats
		WHERE relationship_space_id = $1
	`, spaceID).Scan(&row.CompletedDaysCount, &row.CurrentStreak, &row.LastCompletedCheckinDate)
	return row, err
}
