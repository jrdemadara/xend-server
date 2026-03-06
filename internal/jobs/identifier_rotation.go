package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	pgxconn "github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"xend.chat/m/pkg/idgen"
)

type IdentifierRotationJob struct {
	db            *pgxpool.Pool
	rotationEvery time.Duration
}

func NewIdentifierRotationJob(db *pgxpool.Pool, rotationEvery time.Duration) *IdentifierRotationJob {
	return &IdentifierRotationJob{db: db, rotationEvery: rotationEvery}
}

func (j *IdentifierRotationJob) RotateDueUsers(ctx context.Context) (int, error) {
	rows, err := j.db.Query(ctx, `
		SELECT id
		FROM users
		WHERE auto_rotate_identifier = TRUE
		  AND identifier_rotates_at IS NOT NULL
		  AND identifier_rotates_at <= now()
		  AND deleted_at IS NULL
		LIMIT 100
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	rotated := 0
	for _, userID := range ids {
		if err := j.rotateOne(ctx, userID); err != nil {
			return rotated, err
		}
		rotated++
	}
	return rotated, nil
}

func (j *IdentifierRotationJob) rotateOne(ctx context.Context, userID string) error {
	for range 8 {
		newIdentifier, err := idgen.Identifier(8)
		if err != nil {
			return err
		}

		if j.rotationEvery > 0 {
			_, err = j.db.Exec(ctx, `
				UPDATE users
				SET identifier = $1,
				    identifier_rotates_at = now() + ($3::interval),
				    updated_at = now()
				WHERE id = $2
				  AND auto_rotate_identifier = TRUE
			`, newIdentifier, userID, j.rotationEvery.String())
		} else {
			_, err = j.db.Exec(ctx, `
				UPDATE users
				SET identifier = $1,
				    identifier_rotates_at = now() + ((identifier_rotation_days || ' days')::interval),
				    updated_at = now()
				WHERE id = $2
				  AND auto_rotate_identifier = TRUE
			`, newIdentifier, userID)
		}
		if err == nil {
			return nil
		}
		if !isUniqueViolation(err) {
			return err
		}
	}
	return fmt.Errorf("failed to rotate identifier after retries")
}

func isUniqueViolation(err error) bool {
	pgErr := &pgxconn.PgError{}
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
