package follow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) UserExists(ctx context.Context, userID string) (bool, error) {
	var exists bool
	err := r.db.GetContext(ctx, &exists, `SELECT EXISTS (SELECT 1 FROM tamiyo.users WHERE id = $1)`, userID)
	return exists, err
}

func (r *PostgresRepository) Follow(ctx context.Context, followerID string, followedID string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tamiyo.user_follows (follower_id, followed_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, followerID, followedID)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23503" {
		return ErrUserNotFound
	}
	return err
}

func (r *PostgresRepository) Unfollow(ctx context.Context, followerID string, followedID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM tamiyo.user_follows WHERE follower_id = $1 AND followed_id = $2`, followerID, followedID)
	return err
}

func (r *PostgresRepository) Status(ctx context.Context, viewerID string, userID string) (Status, error) {
	var row struct {
		Followers    int  `db:"followers"`
		Following    int  `db:"following"`
		FollowedByMe bool `db:"followed_by_me"`
		FollowsMe    bool `db:"follows_me"`
	}
	err := r.db.GetContext(ctx, &row, `
		SELECT
			(SELECT COUNT(*) FROM tamiyo.user_follows WHERE followed_id = $2) AS followers,
			(SELECT COUNT(*) FROM tamiyo.user_follows WHERE follower_id = $2) AS following,
			EXISTS (SELECT 1 FROM tamiyo.user_follows WHERE follower_id = $1 AND followed_id = $2) AS followed_by_me,
			EXISTS (SELECT 1 FROM tamiyo.user_follows WHERE follower_id = $2 AND followed_id = $1) AS follows_me
	`, viewerID, userID)
	if err != nil {
		return Status{}, fmt.Errorf("loading follow status: %w", err)
	}
	return Status(row), nil
}

type connectionRow struct {
	ID               string    `db:"id"`
	DisplayName      *string   `db:"display_name"`
	AvatarScryfallID *string   `db:"avatar_scryfall_id"`
	Since            time.Time `db:"since"`
	FollowedByMe     bool      `db:"followed_by_me"`
	Total            int       `db:"total"`
}

func (r *PostgresRepository) connections(ctx context.Context, viewerID string, userID string, page Page, listedColumn string, ownerColumn string) ([]Connection, int, error) {
	query := fmt.Sprintf(`
		SELECT
			u.id,
			u.display_name,
			u.avatar_scryfall_id,
			f.added AS since,
			EXISTS (SELECT 1 FROM tamiyo.user_follows mine WHERE mine.follower_id = $1 AND mine.followed_id = u.id) AS followed_by_me,
			COUNT(*) OVER () AS total
		FROM tamiyo.user_follows f
		JOIN tamiyo.users u ON u.id = f.%s
		WHERE f.%s = $2
		ORDER BY f.added DESC, u.id
		LIMIT $3 OFFSET $4
	`, listedColumn, ownerColumn)
	var rows []connectionRow
	if err := r.db.SelectContext(ctx, &rows, query, viewerID, userID, page.Limit, (page.Number-1)*page.Limit); err != nil {
		return nil, 0, fmt.Errorf("loading connections: %w", err)
	}

	total := 0
	connections := make([]Connection, 0, len(rows))
	for _, row := range rows {
		total = row.Total
		connections = append(connections, Connection{
			ID:               row.ID,
			DisplayName:      row.DisplayName,
			AvatarScryfallID: row.AvatarScryfallID,
			Since:            row.Since,
			FollowedByMe:     row.FollowedByMe,
		})
	}
	if len(rows) == 0 && page.Number > 1 {
		if err := r.db.GetContext(ctx, &total, fmt.Sprintf(`SELECT COUNT(*) FROM tamiyo.user_follows WHERE %s = $1`, ownerColumn), userID); err != nil {
			return nil, 0, fmt.Errorf("counting connections: %w", err)
		}
	}
	return connections, total, nil
}

func (r *PostgresRepository) Followers(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	return r.connections(ctx, viewerID, userID, page, "follower_id", "followed_id")
}

func (r *PostgresRepository) Following(ctx context.Context, viewerID string, userID string, page Page) ([]Connection, int, error) {
	return r.connections(ctx, viewerID, userID, page, "followed_id", "follower_id")
}
