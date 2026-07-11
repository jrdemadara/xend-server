package user

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetProfileByID(ctx context.Context, userID string) (Profile, error) {
	var profile Profile
	err := r.db.QueryRow(ctx, `
		SELECT id, display_name, email::text, avatar_url, identifier
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(
		&profile.ID,
		&profile.DisplayName,
		&profile.Email,
		&profile.AvatarURL,
		&profile.Identifier,
	)
	if err != nil {
		return Profile{}, err
	}
	return profile, nil
}
