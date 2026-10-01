package passwordreset

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type resetTokenRow struct {
	ID        string     `db:"id"`
	UserID    string     `db:"user_id"`
	TokenHash string     `db:"token_hash"`
	Added     time.Time  `db:"added"`
	ExpiresAt time.Time  `db:"expires_at"`
	UsedAt    *time.Time `db:"used_at"`
}

func (r resetTokenRow) toDomain() ResetToken {
	return ResetToken(r)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t ResetToken) (ResetToken, error) {
	query := `
		INSERT INTO tamiyo.password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, added, expires_at, used_at
	`

	var created resetTokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.TokenHash, t.ExpiresAt); err != nil {
		return ResetToken{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) FindByHash(ctx context.Context, tokenHash string) (ResetToken, error) {
	query := `
		SELECT id, user_id, token_hash, added, expires_at, used_at
		FROM tamiyo.password_reset_tokens
		WHERE token_hash = $1
	`

	var row resetTokenRow
	if err := r.db.GetContext(ctx, &row, query, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ResetToken{}, ErrNotFound
		}
		return ResetToken{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tamiyo.password_reset_tokens SET used_at = now() WHERE id = $1`, id)
	return err
}
