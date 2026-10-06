package token

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

type refreshTokenRow struct {
	ID         string     `db:"id"`
	UserID     string     `db:"user_id"`
	TokenHash  string     `db:"token_hash"`
	Added      time.Time  `db:"added"`
	ExpiresAt  time.Time  `db:"expires_at"`
	RevokedAt  *time.Time `db:"revoked_at"`
	ReplacedBy *string    `db:"replaced_by"`
}

func (r refreshTokenRow) toDomain() RefreshToken {
	return RefreshToken(r)
}

type PostgresRepository struct {
	db *sqlx.DB
}

func NewPostgresRepository(db *sqlx.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, t RefreshToken) (RefreshToken, error) {
	query := `
		INSERT INTO tamiyo.refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, added, expires_at, revoked_at, replaced_by
	`

	var created refreshTokenRow
	if err := r.db.GetContext(ctx, &created, query, t.UserID, t.TokenHash, t.ExpiresAt); err != nil {
		return RefreshToken{}, err
	}

	return created.toDomain(), nil
}

func (r *PostgresRepository) FindByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, added, expires_at, revoked_at, replaced_by
		FROM tamiyo.refresh_tokens
		WHERE token_hash = $1
	`

	var row refreshTokenRow
	if err := r.db.GetContext(ctx, &row, query, tokenHash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RefreshToken{}, ErrNotFound
		}
		return RefreshToken{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) FindByID(ctx context.Context, id string) (RefreshToken, error) {
	query := `
		SELECT id, user_id, token_hash, added, expires_at, revoked_at, replaced_by
		FROM tamiyo.refresh_tokens
		WHERE id = $1
	`

	var row refreshTokenRow
	if err := r.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RefreshToken{}, ErrNotFound
		}
		return RefreshToken{}, err
	}

	return row.toDomain(), nil
}

func (r *PostgresRepository) Revoke(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE tamiyo.refresh_tokens SET revoked_at = now() WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) Replace(ctx context.Context, id string, successorID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamiyo.refresh_tokens
		SET revoked_at = COALESCE(revoked_at, now()),
		    replaced_by = COALESCE(replaced_by, $2)
		WHERE id = $1
	`, id, successorID)
	return err
}

func (r *PostgresRepository) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE tamiyo.refresh_tokens
		SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}
