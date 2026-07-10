package dailyritual

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
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
		       display_order,
		       is_active,
		       created_at,
		       updated_at
		FROM daily_ritual_templates
		WHERE is_active = TRUE
		ORDER BY display_order ASC, title ASC
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

func (r *Repository) ListSpaceSelections(ctx context.Context, userID, spaceID string) ([]SpaceSelection, error) {
	if err := r.ensureActiveMembership(ctx, r.db, userID, spaceID); err != nil {
		return nil, err
	}
	return r.listActiveSelections(ctx, r.db, spaceID)
}

func (r *Repository) ReplaceSpaceSelections(ctx context.Context, userID, spaceID string, templateIDs []string) ([]SpaceSelection, error) {
	normalized, err := normalizeTemplateIDs(templateIDs)
	if err != nil {
		return nil, err
	}
	if len(normalized) > MaxActiveSelections {
		return nil, ErrSelectionLimitExceeded
	}

	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := r.ensureActiveMembership(ctx, tx, userID, spaceID); err != nil {
		return nil, err
	}
	for _, templateID := range normalized {
		ok, err := r.activeTemplateExists(ctx, tx, templateID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrTemplateNotFound
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE relationship_space_daily_ritual_selections
		SET is_active = FALSE
		WHERE relationship_space_id = $1
		  AND is_active = TRUE
	`, spaceID); err != nil {
		return nil, err
	}

	for index, templateID := range normalized {
		if _, err := tx.Exec(ctx, `
			INSERT INTO relationship_space_daily_ritual_selections (
				relationship_space_id,
				template_id,
				selected_by_user_id,
				sort_order,
				is_active,
				selected_at
			)
			VALUES ($1, $2, $3, $4, TRUE, now())
			ON CONFLICT (relationship_space_id, template_id)
			DO UPDATE SET
				selected_by_user_id = EXCLUDED.selected_by_user_id,
				sort_order = EXCLUDED.sort_order,
				is_active = TRUE,
				selected_at = EXCLUDED.selected_at
		`, spaceID, templateID, userID, index); err != nil {
			return nil, err
		}
	}

	items, err := r.listActiveSelections(ctx, tx, spaceID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *Repository) ensureActiveMembership(ctx context.Context, q dbtx, userID, spaceID string) error {
	var existingSpaceID string
	err := q.QueryRow(ctx, `
		SELECT rs.id
		FROM relationship_spaces rs
		JOIN relationship_space_members rsm
		  ON rsm.relationship_space_id = rs.id
		WHERE rs.id = $1
		  AND rsm.user_id = $2
		  AND rsm.membership_status = 'active'
		LIMIT 1
	`, spaceID, userID).Scan(&existingSpaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRelationshipSpaceNotFound
		}
		return err
	}
	return nil
}

func (r *Repository) activeTemplateExists(ctx context.Context, q dbtx, templateID string) (bool, error) {
	var existingID string
	err := q.QueryRow(ctx, `
		SELECT id
		FROM daily_ritual_templates
		WHERE id = $1
		  AND is_active = TRUE
		LIMIT 1
	`, templateID).Scan(&existingID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *Repository) listActiveSelections(ctx context.Context, q dbtx, spaceID string) ([]SpaceSelection, error) {
	rows, err := q.Query(ctx, `
		SELECT rsdrs.id,
		       rsdrs.relationship_space_id,
		       rsdrs.template_id,
		       rsdrs.selected_by_user_id,
		       rsdrs.sort_order,
		       rsdrs.selected_at,
		       drt.slug,
		       drt.title,
		       drt.description,
		       drt.category,
		       drt.icon_key,
		       drt.suggested_time,
		       drt.default_points
		FROM relationship_space_daily_ritual_selections rsdrs
		JOIN daily_ritual_templates drt
		  ON drt.id = rsdrs.template_id
		WHERE rsdrs.relationship_space_id = $1
		  AND rsdrs.is_active = TRUE
		  AND drt.is_active = TRUE
		ORDER BY rsdrs.sort_order ASC, rsdrs.selected_at ASC
	`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]SpaceSelection, 0)
	for rows.Next() {
		var item SpaceSelection
		if err := rows.Scan(
			&item.SelectionID,
			&item.RelationshipSpaceID,
			&item.TemplateID,
			&item.SelectedByUserID,
			&item.SortOrder,
			&item.SelectedAt,
			&item.TemplateSlug,
			&item.TemplateTitle,
			&item.TemplateDescription,
			&item.TemplateCategory,
			&item.TemplateIconKey,
			&item.TemplateSuggestedTime,
			&item.TemplateDefaultPoints,
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

func normalizeTemplateIDs(templateIDs []string) ([]string, error) {
	if len(templateIDs) == 0 {
		return []string{}, nil
	}

	items := make([]string, 0, len(templateIDs))
	seen := make(map[string]struct{}, len(templateIDs))
	for _, raw := range templateIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, err := uuid.Parse(id); err != nil {
			return nil, ErrInvalidTemplateID
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		items = append(items, id)
	}
	return items, nil
}
