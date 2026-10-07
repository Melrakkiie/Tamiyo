package emailchange

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type changeTokenRow struct {
	ID        string     `db:"id"`
	UserID    string     `db:"user_id"`
	NewEmail  string     `db:"new_email"`
	TokenHash string     `db:"token_hash"`
	Added     time.Time  `db:"added"`
	ExpiresAt time.Time  `db:"expires_at"`
	UsedAt    *time.Time `db:"used_at"`
}

func (r changeTokenRow) toDomain() ChangeToken {
	return ChangeToken(r)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t ChangeToken) (ChangeToken, error) {
	query := `
		INSERT INTO tamiyo.email_change_tokens (user_id, new_email, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, new_email, token_hash, added, expires_at, used_at
	`

	var created changeTokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.NewEmail, t.TokenHash, t.ExpiresAt); err != nil {
		return ChangeToken{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) FindByHash(ctx context.Context, tokenHash string) (ChangeToken, error) {
	query := `
		SELECT id, user_id, new_email, token_hash, added, expires_at, used_at
		FROM tamiyo.email_change_tokens
		WHERE token_hash = $1
	`

	var row changeTokenRow
	if err := r.db.GetContext(ctx, &row, query, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ChangeToken{}, ErrNotFound
		}
		return ChangeToken{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) MarkUsed(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tamiyo.email_change_tokens SET used_at = now() WHERE id = $1`, id)
	return err
}
